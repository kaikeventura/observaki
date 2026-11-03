-- q04 — Top 10 services by log volume (24h).
-- Catalog: bench/queries/dashboard.tsv (q04, Explore/Logs, sql, 24h).
--
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    count() AS logs
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY service_name
ORDER BY logs DESC
LIMIT 10;
