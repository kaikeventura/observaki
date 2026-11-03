// Package nats implements the durable WAL adapter for the OTLP gateway. The
// gateway publishes a versioned protobuf envelope, compressed with zstd, to a
// sharded NATS JetStream subject and only acknowledges the OTLP request after
// the JetStream publish ack (AD-4, AD-18, AD-22, ADR-0003).
package nats

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/nats-io/nats.go"
	"github.com/observaki/observaki/internal/ingest"
	"github.com/observaki/observaki/internal/ingestpb"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Envelope contract constants shared by the gateway and the writer.
const (
	SchemaVersion = 1

	EncodingHeader     = "Observaki-Encoding"
	EncodingZstd       = "zstd"
	SchemaHeader       = "Observaki-Schema-Version"
	TenantHeader       = "Observaki-Tenant-Id"
	ProjectHeader      = "Observaki-Project-Id"
	GenerationHeader   = "Observaki-Authz-Epoch"
	SignalHeader       = "Observaki-Signal"
	ShardHeader        = "Observaki-Shard"
	RequestCountHeader = "Observaki-Request-Count"

	// MaxDecompressedBytes bounds a decoded envelope to protect the writer
	// from a decompression bomb. A batch is bounded by the gateway config and
	// is far below this limit.
	MaxDecompressedBytes = 64 << 20
)

// Envelope errors.
var (
	ErrMissingEncoding     = errors.New("nats: missing Observaki-Encoding header")
	ErrUnsupportedEncoding = errors.New("nats: unsupported envelope encoding")
	ErrUnsupportedSchema   = errors.New("nats: unsupported envelope schema version")
	ErrInvalidEnvelope     = errors.New("nats: invalid envelope")
	ErrEnvelopeTooLarge    = errors.New("nats: compressed envelope exceeds the NATS payload limit")
)

var encoderPool = sync.Pool{
	New: func() any {
		encoder, err := zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedFastest),
			zstd.WithEncoderConcurrency(1),
			zstd.WithWindowSize(1<<20),
		)
		if err != nil {
			panic(err)
		}
		return encoder
	},
}

// EncodeEnvelope marshals and compresses an envelope.
func EncodeEnvelope(envelope *ingestpb.Envelope) ([]byte, error) {
	if envelope == nil {
		return nil, fmt.Errorf("%w: nil envelope", ErrInvalidEnvelope)
	}
	raw, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	encoder := encoderPool.Get().(*zstd.Encoder)
	defer encoderPool.Put(encoder)
	return encoder.EncodeAll(raw, nil), nil
}

// DecodeMessage decompresses and parses one NATS WAL message. It enforces the
// schema and header contract and the decompressed-size bound.
func DecodeMessage(msg *nats.Msg) (*ingestpb.Envelope, error) {
	if msg == nil {
		return nil, fmt.Errorf("%w: nil message", ErrInvalidEnvelope)
	}
	if msg.Header.Get(EncodingHeader) != EncodingZstd {
		if msg.Header.Get(EncodingHeader) == "" {
			return nil, ErrMissingEncoding
		}
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedEncoding, msg.Header.Get(EncodingHeader))
	}
	envelope, err := decodeEnvelopeData(msg.Data)
	if err != nil {
		return nil, err
	}
	if header := msg.Header.Get(SchemaHeader); header != "" {
		version, parseErr := strconv.ParseUint(header, 10, 32)
		if parseErr != nil || uint32(version) != envelope.GetSchemaVersion() {
			return nil, fmt.Errorf("%w: header %q does not match envelope %d", ErrUnsupportedSchema, header, envelope.GetSchemaVersion())
		}
	}
	return envelope, nil
}

// DecodeEnvelopeData decompresses and parses an envelope payload. It is used
// by tests and by tools that read the WAL without NATS headers.
func DecodeEnvelopeData(data []byte) (*ingestpb.Envelope, error) {
	return decodeEnvelopeData(data)
}

func decodeEnvelopeData(data []byte) (*ingestpb.Envelope, error) {
	decoder, err := zstd.NewReader(bytes.NewReader(data),
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(MaxDecompressedBytes*2),
	)
	if err != nil {
		return nil, fmt.Errorf("create zstd decoder: %w", err)
	}
	defer decoder.Close()

	raw, err := io.ReadAll(io.LimitReader(decoder, MaxDecompressedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("decompress envelope: %w", err)
	}
	if len(raw) > MaxDecompressedBytes {
		return nil, fmt.Errorf("%w: decompressed envelope exceeds %d bytes", ErrInvalidEnvelope, MaxDecompressedBytes)
	}
	envelope := &ingestpb.Envelope{}
	if err := proto.Unmarshal(raw, envelope); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if err := ValidateEnvelope(envelope); err != nil {
		return nil, err
	}
	return envelope, nil
}

// ValidateEnvelope checks the WAL contract and that every request has one
// event_id per leaf event. Writers call it before persisting a message.
func ValidateEnvelope(envelope *ingestpb.Envelope) error {
	if envelope.GetSchemaVersion() != SchemaVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedSchema, envelope.GetSchemaVersion())
	}
	if envelope.GetMessageId() == "" {
		return fmt.Errorf("%w: message_id is required", ErrInvalidEnvelope)
	}
	if envelope.GetTenantId() == "" || envelope.GetProjectId() == "" {
		return fmt.Errorf("%w: tenant_id and project_id are required", ErrInvalidEnvelope)
	}
	signal := ingest.Signal(envelope.GetSignal())
	switch signal {
	case ingest.SignalTraces, ingest.SignalLogs, ingest.SignalMetrics:
	default:
		return fmt.Errorf("%w: signal %q", ErrInvalidEnvelope, envelope.GetSignal())
	}
	if envelope.GetCreatedAtUnixNano() <= 0 {
		return fmt.Errorf("%w: created_at_unix_nano is required", ErrInvalidEnvelope)
	}
	if len(envelope.GetRequests()) == 0 {
		return fmt.Errorf("%w: at least one request is required", ErrInvalidEnvelope)
	}
	for index, request := range envelope.GetRequests() {
		if request.GetRequestId() == "" {
			return fmt.Errorf("%w: request %d has no request_id", ErrInvalidEnvelope, index)
		}
		if len(request.GetOtlpProtobuf()) == 0 {
			return fmt.Errorf("%w: request %d has no payload", ErrInvalidEnvelope, index)
		}
		if request.GetReceivedAtUnixNano() <= 0 {
			return fmt.Errorf("%w: request %d has no received_at_unix_nano", ErrInvalidEnvelope, index)
		}
		events, err := countPayload(signal, request.GetOtlpProtobuf())
		if err != nil {
			return fmt.Errorf("%w: request %d: %v", ErrInvalidEnvelope, index, err)
		}
		if events == 0 {
			return fmt.Errorf("%w: request %d has no events", ErrInvalidEnvelope, index)
		}
		if len(request.GetEventIds()) != events {
			return fmt.Errorf(
				"%w: request %d has %d event_ids for %d events",
				ErrInvalidEnvelope, index, len(request.GetEventIds()), events,
			)
		}
	}
	return nil
}

func countPayload(signal ingest.Signal, payload []byte) (int, error) {
	batch := &ingest.Batch{Signal: signal}
	switch signal {
	case ingest.SignalTraces:
		request := &tracepb.ExportTraceServiceRequest{}
		if err := proto.Unmarshal(payload, request); err != nil {
			return 0, fmt.Errorf("decode OTLP traces: %w", err)
		}
		batch.Traces = request
	case ingest.SignalLogs:
		request := &logspb.ExportLogsServiceRequest{}
		if err := proto.Unmarshal(payload, request); err != nil {
			return 0, fmt.Errorf("decode OTLP logs: %w", err)
		}
		batch.Logs = request
	case ingest.SignalMetrics:
		request := &metricpb.ExportMetricsServiceRequest{}
		if err := proto.Unmarshal(payload, request); err != nil {
			return 0, fmt.Errorf("decode OTLP metrics: %w", err)
		}
		batch.Metrics = request
	default:
		return 0, fmt.Errorf("unsupported signal %q", signal)
	}
	return batch.Events(), nil
}
