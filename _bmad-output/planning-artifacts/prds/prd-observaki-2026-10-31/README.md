# Observaki — PRD e Plano por Fases

**Status:** final. As decisões estruturantes foram confirmadas em 2026-10-31; as pendências restantes não bloqueiam arquitetura/UX/epics.

## Arquivos

- [`prd.md`](./prd.md) — visão, usuários, glossário, 64 FRs, non-goals, MVP, métricas, NFRs, restrições e plano por fases.
- [`addendum.md`](./addendum.md) — arquitetura, stack, evidências de benchmark, modelo de dados, TCO, segurança, governança e desdobramento técnico.
- [`review-pm.md`](./review-pm.md) — revisão adversarial que originou a re-baseline.
- [`.memlog.md`](./.memlog.md) — registro de decisões e premissas da sessão.

**Pacote completo de planejamento:**
- [`../../PROGRAM-PLAN.md`](../../PROGRAM-PLAN.md) — plano executivo ponta a ponta.
- [`../../ARCHITECTURE-SPINE.md`](../../architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md) — invariantes de arquitetura.
- [`../../DESIGN.md`](../../ux-designs/ux-observaki-2026-10-31/DESIGN.md) e [`../../EXPERIENCE.md`](../../ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md) — design e experiência.
- [`../../epics.md`](../../epics.md) — backlog da Fase 0.
- [`../../epics-phase-1.md`](../../epics-phase-1.md) — backlog da Fase 1.
- [`../../epics-phase-2-4.md`](../../epics-phase-2-4.md) — planejamento das Fases 2-4.

## Decisões confirmadas

| Decisão | Escolha |
|---|---|
| **Escala** | 1M eventos/s no benchmark do MVP; 100k/s por design partner em produção inicial; 10M/s no GA; 100M+/s via federação. 1B/s literal em um cluster está fora do v1. |
| **Licença** | Apache 2.0 no núcleo + open-core para recursos enterprise. |
| **ICP do MVP** | Startups e times pequenos (3-50 engenheiros). Enterprise é a Fase 3. |

## Resumo do plano

| Fase | Janela | Entrega | Gate |
|---|---|---|---|
| **0. Fundação e benchmarks** | Semanas 0-4 | Stack provada em 1M eventos/s, ADRs, harness, deploy esqueleto, 3 design partners | G1 benchmark |
| **1. MVP** | Semanas 5-20 | Três sinais, correlação mínima, Explore, dashboards, alertas, auth, audit mínimo, rollups | UJ-1 ponta a ponta; 3 pilotos |
| **2. APM e SLOs** | Semanas 21-32 | Service map completo, RED, error tracking, tail sampling, SLOs, agente beta, migração Grafana/Prometheus | 1M/s em produção assistida; 10 design partners |
| **3. Escala e enterprise** | Semanas 33-46 | HA, multi-tenant, SSO/SCIM, audit completo, operator, DR, agente GA, 10M/s, GA 1.0 | 10M/s; 30 dias de soak; 3 tenants |
| **4. Expansão** | Semanas 47-62+ | RUM, sintéticos, eBPF, profiling, LLM observability, IA, marketplace | ≥ 5 design partners por novo sinal |

**GA de engenharia:** ~semana 46 (~10,5 meses). **Compromisso com buffer de 30%:** ~semana 60 (~14 meses). Time assumido: 5-6 engenheiros em tempo integral.

## Pendências não bloqueantes

1. **Design partners:** quem são os 3 primeiros e qual o compromisso mínimo (dados reais, feedback semanal)?
2. **Retenção default:** 7, 15 ou 30 dias para logs/traces?
3. **Frontend:** SvelteKit (recomendado) ou React se o time for React-heavy?
4. **Fronteira open-core:** quais recursos enterprise (SSO/SCIM, audit avançado, RBAC granular, federação, suporte) ficam fora do Apache 2.0?

## Próximos passos sugeridos

1. Rodar a Fase 0: benchmark, ADRs e recrutamento dos design partners.
2. Com o PRD aprovado, invocar:
   - `bmad-ux` para a especificação de UX/IA;
   - `bmad-architecture` para a arquitetura formal;
   - `bmad-create-epics-and-stories` para o backlog por fase.

## O que este PRD deliberadamente não é

- Não é um clone completo do Datadog no v1.
- Não promete 1B/s em um único cluster.
- Não inclui RUM, session replay, sintéticos, profiling ou eBPF no MVP.
- Não é uma oferta SaaS gerenciada no v1.
