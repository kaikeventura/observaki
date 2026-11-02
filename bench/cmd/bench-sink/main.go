// Command bench-sink is a disposable OTLP/HTTP receiver for local benchmark
// validation. It accepts any POST, drains the body and returns a configurable
// status. It is not the Observaki gateway and must never be used as one: it
// measures the load generator and transport path, not ingestion or storage.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/observaki/observaki/bench/internal/benchversion"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bench-sink: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	var (
		addr        string
		status      int
		logInterval time.Duration
		showVersion bool
	)
	flags := flag.NewFlagSet("bench-sink", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&addr, "addr", "127.0.0.1:4318", "listen address")
	flags.IntVar(&status, "status", http.StatusOK, "HTTP status returned for accepted requests")
	flags.DurationVar(&logInterval, "log-interval", 10*time.Second, "progress log interval")
	flags.BoolVar(&showVersion, "version", false, "print the sink version and exit")
	flags.Usage = func() {
		_, _ = fmt.Fprint(stderr, `bench-sink — disposable OTLP/HTTP receiver for local benchmarks

Usage:
  bench-sink [flags]

Flags:
`)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if showVersion {
		_, err := fmt.Fprintln(stdout, benchversion.String("bench-sink"))
		return err
	}
	if status < 200 || status > 599 {
		return fmt.Errorf("--status must be between 200 and 599")
	}
	if logInterval <= 0 {
		return fmt.Errorf("--log-interval must be greater than zero")
	}

	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := newSink(logger, status)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	server := &http.Server{
		Handler:           sink,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("bench sink ready", "addr", addr, "status", status)
	go logProgress(ctx, logger, sink, logInterval)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	snapshot := sink.snapshot()
	logger.Info("bench sink stopped", "requests", snapshot.Requests, "bytes", snapshot.Bytes, "rejected", snapshot.Rejected)
	return nil
}

type sink struct {
	logger   *slog.Logger
	status   int
	requests atomic.Int64
	bytes    atomic.Int64
	rejected atomic.Int64
}

type sinkSnapshot struct {
	Requests int64
	Bytes    int64
	Rejected int64
}

func newSink(logger *slog.Logger, status int) *sink {
	return &sink{logger: logger, status: status}
}

func (s *sink) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/healthz":
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "ok\n")
		return
	case request.Method == http.MethodGet && request.URL.Path == "/":
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "observaki bench sink\n")
		return
	case request.Method != http.MethodPost:
		s.rejected.Add(1)
		writer.Header().Set("Allow", "POST")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bytes, err := io.Copy(io.Discard, request.Body)
	if err != nil {
		s.rejected.Add(1)
		http.Error(writer, "read request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.requests.Add(1)
	s.bytes.Add(bytes)
	writer.WriteHeader(s.status)
}

func (s *sink) snapshot() sinkSnapshot {
	return sinkSnapshot{
		Requests: s.requests.Load(),
		Bytes:    s.bytes.Load(),
		Rejected: s.rejected.Load(),
	}
}

func logProgress(ctx context.Context, logger *slog.Logger, s *sink, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var previous sinkSnapshot
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := s.snapshot()
			logger.Info("bench sink progress",
				"requests", current.Requests,
				"requests_delta", current.Requests-previous.Requests,
				"bytes", current.Bytes,
				"bytes_delta", current.Bytes-previous.Bytes,
				"rejected", current.Rejected,
			)
			previous = current
		}
	}
}
