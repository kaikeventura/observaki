package ingest

import (
	"context"
	"time"

	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Batch is one validated and redacted OTLP export request. Exactly one of
// Traces, Logs or Metrics is set, matching Signal. TenantID and ProjectID stay
// empty until Story 4.2 resolves the ingest token; the sink must not persist a
// batch without them once auth lands (AD-2).
type Batch struct {
	Signal    Signal
	TenantID  string
	ProjectID string
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
// after Consume returns (AD-4); Story 4.3 replaces DiscardSink with the NATS
// JetStream WAL writer.
type Sink interface {
	Consume(ctx context.Context, batch *Batch) (Result, error)
}

// DiscardSink accepts every event without persisting it. It is the placeholder
// used by the Story 4.1 skeleton and must not be used once the WAL lands.
type DiscardSink struct{}

// Consume implements Sink.
func (DiscardSink) Consume(_ context.Context, batch *Batch) (Result, error) {
	return Result{Accepted: batch.Events()}, nil
}
