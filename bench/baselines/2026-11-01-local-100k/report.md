# Observaki benchmark report

Generated at 2026-11-02T01:52:08Z by benchctl v0.0.1-alpha-17-g5b6096d (commit 5b6096d, built 2026-11-02T01:49:04Z).

This report includes every run that was selected, including failed and blocked runs. Comparisons only use `status: ok`, `mode: run` runs with the same scenario and config hash.

## Environment

| Field | Value |
|---|---|
| Host | kabuntu-zenbook |
| OS / kernel | linux/amd64 7.0.0-38-generic |
| CPU | Intel(R) Core(TM) Ultra 7 155H (22 logical CPUs) |
| Memory | 15.0 GiB |
| Go | go1.27.2 |

## Execuções

| Scenario | Status | Mode | Events/s | Bytes/s | Send p95 | CPU mean | RAM mean | Compression | Error rate | Dropped |
|---|---|---|---|---|---|---|---|---|---|---|
| throughput-100k | ok | run | 99932.3 | 109.0 MiB/s | 38.66 ms | 9.6% | 30.9% | 6.70x (wire) | 0.000% | 0 |
| throughput-100k | ok | run | 99934.1 | 109.0 MiB/s | 39.19 ms | 10.1% | 30.5% | 6.70x (wire) | 0.000% | 0 |
| throughput-100k | ok | run | 99939.0 | 109.0 MiB/s | 39.21 ms | 10.1% | 30.6% | 6.70x (wire) | 0.000% | 0 |

## Detalhe por execução

### throughput-100k — ok

- Result: `/tmp/opencode/bench-results-story24-final-20261102T014908Z/20261102T014908Z-throughput-100k`
- Commit: `5b6096d`; config hash: `f6b7069c2aab`
- Started: 2026-11-02T01:49:08Z; duration: 60.08s
- Hardware: Intel(R) Core(TM) Ultra 7 155H, 22 CPUs, 15.0 GiB RAM, 441.1 GiB disk
- Versions: benchctl v0.0.1-alpha-17-g5b6096d, loadgen v0.0.1-alpha-17-g5b6096d, Go go1.27.2, linux/amd64 7.0.0-38-generic

- Endpoint: `http://127.0.0.1:4318`; duration override: 1m0s; rate override: 100000

| Metric | Value |
|---|---|
| Events/s | 99932.3 (target 100000, delta -0.07%) |
| Bytes/s | 109.0 MiB/s |
| Wire bytes/s | 16.3 MiB/s |
| Send latency p50/p95/p99 | 11.50 / 38.66 / 42.25 ms (6000 samples, 1 shard(s), max across load generator shards) |
| Ingest lag p50/p95/p99 | not collected — requires gateway/WAL ingest metrics (Story 4.3) |
| Query latency p50/p95/p99 | not collected — not declared by this scenario |
| CPU mean/p95/max | 9.64% / 10.88% / 12.31% |
| RAM mean/max | 4.6 GiB (30.95%) / 31.91% |
| Disk free start/end | 147.0 GiB / 147.0 GiB (total 441.1 GiB) |
| Loadgen child CPU/RSS | 6.96% / 59.9 MiB |
| Compression ratio | 6.70x (wire) |
| Wire compression | 6.70x (metrics, logs, traces) |
| Payload compression | 8.65x, 132.3 gzip B/event |
| Error rate | 0.000% |
| Dropped events | 0 |
| Confirmed lost events | not collected — requires gateway/WAL durability accounting (Stories 4.3-4.5) |
| Backpressure events | 0 |
| Pacing lag events | 0 |

**Metric coverage declared by the scenario:**

| Metric | Status | Detail |
|---|---|---|
| events_per_second | collected |  |
| bytes_per_second | collected |  |
| ingest_lag_p50_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| ingest_lag_p95_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| ingest_lag_p99_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| error_rate | collected |  |
| dropped_events | collected |  |
| confirmed_lost_events | not_collected | requires gateway/WAL durability accounting (Stories 4.3-4.5) |
| cpu_percent | collected |  |
| ram_bytes | collected |  |
| disk_bytes | collected |  |
| compression_ratio | collected | source: wire (storage compression lands with Story 3.1-3.3) |
| loadgen_backpressure_events | collected |  |
| loadgen_pacing_lag_events | collected |  |

### throughput-100k — ok

- Result: `/tmp/opencode/bench-results-story24-final-20261102T014908Z/20261102T015008Z-throughput-100k`
- Commit: `5b6096d`; config hash: `f6b7069c2aab`
- Started: 2026-11-02T01:50:08Z; duration: 60.11s
- Hardware: Intel(R) Core(TM) Ultra 7 155H, 22 CPUs, 15.0 GiB RAM, 441.1 GiB disk
- Versions: benchctl v0.0.1-alpha-17-g5b6096d, loadgen v0.0.1-alpha-17-g5b6096d, Go go1.27.2, linux/amd64 7.0.0-38-generic

- Endpoint: `http://127.0.0.1:4318`; duration override: 1m0s; rate override: 100000

| Metric | Value |
|---|---|
| Events/s | 99934.1 (target 100000, delta -0.07%) |
| Bytes/s | 109.0 MiB/s |
| Wire bytes/s | 16.3 MiB/s |
| Send latency p50/p95/p99 | 12.20 / 39.19 / 42.98 ms (6000 samples, 1 shard(s), max across load generator shards) |
| Ingest lag p50/p95/p99 | not collected — requires gateway/WAL ingest metrics (Story 4.3) |
| Query latency p50/p95/p99 | not collected — not declared by this scenario |
| CPU mean/p95/max | 10.14% / 10.61% / 13.43% |
| RAM mean/max | 4.6 GiB (30.46%) / 30.68% |
| Disk free start/end | 147.0 GiB / 147.0 GiB (total 441.1 GiB) |
| Loadgen child CPU/RSS | 7.45% / 59.8 MiB |
| Compression ratio | 6.70x (wire) |
| Wire compression | 6.70x (metrics, logs, traces) |
| Payload compression | 8.65x, 132.3 gzip B/event |
| Error rate | 0.000% |
| Dropped events | 0 |
| Confirmed lost events | not collected — requires gateway/WAL durability accounting (Stories 4.3-4.5) |
| Backpressure events | 0 |
| Pacing lag events | 0 |

**Metric coverage declared by the scenario:**

| Metric | Status | Detail |
|---|---|---|
| events_per_second | collected |  |
| bytes_per_second | collected |  |
| ingest_lag_p50_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| ingest_lag_p95_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| ingest_lag_p99_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| error_rate | collected |  |
| dropped_events | collected |  |
| confirmed_lost_events | not_collected | requires gateway/WAL durability accounting (Stories 4.3-4.5) |
| cpu_percent | collected |  |
| ram_bytes | collected |  |
| disk_bytes | collected |  |
| compression_ratio | collected | source: wire (storage compression lands with Story 3.1-3.3) |
| loadgen_backpressure_events | collected |  |
| loadgen_pacing_lag_events | collected |  |

### throughput-100k — ok

- Result: `/tmp/opencode/bench-results-story24-final-20261102T014908Z/20261102T015108Z-throughput-100k`
- Commit: `5b6096d`; config hash: `f6b7069c2aab`
- Started: 2026-11-02T01:51:08Z; duration: 60.10s
- Hardware: Intel(R) Core(TM) Ultra 7 155H, 22 CPUs, 15.0 GiB RAM, 441.1 GiB disk
- Versions: benchctl v0.0.1-alpha-17-g5b6096d, loadgen v0.0.1-alpha-17-g5b6096d, Go go1.27.2, linux/amd64 7.0.0-38-generic

- Endpoint: `http://127.0.0.1:4318`; duration override: 1m0s; rate override: 100000

| Metric | Value |
|---|---|
| Events/s | 99939.0 (target 100000, delta -0.06%) |
| Bytes/s | 109.0 MiB/s |
| Wire bytes/s | 16.3 MiB/s |
| Send latency p50/p95/p99 | 12.09 / 39.21 / 43.01 ms (6000 samples, 1 shard(s), max across load generator shards) |
| Ingest lag p50/p95/p99 | not collected — requires gateway/WAL ingest metrics (Story 4.3) |
| Query latency p50/p95/p99 | not collected — not declared by this scenario |
| CPU mean/p95/max | 10.11% / 10.77% / 12.77% |
| RAM mean/max | 4.6 GiB (30.60%) / 31.02% |
| Disk free start/end | 147.0 GiB / 147.0 GiB (total 441.1 GiB) |
| Loadgen child CPU/RSS | 7.43% / 58.4 MiB |
| Compression ratio | 6.70x (wire) |
| Wire compression | 6.70x (metrics, logs, traces) |
| Payload compression | 8.64x, 132.3 gzip B/event |
| Error rate | 0.000% |
| Dropped events | 0 |
| Confirmed lost events | not collected — requires gateway/WAL durability accounting (Stories 4.3-4.5) |
| Backpressure events | 0 |
| Pacing lag events | 0 |

**Metric coverage declared by the scenario:**

| Metric | Status | Detail |
|---|---|---|
| events_per_second | collected |  |
| bytes_per_second | collected |  |
| ingest_lag_p50_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| ingest_lag_p95_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| ingest_lag_p99_ms | not_collected | requires gateway/WAL ingest metrics (Story 4.3) |
| error_rate | collected |  |
| dropped_events | collected |  |
| confirmed_lost_events | not_collected | requires gateway/WAL durability accounting (Stories 4.3-4.5) |
| cpu_percent | collected |  |
| ram_bytes | collected |  |
| disk_bytes | collected |  |
| compression_ratio | collected | source: wire (storage compression lands with Story 3.1-3.3) |
| loadgen_backpressure_events | collected |  |
| loadgen_pacing_lag_events | collected |  |

## Variância entre repetições

### throughput-100k — `f6b7069c2aab`

- Runs: 3; commit(s): `5b6096d`; same commit: true
- Documented variance: **pass**

| Metric | Runs | Mean | Min | Max | Spread | Tolerance | Assessed | Result |
|---|---|---|---|---|---|---|---|---|
| events_per_second | 3 | 9.994e+04 | 9.993e+04 | 9.994e+04 | 0.01% | 5.0% | true | pass |
| bytes_per_second | 3 | 1.143e+08 | 1.143e+08 | 1.143e+08 | 0.01% | 5.0% | true | pass |
| send_latency_p50_ms | 3 | 11.93 | 11.5 | 12.2 | 5.93% | 10.0% | true | pass |
| send_latency_p95_ms | 3 | 39.02 | 38.66 | 39.21 | 1.41% | 10.0% | true | pass |
| send_latency_p99_ms | 3 | 42.75 | 42.25 | 43.01 | 1.77% | 10.0% | true | pass |
| cpu_percent_mean | 3 | 9.963 | 9.644 | 10.14 | 4.94% | 10.0% | true | pass |
| memory_used_percent_mean | 3 | 30.67 | 30.46 | 30.95 | 1.60% | 10.0% | true | pass |
| compression_ratio | 3 | 6.697 | 6.696 | 6.699 | 0.04% | 5.0% | true | pass |
| error_rate | 3 | 0 | 0 | 0 | 0.00 pp | 0.1 pp | true | pass |

## Methodology

**Workload:** 60% metrics (~40 B), 30% logs (~200 B) and 10% traces (~500 B), weighted average ~134 B/event after gzip; 60/30/10 signal mix.

### Definitions

| Metric | Definition |
|---|---|
| bytes_per_second | Uncompressed protobuf bytes accepted by the receiver divided by the longest shard duration. |
| compression_ratio | Raw protobuf bytes divided by wire bytes when transport compression is measurable; otherwise the deterministic payload ratio from the same seed/config. Wire and payload ratios can differ because the workload interleaves signal indices while the payload calibration uses isolated batches. Storage compression (ClickHouse) lands with Story 3.1-3.3. |
| confirmed_lost_events | Requires gateway/WAL durability accounting (Stories 4.3-4.5); not collected in Phase 0 throughput runs. |
| cpu_percent | Machine-wide CPU busy percentage sampled once per second; 100 means all CPUs busy. |
| disk_bytes | Filesystem total and free bytes on the result path, sampled once per second. |
| dropped_events | Failed plus rejected events reported by the load generator. |
| error_rate | Failed plus rejected events divided by generated events. |
| events_per_second | Events generated divided by the longest load generator shard duration (runner aggregate). |
| ingest_lag_ms | Time between the event timestamp and the durable write. Requires gateway/WAL metrics (Story 4.3); not collected in Phase 0 throughput runs. |
| query_latency_ms | Query Service latency for the 20-query dashboard set. Requires the Query Service (Story 3.5). |
| ram_bytes | Machine memory in use (MemTotal minus MemAvailable), sampled once per second. |
| send_latency_ms | OTLP export request latency measured by the load generator; with multiple shards the report keeps the worst shard's p50/p95/p99. |

### Documented variance

| Metric | Unit | Tolerance | Minimum runs | Note |
|---|---|---|---|---|
| events_per_second | events/s | 5.0% | 2 | ADR-0006 allows one continuous 1h gate run; with fewer than two runs the variance is not assessed. |
| bytes_per_second | B/s | 5.0% | 2 |  |
| send_latency_p50_ms | ms | 10.0% | 3 |  |
| send_latency_p95_ms | ms | 10.0% | 3 |  |
| send_latency_p99_ms | ms | 10.0% | 3 |  |
| cpu_percent_mean | % | 10.0% | 2 |  |
| memory_used_percent_mean | % | 10.0% | 2 |  |
| compression_ratio | x | 5.0% | 2 |  |
| error_rate | percentage points | 0.1 pp | 2 |  |

### Repetition rules

- Latency and error-rate comparisons need at least 3 runs (ADR-0006 rule 10); the report states when the minimum is not met.
- A 1h throughput gate run may be a single continuous execution when stability metrics (backpressure, pacing lag, resource samples) are published.
- Variance is only assessed for runs with the same scenario and config hash. Runs spanning commits are listed but not judged as reruns.
- Every run is listed, including failed and blocked ones; comparisons use only status=ok, mode=run runs.

### Limitations

- Ingest lag p50/p95/p99 and confirmed lost events require the gateway/WAL (Stories 4.3-4.5); Phase 0 throughput runs report generator send latency instead and mark those metrics not collected.
- Query and cardinality latency require the Query Service and the series generator (Stories 3.4-3.5); their scenarios are reported as plans/blocked until then.
- ClickHouse/VictoriaMetrics storage compression is not measured here; compression_ratio is the wire or deterministic payload ratio, labelled by source.
- Resource sampling is host-level on Linux; other platforms report the metrics as unavailable.
- Pricing assumptions are an open founder decision (ADR-0006 Q-07); the report publishes the volume inputs, not a TCO verdict.

## Cost inputs

- Status: `not_configured` — Pricing assumptions are an open founder decision (ADR-0006 Q-07); this report publishes the volume inputs only.
- Events/day: 8634727417
- Raw payload: 9.876 TB/day
- Wire bytes: 1.475 TB/day
- Reference gzip payload: 1.143 TB/day
- Inputs required for the TCO decision:
  - events per day from the highest sustained throughput run
  - raw and wire bytes per day from the same run
  - reference payload gzip bytes per event from the deterministic generator
  - TCO per TB/day, replica factor and operator hours are not priced here

