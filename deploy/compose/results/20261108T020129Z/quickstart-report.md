# Story 5.3 — quickstart check

- Date (UTC): `20261108T020129Z`
- Host: `Linux 7.0.0-42-generic x86_64`
- Docker: `29.9.0`; Compose: `5.6.0`
- Driver: `deploy/compose/quickstart-check.sh` (fresh=1, build=1)

## AC1 — quickstart under 15 minutes

- `docker compose up -d --wait` (build=1): **29s** to all-healthy.
- Sample telemetry accepted and visible in ClickHouse and VictoriaMetrics.
- Total from `up` to verified data: **60s** (limit 900s).
- Result: **pass**.

## Sample output

```
logs: accepted (HTTP 200)
traces: accepted (HTTP 200)
metrics: accepted (HTTP 200)
```

## Notes

- This is the single-node evaluation stack, not a benchmark and not G1 evidence.
- The quickstart doc is `docs/quickstart.md`; the per-language apps are in `examples/`.
