package loadgen

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RunMetrics collects live counters and latency samples for a generator run.
type RunMetrics struct {
	startedAtUnixNano atomic.Int64
	targetRate        int

	generated [3]atomic.Int64
	sent      [3]atomic.Int64
	failed    [3]atomic.Int64
	rejected  [3]atomic.Int64
	requests  [3]atomic.Int64
	bytes     [3]atomic.Int64

	backpressureEvents    atomic.Int64
	backpressureWaitNanos atomic.Int64
	pacingLagEvents       atomic.Int64

	latency *LatencyRecorder
}

// NewRunMetrics creates a metrics registry for a generator run.
func NewRunMetrics(targetRate int) *RunMetrics {
	metrics := &RunMetrics{
		targetRate: targetRate,
		latency:    NewLatencyRecorder(100_000),
	}
	metrics.MarkStart(time.Now())
	return metrics
}

// MarkStart resets the run start time.
func (m *RunMetrics) MarkStart(start time.Time) {
	if m == nil {
		return
	}
	m.startedAtUnixNano.Store(start.UnixNano())
}

// AddGenerated records events built by the generator.
func (m *RunMetrics) AddGenerated(signal Signal, events int64) {
	if m == nil {
		return
	}
	m.generated[signal].Add(events)
}

// AddBackpressure records events delayed because the send queue was full.
func (m *RunMetrics) AddBackpressure(events int64, wait time.Duration) {
	if m == nil {
		return
	}
	m.backpressureEvents.Add(events)
	m.backpressureWaitNanos.Add(int64(wait))
}

// AddPacingLag records events the generator could not produce on schedule.
func (m *RunMetrics) AddPacingLag(events int64) {
	if m == nil || events <= 0 {
		return
	}
	m.pacingLagEvents.Add(events)
}

// Record records one export request.
func (m *RunMetrics) Record(signal Signal, events int, result SendResult, err error, latency time.Duration) {
	if m == nil {
		return
	}
	m.requests[signal].Add(1)
	m.latency.Record(latency)
	if err != nil {
		m.failed[signal].Add(int64(events))
		return
	}
	accepted := events - result.Rejected
	if accepted < 0 {
		accepted = 0
	}
	m.sent[signal].Add(int64(accepted))
	m.bytes[signal].Add(int64(result.Bytes))
	if result.Rejected > 0 {
		m.rejected[signal].Add(int64(result.Rejected))
	}
}

// SignalMetrics is the per-signal snapshot.
type SignalMetrics struct {
	Signal    string `json:"signal"`
	Generated int64  `json:"generated"`
	Sent      int64  `json:"sent"`
	Failed    int64  `json:"failed"`
	Rejected  int64  `json:"rejected"`
	Requests  int64  `json:"requests"`
	Bytes     int64  `json:"bytes"`
}

// MetricsSnapshot is a point-in-time view of a run.
type MetricsSnapshot struct {
	StartedAt               time.Time       `json:"started_at"`
	ElapsedSeconds          float64         `json:"elapsed_seconds"`
	TargetRate              int             `json:"target_rate"`
	AchievedRate            float64         `json:"achieved_rate"`
	Generated               int64           `json:"generated"`
	Sent                    int64           `json:"sent"`
	Failed                  int64           `json:"failed"`
	Rejected                int64           `json:"rejected"`
	BackpressureEvents      int64           `json:"backpressure_events"`
	BackpressureWaitSeconds float64         `json:"backpressure_wait_seconds"`
	PacingLagEvents         int64           `json:"pacing_lag_events"`
	Latency                 LatencySummary  `json:"latency"`
	Signals                 []SignalMetrics `json:"signals"`
}

// Snapshot returns a consistent-enough view for reports and /metrics.
func (m *RunMetrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	startedAt := time.Unix(0, m.startedAtUnixNano.Load()).UTC()
	elapsed := time.Since(startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	snapshot := MetricsSnapshot{
		StartedAt:               startedAt,
		ElapsedSeconds:          elapsed.Seconds(),
		TargetRate:              m.targetRate,
		BackpressureEvents:      m.backpressureEvents.Load(),
		BackpressureWaitSeconds: float64(m.backpressureWaitNanos.Load()) / float64(time.Second),
		PacingLagEvents:         m.pacingLagEvents.Load(),
		Latency:                 m.latency.Summary(),
		Signals:                 make([]SignalMetrics, 0, len(AllSignals)),
	}
	for _, signal := range AllSignals {
		item := SignalMetrics{
			Signal:    signal.String(),
			Generated: m.generated[signal].Load(),
			Sent:      m.sent[signal].Load(),
			Failed:    m.failed[signal].Load(),
			Rejected:  m.rejected[signal].Load(),
			Requests:  m.requests[signal].Load(),
			Bytes:     m.bytes[signal].Load(),
		}
		snapshot.Generated += item.Generated
		snapshot.Sent += item.Sent
		snapshot.Failed += item.Failed
		snapshot.Rejected += item.Rejected
		snapshot.Signals = append(snapshot.Signals, item)
	}
	if snapshot.ElapsedSeconds > 0 {
		snapshot.AchievedRate = float64(snapshot.Generated) / snapshot.ElapsedSeconds
	}
	return snapshot
}

// Handler exposes Prometheus-style metrics at /metrics.
func (m *RunMetrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshot := m.Snapshot()
		var builder strings.Builder

		builder.WriteString("# HELP observaki_loadgen_events_generated_total Events generated by the workload.\n")
		builder.WriteString("# TYPE observaki_loadgen_events_generated_total counter\n")
		for _, signal := range AllSignals {
			fmt.Fprintf(&builder, "observaki_loadgen_events_generated_total{signal=%q} %d\n", signal.String(), generatedFor(snapshot, signal))
		}

		builder.WriteString("# HELP observaki_loadgen_events_sent_total Events accepted by the receiver.\n")
		builder.WriteString("# TYPE observaki_loadgen_events_sent_total counter\n")
		for _, signal := range AllSignals {
			fmt.Fprintf(&builder, "observaki_loadgen_events_sent_total{signal=%q} %d\n", signal.String(), metricFor(snapshot, signal).Sent)
		}

		builder.WriteString("# HELP observaki_loadgen_events_failed_total Events dropped by transport errors.\n")
		builder.WriteString("# TYPE observaki_loadgen_events_failed_total counter\n")
		for _, signal := range AllSignals {
			fmt.Fprintf(&builder, "observaki_loadgen_events_failed_total{signal=%q} %d\n", signal.String(), metricFor(snapshot, signal).Failed)
		}

		builder.WriteString("# HELP observaki_loadgen_events_rejected_total Events rejected through OTLP partial success.\n")
		builder.WriteString("# TYPE observaki_loadgen_events_rejected_total counter\n")
		for _, signal := range AllSignals {
			fmt.Fprintf(&builder, "observaki_loadgen_events_rejected_total{signal=%q} %d\n", signal.String(), metricFor(snapshot, signal).Rejected)
		}

		builder.WriteString("# HELP observaki_loadgen_requests_total OTLP export requests.\n")
		builder.WriteString("# TYPE observaki_loadgen_requests_total counter\n")
		for _, signal := range AllSignals {
			fmt.Fprintf(&builder, "observaki_loadgen_requests_total{signal=%q} %d\n", signal.String(), metricFor(snapshot, signal).Requests)
		}

		builder.WriteString("# HELP observaki_loadgen_bytes_total Uncompressed protobuf bytes in requests.\n")
		builder.WriteString("# TYPE observaki_loadgen_bytes_total counter\n")
		for _, signal := range AllSignals {
			fmt.Fprintf(&builder, "observaki_loadgen_bytes_total{signal=%q} %d\n", signal.String(), metricFor(snapshot, signal).Bytes)
		}

		builder.WriteString("# HELP observaki_loadgen_send_latency_seconds OTLP export request latency.\n")
		builder.WriteString("# TYPE observaki_loadgen_send_latency_seconds summary\n")
		fmt.Fprintf(&builder, "observaki_loadgen_send_latency_seconds{quantile=\"0.5\"} %.9f\n", snapshot.Latency.P50Ms/1000)
		fmt.Fprintf(&builder, "observaki_loadgen_send_latency_seconds{quantile=\"0.95\"} %.9f\n", snapshot.Latency.P95Ms/1000)
		fmt.Fprintf(&builder, "observaki_loadgen_send_latency_seconds{quantile=\"0.99\"} %.9f\n", snapshot.Latency.P99Ms/1000)
		fmt.Fprintf(&builder, "observaki_loadgen_send_latency_seconds_sum %.9f\n", snapshot.Latency.MeanMs*float64(snapshot.Latency.Count)/1000)
		fmt.Fprintf(&builder, "observaki_loadgen_send_latency_seconds_count %d\n", snapshot.Latency.Count)

		builder.WriteString("# HELP observaki_loadgen_target_rate Requested events per second.\n")
		builder.WriteString("# TYPE observaki_loadgen_target_rate gauge\n")
		fmt.Fprintf(&builder, "observaki_loadgen_target_rate %d\n", snapshot.TargetRate)

		builder.WriteString("# HELP observaki_loadgen_achieved_rate Generated events per second.\n")
		builder.WriteString("# TYPE observaki_loadgen_achieved_rate gauge\n")
		fmt.Fprintf(&builder, "observaki_loadgen_achieved_rate %.3f\n", snapshot.AchievedRate)

		builder.WriteString("# HELP observaki_loadgen_backpressure_events_total Events delayed by a full send queue.\n")
		builder.WriteString("# TYPE observaki_loadgen_backpressure_events_total counter\n")
		fmt.Fprintf(&builder, "observaki_loadgen_backpressure_events_total %d\n", snapshot.BackpressureEvents)

		builder.WriteString("# HELP observaki_loadgen_backpressure_wait_seconds_total Time spent waiting on a full send queue.\n")
		builder.WriteString("# TYPE observaki_loadgen_backpressure_wait_seconds_total counter\n")
		fmt.Fprintf(&builder, "observaki_loadgen_backpressure_wait_seconds_total %.9f\n", snapshot.BackpressureWaitSeconds)

		builder.WriteString("# HELP observaki_loadgen_pacing_lag_events_total Events the generator could not produce on schedule.\n")
		builder.WriteString("# TYPE observaki_loadgen_pacing_lag_events_total counter\n")
		fmt.Fprintf(&builder, "observaki_loadgen_pacing_lag_events_total %d\n", snapshot.PacingLagEvents)

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(builder.String()))
	})
}

func metricFor(snapshot MetricsSnapshot, signal Signal) SignalMetrics {
	for _, item := range snapshot.Signals {
		if item.Signal == signal.String() {
			return item
		}
	}
	return SignalMetrics{Signal: signal.String()}
}

func generatedFor(snapshot MetricsSnapshot, signal Signal) int64 {
	return metricFor(snapshot, signal).Generated
}

// LatencySummary is a latency distribution in milliseconds.
type LatencySummary struct {
	Count  int64   `json:"count"`
	MinMs  float64 `json:"min_ms"`
	MeanMs float64 `json:"mean_ms"`
	P50Ms  float64 `json:"p50_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	MaxMs  float64 `json:"max_ms"`
}

// LatencyRecorder keeps a fixed-size reservoir plus exact min/max/mean.
type LatencyRecorder struct {
	mu       sync.Mutex
	capacity int
	values   []time.Duration
	next     int
	count    int64
	sum      time.Duration
	min      time.Duration
	max      time.Duration
}

// NewLatencyRecorder creates a recorder with the given reservoir capacity.
func NewLatencyRecorder(capacity int) *LatencyRecorder {
	if capacity < 1 {
		capacity = 1
	}
	return &LatencyRecorder{capacity: capacity, values: make([]time.Duration, 0, capacity)}
}

// Record adds one latency sample.
func (r *LatencyRecorder) Record(value time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count == 0 || value < r.min {
		r.min = value
	}
	if value > r.max {
		r.max = value
	}
	r.sum += value
	if len(r.values) < r.capacity {
		r.values = append(r.values, value)
	} else {
		r.values[r.next] = value
		r.next = (r.next + 1) % r.capacity
	}
	r.count++
}

// Summary returns the current distribution.
func (r *LatencyRecorder) Summary() LatencySummary {
	if r == nil {
		return LatencySummary{}
	}
	r.mu.Lock()
	count := r.count
	values := make([]time.Duration, len(r.values))
	copy(values, r.values)
	sum := r.sum
	minValue := r.min
	maxValue := r.max
	r.mu.Unlock()

	summary := LatencySummary{Count: count}
	if count == 0 || len(values) == 0 {
		return summary
	}
	summary.MinMs = float64(minValue) / float64(time.Millisecond)
	summary.MaxMs = float64(maxValue) / float64(time.Millisecond)
	summary.MeanMs = float64(sum) / float64(count) / float64(time.Millisecond)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	summary.P50Ms = quantileMs(values, 0.50)
	summary.P95Ms = quantileMs(values, 0.95)
	summary.P99Ms = quantileMs(values, 0.99)
	return summary
}

func quantileMs(values []time.Duration, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * quantile)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return float64(values[index]) / float64(time.Millisecond)
}
