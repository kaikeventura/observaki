// Package selfobs builds the product health overview (AD-10, AD-24).
//
// It scrapes the data plane's own observability endpoints (/readyz, /metrics,
// /stats and /storage) instead of querying ClickHouse or VictoriaMetrics, so
// the API keeps the AD-3 boundary. Every value carries its observation time;
// when a source is down the overview keeps the last known value and marks the
// component critical (AD-24: degraded mode never shows an empty panel).
package selfobs

import "time"

// Status values used across the overview. They mirror the UX taxonomy
// (Healthy / Degraded / Critical / No data / Muted) plus the internal
// not_configured and idle states.
const (
	StatusHealthy       = "healthy"
	StatusDegraded      = "degraded"
	StatusCritical      = "critical"
	StatusNoData        = "no_data"
	StatusIdle          = "idle"
	StatusNotConfigured = "not_configured"
	StatusDisabled      = "disabled"
	StatusUnknown       = "unknown"
)

// Overview is the JSON document consumed by the health page and the internal
// alerting evaluator.
type Overview struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Status      string      `json:"status"`
	Summary     []string    `json:"summary"`
	Ingestion   Ingestion   `json:"ingestion"`
	Query       Query       `json:"query"`
	Storage     Storage     `json:"storage"`
	Components  []Component `json:"components"`
	Alerting    Alerting    `json:"alerting"`
}

// Ingestion is the end-to-end ingestion section.
type Ingestion struct {
	Status                 string     `json:"status"`
	LagP95Seconds          *float64   `json:"lag_p95_seconds,omitempty"`
	PendingMessages        *int64     `json:"pending_messages,omitempty"`
	DLQMessages            *int64     `json:"dlq_messages,omitempty"`
	WrittenEvents          *uint64    `json:"written_events,omitempty"`
	WriterConnected        *bool      `json:"writer_connected,omitempty"`
	GatewayConnected       *bool      `json:"gateway_connected,omitempty"`
	GatewayPublishedEvents *uint64    `json:"gateway_published_events,omitempty"`
	ObservedAt             *time.Time `json:"observed_at,omitempty"`
	Source                 string     `json:"source,omitempty"`
	Detail                 string     `json:"detail,omitempty"`
}

// Query is the API query-plane error rate. The Query Service is delivered in
// Phase 1; until then the rate counts the API request class, so the panel is
// wired before the service exists.
type Query struct {
	Status       string    `json:"status"`
	Requests     uint64    `json:"requests"`
	Errors       uint64    `json:"errors"`
	ClientErrors uint64    `json:"client_errors"`
	ErrorRate    float64   `json:"error_rate"`
	Since        time.Time `json:"since"`
	Detail       string    `json:"detail,omitempty"`
}

// Storage is the ClickHouse/VictoriaMetrics usage reported by the writer.
type Storage struct {
	Status                string     `json:"status"`
	ClickHouseBytes       *int64     `json:"clickhouse_bytes,omitempty"`
	ClickHouseParts       *int64     `json:"clickhouse_parts,omitempty"`
	VictoriaMetricsBytes  *int64     `json:"victoriametrics_bytes,omitempty"`
	VictoriaMetricsSeries *int64     `json:"victoriametrics_series,omitempty"`
	ObservedAt            *time.Time `json:"observed_at,omitempty"`
	Detail                string     `json:"detail,omitempty"`
}

// Component is one health probe.
type Component struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	LatencyMS  float64   `json:"latency_ms"`
	Detail     string    `json:"detail,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

// Alerting is the dead-man's switch view of the internal evaluator (AD-24,
// AD-32).
type Alerting struct {
	Status               string     `json:"status"`
	Enabled              bool       `json:"enabled"`
	LastEvaluationAt     *time.Time `json:"last_evaluation_at,omitempty"`
	EvaluationAgeSeconds *float64   `json:"evaluation_age_seconds,omitempty"`
	StaleAfterSeconds    float64    `json:"stale_after_seconds,omitempty"`
	Firing               int        `json:"firing"`
	Pending              int        `json:"pending"`
	NotificationsPending int        `json:"notifications_pending"`
	LastError            string     `json:"last_error,omitempty"`
	Detail               string     `json:"detail,omitempty"`
}

// QueryStats is the API request-class snapshot injected into the collector.
type QueryStats struct {
	Requests     uint64
	Errors       uint64
	ClientErrors uint64
	Since        time.Time
}

// QuerySource provides the API query-plane counters.
type QuerySource interface {
	QueryStats() QueryStats
}

// AlertingSource provides the alerting dead-man snapshot.
type AlertingSource interface {
	AlertingSnapshot() Alerting
}
