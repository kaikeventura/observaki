package storageprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSnapshot(t *testing.T) {
	var clickHouseCalls atomic.Int64
	clickHouse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clickHouseCalls.Add(1)
		if r.URL.Query().Get("database") != "observaki" {
			t.Errorf("database = %q", r.URL.Query().Get("database"))
		}
		_, _ = w.Write([]byte("1024\t4\n"))
	}))
	defer clickHouse.Close()

	var vmCalls atomic.Int64
	victoriaMetrics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vmCalls.Add(1)
		switch r.URL.Path {
		case "/api/v1/status/tsdb":
			_, _ = w.Write([]byte(`{"status":"success","data":{"seriesCountByMetricName":[{"name":"a","value":2},{"name":"b","value":3}]}}`))
		case "/metrics":
			_, _ = w.Write([]byte("# TYPE vm_data_size_bytes gauge\nvm_data_size_bytes{type=\"indexdb\"} 128\nvm_data_size_bytes{type=\"storage\"} 2048\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer victoriaMetrics.Close()

	probe := New(Config{
		ClickHouseURL:      clickHouse.URL,
		ClickHouseDatabase: "observaki",
		VictoriaMetricsURL: victoriaMetrics.URL,
		CacheTTL:           time.Minute,
	})
	stats := probe.Snapshot(context.Background())
	if !stats.ClickHouse.OK || stats.ClickHouse.Bytes == nil || *stats.ClickHouse.Bytes != 1024 {
		t.Fatalf("ClickHouse stats = %+v", stats.ClickHouse)
	}
	if stats.ClickHouse.Parts == nil || *stats.ClickHouse.Parts != 4 {
		t.Fatalf("ClickHouse parts = %+v", stats.ClickHouse.Parts)
	}
	if !stats.VictoriaMetrics.OK || stats.VictoriaMetrics.Series == nil || *stats.VictoriaMetrics.Series != 5 {
		t.Fatalf("VictoriaMetrics series = %+v", stats.VictoriaMetrics)
	}
	if stats.VictoriaMetrics.Bytes == nil || *stats.VictoriaMetrics.Bytes != 2176 {
		t.Fatalf("VictoriaMetrics bytes = %+v", stats.VictoriaMetrics.Bytes)
	}

	// A second call inside the TTL is served from the cache.
	_ = probe.Snapshot(context.Background())
	if clickHouseCalls.Load() != 1 || vmCalls.Load() != 2 {
		t.Fatalf("cached call issued requests: clickhouse=%d vm=%d", clickHouseCalls.Load(), vmCalls.Load())
	}
}

func TestSnapshotReportsBackendErrors(t *testing.T) {
	clickHouse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer clickHouse.Close()

	probe := New(Config{ClickHouseURL: clickHouse.URL, ClickHouseDatabase: "observaki"})
	stats := probe.Snapshot(context.Background())
	if stats.ClickHouse.OK {
		t.Fatalf("ClickHouse OK = true on 500")
	}
	if !strings.Contains(stats.ClickHouse.Error, "HTTP 500") {
		t.Fatalf("ClickHouse error = %q", stats.ClickHouse.Error)
	}
	if stats.VictoriaMetrics.OK {
		t.Fatalf("VictoriaMetrics OK = true when not configured")
	}
}

func TestParseVMDataSize(t *testing.T) {
	total, found := parseVMDataSize(strings.NewReader("vm_data_size_bytes{type=\"storage\"} 10\nother 5\nvm_data_size_bytes 2.5\n"))
	if !found || total != 12 {
		t.Fatalf("parseVMDataSize() = %d, %v; want 12, true", total, found)
	}
}
