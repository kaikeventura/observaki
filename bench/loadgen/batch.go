package loadgen

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	logscollectorpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricscollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Batch is one OTLP export request.
type Batch struct {
	Signal  Signal
	Events  int
	Logs    *logscollectorpb.ExportLogsServiceRequest
	Metrics *metricscollectorpb.ExportMetricsServiceRequest
	Traces  *tracecollectorpb.ExportTraceServiceRequest
}

// Proto returns the typed request as a protobuf message.
func (b *Batch) Proto() proto.Message {
	switch b.Signal {
	case Metrics:
		return b.Metrics
	case Logs:
		return b.Logs
	case Traces:
		return b.Traces
	default:
		return nil
	}
}

// Size returns the serialized protobuf size in bytes.
func (b *Batch) Size() int {
	message := b.Proto()
	if message == nil {
		return 0
	}
	return proto.Size(message)
}

// HTTPPath returns the OTLP/HTTP path for the batch signal.
func (b *Batch) HTTPPath() string {
	switch b.Signal {
	case Metrics:
		return "/v1/metrics"
	case Logs:
		return "/v1/logs"
	case Traces:
		return "/v1/traces"
	default:
		return ""
	}
}

// BuildBatch builds the typed request for a signal and its event indices.
func (b *Builder) BuildBatch(signal Signal, indices []int64) (*Batch, error) {
	batch := &Batch{Signal: signal, Events: len(indices)}
	switch signal {
	case Metrics:
		batch.Metrics = b.BuildMetrics(indices)
	case Logs:
		batch.Logs = b.BuildLogs(indices)
	case Traces:
		batch.Traces = b.BuildTraces(indices)
	default:
		return nil, fmt.Errorf("unsupported signal %s", signal)
	}
	return batch, nil
}
