package walconsumer

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/observaki/observaki/internal/ingest"
)

var metricSignals = [...]ingest.Signal{ingest.SignalTraces, ingest.SignalLogs, ingest.SignalMetrics}

func signalIndex(signal ingest.Signal) int {
	switch signal {
	case ingest.SignalTraces:
		return 0
	case ingest.SignalLogs:
		return 1
	case ingest.SignalMetrics:
		return 2
	default:
		return 0
	}
}

type tenantSignalMetrics struct {
	accepted atomic.Uint64
	written  atomic.Uint64
	deduped  atomic.Uint64
	failed   atomic.Uint64
	skipped  atomic.Uint64
	retries  atomic.Uint64
	lag      *durationWindow
}

type writerMetrics struct {
	mu      sync.RWMutex
	tenants map[string]map[ingest.Signal]*tenantSignalMetrics

	messagesFetched [3]atomic.Uint64
	dlqMessages     [3]atomic.Uint64
	consumerPending [3]atomic.Int64
	inFlightEvents  atomic.Int64
	flushDuration   [3]*durationWindow
	lastErrorUnix   atomic.Int64
	consumerRunning atomic.Bool
	connected       atomic.Bool
}

func newWriterMetrics() *writerMetrics {
	m := &writerMetrics{tenants: make(map[string]map[ingest.Signal]*tenantSignalMetrics)}
	for index := range m.flushDuration {
		m.flushDuration[index] = newDurationWindow(4096)
	}
	return m
}

func (m *writerMetrics) tenant(tenantID string, signal ingest.Signal) *tenantSignalMetrics {
	m.mu.RLock()
	signals, ok := m.tenants[tenantID]
	if ok {
		metrics, ok := signals[signal]
		if ok {
			m.mu.RUnlock()
			return metrics
		}
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	signals = m.tenants[tenantID]
	if signals == nil {
		signals = make(map[ingest.Signal]*tenantSignalMetrics)
		m.tenants[tenantID] = signals
	}
	metrics := signals[signal]
	if metrics == nil {
		metrics = &tenantSignalMetrics{lag: newDurationWindow(4096)}
		signals[signal] = metrics
	}
	return metrics
}

func (m *writerMetrics) observeAccepted(tenantID string, signal ingest.Signal, events int) {
	messages := m.tenant(tenantID, signal)
	m.messagesFetched[signalIndex(signal)].Add(1)
	messages.accepted.Add(uint64(events))
}

func (m *writerMetrics) observeResult(tenantID string, signal ingest.Signal, result WriteResult, lag time.Duration) {
	metrics := m.tenant(tenantID, signal)
	metrics.written.Add(uint64(result.Written))
	metrics.deduped.Add(uint64(result.Deduped))
	metrics.failed.Add(uint64(result.Failed))
	metrics.skipped.Add(uint64(result.Skipped))
	metrics.retries.Add(uint64(result.Retries))
	if lag > 0 {
		metrics.lag.observe(lag)
	}
}

func (m *writerMetrics) observeLag(tenantID string, signal ingest.Signal, lag time.Duration) {
	if lag <= 0 {
		return
	}
	m.tenant(tenantID, signal).lag.observe(lag)
}

func (m *writerMetrics) observeFlush(signal ingest.Signal, duration time.Duration) {
	m.flushDuration[signalIndex(signal)].observe(duration)
}

func (m *writerMetrics) observeRetry(tenantID string, signal ingest.Signal) {
	m.tenant(tenantID, signal).retries.Add(1)
}

func (m *writerMetrics) observeDLQ(signal ingest.Signal) {
	m.dlqMessages[signalIndex(signal)].Add(1)
}

func (m *writerMetrics) setPending(signal ingest.Signal, pending int64) {
	m.consumerPending[signalIndex(signal)].Store(pending)
}

func (m *writerMetrics) observeError(err error) {
	if err == nil {
		return
	}
	m.lastErrorUnix.Store(time.Now().Unix())
}

// Stats is the JSON snapshot used by /stats and by the benchmark harness.
type Stats struct {
	Up             bool                   `json:"up"`
	Connected      bool                   `json:"connected"`
	InFlightEvents int64                  `json:"in_flight_events"`
	LastErrorAt    string                 `json:"last_error_at,omitempty"`
	Tenants        []TenantStats          `json:"tenants"`
	Signals        map[string]SignalStats `json:"signals"`
}

// TenantStats is one tenant/signal row.
type TenantStats struct {
	TenantID string  `json:"tenant_id"`
	Signal   string  `json:"signal"`
	Accepted uint64  `json:"accepted_events"`
	Written  uint64  `json:"written_events"`
	Deduped  uint64  `json:"deduped_events"`
	Failed   uint64  `json:"failed_events"`
	Skipped  uint64  `json:"skipped_events"`
	Retries  uint64  `json:"retries"`
	LagP50MS float64 `json:"lag_p50_ms"`
	LagP95MS float64 `json:"lag_p95_ms"`
	LagP99MS float64 `json:"lag_p99_ms"`
}

// SignalStats is one signal-level row.
type SignalStats struct {
	MessagesFetched uint64 `json:"messages_fetched"`
	DLQMessages     uint64 `json:"dlq_messages"`
	PendingMessages int64  `json:"pending_messages"`
}

func (m *writerMetrics) snapshot() Stats {
	stats := Stats{
		Up:             m.consumerRunning.Load(),
		Connected:      m.connected.Load(),
		InFlightEvents: m.inFlightEvents.Load(),
		Signals:        make(map[string]SignalStats, len(metricSignals)),
	}
	if unix := m.lastErrorUnix.Load(); unix > 0 {
		stats.LastErrorAt = time.Unix(unix, 0).UTC().Format(time.RFC3339)
	}
	for index, signal := range metricSignals {
		stats.Signals[signal.String()] = SignalStats{
			MessagesFetched: m.messagesFetched[index].Load(),
			DLQMessages:     m.dlqMessages[index].Load(),
			PendingMessages: m.consumerPending[index].Load(),
		}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	tenants := make([]string, 0, len(m.tenants))
	for tenant := range m.tenants {
		tenants = append(tenants, tenant)
	}
	sort.Strings(tenants)
	for _, tenant := range tenants {
		for _, signal := range metricSignals {
			metrics := m.tenants[tenant][signal]
			if metrics == nil {
				continue
			}
			p50, p95, p99 := metrics.lag.snapshot()
			stats.Tenants = append(stats.Tenants, TenantStats{
				TenantID: tenant,
				Signal:   signal.String(),
				Accepted: metrics.accepted.Load(),
				Written:  metrics.written.Load(),
				Deduped:  metrics.deduped.Load(),
				Failed:   metrics.failed.Load(),
				Skipped:  metrics.skipped.Load(),
				Retries:  metrics.retries.Load(),
				LagP50MS: milliseconds(p50),
				LagP95MS: milliseconds(p95),
				LagP99MS: milliseconds(p99),
			})
		}
	}
	return stats
}

func (m *writerMetrics) write(out *strings.Builder) {
	stats := m.snapshot()
	up := 0
	if stats.Up {
		up = 1
	}
	connected := 0
	if stats.Connected {
		connected = 1
	}
	out.WriteString("# HELP observaki_writer_up Whether the WAL writer workers are running.\n")
	out.WriteString("# TYPE observaki_writer_up gauge\n")
	fmt.Fprintf(out, "observaki_writer_up %d\n", up)
	out.WriteString("# HELP observaki_writer_wal_connected Whether the NATS connection is up.\n")
	out.WriteString("# TYPE observaki_writer_wal_connected gauge\n")
	fmt.Fprintf(out, "observaki_writer_wal_connected %d\n", connected)
	out.WriteString("# HELP observaki_writer_in_flight_events Events fetched from the WAL and not yet acknowledged.\n")
	out.WriteString("# TYPE observaki_writer_in_flight_events gauge\n")
	fmt.Fprintf(out, "observaki_writer_in_flight_events %d\n", stats.InFlightEvents)
	if unix := m.lastErrorUnix.Load(); unix > 0 {
		out.WriteString("# HELP observaki_writer_last_error_timestamp_seconds Unix time of the most recent write error.\n")
		out.WriteString("# TYPE observaki_writer_last_error_timestamp_seconds gauge\n")
		fmt.Fprintf(out, "observaki_writer_last_error_timestamp_seconds %d\n", unix)
	}

	out.WriteString("# HELP observaki_writer_messages_fetched_total WAL envelopes fetched per signal.\n")
	out.WriteString("# TYPE observaki_writer_messages_fetched_total counter\n")
	for index, signal := range metricSignals {
		fmt.Fprintf(out, "observaki_writer_messages_fetched_total{signal=%q} %d\n", signal, m.messagesFetched[index].Load())
	}
	out.WriteString("# HELP observaki_writer_dlq_messages_total Messages moved to the dead-letter stream.\n")
	out.WriteString("# TYPE observaki_writer_dlq_messages_total counter\n")
	for index, signal := range metricSignals {
		fmt.Fprintf(out, "observaki_writer_dlq_messages_total{signal=%q} %d\n", signal, m.dlqMessages[index].Load())
	}
	out.WriteString("# HELP observaki_writer_consumer_pending_messages WAL messages waiting for this consumer group.\n")
	out.WriteString("# TYPE observaki_writer_consumer_pending_messages gauge\n")
	for index, signal := range metricSignals {
		fmt.Fprintf(out, "observaki_writer_consumer_pending_messages{signal=%q} %d\n", signal, m.consumerPending[index].Load())
	}

	writeTenantFamily(out, "observaki_writer_accepted_events_total", "Events fetched from the WAL.", "counter", stats, func(row TenantStats) uint64 { return row.Accepted })
	writeTenantFamily(out, "observaki_writer_written_events_total", "Events confirmed by the storage backend.", "counter", stats, func(row TenantStats) uint64 { return row.Written })
	writeTenantFamily(out, "observaki_writer_deduped_events_total", "Events dropped by the event_id window or the stable token.", "counter", stats, func(row TenantStats) uint64 { return row.Deduped })
	writeTenantFamily(out, "observaki_writer_failed_events_total", "Events moved to the dead-letter queue.", "counter", stats, func(row TenantStats) uint64 { return row.Failed })
	writeTenantFamily(out, "observaki_writer_skipped_events_total", "Data points with a conversion that the MVP path does not support.", "counter", stats, func(row TenantStats) uint64 { return row.Skipped })
	writeTenantFamily(out, "observaki_writer_retries_total", "Downstream write attempts that were retried.", "counter", stats, func(row TenantStats) uint64 { return row.Retries })

	out.WriteString("# HELP observaki_writer_lag_seconds End-to-end lag per tenant, signal and quantile.\n")
	out.WriteString("# TYPE observaki_writer_lag_seconds gauge\n")
	for _, row := range stats.Tenants {
		fmt.Fprintf(out, "observaki_writer_lag_seconds{tenant_id=%q,signal=%q,quantile=\"0.5\"} %.6f\n", row.TenantID, row.Signal, row.LagP50MS/1000)
		fmt.Fprintf(out, "observaki_writer_lag_seconds{tenant_id=%q,signal=%q,quantile=\"0.95\"} %.6f\n", row.TenantID, row.Signal, row.LagP95MS/1000)
		fmt.Fprintf(out, "observaki_writer_lag_seconds{tenant_id=%q,signal=%q,quantile=\"0.99\"} %.6f\n", row.TenantID, row.Signal, row.LagP99MS/1000)
	}

	out.WriteString("# HELP observaki_writer_flush_duration_seconds Time from the first message in a batch to its acknowledgement.\n")
	out.WriteString("# TYPE observaki_writer_flush_duration_seconds gauge\n")
	for index, signal := range metricSignals {
		p50, p95, p99 := m.flushDuration[index].snapshot()
		fmt.Fprintf(out, "observaki_writer_flush_duration_seconds{signal=%q,quantile=\"0.5\"} %.6f\n", signal, p50.Seconds())
		fmt.Fprintf(out, "observaki_writer_flush_duration_seconds{signal=%q,quantile=\"0.95\"} %.6f\n", signal, p95.Seconds())
		fmt.Fprintf(out, "observaki_writer_flush_duration_seconds{signal=%q,quantile=\"0.99\"} %.6f\n", signal, p99.Seconds())
	}
}

func writeTenantFamily(
	out *strings.Builder,
	name, help, kind string,
	stats Stats,
	value func(TenantStats) uint64,
) {
	fmt.Fprintf(out, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
	for _, row := range stats.Tenants {
		fmt.Fprintf(out, "%s{tenant_id=%q,signal=%q} %d\n", name, row.TenantID, row.Signal, value(row))
	}
}

// durationWindow is a fixed-size ring of durations.
type durationWindow struct {
	mu     sync.Mutex
	values []time.Duration
	next   int
	count  int
}

func newDurationWindow(size int) *durationWindow {
	return &durationWindow{values: make([]time.Duration, size)}
}

func (w *durationWindow) observe(value time.Duration) {
	if value < 0 {
		value = 0
	}
	w.mu.Lock()
	w.values[w.next] = value
	w.next = (w.next + 1) % len(w.values)
	if w.count < len(w.values) {
		w.count++
	}
	w.mu.Unlock()
}

func (w *durationWindow) snapshot() (p50, p95, p99 time.Duration) {
	w.mu.Lock()
	values := make([]time.Duration, w.count)
	copy(values, w.values[:w.count])
	w.mu.Unlock()
	if len(values) == 0 {
		return 0, 0, 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return quantile(values, 0.50), quantile(values, 0.95), quantile(values, 0.99)
}

func quantile(values []time.Duration, q float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * q)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func milliseconds(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}
