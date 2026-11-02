package loadgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpcgzip "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/protobuf/proto"

	logscollectorpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricscollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// SendResult describes one export request.
type SendResult struct {
	// Bytes is the uncompressed protobuf size of the request.
	Bytes int
	// CompressedBytes is the request size after transport compression. It is
	// zero when the transport does not expose it (for example gRPC/gzip).
	CompressedBytes int
	// Rejected is the number of events rejected by the receiver through the
	// OTLP partial-success response.
	Rejected int
}

// Sender exports OTLP batches.
type Sender interface {
	Send(ctx context.Context, batch *Batch) (SendResult, error)
	Close() error
}

// NewSender builds the sender for the configured protocol. A dry-run sender
// discards requests after measuring them.
func NewSender(cfg Config) (Sender, error) {
	if cfg.DryRun {
		return discardSender{}, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	switch cfg.Protocol {
	case ProtocolGRPC:
		return newGRPCSender(cfg)
	case ProtocolHTTP:
		return newHTTPSender(cfg)
	default:
		return nil, fmt.Errorf("unsupported protocol %q", cfg.Protocol)
	}
}

type discardSender struct{}

func (discardSender) Send(_ context.Context, batch *Batch) (SendResult, error) {
	return SendResult{Bytes: batch.Size()}, nil
}

func (discardSender) Close() error { return nil }

type grpcSender struct {
	conn       *grpc.ClientConn
	logs       logscollectorpb.LogsServiceClient
	metrics    metricscollectorpb.MetricsServiceClient
	traces     tracecollectorpb.TraceServiceClient
	compressed bool
}

func newGRPCSender(cfg Config) (*grpcSender, error) {
	options := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	if cfg.Compression == CompressionGzip {
		options = append(options, grpc.WithDefaultCallOptions(grpc.UseCompressor(grpcgzip.Name)))
	}
	conn, err := grpc.NewClient(cfg.Endpoint, options...)
	if err != nil {
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}
	return &grpcSender{
		conn:       conn,
		logs:       logscollectorpb.NewLogsServiceClient(conn),
		metrics:    metricscollectorpb.NewMetricsServiceClient(conn),
		traces:     tracecollectorpb.NewTraceServiceClient(conn),
		compressed: cfg.Compression == CompressionGzip,
	}, nil
}

func (s *grpcSender) Send(ctx context.Context, batch *Batch) (SendResult, error) {
	result := SendResult{Bytes: batch.Size()}
	if !s.compressed {
		// Without transport compression the wire size equals the protobuf
		// size. gRPC does not expose the compressed frame size.
		result.CompressedBytes = result.Bytes
	}
	switch batch.Signal {
	case Logs:
		response, err := s.logs.Export(ctx, batch.Logs)
		if err != nil {
			return result, err
		}
		result.Rejected = int(response.GetPartialSuccess().GetRejectedLogRecords())
	case Metrics:
		response, err := s.metrics.Export(ctx, batch.Metrics)
		if err != nil {
			return result, err
		}
		result.Rejected = int(response.GetPartialSuccess().GetRejectedDataPoints())
	case Traces:
		response, err := s.traces.Export(ctx, batch.Traces)
		if err != nil {
			return result, err
		}
		result.Rejected = int(response.GetPartialSuccess().GetRejectedSpans())
	default:
		return result, fmt.Errorf("unsupported signal %s", batch.Signal)
	}
	return result, nil
}

func (s *grpcSender) Close() error {
	return s.conn.Close()
}

type httpSender struct {
	client  *http.Client
	baseURL string
	gzip    bool
}

func newHTTPSender(cfg Config) (*httpSender, error) {
	parsed, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse HTTP endpoint: %w", err)
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return &httpSender{
		client:  &http.Client{Timeout: cfg.RequestTimeout},
		baseURL: parsed.String(),
		gzip:    cfg.Compression == CompressionGzip,
	}, nil
}

func (s *httpSender) Send(ctx context.Context, batch *Batch) (SendResult, error) {
	message := batch.Proto()
	if message == nil {
		return SendResult{}, errors.New("batch has no protobuf message")
	}
	raw, err := proto.Marshal(message)
	if err != nil {
		return SendResult{}, fmt.Errorf("marshal %s: %w", batch.Signal, err)
	}
	body := raw
	if s.gzip {
		body, err = gzipBytes(raw)
		if err != nil {
			return SendResult{}, fmt.Errorf("compress %s: %w", batch.Signal, err)
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+batch.HTTPPath(), bytes.NewReader(body))
	if err != nil {
		return SendResult{}, fmt.Errorf("create HTTP request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("User-Agent", "observaki-loadgen/1.0")
	if s.gzip {
		request.Header.Set("Content-Encoding", "gzip")
	}

	response, err := s.client.Do(request)
	if err != nil {
		return SendResult{}, err
	}
	defer func() { _ = response.Body.Close() }()

	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return SendResult{}, fmt.Errorf("read HTTP response: %w", readErr)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return SendResult{}, fmt.Errorf("HTTP %d: %s", response.StatusCode, truncate(string(payload), 256))
	}

	result := SendResult{Bytes: len(raw), CompressedBytes: len(body)}
	if len(payload) == 0 {
		return result, nil
	}
	switch batch.Signal {
	case Logs:
		var decoded logscollectorpb.ExportLogsServiceResponse
		if err := proto.Unmarshal(payload, &decoded); err != nil {
			return result, fmt.Errorf("decode logs response: %w", err)
		}
		result.Rejected = int(decoded.GetPartialSuccess().GetRejectedLogRecords())
	case Metrics:
		var decoded metricscollectorpb.ExportMetricsServiceResponse
		if err := proto.Unmarshal(payload, &decoded); err != nil {
			return result, fmt.Errorf("decode metrics response: %w", err)
		}
		result.Rejected = int(decoded.GetPartialSuccess().GetRejectedDataPoints())
	case Traces:
		var decoded tracecollectorpb.ExportTraceServiceResponse
		if err := proto.Unmarshal(payload, &decoded); err != nil {
			return result, fmt.Errorf("decode traces response: %w", err)
		}
		result.Rejected = int(decoded.GetPartialSuccess().GetRejectedSpans())
	}
	return result, nil
}

func (s *httpSender) Close() error {
	s.client.CloseIdleConnections()
	return nil
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
