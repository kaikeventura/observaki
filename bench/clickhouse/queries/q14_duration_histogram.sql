-- q14 — Span duration histogram by service (24h).
-- Catalog: bench/queries/dashboard.tsv (q14, Explore/Traces, sql, 24h).
--
-- Parameters: tenant, project, start, end.
-- Explicit buckets keep the result easy to verify and avoid a large array per
-- service. The dashboard renders the same buckets as a stacked bar chart.
SELECT
    service_name,
    countIf(duration_ns < 1000000) AS lt_1ms,
    countIf(duration_ns >= 1000000 AND duration_ns < 10000000) AS lt_10ms,
    countIf(duration_ns >= 10000000 AND duration_ns < 100000000) AS lt_100ms,
    countIf(duration_ns >= 100000000 AND duration_ns < 1000000000) AS lt_1s,
    countIf(duration_ns >= 1000000000 AND duration_ns < 2000000000) AS lt_2s,
    countIf(duration_ns >= 2000000000) AS gte_2s
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY service_name
ORDER BY service_name ASC;
