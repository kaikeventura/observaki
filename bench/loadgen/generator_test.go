package loadgen

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingSender struct {
	mu      sync.Mutex
	batches []*Batch
	delay   time.Duration
	err     error
}

func (s *recordingSender) Send(ctx context.Context, batch *Batch) (SendResult, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return SendResult{}, ctx.Err()
		}
	}
	if s.err != nil {
		return SendResult{}, s.err
	}
	s.mu.Lock()
	s.batches = append(s.batches, batch)
	s.mu.Unlock()
	return SendResult{Bytes: batch.Size()}, nil
}

func (s *recordingSender) Close() error { return nil }

func (s *recordingSender) snapshot() []*Batch {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*Batch, len(s.batches))
	copy(result, s.batches)
	return result
}

func TestGeneratorCountsAreDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 10_000
	cfg.Duration = 100 * time.Millisecond
	cfg.BatchSize = 100
	cfg.Workers = 2
	cfg.MetricsAddr = ""

	run := func() Summary {
		t.Helper()
		sender := &recordingSender{}
		metrics := NewRunMetrics(cfg.Rate)
		generator, err := NewGenerator(cfg, sender, metrics)
		if err != nil {
			t.Fatal(err)
		}
		summary, err := generator.Run(context.Background())
		if err != nil {
			t.Fatalf("Run() = %v", err)
		}
		if len(sender.snapshot()) == 0 {
			t.Fatal("sender received no batches")
		}
		return summary
	}

	first := run()
	second := run()

	if first.PlannedEvents != 1000 || second.PlannedEvents != 1000 {
		t.Fatalf("planned events = %d and %d, want 1000", first.PlannedEvents, second.PlannedEvents)
	}
	if first.DurationSeconds < 0.08 {
		t.Fatalf("duration = %.3fs, want at least 80ms for a 100ms run", first.DurationSeconds)
	}
	if rate := first.Metrics.AchievedRate; rate < 9_000 || rate > 11_000 {
		t.Fatalf("achieved rate = %.0f/s, want 10000 ±10%%", rate)
	}
	if first.Metrics.Generated != 1000 || first.Metrics.Sent != 1000 {
		t.Fatalf("first run generated/sent = %d/%d, want 1000/1000", first.Metrics.Generated, first.Metrics.Sent)
	}
	if first.Metrics.Failed != 0 || first.Metrics.Rejected != 0 {
		t.Fatalf("first run failed/rejected = %d/%d, want 0/0", first.Metrics.Failed, first.Metrics.Rejected)
	}

	want := map[string]int64{"metrics": 600, "logs": 300, "traces": 100}
	for _, signal := range first.Metrics.Signals {
		if signal.Generated != want[signal.Signal] {
			t.Fatalf("%s generated = %d, want %d", signal.Signal, signal.Generated, want[signal.Signal])
		}
	}
	for i := range first.Metrics.Signals {
		if first.Metrics.Signals[i] != second.Metrics.Signals[i] {
			t.Fatalf("runs differ: %+v vs %+v", first.Metrics.Signals[i], second.Metrics.Signals[i])
		}
	}
}

func TestGeneratorReportsBackpressure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 20_000
	cfg.Duration = 20 * time.Millisecond
	cfg.BatchSize = 10
	cfg.Workers = 1
	cfg.MetricsAddr = ""

	sender := &recordingSender{delay: 2 * time.Millisecond}
	metrics := NewRunMetrics(cfg.Rate)
	generator, err := NewGenerator(cfg, sender, metrics)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := generator.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if summary.Metrics.BackpressureEvents == 0 {
		t.Fatal("BackpressureEvents = 0, want > 0")
	}
	if summary.Metrics.BackpressureWaitSeconds <= 0 {
		t.Fatal("BackpressureWaitSeconds = 0, want > 0")
	}
	if summary.Metrics.Generated != summary.PlannedEvents {
		t.Fatalf("generated = %d, want %d", summary.Metrics.Generated, summary.PlannedEvents)
	}
}

func TestGeneratorAccountsForFailures(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Rate = 1000
	cfg.Duration = 10 * time.Millisecond
	cfg.BatchSize = 5
	cfg.Workers = 1
	cfg.MetricsAddr = ""

	sender := &recordingSender{err: errors.New("receiver unavailable")}
	metrics := NewRunMetrics(cfg.Rate)
	generator, err := NewGenerator(cfg, sender, metrics)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := generator.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if summary.Metrics.Failed != summary.PlannedEvents {
		t.Fatalf("failed = %d, want %d", summary.Metrics.Failed, summary.PlannedEvents)
	}
	if summary.Metrics.Sent != 0 {
		t.Fatalf("sent = %d, want 0", summary.Metrics.Sent)
	}
}
