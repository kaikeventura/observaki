# Story 4.4 — writers over the NATS WAL (local)

- **Status:** pass
- **T1 target:** pending (1M/s for 1h; P-07 and the T1 baseline remain pending)
- **Duration per run:** 5s
- **Offered rate:** 5000 events/s total across two tenants
- **Runs:** 1
- **Mix:** 34,33,33
- **Compression:** gzip
- **Chaos:** 1 (ClickHouse 3s, VictoriaMetrics 2s)
- **Dedupe phase:** pass (2500 events deduplicated)

| Metric | Value |
|---|---:|
| Load generator sent (measured runs) | 25000 |
| Gateway WAL events (measured runs) | 25000 |
| Writer accepted (measured runs) | 25000 |
| Writer accepted (all phases) | 30000 |
| Writer written | 26675 |
| Writer deduped | 3325 |
| Writer failed | 0 |
| Writer skipped | 0 |
| Writer retries | 8 |
| ClickHouse logs | 9075 (unique 9075) |
| ClickHouse traces | 8250 (unique 8250) |
| VictoriaMetrics samples | 9350 (64 series) |
| Lag p50 / p95 / p99 | 6.140027s / 7.966322s / 8.088456s |
| Exit codes gateway/writer | 0 / 0 |



The gateway acknowledged only after the JetStream publish. The writer
acknowledged each WAL message only after ClickHouse or VictoriaMetrics
confirmed the flush. During the chaos window the writer retried with
exponential backoff and left the messages unacked in the WAL. This is a local
proxy: it does not prove the T1 throughput target or the 15-minute outage in
Story 4.5.
