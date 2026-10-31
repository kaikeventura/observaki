# Observaki — Project Context

**Produto:** plataforma de observabilidade open source, self-hosted e OTel-native: métricas, logs, traces, dashboards, alertas e correlação.
**Estágio:** planejamento completo; Fase 0 (fundação/benchmark) ainda não iniciada; nenhum código de produto.
**Próximo passo:** Story 1.1 — bootstrap do mono-repo e ambiente de desenvolvimento.
**Estado corrente:** `STATUS.md` (fonte de verdade de onde estamos; atualizar ao fim de cada sessão).
**Prompt de retomada:** `PROMPT.md` (mandar `Leia PROMPT.md e execute.`).

## Decisões confirmadas

- **Escala:** 1M eventos/s no benchmark do MVP; 100k/s por design partner em produção inicial; 10M/s no GA; 100M+/s via federação. 1B/s literal em um cluster está fora do v1.
- **Licença:** Apache 2.0 no núcleo + open-core.
- **ICP:** startups/times pequenos (3-50 engenheiros); enterprise na Fase 3.
- **Stack:** Go 1.27, ClickHouse 26.8 LTS, VictoriaMetrics 1.148 LTS, NATS 2.15, PostgreSQL 18.6, SvelteKit 3.1, Svelte 5.57, uPlot 1.6.32.
- **Arquitetura:** ports & adapters + pipes-and-filters; 35 ADs.
- **UX:** dark-first, desktop-first, keyboard-first; WCAG 2.2 AA.

## Invariantes que não podem ser violados

- Control plane e data plane separados; API nunca consulta ClickHouse/VM diretamente.
- Query Service é o único read path; toda consulta valida contexto assinado e injeta tenant.
- RED calculado no gateway antes do sampling; armazenado no VictoriaMetrics; não no ClickHouse.
- Ack de ingestão só após WAL/flush; dedupe por `event_id`; DLQ para rejeições permanentes.
- `project_id` imutável + epoch; row policies, vmauth e cache invalidado em move/delete.
- Config plane: PostgreSQL desired state → snapshot versionado → data plane; reconciliação única.
- Performance e acessibilidade são definition of done.

## Comandos

Após o scaffold da Story 1.1: `make bootstrap`, `make ci`, `make dev`, `make smoke`, `make benchmark`.

## Onde ler

- `AGENTS.md` — regras de desenvolvimento.
- `_bmad-output/planning-artifacts/README.md` — índice do pacote.
- `_bmad-output/planning-artifacts/PROGRAM-PLAN.md` — fases/gates/staffing.
- `_bmad-output/planning-artifacts/epics.md` — Fase 0 detalhada.
- `_bmad-output/planning-artifacts/architecture/.../ARCHITECTURE-SPINE.md` — AD-1..AD-35.
- `_bmad-output/planning-artifacts/ux-designs/.../DESIGN.md` e `EXPERIENCE.md` — UX.
- `_bmad-output/planning-artifacts/DECISIONS.md` — decisões e pendências.
