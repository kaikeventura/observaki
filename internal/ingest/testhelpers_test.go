package ingest

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var testHTTPClient = &http.Client{
	Transport: &http.Transport{DisableKeepAlives: true},
}

type recordingSink struct {
	mu      sync.Mutex
	batches []*Batch
	result  func(*Batch) (Result, error)
}

func (s *recordingSink) Consume(_ context.Context, batch *Batch) (Result, error) {
	s.mu.Lock()
	s.batches = append(s.batches, batch)
	s.mu.Unlock()
	if s.result != nil {
		return s.result(batch)
	}
	return Result{Accepted: batch.Events()}, nil
}

func (s *recordingSink) last(t *testing.T) *Batch {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.batches) == 0 {
		t.Fatal("sink received no batches")
	}
	return s.batches[len(s.batches)-1]
}

func (s *recordingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.batches)
}

func newTestGateway(t *testing.T, sink Sink, mutate func(*Config)) *Gateway {
	t.Helper()
	cfg := Config{
		GRPCListener:    mustListen(t),
		HTTPListener:    mustListen(t),
		MaxRequestBytes: 1 << 20,
		MaxEvents:       100,
		Sink:            sink,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Redaction:       DefaultRedactionConfig(),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = gw.Shutdown(ctx)
	})
	return gw
}

type gatewayRun struct {
	cancel  context.CancelFunc
	stopped chan struct{}
	err     error
}

func startTestGateway(t *testing.T, gw *Gateway) *gatewayRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	run := &gatewayRun{
		cancel:  cancel,
		stopped: make(chan struct{}),
	}
	go func() {
		run.err = gw.Run(ctx)
		close(run.stopped)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-run.stopped:
			if run.err != nil {
				t.Errorf("Run() error = %v", run.err)
			}
		case <-time.After(5 * time.Second):
			t.Error("gateway did not stop")
		}
	})
	return run
}

func waitReady(t *testing.T, gw *Gateway) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := testHTTPClient.Get("http://" + gw.HTTPAddr() + "/readyz")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("gateway did not become ready")
}

func dialGRPC(t *testing.T, addr string, options ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()
	options = append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, options...)
	conn, err := grpc.NewClient(addr, options...)
	if err != nil {
		t.Fatalf("grpc.NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	return listener
}

func logsRequest(records ...*logspb.LogRecord) *collectorlogspb.ExportLogsServiceRequest {
	return &collectorlogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: &resourcepb.Resource{},
			ScopeLogs: []*logspb.ScopeLogs{{
				Scope:      &commonpb.InstrumentationScope{Name: "test"},
				LogRecords: records,
			}},
		}},
	}
}

func tracesRequest(spans ...*tracepb.Span) *collectortracepb.ExportTraceServiceRequest {
	return &collectortracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{Name: "test"},
				Spans: spans,
			}},
		}},
	}
}

func metricsRequest(metrics ...*metricpb.Metric) *collectormetricspb.ExportMetricsServiceRequest {
	return &collectormetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{{
			Resource: &resourcepb.Resource{},
			ScopeMetrics: []*metricpb.ScopeMetrics{{
				Scope:   &commonpb.InstrumentationScope{Name: "test"},
				Metrics: metrics,
			}},
		}},
	}
}

func gaugeMetric(name string, points ...*metricpb.NumberDataPoint) *metricpb.Metric {
	return &metricpb.Metric{
		Name: name,
		Data: &metricpb.Metric_Gauge{Gauge: &metricpb.Gauge{DataPoints: points}},
	}
}

func stringValue(value string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}
}

func stringAttribute(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: stringValue(value)}
}

func kvListValue(values ...*commonpb.KeyValue) *commonpb.AnyValue {
	return &commonpb.AnyValue{
		Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: values}},
	}
}
