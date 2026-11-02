//go:build linux

package resources

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseCPUTimes(t *testing.T) {
	fixture := "cpu  100 20 30 800 50 10 5 0 0 0\ncpu0 50 10 15 400 25 5 2 0 0 0\n"
	total, idle, err := parseCPUTimes([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if want := uint64(1015); total != want {
		t.Fatalf("total = %d, want %d", total, want)
	}
	if want := uint64(850); idle != want {
		t.Fatalf("idle = %d, want %d", idle, want)
	}
}

func TestParseCPUTimesRejectsMalformedInput(t *testing.T) {
	if _, _, err := parseCPUTimes([]byte("intr 0\n")); err == nil {
		t.Fatal("parseCPUTimes() = nil, want an error")
	}
}

func TestParseMemInfo(t *testing.T) {
	fixture := "MemTotal:       16384000 kB\nMemFree:         1000000 kB\nMemAvailable:    8192000 kB\n"
	total, available, err := parseMemInfo([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if want := uint64(16384000) * 1024; total != want {
		t.Fatalf("total = %d, want %d", total, want)
	}
	if want := uint64(8192000) * 1024; available != want {
		t.Fatalf("available = %d, want %d", available, want)
	}
}

func TestStatsFor(t *testing.T) {
	stats := StatsFor([]float64{10, 20, 30, 40, 50})
	if stats.Min != 10 || stats.Max != 50 {
		t.Fatalf("stats = %+v, want min 10 and max 50", stats)
	}
	if stats.Mean != 30 {
		t.Fatalf("mean = %f, want 30", stats.Mean)
	}
	if stats.P95 != 50 {
		t.Fatalf("p95 = %f, want 50", stats.P95)
	}
	if empty := StatsFor(nil); empty != (Stats{}) {
		t.Fatalf("empty stats = %+v, want zero", empty)
	}
}

func TestSummarizeAvailability(t *testing.T) {
	samples := []Sample{
		{CPUPercent: 10, CPUAvailable: true, MemoryUsedBytes: 100, MemoryTotalBytes: 1000, MemoryUsedPct: 10, MemoryAvailable: true, DiskFreeBytes: 900, DiskTotalBytes: 1000, DiskAvailable: true},
		{CPUPercent: 30, CPUAvailable: true, MemoryUsedBytes: 200, MemoryTotalBytes: 1000, MemoryUsedPct: 20, MemoryAvailable: true, DiskFreeBytes: 800, DiskTotalBytes: 1000, DiskAvailable: true},
	}
	summary := Summarize(samples, time.Second, time.Unix(0, 0), time.Unix(2, 0), nil)
	if !summary.Supported || !summary.CPUAvailable || !summary.MemoryAvailable || !summary.DiskAvailable {
		t.Fatalf("summary availability = %+v", summary)
	}
	if summary.Samples != 2 || summary.CPUPercent.Mean != 20 {
		t.Fatalf("summary = %+v, want 2 samples and mean 20", summary)
	}
	if summary.DiskFreeBytesStart != 900 || summary.DiskFreeBytesEnd != 800 {
		t.Fatalf("disk = %d/%d, want 900/800", summary.DiskFreeBytesStart, summary.DiskFreeBytesEnd)
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	summary := Summary{SchemaVersion: schemaVersion, Supported: true, Samples: 1, CPUAvailable: true}
	samples := []Sample{{CPUPercent: 25, CPUAvailable: true}}
	path, err := Write(dir, summary, samples)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "resources.json" {
		t.Fatalf("path = %s, want resources.json", path)
	}
	file, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Summary.Samples != 1 || len(file.Samples) != 1 || file.Samples[0].CPUPercent != 25 {
		t.Fatalf("round trip = %+v", file)
	}
}

func TestReadRejectsUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.json")
	if err := os.WriteFile(path, []byte(`{"schema_version": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("Read() = nil, want a schema error")
	}
}

func TestChildUsageDelta(t *testing.T) {
	before := ChildUsage{Available: true, UserSeconds: 1, SystemSeconds: 0.5, MaxRSSBytes: 10}
	after := ChildUsage{Available: true, UserSeconds: 2, SystemSeconds: 1, MaxRSSBytes: 20}
	children := before.Delta(after, 2*time.Second, 2)
	if !children.Available {
		t.Fatal("children usage is not available")
	}
	if children.CPUPercent != 37.5 {
		t.Fatalf("cpu = %f, want 37.5", children.CPUPercent)
	}
	if children.MaxRSSBytes != 20 {
		t.Fatalf("max rss = %d, want 20", children.MaxRSSBytes)
	}
}
