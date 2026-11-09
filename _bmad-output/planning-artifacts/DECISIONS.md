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
| P-03 | Fronteira open-core exata (princípios e candidatos em [`docs/governance/open-core.md`](../../docs/governance/open-core.md)) | Fundador | Antes da Fase 1 | Se a comunidade reagir mal, manter segurança básica e observabilidade no OSS |
| P-04 | Frontend: SvelteKit 3 vs React | Fundador + Frontend | Semana 1 | Se o time for React-heavy, React 19 + uPlot |
| P-05 | Compliance: SOC 2/HIPAA/FedRAMP | Fundador | Antes da Fase 3 | Se um contrato exigir, antecipar trilha |
| P-06 | Time/prazo: 5-6 engenheiros, ~14 meses | Fundador | Semana 1 | Se o time for menor, cortar Fase 4 e/ou adiar GA |
| P-07 | Aprovar provedor, teto de gasto e credencial IAM dedicada do cluster T1 (AWS de referência: USD 41,79/h e USD 30.504,49/mês no perfil de 7 dias; ver `BENCHMARKS.md`) | Fundador | Antes do baseline T1 | Se o custo estourar o orçamento, reduzir retenção/discos ou trocar de provedor e atualizar a tabela de preços; o `apply` fica bloqueado até a aprovação |

**ADRs relacionadas:** P-02 é detalhada em [ADR-0005](../../docs/adr/0005-tenancy-e-retencao.md) (mecanismo aprovado; o default 7/15/30 continua pendente); P-04 em [ADR-0004](../../docs/adr/0004-frontend-sveltekit-uplot.md) (SvelteKit é o default; o fallback React 19 permanece como critério de reversão se o time contratado for React-heavy); P-03 em [`docs/governance/open-core.md`](../../docs/governance/open-core.md) (fronteira ainda pendente); P-07 em [ADR-0006](../../docs/adr/0006-metodologia-de-benchmark.md) (perfil T1 e mecanismo de custo entregues pela Story 2.4; a aprovação de gasto continua humana).

**P-01 (2026-11-08):** o dono declarou que **não há pessoas disponíveis para
testar** nesta fase, então o critério de reversão de P-01 passa a valer: o
programa de design partners roda com **dados sintéticos** e o gate de produto do
G1 ("3 design partners com dados reais") fica **adiado**, sem ser marcado como
cumprido. A resposta de engenharia é o harness [`partners-simulate`](../../docs/partners/simulation.md)
(`cmd/partnerssim`), que gera um funil determinístico, roda o validador, a
triagem e o gate reais e grava tudo em `partners/simulation/` (fora do git).
`partnerscheck` **recusa linhas simuladas** sem `-simulation`, de modo que uma
coorte sintética não pode ser commitada como evidência de AC1/AC3. A meta real
(20 candidatos, 5 calls e 3 assinaturas) continua aberta até existir sourcing
humano.

**Story 2.4 (2026-11-01):** o perfil T1 está em `bench/env/t1.json` e a
estimativa de custo em `bench/internal/envprofile` + `bench/env/prices.*.json`.
`BENCHMARKS.md` publica a pré-baseline local e mantém o baseline T1 pendente. O
`apply` do cluster é bloqueado sem `OBSERVAKI_BENCH_ALLOW_SPEND=1` e recusa
credencial root da AWS; falta a decisão P-07.

**Story 3.1 (2026-11-02):** o schema de logs foi validado localmente com 1B
linhas (`bench/clickhouse/results/2026-11-02-local-1b/`): 6,98x de compressão,
`q03` p95 263 ms e TTL 7/15 ok. A estratégia escolhida é TTL por linha
(`retention_days`) em partição diária por `ingested_at`; o fallback de tabelas
por bucket continua para a Story 3.5. O run local parou merges de background
para caber no disco e **não é evidência do G1**. A Q-02 do ADR-0005 continua
aberta para reconciliação por projeto, row policies e isolamento.

**Story 3.2 (2026-11-02):** o schema de traces foi validado localmente com 1B
spans (`bench/clickhouse/traces/results/2026-11-02-local-1b/`): 7,52x de
compressão, trace lookup p95 328 ms, serviço/status/24h p95 178 ms, service map
9 nós/9 arestas e correlação log↔trace nos dois sentidos. A ordem
`tenant/projeto/serviço/status/tempo` e os bloom filters de trace/span/parent
foram escolhidos para as consultas do spike; o service map do spike é derivado
dos spans brutos com amostra de 1% das traces, e a fonte de produção continua
sendo o RED do gateway no VictoriaMetrics (AD-21). O run local parou merges de
background e **não é evidência do G1**. A Q-02 do ADR-0005 continua aberta para
reconciliação por projeto, row policies e isolamento.

**Story 3.3 (2026-11-02):** o caminho de ingestão ClickHouse foi validado
localmente (`bench/clickhouse/ingest/results/2026-11-02-local-400k/`): batches
RowBinary de 200k, spool local com `fsync`/checkpoint, dedupe por `event_id` e
`insert_deduplication_token` estável, 400k/s por 60 s, lag p95 1,4 s, 24M
eventos, 18 parts e zero `Too many parts`. O dedupe de um batch repetido e a
recuperação após crash (insert antes do checkpoint) passaram. O buffer local
durável é o modo single-node permitido pelo AD-4; o NATS é a Story 4.3. A
janela de dedupe em memória não sobrevive ao restart; o índice durável para
duplicatas de produtor entre restarts fica para a Fase 1 (FR-4). O run local
satura em ~400k/s e **não é evidência do G1**; 1M/s por 1 h no T1 depende de
P-07 e da Story 4.4.

**Story 3.5 (2026-11-02):** a consulta, a retenção e o isolamento foram
validados localmente (`bench/clickhouse/results/2026-11-02-local-1b-mix/`):
16 consultas ClickHouse no dataset de 1B eventos (300M logs + 100M spans) com
p95 máximo 432 ms, 4 consultas MetricsQL com p95 799,5 ms, row policies por
tenant com `project_generation`/`authz_epoch` (15 testes de isolamento e epoch
pass), TTL 7/15/30 e mutação de política em 10M linhas (~0,49 s). O tier de
10B/7d é uma projeção window-equivalent (617 ms) e o de 86B/24h foi medido em
rollups de 1 minuto (9 consultas, p95 máximo 87 ms). **Decisão:** TTL por linha
(`retention_days`) no MVP, com o fallback de tabelas por bucket para o T1; Q-02
do ADR-0005 fechada. O raw 10B/86B e o Query Service continuam pendentes de
P-07/Stories 4.1–4.5; o run **não é evidência do G1**.

**Story 4.2 (2026-11-03):** o gateway passou a exigir um snapshot versionado de
auth/quotas (`OBSERVAKI_INGEST_AUTH_FILE`), com tokens SHA-256, escopos,
expiração, `config_version`/`generation` e quota por tenant. Tokens ausentes ou
inválidos retornam 401; tentativa de escrita cross-project retorna 403 e gera
audit com hash chain; excesso de quota retorna 429 com `Retry-After`. O caminho
HTTP reserva uma estimativa de eventos pelo `Content-Length` (EWMA por tenant)
antes de descomprimir/decodificar e reconcilia após o decode, para derrubar o
tenant barulhento antes do trabalho caro. **Decisão:** o snapshot local
(arquivo) é o transporte da Fase 0; a assinatura Ed25519 destacada já é
verificada, mas o reconciliador/rotação/JWKS é da Fase 1. **Evidência local:**
401/403/429 e audit verdes; 26.703 requests limitados; p95 de processamento do
tenant quieto +15,13% em 1M/s ofertado. O isolamento **<10% não foi provado**
localmente porque gateway e load generators dividem o host; a medição fica para
o T1, bloqueado por P-07 e Stories 4.3–4.5. Q-03 do ADR-0002 fica parcialmente
respondida (sem PostgreSQL no hot path), com o cache JWKS ainda pendente.

**Story 4.3 (2026-11-03):** o gateway passou a publicar no WAL NATS JetStream
com envelope protobuf v1 (`observaki.ingest.v1`), zstd e batching por subject.
O subject carrega tenant/projeto em base64url, `authz_epoch`, sinal e shard
(`observaki.ingest.v1.<tenant>.<project>.e<epoch>.<signal>.<shard>`), com 32
shards por default; streams por sinal usam `WorkQueuePolicy`/`DiscardNew` (desde
a Story 4.5), `MaxAge` de 24 h e `MaxBytes` de 16 GiB por stream (default
ajustado na 4.5 para caber em disco de 100 GiB; o T1 configura o sizing). O ack
OTLP só acontece depois do `PublishMsg` confirmado; o
retry usa o mesmo `Nats-Msg-Id`, e a dedupe de retry do SDK fica no `event_id`
determinístico (hash canônico pós-redaction) para o writer da 4.4. Fila cheia
retorna 429 `wal_backpressure`; NATS fora retorna 503 e `/readyz` 503.
**Decisão:** NATS é o WAL em todos os ambientes; `DiscardSink` fica apenas como
fixture de teste. O single-node do Compose pode usar NATS local (Q-03 do
ADR-0003 continua aberta para o fallback de buffer local). **Evidência local:**
100k/s por 10 s com 1M eventos, 5,46x de compressão, 32 shards, zero duplicatas
e p99 de processamento 28,9 ms; 1M/s com payload mínimo sustentou ~740k/s e 5M
eventos íntegros. **Não é G1:** 1M/s por 1 h, retenção de 15 min e o caos de
NATS/ClickHouse continuam pendentes do T1/4.5 (P-07).

**Story 4.4 (2026-11-04):** o writer (`observaki writer`) consome os três
streams com consumidores duráveis `AckExplicit`, mescla requests por
tenant/projeto/geração/epoch/sinal e só confirma a mensagem do NATS depois do
`OnDone` do writer ClickHouse (insert confirmado ou DLQ) ou do HTTP 2xx do
VictoriaMetrics. O dedupe em memória usa
`tenant|project|geração|authz_epoch|event_id`; o
`insert_deduplication_token` do ClickHouse é derivado dos `event_id`, não da
sequência local, então um replay idêntico após restart é absorvido pelo bloco.
O INSERT usa lista explícita de 17 colunas, incluindo
`project_generation`/`authz_epoch` (AD-20). Erros permanentes vão para
`OBSERVAKI_DLQ`; transitórios usam backoff exponencial de 500 ms a 30 s e
mantêm a mensagem não confirmada no WAL. **Evidência local:** 20k/s por 20 s
com 400k eventos, lag p95 0,80 s, contagens exatas em ClickHouse/VictoriaMetrics
e zero falhas; caos de 3 s no ClickHouse e 2 s no VictoriaMetrics com 8 retries
e zero perda. **Não é G1:** 1M/s por 1 h, outage de 15 min e caos de
NATS/gateway são 4.5 + T1 (P-07).

**Story 4.5 (2026-11-04):** o caos local rodou ClickHouse fora por 15 min,
restart de NATS e `SIGKILL` do gateway sob carga contínua. O ack só depois do
WAL e a contabilidade do writer se mantiveram: zero perda para eventos
confirmados, contagens exatas e `uniqExact(event_id)` sem duplicata no delta,
com 429 `wal_backpressure` quando o WAL encheu. **Achado de design:** streams
de WAL com `LimitsPolicy` mantêm mensagens já confirmadas até `MaxAge`/`MaxBytes`;
com `DiscardNew`, um stream cheio rejeitava toda publicação nova mesmo depois
de o ClickHouse voltar. A correção é retenção **WorkQueue** nos streams de
sinal (o ack do consumidor durável remove a mensagem) e `DiscardNew` apenas
para backpressure; o DLQ continua em `LimitsPolicy`. O run de capacidade prova
429 e a retomada após o dreno. **Recomendação AC3:** Go permanece no hot path
do MVP; OTel Collector só na borda; fallback Rust/Rotel se o T1 falhar
15k eventos/s por vCPU ou p95/lag (detalhes em
`docs/dev/story-4.5-hot-path-recommendation.md`). **Não é G1:** 1M/s por 1 h,
R3 do NATS e o sizing do WAL no T1 continuam pendentes (P-07).

**Story 5.1 (2026-11-04):** a stack single-node sobe com `docker compose up`
(raiz do repositório) e usa uma imagem multi-stage que compila o SvelteKit e o
binário Go do zero. Decisões de empacotamento: portas publicadas só em
`127.0.0.1`; healthcheck dos serviços Observaki via `observaki healthcheck`
(imagem distroless não tem shell/curl); a API Go passou a servir
`OBSERVAKI_WEB_DIR` com fallback de SPA e 404 JSON em `/api/*`; o snapshot de
auth de desenvolvimento é público (`tenant-dev`/`project-dev`, token
`observaki-local-dev-token`) e vale só para avaliação local (AD-19 continua
exigindo snapshot assinado em staging/produção); as migrações do ClickHouse
rodam em um serviço one-shot idempotente a cada `up`, então `down`/`up` não
exige passo manual; volumes nomeados guardam ClickHouse, VictoriaMetrics,
PostgreSQL, NATS e o spool do writer. **Evidência local (Linux):** 28 s do
`up --build` sem imagem/volumes até todos os healthchecks verdes; 2 logs, 2
spans e a métrica de amostra gravados; `down`/`up` preservou as linhas e o
`clickhouse-migrate` reaplicou o schema (relatório em
`deploy/compose/results/`). **Pendências:** validação em macOS, Helm 3 nós
(5.2), sample app/quickstart (5.3) e Explore na UI (6.1–6.4); não é G1.

**Story 6.2 (2026-11-08):** o painel de série temporal (`TimeSeriesChart`)
usa uPlot 1.6.32 carregado por `import()` dinâmico (fora do bundle inicial) e
desenha com **decimação min/max** por bucket de pixel (~1 ponto por pixel CSS),
mantendo os 100k pontos em memória e o path sempre cobrindo o início e o fim da
janela visível. O dataset sintético é determinístico e a janela é fixa
(`copyWithin`), então o live a 1 Hz não realoca os arrays. A tabela acessível e
o anúncio em `aria-live` são adiados 120 ms depois do gesto para não roubar
frames da interação. O preenchimento de área saiu do spike porque, sem GPU, o
fill translúcido custava ~6 ms por frame e derrubava o zoom para ~49 fps; a
linha mantém 60 fps. **Evidência local (Linux, Chrome 155 headless):** primeiro
frame com 100k pontos em 11,4 ms, mediana de 60,1 fps em 9 rodadas de
hover/zoom/pan, live com uma instância e o mesmo `<canvas>`, heap de 7–18 MiB
em 10 min; bench em `web/scripts/chart-bench.mjs` e resultados em
`bench/frontend/`. **Pendências:** medição com GPU, Query Service real (Fase 1)
e orçamento de bundle/Lighthouse na 6.4.

**Story 6.3 (2026-11-08):** a tabela do Explore (`VirtualLogTable`) usa
virtualização por altura fixa (40 px, janela visível + 6 de overscan) e mantém
**19 linhas no DOM** no topo (máximo 25 durante o scroll) para 10.000 linhas,
com cabeçalho sticky e linhas espaçadoras preservando a altura total. A tabela
continua nativa (`table`/`thead`/`tbody`), com `aria-sort`, `aria-current`,
`caption` e uma região `aria-live`; scroll, sort, resize, seleção e estados são
mediados pelo bench CDP (`web/scripts/table-bench.mjs`, `make table-bench`).
**Evidência local (Linux, Chrome 155 headless):** **10.000 linhas**, 19 no DOM
no topo e no máximo 25 no scroll; scroll com wheel real a **60,2–60,7 fps**
(mediana 60,5, pior frame 16,8 ms); sort `duration:asc/desc` conferido nos
dados; resize da coluna Service por teclado (150→182 px) e arraste (182→230 px)
preservado após seleção; painel de detalhe com `Esc` mantendo a seleção, `j`
movendo e `Enter` reabrindo; vazio/erro com retry voltando às 10k linhas; CLS
de carga **0**. O CLS acumulado da sessão sintética (0,216) vem quase todo da
troca de linhas durante o scroll da lista virtualizada; o Lighthouse da rota
entra na 6.4. **Pendências:** Query Service real (Fase 1), painel empilhado em
telas <1024 px e orçamento de bundle/Lighthouse na 6.4.

**Story 6.4 (2026-11-09):** o orçamento de frontend passou a ser um gate de
build: `web/scripts/bundle-budget.mjs` roda no fim de `make build` e falha com
exit 1 quando o JS gzip passa de **180 KB na entrada** (`index.html`: runtime +
layout raiz + rota de entrada) ou de **250 KB na rota de dashboards** (closure
de todos os chunks alcançáveis, incluindo o `uPlot` importado dinamicamente).
Medição local: **41,5 KB** na entrada e **74,4 KB** em dashboards. O Lighthouse
(`web/scripts/lighthouse-check.mjs`, `make lighthouse`) mede a rota `/explore`
com perfil desktop de 50 Mbps / 40 ms RTT e 1x CPU: LCP vem de uma navigation
run simulada e INP de uma **timespan run** (o INP só é coletado em fluxo sem
simulação), dirigindo troca de abas, autocomplete e filtros de verdade.
Medição local: **LCP 390 ms** (< 1.500 ms) e **INP 81 ms** (< 200 ms).
`lighthouse@13.6.0` e `puppeteer-core@25.16.0` entram só como
`devDependencies` (ferramenta de CI, nada disso vai para o bundle); o critério
de reversão é o Lighthouse ficar instável no runner, quando o bench CDP próprio
(como em 6.2/6.3) passa a medir os mesmos dois números. **Pendências:** dados
de campo/CrUX, Query Service real e waterfall de traces (Fase 1).

## ADRs da Fase 0

As seis ADRs obrigatórias (AR-12) estão em [`docs/adr/`](../../docs/adr/README.md). Foram **aprovadas em 2026-10-31** (AC3); a evidência dos spikes continua sendo exigida nos demais itens do Gate G1.

| ADR | Tema | Status | Spike | Invariantes |
|---|---|---|---|---|
| [0001](../../docs/adr/0001-storage-clickhouse-victoriametrics.md) | Storage (ClickHouse + VictoriaMetrics) | `aceito` | S0.1, S0.4, S0.8 | AD-5, AD-9, AD-21, AD-29 |
| [0002](../../docs/adr/0002-gateway-go.md) | Gateway (Go) | `aceito` | S0.2, S0.7 | AD-1, AD-4, AD-6, AD-17, AD-21 |
| [0003](../../docs/adr/0003-fila-nats-jetstream.md) | Fila/WAL (NATS JetStream) | `aceito` | S0.3 | AD-4, AD-18, AD-19, AD-22 |
| [0004](../../docs/adr/0004-frontend-sveltekit-uplot.md) | Frontend (SvelteKit + uPlot) | `aceito` | S0.5 | AD-8, AD-14, AD-34 |
| [0005](../../docs/adr/0005-tenancy-e-retencao.md) | Tenancy e retenção | `aceito` | S0.8, S0.4 | AD-2, AD-3, AD-20, AD-28, AD-29, AD-35 |
| [0006](../../docs/adr/0006-metodologia-de-benchmark.md) | Metodologia de benchmark | `aceito` | S0.1–S0.6 | AD-14, AD-16, AD-29 |

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
