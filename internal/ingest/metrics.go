package ingest

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
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

var authFailureReasons = [...]string{
	"missing_token",
	"invalid_token",
	"expired_token",
	"insufficient_scope",
	"cross_project_write",
	"config_stale",
	"other",
}

const authFailureOther = 6

func authFailureIndex(reason string) int {
	for index, candidate := range authFailureReasons {
		if candidate == reason {
			return index
		}
	}
	return authFailureOther
}

// gatewayMetrics holds the gateway's self-observability counters. They are
// exported in Prometheus text format on /metrics (AD-10).
type gatewayMetrics struct {
	requests [3][2]atomic.Uint64
	events   [3][2]atomic.Uint64
	rejected [3][2]atomic.Uint64
	errors   [3][2]atomic.Uint64
	inFlight atomic.Int64

	authFailures         [7]atomic.Uint64
	quotaRejectedEvents  atomic.Uint64
	quotaRejectedRequest atomic.Uint64
	latency              *latencyWindows
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

func (m *gatewayMetrics) observeAuthFailure(reason string) {
	m.authFailures[authFailureIndex(reason)].Add(1)
}

func (m *gatewayMetrics) observeQuotaRejection(events int) {
	m.quotaRejectedRequest.Add(1)
	if events > 0 {
		m.quotaRejectedEvents.Add(uint64(events))
	}
}

func (m *gatewayMetrics) observeLatency(tenant string, duration time.Duration) {
	if m.latency == nil {
		return
	}
	m.latency.observe(tenant, duration)
}

func (g *Gateway) handleSelfMetrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		var out strings.Builder
		g.metrics.write(&out)

		out.WriteString("# HELP observaki_gateway_auth_failures_total OTLP requests rejected by authentication or authorization.\n")
		out.WriteString("# TYPE observaki_gateway_auth_failures_total counter\n")
		for index, reason := range authFailureReasons {
			fmt.Fprintf(&out, "observaki_gateway_auth_failures_total{reason=%q} %d\n", reason, g.metrics.authFailures[index].Load())
		}

		out.WriteString("# HELP observaki_gateway_quota_rejected_events_total Events rejected because the tenant exhausted its ingest quota.\n")
		out.WriteString("# TYPE observaki_gateway_quota_rejected_events_total counter\n")
		fmt.Fprintf(&out, "observaki_gateway_quota_rejected_events_total %d\n", g.metrics.quotaRejectedEvents.Load())
		out.WriteString("# HELP observaki_gateway_quota_rejected_requests_total Requests rejected with 429 because the tenant exhausted its ingest quota.\n")
		out.WriteString("# TYPE observaki_gateway_quota_rejected_requests_total counter\n")
		fmt.Fprintf(&out, "observaki_gateway_quota_rejected_requests_total %d\n", g.metrics.quotaRejectedRequest.Load())

		authEnabled := 0
		if g.auth != nil {
			authEnabled = 1
		}
		out.WriteString("# HELP observaki_gateway_auth_enabled Whether token authentication and quotas are enabled.\n")
		out.WriteString("# TYPE observaki_gateway_auth_enabled gauge\n")
		fmt.Fprintf(&out, "observaki_gateway_auth_enabled %d\n", authEnabled)

		if g.auth != nil {
			state := g.auth.State()
			out.WriteString("# HELP observaki_gateway_config_age_seconds Seconds since the last successful config snapshot load.\n")
			out.WriteString("# TYPE observaki_gateway_config_age_seconds gauge\n")
			fmt.Fprintf(&out, "observaki_gateway_config_age_seconds %.3f\n", state.Age.Seconds())
			out.WriteString("# HELP observaki_gateway_config_expired Whether the config snapshot is past expires_at (fail-closed auth/quota).\n")
			out.WriteString("# TYPE observaki_gateway_config_expired gauge\n")
			expired := 0
			if state.Expired {
				expired = 1
			}
			fmt.Fprintf(&out, "observaki_gateway_config_expired %d\n", expired)
			out.WriteString("# HELP observaki_gateway_config_generation Generation of the loaded config snapshot.\n")
			out.WriteString("# TYPE observaki_gateway_config_generation gauge\n")
			fmt.Fprintf(&out, "observaki_gateway_config_generation %d\n", state.Generation)
			out.WriteString("# HELP observaki_gateway_config_tenants Tenants in the loaded config snapshot.\n")
			out.WriteString("# TYPE observaki_gateway_config_tenants gauge\n")
			fmt.Fprintf(&out, "observaki_gateway_config_tenants %d\n", state.Tenants)
		}

		if g.metrics.latency != nil {
			snapshot := g.metrics.latency.snapshot()
			tenants := make([]string, 0, len(snapshot))
			for tenant := range snapshot {
				tenants = append(tenants, tenant)
			}
			sort.Strings(tenants)
			out.WriteString("# HELP observaki_gateway_request_duration_p50_seconds Recent-window median request duration per tenant.\n")
			out.WriteString("# TYPE observaki_gateway_request_duration_p50_seconds gauge\n")
			for _, tenant := range tenants {
				fmt.Fprintf(&out, "observaki_gateway_request_duration_p50_seconds{tenant_id=%q} %.6f\n", tenant, snapshot[tenant].P50.Seconds())
			}
			out.WriteString("# HELP observaki_gateway_request_duration_p95_seconds Recent-window p95 request duration per tenant.\n")
			out.WriteString("# TYPE observaki_gateway_request_duration_p95_seconds gauge\n")
			for _, tenant := range tenants {
				fmt.Fprintf(&out, "observaki_gateway_request_duration_p95_seconds{tenant_id=%q} %.6f\n", tenant, snapshot[tenant].P95.Seconds())
			}
			out.WriteString("# HELP observaki_gateway_request_duration_p99_seconds Recent-window p99 request duration per tenant.\n")
			out.WriteString("# TYPE observaki_gateway_request_duration_p99_seconds gauge\n")
			for _, tenant := range tenants {
				fmt.Fprintf(&out, "observaki_gateway_request_duration_p99_seconds{tenant_id=%q} %.6f\n", tenant, snapshot[tenant].P99.Seconds())
			}
			out.WriteString("# HELP observaki_gateway_request_duration_samples Number of recent requests behind the per-tenant latency gauges.\n")
			out.WriteString("# TYPE observaki_gateway_request_duration_samples gauge\n")
			for _, tenant := range tenants {
				fmt.Fprintf(&out, "observaki_gateway_request_duration_samples{tenant_id=%q} %d\n", tenant, snapshot[tenant].Count)
			}
		}

		_, _ = io.WriteString(w, out.String())
	})
}

func (m *gatewayMetrics) write(out *strings.Builder) {
	writeFamily := func(name, help, kind string, values *[3][2]atomic.Uint64) {
		fmt.Fprintf(out, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
		for si, signal := range metricSignals {
			for pi, p := range metricProtocols {
				fmt.Fprintf(out, "%s{signal=%q,protocol=%q} %d\n", name, signal, p, values[si][pi].Load())
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
	fmt.Fprintf(out, "observaki_gateway_in_flight_requests %d\n", m.inFlight.Load())
	out.WriteString("# HELP observaki_gateway_up Whether the OTLP gateway is running.\n")
	out.WriteString("# TYPE observaki_gateway_up gauge\n")
	out.WriteString("observaki_gateway_up 1\n")
}
