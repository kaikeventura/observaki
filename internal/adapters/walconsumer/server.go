package walconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type metricsHandler struct {
	metrics *writerMetrics
}

func (h *metricsHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	var out strings.Builder
	h.metrics.write(&out)
	_, _ = io.WriteString(w, out.String())
}

// MetricsHandler exposes the writer counters in Prometheus text format.
func (c *Consumer) MetricsHandler() http.Handler {
	return &metricsHandler{metrics: c.metrics}
}

// Server is the writer's health and metrics endpoint.
type Server struct {
	addr     string
	consumer *Consumer
	logger   *slog.Logger
}

// NewServer returns the HTTP observability server for the writer.
func NewServer(addr string, consumer *Consumer, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{addr: addr, consumer: consumer, logger: logger}
}

// Handler builds the mux. It is separated from Run so tests can use
// httptest without opening a port.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := s.consumer.Healthy(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable", "error": err.Error()})
			return
		}
		_, _ = io.WriteString(w, `{"status":"ready"}`)
	})
	mux.Handle("/metrics", s.consumer.MetricsHandler())
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.consumer.Stats())
	})
	return mux
}

// Run serves until ctx is canceled and then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errorsCh := make(chan error, 1)
	go func() {
		s.logger.Info("writer observability server listening", "addr", s.addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- fmt.Errorf("writer http server: %w", err)
		}
		close(errorsCh)
	}()

	select {
	case err := <-errorsCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("writer http shutdown: %w", err)
		}
		return nil
	}
}
