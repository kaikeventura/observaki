-- q08 — p99 span duration by operation for one service (24h).
-- Catalog: bench/queries/dashboard.tsv (q08, Explore/Traces, sql, 24h).
--
-- Parameters: tenant, project, service, start, end.
SELECT
    operation_name,
    count() AS spans,
    round(quantile(0.99)(duration_ns) / 1000000, 3) AS p99_ms
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY operation_name
ORDER BY p99_ms DESC;
