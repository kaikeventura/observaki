# ADR-0001 — Armazenamento: ClickHouse + VictoriaMetrics

- **Status:** proposto
- **Data:** 2026-10-31
- **Decisores:** Fundador + Arquitetura
- **Revisores:** pendente
- **Consultados:** —
- **Relacionados:** AD-5, AD-9, AD-21, AD-29; D-04; P-02; S0.1, S0.4, S0.8; Stories 3.1–3.5
- **Substitui:** —
- **Substituído por:** —

## Contexto

O Observaki armazena três sinais com padrões de acesso diferentes:

- **Métricas** são séries temporais com muitas séries, consultas PromQL,
  rollups de 1m/5m e retenção longa (90 dias no exemplo do PRD). A
  cardinalidade é o principal risco de custo e de noisy neighbor.
- **Logs** são eventos com busca por texto, atributos, serviço e tempo; o
  volume domina o footprint (30% dos eventos, ~200 B cada).
- **Traces** são spans correlacionados por `trace_id`/`span_id`, com waterfall
  e service map; a compressão e o custo por span importam (10% dos eventos,
  ~500 B cada).

O workload de referência é 60% métricas / 30% logs / 10% traces, ~134 B
comprimidos por evento e 1M eventos/s no benchmark do MVP (≈11,6 TB/dia).
As metas de consulta são p95 < 1 s em 1B linhas/24h e < 3 s em 10B linhas/7d
(NFR-2). A compressão precisa ser ≥ 5x em logs e traces (NFR-13).

Um único store simplificaria a operação, mas teria de entregar PromQL,
compressão de eventos, row policies e custo previsível ao mesmo tempo. O
spine já fixou o split por forma de acesso (AD-5): ClickHouse para logs,
traces e eventos; VictoriaMetrics para métricas, séries temporais e RED. Este
ADR registra a decisão, as alternativas e o que ainda precisa ser provado na
Fase 0.

## Decisão

**Adotar dois stores especializados, atrás do Query Service:**

| Sinal | Store autoritativo | Observações |
|---|---|---|
| Logs | ClickHouse | `MergeTree`, partição/TTL por `ingested_at`, `event_id` para dedupe, busca limitada a campos indexados no MVP |
| Traces | ClickHouse | `MergeTree`, `trace_id`/`span_id`, retenção própria; analytics de traces retidos não é RED autoritativo |
| Eventos genéricos | ClickHouse | Mesmo contrato de identidade e retenção dos logs |
| Métricas | VictoriaMetrics | MetricsQL, retenção global no MVP, rollups 1m/5m |
| RED / span metrics | VictoriaMetrics | Calculado no gateway antes do sampling (AD-21); nenhum dashboard/alerta consome RED do ClickHouse |

Regras que decorrem da decisão:

1. **Métricas não vão para o ClickHouse no MVP.** Logs e traces não vão para o
   VictoriaMetrics. O Query Service roteia por sinal; a UI, a API e os alertas
   nunca escolhem o store (AD-6).
2. **RED tem um único dono:** o gateway calcula antes de qualquer sampling e
   grava no VictoriaMetrics. O antigo `span_metrics` no ClickHouse não é fonte
   de verdade e não pode alimentar dashboard, alerta ou service map (AD-21).
3. **ClickHouse:** schema versionado e aditivo (AD-9); `ingested_at` é a
   autoridade de partição/TTL (AD-29); `event_id` estável; row policies e
   predicado de tenant em toda consulta (AD-3); sem `SELECT *` em produção.
4. **VictoriaMetrics:** acesso via `vmauth`; admissão de cardinalidade antes do
   ack (AD-33); retenção global no MVP; retenção por projeto/tenant de métricas
   é candidata enterprise e depende de P-03.
5. **Retenção:** logs e traces por projeto (FR-40); métricas globais no MVP. O
   default 7/15/30 dias permanece pendente em P-02 e será registrado em
   ADR-0005.
6. **Compressão e custo:** o benchmark mede a razão comprimido/bruto por sinal,
   o custo por TB/dia e o sizing com réplica; nenhum número de vendor entra no
   material sem reprodução própria (ADR-0006).

**Fora do escopo desta decisão:** fator de replicação exato, tiering cold,
archive search e federação. Eles ficam para o fim da Fase 0 ou para as Fases
3/4.

## Alternativas consideradas

| Alternativa | Prós | Contras | Por que não foi escolhida |
|---|---|---|---|
| **ClickHouse + VictoriaMetrics** | Cada store no seu melhor caso: PromQL/cardinalidade no VM, compressão/SQL/TTL no CH; escala independente; prova pública em 10M+/s | Dois sistemas para operar, backup, atualizar e monitorar; consistência entre stores não é transacional | **Escolhida** para o MVP |
| Um único ClickHouse para todos os sinais | Um store a menos; SQL único; operação mais simples | PromQL não é nativo; cardinalidade de métricas exige schema e cuidado; RED e métricas competem com logs/traces | Rejeitada no MVP; reavaliar na Fase 3 com benchmark |
| Elasticsearch para logs/traces | Busca full-text madura, ecossistema | JVM, custo de RAM/disco, gestão de shards, operação pesada | Rejeitada |
| Loki + Tempo | Barato em object storage, integração Grafana | Consulta limitada, indexação mínima, não é APM; dependência de outro produto | Rejeitada para o core |
| OpenObserve | Simples, Rust, Parquet/Vortex, baixo custo | Menos profundidade de APM; benchmarks do vendor | Alternativa para um modo “Lite” futuro, não para o core |
| Prometheus como store de métricas | Padrão de facto, ecossistema | Single-node, RAM alta em cardinalidade, retenção limitada | Compatibilidade via scrape/remote write, não store principal |
| Mimir | Escala horizontal, multi-tenant | Muitas dependências, operação pesada | Rejeitada |
| ClickHouse para métricas + logs/traces | Um store, SQL | Métricas não têm PromQL nativo; cardinalidade e retenção longa pioram o custo | Rejeitada no MVP; reavaliar na Fase 3 |

## Consequências

### Positivas

- Cada sinal usa o motor com melhor razão performance/custo para o seu padrão
  de acesso.
- Métricas mantêm PromQL/MetricsQL e compatibilidade com o ecossistema
  Prometheus; logs/traces ganham SQL, TTL e compressão do ClickHouse.
- Escala independente: um pico de cardinalidade de métricas não derruba a
  busca de logs, e vice-versa.
- O split força o contrato do Query Service, impedindo que UI/alertas criem
  atalhos para os stores.

### Negativas

- Duas operações de storage: upgrade, backup/restore, capacidade, monitoração
  e runbooks duplicados.
- Não há consistência transacional entre os stores; correlação é feita por
  `trace_id` e por consulta, não por join.
- Duas políticas de retenção e duas semânticas de timestamp; a UI precisa
  deixar claro o que é global e o que é por projeto.
- O time precisa dominar dois modelos de schema e duas curvas de upgrade.

### Riscos aceitos e mitigações

| Risco | Probabilidade | Impacto | Mitigação | Dono |
|---|---|---|---|---|
| Schema ClickHouse não atinge p95 < 1 s em 1B linhas | Média | Alto | Spike S0.1; ajustar `ORDER BY`, projeções e MVs; benchmark por consulta | Arquitetura |
| Cardinalidade do VictoriaMetrics estoura e afeta vizinhos | Média | Alto | Admissão antes do ack (AD-33), limites por tenant, teste de noisy neighbor (S0.4) | Arquitetura |
| Retenção por projeto não é viável por coluna/TTL | Média | Médio | Spike S0.8; fallback para tabelas por bucket de retenção | Arquitetura |
| Compressão fica abaixo de 5x | Baixa | Alto | Medir por sinal no S0.1; ajustar codec, ordem e materialização | Benchmark |
| Custo operacional de dois stores supera o ganho | Média | Médio | Medir horas/semana no S0.6 e no TCO; reavaliar store único na Fase 3 | Fundador + Arquitetura |

## Status

`proposto`. A decisão está registrada e sustenta os spikes, mas não pode ser
promovida para `aceito` antes de:

1. S0.1 provar ingestão, consulta e compressão do ClickHouse;
2. S0.4 provar cardinalidade e compatibilidade do VictoriaMetrics;
3. S0.8 provar retenção por projeto e isolamento;
4. revisão humana registrada na seção **Revisão** (AC3 da Story 1.4).

## Revisão

| Revisor | Papel | Data | Resultado | Referência |
|---|---|---|---|---|
| pendente | — | — | pendente | — |

## Questões abertas

| ID | Questão | Spike/story da Fase 0 | Dono | Prazo | Fallback |
|---|---|---|---|---|---|
| Q-01 | O schema ClickHouse sustenta 1M/s, p95 < 1 s em 1B linhas/24h e compressão ≥ 5x? | S0.1 / Story 3.1–3.3 | Arquitetura | Semana 3 | Ajustar `ORDER BY`, projeções/MVs; último recurso, store de eventos alternativo |
| Q-02 | O VictoriaMetrics sustenta 1M samples/s e a cardinalidade de 100k/1M/10M séries sem degradar vizinhos? | S0.4 / Story 3.4 | Arquitetura | Semana 3 | Limites por tenant, agregação na admissão; reavaliar ClickHouse para métricas na Fase 3 |
| Q-03 | TTL por projeto é viável por coluna ou exige tabelas por bucket (7/15/30 dias)? | S0.8 / Story 3.5 | Arquitetura | Semana 4 | Tabelas/MVs por bucket com roteamento explícito |
| Q-04 | Qual o fator de replicação e o sizing de disco por topologia? | S0.1 / Story 2.4 | Benchmark | Fim da Fase 0 | Cluster conservador com headroom; revisar após o primeiro design partner |
| Q-05 | A compressão de logs e traces atinge 5x com o workload de referência? | S0.1 / Story 2.1–2.3 | Benchmark | Semana 3 | Ajustar codec/ordem; reportar o número real e bloquear o gate se ficar abaixo |
| Q-06 | Um store único (ClickHouse) volta a ser competitivo na Fase 3? | Story 2.4 / Fase 3 | Arquitetura | Antes da Fase 3 | Manter o split; decisão só com benchmark próprio |

## Referências

- PRD: FR-7, FR-11, FR-40; NFR-1, NFR-2, NFR-13; §10.1 (escala), §11 (decisões abertas).
- Addendum: §4.3 (logs/traces), §4.4 (métricas), §5 (modelo de dados), §6 (benchmark), §7 (spikes).
- Arquitetura: AD-5, AD-9, AD-21, AD-29; `ARCHITECTURE-SPINE.md`.
- Revisão adversarial: `review-architecture.md`, achados C-02, C-04, A-16, M-05.
- Decisões: D-04 (stack), P-02 (retenção default), P-03 (fronteira open-core).
- História: [`DECISIONS.md`](../../_bmad-output/planning-artifacts/DECISIONS.md).

## Histórico

| Data | Mudança | Autor |
|---|---|---|
| 2026-10-31 | Criação (Story 1.4) | OpenCode |
