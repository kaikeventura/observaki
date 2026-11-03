package nats

import (
	"sort"
	"sync"
	"time"

	"github.com/observaki/observaki/internal/ingest"
)

const publishWindowSize = 4096

type publishWindow struct {
	mu     sync.Mutex
	values [publishWindowSize]time.Duration
	next   int
	count  int
}

func (w *publishWindow) observe(value time.Duration) {
	w.mu.Lock()
	w.values[w.next] = value
	w.next = (w.next + 1) % publishWindowSize
	if w.count < publishWindowSize {
		w.count++
	}
	w.mu.Unlock()
}

func (w *publishWindow) snapshot() (p50, p95, p99 time.Duration) {
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

// Stats implements ingest.SinkStatsProvider.
func (s *Sink) Stats() ingest.SinkStats {
	p50, p95, p99 := s.window.snapshot()
	connected := s.nc != nil && s.nc.IsConnected() && !s.isClosed()
	return ingest.SinkStats{
		Connected:        connected,
		PublishedBatches: s.publishedBatches.Load(),
		PublishedEvents:  s.publishedEvents.Load(),
		PublishErrors:    s.publishErrors.Load(),
		PublishRetries:   s.publishRetries.Load(),
		Backpressure:     s.backpressure.Load(),
		PendingBytes:     s.pendingBytes.Load(),
		PendingEvents:    s.pendingEvents.Load(),
		InFlightBatches:  s.inFlight.Load(),
		PublishP50:       p50,
		PublishP95:       p95,
		PublishP99:       p99,
	}
}
