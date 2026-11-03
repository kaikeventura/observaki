# Internal WAL contract (protobuf)

`observaki/ingest/v1/envelope.proto` is the gateway → NATS → writer contract
(AD-22). The generated Go code lives in `internal/ingestpb/` because it is an
internal data-plane contract, not a public API.

Regenerate after changing the proto:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
protoc -I api/proto \
  --go_out=. --go_opt=module=github.com/observaki/observaki \
  api/proto/observaki/ingest/v1/envelope.proto
```

Rules:

- Never reuse a field number; add new fields and reserve removed ones.
- Bump `schema_version` for a breaking change and keep a dual-read/dual-publish
  window for N-1.
- Update `internal/adapters/nats/envelope_test.go` when the wire contract
  changes; `ValidateEnvelope` enforces one `event_id` per leaf event.
