package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/observaki/observaki/bench/internal/promrw"
)

func testProxy(t *testing.T, limits limitFlags) (*proxy, *httptest.Server, *atomic.Int64) {
	t.Helper()
	var forwarded atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	proxy := newProxy(options{
		upstream:     upstream.URL,
		limits:       limits,
		defaultLimit: 100,
		maxBody:      1 << 20,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)
	return proxy, server, &forwarded
}

func writeBody(t *testing.T, tenant string, start, series int) []byte {
	t.Helper()
	req := &promrw.WriteRequest{}
	for i := start; i < start+series; i++ {
		labels := []promrw.Label{
			{Name: "__name__", Value: "test_metric"},
			{Name: "tenant_id", Value: tenant},
			{Name: "series_id", Value: strings.Repeat("x", i+1)},
		}
		promrw.SortLabels(labels)
		req.Timeseries = append(req.Timeseries, promrw.TimeSeries{
			Labels:  labels,
			Samples: []promrw.Sample{{Value: float64(i), Timestamp: 1_700_000_000_000}},
		})
	}
	body, err := promrw.EncodeCompressed(req)
	if err != nil {
		t.Fatalf("EncodeCompressed: %v", err)
	}
	return body
}

func send(t *testing.T, url, tenant string, start, series int) *http.Response {
	t.Helper()
	body := writeBody(t, tenant, start, series)
	response, err := http.Post(url+"/api/v1/write", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return response
}

func TestAdmissionLimitsArePerTenant(t *testing.T) {
	_, server, forwarded := testProxy(t, limitFlags{"tenant-a": 10, "tenant-b": 20})

	response := send(t, server.URL, "tenant-a", 0, 10)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("tenant-a initial status = %d, want 204", response.StatusCode)
	}
	_ = response.Body.Close()
	response = send(t, server.URL, "tenant-a", 10, 1)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("tenant-a over-limit status = %d, want 429", response.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	_ = response.Body.Close()
	if body["code"] != "cardinality_limit_exceeded" || body["tenant_id"] != "tenant-a" {
		t.Fatalf("rejection body = %+v", body)
	}

	response = send(t, server.URL, "tenant-b", 0, 20)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("tenant-b status = %d, want 204", response.StatusCode)
	}
	_ = response.Body.Close()
	response = send(t, server.URL, "tenant-b", 20, 1)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("tenant-b over-limit status = %d, want 429", response.StatusCode)
	}
	_ = response.Body.Close()
	if got := forwarded.Load(); got != 2 {
		t.Fatalf("forwarded requests = %d, want 2 (rejected requests must not reach VictoriaMetrics)", got)
	}

	stats := fetchStats(t, server.URL)
	tenantA := stats["tenant-a"]
	tenantB := stats["tenant-b"]
	if tenantA.ActiveSeries != 10 || tenantA.RejectedRequests != 1 {
		t.Fatalf("tenant-a stats = %+v", tenantA)
	}
	if tenantB.ActiveSeries != 20 || tenantB.RejectedRequests != 1 {
		t.Fatalf("tenant-b stats = %+v", tenantB)
	}
	if tenantB.RejectedSamples != 1 {
		t.Fatalf("tenant-b rejected samples = %d, want 1 (only the explicit over-limit request)", tenantB.RejectedSamples)
	}
}

type statsTenant struct {
	Limit            int   `json:"limit"`
	ActiveSeries     int   `json:"active_series"`
	AcceptedRequests int64 `json:"accepted_requests"`
	RejectedRequests int64 `json:"rejected_requests"`
	RejectedSeries   int64 `json:"rejected_series"`
	RejectedSamples  int64 `json:"rejected_samples"`
}

func fetchStats(t *testing.T, url string) map[string]statsTenant {
	t.Helper()
	response, err := http.Get(url + "/stats")
	if err != nil {
		t.Fatalf("GET /stats: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	var payload struct {
		Tenants map[string]statsTenant `json:"tenants"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	return payload.Tenants
}

func TestAdmissionMetrics(t *testing.T) {
	_, server, _ := testProxy(t, limitFlags{"tenant-a": 2})
	response := send(t, server.URL, "tenant-a", 0, 2)
	_ = response.Body.Close()
	response = send(t, server.URL, "tenant-a", 10, 1)
	_ = response.Body.Close()
	metrics, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = metrics.Body.Close() }()
	body, err := io.ReadAll(metrics.Body)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	for _, want := range []string{
		`observaki_cardinality_active_series{tenant_id="tenant-a"} 2`,
		`observaki_cardinality_limit{tenant_id="tenant-a"} 2`,
		`observaki_cardinality_rejected_requests_total{tenant_id="tenant-a"} 1`,
		`observaki_cardinality_rejected_series_total{tenant_id="tenant-a"} 1`,
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
}

func TestAdmissionRejectsInvalidPayload(t *testing.T) {
	_, server, forwarded := testProxy(t, limitFlags{})
	response, err := http.Post(server.URL+"/api/v1/write", "application/x-protobuf", strings.NewReader("not snappy"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid payload status = %d, want 400", response.StatusCode)
	}
	if forwarded.Load() != 0 {
		t.Fatalf("invalid payload was forwarded")
	}
}

func TestAdmissionRequiresTenant(t *testing.T) {
	_, server, forwarded := testProxy(t, limitFlags{})
	req := &promrw.WriteRequest{Timeseries: []promrw.TimeSeries{{
		Labels:  []promrw.Label{{Name: "__name__", Value: "test_metric"}},
		Samples: []promrw.Sample{{Value: 1, Timestamp: 1}},
	}}}
	body, err := promrw.EncodeCompressed(req)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+"/api/v1/write", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing tenant status = %d, want 400", response.StatusCode)
	}
	if forwarded.Load() != 0 {
		t.Fatalf("missing-tenant request was forwarded")
	}
}
