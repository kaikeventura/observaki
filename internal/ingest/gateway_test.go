package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type blockingSink struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingSink() *blockingSink {
	return &blockingSink{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (s *blockingSink) Consume(ctx context.Context, batch *Batch) (Result, error) {
	s.once.Do(func() { close(s.started) })
	select {
	case <-s.release:
		return Result{Accepted: batch.Events()}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func TestGatewayGracefulShutdownDrainsHTTP(t *testing.T) {
	sink := newBlockingSink()
	gw := newTestGateway(t, sink, nil)
	run := startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("hello")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}

	type response struct {
		status int
		err    error
	}
	responses := make(chan response, 1)
	go func() {
		httpResponse, err := testHTTPClient.Post(
			"http://"+gw.HTTPAddr()+"/v1/logs",
			"application/x-protobuf",
			bytes.NewReader(body),
		)
		if err != nil {
			responses <- response{err: err}
			return
		}
		defer closeResponse(t, httpResponse)
		responses <- response{status: httpResponse.StatusCode}
	}()

	select {
	case <-sink.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the request did not reach the sink")
	}

	run.cancel()
	select {
	case <-run.stopped:
		t.Fatal("Run() returned before the in-flight request drained")
	case <-time.After(100 * time.Millisecond):
	}

	close(sink.release)
	select {
	case got := <-responses:
		if got.err != nil {
			t.Fatalf("POST /v1/logs error = %v", got.err)
		}
		if got.status != http.StatusOK {
			t.Fatalf("POST /v1/logs status = %d, want 200", got.status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight request did not complete")
	}

	select {
	case <-run.stopped:
		if run.err != nil {
			t.Fatalf("Run() error = %v", run.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gateway did not stop")
	}
}

func TestGatewayGracefulShutdownDrainsGRPC(t *testing.T) {
	sink := newBlockingSink()
	gw := newTestGateway(t, sink, nil)
	run := startTestGateway(t, gw)
	waitReady(t, gw)

	conn := dialGRPC(t, gw.GRPCAddr())
	client := collectortracepb.NewTraceServiceClient(conn)

	type response struct {
		err error
	}
	responses := make(chan response, 1)
	go func() {
		_, err := client.Export(context.Background(), tracesRequest(&tracepb.Span{Name: "test"}))
		responses <- response{err: err}
	}()

	select {
	case <-sink.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the request did not reach the sink")
	}

	run.cancel()
	select {
	case <-run.stopped:
		t.Fatal("Run() returned before the in-flight request drained")
	case <-time.After(100 * time.Millisecond):
	}

	close(sink.release)
	select {
	case got := <-responses:
		if got.err != nil {
			t.Fatalf("Export() error = %v", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight request did not complete")
	}

	select {
	case <-run.stopped:
		if run.err != nil {
			t.Fatalf("Run() error = %v", run.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gateway did not stop")
	}
}

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "nil sink", mutate: func(cfg *Config) { cfg.Sink = nil }},
		{name: "zero max request bytes", mutate: func(cfg *Config) { cfg.MaxRequestBytes = 0 }},
		{name: "negative max request bytes", mutate: func(cfg *Config) { cfg.MaxRequestBytes = -1 }},
		{name: "zero max events", mutate: func(cfg *Config) { cfg.MaxEvents = 0 }},
		{name: "negative max events", mutate: func(cfg *Config) { cfg.MaxEvents = -1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grpcListener := mustListen(t)
			httpListener := mustListen(t)
			defer func() { _ = grpcListener.Close() }()
			defer func() { _ = httpListener.Close() }()

			cfg := Config{
				GRPCListener:    grpcListener,
				HTTPListener:    httpListener,
				MaxRequestBytes: 1024,
				MaxEvents:       10,
				Sink:            DiscardSink{},
			}
			test.mutate(&cfg)
			if _, err := New(cfg); err == nil {
				t.Fatal("New() error = nil, want a validation error")
			}
		})
	}
}

func TestGatewayShutdownIsIdempotent(t *testing.T) {
	gw := newTestGateway(t, DiscardSink{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gw.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := gw.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

func TestGatewaySinkErrorMapsToUnavailable(t *testing.T) {
	sink := &recordingSink{result: func(*Batch) (Result, error) {
		return Result{}, errors.New("wal is down")
	}}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("hello")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response, err := testHTTPClient.Post("http://"+gw.HTTPAddr()+"/v1/logs", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/logs error = %v", err)
	}
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.StatusCode)
	}
	if got := response.Header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestGRPCBackpressureIsRetryable(t *testing.T) {
	// Unavailable is the OTLP-recommended retryable code; the sink error path
	// must not surface as InvalidArgument or Internal.
	sink := &recordingSink{result: func(*Batch) (Result, error) {
		return Result{}, errors.New("wal is down")
	}}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	conn := dialGRPC(t, gw.GRPCAddr())
	client := collectortracepb.NewTraceServiceClient(conn)
	_, err := client.Export(context.Background(), tracesRequest(&tracepb.Span{Name: "test"}))
	if err == nil {
		t.Fatal("Export() error = nil, want Unavailable")
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", got)
	}
}

func TestGatewayWALBackpressureMapsTo429(t *testing.T) {
	sink := &recordingSink{result: func(*Batch) (Result, error) {
		return Result{}, fmt.Errorf("%w: queue full", ErrBackpressure)
	}}
	gw := newTestGateway(t, sink, nil)
	startTestGateway(t, gw)
	waitReady(t, gw)

	body, err := proto.Marshal(logsRequest(&logspb.LogRecord{Body: stringValue("hello")}))
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	response, err := testHTTPClient.Post("http://"+gw.HTTPAddr()+"/v1/logs", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/logs error = %v", err)
	}
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.StatusCode)
	}
	if got := response.Header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

type unhealthySink struct {
	recordingSink
}

func (*unhealthySink) Healthy() error { return errors.New("wal disconnected") }

func TestReadyzFailsWhenWALSinkIsUnhealthy(t *testing.T) {
	gw := newTestGateway(t, &unhealthySink{}, nil)
	startTestGateway(t, gw)
	deadline := time.Now().Add(2 * time.Second)
	for {
		response, err := testHTTPClient.Get("http://" + gw.HTTPAddr() + "/healthz")
		if err == nil {
			_ = response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("gateway did not start: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	response, err := testHTTPClient.Get("http://" + gw.HTTPAddr() + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz error = %v", err)
	}
	defer closeResponse(t, response)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.StatusCode)
	}
}
