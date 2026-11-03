-- r07 — p95 span duration by service (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    sum(spans) AS spans,
    round(quantileMerge(0.95)(p95_state) / 1000000, 3) AS p95_ms
FROM otel_traces_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY service_name
ORDER BY p95_ms DESC;
