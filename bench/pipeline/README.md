# bench/pipeline — gateway → WAL → writers → storage

Este diretório mede a Story 4.4: o gateway publica no WAL NATS JetStream, os
writers consomem os três streams, deduplicam por `event_id`, escrevem logs e
traces no ClickHouse e métricas no VictoriaMetrics, e só confirmam (ack) a
mensagem do WAL depois que o storage confirma o flush.

> **Não é evidência do G1.** O run padrão é single-node e local. O AC1 pede
> 1M/s por 1 h na frota T1; o T1 continua bloqueado por **P-07** e pelo
> baseline. O harness publica um proxy local de 20k/s e um smoke com caos.

## O que roda

| Etapa | O que faz |
|---|---|
| Containers | NATS 2.15 JetStream, ClickHouse 26.8 e VictoriaMetrics 1.148 descartáveis |
| Schema | Aplica as migrações `0001`–`0004` (logs, traces, dedupe e tenancy) |
| Gateway | `observaki gateway` com dois tenants, tokens e quota; ack só após o JetStream |
| Writer | `observaki writer`: três consumidores duráveis, dedupe, retry, DLQ, `/metrics` |
| Dedupe | Dois loads idênticos (mesmo seed e `start-time`); o segundo deve ser deduplicado |
| Carga | `otlploadgen` HTTP/gzip, 34% métricas / 33% logs / 33% traces |
| Caos | Para o ClickHouse por 3 s e o VictoriaMetrics por 2 s; o writer retenta |
| Verificação | Contagens do ClickHouse, amostras no VictoriaMetrics, lag p95, retries e exit codes |
| Relatório | `report.json` e `report.md` em `results/<timestamp>/` |

## Uso

```bash
# smoke com caos (~1 min; 5k/s por 5 s)
make bench-pipeline-check

# run local de 20k/s por 20 s (~2 min)
make bench-pipeline

# equivalente
./bench/pipeline/run.sh --results-dir "$PWD/bench/pipeline/results/2026-11-03-local-20k"
```

Opções úteis: `--rate`, `--duration`, `--runs`, `--mix`, `--chaos`,
`--no-dedupe`, `--results-dir`. O script não remove os containers em caso de
falha de diagnóstico quando `KEEP_CONTAINERS=1`; `KEEP_WORK=1` preserva o
diretório de spool.

## Contrato de verificação

- **Sem perda confirmada (AC1/AC2):** eventos aceitos pelo load generator ==
  eventos publicados no WAL == eventos aceitos pelo writer ==
  `written + deduped + failed + skipped`.
- **ClickHouse:** `count()` e `uniqExact(event_id)` de `otel_logs` e
  `otel_traces` iguais ao `written` por sinal.
- **VictoriaMetrics:** `sum(count_over_time({__name__="bench.http.server.duration"}[2h]))`
  dentro de 99% do `written` de métricas. A diferença é o coalescimento de
  amostras no mesmo milissegundo, limite do protocolo remote write.
- **Lag:** p95 < 10 s; no run local sem caos fica abaixo de 1 s.
- **Retry (AC2):** com `--chaos`, `writer_retries > 0` e nenhuma perda; o
  writer mantém a mensagem não confirmada no WAL enquanto o storage está fora.
- **Dedupe (AC3):** o segundo load idêntico incrementa
  `writer_deduped_events_total` e não muda as contagens do storage.

## Resultados publicados

| Run | Status | Resultado |
|---|---|---|
| [`2026-11-03-local-20k/`](results/2026-11-03-local-20k/) | pass | 400k eventos a 20k/s por 20 s; lag p95 **0,80 s**; logs/traces exatos; 139.400 amostras no VM |
| [`2026-11-03-local-5k-chaos/`](results/2026-11-03-local-5k-chaos/) | pass | 25k eventos a 5k/s por 5 s com ClickHouse fora 3 s e VM fora 2 s; 8 retries; lag p95 8,1 s; zero perda |

## Limitações

- Single-node; sem T1, replicação, vmauth, Query Service ou outage de 15 min
  (Story 4.5).
- O ClickHouse roda com `async_insert=1` e `wait_for_async_insert=1`; o writer
  confirma o WAL depois do HTTP 2xx, não depois de um `SYSTEM SYNC REPLICA`.
- O `insert_deduplication_token` é derivado dos `event_id`, então um replay
  idêntico após restart é absorvido também pelo bloco do ClickHouse. Um lote
  com fronteira diferente pode duplicar dentro da janela de dedupe em memória.
- Métricas usam o timestamp em milissegundos do remote write; amostras da
  mesma série no mesmo ms podem coalescer.
