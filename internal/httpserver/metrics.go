package httpserver

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/observaki/observaki/internal/selfobs"
)

// Request classes used by the self-observability health page.
const (
	classHealth   = "health"
	classQuery    = "query"
	classMetadata = "metadata"
	classUI       = "ui"
	classInternal = "internal"
)

// RequestMetrics counts API requests by class and status code. It is the
// source of the query error rate on the health page until the Query Service
// lands in Phase 1 (AD-10).
type RequestMetrics struct {
	mu     sync.Mutex
	since  time.Time
	counts map[string]map[int]uint64
}

func NewRequestMetrics() *RequestMetrics {
	return &RequestMetrics{
		since:  time.Now().UTC(),
		counts: make(map[string]map[int]uint64),
	}
}

func (m *RequestMetrics) observe(class string, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	codes := m.counts[class]
	if codes == nil {
		codes = make(map[int]uint64)
		m.counts[class] = codes
	}
	codes[code]++
}

// QueryStats implements selfobs.QuerySource.
func (m *RequestMetrics) QueryStats() selfobs.QueryStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := selfobs.QueryStats{Since: m.since}
	for code, count := range m.counts[classQuery] {
		stats.Requests += count
		switch {
		case code >= 500:
			stats.Errors += count
		case code >= 400:
			stats.ClientErrors += count
		}
	}
	return stats
}

func (m *RequestMetrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	var out strings.Builder
	out.WriteString("# HELP observaki_api_requests_total API requests by class and status code.\n")
	out.WriteString("# TYPE observaki_api_requests_total counter\n")
	m.mu.Lock()
	classes := make([]string, 0, len(m.counts))
	for class := range m.counts {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	for _, class := range classes {
		codes := make([]int, 0, len(m.counts[class]))
		for code := range m.counts[class] {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		for _, code := range codes {
			fmt.Fprintf(&out, "observaki_api_requests_total{class=%q,code=%q} %d\n", class, fmt.Sprint(code), m.counts[class][code])
		}
	}
	stats := selfobs.QueryStats{Since: m.since}
	for code, count := range m.counts[classQuery] {
		stats.Requests += count
		if code >= 500 {
			stats.Errors += count
		} else if code >= 400 {
			stats.ClientErrors += count
		}
	}
	uptime := time.Since(m.since).Seconds()
	m.mu.Unlock()

	out.WriteString("# HELP observaki_api_query_requests_total Requests handled by the API query class.\n")
	out.WriteString("# TYPE observaki_api_query_requests_total counter\n")
	fmt.Fprintf(&out, "observaki_api_query_requests_total %d\n", stats.Requests)
	out.WriteString("# HELP observaki_api_query_errors_total Query-class requests that returned 5xx.\n")
	out.WriteString("# TYPE observaki_api_query_errors_total counter\n")
	fmt.Fprintf(&out, "observaki_api_query_errors_total %d\n", stats.Errors)
	out.WriteString("# HELP observaki_api_query_client_errors_total Query-class requests that returned 4xx.\n")
	out.WriteString("# TYPE observaki_api_query_client_errors_total counter\n")
	fmt.Fprintf(&out, "observaki_api_query_client_errors_total %d\n", stats.ClientErrors)
	out.WriteString("# HELP observaki_api_up Whether the API process is running.\n")
	out.WriteString("# TYPE observaki_api_up gauge\n")
	out.WriteString("observaki_api_up 1\n")
	out.WriteString("# HELP observaki_api_uptime_seconds Seconds since the API process started.\n")
	out.WriteString("# TYPE observaki_api_uptime_seconds gauge\n")
	fmt.Fprintf(&out, "observaki_api_uptime_seconds %.3f\n", uptime)
	_, _ = io.WriteString(w, out.String())
}

// middleware records the request class and status code.
func (m *RequestMetrics) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		m.observe(classifyPath(r.URL.Path), recorder.status)
	})
}

func classifyPath(path string) string {
	switch {
	case path == "/healthz" || path == "/api/v1/health" || strings.HasPrefix(path, "/api/v1/health/"):
		return classHealth
	case path == "/metrics":
		return classInternal
	case path == "/api/v1/query" || strings.HasPrefix(path, "/api/v1/query/"):
		return classQuery
	case strings.HasPrefix(path, "/api/"):
		return classMetadata
	default:
		return classUI
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// Unwrap lets http.ResponseController reach Flusher/Hijacker on the wrapped
// writer (SSE and streaming use it from Phase 1).
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}
