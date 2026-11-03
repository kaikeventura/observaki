-- q01 — Log volume by service in 1-minute buckets (scenario window).
-- Catalog: bench/queries/dashboard.tsv (q01, Explore/Logs, sql, scenario).
--
-- Parameters: tenant, project, start, end.
SELECT
    toStartOfMinute(timestamp) AS minute,
    service_name,
    count() AS logs
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY minute, service_name
ORDER BY minute ASC, logs DESC
LIMIT 1000;
