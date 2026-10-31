# bench/ — harness de benchmark

O harness reproduzível da Fase 0. O primeiro componente é o gerador de carga
OTLP da Story 2.1: um binário Go que produz o workload de referência com mix
determinístico, taxa configurável, backpressure explícito e métricas próprias.

Os cenários, o relatório e o ambiente reproduzível entram nas Stories 2.2–2.4.

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

## Build e uso

```bash
make build
./bin/otlploadgen --help
```

Verificar o workload (não envia nada):

```bash
./bin/otlploadgen --check-payload --start-time 2026-10-31T12:00:00Z
```

Saída de referência com o batch default de 1000 eventos: métricas 37,8 B,
logs 199,0 B, traces 499,2 B e média ponderada 132,3 B/evento (dentro de
±10% de 134 B). Os números variam com `--batch-size` porque o gzip é medido por
lote.

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

## Flags principais

| Flag | Default | Descrição |
|---|---|---|
| `--endpoint` | `127.0.0.1:4317` | `host:port` para gRPC ou URL para HTTP |
| `--protocol` | `grpc` | `grpc` ou `http` |
| `--compression` | `gzip` | `gzip` ou `none` para o transporte |
| `--rate` | `100000` | eventos/s alvo |
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

- 100k/s com gzip e sink HTTP local: ~99,7k/s, sem backpressure;
- 1M/s sem compressão de transporte e sink HTTP local: ~999,6k/s, com
  backpressure de ~0,5 s;
- 5M/s em um único processo não se sustenta com esse payload: a máquina
  satura serialização/HTTP antes da taxa. O alvo de 5M/s do benchmark usa
  **múltiplos processos** (e o cluster de referência da Story 2.4), não um
  único gerador.

Para 1M/s e 5M/s, prefira `--compression none` no transporte e distribua a
taxa entre processos com seeds diferentes:

```bash
for seed in 1 2 3 4 5; do
  ./bin/otlploadgen --protocol http --endpoint http://127.0.0.1:4318 \
    --compression none --rate 1000000 --duration 5m --seed "$seed" \
    > "shard-${seed}.json" 2> "shard-${seed}.log" &
done
wait
```

O gateway/armazenamento continua responsável pela compressão de armazenamento;
o alvo de 134 B/event é medido pelo `--check-payload`.

## Determinismo

- O mix usa um padrão de 100 slots com a contagem exata; a cada 100 eventos a
  distribuição é 60/30/10.
- IDs de trace/span, timestamps, atributos e janelas de payload derivam do seed
  e do índice do evento.
- O timestamp de evento parte de `--start-time` (default: agora, para respeitar
  a janela de skew do AD-29) e avança pelo índice. Para reproduzir uma execução,
  fixe `--start-time`; fora ele, não há dependência de relógio de parede no
  conteúdo.
- `--check-payload`, `--dry-run` e os testes de `bench/loadgen` verificam
  contagem, distribuição e tamanho comprimido.
- Não há dependência de relógio de parede para o conteúdo dos eventos.

## Artefatos

`bench/results/` e `bench/.tmp/` não são versionados. O relatório
`BENCHMARKS.md` e o JSON bruto entram na Story 2.3; o provisionamento e o
baseline entram na Story 2.4.
