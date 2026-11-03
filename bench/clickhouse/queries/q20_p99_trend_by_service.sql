-- q20 — p99 latency trend by service (scenario window).
-- Catalog: bench/queries/dashboard.tsv (q20, Dashboard, sql, scenario).
--
-- Parameters: tenant, project, start, end.
SELECT
    toStartOfMinute(timestamp) AS minute,
    service_name,
    round(quantile(0.99)(duration_ns) / 1000000, 3) AS p99_ms
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY minute, service_name
ORDER BY minute ASC, service_name ASC
LIMIT 1000;
