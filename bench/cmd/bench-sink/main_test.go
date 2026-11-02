package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testSink(status int) *sink {
	return newSink(slog.New(slog.NewTextHandler(io.Discard, nil)), status)
}

func TestSinkAcceptsPost(t *testing.T) {
	handler := testSink(http.StatusOK)
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("otlp payload"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	snapshot := handler.snapshot()
	if snapshot.Requests != 1 {
		t.Errorf("requests = %d, want 1", snapshot.Requests)
	}
	if snapshot.Bytes != int64(len("otlp payload")) {
		t.Errorf("bytes = %d, want %d", snapshot.Bytes, len("otlp payload"))
	}
}

func TestSinkCustomStatus(t *testing.T) {
	handler := testSink(http.StatusServiceUnavailable)
	request := httptest.NewRequest(http.MethodPost, "/v1/metrics", strings.NewReader("x"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusServiceUnavailable; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestSinkHealth(t *testing.T) {
	handler := testSink(http.StatusOK)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := strings.TrimSpace(recorder.Body.String()), "ok"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if handler.snapshot().Requests != 0 {
		t.Fatal("health check must not count as an accepted request")
	}
}

func TestSinkRejectsUnsupportedMethod(t *testing.T) {
	handler := testSink(http.StatusOK)
	request := httptest.NewRequest(http.MethodGet, "/v1/logs", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusMethodNotAllowed; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if handler.snapshot().Rejected != 1 {
		t.Fatal("unsupported method must be counted as rejected")
	}
}
