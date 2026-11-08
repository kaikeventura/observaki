package selfobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config configures the collector.
type Config struct {
	WriterURL          string
	GatewayURL         string
	NATSMonitorURL     string
	IngestLagThreshold time.Duration
	Refresh            time.Duration
	Timeout            time.Duration
}

// PostgresPinger checks the control-plane database.
type PostgresPinger interface {
	Ping(ctx context.Context) error
}

// Collector aggregates the data plane's self-observability endpoints.
type Collector struct {
	cfg      Config
	client   *http.Client
	query    QuerySource
	alerting AlertingSource
	postgres PostgresPinger
	now      func() time.Time

	mu          sync.Mutex
	cached      Overview
	cachedAt    time.Time
	lastWriter  *writerReport
	lastGateway *gatewayReport
	lastStorage *storageReport
}

// New builds a collector. query, alerting and postgres may be nil.
func New(cfg Config, query QuerySource, alerting AlertingSource, postgres PostgresPinger) *Collector {
	if cfg.Refresh <= 0 {
		cfg.Refresh = 5 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.IngestLagThreshold <= 0 {
		cfg.IngestLagThreshold = 10 * time.Second
	}
	return &Collector{
		cfg:      cfg,
		client:   &http.Client{Timeout: cfg.Timeout},
		query:    query,
		alerting: alerting,
		postgres: postgres,
		now:      time.Now,
	}
}

type writerReport struct {
	up         bool
	connected  bool
	lagP95     *float64
	pending    int64
	dlq        int64
	written    uint64
	lastError  string
	observedAt time.Time
}

type gatewayReport struct {
	connected  *bool
	published  *uint64
	pending    *int64
	observedAt time.Time
}

type storageEndpoint struct {
	OK        bool    `json:"ok"`
	Bytes     *int64  `json:"bytes"`
	Parts     *int64  `json:"parts"`
	Series    *int64  `json:"series"`
	LatencyMS float64 `json:"latency_ms"`
	Error     string  `json:"error"`
}

type storageReport struct {
	observedAt      time.Time
	clickHouse      storageEndpoint
	victoriaMetrics storageEndpoint
}

type writerStatsDocument struct {
	Up        bool   `json:"up"`
	Connected bool   `json:"connected"`
	LastError string `json:"last_error_at"`
	Tenants   []struct {
		TenantID string  `json:"tenant_id"`
		Signal   string  `json:"signal"`
		Written  uint64  `json:"written_events"`
		LagP95MS float64 `json:"lag_p95_ms"`
	} `json:"tenants"`
	Signals map[string]struct {
		DLQMessages     int64 `json:"dlq_messages"`
		PendingMessages int64 `json:"pending_messages"`
	} `json:"signals"`
}

type storageStatsDocument struct {
	ObservedAt      time.Time       `json:"observed_at"`
	ClickHouse      storageEndpoint `json:"clickhouse"`
	VictoriaMetrics storageEndpoint `json:"victoriametrics"`
}

// Overview returns the cached snapshot, refreshing it when stale.
func (c *Collector) Overview(ctx context.Context) Overview {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now().UTC()
	if !c.cachedAt.IsZero() && now.Sub(c.cachedAt) < c.cfg.Refresh {
		return c.cached
	}

	overview := Overview{GeneratedAt: now, Components: make([]Component, 0, 8)}
	components := make([]Component, 0, 8)

	components = append(components, Component{
		Name:       "api",
		Status:     StatusHealthy,
		ObservedAt: now,
	})

	components = append(components, c.probePostgres(ctx, now))
	components = append(components, c.probeHTTP(ctx, "gateway", c.cfg.GatewayURL, "/readyz", now))
	writerComponent, writer := c.collectWriter(ctx, now)
	components = append(components, writerComponent)
	components = append(components, c.probeHTTP(ctx, "nats", c.cfg.NATSMonitorURL, "/healthz", now))

	storage, storageFetch := c.collectStorage(ctx, now)
	components = append(components,
		c.storageComponent("clickhouse", storage, storageFetch, now),
		c.storageComponent("victoriametrics", storage, storageFetch, now),
	)
	overview.Alerting = c.collectAlerting(now)
	components = append(components, Component{
		Name:       "alerting",
		Status:     overview.Alerting.Status,
		LatencyMS:  0,
		Detail:     overview.Alerting.Detail,
		ObservedAt: now,
	})

	gateway := c.collectGateway(ctx, now)
	overview.Ingestion = buildIngestion(c.cfg, writer, gateway, now)
	overview.Storage = buildStorage(c.cfg, storage, storageFetch, now)
	overview.Query = buildQuery(c.query)
	overview.Components = components
	overview.Status, overview.Summary = summarize(overview, c.cfg.IngestLagThreshold)

	c.cached = overview
	c.cachedAt = now
	return overview
}

func (c *Collector) collectWriter(ctx context.Context, now time.Time) (Component, *writerReport) {
	component := c.probeHTTP(ctx, "writer", c.cfg.WriterURL, "/readyz", now)
	if c.cfg.WriterURL == "" {
		return component, nil
	}
	body, latency, err := c.get(ctx, c.cfg.WriterURL+"/stats")
	component.LatencyMS = latency
	if err != nil {
		if c.lastWriter != nil {
			copied := *c.lastWriter
			return component, &copied
		}
		return component, nil
	}
	var document writerStatsDocument
	if err := json.Unmarshal(body, &document); err != nil {
		component.Status = StatusDegraded
		component.Detail = "writer /stats is not valid JSON: " + err.Error()
		return component, c.lastWriter
	}
	report := &writerReport{
		up:         document.Up,
		connected:  document.Connected,
		pending:    0,
		lastError:  document.LastError,
		observedAt: now,
	}
	var maxLag float64
	var hasLag bool
	var written uint64
	for _, tenant := range document.Tenants {
		if tenant.LagP95MS > maxLag {
			maxLag = tenant.LagP95MS
			hasLag = true
		}
		written += tenant.Written
	}
	if hasLag {
		seconds := maxLag / 1000
		report.lagP95 = &seconds
	}
	report.written = written
	for _, signal := range document.Signals {
		report.pending += signal.PendingMessages
		report.dlq += signal.DLQMessages
	}
	c.lastWriter = report
	return component, report
}

func (c *Collector) collectGateway(ctx context.Context, now time.Time) *gatewayReport {
	if c.cfg.GatewayURL == "" {
		return nil
	}
	body, _, err := c.get(ctx, c.cfg.GatewayURL+"/metrics")
	if err != nil {
		return c.lastGateway
	}
	samples := parsePrometheus(string(body))
	report := &gatewayReport{observedAt: now}
	if value, ok := promFirst(samples, "observaki_gateway_wal_connected"); ok {
		connected := value == 1
		report.connected = &connected
	}
	if value, ok := promSum(samples, "observaki_gateway_wal_published_events_total"); ok {
		published := uint64(value)
		report.published = &published
	}
	if value, ok := promFirst(samples, "observaki_gateway_wal_pending_events"); ok {
		pending := int64(value)
		report.pending = &pending
	}
	c.lastGateway = report
	return report
}

func (c *Collector) collectStorage(ctx context.Context, now time.Time) (*storageReport, error) {
	if c.cfg.WriterURL == "" {
		return nil, nil
	}
	body, _, err := c.get(ctx, c.cfg.WriterURL+"/storage")
	if err != nil {
		if c.lastStorage != nil {
			return c.lastStorage, err
		}
		return nil, err
	}
	var document storageStatsDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return c.lastStorage, err
	}
	observed := document.ObservedAt
	if observed.IsZero() {
		observed = now
	}
	report := &storageReport{
		observedAt:      observed,
		clickHouse:      document.ClickHouse,
		victoriaMetrics: document.VictoriaMetrics,
	}
	c.lastStorage = report
	return report, nil
}

func (c *Collector) collectAlerting(now time.Time) Alerting {
	if c.alerting == nil {
		return Alerting{
			Status:  StatusDisabled,
			Enabled: false,
			Detail:  "internal alerting is disabled",
		}
	}
	alerting := c.alerting.AlertingSnapshot()
	if alerting.Enabled && alerting.LastEvaluationAt == nil {
		alerting.Status = StatusDegraded
		alerting.Detail = "waiting for the first evaluation"
	}
	if alerting.LastEvaluationAt != nil {
		age := now.Sub(*alerting.LastEvaluationAt).Seconds()
		if age < 0 {
			age = 0
		}
		alerting.EvaluationAgeSeconds = &age
	}
	return alerting
}

func (c *Collector) probePostgres(ctx context.Context, now time.Time) Component {
	component := Component{Name: "postgres", Status: StatusNotConfigured, ObservedAt: now}
	if c.postgres == nil {
		return component
	}
	pingContext, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	started := c.now()
	err := c.postgres.Ping(pingContext)
	component.LatencyMS = float64(c.now().Sub(started)) / float64(time.Millisecond)
	if err != nil {
		component.Status = StatusCritical
		component.Detail = err.Error()
		return component
	}
	component.Status = StatusHealthy
	return component
}

func (c *Collector) probeHTTP(ctx context.Context, name, baseURL, path string, now time.Time) Component {
	component := Component{Name: name, Status: StatusNotConfigured, ObservedAt: now}
	if baseURL == "" {
		return component
	}
	body, latency, err := c.get(ctx, strings.TrimRight(baseURL, "/")+path)
	component.LatencyMS = latency
	if err != nil {
		component.Status = StatusCritical
		component.Detail = err.Error()
		return component
	}
	_ = body
	component.Status = StatusHealthy
	return component
}

func (c *Collector) storageComponent(name string, report *storageReport, fetchErr error, now time.Time) Component {
	component := Component{Name: name, Status: StatusNotConfigured, ObservedAt: now}
	if c.cfg.WriterURL == "" {
		return component
	}
	if report == nil {
		component.Status = StatusUnknown
		if fetchErr != nil {
			component.Detail = fetchErr.Error()
		}
		return component
	}
	endpoint := report.clickHouse
	if name == "victoriametrics" {
		endpoint = report.victoriaMetrics
	}
	component.LatencyMS = endpoint.LatencyMS
	component.ObservedAt = report.observedAt
	if endpoint.OK {
		component.Status = StatusHealthy
		return component
	}
	component.Status = StatusCritical
	if endpoint.Error != "" {
		component.Detail = endpoint.Error
	} else {
		component.Detail = "storage health is not available"
	}
	if fetchErr != nil {
		component.Detail = "showing last known values; " + fetchErr.Error()
	}
	return component
}

func buildIngestion(cfg Config, writer *writerReport, gateway *gatewayReport, now time.Time) Ingestion {
	ingestion := Ingestion{Source: "writer /stats + gateway /metrics"}
	if cfg.WriterURL == "" {
		ingestion.Status = StatusNotConfigured
		ingestion.Detail = "set OBSERVAKI_SELF_WRITER_URL to collect ingestion health"
		return ingestion
	}
	if writer == nil {
		ingestion.Status = StatusCritical
		ingestion.Detail = "writer /stats is unreachable"
		return ingestion
	}
	ingestion.ObservedAt = &writer.observedAt
	ingestion.LagP95Seconds = writer.lagP95
	pending := writer.pending
	ingestion.PendingMessages = &pending
	dlq := writer.dlq
	ingestion.DLQMessages = &dlq
	written := writer.written
	ingestion.WrittenEvents = &written
	writerConnected := writer.connected
	ingestion.WriterConnected = &writerConnected
	if gateway != nil {
		ingestion.GatewayConnected = gateway.connected
		ingestion.GatewayPublishedEvents = gateway.published
	}

	switch {
	case !writer.up:
		ingestion.Status = StatusDegraded
		ingestion.Detail = "writer WAL workers are not running"
	case !writer.connected:
		ingestion.Status = StatusDegraded
		ingestion.Detail = "writer is disconnected from the NATS WAL"
	case writer.lagP95 == nil:
		ingestion.Status = StatusNoData
		ingestion.Detail = "no telemetry has been confirmed by the writer yet"
	case time.Duration(*writer.lagP95*float64(time.Second)) > cfg.IngestLagThreshold:
		ingestion.Status = StatusDegraded
		ingestion.Detail = fmt.Sprintf("ingestion lag p95 is above the %s threshold", cfg.IngestLagThreshold)
	default:
		ingestion.Status = StatusHealthy
	}
	if writer.lastError != "" {
		ingestion.Detail = strings.TrimSpace(ingestion.Detail + " (last write error at " + writer.lastError + ")")
	}
	return ingestion
}

func buildStorage(cfg Config, report *storageReport, fetchErr error, now time.Time) Storage {
	storage := Storage{Status: StatusNotConfigured}
	if cfg.WriterURL == "" {
		storage.Detail = "set OBSERVAKI_SELF_WRITER_URL to collect storage usage"
		return storage
	}
	if report == nil {
		storage.Status = StatusUnknown
		if fetchErr != nil {
			storage.Detail = fetchErr.Error()
		}
		return storage
	}
	observed := report.observedAt
	storage.ObservedAt = &observed
	storage.ClickHouseBytes = report.clickHouse.Bytes
	storage.ClickHouseParts = report.clickHouse.Parts
	storage.VictoriaMetricsBytes = report.victoriaMetrics.Bytes
	storage.VictoriaMetricsSeries = report.victoriaMetrics.Series
	switch {
	case report.clickHouse.OK && report.victoriaMetrics.OK:
		storage.Status = StatusHealthy
	case !report.clickHouse.OK && !report.victoriaMetrics.OK:
		storage.Status = StatusCritical
		storage.Detail = "both storage backends are unavailable"
	case !report.clickHouse.OK:
		storage.Status = StatusDegraded
		storage.Detail = "ClickHouse is unavailable"
	default:
		storage.Status = StatusDegraded
		storage.Detail = "VictoriaMetrics is unavailable"
	}
	if fetchErr != nil {
		storage.Detail = strings.TrimSpace(storage.Detail + "; showing last known values (" + fetchErr.Error() + ")")
	}
	return storage
}

func buildQuery(source QuerySource) Query {
	if source == nil {
		return Query{Status: StatusNotConfigured, Detail: "the query plane is not configured"}
	}
	stats := source.QueryStats()
	query := Query{
		Requests:     stats.Requests,
		Errors:       stats.Errors,
		ClientErrors: stats.ClientErrors,
		Since:        stats.Since,
	}
	if stats.Requests > 0 {
		query.ErrorRate = float64(stats.Errors) / float64(stats.Requests)
	}
	switch {
	case stats.Requests == 0:
		query.Status = StatusIdle
		query.Detail = "no query traffic yet; the Query Service arrives in Phase 1"
	case query.ErrorRate >= 0.5:
		query.Status = StatusCritical
	case query.ErrorRate >= 0.05:
		query.Status = StatusDegraded
	default:
		query.Status = StatusHealthy
	}
	return query
}

func summarize(overview Overview, lagThreshold time.Duration) (string, []string) {
	summary := make([]string, 0, 4)
	status := StatusHealthy
	hasCritical, hasDegraded := false, false
	for _, component := range overview.Components {
		switch component.Status {
		case StatusCritical:
			hasCritical = true
			summary = append(summary, component.Name+" is unavailable")
		case StatusDegraded:
			hasDegraded = true
			summary = append(summary, component.Name+" is degraded")
		}
	}
	switch overview.Ingestion.Status {
	case StatusCritical:
		hasCritical = true
	case StatusDegraded:
		hasDegraded = true
	}
	if overview.Ingestion.LagP95Seconds != nil &&
		time.Duration(*overview.Ingestion.LagP95Seconds*float64(time.Second)) > lagThreshold {
		hasDegraded = true
		summary = append(summary, fmt.Sprintf("ingestion lag p95 is above %s", lagThreshold))
	}
	if overview.Ingestion.Status == StatusNoData {
		summary = append(summary, "no telemetry has been ingested yet")
	}
	switch {
	case hasCritical:
		status = StatusCritical
	case hasDegraded:
		status = StatusDegraded
	case overview.Ingestion.Status == StatusNoData:
		status = StatusNoData
	}
	if len(summary) == 0 {
		summary = append(summary, "all systems operational")
	}
	return status, summary
}

func (c *Collector) get(ctx context.Context, url string) ([]byte, float64, error) {
	started := c.now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, float64(c.now().Sub(started)) / float64(time.Millisecond), err
	}
	defer func() { _ = response.Body.Close() }()
	latency := float64(c.now().Sub(started)) / float64(time.Millisecond)
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, latency, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(string(body))
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return nil, latency, fmt.Errorf("%s: HTTP %d: %s", url, response.StatusCode, detail)
	}
	return body, latency, nil
}
