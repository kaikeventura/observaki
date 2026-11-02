# Story 3.2 — ClickHouse traces storage spike

**Verdict: PASS** — compression >= 5x, trace lookup p95 < 1000 ms, service+status p95 < 2000 ms, service map and correlation pass.

This is a single-node local spike, not the T1 cluster and not Gate G1 evidence.
The report records the exact commit, schema hash, hardware and limitations.

## Environment

| Field | Value |
|---|---|
| Commit | `f6d6c41` (dirty: false) |
| Host | kabuntu-zenbook, 22 CPUs, 15.0 GiB RAM |
| Disk | 441.1 GiB total, 66.6 GiB free |
| ClickHouse | 26.8.36.3 (`clickhouse/clickhouse-server:26.8`) |
| Schema | `0002_otel_traces.sql` sha256 `ad5fe2074531bceeed37a01dadb787215931f1e99994923b6a29f3fc5faaeab3` |

## Dataset

| Field | Value |
|---|---:|
| Spans | 1000000000 |
| Traces | 100000000 (10 spans each) |
| Chunks | 20 × 50000000 spans |
| Event-time window | 24 h ending 2026-11-02 18:00:00 |
| Dimensions | 2 tenants × 4 projects × 9 services × 10 operations |
| Status mix | 97% OK / 3% ERROR on non-root spans; 99% OK / 1% ERROR on roots |
| Events / links | 10% of spans have one event; 5% have one link |
| Retention | 7 and 15 days per row |
| IDs | CityHash64, random-looking and deterministic |

## Compression

| Metric | Value |
|---|---:|
| Compressed | 56.38 GiB |
| Uncompressed | 423.98 GiB |
| Ratio | **7.52x** (target ≥ 5x) |
| Compressed bytes/span | 60.5 B |
| Active parts | 1701 |

## Trace lookup — q_trace_by_id

```sql
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
```

| Metric | Value |
|---|---:|
| Runs (after warm-up) | 20 |
| p50 | 308.595 ms |
| p95 | 328.211 ms (target < 1000 ms) |
| p99 | 332.394 ms |
| Rows read (mean) | 6430720 |
| Bytes read (mean) | 238158732 |
| Spans returned (mean) | 10 |
| Trace | `80d31d3a6bc766b73806cb2d982d1548` (tenant-0/project-0) |

## Service + status + 24h — q_service_status_24h

```sql
-- Recent spans for one service and status in a 24h window. The primary key
-- (tenant, project, service, status, timestamp) lets ClickHouse read the
-- newest granules of the requested service/status slice instead of scanning
-- the whole service.
SELECT
    timestamp,
    trace_id,
    span_id,
    service_name,
    operation_name,
    duration_ns,
    status_code,
    status_message
FROM otel_traces
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND status_code = {status:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
ORDER BY timestamp DESC
LIMIT 100;
```

| Metric | Value |
|---|---:|
| Runs (after warm-up) | 20 |
| p50 | 169.854 ms |
| p95 | 177.826 ms (target < 2000 ms) |
| p99 | 180.500 ms |
| Rows read (mean) | 13935188 |
| Bytes read (mean) | 279060732 |
| Spans returned (mean) | 100 |
| Filter | `checkout` + `ERROR` in the 24h window ending 2026-11-02 18:00:00 |

## Service map

Window: 60 minutes ending 2026-11-02 18:00:00. Edges use a 1%
deterministic trace sample and scale the counts by 100; the report labels them
as estimates. The map is derived from raw spans for spike validation only;
production RED and the production service map come from the gateway and
VictoriaMetrics (AD-21).

### Nodes

| Service | Spans | Errors | Avg ms | p95 ms |
|---|---:|---:|---:|---:|
| inventory | 2083332 | 62403 | 250.913 | 477.502 |
| frontend | 1041666 | 10462 | 2000.663 | 2901.181 |
| auth | 1041666 | 31350 | 251.003 | 475.996 |
| fraud | 1041666 | 30843 | 250.808 | 476.645 |
| email | 1041666 | 31415 | 251.041 | 475.901 |
| shipping | 1041666 | 30978 | 250.736 | 477.088 |
| checkout | 1041666 | 31111 | 250.869 | 476.784 |
| search | 1041666 | 31098 | 251.033 | 478.544 |
| payments | 1041666 | 30928 | 251.018 | 475.983 |

### Edges (estimated)

| Caller | Callee | Spans | Errors |
|---|---|---:|---:|
| payments | email | 1054400 | 34400 |
| inventory | fraud | 1054400 | 33000 |
| frontend | checkout | 1054400 | 33600 |
| checkout | payments | 1054400 | 31200 |
| frontend | shipping | 1054400 | 32200 |
| shipping | inventory | 1054400 | 31800 |
| frontend | search | 1054400 | 33700 |
| frontend | auth | 1054400 | 32600 |
| checkout | inventory | 1054400 | 30900 |

## Correlation

| Direction | Result |
|---|---|
| Log → trace | true (spans: 10) |
| Trace → log | true (logs: 1) |

The log row stores the trace's `trace_id` and the root `span_id`; the trace
query returns all ten spans, including that root span.

## AD-21 check

```json
{"span_metrics_tables":0,"materialized_views":0,"red_in_clickhouse":false}
```

No `span_metrics` table, RED table or materialized view exists in ClickHouse.
RED stays in VictoriaMetrics (AD-21).

## Limitations

- single-node local ClickHouse, not the T1 cluster (Story 2.4 profile requires P-07)
- the 1B-span dataset is generated with ClickHouse SQL, not through the OTLP gateway (Story 4.1/3.3)
- the trace-ID lookup uses one known trace; the OS page cache is warm after the warm-up run
- the service map is derived from raw spans with a 1% deterministic trace sample; production RED and the production service map come from the gateway and VictoriaMetrics (AD-21)
- the local host is smaller than the query-1b scenario reference (16 vCPU/64 GB); the report records the real hardware
- no replication, NATS, Query Service, row policies or chaos; those are Stories 3.3/3.5/4.5
- background merges are stopped for the traces table during the local load to bound disk use; the table is measured with unmerged parts

## Reproduction

```bash
./bench/clickhouse/traces/run.sh --rows 1000000000 --chunk-rows 50000000 --runs 20 --trace-index 50000000 --map-window-minutes 60 --stop-merges --base "2026-11-02 18:00:00"
```
