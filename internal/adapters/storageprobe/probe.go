// Package storageprobe reports ClickHouse and VictoriaMetrics storage usage and
// reachability for the self-observability health page (AD-10, AD-24).
//
// The probe runs inside the writer (data plane), so the API never opens a
// telemetry connection to ClickHouse or VictoriaMetrics (AD-3). Results are
// cached to keep the health page cheap under its refresh interval.
package storageprobe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultCacheTTL bounds how often the probe re-reads storage.
const DefaultCacheTTL = 5 * time.Second

// Config configures the probe.
type Config struct {
	ClickHouseURL      string
	ClickHouseDatabase string
	ClickHouseUser     string
	ClickHousePassword string
	VictoriaMetricsURL string
	Timeout            time.Duration
	CacheTTL           time.Duration
}

// EndpointHealth is the health and usage of one storage backend.
type EndpointHealth struct {
	OK        bool    `json:"ok"`
	Bytes     *int64  `json:"bytes,omitempty"`
	Parts     *int64  `json:"parts,omitempty"`
	Series    *int64  `json:"series,omitempty"`
	LatencyMS float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// Stats is the JSON document served by the writer's /storage endpoint.
type Stats struct {
	ObservedAt      time.Time      `json:"observed_at"`
	ClickHouse      EndpointHealth `json:"clickhouse"`
	VictoriaMetrics EndpointHealth `json:"victoriametrics"`
}

// Probe caches the storage snapshot.
type Probe struct {
	cfg    Config
	client *http.Client

	mu       sync.Mutex
	cached   Stats
	cachedAt time.Time
}

// New builds a probe. An empty URL disables that backend (reported as not
// configured instead of an error).
func New(cfg Config) *Probe {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = DefaultCacheTTL
	}
	return &Probe{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

// Snapshot returns a cached storage snapshot, refreshing it when stale.
func (p *Probe) Snapshot(ctx context.Context) Stats {
	p.mu.Lock()
	if !p.cachedAt.IsZero() && time.Since(p.cachedAt) < p.cfg.CacheTTL {
		cached := p.cached
		p.mu.Unlock()
		return cached
	}
	p.mu.Unlock()

	stats := Stats{ObservedAt: time.Now().UTC()}
	stats.ClickHouse = p.clickHouse(ctx)
	stats.VictoriaMetrics = p.victoriaMetrics(ctx)

	p.mu.Lock()
	p.cached = stats
	p.cachedAt = time.Now()
	p.mu.Unlock()
	return stats
}

func (p *Probe) clickHouse(ctx context.Context) EndpointHealth {
	started := time.Now()
	health := EndpointHealth{}
	if p.cfg.ClickHouseURL == "" {
		health.Error = "clickhouse is not configured"
		return health
	}
	health.OK = false
	query := "SELECT sum(bytes_on_disk), count() FROM system.parts WHERE database = currentDatabase() AND active FORMAT TabSeparated"
	endpoint := strings.TrimRight(p.cfg.ClickHouseURL, "/") + "/?" + url.Values{
		"database": {p.cfg.ClickHouseDatabase},
		"query":    {query},
	}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		health.Error = err.Error()
		health.LatencyMS = millisecondsSince(started)
		return health
	}
	if p.cfg.ClickHouseUser != "" {
		request.SetBasicAuth(p.cfg.ClickHouseUser, p.cfg.ClickHousePassword)
	}
	response, err := p.client.Do(request)
	if err != nil {
		health.Error = err.Error()
		health.LatencyMS = millisecondsSince(started)
		return health
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	health.LatencyMS = millisecondsSince(started)
	if response.StatusCode != http.StatusOK {
		health.Error = fmt.Sprintf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		return health
	}
	fields := strings.Split(strings.TrimSpace(string(body)), "\t")
	if len(fields) < 2 {
		health.Error = "unexpected ClickHouse response"
		return health
	}
	bytes, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		health.Error = fmt.Sprintf("parse bytes: %v", err)
		return health
	}
	parts, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		health.Error = fmt.Sprintf("parse parts: %v", err)
		return health
	}
	health.OK = true
	health.Bytes = &bytes
	health.Parts = &parts
	return health
}

type tsdbResponse struct {
	Data struct {
		SeriesCountByMetricName []struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		} `json:"seriesCountByMetricName"`
	} `json:"data"`
}

func (p *Probe) victoriaMetrics(ctx context.Context) EndpointHealth {
	started := time.Now()
	health := EndpointHealth{}
	if p.cfg.VictoriaMetricsURL == "" {
		health.Error = "victoriametrics is not configured"
		return health
	}
	base := strings.TrimRight(p.cfg.VictoriaMetricsURL, "/")

	var series int64
	tsdbRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/status/tsdb", nil)
	if err != nil {
		health.Error = err.Error()
		health.LatencyMS = millisecondsSince(started)
		return health
	}
	tsdbResponseHTTP, err := p.client.Do(tsdbRequest)
	if err != nil {
		health.Error = err.Error()
		health.LatencyMS = millisecondsSince(started)
		return health
	}
	tsdbBody, _ := io.ReadAll(io.LimitReader(tsdbResponseHTTP.Body, 8<<20))
	_ = tsdbResponseHTTP.Body.Close()
	if tsdbResponseHTTP.StatusCode != http.StatusOK {
		health.Error = fmt.Sprintf("tsdb HTTP %d", tsdbResponseHTTP.StatusCode)
		health.LatencyMS = millisecondsSince(started)
		return health
	}
	var tsdb tsdbResponse
	if err := json.Unmarshal(tsdbBody, &tsdb); err == nil {
		for _, row := range tsdb.Data.SeriesCountByMetricName {
			series += row.Value
		}
		health.Series = &series
	}

	// vm_data_size_bytes is the per-storage-component size gauge. It is
	// optional: an older/newer VictoriaMetrics build without it still reports
	// health and the series count.
	var bytes int64
	var found bool
	metricsRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/metrics", nil)
	if err == nil {
		if metricsResponse, err := p.client.Do(metricsRequest); err == nil {
			bytes, found = parseVMDataSize(metricsResponse.Body)
			_ = metricsResponse.Body.Close()
		}
	}
	if found {
		health.Bytes = &bytes
	}
	health.OK = true
	health.LatencyMS = millisecondsSince(started)
	return health
}

// parseVMDataSize sums the vm_data_size_bytes gauge, tolerating labels and
// blank lines.
func parseVMDataSize(r io.Reader) (int64, bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	var total int64
	var found bool
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "vm_data_size_bytes") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		total += int64(value)
		found = true
	}
	return total, found
}

// Handler serves the cached snapshot as JSON on the writer's HTTP server.
func (p *Probe) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p.Snapshot(r.Context()))
	})
}

func millisecondsSince(started time.Time) float64 {
	return float64(time.Since(started)) / float64(time.Millisecond)
}
