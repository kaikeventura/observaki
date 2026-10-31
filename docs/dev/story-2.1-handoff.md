# Story 2.1 — Gerador de carga OTLP (handoff)

**Status:** implementado e validado; ver evidências abaixo e em
[`STATUS.md`](../../STATUS.md).
**Commit de código:** `eaae22c` (`feat(p0-s2.1)`).
**Data:** 2026-10-31.
**Escopo:** gerador determinístico do workload de referência (60% métricas,
30% logs, 10% traces), taxa configurável, backpressure explícito e métricas
próprias. Cenários, relatório e ambiente reproduzível ficam nas Stories
2.2–2.4.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `bench/loadgen/` | Biblioteca: config, mix, payloads, batch, sender gRPC/HTTP, métricas e gerador |
| `bench/cmd/otlploadgen/` | CLI `otlploadgen` |
| `bench/README.md` | Workload, flags, exemplos, métricas e limitações de taxa |
| `Makefile` | `make build` também gera `bin/otlploadgen` |
| Testes | Mix/distribuição, determinismo, tamanho comprimido, compressão ≥5x em logs/traces, taxa, backpressure, erros, métricas, senders HTTP/gRPC, CLI e race detector |

## Workload e determinismo

- Mix em padrão de 100 slots com contagem exata. O default é 60/30/10.
- O gerador usa um seed e o índice global do evento para timestamps, IDs de
  trace/span, atributos e janelas de payload. Duas execuções com o mesmo seed,
  mix, batch e payload geram os mesmos eventos. O timestamp inicial vem de
  `--start-time` (default: agora, dentro da janela de skew do AD-29); fixe-o
  para reproduzir uma execução.
- O payload é sintético e parametrizado por `PayloadSizes`. Métricas usam um
  conjunto de 32 variantes de atributos; logs usam um pool de corpo; traces
  usam um `bench.payload` único por span. Os pools têm entropia controlada
  para manter compressão realista. O atributo `bench.payload` é um
  preenchimento sintético para calibrar o tamanho; ele não representa um label
  de produção.
- O alvo é medido com gzip em lotes de `--batch-size` eventos, não no tamanho
  nominal do payload.

### Medição de referência (`--check-payload`, batch 1000)

| Sinal | Protobuf B/evento | Gzip B/evento | Alvo | Compressão |
|---|---|---|---|---|
| Métricas | 967,0 | 37,8 | 40 B ±10% | 25,6x |
| Logs | 997,2 | 199,0 | 200 B ±10% | 5,01x |
| Traces | 2.644,2 | 499,2 | 500 B ±10% | 5,30x |
| **Ponderado** | — | **132,3** | **134 B ±10%** | — |

## Taxa, backpressure e erros

- O ritmo é controlado em chunks de ~1 ms (`rate/1000` eventos), com relógio
  absoluto. A taxa alcançada é `eventos gerados / duração real`.
- Se a fila de envio enche, o gerador **bloqueia** e registra
  `backpressure_events` e `backpressure_wait_seconds`; não descarta eventos em
  silêncio.
- Se o processo não acompanha o relógio, registra `pacing_lag_events` uma vez
  por episódio de atraso, sem acumular o mesmo gap repetidamente.
- Erros de transporte contam em `events_failed`; rejeições do OTLP
  partial-success contam em `events_rejected`. O CLI sai diferente de zero se
  houver falha ou rejeição.
- `/metrics` expõe taxa alvo/alcançada, contadores por sinal, latência
  p50/p95/p99, bytes, erros, rejeições, backpressure e pacing lag.

## Evidência de execução

| Cenário | Planejado | Gerado | Taxa alcançada | Backpressure | Pacing lag | Veredito |
|---|---|---|---|---|---|---|
| 100k/s dry-run, 5 min | 30.000.000 | 30.000.000 | 99.999,7/s | 0 | 0 | ok (−0,0%) |
| 1M/s dry-run, 5 min | 300.000.000 | 300.000.000 | 999.998,8/s | 43.000 eventos (0,077 s) | 0 | ok (−0,0%) |
| 5M/s sharded (5×1M/s), 5 min | 1.500.000.000 | 1.500.000.000 | 4.999.959,1/s (agregado) | 2.762.000 eventos (2,397 s) | 0 | ok (agregado, −0,0%) |
| 100k/s HTTP gzip, 10 s | 1.000.000 | 1.000.000 | 99.526,9/s | 0 | 0 | ok |
| 1M/s HTTP sem compressão, 10 s | 10.000.000 | 10.000.000 | 998.355,4/s | 1.536.000 eventos (1,379 s) | 0 | ok |

Cada shard de 1M/s ficou entre 999.991,8/s e 999.998,2/s. A distribuição
agregada foi exata: 900M métricas, 450M logs e 150M traces.

Comandos de referência:

```bash
./bin/otlploadgen --dry-run --rate 100000 --duration 5m \
  --start-time 2026-10-31T12:00:00Z --metrics-addr ""
./bin/otlploadgen --dry-run --rate 1000000 --duration 5m \
  --start-time 2026-10-31T12:00:00Z --metrics-addr ""

# 5M/s agregado: cinco processos de 1M/s com seeds diferentes
for seed in 1 2 3 4 5; do
  ./bin/otlploadgen --dry-run --rate 1000000 --duration 5m --seed "$seed" \
    --start-time 2026-10-31T12:00:00Z --metrics-addr "" \
    > "/tmp/shard-${seed}.json" 2> "/tmp/shard-${seed}.log" &
done
wait
```

Smoke com receptor HTTP local (sem armazenamento):

```bash
./bin/otlploadgen --protocol http --endpoint http://127.0.0.1:4318 \
  --compression gzip --rate 100000 --duration 10s \
  --start-time 2026-10-31T12:00:00Z --metrics-addr ""
./bin/otlploadgen --protocol http --endpoint http://127.0.0.1:4318 \
  --compression none --rate 1000000 --duration 10s \
  --start-time 2026-10-31T12:00:00Z --metrics-addr ""
```

## Checks

- `go test -race ./bench/...` — ok.
- `--check-payload --start-time 2026-10-31T12:00:00Z` — 37,8 / 199,0 /
  499,2 B por evento; 132,3 B ponderado.
- `make ci` — ok (incluído no `make smoke`).
- `make smoke` — ok (healthz, API health, web build, versão e payload do loadgen).

## Limitações conhecidas

- **5M/s em um processo** não se sustenta nesta máquina de desenvolvimento
  (22 vCPU, 14 GB): a serialização/HTTP satura antes. O alvo de 5M/s usa
  múltiplos processos com seeds diferentes; o cluster de referência e a
  orquestração entram na Story 2.4.
- **Gzip em 1M/s** é CPU-bound no gerador. Para 1M/5M, use
  `--compression none` no transporte; a compressão de armazenamento continua
  sendo medida pelo backend.
- O payload sintético não substitui dados reais de design partner; a
  calibração de entropia é uma aproximação do workload de referência.

## Próximo

Story 2.2 — cenários nomeados (throughput/query/cardinality/chaos) e
`bench/run.sh`, reutilizando este gerador.
