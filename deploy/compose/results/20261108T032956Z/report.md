# Story 5.1 — single-node Compose smoke

- Date (UTC): `20261108T032956Z`
- Host: `Linux 7.0.0-42-generic x86_64`
- Docker: `29.9.0`; Compose: `5.6.0`
- Driver: `deploy/compose/smoke.sh`; sample: `deploy/compose/sample.sh`

## AC1 — startup and health

- `docker compose up -d --wait` (build=1): **31s** to all-healthy (limit 600s).
- Healthy: clickhouse, victoriametrics, postgres, nats, observaki-api, observaki-gateway, observaki-writer; clickhouse-migrate exited 0.
- UI `http://127.0.0.1:8080/` and `/api/v1/health` reachable; gateway `/readyz` and writer `/readyz` return 200.
- Story 5.5: `/openapi.json` is OpenAPI 3.1 and documents projects, ingestion tokens and the sample query; `/api/v1/projects` returns 401 without a token and the seeded token reads the development project without exposing a digest.

## AC2 — sample telemetry

```
logs: accepted (HTTP 200)
traces: accepted (HTTP 200)
metrics: accepted (HTTP 200)
```

- `otel_logs` rows for project-dev: 22 -> 24.
- `otel_traces` rows for project-dev: 22 -> 24.
- VictoriaMetrics: the range query for `observaki_sample_requests_total{project_id="project-dev"}` returns data (the 30s latency offset is awaited).
- Linux validated here; the macOS run is still pending (see the handoff).

## AC3 — lifecycle

- Persistence check: **pass** (down 4s, up 17s).
- After `docker compose down` / `up`, rows: logs=24 traces=24; the metric series is still present.
- `clickhouse-migrate` re-applied the idempotent migrations automatically; no manual step.

## Notes

- This is a single-node evaluation stack, not a benchmark run and not G1 evidence.
- The landing page has no Explore view yet (Stories 6.1-6.4); data is verified in storage and through the API.
