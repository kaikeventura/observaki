// Package httpserver exposes the Observaki HTTP API.
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

// Server serves the Observaki API over HTTP.
type Server struct {
	version         string
	shutdownTimeout time.Duration
	logger          *slog.Logger
	httpServer      *http.Server
}

// New builds a server listening on addr and advertising the given version.
func New(addr, ver string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	s := &Server{
		version:         ver,
		shutdownTimeout: DefaultShutdownTimeout,
		logger:          logger,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", http.HandlerFunc(s.handleHealthz))
	mux.Handle("GET /api/v1/health", http.HandlerFunc(s.handleHealth))

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
		s.logger.Info("http server listening", "addr", s.httpServer.Addr)
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

// handleHealthz reports liveness and never depends on downstream systems.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// handleHealth reports liveness plus the running build version.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.version})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
