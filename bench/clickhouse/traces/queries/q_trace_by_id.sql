-- Full trace by trace_id. The bloom_filter on trace_id is the access path;
-- the tenant/project predicates keep the tenancy contract in the query.
SELECT
    trace_id,
    span_id,
    parent_span_id,
    service_name,
    operation_name,
    span_kind,
    timestamp,
    duration_ns,
    status_code,
    status_message,
    attributes,
    resource_attributes,
    events,
    links
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND trace_id = {trace_id:String}
ORDER BY timestamp ASC, span_id ASC;
