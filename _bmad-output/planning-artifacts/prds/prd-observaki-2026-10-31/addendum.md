# Addendum: Observaki — Arquitetura, Stack e Evidências

*Documento de apoio ao `prd.md`. Aqui ficam as decisões técnicas, os trade-offs, as evidências públicas, o modelo de dados, os benchmarks e o desdobramento técnico do plano por fases. O PRD decide o produto; este addendum decide o caminho técnico e registra o que foi considerado.*

---

## 1. Resumo executivo das decisões

| Camada | Decisão | Por que |
|---|---|---|
| Gateway de ingestão | **Go 1.27+**, stateless, OTLP nativo, batching e compressão | Concorrência barata, baixo overhead, ecossistema OTel, velocidade de desenvolvimento |
| Agente | **Rust** estático, Tokio, receptor OTLP, coleta de host (containers na Fase 2) | Binário pequeno, sem GC, baixo RSS/CPU, caminho para eBPF |
| Fila / WAL | **NATS JetStream** por padrão; adaptador Kafka/Redpanda opcional | Leve, baixa latência, at-least-once, operação simples; Kafka é pesado para o default |
| Logs e traces | **ClickHouse 26.8 LTS** | Ingestão e compressão comprovadas em escala; SQL analítico; TTL e tiering |
| Métricas | **VictoriaMetrics 1.148 LTS** | PromQL/MetricsQL, alta cardinalidade, baixo RAM; compatibilidade Prometheus |
| Metadados | **PostgreSQL 18.6** | ACID, JSONB, maturidade, ecossistema de operação |
| Cache | **Valkey/Redis** (opcional no single-node) | Cache de consulta, rate limit, sessões; LRU em processo no modo dev |
| Frontend | **SvelteKit 3.1 + Svelte 5.57 + TypeScript + Vite 8** | Runtime pequeno, reatividade simples, bundle controlável |
| Gráficos | **uPlot** para séries temporais; canvas/SVG para heatmaps; ECharts lazy para casos complexos | uPlot é ~50 KB e renderiza 166k pontos em ~25ms |
| Editor de consulta | **CodeMirror 6** | Leve, extensível, syntax highlighting e autocomplete |
| UI kit | **Tailwind CSS 4 + shadcn-svelte/bits-ui** | Componentes acessíveis, tree-shaking, controle visual |
| API | **ConnectRPC (gRPC + HTTP/JSON) + OpenAPI** | Contratos tipados, streaming e compatibilidade REST |
| Auth | **OIDC/OAuth2 + Argon2id** | Padrão corporativo, sem reinventar identidade |
| Deploy | **Docker Compose + Helm + binário único com UI embutida** | Dev rápido, produção Kubernetes, avaliação sem fricção |
| Licença | **Apache 2.0 no núcleo + open-core** (confirmado) | Maximiza adoção e contribuição; recursos enterprise licenciados separadamente |

**Definição de “leve” neste projeto:** não é “um único processo para sempre”. É **custo por evento, RAM por nó e horas de operação por semana**. A stack evita JVM, Elasticsearch e Kafka no caminho padrão; usa Go/Rust, ClickHouse e VictoriaMetrics; e oferece um modo single-node para avaliação. Um cluster de produção continua tendo componentes distribuídos — como qualquer sistema que promete 1M+ eventos/s com durabilidade.

**Workload de referência:** 60% métricas (~40 B), 30% logs (~200 B), 10% traces (~500 B); média ~134 B comprimidos por evento. 1M/s ≈ 134 MB/s ≈ 11,6 TB/dia. Todos os benchmarks, metas de query e o modelo de custo usam esse mix; mudanças exigem recalibração explícita.

---

## 2. Pesquisa de mercado e evidências

### 2.1 O que o Datadog entrega (benchmark de produto, não de arquitetura)

O Datadog é a referência de escopo porque combina, em um único produto: APM com tracing distribuído, log management, métricas e infraestrutura, RUM, synthetic monitoring, observability pipelines, error tracking, SLOs, dashboards e alertas. A página de APM descreve correlação de traces com logs, métricas, RUM e sinais de segurança; a de logs descreve “Logging without Limits”, processamento, roteamento, archive e pattern inspector; a de preços mostra a lógica de cobrança por host, por GB ingerido e por milhão de eventos indexados. O Watchdog adiciona anomalias e correlação automática.

**Lição para o Observaki:** o valor não está em ter “mais um gráfico”, mas em **correlação entre sinais** e **eficiência de custo**. O MVP precisa acertar os três sinais + dashboards + alertas + correlação básica; a paridade de portfólio vem depois.

### 2.2 O que o mercado open source já provou

| Produto | Arquitetura | Forças | Lacunas / lições |
|---|---|---|---|
| **SigNoz** | ClickHouse + Keeper + PostgreSQL; UI própria | OTel-native, APM, logs e métricas; comunidade crescente | Operação de ClickHouse é pesada; Helm ainda arrasta Zookeeper em alguns caminhos; menos profundidade em alguns recursos enterprise |
| **OpenObserve** | Binário Rust + Parquet/Vortex + object storage; nós stateless | Instalação simples, baixo custo de storage, HA sem sharding de banco | Menos foco em APM profundo; benchmarks são majoritariamente do próprio vendor |
| **Uptrace** | ClickHouse + PostgreSQL + Redis; UI Vue | APM OTel-native, SQL/PromQL, compressão forte | Dependências extras (Redis), menos tração comunitária |
| **ClickStack / HyperDX** | ClickHouse + OTel Collector + UI HyperDX | Stack oficial da ClickHouse, correlação e session replay; boa performance | Fortemente atrelado ao roadmap da ClickHouse Inc.; métricas PromQL não são o foco |
| **Grafana LGTM** | Loki + Grafana + Tempo + Mimir/Prometheus | Ecossistema enorme, flexível, maduro | Múltiplos stores e operação fragmentada; não é um produto unificado opiniativo |
| **VictoriaMetrics** | TSDB próprio, cluster version | Eficiência de RAM, alta cardinalidade, MetricsQL | Não é uma plataforma completa; é uma peça |
| **Netdata / Coroot** | Agente + UI; eBPF/auto-discovery | Instalação rápida, visualização de infra | Menos foco em logs/traces de longo prazo e multi-tenant |

**Conclusão:** há espaço para uma distribuição **integrada, OTel-native, performance-first e operável por times pequenos**. O Observaki não precisa inventar um novo banco; precisa orquestrar ClickHouse + VictoriaMetrics com um gateway nativo, um agente leve e uma UX superior.

### 2.3 Evidências de performance que sustentam o alvo

| Evidência | Número | Implicação |
|---|---|---|
| ClickHouse Cloud / LogHouse | **50M eventos/s**, ~10 GB/s comprimidos, 177 PiB não comprimidos | 1M/s é uma fração conservadora; 10M/s é viável com cluster maior |
| Netflix | **10,6M eventos/s**, ~5 PB/dia em ClickHouse | 1M/s com 4-8 nós é plausível, dependendo do tamanho do evento |
| LY Corporation | **7M rows/s** com **24 nós** ClickHouse | ~292k rows/s por nó; 1M/s pede ~4 nós + headroom |
| ClickHouse `otel` pipeline | Deploy inicial em 10M/s, escalou para 50M/s | Arquitetura de gateway + WAL + batch funciona em escala |
| VictoriaMetrics / Airbnb | **100M samples/s** de métricas com vmagent | Métricas não precisam do mesmo cluster de logs/traces |
| OTel Collector (benchmarks oficiais) | Testes de 10k spans/s, 10k logs/s; ~10k eventos/s por instância em configuração típica | Para 1M/s, é preciso gateway nativo ou frota grande de collectors |
| Rotel (Rust) vs OTel Collector | ~12,5k eventos/s por core, ~4x o Collector em tracing | Gateway em linguagem nativa compensa em escala |
| ClickHouse vs OpenObserve (benchmark do vendor) | OpenObserve ~3x mais rápido em 1B logs, 1/3 do disco | Object storage é uma alternativa válida para logs; validar com benchmark próprio |
| uPlot | 166.650 pontos em ~25ms, ~50 KB | Frontend pode renderizar séries grandes sem WebGL |
| Datadog pricing | Logs a partir de $0,10/GB + indexação ~$1,70/M eventos; APM ~$31-40/host; infra ~$15-23/host | Há espaço para 3-5x de economia em volumes altos |

> **Nota de rigor:** benchmarks de vendor (OpenObserve, VictoriaMetrics, ClickHouse) são úteis como ordem de grandeza, não como verdade absoluta. A Fase 0 do Observaki existe para reproduzir os cenários com carga própria e publicar os resultados.

---

## 3. Arquitetura de referência

### 3.1 Visão de componentes

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                              FONTES DE TELEMETRIA                            │
│  OTel SDKs  │  OTel Collector  │  Observaki Agent (Rust)  │  Prometheus     │
│  (metrics/logs/traces)          │  host, containers, K8s   │  scrape/remote  │
└──────────────┬──────────────────┴──────────────┬───────────┴────────┬────────┘
               │ OTLP gRPC/HTTP                  │ OTLP                │ remote_write
               ▼                                 ▼                     ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                         INGEST GATEWAY (Go, stateless)                       │
│  auth/tenant → rate limit → validate → pipeline (filter/redact/enrich)      │
│  → batching → compression → shard by tenant/signal → WAL                    │
└───────────────────────────────┬──────────────────────────────────────────────┘
                                │
                     ┌──────────▼──────────┐
                     │  NATS JetStream     │  (WAL / buffer; Kafka opcional)
                     │  streams por sinal  │
                     └──────────┬──────────┘
                                │
          ┌─────────────────────┼─────────────────────┐
          ▼                     ▼                     ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│ ClickHouse       │  │ VictoriaMetrics  │  │ PostgreSQL       │
│ logs + traces    │  │ métricas         │  │ metadados        │
│ + span metrics   │  │ MetricsQL        │  │ + controle       │
└────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘
         │                     │                     │
         └─────────────────────┼─────────────────────┘
                               ▼
                  ┌──────────────────────────┐
                  │   QUERY SERVICE (Go)     │
                  │  planner, tenant guard,  │
                  │  cache, limits, async    │
                  └────────────┬─────────────┘
                               │
                  ┌────────────▼─────────────┐
                  │  API (ConnectRPC/OpenAPI)│
                  │  + UI SvelteKit embutida │
                  └────────────┬─────────────┘
                               │
                  ┌────────────▼─────────────┐
                  │  Alerting / Scheduler    │
                  │  SLOs, notificações      │
                  └──────────────────────────┘
```

### 3.2 Caminho de ingestão

1. **Recepção:** o gateway aceita OTLP gRPC/HTTP, Prometheus remote write e HTTP JSON (adaptadores syslog/JSON entram na Fase 2). Autenticação por token de projeto; o tenant é resolvido antes de qualquer processamento.
2. **Validação:** limites de tamanho, número de atributos, cardinalidade e taxa. Rejeições parciais retornam `partial_success` (HTTP 200) com contagem e motivo; erros de protocolo retornam 4xx/5xx.
3. **Pipeline:** filtros, drop, redaction, enriquecimento (K8s metadata, owner, environment), amostragem e roteamento. Regras são compiladas e cacheadas; o preview usa uma amostra real.
4. **Batching:** lotes de 10k-100k eventos ou 1-5 segundos, com zstd. O batch é a unidade de compressão e de escrita.
5. **WAL:** publicação no NATS JetStream por subject `tenant.signal.shard`. O gateway só confirma ao cliente depois do ack do JetStream (ou de um buffer local durável, no modo single-node). Para 1M/s, o WAL é dimensionado para 15 minutos de outage: ~900M eventos e ~120 GB comprimidos; com réplica 2, ~240 GB de disco.
6. **Writers:** consumidores em Go leem os streams, fazem dedupe idempotente por `event_id`, escrevem em ClickHouse via protocolo nativo (logs/traces) e em VictoriaMetrics via remote write (métricas). Batches grandes, assíncronos, com retry e backoff; o writer só dá ack no NATS após confirmação de flush/insert no storage.
7. **Confirmação:** o pipeline é **at-least-once**. O produto não promete exactly-once; promete ausência de perda para eventos confirmados dentro da janela de buffer/quórum e deduplicação por `event_id` quando o produtor o fornece. Eventos não confirmados podem ser perdidos; rejeições permanentes vão para DLQ com replay.
8. **Tail sampling (Fase 2):** um roteador com afinidade por `trace_id` bufferiza spans até a decisão de amostragem; a topologia e o capacity model são documentados e testados em 1M/s.

**Por que NATS e não Kafka no default?** Kafka é excelente, mas pesado: JVM, ZooKeeper/KRaft, tuning de partições e operação. NATS JetStream é um binário Go, com baixa latência e at-least-once, e escala horizontalmente por stream. Para clientes que já operam Kafka, um adaptador de ingestão e de consumo permite usar o broker existente. O benchmark da Fase 0 decide se o NATS fica no caminho crítico ou se o gateway escreve direto no ClickHouse com WAL local.

### 3.3 Armazenamento

**ClickHouse (logs e traces).** É o motor principal de dados de eventos. Vantagens: colunar, compressão por codec, SQL analítico, skip indexes, TTL, materialized views, tiering para S3 e ingestão de milhões de linhas por segundo. O schema é desnormalizado e orientado a consultas de observabilidade: `tenant_id`, `project_id`, `service_name`, `timestamp`, atributos em `Map`, `trace_id`, `span_id`, `duration_ns`, `status_code`.

**VictoriaMetrics (métricas).** Métricas têm um padrão de acesso diferente: muitas séries, consultas PromQL, retenção longa, alta cardinalidade. O VictoriaMetrics é mais eficiente em RAM que Prometheus e oferece MetricsQL compatível. O cluster version permite separar `vminsert`, `vmselect` e `vmstorage`.

**PostgreSQL (metadados).** Organizações, projetos, usuários, dashboards, painéis, regras de alerta, canais, silences, SLOs, tokens e auditoria. ACID e JSONB são suficientes; não faz sentido colocar isso em ClickHouse.

**Object storage (S3-compatível).** Cold tier de logs/traces, backups e archive. O modo single-node não exige S3.

**Cache.** Valkey/Redis para cache de consulta, rate limit distribuído e sessões. No single-node, um LRU em processo evita a dependência.

### 3.4 Consulta

O **Query Service** é uma camada Go que:
- valida a consulta e injeta o filtro de tenant obrigatório;
- escolhe o motor (ClickHouse SQL, MetricsQL ou busca de traces);
- aplica limites de tempo, linhas, memória e concorrência;
- usa cache de resultado com chave normalizada (consulta + intervalo + tenant + versão do schema);
- executa consultas pesadas como jobs assíncronos com streaming de progresso;
- devolve resultados em formato colunar para a UI (Arrow/JSON) e metadados de execução.

**Linguagens:** MetricsQL para métricas; SQL para logs/traces com uma camada de filtros amigável; a UI oferece um modo “query builder” e um modo “SQL” para usuários avançados. A API expõe ambas.

### 3.5 Frontend

**SvelteKit 3 + Svelte 5 + TypeScript + Vite 8.** O Svelte compila para JavaScript pequeno e não carrega um runtime grande; a reatividade por runes é simples de ensinar. O SvelteKit dá roteamento, code splitting e SSR opcional — no Observaki, a UI é uma SPA servida pelo binário Go, com fallback de rotas.

**uPlot** para séries temporais: canvas 2D, ~50 KB, 166k pontos em ~25ms. Para heatmaps e gráficos não temporais, usar canvas/SVG customizado ou ECharts carregado sob demanda (nunca no bundle inicial). **CodeMirror 6** para o editor de consulta, com autocomplete de métricas/labels e syntax highlighting. **TanStack Table (adaptador Svelte) ou svelte-headless-table** para tabelas virtualizadas. **Tailwind 4 + shadcn-svelte/bits-ui** para componentes acessíveis e consistentes.

**Orçamento de performance:** JS inicial < 180 KB gzip; rota de dashboard < 250 KB gzip; LCP < 1,5s; INP < 200ms; gráfico de 100k pontos < 100ms; tabela de 10k linhas a 60fps. O CI roda Lighthouse e um benchmark de renderização com dados sintéticos.

**Acessibilidade:** todo gráfico canvas tem tabela alternativa e resumo textual; navegação por teclado; contraste AA; testes axe no CI. Acessibilidade não é um “polish” do fim; entra no definition of done de cada componente.

**Por que não React?** React tem o maior ecossistema e seria uma escolha segura se o time já for React-heavy. Svelte vence em tamanho de runtime e simplicidade para uma UI de dados densa. A decisão fica registrada como ADR; se o time contratado for majoritariamente React, o fallback é React 19 + Vite + TanStack + uPlot, com o mesmo orçamento de bundle.

### 3.6 Alertas e SLOs

O **Scheduler** avalia regras a cada 15-60 segundos. Cada regra é compilada para uma consulta no motor apropriado. O estado do alerta vive no PostgreSQL; notificações passam por uma fila com deduplicação, agrupamento e backoff. SLOs são calculados a partir de SLIs (métricas, logs ou traces) e armazenados como séries derivadas; burn-rate usa janelas curta e longa.

### 3.7 Topologias de deploy

| Topologia | Uso | Componentes |
|---|---|---|
| **Single-node** | Avaliação, dev, times pequenos | Binário Observaki + ClickHouse + VictoriaMetrics + PostgreSQL (Docker Compose) |
| **Cluster** | Produção | 4-6 gateways, 4+ NATS, 6-12 ClickHouse, 3+ VictoriaMetrics, 2+ API/Query, PostgreSQL HA |
| **Federated** | Múltiplas regiões/contas | Clusters regionais + query router global + metadata replicada |

O Helm chart é opinativo: valores padrão funcionam em um cluster de 3 nós; valores avançados expõem replicação, retenção, recursos e affinity. O operator entra na Fase 3 para gerenciar CRDs de tenant, retenção e alertas.

### 3.8 Fast paths e ownership de performance

Performance não é um efeito colateral de escolher ClickHouse; é um conjunto de caminhos explícitos com dono e orçamento:

| Fast path | Mecanismo | Dono | Orçamento |
|---|---|---|---|
| Ingestão | Gateway nativo Go, batches de 10k-100k, zstd, shard por tenant/signal, NATS, writers assíncronos | Backend/Ingest | p99 de processamento < 50ms; lag p95 < 10s |
| Consulta | Rollups 1m/5m, cache de resultado, projeções, `ORDER BY` alinhado, limites e cancelamento | Query/Storage | p95 < 1s (1B/24h); < 3s (10B/7d) |
| UX | uPlot, virtualização, skeletons, prefetch, streaming, cancelamento | Frontend | 100k pontos < 100ms; INP < 200ms |
| Alertas | Avaliação paralela, cache de consulta, prioridade sobre ad-hoc | Platform/SRE | avaliação < 60s; entrega < 90s |

Todo PR que toca o caminho crítico precisa de benchmark antes/depois no CI ou no harness de release. Regressão de performance é tratada como bug de severidade alta.

---

## 4. Stack: alternativas e critérios

### 4.1 Gateway: Go vs Rust vs OTel Collector

| Opção | Prós | Contras | Decisão |
|---|---|---|---|
| **Go** | Concorrência simples, OTel libs maduras, baixo overhead, rapidez de entrega | GC e pausas em picos extremos; menos eficiente que Rust por core | **Escolhido** para o gateway |
| **Rust** | Máxima eficiência, sem GC, binário pequeno | Custo de desenvolvimento maior, ecossistema OTel menos maduro | **Agente** e hot path futuro |
| **OTel Collector** | Compatibilidade total, processors prontos, comunidade | ~10k eventos/s por instância em configuração típica; frota grande em escala; config complexa | **Compatibilidade**, não o core |
| **Rotel** | Rust, ~4x throughput do Collector | Projeto mais novo; menos processors | Referência para o gateway nativo |

**Estratégia:** o Observaki aceita OTel Collector na borda (compatibilidade) e usa um gateway Go nativo para alto volume. A Fase 0 compara Go vs Rotel em 1M/s; se o Go não atingir o alvo, o hot path migra para Rust sem mudar o contrato OTLP.

### 4.2 Fila: NATS JetStream vs Kafka vs escrita direta

| Opção | Prós | Contras | Decisão |
|---|---|---|---|
| **NATS JetStream** | Leve, baixa latência, at-least-once, operação simples | Throughput por nó menor que Kafka; menos ecossistema | **Default** |
| **Kafka/Redpanda** | Throughput e durabilidade comprovados, ecossistema | Peso operacional, JVM/recursos, tuning | **Opcional** para clientes que já têm |
| **Escrita direta no ClickHouse** | Menos componentes, menor latência | Sem buffer durável; outages do CH viram perda ou backpressure | **Fallback** no single-node |

**Critério de decisão na Fase 0:** 1M/s com perda zero em falha de 15 minutos do ClickHouse, custo de operação e latência p95. Se o NATS não passar, o gateway usa WAL local em disco + retry, e o Kafka vira opção de produção.

### 4.3 Logs/traces: ClickHouse vs Elasticsearch vs Loki vs Tempo vs OpenObserve

| Opção | Prós | Contras | Decisão |
|---|---|---|---|
| **ClickHouse** | Performance, compressão, SQL, TTL, tiering, provas públicas em 10M+/s | Operação de cluster; merge pressure; schema design importa | **Escolhido** |
| **Elasticsearch** | Busca full-text madura, ecossistema | Pesado, caro, JVM, gestão de shards | Rejeitado |
| **Loki** | Simples, barato em object storage, integração Grafana | Consultas limitadas, indexação mínima, não é APM | Rejeitado para core |
| **Tempo** | Traces em object storage, barato | Consulta limitada; depende de Grafana | Rejeitado para core |
| **OpenObserve** | Simples, Rust, Parquet/Vortex, baixo custo | Menos profundidade de APM; benchmarks do vendor | Alternativa para “Lite” no futuro |

**Estratégia:** ClickHouse para logs e traces; OpenObserve é uma referência de arquitetura para o modo “object storage first”, não uma dependência.

### 4.4 Métricas: VictoriaMetrics vs Prometheus vs Mimir vs ClickHouse

| Opção | Prós | Contras | Decisão |
|---|---|---|---|
| **VictoriaMetrics** | Baixo RAM, alta cardinalidade, MetricsQL, cluster | Produto separado; menos “padrão” que Prometheus | **Escolhido** |
| **Prometheus** | Padrão de facto, ecossistema | Single-node, RAM alta em cardinalidade, retenção limitada | Compatibilidade via scrape/remote write |
| **Mimir** | Escala horizontal, multi-tenant, Grafana | Pesado, muitas dependências | Rejeitado |
| **ClickHouse** | Um store a menos; SQL | PromQL não é nativo; cardinalidade de métricas exige cuidado | Rejeitado para MVP; reavaliar na Fase 3 |

**Estratégia:** VictoriaMetrics para métricas; Prometheus pode fazer scrape e remote write. Isso mantém compatibilidade com o ecossistema e evita reinventar PromQL.

### 4.5 Metadados: PostgreSQL vs SQLite vs etcd

| Opção | Prós | Contras | Decisão |
|---|---|---|---|
| **PostgreSQL** | ACID, JSONB, replicação, maturidade | Mais um componente | **Produção** |
| **SQLite** | Zero operação, single-node | Sem HA, concorrência limitada | **Modo dev/single-node** |
| **etcd** | Consenso forte | Não é banco de aplicação; queries pobres | Rejeitado |

### 4.6 Frontend: SvelteKit vs React vs Vue

| Opção | Prós | Contras | Decisão |
|---|---|---|---|
| **SvelteKit** | Runtime pequeno, reatividade simples, ótimo para UI densa | Ecossistema menor que React; contratação | **Escolhido** |
| **React** | Maior ecossistema, contratação fácil | Runtime maior; mais decisões de estado | Fallback se o time for React-heavy |
| **Vue** | Meio-termo, bom ecossistema | Runtime maior que Svelte; menos momentum em observabilidade | Rejeitado |

### 4.7 Gráficos: uPlot vs ECharts vs Chart.js vs D3

| Opção | Tamanho | Performance | Decisão |
|---|---|---|---|
| **uPlot** | ~50 KB | 166k pontos em ~25ms; canvas | **Séries temporais** |
| **ECharts** | ~1 MB | Rica, mas pesada; lazy | Apenas heatmaps/gráficos complexos sob demanda |
| **Chart.js** | ~200 KB | SVG/canvas, boa DX, menos adequada a 100k pontos | Rejeitado para core |
| **D3** | ~250 KB + custom | Máxima flexibilidade, muito código | Rejeitado para core |

---

## 5. Modelo de dados

### 5.1 ClickHouse: logs

```sql
CREATE TABLE otel_logs
(
    tenant_id           LowCardinality(String),
    project_id          LowCardinality(String),
    timestamp           DateTime64(9, 'UTC'),
    ingested_at         DateTime64(9, 'UTC'),
    observed_timestamp  DateTime64(9, 'UTC'),
    severity_text       LowCardinality(String),
    severity_number     UInt8,
    service_name        LowCardinality(String),
    body                String CODEC(ZSTD(3)),
    attributes          Map(String, String),
    resource_attributes Map(String, String),
    trace_id            String,
    span_id             String,
    event_id            String,
    INDEX idx_body  body      TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1,
    INDEX idx_trace trace_id  TYPE bloom_filter GRANULARITY 1,
    INDEX idx_span  span_id   TYPE bloom_filter GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(ingested_at)
ORDER BY (tenant_id, project_id, service_name, timestamp)
TTL toDateTime(ingested_at) + INTERVAL 15 DAY
SETTINGS index_granularity = 8192;
```

**Notas:**
- `PARTITION BY toDate(timestamp)` mantém partições gerenciáveis; para volumes muito altos, usar `toStartOfHour` ou partitionamento por tenant + dia.
- `ORDER BY` prioriza o filtro mais comum: tenant, projeto, serviço, tempo.
- `Map` é flexível, mas consultas em labels de alta cardinalidade podem ser caras; materializar campos quentes (status code, route, host) em colunas dedicadas.
- `TTL` implementa retenção; tiering para S3 usa `TTL ... TO VOLUME`.
- `tokenbf_v1` ajuda em full-text simples; no MVP, a busca é limitada a campos indexados e materializados. Inverted index/ngram para busca textual ampla entra na Fase 2, após benchmark de seletividade e custo.

### 5.2 ClickHouse: traces

```sql
CREATE TABLE otel_traces
(
    tenant_id           LowCardinality(String),
    project_id          LowCardinality(String),
    timestamp           DateTime64(9, 'UTC'),
    ingested_at         DateTime64(9, 'UTC'),
    trace_id            String,
    span_id             String,
    parent_span_id      String,
    service_name        LowCardinality(String),
    operation_name      String,
    duration_ns         UInt64,
    status_code         LowCardinality(String),
    status_message      String,
    event_id            String,
    attributes          Map(String, String),
    resource_attributes Map(String, String),
    events              Nested(name String, time DateTime64(9), attributes Map(String, String)),
    links               Nested(trace_id String, span_id String, attributes Map(String, String)),
    INDEX idx_trace trace_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_service service_name TYPE set(1000) GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(ingested_at)
ORDER BY (tenant_id, project_id, service_name, timestamp)
TTL toDateTime(ingested_at) + INTERVAL 15 DAY
SETTINGS index_granularity = 8192;
```

**Retenção por projeto:** os DDLs mostram TTL fixo para legibilidade. A implementação de retenção por projeto usa uma de duas estratégias, decidida no spike da Fase 0: (a) TTL baseado em coluna `retention_days`, se a versão do ClickHouse suportar e o custo de merge for aceitável; (b) tabelas/materialized views separadas por bucket de retenção (7/15/30 dias), com o gateway roteando o evento para o bucket do projeto. A estratégia (b) é mais previsível e é o fallback.

**Span metrics/RED:** RED é calculado no gateway **antes** do tail sampling e gravado apenas no VictoriaMetrics (AD-21), com labels de serviço/operação/status e histograma de duração. Não há MV de RED no ClickHouse no MVP; se uma MV de análise de traces retidos for criada, ela é explicitamente não autoritativa e não alimenta dashboards, alertas ou service map.

### 5.3 VictoriaMetrics

- Tenant por path: `/insert/{tenant}/{project}/prometheus/api/v1/write` e `/select/{tenant}/{project}/prometheus/api/v1/query`; acesso via `vmauth` autenticado, não direto.
- RED/span metrics: armazenados aqui, calculados pelo gateway antes do sampling; retenção segue a política de métricas.
- Retenção por tenant: no OSS do VictoriaMetrics, a retenção é tipicamente por instância; retenção por tenant/projeto exige instâncias ou tenants separados, ou recurso enterprise. O MVP usa retenção global para métricas e retenção por projeto apenas para logs/traces; a retenção por projeto de métricas entra na Fase 2/3, após validação na Fase 0.
- Cardinalidade: limites por tenant via vmagent/vminsert; alertas de crescimento; agregação no gateway.
- Downsampling: no OSS, usar rollups via recording rules ou materialized views; downsampling nativo é enterprise. A estratégia é validada no spike da Fase 0 e não pode criar dependência proprietária no núcleo open source. `[ASSUMPTION: A6 — validar a estratégia de rollup no spike da Fase 0.]`

### 5.4 PostgreSQL: metadados

```sql
CREATE TABLE organizations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    generation BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug)
);

CREATE TABLE retention_policies (
    project_id UUID NOT NULL REFERENCES projects(id),
    signal TEXT NOT NULL CHECK (signal IN ('logs','traces','metrics')),
    retention_days INT NOT NULL,
    storage_tier TEXT NOT NULL DEFAULT 'hot',
    PRIMARY KEY (project_id, signal)
);

CREATE TABLE dashboards (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    title TEXT NOT NULL,
    definition JSONB NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_by UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification_channels (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    project_id UUID REFERENCES projects(id),
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('slack','webhook','email','pagerduty','opsgenie','teams')),
    config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_rules (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    signal TEXT NOT NULL CHECK (signal IN ('metrics','logs','traces')),
    query TEXT NOT NULL,
    condition JSONB NOT NULL,
    for_seconds INT NOT NULL DEFAULT 300,
    severity TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_rule_channels (
    rule_id UUID NOT NULL REFERENCES alert_rules(id),
    channel_id UUID NOT NULL REFERENCES notification_channels(id),
    PRIMARY KEY (rule_id, channel_id)
);

CREATE TABLE memberships (
    user_id UUID NOT NULL,
    org_id UUID NOT NULL REFERENCES organizations(id),
    project_id UUID REFERENCES projects(id),
    role TEXT NOT NULL CHECK (role IN ('admin','editor','viewer')),
    PRIMARY KEY (user_id, org_id, project_id)
);

CREATE TABLE api_tokens (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    scopes TEXT[] NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE silences (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    matchers JSONB NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    created_by UUID
);

CREATE TABLE slos (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    service_name TEXT NOT NULL,
    sli JSONB NOT NULL,
    target NUMERIC NOT NULL,
    window_days INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE error_budgets (
    slo_id UUID PRIMARY KEY REFERENCES slos(id),
    consumed NUMERIC NOT NULL DEFAULT 0,
    remaining NUMERIC NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    org_id UUID NOT NULL,
    actor_id UUID,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    metadata JSONB,
    prev_hash TEXT,
    hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

*O modelo canônico é `notification_channels` + `alert_rule_channels`, `retention_policies` (não colunas duplicadas em `projects`), `memberships` com role, `api_tokens`, `silences`, `slos`, `error_budgets` e `audit_events` append-only com hash chain (AD-28/AD-35). Dashboards usam `definition JSONB` e não têm entidade `PANEL` separada.*

### 5.5 Identidade e isolamento

- Toda linha de telemetria carrega `tenant_id` e `project_id`; `project_id` é imutável e tem `generation`/`authz_epoch`.
- O Query Service valida contexto assinado (AD-17) e injeta o filtro de tenant; não confia em headers do cliente.
- Row policies no ClickHouse restringem o acesso por tenant; o Query Service usa um usuário de leitura com permissões mínimas.
- Table functions perigosas (`url`, `s3`, `remote`, `file`, `mysql`, `postgresql`) e tabelas de sistema são bloqueadas para o usuário da aplicação; archive/federação usam role interno privilegiado e auditado (AD-3, AD-26).
- Quotas de consulta por tenant: concorrência, memória, tempo e linhas; classes de prioridade para alertas, dashboards e ad-hoc.
- Testes de isolamento fazem parte do CI: uma consulta do projeto A nunca retorna dados do projeto B, mesmo com SQL malicioso; o mesmo teste cobre VM via vmauth e cache.
- Tokens de API têm escopo (`ingest`, `query`, `admin`), hash SHA-256/HMAC e expiração.
- OIDC mapeia grupos para papéis; RBAC é aplicado no control plane e no Query Service.

---

## 6. Engenharia de performance

### 6.1 Orçamento de ingestão

Premissas para o cluster de referência T1:
- 1M eventos/s mistos: 60% métricas (~40 B), 30% logs (~200 B), 10% traces (~500 B); média ~134 B comprimidos por evento.
- Retenção: 7-15 dias para logs/traces; 90 dias para métricas.
- Fator de replicação: 2 no ClickHouse; 2 no NATS.

Cálculo:
- 1M eventos/s × 134 bytes = 134 MB/s comprimidos.
- 134 MB/s × 86.400 s = ~11,6 TB/dia.
- 15 dias = ~174 TB comprimidos; com réplica 2 = ~348 TB de disco.
- Um nó com 16-32 TB NVMe; cluster de 6-12 nós para 7 dias e 12-24 nós para 15 dias. Com discos de 40-60 TB, 6-12 nós cobrem 15 dias.
- Se o evento médio for 50 bytes, o cluster cai pela metade; se for 1 KB, dobra.

**Isso mostra por que “1M/s” e “30 dias” são dimensões separadas.** O PRD compromete 1M/s no MVP com retenção configurável; o sizing de 30 dias é uma decisão de custo do cliente. O benchmark deve reportar throughput **e** TB/dia, não só eventos/s.

Metas por componente:
- Gateway: 15-20k eventos/s por vCPU como alvo conservador, com p99 < 50ms de processamento; 4-6 gateways de 16 vCPU para 1M/s, dependendo do benchmark Go vs Rotel vs Collector. A meta de 50k/vCPU só é aceita se reproduzida.
- NATS: ≥ 300k mensagens/s por nó com batches de 1k eventos; 4 nós para 1M/s com headroom.
- ClickHouse: ≥ 300k linhas/s por nó (referência LY: 292k/s por nó); 4-6 nós para throughput e 6-12 para retenção de 7-15 dias.
- VictoriaMetrics: ≥ 1M samples/s por nó; 3 nós para o mix.
- Lag de ingestão p95 < 10s; p99 < 30s.

### 6.2 Orçamento de consulta

| Cenário | Volume | Meta p95 |
|---|---|---|
| Dashboard 24h (dataset MVP) | 1B linhas | < 1s |
| Dashboard 7d (dataset MVP) | 10B linhas | < 2s |
| Janela quente 15 min a 1M/s | ~900M eventos | < 2s |
| Dashboard 24h a 1M/s (GA, com rollups) | 86B linhas | < 3s |
| Dashboard 7d a 1M/s (GA, com rollups) | 600B linhas | < 5s |
| Busca de logs 24h | 1B linhas | < 3s |
| Busca de traces 24h | 100M spans | < 2s |
| Catálogo de métricas | 1M séries | < 2s |
| Trace waterfall | 1k spans | < 500ms |

*Os datasets de 1B e 10B são os do MVP; 86B/24h e 600B/7d representam 1M/s e só são exigíveis no GA com rollups. O p99 deve ficar abaixo de 2x o p95 em cada tier.*

**Técnicas:** partições por dia, `ORDER BY` alinhado aos filtros, skip indexes, projeções, materialized views para span metrics e log patterns, cache de resultado, consultas assíncronas para janelas longas, limites de linhas e timeout. O Query Service mede e expõe `query_duration_ms`, `rows_read`, `bytes_read` e `cache_hit`.

### 6.3 Orçamento de frontend

| Métrica | Meta |
|---|---|
| JS inicial | < 180 KB gzip |
| Rota de dashboard | < 250 KB gzip |
| LCP | < 1,5s |
| INP | < 200ms |
| Gráfico 100k pontos | < 100ms |
| Tabela 10k linhas | 60fps |
| Tempo de resposta percebido | < 100ms para interação local; skeleton para consulta remota |

*Condições de medição: desktop, Chrome estável, rede 50 Mbps, sem cache, dados já carregados para o teste de gráfico; LCP/INP são lab metrics e devem ser complementados por field data quando houver design partners.*

**Técnicas:** code splitting por rota, uPlot, virtualização, Web Workers para transformação de dados, cancelamento de consulta, cache no cliente, prefetch de dashboards, SSE para live tail, compressão Brotli.

### 6.4 Plano de benchmark

**Harness:** `bench/` no repositório, com:
- `telemetrygen` e um gerador Go próprio para métricas/logs/traces;
- cenários de payload: 40 B (métrica), 200 B (log), 500 B (trace), 1 KB (stress);
- cenários de cardinalidade: 100k, 1M, 10M séries;
- queries de dashboard reproduzíveis (20 consultas representativas, incluindo 1B, 10B e 86B linhas);
- testes de caos: kill ClickHouse, NATS, gateway, nó;
- relatório Markdown + JSON com hardware, versões, configuração e resultados.

**Critérios de go/no-go da Fase 0:**
- 1M eventos/s por 1 hora no workload de referência (60/30/10), com lag p95 < 10s e zero perda.
- p95 de consulta < 1s em 1B linhas/24h e < 3s em 10B/7d; plano de validação para 86B/24h com rollups.
- Compressão ≥ 5x em logs e traces.
- Custo de infraestrutura por TB/dia dentro do alvo.
- Frontend com 100k pontos < 100ms e bundle inicial < 180 KB gzip.

**Publicação:** todo release publica `BENCHMARKS.md` com os números e o commit. Benchmarks de vendor não entram no material de marketing sem reprodução própria.

---

## 7. Fase 0: spikes técnicos e decisões

| Spike | Pergunta | Critério de sucesso | Fallback |
|---|---|---|---|
| S0.1 ClickHouse schema | O schema aguenta 1M/s e consultas de dashboard? | 1M/s, p95 < 1s, compressão ≥ 5x | Ajustar `ORDER BY`, projeções, materialized views |
| S0.2 Gateway | Go atinge 15-20k/s por vCPU? | 1M/s com 4-6 gateways de 16 vCPU e headroom | Migrar hot path para Rust/Rotel |
| S0.3 Fila | NATS JetStream sustenta 1M/s com durabilidade? | 1M/s, perda zero em falha de 15 min | WAL local + Kafka opcional |
| S0.4 Métricas | VictoriaMetrics cobre PromQL e cardinalidade? | 1M samples/s, consultas compatíveis | Ajustar limites; considerar ClickHouse para métricas |
| S0.5 Frontend | SvelteKit + uPlot entregam o orçamento? | 100k pontos < 100ms; bundle < 180 KB | React + uPlot; ECharts lazy |
| S0.6 Deploy | Compose/Helm sobem em < 15 min? | Quickstart funcional | Simplificar dependências no single-node |
| S0.7 Auth | OIDC + RBAC cobrem o MVP? | Login, grupos, tokens, isolamento | Keycloak/Zitadel como IdP de referência |
| S0.8 Tenancy/retenção | Row policies e TTL por projeto funcionam? | Isolamento testado; retenção por bucket viável | Tabelas separadas por bucket de retenção |

**Decisões que precisam de ADR antes da Fase 1:** armazenamento de métricas, fila, gateway, frontend, licença e estratégia de multi-tenancy.

---

## 8. Desdobramento técnico por fase

### 8.1 Fase 0 — Fundação (4 semanas)

**Epics:**
- **E0.0 Gate G0:** confirmar unidade de escala, mix e topologia com o fundador.
- **E0.1 Repositório e governança:** mono-repo, licença, DCO, CI, lint, SAST, SBOM, release.
- **E0.2 Benchmark harness:** gerador de carga, cenários, relatório.
- **E0.3 Storage spike:** schemas ClickHouse, VM, retenção, compressão, row policies.
- **E0.4 Ingest spike:** gateway Go, NATS, OTLP, Prometheus, sizing de WAL.
- **E0.5 Frontend spike:** SvelteKit, uPlot, 100k pontos, bundle.
- **E0.6 Deploy skeleton:** Compose, Helm alpha, single-node.
- **E0.7 Design partners:** 3 compromissos, critérios de feedback.
- **E0.8 Production readiness baseline:** backup/restore de metadados, runbook mínimo, upgrade/rollback.

**Saída:** ADRs, benchmark reproduzível, esqueleto deployável, design partners, gate de escala resolvido.

### 8.2 Fase 1 — MVP (16 semanas)

**Epics:**
- **E1.1 Gateway OTLP:** gRPC/HTTP, auth, limites, batching, compressão.
- **E1.2 Prometheus:** scrape com config estática, remote write, metadados.
- **E1.3 Pipeline:** filtros, redaction, enrich, sample, preview, quotas.
- **E1.4 ClickHouse:** schemas de logs/traces, TTL por projeto, ingestão assíncrona, monitoramento de parts, row policies.
- **E1.5 VictoriaMetrics:** retenção global, cardinalidade, catálogo, rollups 1m/5m.
- **E1.6 Query metrics:** MetricsQL, cache, cancelamento, limites.
- **E1.7 Query logs:** busca, facetas, SQL, live tail, parsing JSON/logfmt/regex.
- **E1.8 Query traces:** busca, waterfall, service map básico.
- **E1.9 Correlação mínima:** trace→log, log→trace, exemplar→trace.
- **E1.10 Explore UI:** barra unificada, histórico, autocomplete, deep links.
- **E1.11 Dashboards:** grid, 7 painéis, variáveis, templates, pastas/tags/busca, import/export.
- **E1.12 Alerting:** regras, Slack/webhook/email, estado, dedup, silences, acknowledge.
- **E1.13 Identity:** orgs, projetos, OIDC, RBAC, API tokens, audit mínimo.
- **E1.14 Compatibilidade OTel:** SDKs e Coletor de referência; agente próprio na Fase 2.
- **E1.15 Deploy/docs:** Compose, Helm, quickstart, instrumentação.
- **E1.16 Hardening:** caos, carga, segurança, acessibilidade, performance, production readiness gate.

**Saída:** MVP em piloto, 3 design partners, UJ-1 ponta a ponta, critérios da §10.4 do PRD.

### 8.3 Fase 2 — APM, correlação e SLOs (12 semanas)

**Epics:**
- **E2.1 Service map completo:** dependências, latência, erros, owner.
- **E2.2 Span metrics/RED:** calculados antes do sampling, endpoints, comparação.
- **E2.3 Error tracking:** fingerprint, stack trace, issues, versão.
- **E2.4 Tail sampling:** roteador por trace_id, políticas, erros/lentos, por serviço.
- **E2.5 Correlação avançada:** catálogo de entidades, fallback por serviço/janela, links.
- **E2.6 Prometheus completo:** service discovery, relabel, auth de targets.
- **E2.7 Agente beta:** logs de container, auto-discovery, recursos Kubernetes.
- **E2.8 Migração:** dashboards Grafana e regras Prometheus com relatório de fidelidade.
- **E2.9 SLOs:** SLI, target, budget, burn-rate, runbooks.
- **E2.10 Notificações:** PagerDuty, Opsgenie, Teams, agrupamento, repeat interval, escalonamento.
- **E2.11 Change tracking:** deploys, feature flags, GitHub/GitLab.
- **E2.12 GitOps/CLI:** dashboards e alertas como código, `observaki apply`.
- **E2.13 Custo:** estimador, uso por projeto, cenários de retenção.
- **E2.14 Cold storage beta:** S3, Parquet, archive search.
- **E2.15 Performance:** cache distribuído, pré-agregação, tuning de queries.

**Saída:** 1M/s em produção assistida, MTTD/MTTR melhores, 10 design partners.

### 8.4 Fase 3 — Escala e enterprise (14 semanas)

**Epics:**
- **E3.1 HA:** gateways, NATS, ClickHouse, VM, Postgres; upgrades rolling.
- **E3.2 Multi-tenancy avançado:** quotas, isolamento, limites por projeto.
- **E3.3 SSO/SCIM:** SAML/OIDC, grupos, provisionamento.
- **E3.4 Audit log:** ações, retenção, exportação.
- **E3.5 Operator:** CRDs de Observaki, tenant, retention, alert rules.
- **E3.6 Air-gapped:** bundle offline, assinatura, upgrade.
- **E3.7 DR:** backup, restore, runbook, drill.
- **E3.8 Federation beta:** query router, clusters regionais, limites.
- **E3.9 10M/s benchmark:** sizing, custo, publicação.
- **E3.10 Terraform provider:** projetos, dashboards, alertas, retenção.
- **E3.11 Segurança:** pentest, revisão de dependências, hardening.
- **E3.12 Agente GA:** logs de container, auto-discovery, recursos Kubernetes, assinatura e upgrade.
- **E3.13 GA:** API v1, Helm estável, docs, suporte.

**Saída:** GA 1.0, 10M/s, 99,9/99,95, 3 tenants isolados.

### 8.5 Fase 4 — Expansão (16+ semanas)

**Epics candidatos, em ordem:**
1. RUM e session replay leve.
2. Synthetic monitoring.
3. eBPF auto-instrumentation.
4. Continuous profiling.
5. LLM observability.
6. Anomaly detection e assistente de IA.
7. Security signals (somente com demanda).
8. Marketplace de integrações.

Cada epic exige ≥ 5 design partners e uma métrica de sucesso definida antes do início.

---

## 9. Riscos técnicos e mitigação

| Risco | Sinal | Mitigação |
|---|---|---|
| **Merge pressure no ClickHouse** | `Too many parts`, latência de insert | Async inserts, batches grandes, partições por dia, monitorar parts, TTL |
| **Cardinalidade de métricas** | séries crescendo sem controle | Limites por tenant, vmagent, agregação, cardinality explorer |
| **Gargalo no gateway** | CPU alta, lag de ingestão | Benchmark Go vs Rust; sharding; batching; compressão |
| **NATS não sustenta 1M/s** | lag, redelivery, disco | Benchmark; WAL local; Kafka opcional |
| **Consulta lenta em alta cardinalidade** | p95 acima do alvo | Projeções, materialized views, cache, limites, rollups, índices de texto |
| **Noisy neighbor de consulta** | tenant quente degrada os demais | Quotas de concorrência/memória/tempo, classes de prioridade, isolamento de workload |
| **Tail sampling sem afinidade** | spans do mesmo trace em decisores diferentes | Roteador por `trace_id`, buffer/timeout, capacity model, teste em 1M/s |
| **Span metrics enviesadas por sampling** | RED subcontando requests | Calcular span metrics no gateway antes do sampling; validar com dados reais |
| **Evolução de schema** | upgrade quebra queries/materialized views | Versionamento, migrações testadas, política de deprecação desde a Fase 1 |
| **Produção sem prontidão** | design partner perde dados/confiança | Production readiness gate: backup/restore, runbooks, upgrade/rollback, auditoria, revisão de segurança |
| **Isolamento de tenant** | vazamento de dados | Filtro obrigatório, testes de segurança, row policies, bloqueio de table functions, revisão |
| **Frontend pesado** | bundle > orçamento, INP ruim | Code splitting, uPlot, virtualização, CI de performance |
| **Operação complexa** | tempo de instalação/upgrade alto | Helm opinativo, single-node, operator, docs, runbooks |
| **Supply chain** | dependência vulnerável | SBOM, scanning, imagens assinadas, CVE SLA |
| **Licença/modelo comercial** | adoção ou monetização frustrada | Decidir antes da Fase 1; Apache 2.0 como default; open-core avaliado |

---

## 10. Modelo de custo

### 10.1 Premissas

- Preços de Datadog: tabela pública de outubro/2026; logs a partir de **$0,10/GB** de ingestão + indexação a partir de **$1,70/M eventos** (15 dias); APM **$31-40/host/mês**; infra **$15-23/host/mês**; custom metrics **$1/100** acima da franquia. Contratos reais variam.
- Observaki: hardware alugado (Hetzner/OVH) ou cloud; 16 vCPU/64 GB/4 TB NVMe ~$300-800/mês por nó; object storage ~$0,02/GB/mês; custo de operação humana separado.
- Compressão: 5-10x para logs/traces; 3-5x para métricas.
- Os cenários A/B usam **log médio de 1 KB** (100M eventos ≈ 100 GB/dia; 1B eventos ≈ 1 TB/dia); o workload de referência para benchmark usa 200 B/log. Não misturar as duas premissas.
- Retenção: 30 dias hot para logs/traces e 90 dias para métricas **no cenário de comparação de custo**; o default do produto é 7-15 dias para logs/traces, configurável.

**Metodologia de TCO.** A comparação não é “preço de lista do Datadog vs hardware do Observaki”. O TCO inclui: (a) infraestrutura (nós, disco, object storage, rede, backup); (b) operação (horas/semana × custo de engenharia); (c) migração e treinamento; (d) suporte e desenvolvimento interno; (e) egress e reidratação. A meta do PRD é ≤ 30% do custo de infraestrutura equivalente e ≤ 50% do TCO em volumes acima de 100 GB/dia. Os cenários abaixo usam preço de lista e devem ser recalibrados com cotações reais dos design partners.

*Os cenários A e B são volumes de logs menores que o T1 (1M/s ≈ 11,6 TB/dia); eles servem para ilustrar a economia em adoção inicial, não para dimensionar o cluster de benchmark.*

### 10.2 Cenário A — 100 GB/dia de logs

| Item | Datadog (estimativa) | Observaki (estimativa) |
|---|---|---|
| Ingestão de logs | $0,10/GB × 3.000 GB/mês = **$300** | Incluído no cluster |
| Indexação | 100M eventos/dia × 30 = 3B eventos; $1,70/M = **$5.100** | Incluído |
| APM (50 hosts) | 50 × $31 = **$1.550** | Incluído |
| Infra (50 hosts) | 50 × $15 = **$750** | Incluído |
| Cluster Observaki | — | 3 nós CH + 2 VM + 2 gateways ≈ **$2.000-3.000** |
| Operação | — | 0,25-0,5 FTE |
| **Total aproximado** | **$7.700+/mês** | **$2.000-3.000 + operação** |

Economia de infraestrutura: ~60-75%; com operação, ~40-60%. `[ASSUMPTION: A13 — números ilustrativos; validar com preços reais e volume do design partner.]`

### 10.3 Cenário B — 1 TB/dia de logs

| Item | Datadog (estimativa) | Observaki (estimativa) |
|---|---|---|
| Ingestão | $0,10 × 30.000 GB = **$3.000** | Incluído |
| Indexação | 1B eventos/dia × 30 = 30B; $1,70/M = **$51.000** | Incluído |
| APM (200 hosts) | 200 × $31 = **$6.200** | Incluído |
| Infra (200 hosts) | 200 × $15 = **$3.000** | Incluído |
| Cluster Observaki | — | 12-20 nós ≈ **$8.000-15.000** |
| Operação | — | 0,5-1 FTE |
| **Total aproximado** | **$63.000+/mês** | **$8.000-15.000 + operação** |

Economia de infraestrutura: ~75-85%; com operação, ~60-75%. `[ASSUMPTION: A13]`

**Conclusão:** o alvo de “≤30% do Datadog” é plausível em volumes altos, mas precisa incluir custo de operação. O produto deve tornar o custo visível (FR-43) para que o cliente veja o retorno.

---

## 11. Segurança e privacidade

### 11.1 Modelo de ameaças

- **Isolamento de tenant:** consulta maliciosa tentando ler outro tenant; token vazado; SQL injection.
- **Autenticação/autorização:** brute force, sessão roubada, escalada de privilégio, OIDC misconfiguration.
- **Ingestão:** payload malicioso, cardinalidade explosiva, DoS, SSRF via webhook.
- **Dados sensíveis:** PII em logs, segredos em atributos, exportação indevida.
- **Supply chain:** dependência comprometida, imagem não assinada.

### 11.2 Controles

- TLS 1.3; criptografia em repouso; secrets em vault/KMS.
- OIDC com PKCE; Argon2id para senhas locais; sessões com expiração e revogação.
- RBAC por projeto; tokens com escopo e hash; rotação.
- Query Service com parser e allowlist; filtro de tenant injetado; row policies no ClickHouse; bloqueio de table functions perigosas; quotas de concorrência/memória/tempo por tenant.
- Pipeline de redaction para PII e segredos; regras prontas + custom; corpus de teste com precision/recall.
- Rate limiting e quotas por tenant; WAF opcional no ingress.
- SBOM, scanning de dependências, imagens assinadas (cosign), CVE SLA de 7 dias para críticas.
- Auditoria mínima na Fase 1 (login, permissões, quotas, ingestão); cobertura completa na Fase 3; exportação/exclusão para GDPR.
- Testes de segurança no CI: SAST, DAST, dependency scan, testes de isolamento.

### 11.3 Compliance

- **SOC 2:** trilha de readiness na Fase 3; certificação é uma decisão de negócio.
- **HIPAA:** não no v1; exige BAA, criptografia, auditoria e controles adicionais.
- **GDPR:** exportação e exclusão na Fase 3; DPA e subprocessadores documentados.
- **FedRAMP:** fora do escopo; exigiria arquitetura e auditoria dedicadas.

---

## 12. Governança open source

- **Licença:** Apache 2.0 no núcleo + open-core (confirmado em 2026-10-31).
- **Contribuição:** DCO (sem CLA), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`.
- **Processo:** RFC para mudanças grandes; issue templates; roadmap público.
- **Releases:** cadência quinzenal; semantic versioning; LTS a cada 6 meses (a partir do GA).
- **Qualidade:** CI obrigatório, testes de contrato OTLP/PromQL/OpenAPI, benchmarks por release.
- **Comunidade:** Discord/Slack, office hours, good first issues, programa de design partners.
- **Trademark:** política de uso do nome e logo; evitar confusão com o projeto.
- **Monetização:** open-core (confirmado); o núcleo de observabilidade e a segurança básica não ficam atrás de paywall.

### 12.2 Fronteira open-core

**Aberto (Apache 2.0):** ingestão, armazenamento, consulta, dashboards, alertas, auth básico, RBAC básico, API, CLI, Helm, agente, documentação e o modo single-node.

**Candidatos a enterprise (licença separada):** SSO/SCIM, audit avançado, RBAC granular, retenção por tenant de métricas, federação multi-cluster, operator/air-gapped gerenciado, suporte/SLA e oferta gerenciada.

**Princípios:** (1) o núcleo de observabilidade é sempre open source; (2) segurança básica (TLS, RBAC, redaction, audit mínimo) não é paywall; (3) nenhum recurso aberto será “aberto hoje e fechado amanhã” sem aviso e período de transição; (4) a comunidade participa do roadmap. A lista exata é uma decisão de negócio antes da Fase 1.

### 12.1 Adoção e distribuição

- **Funil:** download → instalação → primeira ingestão em 24h → uso semanal em 4 semanas → retenção em 90 dias.
- **Distribuição:** imagens multi-arch no GHCR, Helm repo, binários assinados, `docker compose` oficial, pacotes para Kubernetes e systemd.
- **Conteúdo:** quickstart de 5 minutos, guias de instrumentação por linguagem, comparações honestas, benchmarks reproduzíveis, blog técnico.
- **Comunidade:** Discord/Slack, office hours quinzenal, good first issues, programa de design partners, roadmap público.
- **Métricas de adoção:** telemetria opt-in e agregada; sem phone-home por padrão; dashboards públicos de downloads e issues.
- **DevRel:** 0,5 FTE a partir da Fase 1; foco em OTel, Kubernetes e migração de Grafana/Datadog.

---

## 13. Arquitetura futura

### 13.1 Federação

- **Query router global:** recebe consulta, resolve tenants/regiões, paraleliza, agrega resultados.
- **Metadados replicados:** organizações, projetos, dashboards e alertas globais; telemetria fica regional.
- **Limites:** cada região impõe timeout, linhas e concorrência; o router agrega com orçamento global.
- **Casos de uso:** multinacional, data residency, multi-cloud, isolamento de blast radius.

### 13.2 RUM e session replay

- SDK browser leve (< 30 KB gzip), amostragem, privacy masking.
- Session replay em object storage, com retenção curta e acesso auditado.
- Correlação com traces e logs por `session_id`.

### 13.3 Synthetic monitoring

- Runners globais; checks HTTP, API, browser.
- Integração com alertas e SLOs; localizações e frequências configuráveis.
- Custo controlado por execução; resultados em séries temporais.

### 13.4 eBPF

- Agente Rust com `aya`; coleta de latência de rede, TCP retransmits, DNS, e dependências.
- Enriquecimento com Kubernetes; sem exigir mudanças na aplicação.
- Guardrails de CPU/memória e opt-in por namespace.

### 13.5 Profiling

- OTel Profiles ou pprof; symbolization; flame graphs.
- Correlação com traces e deploys; retenção curta.
- Amostragem adaptativa para controlar overhead.

### 13.6 LLM observability e IA

- OTel GenAI semantic conventions; tokens, custo, latência, qualidade.
- Assistente de IA para linguagem natural → consulta, resumo de incidente e correlação.
- Guardrails: nenhuma telemetria sai do ambiente sem opt-in; respostas citam a consulta e os dados usados.

---

## 14. Gates de decisão para avançar

1. **G0 — Escala:** confirmado em 2026-10-31: 1M/s no benchmark, 100k/s por piloto, 10M/s no GA, 100M+/s via federação; 1B/s literal fora do v1.
2. **G1 — Benchmark:** 1M/s no workload de referência, p95 < 1s em 1B linhas, compressão ≥ 5x.
3. **G2 — Tenancy/retenção:** boundary definido, row policies, TTL por projeto, quotas de consulta, audit mínimo.
4. **G3 — Correlação:** UJ-1 ponta a ponta no MVP (alerta → trace → log → métrica).
5. **G4 — Performance financiada:** rollups/cache/índices no MVP ou metas reduzidas e qualificadas.
6. **G5 — Production readiness:** backup/restore, runbooks, upgrade/rollback, auditoria, revisão de segurança antes de produção.
7. **G6 — Licença/GTM:** Apache 2.0 + open-core confirmados; fronteira enterprise, funil de adoção e design partners ainda pendentes.
8. **G7 — Números unificados:** uma tabela de referência para topologia, retenção, latência, custo e timeline; PRD e addendum sem divergência.

---

## 15. Referências

*Fontes consultadas em 31/10/2026. Benchmarks de vendor são ordens de grandeza; a Fase 0 reproduz os cenários.*

- Datadog — APM: https://www.datadoghq.com/product/apm
- Datadog — Log Management: https://www.datadoghq.com/product/log-management
- Datadog — Pricing: https://www.datadoghq.com/pricing
- Datadog — Watchdog: https://www.datadoghq.com/product/platform/watchdog
- Datadog — Observability Pipelines: https://www.datadoghq.com/product/observability-pipelines
- ClickHouse — ClickStack: https://clickhouse.com/docs/use-cases/observability/clickstack/overview
- ClickHouse — 50M events/s com OTel: https://clickhouse.com/blog/reliable-otel-ingestion-at-scale
- ClickHouse — 100 PB / wide events: https://clickhouse.com/blog/scaling-observability-beyond-100pb-wide-events-replacing-otel
- ClickHouse — Netflix 10M events/s: https://clickhouse.com/blog/netflix-petabyte-scale-logging
- ClickHouse — LY 7M rows/s: https://clickhouse.com/blog/line-yahoo
- ClickHouse — Rotel vs OTel Collector: https://clickhouse.com/blog/otel-to-rotel-petabyte-scaling-tracing-4x-greater-throughput
- ClickHouse — Async inserts: https://clickhouse.com/blog/asynchronous-data-inserts-in-clickhouse
- VictoriaMetrics — Documentação: https://docs.victoriametrics.com/
- VictoriaMetrics — Airbnb 100M samples/s: https://victoriametrics.com/blog/how-airbnb-built-a-high-volume-metrics-pipeline-with-opentelemetry-and-vmagent/
- VictoriaMetrics — Cardinality: https://victoriametrics.com/blog/cardinality-explorer/
- OpenTelemetry — Collector benchmarks: https://opentelemetry.io/docs/collector/benchmarks
- OpenTelemetry — Collector scaling: https://opentelemetry.io/docs/collector/scaling
- OpenTelemetry — Prometheus bridge benchmarks: https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/bridges/prometheus/BENCHMARKS.md
- uPlot — Repositório e benchmark: https://github.com/leeoniya/uPlot
- OpenObserve — Benchmark vs ClickHouse: https://openobserve.ai/blog/openobserve-vs-clickhouse-one-billion-logs-benchmark/
- OpenObserve — Arquitetura: https://openobserve.ai/docs/architecture
- SigNoz vs Uptrace vs HyperDX (2026): https://performance.qa/blog/signoz-vs-uptrace-vs-hyperdx/
- NATS JetStream — Documentação: https://docs.nats.io/nats-concepts/jetstream
- Kafka vs NATS JetStream: https://blog.vitalvas.com/post/2025/11/06/kafka-vs-nats-jetstream
- Prometheus vs VictoriaMetrics benchmark: https://valyala.medium.com/prometheus-vs-victoriametrics-benchmark-on-node-exporter-metrics-4ca29c75590f
