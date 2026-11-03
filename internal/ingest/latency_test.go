package ingest

import (
	"testing"
	"time"
)

func TestLatencyWindows(t *testing.T) {
	windows := newLatencyWindows()
	for i := 1; i <= 100; i++ {
		windows.observe("tenant-a", time.Duration(i)*time.Millisecond)
	}
	for i := 1; i <= 10; i++ {
		windows.observe("tenant-b", time.Duration(i)*10*time.Millisecond)
	}
	windows.observe("", time.Hour)

	snapshot := windows.snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("snapshot len = %d, want 2", len(snapshot))
	}
	a := snapshot["tenant-a"]
	if a.Count != 100 {
		t.Errorf("tenant-a count = %d, want 100", a.Count)
	}
	if a.P50 != 50*time.Millisecond {
		t.Errorf("tenant-a p50 = %v, want 50ms", a.P50)
	}
	if a.P95 != 95*time.Millisecond {
		t.Errorf("tenant-a p95 = %v, want 95ms", a.P95)
	}
	if _, ok := snapshot[""]; ok {
		t.Error("empty tenant was recorded")
	}
}

func TestLatencyWindowWraps(t *testing.T) {
	tracker := &tenantLatency{}
	for i := 0; i < latencyWindowSize+50; i++ {
		tracker.observe(time.Duration(i) * time.Millisecond)
	}
	snapshot := tracker.snapshot()
	if snapshot.Count != int64(latencyWindowSize+50) {
		t.Errorf("count = %d, want %d", snapshot.Count, latencyWindowSize+50)
	}
	if snapshot.P50 < 50*time.Millisecond {
		t.Errorf("p50 = %v, want at least 50ms (the window must keep the newest samples)", snapshot.P50)
	}
}
