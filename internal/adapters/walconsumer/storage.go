package walconsumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/observaki/observaki/internal/adapters/clickhouse/writer"
	"github.com/observaki/observaki/internal/adapters/victoriametrics"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// WriteResult reports one batch handed to storage.
type WriteResult struct {
	Events   int
	Written  int
	Deduped  int
	Failed   int
	Skipped  int
	Retries  int
	Sequence uint64
}

// StorageConfig wires the durable ClickHouse writers and the VictoriaMetrics
// remote write client. The Storage owns the writers so the terminal-outcome
// callbacks are wired correctly.
type StorageConfig struct {
	Inserter      writer.Inserter
	Metrics       *victoriametrics.Client
	SpoolDir      string
	DedupeWindow  int
	RetentionDays uint16
	QueueDepth    int
	MaxRetries    int
	RetryBackoff  time.Duration
	Logger        *slog.Logger
}

// Storage implements the WAL consumer's downstream port. Logs and traces are
// converted to the ClickHouse RowBinary schema; metrics are converted to
// Prometheus remote write series.
type Storage struct {
	cfg     StorageConfig
	logger  *slog.Logger
	logs    *writer.Writer
	traces  *writer.Writer
	metrics *victoriametrics.Client

	logsOutcomes   *outcomeTracker
	tracesOutcomes *outcomeTracker

	logsDedupe    *eventDedupe
	tracesDedupe  *eventDedupe
	metricsDedupe *eventDedupe
}

// NewStorage opens the ClickHouse writers (replaying any spool left by a
// previous process) and prepares the per-signal dedupe windows.
func NewStorage(ctx context.Context, cfg StorageConfig) (*Storage, error) {
	if cfg.Inserter == nil {
		return nil, errors.New("walconsumer: clickhouse inserter is required")
	}
	if cfg.Metrics == nil {
		return nil, errors.New("walconsumer: victoriametrics client is required")
	}
	if cfg.SpoolDir == "" {
		return nil, errors.New("walconsumer: spool directory is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.DedupeWindow < 0 {
		return nil, errors.New("walconsumer: dedupe window cannot be negative")
	}
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = writer.DefaultRetentionDays
	}
	if cfg.QueueDepth <= 0 {
		cfg.QueueDepth = 4
	}

	logsOutcomes := newOutcomeTracker()
	tracesOutcomes := newOutcomeTracker()
	logs, err := writer.OpenLogs(ctx, writer.Config{
		SpoolDir:              filepath.Join(cfg.SpoolDir, "logs"),
		QueueDepth:            cfg.QueueDepth,
		MaxRetries:            cfg.MaxRetries,
		RetryBackoff:          cfg.RetryBackoff,
		DedupeTokenFromEvents: true,
		OnDone:                logsOutcomes.done,
		Logger:                cfg.Logger,
	}, cfg.Inserter)
	if err != nil {
		return nil, fmt.Errorf("walconsumer: open logs writer: %w", err)
	}
	traces, err := writer.OpenTraces(ctx, writer.Config{
		SpoolDir:              filepath.Join(cfg.SpoolDir, "traces"),
		QueueDepth:            cfg.QueueDepth,
		MaxRetries:            cfg.MaxRetries,
		RetryBackoff:          cfg.RetryBackoff,
		DedupeTokenFromEvents: true,
		OnDone:                tracesOutcomes.done,
		Logger:                cfg.Logger,
	}, cfg.Inserter)
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = logs.Close(closeCtx)
		return nil, fmt.Errorf("walconsumer: open traces writer: %w", err)
	}

	return &Storage{
		cfg:            cfg,
		logger:         cfg.Logger,
		logs:           logs,
		traces:         traces,
		metrics:        cfg.Metrics,
		logsOutcomes:   logsOutcomes,
		tracesOutcomes: tracesOutcomes,
		logsDedupe:     newEventDedupe(cfg.DedupeWindow),
		tracesDedupe:   newEventDedupe(cfg.DedupeWindow),
		metricsDedupe:  newEventDedupe(cfg.DedupeWindow),
	}, nil
}

// WriteLogs converts and stores one OTLP logs request. It returns only after
// ClickHouse confirmed the spooled batch (or the writer moved it to the DLQ).
func (s *Storage) WriteLogs(
	ctx context.Context,
	tenantID, projectID string,
	generation, authzEpoch uint64,
	request *logspb.ExportLogsServiceRequest,
	eventIDs []string,
	receivedAt time.Time,
) (WriteResult, error) {
	records, err := writer.LogsFromOTLP(tenantID, projectID, generation, authzEpoch, request, eventIDs, receivedAt)
	if err != nil {
		return WriteResult{}, &PermanentError{Err: err}
	}
	unique := make([]writer.LogRecord, 0, len(records))
	uniqueIDs := make([]string, 0, len(records))
	local := make(map[string]struct{}, len(records))
	for _, record := range records {
		if _, ok := local[record.EventID]; ok {
			continue
		}
		local[record.EventID] = struct{}{}
		if s.logsDedupe.has(tenantID, projectID, generation, authzEpoch, record.EventID) {
			continue
		}
		record.RetentionDays = s.cfg.RetentionDays
		unique = append(unique, record)
		uniqueIDs = append(uniqueIDs, record.EventID)
	}
	result := WriteResult{Events: len(records), Deduped: len(records) - len(unique)}
	if len(unique) == 0 {
		return result, nil
	}

	submitted, err := s.logs.SubmitLogs(ctx, unique)
	if err != nil {
		return result, classifyWriteError(err)
	}
	outcome, err := s.logsOutcomes.wait(ctx, submitted.Seq)
	if err != nil {
		return result, err
	}
	s.logsDedupe.add(tenantID, projectID, generation, authzEpoch, uniqueIDs)
	if outcome.DLQ {
		result.Failed = outcome.Rows
		result.Retries = max(0, outcome.Attempts-1)
		s.logger.Error("logs batch moved to dlq",
			"tenant_id", tenantID, "project_id", projectID, "rows", outcome.Rows, "error", outcome.Error)
		return result, nil
	}
	result.Written = len(unique)
	result.Retries = max(0, outcome.Attempts-1)
	result.Sequence = submitted.Seq
	return result, nil
}

// WriteTraces is the traces counterpart of WriteLogs.
func (s *Storage) WriteTraces(
	ctx context.Context,
	tenantID, projectID string,
	generation, authzEpoch uint64,
	request *tracepb.ExportTraceServiceRequest,
	eventIDs []string,
	receivedAt time.Time,
) (WriteResult, error) {
	records, err := writer.TracesFromOTLP(tenantID, projectID, generation, authzEpoch, request, eventIDs, receivedAt)
	if err != nil {
		return WriteResult{}, &PermanentError{Err: err}
	}
	unique := make([]writer.SpanRecord, 0, len(records))
	uniqueIDs := make([]string, 0, len(records))
	local := make(map[string]struct{}, len(records))
	for _, record := range records {
		if _, ok := local[record.EventID]; ok {
			continue
		}
		local[record.EventID] = struct{}{}
		if s.tracesDedupe.has(tenantID, projectID, generation, authzEpoch, record.EventID) {
			continue
		}
		record.RetentionDays = s.cfg.RetentionDays
		unique = append(unique, record)
		uniqueIDs = append(uniqueIDs, record.EventID)
	}
	result := WriteResult{Events: len(records), Deduped: len(records) - len(unique)}
	if len(unique) == 0 {
		return result, nil
	}

	submitted, err := s.traces.SubmitTraces(ctx, unique)
	if err != nil {
		return result, classifyWriteError(err)
	}
	outcome, err := s.tracesOutcomes.wait(ctx, submitted.Seq)
	if err != nil {
		return result, err
	}
	s.tracesDedupe.add(tenantID, projectID, generation, authzEpoch, uniqueIDs)
	if outcome.DLQ {
		result.Failed = outcome.Rows
		result.Retries = max(0, outcome.Attempts-1)
		s.logger.Error("traces batch moved to dlq",
			"tenant_id", tenantID, "project_id", projectID, "rows", outcome.Rows, "error", outcome.Error)
		return result, nil
	}
	result.Written = len(unique)
	result.Retries = max(0, outcome.Attempts-1)
	result.Sequence = submitted.Seq
	return result, nil
}

// WriteMetrics converts one OTLP metrics request, deduplicates data points and
// writes the resulting series to VictoriaMetrics.
func (s *Storage) WriteMetrics(
	ctx context.Context,
	tenantID, projectID string,
	generation, authzEpoch uint64,
	request *metricpb.ExportMetricsServiceRequest,
	eventIDs []string,
	receivedAt time.Time,
) (WriteResult, error) {
	events, skipped, err := victoriametrics.ConvertMetrics(tenantID, projectID, request, eventIDs, receivedAt)
	if err != nil {
		return WriteResult{}, &PermanentError{Err: err}
	}
	result := WriteResult{Events: len(events) + skipped, Skipped: skipped}
	unique := make([]victoriametrics.MetricEvent, 0, len(events))
	uniqueIDs := make([]string, 0, len(events))
	local := make(map[string]struct{}, len(events))
	for _, event := range events {
		if _, ok := local[event.EventID]; ok {
			continue
		}
		local[event.EventID] = struct{}{}
		if s.metricsDedupe.has(tenantID, projectID, generation, authzEpoch, event.EventID) {
			continue
		}
		unique = append(unique, event)
		uniqueIDs = append(uniqueIDs, event.EventID)
	}
	result.Deduped = len(events) - len(unique)
	if len(unique) == 0 {
		return result, nil
	}
	series := victoriametrics.GroupEvents(unique)
	written, err := s.metrics.Write(ctx, series)
	if err != nil {
		return result, classifyWriteError(err)
	}
	s.metricsDedupe.add(tenantID, projectID, generation, authzEpoch, uniqueIDs)
	result.Written = len(unique)
	result.Retries = written.Retries
	return result, nil
}

// Close drains the ClickHouse writers and closes idle HTTP connections. A
// timeout leaves unacknowledged batches in the spool for the next start.
func (s *Storage) Close(ctx context.Context) error {
	var (
		wg       sync.WaitGroup
		logsErr  error
		traceErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		logsErr = s.logs.Close(ctx)
	}()
	go func() {
		defer wg.Done()
		traceErr = s.traces.Close(ctx)
	}()
	wg.Wait()
	s.metrics.Close()
	return errors.Join(logsErr, traceErr)
}

// PermanentError marks a message that can never be stored (for example a
// malformed OTLP payload). The consumer publishes it to the DLQ and acks the
// original message.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// IsPermanent reports whether err is a permanent rejection.
func IsPermanent(err error) bool {
	var permanent *PermanentError
	if errors.As(err, &permanent) {
		return true
	}
	return writer.IsPermanent(err) || victoriametrics.IsPermanent(err)
}

func classifyWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, writer.ErrClosed) || errors.Is(err, context.Canceled) {
		return err
	}
	if writer.IsPermanent(err) || victoriametrics.IsPermanent(err) {
		return &PermanentError{Err: err}
	}
	return err
}

// outcomeTracker keeps terminal writer outcomes until the consumer waits for
// them. The ClickHouse writer is sequential, so the wait is a simple
// monotonic barrier; the outcome map only needs to hold the recent window.
type outcomeTracker struct {
	mu       sync.Mutex
	lastDone uint64
	notify   chan struct{}
	outcomes map[uint64]writer.Outcome
}

func newOutcomeTracker() *outcomeTracker {
	return &outcomeTracker{
		notify:   make(chan struct{}),
		outcomes: make(map[uint64]writer.Outcome),
	}
}

func (t *outcomeTracker) done(outcome writer.Outcome) {
	t.mu.Lock()
	if outcome.Seq > t.lastDone {
		t.lastDone = outcome.Seq
	}
	t.outcomes[outcome.Seq] = outcome
	for seq := range t.outcomes {
		if seq+64 < t.lastDone {
			delete(t.outcomes, seq)
		}
	}
	close(t.notify)
	t.notify = make(chan struct{})
	t.mu.Unlock()
}

func (t *outcomeTracker) wait(ctx context.Context, seq uint64) (writer.Outcome, error) {
	if seq == 0 {
		return writer.Outcome{}, nil
	}
	for {
		t.mu.Lock()
		if outcome, ok := t.outcomes[seq]; ok {
			delete(t.outcomes, seq)
			t.mu.Unlock()
			return outcome, nil
		}
		notify := t.notify
		t.mu.Unlock()
		select {
		case <-notify:
		case <-ctx.Done():
			return writer.Outcome{}, ctx.Err()
		}
	}
}

// eventDedupe is a bounded FIFO of event identities. It is in-memory by
// design; a durable cross-restart index is a Phase 1 concern (FR-4). For
// ClickHouse, the stable event token also lets block deduplication absorb a
// replay that outlives the window.
type eventDedupe struct {
	mu       sync.Mutex
	capacity int
	seen     map[string]struct{}
	ring     []string
	next     int
}

func newEventDedupe(capacity int) *eventDedupe {
	if capacity < 0 {
		capacity = 0
	}
	return &eventDedupe{
		capacity: capacity,
		seen:     make(map[string]struct{}, capacity),
		ring:     make([]string, 0, capacity),
	}
}

func (d *eventDedupe) has(tenantID, projectID string, generation, authzEpoch uint64, eventID string) bool {
	if d.capacity == 0 {
		return false
	}
	key := dedupeKey(tenantID, projectID, generation, authzEpoch, eventID)
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.seen[key]
	return ok
}

func (d *eventDedupe) add(tenantID, projectID string, generation, authzEpoch uint64, eventIDs []string) {
	if d.capacity == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, eventID := range eventIDs {
		key := dedupeKey(tenantID, projectID, generation, authzEpoch, eventID)
		if _, ok := d.seen[key]; ok {
			continue
		}
		if len(d.ring) < d.capacity {
			d.ring = append(d.ring, key)
			d.seen[key] = struct{}{}
			continue
		}
		delete(d.seen, d.ring[d.next])
		d.ring[d.next] = key
		d.seen[key] = struct{}{}
		d.next = (d.next + 1) % d.capacity
	}
}

func dedupeKey(tenantID, projectID string, generation, authzEpoch uint64, eventID string) string {
	return tenantID + "\xff" + projectID + "\xff" + strconv.FormatUint(generation, 10) +
		"\xff" + strconv.FormatUint(authzEpoch, 10) + "\xff" + eventID
}
