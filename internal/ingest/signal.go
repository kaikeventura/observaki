// Package ingest implements the OTLP ingestion gateway: it accepts OTLP
// gRPC/HTTP for traces, logs and metrics, applies the request limits and the
// redaction pipeline, and hands batches to the durable sink (the WAL).
package ingest

// Signal is one OTLP telemetry signal.
type Signal string

// Supported OTLP signals.
const (
	SignalTraces  Signal = "traces"
	SignalLogs    Signal = "logs"
	SignalMetrics Signal = "metrics"
)

// String returns the signal name used in logs, metrics and errors.
func (s Signal) String() string { return string(s) }
