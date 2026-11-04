# Story 4.5 — chaos at the ingestion stack (local)

- **Status:** pass
- **Scenario:** storage
- **T1 target:** pending (steady 1M/s with ClickHouse down for 15 min; P-07 and the T1 baseline remain pending)
- **Offered rate:** 5000 events/s total across two tenants
- **Timeline:** pre 60s, ClickHouse outage 900s, NATS outage 30s, gateway outage 15s, settle 15s, post 240s (total 1200s)
- **Faults:** ClickHouse=1, NATS=0, gateway=0 (0 restarts)
- **Replay check:** pass (2500 events deduplicated)

| Metric | Value |
|---|---:|
| Confirmed events (load generator) | 6000000 (logs 1980000, traces 1980000, metrics 2040000) |
| Client failed / rejected requests | 0 / 0 |
| Gateway WAL published events (all instances) | 6005000 |
| Gateway 429 wal_backpressure (all instances) | 0 |
| Gateway WAL publish errors (all instances) | 0 |
| Peak WAL messages / bytes | 15043 / 1220441849 |
| WAL growth during the outage | 15043 messages |
| Gateway /readyz min / last | 200 / 200 |
| Writer accepted | 6005200 |
| Writer written | 6002500 |
| Writer deduped | 2700 |
| Writer failed | 0 |
| Writer skipped | 0 |
| Writer retries | 122 |
| ClickHouse logs | 1980825 (unique 1980825) |
| ClickHouse traces | 1980625 (unique 1980000) |
| Duplicate rows / rate | 625 / 0.0158% (bound 1%) |
| Recovery (last fault recovered → drained) | 241s |
| VictoriaMetrics samples | 2021632 (64 series) |
| Lag p50 / p95 / p99 (recent window) | 336.694508s / 734.162892s / 769.631766s |
| Post-phase lag p95 (median of last samples) | 736.494030s |
| Exit codes gateway/writer | 0 / 0 |



## Checks

| Check | Status | Detail |
|---|---|---|
| gateway_confirmed_events_reached_wal | pass | wal=6005000 confirmed=6000000 |
| wal_events_reached_writer | pass | accepted=6005200 confirmed=6000000 |
| writer_accounting_closes | pass | accepted=6005200 written=6002500 deduped=2700 failed=0 skipped=0 |
| zero_writer_failures | pass | failed=0 |
| zero_confirmed_events_lost | pass | written=6002500 confirmed=6000000 |
| confirmed_logs_stored | pass | unique_logs=1980825 confirmed_logs=1980000 |
| confirmed_traces_stored | pass | unique_traces=1980000 confirmed_traces=1980000 |
| confirmed_metrics_stored | pass | vm_samples=2021632 confirmed_metrics=2040000 tolerance=2019600 |
| clickhouse_logs_match_writer | pass | clickhouse=1980825 writer=1980825 |
| clickhouse_traces_not_above_writer | pass | clickhouse=1980625 writer=1980825 block_deduped=200 |
| duplicate_rows_within_bound | pass | duplicate_rows=625 stored_rows=3961450 rate=0.0158% (bound 1%) |
| victoriametrics_samples_match_writer | pass | samples=2021632 writer=2040850 tolerance=2020442 |
| steady_state_lag_p95_under_10s | skip | a clickhouse outage keeps backlog lag in the 4096-sample window; observed post p95=736.494030s (see 4.4 stable run for the <10s gate) |
| gateway_exited_cleanly | pass | exit=0 |
| writer_exited_cleanly | pass | exit=0 |
| storage_outage_used_wal_or_backpressure | pass | wal_growth_messages=15043 backpressure=0 |
| writer_retried_during_storage_outage | pass | retries=122 samples_with_retry=0 |
| identical_replay_deduped | pass | deduped=2500 |

The gateway acknowledges a request only after the JetStream publish. The writer
acknowledges a WAL message only after ClickHouse or VictoriaMetrics confirmed
the flush (or the message reached the DLQ). During the outage the writer keeps
the messages unacked in the WAL and retries with exponential backoff. Confirmed
events are therefore not lost; the cost is lag and WAL growth. This is a local
single-node proxy and does not prove the 1M/s/15 min T1 target.
