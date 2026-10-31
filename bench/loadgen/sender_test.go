package loadgen

import (
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	logscollectorpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricscollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func TestHTTPSenderExportsLogs(t *testing.T) {
	received := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/logs" {
			t.Errorf("path = %s, want /v1/logs", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-protobuf" {
			t.Errorf("Content-Type = %q, want application/x-protobuf", got)
		}
		if got := r.Header.Get("Content-Encoding"); got != "gzip" {
			t.Errorf("Content-Encoding = %q, want gzip", got)
		}

		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("gzip.NewReader() = %v", err)
			http.Error(w, "bad gzip", http.StatusBadRequest)
			return
		}
		defer func() { _ = reader.Close() }()
		payload, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("read body = %v", err)
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		var request logscollectorpb.ExportLogsServiceRequest
		if err := proto.Unmarshal(payload, &request); err != nil {
			t.Errorf("unmarshal request = %v", err)
			http.Error(w, "bad protobuf", http.StatusBadRequest)
			return
		}
		received <- len(request.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords())

		response, err := proto.Marshal(&logscollectorpb.ExportLogsServiceResponse{
			PartialSuccess: &logscollectorpb.ExportLogsPartialSuccess{RejectedLogRecords: 2},
		})
		if err != nil {
			t.Errorf("marshal response = %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(response)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Protocol = ProtocolHTTP
	cfg.Endpoint = server.URL
	cfg.Compression = CompressionGzip
	cfg.RequestTimeout = 5 * time.Second

	sender, err := NewSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sender.Close() }()

	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := builder.BuildBatch(Logs, []int64{0, 1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sender.Send(context.Background(), batch)
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	if got := <-received; got != 5 {
		t.Fatalf("received %d log records, want 5", got)
	}
	if result.Rejected != 2 {
		t.Fatalf("Rejected = %d, want 2", result.Rejected)
	}
	if result.Bytes == 0 {
		t.Fatal("Bytes = 0, want > 0")
	}
}

func TestHTTPSenderReportsStatusErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "receiver unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Protocol = ProtocolHTTP
	cfg.Endpoint = server.URL
	cfg.Compression = CompressionNone

	sender, err := NewSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sender.Close() }()
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := builder.BuildBatch(Metrics, []int64{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sender.Send(context.Background(), batch)
	if err == nil {
		t.Fatal("Send() = nil, want error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("Send() error = %v, want HTTP 503", err)
	}
}

func TestNewSenderDryRun(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DryRun = true
	cfg.Endpoint = ""
	sender, err := NewSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sender.Close() }()
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := builder.BuildBatch(Traces, []int64{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sender.Send(context.Background(), batch)
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	if result.Bytes == 0 {
		t.Fatal("Bytes = 0, want > 0")
	}
}

type testLogsServer struct {
	logscollectorpb.UnimplementedLogsServiceServer
	received chan int
}

func (s *testLogsServer) Export(_ context.Context, request *logscollectorpb.ExportLogsServiceRequest) (*logscollectorpb.ExportLogsServiceResponse, error) {
	s.received <- len(request.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords())
	return &logscollectorpb.ExportLogsServiceResponse{
		PartialSuccess: &logscollectorpb.ExportLogsPartialSuccess{RejectedLogRecords: 2},
	}, nil
}

type testMetricsServer struct {
	metricscollectorpb.UnimplementedMetricsServiceServer
	received chan int
}

func (s *testMetricsServer) Export(_ context.Context, request *metricscollectorpb.ExportMetricsServiceRequest) (*metricscollectorpb.ExportMetricsServiceResponse, error) {
	s.received <- len(request.GetResourceMetrics()[0].GetScopeMetrics()[0].GetMetrics()[0].GetGauge().GetDataPoints())
	return &metricscollectorpb.ExportMetricsServiceResponse{
		PartialSuccess: &metricscollectorpb.ExportMetricsPartialSuccess{RejectedDataPoints: 1},
	}, nil
}

type testTracesServer struct {
	tracecollectorpb.UnimplementedTraceServiceServer
	received chan int
}

func (s *testTracesServer) Export(_ context.Context, request *tracecollectorpb.ExportTraceServiceRequest) (*tracecollectorpb.ExportTraceServiceResponse, error) {
	s.received <- len(request.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans())
	return &tracecollectorpb.ExportTraceServiceResponse{
		PartialSuccess: &tracecollectorpb.ExportTracePartialSuccess{RejectedSpans: 3},
	}, nil
}

func TestGRPCSenderExportsAllSignals(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	logsReceived := make(chan int, 1)
	metricsReceived := make(chan int, 1)
	tracesReceived := make(chan int, 1)

	server := grpc.NewServer()
	logscollectorpb.RegisterLogsServiceServer(server, &testLogsServer{received: logsReceived})
	metricscollectorpb.RegisterMetricsServiceServer(server, &testMetricsServer{received: metricsReceived})
	tracecollectorpb.RegisterTraceServiceServer(server, &testTracesServer{received: tracesReceived})
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	cfg := DefaultConfig()
	cfg.Endpoint = listener.Addr().String()
	cfg.Protocol = ProtocolGRPC
	cfg.Compression = CompressionGzip
	cfg.RequestTimeout = 5 * time.Second

	sender, err := NewSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sender.Close() }()
	builder, err := NewBuilder(cfg)
	if err != nil {
		t.Fatal(err)
	}

	indices := []int64{0, 1, 2, 3}
	logsBatch, _ := builder.BuildBatch(Logs, indices)
	logsResult, err := sender.Send(context.Background(), logsBatch)
	if err != nil {
		t.Fatalf("logs Send() = %v", err)
	}
	if got := <-logsReceived; got != 4 {
		t.Fatalf("logs received = %d, want 4", got)
	}
	if logsResult.Rejected != 2 {
		t.Fatalf("logs rejected = %d, want 2", logsResult.Rejected)
	}

	metricsBatch, _ := builder.BuildBatch(Metrics, indices)
	metricsResult, err := sender.Send(context.Background(), metricsBatch)
	if err != nil {
		t.Fatalf("metrics Send() = %v", err)
	}
	if got := <-metricsReceived; got != 4 {
		t.Fatalf("metrics received = %d, want 4", got)
	}
	if metricsResult.Rejected != 1 {
		t.Fatalf("metrics rejected = %d, want 1", metricsResult.Rejected)
	}

	tracesBatch, _ := builder.BuildBatch(Traces, indices)
	tracesResult, err := sender.Send(context.Background(), tracesBatch)
	if err != nil {
		t.Fatalf("traces Send() = %v", err)
	}
	if got := <-tracesReceived; got != 4 {
		t.Fatalf("traces received = %d, want 4", got)
	}
	if tracesResult.Rejected != 3 {
		t.Fatalf("traces rejected = %d, want 3", tracesResult.Rejected)
	}
}
