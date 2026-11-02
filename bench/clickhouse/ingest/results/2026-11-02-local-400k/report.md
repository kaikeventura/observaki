# Story 3.3 — ClickHouse ingestion path (local)

**Status:** `pass` · **400000/s for 60s** · 12 generator workers · 4 insert threads

| Check | Result |
|---|---:|
| Sustained throughput | 399994 events/s (target ≥ 380000) |
| Lag p95 (oldest event in batch) | 1403 ms (target < 10000 ms) |
| Confirmed events | 24000000 |
| Stored rows | 24000000 |
| Unique `event_id` | 24000000 |
| Active parts at end | 18 |
| Compression (logs) | 56.31x |
| Insert query errors | 0 |
| `Too many parts` errors | 0 |

## Contracts

- **Dedupe (AC2):** the same deterministic batch was submitted twice; the writer dropped the second batch and the stored count stayed at 50,000. The crash/replay test also reuses ClickHouse's `insert_deduplication_token`, so a block that was committed before the checkpoint is not stored twice.
- **Recovery (AC3):** the writer was killed after a successful ClickHouse insert but before the checkpoint advanced. On restart it replayed the spooled batch; the stored count before and after recovery was 100,000.
- **Throughput (AC1):** no `Too many parts` error and lag p95 below 10 s in this local run. The 1M/s / 1-hour T1 run is **not** executed; it needs P-07 (provider/budget/IAM) plus the gateway/WAL path (Stories 4.1/4.3).

Artifacts: `load.json`, `dedupe.json`, `crash.json`, `recovery.json`, `clickhouse.json`, `parts.tsv`, `compression.tsv`, `run.log`.
