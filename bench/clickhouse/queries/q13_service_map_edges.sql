-- q13 — Service map caller-callee edges with p95 (1h).
-- Catalog: bench/queries/dashboard.tsv (q13, Services, sql, 1h).
--
-- Parameters: tenant, project, start, end.
-- A 1% deterministic trace sample keeps the self-join bounded on the local
-- dataset. The sample is keyed by trace_id, so a whole trace (parent and
-- children) is either in or out. Counts are scaled back by 100 and the report
-- labels them as estimates. This is a spike validation artifact, not the
-- production RED source (AD-21).
WITH sampled AS
(
    SELECT
        trace_id,
        span_id,
        parent_span_id,
        service_name,
        duration_ns,
        status_code
    FROM otel_traces
    WHERE tenant_id = {tenant:String}
      AND project_id = {project:String}
      AND timestamp >= {start:DateTime64(9)}
      AND timestamp < {end:DateTime64(9)}
      AND cityHash64(trace_id) % 100 = 0
)
SELECT
    parent.service_name AS caller,
    child.service_name AS callee,
    count() * 100 AS spans_estimated,
    countIf(child.status_code = 'ERROR') * 100 AS errors_estimated,
    round(quantile(0.95)(child.duration_ns) / 1000000, 3) AS p95_ms
FROM sampled AS child
INNER JOIN sampled AS parent
    ON child.trace_id = parent.trace_id
   AND child.parent_span_id = parent.span_id
WHERE child.parent_span_id != ''
GROUP BY caller, callee
ORDER BY spans_estimated DESC;
