// Command vmloadgen drives VictoriaMetrics with the Prometheus remote write
// protocol at a target sample rate. It is the load generator behind Story 3.4;
// it is not a product binary.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang/snappy"

	"github.com/observaki/observaki/bench/internal/benchversion"
	"github.com/observaki/observaki/bench/internal/vmbench"
	"github.com/observaki/observaki/internal/promrw"
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

type options struct {
	vmURL           string
	tenants         string
	series          int
	rate            int
	duration        time.Duration
	workers         int
	batchSeries     int
	mode            string
	metric          string
	probeTenant     string
	reportPath      string
	timeout         time.Duration
	allowRejections bool
	version         bool
}

func run(args []string, stdout, stderr io.Writer) int {
	opts := options{}
	flags := flag.NewFlagSet("vmloadgen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.vmURL, "vm-url", "http://127.0.0.1:18428", "VictoriaMetrics base URL")
	flags.StringVar(&opts.tenants, "tenants", "tenant-a,tenant-b", "comma-separated tenants (load mode)")
	flags.IntVar(&opts.series, "series", 1_000_000, "total active series")
	flags.IntVar(&opts.rate, "rate", 1_000_000, "target samples/s")
	flags.DurationVar(&opts.duration, "duration", time.Hour, "load duration")
	flags.IntVar(&opts.workers, "workers", 16, "concurrent sender workers")
	flags.IntVar(&opts.batchSeries, "batch-series", 5_000, "series per remote write request")
	flags.StringVar(&opts.mode, "mode", "load", "load or probe")
	flags.StringVar(&opts.metric, "metric", "observaki_cardinality_probe_total", "probe metric name")
	flags.StringVar(&opts.probeTenant, "probe-tenant", "tenant-a", "probe tenant")
	flags.StringVar(&opts.reportPath, "report", "", "report JSON path")
	flags.DurationVar(&opts.timeout, "timeout", 60*time.Second, "per-request timeout")
	flags.BoolVar(&opts.allowRejections, "allow-rejections", false, "treat 429 responses as expected (cardinality test)")
	flags.BoolVar(&opts.version, "version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if opts.version {
		_, _ = fmt.Fprintf(stdout, "vmloadgen %s (%s)\n", benchversion.Version, benchversion.Commit)
		return 0
	}
	if opts.rate <= 0 || opts.series <= 0 || opts.workers <= 0 || opts.batchSeries <= 0 {
		_, _ = fmt.Fprintln(stderr, "vmloadgen: rate, series, workers and batch-series must be positive")
		return 2
	}
	if opts.duration <= 0 {
		_, _ = fmt.Fprintln(stderr, "vmloadgen: duration must be positive")
		return 2
	}

	catalog, err := buildCatalog(opts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vmloadgen: %v\n", err)
		return 2
	}
	if opts.workers > len(catalog.Series) {
		opts.workers = len(catalog.Series)
	}
	if opts.workers == 0 {
		_, _ = fmt.Fprintln(stderr, "vmloadgen: catalog has no series")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	load := &loadRun{
		opts:    opts,
		catalog: catalog,
		client: &http.Client{
			Timeout: opts.timeout,
			Transport: &http.Transport{
				MaxIdleConns:        opts.workers * 2,
				MaxIdleConnsPerHost: opts.workers * 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		start: time.Now(),
	}
	load.logger = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	load.run(ctx)

	report := load.report(ctx.Err() != nil)
	if opts.reportPath != "" {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "vmloadgen: encode report: %v\n", err)
			return 1
		}
		if err := os.WriteFile(opts.reportPath, append(encoded, '\n'), 0o644); err != nil {
			_, _ = fmt.Fprintf(stderr, "vmloadgen: write report: %v\n", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "vmloadgen: %s mode=%s series=%d rate=%.0f/s sent=%d achieved=%.0f/s errors=%d rejected=%d p95=%.1fms\n",
		report.Status, report.Mode, report.Series, report.TargetRate, report.SentSamples, report.AchievedRate, report.Errors, report.Rejected, report.Latency.P95MS)
	if report.Status == "pass" {
		return 0
	}
	return 1
}

func buildCatalog(opts options) (*vmbench.Catalog, error) {
	switch opts.mode {
	case "load":
		tenants := vmbench.ParseTenants(opts.tenants)
		if len(tenants) == 0 {
			return nil, fmt.Errorf("at least one tenant is required")
		}
		perTenant := opts.series / len(tenants)
		if perTenant == 0 {
			return nil, fmt.Errorf("series must be at least the number of tenants")
		}
		return vmbench.NewCatalog(vmbench.CatalogOptions{Tenants: tenants, SeriesPerTenant: perTenant})
	case "probe":
		return vmbench.NewProbeCatalog(opts.probeTenant, opts.metric, opts.series)
	default:
		return nil, fmt.Errorf("unknown mode %q", opts.mode)
	}
}

type loadRun struct {
	opts    options
	catalog *vmbench.Catalog
	client  *http.Client
	start   time.Time
	logger  *slog.Logger

	sent     atomic.Int64
	requests atomic.Int64
	errors   atomic.Int64
	rejected atomic.Int64
	bytes    atomic.Int64

	mu       sync.Mutex
	latency  []float64
	firstErr string
}

func (l *loadRun) run(ctx context.Context) {
	l.logger.Info("starting load",
		"mode", l.opts.mode,
		"series", len(l.catalog.Series),
		"rate", l.opts.rate,
		"duration", l.opts.duration.String(),
		"workers", l.opts.workers,
		"batch_series", l.opts.batchSeries)

	progressDone := make(chan struct{})
	go l.progress(progressDone)

	var wg sync.WaitGroup
	for id := 0; id < l.opts.workers; id++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			l.worker(ctx, worker)
		}(id)
	}
	wg.Wait()
	close(progressDone)

	elapsed := time.Since(l.start).Seconds()
	l.logger.Info("load finished",
		"sent", l.sent.Load(),
		"achieved", float64(l.sent.Load())/math.Max(elapsed, 0.001),
		"errors", l.errors.Load(),
		"rejected", l.rejected.Load())
}

func (l *loadRun) progress(done <-chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			elapsed := time.Since(l.start).Seconds()
			sent := l.sent.Load()
			l.logger.Info("progress",
				"sent", sent,
				"rate", float64(sent)/math.Max(elapsed, 0.001),
				"errors", l.errors.Load(),
				"rejected", l.rejected.Load())
		}
	}
}

func (l *loadRun) worker(ctx context.Context, worker int) {
	seriesCount := len(l.catalog.Series)
	shardStart := worker * seriesCount / l.opts.workers
	shardEnd := (worker + 1) * seriesCount / l.opts.workers
	shardSize := shardEnd - shardStart
	if shardSize == 0 {
		return
	}

	totalSamples := int64(float64(l.opts.rate) * l.opts.duration.Seconds())
	base := totalSamples / int64(l.opts.workers)
	remainder := totalSamples % int64(l.opts.workers)
	workerTotal := base
	if int64(worker) < remainder {
		workerTotal++
	}
	workerRate := float64(l.opts.rate) / float64(l.opts.workers)

	cursor := shardStart
	requestBuf := make([]byte, 0, 1<<20)
	compressed := make([]byte, 0, 1<<20)
	var sent int64
	var latency []float64

	for sent < workerTotal {
		if ctx.Err() != nil {
			break
		}
		if time.Since(l.start) >= l.opts.duration {
			break
		}
		n := l.opts.batchSeries
		if n > shardSize {
			n = shardSize
		}
		if remaining := workerTotal - sent; int64(n) > remaining {
			n = int(remaining)
		}
		target := int64(time.Since(l.start).Seconds() * workerRate)
		if sent+int64(n) > target {
			wait := time.Duration(float64(sent+int64(n)-target) / workerRate * float64(time.Second))
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}

		now := time.Now()
		requestBuf = requestBuf[:0]
		for i := 0; i < n; i++ {
			series := &l.catalog.Series[cursor]
			value := l.catalog.SampleAt(series, l.start, now)
			requestBuf = promrw.AppendTimeSeriesField(requestBuf, series.LabelBytes, value, now.UnixMilli())
			cursor++
			if cursor >= shardEnd {
				cursor = shardStart
			}
		}
		compressed = snappy.Encode(compressed[:0], requestBuf)

		start := time.Now()
		status, err := l.send(ctx, compressed)
		elapsed := float64(time.Since(start)) / float64(time.Millisecond)
		latency = append(latency, elapsed)
		l.requests.Add(1)
		l.bytes.Add(int64(len(compressed)))
		if err != nil {
			if status == http.StatusTooManyRequests {
				l.rejected.Add(int64(n))
			} else {
				l.errors.Add(int64(n))
				l.recordError(err)
			}
		}
		sent += int64(n)
		l.sent.Add(int64(n))
	}

	l.mu.Lock()
	l.latency = append(l.latency, latency...)
	l.mu.Unlock()
}

func (l *loadRun) send(ctx context.Context, body []byte) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, l.opts.vmURL+"/api/v1/write", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Content-Encoding", "snappy")
	request.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	response, err := l.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusOK {
		return response.StatusCode, nil
	}
	return response.StatusCode, fmt.Errorf("remote write returned %d", response.StatusCode)
}

func (l *loadRun) recordError(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.firstErr == "" {
		l.firstErr = err.Error()
	}
}

type latencySummary struct {
	Count int     `json:"count"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
}

type report struct {
	Mode            string         `json:"mode"`
	Status          string         `json:"status"`
	TargetRate      float64        `json:"target_rate"`
	DurationSeconds float64        `json:"duration_seconds"`
	Series          int            `json:"series"`
	Tenants         []string       `json:"tenants"`
	Workers         int            `json:"workers"`
	BatchSeries     int            `json:"batch_series"`
	StartedAt       string         `json:"started_at"`
	FinishedAt      string         `json:"finished_at"`
	ElapsedSeconds  float64        `json:"elapsed_seconds"`
	SentSamples     int64          `json:"sent_samples"`
	AchievedRate    float64        `json:"achieved_rate"`
	Requests        int64          `json:"requests"`
	CompressedBytes int64          `json:"compressed_bytes"`
	Errors          int64          `json:"errors"`
	Rejected        int64          `json:"rejected_samples"`
	Latency         latencySummary `json:"latency_ms"`
	AllowRejections bool           `json:"allow_rejections,omitempty"`
	FirstError      string         `json:"first_error,omitempty"`
}

func (l *loadRun) report(interrupted bool) report {
	elapsed := time.Since(l.start).Seconds()
	sent := l.sent.Load()
	achieved := float64(sent) / math.Max(elapsed, 0.001)
	status := "pass"
	if interrupted {
		status = "interrupted"
	} else if achieved < float64(l.opts.rate)*0.95 || l.errors.Load() != 0 || (l.rejected.Load() != 0 && !l.opts.allowRejections) {
		status = "fail"
	}
	if !interrupted && elapsed < l.opts.duration.Seconds()*0.95 {
		status = "fail"
	}

	l.mu.Lock()
	latencies := append([]float64(nil), l.latency...)
	firstErr := l.firstErr
	l.mu.Unlock()
	sort.Float64s(latencies)

	return report{
		Mode:            l.opts.mode,
		Status:          status,
		TargetRate:      float64(l.opts.rate),
		DurationSeconds: l.opts.duration.Seconds(),
		Series:          len(l.catalog.Series),
		Tenants:         l.catalog.SortedTenants(),
		Workers:         l.opts.workers,
		BatchSeries:     l.opts.batchSeries,
		StartedAt:       l.start.UTC().Format(time.RFC3339Nano),
		FinishedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		ElapsedSeconds:  elapsed,
		SentSamples:     sent,
		AchievedRate:    achieved,
		Requests:        l.requests.Load(),
		CompressedBytes: l.bytes.Load(),
		Errors:          l.errors.Load(),
		Rejected:        l.rejected.Load(),
		AllowRejections: l.opts.allowRejections,
		Latency: latencySummary{
			Count: len(latencies),
			P50MS: vmbench.Percentile(latencies, 50),
			P95MS: vmbench.Percentile(latencies, 95),
			P99MS: vmbench.Percentile(latencies, 99),
			MaxMS: vmbench.Percentile(latencies, 100),
		},
		FirstError: firstErr,
	}
}
