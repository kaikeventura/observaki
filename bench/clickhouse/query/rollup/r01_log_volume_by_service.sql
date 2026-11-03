-- r01 — Log volume by service in 1-minute buckets (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    minute,
    service_name,
    sum(logs) AS logs
FROM otel_logs_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY minute, service_name
ORDER BY minute ASC, logs DESC
LIMIT 1000;
