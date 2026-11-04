# Story 4.5 — chaos at the ingestion stack (local)

- **Status:** pass
- **Scenario:** all
- **T1 target:** pending (steady 1M/s with ClickHouse down for 15 min; P-07 and the T1 baseline remain pending)
- **Offered rate:** 5000 events/s total across two tenants
- **Timeline:** pre 60s, ClickHouse outage 180s, NATS outage 30s, gateway outage 15s, settle 15s, post 120s (total 435s)
- **Faults:** ClickHouse=1, NATS=1, gateway=1 (1 restarts)
- **Replay check:** pass (2500 events deduplicated)

| Metric | Value |
|---|---:|
| Confirmed events (load generator) | 1948600 (logs 642950, traces 642950, metrics 662700) |
| Client failed / rejected requests | 226400 / 0 |
| Gateway WAL published events (all instances) | 1953400 |
| Gateway 429 wal_backpressure (all instances) | 0 |
| Gateway WAL publish errors (all instances) | 757 |
| Peak WAL messages / bytes | 3168 / 257047298 |
| WAL growth during the outage | 3168 messages |
| Gateway /readyz min / last | 000 / 200 |
| Writer accepted | 1953600 |
| Writer written | 1950275 |
| Writer deduped | 3325 |
| Writer failed | 0 |
| Writer skipped | 0 |
| Writer retries | 32 |
| ClickHouse logs | 643775 (unique 643775) |
| ClickHouse traces | 642950 (unique 642950) |
| Duplicate rows / rate | 0 / 0.0000% (bound 1%) |
| Recovery (last fault recovered → drained) | 120s |
| VictoriaMetrics samples | 657626 (64 series) |
| Lag p50 / p95 / p99 (recent window) | 45.278514s / 174.489597s / 188.665820s |
| Post-phase lag p95 (median of last samples) | 174.731230s |
| Exit codes gateway/writer | 0 / 0 |



## Checks

| Check | Status | Detail |
|---|---|---|
| gateway_confirmed_events_reached_wal | pass | wal=1953400 confirmed=1948600 |
| wal_events_reached_writer | pass | accepted=1953600 confirmed=1948600 |
| writer_accounting_closes | pass | accepted=1953600 written=1950275 deduped=3325 failed=0 skipped=0 |
| zero_writer_failures | pass | failed=0 |
| zero_confirmed_events_lost | pass | written=1950275 confirmed=1948600 |
| confirmed_logs_stored | pass | unique_logs=643775 confirmed_logs=642950 |
| confirmed_traces_stored | pass | unique_traces=642950 confirmed_traces=642950 |
| confirmed_metrics_stored | pass | vm_samples=657626 confirmed_metrics=662700 tolerance=656073 |
| clickhouse_logs_match_writer | pass | clickhouse=643775 writer=643775 |
| clickhouse_traces_not_above_writer | pass | clickhouse=642950 writer=642950 block_deduped=0 |
| duplicate_rows_within_bound | pass | duplicate_rows=0 stored_rows=1286725 rate=0.0000% (bound 1%) |
| victoriametrics_samples_match_writer | pass | samples=657626 writer=663550 tolerance=656914 |
| steady_state_lag_p95_under_10s | skip | a clickhouse outage keeps backlog lag in the 4096-sample window; observed post p95=174.731230s (see 4.4 stable run for the <10s gate) |
| gateway_exited_cleanly | pass | exit=0 |
| writer_exited_cleanly | pass | exit=0 |
| storage_outage_used_wal_or_backpressure | pass | wal_growth_messages=3168 backpressure=0 |
| writer_retried_during_storage_outage | pass | retries=32 samples_with_retry=0 |
| gateway_saw_nats_outage | pass | non_200_samples=28 |
| gateway_recovered_after_nats | pass | last_readyz=200 |
| gateway_saw_gateway_outage | pass | non_200_samples=14 |
| gateway_restarted | pass | restarts=1 exit=0 |
| gateway_recovered_after_restart | pass | last_readyz=200 |
| identical_replay_deduped | pass | deduped=2500 |

The gateway acknowledges a request only after the JetStream publish. The writer
acknowledges a WAL message only after ClickHouse or VictoriaMetrics confirmed
the flush (or the message reached the DLQ). During the outage the writer keeps
the messages unacked in the WAL and retries with exponential backoff. Confirmed
events are therefore not lost; the cost is lag and WAL growth. This is a local
single-node proxy and does not prove the 1M/s/15 min T1 target.
