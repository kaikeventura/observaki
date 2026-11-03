-- q10 — Span count by status code (1h).
-- Catalog: bench/queries/dashboard.tsv (q10, Explore/Traces, sql, 1h).
--
-- Parameters: tenant, project, start, end.
SELECT
    status_code,
    count() AS spans
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY status_code
ORDER BY spans DESC;
