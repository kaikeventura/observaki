// Package httpserver exposes the Observaki HTTP API and the web UI.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

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
	metrics         *RequestMetrics
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

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", http.HandlerFunc(s.handleHealthz))
	mux.Handle("GET /api/v1/health", http.HandlerFunc(s.handleHealth))
	mux.Handle("GET /api/v1/health/overview", http.HandlerFunc(s.handleHealthOverview))
	mux.Handle("GET /metrics", s.metrics)
	// Query Service routes arrive in Phase 1 (Story 5.5). The placeholder keeps
	// the query error-rate metric meaningful and returns a structured error.
	mux.HandleFunc("/api/v1/query", s.handleQueryPlaceholder)
	mux.HandleFunc("/api/v1/query/", s.handleQueryPlaceholder)
	mux.Handle("/api/", http.HandlerFunc(handleAPINotFound))
	if s.webDir != "" {
		mux.Handle("/", newStaticHandler(s.webDir, logger))
	} else {
		mux.HandleFunc("/", handleNoWebUI)
	}

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.metrics.middleware(mux),
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

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// handleHealthz reports liveness and never depends on downstream systems.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// handleHealth reports liveness plus the running build version.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.version})
}

// handleHealthOverview serves the self-observability snapshot for the health
// page (AC3). It is read-only and carries no tenant data.
func (s *Server) handleHealthOverview(w http.ResponseWriter, r *http.Request) {
	if s.healthOverview == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{
			Code:    "health_overview_unavailable",
			Message: "self-observability sources are not configured",
		})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.healthOverview.Overview(r.Context()))
}

// handleQueryPlaceholder keeps /api/v1/query as a structured, counted query
// route until Story 5.5 ships the real sample query.
func (s *Server) handleQueryPlaceholder(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, errorResponse{
		Code:    "not_implemented",
		Message: "the query API arrives in Phase 1; use /api/v1/health/overview for system health",
	})
}

// handleAPINotFound keeps unknown API routes JSON so a client never receives
// the SPA shell instead of an error.
func handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, errorResponse{
		Code:    "not_found",
		Message: "no route matches " + r.URL.Path,
	})
}

// handleNoWebUI answers every non-API route when the web build is not
// installed, keeping method semantics for the API paths.
func handleNoWebUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{
			Code:    "method_not_allowed",
			Message: r.Method + " is not supported",
		})
		return
	}
	writeJSON(w, http.StatusNotFound, errorResponse{
		Code:    "ui_not_installed",
		Message: "the web build is not installed; set OBSERVAKI_WEB_DIR or run the SvelteKit dev server",
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
