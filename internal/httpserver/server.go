// Package httpserver exposes the Observaki HTTP API and the web UI.
package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/observaki/observaki/internal/api"
	"github.com/observaki/observaki/internal/selfobs"
)

const (
	// DefaultShutdownTimeout bounds the graceful shutdown of the server.
	DefaultShutdownTimeout = 10 * time.Second
	readHeaderTimeout      = 5 * time.Second
)

// Option configures the HTTP server.
type Option func(*Server)

// WithWebDir serves the static web build from dir (OBSERVAKI_WEB_DIR), with an
// index.html fallback for client-side routes. An empty or missing directory
// leaves the API reachable and disables the UI.
func WithWebDir(dir string) Option {
	return func(s *Server) {
		s.webDir = dir
	}
}

// HealthOverviewProvider builds the self-observability snapshot.
type HealthOverviewProvider interface {
	Overview(ctx context.Context) selfobs.Overview
}

// WithHealthOverview enables GET /api/v1/health/overview.
func WithHealthOverview(provider HealthOverviewProvider) Option {
	return func(s *Server) {
		s.healthOverview = provider
	}
}

// WithControlPlane enables the authenticated metadata endpoints (projects and
// ingestion tokens). Without it those routes answer 503 while health and the
// OpenAPI contract stay available.
func WithControlPlane(store api.ControlPlane) Option {
	return func(s *Server) {
		s.controlPlane = store
	}
}

// WithRequestMetrics lets the process share the API request counters with the
// self-observability collector (query error rate).
func WithRequestMetrics(metrics *RequestMetrics) Option {
	return func(s *Server) {
		if metrics != nil {
			s.metrics = metrics
		}
	}
}

// Server serves the Observaki API and UI over HTTP.
type Server struct {
	version         string
	webDir          string
	shutdownTimeout time.Duration
	logger          *slog.Logger
	httpServer      *http.Server
	healthOverview  HealthOverviewProvider
	controlPlane    api.ControlPlane
	metrics         *RequestMetrics
	api             *api.API
}

// New builds a server listening on addr and advertising the given version.
func New(addr, ver string, logger *slog.Logger, options ...Option) *Server {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	s := &Server{
		version:         ver,
		shutdownTimeout: DefaultShutdownTimeout,
		logger:          logger,
		metrics:         NewRequestMetrics(),
	}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}

	s.api = api.New(api.Config{
		Version: ver,
		Logger:  logger,
		Overview: func(ctx context.Context) (selfobs.Overview, bool) {
			if s.healthOverview == nil {
				return selfobs.Overview{}, false
			}
			return s.healthOverview.Overview(ctx), true
		},
		ControlPlane: s.controlPlane,
	})

	mux := http.NewServeMux()
	s.api.Register(mux)
	mux.Handle("GET /metrics", s.metrics)
	if s.webDir != "" {
		mux.Handle("/", newStaticHandler(s.webDir, logger))
	} else {
		mux.HandleFunc("/", handleNoWebUI)
	}

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           api.RequestID(api.Recover(logger, s.metrics.middleware(mux))),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return s
}

// Handler returns the HTTP handler, which is useful for tests and embeddings.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Run starts the server and blocks until ctx is canceled, then shuts down
// gracefully. It returns nil on a clean shutdown.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", "addr", s.httpServer.Addr, "web_dir", s.webDir)
		errCh <- s.listenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("http server shutting down", "addr", s.httpServer.Addr)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	return s.httpServer.Shutdown(shutdownCtx)
}

func (s *Server) listenAndServe() error {
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// handleNoWebUI answers every non-API route when the web build is not
// installed, keeping method semantics for the API paths.
func handleNoWebUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		api.WriteError(w, r, http.StatusMethodNotAllowed, api.CodeMethodNotAllowed,
			r.Method+" is not supported", map[string]any{"allow": []string{"GET", "HEAD"}})
		return
	}
	api.WriteError(w, r, http.StatusNotFound, "ui_not_installed",
		"the web build is not installed; set OBSERVAKI_WEB_DIR or run the SvelteKit dev server", nil)
}
