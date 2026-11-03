-- r14 — Span duration histogram by service (rollup).
-- Parameters: tenant, project, start, end.
SELECT
    service_name,
    sum(lt_1ms) AS lt_1ms,
    sum(lt_10ms) AS lt_10ms,
    sum(lt_100ms) AS lt_100ms,
    sum(lt_1s) AS lt_1s,
    sum(lt_2s) AS lt_2s,
    sum(gte_2s) AS gte_2s
FROM otel_traces_1m
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND minute >= {start:DateTime}
  AND minute < {end:DateTime}
GROUP BY service_name
ORDER BY service_name ASC;
