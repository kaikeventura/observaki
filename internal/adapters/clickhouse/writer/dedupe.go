package writer

import "sync/atomic"

// dedupeWindow keeps the most recently accepted event identities. It is a
// bounded FIFO: when the window is full the oldest identity is evicted. The
// window is intentionally in-memory and non-durable; a batch that survives a
// restart is protected by its stable insert_deduplication_token instead. A
// durable cross-restart idempotency index is a Phase 1 concern (FR-4).
type dedupeWindow struct {
	capacity int
	seen     map[dedupeKey]struct{}
	ring     []dedupeKey
	next     int
	count    atomic.Int64
}

type dedupeKey struct {
	tenantID  string
	projectID string
	signal    string
	eventID   string
}

func newDedupeWindow(capacity int) *dedupeWindow {
	if capacity < 0 {
		capacity = 0
	}
	return &dedupeWindow{
		capacity: capacity,
		seen:     make(map[dedupeKey]struct{}, capacity),
		ring:     make([]dedupeKey, 0, capacity),
	}
}

// add records key and reports whether it is new. A zero-capacity window
// disables deduplication and always reports true.
func (d *dedupeWindow) add(key dedupeKey) bool {
	if d.capacity == 0 {
		return true
	}
	if _, ok := d.seen[key]; ok {
		return false
	}
	if len(d.ring) < d.capacity {
		d.ring = append(d.ring, key)
		d.seen[key] = struct{}{}
		d.count.Add(1)
		return true
	}
	delete(d.seen, d.ring[d.next])
	d.ring[d.next] = key
	d.seen[key] = struct{}{}
	d.next = (d.next + 1) % d.capacity
	return true
}

func (d *dedupeWindow) size() int { return int(d.count.Load()) }
