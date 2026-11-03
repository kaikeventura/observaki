-- r08 — p99 span duration by operation for one service (rollup).
-- Parameters: tenant, project, service, start, end.
SELECT
    operation_name,
    sum(spans) AS spans,
    round(quantileMerge(0.99)(p99_state) / 1000000, 3) AS p99_ms
FROM otel_traces_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY operation_name
ORDER BY p99_ms DESC;
