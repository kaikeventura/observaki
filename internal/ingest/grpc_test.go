package ingest

import (
	"context"
	"strings"
	"testing"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestGRPCAcceptsAllSignals(t *testing.T) {
	tests := []struct {
		name   string
		events int
		signal Signal
		call   func(context.Context, *grpc.ClientConn) (proto.Message, error)
	}{
		{
			name:   "traces",
			events: 2,
			signal: SignalTraces,
			call: func(ctx context.Context, conn *grpc.ClientConn) (proto.Message, error) {
				return collectortracepb.NewTraceServiceClient(conn).Export(ctx, tracesRequest(
					&tracepb.Span{Name: "one"},
					&tracepb.Span{Name: "two"},
				))
			},
		},
		{
			name:   "logs",
			events: 2,
			signal: SignalLogs,
			call: func(ctx context.Context, conn *grpc.ClientConn) (proto.Message, error) {
				return collectorlogspb.NewLogsServiceClient(conn).Export(ctx, logsRequest(
					&logspb.LogRecord{Body: stringValue("one")},
					&logspb.LogRecord{Body: stringValue("two")},
				))
			},
		},
		{
			name:   "metrics",
			events: 2,
			signal: SignalMetrics,
			call: func(ctx context.Context, conn *grpc.ClientConn) (proto.Message, error) {
				return collectormetricspb.NewMetricsServiceClient(conn).Export(ctx, metricsRequest(
					gaugeMetric("test.metric", &metricpb.NumberDataPoint{}, &metricpb.NumberDataPoint{}),
				))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := &recordingSink{}
			gw := newTestGateway(t, sink, nil)
			startTestGateway(t, gw)
			waitReady(t, gw)

			response, err := test.call(context.Background(), dialGRPC(t, gw.GRPCAddr()))
			if err != nil {
				t.Fatalf("Export() error = %v", err)
			}
			if response == nil {
				t.Fatal("Export() response = nil")
			}

			batch := sink.last(t)
			if batch.Signal != test.signal {
				t.Errorf("sink signal = %q, want %q", batch.Signal, test.signal)
			}
			if got := batch.Events(); got != test.events {
				t.Errorf("sink events = %d, want %d", got, test.events)
			}
		})
	}
}

func TestGRPCPartialSuccess(t *testing.T) {
	sink := &recordingSink{result: func(batch *Batch) (Result, error) {
		return Result{
			Accepted: batch.Events() - 1,
			Rejected: 1,
			Reason:   "one record was invalid",
		}, nil
	}}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	client := collectorlogspb.NewLogsServiceClient(dialGRPC(t, gw.GRPCAddr()))
	response, err := client.Export(context.Background(), logsRequest(
		&logspb.LogRecord{Body: stringValue("one")},
		&logspb.LogRecord{Body: stringValue("two")},
		&logspb.LogRecord{Body: stringValue("three")},
	))
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if response.GetPartialSuccess() == nil {
		t.Fatal("PartialSuccess = nil, want rejected count and reason")
	}
	if got := response.GetPartialSuccess().GetRejectedLogRecords(); got != 1 {
		t.Errorf("RejectedLogRecords = %d, want 1", got)
	}
	if got := response.GetPartialSuccess().GetErrorMessage(); got != "one record was invalid" {
		t.Errorf("ErrorMessage = %q", got)
	}
}

func TestGRPCEventLimit(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, func(cfg *Config) { cfg.MaxEvents = 2 })
	startTestGateway(t, gw)
	waitReady(t, gw)

	client := collectorlogspb.NewLogsServiceClient(dialGRPC(t, gw.GRPCAddr()))
	_, err := client.Export(context.Background(), logsRequest(
		&logspb.LogRecord{Body: stringValue("one")},
		&logspb.LogRecord{Body: stringValue("two")},
		&logspb.LogRecord{Body: stringValue("three")},
	))
	if err == nil {
		t.Fatal("Export() error = nil, want ResourceExhausted")
	}
	if got := status.Code(err); got != codes.ResourceExhausted {
		t.Fatalf("code = %v, want ResourceExhausted", got)
	}
	if !strings.Contains(err.Error(), "limit is 2") {
		t.Errorf("error %q does not explain the limit", err)
	}
	if got := sink.count(); got != 0 {
		t.Errorf("sink received %d batches, want 0", got)
	}
}

func TestGRPCRequestSizeLimit(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, func(cfg *Config) { cfg.MaxRequestBytes = 512 })
	startTestGateway(t, gw)
	waitReady(t, gw)

	client := collectorlogspb.NewLogsServiceClient(dialGRPC(t, gw.GRPCAddr()))
	_, err := client.Export(context.Background(), logsRequest(
		&logspb.LogRecord{Body: stringValue(strings.Repeat("x", 2048))},
	))
	if err == nil {
		t.Fatal("Export() error = nil, want ResourceExhausted")
	}
	if got := status.Code(err); got != codes.ResourceExhausted {
		t.Fatalf("code = %v, want ResourceExhausted", got)
	}
	if got := sink.count(); got != 0 {
		t.Errorf("sink received %d batches, want 0", got)
	}
}

func TestGRPCGzip(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	client := collectorlogspb.NewLogsServiceClient(dialGRPC(t, gw.GRPCAddr(),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
	))
	if _, err := client.Export(context.Background(), logsRequest(
		&logspb.LogRecord{Body: stringValue("compressed")},
	)); err != nil {
		t.Fatalf("Export() with gzip error = %v", err)
	}
	if got := sink.last(t).Events(); got != 1 {
		t.Errorf("sink events = %d, want 1", got)
	}
}

func TestGRPCRedactionPreservesTraceCorrelation(t *testing.T) {
	sink := &recordingSink{}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	traceID := []byte{0x5b, 0x8e, 0xff, 0xf7, 0x98, 0x03, 0x81, 0x03, 0xd2, 0x69, 0xb6, 0x33, 0x81, 0x3f, 0xc6, 0x0c}
	spanID := []byte{0xee, 0x48, 0x02, 0xb5, 0xb8, 0xc4, 0xc0, 0xb1}
	client := collectorlogspb.NewLogsServiceClient(dialGRPC(t, gw.GRPCAddr()))
	if _, err := client.Export(context.Background(), logsRequest(&logspb.LogRecord{
		TraceId: traceID,
		SpanId:  spanID,
		Body:    stringValue("paid with 4111 1111 1111 1111"),
		Attributes: []*commonpb.KeyValue{
			stringAttribute("credit_card", "4111 1111 1111 1111"),
		},
	})); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	record := sink.last(t).Logs.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()[0]
	if got := record.GetAttributes()[0].GetValue().GetStringValue(); got != "[REDACTED]" {
		t.Errorf("credit_card = %q, want [REDACTED]", got)
	}
	if got := record.GetBody().GetStringValue(); got != "paid with [REDACTED]" {
		t.Errorf("body = %q", got)
	}
	if string(record.GetTraceId()) != string(traceID) || string(record.GetSpanId()) != string(spanID) {
		t.Error("trace/span IDs changed during redaction")
	}
}
