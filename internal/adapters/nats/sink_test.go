package nats

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/observaki/observaki/internal/ingest"
	"github.com/observaki/observaki/internal/ingestpb"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	otlplogspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func startJetStream(t *testing.T, storeDir string) *natsserver.Server {
	t.Helper()
	options := &natsserver.Options{
		JetStream:  true,
		StoreDir:   storeDir,
		Port:       -1,
		MaxPayload: 16 << 20,
		NoLog:      true,
		NoSigs:     true,
	}
	server, err := natsserver.NewServer(options)
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

func testSink(t *testing.T, url string, mutate func(*Config)) *Sink {
	t.Helper()
	cfg := Config{
		URL:             url,
		SubjectPrefix:   "observaki.ingest.v1",
		StreamPrefix:    "OBSERVAKI",
		Shards:          8,
		StreamStorage:   "memory",
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
	}
	if mutate != nil {
		mutate(&cfg)
	}
	sink, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return sink
}

func logsBatch(tenant, project string, records int) *ingest.Batch {
	return logsBatchWithOffset(tenant, project, records, 0)
}

func logsBatchWithOffset(tenant, project string, records, offset int) *ingest.Batch {
	logs := make([]*otlplogspb.LogRecord, 0, records)
	for index := 0; index < records; index++ {
		logs = append(logs, &otlplogspb.LogRecord{
			TimeUnixNano: uint64(1_700_000_000_000_000_000 + offset + index),
			Body: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_StringValue{StringValue: "checkout failed"},
			},
			Attributes: []*commonpb.KeyValue{{
				Key:   "http.status",
				Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "500"}},
			}},
		})
	}
	return &ingest.Batch{
		Signal:           ingest.SignalLogs,
		TenantID:         tenant,
		ProjectID:        project,
		RequestID:        "request-" + tenant + "-" + project,
		ConfigVersion:    "test-1",
		ConfigGeneration: 7,
		ReceivedAt:       time.Now().UTC(),
		Logs: &logspb.ExportLogsServiceRequest{ResourceLogs: []*otlplogspb.ResourceLogs{{
			Resource: &resourcepb.Resource{},
			ScopeLogs: []*otlplogspb.ScopeLogs{{
				Scope:      &commonpb.InstrumentationScope{Name: "test"},
				LogRecords: logs,
			}},
		}}},
	}
}

func readAllMessages(t *testing.T, url, streamName string) []*ingestpb.Envelope {
	t.Helper()
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("nats.Connect() error = %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		t.Fatalf("Stream(%q) error = %v", streamName, err)
	}
	consumer, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatalf("OrderedConsumer() error = %v", err)
	}
	var envelopes []*ingestpb.Envelope
	for {
		message, err := consumer.Next(jetstream.FetchMaxWait(200 * time.Millisecond))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, jetstream.ErrNoMessages) {
				break
			}
			t.Fatalf("consumer.Next() error = %v", err)
		}
		envelope, err := DecodeMessage(&nats.Msg{
			Subject: message.Subject(),
			Header:  message.Headers(),
			Data:    message.Data(),
		})
		if err != nil {
			t.Fatalf("DecodeMessage() error = %v", err)
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes
}

func TestSinkPublishesBatchedCompressedEnvelopes(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	sink := testSink(t, server.ClientURL(), nil)

	batches := 40
	eventsPerBatch := 10
	accepted := 0
	for index := 0; index < batches; index++ {
		batch := logsBatchWithOffset("tenant-a", "project-a", eventsPerBatch, index*eventsPerBatch)
		batch.RequestID = "request-" + time.Now().Format("150405.000000000") + "-" + string(rune('a'+index%26))
		result, err := sink.Consume(context.Background(), batch)
		if err != nil {
			t.Fatalf("Consume() error = %v", err)
		}
		if result.Accepted != eventsPerBatch {
			t.Fatalf("Consume() accepted = %d, want %d", result.Accepted, eventsPerBatch)
		}
		accepted += result.Accepted
	}

	envelopes := readAllMessages(t, server.ClientURL(), StreamName("OBSERVAKI", ingest.SignalLogs))
	if len(envelopes) == 0 {
		t.Fatal("stream has no messages")
	}
	totalEvents := 0
	subjects := map[string]struct{}{}
	shards := map[uint32]struct{}{}
	eventIDs := map[string]struct{}{}
	for _, envelope := range envelopes {
		if envelope.GetTenantId() != "tenant-a" || envelope.GetProjectId() != "project-a" {
			t.Fatalf("envelope identity = %q/%q", envelope.GetTenantId(), envelope.GetProjectId())
		}
		if envelope.GetAuthzEpoch() != 7 {
			t.Fatalf("authz epoch = %d, want 7", envelope.GetAuthzEpoch())
		}
		subject := subjectFor("observaki.ingest.v1", &ingest.Batch{
			Signal: ingest.SignalLogs, TenantID: "tenant-a", ProjectID: "project-a", ConfigGeneration: 7,
		}, int(envelope.GetShard()))
		subjects[subject] = struct{}{}
		shards[envelope.GetShard()] = struct{}{}
		for _, request := range envelope.GetRequests() {
			totalEvents += len(request.GetEventIds())
			for _, eventID := range request.GetEventIds() {
				eventIDs[eventID] = struct{}{}
			}
		}
	}
	if totalEvents != accepted {
		t.Fatalf("WAL events = %d, want %d", totalEvents, accepted)
	}
	if len(eventIDs) != accepted {
		t.Fatalf("unique event ids = %d, want %d", len(eventIDs), accepted)
	}
	if len(subjects) < 2 {
		t.Fatalf("subjects = %v, want more than one shard for one tenant", subjects)
	}
	if len(shards) < 2 {
		t.Fatalf("shards = %v, want more than one shard", shards)
	}

	stats := sink.Stats()
	if stats.PublishedBatches == 0 || stats.PublishedEvents != uint64(accepted) {
		t.Fatalf("stats = %+v, want %d published events", stats, accepted)
	}
	if stats.PendingBytes != 0 || stats.PendingEvents != 0 {
		t.Fatalf("pending = %d bytes / %d events, want zero", stats.PendingBytes, stats.PendingEvents)
	}
}

func TestSinkIsIdempotentByEventID(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	sink := testSink(t, server.ClientURL(), nil)
	batch := logsBatch("tenant-a", "project-a", 5)
	batch.RequestID = "duplicate-request"
	if _, err := sink.Consume(context.Background(), batch); err != nil {
		t.Fatalf("first Consume() error = %v", err)
	}
	first := ingest.SynthesizeEventIDs(batch)
	if _, err := sink.Consume(context.Background(), batch); err != nil {
		t.Fatalf("second Consume() error = %v", err)
	}
	second := ingest.SynthesizeEventIDs(batch)
	if len(first) != len(second) {
		t.Fatalf("event ids = %v / %v", first, second)
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("event id %d changed: %q != %q", index, first[index], second[index])
		}
	}
	// The WAL accepts the retry (at-least-once); the writer dedupes by
	// event_id. This test proves the IDs are stable across the retry.
	if got := sink.Stats().PublishedEvents; got != 10 {
		t.Fatalf("published events = %d, want 10 (both attempts reached the WAL)", got)
	}
}

func TestSinkSurvivesNATSServerRestart(t *testing.T) {
	storeDir := t.TempDir()
	server := startJetStream(t, storeDir)
	sink := testSink(t, server.ClientURL(), func(cfg *Config) {
		cfg.StreamStorage = "file"
	})
	accepted := 0
	for index := 0; index < 10; index++ {
		batch := logsBatch("tenant-restart", "project-restart", 5)
		batch.RequestID = "restart-" + time.Now().Format("150405.000000000") + string(rune('a'+index))
		if _, err := sink.Consume(context.Background(), batch); err != nil {
			t.Fatalf("Consume() error = %v", err)
		}
		accepted += 5
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	server.Shutdown()
	server.WaitForShutdown()

	restarted := startJetStream(t, storeDir)
	envelopes := readAllMessages(t, restarted.ClientURL(), StreamName("OBSERVAKI", ingest.SignalLogs))
	events := 0
	for _, envelope := range envelopes {
		for _, request := range envelope.GetRequests() {
			events += len(request.GetEventIds())
		}
	}
	if events != accepted {
		t.Fatalf("events after restart = %d, want %d", events, accepted)
	}
}

func TestSinkBackpressure(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	sink := testSink(t, server.ClientURL(), func(cfg *Config) {
		cfg.MaxPendingBytes = 1
	})
	_, err := sink.Consume(context.Background(), logsBatch("tenant-a", "project-a", 1))
	if !errors.Is(err, ingest.ErrBackpressure) {
		t.Fatalf("Consume() error = %v, want ErrBackpressure", err)
	}
	if sink.Stats().Backpressure == 0 {
		t.Fatal("backpressure counter = 0")
	}
}

func TestSinkHealthAndClosed(t *testing.T) {
	server := startJetStream(t, t.TempDir())
	sink := testSink(t, server.ClientURL(), nil)
	if err := sink.Healthy(); err != nil {
		t.Fatalf("Healthy() error = %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := sink.Healthy(); !errors.Is(err, ingest.ErrSinkClosed) {
		t.Fatalf("Healthy() after Close() error = %v, want ErrSinkClosed", err)
	}
	if _, err := sink.Consume(context.Background(), logsBatch("tenant-a", "project-a", 1)); !errors.Is(err, ingest.ErrSinkClosed) {
		t.Fatalf("Consume() after Close() error = %v, want ErrSinkClosed", err)
	}
}
