# Observaki

**Observabilidade open source, self-hosted e OTel-native:** métricas, logs, traces, dashboards, alertas e correlação em uma única plataforma — com foco em performance, baixo footprint e operação por times pequenos.

> **Status:** Fase 0 (fundação e benchmarks) em andamento. O scaffold do mono-repo nasce na Story 1.1; o estado corrente fica em [`STATUS.md`](STATUS.md).

## Pré-requisitos

| Ferramenta | Versão | Observação |
|---|---|---|
| Go | 1.27 | o `go.mod` fixa a versão e o toolchain é resolvido automaticamente |
| Node.js | 26 LTS | frontend SvelteKit |
| pnpm | 10 | gerenciador de pacotes do `web/` |
| Rust | 1.85 (stable) | agente, com `rustfmt` e `clippy` |
| Docker Compose | v2 | opcional nesta fase (Story 5.1) |
| make, curl | recente | orquestração e smoke test |

## Desenvolvimento local

O guia completo (comandos, portas e troubleshooting) está em [`docs/dev/local-setup.md`](docs/dev/local-setup.md). O fluxo é:

```bash
make bootstrap   # valida a toolchain e instala dependências
make test        # go test + vitest + cargo test
make build       # binário Go + build do SvelteKit + agente
make dev         # API Go (:8080) + SvelteKit (:5173)
make smoke       # valida o binário e o build do web
```

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

## Licença

Apache 2.0 no núcleo + open-core para recursos enterprise. A fronteira exata é uma decisão pendente em [`DECISIONS.md`](_bmad-output/planning-artifacts/DECISIONS.md).
