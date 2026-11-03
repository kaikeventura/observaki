-- q15 — Logs correlated by trace_id (point).
-- Catalog: bench/queries/dashboard.tsv (q15, Explore/Logs, sql, point).
--
-- Parameters: tenant, project, trace_id.
-- The bloom_filter on trace_id is the access path.
SELECT
    event_id,
    timestamp,
    trace_id,
    span_id,
    service_name,
    body
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND trace_id = {trace_id:String}
ORDER BY timestamp ASC
LIMIT 100;
