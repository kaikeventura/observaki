package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/observaki/observaki/internal/api"
	"github.com/observaki/observaki/internal/selfobs"
)

type staticOverview struct {
	overview selfobs.Overview
}

func (s staticOverview) Overview(context.Context) selfobs.Overview { return s.overview }

func TestHealthOverview(t *testing.T) {
	provider := staticOverview{overview: selfobs.Overview{
		GeneratedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		Status:      selfobs.StatusDegraded,
		Summary:     []string{"ingestion lag p95 is above 10s"},
		Ingestion: selfobs.Ingestion{
			Status:     selfobs.StatusDegraded,
			Source:     "writer /stats",
			Detail:     "ingestion lag p95 is above the 10s threshold",
			ObservedAt: timePtr(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)),
		},
		Query: selfobs.Query{Status: selfobs.StatusHealthy, Requests: 4, Since: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)},
	}}
	srv := New(":0", "test", slog.New(slog.DiscardHandler), WithHealthOverview(provider))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health/overview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var overview selfobs.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if overview.Status != selfobs.StatusDegraded || overview.Ingestion.Status != selfobs.StatusDegraded {
		t.Fatalf("overview = %+v", overview)
	}
}

func TestHealthOverviewWithoutProvider(t *testing.T) {
	srv := New(":0", "test", slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health/overview", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestQueryRoutesCountStructuredErrors(t *testing.T) {
	metrics := NewRequestMetrics()
	srv := New(":0", "test", slog.New(slog.DiscardHandler), WithRequestMetrics(metrics))

	// Without a token both query routes fail closed with the AD-15 envelope.
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/query"},
		{http.MethodGet, "/api/v1/query/sample"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401", tc.method, tc.path, rec.Code)
		}
		var body api.Error
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Code != api.CodeUnauthorized || body.RequestID == "" || strings.Contains(body.Message, "goroutine") {
			t.Fatalf("body = %+v", body)
		}
	}

	stats := metrics.QueryStats()
	if stats.Requests != 2 || stats.ClientErrors != 2 || stats.Errors != 0 {
		t.Fatalf("query stats = %+v", stats)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`observaki_api_requests_total{class="query",code="401"} 2`,
		"observaki_api_query_requests_total 2",
		"observaki_api_query_errors_total 0",
		"observaki_api_query_client_errors_total 2",
		"observaki_api_up 1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("/metrics missing %q:\n%s", want, body)
		}
	}
}

func TestClassifyPath(t *testing.T) {
	tests := map[string]string{
		"/healthz":                classHealth,
		"/api/v1/health":          classHealth,
		"/api/v1/health/overview": classHealth,
		"/api/v1/query":           classQuery,
		"/api/v1/query/sample":    classQuery,
		"/api/v1/projects":        classMetadata,
		"/metrics":                classInternal,
		"/openapi.json":           classInternal,
		"/explore":                classUI,
	}
	for path, want := range tests {
		if got := classifyPath(path); got != want {
			t.Fatalf("classifyPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func timePtr(value time.Time) *time.Time { return &value }
