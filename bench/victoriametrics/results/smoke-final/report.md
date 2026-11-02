# Story 3.4 — VictoriaMetrics, MetricsQL and cardinality (local)

**Status:** `pass` · **100000 series at 100000/s for 10s** · 8 workers · 5000 series/request

| Check | Result |
|---|---:|
| Sustained throughput | 99927 samples/s (target ≥ 95000) |
| Sent samples | 1000000 |
| Load errors / rejected | 0 / 0 |
| Load latency p95 | 62.7 ms |
| Active series after load | 100006 (target 100000) |
| Active series after cardinality | 150006 |
| VM RSS max / mean | 0.2 GiB / 0.1 GiB |
| Data size | 0.0 GiB |
| Rows inserted (remote write) | 2494526 |
| 20 queries p50 / p95 / p99 | 8.1 / **15.7** / 15.9 ms |
| Query errors | 0 |
| Slowest query p95 (m14) | 15.9 ms |
| Correctness checks | pass |
| Tenant-a rejected series | 10000 (limit 20000) |
| Tenant-b rejected series | 0 (limit 60000) |
| Neighbor p95 baseline / during | 284.7 / 305.6 ms |
| Neighbor drift | 7.32% (target < 10%) |

## Contracts

- **Ingestion (AC1):** 100000 active series, 100000 samples/s for 10s. The load generator uses Prometheus remote write and reports the achieved rate, request latency and errors.
- **Query (AC2):** 20 representative MetricsQL queries over 24h windows with `nocache=1`; p95 target < 1000 ms. Correctness is checked separately for `rate`, `increase`, `histogram_quantile` and label aggregation on a deterministic dataset.
- **Cardinality (AC3):** a per-tenant admission proxy enforces the series limit before forwarding to VictoriaMetrics. Tenant-a exceeds its limit and receives `429 cardinality_limit_exceeded` plus a rejection metric; tenant-b stays under its limit and its query p95 drift is measured against the baseline.

Artifacts: `load.json`, `queries.json`, `verify.json`, `cardinality-stats.json`, `cardinality-metrics.txt`, `neighbor-baseline.json`, `neighbor-during.json`, `vm.json`, `resources.tsv`, `run.log`.
