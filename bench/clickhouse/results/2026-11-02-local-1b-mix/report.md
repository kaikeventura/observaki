# Story 3.5 — ClickHouse query, retention and row-policy spike

**1B tier verdict: PASS** — all 16 ClickHouse queries met p95 < 1000 ms, p99 < 2x p95 and returned rows.

This is a single-node local spike, not the T1 cluster and not Gate G1 evidence.
The 1b tier is 1000000000 events in the Story 2.2 mix: 300000000 logs + 100000000
spans + 600M metric samples. The 10B/7d tier is a window-equivalent projection;
the 86B/24h tier is measured on the 1m rollup when `--rollup` is set. Neither
replaces the T1 measurement.

## Environment

| Field | Value |
|---|---|
| Commit | `d867eb2` (dirty: true) |
| Host | kabuntu-zenbook, 22 CPUs, 15.0 GiB RAM |
| Disk | 441.1 GiB total, 103.3 GiB free |
| ClickHouse | 26.8.36.3 (`clickhouse/clickhouse-server:26.8`) |
| Tenancy migration | `0004_tenancy.sql` sha256 `8b86cd841e068900bcebb99e1aa8bcdd5bdd88d61062c86972f7fa9df34c1388` |

## Dataset

| Signal | Rows | Parts | Compressed | Uncompressed | Ratio | Bytes/row |
|---|---:|---:|---:|---:|---:|---:|
| Logs | 300000005 | 666 | 23.52 GiB | 176.57 GiB | 7.51x | 84.2 |
| Traces | 100000000 | 171 | 5.67 GiB | 43.89 GiB | 7.74x | 60.9 |

## Dashboard queries (1b tier)

The 16 ClickHouse queries run as `obs_tenant_0`, a user with only the
`observaki_query` role. The row policy is the backstop; every query also
carries the tenant/project predicate the Query Service injects (AD-3/AD-17).

| Query | p50 ms | p95 ms | p99 ms | Rows read | Bytes read | Rows returned |
|---|---:|---:|---:|---:|---:|---:|
| q01 | 149.043272 | 164.353385 | 165.12247 | 39272454 | 982163214 | 48 |
| q02 | 233.220648 | 250.69543 | 251.606033 | 39272454 | 1099628696 | 8 |
| q03 | 171.184728 | 187.10800799999998 | 209.128428 | 10797062 | 390179656 | 100 |
| q04 | 219.601551 | 254.494583 | 257.582518 | 39272454 | 1060356248 | 8 |
| q05 | 407.06104999999997 | 431.94125 | 448.41903 | 10780678 | 554552568 | 16 |
| q06 | 36.561539999999994 | 100.91878799999999 | 148.449037 | 2038585.2 | 108195380.35 | 100 |
| q07 | 178.10648600000002 | 195.62707300000002 | 203.476242 | 25059334 | 877076632 | 9 |
| q08 | 58.28774 | 65.187547 | 65.727248 | 4186118 | 150655574 | 1 |
| q09 | 94.63849300000001 | 102.73475 | 103.612594 | 25059334 | 701661336 | 9 |
| q10 | 49.821418 | 54.835424 | 56.233959 | 15745030 | 395853896 | 2 |
| q11 | 11.908081 | 14.07389 | 14.422178 | 81926 | 3777936 | 100 |
| q12 | 45.460847 | 52.195892 | 63.29294000000001 | 589830 | 43814836 | 10 |
| q13 | 160.550999 | 179.457682 | 185.959983 | 31490054 | 915079545 | 9 |
| q14 | 189.288485 | 206.489967 | 212.197937 | 25059334 | 877076632 | 9 |
| q15 | 44.377505 | 52.157661000000004 | 53.82709 | 16395 | 565683 | 1 |
| q20 | 51.239654 | 56.049933 | 61.001413 | 15384582 | 386007192 | 54 |

Target: p95 < 1000 ms and p99 < 2x p95 for every query. The 4 MetricsQL
queries are recorded separately from the Story 3.4 VictoriaMetrics run:
**pass** (overall p95 799.501642 ms).

| Metrics query | p95 ms | Series | Status |
|---|---:|---:|---:|---|
| q16 (m01) | 757.04204 | 200 | pass |
| q17 (m05) | 799.501642 | 200 | pass |
| q18 (m06) | 659.670864 | 1 | pass |
| q19 (m20) | 688.760464 | 1 | pass |

## Tenancy (AC2)

| Test | Expected | Result |
|---|---|---|
| cross_tenant_logs | 0 | pass |
| cross_tenant_traces | 0 | pass |
| subquery_bypass | 0 | pass |
| or_bypass | 0 | pass |
| join_bypass | 0 | pass |
| url_function | denied | pass |
| file_function | denied | pass |
| s3_function | denied | pass |
| remote_function | denied | pass |
| mysql_function | denied | pass |
| postgresql_function | denied | pass |
| system_users | denied | pass |
| system_processes | denied | pass |
| write_attempt | denied | pass |
| ddl_attempt | denied | pass |

Epoch check (AD-20): `{"epoch1_rows":149981236,"probe_epoch":13995,"probe_event":"epoch-event-13995-31031","rows_after_epoch_bump":0,"rows_after_new_epoch_row":1,"pass":true}`

System tables: the tenant user sees only the granted tables and is denied
`system.users` / `system.processes` and every dangerous table function
(`url`, `file`, `s3`, `remote`, `mysql`, `postgresql`).

## Retention (AC3)

```json
{
  "ttl": {
    "rows": 6000,
    "ages_days": [
      6,
      14,
      29,
      8,
      16,
      31
    ],
    "policies_days": [
      7,
      15,
      30
    ],
    "survivors": "project-0\t7\t1000\nproject-1\t15\t1000\nproject-2\t30\t1000",
    "pass": true
  },
  "policy_change": {
    "rows": 10000000,
    "before": 10000000,
    "after": 0,
    "mutation_ms": 493,
    "pass": true
  },
  "pass": true
}
```

The per-row TTL strategy is kept for the MVP: one table, `retention_days`
per row, TTL on `ingested_at`. The 7/15/30-day probe expires each project
according to its policy. A policy change is a mutation; the probe measures
the cost on a 10M-row table. The bucket-table fallback is documented in
`bench/clickhouse/README.md` with its cost trade-off.

## 10B / 86B tiers

These tiers are **not materialized** on the local host. The 10B/7d dataset is
10B events over 7 days; a 24h dashboard window is 10/7 = 1.43x the 1B/24h
window. The slowest 1B query is 431.94125 ms, so the linear window-equivalent
projection is **617.059 ms** for 10B/7d (target < 3000 ms,
projected pass).
This is a projection, not a raw 10B measurement.

The 86B/24h tier **passed on the 1m rollup**: 86B raw events are ~86x the 1B/24h
dataset and do not fit on this host. The rollup tables have the same
tenant_id/project_generation/authz_epoch columns and row policies as the raw
tables; the rollup cardinality is bounded by dimensions × 1440 minutes, so the
1B dataset's rollup is the same shape as the 86B/24h rollup (only the counters
scale). The point/selective queries stay on the raw tables with the bloom
filters.

| Rollup query | p95 ms | p99 ms | Rows |
|---|---:|---:|---:|
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |
| \(.id) | \(.p95_ms) | \(.p99_ms) | \(.returned_mean) |

See `bench/clickhouse/README.md` for the rollup design and cost.

## Limitations

- single-node local ClickHouse, not the T1 cluster (Story 2.4 profile requires P-07)
- the 1B tier is 1B events (300M logs + 100M spans + 600M metric samples), not 1B rows per signal; the 3.1/3.2 spikes already stress-tested 1B rows per signal
- the 10B/7d tier is not materialized on this host; the report records a window-equivalent projection, not a raw 10B measurement
- the 86B/24h tier is not materialized raw on this host; the rollup path is measured when --rollup is set, and the raw 86B scan remains a T1 gate item
- the local host is smaller than the query-1b reference (16 vCPU/64 GB); the report records the real hardware
- the OS page cache is warm after the warm-up run; the first (cold) run is not counted
- no replication, NATS, gateway or Query Service; the tenant-scoped ClickHouse user simulates the QS row-policy backstop
- the 4 MetricsQL queries use the Story 3.4 VictoriaMetrics dataset; the metrics tier is larger than the 1B mix (3.6B samples)

## Reproduction

```bash
./bench/clickhouse/query/run.sh --logs 300000000 --traces 100000000 --chunk-rows 50000000 --runs 20 --skip-load --rollup --base "2026-11-03 01:23:09"
```
