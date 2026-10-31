# Observaki

**Observabilidade open source, self-hosted e OTel-native.** Métricas, logs, traces, dashboards, alertas e correlação em uma única plataforma — com foco em performance, baixo footprint e operação por times pequenos.

> **Status:** planejamento completo; **Fase 0 ainda não iniciada**. Nenhum código de produto foi escrito. O estado corrente fica em [`STATUS.md`](STATUS.md).

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

## Estrutura do repositório

A estrutura definitiva nasce na Story 1.1 (Fase 0). O alvo:

```text
cmd/observaki/     # binário Go
internal/          # domínio, ports, adapters, ingest, query, alerting
web/               # SvelteKit 3 SPA
agent/             # agente Rust (Fase 2)
bench/             # harness de benchmark
deploy/            # Compose, Helm, jobs
docs/              # ADRs, guias
```

## Gates

- **G1:** 1M eventos/s, p95 < 1s em 1B linhas, compressão ≥ 5x, 3 design partners.
- **G2:** UJ-1 ponta a ponta, 3 pilotos, soak de 7 dias.
- **G3:** 1M/s em produção assistida, 10 parceiros.
- **G4:** 10M/s, 30 dias de soak, 3 tenants, GA 1.0.

Nenhuma fase começa sem o gate anterior. Detalhes em [`PROGRAM-PLAN.md`](_bmad-output/planning-artifacts/PROGRAM-PLAN.md).

## Licença

Apache 2.0 no núcleo + open-core para recursos enterprise. A fronteira exata é uma decisão pendente em [`DECISIONS.md`](_bmad-output/planning-artifacts/DECISIONS.md).
