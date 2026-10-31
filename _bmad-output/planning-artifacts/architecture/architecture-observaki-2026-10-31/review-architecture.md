# Revisão adversarial — Architecture Spine Observaki

**Data:** 2026-10-31  
**Artefatos revisados:** `ARCHITECTURE-SPINE.md`, `prd.md`, `addendum.md`  
**Método:** leitura integral dos três artefatos; construção de pares de unidades um nível abaixo que satisfazem literalmente os ADs e ainda assim são incompatíveis; verificação de versões/suporte em fontes primárias em 31/10/2026; checagem de ownership de dados, envelope operacional e contratos externos.  
**Status:** revisão independente. O spine não foi editado.

---

## Veredito

O spine tem direção arquitetural defensável e escolhas de stack em grande parte atuais, mas **não está decision-ready**: AD-1/AD-6, AD-5, AD-2/AD-4 e a distribuição de configuração/tenancy se contradizem ou deixam ownership indefinido, de modo que duas unidades que obedeçam literalmente aos ADs podem produzir vazamento cross-tenant, perda de eventos já confirmados ao cliente ou métricas RED divergentes.

---

## 1. Sumário executivo

O spine acerta o essencial: separa control plane e data plane, adota OTLP como contrato, usa ClickHouse para eventos e VictoriaMetrics para métricas, evita SDK proprietário obrigatório e trata performance como orçamento. As escolhas de stack também estão, em sua maioria, corretas para outubro de 2026 — Go 1.27.3, ClickHouse 26.8 LTS, VictoriaMetrics 1.148 LTS, PostgreSQL 18.6, NATS 2.15.1, SvelteKit 3.1.1 e ConnectRPC 2.x existem e são correntes.

O problema não é a direção; é o **grau de resolução**. Os ADs foram escritos como princípios, não como contratos. Em vários pontos, dois ADs descrevem o mesmo dado com donos diferentes, ou um AD exige algo que outro AD proíbe. Os casos mais graves:

1. **AD-1 vs. AD-6 vs. fluxo de consulta:** o control plane está no caminho de leitura (API → PostgreSQL para authz → Query Service) enquanto AD-1 diz que ele não participa da consulta e AD-6 diz que o Query Service autentica. Há dois donos da autorização e uma dependência de PostgreSQL no caminho que deveria ser independente.
2. **AD-5 contra si mesmo:** o mesmo AD manda armazenar span metrics no ClickHouse e proíbe métricas no ClickHouse no MVP; FR-20 e o addendum mandam o gateway gravar RED no VictoriaMetrics. São dois donos para o mesmo número.
3. **AD-2/AD-4:** `event_id` não existe no OTLP; o gateway precisa sintetizá-lo, o retry do SDK gera outro ID, o `MergeTree` não deduplica e o writer pode dar ack no NATS antes da durabilidade do insert assíncrono no ClickHouse. A promessa de “sem duplicatas visíveis” e RPO 0 não fecha.
4. **AD-1/AD-7:** retenção, quotas, pipeline e entitlements vivem no PostgreSQL, mas o gateway não pode depender do PostgreSQL. Não há contrato de distribuição, staleness, reconciliação ou fail-closed; a retenção tem dois donos (colunas no PG e TTL no ClickHouse).
5. **AD-2/AD-3:** tenancy não tem identidade imutável nem propagação. `project move`, delete, cache, path do VictoriaMetrics e row policies não compartilham geração/epoch; o resultado pode ser vazamento cross-tenant ou dados invisíveis.

Os demais achados vão de upgrades (a atualização 26.3 LTS → 26.8 LTS tem dezenas de mudanças incompatíveis — 57 em um levantamento da comunidade), passando por backup/restore e DR ausentes apesar do production readiness gate, até contratos OTLP/PromQL/OpenAPI que não estão pinados. A stack precisa de correções pontuais de suporte: Kubernetes 1.33 está EOL desde 28/06/2026 e o PRD ainda declara 1.28+; Helm 3 está em fim de vida programado; Node 24 virou Maintenance LTS em 20/10/2026; o Collector está como “0.1xx”, que não é um pin.

**Conclusão:** o spine serve como visão, não como substrato de build. Antes da Fase 1, são necessários AD-17..AD-25 cobrindo authz/query plane, idempotência/WAL, config plane, tenancy, span metrics, contratos internos/upgrade, envelope operacional, meta-observabilidade e contratos externos.

---

## 2. Achados

### 2.1 Crítico

#### C-01 — O control plane está no caminho de consulta e a autenticação tem dois donos

**Evidência.** AD-1 (linha 39) afirma que “o control plane (PostgreSQL, auth, dashboards, alertas) não participa da ingestão nem da consulta de telemetria” e que “a única ponte é o Query Service e a API”. AD-6 (linha 69) afirma que “toda leitura de telemetria passa pelo Query Service, que aplica autenticação, tenant, limites, timeout, cache e formato de resposta”. O fluxo de consulta (linhas 286–298) mostra o oposto: `UI->>A: query`, `A->>PG: authz, project, quota`, `A->>Q: execute(query, tenant, project)`. Ou seja, a API consulta o PostgreSQL a cada query e passa tenant/project ao QS. AD-13 (linha 111) reforça que “RBAC é aplicado no control plane e no Query Service”; AD-3 (linha 51) diz que o token resolve tenant/projeto no gateway e que o QS injeta o predicado. Há três pontos de decisão de authz (gateway, API, QS) e nenhum é declarado autoritativo. Os diagramas pioram: o contexto (linhas 181–186) mostra `API --> CH` e `API --> VM`; as topologias (linhas 320–322) mostram `API2[API/Query replicas] --> CH2` e `--> VM2`, ambos proibidos por AD-6.

**Impacto.** Uma queda do PostgreSQL derruba consultas mesmo com ClickHouse e VictoriaMetrics saudáveis, contradizendo a independência do data plane e tornando o SLO de consulta 99,9% dependente do banco de metadados. Se o QS confia no header `tenant/project` vindo da API, uma API comprometida ou uma chamada interna forjada atravessa tenants. Se o QS reautentica, ele precisa do PostgreSQL/OIDC e vira dependente do control plane. As duas implementações são compatíveis com alguma frase do spine e incompatíveis entre si; a revogação de token e o `project move` podem valer em um ponto e não no outro.

**Correção.** Criar um AD de plano de consulta que declare: (a) uma única autoridade de authz; (b) o contrato de contexto (idealmente um JWT curto assinado pela API ou pelo IdP, com `tenant_id`, `project_id`, `actor_id`, `authz_epoch`, escopos e expiração); (c) comportamento em indisponibilidade do PG/IdP (cache de JWKS, token curto, fail-closed para authz e fail-open apenas para dados públicos); (d) que o QS valida o contexto e não confia em headers; (e) que a API de controle e o Query Service são processos/rotas separados, ou que a API não é necessária no caminho de leitura. Corrigir os diagramas para que nenhuma seta `API -> CH/VM` exista.

---

#### C-02 — AD-5 cria dois donos para span metrics/RED

**Evidência.** AD-5 (linha 63) diz: “ClickHouse armazena logs, traces, eventos e span metrics; VictoriaMetrics armazena métricas e séries temporais. Métricas não vão para ClickHouse no MVP”. A primeira frase põe span metrics no ClickHouse; a segunda proíbe métricas no ClickHouse. FR-20 (linhas 330–338) exige RED derivado de “todos os spans que passam pelo gateway, antes de qualquer amostragem” e “armazenadas separadamente do store de traces amostrados”. O addendum (linha 346) é explícito: “Em produção, as métricas RED são calculadas no gateway antes do tail sampling e gravadas como métricas; um MV sobre `otel_traces` armazenados só serve para análise de traces retidos e não é a fonte de verdade de RED.” Mesmo assim, o addendum mantém o DDL `span_metrics_1m` (linhas 348–368) no ClickHouse.

**Impacto.** O mesmo RED tem dois donos, dois schemas, duas retenções e duas semânticas: o MV do ClickHouse só vê traces retidos (e após tail sampling), o VictoriaMetrics vê todos os spans. Dashboards, alertas e service map podem mostrar números diferentes para o mesmo serviço. A meta de FR-20 (“desvio < 1% contra a contagem do gateway”) não é verificável se metade dos consumidores lê o MV. A retenção de traces (7–15 dias) e a retenção global de métricas (90 dias) também divergem. O roteamento “por sinal” do AD-5/AD-6 não resolve: um painel de RED é “métrica” e vai para o VM; o service map é “trace” e vai para o CH.

**Correção.** Declarar span metrics/RED como um sinal de métrica com um único dono. A opção mais coerente com FR-20 é: gateway calcula RED antes do sampling e grava no VictoriaMetrics; o MV do ClickHouse é removido ou renomeado para “analytics de traces retidos”, explicitamente não autoritativo, e nenhum alerta/dashboard o consome. Definir schema do RED (serviço, operação, status, histograma de duração, janela), retenção e labels. Se a escolha for ClickHouse, remover o caminho VM e assumir a exceção ao “métricas não vão para CH” — mas então AD-5 precisa ser reescrito, não interpretado.

---

#### C-03 — A idempotência por `event_id` não é implementável como escrita e permite perda após o ack

**Evidência.** AD-2 (linha 45) exige que “todo evento carrega `(tenant_id, project_id, signal, event_id)`; `event_id` é estável e idempotente”. AD-4 (linha 57) exige “dedupe por `event_id`”. O OTLP não tem `event_id` em log records, spans ou data points de métrica; o gateway precisa sintetizá-lo. Se o SDK reenvia o mesmo evento após 429/erro de rede, o gateway gera um novo UUIDv7 e o consumidor vê um ID novo. O DDL do addendum usa `ENGINE = MergeTree` (linha 299/337), que não deduplica; `ReplacingMergeTree` deduplicaria apenas eventualmente e mudaria a ordem de leitura. O addendum (linha 131) diz que os writers escrevem “batches grandes, assíncronos”, e o diagrama de ingestão (linha 270) mostra `W->>CH: insert logs/traces (native, async)` seguido de `W-->>N: ack`. Se o async insert retorna antes da durabilidade e o writer dá ack no NATS, uma queda do ClickHouse perde eventos que o gateway já confirmou ao cliente. Além disso, o VictoriaMetrics pode rejeitar séries por cardinalidade/limite (addendum 5.3, linha 374) e o ClickHouse pode rejeitar por `Too many parts`/schema (addendum §9); nada define DLQ ou reparo depois do ack. O NATS JetStream é at-least-once no consumo; o `Nats-Msg-Id` só dá exactly-once de publicação, não de consumo.

**Impacto.** Duplicatas visíveis em retries de SDK, redelivery de NATS e replay; contagem dupla de RED, quotas e custo. Perda de eventos confirmados em crash do CH durante async insert, em rejeição de cardinalidade do VM e em lag de writer maior que a retenção do stream. A promessa “RPO 0 para eventos confirmados dentro da janela de buffer/quórum” (FR-4) não se sustenta em single-node (sem quórum) nem no caminho assíncrono. A promessa “sem duplicatas visíveis” também não.

**Correção.** Definir um AD de idempotência e durabilidade: (a) chave determinística por evento (hash canônico de tenant/signal/resource/scope/timestamp/payload) ou `Idempotency-Key` fornecido pelo produtor; (b) store de dedupe com janela maior que o maior retry/outage, ou `ReplacingMergeTree`/insert dedupe com semântica explícita; (c) writer só dá ack no NATS depois de `async_insert=0`/`wait_for_async_insert=1` ou de confirmação de flush; (d) retenção do stream NATS maior que a janela de outage + tempo de reparo; (e) DLQ e procedimento de replay para rejeições permanentes; (f) definição por sinal: cumulative metrics podem ser idempotentes por timestamp, delta metrics não são sem estado. Aplicar admissão no gateway para que rejeições permanentes aconteçam antes do ack ou virem DLQ explícita.

---

#### C-04 — Retenção, quota, pipeline e entitlements não têm caminho compatível entre PostgreSQL e data plane

**Evidência.** AD-7 (linha 75) diz que “todo estado de configuração vive no PostgreSQL”. AD-1 (linha 39) diz que o control plane não participa da ingestão. FR-5 (linha 173) exige limites “ajustados sem reiniciar o serviço”; FR-3 (linha 154) exige pipeline configurável por UI/API; FR-40 (linha 548) exige alterar retenção sem reiniciar; AD-11 (linha 99) exige entitlements assinados. O addendum (linha 344) deixa a retenção por projeto como decisão de spike: (a) coluna `retention_days` no TTL ou (b) tabelas/MVs por bucket. O DDL do addendum tem `retention_logs_days`/`retention_traces_days` nas projects (linhas 392–394) e TTL fixo de 15 dias no ClickHouse (linha 302). O ERD do spine tem uma entidade `RETENTION_POLICY` (linha 334), além das colunas. Não há AD dizendo quem lê o PG, quem aplica no CH/VM, com que staleness, e o que acontece se o PG estiver indisponível.

**Impacto.** O gateway não pode ler o PG (AD-1) nem usar arquivo local (AD-7); se ele lê o PG, viola AD-1; se usa snapshot local, não há contrato de atualização e FR-5/FR-40 viram ficção. A retenção tem dois donos: a coluna no PG e o TTL no CH; uma mudança na UI pode não chegar ao storage, ou chegar por um reconciliador do control plane que muta o data plane (violando AD-1). O mesmo vale para quotas, regras de pipeline e entitlements: o gateway/QS precisam deles no hot path, mas não há caminho sancionado. Em indisponibilidade do control plane, ou o data plane usa política velha (risco de segurança/custo) ou para (indisponibilidade).

**Correção.** Criar um AD de **config plane** separado do data plane: PG é o desired state; um snapshot versionado e assinado é distribuído por push/pull (NATS ou endpoint interno) com `config_version`, `generation` e staleness máxima; o data plane aplica o último snapshot conhecido, falha fechado para segurança/quota quando o snapshot está velho, e expõe `config_age_seconds`. Um único reconciliador (no data plane ou num serviço de configuração, nunca no handler da API) aplica retenção no CH/VM. A retenção deve ter um dono único: ou coluna por linha + TTL (se suportado e testado), ou buckets com roteamento explícito; o PG guarda apenas o desejo. Adicionar exceção explícita ao AD-1 para leitura de configuração, ou mover a configuração para um serviço que não é “control plane” no sentido do AD-1.

---

#### C-05 — Tenancy não tem identidade imutável nem propagação para move/delete/cache

**Evidência.** AD-2 (linha 45) define a identidade como `(tenant_id, project_id, signal, event_id)`; AD-3 (linha 51) diz que o token resolve tenant/projeto no gateway, o QS injeta o predicado e o ClickHouse usa row policies; o addendum (linha 372) usa path `/insert/{tenant}/{project}` e `/select/{tenant}/{project}` no VictoriaMetrics. FR-45 (linha 600) permite “mover um projeto entre organizações com auditoria”; FR-44 (linha 584) exige exclusão de dados em hot e cold em até 24h. O cache do QS (addendum 3.4, linha 155) tem chave `(consulta + intervalo + tenant + versão do schema)`, sem `authz_epoch` nem geração do projeto. O ERD (linhas 329–342) não tem geração/epoch; as colunas de retenção estão no projeto, mas o dado já gravado carrega o `tenant_id` antigo no CH e o path antigo no VM.

**Impacto.** Mover um projeto entre organizações muda o `tenant_id`; os dados já gravados continuam com o `tenant_id` antigo, o path do VM antigo, tokens antigos, dashboards e caches antigos. Dependendo da implementação, o novo tenant não vê os dados (parece perda) ou o tenant antigo continua vendo (vazamento). Excluir um projeto remove o registro no PG, mas o CH/VM só expiram por TTL; a promessa de 24h não é cumprida. O cache pode servir resultado de antes do move/revogação. No VM, não há row policy; a única barreira é o path montado pelo QS. No CH, row policy + predicado do QS podem ser aplicados duas vezes, ou a row policy pode não valer para MVs/views com `SQL SECURITY DEFINER`. O teste de isolamento pode passar no CH e falhar no VM, ou passar nos dois e falhar no cache.

**Correção.** Declarar `project_id` imutável e autoritativo; `project move` vira “novo projeto + cópia + delete” com `project_generation`/`authz_epoch` em todas as chaves: linhas do CH, path do VM, subject do NATS, cache, tokens e auditoria. Ou banir `move` no MVP. Definir um único ponto de enforcement por store: no CH, row policy + predicado via AST/parâmetro (não string), com teste de MVs/views; no VM, path com proxy autenticado (vmauth) ou labels injetados no write, com teste de query vazia vs vazamento. Incluir `authz_epoch` na chave de cache e invalidar em move/revogação. Definir delete como job assíncrono com tombstone, cobertura de hot/cold e prova de auditoria.

---

### 2.2 Alto

#### A-01 — Não existe contrato do envelope interno gateway → NATS → writer, nem estratégia N-1 de upgrade

**Evidência.** AD-2/AD-4 definem os campos do evento, mas não o formato de mensagem: protobuf OTLP? JSON? Arrow? qual compressão? qual versão? qual batch? O gateway e os writers são processos separados (containers, linhas 202–208) e podem ser atualizados em momentos diferentes. AD-9 cobre apenas schema do ClickHouse, não o contrato interno. AD-12 exige “mesmas imagens” e “mesmo contrato de configuração”, mas não diz como uma imagem v2 convive com uma v1 durante rolling upgrade. O addendum (linha 131) fala em “batches grandes, assíncronos”, sem schema.

**Impacto.** Um rolling upgrade com gateway v2 publicando um envelope novo e writer v1 lendo o antigo pode descartar campos, falhar o parse ou gravar `tenant_id`/`event_id` errados. Replay de NATS após upgrade pode ser ilegível. O mesmo vale para o contrato QS ↔ API e para o formato de resultado. Sem N-1, a promessa de “upgrade sem downtime” (FR-62) não é testável.

**Correção.** AD de contrato interno: schema protobuf versionado (ou OTLP proto congelado) com `schema_version`, campos reservados, compatibilidade backward/forward, testes de contrato gateway↔writer, e política N-1 (writer novo lê envelope antigo; writer antigo ignora campos novos). Definir upgrade ordering, dual-publish/dual-read durante a janela e replay compatível.

---

#### A-02 — Query Service não tem contrato de admissão, streaming, paginação, async results ou cancelamento

**Evidência.** AD-6 (linha 69) diz que o QS aplica “limites, timeout, cache e formato de resposta”; o addendum (linhas 151–159) acrescenta jobs assíncronos com progresso e resposta Arrow/JSON. FR-13 exige live tail por SSE/WebSocket; FR-27 exige cancelamento com liberação em até 2s; FR-12 exige “primeiros resultados” em 1B de linhas. Não há AD para: prioridade entre alertas, dashboards e ad-hoc; fila/limite de concorrência; paginação/cursor; resultado assíncrono (onde fica? TTL? isolamento?); streaming; cancelamento propagado ao CH/VM; content negotiation.

**Impacto.** Um tenant com dashboard pesado pode ocupar todas as conexões e degradar alertas (noisy neighbor). Live tail com timeout do QS quebra. Resultados assíncronos armazenados em memória somem no restart; armazenados no PG violam AD-1; armazenados no CH sem TTL/tenant podem vazar. Cada cliente (UI, CLI, alertas) implementa um formato diferente. A meta de “20 dashboards simultâneos sem degradar p95 em mais de 20%” (PRD 8.1) não tem mecanismo.

**Correção.** AD de protocolo de consulta: media types (JSON/Arrow), paginação por cursor/keyset, streaming com backpressure, `query_id` idempotente, cancelamento por contexto propagado, classes de prioridade (alertas > dashboards > ad-hoc), quotas de concorrência/memória/tempo por tenant, 429/Retry-After, e store de resultados assíncronos no data plane com TTL e escopo de tenant. Testar fairness e noisy neighbor.

---

#### A-03 — Auto-observabilidade é circular e não tem caminho de bootstrap nem dead-man's switch

**Evidência.** AD-10 (linha 93) exige que “todo serviço expõe `/healthz`, `/readyz`, métricas Prometheus e traces OTel” e que o produto tenha um painel de saúde; “o Observaki usa o próprio produto para se observar quando possível”. FR-61 exige painel com lag, erro de consulta e disco. Não há AD dizendo onde as métricas de auto-observabilidade são armazenadas, se passam pelo gateway/WAL/CH/VM do próprio produto, nem como o painel funciona quando o gateway/CH/VM está fora. Também não há quota reservada para self-telemetry.

**Impacto.** Exatamente no incidente em que o health panel é necessário, ele fica vazio porque o pipeline que o alimenta está quebrado. Se a self-telemetry passa pelo mesmo WAL, um storm de telemetria interna pode competir com tenants. Se é scrape externo, há dois caminhos de métricas e nenhum store declarado. Não há dead-man's switch para alertar que o alerting morreu.

**Correção.** AD de meta-observabilidade: caminho separado (Prometheus/OTel Collector local ou externo), store independente, scrape externo e/ou export para um destino fora do blast radius, quota reservada, alertas de “último dado recebido” e dead-man's switch. Documentar o observer effect e o que o painel mostra em modo degradado (último valor conhecido + timestamp).

---

#### A-04 — Envelope operacional incompleto: ambientes, backup/restore, DR, capacidade, segredos, ingress e network policy

**Evidência.** AD-12 (linha 105) cobre apenas topologias (Compose/Helm). O production readiness gate do PRD (linha 1008) exige backup/restore de metadados, runbooks, upgrade/rollback testado, auditoria mínima e revisão de segurança. FR-63 exige backup/restore e DR; FR-62 exige HA e upgrades sem downtime; FR-64 exige capacity planning. O spine não tem AD de backup, restore, DR, ambientes (dev/staging/prod), capacidade, segredos/bootstrap, ingress/TLS, network policy, PDB/anti-affinity, store auth ou migração. A árvore `deploy/` só tem `compose/` e `helm/observaki/` (linhas 402–404).

**Impacto.** O produto não consegue cumprir o próprio gate de production readiness. Design partners não têm caminho de upgrade/rollback nem restore. Um cluster T1 de 6–12 nós não tem sizing de disco/IOPS, requests/limits, topology spread ou PDB. O RTO < 30 min e o RPO de metadados de 1h não têm mecanismo. A operação de 4h/semana (PRD 8.5) não tem runbooks.

**Correção.** ADs operacionais: (a) ambientes e promoção de config; (b) backup/restore de PG, CH, VM e NATS, com restore testado; (c) DR e drill trimestral; (d) capacity model e T-shirt sizes; (e) secrets bootstrap/rotação; (f) ingress/TLS/cert-manager; (g) network policies e store auth; (h) PDB/anti-affinity/topology spread; (i) migração/upgrade job com advisory lock. Cada um com dono e teste.

---

#### A-05 — Stack: Kubernetes 1.33 está EOL, Helm 3 está em fim de vida, Node 24 virou Maintenance LTS e o Collector não está pinado

**Evidência.** O spine declara Kubernetes 1.33+ (linha 168) e o PRD declara 1.28+ (linha 925). O Kubernetes 1.33 chegou a EOL em 28/06/2026; a linha suportada em outubro de 2026 é 1.35–1.37. O spine declara Helm 3.18+ (linha 169); o Helm 3 teve a última feature release em 09/09/2026 e patches até fevereiro de 2027, enquanto o Helm 4.3.1 é o corrente. O spine declara Node.js 24 LTS (linha 164); o Node 24 entrou em Maintenance LTS em 20/10/2026 e o Node 26 é o Active LTS desde 28/10/2026. O Collector está como “0.1xx (versão fixada no lockfile)” (linha 167), mas “0.1xx” é uma faixa; o corrente é 0.163.0 (out/2026). As demais versões estão corretas: Go 1.27.3 (19/10/2026), ClickHouse 26.8 LTS (27/08/2026, suporte até 27/08/2027), VictoriaMetrics 1.148 LTS (1.148.1 em 31/07/2026), PostgreSQL 18.6 (13/08/2026), NATS 2.15.1 (linha 2.15 de 17/09/2026), SvelteKit 3.1.1, Vite 8.x, Tailwind 4.3.3 e ConnectRPC 2.x (connect-go v2).

**Impacto.** Um produto novo nasce suportando um Kubernetes sem patches de segurança e um Helm em EOL programado; o PRD e o spine divergem sobre o mínimo. O Node 24 ainda é LTS, mas já em manutenção; para um projeto que começa agora, é uma escolha conservadora demais. O Collector sem pin impede reprodutibilidade e SBOM. A matriz de compatibilidade do PRD §8.7 não é confiável.

**Correção.** Subir o mínimo para Kubernetes 1.35 (ou 1.36), Helm 4.3+, Node 26 LTS e Collector 0.163.x pinado; alinhar PRD §8.7 e spine; adicionar política de suporte N-1/N-2 com datas de EOL e um teste de matriz. Manter ClickHouse 26.8 LTS e VM 1.148 LTS, mas incluir o upgrade LTS→LTS no plano (ver A-01).

---

#### A-06 — OTLP: `Map(String,String)` perde tipos, semconv não está pinada e span events estão em depreciação

**Evidência.** O addendum usa `attributes Map(String, String)` e `resource_attributes Map(String, String)` (linhas 290–291, 330–331). OTLP `AnyValue` inclui bool, int, double, string, array, kvlist e bytes; converter tudo para String perde tipo e semântica. FR-1 exige preservar resource attributes, scope attributes e atributos customizados; FR-8 exige mapeamento OTel → MetricsQL documentado. AD-9 diz “OTLP 1.x”, mas não pina semantic conventions; as convenções HTTP, por exemplo, mudaram nomes (`http.status_code` → `http.response.status_code`), e a OTel anunciou em 2026 a depreciação da Span Events API em favor de logs. O DDL de traces ainda tem `events Nested(...)` (linha 332).

**Impacto.** Filtros e agregações por atributo retornam tipos errados (string “200” vs int 200), dashboards quebram quando a semconv muda, e spans de SDKs novos podem não ter `events` no formato esperado. A “compatibilidade OTLP” vira “aceita o protobuf, mas distorce o modelo”. A matriz de compatibilidade do FR-8 não tem como ser validada.

**Correção.** AD de mapeamento OTLP: codificar tipo junto do atributo (ex.: `Map(String, String)` + colunas tipadas para quentes, ou JSON com type tags), pinar a versão das semantic conventions e manter uma camada de tradução, decidir o futuro de span events (logs vs nested), e rodar a suíte de contrato com SDKs Go/Java/Python/Node e com payloads reais de arrays/kvlist.

---

#### A-07 — O “subset PromQL/MetricsQL” não existe e a compatibilidade do VictoriaMetrics não é total

**Evidência.** AD-16 (linha 129) exige “o subset PromQL/MetricsQL documentado” no CI, mas o spine não lista o subset. FR-7 (linha 200) fala em “suíte de conformidade PromQL selecionada”; FR-8 exige `histogram_quantile`, histogramas OTel e exponential histograms. O VictoriaMetrics usa MetricsQL, que é compatível com PromQL, mas não 100% idêntico; resultados públicos de conformidade já mostraram lacunas. O addendum (linha 206) promete converter tipos sem equivalente nativo para histograma explícito, mas não define a matriz.

**Impacto.** “PromQL compatível” é uma promessa não testável e potencialmente falsa. Um dashboard importado do Grafana pode falhar em funções/operadores que o VM não implementa ou implementa diferente. Alertas podem calcular valores diferentes do Prometheus. A matriz de compatibilidade não tem fonte de verdade.

**Correção.** Publicar o subset como lista versionada (funções, operadores, tipos de histograma, subqueries, `@`, `offset`, `bool`, etc.), rodar a suíte de conformidade Prometheus + testes MetricsQL específicos, falhar o CI em regressão, e publicar a matriz por versão do VM. Documentar explicitamente as diferenças de `histogram_quantile` para OTel exponential histograms.

---

#### A-08 — OpenAPI e ConnectRPC são dois contratos concorrentes; `/api/v1` não é um path Connect

**Evidência.** AD-8 (linha 81) diz que o frontend consome “a API versionada (ConnectRPC/OpenAPI)”; AD-9 (linha 87) diz que “a API é `/api/v1` com versionamento semântico”; FR-57 exige “API REST e gRPC com OpenAPI publicado e validado no CI”. O ConnectRPC usa rotas no formato `/package.Service/Method` (ou HTTP annotations), não `/api/v1`. Não há AD dizendo se o proto é a fonte de verdade e o OpenAPI é gerado, ou o contrário. Também não há regra de versionamento para pacote proto, path HTTP, schema JSON e CLI.

**Impacto.** Dois contratos podem divergir silenciosamente; o OpenAPI “validado no CI” pode validar um artefato gerado que não corresponde ao servidor. Clientes gerados quebram em upgrades menores. A versão semântica não tem um eixo definido. A UI e a CLI podem falar dialetos diferentes.

**Correção.** AD de contrato de API: proto é canônico; OpenAPI é gerado e comparado no CI (diff gate); HTTP annotations definem `/api/v1`; erros e paginação são mapeados uma vez; versionamento segue regras explícitas (campo proto reservado, path, JSON schema, deprecação); SDKs gerados. Testes de contrato contra o servidor real, não só contra o arquivo.

---

#### A-09 — Prometheus scrape não tem dono; o gateway é stateless

**Evidência.** FR-2 (linha 144) exige scrape de targets Prometheus com configuração estática; o addendum (linha 126) diz que o gateway aceita “Prometheus remote write” e que adaptadores entram na Fase 2; a árvore de código (linhas 381–408) e o diagrama de containers (linhas 196–234) não têm um componente de scrape. O gateway é declarado stateless (AD-1/AD-4), mas um scraper tem estado (targets, posição, WAL) e, em HA, duas réplicas fariam scrape duplicado.

**Impacto.** A feature não tem owner arquitetural. Em cluster com 2+ gateways, os mesmos targets são raspados duas vezes, gerando séries duplicadas e cardinalidade; em restart, o estado de scrape se perde. Autenticação de targets, relabeling e service discovery (Fase 2) não têm onde viver. O remote write precisa mapear token → tenant, mas o header de tenant do Prometheus não é o token do Observaki.

**Correção.** AD de scrape: componente separado (`promscrape`/agente) com sharding ou leader election, estado durável, target auth, config versionada, e um contrato de remote write que mapeia token → tenant/project. O gateway permanece stateless e só recebe OTLP/remote write.

---

#### A-10 — O modelo de dados do control plane não bate entre ERD e DDL, e não há tabelas de RBAC

**Evidência.** O ERD (linhas 329–342) tem `NOTIFICATION_CHANNEL` como entidade filha de `ALERT_RULE`, `RETENTION_POLICY` como entidade de `PROJECT`, `PANEL` como entidade de `DASHBOARD`, `API_TOKEN` como entidade de `PROJECT`, `MEMBERSHIP` de `USER`/`ORGANIZATION`, `SLO`/`ERROR_BUDGET`, `AUDIT_EVENT`. O DDL do addendum (linhas 379–432) tem `channels JSONB` dentro de `alert_rules`, `retention_logs_days`/`retention_traces_days` dentro de `projects`, `definition JSONB` dentro de `dashboards`, `audit_log` (não `AUDIT_EVENT`), e **não tem** tabelas de memberships, roles, tokens, silences, SLOs ou error budgets. Não há atributo de role no membership; FR-47 exige papéis admin/editor/viewer por projeto.

**Impacto.** O mesmo dado tem duas representações (channel normalizado vs JSONB; retention policy vs colunas; panel vs JSONB), e a implementação vai escolher uma — mas o ERD e o DDL contam histórias diferentes. O RBAC por projeto não é representável: não há membership por projeto nem role. Atualizar um webhook exige editar N regras se `channels` for JSONB; ou exige uma tabela que não existe. A auditoria não tem FK nem modelo de imutabilidade. As migrações do control plane vão divergir.

**Correção.** Escolher o modelo canônico e alinhar ERD/DDL: channels normalizados (com FK por projeto/org) ou JSONB, não ambos; retention columns ou policy table, não ambos; dashboard definition JSONB é ok, mas `PANEL` não deve ser entidade separada; adicionar `memberships(user_id, project_id/org_id, role)`, `roles/permissions`, `api_tokens`, `silences`, `slos`, `error_budgets`; renomear `audit_log`/`AUDIT_EVENT` para um nome só. Testes de migração e de integridade referencial.

---

#### A-11 — Sizing e custo têm erro aritmético de 2x e misturam tamanhos de evento

**Evidência.** O PRD (linha 960) diz que 1M eventos/s × 134 B = 11,6 TB/dia e que “30 dias com réplica 2 exigem ~348 TB”. 11,6 × 30 × 2 = 696 TB, não 348 TB. O addendum (linhas 460–461) diz que 15 dias = 174 TB e, com réplica 2, 348 TB — consistente com 15 dias, não com 30. Os cenários de custo (addendum §10.2/§10.3) usam 100 GB/dia com 100M eventos/dia e 1 TB/dia com 1B eventos/dia; a 200 B/evento, 100M eventos/dia = 20 GB/dia e 1B = 200 GB/dia, não 100 GB/1 TB. Os números de custo estão, na prática, assumindo ~1 KB/evento, não os 200 B do workload de referência.

**Impacto.** O sizing de disco e o custo por TB ficam subestimados em ~2x no cenário de 30 dias; o design partner pode comprar metade do hardware necessário. O benchmark e o modelo de custo usam premissas diferentes, então o “≤30% do Datadog” não é comparável. A decisão de retenção default (7/15/30) fica sem base.

**Correção.** Publicar uma única tabela de sizing com unidades explícitas (TB comprimido/dia, fator de réplica, retenção, overhead de índices/MVs, headroom), reconciliar PRD/addendum, e adicionar um teste de aritmética no CI da documentação. Separar “evento” de “byte” e declarar o tamanho médio por sinal em cada cenário.

---

#### A-12 — Entitlements não têm ponto de enforcement, e alguns cruzam AD-1/AD-3/AD-5

**Evidência.** AD-11 (linha 99) diz que features enterprise usam “entitlement assinado e falham fechadas apenas nelas”; a lista inclui SSO/SCIM, audit avançado, RBAC granular, federação e retenção por tenant de métricas. O addendum (linha 753) repete a lista. Não há AD dizendo onde o entitlement é verificado (API? QS? gateway? VM? scheduler?), como a chave pública é distribuída, como funciona offline/air-gapped (FR-55), o que acontece se o license server estiver indisponível, nem como “falhar fechado apenas na feature” é implementado sem bloquear telemetria. A retenção por tenant de métricas é enterprise, mas FR-40 Fase 2 a exige; o VM OSS só tem retenção global (addendum 5.3, linha 373).

**Impacto.** Ou o OSS não cumpre FR-40 Fase 2, ou o enterprise vaza para o OSS, ou o enforcement no data plane depende do control plane (violando AD-1). Um license server fora do ar pode desligar SSO/RBAC em produção. A fronteira open-core fica indefinida, e a promessa “nenhum caminho de telemetria é paywalled” não tem teste.

**Correção.** AD de entitlements: formato assinado (ex.: JWT/Ed25519), chave pública embutida, grace period offline, enforcement points explícitos (API para SSO/SCIM/RBAC; QS para federação; um job de retenção para métricas), feature flags que falham fechado por feature, auditoria de licença, e teste que prove que ingestão/consulta básica continuam sem licença. Para retenção de métricas, definir a alternativa OSS (instâncias/buckets separados) ou mover a feature para o enterprise sem prometer no OSS.

---

#### A-13 — Segurança interna e autenticação dos stores não estão no spine

**Evidência.** AD-13 (linha 111) cobre OIDC/OAuth2, Argon2id, TLS 1.3, tokens com hash, segredos em vault/KMS e RBAC no control plane/QS. Não cobre TLS/mTLS entre gateway, NATS, CH, VM, PG e QS; não cobre autenticação do ClickHouse (usuário/senha, roles), do VictoriaMetrics (OSS não tem auth embutida; vmauth é um componente separado), do NATS (accounts/credenciais) ou do PG. O QS abre conexão direta com CH/VM (AD-6), mas não há AD de credenciais, rotação ou least privilege. “Tokens de API com hash” não diz qual hash; Argon2id é para senha, não para token por request.

**Impacto.** Um pod comprometido ou um network sniff pode ler toda a telemetria; o CH default user e o VM sem auth são alvos óbvios. Se o QS usa um usuário CH com row policy, a gestão de usuários vira estado de control plane no data plane; se usa um único usuário, a row policy não protege. A ausência de mTLS quebra o “isolamento em profundidade” do AD-3.

**Correção.** AD de segurança interna: mTLS entre serviços, store auth (CH users/roles, NATS accounts, PG TLS, VM via vmauth), network policies, secrets/credenciais rotacionadas, least privilege, egress policy para bloquear SSRF, e definição de hash de token (SHA-256/HMAC, não Argon2id). Testar acesso direto aos stores a partir de um pod não autorizado.

---

#### A-14 — Alerting não tem semântica de entrega, cache bypass ou outbox

**Evidência.** FR-31 exige retry com backoff; FR-32 exige estado, dedupe, silence e acknowledge; o addendum (linha 175) diz que o scheduler avalia a cada 15–60s, o estado vive no PG e as notificações passam por uma fila com dedupe/agrupamento/backoff. AD-6 faz o alerting passar pelo QS, que aplica cache. Não há AD para: identidade de serviço do scheduler, cache bypass/prioridade, dedupe key, silence matching, outbox transacional, idempotência de notificação, dead-man's switch ou SLO de entrega.

**Impacto.** Cache de 5 min + avaliação de 15s = alerta atrasado ou perdido. Crash entre “estado firing” e “enqueue” = alerta sem notificação; crash entre “send” e “ack” = notificação duplicada. Um silence preso a uma regra (ERD) não silencia o mesmo incidente em regras correlatas. Durante uma queda do PG/QS, o alerting morre junto — e o produto não avisa que o alerting morreu.

**Correção.** AD de alerting: scheduler com identidade de serviço e token tenant-scoped; consulta com bypass de cache ou classe de prioridade; outbox transacional no PG com idempotency key `(rule_id, fingerprint, state_version)`; entrega at-least-once + dedupe no notifier; modelo de silence por matcher (não por regra); métricas de evaluation lag e delivery; dead-man's switch independente. Testar falhas de PG, QS, notifier e canal.

---

#### A-15 — Guardrails de cardinalidade não têm dono único e podem rejeitar depois do ack

**Evidência.** FR-5 (linha 173) limita cardinalidade por tenant/projeto; FR-10 (linha 225) exige detectar explosões e sugerir agregação/drop; AD-3 fala em quotas de consulta; o addendum (linha 374) fala em limites por tenant via vmagent/vminsert. O gateway não tem estado para saber cardinalidade; o VM tem limites globais ou por tenant (enterprise). Não há AD dizendo onde a série é admitida/rejeitada, nem como o tenant A não afeta o B.

**Impacto.** Um tenant com 1M de séries pode estourar o VM e degradar todos; o limite global do VM não é fairness. Se o gateway aceita e o VM rejeita depois do ack, há perda de evento confirmado (C-03). Se o gateway dropa por número de labels, isso não é cardinalidade real. A meta de “p95 dos outros tenants não varia mais de 10%” (FR-5) não tem mecanismo.

**Correção.** AD de cardinalidade: limite por tenant no ponto de escrita (vmauth/vminsert com tenant isolation ou proxy), estimador no gateway com estado por tenant, admissão antes do ack, drop/agregação com trilha de auditoria e notificação, cardinality explorer, e teste de noisy neighbor com dois tenants.

---

#### A-16 — TTL usa timestamp do cliente; não há `ingested_at` nem política de skew/futuro

**Evidência.** O DDL do ClickHouse usa `TTL toDateTime(timestamp) + INTERVAL 15 DAY` (linhas 302/340). `timestamp` é o timestamp do evento, controlado pelo cliente/OTLP; o addendum também tem `observed_timestamp`, mas o TTL não o usa. O PRD (linha 929) tolera clock skew de 2 minutos “no waterfall”, mas não define o que o gateway faz com timestamps futuros/passados. FR-40 exige retenção por projeto; FR-4 exige RPO.

**Impacto.** Um cliente com relógio errado (ou malicioso) pode enviar timestamps no futuro e nunca expirar, inflando custo; timestamps muito antigos expiram no primeiro TTL, parecendo perda. Partições por dia com timestamp futuro criam partições fora da janela. A retenção deixa de ser uma política de ingestão e vira uma política do cliente.

**Correção.** AD de tempo: `ingested_at`/`observed_timestamp` como autoridade para TTL/partição; `timestamp` do cliente preservado para consulta, com flag de skew; clamp/rejeição de timestamps fora de uma janela (ex.: ±2 min ou ±24h, configurável); teste com relógio adiantado/atrasado e evento tardio.

---

### 2.3 Médio

#### M-01 — Single-node e cluster prometem o mesmo contrato, mas têm WAL e banco diferentes

**Evidência.** AD-4 (linha 57) permite “buffer local durável no single-node”; AD-7 (linha 75) permite SQLite no single-node; AD-12 (linha 105) exige “mesmas imagens/binários e o mesmo contrato de configuração”. São dois adapters de WAL e dois de banco, com semânticas de durabilidade, transação e migração diferentes. Não há matriz de capacidades nem testes de contrato dos adapters.

**Impacto.** O que passa no Compose pode falhar no Helm e vice-versa; o RPO 0 do single-node não existe sem quórum; migrações SQLite/PG divergem; um bug de SQLite pode não aparecer no cluster. “Nenhum fork de código” é verdade no binário, mas não no comportamento.

**Correção.** Matriz de topologia (feature, RPO, RTO, HA, retenção, WAL, DB), testes de contrato para os ports de WAL/DB, e uma decisão explícita: ou o single-node usa NATS/Postgres, ou o cluster aceita o buffer local. Declarar RPO/RTO por topologia.

#### M-02 — Frontend: embed, CSP, sessão, assets e SSE não têm AD

**Evidência.** AD-8 (linha 81) embute a SPA no binário Go; AD-14 (linha 117) limita o bundle a 180 KB gzip. O frontend usa CodeMirror 6, uPlot, Tailwind e Connect; FR-13 usa SSE/WebSocket. Não há AD para CSP, armazenamento de token (memória vs cookie), CSRF, cache/versionamento de assets, timeouts de ingress para SSE, ou breakdown do bundle por rota.

**Impacto.** XSS pode roubar token; cookies SameSite/Secure quebram no dev proxy; assets antigos após upgrade; live tail cortado por proxy timeout; bundle estoura o orçamento e o CI não sabe qual rota paga a conta.

**Correção.** AD de frontend: CSP, estratégia de sessão/token, CSRF, asset hashing, cache headers, timeouts de SSE/WS, e orçamento por rota (Explore, dashboard) com Lighthouse/axe no CI.

#### M-03 — Audit log não é imutável como FR-49 exige

**Evidência.** FR-49 (linha 635) exige “log imutável e exportável”; o addendum tem `audit_log` (linha 423) com `BIGSERIAL`, inserts da aplicação, sem hash chain, WORM, permissões de append-only ou export contínuo. AD-7 diz que auditoria vive no PG.

**Impacto.** Um admin de banco pode alterar/apagar auditoria; a afirmação de imutabilidade não é verificável; a exportação e a retenção (Fase 3) não têm mecanismo. Para compliance, o log de auditoria é justamente o alvo.

**Correção.** Tabela append-only com permissões restritas, hash chain/assinatura, export para object storage WORM, auditoria de acesso à auditoria, retenção e teste de tamper.

#### M-04 — O envelope de erro não tem mapeamento de transporte nem catálogo de códigos

**Evidência.** AD-15 (linha 123) define `{code, message, details, request_id}`. ConnectRPC usa `code`, `message`, `details` (protobuf Any); HTTP usa status + body. Não há AD para mapear gRPC status ↔ HTTP status, `retryable`, catálogo de códigos estáveis, ou relação `request_id` ↔ `trace_id`.

**Impacto.** Cada cliente interpreta erro de forma diferente; retry pode ser feito em erro permanente ou omitido em transitório; logs não correlacionam com traces; a UI mostra mensagem genérica.

**Correção.** Catálogo de erros versionado, mapeamento canônico por transporte, flag `retryable`, `request_id` = `trace_id` ou campo de link, e testes de contrato de erro.

#### M-05 — O DDL do MV de span metrics está tecnicamente incorreto

**Evidência.** O addendum (linhas 362–365) define `quantileTDigest(0.5)(duration_ns) AS p50_state` em uma tabela `AggregatingMergeTree`. Funções de agregação em `AggregatingMergeTree` precisam de `-State` na escrita e `-Merge` na leitura; `quantileTDigest` retorna `Float64`, não um estado. O alias `p50_state` sugere estado, mas o tipo não é `AggregateFunction`. O próprio addendum diz que o MV é ilustrativo e não é a fonte de verdade de RED, mas o spine o trata como parte do storage de span metrics.

**Impacto.** O MV não agrega corretamente; os p50/p95/p99 ficam errados ou a tabela não cria como esperado; consultas que usem `quantileTDigestMerge` falham. Se alguém seguir o DDL, o RED do CH é inválido.

**Correção.** Usar `quantileTDigestState(0.5)(duration_ns)` na MV e `quantileTDigestMerge(0.5)(p50_state)` na consulta; testar com dados conhecidos; ou remover o MV e deixar RED no VM (C-02).

#### M-06 — AD-3 bloqueia `s3`/`remote`/`url` permanentemente, mas FR-41/FR-16 e federação precisam deles

**Evidência.** AD-3 (linha 51) bloqueia table functions `url`, `s3`, `remote`, `file`, `mysql`, `postgresql` e tabelas de sistema. FR-41 (Fase 3) exige tiering cold em S3; FR-16 exige archive search; a federação (Fase 4) usa query router/remote. AD-3 não tem escopo de fase nem exceção para componentes internos.

**Impacto.** Na Fase 3, ou o time viola AD-3 para implementar archive/federação, ou não entrega a feature. A revisão de segurança futura vai encontrar uma contradição documentada. O mesmo vale para `remote` na federação.

**Correção.** Escopar o bloqueio ao parser/role do usuário, não ao sistema; criar role interno privilegiado para archive/federação, com egress policy e auditoria; documentar a exceção e o teste que impede o usuário de invocá-la.

#### M-07 — Paginação, ordenação estável e formato de resultado não estão definidos

**Evidência.** FR-12 exige “primeiros resultados” em 1B de linhas; FR-23 exige histórico; AD-6 fala em “formato de resposta”; o addendum fala em Arrow/JSON. Não há AD para cursor/keyset, ordenação estável, limite máximo, streaming ou `query_id`.

**Impacto.** Offset pagination em 1B de linhas é lenta e inconsistente; a UI pode duplicar/perder linhas entre páginas; cada cliente inventa um formato; resultados grandes estouram memória.

**Correção.** AD de paginação: keyset/cursor com sort keys estáveis, limite máximo, streaming para janelas grandes, `query_id` e formato único (JSON para controle, Arrow para volume). Testar páginas concorrentes e dados que chegam durante a consulta.

#### M-08 — A árvore de código não cobre entitlements, CLI, backup, scrape ou jobs

**Evidência.** O mapa de capabilities (linha 423) cita `internal/entitlements`, mas a árvore (linhas 381–408) não o inclui. `cmd/observaki` só tem `version`, `serve`, `migrate`; FR-58 exige `observaki query`/`apply`, FR-63 exige backup/restore, FR-64 capacity, e FR-2 scrape. `deploy/` só tem compose/helm; não há jobs de backup/DR/migração.

**Impacto.** Stories e ownership ficam ambíguos; funcionalidades exigidas pelo PRD não têm pacote/entrypoint; a Fase 1 pode “esquecer” backup e scrape porque não estão na árvore.

**Correção.** Alinhar árvore e capability map; adicionar `internal/entitlements`, `internal/promscrape` (ou `cmd/observaki-scrape`), comandos `query`, `apply`, `backup`, `restore`, `doctor`; adicionar `deploy/jobs/` para migração/backup/DR.

---

### 2.4 Baixo

#### B-01 — A tabela de stack mistura produto, versão e faixa

**Evidência.** “NATS JetStream 2.15.1” mistura produto (NATS) com feature (JetStream) e versão; “OpenTelemetry Collector 0.1xx” é uma faixa, não um pin; “ConnectRPC 2.x” é a versão do connect-go, não do protocolo; “Helm 3.18+ (4.x após GA)” está desatualizado; “Kubernetes 1.33+” conflita com o PRD 1.28+.

**Impacto.** Suporte, SBOM e reprodutibilidade ficam ambíguos; a matriz de compatibilidade não é confiável.

**Correção.** Tabela com nome do produto, versão exata, data de release, fim de suporte e fonte; separar “protocolo” de “implementação”.

#### B-02 — Os diagramas contradizem AD-6 e o próprio fluxo de consulta

**Evidência.** Contexto (linhas 181–186): `API --> CH`, `API --> VM`; topologias (linhas 320–322): `API2[API/Query replicas] --> CH2`, `--> VM2`; containers (linhas 226–231): `API --> QS`, mas o contexto diz “API + Query Service”. AD-6 proíbe UI/alertas/CLI de abrir conexão direta, e AD-1 diz que a única ponte é QS/API.

**Impacto.** Implementadores seguem o diagrama e criam o acoplamento que o AD quer proibir; a revisão de arquitetura perde autoridade.

**Correção.** Corrigir os diagramas; adicionar uma regra visual “API nunca conecta em CH/VM”; testes de arquitetura (imports/dependências) para impedir o atalho.

#### B-03 — Premissas e decisões bloqueantes estão espalhadas e sem dono único

**Evidência.** O PRD §11 lista decisões bloqueantes (tenancy, storage, frontend, design partners, retenção default, time/prazo) com deadlines; o addendum §14 lista gates G0–G7; o spine “Deferred” lista replication factor, federação, etc. Não há um único registro de decisão com owner, deadline e critério de aceite; o `.memlog` é um log, não um ADR.

**Impacto.** Decisões se perdem; a Fase 1 começa com premissas não resolvidas; o “final” do spine não significa que os gates foram fechados.

**Correção.** Um `DECISIONS.md`/ADR index com ID, owner, status, deadline, evidência e link; amarrar cada gate a um AD.

#### B-04 — A árvore de código e o mapa de capabilities usam nomes diferentes para o mesmo dono

**Evidência.** Capability map (linhas 412–424) usa `internal/entitlements`, `internal/alerting`, `internal/query`, `internal/ingest`; a árvore (381–408) não tem `entitlements` e tem `internal/api`/`internal/app`. O diagrama de dependência (238–249) mostra `WEB --> API` (web é frontend, API é pacote Go), o que confunde a direção.

**Impacto.** Baixo, mas atrapalha onboarding e code review de arquitetura.

**Correção.** Unificar naming e direção; `web` consome HTTP, não importa `internal/api`.

---

## 3. Pares de unidades um nível abaixo

Esta seção é o teste central: cada par é composto por duas unidades que poderiam ser implementadas por dois times diferentes, cada uma citando os ADs literalmente, e que ainda assim não podem coexistir sem um AD adicional. A coluna “ADs satisfeitos” mostra por que nenhuma das duas pode ser rejeitada com base no spine atual.

### 3.1 Tabela de pares

| ID | Unidade A | Unidade B | ADs literalmente satisfeitos | O que quebra | Achado |
|---|---|---|---|---|---|
| P-01 | `ingest/pipeline.Redactor` (gateway) aplica regras e publica no NATS | `adapters/clickhouse.Writer` aplica “defense in depth” antes do insert | AD-2, AD-3, AD-4, AD-13, AD-15 | Duas versões de redaction; preview mente; `trace_id` pode ser mascarado; hash do `event_id` muda | C-03, A-06 |
| P-02 | `query/cache` com TTL de 5 min, chave `(query, interval, tenant, schema)` | `alerting/scheduler` avalia a cada 15s via QS | AD-6, AD-7, AD-10, FR-30 | Cache velho → alerta atrasado/perdido; cache key sem `authz_epoch` | A-14 |
| P-03 | `adapters/postgres.ProjectRepo` grava `retention_*_days` | `adapters/clickhouse.Schema` usa `TTL ... 15 DAY`; `RetentionReconciler` altera TTL | AD-1, AD-7, AD-9, FR-40 | PG e CH com donos diferentes; project move/delete; TTL fixo vs por projeto | C-04, C-05 |
| P-04 | `ingest/quota` com token bucket local carregado no boot | `api/quota` grava no PG e audita | AD-1, AD-3, AD-7, AD-12, FR-5 | Quota nova não chega sem restart; auditoria diz uma coisa, gateway aplica outra | C-04 |
| P-05 | `query/metrics` roteia para `/select/{tenant}/{project}` no VM | `query/guard` injeta labels `tenant_id`/`project_id` no MetricsQL | AD-3, AD-5, AD-6 | VM sem row policy/auth; path ignorado ou label ausente → vazio ou vazamento | C-05, A-13 |
| P-06 | `adapters/clickhouse` cria usuários/row policies por tenant | `query` usa um único `observaki_reader` e injeta `WHERE tenant_id = ?` | AD-3, AD-6, AD-1 | Row policy + predicado duplicados; usuário único não tem policy; MV/view pode bypassar | C-05, A-13 |
| P-07 | `ingest/otlp` gera UUIDv7 por item | `adapters/nats/consumer` deduplica por `event_id` por 15 min | AD-2, AD-4 | Retry do SDK gera novo ID; redelivery após 15 min; MergeTree não deduplica | C-03 |
| P-08 | `adapters/clickhouse.SpanMetricsMV` grava RED no CH | `ingest/red` grava RED no VM antes do sampling | AD-5 (as duas frases), AD-2, AD-6, FR-20 | Dois REDs; CH vê traces retidos, VM vê todos; retenções diferentes | C-02 |
| P-09 | `api/openapi` gera OpenAPI com `/api/v1` | `api/connect` gera handlers Connect em `/package.Service/Method` | AD-8, AD-9, FR-57 | Dois contratos, dois paths, dois versionamentos; SDKs divergem | A-08 |
| P-10 | `deploy/compose` usa NATS + Postgres | `deploy/helm` usa buffer local + SQLite (ou vice-versa) | AD-4, AD-7, AD-12 | Mesmo binário, semânticas de durabilidade/migração diferentes; RPO diferente | M-01 |
| P-11 | `query/async` guarda resultados no PG | `query/cache` usa Valkey/Redis ou LRU em processo | AD-6, AD-7, AD-1 | Resultado assíncrono no control plane; restart perde; tenant/TTL indefinidos | A-02 |
| P-12 | `alerting/state` grava firing no PG e enfileira | `adapters/notify` envia e dá ack sem outbox | AD-7, AD-10, FR-31, FR-32 | Crash entre estado e fila → sem notificação; entre envio e ack → duplicata | A-14 |
| P-13 | `ingest/gateway` publica por sinal/shard e retorna `partial_success` | `ingest/response` mapeia qualquer falha para 429 | AD-4, FR-1 | Shard 1 ok + shard 2 falha → retry duplica o shard 1; sem atomicidade | C-03 |
| P-14 | `ingest/promscrape` roda no gateway com estado local | `ingest/gateway` é stateless e replica 2+ | AD-1, AD-4, FR-2 | Scrape duplicado em HA; estado perdido no restart; sem leader election | A-09 |
| P-15 | `observability/otel` exporta self-telemetry para o próprio gateway | `observability/prom` expõe `/metrics` para scrape externo | AD-10, AD-1 | Dois caminhos; gateway down cega o painel; sem store/dead-man | A-03 |

### 3.2 Pares detalhados

#### P-03 — Retenção: PostgreSQL manda, ClickHouse decide

- **Unidade A:** o repositório de projetos grava `retention_logs_days` e `retention_traces_days` no PostgreSQL, com auditoria e UI (AD-7; FR-40).
- **Unidade B:** o schema do ClickHouse usa `TTL toDateTime(timestamp) + INTERVAL 15 DAY` (AD-9; addendum 5.1/5.2). Um `RetentionReconciler` no control plane roda `ALTER TABLE ... MODIFY TTL` quando o valor muda.
- **ADs literalmente satisfeitos:** AD-7 (config no PG), AD-9 (migração aditiva; TTL é schema), FR-40 (mudança sem restart).
- **Incompatibilidade:** A e B têm donos diferentes do mesmo dado. Se o reconciliador está no control plane, ele muta o data plane (viola AD-1). Se está no data plane, ele lê o PG (viola AD-1). Se ninguém reconcilia, o PG é decorativo. Se a retenção por projeto usa buckets, o roteamento do gateway precisa saber o bucket antes de publicar; o PG muda, o gateway não. Um `project move` muda o tenant, mas o TTL/bucket continua no tenant antigo.
- **Impacto:** dados apagados cedo (perda) ou retidos além da política (custo/compliance); UI mostra uma retenção que o storage não pratica; o teste de FR-40 passa no PG e falha no CH.
- **Correção:** um único dono da retenção efetiva (serviço de data plane), PG como desired state, snapshot versionado, reconciliação idempotente e teste de convergência; ou TTL por linha com `retention_days` e teste de borda. `project move`/delete entram no mesmo contrato.

#### P-05/P-06 — Tenancy no VictoriaMetrics e no ClickHouse: path, label, row policy e predicado

- **Unidade A:** o QS monta o path `/select/{tenant}/{project}` no VM e assume que o multi-tenancy do VM isola (AD-3, AD-5, AD-6).
- **Unidade B:** o QS injeta `tenant_id`/`project_id` como labels no MetricsQL (AD-3: “injetar o predicado em toda consulta”).
- **Unidade C:** o adaptador do CH cria usuários/row policies por tenant; o QS usa um único usuário `observaki_reader` e injeta `WHERE tenant_id = ?`.
- **ADs literalmente satisfeitos:** AD-3 (row policy + predicado), AD-5 (VM para métricas), AD-6 (QS é o único leitor).
- **Incompatibilidade:** no VM, não há row policy; o path é a única barreira. Se o VM estiver configurado com um tenant default, ou se o path for ignorado, B é a única barreira; mas as métricas OTLP não têm labels `tenant_id`/`project_id` por padrão, então B retorna vazio. Se A e B são aplicados juntos, o path e o label podem não concordar. No CH, se C e o predicado são aplicados juntos, o usuário único não tem row policy (retorna vazio/erro); se o QS usa um usuário por tenant, a gestão de usuários/credenciais vira estado de control plane no data plane. MVs/views com `SQL SECURITY DEFINER` podem não aplicar a row policy do chamador.
- **Impacto:** ou métricas/logs somem (query vazia), ou vazam entre tenants; o teste de isolamento pode passar em um store e falhar no outro; a rotação de credenciais do CH vira um incidente.
- **Correção:** declarar o modo de tenancy do VM (path com proxy autenticado, ou labels injetados no write), injetar labels no gateway/writer se for label-based, e definir um único mecanismo primário no CH (predicado via AST/parâmetro) com row policy como defesa em profundidade usando um contexto de query assinado. Proibir usuários CH por tenant; testar MVs/views, pool de conexões e queries maliciosas.

#### P-07 — `event_id` sintetizado vs dedupe de 15 minutos

- **Unidade A:** o gateway gera UUIDv7 por item OTLP no recebimento (AD-2).
- **Unidade B:** o consumidor NATS deduplica por `event_id` em uma janela de 15 min (AD-4; addendum 3.2).
- **ADs literalmente satisfeitos:** AD-2 (event_id estável), AD-4 (dedupe por event_id).
- **Incompatibilidade:** “estável” não está definido. Se o SDK reenvia o batch após 429, A gera novos IDs; B não vê duplicata. Se o writer fica 16 min fora, o stream expira e o replay perde o evento; se o stream retém, B já esqueceu o ID. O CH `MergeTree` não deduplica; o VM deduplica samples idênticos, mas não eventos. Um replay manual depois de um bug de pipeline duplica tudo.
- **Impacto:** duplicatas visíveis, contagem dupla de RED/quota/custo, alertas errados; a promessa “sem duplicatas visíveis” é falsa em retry e replay.
- **Correção:** `event_id` determinístico (hash canônico) ou chave de idempotência do produtor; dedupe com janela >= retry/outage; `ReplacingMergeTree`/insert dedupe com semântica explícita; teste de retry de SDK, redelivery NATS, crash do writer e replay.

#### P-08 — Span metrics no ClickHouse vs RED no VictoriaMetrics

- **Unidade A:** o adaptador CH mantém `span_metrics_1m` como MV (AD-5: “ClickHouse armazena ... span metrics”).
- **Unidade B:** o gateway calcula RED antes do tail sampling e grava no VM (FR-20; addendum 3.3/5.2).
- **ADs literalmente satisfeitos:** AD-5 (as duas frases), AD-2 (tenant/project), AD-6 (QS roteia por sinal).
- **Incompatibilidade:** A só vê traces retidos e após sampling; B vê todos os spans. A retenção é a de traces (7–15 dias); B é a de métricas (90 dias). Um alerta `signal=metrics` lê B; o service map lê A; os números divergem. O DDL de A está tecnicamente incorreto (M-05), então nem a agregação funciona.
- **Impacto:** RED inconsistente entre dashboards, alertas e service map; decisões de incidente erradas; a meta de desvio <1% não é testável.
- **Correção:** um dono só; declarar RED como métrica no VM (recomendado por FR-20), remover/renomear o MV para analytics não autoritativo, definir schema/retenção e teste de paridade. Se o CH for o dono, remover B e assumir a exceção ao “métricas não vão para CH”.

#### P-10 — Compose e Helm: mesmo binário, semânticas diferentes

- **Unidade A:** o Compose sobe NATS + Postgres e usa o mesmo caminho do cluster.
- **Unidade B:** o Helm single-node usa buffer local + SQLite (AD-4/AD-7 permitem).
- **ADs literalmente satisfeitos:** AD-4 (buffer local no single-node), AD-7 (SQLite no single-node), AD-12 (mesmas imagens/config).
- **Incompatibilidade:** o “mesmo contrato de configuração” não é o mesmo contrato de comportamento. O teste de caos do Compose (NATS/Postgres) não cobre o buffer local/SQLite do Helm; o RPO 0 do cluster não vale no single-node; migrações SQLite/PG divergem; o SQLite não tem row-level security nem as mesmas constraints.
- **Impacto:** “funciona no meu compose”; perda de dados em single-node; upgrade de single-node quebra; dois conjuntos de migração.
- **Correção:** matriz de topologia com RPO/RTO/HA/features; testes de contrato dos adapters; decisão explícita (single-node usa NATS/Postgres ou o cluster aceita buffer local); migrações compartilhadas ou testadas em ambos.

---

## 4. Stack e versões

Verificação em 31/10/2026, com fontes primárias (release notes/documentação). “OK” = versão existe e é suportada; “Ajustar” = existe, mas a política/ pin está errada; “Corrigir” = EOL ou inadequada.

| Componente | Spine | Verificação | Veredito |
|---|---|---|---|
| Go | 1.27.3 | 1.27.3 em 19/10/2026; 1.27 em 19/08/2026 | OK |
| ClickHouse | 26.8 LTS (26.10 stable em benchmark) | 26.8 LTS em 27/08/2026; suporte até 27/08/2027; 26.10.2 stable | OK; incluir upgrade LTS→LTS no AD |
| VictoriaMetrics | 1.148 LTS (1.155 stable) | 1.148.1 em 31/07/2026; linha LTS de 12 meses; 1.155 stable | OK |
| NATS JetStream | 2.15.1 | NATS 2.15 em 17/09/2026; 2.15.2 no Helm | OK; “JetStream” é feature, não versão |
| PostgreSQL | 18.6 | 18.6 em 13/08/2026 | OK |
| SvelteKit | 3.1.1 | 3.1.1 corrente (out/2026) | OK |
| Svelte | 5.57.3 | linha 5.x corrente | OK |
| Vite | 8.x | Vite 8.0 em 12/03/2026; 8.1 corrente | OK |
| Tailwind CSS | 4.3.3 | 4.3.3 no npm | OK |
| uPlot | 1.6.32 | linha 1.6.x | OK |
| CodeMirror | 6.x | 6.x | OK |
| Node.js | 24 LTS | 24 em Maintenance LTS desde 20/10/2026; 26 é Active LTS desde 28/10/2026 | Ajustar para 26 |
| pnpm | 10.x | 10.x | OK |
| Rust (agente) | 1.91+ | 1.91 é antigo; usar corrente | Ajustar o piso |
| OpenTelemetry Collector | 0.1xx (lockfile) | 0.163.0 em 14/10/2026; “0.1xx” é faixa | Corrigir para pin 0.163.x |
| Kubernetes | 1.33+ | 1.33 EOL em 28/06/2026; suportado 1.35–1.37 | Corrigir para 1.35+ |
| Helm | 3.18+ (4.x após GA) | Helm 3: feature até 09/09/2026, patches até 02/2027; Helm 4.3.1 corrente | Corrigir para 4.3+ |
| ConnectRPC | 2.x | connect-go v2 stable (14/10/2026) | OK; resolver path/OpenAPI |
| Docker Compose | 2.40+ | 2.40+ | OK |

**Observações.** As versões do spine não são “inventadas”: as principais existem e estão corretas. O problema é a **política de suporte**: Kubernetes 1.33 e Helm 3.18 não deveriam ser o mínimo de um produto que começa em outubro de 2026; Node 24 já está em manutenção; o Collector não está pinado. O PRD §8.7 declara Kubernetes 1.28+, o que é pior (1.28 está EOL há muito tempo). Alinhar PRD e spine é obrigatório.

**Matriz de compatibilidade que falta.** O PRD §8.7 promete “OpenTelemetry 1.x, PromQL/MetricsQL, Kubernetes 1.28+, Docker, Linux x86_64 e arm64”. O spine não tem uma matriz versão × feature × data de EOL. Para um produto self-hosted, essa matriz é parte do contrato de suporte.

---

## 5. Envelope operacional

O spine descreve topologias, mas não descreve **operação**. A tabela abaixo cobre o que um time de plataforma precisa para rodar o produto e o que o spine não responde.

| Concern | O que o spine diz | Gap | Correção |
|---|---|---|---|
| Ambientes | Nenhum | Não há dev/staging/prod, promoção de config, isolamento de dados | AD de ambientes e config promotion |
| Deploy | Compose e Helm (AD-12) | Sem operator até Fase 3; sem jobs de migração/backup; sem chart values de HA | AD de deploy/upgrade; jobs e hooks |
| Backup/restore | Nada | FR-63 exige; production readiness gate exige | AD de backup/restore de PG/CH/VM/NATS; restore testado |
| DR | Nada | FR-63 Fase 3; RTO <30 min | AD de DR, runbook, drill |
| Capacity | 1M/s de referência | Sem T-shirt sizes, disco/IOPS, requests/limits, HPA | AD de capacity/sizing |
| Secrets | AD-13 (vault/KMS/Secret) | Sem bootstrap, rotação, break-glass, chaves de entitlement | AD de secrets/bootstrap |
| Ingress/TLS | AD-13 (TLS 1.3) | Sem cert-manager, timeouts SSE/WS, CORS, rate limit de edge | AD de ingress/TLS |
| Network | Nada | Sem network policy, egress, store auth | AD de rede/zero trust |
| Store auth | Nada | CH/VM/NATS/PG sem auth/TLS definidos | AD de auth interna (A-13) |
| Upgrade | AD-9 (schema CH) | Sem upgrade de engine, N-1, ordem de migração, rollback | AD de upgrade (A-01) |
| Rollback | FR-62 Fase 3 | Sem mecanismo | AD de rollback e ensaio |
| Observabilidade | AD-10 | Circular, sem store/dead-man | AD de meta-observabilidade (A-03) |
| Custo | FR-43 Fase 2 | Sem metering owner; VM per-tenant é enterprise | AD de metering |
| Air-gapped | FR-55 Fase 3 | Sem bundle/assinatura/upgrade offline | AD de air-gapped |
| Multi-região | Deferred | Sem residência/região | AD futuro |

**Leitura operacional.** O produto promete “operável por um time pequeno” e “< 4h/semana” (PRD 8.5), mas o envelope acima exige um time de plataforma para operar CH+NATS+VM+PG com HA, backup, DR e upgrade. O single-node alivia o começo, mas não cobre o piloto em produção. O production readiness gate (PRD 10.4) é mais rigoroso que o spine; o gate não pode ser cumprido com os ADs atuais.

---

## 6. Ownership de dados

| Dado/Entidade | Store | Escritor | Leitor | Dono do ciclo de vida | Conflito |
|---|---|---|---|---|---|
| Organização/projeto/usuário | PostgreSQL | API | API/QS | Control plane | Sem geração/epoch para move; RBAC incompleto |
| Membership/role | PostgreSQL (não existe no DDL) | API | API/QS | Control plane | ERD tem membership, DDL não; role por projeto ausente |
| Dashboard/panel | PostgreSQL (`definition JSONB`) | API | API | Control plane | ERD tem `PANEL`; DDL só JSONB; versionamento de JSONB ausente |
| Alert rule/channel | PostgreSQL (`channels JSONB`) | API | Alerting | Control plane | ERD tem `NOTIFICATION_CHANNEL`; DDL tem JSONB; dois donos |
| Silence/ack | PostgreSQL (não existe no DDL) | API/alerting | Alerting | Control plane | ERD tem `SILENCE`; DDL não; matcher indefinido |
| SLO/error budget | PostgreSQL + VM (derivado) | API/alerting | API/QS | Ambíguo | ERD tem `ERROR_BUDGET`; addendum diz série derivada; dois donos |
| Token de API | PostgreSQL (não existe no DDL) | API | Gateway/QS | Control plane | Escopo org/projeto indefinido; hash indefinido |
| Auditoria | PostgreSQL (`audit_log`) | API | API | Control plane | Sem imutabilidade/WORM; nome divergente |
| Logs | ClickHouse | Writers | QS | Data plane (?) | Retenção PG vs TTL CH; delete/move |
| Traces | ClickHouse | Writers | QS | Data plane (?) | Retenção PG vs TTL CH; span events/semconv |
| Métricas | VictoriaMetrics | Writers | QS | Data plane (?) | Retenção global; per-tenant enterprise; path vs label |
| Span metrics/RED | ClickHouse **ou** VM | Writers/gateway | QS | Contestado | Dois donos (C-02) |
| `event_id`/dedupe | NATS/CH/VM | Gateway/writer | Writers | Nenhum | Sem chave determinística (C-03) |
| Cache de consulta | Valkey/LRU | QS | QS | Nenhum | Sem invalidação/authz epoch |
| Resultado async | Indefinido | QS | QS/UI | Nenhum | Pode cair no PG (viola AD-1) |
| Estado de alerta | PostgreSQL | Alerting | Alerting/API | Control plane | Sem outbox/idempotência |
| Notificação | Fila (não definida) | Alerting | Notifier | Nenhum | Sem DLQ/entrega |
| Entitlement | Control plane | API | API/QS/VM | Nenhum | Sem enforcement point |
| Self-observabilidade | Indefinido | Todos | Health panel | Nenhum | Circular (A-03) |

**Regra que falta.** O spine deveria ter uma frase por dado: “X é owned by Y; only Y writes; Z reads via W; lifecycle is L; move/delete is M”. Hoje, a regra existe para telemetria (AD-2) e para config (AD-7), mas não para os casos de borda que causam incidentes: dedupe, cache, async results, alert state, entitlements e self-observability.

---

## 7. Contratos OTLP, PromQL e OpenAPI

### 7.1 OTLP

| Requisito | O que o spine diz | Gap |
|---|---|---|
| OTLP gRPC/HTTP, protobuf/JSON, gzip/zstd | AD-4, AD-9, FR-1 | zstd é opcional no OTLP; o fallback precisa ser testado por SDK |
| `partial_success` | FR-1, AD-4 | Mapeamento por sinal (metrics/logs/traces) e mistura com 429/5xx não definido |
| Identidade `event_id` | AD-2, AD-4 | OTLP não tem `event_id`; síntese e estabilidade não definidas |
| Atributos preservados | FR-1 | `Map(String,String)` perde tipos (A-06) |
| Semconv | AD-9 “OTLP 1.x” | Sem pin; mudanças de nomes quebram filtros/dashboards |
| Span events | FR-18, DDL `events Nested` | OTel deprecia Span Events em 2026; schema pode ficar obsoleto |
| Project routing | AD-2/AD-3 | Token resolve tenant/project; resource attribute não pode sobrescrever; não está escrito |
| Remote write | FR-2 | Versão 1.0/2.0, headers, tenant mapping e auth não definidos |

### 7.2 PromQL/MetricsQL

| Requisito | O que o spine diz | Gap |
|---|---|---|
| Subset documentado | AD-16 | Não existe lista; “subset” é uma promessa vazia |
| Conformidade | FR-7, AD-16 | VM não é 100% PromQL; matriz por versão ausente |
| Histogramas OTel | FR-8 | Conversão para explícito, `histogram_quantile`, exponential histograms; sem mapeamento testável |
| Exemplars | FR-8 | VM suporta? O caminho exemplar→trace não tem schema |
| Rollups | FR-11 | 1m/5m no VM OSS; downsampling nativo é enterprise; estratégia não decidida |
| Alertas | FR-30 | Query de alerta em MetricsQL; sem validação/AST; cache do QS pode atrasar |

### 7.3 OpenAPI/ConnectRPC

| Requisito | O que o spine diz | Gap |
|---|---|---|
| API `/api/v1` | AD-9 | ConnectRPC usa `/package.Service/Method`; conflito de path |
| OpenAPI validado no CI | FR-57 | Fonte de verdade (proto vs OpenAPI) não definida |
| REST + gRPC | FR-57 | ConnectRPC HTTP/JSON não é REST resource-oriented; dois modelos |
| Erros | AD-15 | Sem mapeamento gRPC/HTTP, `retryable`, catálogo |
| Versionamento | AD-9 | Sem regra para proto, JSON, CLI, dashboards JSONB |
| Streaming | FR-13 | SSE/WS não está no contrato OpenAPI/Connect |

**Testes de contrato.** O spine fala em “suíte de contrato OTLP/PromQL/OpenAPI” (AD-16, convenções), mas não define fixtures, versões, ambientes ou critérios de aprovação. Sem isso, os ADs 9 e 16 são aspiracionais.

---

## 8. Decision-readiness dos ADs

| AD | Decision-ready? | Bloqueio | O que falta |
|---|---|---|---|
| AD-1 | Não | Control plane no caminho de consulta; config plane | Definir exceções e autoridade |
| AD-2 | Não | `event_id` sintético; métricas; tenant authority | Chave determinística; escopo |
| AD-3 | Não | CH row policy vs predicado; VM sem policy; MVs | Mecanismo único + defesa em profundidade |
| AD-4 | Não | Ack vs async insert; partial publish; dedupe | Contrato de durabilidade e idempotência |
| AD-5 | Não | Span metrics no CH vs “métricas não vão para CH” | Um dono para RED |
| AD-6 | Não | Authz duplicada; streaming; async; cache | Contrato de query |
| AD-7 | Não | Config distribution; SQLite parity; audit | Config plane + schema canônico |
| AD-8 | Parcial | Connect/OpenAPI; embed | Fonte de verdade e build |
| AD-9 | Não | Semconv; PromQL subset; engine upgrade; N-1 | Contrato de upgrade e compatibilidade |
| AD-10 | Parcial | Bootstrap; store; dead-man | Meta-observabilidade |
| AD-11 | Não | Enforcement point; offline; boundary | AD de entitlements |
| AD-12 | Parcial | Topology matrix; SQLite/WAL | Matriz de capacidades |
| AD-13 | Parcial | Token hash; sessão; internal TLS; store auth | AD de auth completa |
| AD-14 | Não | Método de benchmark; dataset; SLO gates | Contrato de performance |
| AD-15 | Parcial | RFC3339 ns; transporte; PII | Catálogo e mapeamento |
| AD-16 | Não | Suítes não definidas; subset | Artefatos e CI |

**Leitura.** “Decision-ready” aqui significa: um time consegue implementar duas unidades independentes e elas se encaixam sem uma decisão adicional. Hoje, 11 dos 16 ADs falham nesse teste e 5 são parciais; nenhum está totalmente suficiente — AD-8 e AD-15 são os mais próximos, e mesmo assim com ressalvas.

---

## 9. Correções mínimas antes da Fase 1

1. **AD-17 — Query/authz plane.** Uma autoridade de authz, contexto assinado, comportamento sem PG/IdP, QS sem confiar em headers, API/QS separados no caminho de leitura. Corrigir diagramas.
2. **AD-18 — Idempotência e durabilidade.** Chave determinística, dedupe, semântica CH/VM, `async_insert` com wait, retenção NATS > outage, DLQ, replay. Definir RPO por topologia.
3. **AD-19 — Config plane.** PG como desired state, snapshot versionado/assinado, staleness, fail-closed, reconciliação única de retenção/quota/pipeline/entitlement.
4. **AD-20 — Tenancy.** `project_id` imutável, `project_generation`/`authz_epoch`, move/delete, cache key, VM path/labels, CH row policy + predicado, testes de isolamento nos dois stores.
5. **AD-21 — Span metrics/RED.** Um dono, um store, schema, retenção, paridade <1%. Remover o caminho perdedor.
6. **AD-22 — Contratos internos e upgrade.** Envelope gateway↔NATS↔writer, N-1, ordem de migração, upgrade de engine (CH 26.8/VM/PG/NATS), rollback.
7. **AD-23 — Envelope operacional.** Ambientes, backup/restore, DR, capacity, secrets, ingress/TLS, network policy, store auth, PDB/anti-affinity.
8. **AD-24 — Meta-observabilidade.** Caminho separado, store, quota reservada, dead-man's switch, observer effect.
9. **AD-25 — Contratos externos.** OTLP types/semconv/span events, PromQL subset + matriz, OpenAPI vs Connect, Prometheus scrape/remote write.
10. **Correções de stack/documentação.** Kubernetes 1.35+, Helm 4.3+, Node 26, Collector 0.163.x; alinhar PRD §8.7; corrigir a aritmética de sizing/custo; alinhar ERD/DDL.

---

## 10. O que está bom

- **Separação de planos.** A intenção de manter o control plane fora do hot path e o data plane independente de PostgreSQL é correta; o problema é que o spine não a implementa de forma consistente.
- **OTLP-first e ausência de SDK proprietário.** É a decisão certa para adoção e para não repetir o erro de agentes fechados.
- **ClickHouse + VictoriaMetrics.** O split por forma de acesso é defensável; métricas em TSDB e eventos em colunar continua sendo o trade-off mais pragmático para o MVP.
- **Ports & adapters.** A direção de dependência (`domain` não importa `adapters`/`api`/`cmd`) é boa e testável.
- **Open-core sem paywall no caminho de telemetria.** O princípio é correto; falta o enforcement.
- **Performance como orçamento.** A ideia de benchmark no release gate é correta; falta o contrato de medição.
- **Single-node + cluster com as mesmas imagens.** É a direção certa para “funciona no meu compose”; falta a matriz de comportamento.
- **Consistência conventions.** Naming, IDs, timestamps e erros estão bem direcionados; faltam detalhes de transporte e interoperabilidade.

---

## 11. Apêndice — checklist de verificação

| Pergunta | Resposta |
|---|---|
| Dois times conseguem implementar unidades independentes sem decisão extra? | Não, em 11/16 ADs |
| O control plane está fora do caminho de consulta? | Não (C-01) |
| Todo dado de telemetria tem um único dono? | Não (span metrics, dedupe, cache, async results) |
| O ack do gateway implica durabilidade? | Não (C-03) |
| Duplicatas são realmente invisíveis? | Não (C-03) |
| Retenção por projeto é aplicável sem violar AD-1/AD-7? | Não (C-04) |
| `project move` preserva isolamento e dados? | Não (C-05) |
| VictoriaMetrics tem isolamento em profundidade? | Não (P-05) |
| ClickHouse row policy cobre MVs/views? | Não testado (P-06) |
| Upgrade rolling tem N-1? | Não (A-01) |
| Backup/restore/DR existem? | Não (A-04) |
| O health panel sobrevive à queda do pipeline? | Não (A-03) |
| OTLP preserva tipos e semconv? | Não (A-06) |
| PromQL subset está definido? | Não (A-07) |
| OpenAPI é a fonte de verdade? | Não definido (A-08) |
| Kubernetes/Helm/Node/Collector estão em linha suportada? | Não (A-05) |
| O sizing de 30 dias está correto? | Não, 2x (A-11) |
| O production readiness gate é cumprível? | Não com os ADs atuais (A-04) |

---

## 12. Fontes de verificação de versões (31/10/2026)

- Go: `go.dev/doc/devel/release` — go1.27.3 em 19/10/2026.
- ClickHouse: release 26.8 LTS em 27/08/2026; 26.10.2 stable; EOL da linha 26.8 em 27/08/2027.
- VictoriaMetrics: 1.148.1 em 31/07/2026; linha 1.148 LTS; 1.155 stable.
- PostgreSQL: 18.6 em 13/08/2026.
- NATS: 2.15 em 17/09/2026; 2.15.2 no Helm.
- SvelteKit: 3.1.1 no npm (out/2026); SvelteKit 3.0 em 01/10/2026.
- Vite: 8.0 em 12/03/2026; 8.1 corrente.
- Tailwind CSS: 4.3.3 no npm.
- ConnectRPC: connect-go v2 stable em 14/10/2026.
- Kubernetes: 1.37.1 em 15/09/2026; linha suportada 1.35–1.37; 1.33 EOL em 28/06/2026.
- Helm: Helm 4.0 em 11/2025; 4.3.1 em 15/10/2026; Helm 3 feature até 09/09/2026, patches até 02/2027.
- Node.js: 24 em Maintenance LTS desde 20/10/2026; 26 Active LTS desde 28/10/2026.
- OpenTelemetry Collector: 0.163.0 em 14/10/2026.
- OpenTelemetry: anúncio de depreciação da Span Events API em 2026; semconv HTTP com nomes estáveis/breaking changes documentados.
- Prometheus: repositório `prometheus/compliance` e suíte de conformidade PromQL; resultados públicos mostram lacunas no VictoriaMetrics.
