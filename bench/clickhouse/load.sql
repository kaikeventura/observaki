-- Synthetic OTLP log records for the Story 3.1 ClickHouse storage spike.
--
-- Parameters:
--   {base:DateTime64(9)}  end of the 24h event-time window (also the max timestamp)
--   {offset:UInt64}       first global row index of the chunk
--   {count:UInt64}        number of rows in the chunk
--
-- The dimensions come from different slices of one CityHash64 so tenant,
-- project, service and severity are independent. A modulo-only generator
-- makes some tenant/project/service/severity combinations impossible
-- (for example, tenant-0 + project-0 + checkout + ERROR), which would make
-- the q03 search return zero rows.
INSERT INTO otel_logs
SELECT
    tenant_id,
    project_id,
    ts AS timestamp,
    ts + toIntervalSecond(h % 5) AS ingested_at,
    ts AS observed_timestamp,
    sev AS severity,
    toUInt8(if(sev = 'INFO', 9, if(sev = 'WARN', 13, 17))) AS severity_number,
    svc AS service_name,
    concat(
        'level=', sev,
        ' service=', svc,
        ' msg="request completed" route=', route,
        ' status=', toString(status),
        ' duration_ms=', toString(intDiv(h, 131072) % 1000),
        ' trace_id=', trace_id,
        ' span_id=', span_id
    ) AS body,
    map('http.method', 'GET', 'http.route', route, 'http.status_code', toString(status), 'region', 'us-east-1') AS attributes,
    map('service.name', svc, 'host.name', concat('bench-host-', toString(host)), 'deployment.environment', 'production') AS resource_attributes,
    trace_id,
    span_id,
    event_id,
    if(intDiv(h, 2) % 4 < 2, 7, 15) AS retention_days
FROM
(
    SELECT
        *,
        concat('tenant-', toString(h % 2)) AS tenant_id,
        concat('project-', toString(intDiv(h, 2) % 4)) AS project_id,
        multiIf(intDiv(h, 64) % 100 < 90, 'INFO', intDiv(h, 64) % 100 < 97, 'WARN', 'ERROR') AS sev,
        ['checkout','payment','cart','inventory','search','auth','shipping','catalog'][(intDiv(h, 8) % 8) + 1] AS svc,
        ['/api/v1/checkout','/api/v1/payment','/api/v1/cart','/api/v1/inventory','/api/v1/search','/api/v1/login','/api/v1/shipping','/api/v1/catalog'][(intDiv(h, 8) % 8) + 1] AS route,
        200 + (intDiv(h, 512) % 5) * 100 AS status,
        intDiv(h, 8192) % 16 AS host,
        concat(hex(cityHash64(h, 1)), hex(cityHash64(h, 2))) AS trace_id,
        hex(cityHash64(h, 3)) AS span_id,
        concat(hex(cityHash64(h, 4)), hex(cityHash64(h, 5))) AS event_id
    FROM
    (
        SELECT
            number,
            cityHash64(number) AS h,
            toDateTime64({base:DateTime64(9)} - toIntervalSecond(number % 86400) - toIntervalNanosecond(number % 1000000000), 9, 'UTC') AS ts
        FROM numbers({offset:UInt64}, {count:UInt64})
    )
);
