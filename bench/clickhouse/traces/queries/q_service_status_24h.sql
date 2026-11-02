-- Recent spans for one service and status in a 24h window. The primary key
-- (tenant, project, service, status, timestamp) lets ClickHouse read the
-- newest granules of the requested service/status slice instead of scanning
-- the whole service.
SELECT
    timestamp,
    trace_id,
    span_id,
    service_name,
    operation_name,
    duration_ns,
    status_code,
    status_message
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND status_code = {status:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
ORDER BY timestamp DESC
LIMIT 100;
