package writer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MetricsHandler exposes the writer counters in Prometheus text format.
func (w *Writer) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		stats := w.Stats()
		response.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		labels := fmt.Sprintf(`{table=%q,signal=%q}`, stats.Table, stats.Signal)
		writeMetric := func(name, help, kind string, value any) {
			_, _ = fmt.Fprintf(response, "# HELP %s %s\n# TYPE %s %s\n%s%s %v\n", name, help, name, kind, name, labels, value)
		}
		writeMetric("observaki_ingest_submitted_batches_total", "Batches durably spooled.", "counter", stats.SubmittedBatches)
		writeMetric("observaki_ingest_submitted_events_total", "Events durably spooled.", "counter", stats.SubmittedEvents)
		writeMetric("observaki_ingest_inserted_events_total", "Events confirmed by ClickHouse.", "counter", stats.InsertedEvents)
		writeMetric("observaki_ingest_duplicate_events_total", "Events dropped by the writer event_id window.", "counter", stats.DuplicateEvents)
		writeMetric("observaki_ingest_clamped_timestamps_total", "Event timestamps clamped to ingested_at.", "counter", stats.ClampedTimestamps)
		writeMetric("observaki_ingest_acked_batches_total", "Batches acknowledged after a ClickHouse flush.", "counter", stats.AckedBatches)
		writeMetric("observaki_ingest_retried_batches_total", "Batches that required at least one retry.", "counter", stats.RetriedBatches)
		writeMetric("observaki_ingest_retry_attempts_total", "Insert attempts that failed and were retried.", "counter", stats.RetryAttempts)
		writeMetric("observaki_ingest_recovered_batches_total", "Batches replayed from the spool after a restart.", "counter", stats.RecoveredBatches)
		writeMetric("observaki_ingest_recovered_events_total", "Events replayed from the spool after a restart.", "counter", stats.RecoveredEvents)
		writeMetric("observaki_ingest_dlq_events_total", "Events moved to the dead-letter queue.", "counter", stats.DLQEvents)
		writeMetric("observaki_ingest_pending_batches", "Batches spooled but not acknowledged.", "gauge", stats.PendingBatches)
		writeMetric("observaki_ingest_pending_bytes", "Payload bytes spooled but not acknowledged.", "gauge", stats.PendingBytes)
		writeMetric("observaki_ingest_last_lag_seconds", "Lag of the last acknowledged batch.", "gauge", stats.LastLagMillis/1000)
		writeMetric("observaki_ingest_max_lag_seconds", "Largest acknowledged batch lag since start.", "gauge", stats.MaxLagMillis/1000)
		writeMetric("observaki_ingest_dedupe_window_used", "Event identities currently in the dedupe window.", "gauge", stats.DedupeWindowUsed)
		_, _ = fmt.Fprintf(response, "observaki_ingest_last_ack_timestamp_seconds%s %d\n", labels, lastAckSeconds(stats.LastAckAt))
		_, _ = fmt.Fprintf(response, "observaki_ingest_up%s 1\n", labels)
	})
}

// StatsHandler exposes the same snapshot as JSON.
func (w *Writer) StatsHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(response)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(w.Stats())
	})
}

func lastAckSeconds(timestamp string) int64 {
	if timestamp == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}
