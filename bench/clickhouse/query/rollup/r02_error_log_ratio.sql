-- r02 — Error log ratio by service (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    sumIf(logs, severity = 'ERROR') AS error_count,
    sum(logs) AS log_count,
    round(error_count / log_count, 4) AS error_ratio
FROM otel_logs_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY service_name
ORDER BY error_ratio DESC;
