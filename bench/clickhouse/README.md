# bench/clickhouse — Story 3.1 storage spike

Este diretório mede o schema ClickHouse de logs da Story 3.1: compressão,
latência da consulta `q03` (serviço + janela de 1 h) e expiração dos buckets
de retenção de 7 e 15 dias.

> **Não é evidência do G1.** O run padrão é single-node, local e usa SQL para
> gerar o dataset; não passa pelo gateway, NATS, Query Service ou row policies.
> O baseline T1 continua bloqueado por P-07 e pelas Stories 3.1–3.5/4.1–4.5.

## O que roda

| Etapa | O que faz |
|---|---|
| Container | Sobe `clickhouse/clickhouse-server:26.8` com o perfil de memória/CPU de `config/bench.xml` |
| Schema | Aplica `internal/adapters/clickhouse/migrations/0001_otel_logs.sql` |
| TTL | Cria uma tabela clone, insere 5 linhas com 7/15 dias e força `OPTIMIZE ... FINAL`; espera 3 sobreviventes |
| Dataset | Gera 1B linhas determinísticas em chunks de 50M, com 24 h de event time ordenado no tempo, 2 tenants, 4 projetos, 8 serviços e 90/7/3% INFO/WARN/ERROR |
| Consulta | Roda `queries/q03_filtered_logs.sql` (tenant + projeto + serviço + severidade + 1 h + `LIMIT 100`) com 1 warm-up e 20 repetições |
| Relatório | Publica `report.md`, `report.json`, `runs.jsonl`, `ttl.json` e `run.log` em `results/<timestamp>/` |

## Requisitos

- Docker com pelo menos 8 GiB de RAM disponível para o container.
- `jq`, `awk`, `sha256sum` e GNU `date`.
- **~150 GiB livres** para o run de 1B (≈89 GiB comprimidos + merges). O script
  aborta se o disco livre cair abaixo de 15 GiB.
- Para um smoke sem o custo do 1B: `--rows 10000000 --chunk-rows 10000000`.

## Uso

```bash
# smoke local (≈30 s de carga)
./bench/clickhouse/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5

# run da story (1B linhas; ≈50–70 min nesta máquina)
# --stop-merges é necessário no host local: sem ele, as parts inativas dos
# merges ocupam ~2,5× o tamanho da tabela e estouram o disco.
./bench/clickhouse/run.sh --rows 1000000000 --chunk-rows 50000000 --runs 20 --stop-merges

# repetir só a medição sobre uma tabela já carregada
./bench/clickhouse/run.sh --reuse --rows 1000000000 --runs 20

# validar apenas o TTL
./bench/clickhouse/run.sh --ttl-only
```

O container é preservado ao final para permitir `--reuse`. Use `--cleanup` para
removê-lo; o diretório de dados em `/tmp/opencode/observaki-ch31` não é apagado
pelo script. Para recriar o container com o perfil atual:

```bash
./bench/clickhouse/run.sh --recreate --rows 10000000 --chunk-rows 10000000 --runs 5
```

## Limpeza (importante)

O run de 1B **não apaga** o container nem os dados. Ao final, ficam:

- container `observaki-ch31` (pode ser removido com `docker rm -f`);
- ~78 GiB em `/tmp/opencode/observaki-ch31` (dataset de 1B linhas).

O relatório publicado fica em `bench/clickhouse/results/2026-11-02-local-1b/`
e não depende do container. Para liberar o espaço:

```bash
# 1) remove o container
docker rm -f observaki-ch31

# 2) remove os dados (irreversível); o container é descartável e roda como root
docker run --rm --user root -v /tmp/opencode/observaki-ch31:/data \
  --entrypoint /bin/sh clickhouse/clickhouse-server:26.8 -c 'rm -rf /data/*'

# 3) confira o espaço
df -h /tmp/opencode
```

O `--cleanup` do `run.sh` remove apenas o container; ele **não** apaga o
diretório de dados. Se você rodar outro benchmark, o `--recreate` recria o
container, mas reaproveita o diretório; use a limpeza acima antes de um run novo
se quiser começar do zero.

## Decisões do schema

- **Partição diária por `ingested_at`.** Mantém o número de parts sob controle
  no spike de 1B. Em volume T1 (≈25,9B logs/dia) o writer pode trocar para
  `toStartOfHour(ingested_at)` depois de uma revisão de capacidade; o `ORDER BY`
  e o TTL não mudam.
- **TTL por linha (`retention_days`).** Um projeto de 7 dias e outro de 15 dias
  convivem na mesma tabela. O valor é o do momento da ingestão; mudar a política
  afeta linhas novas e é reconciliado pelo control plane (Story 3.5). O fallback
  de tabelas por bucket continua documentado no addendum.
- **`ORDER BY (tenant_id, project_id, service_name, timestamp)`.** É o filtro
  mais comum do produto e o da `q03`; a janela de 1 h lê uma fatia limitada.
- **Codecs.** `ZSTD(3)` no body/maps/IDs e `Delta(8) + ZSTD(3)` nos timestamps.
  O body tem forma fixa e IDs aleatórios; não há preenchimento artificial para
  inflar a razão.
- **Índices.** `tokenbf_v1` no body e `bloom_filter` em `trace_id`/`span_id`.
  Busca textual ampla e dedupe por `event_id` ficam para as Stories 3.3/3.5.
- **Merges parados no run local de 1B.** O host tem ~126 GiB livres e a tabela
  comprimida ocupa ~83 GiB; com merges de background, as parts inativas chegam
  a ~2,5× o tamanho ativo e o disco estoura. `--stop-merges` congela os merges
  da tabela principal durante a carga; o relatório mede a tabela com parts não
  consolidadas e registra isso nas limitações. No T1 os merges rodam normal e
  são parte do comportamento medido.
- **Chunks ordenados no tempo.** Cada chunk de 50M cobre ~1,2 h contíguas das
  24 h; a consulta de 1 h toca as parts do último chunk, não as parts de todos
  os chunks. Sem isso, cada part cobriria 24 h e a `q03` abriria ~7.000 parts.
  O limite local de parts é elevado para 5.000/6.000 apenas no run sem merges;
  o schema de produção não carrega esse tuning.

## Limitações

- Single-node local; sem réplica, row policies, NATS, gateway ou Query Service.
- Dataset gerado por SQL, não pelo caminho OTLP real.
- A `q03` roda com cache de página quente depois do warm-up; o relatório registra
  que a primeira execução não entra na estatística.
- O hardware local é menor que a referência `query-1b` (16 vCPU/64 GB); o
  relatório publica CPU, RAM e disco reais.
- O run de 1B local é evidência de schema, não o número do Gate G1. O número
  oficial continua dependendo do cluster T1 (P-07) e das Stories 3.3/4.1.

## Resultados publicados

Os consolidados ficam em [`results/`](results/). O relatório de 2026-11-02 é a
primeira execução local de 1B e está referenciado em
[`BENCHMARKS.md`](../../BENCHMARKS.md).
