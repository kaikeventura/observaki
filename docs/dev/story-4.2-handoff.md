# Story 4.2 — Auth por tenant, quotas e rate limiting

**Status:** concluída na parte funcional; a medição de isolamento p95 em 1M/s
continua pendente do cluster T1 (P-07), como o restante do G1.
**Data:** 2026-11-03
**Epic:** P0-E4 — Gateway OTLP e ingestão
**ADs:** AD-1, AD-2, AD-3, AD-4, AD-13, AD-15, AD-19, AD-20, AD-35

## Escopo entregue

1. **Snapshot de configuração versionado** (`internal/ingest/auth.go`):
   `config_version`, `generation`, `issued_at`, `expires_at`, tenants,
   projetos, tokens com SHA-256 e escopos, quota por tenant. O arquivo é
   relido a cada `OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL` (default 30s). Se o
   snapshot expirar, auth e quota falham fechadas com 503 `config_stale`
   (AD-19).
2. **Auth de ingestão**: `Authorization: Bearer <token>` em HTTP e metadata
   `authorization` em gRPC. Token ausente/inválido/expirado → 401; escopo sem
   `ingest` → 403 `insufficient_scope`. O token resolve tenant/projeto; nenhum
   header de cliente é autoritativo (AD-3).
3. **Isolamento cross-project**: headers `X-Observaki-Tenant-Id` e
   `X-Observaki-Project-Id` (ou metadata gRPC) são assertions. Divergência do
   token → 403 `cross_project_write` e evento de audit com tenant/projeto de
   origem, alvo, request ID, sinal, protocolo e IP remoto.
4. **Quota por tenant**: token bucket em eventos/s com burst. Excesso → 429
   com `Retry-After` (segundos) e `RetryInfo` no `google.rpc.Status`. O
   caminho HTTP reserva a estimativa de eventos pelo `Content-Length` antes de
   descomprimir/decodificar; a estimativa é um EWMA de bytes/evento por tenant
   e é reconciliada após o decode. Assim o tenant barulhento é descartado
   antes do trabalho caro (AD-4, Story 4.2 AC3).
5. **Audit append-only**: `OBSERVAKI_INGEST_AUDIT_FILE` grava JSONL com hash
   chain SHA-256 (`prev_hash`/`hash`), `fsync` por evento e verificação da
   cadeia na abertura. Eventos: `ingest.cross_project_write`,
   `ingest.insufficient_scope`, `ingest.config_loaded`. O WORM/assinatura do
   control plane continua para a Fase 3 (AD-35).
6. **Identidade no batch**: `Batch` carrega `TenantID`, `ProjectID`,
   `ConfigVersion` e `ConfigGeneration`, então o WAL/writer da Story 4.3 não
   grava sem tenancy resolvida (AD-2, AD-20).
7. **Self-observabilidade**: `/metrics` expõe falhas de auth por razão,
   rejeições de quota, idade/expiração/geração/tenants do snapshot e p95/p99
   por tenant. `/healthz`, `/readyz` e `/metrics` continuam sem auth (endpoints
   internos; o deploy deve restringi-los).
8. **CLI**: `observaki gateway` exige `OBSERVAKI_INGEST_AUTH_FILE`. Em
   `staging`/`production`, exige também chave pública Ed25519
   (`OBSERVAKI_INGEST_AUTH_PUBLIC_KEY`) e audit file; a assinatura destacada
   fica em `<arquivo>.sig`. Em desenvolvimento/teste a assinatura é opcional,
   com warning explícito.

## Contrato do snapshot

```json
{
  "config_version": "dev-1",
  "generation": 1,
  "issued_at": "2026-01-01T00:00:00Z",
  "expires_at": "2030-01-01T00:00:00Z",
  "tenants": [{
    "tenant_id": "tenant-a",
    "ingest_quota": {"events_per_second": 100000, "burst_events": 100000},
    "projects": [{
      "project_id": "project-a",
      "tokens": [{
        "token_id": "token-a",
        "sha256": "<sha256 hex do token>",
        "scopes": ["ingest"],
        "expires_at": "2029-01-01T00:00:00Z"
      }]
    }]
  }]
}
```

O `sha256` é calculado com `printf '%s' "$TOKEN" | sha256sum | cut -d' ' -f1`.
Nenhum token em texto claro é gravado no snapshot, no audit, nos logs ou no
repositório.

## Evidência

### Testes e checks

| Comando | Resultado |
|---|---|
| `go test ./...` | ok |
| `go test -race ./internal/ingest` | ok |
| `make fmt` / `make lint` | ok (golangci-lint, prettier, svelte-check, clippy, ADR check) |
| `make build` | ok |
| `make smoke` | ok; cobre 401 sem token, 403 cross-project + audit, 429 de quota, OTLP HTTP/gRPC, métricas e SIGTERM |
| `./bench/gateway/quota/run.sh --smoke` | pass; 8.801 requests 429, p95 de processamento do tenant quieto 1,643→1,727 ms (**+5,11%**) |

### Harness de isolamento (local, 1M/s ofertado)

`./bench/gateway/quota/run.sh --small-payload` (3 repetições, mediana):

| Métrica | Valor |
|---|---|
| Quota do tenant barulhento | 100.000 eventos/s |
| Tenant quieto | 50.000 eventos/s (logs) |
| Tenant barulhento ofertado | 1.000.000 eventos/s |
| Requests 429 | 26.703 |
| p95 de processamento do gateway, quieto sozinho | 5,461 ms |
| p95 de processamento do gateway, quieto sob carga | 6,287 ms |
| Variação do p95 de processamento | **+15,13%** |
| p95 cliente (observado) | 10,535 → 11,610 ms (+10,20%) |

**Veredito:** o status funcional é **pass** (429 + `Retry-After`, quota
independente por tenant, tenant quieto sem erros). O p95 local ficou acima de
10%, então o isolamento está **inconclusivo** neste host: gateway e os dois
geradores de carga compartilham a mesma máquina, e a 1M/s o gerador barulhento
satura CPU/banda de memória. O run com payload pequeno foi usado para o host
sustentar 1M/s; o run com payload de referência (832/820/2400 B) não sustenta
1M/s localmente. A medição definitiva precisa do T1 com load generators em
hosts separados, que continua bloqueado por P-07 e pelas Stories 4.3–4.5.

Resultado bruto: `bench/gateway/quota/results/20261103T030500Z/report.json` e
`report.md`. O limite de 10% no relatório local é o critério da história, não
uma evidência de G1.

### Cobertura de testes adicionada

- validação do snapshot (versão, geração, expiração, duplicatas, hash, escopos,
  quota);
- 401 para token ausente/inválido/expirado; 403 para escopo ausente;
- reload de arquivo e snapshot expirado (fail-closed);
- assinatura Ed25519 válida, inválida e ausente;
- audit: hash chain, reabertura após restart, detecção de tamper;
- quota: burst, refill, Retry-After, tenant desconhecido;
- pré-admissão HTTP por EWMA e rejeição antes do decode (sem chegar ao sink);
- integração HTTP/gRPC: identidade no batch, cross-project, 429 e `RetryInfo`;
- isolamento local de buckets com dois tenants concorrentes.

## Arquivos principais

- `internal/ingest/auth.go` — snapshot, compilação, autenticação, reload.
- `internal/ingest/quota.go` — token bucket, reserva/settle e EWMA.
- `internal/ingest/audit.go` — audit JSONL com hash chain.
- `internal/ingest/authorize.go` — headers/metadata, 403, quota, audit.
- `internal/ingest/latency.go` — p95 por tenant.
- `internal/ingest/{http,grpc,gateway,errors,metrics,sink}.go` — integração.
- `internal/config/config.go`, `cmd/observaki/main.go` — configuração e CLI.
- `bench/gateway/quota/` — harness e README.
- `bench/loadgen` + `bench/cmd/otlploadgen` — `--header`, payload flags.
- `scripts/smoke.sh` — 401/403/429/audit no smoke.
- `.env.example`, `README.md`, `docs/dev/local-setup.md`.

## Limitações e pendências

1. **p95 <10% em 1M/s**: inconclusivo localmente; medir no T1 com load
   generators separados. O código de pré-admissão reduz o custo do tenant
   barulhento, mas a prova numérica fica no baseline.
2. **Snapshot assinado pelo reconciliador**: a assinatura Ed25519 destacada já
   é verificada, mas o publicador (NATS/HTTP, rotação de chave, replay) é da
   Fase 1. O arquivo local é o transporte da Fase 0.
3. **JWKS/OIDC**: Q-03 do ADR-0002 fica parcialmente respondida (auth cache
   local sem PostgreSQL no hot path); o cache JWKS é da Fase 1.
4. **Audit WORM/export**: hash chain local entregue; export contínuo para
   object storage e assinatura ficam para a Fase 3 (AD-35).
5. **Quota por projeto**: a quota é por tenant (história). Quotas por projeto
   e cardinalidade por tenant (AD-33) ficam para a 4.4/3.4 na VM.
6. **Rate limit por request**: a pré-admissão usa `Content-Length`; requests
   chunked (sem tamanho) caem no caminho pós-decode. gRPC é limitado após o
   decode pelo framework; o bulkhead por tenant não foi implementado nesta
   história.

## Próximo passo

Story 4.3 — batching, compressão, sharding e WAL NATS. O gateway já entrega
`Batch` com tenant/projeto e geração de config; a 4.3 substitui o
`DiscardSink` pelo writer NATS JetStream e mantém o ack somente após o WAL
(AD-4).
