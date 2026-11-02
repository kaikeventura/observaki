-- q03 — Filtered log search by service and severity (1h window).
-- Catalog: bench/queries/dashboard.tsv (q03, Explore/Logs, sql, 1h).
--
-- Parameters: tenant, project, service, severity, start, end.
-- The ORDER BY matches the table sort key, so a 1h window for one service
-- reads a bounded slice instead of the whole partition.
SELECT timestamp, severity, service_name, body
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND severity = {severity:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
ORDER BY timestamp DESC
LIMIT 100;
