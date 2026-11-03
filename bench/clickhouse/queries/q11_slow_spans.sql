-- q11 — Slow spans over 2s for one service (15m).
-- Catalog: bench/queries/dashboard.tsv (q11, Explore/Traces, sql, 15m).
--
-- Parameters: tenant, project, service, start, end.
-- The generator gives root spans 1–3s durations and child spans 1–500ms;
-- frontend is the root service, so it has spans over the 2s threshold.
SELECT
    timestamp,
    trace_id,
    span_id,
    operation_name,
    duration_ns,
    status_code
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
  AND duration_ns > 2000000000
ORDER BY duration_ns DESC
LIMIT 100;
