# Observaki API

The HTTP API is the stable surface for the CLI, the UI and future integrations.
It is versioned under `/api/v1`, its contract is OpenAPI 3.1, and every response
carries a request id.

## Endpoints

| Method | Path | Auth scope | Purpose |
|---|---|---|---|
| `GET` | `/healthz` | public | Liveness. No data-plane dependency. |
| `GET` | `/api/v1/health` | public | Liveness plus the build version. |
| `GET` | `/api/v1/health/overview` | public | Ingestion lag, query error rate, storage usage and component health (AD-24). |
| `GET` | `/openapi.json` | public | Generated OpenAPI 3.1 contract. |
| `GET` | `/api/v1/projects` | `query` | Project metadata visible to the token. |
| `GET` | `/api/v1/projects/{projectID}/ingest-tokens` | `admin` | Token name, scopes and expiry. The digest is never returned (AD-19). |
| `GET` | `/api/v1/query/sample?signal=metrics\|logs\|traces` | `query` | Deterministic sample marked `sample=true`; real telemetry arrives with the Query Service in Phase 1. |
| `POST` | `/api/v1/query` | `query` | Reserved for the Query Service. Returns a structured 501. |

The API never connects to ClickHouse or VictoriaMetrics (AD-3, AD-6). Telemetry
reads go through the Query Service; these endpoints only expose health and
control-plane metadata.

## Authentication

Send a project-scoped token as a bearer token:

```bash
curl -H "Authorization: Bearer $OBSERVAKI_API_TOKEN" \
  http://127.0.0.1:8080/api/v1/projects
```

Only the SHA-256 digest of a token is stored (`api_tokens.token_hash`). A token
resolves the organization, the project and its scopes server-side; the API never
trusts a client header for tenancy (AD-3, AD-20). The `admin` scope implies the
other scopes in the MVP.

The single-node evaluation stack seeds the public token
`observaki-local-dev-token` with `ingest`, `query` and `admin` scopes. Never
reuse it outside a local stack.

Status codes:

- `401` when the token is missing, malformed, expired or unknown.
- `403` when the token lacks the required scope or targets another project.
- `503` when the control-plane database is not available. Health and the
  OpenAPI document stay available in that mode.

## Errors

Every failure uses the AD-15 envelope (ADR-0005, `ARCHITECTURE-SPINE.md`):

```json
{
  "code": "unauthorized",
  "message": "an API token is required",
  "details": { "reason": "missing_credentials" },
  "request_id": "019a5f3c-7f1e-7c3a-9f4e-2b6b1f0d9a11"
}
```

- `code` is stable and safe to switch on.
- `message` is for humans and may change.
- `details` carries machine-readable context such as the offending field.
- `request_id` matches the `X-Request-Id` response header and the server log.

No response contains a stack trace, a database DSN or another tenant's data.
The panic recovery path logs the stack server-side and returns
`500 internal_error`.

## Contract generation and validation

The routes and the OpenAPI document come from the same operation table
(`internal/api/routes.go`), so they cannot drift (AD-26):

```bash
make api-gen     # regenerate api/openapi/openapi.json from the Go definitions
make api-check   # fail when the checked-in contract is stale
```

`make lint` depends on `make api-check`, so `make ci` and the GitHub workflow
validate the contract. The contract test also checks that every documented
operation is registered, that `info.version` has an entry in the OpenAPI
changelog, and that the checked-in artifact matches the runtime document.

## Versioning

`/api/v1` is stable. Additive changes ship in place; breaking changes go through
the deprecation policy and a new major version. See
[`versioning.md`](versioning.md) and the
[OpenAPI changelog](../../api/openapi/CHANGELOG.md).
