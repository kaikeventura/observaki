# Story 5.4 — backup/restore, auto-observability and alerting drill

- Date (UTC): `20261108T030610Z`
- Host: `Linux 7.0.0-42-generic x86_64`
- Docker: `29.9.0`; Compose: `5.6.0`
- Driver: `deploy/compose/operator-smoke.sh` (build=0)

## AC3 — auto-observability

- Health overview: `GET /api/v1/health/overview` returned status **healthy**.
- Components: api=healthy, postgres=healthy, gateway=healthy, writer=healthy, nats=healthy, clickhouse=healthy, victoriametrics=healthy, alerting=healthy.
- Ingestion: lag p95 **0.6170511639999999s**, pending **0**, confirmed **6** events.
- Query error rate: **100%** (1 server errors over 1 requests).
- Storage: ClickHouse **41604 bytes** / **6 parts**; VictoriaMetrics **20459 bytes** / **59 series**.
- Alerting heartbeat: **2026-11-08T03:06:20.243343Z**.
- The health page is served at `/health` (client-rendered SPA; `web/build/health.html`); the vitest suite covers the four AC3 sections and the error state. It polls the endpoint every 10 s and keeps the last known value when a source is down.

## AC1 — backup/restore

- Backup: **0s** to `metadata-backup.json` (format 1); restore: **0s** with `--force`.
- Metadata counts: `users|projects|dashboards|alert_rules|channels|routes|silences|slos|tokens` = `1|1|1|1|1|1|1|1|1` before and `1|1|1|1|1|1|1|1|1` after.
- Project name and dashboard title were restored; the backup contains only the token hash, never the plaintext.
- Limit: 30 min (AC1); the observed restore is well below it.

## AC4 — threshold alert, dedupe and silence

- Threshold rule `Story 5.4 drill` (`ingest_lag_p95_seconds >= 0`, `for=0`) fired and delivered **1** webhook notification; the full dedupe/silence drill took **23s**.
- While the state stayed firing, the notification count stayed at **1** (outbox idempotency key).
- A silence suppressed the resolution and the re-fire; the sink count stayed at **1** and the outbox recorded **2** suppressed rows.
- Payload: `{"query":"ingest_lag_p95_seconds","since":"2026-11-08T03:06:25.242653021Z","state":"firing","value":0.6170511639999999,"labels":{"rule_id":"00000000-0000-4000-8000-0000000000b1","severity":"warning","rule_name":"Story 5.4 drill","project_id":"00000000-0000-4000-8000-000000000002"},"rule_id":"00000000-0000-4000-8000-0000000000b1","summary":"Story 5.4 drill is firing: ingestion lag p95 is 617ms (threshold 0s)","version":1,"severity":"warning","rule_name":"Story 5.4 drill","threshold":0,"project_id":"00000000-0000-4000-8000-000000000002","evaluated_at":"2026-11-08T03:06:25.242653021Z","idempotency_key":"00000000-0000-4000-8000-0000000000b1|00000000-0000-4000-8000-0000000000b1|1|00000000-0000-4000-8000-000000000005"}`

## Notes

- This is a single-node evaluation drill, not G1 evidence.
- AC2 (upgrade/rollback) is measured separately by `deploy/compose/upgrade-drill.sh`; the Helm runbook is in `docs/runbooks/upgrade-rollback.md`.
- The Query Service is Phase 1; the query error rate currently counts the API query class (the `/api/v1/query` placeholder returns 501).
