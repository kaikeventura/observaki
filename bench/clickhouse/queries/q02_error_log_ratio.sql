-- q02 — Error log ratio by service (24h).
-- Catalog: bench/queries/dashboard.tsv (q02, Explore/Logs, sql, 24h).
--
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    countIf(severity = 'ERROR') AS errors,
    count() AS logs,
    round(errors / logs, 4) AS error_ratio
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY service_name
ORDER BY error_ratio DESC;
