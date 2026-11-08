# OpenAPI changelog

All notable changes to the `/api/v1` OpenAPI contract are recorded here. The
`info.version` in `openapi.json` must match a heading in this file, and the CI
contract test (`go test ./internal/api`) fails when it does not. The review
process for breaking changes is in [`docs/api/versioning.md`](../../docs/api/versioning.md).

## 1.0.0 — 2026-11-08

- Initial MVP contract (Story 5.5).
- `GET /healthz` and `GET /api/v1/health`: liveness with the build version.
- `GET /api/v1/health/overview`: ingestion lag, query error rate, storage usage
  and component health for the self-observability page.
- `GET /openapi.json`: generated OpenAPI 3.1 document.
- `GET /api/v1/projects`: project metadata scoped to the bearer token.
- `GET /api/v1/projects/{projectID}/ingest-tokens`: token metadata (never the
  digest) for the token's project.
- `GET /api/v1/query/sample`: deterministic sample result marked `sample=true`
  until the Query Service lands in Phase 1.
- `POST /api/v1/query`: reserved operation that returns a structured 501.
- Errors use the AD-15 envelope `{code, message, details, request_id}` and carry
  the `X-Request-Id` response header.
