// Command webhooksink is a local receiver for the Story 5.4 alerting drill. It
// records every notification as one JSON line so the operator smoke can assert
// delivery, dedupe and silence behavior without a real Slack/webhook endpoint.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type record struct {
	ReceivedAt     string          `json:"received_at"`
	Path           string          `json:"path"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	ContentType    string          `json:"content_type,omitempty"`
	Body           json.RawMessage `json:"body"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "webhooksink: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("webhooksink", flag.ContinueOnError)
	addr := flags.String("addr", "127.0.0.1:19090", "listen address")
	out := flags.String("out", "webhooks.jsonl", "JSONL output file")
	if err := flags.Parse(args); err != nil {
		return err
	}

	file, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer func() { _ = file.Close() }()

	var mu sync.Mutex
	write := func(entry record) error {
		mu.Lock()
		defer mu.Unlock()
		encoder := json.NewEncoder(file)
		return encoder.Encode(entry)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	mux.Handle("/", newHandler(write))

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "webhooksink listening on %s, writing %s\n", *addr, *out)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

// newHandler records every POST as a JSON line. GET is a liveness probe.
func newHandler(write func(record) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "ok")
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		entry := record{
			ReceivedAt:     time.Now().UTC().Format(time.RFC3339Nano),
			Path:           r.URL.Path,
			IdempotencyKey: r.Header.Get("Idempotency-Key"),
			ContentType:    r.Header.Get("Content-Type"),
			Body:           json.RawMessage(body),
		}
		if !json.Valid(body) {
			encoded, _ := json.Marshal(string(body))
			entry.Body = encoded
		}
		if err := write(entry); err != nil {
			http.Error(w, "write record", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
