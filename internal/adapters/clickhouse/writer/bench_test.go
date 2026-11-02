package writer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func benchmarkRecords(count int) []LogRecord {
	attributes := map[string]string{
		"http.method":      "GET",
		"http.route":       "/api/v1/items",
		"http.status_code": "200",
	}
	resource := map[string]string{
		"deployment.environment": "bench",
		"service.namespace":      "observaki",
	}
	records := make([]LogRecord, count)
	now := time.Now().UTC()
	for index := range records {
		id := fmt.Sprintf("%032x", index)
		records[index] = LogRecord{
			TenantID:           "tenant-0",
			ProjectID:          "project-0",
			Timestamp:          now,
			Severity:           "INFO",
			SeverityNumber:     9,
			ServiceName:        "checkout",
			Body:               "request completed method=GET path=/api/v1/items status=200 duration_ms=12",
			Attributes:         attributes,
			ResourceAttributes: resource,
			TraceID:            id,
			SpanID:             id,
			EventID:            id,
			RetentionDays:      15,
		}
	}
	return records
}

func BenchmarkEncodeAccepted200k(b *testing.B) {
	records := benchmarkRecords(200_000)
	accepted := make([]int, len(records))
	times := make([]time.Time, len(records))
	for index := range records {
		accepted[index] = index
		times[index] = records[index].Timestamp
	}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = encodeAccepted(records, accepted, times, time.Now().UTC(), 8)
	}
}

func BenchmarkSHA256Batch(b *testing.B) {
	data := make([]byte, 54<<20)
	b.SetBytes(int64(len(data)))
	for iteration := 0; iteration < b.N; iteration++ {
		_ = sha256.Sum256(data)
	}
}

func BenchmarkDedupe200k(b *testing.B) {
	records := benchmarkRecords(200_000)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		window := newDedupeWindow(2_000_000)
		for index := range records {
			identity := records[index].identity()
			window.add(dedupeKey{tenantID: identity.tenantID, projectID: identity.projectID, signal: SignalLogs, eventID: identity.eventID})
		}
	}
}

func BenchmarkSpoolWrite54MB(b *testing.B) {
	data := make([]byte, 54<<20)
	sum := sha256.Sum256(data)
	for iteration := 0; iteration < b.N; iteration++ {
		dir := b.TempDir()
		spool, err := openSpool(dir)
		if err != nil {
			b.Fatal(err)
		}
		meta := &batchMeta{
			Seq:                  1,
			Table:                "otel_logs",
			Signal:               SignalLogs,
			Rows:                 200_000,
			DedupeToken:          "token",
			MinTimestampUnixNano: time.Now().UnixNano(),
			MaxTimestampUnixNano: time.Now().UnixNano(),
			DataSHA256:           hex.EncodeToString(sum[:]),
		}
		if _, err := spool.write(meta, data); err != nil {
			b.Fatal(err)
		}
	}
}
