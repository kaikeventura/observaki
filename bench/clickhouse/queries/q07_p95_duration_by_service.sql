-- q07 — p95 span duration by service (24h).
-- Catalog: bench/queries/dashboard.tsv (q07, Explore/Traces, sql, 24h).
--
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    count() AS spans,
    round(quantile(0.95)(duration_ns) / 1000000, 3) AS p95_ms
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY service_name
ORDER BY p95_ms DESC;
