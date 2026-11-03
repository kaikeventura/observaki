-- r04 — Top 10 services by log volume (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    sum(logs) AS logs
FROM otel_logs_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY service_name
ORDER BY logs DESC
LIMIT 10;
