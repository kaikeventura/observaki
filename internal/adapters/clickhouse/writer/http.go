package writer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// InsertRequest is one batch written with FORMAT RowBinary.
type InsertRequest struct {
	Table       string
	Data        []byte
	Rows        int
	DedupeToken string
}

// InsertResult is ClickHouse's confirmation for one insert. WrittenRows is
// the row count reported by the HTTP summary; ClickHouse reports the processed
// block size even when block deduplication drops it, so the authoritative
// stored-row count is measured with a query (the benchmark does this).
type InsertResult struct {
	WrittenRows  uint64
	WrittenBytes uint64
}

// Inserter is the storage boundary used by the writer. It is an interface so
// the replay, retry and dedupe logic can be tested without ClickHouse.
type Inserter interface {
	Insert(ctx context.Context, request InsertRequest) (InsertResult, error)
}

// InsertError classifies a failed insert. Permanent errors are moved to the
// DLQ; transient errors are retried with backoff.
type InsertError struct {
	StatusCode int
	Code       string
	Message    string
	Permanent  bool
}

func (e *InsertError) Error() string {
	if e.StatusCode == 0 {
		return fmt.Sprintf("clickhouse insert: %s", e.Message)
	}
	return fmt.Sprintf("clickhouse insert: status %d code %s: %s", e.StatusCode, e.Code, e.Message)
}

// IsPermanent reports whether err is a non-retryable rejection.
func IsPermanent(err error) bool {
	var insertErr *InsertError
	return errors.As(err, &insertErr) && insertErr.Permanent
}

// HTTPConfig configures the ClickHouse HTTP inserter. The adapter uses the
// HTTP interface so the data plane has no native-protocol dependency.
type HTTPConfig struct {
	Endpoint      string
	Database      string
	User          string
	Password      string
	Compression   string
	Timeout       time.Duration
	Client        *http.Client
	SyncInsert    bool
	InsertThreads int
}

// HTTPInserter writes RowBinary batches to ClickHouse over HTTP.
type HTTPInserter struct {
	endpoint      *url.URL
	database      string
	user          string
	password      string
	compression   string
	client        *http.Client
	syncInsert    bool
	insertThreads int
}

// NewHTTPInserter validates cfg and returns an HTTP inserter.
func NewHTTPInserter(cfg HTTPConfig) (*HTTPInserter, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("clickhouse endpoint is empty")
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse clickhouse endpoint: %w", err)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("clickhouse endpoint scheme = %q, want http or https", endpoint.Scheme)
	}
	if endpoint.Host == "" {
		return nil, errors.New("clickhouse endpoint has no host")
	}
	if cfg.Database == "" {
		return nil, errors.New("clickhouse database is empty")
	}
	compression := strings.ToLower(cfg.Compression)
	if compression == "" {
		compression = "none"
	}
	if compression != "none" && compression != "gzip" {
		return nil, fmt.Errorf("clickhouse compression = %q, want none or gzip", cfg.Compression)
	}
	insertThreads := cfg.InsertThreads
	if insertThreads <= 0 {
		insertThreads = 1
	}
	if insertThreads > 16 {
		return nil, fmt.Errorf("clickhouse insert threads = %d, want <= 16", insertThreads)
	}
	client := cfg.Client
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 8
		transport.MaxIdleConnsPerHost = 4
		transport.DisableCompression = true
		client = &http.Client{Timeout: timeout, Transport: transport}
	}
	return &HTTPInserter{
		endpoint:      endpoint,
		database:      cfg.Database,
		user:          cfg.User,
		password:      cfg.Password,
		compression:   compression,
		client:        client,
		syncInsert:    cfg.SyncInsert,
		insertThreads: insertThreads,
	}, nil
}

// Insert sends one batch and waits for ClickHouse's confirmation. The
// insert_deduplication_token is stable for the lifetime of the spooled batch.
func (h *HTTPInserter) Insert(ctx context.Context, request InsertRequest) (InsertResult, error) {
	if request.Rows <= 0 {
		return InsertResult{}, errors.New("insert request has no rows")
	}
	if request.DedupeToken == "" {
		return InsertResult{}, errors.New("insert request has no deduplication token")
	}
	if err := validateTable(request.Table); err != nil {
		return InsertResult{}, err
	}

	query := url.Values{}
	query.Set("database", h.database)
	query.Set("query", "INSERT INTO "+request.Table+" FORMAT RowBinary")
	if h.syncInsert {
		query.Set("async_insert", "0")
	} else {
		query.Set("async_insert", "1")
		query.Set("wait_for_async_insert", "1")
	}
	query.Set("insert_deduplicate", "1")
	query.Set("insert_deduplication_token", request.DedupeToken)
	query.Set("max_insert_threads", strconv.Itoa(h.insertThreads))
	if h.user != "" {
		query.Set("user", h.user)
	}
	if h.password != "" {
		query.Set("password", h.password)
	}

	target := *h.endpoint
	target.RawQuery = query.Encode()

	body := request.Data
	encoding := ""
	if h.compression == "gzip" {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(request.Data); err != nil {
			_ = writer.Close()
			return InsertResult{}, fmt.Errorf("compress clickhouse batch: %w", err)
		}
		if err := writer.Close(); err != nil {
			return InsertResult{}, fmt.Errorf("close clickhouse batch compressor: %w", err)
		}
		body = compressed.Bytes()
		encoding = "gzip"
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return InsertResult{}, fmt.Errorf("build clickhouse request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/octet-stream")
	if encoding != "" {
		httpRequest.Header.Set("Content-Encoding", encoding)
	}

	response, err := h.client.Do(httpRequest)
	if err != nil {
		return InsertResult{}, &InsertError{Message: err.Error(), Permanent: false}
	}
	defer func() { _ = response.Body.Close() }()

	payload, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return InsertResult{}, classifyInsertError(response.StatusCode, payload)
	}

	result := InsertResult{WrittenRows: uint64(request.Rows)}
	if summary := response.Header.Get("X-ClickHouse-Summary"); summary != "" {
		if written, ok := summaryUint(summary, "written_rows"); ok {
			result.WrittenRows = written
		}
		if written, ok := summaryUint(summary, "written_bytes"); ok {
			result.WrittenBytes = written
		}
	}
	return result, nil
}

func validateTable(table string) error {
	if table == "" {
		return errors.New("clickhouse table is empty")
	}
	for index, char := range table {
		if char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return fmt.Errorf("clickhouse table %q is not a simple identifier", table)
	}
	return nil
}

func classifyInsertError(status int, payload []byte) error {
	message := strings.TrimSpace(string(payload))
	if message == "" {
		message = http.StatusText(status)
	}
	code := ""
	if strings.HasPrefix(message, "Code: ") {
		if end := strings.Index(message, "."); end > len("Code: ") {
			code = strings.TrimSpace(message[len("Code: "):end])
		}
	}
	permanent := status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
	return &InsertError{
		StatusCode: status,
		Code:       code,
		Message:    truncate(message, 2048),
		Permanent:  permanent,
	}
}

func summaryUint(summary, field string) (uint64, bool) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(summary), &values); err != nil {
		return 0, false
	}
	raw, ok := values[field]
	if !ok {
		return 0, false
	}
	text := strings.Trim(string(raw), `"`)
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
