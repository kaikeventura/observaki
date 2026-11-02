package writer

import (
	"sync/atomic"
	"time"
)

type counters struct {
	submittedBatches  atomic.Uint64
	submittedEvents   atomic.Uint64
	insertedEvents    atomic.Uint64
	duplicateEvents   atomic.Uint64
	clampedTimestamps atomic.Uint64
	ackedBatches      atomic.Uint64
	retriedBatches    atomic.Uint64
	retryAttempts     atomic.Uint64
	recoveredBatches  atomic.Uint64
	recoveredEvents   atomic.Uint64
	dlqEvents         atomic.Uint64
	pendingBatches    atomic.Int64
	pendingBytes      atomic.Int64
	lastAckUnixNano   atomic.Int64
	lastLagNanos      atomic.Int64
	maxLagNanos       atomic.Int64
}

// Stats is a point-in-time snapshot of the writer counters. Submitted means
// durably spooled (the producer can stop worrying); Inserted is the row count
// reported by ClickHouse's HTTP summary. Recovered counts batches and rows
// replayed from the spool after a restart; a replay reuses the original
// insert_deduplication_token, so ClickHouse may drop it. The exact stored-row
// count is measured by the benchmark query, not inferred from the summary.
type Stats struct {
	Table             string  `json:"table"`
	Signal            string  `json:"signal"`
	SubmittedBatches  uint64  `json:"submitted_batches"`
	SubmittedEvents   uint64  `json:"submitted_events"`
	InsertedEvents    uint64  `json:"inserted_events"`
	DuplicateEvents   uint64  `json:"duplicate_events"`
	ClampedTimestamps uint64  `json:"clamped_timestamps"`
	AckedBatches      uint64  `json:"acked_batches"`
	RetriedBatches    uint64  `json:"retried_batches"`
	RetryAttempts     uint64  `json:"retry_attempts"`
	RecoveredBatches  uint64  `json:"recovered_batches"`
	RecoveredEvents   uint64  `json:"recovered_events"`
	DLQEvents         uint64  `json:"dlq_events"`
	PendingBatches    int64   `json:"pending_batches"`
	PendingBytes      int64   `json:"pending_bytes"`
	LastAckAt         string  `json:"last_ack_at,omitempty"`
	LastLagMillis     float64 `json:"last_lag_ms"`
	MaxLagMillis      float64 `json:"max_lag_ms"`
	DedupeWindow      int     `json:"dedupe_window"`
	DedupeWindowUsed  int     `json:"dedupe_window_used"`
	SpoolDir          string  `json:"spool_dir"`
	FatalError        string  `json:"fatal_error,omitempty"`
}

func (c *counters) snapshot(table, signal string, dedupe *dedupeWindow, spoolDir, fatal string) Stats {
	lastAck := c.lastAckUnixNano.Load()
	lastAckAt := ""
	if lastAck > 0 {
		lastAckAt = time.Unix(0, lastAck).UTC().Format(time.RFC3339Nano)
	}
	used := 0
	if dedupe != nil {
		used = dedupe.size()
	}
	return Stats{
		Table:             table,
		Signal:            signal,
		SubmittedBatches:  c.submittedBatches.Load(),
		SubmittedEvents:   c.submittedEvents.Load(),
		InsertedEvents:    c.insertedEvents.Load(),
		DuplicateEvents:   c.duplicateEvents.Load(),
		ClampedTimestamps: c.clampedTimestamps.Load(),
		AckedBatches:      c.ackedBatches.Load(),
		RetriedBatches:    c.retriedBatches.Load(),
		RetryAttempts:     c.retryAttempts.Load(),
		RecoveredBatches:  c.recoveredBatches.Load(),
		RecoveredEvents:   c.recoveredEvents.Load(),
		DLQEvents:         c.dlqEvents.Load(),
		PendingBatches:    c.pendingBatches.Load(),
		PendingBytes:      c.pendingBytes.Load(),
		LastAckAt:         lastAckAt,
		LastLagMillis:     float64(c.lastLagNanos.Load()) / float64(time.Millisecond),
		MaxLagMillis:      float64(c.maxLagNanos.Load()) / float64(time.Millisecond),
		DedupeWindow:      dedupeCapacity(dedupe),
		DedupeWindowUsed:  used,
		SpoolDir:          spoolDir,
		FatalError:        fatal,
	}
}

func (c *counters) observeLag(lag time.Duration) {
	if lag < 0 {
		lag = 0
	}
	c.lastLagNanos.Store(int64(lag))
	c.lastAckUnixNano.Store(time.Now().UnixNano())
	for {
		current := c.maxLagNanos.Load()
		if int64(lag) <= current {
			return
		}
		if c.maxLagNanos.CompareAndSwap(current, int64(lag)) {
			return
		}
	}
}

func dedupeCapacity(window *dedupeWindow) int {
	if window == nil {
		return 0
	}
	return window.capacity
}
