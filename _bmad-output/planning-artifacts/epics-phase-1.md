---
stepsCompleted: [1, 2, 3, 4]
inputDocuments:
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md
  - _bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md
  - _bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/DESIGN.md
  - _bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md
  - _bmad-output/planning-artifacts/epics.md
project: Observaki
phase: 1
status: ready-for-refinement
created: 2026-10-31
updated: 2026-10-31
---

# Observaki — Epic Breakdown (Fase 1: MVP)

**Escopo:** 10 epics, 46 stories. A Fase 1 entrega a fatia vertical que prova o produto: três sinais, correlação mínima, Explore, dashboards, alertas, auth/tenancy, deploy e UJ-1 ponta a ponta. As histórias usam AC resumidos; o detalhamento Given/When/Then será feito por story antes do desenvolvimento, como na Fase 0.

**Fora da Fase 1:** RUM, sintéticos, profiling, eBPF, SSO/SCIM, audit completo, operator, air-gapped, federação, agente próprio (Fase 2), migração Grafana/Prometheus (Fase 2), SLOs (Fase 2), error tracking/tail sampling (Fase 2).

---

## P1-E1 — Ingestão unificada em produção

**Goal:** qualquer SDK/Coletor OTel envia métricas, logs e traces para um gateway durável, com quotas, pipeline e diagnóstico.
**FRs:** FR-1, FR-2, FR-3, FR-4, FR-5, FR-50. **NFRs:** NFR-1, NFR-5, NFR-6, NFR-14.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 1.1 | FR-1 | Gateway OTLP production-ready | gRPC/HTTP, partial_success, batch limits, graceful shutdown, contract tests com SDKs Go/Java/Python/Node | M | Fase 0 |
| 1.2 | FR-2 | Prometheus estático + remote write | 100 targets, import report, remote_write auth, labels normalizadas | M | 1.1 |
| 1.3 | FR-3, FR-50 | Pipeline com filtros/redaction/preview | Regras por tenant/projeto, preview de 15m, PII mascarada antes do WAL, correlação preservada | M | 1.1 |
| 1.4 | FR-4 | WAL/backpressure/dedupe production | NATS durável, 429+Retry-After, dedupe por event_id, caos de 15m sem perda confirmada | L | 1.1, 1.3 |
| 1.5 | FR-5 | Quotas e isolamento de ingestão | Limites por tenant/projeto, 429, auditoria, noisy neighbor <10% | M | 1.4 |
| 1.6 | FR-1 | Diagnóstico de ingestão | “Por que meu dado não aparece?” com endpoint/token/clock/sampling/firewall | S | 1.1 |

**Exit:** 3 pilotos ingerindo 100k/s cada; zero perda P0 em 7 dias; diagnóstico cobre 5 falhas comuns.

---

## P1-E2 — Métricas, catálogo e cardinalidade

**Goal:** o usuário consulta métricas em MetricsQL, entende cardinalidade e vê dashboards rápidos.
**FRs:** FR-7, FR-8, FR-9, FR-10, FR-11, FR-40. **NFRs:** NFR-1, NFR-2, NFR-13.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 2.1 | FR-7 | Query service MetricsQL | Instant/range, funções, subqueries, erro com posição, p95 <1s em 1B/24h | L | Fase 0 |
| 2.2 | FR-8 | OTel metrics + exemplars | Delta/cumulative, histogramas, exponential histograms, exemplar→trace, matriz de compatibilidade | M | 2.1 |
| 2.3 | FR-9 | Catálogo/explorer | Nomes/labels/valores, top séries, busca, copiar query, <2s para 1M séries | M | 2.1 |
| 2.4 | FR-10 | Guardrails de cardinalidade | Limites por tenant, top labels, drop/aggregate sugerido, alerta em 5m, sem derrubar cluster | M | 2.3 |
| 2.5 | FR-11, FR-40 | Rollups 1m/5m + retenção | Rollups independentes, query 7d <2s, retenção global de métricas clara na UI | M | 2.1 |

**Exit:** p95 <1s em 1B/24h; 10B/7d <3s; cardinality spike contida; rollups sem >10% de overhead.

---

## P1-E3 — Logs e pattern inspector

**Goal:** buscar, acompanhar e agrupar logs sem perder o trace.
**FRs:** FR-12, FR-13, FR-14, FR-15. **NFRs:** NFR-2, NFR-3, NFR-14.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 3.1 | FR-12 | Busca e facetas | Texto + campos, severidade/serviço/host, SQL limitado, p95 <3s em 1B, salvar busca | L | Fase 0 |
| 3.2 | FR-13 | Live tail | SSE/WebSocket, pausa/retomada, 10k linhas/s sem travar, abrir trace | M | 3.1 |
| 3.3 | FR-14 | Parsing + pattern inspector | JSON/logfmt/regex, top 100 padrões, 90% agrupados, sem detecção de novos (Fase 2) | M | 3.1 |
| 3.4 | FR-15 | Correlação log-trace | trace_id/span_id, log→trace e trace→log, fallback “trace amostrado” | M | 3.1, P1-E4 |

**Exit:** 1B linhas p95 <3s; 10k/s live tail estável; 90% dos logs agrupados; correlação bidirecional.

---

## P1-E4 — Traces, waterfall e service map

**Goal:** encontrar o trace lento, ver o caminho crítico e navegar para logs/métricas.
**FRs:** FR-17, FR-18, FR-19, FR-36. **NFRs:** NFR-2, NFR-3.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 4.1 | FR-17 | Busca de traces | Serviço/operação/status/duração/atributos, 1M spans/s ingest, p95 <2s/24h | L | Fase 0 |
| 4.2 | FR-18 | Waterfall e detalhe | 1k spans <500ms, critical path destacado, erro distinto, drawer de atributos | M | 4.1 |
| 4.3 | FR-19 | Service map básico | 500 serviços <1s, arestas latência/erro, clique filtra traces, “sem dados” explícito | M | 4.1 |
| 4.4 | FR-36 | Correlação mínima | trace→log, log→trace, exemplar→trace, fallback por serviço/janela | M | 4.2, P1-E3 |

**Exit:** UJ-1 executável em um piloto; waterfall 1k spans <500ms; mapa 500 serviços <1s.

---

## P1-E5 — Explore e dashboards

**Goal:** uma superfície de investigação unificada e dashboards compartilháveis, rápidos e organizados.
**FRs:** FR-23, FR-24, FR-25, FR-26, FR-27. **NFRs:** NFR-3, NFR-10.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 5.1 | FR-23 | Explore unificado | Troca de sinal preserva tempo/serviço, autocomplete <200ms, histórico 50, estados de erro/vazio | L | P1-E2/3/4 |
| 5.2 | FR-24 | Dashboards CRUD/grid | 7 painéis, pastas/tags/busca, 6 painéis <2s, 12 <3s, drag/resize | L | 5.1 |
| 5.3 | FR-25 | Variáveis/templates | Serviço/host/cluster/namespace, multi-select, templates Go/Java/Python/Node/K8s/Postgres | M | 5.2 |
| 5.4 | FR-26 | Import/export/links | JSON interno, link com tempo/filtros, import Grafana na Fase 2 | M | 5.2 |
| 5.5 | FR-27 | Performance rendering | uPlot 100k <100ms, tabela 10k 60fps, cancelamento/cache, bundle <180KB | M | 5.1 |

**Exit:** 6 painéis <2s; 12 <3s; 100k pontos <100ms; bundle inicial <180KB; URL reproduzível.

---

## P1-E6 — Alertas

**Goal:** criar alertas acionáveis em métricas/logs, com estado, silences e canais.
**FRs:** FR-30, FR-31, FR-32. **NFRs:** NFR-4, NFR-7.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 6.1 | FR-30 | Rule builder metrics/logs | Threshold/relative/absence/for, testar 24h, preview, validação bloqueia save inválido | L | P1-E2/3 |
| 6.2 | FR-31 | Canais Slack/webhook/email | Notificação com link de painel/trace, retry/backoff, teste de canal | M | 6.1 |
| 6.3 | FR-32 | Estado/dedup/silence/ack | firing/resolved/ack, dedup por regra, silence com duração/motivo, histórico | M | 6.1 |
| 6.4 | FR-30 | Alerta a partir do contexto | Criar regra de painel/log/trace com query pré-preenchida; warning de ruído >20/dia | S | 6.1, P1-E5 |

**Exit:** alerta dispara <90s; notificação com contexto; silences expiram; teste 24h.

---

## P1-E7 — Identidade, tenancy e audit

**Goal:** separar organizações/projetos, autenticar, autorizar e auditar o mínimo.
**FRs:** FR-45, FR-46, FR-47, FR-49, FR-50. **NFRs:** NFR-6, NFR-7, NFR-14.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 7.1 | FR-45 | Orgs/projetos | Isolamento lógico, quotas, mover projeto com auditoria, 403 explícito | M | Fase 0 |
| 7.2 | FR-46 | Auth local + OIDC | Argon2id, sessão/revogação, OIDC com groups, primeiro login | M | 7.1 |
| 7.3 | FR-47 | RBAC + API tokens | admin/editor/viewer, token escopo/expiração/hash, 403 testado | M | 7.2 |
| 7.4 | FR-49 | Audit mínimo | login, permissão, quota, ingestão; imutável/exportável | S | 7.3 |
| 7.5 | FR-50 | PII redaction | Regras prontas + custom, precision/recall em corpus, correlação preservada | M | P1-E1 |

**Exit:** cross-tenant negado; OIDC funcional; RBAC testado; audit mínimo; PII mascarada.

---

## P1-E8 — Deploy, API e auto-observabilidade

**Goal:** instalar, operar e integrar o MVP sem depender de um exército.
**FRs:** FR-52, FR-53, FR-57, FR-61, FR-63. **NFRs:** NFR-8, NFR-9, NFR-11, NFR-15.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 8.1 | FR-52 | Compatibilidade OTel Collector | Contrib envia 3 sinais; exemplo de config; sem agente proprietário | S | P1-E1 |
| 8.2 | FR-53 | Compose/Helm/single-node | Compose <10 min, Helm em 3 nós, upgrade/rollback, dados persistem | L | P1-E1..E7 |
| 8.3 | FR-57 | API/OpenAPI | /api/v1 health/projects/tokens/query; spec validada no CI; erros estruturados | M | P1-E7 |
| 8.4 | FR-61 | Auto-observabilidade | health/ready, métricas Prometheus, traces, painel de lag/erro/storage | M | 8.2 |
| 8.5 | FR-63 | Backup/restore mínimo | pg_dump/restore <30 min, runbook de upgrade/rollback, teste | M | 8.2 |

**Exit:** Compose <15 min; Helm 3 nós; API/OpenAPI; health page; restore <30 min.

---

## P1-E9 — Onboarding e design partners

**Goal:** um novo usuário chega ao primeiro dashboard em 5 minutos; 3 pilotos com dados reais.
**NFRs:** NFR-15; AR-9.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 9.1 | NFR-15 | Quickstart + sample apps | Go/Python/Node/Collector, endpoint/token, troubleshooting, <15 min | M | P1-E8 |
| 9.2 | NFR-15 | Onboarding checklist | 4 passos, auto-detecta primeiro sinal, dismissível, reabre no Help | M | 9.1 |
| 9.3 | AR-9 | Programa de pilotos | 3 parceiros, dados reais, feedback semanal, baseline de MTTD/MTTR | M | 9.1 |

**Exit:** TTFV p50 <5 min/p90 <15 min; 3 pilotos ativos; 4 sessões de feedback.

---

## P1-E10 — Hardening e UJ-1 end-to-end

**Goal:** provar a jornada âncora e a prontidão de produção.
**NFRs:** NFR-1..NFR-10.

| Story | FR | Outcome | AC resumido | Size | Depends |
|---|---|---|---|---|---|
| 10.1 | UJ-1 | Teste ponta a ponta | alerta→painel→trace→log→métrica, em um piloto, com evidência | M | P1-E5/E6 |
| 10.2 | NFR-1/2 | Soak e performance | 7 dias 100k/s, lag <10s p95, p95 query <1s/1B, zero perda P0 | M | P1-E1..E5 |
| 10.3 | NFR-6/10 | Segurança/acessibilidade | SAST/DAST, isolamento, axe, teclado, contraste, tabela alternativa | M | P1-E5/E7 |
| 10.4 | NFR-8 | Production readiness gate | backup/restore, runbooks, upgrade/rollback, auditoria, revisão de segurança | M | 10.2/10.3 |
| 10.5 | NFR-15 | Docs e release notes | Quickstart, instrumentação, operação, troubleshooting, changelog | S | 10.4 |

**Exit:** UJ-1 aprovada; 7 dias de soak; gate de produção; docs publicadas; `v0.5.0-beta`.

---

## FR Coverage Map (Fase 1)

| FR | Epic |
|---|---|
| FR-1, FR-2, FR-3, FR-4, FR-5 | P1-E1 |
| FR-7, FR-8, FR-9, FR-10, FR-11 | P1-E2 |
| FR-12, FR-13, FR-14, FR-15 | P1-E3 |
| FR-17, FR-18, FR-19, FR-36 | P1-E4 |
| FR-23, FR-24, FR-25, FR-26, FR-27 | P1-E5 |
| FR-30, FR-31, FR-32 | P1-E6 |
| FR-45, FR-46, FR-47, FR-49, FR-50 | P1-E7 |
| FR-52, FR-53, FR-57, FR-61, FR-63 | P1-E8 |
| FR-40 (logs/traces por projeto) | P1-E2/P1-E3 |
| NFR-1..NFR-15 | Todos os epics, com gates em P1-E10 |

**Fora da Fase 1:** FR-6, FR-8? (incluído), FR-16, FR-20, FR-21, FR-22, FR-28, FR-29, FR-33, FR-34, FR-35, FR-37, FR-38, FR-39, FR-41, FR-42, FR-43, FR-44, FR-48, FR-51, FR-54, FR-55, FR-56, FR-58, FR-59, FR-60, FR-62, FR-64.

---

## Fase 1 Exit Criteria (G2)

1. 3 design partners em piloto, ≥ 100k/s cada, com dados reais.
2. UJ-1 ponta a ponta aprovada em pelo menos um parceiro.
3. TTFV p50 < 5 min; p90 < 15 min.
4. p95 dashboard 6 painéis < 2s; 12 painéis < 3s; 1B/24h < 1s; 10B/7d < 3s.
5. Busca de logs < 3s; busca de traces < 2s; 100k pontos < 100ms; bundle < 180KB.
6. 7 dias de soak sem perda P0; lag <10s p95.
7. Audit mínimo e production readiness gate aprovados.
8. `v0.5.0-beta` publicado com docs e notas.

---

## Plano de 8 sprints (16 semanas)

| Sprint | Semanas | Foco | Stories |
|---|---:|---|---|
| S1 | 5-6 | Ingestão + fundação de identidade | 1.1, 1.2, 1.3, 7.1, 7.2, 8.1 |
| S2 | 7-8 | WAL/quotas + métricas base | 1.4, 1.5, 1.6, 2.1, 2.2, 8.3 |
| S3 | 9-10 | Catálogo/cardinalidade + logs | 2.3, 2.4, 2.5, 3.1, 3.2, 5.1 |
| S4 | 11-12 | Traces + Explore/dashboards | 3.3, 3.4, 4.1, 4.2, 5.2, 5.3 |
| S5 | 13-14 | Correlação + dashboards avançados | 4.3, 4.4, 5.4, 5.5, 6.1 |
| S6 | 15-16 | Alertas + RBAC/audit | 6.2, 6.3, 6.4, 7.3, 7.4, 7.5 |
| S7 | 17-18 | Onboarding + performance | 8.4, 8.5, 9.1, 9.2, 10.1, 10.2 |
| S8 | 19-20 | Hardening + beta | 9.3, 10.3, 10.4, 10.5 + buffer |

**Regra de sprint:** cada sprint termina com demo, benchmark quando aplicável e atualização do `PROGRAM-PLAN`/riscos. Se uma story de ingestão/query atrasar, corta-se escopo de UI, não a durabilidade ou o isolamento.
