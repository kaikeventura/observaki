package selfobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeQuery struct {
	stats QueryStats
}

func (f fakeQuery) QueryStats() QueryStats { return f.stats }

type fakeAlerting struct {
	snapshot Alerting
}

func (f fakeAlerting) AlertingSnapshot() Alerting { return f.snapshot }

type fakePostgres struct {
	err error
}

func (f fakePostgres) Ping(context.Context) error { return f.err }

func newTestCollector(t *testing.T, writer *httptest.Server, gateway *httptest.Server, nats *httptest.Server, query QuerySource, alerting AlertingSource) *Collector {
	t.Helper()
	cfg := Config{
		IngestLagThreshold: 10 * time.Second,
		Refresh:            time.Millisecond,
		Timeout:            time.Second,
	}
	if writer != nil {
		cfg.WriterURL = writer.URL
	}
	if gateway != nil {
		cfg.GatewayURL = gateway.URL
	}
	if nats != nil {
		cfg.NATSMonitorURL = nats.URL
	}
	collector := New(cfg, query, alerting, fakePostgres{})
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	collector.now = func() time.Time { return now }
	return collector
}

func healthyWriter(t *testing.T, lagMS float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		case "/stats":
			_, _ = w.Write([]byte(`{
				"up": true, "connected": true,
				"tenants": [
					{"tenant_id":"tenant-dev","signal":"logs","written_events":120,"lag_p95_ms":` + formatFloat(lagMS) + `},
					{"tenant_id":"tenant-dev","signal":"traces","written_events":30,"lag_p95_ms":1}
				],
				"signals": {
					"logs":{"dlq_messages":0,"pending_messages":0},
					"traces":{"dlq_messages":2,"pending_messages":1},
					"metrics":{"dlq_messages":0,"pending_messages":0}
				}
			}`))
		case "/storage":
			_, _ = w.Write([]byte(`{
				"observed_at":"2026-11-08T12:00:00Z",
				"clickhouse":{"ok":true,"bytes":4096,"parts":3,"latency_ms":1.2},
				"victoriametrics":{"ok":true,"bytes":2048,"series":7,"latency_ms":0.8}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func healthyGateway(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		case "/metrics":
			_, _ = w.Write([]byte(`# TYPE observaki_gateway_wal_connected gauge
observaki_gateway_wal_connected 1
observaki_gateway_wal_published_events_total 500
observaki_gateway_wal_pending_events 0
observaki_gateway_events_total{signal="logs",protocol="http"} 500
`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func TestOverviewHealthy(t *testing.T) {
	writer := healthyWriter(t, 800)
	defer writer.Close()
	gateway := healthyGateway(t)
	defer gateway.Close()
	nats := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer nats.Close()

	collector := newTestCollector(t, writer, gateway, nats, fakeQuery{stats: QueryStats{
		Requests: 10, Errors: 0, Since: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
	}}, fakeAlerting{snapshot: Alerting{
		Enabled: true, Status: StatusHealthy, StaleAfterSeconds: 45,
		LastEvaluationAt: timePtr(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)),
	}})

	overview := collector.Overview(context.Background())
	if overview.Status != StatusHealthy {
		t.Fatalf("status = %q, summary = %v", overview.Status, overview.Summary)
	}
	if overview.Ingestion.LagP95Seconds == nil || *overview.Ingestion.LagP95Seconds != 0.8 {
		t.Fatalf("lag = %+v", overview.Ingestion.LagP95Seconds)
	}
	if overview.Ingestion.PendingMessages == nil || *overview.Ingestion.PendingMessages != 1 {
		t.Fatalf("pending = %+v", overview.Ingestion.PendingMessages)
	}
	if overview.Ingestion.DLQMessages == nil || *overview.Ingestion.DLQMessages != 2 {
		t.Fatalf("dlq = %+v", overview.Ingestion.DLQMessages)
	}
	if overview.Ingestion.WrittenEvents == nil || *overview.Ingestion.WrittenEvents != 150 {
		t.Fatalf("written = %+v", overview.Ingestion.WrittenEvents)
	}
	if overview.Storage.ClickHouseBytes == nil || *overview.Storage.ClickHouseBytes != 4096 {
		t.Fatalf("storage = %+v", overview.Storage)
	}
	if overview.Storage.VictoriaMetricsSeries == nil || *overview.Storage.VictoriaMetricsSeries != 7 {
		t.Fatalf("series = %+v", overview.Storage.VictoriaMetricsSeries)
	}
	if overview.Query.Status != StatusHealthy || overview.Query.ErrorRate != 0 {
		t.Fatalf("query = %+v", overview.Query)
	}
	if len(overview.Components) != 8 {
		t.Fatalf("components = %d, want 8", len(overview.Components))
	}
}

func TestOverviewDegradedLag(t *testing.T) {
	writer := healthyWriter(t, 12000)
	defer writer.Close()
	gateway := healthyGateway(t)
	defer gateway.Close()
	nats := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer nats.Close()

	collector := newTestCollector(t, writer, gateway, nats, fakeQuery{}, fakeAlerting{snapshot: Alerting{
		Enabled: true, Status: StatusHealthy,
		LastEvaluationAt: timePtr(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)),
	}})
	overview := collector.Overview(context.Background())
	if overview.Status != StatusDegraded {
		t.Fatalf("status = %q, want degraded", overview.Status)
	}
	if overview.Ingestion.Status != StatusDegraded {
		t.Fatalf("ingestion = %q, want degraded", overview.Ingestion.Status)
	}
	found := false
	for _, line := range overview.Summary {
		if strings.Contains(line, "ingestion lag p95 is above") {
			found = true
		}
	}
	if !found {
		t.Fatalf("summary = %v, want lag message", overview.Summary)
	}
}

func TestOverviewKeepsLastKnownValues(t *testing.T) {
	writerUp := true
	writer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !writerUp {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/readyz":
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		case "/stats":
			_, _ = w.Write([]byte(`{"up":true,"connected":true,"tenants":[{"tenant_id":"t","signal":"logs","written_events":1,"lag_p95_ms":500}],"signals":{}}`))
		case "/storage":
			_, _ = w.Write([]byte(`{"clickhouse":{"ok":true,"bytes":10},"victoriametrics":{"ok":true,"series":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer writer.Close()

	collector := newTestCollector(t, writer, nil, nil, fakeQuery{}, nil)
	first := collector.Overview(context.Background())
	if first.Ingestion.LagP95Seconds == nil {
		t.Fatal("first overview has no lag")
	}

	writerUp = false
	collector.now = func() time.Time { return first.GeneratedAt.Add(time.Minute) }
	second := collector.Overview(context.Background())
	if second.Status != StatusCritical {
		t.Fatalf("status = %q, want critical", second.Status)
	}
	if second.Ingestion.LagP95Seconds == nil || *second.Ingestion.LagP95Seconds != 0.5 {
		t.Fatalf("last known lag = %+v", second.Ingestion.LagP95Seconds)
	}
	if second.Storage.ClickHouseBytes == nil || *second.Storage.ClickHouseBytes != 10 {
		t.Fatalf("last known storage = %+v", second.Storage)
	}
}

func TestOverviewQueryIdleAndErrors(t *testing.T) {
	collector := newTestCollector(t, nil, nil, nil, fakeQuery{
		stats: QueryStats{Requests: 0, Since: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)},
	}, nil)
	overview := collector.Overview(context.Background())
	if overview.Query.Status != StatusIdle {
		t.Fatalf("query status = %q, want idle", overview.Query.Status)
	}
	if overview.Status != StatusHealthy {
		t.Fatalf("overall status = %q, want healthy without configured sources", overview.Status)
	}
	if overview.Alerting.Status != StatusDisabled {
		t.Fatalf("alerting = %q, want disabled", overview.Alerting.Status)
	}
}
