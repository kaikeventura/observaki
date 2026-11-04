-- 0002_otel_traces.sql — OTLP traces, one row per span.
--
-- Storage contract (AD-5, AD-9, AD-21, AD-29):
--   * ClickHouse owns logs and traces; RED and span metrics stay in
--     VictoriaMetrics. This table stores raw spans only. It has no
--     span_metrics materialized view and no RED aggregate. A service map
--     derived from these rows is a spike validation artifact, not the
--     production RED source (AD-21).
--   * ingested_at is the TTL authority, not the producer timestamp.
--   * Retention is per row through retention_days (7/15/30), the strategy
--     validated for logs in Story 3.1. A project keeps the value it was
--     ingested with; changing a policy affects new rows and is reconciled by
--     the control plane (Story 3.5).
--   * Daily partitions by ingested_at keep the part count manageable for the
--     1B-span spike. At T1 volume the writer can switch to
--     toStartOfHour(ingested_at) after a capacity review; ORDER BY and TTL do
--     not change.
--
-- Query shape:
--   * Trace ID lookup uses the bloom_filter on trace_id. span_id and
--     parent_span_id have their own bloom filters for waterfall and service-map
--     joins.
--   * Service analysis uses the primary key: tenant, project and service come
--     first, then status_code, then time. A service + status + window query
--     reads a narrow slice instead of every span of the service.
--
-- Codecs: ZSTD(3) on maps, arrays, IDs and strings; Delta(8) on timestamps and
-- duration. The 2026-11-02 local spike measured the compression ratio on 1B
-- spans in bench/clickhouse/traces/results/.
CREATE TABLE IF NOT EXISTS otel_traces
(
    tenant_id           LowCardinality(String),
    project_id          LowCardinality(String),
    timestamp           DateTime64(9, 'UTC') CODEC(Delta(8), ZSTD(3)),
    ingested_at         DateTime64(9, 'UTC') CODEC(Delta(8), ZSTD(3)),
    trace_id            String CODEC(ZSTD(3)),
    span_id             String CODEC(ZSTD(3)),
    parent_span_id      String CODEC(ZSTD(3)),
    service_name        LowCardinality(String),
    operation_name      LowCardinality(String),
    span_kind           LowCardinality(String),
    duration_ns         UInt64 CODEC(Delta(8), ZSTD(3)),
    status_code         LowCardinality(String),
    status_message      String CODEC(ZSTD(3)),
    attributes          Map(String, String) CODEC(ZSTD(3)),
    resource_attributes Map(String, String) CODEC(ZSTD(3)),
    events              Array(Tuple(name String, timestamp DateTime64(9, 'UTC'), attributes Map(String, String))) CODEC(ZSTD(3)),
    links               Array(Tuple(trace_id String, span_id String, attributes Map(String, String))) CODEC(ZSTD(3)),
    event_id            String CODEC(ZSTD(3)),
    retention_days      UInt16 DEFAULT 15,
    INDEX idx_trace  trace_id       TYPE bloom_filter GRANULARITY 1,
    INDEX idx_span   span_id        TYPE bloom_filter GRANULARITY 1,
    INDEX idx_parent parent_span_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_service service_name  TYPE set(1000) GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(ingested_at)
ORDER BY (tenant_id, project_id, service_name, status_code, timestamp)
TTL ingested_at + INTERVAL retention_days DAY
SETTINGS index_granularity = 8192;
