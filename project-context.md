# Observaki — Project Context

**Produto:** plataforma de observabilidade open source, self-hosted e OTel-native: métricas, logs, traces, dashboards, alertas e correlação.
**Estágio:** Fase 0 (fundação/benchmark) em andamento; Stories 1.1–1.4 e 2.1–2.3 concluídas, com gerador OTLP, cenários de benchmark, catálogo de consultas, coleta de recursos e gerador de relatório implementados. A Story 2.4 entregou o perfil T1, o provisionamento de referência, o custo/cleanup e uma pré-baseline local; a execução T1 está bloqueada por provedor/orçamento e pelas Stories 3.1/4.1.
**Próximo passo:** aprovar P-07 (provedor, teto de gasto e credencial IAM dedicada) e concluir 3.1/4.1 para executar o baseline T1.
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

Após o scaffold da Story 1.1: `make bootstrap`, `make ci`, `make dev`, `make smoke`, `make benchmark`, `make bench-env-check`. O baseline usa `./bench/env/baseline.sh`; o cluster T1 usa `./bench/env/cluster.sh`.

## Onde ler

- `AGENTS.md` — regras de desenvolvimento.
- `_bmad-output/planning-artifacts/README.md` — índice do pacote.
- `_bmad-output/planning-artifacts/PROGRAM-PLAN.md` — fases/gates/staffing.
- `_bmad-output/planning-artifacts/epics.md` — Fase 0 detalhada.
- `_bmad-output/planning-artifacts/architecture/.../ARCHITECTURE-SPINE.md` — AD-1..AD-35.
- `_bmad-output/planning-artifacts/ux-designs/.../DESIGN.md` e `EXPERIENCE.md` — UX.
- `_bmad-output/planning-artifacts/DECISIONS.md` — decisões e pendências.
- `BENCHMARKS.md` — pré-baseline publicada, perfil T1 e bloqueios do G1.
- `bench/env/README.md` — provisionamento, custo e protocolo do baseline.
- `docs/dev/story-2.4-handoff.md` — evidência e pendências da Story 2.4.
