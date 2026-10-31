---
stepsCompleted: [1, 2, 3, 4]
inputDocuments:
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md
  - _bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md
  - _bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/DESIGN.md
  - _bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md
  - _bmad-output/planning-artifacts/epics.md
  - _bmad-output/planning-artifacts/epics-phase-1.md
project: Observaki
phase: 2-4
status: planned
created: 2026-10-31
updated: 2026-10-31
---

# Observaki — Epic Breakdown (Fases 2-4)

**Escopo:** planejamento de portfólio para depois do MVP. As stories estão em nível de epic/story (título + outcome + dependências), não em AC Given/When/Then. O detalhamento fino acontece após o gate anterior, quando arquitetura, UX e dados de design partners estiverem mais claros.

**Regra:** nenhuma fase começa sem o gate anterior; nenhum novo sinal entra antes do GA; nenhuma promessa de performance sem benchmark reproduzível.

---

# Fase 2 — APM, correlação e SLOs (12 semanas)

**Goal:** sair de “dados disponíveis” para “diagnóstico rápido”. Esta fase aproxima o produto da promessa Datadog-like: service map completo, RED, error tracking, tail sampling, correlação avançada, SLOs, agente beta/GA e migração.

**Gate de entrada:** G2 (MVP aprovado, UJ-1, 3 pilotos).
**Gate de saída:** G3 (1M/s em produção assistida, 10 design partners, MTTD/MTTR).

## P2-E1 — APM: service map completo, RED e error tracking

**FRs:** FR-19, FR-20, FR-21. **NFRs:** NFR-1, NFR-2, NFR-5.

| Story | Outcome | Depende |
|---|---|---|
| 1.1 | Service map completo com owner, dependências e drill-down | P1-E4 |
| 1.2 | Span metrics/RED calculados antes do sampling, com comparação por endpoint | P1-E4 |
| 1.3 | Error tracking com fingerprint, stack trace, versão e primeiro/último evento | P1-E4 |
| 1.4 | Catálogo de serviços com owner, ambiente e última telemetria | P1-E7 |

**Exit:** mapa 1k serviços <2s; RED sem viés de sampling (desvio <1%); 10k erros agrupados em <100 issues.

## P2-E2 — Tail sampling e correlação avançada

**FRs:** FR-22, FR-36, FR-37, FR-39. **NFRs:** NFR-1, NFR-5.

| Story | Outcome | Depende |
|---|---|---|
| 2.1 | Roteador de tail sampling com afinidade por trace_id e buffer | P1-E1 |
| 2.2 | Políticas de amostragem por serviço/atributo, preservando 100% dos erros | 2.1 |
| 2.3 | Correlação avançada por serviço/janela quando não há trace_id | P1-E3/E4 |
| 2.4 | Catálogo de entidades (host, pod, namespace, owner) | 1.4 |
| 2.5 | Change tracking de deploys/feature flags sobre os gráficos | P1-E5 |

**Exit:** 100% dos erros preservados; tail sampling em 1M/s sem perda; correlação fallback funciona; deploy marker em <10s.

## P2-E3 — SLOs, burn-rate e runbooks

**FRs:** FR-33, FR-34, FR-35. **NFRs:** NFR-4, NFR-5.

| Story | Outcome | Depende |
|---|---|---|
| 3.1 | Modelo de SLI/SLO com target, janela e error budget | P1-E2/E3/E4 |
| 3.2 | Cálculo de budget consumido e projeção de esgotamento | 3.1 |
| 3.3 | Alertas multi-janela de burn-rate (14,4x/6x/1x) | 3.2 |
| 3.4 | Runbooks, links de incidente e histórico de SLO | 3.3 |

**Exit:** SLO 99,9% em 30d; burn-rate sem falso positivo em pico curto; runbook na notificação.

## P2-E4 — Notificações e workflow de incidente

**FRs:** FR-31, FR-32, FR-35. **NFRs:** NFR-4.

| Story | Outcome | Depende |
|---|---|---|
| 4.1 | PagerDuty, Opsgenie e Teams | P1-E6 |
| 4.2 | Agrupamento por serviço/causa, repeat interval e escalonamento | P1-E6 |
| 4.3 | Timeline de incidente com alertas, deploys, logs e traces | P2-E2 |
| 4.4 | Acknowledge/resolve com contexto e postmortem export | 4.3 |

**Exit:** alerta agrupado; escalonamento testado; timeline exportável; ruído <20% nos parceiros.

## P2-E5 — GitOps, CLI e migração

**FRs:** FR-26, FR-58, FR-60. **NFRs:** NFR-11, NFR-15.

| Story | Outcome | Depende |
|---|---|---|
| 5.1 | CLI `observaki` para query/apply/status | P1-E8 |
| 5.2 | Dashboards/alertas como código (YAML/JSON) com schema e diff | P1-E5/E6 |
| 5.3 | Import de dashboards Grafana com relatório de fidelidade | P1-E5 |
| 5.4 | Import de regras Prometheus com relatório de conversão | P1-E6 |
| 5.5 | Guia de coexistência/migração | 5.3/5.4 |

**Exit:** `observaki apply` idempotente; CI valida schema; import Grafana/Prometheus com relatório; migração documentada.

## P2-E6 — Custo, retenção e cold storage beta

**FRs:** FR-40, FR-41, FR-42, FR-43, FR-64. **NFRs:** NFR-13.

| Story | Outcome | Depende |
|---|---|---|
| 6.1 | Cold storage beta (S3/Parquet) com archive search | P1-E2/E3 |
| 6.2 | Estimador de custo/uso por projeto e cenário de retenção | P1-E7 |
| 6.3 | Amostragem de logs/traces e agregação de métricas | P2-E2 |
| 6.4 | Recomendação de sizing/capacidade | 6.2 |

**Exit:** archive 1 ano <30s para 1h; custo por projeto; simulação de retenção; sizing recomendado.

## P2-E7 — Agente beta/GA

**FRs:** FR-51. **NFRs:** NFR-9.

| Story | Outcome | Depende |
|---|---|---|
| 7.1 | Agente Rust beta: host metrics, logs de container, OTLP | P0-E4 |
| 7.2 | Auto-discovery de Kubernetes e metadados | 7.1 |
| 7.3 | Upgrade/rollback, assinatura e pacotes | 7.1 |
| 7.4 | Benchmark de CPU/RAM em 1k/5k/10k eventos/s | 7.2 |

**Exit:** <2% CPU/<100MB a 5k/s; DaemonSet estável; upgrade sem perda; docs.

## P2-E8 — Performance e escala

**FRs:** NFR-1, NFR-2, NFR-13. **NFRs:** NFR-1, NFR-2.

| Story | Outcome | Depende |
|---|---|---|
| 8.1 | Cache distribuído e pré-agregação para dashboards grandes | P1-E5 |
| 8.2 | 1M/s em produção assistida com 6-12 CH + 3 VM + 4-6 gateways | P1-E1 |
| 8.3 | Tuning de queries e concorrência por tenant | P1-E2 |
| 8.4 | Relatório de benchmark e custo por TB/dia | 8.2 |

**Exit:** 1M/s em produção assistida; p95 <1s/1B; custo dentro do alvo; relatório publicado.

## P2-E9 — Design partners e feedback

**FRs:** NFR-15; AR-9.

| Story | Outcome | Depende |
|---|---|---|
| 9.1 | 10 design partners ativos com dados reais | P1-E9 |
| 9.2 | Baseline de MTTD/MTTR e NPS | 9.1 |
| 9.3 | Casos de uso e referências públicas | 9.2 |
| 9.4 | Conselho de produto mensal | 9.1 |

**Exit:** 10 parceiros; MTTD <5 min; MTTR -30%; 2 casos de referência.

---

# Fase 3 — Escala e enterprise (14 semanas)

**Goal:** tornar o Observaki confiável para organizações grandes e para o GA 1.0.
**Gate de entrada:** G3.
**Gate de saída:** G4 (GA 1.0, 10M/s, 30 dias de soak, 3 tenants).

## P3-E1 — HA e upgrades

**FRs:** FR-62. **NFRs:** NFR-5, NFR-8.

| Story | Outcome | Depende |
|---|---|---|
| 1.1 | HA do control plane (PostgreSQL HA, API/Query replicas) | P2-E8 |
| 1.2 | HA do data plane (NATS/ClickHouse/VM, failover) | P2-E8 |
| 1.3 | Upgrade rolling e rollback sem downtime | 1.1/1.2 |
| 1.4 | Teste de caos multi-nó e relatório | 1.3 |

**Exit:** 99,9/99,95 em 30 dias; upgrade sem queda; rollback <15 min.

## P3-E2 — Multi-tenancy avançado

**FRs:** FR-5, FR-40, FR-45. **NFRs:** NFR-6, NFR-13.

| Story | Outcome | Depende |
|---|---|---|
| 2.1 | Quotas de consulta/concorrência/memória por tenant | P1-E7 |
| 2.2 | Retenção por tenant de métricas (VM/instâncias) | P2-E6 |
| 2.3 | Isolamento de workload e prioridade (alertas vs ad-hoc) | 2.1 |
| 2.4 | Teste de noisy neighbor e relatório | 2.3 |

**Exit:** 3 tenants isolados; p95 dos outros <10% sob carga; retenção por tenant.

## P3-E3 — SSO/SCIM e audit completo

**FRs:** FR-48, FR-49. **NFRs:** NFR-6, NFR-7.

| Story | Outcome | Depende |
|---|---|---|
| 3.1 | SAML/OIDC corporativo com mapeamento de grupos | P1-E7 |
| 3.2 | SCIM de usuários/grupos | 3.1 |
| 3.3 | Audit completo (login, permissão, dashboard, exportação) | P1-E7 |
| 3.4 | Retenção/exportação de auditoria | 3.3 |

**Exit:** usuário desativado perde acesso <5 min; grupos mapeiam papéis; auditoria imutável/exportável.

## P3-E4 — Operator, air-gapped e DR

**FRs:** FR-54, FR-55, FR-63. **NFRs:** NFR-8.

| Story | Outcome | Depende |
|---|---|---|
| 4.1 | Kubernetes Operator com CRDs de tenant/retention/alert | P3-E1 |
| 4.2 | Bundle air-gapped assinado e upgrade offline | 4.1 |
| 4.3 | Backup/restore de metadados e DR de telemetria | P1-E8 |
| 4.4 | Drill de DR trimestral | 4.3 |

**Exit:** operator instala/atualiza; air-gapped funciona; restore <30 min; drill documentado.

## P3-E5 — Federation beta

**FRs:** FR-38 (timeline multi-cluster), arquitetura de federação. **NFRs:** NFR-12.

| Story | Outcome | Depende |
|---|---|---|
| 5.1 | Query router global com limites por região | P3-E1 |
| 5.2 | Clusters regionais e metadata replicada | 5.1 |
| 5.3 | Query federada com timeout/orçamento | 5.2 |
| 5.4 | Teste multi-região e custo | 5.3 |

**Exit:** 2 regiões consultáveis; limite global; data residency documentada.

## P3-E6 — 10M/s benchmark e sizing

**FRs:** NFR-1, NFR-2, NFR-12.

| Story | Outcome | Depende |
|---|---|---|
| 6.1 | Cluster de referência 30-60 nós, 7-15 dias | P2-E8 |
| 6.2 | Benchmark 10M/s por 1h com relatório | 6.1 |
| 6.3 | Modelo de sizing/custo por tier | 6.2 |
| 6.4 | Publicação `BENCHMARKS.md` | 6.2 |

**Exit:** 10M/s; lag <10s; p95 query; custo/TB; relatório reproduzível.

## P3-E7 — Terraform provider e API v1

**FRs:** FR-57, FR-59. **NFRs:** NFR-11.

| Story | Outcome | Depende |
|---|---|---|
| 7.1 | API v1 estável com deprecation policy | P1-E8 |
| 7.2 | Terraform provider (projetos/dashboards/alertas/retenção) | 7.1 |
| 7.3 | Publicação no registry e docs | 7.2 |
| 7.4 | Contract tests e changelog | 7.1 |

**Exit:** `terraform apply` cria projeto/dashboard; drift; API v1 congelada.

## P3-E8 — Segurança, pentest e compliance

**FRs:** NFR-6, NFR-7, NFR-14.

| Story | Outcome | Depende |
|---|---|---|
| 8.1 | Pentest externo e correções | P3-E2/E3 |
| 8.2 | SBOM, imagens assinadas, CVE SLA | P1-E8 |
| 8.3 | Trilha de readiness SOC 2 | 8.1/8.2 |
| 8.4 | DPA/GDPR e data residency | 8.3 |

**Exit:** pentest sem críticos abertos; SBOM; SOC 2 readiness; DPA.

## P3-E9 — GA release e docs

**FRs:** NFR-8, NFR-15.

| Story | Outcome | Depende |
|---|---|---|
| 9.1 | Release v1.0.0, Helm estável, upgrade path | P3-E1..E8 |
| 9.2 | Docs de operação, runbooks, troubleshooting | 9.1 |
| 9.3 | Notas de release e breaking changes | 9.1 |
| 9.4 | Suporte/LTS policy | 9.1 |

**Exit:** v1.0.0; Helm estável; docs; política de suporte.

## P3-E10 — Enterprise design partners

**FRs:** NFR-15; AR-9.

| Story | Outcome | Depende |
|---|---|---|
| 10.1 | 3 tenants enterprise isolados | P3-E2/E3 |
| 10.2 | Casos de referência e métricas | 10.1 |
| 10.3 | Feedback de governança/auditoria | 10.1 |
| 10.4 | Roadmap enterprise pós-GA | 10.3 |

**Exit:** 3 tenants; 2 referências; roadmap enterprise.

---

# Fase 4 — Expansão (16+ semanas)

**Goal:** ampliar o portfólio para casos adjacentes, sem comprometer o núcleo. Cada novo sinal exige ≥5 design partners e uma métrica de sucesso definida antes do início.

## P4-E1 — RUM e session replay leve

**Stories:**
1.1 SDK browser leve (<30KB gzip), amostragem e privacy masking.
1.2 Captura de Web Vitals, erros e recursos.
1.3 Session replay em object storage, retenção curta e acesso auditado.
1.4 Correlação RUM ↔ trace/log por `session_id`.
1.5 Dashboard de experiência do usuário.

**Gate:** 5 design partners; <30KB; PII mask 100%; replay <5% do custo de logs.

## P4-E2 — Synthetic monitoring

**Stories:**
2.1 Runners globais e checks HTTP/API.
2.2 Browser checks e jornadas.
2.3 Integração com alertas/SLOs.
2.4 Localizações, frequência e custo por execução.

**Gate:** 5 parceiros; checks <60s; custo controlado.

## P4-E3 — eBPF auto-instrumentation

**Stories:**
3.1 Agente Rust com aya e coleta de latência de rede.
3.2 Enriquecimento Kubernetes e dependências.
3.3 Opt-in por namespace e guardrails de CPU/RAM.
3.4 Service map sem instrumentação.

**Gate:** overhead <3% CPU; 5 parceiros; cobertura de 80% dos serviços.

## P4-E4 — Continuous profiling

**Stories:**
4.1 Coleta pprof/OTel Profiles e symbolization.
4.2 Flame graphs e correlação com traces/deploys.
4.3 Amostragem adaptativa e retenção curta.

**Gate:** overhead <2%; 5 parceiros; flame graph <2s.

## P4-E5 — LLM observability

**Stories:**
5.1 OTel GenAI semantic conventions.
5.2 Tokens, custo, latência e qualidade por chain.
5.3 Correlação com APM e RUM.
5.4 Guardrails de privacidade.

**Gate:** 5 parceiros; custo preciso; PII safe.

## P4-E6 — AI assistant e anomalias

**Stories:**
6.1 Linguagem natural → query com citação da consulta.
6.2 Resumo de incidente e correlação assistida.
6.3 Detecção de anomalias com baseline.
6.4 Guardrails: opt-in, sem vazamento, explicabilidade.

**Gate:** 5 parceiros; precisão >80%; nenhum dado sai sem opt-in.

## P4-E7 — Security signals

**Stories:**
7.1 Ingestão de eventos de segurança.
7.2 Correlação com serviços e identidades.
7.3 Regras e alertas de segurança.
7.4 Apenas se houver demanda clara; não virar SIEM.

**Gate:** 5 parceiros; não canibalizar o core.

## P4-E8 — Marketplace de integrações

**Stories:**
8.1 Registry de dashboards/exporters/plugins.
8.2 Verificação e assinatura.
8.3 Publicação e descoberta.
8.4 Programa de parceiros.

**Gate:** 20 integrações; 10 mantenedores externos.

---

## Programa de gates e revisões

| Gate | Quando | Evidência |
|---|---|---|
| G2 | Fim da Fase 1 | UJ-1, 3 pilotos, soak 7d |
| G3 | Fim da Fase 2 | 1M/s produção assistida, 10 parceiros, MTTD/MTTR |
| G4 | Fim da Fase 3 | 10M/s, 30d soak, 3 tenants, GA |
| G5 | Fase 4 | ≥5 parceiros por novo sinal, métrica definida |

**Regra final:** o programa prioriza confiabilidade e clareza de escopo sobre velocidade de portfólio. Nenhum novo sinal entra antes do GA; nenhuma fase começa sem o gate anterior.

---

## Sequência recomendada de sprints

### Fase 2 — 6 sprints de 2 semanas

| Sprint | Foco | Epics |
|---|---|---|
| S1 | Service map completo + RED | P2-E1.1, E1.2 |
| S2 | Error tracking + catálogo | P2-E1.3, E1.4, E2.4 |
| S3 | Tail sampling + correlação avançada | P2-E2.1, E2.2, E2.3, E2.5 |
| S4 | SLOs + burn-rate | P2-E3.1, E3.2, E3.3, E3.4 |
| S5 | Notificações + GitOps/CLI + migração | P2-E4, E5 |
| S6 | Agente beta + custo/cold + performance | P2-E6, E7, E8, E9 |

### Fase 3 — 7 sprints de 2 semanas

| Sprint | Foco | Epics |
|---|---|---|
| S1 | HA do control plane + data plane | P3-E1 |
| S2 | Multi-tenancy + quotas | P3-E2 |
| S3 | SSO/SCIM + audit completo | P3-E3 |
| S4 | Operator + air-gapped + DR | P3-E4 |
| S5 | Federação beta + 10M/s benchmark | P3-E5, E6 |
| S6 | Terraform + API v1 + segurança | P3-E7, E8 |
| S7 | GA release + enterprise partners | P3-E9, E10 |

### Fase 4 — 1 épico por ciclo de 2-4 semanas

Cada epic (RUM, sintéticos, eBPF, profiling, LLM, IA, security, marketplace) é um ciclo com gate de 5 design partners e métrica de sucesso definida antes do início. Não há compromisso de data para o portfólio completo.
