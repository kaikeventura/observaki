package loadgen

import "fmt"

// Signal is one of the three OTLP signals supported by the generator.
type Signal uint8

const (
	Metrics Signal = iota
	Logs
	Traces
)

// AllSignals is the canonical signal order used by the harness.
var AllSignals = []Signal{Metrics, Logs, Traces}

// String returns the stable lowercase name used in metrics and reports.
func (s Signal) String() string {
	switch s {
	case Metrics:
		return "metrics"
	case Logs:
		return "logs"
	case Traces:
		return "traces"
	default:
		return fmt.Sprintf("signal(%d)", s)
	}
}
