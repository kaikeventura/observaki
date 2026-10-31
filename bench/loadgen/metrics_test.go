package loadgen

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunMetricsRecord(t *testing.T) {
	metrics := NewRunMetrics(1000)
	metrics.MarkStart(time.Now().Add(-time.Second))
	metrics.AddGenerated(Metrics, 10)
	metrics.Record(Metrics, 10, SendResult{Bytes: 100, Rejected: 2}, nil, 5*time.Millisecond)
	metrics.AddGenerated(Logs, 5)
	metrics.Record(Logs, 5, SendResult{}, errors.New("receiver unavailable"), 2*time.Millisecond)

	snapshot := metrics.Snapshot()
	if snapshot.Generated != 15 {
		t.Fatalf("Generated = %d, want 15", snapshot.Generated)
	}
	if snapshot.Sent != 8 {
		t.Fatalf("Sent = %d, want 8", snapshot.Sent)
	}
	if snapshot.Rejected != 2 {
		t.Fatalf("Rejected = %d, want 2", snapshot.Rejected)
	}
	if snapshot.Failed != 5 {
		t.Fatalf("Failed = %d, want 5", snapshot.Failed)
	}
	if snapshot.Latency.Count != 2 {
		t.Fatalf("Latency.Count = %d, want 2", snapshot.Latency.Count)
	}
	if snapshot.AchievedRate <= 0 {
		t.Fatalf("AchievedRate = %f, want > 0", snapshot.AchievedRate)
	}
}

func TestLatencyRecorderSummary(t *testing.T) {
	recorder := NewLatencyRecorder(10)
	for _, value := range []time.Duration{1, 2, 3, 4, 5} {
		recorder.Record(value * time.Millisecond)
	}
	summary := recorder.Summary()
	if summary.Count != 5 {
		t.Fatalf("Count = %d, want 5", summary.Count)
	}
	if summary.MinMs != 1 || summary.MaxMs != 5 {
		t.Fatalf("min/max = %.1f/%.1f, want 1/5", summary.MinMs, summary.MaxMs)
	}
	if summary.MeanMs != 3 {
		t.Fatalf("mean = %.1f, want 3", summary.MeanMs)
	}
	if summary.P50Ms != 3 {
		t.Fatalf("p50 = %.1f, want 3", summary.P50Ms)
	}
	if summary.P95Ms < 4 || summary.P95Ms > 5 {
		t.Fatalf("p95 = %.1f, want between 4 and 5", summary.P95Ms)
	}
}

func TestLatencyRecorderReservoir(t *testing.T) {
	recorder := NewLatencyRecorder(3)
	for _, value := range []time.Duration{10, 20, 30, 40, 50} {
		recorder.Record(value * time.Millisecond)
	}
	summary := recorder.Summary()
	if summary.Count != 5 {
		t.Fatalf("Count = %d, want 5", summary.Count)
	}
	if summary.MinMs != 10 || summary.MaxMs != 50 {
		t.Fatalf("min/max = %.1f/%.1f, want 10/50", summary.MinMs, summary.MaxMs)
	}
	if summary.MeanMs != 30 {
		t.Fatalf("mean = %.1f, want 30", summary.MeanMs)
	}
}

func TestRunMetricsHandler(t *testing.T) {
	metrics := NewRunMetrics(1000)
	metrics.AddGenerated(Metrics, 10)
	metrics.Record(Metrics, 10, SendResult{Bytes: 100}, nil, time.Millisecond)

	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", contentType)
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		`observaki_loadgen_events_generated_total{signal="metrics"} 10`,
		`observaki_loadgen_events_sent_total{signal="metrics"} 10`,
		`observaki_loadgen_target_rate 1000`,
		`observaki_loadgen_send_latency_seconds{quantile="0.5"}`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics body does not contain %q:\n%s", expected, body)
		}
	}
}
