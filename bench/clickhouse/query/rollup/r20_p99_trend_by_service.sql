-- r20 — p99 latency trend by service (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    minute,
    service_name,
    round(quantileMerge(0.99)(p99_state) / 1000000, 3) AS p99_ms
FROM otel_traces_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY minute, service_name
ORDER BY minute ASC, service_name ASC
LIMIT 1000;
