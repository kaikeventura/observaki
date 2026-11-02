# Observaki — Project Context

**Produto:** plataforma de observabilidade open source, self-hosted e OTel-native: métricas, logs, traces, dashboards, alertas e correlação.
**Estágio:** Fase 0 (fundação/benchmark) em andamento; Stories 1.1–1.4, 2.1–2.4 (scaffolding) e 3.1 concluídas, com gerador OTLP, cenários de benchmark, catálogo de consultas, coleta de recursos, gerador de relatório e o schema ClickHouse de logs. A Story 2.4 entregou o perfil T1, o provisionamento de referência, o custo/cleanup e uma pré-baseline local; a Story 3.1 entregou o schema de logs e um spike local de 1B linhas (6,98x, `q03` p95 263 ms, TTL 7/15). O baseline T1 continua bloqueado por provedor/orçamento e pelas Stories 3.2–3.5/4.1–4.5.
**Próximo passo:** Story 3.2 — schema ClickHouse para traces. P-07 (provedor, teto de gasto e credencial IAM dedicada) continua necessário para o baseline T1.
**Estado corrente:** `STATUS.md` (fonte de verdade de onde estamos; atualizar ao fim de cada sessão).
**Prompt de retomada:** `PROMPT.md` (mandar `Leia PROMPT.md e execute.`).

## Decisões confirmadas

- **Escala:** 1M eventos/s no benchmark do MVP; 100k/s por design partner em produção inicial; 10M/s no GA; 100M+/s via federação. 1B/s literal em um cluster está fora do v1.
- **Licença:** Apache 2.0 no núcleo + open-core.
- **ICP:** startups/times pequenos (3-50 engenheiros); enterprise na Fase 3.
- **Stack:** Go 1.27, ClickHouse 26.8 LTS, VictoriaMetrics 1.148 LTS, NATS 2.15, PostgreSQL 18.6, SvelteKit 3.1, Svelte 5.57, uPlot 1.6.32.
- **Arquitetura:** ports & adapters + pipes-and-filters; 35 ADs.
- **UX:** dark-first, desktop-first, keyboard-first; WCAG 2.2 AA.
- **Schema de logs:** partição diária por `ingested_at`, TTL por `retention_days` e `ORDER BY` tenant/projeto/serviço/tempo; spike local de 1B em 2026-11-02 (6,98x, `q03` p95 263 ms, TTL 7/15).

## Invariantes que não podem ser violados

- Control plane e data plane separados; API nunca consulta ClickHouse/VM diretamente.
- Query Service é o único read path; toda consulta valida contexto assinado e injeta tenant.
- RED calculado no gateway antes do sampling; armazenado no VictoriaMetrics; não no ClickHouse.
- Ack de ingestão só após WAL/flush; dedupe por `event_id`; DLQ para rejeições permanentes.
- `project_id` imutável + epoch; row policies, vmauth e cache invalidado em move/delete.
- Config plane: PostgreSQL desired state → snapshot versionado → data plane; reconciliação única.
- Performance e acessibilidade são definition of done.

## Comandos

Após o scaffold da Story 1.1: `make bootstrap`, `make ci`, `make dev`, `make smoke`, `make benchmark`, `make bench-env-check`, `make bench-clickhouse-check` (10M) e `make bench-clickhouse` (1B local). O baseline T1 usa `./bench/env/baseline.sh`; o cluster T1 usa `./bench/env/cluster.sh`; o spike de armazenamento usa `./bench/clickhouse/run.sh`.

## Onde ler

- `AGENTS.md` — regras de desenvolvimento.
- `_bmad-output/planning-artifacts/README.md` — índice do pacote.
- `_bmad-output/planning-artifacts/PROGRAM-PLAN.md` — fases/gates/staffing.
- `_bmad-output/planning-artifacts/epics.md` — Fase 0 detalhada.
- `_bmad-output/planning-artifacts/architecture/.../ARCHITECTURE-SPINE.md` — AD-1..AD-35.
- `_bmad-output/planning-artifacts/ux-designs/.../DESIGN.md` e `EXPERIENCE.md` — UX.
- `_bmad-output/planning-artifacts/DECISIONS.md` — decisões e pendências.
- `BENCHMARKS.md` — pré-baseline, spike local da Story 3.1, perfil T1 e bloqueios do G1.
- `bench/env/README.md` — provisionamento, custo e protocolo do baseline.
- `bench/clickhouse/README.md` — schema, carga, compressão, `q03` e TTL do spike 3.1.
- `docs/dev/story-2.4-handoff.md` — evidência e pendências da Story 2.4.
- `docs/dev/story-3.1-handoff.md` — evidência e pendências da Story 3.1.
