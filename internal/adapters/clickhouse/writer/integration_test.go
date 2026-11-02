package writer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestIntegrationClickHouse runs only when OBSERVAKI_CLICKHOUSE_URL points to
// a ClickHouse with migrations 0001-0003 applied. The benchmark harness sets
// it; plain `go test ./...` skips the test.
func TestIntegrationClickHouse(t *testing.T) {
	endpoint := os.Getenv("OBSERVAKI_CLICKHOUSE_URL")
	if endpoint == "" {
		t.Skip("OBSERVAKI_CLICKHOUSE_URL is not set")
	}
	database := envOr("OBSERVAKI_CLICKHOUSE_DATABASE", "observaki_bench")
	user := envOr("OBSERVAKI_CLICKHOUSE_USER", "observaki")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inserter, err := NewHTTPInserter(HTTPConfig{Endpoint: endpoint, Database: database, User: user})
	if err != nil {
		t.Fatalf("NewHTTPInserter() error = %v", err)
	}
	tenant := fmt.Sprintf("it-%d", time.Now().UnixNano())
	cfg := testConfig(t.TempDir())
	logs, err := OpenLogs(ctx, cfg, inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		closeContext, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()
		_ = logs.Close(closeContext)
	}()

	eventTime := time.Now().UTC()
	records := []LogRecord{
		{
			TenantID:       tenant,
			ProjectID:      "project-0",
			Timestamp:      eventTime,
			Severity:       "INFO",
			SeverityNumber: 9,
			ServiceName:    "checkout",
			Body:           "integration log 1",
			EventID:        tenant + "-e1",
		},
		{
			TenantID:       tenant,
			ProjectID:      "project-0",
			Timestamp:      eventTime,
			Severity:       "INFO",
			SeverityNumber: 9,
			ServiceName:    "checkout",
			Body:           "integration log 2",
			EventID:        tenant + "-e2",
		},
	}
	result, err := logs.SubmitLogs(ctx, records)
	if err != nil {
		t.Fatalf("SubmitLogs() error = %v", err)
	}
	if result.Rows != 2 {
		t.Fatalf("SubmitLogs() rows = %d, want 2", result.Rows)
	}
	waitFor(t, "integration log ack", func() bool { return logs.Stats().AckedBatches == 1 })

	duplicate, err := logs.SubmitLogs(ctx, records)
	if err != nil {
		t.Fatalf("SubmitLogs(duplicate) error = %v", err)
	}
	if duplicate.Rows != 0 || duplicate.Duplicates != 2 {
		t.Fatalf("duplicate SubmitLogs() = %+v, want 2 duplicates and no rows", duplicate)
	}

	count := clickhouseCount(t, endpoint, database, user, "otel_logs", tenant)
	if count != 2 {
		t.Fatalf("otel_logs rows = %d, want 2", count)
	}
	if got := logs.Stats().DuplicateEvents; got != 2 {
		t.Fatalf("DuplicateEvents = %d, want 2", got)
	}

	traces, err := OpenTraces(ctx, testConfig(t.TempDir()), inserter)
	if err != nil {
		t.Fatalf("OpenTraces() error = %v", err)
	}
	defer func() {
		closeContext, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()
		_ = traces.Close(closeContext)
	}()
	spanResult, err := traces.SubmitTraces(ctx, []SpanRecord{{
		TenantID:      tenant,
		ProjectID:     "project-0",
		Timestamp:     eventTime,
		TraceID:       tenant + "-trace",
		SpanID:        tenant + "-span",
		ServiceName:   "checkout",
		OperationName: "GET /cart",
		SpanKind:      "server",
		DurationNs:    1200,
		StatusCode:    "OK",
		EventID:       tenant + "-span-event",
	}})
	if err != nil {
		t.Fatalf("SubmitTraces() error = %v", err)
	}
	if spanResult.Rows != 1 {
		t.Fatalf("SubmitTraces() rows = %d, want 1", spanResult.Rows)
	}
	waitFor(t, "integration span ack", func() bool { return traces.Stats().AckedBatches == 1 })
	if got := clickhouseCount(t, endpoint, database, user, "otel_traces", tenant); got != 1 {
		t.Fatalf("otel_traces rows = %d, want 1", got)
	}
}

// TestIntegrationReplayDeduplicates reproduces the crash-after-insert window:
// the block is already in ClickHouse but the writer's checkpoint did not
// advance. The replay must reuse the token and ClickHouse must drop it.
func TestIntegrationReplayDeduplicates(t *testing.T) {
	endpoint := os.Getenv("OBSERVAKI_CLICKHOUSE_URL")
	if endpoint == "" {
		t.Skip("OBSERVAKI_CLICKHOUSE_URL is not set")
	}
	database := envOr("OBSERVAKI_CLICKHOUSE_DATABASE", "observaki_bench")
	user := envOr("OBSERVAKI_CLICKHOUSE_USER", "observaki")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inserter, err := NewHTTPInserter(HTTPConfig{Endpoint: endpoint, Database: database, User: user})
	if err != nil {
		t.Fatalf("NewHTTPInserter() error = %v", err)
	}
	tenant := fmt.Sprintf("replay-%d", time.Now().UnixNano())
	eventTime := time.Now().UTC()
	record := LogRecord{
		TenantID:       tenant,
		ProjectID:      "project-0",
		Timestamp:      eventTime,
		Severity:       "INFO",
		SeverityNumber: 9,
		ServiceName:    "checkout",
		Body:           "replayed log",
		EventID:        tenant + "-e1",
	}
	data := record.appendRowBinary(nil, eventTime, eventTime)
	sum := sha256.Sum256(data)
	token := batchToken(SignalLogs, 1, sum)

	first, err := inserter.Insert(ctx, InsertRequest{Table: "otel_logs", Data: data, Rows: 1, DedupeToken: token})
	if err != nil {
		t.Fatalf("first Insert() error = %v", err)
	}
	if first.WrittenRows != 1 {
		t.Fatalf("first Insert() written rows = %d, want 1", first.WrittenRows)
	}
	if got := clickhouseCount(t, endpoint, database, user, "otel_logs", tenant); got != 1 {
		t.Fatalf("otel_logs rows after first insert = %d, want 1", got)
	}

	dir := t.TempDir()
	spool, err := openSpool(dir)
	if err != nil {
		t.Fatalf("openSpool() error = %v", err)
	}
	meta := &batchMeta{
		Seq:                  1,
		Table:                "otel_logs",
		Signal:               SignalLogs,
		Rows:                 1,
		DedupeToken:          token,
		MinTimestampUnixNano: eventTime.UnixNano(),
		MaxTimestampUnixNano: eventTime.UnixNano(),
		DataSHA256:           hex.EncodeToString(sum[:]),
	}
	if _, err := spool.write(meta, data); err != nil {
		t.Fatalf("spool.write() error = %v", err)
	}

	replayed, err := OpenLogs(ctx, testConfig(dir), inserter)
	if err != nil {
		t.Fatalf("OpenLogs() error = %v", err)
	}
	defer func() {
		closeContext, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer closeCancel()
		_ = replayed.Close(closeContext)
	}()
	stats := replayed.Stats()
	if stats.RecoveredEvents != 1 {
		t.Fatalf("RecoveredEvents = %d, want 1", stats.RecoveredEvents)
	}
	if stats.InsertedEvents != 1 {
		t.Fatalf("InsertedEvents = %d, want the processed row count (1)", stats.InsertedEvents)
	}
	if stats.RecoveredBatches != 1 {
		t.Fatalf("RecoveredBatches = %d, want 1", stats.RecoveredBatches)
	}
	if got := clickhouseCount(t, endpoint, database, user, "otel_logs", tenant); got != 1 {
		t.Fatalf("otel_logs rows after replay = %d, want 1", got)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func clickhouseCount(t *testing.T, endpoint, database, user, table, tenant string) int {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	query := parsed.Query()
	query.Set("database", database)
	query.Set("user", user)
	query.Set("default_format", "JSONEachRow")
	query.Set("query", fmt.Sprintf("SELECT count() AS count FROM %s WHERE tenant_id = '%s'", table, tenant))
	parsed.RawQuery = query.Encode()

	request, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		t.Fatalf("build query request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("query ClickHouse: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("query ClickHouse status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var row struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &row); err != nil {
		t.Fatalf("decode ClickHouse response %q: %v", string(body), err)
	}
	return row.Count
}
