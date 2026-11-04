package walconsumer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/observaki/observaki/internal/adapters/clickhouse/writer"
	"github.com/observaki/observaki/internal/adapters/victoriametrics"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	otlplogspb "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type storageInserter struct {
	mu       sync.Mutex
	requests []writer.InsertRequest
	fail     error
}

func (f *storageInserter) Insert(_ context.Context, request writer.InsertRequest) (writer.InsertResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return writer.InsertResult{}, f.fail
	}
	f.requests = append(f.requests, request)
	return writer.InsertResult{WrittenRows: uint64(request.Rows)}, nil
}

func (f *storageInserter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func storageLogsRequest(count int) *logspb.ExportLogsServiceRequest {
	records := make([]*otlplogspb.LogRecord, 0, count)
	for index := 0; index < count; index++ {
		records = append(records, &otlplogspb.LogRecord{
			TimeUnixNano: uint64(time.Now().Add(-time.Duration(index) * time.Millisecond).UnixNano()),
			Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "ok"}},
		})
	}
	return &logspb.ExportLogsServiceRequest{ResourceLogs: []*otlplogspb.ResourceLogs{{
		Resource:  &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKV("service.name", "checkout")}},
		ScopeLogs: []*otlplogspb.ScopeLogs{{LogRecords: records}},
	}}}
}

func TestStorageDedupesLogsByEventID(t *testing.T) {
	inserter := &storageInserter{}
	store, err := NewStorage(context.Background(), StorageConfig{
		Inserter:     inserter,
		Metrics:      testVMClient(t, nil),
		SpoolDir:     t.TempDir(),
		DedupeWindow: 100,
		QueueDepth:   2,
		RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = store.Close(context.Background()) }()

	request := storageLogsRequest(2)
	ids := []string{"e1", "e2"}
	first, err := store.WriteLogs(context.Background(), "t", "p", 1, 1, request, ids, time.Now())
	if err != nil {
		t.Fatalf("first WriteLogs() error = %v", err)
	}
	if first.Written != 2 || first.Deduped != 0 || first.Failed != 0 {
		t.Fatalf("first = %+v", first)
	}
	second, err := store.WriteLogs(context.Background(), "t", "p", 1, 1, request, ids, time.Now())
	if err != nil {
		t.Fatalf("second WriteLogs() error = %v", err)
	}
	if second.Written != 0 || second.Deduped != 2 {
		t.Fatalf("second = %+v, want all deduped", second)
	}
	if inserter.count() != 1 {
		t.Fatalf("inserts = %d, want 1", inserter.count())
	}
}

func TestStorageWritesTraces(t *testing.T) {
	inserter := &storageInserter{}
	store, err := NewStorage(context.Background(), StorageConfig{
		Inserter:     inserter,
		Metrics:      testVMClient(t, nil),
		SpoolDir:     t.TempDir(),
		DedupeWindow: 100,
		QueueDepth:   2,
		RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = store.Close(context.Background()) }()

	request := &tracepb.ExportTraceServiceRequest{ResourceSpans: []*otlptracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKV("service.name", "cart")}},
		ScopeSpans: []*otlptracepb.ScopeSpans{{Spans: []*otlptracepb.Span{{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "GET /cart",
			StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
			EndTimeUnixNano:   uint64(time.Now().UnixNano()),
		}}}},
	}}}
	result, err := store.WriteTraces(context.Background(), "t", "p", 1, 1, request, []string{"span-1"}, time.Now())
	if err != nil {
		t.Fatalf("WriteTraces() error = %v", err)
	}
	if result.Written != 1 || inserter.count() != 1 {
		t.Fatalf("result = %+v, inserts = %d", result, inserter.count())
	}
}

func TestStorageDedupesMetricsAndWritesRemoteWrite(t *testing.T) {
	var requests atomic.Int64
	client := testVMClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	inserter := &storageInserter{}
	store, err := NewStorage(context.Background(), StorageConfig{
		Inserter:     inserter,
		Metrics:      client,
		SpoolDir:     t.TempDir(),
		DedupeWindow: 100,
		QueueDepth:   2,
		RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = store.Close(context.Background()) }()

	request := &metricpb.ExportMetricsServiceRequest{ResourceMetrics: []*otlpmetricpb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKV("service.name", "checkout")}},
		ScopeMetrics: []*otlpmetricpb.ScopeMetrics{{Metrics: []*otlpmetricpb.Metric{{
			Name: "bench.http.server.duration",
			Data: &otlpmetricpb.Metric_Gauge{Gauge: &otlpmetricpb.Gauge{DataPoints: []*otlpmetricpb.NumberDataPoint{{
				TimeUnixNano: uint64(time.Now().UnixNano()),
				Value:        &otlpmetricpb.NumberDataPoint_AsDouble{AsDouble: 1.5},
			}}}},
		}}}},
	}}}
	ids := []string{"m1"}
	first, err := store.WriteMetrics(context.Background(), "t", "p", 1, 1, request, ids, time.Now())
	if err != nil {
		t.Fatalf("first WriteMetrics() error = %v", err)
	}
	if first.Written != 1 || first.Deduped != 0 {
		t.Fatalf("first = %+v", first)
	}
	second, err := store.WriteMetrics(context.Background(), "t", "p", 1, 1, request, ids, time.Now())
	if err != nil {
		t.Fatalf("second WriteMetrics() error = %v", err)
	}
	if second.Written != 0 || second.Deduped != 1 {
		t.Fatalf("second = %+v", second)
	}
	if requests.Load() != 1 {
		t.Fatalf("remote write requests = %d, want 1", requests.Load())
	}
}

func TestStorageReportsDLQ(t *testing.T) {
	inserter := &storageInserter{fail: &writer.InsertError{StatusCode: 400, Permanent: true, Message: "bad"}}
	store, err := NewStorage(context.Background(), StorageConfig{
		Inserter:     inserter,
		Metrics:      testVMClient(t, nil),
		SpoolDir:     t.TempDir(),
		DedupeWindow: 100,
		QueueDepth:   2,
		RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = store.Close(context.Background()) }()

	result, err := store.WriteLogs(context.Background(), "t", "p", 1, 1, storageLogsRequest(2), []string{"e1", "e2"}, time.Now())
	if err != nil {
		t.Fatalf("WriteLogs() error = %v", err)
	}
	if result.Failed != 2 || result.Written != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func testVMClient(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *victoriametrics.Client {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	client, err := victoriametrics.Open(victoriametrics.Config{
		URL:             server.URL,
		RetryBackoff:    time.Millisecond,
		MaxRetryBackoff: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("victoriametrics.Open() error = %v", err)
	}
	return client
}
