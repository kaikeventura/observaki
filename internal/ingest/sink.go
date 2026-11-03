package ingest

import (
	"context"
	"errors"
	"time"

	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Sink errors shared by the gateway and the durable adapters.
var (
	// ErrBackpressure signals that the WAL cannot accept more data right
	// now. The gateway maps it to 429 + Retry-After instead of acking
	// silently (AD-4, ADR-0003).
	ErrBackpressure = errors.New("ingest: sink is at capacity")
	// ErrSinkClosed signals that the sink is shutting down. The gateway
	// maps it to 503 and the client retries.
	ErrSinkClosed = errors.New("ingest: sink is closed")
)

// Batch is one validated and redacted OTLP export request. Exactly one of
// Traces, Logs or Metrics is set, matching Signal. TenantID and ProjectID stay
// empty until Story 4.2 resolves the ingest token; the sink must not persist a
// batch without them once auth lands (AD-2).
type Batch struct {
	Signal    Signal
	TenantID  string
	ProjectID string
	// RequestID is the gateway request ID used in logs, errors and the WAL
	// envelope. Empty values are replaced by the sink.
	RequestID string
	// ConfigVersion and ConfigGeneration identify the auth snapshot that
	// resolved the identity, so the WAL and audit can reject stale epochs
	// (AD-19, AD-20).
	ConfigVersion    string
	ConfigGeneration int64
	ReceivedAt       time.Time
	Traces           *tracepb.ExportTraceServiceRequest
	Logs             *logspb.ExportLogsServiceRequest
	Metrics          *metricpb.ExportMetricsServiceRequest
}

// Events returns the number of spans, log records or data points in the batch.
func (b *Batch) Events() int {
	if b == nil {
		return 0
	}
	switch b.Signal {
	case SignalTraces:
		return countSpans(b.Traces)
	case SignalLogs:
		return countLogRecords(b.Logs)
	case SignalMetrics:
		return countDataPoints(b.Metrics)
	default:
		return 0
	}
}

// Result reports what the sink accepted. Accepted+Rejected must equal the
// batch event count; Rejected may be zero with a non-empty Reason to convey a
// warning in partial_success, as OTLP allows.
type Result struct {
	Accepted int
	Rejected int
	Reason   string
}

// Sink is the durable ingestion port. The gateway only acknowledges a request
// after Consume returns (AD-4); the production sink is the NATS JetStream WAL
// in internal/adapters/nats.
type Sink interface {
	Consume(ctx context.Context, batch *Batch) (Result, error)
}

// SinkHealth is implemented by durable sinks that can fail readiness when the
// WAL is unreachable or closed (AD-4, AD-10).
type SinkHealth interface {
	Healthy() error
}

// SinkStats is a point-in-time view of the durable sink. The gateway exports
// it on /metrics.
type SinkStats struct {
	Connected        bool
	PublishedBatches uint64
	PublishedEvents  uint64
	PublishErrors    uint64
	PublishRetries   uint64
	Backpressure     uint64
	PendingBytes     int64
	PendingEvents    int64
	InFlightBatches  int64
	PublishP50       time.Duration
	PublishP95       time.Duration
	PublishP99       time.Duration
}

// SinkStatsProvider is implemented by durable sinks that expose counters.
type SinkStatsProvider interface {
	Stats() SinkStats
}

// DiscardSink accepts every event without persisting it. It is a test fixture
// only; the gateway command always uses the durable NATS WAL.
type DiscardSink struct{}

// Consume implements Sink.
func (DiscardSink) Consume(_ context.Context, batch *Batch) (Result, error) {
	return Result{Accepted: batch.Events()}, nil
}
