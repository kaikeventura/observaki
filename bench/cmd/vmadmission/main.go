// Command vmadmission is a small per-tenant series admission proxy in front of
// VictoriaMetrics. It is the spike implementation of AD-33: the gateway (Story
// 4.1/4.2) owns this decision in production, but the benchmark needs a real
// enforcement point to test the noisy-neighbor guardrail.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/observaki/observaki/bench/internal/benchversion"
	"github.com/observaki/observaki/bench/internal/promrw"
	"github.com/observaki/observaki/bench/internal/vmbench"
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

type limitFlags map[string]int

func (l *limitFlags) String() string {
	parts := make([]string, 0, len(*l))
	for tenant, limit := range *l {
		parts = append(parts, fmt.Sprintf("%s=%d", tenant, limit))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (l *limitFlags) Set(value string) error {
	tenant, limit, err := vmbench.ParseLimit(value)
	if err != nil {
		return err
	}
	if *l == nil {
		*l = map[string]int{}
	}
	(*l)[tenant] = limit
	return nil
}

type options struct {
	listen       string
	upstream     string
	limits       limitFlags
	defaultLimit int
	maxBody      int64
	version      bool
}

func run(args []string, stdout, stderr io.Writer) int {
	opts := options{}
	flags := flag.NewFlagSet("vmadmission", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.listen, "listen", "127.0.0.1:18429", "listen address")
	flags.StringVar(&opts.upstream, "upstream", "http://127.0.0.1:18428", "VictoriaMetrics base URL")
	flags.Var(&opts.limits, "limit", "per-tenant series limit (tenant=N); repeatable")
	flags.IntVar(&opts.defaultLimit, "default-limit", 1_000_000, "default series limit")
	flags.Int64Var(&opts.maxBody, "max-body-bytes", 32<<20, "maximum request body")
	flags.BoolVar(&opts.version, "version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if opts.version {
		_, _ = fmt.Fprintf(stdout, "vmadmission %s (%s)\n", benchversion.Version, benchversion.Commit)
		return 0
	}
	if opts.defaultLimit < 0 {
		_, _ = fmt.Fprintln(stderr, "vmadmission: default-limit must be non-negative")
		return 2
	}

	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	proxy := newProxy(opts, logger)
	server := &http.Server{
		Addr:              opts.listen,
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("starting admission proxy", "listen", opts.listen, "upstream", opts.upstream, "limits", opts.limits.String(), "default_limit", opts.defaultLimit)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		_, _ = fmt.Fprintf(stderr, "vmadmission: %v\n", err)
		return 1
	}
	return 0
}

type tenantState struct {
	limit int
	mu    sync.Mutex

	series           map[uint64]struct{}
	rejectedUnique   map[uint64]struct{}
	acceptedRequests int64
	rejectedRequests int64
	acceptedSeries   int64
	rejectedSeries   int64
	acceptedSamples  int64
	rejectedSamples  int64
}

type proxy struct {
	opts   options
	logger *slog.Logger
	client *http.Client

	mu      sync.Mutex
	tenants map[string]*tenantState
}

func newProxy(opts options, logger *slog.Logger) *proxy {
	return &proxy{
		opts:   opts,
		logger: logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 64,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		tenants: map[string]*tenantState{},
	}
}

func (p *proxy) tenant(name string) *tenantState {
	p.mu.Lock()
	defer p.mu.Unlock()
	state, ok := p.tenants[name]
	if !ok {
		limit := p.opts.defaultLimit
		if explicit, ok := p.opts.limits[name]; ok {
			limit = explicit
		}
		state = &tenantState{limit: limit, series: map[uint64]struct{}{}, rejectedUnique: map[uint64]struct{}{}}
		p.tenants[name] = state
	}
	return state
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	case "/metrics":
		p.writeMetrics(w)
	case "/stats":
		p.writeStats(w)
	case "/api/v1/write", "/prometheus/api/v1/write":
		p.handleWrite(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *proxy) handleWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, p.opts.maxBody+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "body_read_failed", err.Error())
		return
	}
	if int64(len(body)) > p.opts.maxBody {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds the limit")
		return
	}
	req, err := promrw.DecodeCompressed(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_remote_write", err.Error())
		return
	}
	if len(req.Timeseries) == 0 {
		writeError(w, http.StatusBadRequest, "empty_remote_write", "the request has no time series")
		return
	}

	type group struct {
		state   *tenantState
		name    string
		keys    map[uint64]struct{}
		samples int
	}
	groups := map[string]*group{}
	for _, series := range req.Timeseries {
		tenant := ""
		for _, label := range series.Labels {
			if label.Name == "tenant_id" {
				tenant = label.Value
				break
			}
		}
		if tenant == "" {
			writeError(w, http.StatusBadRequest, "missing_tenant", "every series must carry tenant_id")
			return
		}
		item, ok := groups[tenant]
		if !ok {
			item = &group{state: p.tenant(tenant), name: tenant, keys: map[uint64]struct{}{}}
			groups[tenant] = item
		}
		item.keys[hashLabels(series.Labels)] = struct{}{}
		item.samples++
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		groups[name].state.mu.Lock()
	}
	unlock := func() {
		for _, name := range names {
			groups[name].state.mu.Unlock()
		}
	}

	for _, name := range names {
		item := groups[name]
		newSeries := 0
		for key := range item.keys {
			if _, ok := item.state.series[key]; !ok {
				newSeries++
			}
		}
		if len(item.state.series)+newSeries > item.state.limit {
			active := len(item.state.series)
			item.state.rejectedRequests++
			item.state.rejectedSeries += int64(newSeries)
			item.state.rejectedSamples += int64(item.samples)
			for key := range item.keys {
				if _, ok := item.state.series[key]; !ok {
					item.state.rejectedUnique[key] = struct{}{}
				}
			}
			unlock()
			p.logger.Warn("cardinality limit exceeded",
				"tenant_id", name,
				"limit", item.state.limit,
				"active_series", active,
				"requested_new_series", newSeries)
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"code":                 "cardinality_limit_exceeded",
				"message":              "tenant exceeded its active series limit",
				"tenant_id":            name,
				"limit":                item.state.limit,
				"active_series":        active,
				"requested_new_series": newSeries,
			})
			return
		}
	}
	for _, name := range names {
		item := groups[name]
		for key := range item.keys {
			item.state.series[key] = struct{}{}
		}
		item.state.acceptedRequests++
		item.state.acceptedSeries += int64(len(item.keys))
		item.state.acceptedSamples += int64(item.samples)
	}
	unlock()

	upstream := strings.TrimRight(p.opts.upstream, "/") + r.URL.Path
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstream, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_request_failed", err.Error())
		return
	}
	request.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	request.Header.Set("Content-Encoding", r.Header.Get("Content-Encoding"))
	request.Header.Set("X-Prometheus-Remote-Write-Version", r.Header.Get("X-Prometheus-Remote-Write-Version"))
	response, err := p.client.Do(request)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_unavailable", err.Error())
		return
	}
	defer func() { _ = response.Body.Close() }()
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, 1<<20))
}

func hashLabels(labels []promrw.Label) uint64 {
	sorted := append([]promrw.Label(nil), labels...)
	promrw.SortLabels(sorted)
	hash := fnv.New64a()
	for _, label := range sorted {
		_, _ = hash.Write([]byte(label.Name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(label.Value))
		_, _ = hash.Write([]byte{0xff})
	}
	return hash.Sum64()
}

func (p *proxy) writeStats(w http.ResponseWriter) {
	p.mu.Lock()
	names := make([]string, 0, len(p.tenants))
	for name := range p.tenants {
		names = append(names, name)
	}
	sort.Strings(names)
	states := make(map[string]*tenantState, len(names))
	for _, name := range names {
		states[name] = p.tenants[name]
	}
	p.mu.Unlock()

	type tenantStats struct {
		Limit            int   `json:"limit"`
		ActiveSeries     int   `json:"active_series"`
		AcceptedRequests int64 `json:"accepted_requests"`
		RejectedRequests int64 `json:"rejected_requests"`
		AcceptedSeries   int64 `json:"accepted_series_observations"`
		RejectedSeries   int64 `json:"rejected_series_observations"`
		RejectedUnique   int   `json:"rejected_unique_series"`
		AcceptedSamples  int64 `json:"accepted_samples"`
		RejectedSamples  int64 `json:"rejected_samples"`
	}
	stats := map[string]tenantStats{}
	for _, name := range names {
		state := states[name]
		state.mu.Lock()
		stats[name] = tenantStats{
			Limit:            state.limit,
			ActiveSeries:     len(state.series),
			AcceptedRequests: state.acceptedRequests,
			RejectedRequests: state.rejectedRequests,
			AcceptedSeries:   state.acceptedSeries,
			RejectedSeries:   state.rejectedSeries,
			RejectedUnique:   len(state.rejectedUnique),
			AcceptedSamples:  state.acceptedSamples,
			RejectedSamples:  state.rejectedSamples,
		}
		state.mu.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": stats})
}

func (p *proxy) writeMetrics(w http.ResponseWriter) {
	p.mu.Lock()
	names := make([]string, 0, len(p.tenants))
	for name := range p.tenants {
		names = append(names, name)
	}
	sort.Strings(names)
	states := make(map[string]*tenantState, len(names))
	for _, name := range names {
		states[name] = p.tenants[name]
	}
	p.mu.Unlock()

	var builder strings.Builder
	builder.WriteString("# HELP observaki_cardinality_active_series Active series admitted per tenant.\n")
	builder.WriteString("# TYPE observaki_cardinality_active_series gauge\n")
	builder.WriteString("# HELP observaki_cardinality_limit Series limit per tenant.\n")
	builder.WriteString("# TYPE observaki_cardinality_limit gauge\n")
	builder.WriteString("# HELP observaki_cardinality_accepted_requests_total Remote write requests admitted.\n")
	builder.WriteString("# TYPE observaki_cardinality_accepted_requests_total counter\n")
	builder.WriteString("# HELP observaki_cardinality_rejected_requests_total Remote write requests rejected by the series limit.\n")
	builder.WriteString("# TYPE observaki_cardinality_rejected_requests_total counter\n")
	builder.WriteString("# HELP observaki_cardinality_rejected_series_total Series observations rejected by the series limit.\n")
	builder.WriteString("# TYPE observaki_cardinality_rejected_series_total counter\n")
	builder.WriteString("# HELP observaki_cardinality_rejected_unique_series Unique series rejected by the series limit.\n")
	builder.WriteString("# TYPE observaki_cardinality_rejected_unique_series gauge\n")
	for _, name := range names {
		state := states[name]
		state.mu.Lock()
		active := len(state.series)
		limit := state.limit
		accepted := state.acceptedRequests
		rejected := state.rejectedRequests
		rejectedSeries := state.rejectedSeries
		rejectedUnique := len(state.rejectedUnique)
		state.mu.Unlock()
		_, _ = fmt.Fprintf(&builder, "observaki_cardinality_active_series{tenant_id=%q} %d\n", name, active)
		_, _ = fmt.Fprintf(&builder, "observaki_cardinality_limit{tenant_id=%q} %d\n", name, limit)
		_, _ = fmt.Fprintf(&builder, "observaki_cardinality_accepted_requests_total{tenant_id=%q} %d\n", name, accepted)
		_, _ = fmt.Fprintf(&builder, "observaki_cardinality_rejected_requests_total{tenant_id=%q} %d\n", name, rejected)
		_, _ = fmt.Fprintf(&builder, "observaki_cardinality_rejected_series_total{tenant_id=%q} %d\n", name, rejectedSeries)
		_, _ = fmt.Fprintf(&builder, "observaki_cardinality_rejected_unique_series{tenant_id=%q} %d\n", name, rejectedUnique)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = io.WriteString(w, builder.String())
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
