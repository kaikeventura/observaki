-- Synthetic OTLP log records for the Story 3.5 query benchmark.
--
-- This is the Story 3.1 generator plus the fields the dashboard catalog needs:
--   * resource_attributes['pod.name'] for q05;
--   * an "upstream timeout" message for the q06 free-text search;
--   * retention_days 7/15/30 by project for AC3;
--   * project_generation/authz_epoch for AD-20.
--
-- Parameters:
--   {base:DateTime64(9)}  end of the 24h event-time window (max timestamp)
--   {offset:UInt64}       first global row index of the chunk
--   {count:UInt64}        number of rows in the chunk
--   {rows:UInt64}         total rows in the dataset
--
-- The dimensions come from different slices of one CityHash64 so tenant,
-- project, service and severity are independent. Timestamps are ordered by
-- the global row index: each chunk covers a contiguous slice of the 24h
-- window, as real ingestion arrives over time.
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
        ' msg="', msg, '" route=', route,
        ' status=', toString(status),
        ' duration_ms=', toString(intDiv(h, 131072) % 1000),
        ' trace_id=', trace_id,
        ' span_id=', span_id
    ) AS body,
    map('http.method', 'GET', 'http.route', route, 'http.status_code', toString(status), 'region', 'us-east-1') AS attributes,
    map(
        'service.name', svc,
        'host.name', concat('bench-host-', toString(host)),
        'pod.name', concat('bench-pod-', toString(pod)),
        'deployment.environment', 'production'
    ) AS resource_attributes,
    trace_id,
    span_id,
    event_id,
    retention_days,
    toUInt64(1) AS project_generation,
    toUInt64(1) AS authz_epoch
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
        intDiv(h, 16384) % 4 AS pod,
        if(h % 97 = 0, 'upstream timeout', 'request completed') AS msg,
        multiIf(
            intDiv(h, 2) % 4 = 0, toUInt16(7),
            intDiv(h, 2) % 4 = 1, toUInt16(15),
            intDiv(h, 2) % 4 = 2, toUInt16(30),
            toUInt16(15)
        ) AS retention_days,
        concat(hex(cityHash64(h, 1)), hex(cityHash64(h, 2))) AS trace_id,
        hex(cityHash64(h, 3)) AS span_id,
        concat(hex(cityHash64(h, 4)), hex(cityHash64(h, 5))) AS event_id
    FROM
    (
        SELECT
            number,
            cityHash64(number) AS h,
            toDateTime64({base:DateTime64(9)} - toIntervalSecond(86400) + toIntervalSecond(intDiv(number * 86400, {rows:UInt64}) + 1), 9, 'UTC') AS ts
        FROM numbers({offset:UInt64}, {count:UInt64})
    )
);
