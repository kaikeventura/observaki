---
stepsCompleted: [1, 2, 3, 4]
inputDocuments:
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/review-pm.md
project: Observaki
phase: 0
status: ready-for-dev
created: 2026-10-31
updated: 2026-10-31
---

# Observaki — Epic Breakdown (Fase 0)

## Overview

Este documento decompõe a **Fase 0 — Fundação e Benchmarks** do Observaki em epics e stories implementáveis. A Fase 0 dura **4 semanas** e existe para reduzir o risco técnico antes de qualquer feature de usuário: provar a stack em 1M eventos/s, fechar as ADRs, criar um esqueleto deployável, validar o orçamento de frontend e comprometer 3 design partners.

**Escopo:** 7 epics, 31 stories. O backlog das Fases 1-4 será criado depois do Gate G1 (benchmark), usando o mesmo PRD e as ADRs resultantes.

**Contrato de arquitetura:** `ARCHITECTURE-SPINE.md` (AD-1..AD-35) é binding. Se uma story contradizer um AD, o AD vence e a story é ajustada antes do desenvolvimento.

**Fontes:** `prd.md` (64 FRs, NFRs, fases), `addendum.md` (arquitetura, stack, benchmark, modelo de dados) e `review-pm.md` (riscos e gates). Não há spec de UX formal ainda; a Fase 0 só precisa de um shell de UI para validar performance e orçamento de bundle.

*Os enunciados das stories seguem o formato canônico do template BMad (`As a / I want / So that`); os critérios de aceitação e o restante do documento estão em Português-BR.*

---

## Requirements Inventory

### Functional Requirements

*Os 64 FRs do PRD estão listados abaixo. A marca **[F0]** indica que o requisito é exercitado, total ou parcialmente, na Fase 0. Os demais serão decompostos nas fases seguintes.*

**Ingestão e pipelines**
- **FR-1 [F0]** — Ingestão OTLP unificada (gRPC/HTTP) para métricas, logs e traces, com `partial_success`, gzip e zstd opcional.
- **FR-2 [F0]** — Compatibilidade Prometheus: scrape com config estática e remote write; service discovery completo na Fase 2.
- **FR-3** — Pipeline de transformação (filtro, drop, redaction, enrich, sample, rotação) com preview.
- **FR-4 [F0]** — Durabilidade e backpressure com WAL, at-least-once, 429/Retry-After e dedupe por `event_id`.
- **FR-5 [F0]** — Quotas e isolamento de ingestão por tenant/projeto.
- **FR-6** — Ingestão de eventos de mudança (deploy, feature flag, incidente).

**Métricas**
- **FR-7 [F0]** — Consulta MetricsQL/PromQL (instant/range, funções, subqueries).
- **FR-8** — Métricas OTel nativas (delta/cumulative, histogramas, exponential histograms e exemplars).
- **FR-9** — Catálogo e explorer de métricas (nomes, labels, cardinalidade).
- **FR-10 [F0]** — Guardrails de cardinalidade por tenant.
- **FR-11 [F0]** — Rollups de 1m/5m no MVP; 1h na Fase 2.

**Logs**
- **FR-12 [F0]** — Busca full-text e estruturada em logs, com facetas e modo SQL limitado a campos indexados no MVP.
- **FR-13** — Live tail com SSE/WebSocket e backpressure.
- **FR-14** — Parsing JSON/logfmt/regex e pattern inspector básico.
- **FR-15 [F0]** — Correlação log-trace mínima por `trace_id`/`span_id`.
- **FR-16** — Archive search em object storage (Fase 3).

**Traces e APM**
- **FR-17 [F0]** — Ingestão e busca de traces por serviço, operação, status, duração e atributos.
- **FR-18** — Waterfall e detalhes de span.
- **FR-19 [F0]** — Service map básico derivado de spans.
- **FR-20** — Span metrics/RED calculados antes do sampling.
- **FR-21** — Error tracking com fingerprint.
- **FR-22** — Tail sampling com roteador por `trace_id`.

**Explore e dashboards**
- **FR-23 [F0]** — Explore unificado com troca de sinal, autocomplete, histórico e estados de erro/vazio.
- **FR-24 [F0]** — Dashboards com grid, 7 tipos de painel, pastas, tags, busca e favoritos.
- **FR-25** — Variáveis e templates de dashboard.
- **FR-26** — Import/export JSON interno; migração Grafana/Prometheus na Fase 2.
- **FR-27 [F0]** — Renderização performática com uPlot, tabelas virtualizadas e cancelamento.
- **FR-28** — Views salvas e colaboração.
- **FR-29** — Dashboards públicos/embed (Fase 3).

**Alertas e SLOs**
- **FR-30 [F0]** — Regras de alerta para métricas e logs (traces na Fase 2).
- **FR-31 [F0]** — Canais Slack, webhook e email (PagerDuty/Opsgenie/Teams na Fase 2).
- **FR-32 [F0]** — Estado, dedup, silences e acknowledge básicos.
- **FR-33** — SLOs e error budgets (Fase 2).
- **FR-34** — Burn-rate alerts (Fase 2).
- **FR-35** — Runbooks e links de incidente (Fase 2).

**Correlação e contexto**
- **FR-36 [F0]** — Correlação mínima trace→log, log→trace e exemplar→trace; avançada na Fase 2.
- **FR-37** — Catálogo de entidades (Fase 2).
- **FR-38** — Timeline de incidente (Fase 3).
- **FR-39** — Change tracking (Fase 2).

**Ciclo de vida e custo**
- **FR-40 [F0]** — Retenção por projeto para logs/traces; retenção global para métricas no MVP.
- **FR-41** — Tiering hot/warm/cold (Fase 3).
- **FR-42** — Amostragem e agregação (Fase 2).
- **FR-43** — Estimador de custo e uso (Fase 2).
- **FR-44** — Exportação/exclusão de dados (Fase 3).

**Multi-tenancy, auth e governança**
- **FR-45 [F0]** — Organizações e projetos com isolamento lógico.
- **FR-46 [F0]** — Auth local e OIDC.
- **FR-47 [F0]** — RBAC básico e API tokens.
- **FR-48** — SSO/SCIM (Fase 3).
- **FR-49 [F0]** — Audit log mínimo no MVP; completo na Fase 3.
- **FR-50 [F0]** — Redaction de PII e segredos no pipeline.
- **FR-51** — Agente leve (Fase 2 beta; Fase 3 GA).
- **FR-52 [F0]** — Compatibilidade com OTel Collector.
- **FR-53 [F0]** — Helm, Docker Compose e modo single-node.
- **FR-54** — Kubernetes Operator (Fase 3).
- **FR-55** — Instalação air-gapped (Fase 3).
- **FR-56** — Auto-discovery/eBPF (Fase 4).

**API, CLI e IaC**
- **FR-57 [F0]** — API REST/gRPC versionada com OpenAPI.
- **FR-58** — CLI (Fase 2).
- **FR-59** — Terraform provider (Fase 3).
- **FR-60** — Dashboards/alertas como código (Fase 2).

**Auto-observabilidade e operação**
- **FR-61 [F0]** — Auto-observabilidade: métricas, logs, traces internos, health e painel de saúde.
- **FR-62** — HA e upgrades sem downtime (Fase 3).
- **FR-63 [F0]** — Backup/restore mínimo de metadados no MVP; DR completo na Fase 3.
- **FR-64** — Planejamento de capacidade (Fase 2).

### NonFunctional Requirements

- **NFR-1 [F0]** — Ingestão: 1M eventos/s no benchmark; lag p95 < 10s e p99 < 30s.
- **NFR-2 [F0]** — Consulta: p95 < 1s em 1B linhas/24h; < 3s em 10B/7d; < 3s em 86B/24h no GA com rollups; p99 < 2x p95.
- **NFR-3 [F0]** — Frontend: JS inicial < 180 KB gzip; dashboard < 250 KB; LCP < 1,5s; INP < 200ms; 100k pontos < 100ms.
- **NFR-4** — Alertas: avaliação < 60s; notificação < 90s.
- **NFR-5 [F0]** — Confiabilidade: 99,9% consulta / 99,95% ingestão; RPO 0 para eventos confirmados dentro do buffer/quórum; RTO < 30 min.
- **NFR-6 [F0]** — Segurança: TLS, OIDC, RBAC, row policies, bloqueio de table functions, quotas de consulta.
- **NFR-7 [F0]** — Audit log mínimo (login, permissões, quotas, ingestão) no MVP.
- **NFR-8 [F0]** — Operabilidade: instalação single-node < 15 min; Kubernetes < 1h; upgrade/rollback documentado.
- **NFR-9 [F0]** — Footprint: < 8 GB RAM por gateway; < 64 GB por nó CH/VM no T1; < 4h/semana de operação no T1.
- **NFR-10 [F0]** — Acessibilidade: WCAG 2.2 AA; teclado; tabela alternativa para gráficos; axe no CI.
- **NFR-11 [F0]** — Compatibilidade: OTel 1.x, PromQL/MetricsQL, OpenAPI, Linux x86_64/arm64.
- **NFR-12** — Escalabilidade: federação multi-cluster na Fase 4; 1B/s literal fora do v1.
- **NFR-13 [F0]** — Retenção e custo: 7-15 dias default para logs/traces; compressão ≥ 5x; TCO ≤ 30% da infra equivalente ao Datadog e ≤ 50% total.
- **NFR-14 [F0]** — Privacidade: sem phone-home; telemetria opt-in; redaction de PII.
- **NFR-15 [F0]** — Documentação: quickstart de 5 min, instrumentação, operação e troubleshooting.

### Additional Requirements

- **AR-1** — Stack de referência: Go no gateway, Rust no agente (Fase 2), ClickHouse para logs/traces, VictoriaMetrics para métricas, PostgreSQL para metadados, NATS JetStream para WAL, SvelteKit + uPlot no frontend.
- **AR-2** — Workload de referência: 60% métricas (~40 B), 30% logs (~200 B), 10% traces (~500 B); média ~134 B comprimidos; 1M/s ≈ 11,6 TB/dia.
- **AR-3** — Benchmark reproduzível: harness `bench/`, gerador OTLP, cenários de throughput/query/cardinalidade/caos, relatório Markdown + JSON com hardware, versões e config.
- **AR-4** — Topologia T1: 6-12 ClickHouse, 3 VictoriaMetrics, 4-6 gateways, retenção 7-15 dias.
- **AR-5** — Tenancy: `tenant_id`/`project_id` em toda linha; row policies; filtro obrigatório no Query Service; bloqueio de table functions.
- **AR-6** — Retenção por projeto: validar TTL por coluna ou tabelas por bucket (7/15/30 dias) no spike da Fase 0.
- **AR-7** — Deploy: Docker Compose single-node, Helm alpha para 3 nós, healthchecks, backup/restore de metadados, upgrade/rollback.
- **AR-8** — Open source: Apache 2.0 no núcleo + open-core; DCO; CI com SAST, SBOM e imagens assinadas.
- **AR-9** — Design partners: 3 parceiros com OTel/K8s, 10k+ eventos/s, dados reais e feedback semanal.
- **AR-10** — Frontend: SvelteKit 3 + Svelte 5 + TypeScript + Vite; uPlot; CodeMirror 6; Tailwind 4 + shadcn-svelte/bits-ui.
- **AR-11** — Production readiness baseline: backup/restore de metadados, runbooks, upgrade/rollback testado, auditoria mínima e revisão de segurança antes de produção.
- **AR-12** — ADRs obrigatórias: storage (CH+VM), gateway (Go), fila (NATS), frontend (SvelteKit), tenancy/retention, benchmark methodology.

### UX Design Requirements

**Nenhuma UX-DR formal nesta fase.** A Fase 0 é infraestrutura/benchmark; a única superfície de UI é o shell de validação de performance (Epic 6). A spec de UX (`bmad-ux`) será criada depois do Gate G1 e antes da Fase 1.

- **UX-DR-0 (futuro)** — Explore, dashboards, navegação, estados e acessibilidade serão especificados no `bmad-ux`; o shell da Fase 0 não deve criar dívida visual.

### FR Coverage Map (Fase 0)

| FR | Epic da Fase 0 | Observação |
|---|---|---|
| FR-1 | Epic 2, Epic 4 | Ingestão OTLP usada no benchmark e no gateway |
| FR-2 | Epic 2 | Scrape estático/remote write no benchmark |
| FR-4 | Epic 2, Epic 4 | WAL, at-least-once, dedupe e caos |
| FR-5 | Epic 4 | Quotas e isolamento de ingestão |
| FR-7 | Epic 2, Epic 3 | MetricsQL no benchmark de consulta |
| FR-10 | Epic 3 | Guardrails de cardinalidade no VM |
| FR-11 | Epic 3 | Rollups 1m/5m no schema/consulta |
| FR-12 | Epic 2, Epic 3 | Busca de logs no benchmark |
| FR-15 | Epic 3 | Correlação log-trace mínima no spike de traces |
| FR-17 | Epic 2, Epic 3 | Busca de traces no benchmark |
| FR-19 | Epic 3 | Service map derivado no spike de traces |
| FR-23 | Epic 6 | Shell de Explore (mock) |
| FR-24 | Epic 6 | Shell de dashboard (mock) |
| FR-27 | Epic 6 | uPlot/virtualização/bundle |
| FR-30 | Epic 5 | Health/alertas internos básicos |
| FR-31 | Epic 5 | Canal de teste para alertas internos |
| FR-32 | Epic 5 | Estado/dedup básico no esqueleto |
| FR-36 | Epic 3 | Correlação mínima no spike de traces |
| FR-40 | Epic 3 | TTL/retenção por projeto |
| FR-45 | Epic 4, Epic 5 | Organizações/projetos no esqueleto |
| FR-46 | Epic 4 | Auth/token no gateway |
| FR-47 | Epic 4 | RBAC/tokens no esqueleto |
| FR-49 | Epic 5 | Audit mínimo |
| FR-50 | Epic 4 | Redaction no pipeline (spike) |
| FR-52 | Epic 2, Epic 4 | Compatibilidade OTel Collector |
| FR-53 | Epic 5 | Compose/Helm/single-node |
| FR-57 | Epic 5 | API/OpenAPI no esqueleto |
| FR-61 | Epic 5 | Auto-observabilidade |
| FR-63 | Epic 5 | Backup/restore mínimo |

**Fora da Fase 0:** FR-3, FR-6, FR-8, FR-9, FR-13, FR-14, FR-16, FR-18, FR-20, FR-21, FR-22, FR-25, FR-26, FR-28, FR-29, FR-33, FR-34, FR-35, FR-37, FR-38, FR-39, FR-41, FR-42, FR-43, FR-44, FR-48, FR-51, FR-54, FR-55, FR-56, FR-58, FR-59, FR-60, FR-62, FR-64. Eles permanecem no PRD e serão decompostos nas Fases 1-4. Eles permanecem no PRD e serão decompostos nas Fases 1-4.

---

## Epic List

### Epic 1: Fundação do repositório e governança
**Goal:** um contribuidor consegue clonar, construir, testar e publicar uma release alpha com licença e governança claras.
**FRs/NFRs:** AR-1, AR-8, AR-12; NFR-11, NFR-15.
**Stories:** 4.

### Epic 2: Benchmark reproduzível do workload de referência
**Goal:** Léo consegue rodar 100k/1M/5M eventos/s e gerar um relatório confiável para decidir go/no-go.
**FRs/NFRs:** FR-1, FR-2, FR-4, FR-7, FR-12, FR-17, FR-52; NFR-1, NFR-2, NFR-13; AR-2, AR-3, AR-4.
**Stories:** 4.

### Epic 3: Spikes de armazenamento, consulta e tenancy
**Goal:** o time prova que ClickHouse + VictoriaMetrics atingem as metas de ingestão, consulta, compressão, retenção e isolamento.
**FRs/NFRs:** FR-7, FR-10, FR-11, FR-12, FR-17, FR-19, FR-36, FR-40; NFR-1, NFR-2, NFR-5, NFR-6, NFR-13; AR-5, AR-6.
**Stories:** 5.

### Epic 4: Spike do gateway de ingestão e WAL
**Goal:** o time prova que o gateway + NATS + writers sustentam 1M/s com zero perda para eventos confirmados.
**FRs/NFRs:** FR-1, FR-4, FR-5, FR-45, FR-46, FR-47, FR-50, FR-52; NFR-1, NFR-5, NFR-6; AR-1, AR-5.
**Stories:** 5.

### Epic 5: Esqueleto de deploy, operação e API
**Goal:** Ravi sobe o Observaki single-node em < 15 min; Ana consegue instalar em Kubernetes, fazer upgrade/rollback e ver a saúde do sistema.
**FRs/NFRs:** FR-30, FR-31, FR-32, FR-45, FR-49, FR-53, FR-57, FR-61, FR-63; NFR-7, NFR-8, NFR-9, NFR-15; AR-7, AR-11.
**Stories:** 5.

### Epic 6: Spike de frontend e orçamento de performance
**Goal:** o time prova que SvelteKit + uPlot entregam a experiência rápida dentro do orçamento de bundle.
**FRs/NFRs:** FR-23, FR-24, FR-27; NFR-3, NFR-10; AR-10.
**Stories:** 4.

### Epic 7: Design partners e validação inicial
**Goal:** Kaike tem 3 design partners comprometidos, com dados reais e um ciclo de feedback semanal.
**FRs/NFRs:** NFR-15; AR-9.
**Stories:** 4.

---

## Epic 1: Fundação do repositório e governança

**Goal:** um contribuidor consegue clonar, construir, testar e publicar uma release alpha com licença e governança claras.

### Story 1.1: Bootstrap do mono-repo e ambiente de desenvolvimento

As a contributor,
I want to clone the repository and run the full local toolchain with one command,
So that I can start contributing in less than 15 minutes.

**Depends on:** none.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Estrutura e bootstrap**
**Given** a fresh clone of the repository
**When** I run `make bootstrap`
**Then** the Go, Node and Rust toolchains are installed or validated
**And** the directory structure `cmd/`, `internal/`, `web/`, `bench/`, `deploy/`, `docs/` exists with README files.

**AC2 — Build e testes**
**Given** the bootstrapped environment
**When** I run `make test` and `make build`
**Then** unit tests and the development build pass
**And** the commands return a non-zero exit code on failure.

**AC3 — Documentação**
**Given** a new contributor
**When** I open `README.md`
**Then** I see prerequisites, bootstrap, test, build and run instructions
**And** the instructions are validated on Linux and macOS.

**Task breakdown:** [`stories/story-1.1-tasks.md`](stories/story-1.1-tasks.md) — 9 tarefas, 5,25 dev-days.

### Story 1.2: CI de qualidade e release alpha assinada

As a maintainer,
I want every pull request to run lint, tests, build, SAST and SBOM checks and every tag to produce signed artifacts,
So that the project has a reliable quality gate and a reproducible alpha release.

**Depends on:** 1.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Pull request gate**
**Given** a pull request with a failing test or lint error
**When** CI runs
**Then** the merge is blocked
**And** the failing job is visible with logs.

**AC2 — Security and supply chain**
**Given** a pull request or release build
**When** CI completes
**Then** SAST, dependency scan and SBOM generation run
**And** critical findings fail the pipeline.

**AC3 — Alpha release**
**Given** a tag `v0.0.1-alpha`
**When** the release workflow runs
**Then** multi-arch container images and binaries are built, signed and published
**And** the SBOM and checksums are attached to the release.

### Story 1.3: Governança open source e fronteira open-core

As a contributor or adopter,
I want clear license, contribution and security policies,
So that I know how to use, contribute and report vulnerabilities.

**Depends on:** 1.1.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Arquivos de governança**
**Given** the repository root
**When** I inspect it
**Then** `LICENSE` (Apache 2.0), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, issue templates and PR template exist.

**AC2 — DCO**
**Given** a pull request without a Signed-off-by line
**When** CI runs
**Then** the DCO check fails with an actionable message.

**AC3 — Fronteira open-core**
**Given** the governance documentation
**When** I read it
**Then** the open-core principles are documented
**And** the list of candidate enterprise features is explicitly marked as pending business decision.

### Story 1.4: ADRs e registro de decisões de arquitetura

As an engineer,
I want the key architecture decisions recorded as ADRs,
So that the team can review, challenge and revisit them without relying on memory.

**Depends on:** 1.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Template e processo**
**Given** a new architecture decision
**When** I create an ADR from the template
**Then** it contains context, decision, alternatives, consequences and status.

**AC2 — ADRs obrigatórias**
**Given** the Phase 0 scope
**When** I list the ADRs
**Then** at least these exist: storage (ClickHouse + VictoriaMetrics), gateway (Go), queue/WAL (NATS), frontend (SvelteKit + uPlot), tenancy/retention, benchmark methodology.

**AC3 — Revisão**
**Given** each ADR
**When** it is marked accepted
**Then** it was reviewed by at least one other engineer
**And** open questions are linked to Phase 0 spikes.

---

## Epic 2: Benchmark reproduzível do workload de referência

**Goal:** Léo consegue rodar 100k/1M/5M eventos/s e gerar um relatório confiável para decidir go/no-go.

### Story 2.1: Gerador de carga OTLP no workload de referência

As a performance engineer,
I want to generate a deterministic OTLP workload with the 60/30/10 signal mix,
So that every benchmark run is comparable and reproducible.

**Depends on:** 1.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Mix e payloads**
**Given** the workload config
**When** I run the generator
**Then** it produces 60% metrics (~40 B), 30% logs (~200 B) and 10% traces (~500 B)
**And** the observed average compressed size is within ±10% of 134 B.

**AC2 — Taxa configurável**
**Given** a target rate of 100k, 1M or 5M events/s
**When** the generator runs for 5 minutes
**Then** the achieved rate is within ±5% of the target
**And** backpressure is reported instead of silently dropping events.

**AC3 — Determinismo e observabilidade**
**Given** the same seed and config
**When** I run the generator twice
**Then** the event count and distribution are identical
**And** the generator exposes its own rate, errors and latency metrics.

### Story 2.2: Cenários de benchmark e datasets de consulta

As a performance engineer,
I want predefined scenarios for throughput, query, cardinality and chaos,
So that I can run the same benchmark matrix on any cluster.

**Depends on:** 2.1.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Cenários**
**Given** the `bench/` directory
**When** I list the scenarios
**Then** I find throughput (100k/1M/5M), query (1B/10B/86B), cardinality (100k/1M/10M series) and chaos (ClickHouse/NATS/gateway failure).

**AC2 — Execução**
**Given** a scenario name
**When** I run `bench/run.sh <scenario>`
**Then** it validates prerequisites, starts the workload and writes results to a timestamped folder.

**AC3 — Documentação**
**Given** the benchmark README
**When** I read it
**Then** each scenario documents hardware assumptions, duration, metrics collected and expected output.

### Story 2.3: Coleta de métricas e gerador de relatório

As a decision-maker,
I want a report with throughput, latency, resource usage and compression,
So that I can compare configurations and make a go/no-go decision.

**Depends on:** 2.1, 2.2.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Métricas coletadas**
**Given** a completed benchmark run
**When** the report is generated
**Then** it includes events/s, bytes/s, lag p50/p95/p99, CPU, RAM, disk, compression ratio, query p50/p95/p99, error rate and dropped events.

**AC2 — Formato**
**Given** the same run
**When** I inspect the output
**Then** I find both `report.md` and `report.json`
**And** the JSON contains hardware, versions, config hash and raw results.

**AC3 — Reprodutibilidade**
**Given** the report
**When** another engineer reruns the same config and commit
**Then** the results are within the documented variance
**And** the variance is stated in the report.

### Story 2.4: Ambiente reproduzível e baseline publicado

As a performance engineer,
I want a scripted environment and a published baseline,
So that the benchmark is not a one-off laptop experiment.

**Depends on:** 2.2, 2.3.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Provisionamento**
**Given** a cloud account or local machine with the documented prerequisites
**When** I run the provisioning script
**Then** the T1 reference cluster (6-12 ClickHouse, 3 VictoriaMetrics, 4-6 gateways) is created
**And** the script is idempotent.

**AC2 — Baseline**
**Given** the provisioned cluster
**When** I run the baseline scenario
**Then** the results are published in `BENCHMARKS.md` with commit, hardware and config.

**AC3 — Custo e limpeza**
**Given** the benchmark is finished
**When** I run the cleanup command
**Then** all resources are destroyed
**And** the estimated cost of the run is reported.

---

## Epic 3: Spikes de armazenamento, consulta e tenancy

**Goal:** o time prova que ClickHouse + VictoriaMetrics atingem as metas de ingestão, consulta, compressão, retenção e isolamento.

### Story 3.1: Schema ClickHouse para logs

As a storage engineer,
I want a logs table optimized for tenant, service and time queries,
So that log search meets the latency target and compresses well.

**Depends on:** 2.2.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Schema**
**Given** the logs DDL
**When** I inspect it
**Then** it contains `tenant_id`, `project_id`, `timestamp`, `ingested_at`, `service_name`, `severity`, `body`, `attributes`, `resource_attributes`, `trace_id`, `span_id` and `event_id`
**And** it is partitioned/TTL by `ingested_at` and ordered by tenant, project, service and time.

**AC2 — Ingestão e compressão**
**Given** 1B synthetic log rows
**When** I ingest and query by service + 1h window
**Then** compression is ≥ 5x
**And** query p95 is < 1s on the benchmark dataset.

**AC3 — Retenção**
**Given** a 7-day and a 15-day retention bucket
**When** the TTL/retention job runs
**Then** expired data is removed from hot storage
**And** the chosen strategy (column TTL or bucket tables) is documented.

### Story 3.2: Schema ClickHouse para traces

As a storage engineer,
I want a traces table optimized for trace ID lookup and service analysis,
So that trace search and waterfall queries meet their targets.

**Depends on:** 3.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Schema**
**Given** the traces DDL
**When** I inspect it
**Then** it contains `trace_id`, `span_id`, `parent_span_id`, `service_name`, `operation_name`, `duration_ns`, `status_code`, `attributes`, `events`, `links`, `event_id` and `ingested_at`
**And** it has indexes for trace ID and service; RED metrics are not stored here (AD-21).

**AC2 — Consulta por trace**
**Given** 1B synthetic spans
**When** I query a trace by ID
**Then** the full trace is returned in < 1s.

**AC3 — Consulta por serviço**
**Given** the same dataset
**When** I query spans by service, status and 24h window
**Then** p95 is < 2s
**And** compression is ≥ 5x.

**AC4 — Correlação mínima e service map**
**Given** a log with `trace_id` and the corresponding spans
**When** I navigate from the log to the trace and back
**Then** the correlation works in both directions
**And** a basic service map with nodes/edges is derived from the spans.

### Story 3.3: Caminho de ingestão ClickHouse com dedupe

As a storage engineer,
I want batched async inserts with event_id deduplication,
So that 1M/s ingestion is stable and duplicate retries are invisible.

**Depends on:** 3.1, 3.2.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Throughput**
**Given** the T1 cluster and 1M events/s workload
**When** ingestion runs for 1 hour
**Then** ClickHouse sustains the rate with lag p95 < 10s
**And** no `Too many parts` error occurs.

**AC2 — Dedupe**
**Given** the same `event_id` is inserted twice
**When** both writes complete
**Then** the query returns one logical row
**And** a dedupe metric counts the duplicate.

**AC3 — Recuperação**
**Given** a writer restart during ingestion
**When** it resumes
**Then** it continues from the last acknowledged batch
**And** no confirmed event is lost.

### Story 3.4: VictoriaMetrics, MetricsQL e cardinalidade

As a storage engineer,
I want to validate VictoriaMetrics for metrics ingestion and query,
So that metrics meet the latency target without a separate query engine.

**Depends on:** 3.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Ingestão**
**Given** 1M samples/s and 1M active series
**When** ingestion runs for 1 hour
**Then** the ingestion rate is sustained
**And** memory stays within the documented footprint.

**AC2 — Consulta**
**Given** the metrics dataset
**When** I run 20 representative MetricsQL queries
**Then** p95 is < 1s for 24h windows
**And** `histogram_quantile`, `rate` and label aggregation return correct results.

**AC3 — Cardinalidade**
**Given** a tenant exceeding its series limit
**When** it sends more series
**Then** the limit is enforced with a clear error/metric
**And** other tenants are unaffected.

### Story 3.5: Benchmark de consulta, retenção e row policies

As a security and performance engineer,
I want to validate query latency, per-project retention and tenant isolation,
So that the storage design is safe and fast enough for the MVP.

**Depends on:** 3.1, 3.2, 3.3, 3.4.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Latência**
**Given** 1B, 10B and 86B datasets
**When** I run the 20 dashboard queries
**Then** p95 meets the PRD targets (1s/3s/3s with rollups)
**And** p99 is < 2x p95.

**AC2 — Isolamento**
**Given** two projects in different tenants
**When** I run a query as tenant A
**Then** no row from tenant B is returned
**And** a direct SQL attempt with a table function is blocked.

**AC3 — Retenção**
**Given** projects with 7, 15 and 30-day retention
**When** the retention job runs
**Then** each project's data expires according to its policy
**And** the chosen strategy is documented with cost implications.

---

## Epic 4: Spike do gateway de ingestão e WAL

**Goal:** o time prova que o gateway + NATS + writers sustentam 1M/s com zero perda para eventos confirmados.

### Story 4.1: Skeleton do gateway OTLP

As a platform engineer,
I want a Go gateway that accepts OTLP gRPC/HTTP for all three signals,
So that any OTel SDK or Collector can send telemetry without a proprietary agent.

**Depends on:** 1.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Recepção**
**Given** the gateway is running
**When** the OTel demo sends metrics, logs and traces via OTLP gRPC and HTTP
**Then** all three signals are accepted
**And** `partial_success` reports rejected counts with reasons.

**AC2 — Limites**
**Given** a batch larger than 10 MB or 10k events
**When** it is sent
**Then** the gateway rejects it with a clear error
**And** the limit is configurable.

**AC3 — Operação**
**Given** a SIGTERM
**When** the gateway shuts down
**Then** it stops accepting new requests, drains in-flight batches and exits cleanly.

**AC4 — Redaction**
**Given** a pipeline rule that masks a test credit-card field
**When** a log containing that field is ingested
**Then** the field is masked before the WAL
**And** the trace ID correlation is preserved.

### Story 4.2: Auth por tenant, quotas e rate limiting

As a platform engineer,
I want tenant-scoped tokens and rate limits,
So that one noisy tenant cannot starve the others.

**Depends on:** 4.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Auth**
**Given** an invalid or expired token
**When** I send telemetry
**Then** the gateway returns 401
**And** no data is stored.

**AC2 — Isolamento**
**Given** a token for project A
**When** it attempts to write to project B
**Then** the write is rejected with 403
**And** the attempt is audited.

**AC3 — Quota**
**Given** a tenant with a 100k/s quota
**When** it sends 1M/s
**Then** excess traffic receives 429 with `Retry-After`
**And** the other tenants' p95 latency changes by less than 10%.

### Story 4.3: Batching, compressão, sharding e WAL NATS

As a platform engineer,
I want the gateway to batch, compress, shard and persist events in NATS,
So that ingestion survives downstream failures without losing confirmed data.

**Depends on:** 4.1, 4.2.
**Size:** L.

**Acceptance Criteria:**

**AC1 — Throughput**
**Given** the T1 gateway fleet and 1M/s workload
**When** ingestion runs for 1 hour
**Then** the gateway accepts 1M/s with p99 processing < 50ms
**And** the WAL backlog stays bounded.

**AC2 — Durabilidade**
**Given** a gateway restart during ingestion
**When** it comes back
**Then** no confirmed event is lost
**And** duplicates are deduplicated by `event_id`.

**AC3 — Sharding**
**Given** multiple tenants and signals
**When** events are published
**Then** subjects are sharded by tenant/signal/shard
**And** a single tenant cannot monopolize a subject.

### Story 4.4: Writers para ClickHouse e VictoriaMetrics

As a platform engineer,
I want consumers that write batches to ClickHouse and VictoriaMetrics with retry,
So that the end-to-end pipeline is complete and observable.

**Depends on:** 4.3, 3.3, 3.4.
**Size:** M.

**Acceptance Criteria:**

**AC1 — End-to-end**
**Given** 1M/s input
**When** writers run for 1 hour
**Then** the stored event count matches the accepted count (minus dedupe)
**And** end-to-end lag p95 is < 10s.

**AC2 — Retry**
**Given** a transient ClickHouse or VM error
**When** the writer retries
**Then** it uses exponential backoff
**And** no data is lost within the WAL window.

**AC3 — Observabilidade**
**Given** the writers are running
**When** I inspect metrics
**Then** I see accepted, written, failed, retried, deduped and lag metrics per tenant/signal.

### Story 4.5: Teste de caos e go/no-go da stack de ingestão

As a decision-maker,
I want a chaos test that proves durability and a go/no-go recommendation,
So that we know whether the gateway/NATS/ClickHouse stack is safe for the MVP.

**Depends on:** 4.3, 4.4.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Falha de storage**
**Given** steady ingestion at 1M/s
**When** ClickHouse is stopped for 15 minutes
**Then** the gateway applies backpressure/WAL
**And** after recovery, zero confirmed events are lost.

**AC2 — Falha de gateway/NATS**
**Given** a gateway or NATS node failure
**When** the cluster recovers
**Then** ingestion resumes automatically
**And** duplicates are within the documented bound.

**AC3 — Recomendação**
**Given** the chaos results
**When** the report is written
**Then** it recommends Go, Rust/Rotel or OTel Collector for the hot path
**And** lists the conditions for the recommendation.

---

## Epic 5: Esqueleto de deploy, operação e API

**Goal:** Ravi sobe o Observaki single-node em < 15 min; Ana consegue instalar em Kubernetes, fazer upgrade/rollback e ver a saúde do sistema.

### Story 5.1: Docker Compose single-node

As a developer,
I want to start the full stack with `docker compose up`,
So that I can evaluate Observaki without a Kubernetes cluster.

**Depends on:** 4.1, 3.1, 1.2.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Subida**
**Given** Docker and Docker Compose installed
**When** I run `docker compose up`
**Then** Observaki, ClickHouse, VictoriaMetrics, PostgreSQL and NATS start with healthchecks
**And** the API/UI is reachable in < 10 minutes.

**AC2 — Dados de exemplo**
**Given** the running stack
**When** I run the sample app
**Then** I see metrics, logs and traces in the UI
**And** the quickstart is validated on Linux and macOS.

**AC3 — Ciclo de vida**
**Given** the stack is running
**When** I run `docker compose down` and `up` again
**Then** data persists in volumes
**And** no manual migration is required.

### Story 5.2: Helm chart alpha para cluster de 3 nós

As a platform engineer,
I want to install Observaki on Kubernetes with a Helm chart,
So that I can validate a production-like deployment.

**Depends on:** 5.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Instalação**
**Given** a 3-node Kubernetes cluster
**When** I run `helm install observaki ./chart`
**Then** the gateway, query/API, control plane and dependencies become ready
**And** the values file documents CPU/memory/storage.

**AC2 — Upgrade/rollback**
**Given** a running release
**When** I upgrade to a new chart version and then rollback
**Then** the pods return to the previous version
**And** no data is lost.

**AC3 — Documentação**
**Given** the chart README
**When** I read it
**Then** I see prerequisites, values, sizing, upgrade and troubleshooting.

### Story 5.3: Quickstart, sample app e documentação de instrumentação

As a developer,
I want a 5-minute quickstart with a sample app,
So that I can see my first telemetry without reading the whole docs.

**Depends on:** 5.1.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Quickstart**
**Given** a fresh machine
**When** I follow the quickstart
**Then** I have Observaki running and the sample app sending data in < 15 minutes.

**AC2 — Instrumentação**
**Given** the docs
**When** I look for Go, Python and Node.js examples
**Then** each example includes the OTLP endpoint, auth token and a minimal snippet.

**AC3 — Troubleshooting**
**Given** a common failure (wrong endpoint, invalid token, no data)
**When** I read the troubleshooting section
**Then** I find the symptom, cause and fix.

### Story 5.4: Backup/restore, upgrade/rollback e auto-observabilidade

As an operator,
I want metadata backup/restore and health visibility,
So that I can safely run a pilot and recover from mistakes.

**Depends on:** 5.2, 5.3.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Backup/restore**
**Given** a running deployment with metadata
**When** I run the backup and restore procedure
**Then** projects, dashboards, alert rules and users are restored in < 30 minutes.

**AC2 — Upgrade/rollback**
**Given** a versioned deployment
**When** I follow the upgrade and rollback runbook
**Then** the procedure is reproducible
**And** the rollback returns to the previous version in < 15 minutes.

**AC3 — Auto-observabilidade**
**Given** the running stack
**When** I open the health page
**Then** I see ingestion lag, query error rate, storage usage and component health
**And** an internal alert fires when ingestion lag exceeds the threshold.

**AC4 — Alerta mínimo**
**Given** a threshold rule on ingestion lag
**When** the condition is met and then resolved
**Then** the alert changes state, is deduplicated and can be silenced
**And** a Slack/webhook test channel receives the notification.

### Story 5.5: API mínima versionada e OpenAPI

As a developer,
I want a minimal versioned API with an OpenAPI contract,
So that the CLI, UI and future integrations can be built on a stable surface.

**Depends on:** 4.1, 4.2.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Contrato**
**Given** the API service
**When** I fetch `/openapi.json`
**Then** it describes the MVP endpoints for health, projects, ingestion tokens and a sample query
**And** the spec is validated in CI.

**AC2 — Versionamento e erros**
**Given** a request to `/api/v1/...`
**When** the input is invalid or unauthorized
**Then** the API returns a structured error with code, message and detail
**And** no internal stack trace is leaked.

**AC3 — Compatibilidade**
**Given** a breaking change proposal
**When** it is reviewed
**Then** it follows the versioning/deprecation policy
**And** the change is documented in the OpenAPI changelog.

---

## Epic 6: Spike de frontend e orçamento de performance

**Goal:** o time prova que SvelteKit + uPlot entregam a experiência rápida dentro do orçamento de bundle.

### Story 6.1: Shell SvelteKit, design tokens e navegação

As a frontend engineer,
I want an app shell with navigation, project switcher and states,
So that the UI foundation is ready for Explore and dashboards.

**Depends on:** 1.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Shell**
**Given** the frontend app
**When** I open it
**Then** I see navigation for Explore, Dashboards, Alerts and Settings
**And** a project switcher with at least two mock projects.

**AC2 — Estados**
**Given** a loading, empty, error or timeout condition
**When** the state is triggered
**Then** the UI shows an actionable message
**And** the layout does not shift.

**AC3 — Acessibilidade**
**Given** keyboard-only navigation
**When** I tab through the shell
**Then** all interactive elements are reachable
**And** focus is visible.

### Story 6.2: Componente uPlot com 100k pontos

As a frontend engineer,
I want a time-series chart that renders 100k points quickly,
So that large dashboards feel instant.

**Depends on:** 6.1.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Render**
**Given** 100k points already loaded
**When** the chart renders
**Then** the first paint is < 100ms on the reference laptop.

**AC2 — Interação**
**Given** the chart is rendered
**When** I zoom, pan and hover
**Then** interaction stays at 60fps
**And** the tooltip shows the correct timestamp/value.

**AC3 — Atualização**
**Given** a live data update at 1 Hz
**When** new points arrive
**Then** the chart updates without recreating the whole plot
**And** memory stays stable for 10 minutes.

### Story 6.3: Tabela virtualizada de 10k linhas

As a frontend engineer,
I want a virtualized log/trace table,
So that large result sets are usable without freezing the browser.

**Depends on:** 6.1.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Render**
**Given** 10k rows
**When** the table renders
**Then** only the visible rows are in the DOM
**And** scrolling stays at 60fps.

**AC2 — Colunas e seleção**
**Given** the table
**When** I resize columns, sort and select a row
**Then** the state is preserved
**And** the selected row can open a detail panel.

**AC3 — Estados**
**Given** an empty result or an error
**When** the table renders
**Then** it shows a clear empty/error state
**And** the user can retry.

### Story 6.4: Orçamento de bundle no CI e Explore mock

As a frontend lead,
I want a CI budget and a mock Explore screen,
So that performance regressions are caught before the MVP.

**Depends on:** 6.1, 6.2, 6.3.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Budget**
**Given** a production build
**When** CI runs
**Then** initial JS is < 180 KB gzip and the dashboard route is < 250 KB
**And** a regression fails the build.

**AC2 — Lighthouse**
**Given** the reference laptop and 50 Mbps network
**When** Lighthouse runs on the Explore route
**Then** LCP < 1.5s and INP < 200ms.

**AC3 — Mock Explore**
**Given** the mock Explore screen
**When** I switch between metrics, logs and traces
**Then** the time range and service filter persist
**And** the query bar supports autocomplete with mock data.

---

## Epic 7: Design partners e validação inicial

**Goal:** Kaike tem 3 design partners comprometidos, com dados reais e um ciclo de feedback semanal.

### Story 7.1: Pipeline de recrutamento de design partners

As a founder,
I want a pipeline of candidate design partners,
So that Phase 0 ends with 3 committed partners, not just interested conversations.

**Depends on:** none.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Lista**
**Given** the ICP (startups/times pequenos, OTel, K8s)
**When** I open the pipeline
**Then** it has at least 20 qualified candidates with contact, stack, volume and status.

**AC2 — Outreach**
**Given** a candidate
**When** I send the outreach
**Then** the message explains the value, the commitment and the data handling
**And** the pipeline tracks reply, call and decision.

**AC3 — Meta**
**Given** the Phase 0 timeline
**When** the phase ends
**Then** at least 5 calls happened and 3 partners signed.

### Story 7.2: Critérios de seleção, acordo e tratamento de dados

As a founder,
I want clear selection criteria and a data-handling agreement,
So that design partners are a good fit and their data is safe.

**Depends on:** 7.1.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Critérios**
**Given** a candidate
**When** I evaluate it
**Then** I check: uses OTel, runs Kubernetes, has ≥ 10k events/s, can share data, can give weekly feedback.

**AC2 — Acordo**
**Given** a selected partner
**When** the agreement is signed
**Then** it covers scope, data retention, anonymization, feedback cadence and exit.

**AC3 — Baseline**
**Given** the partner's environment
**When** onboarding starts
**Then** we capture current tools, volume, cost and top 3 pain points.

### Story 7.3: Onboarding e primeira instrumentação

As a design partner,
I want a guided onboarding,
So that I can send real telemetry and see value within one week.

**Depends on:** 7.2, 5.3.
**Size:** M.

**Acceptance Criteria:**

**AC1 — Sessão**
**Given** a partner
**When** the 60-minute onboarding session happens
**Then** the partner sends at least one signal to Observaki
**And** the first dashboard is created.

**AC2 — Checklist**
**Given** the onboarding guide
**When** the partner follows it
**Then** endpoint, token, collector config, retention and alerts are validated.

**AC3 — Suporte**
**Given** a blocker
**When** the partner reports it
**Then** it is triaged within 1 business day
**And** the resolution is documented.

### Story 7.4: Cadência de feedback e demo da Fase 0

As a product manager,
I want a weekly feedback loop and a Phase 0 demo,
So that the benchmark and product decisions are grounded in real partner needs.

**Depends on:** 7.3.
**Size:** S.

**Acceptance Criteria:**

**AC1 — Cadência**
**Given** 3 partners
**When** the phase runs
**Then** there are at least 4 weekly 30-minute sessions
**And** notes are recorded in the shared backlog.

**AC2 — Priorização**
**Given** the feedback
**When** the phase ends
**Then** the top 5 pain points are ranked by frequency and impact
**And** each has a proposed Phase 1 response.

**AC3 — Demo**
**Given** the Phase 0 results
**When** the demo happens
**Then** it shows the benchmark, the deploy skeleton and the partner feedback
**And** the go/no-go decision is recorded.

---

## Story Map

| Story | Title | Epic | Size | Depends on |
|---|---|---|---|---|
| 1.1 | Bootstrap do mono-repo | 1 | S | — |
| 1.2 | CI e release alpha | 1 | M | 1.1 |
| 1.3 | Governança open source | 1 | S | 1.1 |
| 1.4 | ADRs | 1 | M | 1.1 |
| 2.1 | Gerador OTLP | 2 | M | 1.1 |
| 2.2 | Cenários de benchmark | 2 | S | 2.1 |
| 2.3 | Relatório de benchmark | 2 | M | 2.1, 2.2 |
| 2.4 | Ambiente + baseline | 2 | M | 2.2, 2.3 |
| 3.1 | Schema de logs | 3 | M | 2.2 |
| 3.2 | Schema de traces | 3 | M | 3.1 |
| 3.3 | Ingestão ClickHouse + dedupe | 3 | M | 3.1, 3.2 |
| 3.4 | VictoriaMetrics + cardinalidade | 3 | M | 3.1 |
| 3.5 | Query/retenção/row policies | 3 | M | 3.1-3.4 |
| 4.1 | Gateway OTLP | 4 | M | 1.1 |
| 4.2 | Auth/quotas | 4 | M | 4.1 |
| 4.3 | Batching/WAL NATS | 4 | L | 4.1, 4.2 |
| 4.4 | Writers CH/VM | 4 | M | 4.3, 3.3, 3.4 |
| 4.5 | Caos + go/no-go | 4 | M | 4.3, 4.4 |
| 5.1 | Docker Compose | 5 | M | 4.1, 3.1, 1.2 |
| 5.2 | Helm chart | 5 | M | 5.1 |
| 5.3 | Quickstart + sample app | 5 | S | 5.1 |
| 5.4 | Backup/restore + auto-observabilidade | 5 | M | 5.2, 5.3 |
| 5.5 | API mínima + OpenAPI | 5 | S | 4.1, 4.2 |
| 6.1 | Shell SvelteKit | 6 | M | 1.1 |
| 6.2 | uPlot 100k pontos | 6 | M | 6.1 |
| 6.3 | Tabela virtualizada | 6 | S | 6.1 |
| 6.4 | Bundle CI + Explore mock | 6 | M | 6.1-6.3 |
| 7.1 | Pipeline de design partners | 7 | S | — |
| 7.2 | Critérios/acordo | 7 | S | 7.1 |
| 7.3 | Onboarding | 7 | M | 7.2, 5.3 |
| 7.4 | Feedback + demo | 7 | S | 7.3 |

---

## Phase 0 Delivery Plan (4 semanas)

### Semana 1 — Fundação e arranque
- 1.1 bootstrap, 1.2 CI, 1.3 governança, 1.4 ADRs.
- 2.1 gerador OTLP e 2.2 cenários.
- 3.1 schema de logs e 4.1 gateway skeleton.
- 7.1 pipeline de design partners.
- **Demo semanal:** repo builds, CI verde, primeiro trace no gateway.

### Semana 2 — Spikes principais
- 2.3 relatório, 3.2 traces, 3.3 ingestão CH, 3.4 VictoriaMetrics.
- 4.2 auth/quotas, 4.3 batching/WAL.
- 5.1 Compose, 6.1 shell SvelteKit.
- 7.2 critérios/acordo.
- **Demo semanal:** 100k/s end-to-end; Compose sobe; shell renderiza.

### Semana 3 — Escala e integração
- 2.4 baseline, 3.5 query/retenção, 4.4 writers.
- 5.2 Helm, 5.3 quickstart, 5.5 API mínima/OpenAPI.
- 6.2 uPlot, 6.3 tabela.
- 7.3 onboarding do primeiro partner.
- **Demo semanal:** 1M/s; 1B/10B query; Helm instala.

### Semana 4 — Caos, evidência e decisão
- 4.5 caos, 5.4 backup/auto-observabilidade, 6.4 bundle CI.
- 7.4 feedback/demo.
- Rodar o benchmark final, publicar `BENCHMARKS.md`, fechar ADRs.
- **Demo final:** 1M/s com relatório, 3 design partners, go/no-go.

---

## Phase 0 Exit Criteria

A Fase 0 só termina quando todos os critérios abaixo forem verdadeiros:

1. **Benchmark:** 1M eventos/s por 1 hora no workload de referência, lag p95 < 10s e zero perda para eventos confirmados.
2. **Consulta:** p95 < 1s em 1B linhas/24h; < 3s em 10B/7d; plano para 86B/24h com rollups.
3. **Compressão:** ≥ 5x em logs e traces.
4. **Retenção/tenancy:** row policies, retenção por projeto para logs/traces e quotas de consulta validados.
5. **Deploy:** Compose single-node em < 15 min; Helm alpha em 3 nós; upgrade/rollback testado.
6. **Frontend:** 100k pontos < 100ms; bundle inicial < 180 KB gzip; LCP < 1,5s no laboratório.
7. **Design partners:** 3 parceiros comprometidos, com dados reais e feedback semanal.
8. **ADRs:** storage, gateway, fila, frontend, tenancy/retention e benchmark aprovadas.
9. **Go/no-go:** recomendação registrada para cada spike (Go vs Rust/Rotel, ClickHouse+VM, NATS, SvelteKit).

---

## Riscos e decisões Go/No-Go

| Spike | Se falhar | Decisão |
|---|---|---|
| Gateway Go não atinge 1M/s | Migrar hot path para Rust/Rotel | 4.5 |
| NATS não sustenta 1M/s | WAL local + Kafka opcional | 4.5 |
| ClickHouse não atinge query/latência | Ajustar schema/projeções ou considerar store alternativo | 3.5 |
| VictoriaMetrics não cobre cardinalidade/retenção | Ajustar limites ou avaliar ClickHouse para métricas | 3.5 |
| SvelteKit/uPlot estoura o bundle | React + uPlot ou reduzir escopo da UI | 6.4 |
| Design partners não aparecem | Usar dados sintéticos e adiar o gate de produto | 7.4 |
| Retenção por projeto não funciona | Tabelas por bucket de retenção | 3.5 |

---

## Out of Scope (Fase 0)

- Features de usuário completas (Explore real, dashboards reais, alertas reais, auth UI).
- RUM, session replay, sintéticos, profiling, eBPF.
- Agente Rust próprio (Fase 2).
- SSO/SCIM, audit completo, operator, air-gapped, federação.
- Oferta SaaS gerenciada, billing, marketplace.
- Otimização de custo de produção além do benchmark de referência.

---

## Definition of Ready / Done

**Ready:** story com contexto, dependências, critérios de aceitação testáveis e ambiente de benchmark definido.

**Done:**
- Código revisado e mergeado.
- Testes automatizados relevantes passando.
- Documentação/ADR atualizada.
- Métrica de benchmark registrada quando aplicável.
- Demo ou evidência anexada à story.
- Nenhum bug P0 conhecido.

---

## Próximo passo após a Fase 0

Com o Gate G1 aprovado, o próximo artefato é o **backlog da Fase 1 (MVP)**, decomposto a partir dos FRs de ingestão, métricas, logs, traces, Explore, dashboards, alertas, auth e correlação mínima. As ADRs da Fase 0 alimentam `bmad-architecture`; o shell da Fase 0 alimenta `bmad-ux`.
