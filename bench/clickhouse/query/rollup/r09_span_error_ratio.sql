-- r09 — Span error ratio by service (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    sum(errors) AS error_count,
    sum(spans) AS span_count,
    round(error_count / span_count, 4) AS error_ratio
FROM otel_traces_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY service_name
ORDER BY error_ratio DESC;
