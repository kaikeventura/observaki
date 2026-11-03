package nats

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/nats-io/nats.go"
	"github.com/observaki/observaki/internal/ingest"
	"github.com/observaki/observaki/internal/ingestpb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func testEnvelope(t *testing.T) *ingestpb.Envelope {
	t.Helper()
	batch := logsBatch("tenant.a", "project:b", 2)
	eventIDs := ingest.SynthesizeEventIDs(batch)
	payload, err := proto.Marshal(batch.Logs)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	return &ingestpb.Envelope{
		SchemaVersion:     SchemaVersion,
		MessageId:         "01920000-0000-7000-8000-000000000001",
		TenantId:          batch.TenantID,
		ProjectId:         batch.ProjectID,
		ConfigVersion:     batch.ConfigVersion,
		ConfigGeneration:  batch.ConfigGeneration,
		AuthzEpoch:        batch.ConfigGeneration,
		Signal:            batch.Signal.String(),
		Shard:             3,
		CreatedAtUnixNano: time.Now().UnixNano(),
		Requests: []*ingestpb.Request{{
			RequestId:          batch.RequestID,
			OtlpProtobuf:       payload,
			EventIds:           eventIDs,
			ReceivedAtUnixNano: batch.ReceivedAt.UnixNano(),
		}},
	}
}

func testMessage(t *testing.T, envelope *ingestpb.Envelope) *nats.Msg {
	t.Helper()
	data, err := EncodeEnvelope(envelope)
	if err != nil {
		t.Fatalf("EncodeEnvelope() error = %v", err)
	}
	return &nats.Msg{
		Subject: "observaki.ingest.v1.dGVuYW50LmE.cHJvamVjdDpi.e3.logs.3",
		Header: nats.Header{
			EncodingHeader: []string{EncodingZstd},
			SchemaHeader:   []string{"1"},
		},
		Data: data,
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	envelope := testEnvelope(t)
	message := testMessage(t, envelope)
	decoded, err := DecodeMessage(message)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if decoded.GetMessageId() != envelope.GetMessageId() {
		t.Fatalf("message_id = %q", decoded.GetMessageId())
	}
	if decoded.GetTenantId() != "tenant.a" || decoded.GetProjectId() != "project:b" {
		t.Fatalf("identity = %q/%q", decoded.GetTenantId(), decoded.GetProjectId())
	}
	if len(decoded.GetRequests()) != 1 || len(decoded.GetRequests()[0].GetEventIds()) != 2 {
		t.Fatalf("requests = %+v", decoded.GetRequests())
	}
	if err := ValidateEnvelope(decoded); err != nil {
		t.Fatalf("ValidateEnvelope() error = %v", err)
	}
}

func TestDecodeMessageRequiresEncodingHeader(t *testing.T) {
	message := testMessage(t, testEnvelope(t))
	message.Header.Del(EncodingHeader)
	if _, err := DecodeMessage(message); !errors.Is(err, ErrMissingEncoding) {
		t.Fatalf("DecodeMessage() error = %v, want ErrMissingEncoding", err)
	}
}

func TestDecodeMessageRejectsUnsupportedSchema(t *testing.T) {
	envelope := testEnvelope(t)
	envelope.SchemaVersion = 2
	data, err := EncodeEnvelope(envelope)
	if err != nil {
		t.Fatalf("EncodeEnvelope() error = %v", err)
	}
	message := &nats.Msg{
		Header: nats.Header{EncodingHeader: []string{EncodingZstd}, SchemaHeader: []string{"2"}},
		Data:   data,
	}
	if _, err := DecodeMessage(message); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("DecodeMessage() error = %v, want ErrUnsupportedSchema", err)
	}
}

func TestValidateEnvelopeRequiresOneEventIDPerEvent(t *testing.T) {
	envelope := testEnvelope(t)
	envelope.Requests[0].EventIds = envelope.Requests[0].EventIds[:1]
	if err := ValidateEnvelope(envelope); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("ValidateEnvelope() error = %v, want ErrInvalidEnvelope", err)
	}
}

func TestDecodeEnvelopeToleratesUnknownFields(t *testing.T) {
	envelope := testEnvelope(t)
	raw, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	raw = protowire.AppendTag(raw, 111, protowire.VarintType)
	raw = protowire.AppendVarint(raw, 42)
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("zstd.NewWriter() error = %v", err)
	}
	compressed := encoder.EncodeAll(raw, nil)
	_ = encoder.Close()
	message := &nats.Msg{
		Header: nats.Header{EncodingHeader: []string{EncodingZstd}, SchemaHeader: []string{"1"}},
		Data:   compressed,
	}
	decoded, err := DecodeMessage(message)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if decoded.GetMessageId() != envelope.GetMessageId() {
		t.Fatalf("unknown field broke the envelope: %q", decoded.GetMessageId())
	}
}

func TestEnvelopeCompression(t *testing.T) {
	envelope := testEnvelope(t)
	raw, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	compressed, err := EncodeEnvelope(envelope)
	if err != nil {
		t.Fatalf("EncodeEnvelope() error = %v", err)
	}
	if len(compressed) == 0 || bytes.Equal(compressed, raw) {
		t.Fatal("envelope was not compressed")
	}
	if len(compressed) >= len(raw) {
		t.Fatalf("compressed = %d bytes, raw = %d bytes", len(compressed), len(raw))
	}
}
