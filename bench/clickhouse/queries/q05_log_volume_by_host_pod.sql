-- q05 — Log volume by host and pod for one service (24h).
-- Catalog: bench/queries/dashboard.tsv (q05, Explore/Logs, sql, 24h).
--
-- Parameters: tenant, project, service, start, end.
-- host.name and pod.name are resource attributes; the schema keeps them in the
-- Map column and does not materialize them because they are not in the sort
-- key. The service predicate is the selective part of the query.
SELECT
    resource_attributes['host.name'] AS host,
    resource_attributes['pod.name'] AS pod,
    count() AS logs
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY host, pod
ORDER BY logs DESC
LIMIT 100;
