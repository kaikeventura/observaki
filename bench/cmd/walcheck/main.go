// Command walcheck reads the Observaki NATS JetStream WAL without acking or
// deleting messages and reports message/event counts, duplicates, compression
// and shard distribution. It is the verification tool for Story 4.3 and the
// smoke test.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	natsadapter "github.com/observaki/observaki/internal/adapters/nats"
	"github.com/observaki/observaki/internal/ingest"
)

type streamReport struct {
	Messages     uint64  `json:"messages"`
	Events       uint64  `json:"events"`
	MessageBytes uint64  `json:"message_bytes"`
	PayloadBytes uint64  `json:"payload_bytes"`
	Compression  float64 `json:"compression_ratio"`
}

type report struct {
	Status          string                  `json:"status"`
	URL             string                  `json:"url"`
	StreamPrefix    string                  `json:"stream_prefix"`
	Streams         map[string]streamReport `json:"streams"`
	TotalMessages   uint64                  `json:"total_messages"`
	TotalEvents     uint64                  `json:"total_events"`
	UniqueEvents    uint64                  `json:"unique_events"`
	DuplicateEvents uint64                  `json:"duplicate_events"`
	MessageBytes    uint64                  `json:"message_bytes"`
	PayloadBytes    uint64                  `json:"payload_bytes"`
	Compression     float64                 `json:"compression_ratio"`
	Subjects        map[string]uint64       `json:"subjects"`
	Shards          map[string]uint64       `json:"shards"`
	Signals         map[string]uint64       `json:"signals"`
	SchemaVersions  map[string]uint64       `json:"schema_versions"`
	OldestAgeSecs   float64                 `json:"oldest_age_seconds"`
	NewestAgeSecs   float64                 `json:"newest_age_seconds"`
	ExpectedEvents  uint64                  `json:"expected_events,omitempty"`
	Errors          []string                `json:"errors,omitempty"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "walcheck: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("walcheck", flag.ContinueOnError)
	url := flags.String("url", envOr("OBSERVAKI_NATS_URL", "nats://127.0.0.1:4222"), "NATS URL")
	prefix := flags.String("stream-prefix", "OBSERVAKI", "JetStream stream prefix")
	signal := flags.String("signal", "all", "signal to read: all, traces, logs or metrics")
	expectEvents := flags.Uint64("expect-events", 0, "fail when the WAL does not contain exactly this many events")
	failOnDuplicates := flags.Bool("fail-on-duplicates", false, "fail when duplicate event_ids are found")
	timeout := flags.Duration("timeout", 30*time.Second, "per-stream read timeout")
	outPath := flags.String("out", "", "write the JSON report to this path instead of stdout")
	quiet := flags.Bool("quiet", false, "suppress the human-readable summary on stderr")
	if err := flags.Parse(args); err != nil {
		return err
	}

	result := &report{
		Status:         "pass",
		URL:            *url,
		StreamPrefix:   *prefix,
		Streams:        map[string]streamReport{},
		Subjects:       map[string]uint64{},
		Shards:         map[string]uint64{},
		Signals:        map[string]uint64{},
		SchemaVersions: map[string]uint64{},
		ExpectedEvents: *expectEvents,
	}

	connection, err := nats.Connect(*url, nats.Timeout(5*time.Second), nats.Name("observaki-walcheck"))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer connection.Close()
	js, err := jetstream.New(connection)
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}

	signals := []ingest.Signal{ingest.SignalTraces, ingest.SignalLogs, ingest.SignalMetrics}
	if *signal != "all" {
		candidate := ingest.Signal(*signal)
		switch candidate {
		case ingest.SignalTraces, ingest.SignalLogs, ingest.SignalMetrics:
			signals = []ingest.Signal{candidate}
		default:
			return fmt.Errorf("unknown signal %q", *signal)
		}
	}

	unique := make(map[string]struct{})
	now := time.Now()
	oldest := now
	newest := time.Time{}
	for _, candidate := range signals {
		name := natsadapter.StreamName(*prefix, candidate)
		streamResult, streamOldest, streamNewest, err := readStream(js, name, *timeout, unique, result)
		if err != nil {
			result.Status = "fail"
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		result.Streams[name] = streamResult
		result.TotalMessages += streamResult.Messages
		result.TotalEvents += streamResult.Events
		result.MessageBytes += streamResult.MessageBytes
		result.PayloadBytes += streamResult.PayloadBytes
		if streamOldest.Before(oldest) {
			oldest = streamOldest
		}
		if streamNewest.After(newest) {
			newest = streamNewest
		}
	}
	result.UniqueEvents = uint64(len(unique))
	if result.TotalEvents >= result.UniqueEvents {
		result.DuplicateEvents = result.TotalEvents - result.UniqueEvents
	}
	result.Compression = ratio(result.PayloadBytes, result.MessageBytes)
	if !oldest.Equal(now) {
		result.OldestAgeSecs = now.Sub(oldest).Seconds()
	}
	if !newest.IsZero() {
		result.NewestAgeSecs = now.Sub(newest).Seconds()
	}

	if *expectEvents > 0 && result.TotalEvents != *expectEvents {
		result.Status = "fail"
		result.Errors = append(result.Errors,
			fmt.Sprintf("events = %d, want %d", result.TotalEvents, *expectEvents))
	}
	if *failOnDuplicates && result.DuplicateEvents > 0 {
		result.Status = "fail"
		result.Errors = append(result.Errors,
			fmt.Sprintf("duplicate event ids = %d", result.DuplicateEvents))
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if *outPath != "" {
		if err := os.WriteFile(*outPath, append(encoded, '\n'), 0o644); err != nil {
			return err
		}
	} else {
		fmt.Println(string(encoded))
	}
	if !*quiet {
		fmt.Fprintf(os.Stderr,
			"walcheck: status=%s messages=%d events=%d unique=%d duplicates=%d compression=%.2fx\n",
			result.Status, result.TotalMessages, result.TotalEvents, result.UniqueEvents,
			result.DuplicateEvents, result.Compression,
		)
	}
	if result.Status != "pass" {
		return fmt.Errorf("status %s: %v", result.Status, result.Errors)
	}
	return nil
}

func readStream(
	js jetstream.JetStream,
	name string,
	timeout time.Duration,
	unique map[string]struct{},
	result *report,
) (streamReport, time.Time, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stream, err := js.Stream(ctx, name)
	if err != nil {
		return streamReport{}, time.Time{}, time.Time{}, fmt.Errorf("stream %s: %w", name, err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return streamReport{}, time.Time{}, time.Time{}, fmt.Errorf("stream %s info: %w", name, err)
	}
	if len(info.Config.Subjects) == 0 {
		return streamReport{}, time.Time{}, time.Time{}, fmt.Errorf("stream %s has no subjects", name)
	}
	// The WAL streams use WorkQueue retention, which only accepts explicit-ack
	// consumers. walcheck deliberately does not ack, so the messages stay in
	// the WAL for the writer; AckWait is long enough to avoid a redelivery
	// while the inspector is reading.
	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		FilterSubject: info.Config.Subjects[0],
		MaxAckPending: 1_000_000,
		AckWait:       time.Hour,
	})
	if err != nil {
		return streamReport{}, time.Time{}, time.Time{}, fmt.Errorf("consumer %s: %w", name, err)
	}
	var (
		streamResult streamReport
		oldest       time.Time
		newest       time.Time
	)
	for {
		batch, err := consumer.Fetch(256, jetstream.FetchMaxWait(500*time.Millisecond))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, jetstream.ErrNoMessages) {
				break
			}
			return streamResult, oldest, newest, fmt.Errorf("read %s: %w", name, err)
		}
		count := 0
		for message := range batch.Messages() {
			count++
			envelope, err := natsadapter.DecodeMessage(&nats.Msg{
				Subject: message.Subject(),
				Header:  message.Headers(),
				Data:    message.Data(),
			})
			if err != nil {
				return streamResult, oldest, newest, fmt.Errorf("decode %s: %w", name, err)
			}
			streamResult.Messages++
			streamResult.MessageBytes += uint64(len(message.Data()))
			result.Subjects[message.Subject()]++
			result.Shards[fmt.Sprintf("%d", envelope.GetShard())]++
			result.Signals[envelope.GetSignal()]++
			result.SchemaVersions[fmt.Sprintf("%d", envelope.GetSchemaVersion())]++
			created := time.Unix(0, envelope.GetCreatedAtUnixNano())
			if oldest.IsZero() || created.Before(oldest) {
				oldest = created
			}
			if created.After(newest) {
				newest = created
			}
			for _, request := range envelope.GetRequests() {
				streamResult.Events += uint64(len(request.GetEventIds()))
				streamResult.PayloadBytes += uint64(len(request.GetOtlpProtobuf()))
				for _, eventID := range request.GetEventIds() {
					unique[eventID] = struct{}{}
				}
			}
		}
		if count == 0 {
			break
		}
	}
	streamResult.Compression = ratio(streamResult.PayloadBytes, streamResult.MessageBytes)
	return streamResult, oldest, newest, nil
}

func ratio(payload, message uint64) float64 {
	if message == 0 {
		return 0
	}
	return float64(payload) / float64(message)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
