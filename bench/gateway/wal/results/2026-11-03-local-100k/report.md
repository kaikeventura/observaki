# Story 4.3 — gateway WAL (local)

- **Status:** pass
- **T1 target:** pending (1M/s for 1h; P-07 and the T1 baseline remain pending)
- **Duration per run:** 10s
- **Offered rate:** 100000 events/s total across two tenants
- **Batch:** 500 events/request; linger 5ms
- **Runs:** 1
- **Shards per tenant/project/epoch/signal:** 32
- **Mix:** 34,33,33
- **Compression:** gzip

| Metric | Value |
|---|---:|
| Accepted events | 1000000 |
| WAL events | 1000000 |
| Unique event IDs | 1000000 |
| Duplicate event IDs | 0 |
| WAL compression ratio | 5.46x |
| Subjects | 192 |
| Shards used | 32 |
| Largest subject share | 0.95% |
| Gateway p99 processing | 28.95 ms (AC1 target < 50 ms) |
| Client p99 | 54.49 ms |
| Failed / rejected events | 0 / 0 |



The gateway acknowledged only after the JetStream publish ack. `walcheck`
read every message back without acking and confirmed the event count, unique
`event_id` values and shard distribution. This is a local proxy: it does not
prove the T1 throughput target or a 15-minute ClickHouse outage.
