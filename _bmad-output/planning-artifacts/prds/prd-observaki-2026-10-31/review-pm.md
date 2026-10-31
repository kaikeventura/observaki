# Revisão Adversarial de PRD — Observaki

**Revisor:** PM adversarial (subagente)
**Data:** 2026-10-31
**Arquivos revisados:** `prd.md` (1.082 linhas) e `addendum.md` (774 linhas)
**Lente:** decision-readiness para arquitetura, UX e epics/stories; consistência interna e entre documentos; disciplina de escopo; realismo de fases; lacunas de produto Datadog-like open source; credibilidade de performance; riscos de produto/execução.
**Nota:** as referências de linha correspondem aos arquivos lidos nesta data. Esta revisão não altera o PRD nem o addendum.

---

## Veredito

O PRD tem boa visão, non-goals fortes e disciplina de benchmark, mas não está decision-ready: promete uma plataforma integrada e performática em prazos irreais para 4-6 engenheiros, mede a performance em uma ordem de grandeza incompatível com a meta de 1M/s e adia para fases posteriores justamente os elementos que tornariam o produto defensável — correlação, operabilidade enterprise, multi-tenancy real e segurança.

## Decisão recomendada

**Não avançar para arquitetura/epics/stories como plano comprometido.** Avançar apenas para uma fase curta de decisão e re-baseline, com os gates da seção final. O PRD serve bem como visão e backlog; ainda não serve como contrato de entrega.

## Sumário de severidade

| Severidade | Qtde | Temas dominantes |
|---|---:|---|
| Crítico | 5 | escala/benchmark; realismo do plano; correlação fora do MVP; performance dependente da Fase 2; multi-tenancy/retenção/auditoria |
| Alto | 18 | prontidão de produção; durabilidade/RPO; credibilidade de throughput; busca; sampling; tail sampling; noisy neighbor; custo; migração; adoção/GTM; UX; OTel/PromQL; operabilidade; fast path; design partners; licença; open questions; FRs parcialmente faseados |
| Médio | 21 | números inconsistentes; UJ-4; exit criteria; retenção; single-node; FRs ausentes; Prometheus import; OTLP; budgets de frontend; acessibilidade; dashboards; alertas; WAL; upgrade; SLO self-hosted; compliance; docs; Keeper/HA; Fase 4; PII; segurança de dashboards públicos |
| Baixo | 6 | glossário; i18n; mobile; timezone; ARM; release/trademark |

**Critério de severidade:** Crítico = bloqueia decisão ou invalida uma promessa central; Alto = gera retrabalho caro, risco de execução ou de confiança; Médio = ambiguidade que produz inconsistência entre times; Baixo = polimento e disciplina documental.

---

## Achados Críticos

### C-01 — A escala-promessa não é uma unidade comparável, não foi confirmada e é medida em outra ordem de grandeza

**Evidência:** PRD §1 (l. 24), §10.1 (l. 910-924), FR-7 (l. 194), FR-12 (l. 244), SM-2 (l. 819), SM-3 (l. 820), §8.2 (l. 852), A1 (l. 1070), Open Question 1 (l. 1055); Addendum §6.1 (l. 428-441), §6.2 (l. 452-460), glossário de evento (PRD l. 84).

**Análise:** 1M eventos/s = 86,4B eventos/dia. Os benchmarks de consulta usam 1B linhas/24h (≈11,6k eventos/s), 10B/7d (≈16,5k/s) e 40B/30d (≈15,4k/s). Ou seja, a performance headline é validada a ~1-2% da escala de ingestão prometida. Além disso, “evento” agrega sample de métrica, linha de log e span; sem mix e payload de referência, 1M/s pode significar 150 MB/s ou vários GB/s. E o requisito do fundador (“milhões/bilhões por segundo”) foi convertido em 1M/s MVP e 10M/s GA por assunção (A1), não por confirmação.

**Impacto:** não é possível dimensionar hardware, comparar benchmarks, defender p95<1s em 1M/s, calcular custo/TB ou saber se o produto atende ao requisito real do fundador. O número mais importante do PRD não é falsificável.

**Correção sugerida:**
1. Obter confirmação escrita do fundador sobre unidade e topologia: 1B/s por cluster, agregado, ou 1B/dia?
2. Definir workload de referência com mix por sinal e distribuição de payload (p50/p95).
3. Benchmarkar T1 a 86,4B eventos/dia e T2 a 864B/dia, reportando eventos/s, bytes/s, TB/dia e p95 de consulta em 1B/86B/605B/2,6T linhas.
4. Usar o mesmo mix em SM-2, SM-3, UJ-4 e no modelo de custo.
5. Se 1B/s for real, re-arquitetar antes da Fase 0; se for 1B/dia, simplificar o plano.

### C-02 — O cronograma de 12 meses para 4-6 engenheiros não sobrevive a uma estimativa bottom-up

**Evidência:** PRD §9 (l. 903), §10.2 (l. 928-936), Fase 0 (l. 938-953), Fase 1 (l. 955-975); Addendum Fase 1 (l. 530-549), Fase 2 (l. 551-567), Fase 3 (l. 569-585), Fase 4 (l. 587-599).

**Análise:** a Fase 0 de 3 semanas precisa montar 11-17 nós, provar 1M/s por 1h, p95<1s em 1B linhas, compressão ≥5x, caos, Helm/Compose, ADRs e recrutar 3 design partners. A Fase 1 de 12 semanas concentra cerca de **33 dos 64 FRs do produto (mais da metade de todo o escopo funcional)** em 15 epics: gateway OTLP, Prometheus scrape/remote write, pipeline com preview, ClickHouse, VictoriaMetrics, consulta de métricas/logs/traces, Explore, dashboards, alerting, identidade, agente Rust beta, deploy e hardening. A Fase 2 tem 12 epics e ~24 FRs em 10 semanas; a Fase 3 tem 12 epics em 12 semanas incluindo HA, SSO/SCIM, operator, air-gapped, DR, federação, 10M/s, Terraform, pentest e GA. Não há buffer, contratação, designer, SRE, technical writer ou DevRel. O próprio A4 diz que as durações não incluem férias, contratação ou mudanças de escopo (l. 936).

**Impacto:** o plano não é crível; a equipe vai cortar qualidade, segurança ou escopo silenciosamente; os gates vão escorregar em cascata; o compromisso de 12 meses vira risco de credibilidade com design partners e investidores.

**Correção sugerida:** refazer a estimativa por epic com o time real; cortar o MVP para uma fatia vertical de um caso de uso (ex.: OTLP metrics+logs+traces + Explore + dashboards + alertas + auth básica); adiar agente GA, Prometheus scrape, import Grafana, consultas avançadas e enterprise; mover o agente para a Fase 2; separar “benchmark spike” de “production readiness”; adicionar 30-40% de buffer; definir papéis (frontend, Rust, SRE, design, docs) e um plano de contratação.

### C-03 — O MVP não entrega a correlação que justifica o produto

**Evidência:** Vision (l. 20), UJ-1 (l. 48-54), FR-15 (l. 266-273), FR-36 (l. 479-486), MVP Scope (l. 784-797), critério de saída da Fase 1 (l. 970-975).

**Análise:** a visão promete “unifica métricas, logs, traces, dashboards, alertas e correlação em uma única experiência”. A UJ-1 — a jornada âncora — depende de “logs deste trace”, de métrica no mesmo painel e de navegação trace→log→métrica. FR-15 (correlação log-trace) e FR-36 (correlação trace-log-metric) são Fase 2. O MVP entrega três silos + dashboards + alertas; o critério de saída da Fase 1 não testa a UJ-1 de ponta a ponta. O resultado é um produto que, no lançamento, se parece com SigNoz/Grafana em features onde eles já são maduros, sem o diferencial que o PRD usa para justificar a existência.

**Impacto:** proposta de valor não demonstrável no MVP; design partners não veem o “unificado”; risco de construir “mais um SigNoz com Svelte”; UJ-1 falha no primeiro contato.

**Correção sugerida:** trazer para a Fase 1 a correlação mínima por `trace_id`/`span_id` (log→trace, trace→log e pelo menos um link trace→métrica/exemplar); adicionar “UJ-1 end-to-end” como critério de saída da Fase 1; ou reposicionar explicitamente o MVP como “três sinais + dashboards” e mover a UJ-1 para a Fase 2, ajustando a visão e o marketing.

### C-04 — As metas de performance da Fase 1 são financiadas por capacidades da Fase 2

**Evidência:** FR-24 (l. 364), FR-27 (l. 391-392), SM-2 (l. 819), NFR §8.1 (l. 842-847); FR-11 rollups (l. 224-231); Addendum E2.12 cache/pré-agregação (l. 565), §6.2 técnicas (l. 462).

**Análise:** o PRD exige 12 painéis em <2s, 100k pontos em <100ms, p95<1s em 1B linhas e 30 dias em <3s. Rollups são Fase 2 (FR-11), cache e otimização de consulta são Fase 2 (E2.12), tiering é Fase 3. A Fase 1 tem ClickHouse/VM crus, sem pré-agregação. Com 12 consultas concorrentes sobre 1B linhas, p95<2s não é crível sem over-provisioning. O addendum até qualifica “30d <3s (com rollups)”, mas o PRD vende o número sem a qualificação.

**Impacto:** o gate da Fase 1 não pode ser cumprido sem mover escopo ou inflar hardware; a promessa de performance perde credibilidade; o custo por volume (SM-4) quebra.

**Correção sugerida:** mover rollups/cache/índices para a Fase 1 ou reduzir o alvo da Fase 1 para um volume e um número de painéis definidos (ex.: 6 painéis, 1B linhas, p95<2s); criar orçamento por painel e por dashboard; declarar que 30d/<3s depende de rollups e só vale a partir da Fase 2; benchmarkar o dashboard de 12 painéis no volume-alvo, não só a query isolada.

### C-05 — Multi-tenancy, retenção por tenant e auditoria não fecham na Fase 1

**Evidência:** FR-40 (l. 521-528), FR-45 (l. 572-579), FR-5 auditoria (l. 170), FR-49 Fase 3 (l. 608-615), Addendum §5.3 (l. 349-354), A6 (l. 354), TTL fixo no ClickHouse (l. 284, 321), §5.5 (l. 414-420), FR-12 SQL (l. 241).

**Análise:** FR-40 promete retenção por tenant/projeto na Fase 1; o schema ClickHouse tem TTL global fixo (30d logs, 15d traces) e não há coluna de retenção. O VictoriaMetrics OSS tem retenção por tenant “quando disponível” e downsampling enterprise (A6), o que pode obrigar a usar VM Enterprise e quebrar a premissa Apache 2.0/OSS. FR-5 e FR-45 exigem eventos de auditoria na Fase 1, mas FR-49 (audit log) é Fase 3. O isolamento de consulta depende de parser/allowlist de SQL; não há row policies, bloqueio de table functions (`url`, `s3`, `remote`, `file`) nem quotas de consulta por tenant.

**Impacto:** critérios de aceitação da Fase 1 não podem ser cumpridos; risco de vazamento cross-tenant, noisy neighbor e dependência de componente proprietário; GDPR/exclusão de dados fica sem mecanismo; confiança enterprise comprometida.

**Correção sugerida:** decidir o boundary de tenancy (tenant = isolamento forte; projeto = subdivisão); desenhar retenção por tenant/projeto (tabelas/partições separadas ou coluna de TTL); validar retenção/downsampling do VM OSS na Fase 0; usar row policies/roles do ClickHouse e bloquear table functions/system tables; adicionar quotas de consulta por tenant; entregar auditoria mínima (auth, permissões, quotas, exportação) na Fase 1 e expandir na Fase 3.

---

## Achados Altos

### A-01 — Produção na Fase 1 sem os controles mínimos de produção

**Evidência:** critério de saída da Fase 1 (l. 970-975), FR-62 HA Fase 3 (l. 743-750), FR-63 backup/DR Fase 3 (l. 752-759), FR-49 audit Fase 3 (l. 608-615), Addendum E3.11 pentest (l. 582), runbooks Fase 3 (l. 1009).

**Impacto:** design partners rodando em produção sem HA, backup/restore, auditoria, pentest, runbooks ou upgrade path; um incidente ou perda de dados pode destruir a confiança antes do GA.

**Correção:** definir um “production readiness gate” para a Fase 1: backup/restore de metadados, runbooks, auditoria administrativa, revisão de segurança externa, upgrade/rollback testado e processo de incidente; ou rotular a Fase 1 como “piloto” e reservar “produção” para a Fase 3.

### A-02 — Durabilidade e RPO 0 não são sustentados pelo desenho

**Evidência:** FR-4 at-least-once e “sem duplicatas visíveis” (l. 157-160), Addendum §3.2 dedupe “quando possível” (l. 129-130), schemas `MergeTree` sem `event_id` em traces (l. 281, 318), FR-63 RPO 0 (l. 758), replicação 2 (l. 432).

**Impacto:** eventos confirmados podem ser perdidos em falha de nó/replicação assíncrona; duplicatas podem aparecer; RPO 0 é falso; requisitos de compliance/trust não são atendidos.

**Correção:** definir durabilidade por sinal; usar `ReplacingMergeTree`/token de dedupe; manter mensagens no NATS até confirmação de replicação/quorum no ClickHouse; especificar RPO por domínio de falha; alinhar FR-4 e FR-63.

### A-03 — Throughput do gateway e do agente não é crível com as evidências citadas

**Evidência:** Addendum l. 62-63 (Collector ~10k/instância; Rotel ~12,5k/core), l. 444 (gateway ≥50k eventos/s/vCPU), l. 504 (S0.2); PRD FR-51/SM-7 (l. 634, 827).

**Impacto:** 50k/vCPU é 4x o Rotel (Rust) citado e ~5x o Collector; 2 gateways podem não chegar a 1M/s. O agente a <1% de CPU e 10k eventos/s/host implica, em um host de 16 cores, ~62,5k eventos/s/core — implausível para coleta + parsing + OTLP. As metas de baixo footprint podem ser ficção de benchmark.

**Correção:** benchmark com o mesmo workload (spans vs logs vs métricas, payload, batching) para Go, Rotel e Collector; definir alvo por vCPU e por core com evidência; especificar vCPU por gateway; separar metas de agente para host metrics, logs e OTLP; publicar os resultados da Fase 0.

### A-04 — Busca full-text/atributos não tem índice adequado para 1B linhas/100M spans

**Evidência:** FR-12 (l. 241-246), FR-17 (l. 292-297), schema de logs com `tokenbf_v1` (l. 277), schema de traces com índices apenas em `trace_id` e `service_name` (l. 315-316), Addendum reconhece que `tokenbf_v1` é limitado (l. 293).

**Impacto:** buscas por texto livre e atributos customizados farão full scan; p95 de 3s/2s não será atingido; custo de query explode; a UX de investigação — o coração do produto — fica lenta.

**Correção:** adicionar inverted index/coluna tokenizada para full-text, materializar campos quentes (status, route, host, atributos top-N) e índices para atributos de trace; benchmarkar consultas seletivas e não seletivas; na Fase 1, limitar a busca aos campos indexados e documentar isso.

### A-05 — Span metrics/RED derivados de traces amostrados são enviesados

**Evidência:** FR-20 (l. 317-324), FR-22 tail sampling (l. 335-342), Addendum MV `span_metrics_1m` sobre `otel_traces` (l. 325-347).

**Impacto:** com tail sampling ou head sampling, as métricas RED calculadas a partir dos traces armazenados subcontam requests e distorcem latência/erro; SLOs, alertas e service map ficam errados. Dizer que “as métricas são retidas mesmo se o trace for amostrado” não resolve a acurácia.

**Correção:** calcular span metrics antes do sampling (gateway/collector) ou aplicar pesos de amostragem e validar; separar “métricas derivadas de todos os spans” do “store de traces amostrados”; testar com tail sampling ligado.

### A-06 — Tail sampling não tem componente, afinidade por trace nem buffer

**Evidência:** FR-22 (l. 335-342), arquitetura sem componente de tail sampling (Addendum l. 76-120), gateway stateless (l. 86), NATS com shard por tenant/signal (l. 87).

**Impacto:** decisão por trace exige bufferizar todos os spans de um trace e roteá-los para o mesmo decisor; um gateway stateless com shard por tenant/signal não garante isso. A implementação será ad hoc, com latência/memória imprevisíveis.

**Correção:** adicionar um tail-sampling router com afinidade por `trace_id`, buffer/timeout e modelo de capacidade; ou delegar ao OTel Collector e documentar topologias suportadas; incluir no benchmark de 1M/s.

### A-07 — Noisy neighbor de consulta não está resolvido

**Evidência:** FR-5 (l. 164-171), tabelas ClickHouse compartilhadas (l. 262-285), Query Service com limites por consulta (Addendum l. 148-154).

**Impacto:** um tenant com consultas pesadas ou merges grandes pode degradar os demais; o teste de “latência p95 dos outros tenants não varia mais de 10%” não tem baseline nem mecanismo de isolamento; SLO enterprise inviável.

**Correção:** quotas de concorrência/memória/CPU por tenant, isolamento de workload (shards/VM tenants), classes de prioridade (alertas vs dashboards vs ad hoc) e teste de caos com um tenant “quente”.

### A-08 — A economia vs Datadog não é TCO e os números não batem

**Evidência:** SM-4 (l. 821), UJ-4 20-30% (l. 76), Addendum §10 (l. 641, 655), A13 (l. 1082), cenário A (l. 637).

**Impacto:** a cunha central (“fração do custo do Datadog”) não é demonstrável; 20-30% (UJ-4) vs ≤30% infra/≤50% total (SM-4) vs 60-75% de economia (Addendum) são mensagens diferentes; a comparação usa preço de lista do Datadog contra custo de infraestrutura, sem migração, suporte, egress, backup e engenharia.

**Correção:** definir metodologia de TCO (infra + operação + migração + suporte), usar cotações reais de design partners, publicar modelo reproduzível com premissas e alinhar SM-4/UJ-4/Addendum em uma única narrativa.

### A-09 — Não há caminho de migração para alertas/dashboards existentes

**Evidência:** FR-26 import/export de dashboards (l. 377-384), FR-30 regras de alerta (l. 419-426), non-goal de substituir Prometheus/Grafana (l. 774).

**Impacto:** o custo de troca continua alto; usuários não migram centenas de regras Prometheus nem dashboards Grafana; a estratégia de convivência não tem ponte.

**Correção:** adicionar import de regras Prometheus/Grafana, import de dashboards com relatório de fidelidade, guia de coexistência e teste com configs reais; definir o que é “suportado” vs “melhor esforço”.

### A-10 — Adoção OSS/GTM sem funil, metas mensuráveis ou plano de distribuição

**Evidência:** SM-5 (l. 822), “sem phone-home por padrão” (l. 871), Addendum §12 (l. 692-701).

**Impacto:** “30 implantações ativas, 10 design partners, 15 contribuidores” não tem definição de ativo/contribuidor, baseline ou mecanismo de medição; com telemetria opt-in, a adoção será subcontada; não há plano de distribuição (Helm repo, imagens, binários), conteúdo, DevRel ou pipeline de design partners.

**Correção:** definir ativação/retenção (ex.: instalação com ingestão em 24h, uso semanal), telemetria opt-in, funil download→install→active→retained, metas por fase, distribuição e plano de comunidade/DevRel.

### A-11 — UX é consequência, não disciplina; “frontend moderno” não é especificado

**Evidência:** FR-23-29 (l. 350-411), NFR §8.6 (l. 882-887), ausência de IA/navegação/fluxos/design system/testes de usabilidade.

**Impacto:** a equipe de UX vai inventar padrões; a promessa de “frontend que parece instantâneo” não é testável; dashboards para 40 times não têm folders/tags/busca; acessibilidade de gráficos canvas não é tratada.

**Correção:** adicionar requisitos de UX: IA/navegação, project switcher, folders/tags/busca de dashboards, estados de erro/vazio/timeout, acessibilidade de gráficos, testes de usabilidade com design partners; incluir designer no staffing.

### A-12 — OTel-native e PromQL compatibility são promessas sem design de tradução

**Evidência:** FR-8 (l. 197-204), FR-7 (l. 188-195), Addendum VictoriaMetrics (l. 138, 349-354), glossário MetricsQL (l. 113).

**Impacto:** histogramas exponenciais OTel e exemplars não têm mapeamento para VM; `histogram_quantile` pode não retornar buckets corretos; dashboards/alertas PromQL migrados podem quebrar por diferenças de MetricsQL. A promessa OTel-native vira marketing.

**Correção:** desenhar o mapeamento OTel→VM (delta/cumulative, exponential histograms, exemplars) ou armazenar métricas OTel nativas no ClickHouse; publicar matriz de compatibilidade PromQL, testes de conformidade e ferramenta de migração; validar com SDKs reais.

### A-13 — Operabilidade/baixo footprint não são medidos e a topologia contradiz a promessa

**Evidência:** Addendum define “leve” (l. 27), PRD constraint (l. 900), topologia de referência (l. 852, 917), NFR §8.5 (l. 876-880).

**Impacto:** “operável por time pequeno” com 6 componentes stateful (ClickHouse, VM, NATS, Postgres, gateway, query/alerting) e 11-17 nós não é baixo footprint; não há meta de RAM/nó, horas de operação/semana ou custo/TB; a promessa não é falsificável.

**Correção:** definir SLOs de footprint (RAM/nó, ops hours/week, custo/TB, tempo de install/upgrade), medir no benchmark e simplificar a topologia ou investir em operator/automação desde a Fase 1.

### A-14 — Não há fast path explícito; performance é aspiração, não mecanismo

**Evidência:** FR-1/FR-3 (l. 128-153), Addendum ingestão (l. 122-132), consulta (l. 146-156), epics da Fase 1 (l. 530-549).

**Impacto:** o produto se posiciona por performance, mas não há um “fast path” de ingestão/consulta/UX com orçamento e ownership; a performance fica dependendo de tuning de ClickHouse/VM e de features da Fase 2.

**Correção:** especificar fast paths: ingestão (gateway nativo, batching, zero-copy), consulta (pré-agregação, cache, índices), UX (streaming/skeletons/cancelamento); atribuir FRs, fases e budgets; medir p99, não só p95.

### A-15 — Design partners são um gate opcional

**Evidência:** A9 (l. 1078), critério de saída da Fase 1 (l. 970-975).

**Impacto:** o plano pode “passar” a Fase 1 sem validação de produto, construindo a coisa errada com excelência técnica; o risco de product-market fit fica invisível.

**Correção:** tornar design partners um gate duro; definir pipeline de recrutamento, compromissos (dados reais, feedback semanal, caso de uso), evidência de valor e critérios de saída que incluam a UJ principal; não aceitar “apenas técnico”.

### A-16 — Licença/modelo comercial e fronteira open-core não estão decididos

**Evidência:** A2 (l. 1071), §8.7 (l. 894), Open Questions 2/3 (l. 1056-1057), Addendum (l. 25, 694-701).

**Impacto:** contribuidores e design partners não sabem o que será aberto vs pago; a decisão de open-core pode minar a confiança; se recursos necessários (retenção/downsampling) forem enterprise do VM, o produto não é 100% OSS.

**Correção:** decidir licença e fronteira open-core antes da Fase 1; publicar princípios de governança/monetização; clarificar trademark, DCO/CLA e implicações de contribuições; validar custos de VM Enterprise.

### A-17 — O PRD depende de 12 open questions, várias bloqueantes

**Evidência:** §11 (l. 1053-1066).

**Impacto:** arquitetura, UX e epics não podem tomar decisões irreversíveis sem retrabalho; o documento se declara fonte de verdade, mas delega decisões fundamentais.

**Correção:** fechar ou time-box as decisões necessárias para Fase 0/1 (unidade de escala, mix, retenção default, licença, storage, frontend, tenancy); atribuir owner e data; converter o resto em assunções explícitas com critério de validação.

### A-18 — FRs com fase “básico/avançado” não definem o corte do MVP

**Evidência:** FR-14 (l. 257-264), FR-19 (l. 308-315), FR-30 (l. 419-426), FR-32 (l. 437-444), FR-40 (l. 521-528), FR-51 (l. 632-639).

| FR | Fase declarada | O que falta definir |
|---|---|---|
| FR-14 | Fase 1 básico / Fase 2 avançado | “básico” inclui JSON, logfmt, regex, pattern inspector, alerta? |
| FR-19 | Fase 1 básico / Fase 2 completo | service map básico = arestas? latência? erros? owner? |
| FR-30 | Fase 1 métricas/logs; Fase 2 traces | semântica de regra para logs, backtesting, ausência de dados |
| FR-32 | Fase 1 básico; Fase 2 avançado | estado/dedup/silence “básico” vs agrupamento avançado |
| FR-40 | Fase 1 básico; Fase 2 avançado | retenção por projeto? por sinal? TTL? |
| FR-51 | Fase 1 beta; Fase 2 GA | o que é beta: Linux, host metrics, container logs, OTLP? |

**Impacto:** engenharia não sabe o que entra no MVP; estimativas e critérios de aceitação ficam ambíguos; o escopo real da Fase 1 pode ser maior ou menor do que o planejado.

**Correção:** substituir “básico/avançado” por dois FRs separados com consequências testáveis e fase explícita, ou definir o corte em uma frase verificável por FR.

### Matriz UJ → fase mínima para a jornada completa

| UJ | Fase mínima para a jornada completa | Evidência |
|---|---|---|
| UJ-1 (Marina) | Fase 2 | FR-15 e FR-36 são Fase 2; service map completo é Fase 2 |
| UJ-2 (Ravi) | Fase 1 | FR-1, FR-52, FR-53; agente beta na Fase 1 |
| UJ-3 (Ana) | Fase 2/3 | FR-43 custo é Fase 2; FR-49 audit e FR-48 SSO/SCIM são Fase 3 |
| UJ-4 (Léo) | Fase 0/1 (benchmark) | §10.1; mas a topologia da UJ-4 está errada (ver M-03) |

**Leitura:** só a UJ-2 é plenamente MVP. A UJ-1, que carrega o diferencial, e a UJ-3, que carrega o enterprise, não são. Isso deveria estar explícito no PRD; hoje o leitor assume que as quatro jornadas são o produto.

---

## Achados Médios

### M-01 — A linha do tempo é internamente inconsistente (GA na semana 37 vs 12 meses)

**Evidência:** A4 (l. 903), tabela de fases (l. 928-936), Addendum (l. 517-599).

**Impacto:** planejamento, staffing e expectativa do fundador ficam ambíguos; o GA está a ~8,5 meses, não 12; a Fase 4 começa depois do GA e vai até a semana 49+.

**Correção:** reconciliar calendário, declarar buffer e definir se a Fase 4 é pós-GA ou parte dos 12 meses.

### M-02 — Números inconsistentes entre PRD, addendum e UJs

**Evidência:** tabela abaixo.

**Impacto:** leitores não sabem qual número é o compromisso; benchmarks e custos perdem credibilidade.

**Correção:** uma única tabela de referência (topologia, retenção, latência, custo) referenciada por todos os documentos; remover duplicações.

| Tema | PRD | Addendum | Conflito / leitura |
|---|---|---|---|
| Nós ClickHouse para 1M/s | 6-12 (l. 820, 852, 917, 950) | 4-6 (l. 446); 12-24 para 15d (l. 438) | faixa de 3x; sizing e custo ambíguos |
| Topologia UJ-4 | 3 CH + 2 VM + 1 gateway, 5M/s (l. 74-76) | 4-6 CH para 1M/s (l. 446) | 3 nós não sustentam 5M/s; UJ-4 impossível |
| Throughput gateway | 1M/s com 2 gateways (l. 917, 950) | ≥50k eventos/s/vCPU (l. 444); Rotel 12,5k/core (l. 63) | 50k/vCPU = 4x Rotel; falta vCPU por gateway |
| CPU do agente | <1% a 10k/s/host (l. 634, 827) | sem baseline | em host 16-core, >60k/s/core; implausível |
| Volume de consulta | 1B linhas/24h (l. 194, 244, 819) | 1B/24h, 10B/7d, 40B/30d (l. 452-460) | 1M/s = 86,4B/24h; budgets ~1-2% da escala |
| Retenção | UJ-3 30d logs/90d métricas (l. 66); SM-3 7-15d (l. 820) | TTL 30d logs/15d traces (l. 284, 321); custo 30d (l. 627); referência 15d (l. 431) | 4 defaults diferentes; sizing/custo inconsistentes |
| Lag de ingestão | 5s p95 (l. 842); 10s p95 (l. 823, 974) | 10s p95/30s p99 (l. 448) | alvo 2x diferente; arquitetura de batching |
| Timing de alerta | condição 60s (l. 424); notificação 30s (l. 847) | avaliação 15-60s (l. 170) | até 90s end-to-end; SLO ambíguo |
| TTFV | p50<5min/p90<15min (l. 818); UJ-2 10min (l. 60); Compose <10min (l. 655) | install <15min single-node/<1h K8s (l. 876) | 5 vs 10 vs 15 vs 60; falta segmentação |
| Custo | ≤30% infra/≤50% total (l. 821); UJ-4 20-30% (l. 76) | economia 60-75%/40-60% (l. 641); 75-85%/60-75% (l. 655) | 20-30% vs ≤50%; lista vs TCO |
| Timeline | 12 meses até GA (l. 903) | GA semana 37; Fase 4 38-49+ (l. 928-936) | ~8,5 vs 12 meses |
| Meta de escala | 1M MVP/10M GA/100M federação (l. 1070) | 1B+ fora do v1 (l. 920) | requisito do fundador descopado por assunção |

### M-03 — UJ-4 contradiz a topologia de referência

**Evidência:** UJ-4 (l. 74-76), SM-3 (l. 820), Addendum §6.1 (l. 446).

**Impacto:** 3 nós ClickHouse não sustentam 5M eventos/s nem 1M/s com folga; a jornada de validação (Léo) é tecnicamente impossível como escrita, o que destrói a credibilidade da persona cética.

**Correção:** atualizar UJ-4 para a topologia T1 real (6-12 CH, 3 VM, 2 gateways) e separar cenários 100k/1M/5M com hardware explícito.

### M-04 — O critério de saída da Fase 1 valida 100k/s por design partner, não 1M/s

**Evidência:** Fase 1 exit (l. 971) vs SM-3/§8.2 (l. 820, 852).

**Impacto:** o “MVP 1M/s” é apenas benchmark, não produção; a promessa de escala não é validada com usuários reais até a Fase 2.

**Correção:** alinhar o critério de saída ao claim ou rotular 1M/s como “meta de benchmark” e 100k/s como “meta de produção do MVP”.

### M-05 — UJ-3 não é realizável no MVP

**Evidência:** UJ-3 (l. 64-70), FR-43 custo Fase 2 (l. 548), FR-49 audit Fase 3 (l. 608), FR-48 SSO Fase 3 (l. 599).

**Impacto:** a jornada enterprise usada para justificar multi-tenancy, cotas e governança não pode ser demonstrada no MVP; UX e epics podem superestimar o que existe.

**Correção:** mapear cada UJ para a fase; mover UJ-3 para a Fase 3 ou adicionar uma fatia enterprise mínima (custos por tenant + audit básico) na Fase 1/2.

### M-06 — Retenção default não está decidida e varia em cada documento

**Evidência:** UJ-3 30d logs/90d métricas (l. 66), SM-3 7-15d (l. 820), Open Question 7 (l. 1061), Addendum TTL 30d logs/15d traces (l. 284, 321), custo 30d (l. 627), referência 15d (l. 431).

**Impacto:** sizing, custo, UX e schema dependem de um número que não existe; cada área vai assumir um valor diferente.

**Correção:** decidir defaults por sinal/tier e usá-los em todos os cálculos; documentar a política de alteração.

### M-07 — “SQLite ou Postgres local” no single-node é ambíguo

**Evidência:** FR-53 (l. 657), Addendum (l. 176, 235).

**Impacto:** duas implementações de metadados, migrações e testes; o modo single-node pode divergir do produção.

**Correção:** escolher um default para single-node (SQLite) e um para produção (Postgres), com paridade de schema testada.

### M-08 — Syslog, HTTP JSON e Kafka são prometidos sem FR

**Evidência:** non-goal (l. 777) diz que o MVP cobre syslog/HTTP JSON; FR-1/FR-2 não os incluem; Addendum menciona HTTP JSON e Kafka opcional (l. 124, 132, 202).

**Impacto:** escopo do MVP ambíguo; arquitetura pode ou não implementar; testes de aceitação ausentes.

**Correção:** adicionar FRs ou remover a promessa do MVP; declarar Kafka como não-objetivo ou Fase 3.

### M-09 — “Importar prometheus.yml com 1.000 targets sem edição” é forte demais

**Evidência:** FR-2 (l. 142).

**Impacto:** configs reais usam service discovery, relabeling, auth e TLS; a promessa gera expectativa falsa e suporte.

**Correção:** definir o subset suportado (static configs, relabel básico) e testar com configs representativas; documentar limitações.

### M-10 — Semântica de `partial_success` e zstd no OTLP está imprecisa

**Evidência:** FR-1 (l. 130, 134).

**Impacto:** `partial_success` é resposta de sucesso (HTTP 200) com contagem de rejeitados, não “resposta de erro”; zstd não é compressão padrão OTLP em todos os SDKs. A implementação pode divergir do spec e quebrar interoperabilidade.

**Correção:** alinhar ao OTLP spec (gzip default, zstd opcional/extensão), separar erros HTTP de partial success e testar com SDKs oficiais.

### M-11 — Orçamentos de frontend não são testáveis como escritos

**Evidência:** NFR §8.1 (l. 845-846), Addendum §6.3 (l. 464-476), FR-27 (l. 391-392).

**Impacto:** “JS inicial <180KB”, “LCP <1,5s”, “100k pontos <100ms” não definem rota, device, rede, payload, se inclui parse/render/network. Cada time mede diferente.

**Correção:** definir rota inicial, device/rede de referência, se o budget inclui dados/render, e separar lab vs field (LCP/INP).

### M-12 — Acessibilidade não tem critério de aceitação

**Evidência:** NFR §8.6 (l. 884-887).

**Impacto:** WCAG 2.1 AA em gráficos canvas exige alternativas textuais, navegação por teclado e contraste; sem testes, a promessa não é verificável.

**Correção:** adicionar testes automatizados (axe), checklist manual, alternativas tabulares para gráficos e critérios por componente.

### M-13 — Dashboards não têm organização para escala (folders/tags/busca)

**Evidência:** FR-24-28 (l. 359-402), schema de dashboards sem folder/tag (Addendum l. 378-386), UJ-3 40 times (l. 64-70).

**Impacto:** com 40 times e centenas de dashboards, descoberta e governança quebram; a experiência “moderna” não escala.

**Correção:** adicionar folders, tags, busca, favoritos e permissões por pasta; incluir na Fase 1 ou 2 com critério de UX.

### M-14 — Alertas não têm repetição, escalonamento, on-call ou janelas de manutenção

**Evidência:** FR-30-35 (l. 419-471), FR-32 (l. 437-444).

**Impacto:** um alerta que continua firing pode nunca renotificar; não há escalonamento; SREs não conseguem operar; a promessa de “difícil de ignorar” não se sustenta.

**Correção:** adicionar repeat interval, escalation policies, on-call schedules, maintenance windows e grouping keys; integrar com PagerDuty/Opsgenie na Fase 2.

### M-15 — WAL/buffer de 15 minutos a 1M/s não tem sizing

**Evidência:** FR-4 (l. 160), Addendum NATS (l. 91-93, 445).

**Impacto:** 15 min a 1M/s = 900M eventos, ~135GB comprimidos (a 150B); com replicação 2, ~270GB. Sem sizing de disco/retenção do NATS, o teste de caos pode falhar ou o custo explodir.

**Correção:** dimensionar WAL para a janela de outage alvo, definir política de 429/eviction e testar 15 min a 1M/s.

### M-16 — Não há estratégia de evolução de schema/upgrade antes da Fase 3

**Evidência:** FR-53 Helm Fase 1 (l. 650-657), FR-62 upgrades Fase 3 (l. 743-750), schemas ClickHouse (l. 261-322).

**Impacto:** design partners ficam presos em versões antigas; mudanças de schema/materialized views quebram queries; upgrades sem downtime só na Fase 3.

**Correção:** adicionar versionamento/migração de schema, testes de upgrade e política de deprecação desde a Fase 1; ou declarar que a Fase 1 não é para produção.

### M-17 — SLO de disponibilidade não distingue produto self-hosted de operação do cliente

**Evidência:** SM-6 (l. 823), NFR §8.3 (l. 859), constraint self-hosted (l. 898).

**Impacto:** 99,9%/99,95% não são exigíveis de um produto self-hosted sem definir responsabilidades, medição e exclusões; risco de suporte/legal.

**Correção:** separar “SLO de referência do produto” (benchmark) de “SLO operado pelo cliente”; definir medição, responsabilidades e o que não está coberto.

### M-18 — Compliance e data residency não fecham com a persona enterprise

**Evidência:** A12 (l. 1081), non-goal (l. 780, 808), UJ-3 (l. 64-70).

**Impacto:** o comprador enterprise de UJ-3 provavelmente exige SOC 2, auditoria e residência de dados; o PRD os coloca depois do GA, sem validação com design partners.

**Correção:** validar requisitos com design partners; adicionar trilha de readiness (SOC 2, auditoria, residência) e critérios de “enterprise-ready” antes de vender para esse perfil.

### M-19 — Idioma da documentação está em aberto e é um entregável da Fase 1

**Evidência:** A8 (l. 1077), docs Fase 1 (l. 797, 975).

**Impacto:** docs em inglês vs português afeta adoção OSS, contribuidores e suporte; a decisão não pode ficar para depois do MVP.

**Correção:** decidir inglês como idioma primário da docs pública e PT-BR para docs de produto; definir critérios de aceitação de docs e glossário.

### M-20 — Topologia não menciona ClickHouse Keeper/Postgres HA

**Evidência:** Addendum arquitetura (l. 76-120), storage (l. 136-140), topologias (l. 172-180), lição do SigNoz sobre Keeper (l. 43).

**Impacto:** replicação do ClickHouse exige Keeper/ZooKeeper; Postgres HA exige operador/Patroni; a operabilidade prometida fica subestimada.

**Correção:** incluir Keeper e HA do Postgres na topologia, no Helm e no custo; testar failover.

### M-21 — Redaction de PII é regex, sem classificação de dados nem teste de falso negativo

**Evidência:** FR-50 (l. 617-624), NFR §8.4 (l. 869), Addendum §11.2 (l. 677).

**Impacto:** regras regex para cartão de crédito não cobrem PII real (nomes, emails, tokens, chaves); um falso negativo vaza dados e compromete compliance; não há métrica de precisão/recall nem quarentena.

**Correção:** adicionar classificação de dados, biblioteca de detectores, corpus de teste com precision/recall, quarentena para eventos suspeitos e auditoria de redaction.

### M-22 — Dashboards públicos/embed sem modelo de abuso e segurança

**Evidência:** FR-29 (l. 404-411), NFR §8.4 (l. 864-872).

**Impacto:** link assinado/iframe pode virar vetor de scraping, DDoS ou vazamento se o escopo/expiração não forem auditados; não há rate limit nem CSP.

**Correção:** definir escopo mínimo, expiração, revogação, rate limit, CSP/frame-ancestors e auditoria de acesso; testar com dados sensíveis.

---

## Achados Baixos

### B-01 — A disciplina de glossário é violada pelo próprio documento

**Evidência:** glossário (l. 80-116), uso de “tenant” vs “organização” (l. 95, 574), “sinal” para RUM/profiling (l. 778, 1021-1029), “evento” vs “evento de telemetria” (l. 84).

**Impacto:** ambiguidade em FRs e stories; retrabalho de nomenclatura.

**Correção:** normalizar termos e adicionar um lint de glossário no review de PRD.

### B-02 — Idioma da UI e i18n não são especificados

**Evidência:** nenhum requisito de i18n; A8 só trata docs (l. 1077).

**Impacto:** decisões tardias de UX/engenharia; lançamento em inglês ou português sem critério.

**Correção:** declarar UI em inglês no v1 (ou PT-BR) e i18n como não-objetivo.

### B-03 — Mobile/responsivo não é non-goal

**Evidência:** non-goals (l. 770-780) não mencionam mobile; NFR de frontend (l. 845) fala em desktop.

**Impacto:** escopo implícito; dashboards em mobile podem ser esperados por usuários.

**Correção:** declarar “desktop-first; mobile somente leitura fora do v1” ou incluir requisito mínimo.

### B-04 — Timezone/clock skew não têm requisito apesar de a UJ-2 citar relógio dessincronizado

**Evidência:** UJ-2 edge case (l. 62), sem NFR de sincronização.

**Impacto:** traces com clock skew distorcem waterfall/caminho crítico; diagnóstico pode ser errado.

**Correção:** adicionar requisito de NTP/clock skew, tolerância e UX de aviso.

### B-05 — Suporte a ARM no backend não está claro

**Evidência:** compatibilidade Linux x86_64/arm64 (l. 891), agente arm64 (l. 634), imagens de backend não especificadas.

**Impacto:** promessa de ARM pode não valer para ClickHouse/VM/gateway; clientes ARM podem ser surpreendidos.

**Correção:** definir matriz de imagens multi-arch e testar.

### B-06 — Cadência de release/trademark/governança só existem no addendum

**Evidência:** Addendum §12 (l. 692-701), PRD Open Question 11 (l. 1065).

**Impacto:** o PRD (fonte de verdade) não contém a promessa de release/suporte; inconsistência de governança.

**Correção:** resumir no PRD ou referenciar explicitamente o addendum como normativo.

---

## O que está bom (e deve ser preservado)

- **Non-goals explícitos e corajosos** (l. 770-780): SIEM, BI, SDK proprietário, SaaS, paridade total com Datadog, bilhões/s e compliance no dia um. Isso evita a morte por escopo.
- **Calibração de escala** (l. 910-924): separar unidade e topologia, e reconhecer que 1B/s é 100x o GA, é exatamente o tipo de rigor que falta em PRDs de observabilidade.
- **OTel-native sem SDK proprietário** (l. 899, 775) e compatibilidade com Collector (FR-52).
- **Disciplina de benchmark** (Addendum l. 68, 478-495): harness reproduzível, cenários de caos, relatório por release, caveat sobre benchmarks de vendor. É a melhor parte do addendum.
- **Counter-metrics** (l. 831-836): volume, frescor, alertas e compressão. Raro e valioso.
- **Edge cases das UJs** (l. 54, 62, 70, 78): amostragem, diagnóstico de ingestão, cardinalidade, gargalo de benchmark. Mostram maturidade de produto.
- **Acessibilidade e segurança básica** (l. 884, 866-872): WCAG, SBOM, imagens assinadas, CVE SLA.
- **Governança OSS** (Addendum l. 692-701): Apache 2.0, DCO, RFC, COC, SECURITY, roadmap público.
- **Custo visível** (FR-43): tornar custo uma feature de produto é a decisão certa para a cunha de eficiência.
- **Separação PRD/addendum**: em princípio, boa; na prática, precisa de uma fonte única para números (ver M-02).

---

## Gates para avançar para arquitetura/epics

1. **Escala confirmada:** o fundador assina unidade, topologia e mix de referência; 1B/s vs 1B/dia resolvido.
2. **Plano re-baselineado:** estimativa bottom-up, MVP cortado para uma fatia vertical, buffer de 30-40%, staffing por papel.
3. **Correlação no MVP ou posicionamento corrigido:** trace-log na Fase 1 ou UJ-1 explicitamente Fase 2.
4. **Tenancy/retenção/auditoria desenhadas:** boundary, TTL por tenant, row policies, quotas de consulta, audit mínimo na Fase 1; VM OSS validado.
5. **Performance financiada:** rollups/cache/índices na Fase 1 ou metas da Fase 1 reduzidas e qualificadas; orçamento por painel.
6. **Production readiness gate:** backup/DR, runbooks, auditoria, pentest, upgrade/rollback antes de design partners em produção.
7. **Licença/GTM/adoção decididos:** Apache 2.0 + fronteira open-core, funil de adoção, distribuição e pipeline de design partners.
8. **Números unificados:** uma tabela de referência para topologia, retenção, latência, custo e timeline; PRD e addendum sem duplicação divergente.

---

## Perguntas que o fundador precisa responder

1. “Bilhões por segundo” é por cluster, agregado ou por dia? Qual o alvo comprometido: 1M/s, 10M/s, 100M/s ou 1B/s?
2. Qual é o ICP primário do MVP: startup de 40 pessoas (Ravi) ou enterprise de 1.200 engenheiros (Ana)? O MVP não serve bem aos dois.
3. O MVP pode ser lançado sem correlação trace-log, ou a correlação é o mínimo para o design partner?
4. Qual é o orçamento máximo de hardware e o limite de horas/semana de operação para 1M/s?
5. Apache 2.0 + open-core é aceitável? Quais features podem ser proprietárias sem quebrar a promessa OSS?
6. Quem são os 3 design partners e qual o compromisso mínimo (dados reais, feedback semanal, caso de uso)?
7. Qual é a restrição real de prazo e de time? 12 meses e 4-6 engenheiros são um limite ou uma estimativa?
8. Retenção de 30 dias para logs é requisito de produto ou hipótese de custo?

---

## Nota final

O PRD é mais forte em visão, non-goals e disciplina de benchmark do que a média dos PRDs de observabilidade. O problema não é falta de ambição nem falta de estrutura; é que o documento tenta comprimir um produto de plataforma (três sinais, correlação, multi-tenancy, enterprise, escala de 10M/s) em um MVP de 12 semanas com 4-6 engenheiros, e resolve a tensão com números que não se cruzam. A decisão mais valiosa agora não é “qual banco usar”; é **qual promessa o MVP precisa provar** e **qual escala é real**. Sem isso, arquitetura e epics vão produzir um plano tecnicamente elegante para um produto que o fundador não pediu ou que o time não consegue entregar.
