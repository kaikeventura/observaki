# Story 3.3 — Caminho de ingestão ClickHouse com dedupe (handoff)

**Status:** AC2 e AC3 concluídos com evidência local; AC1 validado como proxy
local em 400k/s por 60 s. O alvo de 1M/s por 1 h no cluster T1 **não** foi
executado; **não é o Gate G1**.
**Data:** 2026-11-02 (run em UTC de 2026-11-02).
**Escopo:** writer ClickHouse com batches RowBinary, spool durável, checkpoint,
dedupe por `event_id`, token de dedupe de bloco, retry e recuperação após crash.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `internal/adapters/clickhouse/writer/` | Writer de logs/traces: validação AD-2, RowBinary, spool atômico, checkpoint, replay, janela de dedupe, token, retry, DLQ e métricas |
| `internal/adapters/clickhouse/migrations/0003_ingest_dedupe.sql` | `non_replicated_deduplication_window = 10000` em `otel_logs`/`otel_traces` |
| `internal/adapters/clickhouse/writer/*_test.go` | Contrato RowBinary, schema parity, dedupe, replay, DLQ, clamp, concorrência, handlers e integração real com ClickHouse |
| `bench/cmd/chloadgen/` | Gerador de logs em 1M/s, modos `--dedupe-test`, `--recover`, `--crash-after-insert`, `--dry-run` e relatório JSON |
| `bench/clickhouse/ingest/run.sh` | Container, migrations, testes de integração, dedupe, crash/recovery, carga, coleta e relatório |
| `bench/clickhouse/ingest/results/2026-11-02-local-400k/` | `report.md`, `report.json`, `load.json`, `dedupe.json`, `crash.json`, `recovery.json`, `clickhouse.json`, `parts.tsv`, `run.log` |
| `Makefile` / `scripts/smoke.sh` | `bench-clickhouse-ingest`, `bench-clickhouse-ingest-check` e smoke do `chloadgen` |
| `bench/clickhouse/ingest/README.md` | Uso, contrato, decisões e limitações |

## AC1 — Throughput

**Run local:** 400k/s por 60 s, batches de 200k, 12 workers de geração/encode,
4 threads de insert, `async_insert=1` + `wait_for_async_insert=1`.

| Métrica | Valor |
|---|---:|
| Eventos confirmados | 24.000.000 |
| Throughput | **399.994,4/s** (alvo local 400k/s) |
| Lag p95 (evento mais antigo do batch) | **1.403 ms** (alvo < 10.000 ms) |
| Lag p99 | 1.434,6 ms |
| Lag máximo | 1.458,7 ms |
| Linhas armazenadas | 24.000.000 |
| `uniqExact(event_id)` | 24.000.000 |
| Parts ativas ao fim | 18 (pico 18 durante o run) |
| Compressão | 56,31x |
| Erros de insert | 0 |
| `Too many parts` | 0 |
| Retries / DLQ | 0 / 0 |
| Drain após a geração | 0,96 s |

**Sobre 1M/s local:** uma tentativa de 500k/s por 60 s sustentou 397–447k/s
com lag p95 de 12–17 s no host local. O alvo publicado é 400k/s, que é a
capacidade estável do nó local para este payload. O run de 1M/s por 1 h no T1
depende do cluster de 6 nós e de 6 writers, do gateway OTLP e do WAL NATS
(Stories 4.1/4.3) e de **P-07** (provedor, orçamento e credencial IAM). Não é
reivindicado como resultado do G1.

## AC2 — Dedupe

**Teste de batch duplicado:** o mesmo batch determinístico de 50.000 eventos foi
submetido duas vezes. O writer descartou o segundo batch na janela de
`event_id`; a tabela permaneceu com 50.000 linhas e 50.000 `event_id` únicos, e
o contador `duplicate_events` registrou 50.000 duplicatas.

**Teste de replay com token:** o teste de integração
`TestIntegrationReplayDeduplicates` insere um bloco, grava o mesmo batch no
spool com `insert_deduplication_token` e reabre o writer. O ClickHouse descarta
o bloco reenviado; a consulta continua retornando uma linha. O resumo HTTP do
ClickHouse reporta linhas processadas, não linhas armazenadas, então a
verificação autoritativa é `count()`/`uniqExact(event_id)`, não o header
`X-ClickHouse-Summary`.

## AC3 — Recuperação

O `chloadgen --crash-after-insert 1` encerra o processo com exit 137 **depois**
do insert de 100.000 eventos no ClickHouse e **antes** do checkpoint. O spool
fica com um batch; o count antes da recuperação é 100.000. O `--recover` reabre
o writer, reenvia o batch em ordem e avança o checkpoint. O count depois é
100.000 (o bloco foi deduplicado) e o spool fica apenas com o checkpoint.

| Verificação | Antes | Depois |
|---|---:|---:|
| Linhas do tenant de crash | 100.000 | 100.000 |
| Batches no spool | 1 | 0 |
| Checkpoint | não avançado | avançado |
| `RecoveredEvents` | — | 100.000 |

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Formatação | `make fmt` | ok |
| Lint | `make lint` | ok (Go, web, Rust, ADR) |
| Testes | `make test` | ok (inclui writer e integração condicional) |
| Build | `make build` | ok (Go, chloadgen, web, agente) |
| Smoke | `SMOKE_SKIP_CI=1 make smoke` | ok (healthz, API, web, `chloadgen`, cenários e relatório) |
| Integração real | `OBSERVAKI_CLICKHOUSE_URL=... go test -run TestIntegration ./internal/adapters/clickhouse/writer` | ok (RowBinary, dedupe ao vivo, replay) |
| Smoke de ingestão | `./bench/clickhouse/ingest/run.sh --smoke` | 99.997/s; dedupe 50k; recuperação 100k; `status: pass` |
| Run publicado | `./bench/clickhouse/ingest/run.sh --reset --results-dir .../2026-11-02-local-400k` | 399.994,4/s; p95 1,4 s; 24M linhas; 18 parts; `status: pass` |
| Relatório | [`report.md`](../../bench/clickhouse/ingest/results/2026-11-02-local-400k/report.md) | commit `9650da1`, `dirty: false` |

## Decisões e limitações

- **WAL local, não NATS.** O spool de arquivos com `fsync` é o buffer durável
  do modo single-node permitido pelo AD-4; o NATS JetStream é a Story 4.3. O
  contrato de ACK é o mesmo: só confirma depois do `fsync` do batch.
- **Dedupe em memória + token.** A janela de `event_id` é limitada e não
  sobrevive ao restart. O token cobre o replay de batches não confirmados; um
  índice durável para duplicatas de produtor entre restarts é Fase 1 (FR-4).
- **Janela do benchmark.** O run usa 200k identidades (um batch de 200k). O
  default do produto é 2M; o teste de dedupe usa 100k para um batch de 50k. A
  janela menor evita custo de CPU/memória no caminho de 400k/s; a decisão de
  produção precisa equilibrar a janela de retry e o custo.
- **Compressão sintética.** O corpo é repetitivo (56x), acima do alvo de 5x;
  o número oficial de compressão continua sendo o dos spikes 3.1/3.2.
- **Single-node.** Sem réplica, gateway, NATS, Query Service, row policies ou
  caos. O lag medido é writer → ClickHouse, não gateway → WAL → writer.
- **Merges normais.** O run não parou merges; o pico de parts foi 18. O
  container usa o perfil `bench.xml` (8 GiB de teto de memória).
- **Não é G1.** O run de 1M/s por 1 h no T1 e o teste de caos de 15 min
  continuam pendentes.

## Próximo

1. **Story 3.4 — VictoriaMetrics, MetricsQL e cardinalidade.**
2. O baseline T1 da Story 2.4 continua bloqueado por **P-07** e pelas Stories
   3.4/3.5/4.1–4.5.
3. A dedupe durável entre restarts e o NATS entram nas Stories 4.3/4.4 e na
   Fase 1 (FR-4).
