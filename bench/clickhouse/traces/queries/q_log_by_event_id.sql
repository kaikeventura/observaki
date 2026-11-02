-- Log -> trace correlation: find the trace_id/span_id stored on the log.
SELECT
    trace_id,
    span_id,
    service_name,
    body
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND event_id = {event_id:String}
LIMIT 1;
