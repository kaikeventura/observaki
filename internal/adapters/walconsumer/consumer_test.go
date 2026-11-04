package walconsumer

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	natsadapter "github.com/observaki/observaki/internal/adapters/nats"
	"github.com/observaki/observaki/internal/ingest"
	logspbcollector "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpbcollector "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepbcollector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	otlplogspb "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func startJetStream(t *testing.T, storeDir string) *natsserver.Server {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{
		JetStream:  true,
		StoreDir:   storeDir,
		Port:       -1,
		MaxPayload: 16 << 20,
		NoLog:      true,
		NoSigs:     true,
	})
	if err != nil {
		t.Fatalf("nats-server NewServer() error = %v", err)
	}
	go server.Start()
	if !server.ReadyForConnections(10 * time.Second) {
		server.Shutdown()
		t.Fatal("nats-server did not become ready")
	}
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})
	return server
}

func testSink(t *testing.T, url string) *natsadapter.Sink {
	t.Helper()
	sink, err := natsadapter.Open(context.Background(), natsadapter.Config{
		URL:             url,
		StreamStorage:   "file",
		StreamMaxAge:    time.Hour,
		StreamMaxBytes:  64 << 20,
		BatchMaxEvents:  1000,
		BatchMaxBytes:   1 << 20,
		BatchLinger:     2 * time.Millisecond,
		ConnectTimeout:  5 * time.Second,
		PublishTimeout:  5 * time.Second,
		MaxPendingBytes: 16 << 20,
		PublishWorkers:  2,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("nats Open() error = %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return sink
}

func logsBatch(tenant, project string, records int) *ingest.Batch {
	logs := make([]*otlplogspb.LogRecord, 0, records)
	for index := 0; index < records; index++ {
		logs = append(logs, &otlplogspb.LogRecord{
			TimeUnixNano: uint64(time.Now().Add(-time.Duration(index) * time.Millisecond).UnixNano()),
			Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "request completed"}},
			Attributes:   []*commonpb.KeyValue{stringKV("http.status_code", "200")},
		})
	}
	return &ingest.Batch{
		Signal:           ingest.SignalLogs,
		TenantID:         tenant,
		ProjectID:        project,
		RequestID:        fmt.Sprintf("logs-%s-%d", tenant, time.Now().UnixNano()),
		ConfigVersion:    "test",
		ConfigGeneration: 1,
		ReceivedAt:       time.Now().UTC(),
		Logs: &logspbcollector.ExportLogsServiceRequest{ResourceLogs: []*otlplogspb.ResourceLogs{{
			Resource:  &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKV("service.name", "checkout")}},
			ScopeLogs: []*otlplogspb.ScopeLogs{{LogRecords: logs}},
		}}},
	}
}

func tracesBatch(tenant, project string, spans int) *ingest.Batch {
	items := make([]*otlptracepb.Span, 0, spans)
	for index := 0; index < spans; index++ {
		start := time.Now().Add(-time.Duration(index) * time.Millisecond)
		traceID := make([]byte, 16)
		spanID := make([]byte, 8)
		traceID[0] = byte(index + 1)
		spanID[0] = byte(index + 1)
		items = append(items, &otlptracepb.Span{
			TraceId:           traceID,
			SpanId:            spanID,
			Name:              "GET /checkout",
			Kind:              otlptracepb.Span_SPAN_KIND_SERVER,
			StartTimeUnixNano: uint64(start.UnixNano()),
			EndTimeUnixNano:   uint64(start.Add(20 * time.Millisecond).UnixNano()),
			Status:            &otlptracepb.Status{Code: otlptracepb.Status_STATUS_CODE_OK},
		})
	}
	return &ingest.Batch{
		Signal:           ingest.SignalTraces,
		TenantID:         tenant,
		ProjectID:        project,
		RequestID:        fmt.Sprintf("traces-%s-%d", tenant, time.Now().UnixNano()),
		ConfigVersion:    "test",
		ConfigGeneration: 1,
		ReceivedAt:       time.Now().UTC(),
		Traces: &tracepbcollector.ExportTraceServiceRequest{ResourceSpans: []*otlptracepb.ResourceSpans{{
			Resource:   &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKV("service.name", "checkout")}},
			ScopeSpans: []*otlptracepb.ScopeSpans{{Spans: items}},
		}}},
	}
}

func metricsBatch(tenant, project string, points int) *ingest.Batch {
	items := make([]*otlpmetricpb.NumberDataPoint, 0, points)
	for index := 0; index < points; index++ {
		items = append(items, &otlpmetricpb.NumberDataPoint{
			TimeUnixNano: uint64(time.Now().Add(-time.Duration(index) * time.Millisecond).UnixNano()),
			Value:        &otlpmetricpb.NumberDataPoint_AsDouble{AsDouble: float64(index)},
			Attributes:   []*commonpb.KeyValue{stringKV("http.route", "/checkout")},
		})
	}
	metric := &otlpmetricpb.Metric{
		Name: "bench.http.server.duration",
		Data: &otlpmetricpb.Metric_Gauge{Gauge: &otlpmetricpb.Gauge{DataPoints: items}},
	}
	return &ingest.Batch{
		Signal:           ingest.SignalMetrics,
		TenantID:         tenant,
		ProjectID:        project,
		RequestID:        fmt.Sprintf("metrics-%s-%d", tenant, time.Now().UnixNano()),
		ConfigVersion:    "test",
		ConfigGeneration: 1,
		ReceivedAt:       time.Now().UTC(),
		Metrics: &metricpbcollector.ExportMetricsServiceRequest{ResourceMetrics: []*otlpmetricpb.ResourceMetrics{{
			Resource:     &resourcepb.Resource{Attributes: []*commonpb.KeyValue{stringKV("service.name", "checkout")}},
			ScopeMetrics: []*otlpmetricpb.ScopeMetrics{{Metrics: []*otlpmetricpb.Metric{metric}}},
		}}},
	}
}

func stringKV(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

type fakeSink struct {
	mu        sync.Mutex
	logs      int
	traces    int
	metrics   int
	attempts  int
	failFirst int
	permanent bool
	block     chan struct{}
}

func (f *fakeSink) WriteLogs(_ context.Context, _, _ string, _, _ uint64, request *logspbcollector.ExportLogsServiceRequest, eventIDs []string, _ time.Time) (WriteResult, error) {
	return f.write(len(eventIDs), &f.logs)
}

func (f *fakeSink) WriteTraces(_ context.Context, _, _ string, _, _ uint64, request *tracepbcollector.ExportTraceServiceRequest, eventIDs []string, _ time.Time) (WriteResult, error) {
	return f.write(len(eventIDs), &f.traces)
}

func (f *fakeSink) WriteMetrics(_ context.Context, _, _ string, _, _ uint64, request *metricpbcollector.ExportMetricsServiceRequest, eventIDs []string, _ time.Time) (WriteResult, error) {
	return f.write(len(eventIDs), &f.metrics)
}

func (f *fakeSink) write(events int, counter *int) (WriteResult, error) {
	f.mu.Lock()
	f.attempts++
	attempt := f.attempts
	f.mu.Unlock()
	if f.block != nil {
		<-f.block
	}
	if f.permanent {
		return WriteResult{}, &PermanentError{Err: errors.New("permanent test failure")}
	}
	if attempt <= f.failFirst {
		return WriteResult{}, errors.New("transient test failure")
	}
	f.mu.Lock()
	*counter += events
	f.mu.Unlock()
	return WriteResult{Events: events, Written: events}, nil
}

func (f *fakeSink) counts() (int, int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs, f.traces, f.metrics, f.attempts
}

func testConsumer(t *testing.T, url string, sink Sink, mutate func(*Config)) *Consumer {
	t.Helper()
	cfg := Config{
		URL:              url,
		BatchMaxEvents:   100,
		BatchMaxBytes:    1 << 20,
		FetchMaxMessages: 8,
		MaxAckPending:    64,
		AckWait:          5 * time.Second,
		BatchLinger:      20 * time.Millisecond,
		ConnectTimeout:   5 * time.Second,
		RetryBackoff:     time.Millisecond,
		MaxRetryBackoff:  5 * time.Millisecond,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	consumer, err := Open(context.Background(), cfg, sink)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = consumer.Close(ctx)
	})
	return consumer
}

func waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func TestConsumerWritesAllSignals(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	sink := testSink(t, server.ClientURL())
	fake := &fakeSink{}
	consumer := testConsumer(t, server.ClientURL(), fake, nil)

	if _, err := sink.Consume(context.Background(), logsBatch("tenant-a", "project-a", 5)); err != nil {
		t.Fatalf("logs Consume() error = %v", err)
	}
	if _, err := sink.Consume(context.Background(), tracesBatch("tenant-a", "project-a", 4)); err != nil {
		t.Fatalf("traces Consume() error = %v", err)
	}
	if _, err := sink.Consume(context.Background(), metricsBatch("tenant-b", "project-b", 3)); err != nil {
		t.Fatalf("metrics Consume() error = %v", err)
	}

	waitFor(t, "all signals written", func() bool {
		logs, traces, metrics, _ := fake.counts()
		return logs == 5 && traces == 4 && metrics == 3
	})
	stats := consumer.Stats()
	if len(stats.Tenants) != 3 {
		t.Fatalf("tenant rows = %d, want 3: %+v", len(stats.Tenants), stats.Tenants)
	}
	for _, row := range stats.Tenants {
		if row.Accepted != row.Written || row.Failed != 0 || row.Deduped != 0 {
			t.Fatalf("row = %+v", row)
		}
		if row.LagP95MS < 0 {
			t.Fatalf("negative lag: %+v", row)
		}
	}
}

func TestConsumerRetriesTransientFailure(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	sink := testSink(t, server.ClientURL())
	fake := &fakeSink{failFirst: 2}
	consumer := testConsumer(t, server.ClientURL(), fake, nil)

	if _, err := sink.Consume(context.Background(), logsBatch("tenant-a", "project-a", 3)); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	waitFor(t, "retry then write", func() bool {
		logs, _, _, attempts := fake.counts()
		return logs == 3 && attempts >= 3
	})
	waitFor(t, "retry counter", func() bool {
		stats := consumer.Stats()
		for _, row := range stats.Tenants {
			if row.Signal == "logs" && row.Retries >= 2 {
				return true
			}
		}
		return false
	})
}

func TestConsumerMovesPermanentFailureToDLQ(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	sink := testSink(t, server.ClientURL())
	fake := &fakeSink{permanent: true}
	consumer := testConsumer(t, server.ClientURL(), fake, nil)

	if _, err := sink.Consume(context.Background(), logsBatch("tenant-a", "project-a", 3)); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	waitFor(t, "dlq message", func() bool {
		return consumer.Stats().Signals["logs"].DLQMessages == 1
	})
	if logs, _, _, _ := fake.counts(); logs != 0 {
		t.Fatalf("fake sink wrote %d logs, want 0", logs)
	}
	stats := consumer.Stats()
	found := false
	for _, row := range stats.Tenants {
		if row.Signal == "logs" && row.Failed == 3 {
			found = true
		}
	}
	if !found {
		t.Fatalf("failed events not recorded: %+v", stats.Tenants)
	}
}

func TestConsumerMovesInvalidEnvelopeToDLQ(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	testSink(t, server.ClientURL())
	fake := &fakeSink{}
	consumer := testConsumer(t, server.ClientURL(), fake, nil)

	nc, err := nats.Connect(server.ClientURL())
	if err != nil {
		t.Fatalf("nats.Connect() error = %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New() error = %v", err)
	}
	subject := "observaki.ingest.v1." + base64.RawURLEncoding.EncodeToString([]byte("tenant-a")) +
		"." + base64.RawURLEncoding.EncodeToString([]byte("project-a")) + ".e1.logs.0"
	if _, err := js.Publish(context.Background(), subject, []byte("not a zstd envelope")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	waitFor(t, "invalid envelope dlq", func() bool {
		return consumer.Stats().Signals["logs"].DLQMessages >= 1
	})
	if logs, _, _, _ := fake.counts(); logs != 0 {
		t.Fatalf("fake sink wrote %d logs, want 0", logs)
	}
}

func TestConsumerHealthyAndStats(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	testSink(t, server.ClientURL())
	fake := &fakeSink{}
	consumer := testConsumer(t, server.ClientURL(), fake, nil)
	if err := consumer.Healthy(); err != nil {
		t.Fatalf("Healthy() error = %v", err)
	}
	if stats := consumer.Stats(); !stats.Up || !stats.Connected {
		t.Fatalf("stats = %+v", stats)
	}
}
