-- q06 — Free-text incident log search (15m).
-- Catalog: bench/queries/dashboard.tsv (q06, Explore/Logs, sql, 15m).
--
-- Parameters: tenant, project, start, end.
-- hasToken uses the tokenbf_v1 index on body. The MVP search is limited to
-- indexed/materialized fields; broad inverted-index search is Phase 2.
SELECT timestamp, service_name, severity, body
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
  AND hasToken(body, 'timeout')
ORDER BY timestamp DESC
LIMIT 100;
