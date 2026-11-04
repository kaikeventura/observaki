# Observaki — Benchmarks

> **Status (2026-11-04):** pré-baseline local do harness, spikes locais de
> armazenamento de logs (1B linhas) e traces (1B spans), o caminho de ingestão
> ClickHouse (400k/s, 60 s), o spike de VictoriaMetrics (1M séries, 1M/s, 1 h)
> e o pipeline com WAL/gateway/writers (20k/s + caos) publicados; baseline T1
> **não executado**. Nenhum número deste arquivo é evidência do Gate G1 até a
> execução no cluster T1 com gateway, armazenamento e o commit registrados.

Este arquivo segue o ADR-0006: todo número publicado declara commit, hardware,
configuração, seed, duração e veredito; falhas e limitações ficam visíveis; e
benchmark de fornecedor não entra sem reprodução própria.

## Resumo

| Item | Situação |
|---|---|
| Pré-baseline local do harness (100k/s, 3 × 60 s) | ✅ publicada; mecanismo de geração, transporte, coleta e relatório validado |
| Schema ClickHouse de logs (1B linhas, local) | ✅ publicado; 6,98x, `q03` p95 263 ms e TTL 7/15 ok; **não é G1** |
| Schema ClickHouse de traces (1B spans, local) | ✅ publicado; 7,52x, trace lookup p95 328 ms, serviço/status p95 178 ms e correlação ok; **não é G1** |
| Ingestão ClickHouse + dedupe (400k/s, 60 s, local) | ✅ publicado; 399.994/s, lag p95 1,4 s, 24M linhas, dedupe e recuperação ok; **não é G1** |
| VictoriaMetrics + cardinalidade (1M séries, 1M/s, 1 h, local) | ✅ publicado; 999.738/s, p95 de carga 13 ms, 20 consultas p95 799,5 ms, drift do vizinho 4,82%; **não é G1** |
| Consulta, retenção e row policies (1B eventos, local) | ✅ publicado; 16 consultas p95 ≤ 432 ms, 4 MetricsQL p95 799,5 ms, 7/15/30 TTL, isolamento e 86B rollup p95 ≤ 87 ms; **não é G1** |
| Gateway: auth, quota e isolamento (local, 1M/s ofertado) | ⚠️ funcional pass (26.703 requests 429, 401/403/audit); p95 local +15,13% (inconclusivo por host compartilhado); T1 pendente |
| Gateway: batching, zstd, sharding e WAL NATS (local, 100k/s) | ✅ publicado; 1M eventos, 5,46x, p99 28,9 ms, 32 shards, 0 duplicatas; **não é G1** |
| Writers ClickHouse/VM sobre o WAL (local, 20k/s + caos) | ✅ publicado; 400k eventos, lag p95 0,80 s, contagens exatas, 0 falhas; caos com 8 retries e zero perda; **não é G1** |
| Baseline T1 (1M/s, 1 h, 6 ClickHouse + 3 VictoriaMetrics + 6 gateways) | ⛔ bloqueado: provedor/orçamento (P-07) e Story 4.5 |
| Perfil T1 e capacidade | ✅ definidos e validados (`bench/env/t1.json`) |
| Provisionamento idempotente + destroy + custo | ✅ implementado e validado sem `apply`; execução bloqueada por gasto |
| Gate G1 | ⛔ aberto |

## Pré-baseline local do harness

**O que é:** três execuções de 60 s do `throughput-100k` contra um receptor
OTLP/HTTP local que apenas drena o corpo e devolve 200. Mede o gerador, o
transporte HTTP/gzip, a coleta de recursos e o gerador de relatório. **Não é o
gateway do produto**, não mede ingestão durável, lag, perda, armazenamento ou
consulta. É a evidência de que o mecanismo da Story 2.3 funciona de ponta a ponta
e de que o protocolo da Story 2.4 é executável.

### Identificação

| Campo | Valor |
|---|---|
| Commit do harness | `5b6096d` (limpo, sem alterações não commitadas) |
| Data (UTC) | 2026-11-02T01:49:08Z a 2026-11-02T01:52:08Z |
| Cenário | `throughput-100k`, 100.000 eventos/s, HTTP, gzip |
| Duração | 3 × 60 s (60,08 / 60,11 / 60,10 s) |
| Workload | 60% métricas, 30% logs, 10% traces; 134 B/evento comprimidos |
| Seeds | 1 (um shard) |
| Config hash | `sha256:f6b7069c2aabbb4011b1e06efa1011f2b1e6edf41d6f65a61d31cb2ab59d1978` |
| Endpoint | `http://127.0.0.1:4318` (bench-sink local) |
| Artefatos | [`bench/baselines/2026-11-01-local-100k/report.md`](bench/baselines/2026-11-01-local-100k/report.md) e [`report.json`](bench/baselines/2026-11-01-local-100k/report.json) |

### Hardware

| Campo | Valor |
|---|---|
| Host | `kabuntu-zenbook` (máquina de desenvolvimento, não é o T1) |
| CPU | Intel Core Ultra 7 155H, 22 CPUs lógicas |
| RAM | 15,0 GiB (16.092.491.776 B) |
| Disco | 441,1 GiB no total; 147,0 GiB livres no início/fim |
| Kernel | `7.0.0-38-generic` (linux/amd64) |
| Go | go1.27.2 |
| Binários | `benchctl`/`otlploadgen` `v0.0.1-alpha-17-g5b6096d` |

O receptor local roda na mesma máquina; CPU e RAM do host incluem o sink e
qualquer carga do desktop. O CPU/RSS do processo gerador é reportado à parte.

### Resultados

| Métrica | Execução 1 | Execução 2 | Execução 3 | Média / dispersão |
|---|---:|---:|---:|---:|
| Eventos/s | 99.932,3 | 99.934,1 | 99.939,0 | 99.935,1; 0,007% |
| Delta vs. alvo | −0,07% | −0,07% | −0,06% | −0,065% |
| Bytes/s (protobuf) | 109,0 MiB/s | 109,0 MiB/s | 109,0 MiB/s | 109,0 MiB/s |
| Bytes/s no fio (gzip) | 16,3 MiB/s | 16,3 MiB/s | 16,3 MiB/s | 16,3 MiB/s |
| Latência de envio p50 | 11,50 ms | 12,20 ms | 12,09 ms | 11,93 ms |
| Latência de envio p95 | 38,66 ms | 39,19 ms | 39,21 ms | 39,02 ms |
| Latência de envio p99 | 42,25 ms | 42,98 ms | 43,01 ms | 42,75 ms |
| CPU do host (média) | 9,64% | 10,14% | 10,11% | 9,96% |
| RAM do host (média) | 30,95% | 30,46% | 30,60% | 30,67% |
| Compressão no fio | 6,70x | 6,70x | 6,70x | 6,70x |
| Compressão do payload | 8,65x | 8,65x | 8,64x | 8,65x; 132,27 gzip B/evento |
| Erros | 0 | 0 | 0 | 0 |
| Eventos descartados | 0 | 0 | 0 | 0 |
| Backpressure | 0 | 0 | 0 | 0 |
| Pacing lag | 0 | 0 | 0 | 0 |

**Variância observada vs. tolerância documentada:**

| Métrica | Dispersão | Tolerância | Resultado |
|---|---:|---:|---|
| Eventos/s, bytes/s | 0,007% | ±5% | pass |
| Latência p50 | 5,93% | ±10% | pass |
| Latência p95 | 1,41% | ±10% | pass |
| Latência p99 | 1,77% | ±10% | pass |
| CPU média | 4,94% | ±10% | pass |
| RAM média | 1,60% | ±10% | pass |
| Compressão | 0,042% | ±5% | pass |
| Error rate | 0 p.p. | ±0,1 p.p. | pass |

### Veredito

**Pass para o mecanismo do harness.** As três execuções ficaram dentro da
tolerância documentada; a taxa ficou 0,06–0,07% abaixo do alvo; não houve erro,
descarte, backpressure ou pacing lag. O relatório marca as métricas de gateway,
lag de ingestão, perda confirmada, consulta e armazenamento como
`not_collected`, com a story responsável. Isso é intencional: o relatório não
inventa números.

**Não é um resultado do G1.** O receptor é um sink local; o hardware é um
laptop; a duração é de 60 s; não há ClickHouse, VictoriaMetrics, NATS ou
gateway. O G1 exige 1M/s por 1 h no cluster T1, lag p95 < 10 s e zero perda
confirmada.

## Story 3.1 — schema ClickHouse de logs (local, 1B)

**O que é:** o spike de schema da Story 3.1 em um ClickHouse 26.8 single-node
local, com 1B de linhas sintéticas em 24 h. Mede compressão, a consulta `q03`
(tenant + projeto + serviço + severidade + janela de 1 h + `LIMIT 100`) e a
expiração dos buckets de retenção de 7 e 15 dias. **Não é o gateway, não é o
cluster T1 e não é evidência do Gate G1.**

### Identificação

| Campo | Valor |
|---|---|
| Commit do schema/harness | `494391a` (limpo, sem alterações não commitadas) |
| Carga (UTC) | 2026-11-02T18:07:20Z a 2026-11-02T18:47:40Z (40 min 20 s; ~413k linhas/s) |
| Dataset | 1.000.000.000 linhas; 20 chunks × 50.000.000; 24 h ordenadas no tempo |
| Dimensões | 2 tenants × 4 projetos × 8 serviços; 90% INFO / 7% WARN / 3% ERROR |
| Corpo | 165,3 B de média; IDs de trace/span aleatórios |
| Retenção | 7 e 15 dias por linha (`retention_days`) |
| Hardware | `kabuntu-zenbook`, 22 CPUs, 15,0 GiB de RAM, disco de 441,1 GiB (42,6 GiB livres ao fim) |
| ClickHouse | 26.8.36.3 (`clickhouse/clickhouse-server:26.8`) |
| Schema | [`0001_otel_logs.sql`](internal/adapters/clickhouse/migrations/0001_otel_logs.sql), sha256 `2daca91e22cdbfe3cfb367fb8f8241e9839464653f2a0125f3352c35e7197e05` |
| Artefatos | [`report.md`](bench/clickhouse/results/2026-11-02-local-1b/report.md), [`report.json`](bench/clickhouse/results/2026-11-02-local-1b/report.json), [`handoff`](docs/dev/story-3.1-handoff.md) |

### Resultados

| Métrica | Valor |
|---|---:|
| Compressão | **6,98x** (77,55 GiB comprimidos / 541,09 GiB sem compressão) |
| Bytes comprimidos/linha | 83,3 B |
| Partes ativas | 1.921 (merges de background parados no run local) |
| `q03` p50 | 248,8 ms |
| `q03` p95 | **263,0 ms** (alvo < 1.000 ms) |
| `q03` p99 | 298,5 ms |
| Linhas lidas (média) | 21.282.816 |
| Bytes lidos (média) | 426.995.180 B |
| Linhas retornadas | 100 |
| TTL 7/15 | **pass**: 5 inseridas, 3 sobreviveram; expiradas de 7 e 15 dias removidas; a linha de 15 dias com 8 dias de idade sobreviveu |

### Veredito

**Pass local para o schema.** A compressão ficou 39,6% acima do alvo de 5x, a
`q03` ficou 3,8x abaixo do alvo de 1 s e os dois buckets de TTL removeram apenas
o que estava expirado. O run usou SQL para gerar o dataset, parou os merges de
background para caber no disco local e mediu a tabela com 1.921 partes; esses
fatos estão nas limitações do relatório.

**Não é um resultado do G1.** Não há gateway, NATS, Query Service, row
policies, réplica, caos nem o hardware T1. O baseline T1 continua bloqueado por
P-07 e pelas Stories 3.2–3.5/4.1–4.5.

## Story 3.2 — schema ClickHouse de traces (local, 1B)

**O que é:** o spike de schema da Story 3.2 em um ClickHouse 26.8 single-node
local, com 1B de spans sintéticos (100M traces de 10 spans) em 24 h. Mede a
busca de uma trace completa por `trace_id`, a busca por serviço + status em
24 h, a compressão, um service map básico e a correlação log↔trace. **Não é o
gateway, não é o cluster T1 e não é evidência do Gate G1.**

### Identificação

| Campo | Valor |
|---|---|
| Commit do schema/harness | `f6d6c41` (limpo, sem alterações não commitadas) |
| Carga (UTC) | 2026-11-02T19:05:20Z a 19:37:51Z (32 min 31 s; ~512 mil spans/s) |
| Dataset | 1.000.000.000 spans; 100.000.000 traces de 10 spans; 20 chunks × 50M; 24 h ordenadas no tempo |
| Dimensões | 2 tenants × 4 projetos × 9 serviços × 10 operações; 97/3% OK/ERROR em spans não-raiz e 99/1% em raízes; 10% com event e 5% com link |
| IDs | CityHash64, aleatórios na aparência e determinísticos |
| Retenção | 7 e 15 dias por linha (`retention_days`) |
| Hardware | `kabuntu-zenbook`, 22 CPUs, 15,0 GiB de RAM, disco de 441,1 GiB (66,6 GiB livres ao fim) |
| ClickHouse | 26.8.36.3 (`clickhouse/clickhouse-server:26.8`) |
| Schema | [`0002_otel_traces.sql`](internal/adapters/clickhouse/migrations/0002_otel_traces.sql), sha256 `ad5fe2074531bceeed37a01dadb787215931f1e99994923b6a29f3fc5faaeab3` |
| Artefatos | [`report.md`](bench/clickhouse/traces/results/2026-11-02-local-1b/report.md), [`report.json`](bench/clickhouse/traces/results/2026-11-02-local-1b/report.json), [`handoff`](docs/dev/story-3.2-handoff.md) |

### Resultados

| Métrica | Valor |
|---|---:|
| Compressão | **7,52x** (56,38 GiB comprimidos / 423,98 GiB sem compressão) |
| Bytes comprimidos/span | 60,5 B |
| Partes ativas | 1.701 (merges de background parados no run local) |
| Trace lookup p50 | 308,6 ms |
| Trace lookup p95 | **328,2 ms** (alvo < 1.000 ms) |
| Trace lookup p99 | 332,4 ms |
| Trace lookup linhas lidas | 6.430.720 |
| Trace lookup spans retornados | 10 |
| Serviço + status + 24h p50 | 169,9 ms |
| Serviço + status + 24h p95 | **177,8 ms** (alvo < 2.000 ms) |
| Serviço + status + 24h p99 | 180,5 ms |
| Serviço + status + 24h linhas lidas | 13.935.188 |
| Serviço + status + 24h spans retornados | 100 |
| Service map | 9 nós, 9 arestas; nós 207,6 ms, arestas 596,8 ms (amostra de 1% das traces) |
| Correlação log→trace | ok (10 spans) |
| Correlação trace→log | ok (1 log) |
| RED no ClickHouse | 0 tabelas, 0 MVs (AD-21) |

### Veredito

**Pass local para o schema.** A compressão ficou 50,4% acima do alvo de 5x, a
busca por trace ficou 3,0x abaixo do alvo de 1 s e a busca por serviço/status
ficou 11,2x abaixo do alvo de 2 s. O service map e a correlação funcionaram nos
dois sentidos. O run usou SQL para gerar o dataset, parou os merges de
background para caber no disco local e mediu a tabela com 1.701 partes; esses
fatos estão nas limitações do relatório.

**Não é um resultado do G1.** Não há gateway, NATS, Query Service, row
policies, réplica, caos nem o hardware T1. O baseline T1 continua bloqueado por
P-07 e pelas Stories 3.2–3.5/4.1–4.5.

## Story 3.3 — caminho de ingestão ClickHouse (local, 400k/s)

**O que é:** o caminho de ingestão da Story 3.3 em um ClickHouse 26.8
single-node local: batches RowBinary de 200k eventos pelo HTTP, spool durável
com checkpoint, dedupe por `event_id` e token de bloco. Mede throughput, lag
ponta a ponta do writer, parts e erros de insert. **Não é o gateway, não usa
NATS e não é evidência do Gate G1.**

### Identificação

| Campo | Valor |
|---|---|
| Commit do writer/harness | `9650da1` (limpo, `dirty: false` no relatório) |
| Run (UTC) | 2026-11-02T20:28:37Z a 20:29:38Z |
| Workload | 400k/s por 60 s; 120 batches de 200k; 2 tenants × 4 projetos × 8 serviços |
| Writer | 12 workers de geração/encode; 4 threads de insert; `async_insert=1` + `wait_for_async_insert=1` |
| Dedupe | janela de 200k `event_id`; token estável por batch; migration `0003_ingest_dedupe.sql` |
| Hardware | `kabuntu-zenbook`, 22 CPUs, 15,0 GiB de RAM; ClickHouse 26.8.36.3 |
| Schema | [`0001_otel_logs.sql`](internal/adapters/clickhouse/migrations/0001_otel_logs.sql), [`0003_ingest_dedupe.sql`](internal/adapters/clickhouse/migrations/0003_ingest_dedupe.sql) |
| Artefatos | [`report.md`](bench/clickhouse/ingest/results/2026-11-02-local-400k/report.md), [`report.json`](bench/clickhouse/ingest/results/2026-11-02-local-400k/report.json), [`handoff`](docs/dev/story-3.3-handoff.md) |

### Resultados

| Métrica | Valor |
|---|---:|
| Eventos confirmados | 24.000.000 |
| Throughput | **399.994,4/s** |
| Lag p50 (evento mais antigo) | 679,7 ms |
| Lag p95 | **1.403,0 ms** (alvo < 10.000 ms) |
| Lag p99 | 1.434,6 ms |
| Linhas armazenadas | 24.000.000 |
| `uniqExact(event_id)` | 24.000.000 |
| Parts ativas ao fim | 18 |
| Compressão | 56,31x (corpo sintético repetitivo) |
| Erros de insert | 0 |
| `Too many parts` | 0 |
| Dedupe (50k iguais) | 50.000 linhas / 50.000 duplicatas |
| Recuperação (100k) | 100.000 antes e depois; 1 batch reenviado; checkpoint avançado |

### Veredito

**Pass local para o caminho de ingestão.** O writer sustentou 400k/s por 60 s
com lag p95 de 1,4 s, sem `Too many parts`, sem erro de insert e sem perda de
eventos confirmados. O teste de dedupe manteve 50k linhas únicas ao submeter o
mesmo batch duas vezes; o teste de crash depois do insert e antes do checkpoint
manteve 100k linhas após o replay.

**Não é o G1.** O host local satura em ~400k/s; uma tentativa de 500k/s por
60 s ficou em 397–447k/s com lag p95 de 12–17 s. O run de 1M/s por 1 h exige o
cluster T1 (6 ClickHouse + 6 writers) e o caminho gateway → NATS → writer
(Stories 4.1/4.3), além de P-07. O WAL do run local é o spool de arquivos
permitido pelo AD-4 no modo single-node.

## Story 3.4 — VictoriaMetrics, MetricsQL e cardinalidade (local, 1M séries)

**O que é:** o spike da Story 3.4 em um VictoriaMetrics 1.148 LTS
single-node local: 1M séries ativas a 1M amostras/s por 1 h via Prometheus
remote write, 20 consultas MetricsQL em janelas de 24 h e admissão de
cardinalidade por tenant com um proxy de spike (AD-33). **Não é o gateway,
não usa vmauth/Query Service e não é evidência do Gate G1.**

### Identificação

| Campo | Valor |
|---|---|
| Commit do harness | `c153046` + harness da Story 3.4 (working tree `dirty: true`; o commit de publicação é o desta sessão) |
| Run (UTC) | 2026-11-02T22:27:01Z a 23:29:32Z |
| Workload | 1M séries, 1M amostras/s, 1 h, 2 tenants × 2 projetos; remote write protobuf + snappy |
| Consultas | 20 MetricsQL em 24 h, step 5 m, `nocache=1`, 5 execuções + 1 warm-up |
| Cardinalidade | limite tenant-a 150k, tenant-b 100k; probe de 250k/50k séries; drift do vizinho |
| Hardware | `kabuntu-zenbook`, 22 CPUs, 15,0 GiB de RAM; VictoriaMetrics 1.148.0 |
| Artefatos | [`report.md`](bench/victoriametrics/results/2026-11-02-local-1m-1h/report.md), [`report.json`](bench/victoriametrics/results/2026-11-02-local-1m-1h/report.json), [`handoff`](docs/dev/story-3.4-handoff.md) |

### Resultados

| Métrica | Valor |
|---|---:|
| Amostras enviadas | 3.600.000.000 |
| Throughput | **999.737,99/s** (99,974% de 1M/s) |
| Erros / rejeições | 0 / 0 |
| Latência de request p50 / p95 / p99 | 7,13 / **13,09** / 21,28 ms |
| Séries ativas após a carga | 1.000.006 (1M do catálogo + 6 da verificação) |
| Séries ativas após a cardinalidade | 1.200.006 |
| RSS máximo | 1,24 GiB (1.331.040.256 B) |
| Tamanho do diretório | 15,27 GiB (16.390.650.204 B) |
| 20 consultas p50 / p95 / p99 | 573,95 / **799,50** / 1.216,71 ms |
| Consultas com erro | 0 |
| Correção (`rate`, `histogram_quantile`, labels) | 7/7 checks |
| Tenant-a rejeitado | 100.000 séries (1.004 requests) |
| Tenant-b rejeitado | 0 |
| Drift do vizinho | **4,82%** (alvo < 10%) |

### Veredito

**Pass local para o spike de métricas.** O VictoriaMetrics sustentou 999.738
amostras/s com 1M séries ativas por 1 h, sem erro de ingestão, com RSS máximo
de 1,24 GiB (bem dentro do orçamento de 32 GiB do nó T1). As 20 consultas
MetricsQL em janelas de 24 h ficaram com p95 agregado de **799,50 ms** (alvo
< 1 s), 0 erros e 20/20 consultas com resultado; `m14` (visão de operador) foi
a única acima de 1 s, com 1.305 ms. O proxy de admissão rejeitou as 100.000
séries excedentes do tenant-a com `429 cardinality_limit_exceeded` e manteve o
tenant-b sem rejeições; o p95 do vizinho variou **4,82%** (alvo < 10%).

**Não é o G1.** O run é single-node e local; o perfil T1 tem 16 vCPU e 64 GiB.
O `m14` acima de 1 s é uma visão de operador cross-tenant; as 19 consultas de
dashboard ficaram abaixo de 1 s. O baseline T1 continua bloqueado por **P-07**
e pelas Stories 4.1/4.3/4.5.

## Story 3.5 — consulta, retenção e row policies (local, 1B eventos)

**O que é:** o spike da Story 3.5 em um ClickHouse 26.8 single-node local com
o dataset de 1B eventos do cenário `query-1b` (60% métricas, 30% logs, 10%
traces), ou seja, **300M logs + 100M spans + 600M amostras de métricas**. Mede
as 16 consultas SQL do catálogo, as 4 consultas MetricsQL (do run da Story 3.4),
o isolamento por tenant com row policies e `authz_epoch`, a retenção 7/15/30 e
o tier de 86B/24h em rollups de 1 minuto. **Não é o cluster T1 e não é
evidência do Gate G1.**

### Identificação

| Campo | Valor |
|---|---|
| Commit do harness | `d867eb2` + harness da Story 3.5 (working tree `dirty: true`; o commit de publicação é o desta sessão) |
| Run (UTC) | 2026-11-03T01:23:09Z a 01:49:37Z |
| Dataset | 300.000.000 logs + 100.000.000 spans em 24 h; 2 tenants × 4 projetos; retenção 7/15/30 |
| Consultas | 16 SQL (q01–q15, q20) + 4 MetricsQL (q16–q19); 20 execuções + 1 warm-up por consulta |
| Isolamento | 15 testes de cross-tenant, subquery, `OR 1=1`, join, table functions, system tables, escrita/DDL e epoch |
| Rollup 86B | 1 minuto; 9 consultas agregadas; mesmas row policies |
| Hardware | `kabuntu-zenbook`, 22 CPUs, 15,0 GiB de RAM; ClickHouse 26.8.36.3 |
| Artefatos | [`report.md`](bench/clickhouse/results/2026-11-02-local-1b-mix/report.md), [`report.json`](bench/clickhouse/results/2026-11-02-local-1b-mix/report.json), [`handoff`](docs/dev/story-3.5-handoff.md) |

### Resultados

| Métrica | Valor |
|---|---:|
| Logs comprimidos | 23,52 GiB (7,51x; 84,2 B/linha) |
| Spans comprimidos | 5,67 GiB (7,74x; 60,9 B/linha) |
| 16 consultas SQL p95 máximo | **431,94 ms** (q05; alvo < 1.000 ms) |
| 16 consultas SQL p99 máximo | 448,42 ms (q05; alvo < 2x p95) |
| 4 consultas MetricsQL p95 | **799,50 ms** (alvo < 1.000 ms) |
| Testes de isolamento | 15/15 pass; cross-tenant 0 linhas |
| Epoch (`authz_epoch`) | linhas antigas invisíveis; linha nova visível; pass |
| TTL 7/15/30 | 1.000 linhas sobreviventes por projeto; pass |
| Mutação de política (10M linhas) | **493 ms**; 10.000.000 → 0 após expirar |
| 10B/7d (projeção window-equivalent) | **617,06 ms** (alvo < 3.000 ms) |
| 86B/24h rollup (9 consultas) | p95 máximo **87,14 ms** (q07; alvo < 3.000 ms) |

### Veredito

**Pass local para o spike de consulta/retenção/isolamento.** O dataset de 1B
eventos manteve as 16 consultas SQL com p95 máximo de **431,94 ms** e p99
abaixo de 2x o p95; as 4 consultas MetricsQL da Story 3.4 ficaram em **799,50
ms** de p95. As row policies isolaram os tenants em todos os 15 testes,
incluindo subquery, `OR 1=1`, join e table functions perigosas; o bump de
`authz_epoch` invalidou as linhas antigas. O TTL 7/15/30 expirou cada projeto
conforme a política e a mutação de troca de política em 10M linhas levou
**493 ms**. O tier de 86B/24h foi medido no rollup de 1 minuto (9 consultas,
p95 máximo **87,14 ms**); o de 10B/7d é uma projeção window-equivalent
(**617,06 ms**), não uma medição raw.

**Não é o G1.** O raw 10B/86B não foi materializado; o host tem 15 GiB de RAM e
~143 GiB livres. O Query Service, o gateway e o vmauth continuam pendentes das
Stories 4.1–4.5. O baseline T1 continua bloqueado por **P-07** e pelas Stories
4.1–4.5.

## Story 4.2 — auth, quota e isolamento do gateway (local)

**O que é:** um gateway local com dois tenants, tokens de projeto e quotas
independentes. O tenant quieto envia 50k eventos/s (logs); o barulhento oferece
1M eventos/s contra uma quota de 100k/s. O harness mede o p95 ponta a ponta e o
p95 de processamento do gateway por tenant, em 3 repetições. **Não usa WAL,
armazenamento ou Query Service** e não é evidência do G1.

### Identificação

| Campo | Valor |
|---|---|
| Story | 4.2 — Auth por tenant, quotas e rate limiting |
| Commit | `dab5e98` |
| Harness | `bench/gateway/quota/run.sh --small-payload` |
| Payload | 0 B métrica, 64 B log, 0 B trace |
| Compressão | quiet gzip; barulhento none |
| Duração | 10 s por fase × 3 repetições; mediana |
| Host | Intel Core Ultra 7 155H, 22 threads, 15 GiB RAM |
| Data | 2026-11-03 |

### Resultados

| Métrica | Valor |
|---|---|
| Quota do tenant barulhento | 100.000 eventos/s |
| Tenant quieto | 50.000 eventos/s |
| Tenant barulhento ofertado | 1.000.000 eventos/s |
| Requests 429 | 26.703 |
| p95 de processamento do gateway, quieto sozinho | 5,461 ms |
| p95 de processamento do gateway, quieto sob carga | 6,287 ms |
| Variação do p95 de processamento | **+15,13%** |
| p95 cliente, quieto sozinho | 10,535 ms |
| p95 cliente, quieto sob carga | 11,610 ms |
| Variação do p95 cliente | **+10,20%** |
| Run de smoke (100k/s ofertado, 10k/s quota) | **+5,11%** no p95 do gateway |

### Veredito

**Pass funcional; isolamento local inconclusivo.** O gateway devolveu 429 com
`Retry-After` para 26.703 requests do tenant barulhento, o tenant quieto não
teve erros e os buckets são independentes; 401/403/audit também foram
exercitados no `make smoke`. O p95 local de 15,13% (gateway) e 10,20%
(cliente) ficou acima do limite de 10%, mas o gateway e os dois geradores de
carga compartilham o mesmo host: a 1M/s o gerador barulhento satura CPU e banda
de memória, o que contamina a comparação. O run com payload pequeno foi
necessário para o host sustentar 1M/s; com o payload de referência o gerador
não passa de ~640k/s com gzip.

**A medição de aceite (`<10%`) fica para o T1**, com load generators em hosts
separados, e continua bloqueada por **P-07** e pelas Stories 4.3–4.5. O veredito
funcional e a pré-admissão antes do decode estão registrados em
[`docs/dev/story-4.2-handoff.md`](docs/dev/story-4.2-handoff.md).

## Story 4.3 — gateway, zstd e WAL NATS (local)

**O que é:** dois tenants enviam 100k eventos/s no total (50k/s cada) por 10 s
para o gateway, que aplica redaction, sintetiza `event_id`, agrupa em envelopes
protobuf, comprime com zstd e publica em 32 shards por sinal no NATS JetStream.
O gateway só confirma depois do ack do JetStream. O `walcheck` relê todas as
mensagens sem ack e confere contagem, IDs únicos, compressão e distribuição.
**Não é o run de 1M/s por 1 h do T1** e não é evidência do G1.

### Identificação

| Campo | Valor |
|---|---|
| Story | 4.3 — Batching, compressão, sharding e WAL NATS |
| Harness | `bench/gateway/wal/run.sh` |
| Resultado | `bench/gateway/wal/results/2026-11-03-local-100k/` |
| NATS | 2.15.2-alpine, JetStream, `max_payload` 16 MiB |
| Shards | 32 por tenant/projeto/epoch/signal |
| Batch | 500 eventos/request; linger 5 ms |
| Compressão | gzip no cliente, zstd no envelope |
| Duração | 10 s |
| Host | Intel Core Ultra 7 155H, 22 threads, 15 GiB RAM |
| Data | 2026-11-03 |

### Resultados

| Métrica | Valor |
|---|---:|
| Taxa ofertada | 100.000 eventos/s |
| Eventos aceitos | 1.000.000 |
| Eventos no WAL | 1.000.000 |
| `event_id` únicos | 1.000.000 |
| Duplicatas | 0 |
| Compressão do envelope | **5,46x** |
| Subjects | 192 (2 tenants × 3 sinais × 32 shards) |
| Maior subject | 0,95% das mensagens |
| p99 de processamento do gateway | **28,9 ms** (alvo AC1 < 50 ms) |
| p99 cliente | 54,5 ms |
| Falhas / rejeições | 0 / 0 |
| Mensagens publicadas | 2.000 |
| WAL publish p99 | 1,9 ms |

**Run de 1M/s com payload mínimo:** 5.000.000 eventos, ~740k/s sustentados no
host compartilhado, 32 shards, 5M IDs únicos, compressão 3,13x e p99 de
processamento **25,4 ms**. O payload de referência não sustenta 1M/s localmente
com os load generators no mesmo host.

### Veredito

**Pass funcional local; AC1 de throughput pendente do T1.** O caminho de
batching, compressão, sharding, ack durável e backpressure funciona, e o run de
100k/s ficou bem abaixo do alvo de p99. O AC1 exige 1M/s por 1 h na frota T1,
com load generators em hosts separados; P-07 e o baseline T1 continuam
bloqueados. O teste de NATS/ClickHouse fora por 15 min é a Story 4.5.

**Evidência:** [`docs/dev/story-4.3-handoff.md`](docs/dev/story-4.3-handoff.md).

## Story 4.4 — writers sobre o WAL NATS (local)

**O que é:** o gateway publica no WAL; três consumidores duráveis leem os
streams, deduplicam por `event_id`, convertem o OTLP e escrevem logs/traces no
ClickHouse e métricas no VictoriaMetrics. O ack do WAL acontece só depois do
flush. O run principal usa 20k eventos/s por 20 s em dois tenants; um smoke
com caos para o ClickHouse por 3 s e o VictoriaMetrics por 2 s e verifica o
retry. **Não é o run de 1M/s por 1 h do T1** e não é evidência do G1.

### Identificação

| Campo | Valor |
|---|---|
| Story | 4.4 — Writers para ClickHouse e VictoriaMetrics |
| Harness | `bench/pipeline/run.sh` |
| Resultado principal | `bench/pipeline/results/2026-11-03-local-20k/` |
| Resultado de caos | `bench/pipeline/results/2026-11-03-local-5k-chaos/` |
| Componentes | NATS 2.15.2 JetStream, ClickHouse 26.8, VictoriaMetrics 1.148 |
| Migrações | `0001`–`0004` (logs, traces, dedupe e tenancy) |
| Batch | 200 eventos/request; writer 20k eventos ou 200 ms |
| Compressão | gzip no cliente OTLP e no INSERT do ClickHouse |
| Host | Intel Core Ultra 7 155H, 22 threads, 15 GiB RAM |
| Data | 2026-11-03 |

### Resultados — 20k/s estável

| Métrica | Valor |
|---|---:|
| Eventos ofertados (runs medidas) | 400.000 |
| Eventos no WAL | 400.000 |
| Eventos aceitos pelo writer (todas as fases) | 420.000 |
| Escritos | 406.700 |
| Deduplicados | 13.300 |
| Falhas / rejeições | 0 / 0 |
| Retries | 0 |
| Logs no ClickHouse | 135.300 (135.300 únicos) |
| Traces no ClickHouse | 132.000 (132.000 únicos) |
| Amostras no VictoriaMetrics | 139.400 (64 séries) |
| Lag p50 / p95 / p99 | **0,515 s / 0,802 s / 0,887 s** |
| Exit codes gateway/writer | 0 / 0 |

O total aceito inclui a fase de dedupe: 10.000 eventos de um replay idêntico
foram deduplicados e 3.300 traces colidiram pelo `trace_id`/`span_id`
determinístico do load generator. A soma `written + deduped + failed` fecha
com o aceito.

### Resultados — 5k/s com caos

| Métrica | Valor |
|---|---:|
| Eventos ofertados (runs medidas) | 25.000 |
| Aceitos (todas as fases) | 30.000 |
| Escritos | 26.675 |
| Deduplicados | 3.325 |
| Retries | **8** |
| Logs / traces no ClickHouse | 9.075 / 8.250 |
| Amostras no VictoriaMetrics | 9.350 (9.350 escritas; igual ao writer neste run) |
| Lag p95 | **7,97 s** durante o caos |
| Falhas | 0 |

O ClickHouse ficou fora por 3 s e o VictoriaMetrics por 2 s. O writer manteve
as mensagens não confirmadas no WAL, retentou com backoff e fechou as
contagens depois da recuperação; nenhum evento confirmado foi perdido.

### Veredito

**Pass funcional local; AC1 de throughput pendente do T1.** A contagem
armazenada bate com a aceita nos logs e traces, o remote write bate com o
`written` de métricas dentro do coalescimento de milissegundos, o lag p95
ficou abaixo de 1 s no run estável e abaixo de 10 s no caos, e o retry
exponencial não perdeu dados. O AC1 exige 1M/s por 1 h na frota T1; P-07 e o
baseline T1 continuam bloqueados. O outage de 15 min é a Story 4.5.

**Evidência:** [`docs/dev/story-4.4-handoff.md`](docs/dev/story-4.4-handoff.md).

## Cluster de referência T1 (pendente)

O perfil está em [`bench/env/t1.json`](bench/env/t1.json) e a documentação de
provisionamento em [`bench/env/README.md`](bench/env/README.md).

| Papel | Nós | Instância | Disco | Capacidade |
|---|---:|---|---|---:|
| ClickHouse | 6 (6–12) | 16 vCPU / 64 GiB | 2 × 16 TiB gp3, 500 MB/s, 16k IOPS | 192 TiB raw / 96 TiB lógicos; 121 TiB exigidos pela retenção de 7 dias; +58,7% |
| VictoriaMetrics | 3 | 16 vCPU / 64 GiB | 1 × 12 TiB gp3, 500 MB/s, 16k IOPS | 36 TiB raw / 18 TiB lógicos; 26,4 TiB exigidos; +36,3% |
| Gateway | 6 (4–6) | 16 vCPU / 32 GiB | root 100 GiB | 1,44M eventos/s a 15k eventos/s/vCPU |
| NATS JetStream | 4 | 8 vCPU / 32 GiB | 1 × 512 GiB gp3 | 2 TiB raw / 1 TiB lógico; ≥1 h de outage; +127,9% |
| Loadgen | 2 | 16 vCPU / 32 GiB | root 100 GiB | 1,92M eventos/s a 60k eventos/s/vCPU |

Premissas: 1M eventos/s, 60% métricas (~40 B), 30% logs (~200 B), 10% traces
(~500 B), 134 B/evento comprimidos, retenção de 168 h, réplica 2 no ClickHouse,
2 no VictoriaMetrics e 2 no NATS, single-AZ. O perfil **não** inclui PostgreSQL,
object storage, backup, suporte, NAT gateway ou tráfego de saída.

### Custo estimado

Snapshot On-Demand de `us-east-1` em 2026-11-01
([`bench/env/prices.aws-us-east-1.json`](bench/env/prices.aws-us-east-1.json)),
apenas para decisão; não é uma fatura.

| Item | Valor |
|---|---:|
| Custo por hora | USD 41,79 |
| Custo de um run de 4 h | USD 167,15 |
| Custo mensal (730 h) | USD 30.504,49 |
| Custo por TB/dia | USD 86,62 (11,58 TB/dia) |

O disco do ClickHouse domina o custo: 6 × 32 TiB gp3 para 7 dias de retenção.
O addendum do PRD cita ~USD 300–800 por nó/mês com 4 TB; essa conta cobre
aproximadamente **um dia** de 1M/s com réplica 2, não sete. A diferença é um
achado do benchmark, não um erro de arredondamento. Um run de 1 h usa o mesmo
cluster, mas pode ser reduzido com um perfil de retenção menor — a diferença
tem de ser declarada no relatório.

### Provisionamento e limpeza

```bash
make build
cp bench/env/terraform/terraform.tfvars.example bench/env/terraform/terraform.tfvars
# edite admin_cidr e ssh_public_key

./bench/env/cluster.sh check
./bench/env/cluster.sh cost --hours 4
./bench/env/cluster.sh plan
OBSERVAKI_BENCH_ALLOW_SPEND=1 ./bench/env/cluster.sh apply
# ... execute o baseline ...
./bench/env/cluster.sh destroy
```

O `apply` é idempotente via Terraform, exige `OBSERVAKI_BENCH_ALLOW_SPEND=1`,
recusa credenciais root da AWS e pede confirmação digitada. O `destroy` calcula
as horas desde o `apply`, imprime o custo estimado do run, destrói os recursos e
verifica que o state ficou vazio. O `terraform.tfvars` guarda só `admin_cidr`,
`ssh_public_key` e tags; o shape vem do perfil.

### Protocolo do baseline

```bash
./bench/env/baseline.sh --scenario throughput-1m \
  --endpoint http://<gateway-privado>:4318 \
  --results-dir /var/lib/observaki-bench/results
```

1. **warm-up:** 1M/s por 10 min, não medido.
2. **ramp:** 10%, 25%, 50%, 75% e 100% da taxa, 1 min por degrau, não medido.
3. **medido:** 1M/s por 1 h. É o número do gate.

O relatório inclui as três fases; warm-up e ramp têm `--rate`/`--duration`
diferentes e não entram na comparação de variância. O script pode ser validado
sem cluster com `--dry-run`.

### O que falta para publicar o baseline T1

1. **Aprovação de provedor, orçamento e credencial dedicada** (P-07 em
   `DECISIONS.md`). O perfil de referência é AWS; outra nuvem exige um adapter
   Terraform e uma tabela de preços, mas não muda o perfil.
2. **Gateway OTLP (Story 4.1)** para receber o workload e calcular RED.
3. **Ingestão ClickHouse (Story 3.3)** para medir lag e perda confirmada; o
   writer/spool/dedupe foi validado localmente a 400k/s, mas o número de 1M/s
   por 1 h precisa do cluster T1 e do caminho gateway → NATS → writer.
4. **VictoriaMetrics e cardinalidade (Story 3.4)** validadas localmente
   (1M séries, 1M/s, p95 799,5 ms, drift 4,82%); falta o RED do gateway
   (AD-21) e o vmauth no cluster T1.
5. **Query Service (Fase 1/Story 5.5)** para executar as 20 consultas via
   contexto assinado; o catálogo SQL e as latências locais foram validados na
   Story 3.5, mas o QS não existe. O raw 10B/86B também continua pendente: o
   tier de 10B/7d é uma projeção e o de 86B/24h foi medido no rollup.
6. **Automação de caos (Story 4.5)** para o teste de falha e DLQ.
7. **Validação do protocolo de rampa no cluster:** o `baseline.sh` executa fases
   separadas de taxa constante; a rampa contínua dentro de uma única execução
   ainda não está no gerador. O número do gate só será publicado quando o
   protocolo completo for executado e as fases ficarem registradas.

## Reprodutibilidade

### Pré-baseline local

```bash
make build
./bin/bench-sink --addr 127.0.0.1:4318 &
for i in 1 2 3; do
  ./bench/run.sh run throughput-100k --duration 60s \
    --endpoint http://127.0.0.1:4318 \
    --results-dir /tmp/observaki-bench/results
done
./bench/run.sh report --results-dir /tmp/observaki-bench/results \
  --out /tmp/observaki-bench/report
```

O `bench-sink` é um receptor descartável: aceita qualquer POST, drena o corpo e
devolve 200. Ele não é o gateway e não deve ser usado como um.

### Spikes locais de armazenamento

```bash
# ClickHouse: logs, traces e ingestão
./bench/clickhouse/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5
./bench/clickhouse/traces/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5 --stop-merges
./bench/clickhouse/ingest/run.sh --smoke

# VictoriaMetrics: 1M séries, 1M/s, 1 h
make bench-victoriametrics-check
make bench-victoriametrics
```

### Cluster T1

```bash
./bin/benchctl env check
./bin/benchctl env cost --hours 4
./bench/env/cluster.sh plan
OBSERVAKI_BENCH_ALLOW_SPEND=1 ./bench/env/cluster.sh apply
./bench/env/baseline.sh --scenario throughput-1m \
  --endpoint http://<gateway-privado>:4318
./bench/env/cluster.sh destroy
```

## Limitações e regras de publicação

- Números de vendor não entram sem reprodução própria.
- Toda execução entra no relatório, inclusive `failed` e `blocked`; só
  `status: ok` e `mode: run` entram na comparação.
- O baseline T1 só é publicado com commit, hardware, versões, config hash,
  seed, custo e veredito.
- Regressão de throughput, latência, compressão ou bundle é bug P1.
- O custo do cluster é parte do resultado: performance sem custo não decide o
  gate.
- O relatório do baseline T1 substituirá a seção "pendente" acima; a
  pré-baseline local permanece como evidência do mecanismo, não como G1.
