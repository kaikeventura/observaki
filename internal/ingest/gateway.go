package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/observaki/observaki/internal/id"
	logspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// DefaultShutdownTimeout bounds how long the gateway drains in-flight requests
// after SIGTERM before it closes the listeners.
const DefaultShutdownTimeout = 15 * time.Second

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	maxHeaderBytes    = 1 << 20
)

// Config configures the OTLP gateway.
type Config struct {
	// GRPCAddr is the OTLP/gRPC listen address (default port 4317).
	GRPCAddr string
	// HTTPAddr is the OTLP/HTTP listen address (default port 4318).
	HTTPAddr string
	// GRPCListener and HTTPListener override the addresses; tests use them to
	// bind ephemeral ports.
	GRPCListener net.Listener
	HTTPListener net.Listener

	// MaxRequestBytes is the largest request body accepted, after
	// decompression (10 MiB by default in the process config).
	MaxRequestBytes int64
	// MaxEvents is the largest number of spans, log records or data points
	// accepted in one request.
	MaxEvents int

	// ShutdownTimeout bounds the graceful drain. Zero uses
	// DefaultShutdownTimeout.
	ShutdownTimeout time.Duration
	// Redaction configures the pipeline redaction stage.
	Redaction RedactionConfig
	// Auth enables token authentication and per-tenant quotas. Nil disables
	// both; the observaki gateway command always requires it.
	Auth *AuthConfig
	// Sink receives validated, redacted batches. Required.
	Sink Sink
	// Logger receives structured operational logs. Nil discards logs.
	Logger *slog.Logger
}

// Gateway is the stateless OTLP ingestion service (ADR-0002).
type Gateway struct {
	cfg      Config
	sink     Sink
	redactor *redactor
	auth     *Authenticator
	logger   *slog.Logger
	metrics  *gatewayMetrics
	health   *health.Server

	grpcServer   *grpc.Server
	httpServer   *http.Server
	grpcListener net.Listener
	httpListener net.Listener

	ready        atomic.Bool
	shutdownOnce sync.Once
	shutdownErr  error
}

// New builds a gateway and binds its listeners. Call Run to serve requests.
func New(cfg Config) (*Gateway, error) {
	if cfg.Sink == nil {
		return nil, errors.New("ingest: sink is required")
	}
	if cfg.MaxRequestBytes <= 0 {
		return nil, errors.New("ingest: MaxRequestBytes must be positive")
	}
	if cfg.MaxEvents <= 0 {
		return nil, errors.New("ingest: MaxEvents must be positive")
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = DefaultShutdownTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}

	grpcListener, err := listener(cfg.GRPCAddr, cfg.GRPCListener)
	if err != nil {
		return nil, fmt.Errorf("listen otlp grpc on %q: %w", cfg.GRPCAddr, err)
	}
	httpListener, err := listener(cfg.HTTPAddr, cfg.HTTPListener)
	if err != nil {
		_ = grpcListener.Close()
		return nil, fmt.Errorf("listen otlp http on %q: %w", cfg.HTTPAddr, err)
	}

	g := &Gateway{
		cfg:          cfg,
		sink:         cfg.Sink,
		redactor:     newRedactor(cfg.Redaction),
		logger:       cfg.Logger,
		metrics:      &gatewayMetrics{latency: newLatencyWindows()},
		health:       health.NewServer(),
		grpcListener: grpcListener,
		httpListener: httpListener,
	}
	if cfg.Auth != nil {
		authCfg := *cfg.Auth
		if authCfg.Logger == nil {
			authCfg.Logger = cfg.Logger
		}
		authenticator, err := NewAuthenticator(authCfg)
		if err != nil {
			_ = grpcListener.Close()
			_ = httpListener.Close()
			return nil, err
		}
		g.auth = authenticator
	}

	g.grpcServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(int(cfg.MaxRequestBytes)),
		grpc.ChainUnaryInterceptor(g.unaryInterceptor),
	)
	tracepb.RegisterTraceServiceServer(g.grpcServer, &traceService{gateway: g})
	logspb.RegisterLogsServiceServer(g.grpcServer, &logsService{gateway: g})
	metricpb.RegisterMetricsServiceServer(g.grpcServer, &metricsService{gateway: g})
	healthpb.RegisterHealthServer(g.grpcServer, g.health)
	reflection.Register(g.grpcServer)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", g.handleTraces)
	mux.HandleFunc("/v1/logs", g.handleLogs)
	mux.HandleFunc("/v1/metrics", g.handleMetrics)
	mux.HandleFunc("/healthz", g.handleHealthz)
	mux.HandleFunc("/readyz", g.handleReadyz)
	mux.Handle("/metrics", g.handleSelfMetrics())
	mux.HandleFunc("/", g.handleNotFound)

	g.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	return g, nil
}

// GRPCAddr returns the bound OTLP/gRPC address.
func (g *Gateway) GRPCAddr() string {
	if g.grpcListener == nil {
		return ""
	}
	return g.grpcListener.Addr().String()
}

// HTTPAddr returns the bound OTLP/HTTP address.
func (g *Gateway) HTTPAddr() string {
	if g.httpListener == nil {
		return ""
	}
	return g.httpListener.Addr().String()
}

// Run serves OTLP until ctx is canceled, then stops accepting new requests,
// drains in-flight batches and closes the sink. It returns nil on a clean
// shutdown.
func (g *Gateway) Run(ctx context.Context) error {
	g.ready.Store(true)
	g.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	if g.auth != nil {
		g.auth.Start()
	}
	g.logger.Info(
		"otlp gateway listening",
		"grpc_addr", g.GRPCAddr(),
		"http_addr", g.HTTPAddr(),
		"max_request_bytes", g.cfg.MaxRequestBytes,
		"max_events", g.cfg.MaxEvents,
		"auth_enabled", g.auth != nil,
	)

	errCh := make(chan error, 2)
	go func() { errCh <- serveGRPC(g.grpcServer, g.grpcListener) }()
	go func() { errCh <- serveHTTP(g.httpServer, g.httpListener) }()

	var runErr error
	received := 0
	select {
	case <-ctx.Done():
		g.logger.Info("otlp gateway shutting down")
	case err := <-errCh:
		received = 1
		runErr = err
	}

	g.ready.Store(false)
	g.health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), g.cfg.ShutdownTimeout)
	defer cancel()
	if err := g.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = err
	}

	for received < 2 {
		if err := <-errCh; err != nil && runErr == nil {
			runErr = err
		}
		received++
	}
	return runErr
}

// Shutdown stops the listeners, drains in-flight requests and closes the sink.
// It is safe to call more than once.
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.shutdownOnce.Do(func() {
		var wg sync.WaitGroup
		var httpErr, grpcErr error

		wg.Add(2)
		go func() {
			defer wg.Done()
			httpErr = g.httpServer.Shutdown(ctx)
		}()
		go func() {
			defer wg.Done()
			done := make(chan struct{})
			go func() {
				g.grpcServer.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
			case <-ctx.Done():
				g.grpcServer.Stop()
				<-done
				grpcErr = ctx.Err()
			}
		}()
		wg.Wait()

		if errors.Is(httpErr, http.ErrServerClosed) {
			httpErr = nil
		}
		if g.auth != nil {
			g.auth.Close()
		}
		if closer, ok := g.sink.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				grpcErr = errors.Join(grpcErr, err)
			}
		}
		g.shutdownErr = errors.Join(httpErr, grpcErr)
	})
	return g.shutdownErr
}

func listener(addr string, provided net.Listener) (net.Listener, error) {
	if provided != nil {
		return provided, nil
	}
	if addr == "" {
		return nil, errors.New("address is required when no listener is provided")
	}
	return net.Listen("tcp", addr)
}

func serveGRPC(server *grpc.Server, listener net.Listener) error {
	err := server.Serve(listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

func serveHTTP(server *http.Server, listener net.Listener) error {
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

type requestIDKey struct{}

func (g *Gateway) unaryInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx = context.WithValue(ctx, requestIDKey{}, id.NewUUIDv7())
	g.metrics.inFlight.Add(1)
	defer g.metrics.inFlight.Add(-1)
	return handler(ctx, req)
}

func requestIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDKey{}).(string); ok && value != "" {
		return value
	}
	return id.NewUUIDv7()
}
