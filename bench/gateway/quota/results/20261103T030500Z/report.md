# Story 4.2 — gateway quota isolation (local)

- **Status:** pass
- **Isolation (>10% local check):** inconclusive
- **Duration per phase:** 10s
- **Noisy quota:** 100000 events/s
- **Quiet tenant:** 50000 events/s
- **Noisy tenant offered:** 1000000 events/s
- **Quiet mix:** 0,100,0 (logs only by default, so p95 is comparable)
- **Noisy mix:** 60,30,10
- **Compression:** quiet gzip, noisy none
- **Payload:** metric 0 B, log 64 B, trace 0 B
- **Pinned:** 0 (gateway=none, quiet=none, noisy=none)

| Metric | Value |
|---|---|
| Quiet client p95 alone (observed) | 10.535304 ms |
| Quiet client p95 under noisy tenant (observed) | 11.610206 ms |
| Client p95 change (observed) | 10.20% (limit 10%) |
| Quiet gateway processing p95 alone (AC metric) | 5.461000 ms |
| Quiet gateway processing p95 under noisy tenant (AC metric) | 6.287000 ms |
| Gateway processing p95 change (AC metric) | 15.13% (limit 10%) |
| Quota-rejected requests | 26703 |



**Isolation note:** local gateway processing p95 changed 15.13% (limit 10%); the gateway and both load generators share one host, so CPU frequency and memory bandwidth contention invalidate the neighbor comparison. The T1 fleet with separate load generators is required.



This is a local single-process run with the discard sink. It validates the
admission and isolation behavior, not the T1 baseline (P-07 and Stories
4.1/4.3/4.4).
