---
title: Observaki
status: final
created: 2026-10-31
updated: 2026-10-31
---

# PRD: Observaki

*Nome de trabalho confirmado pelo fundador.*

## 0. Document Purpose

Este PRD é a fonte de verdade para o MVP e para o roadmap inicial do Observaki. Ele foi escrito para Kaike (fundador e PM), para os primeiros contribuidores de engenharia, para o trabalho de UX e para os workflows downstream de arquitetura, epics e stories. A estrutura segue uma lógica de produto: problema, usuários, vocabulário, features com requisitos funcionais numerados, escopo de MVP, métricas de sucesso, requisitos não funcionais, restrições e um plano de entrega por fases.

Decisões técnicas, evidências de benchmark, alternativas consideradas e detalhes de arquitetura ficam no `addendum.md`; este documento decide **o que** o produto precisa ser e **por que**, não **como** cada componente será implementado. Premissas não confirmadas estão marcadas inline como `[ASSUMPTION: A<n> — ...]` e consolidadas na §12.

## 1. Vision

**Observaki é uma plataforma de observabilidade open source, self-hosted e OTel-native para times que operam sistemas distribuídos em alto volume.** Ela unifica métricas, logs, traces, dashboards, alertas e correlação em uma única experiência — com a promessa de ser radicalmente mais leve e barata de operar do que as plataformas SaaS tradicionais, sem abrir mão da profundidade necessária para diagnosticar incidentes em produção.

O produto nasce de uma tensão concreta: à medida que a telemetria cresce, o custo e a complexidade de Datadog, Splunk e ELK crescem mais rápido que o valor entregue. Ao mesmo tempo, o OpenTelemetry consolidou o padrão de instrumentação e o ClickHouse provou que é possível consultar bilhões de eventos com latência sub-segundo. Observaki combina essas peças com um agente de baixo footprint, um gateway de ingestão nativo e um frontend que parece instantâneo. O resultado é uma plataforma que um time pequeno consegue operar — e que um time grande consegue escalar horizontalmente.

O sucesso do Observaki é medido por três coisas: **(1)** um engenheiro consegue sair do zero ao primeiro dashboard em menos de cinco minutos; **(2)** um SRE consegue encontrar a causa raiz de um incidente sem trocar de ferramenta; **(3)** um cluster de 1 milhão de eventos por segundo cabe em hardware modesto e custa uma fração do equivalente proprietário. O produto é open source, self-hosted first, e não depende de nenhum serviço SaaS para funcionar.

**Por que agora.** O OpenTelemetry virou o denominador comum da instrumentação; o ClickHouse amadureceu como motor analítico para observabilidade; e a pressão por eficiência de custo tornou a conta de Datadog um problema de arquitetura, não só de orçamento. O espaço de alternativas open source existe, mas ainda força o time a operar um conjunto de peças desconectadas ou a aceitar um produto com lacunas de APM. Observaki entra como uma distribuição integrada, opinativa e obsessivamente eficiente.

## 2. Target User

### 2.1 Jobs To Be Done

*ICP primário do MVP: startups e times pequenos de engenharia (3-50 engenheiros) que já usam OpenTelemetry e querem uma stack self-hosted sem equipe dedicada de observabilidade. Enterprise é a jornada da Fase 3, não o alvo do primeiro lançamento.*

- **SRE / engenheiro de plataforma:** ingerir todos os sinais relevantes sem explosão de custo; encontrar a causa raiz de um incidente em minutos; provar para a liderança que a plataforma é confiável e econômica.
- **Engenheiro de backend:** instrumentar um serviço novo em menos de dez minutos; alternar entre trace, log e métrica sem perder o contexto; entender o impacto de um deploy.
- **Time pequeno / startup:** subir uma stack de observabilidade completa com um comando; não precisar de um especialista dedicado para mantê-la; manter os dados dentro da própria infraestrutura.
- **Líder de plataforma / enterprise:** oferecer observabilidade self-service para vários times com isolamento, RBAC, SSO e auditoria; atender requisitos de soberania de dados.
- **Contribuidor open source:** estender o produto via OpenTelemetry, API e dashboards como código; influenciar o roadmap em público.

### 2.2 Non-Users (v1)

- Analistas de negócio que querem BI genérico ou exploração de data warehouse.
- Times que precisam de SIEM, detecção de ameaças ou compliance de segurança como caso primário.
- Organizações que só aceitam uma solução totalmente gerenciada, sem operar infraestrutura.
- Usuários que precisam de session replay, RUM ou sintéticos no primeiro dia.
- Ambientes Windows-only no MVP; o foco inicial é Linux, containers e Kubernetes.

### 2.3 Key User Journeys

- **UJ-1. Marina acorda com um alerta e encontra a causa raiz antes do café.**
  - **Persona + contexto:** Marina é SRE em uma empresa de logística, responsável por 180 serviços em Kubernetes. O checkout é o fluxo mais crítico da empresa.
  - **Entry state:** já autenticada; recebe um alerta no Slack às 6h12: “p99 do checkout acima de 2s por 5 minutos”.
  - **Path:** clica no alerta, cai direto no painel do checkout; o service map mostra que o serviço `payment-authorizer` concentra a latência; abre a lista de traces lentos, seleciona um trace e vê que 80% do tempo está em uma chamada ao banco; clica em “logs deste trace” e encontra `connection pool exhausted`; abre a métrica de pool do banco no mesmo painel.
  - **Climax:** Marina identifica que um deploy aumentou o pool de conexões sem ajustar o limite do banco. Ela aciona o rollback no CI/CD, copia o bundle de evidências (trace, logs, métrica, janela) para o incidente e vê a latência normalizar no painel após o refresh.
  - **Resolution:** o incidente é resolvido em 11 minutos; o trace e os logs ficam salvos no postmortem; Marina cria uma regra de alerta para o pool do banco.
  - **Edge case:** se a amostragem tiver descartado o trace específico, o painel mostra a métrica derivada e os logs correlacionados por serviço, sem deixar Marina sem caminho.

- **UJ-2. Ravi instrumenta um serviço novo antes do primeiro deploy.**
  - **Persona + contexto:** Ravi é engenheiro de backend em uma startup de 40 pessoas. Ele nunca operou um cluster de observabilidade.
  - **Entry state:** tem o Observaki rodando em Docker Compose no notebook e um cluster de staging na nuvem; a aplicação é Go.
  - **Path:** copia o endpoint OTLP e a chave do projeto; adiciona o exporter OTel; sobe o serviço; abre o Explore e vê as primeiras métricas e traces; escolhe um template de dashboard “Go service”; publica o link no canal do time.
  - **Climax:** em menos de dez minutos o serviço aparece no service map, com latência, erros e logs correlacionados.
  - **Resolution:** Ravi documenta o setup no README; o time adota o mesmo padrão para os próximos serviços.
  - **Edge case:** se o serviço não aparecer, o Observaki mostra um diagnóstico de ingestão com o motivo (endpoint errado, chave inválida, amostragem, relógio dessincronizado) em vez de uma tela vazia.

- **UJ-3. Ana sobe o Observaki para 200 serviços e 40 times. *(Jornada de Fase 3 — enterprise.)***
  - **Persona + contexto:** Ana lidera a plataforma em uma empresa de 1.200 engenheiros; precisa substituir parte do Datadog sem perder governança.
  - **Entry state:** cluster Kubernetes com 3 nós de dados; OIDC corporativo; política de retenção de 30 dias para logs e 90 dias para métricas.
  - **Path:** instala o Helm chart; configura OIDC e mapeia grupos para papéis; cria organizações e projetos por domínio; publica o agente como DaemonSet; importa dashboards existentes via JSON; define quotas por time.
  - **Climax:** em uma semana, 40 times estão ingerindo telemetria; o custo por TB está dentro do orçamento; Ana consegue ver consumo por tenant e bloquear um time que estourou a cota.
  - **Resolution:** a plataforma vira self-service; Ana mantém um runbook de upgrade e um painel de saúde do Observaki.
  - **Edge case:** durante o rollout, um time envia cardinalidade 50x acima do normal; o guardrail limita a ingestão e mostra exatamente quais labels causaram a explosão.

- **UJ-4. Léo valida que a escala prometida cabe no orçamento. *(Jornada de benchmark, Fase 0/1.)***
  - **Persona + contexto:** Léo é engenheiro de plataforma cético em relação a promessas de performance; foi encarregado de validar o Observaki antes da adoção.
  - **Entry state:** tem o cluster de benchmark de referência com 6 nós ClickHouse, 3 nós VictoriaMetrics e 4-6 gateways; gera carga sintética com o harness oficial.
  - **Path:** roda 100k, 1M e 5M eventos/s (o cenário de 5M usa um cluster maior); mede latência de consulta em 1B, 10B e 86B de linhas; compara armazenamento comprimido com a estimativa do Datadog; revisa o relatório reproduzível.
  - **Climax:** o cluster sustenta 1M eventos/s com folga, p95 de consulta abaixo de 1s para 1B de linhas e custo de infraestrutura de 20-30% do equivalente proprietário.
  - **Resolution:** Léo aprova o piloto; o benchmark vira parte do processo de release.
  - **Edge case:** se um número não bater, o relatório mostra o gargalo (CPU, disco, rede, cardinalidade) e a configuração exata usada, permitindo reproduzir e corrigir.

**Cobertura por fase:** UJ-1 e UJ-2 são a fatia vertical do MVP (Fase 1); UJ-3 é a jornada enterprise, plenamente realizável na Fase 3; UJ-4 é a jornada de validação técnica da Fase 0/1.

## 3. Glossary

*Termos usados no restante do documento. FRs, UJs e SMs usam estes termos exatamente; sinônimos são uma violação de disciplina.*

- **Evento de telemetria** — Unidade individual ingerida: um sample de métrica, uma linha de log ou um span. A escala do produto é medida em eventos por segundo.
- **Sinal** — Categoria de telemetria: métrica, log ou trace.
- **Métrica** — Medição numérica ao longo do tempo, identificada por nome e labels. Pode ser counter, gauge, histograma ou exponential histogram.
- **Série temporal** — Conjunto de samples de uma métrica com o mesmo conjunto de labels.
- **Log** — Registro textual estruturado ou semiestruturado, com timestamp, severidade, corpo e atributos.
- **Span** — Unidade de trabalho dentro de um trace, com operação, duração, status e atributos.
- **Trace** — Árvore de spans que representa uma requisição distribuída de ponta a ponta.
- **Serviço** — Unidade lógica de software que produz telemetria; derivada de `service.name` e resource attributes.
- **Recurso (Resource)** — Entidade que produziu a telemetria: host, pod, container, função, processo.
- **Atributo** — Par chave-valor anexado a um evento, span, log ou métrica.
- **Cardinalidade** — Número de séries temporais únicas; o principal driver de custo e complexidade de métricas.
- **Tenant** — Organização isolada logicamente no Observaki; pode conter um ou mais projetos.
- **Projeto** — Subdivisão de um tenant para times, aplicações ou ambientes; unidade de quota, permissão e retenção.
- **Ingestão** — Caminho de entrada da telemetria, do SDK/agente até o armazenamento.
- **Gateway de ingestão** — Serviço stateless que recebe, valida, transforma e enfileira eventos.
- **Agente** — Binário leve instalado no host ou nó Kubernetes para coletar telemetria local e encaminhar ao gateway.
- **Coletor** — OpenTelemetry Collector ou distribuição compatível usada para coletar e exportar sinais.
- **Armazenamento hot** — Dados em disco local/NVMe, consultáveis em baixa latência.
- **Armazenamento warm** — Dados agregados ou em storage mais lento, ainda consultáveis.
- **Armazenamento cold** — Dados arquivados em object storage, consultáveis sob demanda.
- **Retenção** — Período durante o qual um sinal fica disponível antes de ser descartado ou arquivado.
- **Consulta** — Requisição de leitura a um sinal, seja via UI, API ou alerta.
- **Painel** — Componente visual de um dashboard que renderiza uma consulta.
- **Dashboard** — Coleção de painéis organizados em grid, com variáveis e período.
- **Regra de alerta** — Condição avaliada periodicamente que dispara uma notificação quando satisfeita.
- **SLO** — Service Level Objective: meta mensurável de confiabilidade para um serviço.
- **Error budget** — Orçamento de erro derivado de um SLO; quanto de falha é tolerável em uma janela.
- **Amostragem** — Descarte controlado de eventos para reduzir volume, preservando os mais relevantes.
- **PromQL** — Linguagem de consulta do Prometheus.
- **MetricsQL** — Extensão compatível com PromQL usada pelo VictoriaMetrics.
- **OTLP** — OpenTelemetry Protocol, transporte padrão de metrics, logs e traces.
- **Plano de controle** — Componentes que gerenciam usuários, permissões, dashboards, alertas e configuração.
- **Plano de dados** — Componentes que recebem, armazenam e consultam telemetria.

## 4. Features

*Cada subseção é uma feature coerente. FRs são numerados globalmente e marcados com a fase em que entram no produto. A §10 detalha o plano por fase; a §6 resume o MVP.*

### 4.1 Ingestão unificada e pipelines

**Description:** A porta de entrada aceita OTLP para métricas, logs e traces, além de scrape Prometheus e remote write. O agente Observaki é opcional: qualquer SDK ou Coletor OpenTelemetry funciona. O pipeline é configurável por tenant e projeto, com filtros, redaction, enriquecimento, amostragem e roteamento. A promessa é que o usuário não precise escolher entre compatibilidade e performance: o caminho padrão é OTLP; o caminho de alto volume usa o gateway nativo.

**Functional Requirements:**

#### FR-1: Ingestão OTLP unificada (Fase 1)

O sistema aceita métricas, logs e traces via OTLP gRPC (porta 4317) e OTLP HTTP (porta 4318), com protobuf e JSON, compressão gzip e zstd opcional. Preserva resource attributes, scope attributes e atributos customizados. Realiza UJ-2.

**Consequences (testable):**
- A aplicação de demonstração oficial do OpenTelemetry envia os três sinais e todos aparecem no Explore em menos de 30 segundos.
- Rejeições parciais usam a semântica de `partial_success` do OTLP (HTTP 200) com contagem de eventos rejeitados e motivo; erros de protocolo retornam 4xx/5xx.
- O sistema aceita batches de até 10 MB e 10.000 eventos por requisição, com limites configuráveis por tenant.
- Se o cliente enviar zstd e o gateway não suportar, o erro é explícito e o cliente pode fazer fallback para gzip; gzip é o default interoperável.

#### FR-2: Compatibilidade com Prometheus (Fase 1 básico; Fase 2 completo)

O sistema executa scrape de targets Prometheus com configuração estática, aceita remote write e expõe metadados de métricas para consulta. Service discovery completo, relabeling avançado e autenticação de targets entram na Fase 2. Realiza UJ-3.

**Consequences (testable):**
- Um `prometheus.yml` com 100 targets estáticos é importado sem edição manual; configs com service discovery e relabel avançado são aceitas na Fase 2.
- Um Prometheus externo consegue enviar remote write para o Observaki com autenticação por token.
- Métricas scrapeadas aparecem no catálogo com labels de `job`, `instance` e `namespace`.
- A importação reporta quais diretivas foram suportadas, ignoradas ou convertidas.

#### FR-3: Pipeline de transformação (Fase 1)

O usuário pode filtrar, descartar, redigir, enriquecer, amostrar e rotear eventos por sinal, tenant e projeto. Regras são configuradas pela UI ou API, com preview de impacto antes de salvar. Realiza UJ-3.

**Consequences (testable):**
- Uma regra de redaction remove um campo `credit_card` antes de o evento chegar ao armazenamento.
- O preview mostra quantos eventos e bytes seriam afetados em uma janela de 15 minutos.
- Uma regra de drop reduz o volume em pelo menos 90% sem afetar os eventos que casam com a exceção.

#### FR-4: Durabilidade e backpressure (Fase 1)

O caminho de ingestão usa fila durável em disco (WAL) e entrega pelo menos uma vez. O gateway só confirma um evento depois do ack da fila (ou de um buffer local durável, no modo single-node). Quando o armazenamento está indisponível, o gateway aplica backpressure, devolve HTTP 429 com `Retry-After` e não perde eventos confirmados dentro da janela de buffer configurada. Duplicatas são deduplicadas por `event_id` quando o produtor o fornece.

**Consequences (testable):**
- Um teste de caos derruba o ClickHouse por 15 minutos; após o retorno, todos os eventos confirmados são armazenados; duplicatas são deduplicadas por `event_id` e contabilizadas em métrica.
- Um tenant que envia 10x acima da cota recebe 429 e não degrada a latência dos demais tenants.
- O painel de saúde mostra lag de ingestão, tamanho da fila e eventos descartados por motivo.
- O RPO é 0 para eventos confirmados dentro da janela de buffer e da política de quórum; eventos não confirmados podem ser perdidos. O RTO documentado é < 30 minutos.

#### FR-5: Quotas e isolamento de ingestão (Fase 1)

O sistema aplica limites por tenant e projeto: eventos por segundo, bytes por segundo, tamanho máximo de evento, número máximo de atributos e cardinalidade. Limites podem ser ajustados sem reiniciar o serviço.

**Consequences (testable):**
- Um tenant com cota de 100k eventos/s não consegue consumir mais de 100k eventos/s.
- O estouro de cota gera um evento de auditoria e uma notificação configurável.
- A latência p95 dos outros tenants não varia mais de 10% durante o estouro.

#### FR-6: Ingestão de eventos de mudança (Fase 2)

O sistema aceita eventos de deploy, feature flag, incidente e configuração via API ou webhook, e os exibe como marcadores sobre métricas, logs e traces. Realiza UJ-1.

**Consequences (testable):**
- Um deploy marcado via API aparece como linha vertical no painel em menos de 10 segundos.
- O usuário consegue filtrar traces e logs pelo intervalo de um deploy específico.
- O evento preserva autor, timestamp, serviço e commit SHA quando fornecidos.

### 4.2 Métricas

**Description:** Métricas são cidadãs de primeira classe, com compatibilidade PromQL/MetricsQL, suporte a métricas OTel nativas, exemplars e guardrails de cardinalidade. O objetivo é cobrir tanto o mundo Prometheus quanto aplicações instrumentadas com OTel, sem obrigar o usuário a trocar de linguagem de consulta.

**Functional Requirements:**

#### FR-7: Consulta MetricsQL/PromQL (Fase 1)

O usuário consulta métricas instantâneas e em range usando PromQL/MetricsQL, com funções de agregação, rate, histogram_quantile, subqueries e operadores binários. Realiza UJ-1 e UJ-4.

**Consequences (testable):**
- A suíte de conformidade PromQL selecionada passa para as funções documentadas.
- Uma consulta de 24 horas sobre 1 bilhão de samples retorna em p95 abaixo de 1 segundo.
- O Editor mostra erro de sintaxe com posição e sugestão de correção.

#### FR-8: Métricas OTel nativas e exemplars (Fase 1)

O sistema aceita métricas OTel com temporality delta e cumulative, histogramas e exponential histograms, e preserva exemplars que apontam para traces. O mapeamento para o backend de métricas é documentado e testado; quando um tipo não tem equivalente nativo, o sistema converte para histograma explícito e registra um aviso.

**Consequences (testable):**
- Um histograma OTel é consultado com `histogram_quantile` e retorna buckets corretos.
- Um exemplar de uma métrica abre o trace correspondente em um clique.
- A conversão de temporality não altera a soma dos valores em uma janela de teste.
- A matriz de compatibilidade OTel → MetricsQL é publicada e validada com SDKs Go, Java, Python e Node.js.

#### FR-9: Catálogo e explorer de métricas (Fase 1)

O usuário navega por nomes de métricas, labels, valores de label e cardinalidade, com busca e ordenação por volume.

**Consequences (testable):**
- O catálogo lista todas as métricas de um projeto em menos de 2 segundos.
- O usuário vê o top 20 de séries por métrica e o número de séries únicas.
- É possível copiar uma consulta pronta a partir do catálogo.

#### FR-10: Guardrails de cardinalidade (Fase 1)

O sistema limita séries por tenant, detecta explosões de cardinalidade, sugere agregação ou drop e mostra as labels responsáveis. Realiza UJ-3.

**Consequences (testable):**
- Uma métrica com 1 milhão de séries é bloqueada ou agregada conforme a política, sem derrubar o cluster.
- O alerta de cardinalidade dispara em até 5 minutos após o início da explosão.
- O relatório mostra a contribuição de cada label para o crescimento.

#### FR-11: Rollups e downsampling (Fase 1 para 1m/5m; Fase 2 para 1h)

O sistema mantém rollups de 1 minuto e 5 minutos para métricas de alta frequência no MVP, com retenção independente; rollups de 1 hora entram na Fase 2.

**Consequences (testable):**
- Uma consulta de 7 dias usa rollups e retorna em p95 abaixo de 2 segundos.
- O rollup preserva a precisão necessária para alertas de SLO.
- O usuário consegue escolher a resolução da consulta na UI.
- A criação de rollup não aumenta o volume de ingestão em mais de 10%.

### 4.3 Logs

**Description:** Logs são pesquisáveis por texto e por estrutura, com facetas, live tail e agrupamento por padrão. A experiência precisa ser tolerante a volume: o usuário vê resultados úteis antes de ver uma tabela infinita. A correlação com traces é parte do produto, não um plugin.

**Functional Requirements:**

#### FR-12: Busca full-text e estruturada (Fase 1)

O usuário busca logs por texto livre, filtros de campo, severidade, serviço, host e intervalo de tempo. A UI oferece facetas e um modo SQL para consultas avançadas. No MVP, a busca é otimizada para campos indexados (corpo tokenizado, serviço, severidade, `trace_id` e atributos materializados); buscas não seletivas são executadas com aviso de custo. Realiza UJ-1.

**Consequences (testable):**
- Uma busca seletiva em 1 bilhão de linhas retorna os primeiros resultados em p95 abaixo de 3 segundos.
- Filtros de campo usam índices e não fazem full scan quando o campo é indexado.
- Uma busca não seletiva mostra aviso de que pode levar mais tempo e permite cancelar.
- O usuário consegue salvar uma busca e reutilizá-la.

#### FR-13: Live tail (Fase 1)

O usuário acompanha logs em tempo real com filtros, pausa, retomada e destaque de termos. A conexão usa SSE ou WebSocket e aplica backpressure na UI.

**Consequences (testable):**
- 10.000 linhas por segundo são exibidas sem travar a interface; a UI descarta ou agrupa linhas antigas.
- Pausar e retomar não perde a posição do stream.
- O usuário consegue abrir um trace a partir de uma linha de log que contém `trace_id`.

#### FR-14: Parsing e pattern inspector (Fase 1 básico; Fase 2 avançado)

O sistema interpreta JSON, logfmt e regex e extrai campos. No MVP, o pattern inspector agrupa logs semelhantes e mostra os padrões mais frequentes; detecção de padrões novos/raros e criação de alerta a partir de um padrão entram na Fase 2.

**Consequences (testable):**
- 90% dos logs de um serviço são agrupados em menos de 100 padrões.
- O usuário vê os 20 padrões mais frequentes e o volume de cada um.
- Um padrão novo é destacado em até 1 minuto após a primeira ocorrência (Fase 2).
- O usuário consegue criar um alerta a partir de um padrão (Fase 2).

#### FR-15: Correlação log-trace (Fase 1 mínima; Fase 2 avançada)

O sistema extrai `trace_id` e `span_id` dos logs e oferece navegação bidirecional entre log e trace. No MVP, a correlação por IDs é obrigatória; correlação por serviço/janela e reconstrução de traces incompletos entram na Fase 2. Realiza UJ-1.

**Consequences (testable):**
- Um log com `trace_id` abre o trace correspondente em um clique.
- Um trace mostra os logs correlacionados, ordenados por timestamp.
- A correlação funciona mesmo quando o log chega antes do trace.
- Se o trace foi amostrado, a UI mostra os logs correlacionados e explica que o trace não está disponível.

#### FR-16: Archive search (Fase 3)

O sistema arquiva logs em object storage (S3-compatível) em formato colunar e permite consultas sob demanda no archive.

**Consequences (testable):**
- Uma consulta em logs de 1 ano atrás retorna em menos de 30 segundos para um intervalo de 1 hora.
- O archive é opcional e configurável por projeto.
- O custo de storage do archive é pelo menos 5x menor que o hot storage.

### 4.4 Traces e APM

**Description:** Traces são a espinha dorsal do diagnóstico. O produto oferece busca, waterfall, service map, span metrics derivadas e error tracking. A instrumentação é OTel; o valor está na correlação e na experiência, não em um SDK proprietário.

**Functional Requirements:**

#### FR-17: Ingestão e busca de traces (Fase 1)

O sistema ingere spans via OTLP e permite buscar por serviço, operação, status, duração, atributos e trace ID. Realiza UJ-1 e UJ-4.

**Consequences (testable):**
- Um cluster sustenta 1 milhão de spans por segundo no benchmark de referência.
- A busca por traces lentos retorna em p95 abaixo de 2 segundos para 24 horas de dados.
- O usuário consegue filtrar por `http.status_code`, `error` e atributos customizados.

#### FR-18: Waterfall e detalhes de span (Fase 1)

O usuário visualiza o trace como waterfall, com caminho crítico, duração por span, status, atributos, eventos e links. Realiza UJ-1.

**Consequences (testable):**
- Um trace com 1.000 spans renderiza em menos de 500ms.
- O caminho crítico é destacado e soma a duração total do trace.
- Spans com erro são visualmente distintos e filtráveis.

#### FR-19: Service map (Fase 1 básico; Fase 2 completo)

O sistema deriva um mapa de serviços e dependências a partir dos spans. No MVP, o mapa mostra serviços, arestas e latência/erro agregados; owner, catálogo de entidades e drill-down avançado entram na Fase 2. Realiza UJ-1 e UJ-3.

**Consequences (testable):**
- Um mapa com 500 serviços renderiza em menos de 1 segundo.
- O usuário clica em uma aresta e vê os traces que a compõem.
- Serviços sem telemetria recente aparecem como “sem dados”, não como erro.
- O mapa distingue latência de rede de latência interna quando os spans permitem.

#### FR-20: Span metrics e RED (Fase 2)

O sistema deriva request rate, error rate e duration (RED) a partir de todos os spans que passam pelo gateway, antes de qualquer amostragem de traces. As métricas são armazenadas separadamente do store de traces amostrados, para não subcontar requests nem distorcer latência.

**Consequences (testable):**
- As métricas RED aparecem para todo serviço que envia spans, mesmo com tail sampling ligado.
- O usuário compara a latência do serviço com a dos endpoints downstream.
- As métricas são retidas mesmo se o trace individual for amostrado.
- Os valores de RED batem com a contagem de spans recebidos no gateway, com desvio < 1%.

#### FR-21: Error tracking (Fase 2)

O sistema agrupa erros por fingerprint, mostra stack traces, frequência, serviços afetados e primeiro/último evento. Realiza UJ-1.

**Consequences (testable):**
- 10.000 erros de teste são agrupados em menos de 100 issues.
- O usuário vê a taxa de erro por versão de serviço.
- Um erro pode ser silenciado ou vinculado a um alerta.

#### FR-22: Tail sampling (Fase 2)

O sistema aplica políticas de amostragem no final do trace, preservando erros, traces lentos e traces de serviços críticos. A decisão usa um roteador com afinidade por `trace_id` e buffer limitado; a topologia suportada é documentada e testada em escala.

**Consequences (testable):**
- 100% dos traces com erro são preservados em uma carga de teste.
- 1% dos traces bem-sucedidos é preservado por padrão.
- A política pode ser configurada por serviço e por atributo.
- O roteador mantém a afinidade por trace e não perde spans por timeout em uma falha de nó.

### 4.5 Explore e dashboards

**Description:** O Explore é a superfície de investigação: uma barra de consulta, um seletor de sinal e um painel de resultados. Dashboards são a superfície de monitoramento contínuo. Ambas precisam ser rápidas, previsíveis e compartilháveis.

**Functional Requirements:**

#### FR-23: Explore unificado (Fase 1)

O usuário alterna entre métricas, logs e traces sem perder o intervalo de tempo, o serviço ou os filtros. A barra de consulta oferece autocomplete, histórico e exemplos. Realiza UJ-1 e UJ-2.

**Consequences (testable):**
- Alternar de trace para log mantém `service` e intervalo de tempo.
- O autocomplete sugere métricas, labels e operações em menos de 200ms.
- O histórico guarda as últimas 50 consultas por usuário.
- Estados de carregamento, vazio, erro e timeout têm mensagens acionáveis e não deixam a tela em branco.

#### FR-24: Dashboards e painéis (Fase 1)

O usuário cria dashboards em grid, com painéis de time series, stat, table, top list, heatmap, logs e trace. Cada painel tem unidade, thresholds, legenda e link. Dashboards são organizados em pastas, com tags, busca e favoritos.

**Consequences (testable):**
- Um dashboard com 6 painéis carrega em menos de 2 segundos em banda larga; 12 painéis carregam em menos de 3 segundos no MVP e menos de 2 segundos na Fase 2.
- O usuário consegue criar um dashboard funcional em menos de 2 minutos.
- Painéis podem ser duplicados e reorganizados por drag-and-drop.
- O usuário encontra um dashboard por pasta, tag ou busca em menos de 2 segundos.

#### FR-25: Variáveis e templates (Fase 1)

Dashboards suportam variáveis de serviço, host, cluster, namespace e valores customizados, com seleção múltipla e interpolação segura.

**Consequences (testable):**
- Trocar uma variável atualiza todos os painéis sem recarregar a página.
- Variáveis multi-select geram consultas válidas.
- Templates oficiais existem para Go, Java, Python, Node.js, Kubernetes e PostgreSQL.

#### FR-26: Import, export e links (Fase 1 interno; Fase 2 migração)

O usuário importa e exporta dashboards em JSON e compartilha links com intervalo de tempo. A importação de dashboards Grafana e de regras de alerta Prometheus entra na Fase 2, com relatório de fidelidade.

**Consequences (testable):**
- Um dashboard exportado e importado em outro projeto mantém painéis e variáveis.
- Um link compartilhado abre com o mesmo intervalo de tempo e filtros.
- A importação de um dashboard Grafana não suportado falha com mensagem clara e lista de campos ignorados (Fase 2).
- A importação de regras Prometheus lista quais expressões foram convertidas, quais precisam de ajuste e quais foram rejeitadas (Fase 2).

#### FR-27: Renderização performática (Fase 1)

A UI usa uPlot para séries temporais, tabelas virtualizadas e cancelamento de consulta. O usuário vê skeletons e nunca fica com a tela congelada.

**Consequences (testable):**
- Um gráfico com 100.000 pontos renderiza em menos de 100ms.
- Uma tabela com 10.000 linhas rola a 60fps.
- Consultas canceladas liberam o recurso no servidor em até 2 segundos.

#### FR-28: Views salvas e colaboração (Fase 2)

O usuário salva views de Explore, compartilha com o time e adiciona anotações em dashboards.

**Consequences (testable):**
- Uma view salva preserva consulta, filtros e intervalo.
- Anotações aparecem em todos os painéis do dashboard.
- O histórico de alterações de um dashboard é auditável.

#### FR-29: Dashboards públicos e embed (Fase 3)

O usuário publica um dashboard somente leitura via link assinado ou iframe, com escopo e expiração.

**Consequences (testable):**
- Um link público não permite acesso a dados de outros projetos.
- O embed funciona em página externa sem dependência de cookies de terceiros.
- O usuário revoga o link e o acesso cessa em até 1 minuto.

### 4.6 Alertas e SLOs

**Description:** Alertas precisam ser fáceis de criar e difíceis de ignorar. O produto começa com regras claras sobre métricas e logs e evolui para SLOs, burn-rate e roteamento avançado. A meta é reduzir ruído, não aumentar o número de notificações.

**Functional Requirements:**

#### FR-30: Regras de alerta multi-sinal (Fase 1 para métricas e logs; Fase 2 para traces)

O usuário cria regras com threshold, comparação relativa, ausência de dados, contagem e duração. Regras podem consultar métricas, logs e traces.

**Consequences (testable):**
- Uma regra de “p99 > 2s por 5 minutos” dispara em até 60 segundos após a condição.
- Uma regra de ausência de dados dispara quando o serviço para de enviar telemetria.
- O usuário testa a regra contra dados históricos antes de salvar.

#### FR-31: Canais de notificação (Fase 1 para Slack, webhook e email; Fase 2 para PagerDuty, Opsgenie e Teams)

O sistema entrega notificações com contexto: painel, consulta, serviço, janela e link para o trace/log relevante.

**Consequences (testable):**
- Uma notificação de Slack contém link direto para o painel e para o trace do incidente.
- O reenvio em caso de falha usa backoff exponencial.
- O usuário testa o canal com uma notificação de exemplo.

#### FR-32: Estado, deduplicação, agrupamento e silences (Fase 1 básico; Fase 2 avançado)

O sistema mantém estado de alerta (firing, resolved, acknowledged), deduplica notificações por regra, permite silences com expiração e agrupa alertas relacionados. No MVP entram estado, dedup por regra, silence e acknowledge; agrupamento por serviço, repeat interval e escalonamento entram na Fase 2.

**Consequences (testable):**
- Um alerta que continua firing não envia uma nova notificação a cada avaliação no MVP; o repeat interval configurável chega na Fase 2.
- Alertas do mesmo serviço e mesma causa são agrupados em uma única notificação na Fase 2.
- Um silence expira automaticamente e o alerta volta a notificar.
- O histórico de estado do alerta é preservado para auditoria.

#### FR-33: SLOs e error budgets (Fase 2)

O usuário define SLIs a partir de métricas, logs ou traces, com target, janela e error budget. O sistema calcula o budget consumido e projeta o esgotamento.

**Consequences (testable):**
- Um SLO de 99,9% em 30 dias mostra o budget restante em tempo real.
- O sistema projeta a data de esgotamento com base na taxa atual.
- O SLO aparece no dashboard e no catálogo de serviços.

#### FR-34: Burn-rate alerts (Fase 2)

O sistema suporta alertas multi-janela baseados em burn rate, com janelas curta e longa para reduzir falsos positivos.

**Consequences (testable):**
- Um burn rate 14,4x em 1 hora e 5 minutos dispara alerta crítico.
- Um pico curto sem persistência não dispara alerta.
- O alerta inclui o SLO, o budget restante e o serviço afetado.

#### FR-35: Runbooks e links de incidente (Fase 2)

O usuário anexa runbooks, links de incidente e playbooks a regras de alerta e SLOs.

**Consequences (testable):**
- A notificação inclui o link do runbook.
- O runbook pode ser um URL externo ou uma página interna.
- O histórico de incidentes mantém o vínculo com a regra que disparou.

### 4.7 Correlação e contexto

**Description:** O diferencial de uma plataforma integrada é não obrigar o usuário a saltar entre ferramentas. A correlação precisa ser automática quando os IDs existem e assistida quando não existem.

**Functional Requirements:**

#### FR-36: Correlação trace-log-metric (Fase 1 mínima; Fase 2 avançada)

O sistema correlaciona traces, logs e métricas por trace ID, span ID, serviço, host, pod e atributos comuns. No MVP entram trace→log, log→trace e exemplar de métrica→trace; correlação por serviço/janela, catálogo de entidades e timeline entram na Fase 2. Realiza UJ-1.

**Consequences (testable):**
- Do trace, o usuário vê logs do mesmo `trace_id` e a métrica do serviço na janela.
- Do log, o usuário abre o trace correspondente.
- Do exemplar de métrica, o usuário abre o trace correspondente.
- A correlação por serviço/janela é oferecida como fallback quando o `trace_id` não existe (Fase 2).

#### FR-37: Catálogo de entidades (Fase 2)

O sistema mantém um catálogo de serviços, hosts, pods, namespaces e owners, derivado de resource attributes e de configuração.

**Consequences (testable):**
- O catálogo lista 1.000 serviços com owner e última telemetria.
- O usuário filtra traces e logs por entidade do catálogo.
- Serviços órfãos ou sem owner são destacados.

#### FR-38: Timeline de incidente (Fase 3)

O sistema monta uma timeline unificada com alertas, deploys, mudanças de configuração, logs e traces de um incidente.

**Consequences (testable):**
- A timeline ordena eventos de múltiplas fontes por timestamp.
- O usuário navega da timeline para o trace ou log original.
- A timeline pode ser exportada para o postmortem.

#### FR-39: Change tracking (Fase 2)

O sistema registra deploys, feature flags e mudanças de configuração e os sobrepõe aos gráficos.

**Consequences (testable):**
- Um deploy aparece como marcador em todos os painéis do serviço.
- O usuário compara a latência antes e depois do deploy.
- A integração aceita GitHub Actions, GitLab CI e webhook genérico.

### 4.8 Ciclo de vida e custo

**Description:** O produto precisa tornar o custo visível e controlável. O usuário deve entender quanto está ingerindo, quanto está armazenando e quanto pode economizar sem perder sinal.

**Functional Requirements:**

#### FR-40: Retenção por sinal e tenant (Fase 1 básico; Fase 2/3 avançado)

O usuário configura retenção para métricas, logs e traces por tenant e projeto, com padrões sensatos e TTL aplicado no armazenamento. No MVP, logs e traces têm retenção por projeto; métricas usam retenção global da instância. Retenção de métricas por projeto/tenant entra na Fase 2 (via tenants/instâncias separadas ou recurso validado no backend) e a política unificada na Fase 3.

**Consequences (testable):**
- Logs com retenção de 7 dias desaparecem do hot storage no oitavo dia.
- Métricas com retenção de 90 dias continuam consultáveis após 30 dias.
- Alterar a retenção não exige reiniciar o serviço.
- A UI mostra claramente quando uma retenção é global e quando é por projeto.

#### FR-41: Tiering hot/warm/cold (Fase 3)

O sistema move dados antigos para warm e cold storage, mantendo consultas sob demanda.

**Consequences (testable):**
- Dados de 90 dias são consultáveis no cold storage em menos de 30 segundos para uma janela de 1 hora.
- O custo por GB no cold é pelo menos 5x menor que no hot.
- O usuário vê a distribuição de storage por tier.

#### FR-42: Amostragem e agregação (Fase 2)

O sistema oferece amostragem de traces, amostragem de logs e rollups de métricas, com políticas por serviço e por ambiente.

**Consequences (testable):**
- Uma política de amostragem reduz o volume em 80% e preserva 100% dos erros.
- O usuário vê a taxa de amostragem aplicada em cada serviço.
- A agregação não altera a ordem de magnitude dos alertas.

#### FR-43: Estimador de custo e uso (Fase 2)

O sistema mostra volume ingerido, armazenado, séries ativas e custo estimado por projeto, com comparativo de cenários de retenção e amostragem.

**Consequences (testable):**
- O usuário vê o custo estimado do mês corrente por projeto.
- O simulador mostra a economia de reduzir a retenção de 30 para 15 dias.
- O custo é calculado a partir de preços configuráveis, não de uma tabela fixa.

#### FR-44: Exportação e exclusão de dados (Fase 3)

O usuário exporta ou exclui dados de um projeto para atender a requisitos de privacidade e portabilidade.

**Consequences (testable):**
- A exportação gera um arquivo com os sinais solicitados e metadados.
- A exclusão remove os dados do hot e do cold storage em até 24 horas.
- A operação é registrada em auditoria.

### 4.9 Multi-tenancy, autenticação e governança

**Description:** Mesmo em uma instalação self-hosted, o produto precisa separar times e projetos. O MVP entrega isolamento lógico, autenticação local e OIDC, RBAC básico e audit log mínimo; recursos corporativos entram na Fase 3.

**Functional Requirements:**

#### FR-45: Organizações e projetos (Fase 1)

O sistema suporta tenants (organizações) e projetos, com isolamento de dados, quotas e permissões.

**Consequences (testable):**
- Uma consulta de um projeto não retorna dados de outro.
- Um usuário sem permissão recebe 403, não uma lista vazia ambígua.
- O admin consegue mover um projeto entre organizações com auditoria.

#### FR-46: Autenticação local e OIDC (Fase 1)

O sistema oferece login local com Argon2id, sessões seguras e login via OIDC/OAuth2 com mapeamento de grupos.

**Consequences (testable):**
- O login local funciona sem dependências externas.
- O login OIDC cria ou atualiza o usuário no primeiro acesso.
- Sessões expiram e podem ser revogadas pelo admin.

#### FR-47: RBAC e API tokens (Fase 1)

O sistema tem papéis admin, editor e viewer, com permissões por projeto e tokens de API com escopo e expiração.

**Consequences (testable):**
- Um viewer não consegue criar dashboard nem alterar alerta.
- Um token com escopo `query` não consegue excluir dados.
- O token é exibido uma única vez e armazenado com hash.

#### FR-48: SSO e SCIM (Fase 3)

O sistema suporta SAML/OIDC corporativo e provisionamento SCIM de usuários e grupos.

**Consequences (testable):**
- Um usuário desativado no IdP perde acesso em até 5 minutos.
- Grupos do IdP mapeiam para papéis e projetos.
- O provisionamento SCIM cria, atualiza e desativa usuários.

#### FR-49: Audit log (Fase 1 básico; Fase 3 completo)

O sistema registra ações administrativas e de acesso a dados sensíveis com autor, timestamp, recurso e resultado. No MVP entram login, alteração de permissão, quota e configuração de ingestão; cobertura completa, retenção e exportação entram na Fase 3.

**Consequences (testable):**
- Login, alteração de permissão, alteração de quota e configuração de ingestão são auditados no MVP.
- O log é imutável e exportável.
- O usuário filtra auditoria por período, ator e recurso.

#### FR-50: Redaction de PII e segredos (Fase 1)

O pipeline de ingestão identifica e remove ou mascara PII e segredos antes do armazenamento, com regras prontas e customizáveis.

**Consequences (testable):**
- Um padrão de cartão de crédito é mascarado em logs de teste.
- O usuário vê quantos eventos foram afetados por regra.
- A redaction não quebra a correlação por trace ID.

### 4.10 Agente e implantação

**Description:** A experiência de instalação é parte do produto. O modo dev precisa caber em um notebook; o modo produção precisa caber em um Helm chart e escalar horizontalmente. O agente é opcional, leve e compatível com OTel.

**Functional Requirements:**

#### FR-51: Agente leve (Fase 2 beta; Fase 3 GA)

O agente Observaki é um binário Rust estático para Linux x86_64 e arm64. A ingestão do MVP usa SDKs e Coletor OTel; o agente entra na Fase 2 (beta) com métricas de host e receptor OTLP, e na Fase 3 (GA) com logs de container, auto-discovery e recursos Kubernetes. A meta é consumir menos de 2% de CPU e menos de 100 MB de RAM a 5.000 eventos/s por host, com benchmark publicado em 1k, 5k e 10k eventos/s. Realiza UJ-3.

**Consequences (testable):**
- O agente roda em um nó Kubernetes sem privilégios excessivos.
- O consumo fica dentro do orçamento em um teste de 24 horas.
- O agente sobrevive a reinícios e não perde a posição dos logs (Fase 3).
- O agente é opcional: a mesma telemetria pode ser enviada por um Coletor OTel.

#### FR-52: Compatibilidade com OpenTelemetry Collector (Fase 1)

Qualquer Coletor OTel padrão pode enviar para o Observaki sem modificação de código ou exporter proprietário.

**Consequences (testable):**
- A distribuição contrib do Collector envia os três sinais via OTLP.
- O Observaki documenta um exemplo de configuração para o Collector.
- O usuário não precisa instalar o agente para começar.

#### FR-53: Helm, Docker Compose e modo single-node (Fase 1)

O produto oferece Docker Compose para desenvolvimento, Helm chart para Kubernetes e um modo single-node para avaliação.

**Consequences (testable):**
- `docker compose up` sobe a stack completa e o primeiro dashboard em menos de 10 minutos.
- O Helm chart instala em um cluster de 3 nós com valores documentados.
- O modo single-node usa SQLite ou Postgres local e não exige object storage.

#### FR-54: Kubernetes Operator (Fase 3)

O produto oferece um operator com CRDs para Observaki, tenants, retention policies e alert rules.

**Consequences (testable):**
- O operator instala e atualiza a stack sem downtime.
- Uma CRD de retenção é reconciliada em até 1 minuto.
- O status da CRD mostra a saúde dos componentes.

#### FR-55: Instalação air-gapped (Fase 3)

O produto oferece um bundle offline de imagens, charts e binários, com verificação de assinatura.

**Consequences (testable):**
- A instalação funciona sem acesso à internet.
- As imagens são verificadas por digest e assinatura.
- O usuário consegue atualizar o bundle offline.

#### FR-56: Auto-discovery e eBPF (Fase 4)

O agente descobre serviços, pods, processos e dependências automaticamente, com opção de coleta via eBPF para traces e métricas de rede.

**Consequences (testable):**
- Um serviço novo em Kubernetes aparece no catálogo sem configuração manual.
- O modo eBPF coleta latência de rede sem alterar a aplicação.
- O usuário pode desligar a coleta eBPF por namespace.

### 4.11 API, CLI e infraestrutura como código

**Description:** Tudo o que a UI faz precisa ser possível via API. O produto é developer-first e precisa se encaixar em GitOps, CI/CD e automação.

**Functional Requirements:**

#### FR-57: API REST e gRPC com OpenAPI (Fase 1)

O sistema expõe API versionada para ingestão, consulta, dashboards, alertas, projetos e usuários, com OpenAPI publicado e autenticação por token.

**Consequences (testable):**
- A API cobre todas as operações de CRUD do MVP.
- O OpenAPI é gerado e validado no CI.
- A API retorna erros estruturados com código, mensagem e detalhe.

#### FR-58: CLI (Fase 2)

O sistema oferece uma CLI para consultar sinais, aplicar dashboards e gerenciar alertas em pipelines.

**Consequences (testable):**
- `observaki query` retorna JSON consumível por `jq`.
- `observaki apply -f dashboard.yaml` cria ou atualiza um dashboard.
- A CLI usa as mesmas credenciais e permissões da API.

#### FR-59: Terraform provider (Fase 3)

O sistema oferece um provider Terraform para projetos, dashboards, alertas e políticas de retenção.

**Consequences (testable):**
- Um `terraform apply` cria um projeto e um dashboard.
- O state detecta drift.
- O provider é publicado no registry.

#### FR-60: Dashboards e alertas como código (Fase 2)

O usuário versiona dashboards, alertas e políticas em YAML/JSON e aplica via CLI ou GitOps.

**Consequences (testable):**
- Um dashboard versionado é aplicado sem perda de variáveis.
- O CI valida o schema antes do apply.
- O usuário vê o diff entre o estado atual e o desejado.

### 4.12 Auto-observabilidade e operação

**Description:** O Observaki precisa observar a si mesmo. Se a plataforma falhar, o usuário precisa saber antes de perder confiança.

**Functional Requirements:**

#### FR-61: Auto-observabilidade (Fase 1)

O sistema expõe métricas, logs e traces internos, health checks e um painel de saúde com ingestão, consulta, storage e alertas.

**Consequences (testable):**
- O painel mostra lag de ingestão, taxa de erro de consulta e uso de disco.
- O health check distingue “vivo” de “pronto para tráfego”.
- O usuário configura alertas sobre o próprio Observaki.

#### FR-62: HA e upgrades sem downtime (Fase 3)

O sistema suporta alta disponibilidade do plano de controle e do plano de dados, com upgrades rolling e rollback.

**Consequences (testable):**
- Um upgrade de versão não derruba consultas em andamento.
- A perda de um nó de dados não interrompe a ingestão.
- O rollback retorna à versão anterior em menos de 15 minutos.

#### FR-63: Backup, restore e DR (Fase 1 mínimo; Fase 3 completo)

O sistema oferece backup e restore de metadados e configuração, e uma estratégia de DR documentada para telemetria. No MVP, o backup mínimo de metadados é parte do production readiness gate; o DR completo e os drills entram na Fase 3.

**Consequences (testable):**
- O restore de metadados leva menos de 30 minutos.
- O RPO de metadados é de 1 hora; o RPO de telemetria é 0 dentro da retenção.
- O runbook de DR é testado em um drill trimestral (Fase 3).

#### FR-64: Planejamento de capacidade (Fase 2)

O sistema recomenda sizing com base no volume atual e projeta o custo de crescimento.

**Consequences (testable):**
- O usuário vê uma recomendação de nós para o volume dos últimos 30 dias.
- A projeção mostra o custo de dobrar a ingestão.
- A recomendação considera retenção, cardinalidade e réplicas.

## 5. Non-Goals (Explicit)

- **Não é um SIEM.** Detecção de ameaças, correlação de segurança e resposta a incidentes de segurança não são o caso primário do v1.
- **Não é um data warehouse / BI.** O produto não compete com ClickHouse, Snowflake ou BigQuery para analytics genérico.
- **Não substitui o Prometheus/Grafana de um dia para o outro.** A convivência via OTLP, PromQL e importação de dashboards é parte da estratégia.
- **Não constrói SDKs de instrumentação proprietários no v1.** A instrumentação é OpenTelemetry; o valor está na plataforma.
- **Não é uma oferta SaaS gerenciada no v1.** O foco é self-hosted; uma oferta gerenciada é uma decisão de negócio futura.
- **Não cobre todas as integrações do Datadog.** O MVP cobre OTLP, Prometheus (config estática), Kubernetes, Docker e os canais de notificação principais.
- **Não faz session replay, RUM ou sintéticos no MVP.** Esses sinais entram na Fase 4, se validados com usuários.
- **Não promete “billions of events per second” em um único cluster no v1.** A arquitetura é preparada para federação; a meta de benchmark do MVP é 1M eventos/s por cluster, com pilotos de 100k/s, e 10M/s no GA. Ver §10.1.
- **Não é uma plataforma de compliance certificada no dia um.** SOC 2, HIPAA e FedRAMP são trilhas posteriores, dependentes de demanda comercial.

## 6. MVP Scope

### 6.1 In Scope

- Ingestão OTLP para métricas, logs e traces; scrape Prometheus com configuração estática e remote write.
- Pipeline de transformação com filtros, redaction e quotas.
- Métricas com PromQL/MetricsQL, catálogo, cardinalidade, rollups de 1m/5m e retenção global.
- Logs com busca, facetas, live tail e parsing JSON/logfmt/regex; pattern inspector básico.
- Traces com busca, waterfall e service map básico.
- Correlação mínima trace→log, log→trace e exemplar de métrica→trace.
- Explore unificado e dashboards com variáveis, templates, pastas/tags, busca e import/export.
- Alertas para métricas e logs com Slack, webhook e email; estado, dedup, silences e acknowledge básicos.
- Retenção configurável por projeto para logs/traces; retenção global para métricas.
- Organizações, projetos, autenticação local, OIDC, RBAC básico e audit log mínimo.
- Compatibilidade com OTel Collector/SDKs; o agente próprio entra na Fase 2.
- Helm, Docker Compose e modo single-node.
- API REST/gRPC e OpenAPI; auto-observabilidade.
- Documentação de quickstart, instrumentação e operação.

### 6.2 Out of Scope for MVP

- **RUM, session replay e sintéticos** — alto custo de construção e armazenamento; validar demanda na Fase 4. `[NOTE FOR PM: é a lacuna mais visível contra o Datadog; considerar um piloto com design partners.]`
- **Continuous profiling e eBPF** — a instrumentação OTel cobre o caso principal; eBPF entra como acelerador na Fase 4.
- **SLOs, error budgets e burn-rate** — importantes, mas dependem de confiança nos dados e entram na Fase 2.
- **Error tracking e tail sampling** — Fase 2; o MVP já entrega traces, busca e correlação mínima.
- **Correlação avançada (catálogo de entidades, timeline, fallback por serviço/janela)** — Fase 2.
- **Prometheus service discovery completo, relabel avançado e autenticação de targets** — Fase 2; o MVP cobre config estática e remote write.
- **Agente leve próprio (beta/GA)** — Fase 2/3; o MVP usa OTel Collector ou SDKs. A compatibilidade OTel é requisito do MVP.
- **Importação de dashboards Grafana e regras Prometheus** — Fase 2; o MVP importa/exporta JSON interno.
- **SSO/SCIM, audit log completo e RBAC avançado** — Fase 3; o MVP entrega RBAC básico e audit mínimo.
- **Tiering cold storage e archive search** — Fase 3; o MVP usa hot storage com retenção configurável.
- **Kubernetes Operator, Terraform provider e air-gapped** — Fase 3.
- **Multi-região e federação** — Fase 3/4; o MVP é um cluster por região.
- **Billing, marketplace e oferta gerenciada** — fora do escopo de produto open source no v1.
- **Agente Windows** — Fase 3/4, dependente de demanda.

## 7. Success Metrics

*Cada SM referencia os FRs que valida. Counter-metrics são tão importantes quanto as métricas primárias.*

**Primary**

- **SM-1: Time to First Value (TTFV)** — Tempo entre o início da instalação e o primeiro dashboard com dados reais. Meta: p50 < 5 minutos, p90 < 15 minutos. Valida FR-23, FR-24, FR-52, FR-53.
- **SM-2: Latência de consulta** — p95 de consultas de dashboard no workload de referência. MVP: < 1 segundo em 1 bilhão de linhas / 24h, < 3 segundos em 10 bilhões / 7 dias e < 2 segundos na janela quente de 15 minutos com ingestão de 1M/s. GA: < 3 segundos em 86 bilhões / 24h (1M/s) e < 5 segundos em 600 bilhões / 7 dias, usando rollups. Valida FR-7, FR-12, FR-17, FR-27.
- **SM-3: Eficiência de ingestão** — Eventos por segundo sustentados no workload de referência: 60% métricas (~40 B), 30% logs (~200 B), 10% traces (~500 B), média ~134 B comprimidos; 1M/s ≈ 11,6 TB/dia. Cluster: 6-12 nós ClickHouse + 3 nós VictoriaMetrics + 4-6 gateways, retenção de 7-15 dias. Meta de benchmark: 1M/s no MVP; meta de produção inicial: 100k/s por design partner; 10M/s no GA com 30-60 nós. Valida FR-1, FR-3, FR-4.
- **SM-4: Custo por volume** — Custo mensal de infraestrutura para 100 GB/dia de logs, 1M séries e 30 dias de retenção. Meta: ≤ 30% do custo de infraestrutura equivalente ao Datadog na tabela pública e ≤ 50% do custo total incluindo operação. A metodologia de TCO está no addendum §10. Valida FR-10, FR-40, FR-42, FR-43.
- **SM-5: Adoção** — Funil definido: download → instalação → primeira ingestão em 24h → uso semanal em 4 semanas. Meta até o GA: 30 implantações ativas (uso semanal), 10 design partners e 15 contribuidores externos com PR mergeado. Valida a proposta open source como um todo.
- **SM-6: Confiabilidade** — SLO de referência do produto (não uma garantia operada pelo cliente): disponibilidade de consulta ≥ 99,9%; ingestão ≥ 99,95%; lag de ingestão p95 < 10 segundos. O cliente opera o SLO na própria infraestrutura. Valida FR-4, FR-61.

**Secondary**

- **SM-7: Overhead do agente** — < 2% de CPU e < 100 MB de RAM a 5.000 eventos/s por host, com benchmark em 1k, 5k e 10k eventos/s. Valida FR-51.
- **SM-8: Tempo de detecção** — MTTD < 5 minutos para incidentes cobertos por alertas; taxa de alertas acionáveis > 80% nos design partners. Valida FR-30, FR-32, FR-34.
- **SM-9: Resolução sem troca de ferramenta** — ≥ 70% dos incidentes dos design partners são diagnosticados sem sair do Observaki (survey + telemetria de navegação). Valida FR-36, FR-37, FR-38.

**Counter-metrics (do not optimize)**

- **SM-C1: Volume ingerido por tenant** — Não otimizar para “quanto mais dados, melhor”. Volume sem uso aumenta custo e latência. Counterbalanceia SM-3 e SM-5.
- **SM-C2: Latência versus frescor** — Não sacrificar correção ou frescor dos dados para bater SM-2. Medir a idade do último dado retornado. Counterbalanceia SM-2.
- **SM-C3: Número de alertas** — Não otimizar para quantidade de alertas. Medir alertas acionáveis e ruído. Counterbalanceia SM-8.
- **SM-C4: Compressão versus flexibilidade de consulta** — Não comprimir ao ponto de inviabilizar consultas ad hoc. Medir a taxa de consultas que precisam de hot storage. Counterbalanceia SM-4.

## 8. Cross-Cutting NFRs

### 8.1 Performance

- Ingestão: latência p95 do agente ao gateway < 1 segundo; lag do gateway ao dado consultável p95 < 10 segundos e p99 < 30 segundos.
- Consulta: p95 < 1 segundo para 1 bilhão de linhas / 24h; < 3 segundos para 10 bilhões / 7 dias; < 2 segundos na janela quente de 15 minutos com ingestão de 1M/s; < 3 segundos para 86 bilhões / 24h no GA com rollups; p99 < 2x o p95 em cada tier.
- Dashboard: 6 painéis em < 2 segundos no MVP; 12 painéis em < 3 segundos no MVP e < 2 segundos na Fase 2, em banda larga.
- Frontend: JavaScript inicial < 180 KB gzip na rota de Explore; LCP < 1,5 segundo em desktop, rede 50 Mbps, sem cache; INP < 200ms; rotas de dashboard podem chegar a 250 KB gzip.
- Gráficos: 100.000 pontos em < 100ms (parse + render local, dados já carregados); tabelas de 10.000 linhas a 60fps.
- Alertas: avaliação de regra em < 60 segundos; notificação entregue em < 90 segundos após a condição.
- Consultas concorrentes: 20 dashboards simultâneos por tenant sem degradar o p95 em mais de 20% no cluster de referência.

### 8.2 Escalabilidade

- Escala horizontal linear para ingestão e consulta; sem estado local obrigatório nos gateways.
- Workload de referência: 60% métricas (~40 B), 30% logs (~200 B), 10% traces (~500 B), média ~134 B comprimidos por evento; 1M/s ≈ 11,6 TB/dia.
- Gate G0: o fundador confirma unidade de escala (por cluster, agregado ou por dia), mix e topologia antes da Fase 0. Sem confirmação, 1M/s é uma hipótese de benchmark, não um compromisso de produção.
- Meta de benchmark do MVP: 1M eventos/s por cluster com 6-12 nós ClickHouse, 3 nós VictoriaMetrics e 4-6 gateways; retenção de 7-15 dias para logs/traces; produção inicial limitada a pilotos de 100k/s por design partner.
- Meta de GA: 10M eventos/s por cluster com 30-60 nós e retenção de 7-15 dias. Retenção de 30 dias em 10M/s exige um cluster maior; throughput e retenção são dimensões separadas.
- Federação multi-cluster para 100M+ eventos/s na Fase 4; “bilhões por segundo” é um programa de escala separado, não um requisito de v1. Ver §10.1.
- Suporte a 10.000 serviços, 1M séries ativas e 100 TB a 1 PB de telemetria comprimida por cluster, dependendo do tier.

### 8.3 Confiabilidade

- Disponibilidade de consulta ≥ 99,9%; ingestão ≥ 99,95%.
- Entrega pelo menos uma vez; deduplicação por `event_id`; RPO 0 para eventos confirmados dentro da janela de buffer e da política de quórum; eventos não confirmados podem ser perdidos. RTO < 30 minutos.
- Degradação graciosa: modo somente leitura, shedding de carga e fila durável quando o storage falha.
- Testes de caos trimestrais com falha de nó, storage e rede.

### 8.4 Segurança e privacidade

- TLS 1.3 em trânsito; criptografia em repouso (AES-256 ou equivalente do storage).
- Autenticação local e OIDC; RBAC; tokens com escopo e expiração.
- Isolamento de tenant no armazenamento e na consulta; row policies no ClickHouse, filtro obrigatório no Query Service, bloqueio de table functions perigosas (`url`, `s3`, `remote`, `file`) e quotas de consulta por tenant.
- Redaction de PII e segredos no pipeline; audit log mínimo no MVP (login, permissões, quotas, configuração de ingestão) e cobertura completa na Fase 3.
- SBOM, imagens assinadas, scanning de dependências e SLA de 7 dias para CVEs críticas.
- Sem phone-home por padrão; telemetria de uso anônima e opt-in.
- Exportação e exclusão de dados para GDPR na Fase 3.

### 8.5 Operabilidade

- Instalação em < 15 minutos no modo single-node; < 1 hora em Kubernetes.
- Upgrades sem downtime em HA; rollback documentado.
- Backup e restore de metadados; DR testado.
- Footprint de referência: < 8 GB de RAM por nó de gateway, < 64 GB por nó ClickHouse/VM no tier T1, e < 4 horas/semana de operação para um cluster T1.
- Custo de infraestrutura por TB/dia publicado no benchmark de cada release.
- Auto-observabilidade com painéis e alertas prontos.
- Logs estruturados, métricas Prometheus e traces OTel para o próprio produto.

### 8.6 Acessibilidade e UX

- WCAG 2.2 AA; navegação por teclado; contraste adequado; modo claro e escuro.
- Paleta segura para daltonismo nos gráficos e estados de alerta.
- Todo gráfico canvas tem tabela alternativa e resumo textual; testes axe no CI.
- Mensagens de erro acionáveis, com link para documentação e diagnóstico.
- Onboarding guiado: quickstart, checklist e templates.

### 8.7 Compatibilidade

- OpenTelemetry 1.x (OTLP gRPC/HTTP), PromQL/MetricsQL, Kubernetes 1.35+, Helm 4.3+, Node.js 26 LTS, Docker, Linux x86_64 e arm64.
- PostgreSQL 18.6+, ClickHouse 26.8 LTS, VictoriaMetrics 1.148 LTS, NATS 2.15, Go 1.27+, SvelteKit 3.1+ como dependências de produção. `[ASSUMPTION: A6 — versões exatas serão fixadas no lockfile e no Helm chart.]`
- Navegadores modernos: Chrome, Firefox, Safari e Edge nas duas últimas versões.
- Desktop-first; mobile é somente leitura fora do v1. A UI é em inglês no v1; i18n é não-objetivo.
- Tolerância a clock skew de até 2 minutos no waterfall; a UI avisa quando o skew pode distorcer o caminho crítico.
- Licença Apache 2.0 no núcleo, com modelo open-core para recursos enterprise. `[Confirmado em 2026-10-31; a fronteira exata entre OSS e enterprise será definida antes da Fase 1.]`

## 9. Constraints and Guardrails

- **Open source e self-hosted first.** O produto não pode depender de um serviço SaaS para funcionar; object storage é opcional.
- **OTel-native.** Nenhum SDK proprietário obrigatório; qualquer Coletor OTel compatível deve funcionar.
- **Baixo footprint.** O agente precisa ser viável em ambientes com recursos limitados; o backend precisa ser operável por um time pequeno.
- **Sem lock-in de dados.** Formatos abertos, exportação e APIs documentadas.
- **Soberania de dados.** O usuário controla onde a telemetria fica; sem envio para fora por padrão.
- **Time enxuto.** O plano assume 5-6 engenheiros em tempo integral (2 backend, 1 frontend, 1 infra/SRE, 1 agente/plataforma, 0,5 design/docs) e ~14 meses até o GA, incluindo buffer de 30%; a engenharia pura ocupa ~10,5 meses. `[ASSUMPTION: A4 — se o time for menor, as durações escalam; se for maior, as fases podem ser paralelizadas.]`
- **Orçamento de infraestrutura de desenvolvimento.** Benchmarks usam hardware alugado ou máquinas locais; o custo precisa ser documentado.
- **Licenciamento e governança.** DCO, Apache 2.0 no núcleo, open-core para enterprise, trademark e política de contribuição definidos na Fase 0. A fronteira exata é uma decisão bloqueante antes da Fase 1.
- **Não quebrar compatibilidade.** OTLP, PromQL e OpenAPI são contratos públicos; mudanças seguem versionamento semântico.

## 10. Plano de Entrega por Fases

### 10.1 Calibração de escala

Antes de prometer “milhões/bilhões de eventos por segundo”, é preciso separar **unidade** e **topologia**:

| Tier | Alvo | Topologia de referência | Quando |
|---|---|---|---|
| T0 — Dev | 10k eventos/s | 1 nó, Docker Compose | Fase 0 |
| T1 — Benchmark do MVP | 1M eventos/s por cluster | 6-12 ClickHouse + 3 VictoriaMetrics + 4-6 gateways; retenção 7-15 dias | Fase 1 |
| T2 — Scale | 10M eventos/s por cluster | 30-60 nós, storage separado, HA; retenção 7-15 dias | Fase 3 |
| T3 — Federation | 100M+ eventos/s agregados | Multi-cluster, query federada, multi-região | Fase 4 |
| T4 — Hyperscale | 1B+ eventos/s | Programa dedicado de hardware e sharding customizado | Fora do v1 |

**Leitura do requisito:** 1 bilhão de eventos por segundo é ~100x o alvo de GA (10M/s) e ~1.000x o alvo do MVP (1M/s); está acima da operação pública de referência de empresas como Netflix (~10,6M/s) e ClickHouse Cloud (~50M/s). `[Confirmado: A1 — o alvo comprometido é 1M/s no benchmark do MVP, 100k/s por piloto, 10M/s no GA e 100M+/s via federação; 1B/s literal em um cluster está fora do v1. Se a intenção fosse “bilhões por dia”, o alvo seria ~11,6k/s em média, com picos de 100k/s.]`

**Throughput não é o mesmo que retenção.** 1M eventos/s com 134 bytes comprimidos por evento gera ~11,6 TB/dia; 15 dias com réplica 2 exigem ~348 TB de disco e 30 dias exigem ~696 TB. O cluster de 6-12 nós cobre o throughput e uma retenção de 7-15 dias; para 30 dias, o sizing cresce. O benchmark da Fase 0 reporta os dois números, não apenas eventos/s.

### 10.2 Visão geral das fases

| Fase | Janela | Objetivo | Escopo principal | Critério de saída |
|---|---|---|---|---|
| **0. Fundação e benchmarks** | Semanas 0-4 | Provar a stack e reduzir risco técnico | Gate de escala, ADRs, repo, CI/CD, harness de benchmark, esqueleto de deploy, design partners | 1M eventos/s em benchmark; p95 de consulta < 1s; ADRs aprovadas; 3 design partners |
| **1. MVP — Observabilidade unificada** | Semanas 5-20 | Entregar os três sinais + correlação mínima + dashboards + alertas | OTLP/Prometheus básico, métricas, logs, traces, Explore, dashboards, alertas, auth, audit mínimo, rollups 1m/5m | 3 design partners em piloto; UJ-1 ponta a ponta; TTFV < 5 min; 7 dias de soak sem perda P0 |
| **2. APM, correlação e SLOs** | Semanas 21-32 | Transformar dados em diagnóstico | Service map completo, span metrics, error tracking, tail sampling, correlação avançada, SLOs, burn-rate, GitOps, agente beta | 1M eventos/s em produção assistida; MTTD/MTTR melhores; 10 design partners |
| **3. Escala e enterprise** | Semanas 33-46 | Tornar o produto confiável para grandes organizações | HA, multi-tenant avançado, SSO/SCIM, audit completo, operator, air-gapped, DR, agente GA, 10M/s, federação beta | 10M eventos/s; 30 dias de soak; 3 tenants isolados; GA 1.0 |
| **4. Expansão** | Semanas 47-62+ | Ampliar o portfólio de sinais e o ecossistema | RUM, sintéticos, eBPF, profiling, LLM observability, IA, marketplace | Adoção dos novos sinais; paridade nos nichos escolhidos |

*Durações assumem 5-6 engenheiros em tempo integral e não incluem férias, contratação ou mudanças de escopo. `[ASSUMPTION: A4]` O GA de engenharia termina na semana 46 (~10,5 meses); com buffer de 30%, o compromisso é ~semana 60 (~14 meses). A Fase 4 começa depois do GA.*

### 10.3 Fase 0 — Fundação e benchmarks (4 semanas)

**Objetivo:** reduzir o risco de que a stack escolhida não entregue a performance prometida. Nenhuma feature de usuário é construída antes de o benchmark provar o caminho.

**Gate G0 — escala confirmada em 2026-10-31:** 1M eventos/s no benchmark do MVP, 100k/s por design partner em produção inicial, 10M/s no GA e 100M+/s via federação na Fase 4. “1 bilhão por segundo” em um único cluster está fora do v1; se essa for a intenção literal, o plano é re-arquitetado antes da Fase 0.

**Entregáveis:**
- Repositório mono-repo, licença Apache 2.0, DCO, CODE_OF_CONDUCT, CONTRIBUTING e política de releases.
- ADRs: armazenamento (ClickHouse + VictoriaMetrics), gateway (Go), agente (Rust), fila/WAL (NATS JetStream), frontend (SvelteKit + uPlot), autenticação (OIDC).
- Harness de benchmark reproduzível: gerador de carga OTLP, cenários de cardinalidade, consultas de dashboard, relatório em Markdown.
- Esqueleto de deploy: Docker Compose, Helm chart alpha, modo single-node.
- 3 design partners comprometidos com feedback semanal.

**Critério de saída:**
- 1M eventos/s sustentados por 1 hora no workload de referência (60% métricas / 30% logs / 10% traces) no cluster de 6-12 nós ClickHouse, 3 VictoriaMetrics e 4-6 gateways, com retenção de 7-15 dias no tamanho de evento do benchmark.
- p95 de consulta < 1s em 1 bilhão de linhas para 24 horas; < 3s em 10 bilhões para 7 dias; plano de validação para 86 bilhões/24h com rollups.
- Compressão de logs e traces ≥ 5x.
- ADRs aprovadas e riscos técnicos documentados.

### 10.4 Fase 1 — MVP: Observabilidade unificada (16 semanas)

**Objetivo:** entregar uma plataforma utilizável por um time real, com os três sinais, correlação mínima, dashboards e alertas. O MVP não precisa de paridade com Datadog; precisa resolver o caso de uso principal melhor do que a alternativa atual do design partner.

**Workstreams e entregáveis:**
- **Ingestão:** gateway Go com OTLP gRPC/HTTP, scrape Prometheus com config estática, remote write, WAL, quotas, pipeline de transformação e redaction.
- **Armazenamento:** schemas ClickHouse para logs e traces, VictoriaMetrics para métricas, TTL por projeto para logs/traces, retenção global para métricas, rollups de 1m/5m.
- **Consulta:** MetricsQL/PromQL, busca de logs, busca de traces, cache de consulta, cancelamento e limites por tenant.
- **Correlação mínima:** trace→log, log→trace e exemplar de métrica→trace; links diretos na UI.
- **Frontend:** shell SvelteKit, Explore unificado, dashboards com 7 tipos de painel, uPlot, tabelas virtualizadas, variáveis, templates, pastas/tags/busca.
- **Alertas:** regras de métricas e logs, Slack/webhook/email, estado, dedup, silences e acknowledge.
- **Identidade:** organizações, projetos, login local, OIDC, RBAC básico, API tokens e audit log mínimo.
- **Compatibilidade:** SDKs e Coletor OTel de referência; o agente próprio entra na Fase 2.
- **Deploy:** Docker Compose, Helm, single-node, quickstart e documentação de instrumentação.
- **Qualidade:** testes de contrato OTLP, testes de carga, caos de storage, SBOM e imagens assinadas.
- **Production readiness gate:** backup/restore de metadados, runbooks, upgrade/rollback testado, auditoria mínima e revisão de segurança antes de qualquer design partner ir a produção. Pilotos podem rodar com SLA reduzido e dados não críticos.

**Critério de saída:**
- 3 design partners em **piloto** ingerindo ≥ 100k eventos/s cada (não produção crítica).
- UJ-1 executada de ponta a ponta em um design partner: alerta → painel → trace → log → métrica → bundle de evidências/handoff.
- TTFV p50 < 5 minutos; p90 < 15 minutos.
- p95 de dashboard com 6 painéis < 2s e 12 painéis < 3s; consulta de 1B linhas/24h < 1s; 10B/7d < 3s; busca de logs < 3s; busca de traces < 2s.
- 7 dias de soak sem perda de dados P0; lag de ingestão < 10s p95.
- Audit log mínimo e production readiness gate aprovados.
- Documentação publicada e um vídeo de 5 minutos de quickstart.

### 10.5 Fase 2 — APM, correlação e SLOs (12 semanas)

**Objetivo:** sair de “dados disponíveis” para “diagnóstico rápido”. É a fase que mais aproxima o produto da promessa de “Datadog-like”.

**Workstreams e entregáveis:**
- Service map completo, span metrics RED, error tracking e tail sampling.
- Correlação avançada: catálogo de entidades, fallback por serviço/janela e timeline.
- Prometheus service discovery completo, relabel avançado e autenticação de targets.
- Agente beta e GA com logs de container, auto-discovery e recursos Kubernetes.
- Migração de dashboards Grafana e regras Prometheus com relatório de fidelidade.
- SLOs, error budgets, burn-rate alerts e runbooks.
- PagerDuty, Opsgenie, Teams; agrupamento e dedup avançados.
- Change tracking (deploys, feature flags) e anotações.
- Dashboards e alertas como código; CLI; GitOps.
- Cold storage beta e estimador de custo.
- Otimização de consulta: cache distribuído, pré-agregação e tuning de dashboards grandes.

**Critério de saída:**
- 1M eventos/s em produção assistida em pelo menos um design partner; HA plena entra na Fase 3.
- MTTD < 5 minutos e MTTR reduzido em ≥ 30% nos design partners (linha de base registrada na Fase 1).
- 10 design partners ativos; ≥ 70% dos incidentes diagnosticados sem sair do Observaki.
- Retenção de 15-30 dias dentro do custo-alvo, com cold storage beta.

### 10.6 Fase 3 — Escala e enterprise (14 semanas)

**Objetivo:** tornar o Observaki confiável para organizações grandes e para o anúncio de GA 1.0.

**Workstreams e entregáveis:**
- HA do plano de controle e do plano de dados; upgrades rolling; rollback.
- Multi-tenancy avançado, quotas, retenção por tenant, SSO/SCIM, audit log completo e RBAC granular.
- Kubernetes Operator, air-gapped bundle, backup/restore e DR.
- Federation beta para múltiplos clusters; query federada.
- Benchmark de 10M eventos/s; relatório de sizing e custo.
- Terraform provider; API estável v1; política de deprecação.
- Hardening de segurança, pentest, revisão de dependências e trilha de readiness SOC 2.
- Documentação de operação, runbooks e programa de design partners.

**Critério de saída:**
- 10M eventos/s sustentados por 1 hora no cluster de referência (30-60 nós, retenção de 7-15 dias).
- 30 dias de soak com 99,9% de disponibilidade de consulta e 99,95% de ingestão.
- 3 tenants isolados com quotas e auditoria.
- GA 1.0 com API v1, Helm chart estável e política de suporte.

### 10.7 Fase 4 — Expansão (16+ semanas)

**Objetivo:** ampliar o portfólio para casos adjacentes, sem comprometer o núcleo. Cada novo sinal precisa passar por validação com design partners antes de entrar no roadmap comprometido.

**Candidatos, em ordem de prioridade:**
1. **RUM e session replay leve** — maior lacuna competitiva; alto custo de armazenamento e privacidade.
2. **Synthetic monitoring** — validação de endpoints e jornadas; bom encaixe com alertas.
3. **eBPF auto-instrumentation** — reduz o atrito de adoção e melhora o service map.
4. **Continuous profiling** — complementa traces para diagnóstico de CPU/memória.
5. **LLM observability** — traces de chains, tokens, custo e qualidade.
6. **Anomaly detection e assistente de IA** — correlação assistida e resumo de incidentes.
7. **Security signals** — somente se houver demanda clara; não transformar o produto em SIEM.
8. **Marketplace de integrações** — dashboards, exporters e plugins da comunidade.

**Critério de saída:** cada item só avança se tiver ≥ 5 design partners pedindo e uma métrica de sucesso definida antes do início.

### 10.8 Sequenciamento e dependências

- **Fase 0 → Fase 1:** o benchmark precisa provar 1M/s no workload de referência e p95 < 1s antes de começar o MVP. Se falhar, a stack é revisitada antes de escrever features.
- **Fase 1 → Fase 2:** a correlação mínima (trace-log e exemplar) já está no MVP; a Fase 2 depende de trace ID consistente, schema de logs estável e catálogo de entidades.
- **Fase 2 → Fase 3:** SLOs e multi-tenancy precisam estar estáveis antes de HA e federação; a escala sem isolamento é perigosa.
- **Fase 3 → Fase 4:** GA 1.0 é o gate. Nenhum novo sinal entra antes de a base estar confiável e documentada.

### 10.9 Riscos do plano e mitigação

| Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|
| A stack não atinge 1M/s no benchmark | Média | Alto | Fase 0 dedicada; fallback para gateway Rust; ajuste de schema e batch |
| Cardinalidade explode em produção | Alta | Alto | Guardrails desde o MVP; limites por tenant; cardinality explorer |
| Busca full-text não escala em 1B linhas | Média | Alto | Campos materializados no MVP; inverted index/ngram na Fase 2; limites documentados |
| Noisy neighbor de consulta | Média | Alto | Quotas de concorrência/memória; classes de prioridade; testes de caos |
| Escopo “Datadog-like” cresce demais | Alta | Alto | Non-goals explícitos; fatia vertical; fases com gates; design partners |
| Design partners não aparecem ou não compartilham dados | Média | Alto | Pipeline de recrutamento desde a Fase 0; gate duro; dados sintéticos como fallback |
| Produção sem prontidão | Média | Alto | Pilotos com SLA reduzido; production readiness gate antes de produção crítica |
| Operação de ClickHouse é pesada para times pequenos | Média | Médio | Helm chart opinativo; single-node; runbooks; operator na Fase 3 |
| UX não é rápida o suficiente | Média | Alto | Orçamento de performance no CI; uPlot; virtualização; testes de regressão |
| Adoção open source abaixo do esperado | Média | Alto | Design partners desde a Fase 0; docs; quickstart; comunidade; funil de ativação |
| Licença/modelo comercial inadequado | Baixa | Alto | Decidir antes da Fase 1; Apache 2.0 como default |
| Time pequeno perde ritmo | Média | Alto | Fases curtas; buffer de 30%; cortar escopo, não qualidade; contratar após o MVP |

## 11. Open Questions

**Decisões confirmadas em 2026-10-31:**

| Decisão | Escolha | Impacto |
|---|---|---|
| Escala do primeiro ano | 1M eventos/s no benchmark do MVP; 100k/s por design partner em produção inicial; 10M/s no GA; 100M+/s via federação na Fase 4. 1B/s literal em um cluster está fora do v1. | Calibra sizing, benchmark e roadmap |
| Licença | Apache 2.0 no núcleo + modelo open-core para recursos enterprise | Maximiza adoção; fronteira exata a definir |
| ICP do MVP | Startups e times pequenos (3-50 engenheiros); enterprise é a Fase 3 | Foco do MVP, UX e design partners |

*Pendências abaixo não bloqueiam a arquitetura; as bloqueantes estão na tabela ao final.*

1. **Time e prazo:** 5-6 engenheiros e ~14 meses até o GA (incluindo buffer) é realista? Se não, quais fases cortar?
2. **Frontend:** SvelteKit é a recomendação técnica, mas o time tem experiência com React? A decisão final fica no addendum/ADR.
3. **Dois armazenamentos:** ClickHouse + VictoriaMetrics é o melhor trade-off, ou um único ClickHouse reduz a operação? A decisão precisa de benchmark na Fase 0.
4. **Retenção padrão:** 7, 15 ou 30 dias para logs? Qual o default que equilibra custo e utilidade?
5. **Fronteira open-core:** quais recursos enterprise (SSO/SCIM, audit avançado, RBAC granular, federação, suporte) ficam fora do Apache 2.0? Princípio: o núcleo de observabilidade e a segurança básica nunca ficam atrás de paywall.
6. **SSO/SCIM:** é requisito de MVP para algum design partner? Se sim, antecipar para a Fase 2.
7. **Compliance:** SOC 2, HIPAA ou FedRAMP são necessários para fechar os primeiros contratos? Isso muda a Fase 3.
8. **Agente Windows:** há demanda real no curto prazo? Se sim, entra na Fase 3.
9. **Política de suporte:** haverá LTS? Qual a cadência de releases? O produto open source precisa de uma promessa clara.
10. **Design partners:** quem são os 3 primeiros? O acesso a dados reais é o maior acelerador de qualidade.

**Decisões bloqueantes (time-box):**

| Decisão | Owner | Deadline | Bloqueia |
|---|---|---|---|
| Fronteira open-core | Fundador | Antes da Fase 1 | Licenciamento, comunidade, features enterprise |
| Tenancy boundary e retenção | Fundador + Arquitetura | Semana 2 | Schema, segurança, TTL, quotas |
| Storage: ClickHouse + VM vs unificado | Arquitetura | Fim da Fase 0 | ADR, custo, operação |
| Frontend: SvelteKit vs React | Fundador + Frontend | Semana 1 | Contratação, design system |
| Design partners e compromisso mínimo | Fundador | Semana 4 | Gates de produto, dados reais |
| Retenção default por sinal | Produto | Semana 2 | Sizing, custo, UX |
| Time e prazo | Fundador | Semana 1 | Staffing, fases, buffer |

## 12. Assumptions Index

- **A1** — Confirmado em 2026-10-31: 1M eventos/s no benchmark do MVP, 100k/s por design partner em produção inicial, 10M/s no GA e 100M+/s via federação; 1B/s literal em um cluster está fora do v1.
- **A2** — Confirmado em 2026-10-31: Apache 2.0 no núcleo + modelo open-core; a fronteira exata entre OSS e enterprise será definida antes da Fase 1.
- **A3** — Self-hosted first; nenhuma oferta SaaS gerenciada no v1.
- **A4** — Time de 5-6 engenheiros em tempo integral e ~14 meses até o GA incluindo buffer de 30%; a engenharia pura ocupa ~10,5 meses. Durações escalam se o time for menor.
- **A5** — OTel-native; nenhum SDK proprietário obrigatório; o agente é opcional.
- **A6** — Stack de referência: ClickHouse 26.8 LTS para logs/traces, VictoriaMetrics 1.148 LTS para métricas, PostgreSQL 18.6 para metadados, NATS 2.15 para fila, Go 1.27 no backend, Rust no agente, SvelteKit 3.1 + Svelte 5.57 + uPlot no frontend. Versões exatas no lockfile/Helm.
- **A7** — Kubernetes e Docker Compose são os alvos de deploy do MVP; single-node existe para avaliação.
- **A8** — Documentação pública e UI em inglês no v1; documentos de produto em Português-BR. `[ASSUMPTION: A8 — confirmar se a documentação pública deve ser em inglês.]`
- **A9** — O MVP terá 3 design partners com dados reais e feedback semanal; sem eles, o gate de produto não é cumprido e o risco de construir a coisa errada aumenta.
- **A10** — Multi-tenancy lógica (organizações/projetos) existe desde o MVP, mesmo em instalações single-tenant.
- **A11** — Não haverá billing, marketplace ou oferta gerenciada no v1.
- **A12** — O produto não terá certificação SOC 2/HIPAA no GA; a trilha começa na Fase 3 se houver demanda comercial.
- **A13** — Os números de custo no addendum são ilustrativos e usam a tabela pública do Datadog em outubro/2026; contratos reais, volumes e custo de operação precisam ser validados com design partners.
- **A14** — Workload de referência: 60% métricas (~40 B), 30% logs (~200 B), 10% traces (~500 B), média ~134 B comprimidos; 1M/s ≈ 11,6 TB/dia.
- **A15** — ICP primário do MVP: startups e times pequenos (3-50 engenheiros); enterprise é a jornada da Fase 3.
