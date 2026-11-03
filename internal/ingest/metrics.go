package ingest

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

type protocol int

const (
	protocolGRPC protocol = iota
	protocolHTTP
)

func (p protocol) String() string {
	if p == protocolHTTP {
		return "http"
	}
	return "grpc"
}

func signalIndex(signal Signal) int {
	switch signal {
	case SignalTraces:
		return 0
	case SignalLogs:
		return 1
	case SignalMetrics:
		return 2
	default:
		return 0
	}
}

var metricSignals = [...]Signal{SignalTraces, SignalLogs, SignalMetrics}
var metricProtocols = [...]protocol{protocolGRPC, protocolHTTP}

// gatewayMetrics holds the gateway's self-observability counters. They are
// exported in Prometheus text format on /metrics (AD-10).
type gatewayMetrics struct {
	requests [3][2]atomic.Uint64
	events   [3][2]atomic.Uint64
	rejected [3][2]atomic.Uint64
	errors   [3][2]atomic.Uint64
	inFlight atomic.Int64
}

func (m *gatewayMetrics) observeRequest(signal Signal, p protocol) {
	m.requests[signalIndex(signal)][p].Add(1)
}

func (m *gatewayMetrics) observeEvents(signal Signal, p protocol, received, rejected int) {
	m.events[signalIndex(signal)][p].Add(uint64(received))
	m.rejected[signalIndex(signal)][p].Add(uint64(rejected))
}

func (m *gatewayMetrics) observeError(signal Signal, p protocol) {
	m.errors[signalIndex(signal)][p].Add(1)
}

func (m *gatewayMetrics) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		var out strings.Builder

		writeFamily := func(name, help, kind string, values *[3][2]atomic.Uint64) {
			fmt.Fprintf(&out, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
			for si, signal := range metricSignals {
				for pi, p := range metricProtocols {
					fmt.Fprintf(&out, "%s{signal=%q,protocol=%q} %d\n", name, signal, p, values[si][pi].Load())
				}
			}
		}

		writeFamily(
			"observaki_gateway_requests_total",
			"OTLP export requests received by the gateway.",
			"counter",
			&m.requests,
		)
		writeFamily(
			"observaki_gateway_events_total",
			"Spans, log records and data points received by the gateway.",
			"counter",
			&m.events,
		)
		writeFamily(
			"observaki_gateway_rejected_events_total",
			"Events rejected by the pipeline or the sink.",
			"counter",
			&m.rejected,
		)
		writeFamily(
			"observaki_gateway_errors_total",
			"OTLP export requests that returned an error.",
			"counter",
			&m.errors,
		)

		out.WriteString("# HELP observaki_gateway_in_flight_requests OTLP export requests currently being processed.\n")
		out.WriteString("# TYPE observaki_gateway_in_flight_requests gauge\n")
		fmt.Fprintf(&out, "observaki_gateway_in_flight_requests %d\n", m.inFlight.Load())
		out.WriteString("# HELP observaki_gateway_up Whether the OTLP gateway is running.\n")
		out.WriteString("# TYPE observaki_gateway_up gauge\n")
		out.WriteString("observaki_gateway_up 1\n")

		_, _ = io.WriteString(w, out.String())
	})
}
