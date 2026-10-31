# Observaki — Pacote de Planejamento

**Status:** planejamento completo, nenhum código de produto escrito. Pronto para revisão e para iniciar a Fase 0.

## Mapa dos artefatos

| Artefato | Status | O que contém |
|---|---|---|
| [`PROGRAM-PLAN.md`](PROGRAM-PLAN.md) | final | Plano executivo ponta a ponta: fases, gates, epics, staffing, releases, riscos |
| [`DECISIONS.md`](DECISIONS.md) | vivo | Registro de decisões confirmadas, pendências e índice de ADs |
| [`prds/prd-observaki-2026-10-31/prd.md`](prds/prd-observaki-2026-10-31/prd.md) | final | Visão, 64 FRs, NFRs, non-goals, métricas, fases |
| [`prds/prd-observaki-2026-10-31/addendum.md`](prds/prd-observaki-2026-10-31/addendum.md) | final | Stack, benchmarks, modelo de dados, TCO, segurança, governança |
| [`architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md`](architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md) | final | 35 invariantes (AD-1..AD-35), diagramas, convenções, stack pinada |
| [`architecture/architecture-observaki-2026-10-31/review-architecture.md`](architecture/architecture-observaki-2026-10-31/review-architecture.md) | revisão | Achados adversariais e correções aplicadas |
| [`ux-designs/ux-observaki-2026-10-31/DESIGN.md`](ux-designs/ux-observaki-2026-10-31/DESIGN.md) | final | Identidade visual, tokens, tipografia, componentes |
| [`ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md`](ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md) | final | IA, fluxos, estados, atalhos, acessibilidade |
| [`ux-designs/ux-observaki-2026-10-31/review-ux.md`](ux-designs/ux-observaki-2026-10-31/review-ux.md) | revisão | Achados adversariais e correções aplicadas |
| [`ux-designs/ux-observaki-2026-10-31/mockups/`](ux-designs/ux-observaki-2026-10-31/mockups/) | referência | 5 mocks HTML offline: Explore, Dashboard, Service Map, Onboarding, States |
| [`epics.md`](epics.md) | ready-for-dev | Fase 0 detalhada: 7 epics, 31 stories, AC Given/When/Then |
| [`epics-phase-1.md`](epics-phase-1.md) | ready-for-refinement | Fase 1: 10 epics, 46 stories, AC resumidos |
| [`epics-phase-2-4.md`](epics-phase-2-4.md) | planned | Fases 2-4: epics/stories em nível de planejamento |
| [`stories/story-1.1-tasks.md`](stories/story-1.1-tasks.md) | ready-for-dev | Story 1.1 quebrada em 9 tarefas, 5,25 dev-days |

## Ordem de leitura

1. **`PROGRAM-PLAN.md`** — entenda o programa, fases e gates.
2. **PRD + addendum** — entenda o produto, requisitos, stack e benchmarks.
3. **ARCHITECTURE-SPINE.md** — entenda os invariantes que o código deve respeitar.
4. **DESIGN.md + EXPERIENCE.md** — entenda a UI e os fluxos.
5. **`epics.md`** — comece a executar a Fase 0.
6. **`epics-phase-1.md` / `epics-phase-2-4.md`** — veja o que vem depois; detalhe apenas quando o gate anterior for aprovado.

## Gates

| Gate | Quando | Evidência |
|---|---|---|
| **G0** | Antes da Fase 0 | Escala, licença e ICP confirmados (feito em 2026-10-31) |
| **G1** | Fim da Fase 0 | 1M/s, p95 < 1s em 1B, compressão ≥ 5x, 3 design partners |
| **G2** | Fim da Fase 1 | UJ-1 ponta a ponta, 3 pilotos, 7 dias de soak |
| **G3** | Fim da Fase 2 | 1M/s em produção assistida, 10 parceiros, MTTD/MTTR |
| **G4** | Fim da Fase 3 | 10M/s, 30 dias de soak, 3 tenants, GA 1.0 |
| **G5** | Fase 4 | ≥ 5 parceiros por novo sinal |

## Decisões confirmadas

- **Escala:** 1M/s no benchmark do MVP, 100k/s por piloto, 10M/s no GA, 100M+/s via federação; 1B/s literal fora do v1.
- **Licença:** Apache 2.0 no núcleo + open-core.
- **ICP:** startups/times pequenos (3-50 engenheiros); enterprise na Fase 3.
- **Stack:** Go 1.27, ClickHouse 26.8 LTS, VictoriaMetrics 1.148 LTS, NATS 2.15, PostgreSQL 18.6, SvelteKit 3.1, Svelte 5.57, uPlot 1.6.32.
- **Arquitetura:** ports & adapters + pipes-and-filters; 35 ADs.
- **UX:** dark-first, desktop-first, keyboard-first.

## Pendências não bloqueantes

1. Design partners: nomes e compromisso mínimo.
2. Retenção default: 7, 15 ou 30 dias.
3. Fronteira open-core exata.
4. Frontend: SvelteKit 3 confirmado? Se o time for React-heavy, reavaliar.
5. Compliance: SOC 2/HIPAA/FedRAMP no roadmap comercial.
6. Time/prazo: 5-6 engenheiros e ~14 meses são realistas?

## Próximo passo

Revisar este pacote e, se aprovado, iniciar a **Fase 0** por `epics.md` — começando pela Story 1.1 (`stories/story-1.1-tasks.md`). Nenhuma fase começa sem o gate anterior.
