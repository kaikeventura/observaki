-- r10 — Span count by status code (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    status_code,
    sum(spans) AS spans
FROM otel_traces_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY status_code
ORDER BY spans DESC;
