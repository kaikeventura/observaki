# Story 3.1 — ClickHouse logs storage spike

**Verdict: PASS** — compression >= 5x, q03 p95 < 1000 ms and q03 returned rows.

This is a single-node local spike, not the T1 cluster and not Gate G1 evidence.
The report records the exact commit, schema hash, hardware and limitations.

## Environment

| Field | Value |
|---|---|
| Commit | `494391a` (dirty: false) |
| Host | kabuntu-zenbook, 22 CPUs, 15.0 GiB RAM |
| Disk | 441.1 GiB total, 42.5 GiB free |
| ClickHouse | 26.8.36.3 (`clickhouse/clickhouse-server:26.8`) |
| Schema | `0001_otel_logs.sql` sha256 `2daca91e22cdbfe3cfb367fb8f8241e9839464653f2a0125f3352c35e7197e05` |

## Dataset

| Field | Value |
|---|---:|
| Rows | 1000000000 |
| Chunks | 20 × 50000000 rows |
| Event-time window | 24 h ending 2026-11-02 18:07:19 |
| Body average | 165.3 B |
| Dimensions | 2 tenants × 4 projects × 8 services; 90% INFO / 7% WARN / 3% ERROR |
| Retention | 7 and 15 days per row |

## Compression

| Metric | Value |
|---|---:|
| Compressed | 77.55 GiB |
| Uncompressed | 541.09 GiB |
| Ratio | **6.98x** (target ≥ 5x) |
| Compressed bytes/row | 83.3 B |
| Active parts | 1921 |

## q03 — filtered log search (1h window)

```sql
-- q03 — Filtered log search by service and severity (1h window).
-- Catalog: bench/queries/dashboard.tsv (q03, Explore/Logs, sql, 1h).
--
-- Parameters: tenant, project, service, severity, start, end.
-- The ORDER BY matches the table sort key, so a 1h window for one service
-- reads a bounded slice instead of the whole partition.
SELECT timestamp, severity, service_name, body
FROM otel_logs
WHERE tenant_id = {tenant:String}
  AND project_id = {project:String}
  AND service_name = {service:String}
  AND severity = {severity:String}
  AND timestamp >= {start:DateTime64(9)}
  AND timestamp < {end:DateTime64(9)}
ORDER BY timestamp DESC
LIMIT 100;
```

| Metric | Value |
|---|---:|
| Runs (after warm-up) | 20 |
| p50 | 248.841 ms |
| p95 | 262.983 ms (target < 1000 ms) |
| p99 | 298.510 ms |
| Rows read (mean) | 21282816 |
| Bytes read (mean) | 426995180 |
| Rows returned (mean) | 100 |

## TTL check

| Bucket | Inserted | Remaining | Expected |
|---|---:|---:|---:|
| 7-day, 8 days old | 1 | 0 | 0 |
| 7-day, 1 day old | 1 | 1 | 1 |
| 15-day, 16 days old | 1 | 0 | 0 |
| 15-day, 8 days old | 1 | 1 | 1 |
| 15-day, 1 day old | 1 | 1 | 1 |

The 15-day/8-day-old row is the bucket boundary test: it must survive because
only the 7-day policy expires at 8 days. The expired rows are removed by the
TTL merge, not by the query.

## Limitations

- single-node local ClickHouse, not the T1 cluster (Story 2.4 profile requires P-07)
- the 1B dataset is generated with ClickHouse SQL, not through the OTLP gateway (Story 4.1/3.3)
- q03 uses the last hour of the synthetic 24h window; the OS page cache is warm after the warm-up run
- the local host is smaller than the query-1b scenario reference (16 vCPU/64 GB); the report records the real hardware
- no replication, NATS, Query Service, row policies or chaos; those are Stories 3.3/3.5/4.5
- background merges are stopped for the main table during the local load to bound disk use; the table is measured with unmerged parts

## Reproduction

```bash
./bench/clickhouse/run.sh --rows 1000000000 --chunk-rows 50000000 --runs 20 --stop-merges --base "2026-11-02 18:07:19"
```
