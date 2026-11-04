// Package httpserver exposes the Observaki HTTP API and the web UI.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
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

// Server serves the Observaki API and UI over HTTP.
type Server struct {
	version         string
	webDir          string
	shutdownTimeout time.Duration
	logger          *slog.Logger
	httpServer      *http.Server
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
	}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", http.HandlerFunc(s.handleHealthz))
	mux.Handle("GET /api/v1/health", http.HandlerFunc(s.handleHealth))
	mux.Handle("/api/", http.HandlerFunc(handleAPINotFound))
	if s.webDir != "" {
		mux.Handle("/", newStaticHandler(s.webDir, logger))
	} else {
		mux.HandleFunc("/", handleNoWebUI)
	}

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
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
