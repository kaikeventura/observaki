# Story 3.4 — VictoriaMetrics, MetricsQL and cardinality (local)

**Status:** `pass` · **1000000 series at 1000000/s for 1h** · 16 workers · 5000 series/request

| Check | Result |
|---|---:|
| Sustained throughput | 999738 samples/s (target ≥ 950000) |
| Sent samples | 3600000000 |
| Load errors / rejected | 0 / 0 |
| Load latency p95 | 13.1 ms |
| Active series after load | 1000006 (target 1000000) |
| Active series after cardinality | 1200006 |
| VM RSS max / mean | 1.2 GiB / 1.0 GiB |
| Data size | 15.3 GiB |
| Rows inserted (remote write) | 3603199726 |
| 20 queries p50 / p95 / p99 | 573.9 / **799.5** / 1216.7 ms |
| Query errors | 0 |
| Slowest query p95 (m14, operator view) | 1305.2 ms |
| Correctness checks | pass |
| Tenant-a rejected series | 100000 (limit 150000) |
| Tenant-b rejected series | 0 (limit 100000) |
| Neighbor p95 baseline / during | 584.5 / 612.6 ms |
| Neighbor drift | 4.82% (target < 10%) |

## Contracts

- **Ingestion (AC1):** 1000000 active series, 1000000 samples/s for 1h. The load generator uses Prometheus remote write and reports the achieved rate, request latency and errors.
- **Query (AC2):** 20 representative MetricsQL queries over 24h windows with `nocache=1`; p95 target < 1000 ms. Correctness is checked separately for `rate`, `increase`, `histogram_quantile` and label aggregation on a deterministic dataset.
- **Cardinality (AC3):** a per-tenant admission proxy enforces the series limit before forwarding to VictoriaMetrics. Tenant-a exceeds its limit and receives `429 cardinality_limit_exceeded` plus a rejection metric; tenant-b stays under its limit and its query p95 drift is measured against the baseline.

Artifacts: `load.json`, `queries.json`, `verify.json`, `cardinality-stats.json`, `cardinality-metrics.txt`, `neighbor-baseline.json`, `neighbor-during.json`, `vm.json`, `resources.tsv`, `run.log`.
