package victoriametrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/observaki/observaki/internal/promrw"
)

func testSeries(name string, value float64) promrw.TimeSeries {
	return promrw.TimeSeries{
		Labels: []promrw.Label{
			{Name: "__name__", Value: name},
			{Name: "tenant_id", Value: "tenant-a"},
		},
		Samples: []promrw.Sample{{Value: value, Timestamp: 1_700_000_000_000}},
	}
}

func TestWriteSendsRemoteWritePayload(t *testing.T) {
	var (
		contentType  string
		encoding     string
		decoded      *promrw.WriteRequest
		requestCount atomic.Int64
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/api/v1/write" {
			t.Errorf("path = %q, want /api/v1/write", r.URL.Path)
		}
		contentType = r.Header.Get("Content-Type")
		encoding = r.Header.Get("Content-Encoding")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		decoded, err = promrw.DecodeCompressed(body)
		if err != nil {
			t.Errorf("decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := Open(Config{URL: server.URL})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()
	result, err := client.Write(context.Background(), []promrw.TimeSeries{
		testSeries("http_requests_total", 1),
		testSeries("go_goroutines", 2),
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if result.Series != 2 || result.Samples != 2 || result.Requests != 1 {
		t.Fatalf("result = %+v", result)
	}
	if contentType != "application/x-protobuf" || encoding != "snappy" {
		t.Fatalf("headers = %q/%q", contentType, encoding)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requestCount.Load())
	}
	if decoded == nil || len(decoded.Timeseries) != 2 {
		t.Fatalf("decoded = %+v", decoded)
	}
	if decoded.Timeseries[0].Labels[0].Name != "__name__" || decoded.Timeseries[0].Samples[0].Value != 1 {
		t.Fatalf("decoded series = %+v", decoded.Timeseries[0])
	}
}

func TestWriteRetriesTransientErrors(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "try again")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := Open(Config{
		URL:             server.URL,
		MaxRetries:      5,
		RetryBackoff:    time.Millisecond,
		MaxRetryBackoff: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()
	result, err := client.Write(context.Background(), []promrw.TimeSeries{testSeries("m", 1)})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
	if result.Retries != 2 {
		t.Fatalf("retries = %d, want 2", result.Retries)
	}
}

func TestWriteDoesNotRetryPermanentErrors(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "invalid series")
	}))
	defer server.Close()

	client, err := Open(Config{
		URL:             server.URL,
		MaxRetries:      5,
		RetryBackoff:    time.Millisecond,
		MaxRetryBackoff: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()
	_, err = client.Write(context.Background(), []promrw.TimeSeries{testSeries("m", 1)})
	if err == nil {
		t.Fatal("Write() error = nil, want permanent rejection")
	}
	if !IsPermanent(err) {
		t.Fatalf("IsPermanent(%v) = false", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestWriteHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := Open(Config{
		URL:             server.URL,
		RetryBackoff:    time.Hour,
		MaxRetryBackoff: time.Hour,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = client.Write(ctx, []promrw.TimeSeries{testSeries("m", 1)})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Write() error = %v, want context deadline", err)
	}
}

func TestWriteSplitsLargeBatches(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := Open(Config{URL: server.URL, MaxSeriesPerRequest: 1})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()
	result, err := client.Write(context.Background(), []promrw.TimeSeries{
		testSeries("a", 1), testSeries("b", 2), testSeries("c", 3),
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if requests.Load() != 3 || result.Requests != 3 {
		t.Fatalf("requests = %d/%d, want 3", requests.Load(), result.Requests)
	}
}

func TestOpenRejectsInvalidConfig(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{URL: "ftp://example.com"},
		{URL: "http://example.com", MaxRetries: -1},
	} {
		if _, err := Open(cfg); err == nil {
			t.Fatalf("Open(%+v) error = nil", cfg)
		}
	}
}

func TestWriteStopsAfterMaxRetries(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := Open(Config{
		URL:             server.URL,
		MaxRetries:      2,
		RetryBackoff:    time.Millisecond,
		MaxRetryBackoff: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()
	if _, err := client.Write(context.Background(), []promrw.TimeSeries{testSeries("m", 1)}); err == nil {
		t.Fatal("Write() error = nil, want retry exhaustion")
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3 (first + 2 retries)", attempts.Load())
	}
}
