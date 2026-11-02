package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"service":"svc-000"},"values":[[1700000000,"1.5"],[1700000300,"2.5"]]}]}}`)
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "queries.json")
	code := runQueries([]string{
		"--vm-url", server.URL,
		"--catalog", filepath.Join("..", "..", "victoriametrics", "queries", "catalog.tsv"),
		"--runs", "1",
		"--warmup", "0",
		"--out", out,
	}, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("runQueries exit = %d, want 0", code)
	}
	payload, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report struct {
		Status  string `json:"status"`
		Queries []struct {
			ID           string `json:"id"`
			ResultSeries int    `json:"result_series"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("report status = %q", report.Status)
	}
	if len(report.Queries) != 20 {
		t.Fatalf("queries = %d, want 20", len(report.Queries))
	}
	if report.Queries[0].ResultSeries != 1 {
		t.Fatalf("result series = %d, want 1", report.Queries[0].ResultSeries)
	}
}

func TestRunVerify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/write":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/query":
			query := r.URL.Query().Get("query")
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.Contains(query, `sum by (service)`):
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"service":"alpha"},"value":[1700000000,"0.6666666667"]},{"metric":{"service":"beta"},"value":[1700000000,"0.3333333333"]}]}}`)
			case strings.Contains(query, `sum by (status)`):
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"status":"200"},"value":[1700000000,"200"]},{"metric":{"status":"500"},"value":[1700000000,"100"]}]}}`)
			case strings.Contains(query, `histogram_quantile(0.95`):
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"0.475"]}]}}`)
			case strings.Contains(query, `histogram_quantile(0.50`):
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"0.25"]}]}}`)
			case strings.Contains(query, `sum(increase`):
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"200"]}]}}`)
			case strings.Contains(query, `sum(observaki_verify_requests_total)`):
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"300"]}]}}`)
			case strings.Contains(query, `sum(rate`):
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"0.6666666667"]}]}}`)
			default:
				_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "verify.json")
	code := runVerify([]string{"--vm-url", server.URL, "--out", out}, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("runVerify exit = %d, want 0", code)
	}
	payload, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read verification: %v", err)
	}
	var report struct {
		Status string `json:"status"`
		Checks []struct {
			Name string `json:"name"`
			Pass bool   `json:"pass"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode verification: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("verification status = %q: %+v", report.Status, report)
	}
	if len(report.Checks) != 7 {
		t.Fatalf("checks = %d, want 7", len(report.Checks))
	}
}
