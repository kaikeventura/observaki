# bench/ — harness de benchmark

O harness reproduzível da Fase 0. Componentes:

| Componente | Story | O que faz |
|---|---|---|
| `bench/loadgen` + `otlploadgen` | 2.1 | Gera o workload OTLP determinístico (60/30/10) com taxa, backpressure e métricas próprias |
| `bench/scenarios` + `bench/run.sh` | 2.2 | Define cenários de throughput, query, cardinalidade e caos; valida pré-requisitos e grava resultados |
| `bench/queries` | 2.2 | Catálogo das 20 consultas representativas de dashboard |
| Relatório `report.md`/`report.json` | 2.3 | Coleta CPU/RAM/disco, normaliza as métricas e compara repetições com variância documentada |
| Ambiente reproduzível e baseline | 2.4 | Perfil T1, Terraform, custo/cleanup e protocolo de baseline; execução T1 pendente |
| `bench/clickhouse` | 3.1 | Schema ClickHouse de logs, carga sintética, compressão, `q03` e TTL 7/15 dias em container local |
| `bench/victoriametrics` | 3.4 | 1M séries a 1M amostras/s, 20 consultas MetricsQL em 24 h e admissão de cardinalidade por tenant |

### Story 3.1 — ClickHouse logs

O spike de schema de logs fica em [`clickhouse/`](clickhouse/README.md). Ele sobe
um ClickHouse 26.8 descartável, aplica a migração
`internal/adapters/clickhouse/migrations/0001_otel_logs.sql`, mede compressão,
a consulta `q03` em 1B linhas e valida o TTL de 7/15 dias.

```bash
./bench/clickhouse/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5  # smoke
./bench/clickhouse/run.sh --rows 1000000000 --chunk-rows 50000000 --runs 20
```

O run local é evidência de schema, não do G1: não há gateway, NATS, Query
Service, row policies nem réplica.

### Story 3.4 — VictoriaMetrics

O spike de métricas fica em [`victoriametrics/`](victoriametrics/README.md).
Ele sobe um VictoriaMetrics 1.148 LTS descartável, valida `rate`,
`histogram_quantile` e agregação por label, envia 1M séries a 1M amostras/s
por 1 h, roda as 20 consultas MetricsQL em janelas de 24 h e testa o limite de
séries por tenant com um proxy de admissão (AD-33).

```bash
./bench/victoriametrics/run.sh --smoke                 # smoke
./bench/victoriametrics/run.sh --reset                 # run publicado
```

O run é single-node e local; não substitui o baseline T1 (P-07) nem o G1.

## Workload de referência

| Sinal | Mix | Payload comprimido | Alvo no `--check-payload` |
|---|---|---|---|
| Métricas | 60% | ~40 B | 40 B ±10% |
| Logs | 30% | ~200 B | 200 B ±10% |
| Traces | 10% | ~500 B | 500 B ±10% |
| **Média ponderada** | 100% | **~134 B** | **134 B ±10%** |

- 1M eventos/s ≈ 134 MB/s comprimidos ≈ 11,6 TB/dia.
- O gerador mede o tamanho **gzip** por evento em lotes do tamanho configurado
  (`--batch-size`), não o tamanho nominal do payload.
- O seed determina IDs, timestamps, atributos e o conteúdo dos pools de
  payload. Duas execuções com o mesmo seed, mix, batch e payload produzem os
  mesmos eventos e a mesma distribuição.
- O atributo `bench.payload` é um preenchimento sintético para calibrar o
  tamanho comprimido; ele não representa um label de produção.

## Cenários

Os cenários são arquivos `.env` versionados em `bench/scenarios/`. Cada arquivo
declara hardware, duração, pré-requisitos, métricas coletadas e o veredito
esperado; o `benchctl` valida o contrato antes de qualquer execução.

| Cenário | Família | Carga | Duração | Hardware mínimo | Veredito esperado | Execução |
|---|---|---|---|---|---|---|
| `throughput-100k` | throughput | 100k eventos/s, 1 shard | 1h | 8 vCPU/16 GB loadgen; single-node 8 vCPU/32 GB | Taxa ±5%; lag p95 < 10s; zero perda confirmada | pronta |
| `throughput-1m` | throughput | 1M eventos/s, 1 shard | 1h | 16 vCPU/32 GB loadgen; single-node 16 vCPU/64 GB | Taxa ±5% (gate G1); lag p95 < 10s; zero perda | pronta |
| `throughput-5m` | throughput | 5M eventos/s, 5 shards de 1M/s | 1h | 2 loadgens 16 vCPU; cluster T1 da Story 2.4 | Agregado ±5%; lag p95 < 10s; zero perda | pronta |
| `query-1b` | query | 1B eventos em 24h; 20 consultas × 3 | 20m | 1 ClickHouse 16 vCPU/64 GB; 1 VM; 1 gateway | p95 < 1s; p99 < 2× p95 | planejada (3.5) |
| `query-10b` | query | 10B eventos em 7d; 20 consultas × 3 | 45m | 3 ClickHouse 32 vCPU/128 GB; 3 VM | p95 < 3s; p99 < 2× p95 | planejada (3.5) |
| `query-86b` | query | 86,4B eventos em 24h; 20 consultas × 3 | 5h | cluster T1; rollups 1m/5m | p95 < 3s; p99 < 2× p95 | planejada (3.5) |
| `cardinality-100k` | cardinality | 100k séries, 1 amostra/s, 2 tenants | 1h | 1 VictoriaMetrics 8 vCPU/32 GB | Drift do vizinho < 10% | planejada (3.4) |
| `cardinality-1m` | cardinality | 1M séries, 1M amostras/s, 2 tenants | 1h | 1 VictoriaMetrics 16 vCPU/64 GB | Drift do vizinho < 10% | planejada (3.4) |
| `cardinality-10m` | cardinality | 10M séries, 10M amostras/s, 2 tenants | 1h | 3 VictoriaMetrics 32 vCPU/128 GB | Drift do vizinho < 10% | planejada (3.4) |
| `chaos-clickhouse` | chaos | 1M/s; ClickHouse fora por 15m | 1h | cluster T1 | Zero perda confirmada; recuperação automática | planejada (4.5) |
| `chaos-nats` | chaos | 1M/s; NATS fora por 5m | 30m | cluster T1 | Zero perda confirmada; backpressure no produtor | planejada (4.5) |
| `chaos-gateway` | chaos | 1M/s; 1 de 4 gateways morto por 5m | 30m | cluster T1 | Zero perda; tráfego absorvido pelos demais | planejada (4.5) |

O hardware acima é o **mínimo documentado** para o cenário; o perfil T1 e o
cluster de referência são fechados na Story 2.4 (Q-01 do ADR-0006). O texto
completo de cada cenário está no respectivo `.env`.

Métricas coletadas por família (a lista canônica de cada cenário está no `.env`
e é validada pelo `benchctl check`):

| Família | Métricas |
|---|---|
| throughput | `events_per_second`, `bytes_per_second`, lag p50/p95/p99, `error_rate`, `dropped_events`, `confirmed_lost_events`, CPU, RAM, disco, `compression_ratio`, backpressure, pacing lag |
| query | latência p50/p95/p99, `rows_read`, `bytes_read`, `cache_hit_ratio`, `error_rate`, CPU, RAM |
| cardinality | `series_active`, `series_limit_errors`, `neighbor_p95_drift_percent`, lag p95, `error_rate`, CPU, RAM |
| chaos | `events_per_second`, lag p50/p95/p99, `error_rate`, `dropped_events`, `confirmed_lost_events`, `recovery_seconds`, `dlq_events`, CPU, RAM |

> **Status de execução:** throughput executa hoje. Query, cardinalidade e caos
> têm contrato, validação de pré-requisitos e plano registrados; a execução
> entra nas Stories 3.4, 3.5 e 4.5, quando existirem os backends. `run` nesses
> cenários grava `status: blocked` e sai com código 3; `--plan` grava o plano e
> sai com 0.

## Ambiente de referência T1

O perfil do cluster T1 está em [`env/t1.json`](env/t1.json) e o provisionamento
de referência (AWS + Terraform) em [`env/`](env/README.md). O perfil é a fonte
de verdade de região, contagens, instâncias e discos; o `cluster.sh` gera as
variáveis do Terraform a partir dele.

```bash
make build
./bench/env/cluster.sh check       # valida perfil, preços e capacidade
./bench/env/cluster.sh cost        # estimativa para 4 h
./bench/env/cluster.sh plan        # terraform init + plan (não gasta)
OBSERVAKI_BENCH_ALLOW_SPEND=1 ./bench/env/cluster.sh apply
./bench/env/cluster.sh destroy
```

O `apply` é bloqueado sem `OBSERVAKI_BENCH_ALLOW_SPEND=1` e recusa credenciais
root da AWS. O baseline usa [`env/baseline.sh`](env/baseline.sh): warm-up de
10 min, ramp de 5 min e 1 h medida. O protocolo pode ser validado localmente com
`--dry-run`.

O baseline T1 ainda **não foi executado**: dependem dele o gateway (Story 4.1),
o schema ClickHouse (Story 3.1) e a aprovação de provedor/orçamento. O baseline
local publicado em [`BENCHMARKS.md`](../BENCHMARKS.md) é uma pré-baseline do
harness, não evidência do G1.

## Execução

```bash
make build                 # inclui bin/benchctl, bin/otlploadgen e bin/bench-sink
./bench/run.sh list        # inventário dos cenários
./bench/run.sh check       # valida contratos e o catálogo de consultas
./bench/run.sh queries     # imprime as 20 consultas
./bench/run.sh report      # consolida bench/results em report.md + report.json
```

Executar um cenário:

```bash
# valida e grava o plano, sem tráfego
./bench/run.sh run throughput-100k --plan

# execução curta de validação contra um receptor OTLP local
./bench/run.sh run throughput-100k --duration 10s \
  --endpoint http://127.0.0.1:4318

# 5M/s agregado sem enviar (5 processos de 1M/s)
./bench/run.sh run throughput-5m --duration 10s --dry-run

# plano do dataset de 1B eventos/24h (execução bloqueada até a Story 3.5)
./bench/run.sh run query-1b --plan
```

Flags de `run`:

| Flag | Default | Descrição |
|---|---|---|
| `--plan` | `false` | Valida e grava o plano, sem iniciar workload |
| `--dry-run` | `false` | Executa o loadgen sem enviar (throughput) |
| `--duration` | do cenário | Override da duração |
| `--rate` | do cenário | Override da taxa total |
| `--endpoint` | `$OBSERVAKI_BENCH_ENDPOINT` ou `http://127.0.0.1:4318` | Endpoint OTLP |
| `--start-time` | agora | Timestamp inicial em RFC3339 para reproduzir conteúdo |
| `--results-dir` | `bench/results` | Raiz dos resultados |
| `--loadgen` | `bin/otlploadgen` | Binário do gerador |

Pré-requisitos verificados em `run` (quando o cenário os declara):

| Serviço | Variável | Default |
|---|---|---|
| Gateway/OTLP | `OBSERVAKI_BENCH_ENDPOINT` | `http://127.0.0.1:4318` |
| ClickHouse | `OBSERVAKI_BENCH_CLICKHOUSE_URL` | `http://127.0.0.1:8123` |
| VictoriaMetrics | `OBSERVAKI_BENCH_VICTORIAMETRICS_URL` | `http://127.0.0.1:8428` |
| NATS | `OBSERVAKI_BENCH_NATS_ADDR` | `127.0.0.1:4222` |

Códigos de saída: `0` ok/planejado, `1` falha, `3` bloqueado por story futura.

### Resultados

Cada execução grava uma pasta com timestamp UTC e o nome do cenário:

```text
bench/results/20261102T003723Z-throughput-100k/
  run.json          # metadados, checks, comandos, shards e agregado
  resources.json    # amostras de CPU/RAM/disco e uso dos processos do loadgen
  scenario.env      # cópia exata do contrato usado
  loadgen-00.json   # resumo do gerador
  loadgen-00.log    # log estruturado
```

`run.json` inclui commit e estado sujo do git, host (CPU/RAM/SO), versão,
`config_hash`, checks de pré-requisitos, opções resolvidas, comandos exatos e o
agregado de taxa. `resources.json` amostra CPU, memória e disco a cada segundo
(Linux) e registra CPU/RSS acumulados dos processos do gerador. `bench/results/`,
`bench/reports/` e `bench/.tmp/` não são versionados.

### Relatório

O gerador consolida uma ou mais execuções em `report.md` (leitura humana) e
`report.json` (dados brutos):

```bash
# todas as execuções de bench/results, saída com timestamp em bench/reports/
./bench/run.sh report

# somente um cenário
./bench/run.sh report --scenario throughput-100k

# pastas explícitas e destino fixo
./bench/run.sh report \
  --results-dir bench/results \
  --out bench/reports/2026-11-01-throughput \
  bench/results/20261101T120000Z-throughput-100k \
  bench/results/20261101T130000Z-throughput-100k
```

O `report.json` contém `hardware`, `versions`, `config_hash`, métricas
normalizadas, o status de **cada métrica declarada no cenário** e os resultados
brutos (`run.json`, resumos do loadgen, `resources.json` e a medição de payload).
O `report.md` repete a leitura humana, a metodologia, a variância observada e as
limitações.

**Config hash.** SHA-256 sobre o contrato do workload: cenário, taxa, duração,
shards, seeds, protocolo, compressão, dataset/query set e parâmetros do gerador
(mix, batch, payload). Endpoint, host e `--start-time` ficam de fora; o mesmo
workload em outra máquina produz o mesmo hash.

**Métricas e fontes.**

| Métrica | Fonte | Disponibilidade |
|---|---|---|
| `events_per_second`, `bytes_per_second` | agregado dos shards e resumo do loadgen | throughput hoje |
| latência de envio p50/p95/p99 | loadgen (pior shard) | throughput hoje |
| CPU, RAM, disco | `resources.json` (1 Hz) | Linux hoje |
| CPU/RSS do gerador | `getrusage(RUSAGE_CHILDREN)` | Linux hoje |
| `error_rate`, `dropped_events`, backpressure, pacing lag | resumo do loadgen | throughput hoje |
| `compression_ratio` | bytes protobuf / bytes no fio (HTTP) ou payload determinístico | throughput hoje; compressão de storage entra na 3.1-3.3 |
| `ingest_lag_*`, `confirmed_lost_events` | gateway/WAL | a partir da 4.3-4.5; reportado como `not_collected` até lá |
| `query_latency_*`, `rows_read`, `cache_hit_ratio` | Query Service | 3.5; reportado como `not_collected` até lá |
| `series_active`, `neighbor_p95_drift_percent` | gerador de séries | 3.4; reportado como `not_collected` até lá |
| `recovery_seconds`, `dlq_events` | automação de caos | 4.5; reportado como `not_collected` até lá |

O relatório **não inventa** métricas que a stack da Fase 0 ainda não mede: cada
métrica declarada aparece como `collected` ou `not_collected` com o motivo e a
story que a habilita.

**Variância documentada.** A comparação agrupa execuções com o mesmo cenário e
`config_hash`; a variância só é avaliada quando todas são do mesmo commit. As
tolerâncias são:

| Métrica | Tolerância | Mínimo de execuções |
|---|---|---|
| `events_per_second`, `bytes_per_second` | ±5% | 2 |
| latência de envio p50/p95/p99 | ±10% | 3 |
| CPU média, RAM média | ±10% | 2 |
| `compression_ratio` | ±5% | 2 |
| `error_rate` | ±0,1 ponto percentual | 2 |

O ADR-0006 permite uma execução contínua de 1h para o gate de throughput; nesse
caso a variância não é avaliada e o relatório diz isso explicitamente. Execuções
que atravessam commits são listadas, mas não julgadas como repetição. Toda
execução entra no relatório, inclusive `failed` e `blocked`; só `status: ok` e
`mode: run` entram na comparação.

**Custo.** O relatório publica os insumos de volume (eventos/dia, TB/dia bruto,
no fio e no payload de referência). O preço por TB/dia, fator de réplica e horas
de operação continuam pendentes de decisão do fundador (Q-07 do ADR-0006); o
relatório marca o modelo como `not_configured` em vez de inventar preços.

## Datasets de consulta

Os datasets são sintéticos, determinísticos e sem dependência de relógio de
parede no conteúdo. O tamanho é contado em **eventos de telemetria** (o mesmo
universo do workload 60/30/10): 60% métricas vão para o VictoriaMetrics e 40%
logs/traces para o ClickHouse. O alvo de 86B do PRD é 1M/s × 24h = 86,4B
eventos (o PRD arredonda para 86B).

| Dataset | Eventos | Janela | Event-time rate | Seed (wall clock) | Duração do cenário |
|---|---|---|---|---|---|
| 1B/24h | 1.000.000.000 | 24h | 11.574 eventos/s | 1M/s × 1 shard = 16m40s | 20m |
| 10B/7d | 10.000.000.000 | 7d | 16.534 eventos/s | 5M/s × 5 shards = 33m20s | 45m |
| 86B/24h | 86.400.000.000 | 24h | 1.000.000 eventos/s | 5M/s × 5 shards = 4h48m | 5h |

O `--event-time-rate` desacopla o avanço dos timestamps da taxa de envio: o
dataset é semeado mais rápido que o tempo real, mas os eventos ficam
distribuídos na janela de consulta. O plano usa
`--start-time = agora - janela` para que os dados terminem perto do momento da
consulta. A contagem e a distribuição são determinísticas; o instante absoluto
só muda se você fixar `--start-time`.

As **20 consultas representativas** estão em
[`queries/dashboard.tsv`](queries/dashboard.tsv): volume e busca de logs, p95/p99
e erros de traces, mapa de serviços, correlação por `trace_id`, rate/erro/p95 de
métricas e top-10 por cardinalidade. Cada linha declara superfície, sinal,
engine, janela e datasets aplicáveis. O catálogo é o contrato; o SQL/MetricsQL
concreto é vinculado na Story 3.5, quando o schema do ClickHouse e o Query
Service existirem — não há schema inventado aqui. Alvos: p95 < 1s em 1B/24h,
< 3s em 10B/7d e < 3s em 86B/24h com rollups; p99 < 2× p95; no mínimo 3
repetições.

## Build e uso do gerador

```bash
make build
./bin/otlploadgen --help
```

Verificar o workload (não envia nada):

```bash
./bin/otlploadgen --check-payload --start-time 2026-10-31T12:00:00Z
```

Saída de referência com o batch default de 1000 eventos: métricas 37,8 B,
logs 199,0 B, traces 499,2 B e média ponderada 132,3 B/evento (dentro de ±10%
de 134 B). Os números variam com `--batch-size` porque o gzip é medido por lote.

Exemplo gRPC (OTLP em `127.0.0.1:4317`):

```bash
./bin/otlploadgen \
  --endpoint 127.0.0.1:4317 \
  --protocol grpc \
  --compression gzip \
  --rate 100000 \
  --duration 5m
```

Exemplo OTLP/HTTP (sem compressão de transporte, para taxas altas):

```bash
./bin/otlploadgen \
  --endpoint http://127.0.0.1:4318 \
  --protocol http \
  --compression none \
  --rate 1000000 \
  --duration 5m
```

Dry-run, útil para validar taxa e determinismo sem um receptor:

```bash
./bin/otlploadgen --dry-run --rate 100000 --duration 1m
```

O resumo final é um JSON no stdout; logs estruturados vão para o stderr. O
processo sai com código diferente de zero se houver evento não enviado ou
rejeitado pelo receptor.

## Flags principais do gerador

| Flag | Default | Descrição |
|---|---|---|
| `--endpoint` | `127.0.0.1:4317` | `host:port` para gRPC ou URL para HTTP |
| `--protocol` | `grpc` | `grpc` ou `http` |
| `--compression` | `gzip` | `gzip` ou `none` para o transporte |
| `--rate` | `100000` | eventos/s alvo |
| `--event-time-rate` | `--rate` | eventos/s usados para avançar os timestamps; menor que a taxa de envio para semear datasets |
| `--duration` | `5m` | duração da execução |
| `--start-time` | agora | timestamp inicial dos eventos em RFC3339; use um valor fixo para reproduzir |
| `--seed` | `1` | seed determinística |
| `--batch-size` | `1000` | eventos por request OTLP |
| `--workers` | `4` | envios concorrentes |
| `--mix` | `60,30,10` | mix em eventos por 100: métricas, logs, traces |
| `--metrics-addr` | `127.0.0.1:9090` | endereço do `/metrics`; vazio desabilita |
| `--request-timeout` | `10s` | timeout por request |
| `--log-interval` | `5s` | intervalo do log de progresso |
| `--dry-run` | `false` | gera sem enviar |
| `--check-payload` | `false` | mede o payload e sai |

O default de `--start-time` é o horário atual para que os eventos caiam dentro
da janela de skew do gateway (AD-29). Fixe um valor apenas para reproduzir uma
execução e mantenha-o próximo do horário do receptor.

## Métricas

Com `--metrics-addr` ativo, `GET /metrics` expõe:

- `observaki_loadgen_events_generated_total{signal}`
- `observaki_loadgen_events_sent_total{signal}`
- `observaki_loadgen_events_failed_total{signal}`
- `observaki_loadgen_events_rejected_total{signal}`
- `observaki_loadgen_requests_total{signal}`
- `observaki_loadgen_bytes_total{signal}`
- `observaki_loadgen_send_latency_seconds{quantile}` + `_sum`/`_count`
- `observaki_loadgen_target_rate` e `observaki_loadgen_achieved_rate`
- `observaki_loadgen_backpressure_events_total`
- `observaki_loadgen_backpressure_wait_seconds_total`
- `observaki_loadgen_pacing_lag_events_total`

O resumo JSON inclui a configuração, os eventos planejados, a duração real, a
taxa alcançada, a distribuição por sinal, a latência de envio, os erros e o
backpressure.

## Taxa, backpressure e 5M/s

O gerador **não descarta eventos em silêncio**. Se a fila de envio encher, ele
bloqueia, mede a espera e reporta `backpressure_events`/`backpressure_wait`.
Se o processo não conseguir acompanhar o relógio, reporta `pacing_lag_events`.
A taxa alcançada é sempre calculada sobre os eventos realmente gerados.

Em uma máquina de desenvolvimento (22 vCPU, 14 GB), o smoke test da Story 2.1
mostrou:

- 100k/s com gzip e sink HTTP local: ~99,5k/s, sem backpressure;
- 1M/s sem compressão de transporte e sink HTTP local: ~998,4k/s, com
  backpressure de ~1,4 s;
- 5M/s em um único processo não se sustenta com esse payload: a máquina
  satura serialização/HTTP antes da taxa. O alvo de 5M/s do benchmark usa
  **múltiplos processos** (e o cluster de referência da Story 2.4), não um
  único gerador.

Para 1M/s e 5M/s, prefira `--compression none` no transporte e distribua a
taxa entre processos com seeds diferentes. O cenário `throughput-5m` já faz
isso com cinco shards de 1M/s.

O gateway/armazenamento continua responsável pela compressão de armazenamento;
o alvo de 134 B/event é medido pelo `--check-payload`.

## Determinismo

- O mix usa um padrão de 100 slots com a contagem exata; a cada 100 eventos a
  distribuição é 60/30/10.
- IDs de trace/span, timestamps, atributos e janelas de payload derivam do seed
  e do índice do evento.
- O timestamp de evento parte de `--start-time` (default: agora, para respeitar
  a janela de skew do AD-29) e avança por `--event-time-rate` (default: a taxa
  de envio). Para reproduzir uma execução, fixe `--start-time`; fora ele, não
  há dependência de relógio de parede no conteúdo.
- `--check-payload`, `--dry-run` e os testes de `bench/loadgen` verificam
  contagem, distribuição e tamanho comprimido.
- `benchctl check` valida os 12 cenários, o catálogo de 20 consultas e os
  campos obrigatórios de cada família.

## Limitações atuais

- **Rampa e aquecimento:** o protocolo existe em `bench/env/baseline.sh`
  (10 min de warm-up, 5 min de ramp em cinco degraus e 1 h medida), mas o
  gerador executa taxa constante dentro de cada fase. A execução T1 e a
  validação do protocolo no cluster ainda não ocorreram; os números desta fase
  não são evidência do gate G1.
- **Query/cardinalidade/caos:** o contrato e o plano existem; a execução
  depende de ClickHouse/Query Service (3.4/3.5) e da automação de falhas (4.5).
  O relatório marca as métricas dessas famílias como `not_collected` com a story
  que as habilita.
- **Ingest lag:** o relatório mede latência de envio do loadgen, não o lag de
  ingestão durável. `ingest_lag_*` e `confirmed_lost_events` ficam
  `not_collected` até o gateway/WAL (4.3-4.5).
- **Compressão:** `compression_ratio` é a razão no fio (HTTP) ou a razão
  determinística do payload de referência. A compressão de armazenamento do
  ClickHouse/VictoriaMetrics entra nas Stories 3.1-3.4.
- **Recursos:** a amostragem é de host, a 1 Hz, em Linux. Em outras plataformas
  CPU/RAM aparecem como indisponíveis; o disco e o RSS do gerador também podem
  variar por SO.
- **Cardinalidade:** o gerador atual ainda não varia séries além dos 32
  conjuntos de atributos; o gerador de séries e os guardrails entram na 3.4.
- **5M/s em um processo** não se sustenta nesta máquina de desenvolvimento;
  o cenário usa cinco shards.
- O payload sintético não substitui dados reais de design partner; a
  calibração de entropia é uma aproximação do workload de referência.

## Artefatos

`bench/results/`, `bench/reports/` e `bench/.tmp/` não são versionados. O
relatório [`BENCHMARKS.md`](../BENCHMARKS.md) publica a pré-baseline local e
mantém o baseline T1 explicitamente pendente; os artefatos publicados ficam em
[`bench/baselines/`](baselines/). O gerador de `report.md`/`report.json` é o
mecanismo usado pela 2.4. O contrato de cada cenário (`bench/scenarios/`), o
catálogo de consultas (`bench/queries/`), o runner (`bench/run.sh`) e o perfil
T1 (`bench/env/`) são versionados.
