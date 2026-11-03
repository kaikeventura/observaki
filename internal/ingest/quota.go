package ingest

import (
	"math"
	"sync"
	"time"
)

// eventLimiter is a per-tenant token bucket measured in events. It also keeps
// a per-tenant payload estimate so HTTP requests can be admitted before they
// are decompressed and decoded: a tenant sending far above its quota is
// rejected after the headers instead of consuming decode CPU (Story 4.2 AC3).
type eventLimiter struct {
	mu        sync.Mutex
	quotas    map[string]QuotaConfig
	buckets   map[string]*eventBucket
	estimates map[string]*payloadEstimate
}

type eventBucket struct {
	tokens float64
	last   time.Time
}

// payloadEstimate is an EWMA of compressed bytes per event. samples==0 means
// the estimate is not trustworthy yet and the first request is decoded.
type payloadEstimate struct {
	bytesPerEvent float64
	samples       int
}

// Reservation is a pre-decode quota charge. The HTTP handler settles it with
// the real event count after decoding, or releases it on an error path.
type Reservation struct {
	tenant   string
	events   int
	bodySize int64
}

const payloadEWMAAlpha = 0.2

func newEventLimiter() *eventLimiter {
	return &eventLimiter{
		quotas:    make(map[string]QuotaConfig),
		buckets:   make(map[string]*eventBucket),
		estimates: make(map[string]*payloadEstimate),
	}
}

// setQuotas swaps the quota table, preserving buckets and estimates for
// tenants that remain and clamping tokens to the new burst.
func (l *eventLimiter) setQuotas(quotas map[string]QuotaConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.quotas = quotas
	for tenant, bucket := range l.buckets {
		quota, ok := quotas[tenant]
		if !ok {
			delete(l.buckets, tenant)
			continue
		}
		if bucket.tokens > float64(quota.BurstEvents) {
			bucket.tokens = float64(quota.BurstEvents)
		}
	}
	for tenant := range l.estimates {
		if _, ok := quotas[tenant]; !ok {
			delete(l.estimates, tenant)
		}
	}
}

// admit consumes events from the tenant's bucket after decode.
func (l *eventLimiter) admit(tenant string, events int, now time.Time) (retryAfter time.Duration, allowed, configured bool) {
	if events <= 0 {
		return 0, true, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.consumeLocked(tenant, events, now)
}

// estimate returns the event estimate for a request body. warm is true when
// the tenant has no payload samples yet, so the caller must decode and settle.
func (l *eventLimiter) estimate(tenant string, bodyBytes int64) (events int, warm, configured bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.quotas[tenant]; !ok {
		return 0, false, false
	}
	estimate := l.estimates[tenant]
	if estimate == nil || estimate.samples == 0 || estimate.bytesPerEvent <= 0 {
		return 0, true, true
	}
	events = int(math.Ceil(float64(bodyBytes) / estimate.bytesPerEvent))
	if events < 1 {
		events = 1
	}
	return events, false, true
}

// reserve charges the estimated events before the body is decoded.
func (l *eventLimiter) reserve(tenant string, events int, bodyBytes int64, now time.Time) (Reservation, time.Duration, bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	retryAfter, allowed, configured := l.consumeLocked(tenant, events, now)
	if !allowed {
		return Reservation{}, retryAfter, false, configured
	}
	return Reservation{tenant: tenant, events: events, bodySize: bodyBytes}, 0, true, true
}

// settle adjusts the reservation to the actual event count and learns the
// tenant's payload size. A request that exceeds the reservation by more than
// the remaining bucket is rejected and fully refunded.
func (l *eventLimiter) settle(reservation Reservation, actual int, now time.Time) (time.Duration, bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.quotas[reservation.tenant]; !ok {
		return 0, false, false
	}
	if actual > reservation.events {
		deficit := actual - reservation.events
		if retryAfter, allowed, configured := l.consumeLocked(reservation.tenant, deficit, now); !allowed || !configured {
			l.refundLocked(reservation)
			return retryAfter, false, configured
		}
	} else if actual < reservation.events {
		l.refundLocked(Reservation{tenant: reservation.tenant, events: reservation.events - actual})
	}
	l.observePayloadLocked(reservation.tenant, reservation.bodySize, actual)
	return 0, true, true
}

// release refunds a reservation that was not settled.
func (l *eventLimiter) release(reservation Reservation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refundLocked(reservation)
}

// recordPayload learns the payload size of a request that skipped the
// pre-decode estimate (no Content-Length or first request).
func (l *eventLimiter) recordPayload(tenant string, bodyBytes int64, events int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.observePayloadLocked(tenant, bodyBytes, events)
}

func (l *eventLimiter) consumeLocked(tenant string, events int, now time.Time) (time.Duration, bool, bool) {
	quota, ok := l.quotas[tenant]
	if !ok {
		return 0, false, false
	}
	bucket, ok := l.buckets[tenant]
	if !ok {
		bucket = &eventBucket{tokens: float64(quota.BurstEvents), last: now}
		l.buckets[tenant] = bucket
	}
	if elapsed := now.Sub(bucket.last).Seconds(); elapsed > 0 {
		bucket.tokens = math.Min(float64(quota.BurstEvents), bucket.tokens+elapsed*quota.EventsPerSecond)
		bucket.last = now
	}
	if float64(events) <= bucket.tokens {
		bucket.tokens -= float64(events)
		return 0, true, true
	}
	deficit := float64(events) - bucket.tokens
	seconds := math.Ceil(deficit / quota.EventsPerSecond)
	if seconds < 1 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second, false, true
}

func (l *eventLimiter) refundLocked(reservation Reservation) {
	quota, ok := l.quotas[reservation.tenant]
	if !ok {
		return
	}
	bucket := l.buckets[reservation.tenant]
	if bucket == nil {
		return
	}
	bucket.tokens = math.Min(float64(quota.BurstEvents), bucket.tokens+float64(reservation.events))
}

func (l *eventLimiter) observePayloadLocked(tenant string, bodyBytes int64, events int) {
	if bodyBytes <= 0 || events <= 0 {
		return
	}
	if _, ok := l.quotas[tenant]; !ok {
		return
	}
	observed := float64(bodyBytes) / float64(events)
	estimate := l.estimates[tenant]
	if estimate == nil {
		estimate = &payloadEstimate{}
		l.estimates[tenant] = estimate
	}
	if estimate.samples == 0 || estimate.bytesPerEvent <= 0 {
		estimate.bytesPerEvent = observed
	} else {
		estimate.bytesPerEvent = payloadEWMAAlpha*observed + (1-payloadEWMAAlpha)*estimate.bytesPerEvent
	}
	estimate.samples++
}
