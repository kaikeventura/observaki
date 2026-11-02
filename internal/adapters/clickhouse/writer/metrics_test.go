package writer

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAndStatsHandlers(t *testing.T) {
	w := &Writer{
		cfg:    Config{Table: "otel_logs", Signal: SignalLogs, SpoolDir: "/tmp/spool"},
		dedupe: newDedupeWindow(3),
	}
	w.stats.submittedEvents.Store(5)
	w.stats.duplicateEvents.Store(2)
	w.stats.insertedEvents.Store(3)
	w.dedupe.add(dedupeKey{tenantID: "t", projectID: "p", signal: SignalLogs, eventID: "e"})

	metrics := httptest.NewRecorder()
	w.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if metrics.Code != 200 {
		t.Fatalf("metrics status = %d, want 200", metrics.Code)
	}
	for _, fragment := range []string{
		`observaki_ingest_submitted_events_total{table="otel_logs",signal="logs"} 5`,
		`observaki_ingest_duplicate_events_total{table="otel_logs",signal="logs"} 2`,
		`observaki_ingest_inserted_events_total{table="otel_logs",signal="logs"} 3`,
		`observaki_ingest_dedupe_window_used{table="otel_logs",signal="logs"} 1`,
	} {
		if !strings.Contains(metrics.Body.String(), fragment) {
			t.Errorf("metrics output is missing %q", fragment)
		}
	}

	stats := httptest.NewRecorder()
	w.StatsHandler().ServeHTTP(stats, httptest.NewRequest("GET", "/stats", nil))
	if stats.Code != 200 {
		t.Fatalf("stats status = %d, want 200", stats.Code)
	}
	for _, fragment := range []string{`"table": "otel_logs"`, `"submitted_events": 5`, `"dedupe_window_used": 1`} {
		if !strings.Contains(stats.Body.String(), fragment) {
			t.Errorf("stats output is missing %q", fragment)
		}
	}
}
