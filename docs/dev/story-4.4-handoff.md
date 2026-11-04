# Story 4.4 — Writers para ClickHouse e VictoriaMetrics (handoff)

**Status:** AC2 e AC3 concluídos com evidência local; AC1 validado como proxy
local de 20k/s (lag p95 0,80 s) e de 5k/s sob caos (lag p95 7,97 s, 8
retries). O alvo de 1M/s por 1 h no cluster T1 **não** foi executado; **não é
o Gate G1**.
**Data:** 2026-11-04 (UTC).
**Commit:** `0699eed`.
**Epic:** P0-E4 — Gateway OTLP e ingestão.
**ADs:** AD-4 (ack após durabilidade), AD-5, AD-9, AD-18, AD-20, AD-21, AD-22,
AD-29, AD-31, AD-33; ADR-0002/0003/0004.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `internal/adapters/walconsumer/consumer.go` | Três consumidores duráveis JetStream (traces/logs/metrics), fetch em lote, decode/validação do envelope, agrupamento por tenant/projeto/geração/epoch/sinal, ack só após o flush, retry exponencial, DLQ e shutdown gracioso |
| `internal/adapters/walconsumer/storage.go` | Conversão OTLP → linhas ClickHouse / séries remote write, dedupe por `event_id` + geração/epoch, espera pelo `Outcome` do writer, DLQ e contadores |
| `internal/adapters/walconsumer/metrics.go` | Séries Prometheus por tenant/sinal: accepted, written, deduped, failed, skipped, retries, lag p50/p95/p99, fila e DLQ |
| `internal/adapters/walconsumer/server.go` | `/healthz`, `/readyz`, `/metrics` e `/stats` do writer |
| `internal/adapters/victoriametrics/` | Cliente Prometheus remote write (protobuf + snappy), retry exponencial, classificação permanente/transitória e chunking por séries |
| `internal/adapters/victoriametrics/convert.go` | OTLP gauge/sum/histograma/resumo/exponential histogram → séries; labels sanitizados + tenant/projeto; agregação de amostras |
| `internal/adapters/clickhouse/writer/convert.go` | OTLP logs/traces → `LogRecord`/`SpanRecord`, severidade, span kind/status, eventos/links, IDs hex e fallback de timestamp |
| `internal/adapters/clickhouse/writer/{record,http,writer}.go` | 17 colunas explícitas (inclui `project_generation`/`authz_epoch`), `insert_deduplication_token` derivado dos `event_id`, `OnDone` terminal (ack ou DLQ), retry e spool |
| `internal/promrw/` | Codec remote write promovido de `bench/internal/promrw` para uso do produto e do harness |
| `internal/otlputil/` | Conversão determinística de `AnyValue`/atributos, service.name e nomes de label |
| `internal/config/config.go` + `.env.example` | Configuração `OBSERVAKI_WRITER_*`, `OBSERVAKI_CLICKHOUSE_*` e `OBSERVAKI_VICTORIAMETRICS_*` validada |
| `cmd/observaki/main.go` | Novo comando `observaki writer` |
| `bench/pipeline/run.sh` + `README.md` | Harness end-to-end NATS + ClickHouse + VictoriaMetrics + gateway + writer + loadgen, com caos e dedupe |
| `bench/cmd/...` e `bench/internal/...` | Imports do `promrw` atualizados; o harness 3.3 aplica a migração 0004 |
| `docs/dev/story-4.4-handoff.md` | Este documento |

## Contrato dos writers

### Consumo e ack

- Um consumidor durável por sinal (`observaki-writer_traces`, `_logs`,
  `_metrics`), `AckExplicit`, `MaxAckPending` 4096, `AckWait` 30 min e entrega
  de todas as mensagens. Várias instâncias do writer compartilham o mesmo
  consumidor (queue semantics).
- O writer busca até 64 mensagens, mescla os requests por
  tenant/projeto/geração/epoch/sinal e grava **um** lote por grupo. Só depois
  do `OnDone` do writer ClickHouse (insert confirmado ou DLQ) ou do HTTP 2xx
  do VictoriaMetrics ele chama `msg.Ack()`.
- Se o storage está fora, a mensagem permanece não confirmada no WAL e o
  writer retenta com backoff exponencial (500 ms → 30 s por padrão). O WAL
  retém por `MaxAge`/`MaxBytes`; nenhum evento confirmado é perdido dentro
  dessa janela.
- Erro permanente de conversão ou de insert vai para o stream
  `OBSERVAKI_DLQ` (`observaki.dlq.v1.<sinal>`) com headers de causa e
  mensagem original; só então a mensagem é confirmada. O counter
  `observaki_writer_failed_events_total` registra os eventos.
- `/readyz` reflete NATS/consumidores, **não** a disponibilidade do storage:
  durante um outage o writer continua vivo e retentando, em vez de reiniciar
  em loop.

### Dedupe

- O writer mantém uma janela FIFO em memória por sinal, com
  `OBSERVAKI_WRITER_DEDUPE_WINDOW` (default 500k).
- A chave é `tenant | project | geração | authz_epoch | event_id`. Um move de
  projeto ou uma revogação de epoch não deduplica contra o escopo antigo
  (AD-20).
- Logs/traces também usam `insert_deduplication_token` derivado dos
  `event_id` (e não da sequência local), então um replay idêntico após restart
  é absorvido pelo bloco do ClickHouse.
- Métricas não têm token no remote write; a janela em memória e a
  idempotência de amostras do VictoriaMetrics cobrem o retry. Um replay após
  restart pode reescrever a mesma amostra, o que é inofensivo.
- O produto é **at-least-once**. Um índice durável cross-restart é assunto da
  Fase 1 (FR-4), como já registrado na Story 3.3.

### Tenancy e schema

- O INSERT do ClickHouse usa uma lista explícita de 17 colunas:
  as 15 do schema original mais `project_generation` e `authz_epoch`.
  As linhas carregam a geração/epoch do envelope (AD-20); valores ausentes
  caem no default 1.
- O envelope v1 é o contrato; campos novos usam números novos e o leitor
  ignora desconhecidos (AD-22).
- Logs e traces vão para o ClickHouse; métricas vão para o VictoriaMetrics.
  Não há RED no ClickHouse (AD-21).

### Observabilidade

- `observaki_writer_accepted_events_total{tenant_id,signal}`
- `observaki_writer_written_events_total{tenant_id,signal}`
- `observaki_writer_deduped_events_total{tenant_id,signal}`
- `observaki_writer_failed_events_total{tenant_id,signal}`
- `observaki_writer_skipped_events_total{tenant_id,signal}`
- `observaki_writer_retries_total{tenant_id,signal}`
- `observaki_writer_lag_seconds{tenant_id,signal,quantile}` (end-to-end desde
  `received_at` do gateway até o ack do storage)
- `observaki_writer_consumer_pending_messages{signal}`,
  `observaki_writer_in_flight_events`, `observaki_writer_dlq_messages_total`
  e `observaki_writer_flush_duration_seconds`.

## AC1 — End-to-end

O AC1 pede 1M/s por 1 h no T1, com contagem armazenada == aceita e lag p95 <
10 s. O T1 continua bloqueado por **P-07**.

**Run publicado de 20k/s por 20 s:**
`bench/pipeline/results/2026-11-03-local-20k/`.

| Métrica | Valor |
|---|---:|
| Eventos ofertados (runs medidas) | 400.000 |
| Eventos no WAL | 400.000 |
| Aceitos pelo writer (todas as fases) | 420.000 |
| Escritos | 406.700 |
| Deduplicados | 13.300 |
| Falhas | 0 |
| Logs no ClickHouse | 135.300 (135.300 únicos) |
| Traces no ClickHouse | 132.000 (132.000 únicos) |
| Amostras no VictoriaMetrics | 139.400 (64 séries) |
| Lag p50 / p95 / p99 | **0,515 s / 0,802 s / 0,887 s** |
| Exit codes gateway/writer | 0 / 0 |

A fase de dedupe usa dois loads idênticos (mesmo seed e `start-time`): o
segundo é deduplicado. Os 13.300 deduplicados incluem 10.000 da fase e 3.300
traces que reutilizam `trace_id`/`span_id` determinísticos do load generator
com o mesmo seed; o storage trata isso como o mesmo span.

**Run com caos:** `bench/pipeline/results/2026-11-03-local-5k-chaos/`. 25.000
eventos medidos a 5k/s por 5 s, ClickHouse parado 3 s e VictoriaMetrics 2 s:
26.675 escritos, 3.325 deduplicados, **8 retries**, zero falhas, lag p95
7,97 s, contagens exatas nos dois storages (9.350 amostras de métricas, igual ao writer).

## AC2 — Retry

- O cliente VictoriaMetrics e o writer ClickHouse classificam erros
  transitórios (rede, 408, 429, 5xx) e usam backoff exponencial com teto.
  Erros 4xx permanentes vão para a DLQ.
- O harness para o ClickHouse e o VictoriaMetrics durante a carga; o writer
  mantém as mensagens não confirmadas no WAL, retenta e fecha as contagens
  depois da recuperação. O run publica **8 retries, 0 falhas e 0 perda**.
- `TestConsumerRetriesTransientFailure` cobre o retry no consumidor;
  `TestWriteRetriesTransientErrors` cobre o remote write; os testes do writer
  ClickHouse cobrem o retry do insert e a DLQ (`TestOnDoneReportsDLQ`).

## AC3 — Observabilidade

- O `/metrics` do writer expõe accepted, written, failed, retried, deduped e
  lag por tenant/sinal, além da fila do consumidor e da DLQ.
- `/stats` devolve o mesmo snapshot em JSON para o harness e operadores.
- O harness só considera o run concluído quando
  `accepted == written + deduped + failed + skipped` em todos os sinais.

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Formatação | `make fmt` | ok |
| Lint | `make lint` | ok (0 issues) |
| Testes | `make test` | ok |
| Race | `go test -race ./internal/adapters/walconsumer ./internal/adapters/victoriametrics ./internal/adapters/nats` | ok |
| Build | `make build` | ok |
| Smoke | `SMOKE_SKIP_CI=1 make smoke` | ok |
| Ingestão 3.3 com o schema de 17 colunas | `make bench-clickhouse-ingest-check` | pass (100k/s, dedupe e recovery) |
| Pipeline 20k/s | `./bench/pipeline/run.sh --results-dir bench/pipeline/results/2026-11-03-local-20k` | pass; 0,80 s p95; contagens exatas |
| Pipeline com caos | `./bench/pipeline/run.sh --smoke` | pass; 8 retries; zero perda |
| Pipeline smoke (Make) | `make bench-pipeline-check` | pass |

## Decisões e limitações

1. **Ack em dois estágios.** O writer ClickHouse grava o lote no spool durável
   (`fsync`) antes de confirmar; o consumidor espera o `OnDone` do insert (ou
   da DLQ) antes de confirmar o NATS. O spool é a proteção contra crash entre
   o insert e o ack.
2. **Replay de startup.** `OpenLogs`/`OpenTraces` reproduzem o spool antes de
   aceitar lotes novos, para preservar a ordem do checkpoint. Se o ClickHouse
   estiver fora quando o writer reinicia, o processo aguarda a recuperação
   antes de abrir o `/readyz`; o WAL mantém os dados e o spool é durável. Um
   replay assíncrono fica para a Fase 1, quando o control plane e o sizing do
   T1 definirem o orçamento de startup.
3. **Colunas explícitas.** O INSERT não depende da ordem física da tabela:
   usa a lista de 17 colunas. Isso permite adicionar colunas com default sem
   quebrar o writer e garante que `project_generation`/`authz_epoch` sejam
   persistidos.
4. **Stable token.** O `insert_deduplication_token` deriva dos `event_id`,
   não da sequência local; um replay após restart reusa o mesmo bloco lógico.
   A janela de dedupe do ClickHouse é 10.000 blocos (migração 0003).
5. **DLQ do WAL.** Erros permanentes de conversão/insert são publicados em
   `OBSERVAKI_DLQ`; a mensagem original só é confirmada depois do publish.
   O stream tem retenção de 7 dias e 8 GiB por padrão.
6. **Métricas e ms.** O remote write usa milissegundos; amostras da mesma
   série no mesmo ms podem coalescer. O harness aceita 99% de tolerância e
   documenta a diferença.
7. **Sem T1.** O host local não sustenta 1M/s com gateway, NATS, ClickHouse,
   VictoriaMetrics e load generators juntos. O AC1 de 1M/s/1h, o outage de
   15 min e o caos de NATS/gateway são 4.5 + T1 (P-07).
8. **Sem exactly-once.** O produto é at-least-once; a dedupe é uma janela
   limitada em memória mais o token estável do ClickHouse. Um índice durável
   é Fase 1.

## Próximo

1. **Story 4.5 — caos + go/no-go:** ClickHouse fora 15 min, falha de
   gateway/NATS, backpressure, zero perda confirmada e recomendação Go vs
   Rust/Collector.
2. **T1/P-07:** sizing do WAL e do spool, 1M/s por 1 h, lag p95 e custo.
3. **Fase 1:** índice durável de dedupe (FR-4), Query Service e vmauth.
