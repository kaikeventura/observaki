// Package victoriametrics implements the Prometheus remote write adapter used
// by the WAL writer. Metrics and RED live in VictoriaMetrics (AD-5, AD-21); the
// adapter is deliberately signal-specific and never handles logs or traces.
package victoriametrics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang/snappy"
	"github.com/observaki/observaki/internal/promrw"
)

// Defaults keep a local single-node useful without configuration.
const (
	defaultTimeout             = 30 * time.Second
	defaultRetryBackoff        = 500 * time.Millisecond
	defaultMaxRetryBackoff     = 30 * time.Second
	defaultMaxSeriesPerRequest = 5_000
)

// Config configures the remote write client.
type Config struct {
	// URL is the VictoriaMetrics base URL (for example http://127.0.0.1:8428).
	// The adapter appends /api/v1/write.
	URL string
	// Timeout bounds one HTTP request attempt.
	Timeout time.Duration
	// MaxRetries is the maximum number of retries after the first attempt. A
	// value of zero means retry until the context is canceled (the WAL
	// consumer holds the messages unacked, so no data is lost within the WAL
	// window).
	MaxRetries int
	// RetryBackoff is the base exponential backoff.
	RetryBackoff time.Duration
	// MaxRetryBackoff caps the exponential backoff.
	MaxRetryBackoff time.Duration
	// MaxSeriesPerRequest splits large batches into remote write requests.
	MaxSeriesPerRequest int
	Client              *http.Client
	Logger              *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = defaultRetryBackoff
	}
	if c.MaxRetryBackoff <= 0 {
		c.MaxRetryBackoff = defaultMaxRetryBackoff
	}
	if c.MaxSeriesPerRequest <= 0 {
		c.MaxSeriesPerRequest = defaultMaxSeriesPerRequest
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.DiscardHandler)
	}
	return c
}

func (c Config) validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("victoriametrics: URL is empty")
	}
	parsed, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("victoriametrics: parse URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("victoriametrics: URL scheme %q must be http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("victoriametrics: URL has no host")
	}
	if c.MaxRetries < 0 {
		return errors.New("victoriametrics: max retries cannot be negative")
	}
	if c.MaxSeriesPerRequest < 1 {
		return errors.New("victoriametrics: max series per request must be positive")
	}
	return nil
}

// Client writes snappy-compressed Prometheus remote write requests.
type Client struct {
	cfg      Config
	endpoint string
	client   *http.Client
}

// Result reports one Write call.
type Result struct {
	Requests int `json:"requests"`
	Series   int `json:"series"`
	Samples  int `json:"samples"`
	Retries  int `json:"retries"`
}

// Open validates the configuration and returns a client. It does not connect;
// a VictoriaMetrics outage is handled by retry, which is what keeps the WAL
// messages unacked.
func Open(cfg Config) (*Client, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	client := cfg.Client
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 16
		transport.MaxIdleConnsPerHost = 8
		client = &http.Client{Timeout: cfg.Timeout, Transport: transport}
	}
	return &Client{
		cfg:      cfg,
		endpoint: strings.TrimRight(cfg.URL, "/") + "/api/v1/write",
		client:   client,
	}, nil
}

// Write sends series to VictoriaMetrics in bounded requests. A transient error
// is retried with exponential backoff; a permanent 4xx error is returned
// immediately so the caller can move the batch to the DLQ.
func (c *Client) Write(ctx context.Context, series []promrw.TimeSeries) (Result, error) {
	if len(series) == 0 {
		return Result{}, nil
	}
	var result Result
	for start := 0; start < len(series); start += c.cfg.MaxSeriesPerRequest {
		end := start + c.cfg.MaxSeriesPerRequest
		if end > len(series) {
			end = len(series)
		}
		chunk := series[start:end]
		retries, err := c.writeChunk(ctx, chunk)
		result.Requests++
		result.Retries += retries
		result.Series += len(chunk)
		for index := range chunk {
			result.Samples += len(chunk[index].Samples)
		}
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (c *Client) writeChunk(ctx context.Context, series []promrw.TimeSeries) (int, error) {
	raw, err := promrw.Marshal(&promrw.WriteRequest{Timeseries: series})
	if err != nil {
		return 0, &WriteError{Message: err.Error(), Permanent: true}
	}
	body := snappy.Encode(nil, raw)

	retries := 0
	for attempt := 1; ; attempt++ {
		err = c.post(ctx, body)
		if err == nil {
			return retries, nil
		}
		if ctx.Err() != nil {
			return retries, ctx.Err()
		}
		if IsPermanent(err) {
			return retries, err
		}
		if c.cfg.MaxRetries > 0 && attempt > c.cfg.MaxRetries {
			return retries, fmt.Errorf("victoriametrics: remote write failed after %d attempts: %w", attempt, err)
		}
		retries++
		delay := retryDelay(c.cfg.RetryBackoff, c.cfg.MaxRetryBackoff, attempt)
		c.cfg.Logger.Warn(
			"victoriametrics remote write failed; retrying",
			"attempt", attempt,
			"delay", delay,
			"error", err,
		)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return retries, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) post(ctx context.Context, body []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return &WriteError{Message: err.Error(), Permanent: true}
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Content-Encoding", "snappy")
	request.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	response, err := c.client.Do(request)
	if err != nil {
		return &WriteError{Message: err.Error()}
	}
	defer func() { _ = response.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	return classifyWriteError(response.StatusCode, payload)
}

// WriteError classifies a failed remote write.
type WriteError struct {
	StatusCode int
	Message    string
	Permanent  bool
}

func (e *WriteError) Error() string {
	if e.StatusCode == 0 {
		return fmt.Sprintf("victoriametrics remote write: %s", e.Message)
	}
	return fmt.Sprintf("victoriametrics remote write: status %d: %s", e.StatusCode, e.Message)
}

// IsPermanent reports whether err is a non-retryable rejection.
func IsPermanent(err error) bool {
	var writeErr *WriteError
	return errors.As(err, &writeErr) && writeErr.Permanent
}

func classifyWriteError(status int, payload []byte) error {
	message := strings.TrimSpace(string(payload))
	if message == "" {
		message = http.StatusText(status)
	}
	permanent := status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
	return &WriteError{StatusCode: status, Message: truncate(message, 2048), Permanent: permanent}
}

func retryDelay(base, maximum time.Duration, attempt int) time.Duration {
	if attempt > 10 {
		attempt = 10
	}
	delay := base * time.Duration(1<<(attempt-1))
	if delay > maximum {
		return maximum
	}
	return delay
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// Close releases idle HTTP connections.
func (c *Client) Close() {
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}
