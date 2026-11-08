# API versioning and deprecation policy

This policy implements AD-9 (versioned schemas and contracts) and AD-26
(external contracts are pinned and OpenAPI is compared in CI). It applies to
everything under `/api/v1` and to the OpenAPI document served at
`/openapi.json`.

## Version scheme

- The URL major version is the compatibility boundary: `/api/v1` today.
- `info.version` in the OpenAPI document is semantic (`MAJOR.MINOR.PATCH`):
  - `MAJOR` changes only with a new URL major (`/api/v2`).
  - `MINOR` adds endpoints, fields, enum values or response codes.
  - `PATCH` fixes descriptions, examples or constraints without changing the
    accepted input or the guaranteed output.
- The runtime document is generated from the same operation table that
  registers the routes. CI (`make api-check`) fails when the checked-in
  `api/openapi/openapi.json` is stale.

## Additive (compatible) changes

Any of the following may ship inside `/api/v1`:

- a new endpoint under `/api/v1/...`;
- a new optional request parameter or a new optional response field;
- a new error `code` for a new failure mode;
- relaxing a validation rule (for example, accepting a wider range);
- documentation, example and description fixes.

Clients must ignore unknown response fields and must not rely on field order.

## Breaking changes

A breaking change includes removing or renaming an endpoint, field or error
`code`; changing a type; making an optional parameter required; tightening
validation; changing the meaning of an existing value; or changing the default
of a parameter in a way that changes results.

A breaking change:

1. is proposed in an issue labelled `api-breaking`, with the client impact and
   the migration path;
2. gets a new URL major under `/api/v2`; `/api/v1` keeps working during the
   deprecation window;
3. is recorded in [`api/openapi/CHANGELOG.md`](../../api/openapi/CHANGELOG.md)
   in the same commit that changes the contract;
4. is reviewed by a maintainer before merge. The review is the gate for AC3 of
   Story 5.5: no breaking change lands without the changelog entry and the
   version bump.

## Deprecation window

- Announce the deprecation in the OpenAPI changelog and in the release notes.
- Deprecated operations and fields keep working for **at least 90 days or two
  minor releases, whichever is longer**.
- Responses for deprecated operations carry the `Deprecation` header (RFC 9745,
  an RFC 3339 timestamp) and, once a removal date is known, the `Sunset` header
  (RFC 8594).
- The OpenAPI operation is marked `deprecated: true` with a description that
  names the replacement and the removal date.
- Removal happens only at the next major version. The `/api/v1` document never
  loses an operation during its lifetime.

## Process checklist

For every API change:

1. Update `internal/api/routes.go` (and, when needed, the handlers).
2. Bump `OpenAPIVersion` in `internal/api/openapi.go` according to the rules
   above.
3. Add or update the entry in `api/openapi/CHANGELOG.md`; the contract test
   requires a heading that matches `info.version`.
4. Run `make api-gen` and commit `api/openapi/openapi.json`.
5. Run `make api-check`, `go test ./internal/api` and `make smoke`.
6. For a breaking change, add the `/api/v2` surface and the deprecation headers
   before removing anything from `/api/v1`.
