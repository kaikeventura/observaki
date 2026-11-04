# bench/clickhouse/ingest — caminho de ingestão e dedupe

Este diretório mede a Story 3.3: escrever logs no ClickHouse pelo caminho de
ingestão do produto (batches RowBinary, spool durável, checkpoint, dedupe por
`event_id` e token de bloco), em vez de gerar o dataset por SQL como nos spikes
3.1/3.2.

> **Não é evidência do G1.** O run padrão é single-node, local e usa 400k/s por
> 60 s. O alvo de 1M/s por 1 h depende do cluster T1 (P-07) e do gateway/WAL
> (Stories 4.1/4.3). A saturação local fica em ~400k/s; o perfil T1 tem seis
> nós ClickHouse e seis writers.

## O que roda

| Etapa | O que faz |
|---|---|
| Container | `clickhouse/clickhouse-server:26.8` com `bench/clickhouse/config/bench.xml` e o usuário local de `config/users.xml` |
| Schema | Aplica `0001_otel_logs.sql` a `0004_tenancy.sql` (inclui `project_generation`/`authz_epoch`) |
| Integração | `go test -run TestIntegration ./internal/adapters/clickhouse/writer` contra o ClickHouse real (RowBinary, dedupe ao vivo, replay com token) |
| Dedupe | Submete o mesmo batch de 50k eventos duas vezes; espera 50k linhas e 50k duplicatas contabilizadas |
| Recuperação | Mata o writer depois do insert no ClickHouse e antes do checkpoint; reinicia e reenvia o batch do spool; espera o mesmo count e o checkpoint avançado |
| Carga | 400k/s por 60 s, batches de 200k, 12 workers de geração/encode e 4 threads de insert |
| Coleta | `count()`, `uniqExact(event_id)`, parts ativas, compressão, erros de insert e `Too many parts` |
| Relatório | `report.md` e `report.json` em `results/<timestamp>/` |

## Uso

```bash
# smoke local (≈30 s; 100k/s por 5 s)
./bench/clickhouse/ingest/run.sh --smoke

# run publicado da story (400k/s por 60 s; ≈2 min)
./bench/clickhouse/ingest/run.sh --reset \
  --results-dir "$PWD/bench/clickhouse/ingest/results/2026-11-02-local-400k"

# repetir só os checks de contrato
./bench/clickhouse/ingest/run.sh --helpers

# 1M/s local (não passa no host atual; use o T1 para o gate)
./bench/clickhouse/ingest/run.sh --rate 1000000 --duration 60s
```

O script preserva o container ao final para inspeção. Use `--cleanup` para
removê-lo; o diretório `/tmp/opencode/observaki-ch33-ingest` não é apagado pelo
script. `--reset` remove o container e os dados antes de começar.

## Contrato do writer

`internal/adapters/clickhouse/writer`:

- **Batch RowBinary.** Cada batch é uma sequência de linhas no formato
  `FORMAT RowBinary`; o writer não usa JSON no caminho de produção. O insert HTTP
  usa `async_insert=1` + `wait_for_async_insert=1` por padrão (o perfil
  `--sync-insert` usa `async_insert=0`, também aceito pelo AD-18).
- **`event_id` obrigatório.** Tenant, projeto e `event_id` são validados antes
  de qualquer escrita (AD-2). Timestamps fora de ±24 h são clampados para
  `ingested_at` e contados (AD-29).
- **Spool durável.** O batch é escrito em `batch-<seq>.bin` (header JSON +
  payload + SHA-256), com `fsync` do arquivo e do diretório, e só então é
  confirmado ao produtor. O checkpoint (`checkpoint.json`) só avança depois do
  ACK do ClickHouse. No restart, batches acima do checkpoint são reenviados em
  ordem.
- **Dedupe em duas camadas.**
  1. Janela em memória de `(tenant, project, signal, event_id)`; o default do
     produto é 2M identidades. O benchmark usa 200k (≈1 batch) para manter o
     mapa fora do caminho crítico de CPU; o teste de dedupe usa 100k para um
     batch de 50k.
  2. `insert_deduplication_token` estável por batch. Um crash depois do insert
     e antes do checkpoint reenvia o mesmo bloco; o ClickHouse o descarta. O
     resumo HTTP do ClickHouse reporta as linhas processadas, não as linhas
     efetivamente armazenadas, então o benchmark verifica o dedupe por
     `count()`/`uniqExact(event_id)`, não pelo header.
- **Retry e DLQ.** Erros 5xx/429/rede são retentados com backoff; 4xx permanentes
  vão para `dlq/` com um `*.error.json` e o checkpoint avança (o evento não é
  perdido, mas não fica consultável).
- **Métricas.** `MetricsHandler()` e `StatsHandler()` expõem
  `observaki_ingest_*` (submetidos, inseridos, duplicados, recuperados, DLQ,
  pendentes, lag) e o snapshot JSON. O `chloadgen` serve `/metrics`, `/stats` e
  `/healthz` quando `--metrics-addr` é passado.

## Resultado local (2026-11-02)

Ver [`results/2026-11-02-local-400k/report.md`](results/2026-11-02-local-400k/report.md).

| Métrica | Valor |
|---|---:|
| Eventos confirmados | 24.000.000 |
| Throughput | **399.995,5/s** (alvo local 400k/s; ≥95% pass) |
| Lag p95 (evento mais antigo do batch) | **1.283 ms** (alvo < 10.000 ms) |
| Lag p99 | 1.962 ms |
| Linhas armazenadas | 24.000.000 |
| `uniqExact(event_id)` | 24.000.000 |
| Parts ativas ao fim | 18 |
| Compressão | 56,42x |
| Erros de insert | 0 |
| `Too many parts` | 0 |
| Dedupe (50k iguais) | 50.000 linhas, 50.000 duplicatas |
| Recuperação (100k) | 100.000 antes e depois; 1 batch reenviado |

O corpo sintético é repetitivo, por isso a compressão local é muito acima do
alvo de 5x; o número de compressão oficial continua sendo o dos spikes 3.1/3.2.

**Tentativa de 500k/s:** o host sustentou 397–447k/s por 60 s com lag p95 de
12–17 s, então o alvo local foi fixado em 400k/s. O número de 1M/s não é
reivindicado para um único nó local; é o gate do cluster T1.

## Limitações

- Single-node local; sem réplica, NATS, gateway OTLP, Query Service, row
  policies ou caos. O WAL do produto é o NATS (Story 4.3); este spike usa o
  buffer local durável permitido pelo AD-4 no modo single-node.
- A janela de dedupe em memória não sobrevive ao restart. Retries de batches
  não confirmados são cobertos pelo token; duplicatas de produtor que chegam
  depois de um restart e fora da janela ficam para o índice durável da Fase 1
  (FR-4).
- O run usa merges de background normais; o pico de parts foi 20. Não foi
  necessário parar merges como nos runs de 1B.
- O relatório mede o caminho writer → ClickHouse, não o gateway → NATS →
  writer. O lag é o tempo entre o timestamp do evento e o ACK do batch.
- O hardware é menor que o perfil T1 (6 × ClickHouse, 3 × VictoriaMetrics,
  6 × gateway, 4 × NATS, 2 × loadgen).
