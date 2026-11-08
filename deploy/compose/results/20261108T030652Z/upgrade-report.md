# Story 5.4 — upgrade/rollback drill (AC2)

- Date (UTC): `20261108T030652Z`
- Host: `Linux 7.0.0-42-generic x86_64`
- Docker: `29.9.0`; Compose: `5.6.0`
- Images: `observaki:5.4-drill-a` (VERSION=5.4-drill-a) and `observaki:5.4-drill-b` (VERSION=5.4-drill-b)

## Procedure

```bash
OBSERVAKI_IMAGE=observaki:5.4-drill-a docker compose up -d --wait --no-build
OBSERVAKI_IMAGE=observaki:5.4-drill-b docker compose up -d --wait --no-build   # upgrade
OBSERVAKI_IMAGE=observaki:5.4-drill-a docker compose up -d --wait --no-build   # rollback
```

## Results

| Step | Time | API version | Metadata | Logs (project-dev) |
|---|---|---|---|---|
| Start A | 13s | 5.4-drill-a | `1|1|1|1|Development overview` | 22 |
| Upgrade to B | 13s | 5.4-drill-b | `1|1|1|1|Development overview` | 22 |
| Rollback to A | 14s | 5.4-drill-a | `1|1|1|1|Development overview` | 22 |

- Metadata format: `users|projects|dashboards|alert_rules|dashboard-title`.
- Rollback limit: 900s (AC2: 15 min). Observed: **14s**.
- The PostgreSQL and ClickHouse migration jobs re-ran on each image change and stayed idempotent.

## Notes

- This is a single-node Compose drill; the Helm equivalent (install/upgrade/rollback on kind) is documented in `docs/runbooks/upgrade-rollback.md` and was measured in Story 5.2 (22s/17s).
- The drill leaves the stack running on version A.
- Not G1 evidence.
