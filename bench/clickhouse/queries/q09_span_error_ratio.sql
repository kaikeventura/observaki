-- q09 — Span error ratio by service (24h).
-- Catalog: bench/queries/dashboard.tsv (q09, Explore/Traces, sql, 24h).
--
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    countIf(status_code = 'ERROR') AS errors,
    count() AS spans,
    round(errors / spans, 4) AS error_ratio
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY service_name
ORDER BY error_ratio DESC;
