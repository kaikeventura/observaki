# bench/clickhouse/traces — Story 3.2 storage spike

Este diretório mede o schema ClickHouse de traces da Story 3.2: compressão,
busca de uma trace completa por `trace_id`, busca por serviço + status em 24 h,
service map básico (nós e arestas) e correlação log↔trace.

> **Não é evidência do G1.** O run padrão é single-node, local e usa SQL para
> gerar o dataset; não passa pelo gateway, NATS, Query Service, row policies ou
> tail sampling. O baseline T1 continua bloqueado por P-07 e pelas Stories
> 3.2–3.5/4.1–4.5.

## O que roda

| Etapa | O que faz |
|---|---|
| Container | Sobe `clickhouse/clickhouse-server:26.8` com o perfil de memória/CPU de `../config/bench.xml` |
| Schema | Aplica `0001_otel_logs.sql` (para a correlação) e `0002_otel_traces.sql` |
| Dataset | Gera `--rows` spans determinísticos em chunks de 50M; cada 10 spans formam uma trace com 9 serviços e arestas pai/filho |
| IDs | `trace_id`, `span_id`, `parent_span_id` e `event_id` vêm de `CityHash64`; parecem aleatórios e não inflam a compressão |
| Trace lookup | Roda `queries/q_trace_by_id.sql` com 1 warm-up e `--runs` repetições |
| Service query | Roda `queries/q_service_status_24h.sql` (serviço + status + 24 h + `LIMIT 100`) |
| Service map | `queries/q_service_map_nodes.sql` e `queries/q_service_map_edges.sql`; as arestas usam uma amostra determinística de 1% das traces e escalam a contagem por 100 |
| Correlação | Insere um log com o `trace_id`/`span_id` de uma trace conhecida e valida log→trace e trace→log |
| AD-21 | Confere que não existe tabela `span_metrics`/RED nem materialized view no ClickHouse |
| Relatório | Publica `report.md`, `report.json`, `runs.jsonl`, `service_map_*.json`, `correlation.json`, `red_check.json` e `run.log` em `results/<timestamp>/` |

## Requisitos

- Docker com pelo menos 8 GiB de RAM disponível para o container.
- `jq`, `awk`, `sha256sum` e GNU `date`.
- **~80 GiB livres** para o run de 1B (o smoke de 10M mediu ~0,6 GiB
  comprimidos; 1B fica na casa de 62 GiB, mais parts e margem). O script aborta
  se o disco livre cair abaixo de 15 GiB.
- Para um smoke sem o custo do 1B:
  `--rows 10000000 --chunk-rows 10000000 --runs 5`.

## Uso

```bash
# smoke local (≈20 s de carga)
./bench/clickhouse/traces/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5 --stop-merges

# run da story (1B spans; ≈40–60 min nesta máquina)
# --stop-merges é necessário no host local: sem ele, as parts inativas dos
# merges ocupam ~2,5× o tamanho da tabela e estouram o disco.
./bench/clickhouse/traces/run.sh --rows 1000000000 --chunk-rows 50000000 --runs 20 --stop-merges

# repetir só a medição sobre uma tabela já carregada
./bench/clickhouse/traces/run.sh --reuse --rows 1000000000 --runs 20

# validar o schema e as consultas com um dataset pequeno e OPTIMIZE
./bench/clickhouse/traces/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5 --optimize
```

O container é preservado ao final para permitir `--reuse`. Use `--cleanup` para
removê-lo; o diretório de dados em `/tmp/opencode/observaki-ch32` não é apagado
pelo script.

## Limpeza (importante)

O run de 1B **não apaga** o container nem os dados. Ao final, ficam:

- container `observaki-ch32` (pode ser removido com `docker rm -f`);
- ~62 GiB em `/tmp/opencode/observaki-ch32` (dataset de 1B spans).

O relatório publicado fica em `results/` e não depende do container. Para
liberar o espaço:

```bash
# 1) remove o container
docker rm -f observaki-ch32

# 2) remove os dados (irreversível); o container é descartável e roda como root
docker run --rm --user root -v /tmp/opencode/observaki-ch32:/data \
  --entrypoint /bin/sh clickhouse/clickhouse-server:26.8 -c 'rm -rf /data/*'

# 3) confira o espaço
df -h /tmp/opencode
```

## Decisões do schema

- **Partição diária por `ingested_at`.** Mesma autoridade de TTL da Story 3.1
  (AD-29). Em volume T1 o writer pode trocar para
  `toStartOfHour(ingested_at)` depois de uma revisão de capacidade; o `ORDER BY`
  e o TTL não mudam.
- **TTL por linha (`retention_days`).** Um projeto de 7 dias e outro de 15 dias
  convivem na mesma tabela; a reconciliação por projeto/row policies fica para a
  Story 3.5.
- **`ORDER BY (tenant_id, project_id, service_name, status_code, timestamp)`.**
  O filtro de serviço + status + janela lê uma fatia estreita; a busca por
  `trace_id` usa o índice `bloom_filter`.
- **Índices.** `bloom_filter` em `trace_id`, `span_id` e `parent_span_id`;
  `set(1000)` em `service_name`. O `ORDER BY` já cobre serviço, mas o índice
  explícito satisfaz o contrato da story e documenta a intenção.
- **`events` e `links` como `Array(Tuple(...))`.** Mantém o evento OTLP
  autocontido na linha e evita a explosão de subcolunas do `Nested`; a
  comparação com `Nested` fica para a Story 3.5, se o Query Service precisar de
  `ARRAY JOIN` em colunas nomeadas.
- **Sem RED no ClickHouse.** Nenhuma MV de `span_metrics`; o service map do
  spike é derivado dos spans brutos e não é a fonte de produção (AD-21). RED é
  calculado no gateway e gravado no VictoriaMetrics.
- **IDs com CityHash64.** `trace_id`/`span_id`/`event_id` parecem aleatórios,
  como no workload real; IDs sequenciais comprimiriam de forma artificial.
- **Chunks ordenados no tempo.** Cada chunk de 50M spans cobre ~1,2 h contíguas
  das 24 h; a consulta de 1 h toca as parts do último chunk, não as parts de
  todos os chunks.

## Consultas

| Arquivo | Uso | Alvo |
|---|---|---|
| `queries/q_trace_by_id.sql` | Trace completa por `trace_id` | p95 < 1 s |
| `queries/q_service_status_24h.sql` | Serviço + status + 24 h + `LIMIT 100` | p95 < 2 s |
| `queries/q_service_map_nodes.sql` | Nós do service map em uma janela | funcional |
| `queries/q_service_map_edges.sql` | Arestas pai/filho, amostra de 1% das traces | funcional |
| `queries/q_log_by_event_id.sql` | Log → `trace_id`/`span_id` | funcional |
| `queries/q_logs_by_trace_id.sql` | Trace → logs | funcional |
| `queries/q_ids_for_index.sql` | Reproduz o `trace_id`/root `span_id` de um índice de trace | suporte |

## Limitações

- Single-node local; sem réplica, row policies, NATS, gateway, tail sampling ou
  Query Service.
- Dataset gerado por SQL, não pelo caminho OTLP real.
- A busca por trace usa uma trace conhecida e cache de página quente depois do
  warm-up.
- O service map usa uma amostra de 1% das traces para limitar o self-join; as
  contagens são estimadas. Em produção, o service map vem do RED do gateway
  (AD-21), não de uma varredura de spans.
- O hardware local é menor que a referência `query-1b` (16 vCPU/64 GB); o
  relatório publica CPU, RAM e disco reais.
- O run de 1B local é evidência de schema, não o número do Gate G1. O número
  oficial continua dependendo do cluster T1 (P-07) e das Stories 3.3/4.1.

## Resultados publicados

Os consolidados ficam em [`results/`](results/) e são referenciados em
[`BENCHMARKS.md`](../../../BENCHMARKS.md).
