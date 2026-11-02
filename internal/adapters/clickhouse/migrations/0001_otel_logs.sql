-- 0001_otel_logs.sql — OTLP logs, one row per log record.
--
-- Storage contract (AD-5, AD-9, AD-29):
--   * ClickHouse owns logs and traces; metrics and RED stay in VictoriaMetrics.
--   * ingested_at is the TTL authority, not the producer timestamp.
--   * Retention is per row through retention_days (7/15/30). A project keeps
--     the value it was ingested with; changing a policy affects new rows and
--     is reconciled by the control plane (Story 3.5). The bucket-table
--     fallback remains available if a later spike shows that per-row TTL is
--     too expensive.
--   * Daily partitions keep the part count manageable for the 1B-row spike.
--     At the T1 rate the writer can switch to toStartOfHour(ingested_at)
--     after a capacity review; the ORDER BY and TTL do not change.
--
-- Codecs: ZSTD(3) on the body, maps and identifiers plus Delta(8) on the
-- timestamps. The 2026-11-02 spike measured 6.5x compression on 100M rows,
-- above the 5x NFR-13 target. The body keeps a fixed shape with random
-- trace/span IDs, so the benchmark does not game the ratio with filler.
CREATE TABLE IF NOT EXISTS otel_logs
(
    tenant_id           LowCardinality(String),
    project_id          LowCardinality(String),
    timestamp           DateTime64(9, 'UTC') CODEC(Delta(8), ZSTD(3)),
    ingested_at         DateTime64(9, 'UTC') CODEC(Delta(8), ZSTD(3)),
    observed_timestamp  DateTime64(9, 'UTC') CODEC(Delta(8), ZSTD(3)),
    severity            LowCardinality(String),
    severity_number     UInt8,
    service_name        LowCardinality(String),
    body                String CODEC(ZSTD(3)),
    attributes          Map(String, String) CODEC(ZSTD(3)),
    resource_attributes Map(String, String) CODEC(ZSTD(3)),
    trace_id            String CODEC(ZSTD(3)),
    span_id             String CODEC(ZSTD(3)),
    event_id            String CODEC(ZSTD(3)),
    retention_days      UInt16 DEFAULT 15,
    INDEX idx_body  body      TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1,
    INDEX idx_trace trace_id  TYPE bloom_filter GRANULARITY 1,
    INDEX idx_span  span_id   TYPE bloom_filter GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(ingested_at)
ORDER BY (tenant_id, project_id, service_name, timestamp)
TTL ingested_at + INTERVAL retention_days DAY
SETTINGS index_granularity = 8192;
