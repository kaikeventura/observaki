# Story 5.5 — API mínima versionada e OpenAPI

**Status:** done (2026-11-08)
**Escopo:** FR-57, NFR-11; `epics.md` (P0-E5, Story 5.5)
**Dependências:** 4.1 e 4.2 (ambas done)

## O que foi entregue

| AC | Resultado |
|---|---|
| AC1 — Contrato | `GET /openapi.json` serve um documento OpenAPI 3.1 gerado da mesma tabela de operações que registra as rotas (`internal/api/routes.go`). Cobre `health`, `health/overview`, `projects`, `ingest-tokens` e `query/sample` (além de `POST /api/v1/query` reservado). O CI valida com `make api-check` e o teste de contrato `go test ./internal/api`. |
| AC2 — Versionamento e erros | Toda falha retorna o envelope AD-15 `{code, message, details, request_id}` e o header `X-Request-Id`; sem stack trace. 400/401/403/404/405/500/501/503 têm códigos estáveis. O panic recovery registra o stack só no servidor. |
| AC3 — Compatibilidade | Política em `docs/api/versioning.md` (semver, mudanças aditivas vs. breaking, janela de depreciação de 90 dias/2 minors, headers `Deprecation`/`Sunset`, revisão obrigatória) e changelog em `api/openapi/CHANGELOG.md`; o teste exige uma entrada para `info.version`. |

## Endpoints

| Método | Rota | Auth | Observação |
|---|---|---|---|
| GET | `/healthz` | pública | Liveness sem dependência do data plane. |
| GET | `/api/v1/health` | pública | Liveness + versão do build. |
| GET | `/api/v1/health/overview` | pública | Snapshot de auto-observabilidade da 5.4. |
| GET | `/openapi.json` | pública | Contrato OpenAPI 3.1 gerado. |
| GET | `/api/v1/projects` | `query` | Projeto vinculado ao token (tenancy vem do token). |
| GET | `/api/v1/projects/{projectID}/ingest-tokens` | `admin` | Metadados (nome, escopos, expiração); nunca o digest. |
| GET | `/api/v1/query/sample?signal=metrics\|logs\|traces` | `query` | Amostra determinística com `sample=true`. |
| POST | `/api/v1/query` | `query` | Reservado ao Query Service; 501 estruturado. |

`admin` implica os outros escopos no MVP. O token de avaliação
`observaki-local-dev-token` (seed da Story 5.4) tem `ingest`, `query` e `admin`.

## Decisões de implementação

- **Auth no control plane, não no header.** O token Bearer é reduzido a SHA-256
  e resolvido em `api_tokens` + `projects` (AD-7, AD-19, AD-20). O `project_id`
  e o `org_id` vêm do token; um `projectID` de outro projeto retorna 403 sem
  confirmar existência (AD-3). O endpoint de tokens nunca seleciona
  `token_hash`.
- **Health e contrato públicos.** Não carregam dados de tenant e mantêm a
  página `/health` e a descoberta do contrato funcionando sem credencial, como
  na 5.4. Metadados exigem token; sem banco, respondem 503 e não 500.
- **OpenAPI gerado de uma fonte única.** `Operations()` alimenta o roteador e o
  documento; `cmd/openapi-gen` materializa `api/openapi/openapi.json` e
  `make api-check` falha quando o artefato está desatualizado (AD-26).
- **Sem Query Service no MVP.** `/api/v1/query/sample` é explicitamente uma
  amostra (`sample=true`, `query_service="phase-1"`); a API não abre conexão com
  ClickHouse/VictoriaMetrics (AD-3, AD-6). O `POST /api/v1/query` retorna 501
  estruturado até a Story 8.3.
- **Migração aditiva.** `0003_api_tokens_hash.sql` cria um índice único em
  `api_tokens.token_hash` para a autenticação por digest. Nenhuma tabela/coluna
  nova; o backup JSON v1 continua válido.
- **Sem dependência nova.** Nenhum gerador/validador externo; o contrato é
  gerado e comparado com a biblioteca padrão.

## Evidência

Comandos executados nesta máquina (Linux):

```bash
make fmt
make lint            # inclui adr-check e api-check
make test
make build
SMOKE_SKIP_CI=1 make smoke
./deploy/compose/smoke.sh
```

Resultados:

- `make lint`: ADR 6/6, `api-check` 1.0.0, golangci-lint 0 issues,
  svelte-check 0 errors, clippy 0 warnings.
- `make test`: Go, Vitest (5 testes) e cargo (2 testes) verdes; o teste de
  contrato cobre envelope, 401/403/400/404/405/500, isolamento de projeto,
  amostra, golden e changelog.
- `make smoke`: OpenAPI + 401/404/405 estruturados, `X-Request-Id`, gateway,
  WAL, quota, build web e benchmark report.
- `./deploy/compose/smoke.sh`: stack healthy em **31 s**; `/openapi.json` 3.1
  com as rotas MVP; `/api/v1/projects` 401 sem token; o token seed lê o projeto
  de desenvolvimento e os metadados de token sem digest; amostra de telemetria
  e persistência pass; relatório em
  `deploy/compose/results/20261108T032956Z/report.md`.
- `TestIntegrationAPIStore` (gated por `OBSERVAKI_POSTGRES_TEST_URL`) rodou contra
  um PostgreSQL 18.6 descartável: digest válido, token desconhecido, token
  expirado, projeto e listagem sem digest passaram (`ok ... 0.201s`), junto com
  os testes de integração de alerting e backup/restore. O compose smoke também
  exercitou o mesmo caminho com o PostgreSQL real da stack.

## Pendências e limites

- O Query Service real (FR-57/Story 8.3) chega na Fase 1; `POST /api/v1/query`
  continua 501 e o sample é o contrato, não telemetria.
- Criação/rotação de tokens via API não faz parte do MVP (só listagem); a
  rotação continua no control plane/CLI. RBAC granular é Fase 1.
- Health e `/openapi.json` seguem públicos; a UI ainda não envia token nas
  telas de metadados, porque elas não existem (Explore 6.1–6.4).
- P-07 continua bloqueando o baseline T1; a validação macOS e EKS/GKE seguem
  pendentes das stories anteriores.
