-- Service-map nodes from raw spans in a window. This is a spike validation
-- artifact, not the production RED source: RED is computed by the gateway and
-- stored in VictoriaMetrics (AD-21).
SELECT
    service_name,
    count() AS spans,
    countIf(status_code = 'ERROR') AS errors,
    round(avg(duration_ns) / 1000000, 3) AS avg_duration_ms,
    round(quantile(0.95)(duration_ns) / 1000000, 3) AS p95_duration_ms
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
GROUP BY service_name
ORDER BY spans DESC;
