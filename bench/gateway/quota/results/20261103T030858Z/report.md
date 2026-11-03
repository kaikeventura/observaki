# Story 4.2 — gateway quota isolation (local)

- **Status:** pass
- **Isolation (>10% local check):** pass
- **Duration per phase:** 5s
- **Noisy quota:** 10000 events/s
- **Quiet tenant:** 1000 events/s
- **Noisy tenant offered:** 100000 events/s
- **Quiet mix:** 0,100,0 (logs only by default, so p95 is comparable)
- **Noisy mix:** 60,30,10
- **Compression:** quiet gzip, noisy none
- **Payload:** metric 832 B, log 820 B, trace 2400 B
- **Pinned:** 0 (gateway=none, quiet=none, noisy=none)

| Metric | Value |
|---|---|
| Quiet client p95 alone (observed) | 3.429314 ms |
| Quiet client p95 under noisy tenant (observed) | 3.344248 ms |
| Client p95 change (observed) | -2.48% |
| Quiet gateway processing p95 alone (AC metric) | 1.643000 ms |
| Quiet gateway processing p95 under noisy tenant (AC metric) | 1.727000 ms |
| Gateway processing p95 change (AC metric) | 5.11% (limit 10%) |
| Quota-rejected requests | 8801 |







This is a local single-process run with the discard sink. It validates the
admission and isolation behavior, not the T1 baseline (P-07 and Stories
4.1/4.3/4.4).
