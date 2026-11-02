package main

import (
	"testing"
	"time"

	"github.com/observaki/observaki/internal/adapters/clickhouse/writer"
)

func TestPercentile(t *testing.T) {
	values := []time.Duration{
		10 * time.Millisecond,
		30 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
	}
	tests := []struct {
		p    float64
		want time.Duration
	}{
		{p: 50, want: 20 * time.Millisecond},
		{p: 95, want: 40 * time.Millisecond},
		{p: 100, want: 40 * time.Millisecond},
	}
	for _, test := range tests {
		if got := percentile(values, test.p); got != test.want {
			t.Errorf("percentile(%v, %v) = %v, want %v", values, test.p, got, test.want)
		}
	}
	if got := percentile(nil, 95); got != 0 {
		t.Errorf("percentile(nil, 95) = %v, want 0", got)
	}
}

func TestGeneratorIDsAreUniqueAndNamespaced(t *testing.T) {
	opts := options{
		tenantPrefix: "test-run",
		rate:         100000,
		batchRows:    1000,
		generate:     4,
	}
	generator := newGenerator(opts)
	generator.fill(0, time.Now())
	seen := make(map[string]struct{}, len(generator.records))
	for index, record := range generator.records {
		if record.TenantID != "test-run-0" && record.TenantID != "test-run-1" {
			t.Fatalf("record %d tenant = %q, want the test-run prefix", index, record.TenantID)
		}
		if len(record.EventID) != 32 {
			t.Fatalf("record %d event_id = %q, want 32 hex characters", index, record.EventID)
		}
		if _, ok := seen[record.EventID]; ok {
			t.Fatalf("record %d repeats event_id %q", index, record.EventID)
		}
		seen[record.EventID] = struct{}{}
		if record.TraceID != generator.records[(index/10)*10].TraceID {
			t.Fatalf("record %d breaks the 10-spans-per-trace grouping", index)
		}
	}
	if len(seen) != len(generator.records) {
		t.Fatalf("unique event IDs = %d, want %d", len(seen), len(generator.records))
	}
}

func TestGeneratorNamespacesSeparateRuns(t *testing.T) {
	first := newGenerator(options{tenantPrefix: "run-a", rate: 100000, batchRows: 100, generate: 2})
	second := newGenerator(options{tenantPrefix: "run-b", rate: 100000, batchRows: 100, generate: 2})
	first.fill(0, time.Now())
	second.fill(0, time.Now())
	if first.records[0].EventID == second.records[0].EventID {
		t.Fatal("two runs produced the same event ID; dedupe would corrupt cross-run counts")
	}
}

func TestLoadStatus(t *testing.T) {
	report := loadReport{TargetRate: 400000, ThroughputEPS: 390000, Lag: lagSummary{P95Ms: 1000}}
	if got := loadStatus(report); got != "pass" {
		t.Fatalf("loadStatus() = %q, want pass", got)
	}
	report.ThroughputEPS = 300000
	if got := loadStatus(report); got != "fail" {
		t.Fatalf("low throughput loadStatus() = %q, want fail", got)
	}
	report.ThroughputEPS = 390000
	report.Lag.P95Ms = 12000
	if got := loadStatus(report); got != "fail" {
		t.Fatalf("high lag loadStatus() = %q, want fail", got)
	}
	report.Lag.P95Ms = 1000
	report.Stats.DLQEvents = 1
	if got := loadStatus(report); got != "fail" {
		t.Fatalf("dlq loadStatus() = %q, want fail", got)
	}
}

func TestSummarizeLagsUsesOldestEvent(t *testing.T) {
	acks := []writer.Ack{
		{Lag: 100 * time.Millisecond, OldestLag: 400 * time.Millisecond},
		{Lag: 200 * time.Millisecond, OldestLag: 800 * time.Millisecond},
	}
	summary := summarizeLags(acks)
	if summary.Samples != 2 {
		t.Fatalf("Samples = %d, want 2", summary.Samples)
	}
	if summary.P95Ms != 800 {
		t.Fatalf("P95Ms = %v, want 800 (oldest event)", summary.P95Ms)
	}
	if summary.NewestP95Ms != 200 {
		t.Fatalf("NewestP95Ms = %v, want 200", summary.NewestP95Ms)
	}
}
