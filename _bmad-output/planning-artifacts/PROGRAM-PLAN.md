# Observaki — Plano de Programa (ponta a ponta)

**Status:** final para planejamento. Nenhum código de produto foi escrito ainda.
**Data:** 2026-10-31.
**Fontes:** `prds/prd-observaki-2026-10-31/prd.md`, `prds/prd-observaki-2026-10-31/addendum.md`, `architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md`, `ux-designs/ux-observaki-2026-10-31/DESIGN.md`, `ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md`, `epics.md`.

---

## 1. O que este plano cobre

O Observaki é uma plataforma de observabilidade open source, self-hosted e OTel-native. O programa vai de **zero a GA 1.0** em ~14 meses (com buffer), seguido de expansão. Este documento é o mapa executivo: fases, gates, epics, workstreams, staffing, dependências, releases e riscos. O detalhe vive nos artefatos linkados:

| Artefato | O que contém |
|---|---|
| [`PRD`](prds/prd-observaki-2026-10-31/prd.md) | Visão, 64 FRs, NFRs, non-goals, métricas, fases |
| [`DECISIONS.md`](DECISIONS.md) | Decisões confirmadas, pendências e índice de ADs |
| [`addendum.md`](prds/prd-observaki-2026-10-31/addendum.md) | Stack, benchmarks, modelo de dados, TCO, segurança, governança |
| [`ARCHITECTURE-SPINE.md`](architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md) | 35 invariantes (AD-1..AD-35), diagramas, convenções, stack pinada |
| [`DESIGN.md`](ux-designs/ux-observaki-2026-10-31/DESIGN.md) | Identidade visual, tokens, tipografia, componentes |
| [`EXPERIENCE.md`](ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md) | IA, fluxos, estados, atalhos, acessibilidade |
| [`epics.md`](epics.md) | Fase 0 detalhada: 7 epics, 31 stories, AC Given/When/Then |
| [`epics-phase-1.md`](epics-phase-1.md) | Fase 1 detalhada: epics + stories + AC resumidos |
| [`epics-phase-2-4.md`](epics-phase-2-4.md) | Fases 2-4: epics + stories em nível de planejamento |

---

## 2. Estratégia

**Uma plataforma, três promessas:** (1) **rápida** — 1M eventos/s no benchmark, p95 < 1s em 1B linhas, frontend < 180 KB; (2) **leve** — agente Rust, Go no gateway, ClickHouse + VictoriaMetrics, sem JVM/Elasticsearch/Kafka no caminho padrão; (3) **operável por time pequeno** — single-node em < 15 min, Helm em 1h, menos de 4h/semana no T1.

**Sequência de risco:** primeiro provar a stack (Fase 0), depois entregar a fatia vertical que prova valor (Fase 1: três sinais + correlação + dashboards + alertas), depois profundidade de APM/SLOs (Fase 2), depois escala/enterprise/GA (Fase 3), depois expansão de portfólio (Fase 4). Nenhuma fase começa sem o gate anterior.

**Open-core:** o núcleo de observabilidade é Apache 2.0 e funciona sem licença. Recursos enterprise (SSO/SCIM, audit avançado, RBAC granular, federação, retenção por tenant de métricas, suporte) usam entitlement assinado; nenhum caminho de telemetria é paywalled.

---

## 3. Linha do tempo e gates

| Fase | Janela | Objetivo | Gate de saída |
|---|---:|---|---|
| **0. Fundação e benchmarks** | Semanas 0-4 | Provar stack, ADRs, deploy esqueleto, 3 design partners | **G1**: 1M/s, p95 < 1s em 1B, compressão ≥ 5x |
| **1. MVP** | Semanas 5-20 | Três sinais, correlação mínima, Explore, dashboards, alertas, auth, audit mínimo | **G2**: UJ-1 ponta a ponta; 3 pilotos a 100k/s |
| **2. APM e SLOs** | Semanas 21-32 | Service map completo, RED, error tracking, tail sampling, SLOs, agente beta, migração | **G3**: 1M/s em produção assistida; 10 design partners |
| **3. Escala e enterprise** | Semanas 33-46 | HA, multi-tenant, SSO/SCIM, audit, operator, DR, 10M/s, GA | **G4**: 10M/s; 30 dias de soak; 3 tenants; GA 1.0 |
| **4. Expansão** | Semanas 47-62+ | RUM, sintéticos, eBPF, profiling, LLM observability, IA, marketplace | **G5**: ≥ 5 design partners por novo sinal |

**GA de engenharia:** ~semana 46 (~10,5 meses). **Compromisso com buffer de 30%:** ~semana 60 (~14 meses). O buffer cobre contratação, férias, descoberta e correções de benchmark.

**Marcos de release:**
- `v0.1.0-alpha` — fim da Fase 0 (semana 4): stack provada, deploy esqueleto.
- `v0.5.0-beta` — fim da Fase 1 (semana 20): MVP em piloto.
- `v0.8.0-beta` — fim da Fase 2 (semana 32): APM/SLOs, produção assistida.
- `v1.0.0` — fim da Fase 3 (semana 46 + buffer): GA.
- `v1.x` — Fase 4: novos sinais, cada um com gate de demanda.

---

## 4. Workstreams e staffing

| Workstream | Dono | FTE | Responsabilidade |
|---|---|---:|---|
| Produto/PM | Fundador | 1,0 | Visão, priorização, design partners, PRD/backlog, gates |
| Backend/Data | 2 engenheiros Go | 2,0 | Gateway, query, storage, API, alerting |
| Frontend | 1 engenheiro Svelte | 1,0 | Explore, dashboards, services, alerts, design system |
| Platform/SRE | 1 engenheiro | 1,0 | Benchmark, deploy, Helm, HA, DR, observabilidade |
| Agente/Rust | 0,5-1 engenheiro | 0,5-1,0 | Agente (Fase 2+), eBPF (Fase 4) |
| Design/UX | 0,5 designer | 0,5 | DESIGN/EXPERIENCE, mocks, usabilidade |
| Docs/DevRel | 0,5 technical writer/DevRel | 0,5 | Quickstart, docs, conteúdo, comunidade |
| **Total** | | **5,5-6,5** | |

**Gaps assumidos:** sem QA dedicado (engenheiros são donos dos testes), sem security engineer dedicado (pentest externo na Fase 3), sem SRE 24/7 (o produto é self-hosted; o suporte é comunitário/enterprise).

**Rituais:**
- Daily assíncrono de 15 min.
- Demo semanal (sexta) com design partners.
- Release quinzenal a partir da Fase 1.
- Benchmark mensal publicado em `BENCHMARKS.md`.
- Revisão de gate ao fim de cada fase, com evidência e go/no-go.
- Conselho de design partners mensal a partir da Fase 1.

---

## 5. Inventário de epics por fase

### Fase 0 — 7 epics, 31 stories (detalhado em `epics.md`)

| ID | Epic | FRs/NFRs | Resultado |
|---|---|---|---|
| P0-E1 | Fundação do repositório e governança | AR-1, AR-8, AR-12; NFR-11/15 | Repo, CI, licença, ADRs |
| P0-E2 | Benchmark reproduzível | FR-1/2/4/7/12/17/52; NFR-1/2/13 | Harness 100k/1M/5M, relatório |
| P0-E3 | Storage, query e tenancy | FR-7/10/11/12/17/19/36/40; NFR-1/2/5/6/13 | ClickHouse + VM + row policies |
| P0-E4 | Gateway e WAL | FR-1/4/5/45/46/47/50/52; NFR-1/5/6 | 1M/s, zero perda confirmada |
| P0-E5 | Deploy, operação e API | FR-30/31/32/45/49/53/57/61/63; NFR-7/8/9/15 | Compose, Helm, quickstart, health, OpenAPI |
| P0-E6 | Frontend e performance | FR-23/24/27; NFR-3/10 | Shell, uPlot, tabela, bundle |
| P0-E7 | Design partners | NFR-15; AR-9 | 3 parceiros, feedback semanal |

### Fase 1 — 10 epics, ~42 stories (detalhado em `epics-phase-1.md`)

| ID | Epic | FRs | Resultado |
|---|---|---|---|
| P1-E1 | Ingestão unificada em produção | FR-1..5 | OTLP/Prometheus, pipeline, WAL, quotas |
| P1-E2 | Métricas e catálogo | FR-7..11 | MetricsQL, OTel metrics, catálogo, cardinalidade, rollups |
| P1-E3 | Logs e pattern inspector | FR-12..15 | Busca, live tail, parsing, correlação log-trace |
| P1-E4 | Traces, waterfall e service map | FR-17..19, FR-36 | Busca, waterfall, mapa básico, correlação |
| P1-E5 | Explore e dashboards | FR-23..27 | Query unificada, grid, variáveis, import/export, performance |
| P1-E6 | Alertas | FR-30..32 | Regras metrics/logs, canais, estado/dedup/silence |
| P1-E7 | Identidade, tenancy e audit | FR-45..47, FR-49, FR-50 | Orgs/projetos, OIDC, RBAC, audit mínimo, redaction |
| P1-E8 | Deploy, API e auto-observabilidade | FR-52/53/57/61/63 | OTel Collector, Compose/Helm, OpenAPI, health, backup |
| P1-E9 | Onboarding e design partners | NFR-15 | Quickstart, checklist, 3 pilotos |
| P1-E10 | Hardening e UJ-1 | NFR-1..10 | UJ-1 ponta a ponta, soak, segurança, acessibilidade, production readiness |

### Fase 2 — 9 epics, ~28 stories (planejamento em `epics-phase-2-4.md`)

| ID | Epic | FRs | Resultado |
|---|---|---|---|
| P2-E1 | APM: service map completo, RED, error tracking | FR-19/20/21 | Diagnóstico de serviço e erro |
| P2-E2 | Tail sampling e correlação avançada | FR-22/36/37 | Amostragem por trace, catálogo de entidades |
| P2-E3 | SLOs, burn-rate e runbooks | FR-33/34/35 | Confiabilidade mensurável |
| P2-E4 | Notificações e incident workflow | FR-31/32/35 | PagerDuty/Opsgenie/Teams, escalonamento, incident timeline |
| P2-E5 | GitOps, CLI e migração | FR-26/58/60 | Dashboards/alertas como código, import Grafana/Prometheus |
| P2-E6 | Custo, retenção e cold storage | FR-40..43, FR-64 | TCO visível, archive beta |
| P2-E7 | Agente beta/GA | FR-51 | Agente Rust com logs/auto-discovery |
| P2-E8 | Performance e escala | NFR-1/2/13 | 1M/s em produção assistida, cache, tuning |
| P2-E9 | Design partners e feedback | NFR-15 | 10 parceiros, MTTD/MTTR |

### Fase 3 — 10 epics, ~30 stories (planejamento em `epics-phase-2-4.md`)

| ID | Epic | FRs | Resultado |
|---|---|---|---|
| P3-E1 | HA e upgrades | FR-62 | 99,9/99,95, rolling upgrade |
| P3-E2 | Multi-tenancy avançado | FR-5/40/45 | Quotas, retenção por tenant, isolamento |
| P3-E3 | SSO/SCIM e audit completo | FR-48/49 | Enterprise identity/governança |
| P3-E4 | Operator, air-gapped e DR | FR-54/55/63 | Operação enterprise |
| P3-E5 | Federation beta | FR-38 | Multi-cluster/região |
| P3-E6 | 10M/s benchmark e sizing | NFR-1/2/12 | Escala de GA |
| P3-E7 | Terraform provider e API v1 | FR-57/59 | IaC e contrato estável |
| P3-E8 | Segurança, pentest e compliance | NFR-6/7 | Hardening e readiness SOC 2 |
| P3-E9 | GA release e docs | NFR-8/15 | v1.0.0 |
| P3-E10 | Enterprise design partners | NFR-15 | 3 tenants, casos de referência |

### Fase 4 — 8 epics, ~20 stories (planejamento em `epics-phase-2-4.md`)

| ID | Epic | Resultado |
|---|---|---|
| P4-E1 | RUM e session replay leve | Experiência do usuário final |
| P4-E2 | Synthetic monitoring | Probes globais e jornadas |
| P4-E3 | eBPF auto-instrumentation | Service map sem instrumentação |
| P4-E4 | Continuous profiling | CPU/memória por trace |
| P4-E5 | LLM observability | Chains, tokens, custo, qualidade |
| P4-E6 | AI assistant e anomalias | Investigação assistida |
| P4-E7 | Security signals | Correlação de segurança (se validado) |
| P4-E8 | Marketplace de integrações | Ecossistema |

---

## 6. Dependências e caminho crítico

```text
G0 escala confirmada
  └─> Fase 0 (benchmark, ADRs, deploy)
        └─> G1 1M/s + p95
              └─> Fase 1 (MVP: três sinais + correlação + dashboards + alertas)
                    └─> G2 UJ-1 + 3 pilotos
                          └─> Fase 2 (APM + SLOs + agente + migração)
                                └─> G3 1M/s produção assistida + 10 parceiros
                                      └─> Fase 3 (HA + enterprise + 10M/s + GA)
                                            └─> G4 GA 1.0
                                                  └─> Fase 4 (RUM, sintéticos, eBPF, profiling, IA)
```

**Dependências duras:**
- Fase 1 não começa sem G1 (benchmark).
- Correlação mínima (trace-log) é pré-requisito de UJ-1 e do MVP; correlação avançada fica na Fase 2.
- Rollups/cache entram no MVP para financiar as metas de query; não podem ser adiados para a Fase 2.
- Multi-tenancy/row policies entram no MVP, mesmo em instalação single-tenant; enterprise (SSO/SCIM/audit completo) espera a Fase 3.
- Agente próprio e migração Grafana/Prometheus ficam na Fase 2; o MVP usa SDKs/Coletor OTel.
- Federação só depois de 10M/s estável e 3 tenants isolados.
- RUM/sintéticos/eBPF/profiling/IA só depois do GA.

---

## 7. Modelo de trabalho

**Epic → stories → tasks.** A Fase 0 já tem stories com AC Given/When/Then e a Story 1.1 quebrada em tarefas. A Fase 1 tem stories com AC resumidos e critérios de saída por epic. As Fases 2-4 têm epics e stories em nível de planejamento; serão detalhadas após o gate anterior, quando a arquitetura/UX e os dados de design partners estiverem mais claros.

**Definition of Ready (story):**
- Contexto e valor claros.
- Dependências identificadas.
- AC testáveis.
- Design/UX e contrato de API definidos (quando aplicável).
- Benchmark/telemetria de sucesso definido.

**Definition of Done (story):**
- Código revisado e mergeado.
- Testes unitários/contrato/integração relevantes.
- Documentação/ADR atualizada.
- Métrica de performance/benchmark registrada quando aplicável.
- Acessibilidade verificada (WCAG 2.2 AA) em UI.
- Nenhum bug P0/P1 conhecido.
- Demo ou evidência anexada.

**Qualidade de release:**
- CI verde (lint, test, build, SAST, SBOM).
- Benchmark de regressão comparado ao release anterior.
- Helm/Compose smoke test.
- Migração/rollback testados.
- Notas de release e breaking changes.

---

## 8. Métricas do programa

| Métrica | Meta | Onde medir |
|---|---|---|
| Benchmark de ingestão | 1M/s no MVP; 10M/s no GA | `BENCHMARKS.md` |
| Latência de query | p95 < 1s em 1B/24h; < 3s em 10B/7d | Benchmark/CI |
| Bundle inicial | < 180 KB gzip | Lighthouse/CI |
| TTFV | p50 < 5 min; p90 < 15 min | Design partners |
| Adoção | 30 implantações ativas no GA | Telemetria opt-in + GitHub |
| Confiabilidade | 99,9% query / 99,95% ingestão | Soak/field |
| Custo | ≤ 30% da infra equivalente ao Datadog; ≤ 50% do TCO | Addendum §10 |
| MTTD/MTTR | MTTD < 5 min; MTTR -30% nos parceiros | Design partners |
| Cobertura de testes | ≥ 70% no core; 100% dos contratos | CI |

---

## 9. Riscos e mitigação

| Risco | Prob. | Impacto | Mitigação |
|---|---|---|---|
| Stack não atinge 1M/s | Média | Alto | Fase 0; fallback Rust/Rotel; schema/batch |
| Cardinalidade explode | Alta | Alto | Guardrails no MVP; cardinality explorer; quotas |
| Escopo “Datadog-like” cresce | Alta | Alto | Non-goals; fases com gates; design partners |
| ClickHouse é pesado para times pequenos | Média | Médio | Helm opinativo; single-node; operator na Fase 3 |
| UX não é rápida o suficiente | Média | Alto | Orçamento no CI; uPlot; virtualização |
| Design partners não aparecem | Média | Alto | Pipeline desde a Fase 0; dados sintéticos como fallback |
| Produção sem prontidão | Média | Alto | Pilotos com SLA reduzido; production readiness gate |
| Time pequeno perde ritmo | Média | Alto | Buffer 30%; cortar escopo, não qualidade |
| Licença/open-core mal calibrado | Baixa | Alto | Fronteira definida antes da Fase 1 |
| Concorrência (SigNoz/OpenObserve/Grafana) | Alta | Médio | Foco em performance + UX + correlação; open source |

---

## 10. Decisões confirmadas e pendências

**Confirmadas em 2026-10-31:**
- Escala: 1M/s no benchmark do MVP, 100k/s por piloto, 10M/s no GA, 100M+/s via federação; 1B/s literal fora do v1.
- Licença: Apache 2.0 no núcleo + open-core.
- ICP: startups/times pequenos (3-50 engenheiros); enterprise na Fase 3.
- Stack: Go 1.27, ClickHouse 26.8 LTS, VictoriaMetrics 1.148 LTS, NATS 2.15, PostgreSQL 18.6, SvelteKit 3.1, Svelte 5.57, uPlot 1.6.32.
- Arquitetura: ports & adapters + pipes-and-filters; 35 ADs.
- UX: dark-first, desktop-first, keyboard-first; DESIGN/EXPERIENCE spines.

**Pendências não bloqueantes:**
1. Design partners: nomes e compromisso mínimo.
2. Retenção default: 7, 15 ou 30 dias.
3. Fronteira open-core exata (quais features enterprise).
4. Frontend: SvelteKit 3 confirmado? Se o time for React-heavy, reavaliar antes da Fase 1.
5. Compliance: SOC 2/HIPAA/FedRAMP no roadmap comercial.
6. Time/prazo: 5-6 engenheiros e ~14 meses são realistas?

---

## 11. Próximos passos

1. **Aprovar este plano** e os spines de arquitetura/UX.
2. **Fase 0:** executar `epics.md` (31 stories), começando pela Story 1.1 (9 tarefas).
3. **Gate G1:** benchmark, ADRs, design partners; decidir go/no-go.
4. **Fase 1:** detalhar/executar `epics-phase-1.md`; criar `bmad-ux` mocks finais das telas de Explore/Dashboard/Services.
5. **Fase 2+:** detalhar `epics-phase-2-4.md` somente após o gate anterior.

**Regra do programa:** nenhuma fase começa sem o gate anterior; nenhum novo sinal entra antes do GA; nenhuma promessa de performance sem benchmark reproduzível.
