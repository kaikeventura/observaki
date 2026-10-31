# Observaki

**Observabilidade open source, self-hosted e OTel-native:** métricas, logs, traces, dashboards, alertas e correlação em uma única plataforma — com foco em performance, baixo footprint e operação por times pequenos.

> **Status:** Fase 0 (fundação e benchmarks) em andamento. O estado corrente fica em [`STATUS.md`](STATUS.md).

## Pré-requisitos

| Ferramenta | Versão | Observação |
|---|---|---|
| Go | 1.27 | o `go.mod` fixa a versão e o toolchain é resolvido automaticamente |
| Node.js | 26 LTS | frontend SvelteKit |
| pnpm | 10 | gerenciador de pacotes do `web/` |
| Rust | 1.85 (stable) | agente, com `rustfmt` e `clippy` |
| Docker Compose | v2 | opcional nesta fase (Story 5.1) |
| make, curl | recente | orquestração e smoke test |

## Quickstart (5 comandos)

Guia completo (portas, configuração e troubleshooting): [`docs/dev/local-setup.md`](docs/dev/local-setup.md).

```bash
make bootstrap   # valida a toolchain, instala dependências e cria .env
make test        # go test + vitest + cargo test
make build       # binário Go + build do SvelteKit + agente
make dev         # API Go (:8080) + SvelteKit (:5173)
make smoke       # em outro terminal: CI completo + smoke do binário
```

Abra <http://localhost:5173> — o cartão **API status** fica “Operational”
quando a API responde. `make lint` exige o golangci-lint, instalável com
`make tools`.

## Estrutura do repositório

```text
cmd/observaki/     # binário Go
internal/          # domínio, ports, adapters, ingest, query, alerting
web/               # SvelteKit 3 SPA
agent/             # agente Rust (Fase 2)
bench/             # harness de benchmark
deploy/            # Compose, Helm, jobs
docs/              # ADRs, guias
```

Módulo Go: `github.com/observaki/observaki` (premissa da Story 1.1 — ajustar
caso o org ou o nome definitivo do repositório sejam outros).

## Como o desenvolvimento funciona

Cada sessão do OpenCode é independente. O agente lê `AGENTS.md`, `project-context.md` e `STATUS.md`, continua do `next_action` e atualiza o estado ao terminar.

Prompt de retomada: [`PROMPT.md`](PROMPT.md). Basta mandar:

```text
Leia PROMPT.md e execute.
```

## Planejamento

- [`PROGRAM-PLAN.md`](_bmad-output/planning-artifacts/PROGRAM-PLAN.md) — fases, gates, staffing, releases.
- [`PRD`](_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md) + [`addendum`](_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md) — produto, requisitos, stack, benchmarks.
- [`ARCHITECTURE-SPINE.md`](_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md) — 35 invariantes (AD-1..AD-35).
- [`DESIGN.md`](_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/DESIGN.md) + [`EXPERIENCE.md`](_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md) — UX/UI.
- [`epics.md`](_bmad-output/planning-artifacts/epics.md) — Fase 0 detalhada.
- [`DECISIONS.md`](_bmad-output/planning-artifacts/DECISIONS.md) — decisões e pendências.

## Gates

- **G1:** 1M eventos/s, p95 < 1s em 1B linhas, compressão ≥ 5x, 3 design partners.
- **G2:** UJ-1 ponta a ponta, 3 pilotos, soak de 7 dias.
- **G3:** 1M/s em produção assistida, 10 parceiros.
- **G4:** 10M/s, 30 dias de soak, 3 tenants, GA 1.0.

Nenhuma fase começa sem o gate anterior. Detalhes em [`PROGRAM-PLAN.md`](_bmad-output/planning-artifacts/PROGRAM-PLAN.md).

## Configuração

O binário lê a configuração de variáveis de ambiente; os defaults seguros
estão documentados em [`.env.example`](.env.example):

| Variável | Default | Descrição |
|---|---|---|
| `OBSERVAKI_HTTP_ADDR` | `:8080` | endereço de escuta da API |
| `OBSERVAKI_LOG_LEVEL` | `info` | `debug`, `info`, `warn` ou `error` |
| `OBSERVAKI_ENV` | `development` | `development`, `test`, `staging` ou `production` |
| `OBSERVAKI_DATABASE_URL` | — | PostgreSQL do control plane (opcional na Fase 0) |
| `OBSERVAKI_WEB_DIR` | `web/build` | build estático servido pelo binário |

`make bootstrap` cria `.env` a partir de `.env.example` e `make dev` carrega o
arquivo antes de subir a API e o SvelteKit. Valores inválidos falham com o nome
da variável e o valor recusado.

## Contribuindo

Contribuições são bem-vindas. Leia [`CONTRIBUTING.md`](CONTRIBUTING.md) para
setup, padrões, RFC e o **DCO** (todo commit precisa de `git commit -s`). Ao
participar, você concorda com o [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

Vulnerabilidades devem ser reportadas em privado, conforme o
[`SECURITY.md`](SECURITY.md) — nunca em issue pública.

## Licença

O núcleo é [Apache 2.0](LICENSE). O modelo open-core e a lista de candidatos a
recursos enterprise — ainda **pendente de decisão de negócio** (P-03) — estão em
[`docs/governance/open-core.md`](docs/governance/open-core.md). A fronteira
exata também é acompanhada em [`DECISIONS.md`](_bmad-output/planning-artifacts/DECISIONS.md).
