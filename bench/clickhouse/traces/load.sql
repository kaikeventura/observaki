-- Synthetic OTLP spans for the Story 3.2 ClickHouse storage spike.
--
-- Parameters:
--   {base:DateTime64(9)}  end of the 24h event-time window (max timestamp)
--   {offset:UInt64}       first global span index of the chunk
--   {count:UInt64}        number of spans in the chunk
--   {rows:UInt64}         total spans in the dataset
--
-- Every ten consecutive rows form one trace. The ten span slots are a small
-- deterministic service graph: frontend -> checkout -> payments -> email,
-- checkout -> inventory -> fraud, frontend -> shipping -> inventory and
-- frontend -> auth/search. That gives the service-map query real parent/child
-- edges instead of a flat set of services.
--
-- Trace, span, parent and event IDs come from CityHash64, so they look random
-- and do not inflate the compression ratio with a sequential pattern. They are
-- still deterministic, which makes the trace lookup reproducible.
--
-- Timestamps are ordered by the global span index: each 50M-span chunk covers
-- a contiguous ~1.2h slice of the 24h window. That is how real ingestion
-- arrives over time, and it keeps a one-hour service query on the last chunk
-- instead of touching every part of every chunk.
INSERT INTO otel_traces
SELECT
    concat('tenant-', toString(trace_idx % 2)) AS tenant_id,
    concat('project-', toString(trace_idx % 4)) AS project_id,
    ts AS timestamp,
    ts + toIntervalMillisecond(100 + h % 900) AS ingested_at,
    trace_id,
    span_id,
    parent_span_id,
    svc AS service_name,
    op AS operation_name,
    kind AS span_kind,
    if(span_idx = 0, 1000000000 + h % 2000000000, 1000000 + h % 500000000) AS duration_ns,
    status AS status_code,
    if(status = 'ERROR', concat('error-', toString(h % 10)), '') AS status_message,
    map(
        'http.method', if(span_idx = 0, 'GET', 'POST'),
        'http.status_code', if(status = 'ERROR', '500', '200'),
        'peer.service', peer_svc
    ) AS attributes,
    map(
        'service.name', svc,
        'service.version', '1.0.0',
        'deployment.environment', 'bench',
        'host.name', concat('host-', toString(trace_idx % 32))
    ) AS resource_attributes,
    arraySlice(
        [tuple('exception', ts + toIntervalMillisecond(1), map('exception.type', 'BenchError', 'exception.message', concat('failure-', toString(h % 5))))],
        1,
        if(h % 10 = 0, 1, 0)
    ) AS events,
    arraySlice(
        [tuple(link_trace_id, link_span_id, map('link.kind', 'follows'))],
        1,
        if(h % 20 = 0, 1, 0)
    ) AS links,
    event_id,
    if(trace_idx % 4 < 2, 7, 15) AS retention_days
FROM
(
    SELECT
        number,
        intDiv(number, 10) AS trace_idx,
        number % 10 AS span_idx,
        cityHash64(number) AS h,
        arrayElement([255, 0, 1, 1, 0, 2, 0, 0, 3, 4], (number % 10) + 1) AS parent_slot,
        arrayElement(['frontend', 'checkout', 'payments', 'inventory', 'shipping', 'email', 'auth', 'search', 'fraud', 'inventory'], (number % 10) + 1) AS svc,
        arrayElement(['GET /checkout', 'POST /order', 'charge', 'reserve', 'quote', 'send', 'verify', 'query', 'score', 'sync'], (number % 10) + 1) AS op,
        arrayElement(['SERVER', 'CLIENT', 'CLIENT', 'CLIENT', 'CLIENT', 'CLIENT', 'CLIENT', 'CLIENT', 'CLIENT', 'CLIENT'], (number % 10) + 1) AS kind,
        if(h % 100 < if(number % 10 = 0, 1, 3), 'ERROR', 'OK') AS status,
        if(
            parent_slot = 255,
            'frontend',
            arrayElement(['frontend', 'checkout', 'payments', 'inventory', 'shipping', 'email', 'auth', 'search', 'fraud', 'inventory'], parent_slot + 1)
        ) AS peer_svc,
        lower(concat(
            leftPad(hex(cityHash64(trace_idx, 1)), 16, '0'),
            leftPad(hex(cityHash64(trace_idx, 2)), 16, '0')
        )) AS trace_id,
        lower(leftPad(hex(cityHash64(trace_idx, span_idx, 3)), 16, '0')) AS span_id,
        if(parent_slot = 255, '', lower(leftPad(hex(cityHash64(trace_idx, parent_slot, 3)), 16, '0'))) AS parent_span_id,
        lower(concat(
            leftPad(hex(cityHash64(trace_idx, span_idx, 4)), 16, '0'),
            leftPad(hex(cityHash64(trace_idx, span_idx, 5)), 16, '0')
        )) AS event_id,
        lower(concat(
            leftPad(hex(cityHash64((trace_idx + 1) % intDiv({rows:UInt64}, 10), 1)), 16, '0'),
            leftPad(hex(cityHash64((trace_idx + 1) % intDiv({rows:UInt64}, 10), 2)), 16, '0')
        )) AS link_trace_id,
        lower(leftPad(hex(cityHash64((trace_idx + 1) % intDiv({rows:UInt64}, 10), 1, 3)), 16, '0')) AS link_span_id,
        toDateTime64(
            {base:DateTime64(9)}
            - toIntervalSecond(86400)
            + toIntervalSecond(intDiv(trace_idx * 86400, intDiv({rows:UInt64}, 10)))
            + toIntervalMillisecond((number % 10) * 10),
            9,
            'UTC'
        ) AS ts
    FROM numbers({offset:UInt64}, {count:UInt64})
);
