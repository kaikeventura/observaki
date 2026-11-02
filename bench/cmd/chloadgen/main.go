// Command chloadgen drives the ClickHouse ingestion path at a target rate,
// measures end-to-end batch lag and exercises the spool/replay and dedupe
// contracts. It is the harness behind Story 3.3; it is not a product binary.
package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/observaki/observaki/bench/internal/benchversion"
	"github.com/observaki/observaki/internal/adapters/clickhouse/writer"
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

type options struct {
	chEndpoint    string
	chDatabase    string
	chUser        string
	chPassword    string
	compression   string
	syncInsert    bool
	insertThreads int
	spool         string
	table         string
	tenantPrefix  string
	rate          int
	duration      time.Duration
	batchRows     int
	generate      int
	queueDepth    int
	dedupeWindow  int
	maxSkew       time.Duration
	metricsAddr   string
	reportPath    string
	recover       bool
	dedupeTest    bool
	crashAfter    int
	dryRun        bool
	resetSpool    bool
	logLevel      string
	version       bool
}

func run(args []string, stdout, stderr io.Writer) int {
	opts := options{}
	flags := flag.NewFlagSet("chloadgen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.chEndpoint, "ch-endpoint", "http://127.0.0.1:18123", "ClickHouse HTTP endpoint")
	flags.StringVar(&opts.chDatabase, "ch-database", "observaki_bench", "ClickHouse database")
	flags.StringVar(&opts.chUser, "ch-user", "observaki", "ClickHouse user")
	flags.StringVar(&opts.chPassword, "ch-password", os.Getenv("OBSERVAKI_CLICKHOUSE_PASSWORD"), "ClickHouse password (prefer OBSERVAKI_CLICKHOUSE_PASSWORD)")
	flags.StringVar(&opts.compression, "compression", "none", "HTTP compression: none or gzip")
	flags.BoolVar(&opts.syncInsert, "sync-insert", false, "use async_insert=0 instead of the async path")
	flags.IntVar(&opts.insertThreads, "insert-threads", 1, "ClickHouse max_insert_threads")
	flags.StringVar(&opts.spool, "spool", "", "durable spool directory (required)")
	flags.StringVar(&opts.table, "table", "otel_logs", "target table")
	flags.StringVar(&opts.tenantPrefix, "tenant-prefix", "bench", "tenant prefix for generated events")
	flags.IntVar(&opts.rate, "rate", 1_000_000, "target events per second")
	flags.DurationVar(&opts.duration, "duration", 60*time.Second, "generation duration")
	flags.IntVar(&opts.batchRows, "batch-rows", 100_000, "events per batch")
	flags.IntVar(&opts.generate, "generate-workers", min(8, runtime.GOMAXPROCS(0)), "parallel record generators")
	flags.IntVar(&opts.queueDepth, "queue-depth", 4, "writer queue depth")
	flags.IntVar(&opts.dedupeWindow, "dedupe-window", 2_000_000, "writer event_id window")
	flags.DurationVar(&opts.maxSkew, "max-skew", 24*time.Hour, "maximum event-time skew before clamping")
	flags.StringVar(&opts.metricsAddr, "metrics-addr", "", "optional metrics address, e.g. 127.0.0.1:19090")
	flags.StringVar(&opts.reportPath, "report", "", "write the JSON report to this path")
	flags.BoolVar(&opts.recover, "recover", false, "replay the spool and exit")
	flags.BoolVar(&opts.dedupeTest, "dedupe-test", false, "submit one deterministic batch twice and exit")
	flags.IntVar(&opts.crashAfter, "crash-after-insert", 0, "exit after N successful inserts (0 disables)")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "spool and checkpoint without sending to ClickHouse")
	flags.BoolVar(&opts.resetSpool, "reset-spool", false, "remove the spool directory before starting")
	flags.StringVar(&opts.logLevel, "log-level", "info", "log level: debug, info, warn, error")
	flags.BoolVar(&opts.version, "version", false, "print version and exit")
	flags.Usage = func() { usage(stderr) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if opts.version {
		_, _ = fmt.Fprintln(stdout, benchversion.String("chloadgen"))
		return 0
	}
	if opts.spool == "" {
		_, _ = fmt.Fprintln(stderr, "chloadgen: --spool is required")
		return 1
	}
	if opts.rate <= 0 || opts.batchRows <= 0 {
		_, _ = fmt.Fprintln(stderr, "chloadgen: --rate and --batch-rows must be positive")
		return 1
	}
	if opts.duration <= 0 && !opts.recover && !opts.dedupeTest {
		_, _ = fmt.Fprintln(stderr, "chloadgen: --duration must be positive")
		return 1
	}
	if opts.resetSpool {
		if err := os.RemoveAll(opts.spool); err != nil {
			_, _ = fmt.Fprintf(stderr, "chloadgen: reset spool: %v\n", err)
			return 1
		}
	}

	logger := newLogger(opts.logLevel, stderr)
	inserter, err := writer.NewHTTPInserter(writer.HTTPConfig{
		Endpoint:      opts.chEndpoint,
		Database:      opts.chDatabase,
		User:          opts.chUser,
		Password:      opts.chPassword,
		Compression:   opts.compression,
		Timeout:       5 * time.Minute,
		SyncInsert:    opts.syncInsert,
		InsertThreads: opts.insertThreads,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: %v\n", err)
		return 1
	}
	var storage writer.Inserter = inserter
	if opts.dryRun {
		storage = noopInserter{}
	}
	if opts.crashAfter > 0 {
		storage = &crashInserter{inner: storage, after: int64(opts.crashAfter), stderr: stderr}
	}

	switch {
	case opts.recover:
		return runRecovery(opts, storage, logger, stdout, stderr)
	case opts.dedupeTest:
		return runDedupeTest(opts, storage, logger, stdout, stderr)
	default:
		return runLoad(opts, storage, logger, stdout, stderr)
	}
}

func usage(output io.Writer) {
	_, _ = fmt.Fprintf(output, `chloadgen — ClickHouse ingestion harness for Story 3.3.

Usage: chloadgen [flags]

The default mode generates logs at --rate for --duration and writes one
RowBinary batch every --batch-rows events. --recover replays the spool and
exits; --dedupe-test submits the same deterministic batch twice.

`)
}

func runLoad(opts options, inserter writer.Inserter, logger *slog.Logger, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var ackMu sync.Mutex
	var acks []writer.Ack
	cfg := writerConfig(opts, logger, func(ack writer.Ack) {
		ackMu.Lock()
		acks = append(acks, ack)
		ackMu.Unlock()
	})

	started := time.Now().UTC()
	load, err := writer.OpenLogs(ctx, cfg, inserter)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: open writer: %v\n", err)
		return 1
	}
	server := startMetricsServer(opts.metricsAddr, load, logger)
	defer stopMetricsServer(server)

	generator := newGenerator(opts)
	totalBatches := int(math.Ceil(float64(opts.rate) * opts.duration.Seconds() / float64(opts.batchRows)))
	if totalBatches < 1 {
		totalBatches = 1
	}
	batchDuration := time.Duration(float64(opts.batchRows) / float64(opts.rate) * float64(time.Second))

	logger.Info("starting load",
		"rate", opts.rate,
		"batch_rows", opts.batchRows,
		"batches", totalBatches,
		"table", opts.table,
		"tenant_prefix", opts.tenantPrefix,
	)
	// Two buffers let the generator fill batch N+1 while the writer encodes
	// and spools batch N. Submissions stay in batch order, so the spool
	// sequence and the lag window remain monotonic.
	slots := make(chan []writer.LogRecord, 2)
	for index := 0; index < cap(slots); index++ {
		slots <- make([]writer.LogRecord, opts.batchRows)
	}
	results := make([]writer.SubmitResult, totalBatches)
	submitErrors := make([]error, totalBatches)
	var pending chan struct{}
	generationStart := time.Now()
batchLoop:
	for batch := 0; batch < totalBatches; batch++ {
		if ctx.Err() != nil {
			logger.Warn("generation interrupted", "error", ctx.Err())
			break
		}
		var records []writer.LogRecord
		select {
		case records = <-slots:
		case <-ctx.Done():
			break batchLoop
		}
		scheduled := generationStart.Add(time.Duration(batch) * batchDuration)
		if wait := time.Until(scheduled); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				slots <- records
				break batchLoop
			case <-timer.C:
			}
		}
		generator.fillInto(records, batch, scheduled)
		if pending != nil {
			<-pending
			if batch > 0 && submitErrors[batch-1] != nil {
				slots <- records
				break
			}
		}
		pending = make(chan struct{})
		go func(batch int, records []writer.LogRecord, done chan struct{}) {
			defer close(done)
			defer func() { slots <- records }()
			results[batch], submitErrors[batch] = load.SubmitLogs(ctx, records)
		}(batch, records, pending)
		if batch%100 == 0 || batch == totalBatches-1 {
			logger.Info("progress",
				"batch", batch+1,
				"batches", totalBatches,
				"pending_batches", load.Stats().PendingBatches,
				"max_lag_ms", load.Stats().MaxLagMillis,
			)
		}
	}
	if pending != nil {
		<-pending
	}
	for batch := range results {
		if submitErrors[batch] != nil {
			logger.Error("submit failed", "batch", batch, "error", submitErrors[batch])
			_ = closeWriter(load, 30*time.Second)
			return 1
		}
	}
	var submittedBatches int
	var confirmedEvents, duplicateEvents uint64
	for _, result := range results {
		if result.Rows > 0 {
			submittedBatches++
		}
		confirmedEvents += uint64(result.Rows)
		duplicateEvents += uint64(result.Duplicates)
	}

	// Hold the run open for the last batch window so a short run still covers
	// the requested duration and the throughput denominator is honest.
	if ctx.Err() == nil {
		if remaining := time.Until(generationStart.Add(time.Duration(totalBatches) * batchDuration)); remaining > 0 {
			timer := time.NewTimer(remaining)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
		}
	}

	generationEnd := time.Now()
	statsBeforeDrain := load.Stats()
	waitForDrain(ctx, load, logger)
	drainEnd := time.Now()
	closeErr := closeWriter(load, 60*time.Second)
	finished := time.Now().UTC()

	ackMu.Lock()
	collected := append([]writer.Ack(nil), acks...)
	ackMu.Unlock()
	report := buildLoadReport(opts, started, finished, generationStart, generationEnd, drainEnd, submittedBatches, confirmedEvents, duplicateEvents, collected, load.Stats(), statsBeforeDrain)
	if closeErr != nil {
		report.Notes = append(report.Notes, "close: "+closeErr.Error())
	}
	if report.Stats.FatalError != "" {
		report.Notes = append(report.Notes, "writer: "+report.Stats.FatalError)
	}
	report.Status = loadStatus(report)
	if err := writeReport(opts.reportPath, report); err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: write report: %v\n", err)
		return 1
	}
	printSummary(stdout, report)
	if report.Status != "pass" {
		return 2
	}
	return 0
}

func runRecovery(opts options, inserter writer.Inserter, logger *slog.Logger, stdout, stderr io.Writer) int {
	ctx := context.Background()
	started := time.Now().UTC()
	load, err := writer.OpenLogs(ctx, writerConfig(opts, logger, nil), inserter)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: recover: %v\n", err)
		return 1
	}
	stats := load.Stats()
	if err := closeWriter(load, 60*time.Second); err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: close recovered writer: %v\n", err)
		return 1
	}
	report := loadReport{
		Kind:             "recovery",
		Version:          benchversion.Version,
		Commit:           benchversion.Commit,
		StartedAt:        started.Format(time.RFC3339Nano),
		FinishedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Table:            opts.table,
		TenantPrefix:     opts.tenantPrefix,
		RecoveredBatches: stats.RecoveredBatches,
		RecoveredEvents:  stats.RecoveredEvents,
		Stats:            stats,
		Status:           "pass",
	}
	if stats.DLQEvents > 0 || stats.FatalError != "" {
		report.Status = "fail"
		report.Notes = append(report.Notes, "recovery had dlq events or a fatal writer error")
	}
	if err := writeReport(opts.reportPath, report); err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: write report: %v\n", err)
		return 1
	}
	logger.Info("recovery finished", "batches", stats.RecoveredBatches, "events", stats.RecoveredEvents)
	printSummary(stdout, report)
	if report.Status != "pass" {
		return 2
	}
	return 0
}

func runDedupeTest(opts options, inserter writer.Inserter, logger *slog.Logger, stdout, stderr io.Writer) int {
	ctx := context.Background()
	started := time.Now().UTC()
	load, err := writer.OpenLogs(ctx, writerConfig(opts, logger, nil), inserter)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: dedupe test: %v\n", err)
		return 1
	}
	generator := newGenerator(opts)
	generator.fill(0, time.Now())
	first, err := load.SubmitLogs(ctx, generator.records)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: dedupe first submit: %v\n", err)
		_ = closeWriter(load, 60*time.Second)
		return 1
	}
	second, err := load.SubmitLogs(ctx, generator.records)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: dedupe second submit: %v\n", err)
		_ = closeWriter(load, 60*time.Second)
		return 1
	}
	waitForDrain(ctx, load, logger)
	closeErr := closeWriter(load, 60*time.Second)
	stats := load.Stats()
	submittedBatches := 0
	if first.Rows > 0 {
		submittedBatches++
	}
	if second.Rows > 0 {
		submittedBatches++
	}
	report := loadReport{
		Kind:             "dedupe",
		Version:          benchversion.Version,
		Commit:           benchversion.Commit,
		StartedAt:        started.Format(time.RFC3339Nano),
		FinishedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Table:            opts.table,
		TenantPrefix:     opts.tenantPrefix,
		BatchRows:        opts.batchRows,
		SubmittedBatches: submittedBatches,
		ConfirmedEvents:  uint64(first.Rows),
		DuplicateEvents:  uint64(second.Duplicates),
		Stats:            stats,
		Status:           "pass",
		Notes:            []string{"second submit must be dropped by the writer event_id window"},
	}
	if closeErr != nil {
		report.Notes = append(report.Notes, "close: "+closeErr.Error())
	}
	if second.Duplicates != opts.batchRows || stats.DuplicateEvents != uint64(opts.batchRows) {
		report.Status = "fail"
		report.Notes = append(report.Notes, "the second batch was not fully deduplicated")
	}
	if err := writeReport(opts.reportPath, report); err != nil {
		_, _ = fmt.Fprintf(stderr, "chloadgen: write report: %v\n", err)
		return 1
	}
	logger.Info("dedupe finished", "first_rows", first.Rows, "duplicates", second.Duplicates)
	printSummary(stdout, report)
	if report.Status != "pass" {
		return 2
	}
	return 0
}

func writerConfig(opts options, logger *slog.Logger, onAck func(writer.Ack)) writer.Config {
	return writer.Config{
		Table:         opts.table,
		SpoolDir:      opts.spool,
		QueueDepth:    opts.queueDepth,
		DedupeWindow:  opts.dedupeWindow,
		MaxSkew:       opts.maxSkew,
		RetryBackoff:  500 * time.Millisecond,
		EncodeWorkers: opts.generate,
		Logger:        logger,
		OnAck:         onAck,
	}
}

type loadReport struct {
	Kind              string       `json:"kind"`
	Version           string       `json:"version"`
	Commit            string       `json:"commit"`
	StartedAt         string       `json:"started_at"`
	FinishedAt        string       `json:"finished_at"`
	Table             string       `json:"table"`
	TenantPrefix      string       `json:"tenant_prefix"`
	TargetRate        int          `json:"target_rate"`
	RequestedDuration string       `json:"requested_duration,omitempty"`
	BatchRows         int          `json:"batch_rows"`
	TotalBatches      int          `json:"total_batches,omitempty"`
	SubmittedBatches  int          `json:"submitted_batches"`
	ConfirmedEvents   uint64       `json:"confirmed_events"`
	DuplicateEvents   uint64       `json:"duplicate_events"`
	RecoveredBatches  uint64       `json:"recovered_batches,omitempty"`
	RecoveredEvents   uint64       `json:"recovered_events,omitempty"`
	GenerationSeconds float64      `json:"generation_seconds"`
	DrainSeconds      float64      `json:"drain_seconds"`
	TotalSeconds      float64      `json:"total_seconds"`
	ThroughputEPS     float64      `json:"throughput_events_per_second"`
	Lag               lagSummary   `json:"lag"`
	Stats             writer.Stats `json:"writer"`
	Status            string       `json:"status"`
	Notes             []string     `json:"notes,omitempty"`
}

// lagSummary uses the oldest event in each acknowledged batch as the
// authoritative ingestion lag. Newest* is ack time minus the newest event in
// the batch and can be negative when the generator schedules the batch ahead
// of the wall clock.
type lagSummary struct {
	Samples     int     `json:"samples"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	P99Ms       float64 `json:"p99_ms"`
	MaxMs       float64 `json:"max_ms"`
	NewestP50Ms float64 `json:"newest_p50_ms"`
	NewestP95Ms float64 `json:"newest_p95_ms"`
	NewestP99Ms float64 `json:"newest_p99_ms"`
	NewestMaxMs float64 `json:"newest_max_ms"`
}

func buildLoadReport(opts options, started, finished, generationStart, generationEnd, drainEnd time.Time, submitted int, confirmed, duplicates uint64, acks []writer.Ack, stats, statsBeforeDrain writer.Stats) loadReport {
	generationSeconds := generationEnd.Sub(generationStart).Seconds()
	drainSeconds := drainEnd.Sub(generationEnd).Seconds()
	totalSeconds := finished.Sub(started).Seconds()
	throughput := 0.0
	if generationSeconds > 0 {
		throughput = float64(confirmed) / generationSeconds
	}
	return loadReport{
		Kind:              "load",
		Version:           benchversion.Version,
		Commit:            benchversion.Commit,
		StartedAt:         started.Format(time.RFC3339Nano),
		FinishedAt:        finished.Format(time.RFC3339Nano),
		Table:             opts.table,
		TenantPrefix:      opts.tenantPrefix,
		TargetRate:        opts.rate,
		RequestedDuration: opts.duration.String(),
		BatchRows:         opts.batchRows,
		TotalBatches:      int(math.Ceil(float64(opts.rate) * opts.duration.Seconds() / float64(opts.batchRows))),
		SubmittedBatches:  submitted,
		ConfirmedEvents:   confirmed,
		DuplicateEvents:   duplicates,
		GenerationSeconds: round(generationSeconds, 3),
		DrainSeconds:      round(drainSeconds, 3),
		TotalSeconds:      round(totalSeconds, 3),
		ThroughputEPS:     round(throughput, 1),
		Lag:               summarizeLags(acks),
		Stats:             stats,
		Notes: []string{
			fmt.Sprintf("drain started with %d pending batches and %d bytes", statsBeforeDrain.PendingBatches, statsBeforeDrain.PendingBytes),
			"local single-node run; the 1-hour T1 gate is not executed here",
		},
	}
}

func summarizeLags(acks []writer.Ack) lagSummary {
	if len(acks) == 0 {
		return lagSummary{}
	}
	oldest := make([]time.Duration, 0, len(acks))
	newest := make([]time.Duration, 0, len(acks))
	for _, ack := range acks {
		oldest = append(oldest, ack.OldestLag)
		newest = append(newest, ack.Lag)
	}
	return lagSummary{
		Samples:     len(acks),
		P50Ms:       milliseconds(percentile(oldest, 50)),
		P95Ms:       milliseconds(percentile(oldest, 95)),
		P99Ms:       milliseconds(percentile(oldest, 99)),
		MaxMs:       milliseconds(percentile(oldest, 100)),
		NewestP50Ms: milliseconds(percentile(newest, 50)),
		NewestP95Ms: milliseconds(percentile(newest, 95)),
		NewestP99Ms: milliseconds(percentile(newest, 99)),
		NewestMaxMs: milliseconds(percentile(newest, 100)),
	}
}

func loadStatus(report loadReport) string {
	if report.Stats.FatalError != "" || report.Stats.DLQEvents > 0 {
		return "fail"
	}
	if report.ThroughputEPS < float64(report.TargetRate)*0.95 {
		return "fail"
	}
	if report.Lag.P95Ms > 10_000 {
		return "fail"
	}
	return "pass"
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func waitForDrain(ctx context.Context, load *writer.Writer, logger *slog.Logger) {
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		stats := load.Stats()
		if stats.PendingBatches == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	logger.Warn("drain timed out", "pending_batches", load.Stats().PendingBatches)
}

func startMetricsServer(address string, load *writer.Writer, logger *slog.Logger) *http.Server {
	if address == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", load.MetricsHandler())
	mux.Handle("/stats", load.StatsHandler())
	mux.HandleFunc("/healthz", func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"status":"ok"}`))
	})
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("metrics server stopped", "error", err)
		}
	}()
	return server
}

func stopMetricsServer(server *http.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func writeReport(path string, report loadReport) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func printSummary(output io.Writer, report loadReport) {
	_, _ = fmt.Fprintf(output, "chloadgen: kind=%s status=%s events=%d throughput=%.0f/s lag_p95=%.0fms recovered=%d duplicates=%d\n",
		report.Kind,
		report.Status,
		report.ConfirmedEvents,
		report.ThroughputEPS,
		report.Lag.P95Ms,
		report.RecoveredEvents,
		report.DuplicateEvents,
	)
}

func newLogger(level string, output io.Writer) *slog.Logger {
	var parsed slog.Level
	switch level {
	case "debug":
		parsed = slog.LevelDebug
	case "warn":
		parsed = slog.LevelWarn
	case "error":
		parsed = slog.LevelError
	default:
		parsed = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: parsed}))
}

func closeWriter(load *writer.Writer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return load.Close(ctx)
}

func round(value float64, digits int) float64 {
	scale := math.Pow10(digits)
	return math.Round(value*scale) / scale
}

func milliseconds(value time.Duration) float64 {
	return round(float64(value)/float64(time.Millisecond), 3)
}

// crashInserter exits after the configured number of successful inserts,
// before the writer can advance its checkpoint. It reproduces the
// crash-after-insert window for the recovery test.
type noopInserter struct{}

func (noopInserter) Insert(_ context.Context, request writer.InsertRequest) (writer.InsertResult, error) {
	return writer.InsertResult{WrittenRows: uint64(request.Rows)}, nil
}

type crashInserter struct {
	inner  writer.Inserter
	after  int64
	count  atomic.Int64
	stderr io.Writer
}

func (c *crashInserter) Insert(ctx context.Context, request writer.InsertRequest) (writer.InsertResult, error) {
	result, err := c.inner.Insert(ctx, request)
	if err == nil && c.count.Add(1) == c.after {
		_, _ = fmt.Fprintf(c.stderr, "chloadgen: simulated crash after insert %d (before checkpoint)\n", c.after)
		os.Exit(137)
	}
	return result, err
}

// generator fills a reusable record slice. Maps and bodies are shared; only
// the event-specific strings are allocated. The slice is reused across
// batches, which keeps the 1M/s generator out of the garbage collector's way.
type generator struct {
	opts       options
	records    []writer.LogRecord
	ids        []byte
	attributes map[string]string
	resource   map[string]string
	tenants    []string
	projects   []string
	services   []string
	bodies     [3]string
	severities [3]string
}

func newGenerator(opts options) *generator {
	generator := &generator{
		opts:     opts,
		records:  make([]writer.LogRecord, opts.batchRows),
		tenants:  []string{opts.tenantPrefix + "-0", opts.tenantPrefix + "-1"},
		projects: []string{"project-0", "project-1", "project-2", "project-3"},
		services: []string{"frontend", "checkout", "payments", "inventory", "shipping", "email", "auth", "search"},
		bodies: [3]string{
			"request completed method=GET path=/api/v1/items status=200 duration_ms=12",
			"request completed method=POST path=/api/v1/items status=201 duration_ms=48",
			"request failed method=GET path=/api/v1/items status=500 duration_ms=250",
		},
		severities: [3]string{"INFO", "WARN", "ERROR"},
		attributes: map[string]string{
			"http.method":      "GET",
			"http.route":       "/api/v1/items",
			"http.status_code": "200",
		},
		resource: map[string]string{
			"deployment.environment": "bench",
			"service.namespace":      "observaki",
		},
	}
	return generator
}

func (g *generator) fill(batch int, start time.Time) {
	g.fillInto(g.records, batch, start)
}

func (g *generator) fillInto(records []writer.LogRecord, batch int, start time.Time) {
	base := uint64(batch) * uint64(len(records))
	batchDuration := time.Duration(float64(len(records)) / float64(g.opts.rate) * float64(time.Second))

	// Build one contiguous hex string per batch and slice it per row. A string
	// slice shares the backing array, so the 1M/s generator allocates one ID
	// buffer per batch instead of three strings per event.
	needed := len(records) * 32
	if cap(g.ids) < needed {
		g.ids = make([]byte, needed)
	} else {
		g.ids = g.ids[:needed]
	}
	var raw [16]byte
	namespace := fnv.New64a()
	_, _ = namespace.Write([]byte(g.opts.tenantPrefix))
	binary.BigEndian.PutUint64(raw[0:8], namespace.Sum64())
	for index := 0; index < len(records); index++ {
		binary.BigEndian.PutUint64(raw[8:16], base+uint64(index))
		hex.Encode(g.ids[index*32:(index+1)*32], raw[:])
	}
	ids := string(g.ids)

	workers := g.opts.generate
	if workers <= 0 {
		workers = 1
	}
	chunk := (len(records) + workers - 1) / workers
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		begin := worker * chunk
		end := begin + chunk
		if begin >= len(records) {
			break
		}
		if end > len(records) {
			end = len(records)
		}
		group.Add(1)
		go func(begin, end int) {
			defer group.Done()
			for index := begin; index < end; index++ {
				id := base + uint64(index)
				severity := 0
				switch {
				case id%100 < 3:
					severity = 2
				case id%100 < 10:
					severity = 1
				}
				eventID := ids[index*32 : (index+1)*32]
				records[index] = writer.LogRecord{
					TenantID:           g.tenants[id%uint64(len(g.tenants))],
					ProjectID:          g.projects[id%uint64(len(g.projects))],
					Timestamp:          start.Add(time.Duration(index) * batchDuration / time.Duration(len(records))),
					Severity:           g.severities[severity],
					SeverityNumber:     uint8(9 + severity*4),
					ServiceName:        g.services[id%uint64(len(g.services))],
					Body:               g.bodies[severity],
					Attributes:         g.attributes,
					ResourceAttributes: g.resource,
					TraceID:            ids[(index/10)*32 : (index/10+1)*32],
					SpanID:             eventID,
					EventID:            eventID,
					RetentionDays:      15,
				}
			}
		}(begin, end)
	}
	group.Wait()
}
