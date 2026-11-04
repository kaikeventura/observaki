package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeAcceptsHealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	if err := Probe(context.Background(), nil, server.URL); err != nil {
		t.Fatalf("Probe() error = %v, want nil", err)
	}
}

func TestProbeRejectsUnhealthyStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "service unavailable", status: http.StatusServiceUnavailable},
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "not found", status: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			}))
			defer server.Close()

			err := Probe(context.Background(), nil, server.URL)
			if err == nil {
				t.Fatalf("Probe() error = nil, want HTTP %d", test.status)
			}
			if want := fmt.Sprintf("HTTP %d", test.status); !strings.Contains(err.Error(), want) {
				t.Fatalf("Probe() error = %q, want it to contain %q", err, want)
			}
		})
	}
}

func TestProbeRejectsEmptyURL(t *testing.T) {
	if err := Probe(context.Background(), nil, ""); err == nil {
		t.Fatal("Probe() error = nil, want an error for a blank URL")
	}
}

func TestProbeRejectsUnreachableEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	if err := Probe(context.Background(), nil, url); err == nil {
		t.Fatal("Probe() error = nil, want a connection error")
	}
}

func TestProbeHonorsContextDeadline(t *testing.T) {
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(block)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := Probe(ctx, nil, server.URL)
	if err == nil {
		t.Fatal("Probe() error = nil, want a timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Probe() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestProbeRejectsMalformedURL(t *testing.T) {
	if err := Probe(context.Background(), nil, "://missing-scheme"); err == nil {
		t.Fatal("Probe() error = nil, want a request build error")
	}
}
