# Story 4.4 — writers over the NATS WAL (local)

- **Status:** pass
- **T1 target:** pending (1M/s for 1h; P-07 and the T1 baseline remain pending)
- **Duration per run:** 20s
- **Offered rate:** 20000 events/s total across two tenants
- **Runs:** 1
- **Mix:** 34,33,33
- **Compression:** gzip
- **Chaos:** 0 (ClickHouse 3s, VictoriaMetrics 2s)
- **Dedupe phase:** pass (10000 events deduplicated)

| Metric | Value |
|---|---:|
| Load generator sent (measured runs) | 400000 |
| Gateway WAL events (measured runs) | 400000 |
| Writer accepted (measured runs) | 400000 |
| Writer accepted (all phases) | 420000 |
| Writer written | 406700 |
| Writer deduped | 13300 |
| Writer failed | 0 |
| Writer skipped | 0 |
| Writer retries | 0 |
| ClickHouse logs | 135300 (unique 135300) |
| ClickHouse traces | 132000 (unique 132000) |
| VictoriaMetrics samples | 139400 (64 series) |
| Lag p50 / p95 / p99 | 0.514624s / 0.801832s / 0.886795s |
| Exit codes gateway/writer | 0 / 0 |



The gateway acknowledged only after the JetStream publish. The writer
acknowledged each WAL message only after ClickHouse or VictoriaMetrics
confirmed the flush. During the chaos window the writer retried with
exponential backoff and left the messages unacked in the WAL. This is a local
proxy: it does not prove the T1 throughput target or the 15-minute outage in
Story 4.5.
