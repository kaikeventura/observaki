# Observaki — Registro de Decisões

**Status:** vivo. Cada decisão tem dono, estado e critério de reversão. O rationale completo está nos `.memlog.md` de cada run; este arquivo é o índice executivo.

## Decisões confirmadas

| ID | Decisão | Data | Dono | Evidência |
|---|---|---|---|---|
| D-01 | Escala: 1M/s no benchmark do MVP, 100k/s por piloto, 10M/s no GA, 100M+/s via federação; 1B/s literal fora do v1 | 2026-10-31 | Fundador | PRD §10.1, A1 |
| D-02 | Licença Apache 2.0 no núcleo + open-core | 2026-10-31 | Fundador | PRD §8.7, addendum §12.2 |
| D-03 | ICP do MVP: startups/times pequenos (3-50 engenheiros); enterprise na Fase 3 | 2026-10-31 | Fundador | PRD §2.1, A15 |
| D-04 | Stack: Go 1.27, ClickHouse 26.8 LTS, VictoriaMetrics 1.148 LTS, NATS 2.15, PostgreSQL 18.6, SvelteKit 3.1, Svelte 5.57, uPlot 1.6.32 | 2026-10-31 | Fundador + Arquitetura | Architecture stack |
| D-05 | Paradigma: ports & adapters + pipes-and-filters; 35 ADs | 2026-10-31 | Arquitetura | ARCHITECTURE-SPINE.md |
| D-06 | UX: dark-first, desktop-first, keyboard-first; DESIGN/EXPERIENCE spines | 2026-10-31 | Produto + Design | UX spines |

## Pendências não bloqueantes

| ID | Decisão | Dono | Deadline | Critério de reversão |
|---|---|---|---|---|
| P-01 | Design partners: 3 nomes e compromisso mínimo | Fundador | Semana 4 | Se não houver 3, usar dados sintéticos e adiar o gate de produto |
| P-02 | Retenção default: 7, 15 ou 30 dias | Produto | Semana 2 | Se o custo do design partner exigir, reduzir para 7 |
| P-03 | Fronteira open-core exata | Fundador | Antes da Fase 1 | Se a comunidade reagir mal, manter segurança básica e observabilidade no OSS |
| P-04 | Frontend: SvelteKit 3 vs React | Fundador + Frontend | Semana 1 | Se o time for React-heavy, React 19 + uPlot |
| P-05 | Compliance: SOC 2/HIPAA/FedRAMP | Fundador | Antes da Fase 3 | Se um contrato exigir, antecipar trilha |
| P-06 | Time/prazo: 5-6 engenheiros, ~14 meses | Fundador | Semana 1 | Se o time for menor, cortar Fase 4 e/ou adiar GA |

## Índice de ADs

| AD | Tema | Fase |
|---|---|---|
| AD-1 | Dois planos, um produto | 0 |
| AD-2 | Identidade de telemetria | 0 |
| AD-3 | Isolamento em profundidade | 0 |
| AD-4 | Ingestão OTLP at-least-once/WAL | 0 |
| AD-5 | Split de storage | 0 |
| AD-6 | Query Service é o único read path | 0 |
| AD-7 | PostgreSQL system of record | 0 |
| AD-8 | Frontend SPA tipada | 0 |
| AD-9 | Schema/contratos versionados | 0 |
| AD-10 | Auto-observabilidade | 0 |
| AD-11 | Open-core | 0 |
| AD-12 | Mesmas imagens, topologias diferentes | 0 |
| AD-13 | Segurança sem criptografia caseira | 0 |
| AD-14 | Performance é orçamento de CI | 0 |
| AD-15 | Erros/dados com forma única | 0 |
| AD-16 | OTLP/PromQL são compatibilidade | 0 |
| AD-17 | Contexto de authz assinado | 1 |
| AD-18 | Idempotência/durabilidade | 1 |
| AD-19 | Config plane separado | 1 |
| AD-20 | Tenant identity/epoch | 1 |
| AD-21 | RED tem um dono (gateway→VM) | 2 |
| AD-22 | Contrato interno protobuf N-1 | 1 |
| AD-23 | Protocolo de consulta | 1 |
| AD-24 | Meta-observabilidade | 1 |
| AD-25 | Envelope operacional | 1/3 |
| AD-26 | Contratos externos/support matrix | 0/1 |
| AD-27 | Scrape owner | 1/2 |
| AD-28 | Modelo canônico do control plane | 1 |
| AD-29 | Ingested_at autoridade de TTL | 0/1 |
| AD-30 | Entitlements | 1/3 |
| AD-31 | Auth interna/least privilege | 1 |
| AD-32 | Alerting outbox | 1/2 |
| AD-33 | Cardinality admission | 1 |
| AD-34 | Frontend security | 1 |
| AD-35 | Audit immutability | 1/3 |

## Gates

| Gate | Critério | Evidência |
|---|---|---|
| G1 | 1M/s, p95 <1s/1B, compressão ≥5x | `BENCHMARKS.md` |
| G2 | UJ-1, 3 pilotos, soak 7d | Pilot report |
| G3 | 1M/s produção assistida, 10 parceiros, MTTD/MTTR | Field report |
| G4 | 10M/s, 30d soak, 3 tenants, GA | GA checklist |
| G5 | ≥5 parceiros por novo sinal | Phase 4 gate |

## Como usar

- Toda story que toca tenancy, storage, auth, RED, retenção ou upgrade deve citar o AD correspondente.
- Toda mudança de decisão confirmada exige novo AD ou emenda no AD (nunca renumerar).
- Decisão pendente que virar bloqueante entra no gate da fase; decisão não bloqueante pode ser deferida com dono/data.
