package ingest

import (
	"math"
	"sort"
	"sync"
	"time"
)

// latencyWindowSize bounds the per-tenant latency reservoir. It is large
// enough for a stable p95 and small enough to keep a recent window: a tenant
// sending 50 requests/s refreshes it every ~5 seconds.
const latencyWindowSize = 256

// tenantLatency keeps a fixed-size ring of recent request durations per
// tenant. Quantiles are computed on scrape, so the gateway can report the
// noisy-neighbor impact without a metrics dependency (AD-10, Story 4.2 AC3).
type tenantLatency struct {
	values [latencyWindowSize]time.Duration
	next   int
	count  int64
}

func (l *tenantLatency) observe(duration time.Duration) {
	l.values[l.next] = duration
	l.next = (l.next + 1) % latencyWindowSize
	l.count++
}

func (l *tenantLatency) snapshot() LatencyQuantiles {
	size := int(l.count)
	if size > latencyWindowSize {
		size = latencyWindowSize
	}
	if size == 0 {
		return LatencyQuantiles{}
	}
	samples := make([]time.Duration, size)
	copy(samples, l.values[:size])
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return LatencyQuantiles{
		Count: l.count,
		P50:   quantile(samples, 0.50),
		P95:   quantile(samples, 0.95),
		P99:   quantile(samples, 0.99),
	}
}

// LatencyQuantiles is a point-in-time latency summary.
type LatencyQuantiles struct {
	Count int64
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
}

func quantile(samples []time.Duration, q float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(samples))*q)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(samples) {
		index = len(samples) - 1
	}
	return samples[index]
}

type latencyWindows struct {
	mu      sync.Mutex
	tenants map[string]*tenantLatency
}

func newLatencyWindows() *latencyWindows {
	return &latencyWindows{tenants: make(map[string]*tenantLatency)}
}

func (w *latencyWindows) observe(tenant string, duration time.Duration) {
	if tenant == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	tracker := w.tenants[tenant]
	if tracker == nil {
		tracker = &tenantLatency{}
		w.tenants[tenant] = tracker
	}
	tracker.observe(duration)
}

func (w *latencyWindows) snapshot() map[string]LatencyQuantiles {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make(map[string]LatencyQuantiles, len(w.tenants))
	for tenant, tracker := range w.tenants {
		result[tenant] = tracker.snapshot()
	}
	return result
}
