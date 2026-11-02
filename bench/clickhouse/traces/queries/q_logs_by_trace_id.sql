-- Trace -> logs correlation: find every log that points at the trace.
SELECT
    event_id,
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
