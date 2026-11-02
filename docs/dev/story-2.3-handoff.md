# Story 2.3 — Coleta de métricas e gerador de relatório (handoff)

**Status:** implementado e validado; ver evidências abaixo e em
[`STATUS.md`](../../STATUS.md).
**Data:** 2026-11-01.
**Escopo:** coletar CPU/RAM/disco e uso dos processos do gerador durante uma
execução, normalizar as métricas disponíveis e gerar `report.md` +
`report.json` com hardware, versões, `config_hash`, resultados brutos e
variância documentada. Métricas que dependem de gateway, Query Service ou
automação de caos são marcadas como `not_collected`, com a story que as
habilita — o relatório não inventa números.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `bench/internal/resources` | Amostragem a 1 Hz de CPU, RAM e disco; RSS/CPU acumulados dos processos do loadgen; `resources.json` com amostras e resumo; fallback explícito fora do Linux |
| `bench/internal/report` | `Build`/`Write`, normalização por execução, status de cada métrica declarada, comparação de repetições, variância, custo (insumos), `report.md` e `report.json` |
| `bench/cmd/benchctl report` | `--results-dir`, `--out`, `--scenario` e pastas explícitas; integrado ao `bench/run.sh` |
| `bench/internal/runner` | `config_hash`, `versions`, `host.cpu_model/kernel`, `resources.json`, `warnings`; coleta de recursos em throughput/dry-run |
| `bench/loadgen` | `compressed_bytes` por sinal no resumo e em `/metrics`; compressão de transporte mensurável no HTTP |
| `bench/internal/benchversion` | Versão/commit/data nos binários `benchctl` e `otlploadgen` (Makefile com ldflags) |
| `bench/internal/scenario` | `UnmarshalJSON` para `Duration` (o `run.json` já gravava string; o relatório agora lê o contrato) |
| `bench/README.md` | Seção de relatório, fontes de métricas, variância, config hash e limitações |
| `scripts/smoke.sh` | Smoke gera um plano, um relatório e exige `report.md` + `report.json` |
| Testes | Recursos, parser do `/proc`, config hash, resources no run, compressed bytes, relatório, variância, execuções planejadas, dry-run fora da comparação e CLI |

## AC1 — métricas coletadas

Cada métrica declarada no `.env` do cenário aparece em `metric_status` como
`collected` ou `not_collected` com motivo. Para uma execução de throughput:

| Métrica | Fonte | Situação |
|---|---|---|
| events/s, bytes/s | agregado dos shards + resumo do loadgen | coletado |
| latência de envio p50/p95/p99 | loadgen (pior shard) | coletado |
| CPU, RAM, disco | `resources.json` (1 Hz) | coletado em Linux |
| CPU/RSS do loadgen | `getrusage(RUSAGE_CHILDREN)` | coletado em Linux |
| compression ratio | bytes protobuf / bytes no fio (HTTP gzip) | coletado; storage não |
| error rate, dropped events | failed + rejected / generated | coletado |
| backpressure, pacing lag | resumo do loadgen | coletado |
| ingest lag p50/p95/p99 | gateway/WAL | `not_collected` (Story 4.3) |
| confirmed lost events | gateway/WAL | `not_collected` (Stories 4.3–4.5) |
| query p50/p95/p99, rows/bytes read, cache hit | Query Service | `not_collected` (Story 3.5) |
| séries e drift do vizinho | gerador de séries | `not_collected` (Story 3.4) |
| recovery, DLQ | automação de caos | `not_collected` (Story 4.5) |

O plano de `query-1b` foi incluído no relatório de evidência para mostrar a
cobertura declarada das métricas de consulta; a execução continua bloqueada até
a Story 3.5.

## AC2 — formato

`report.json` contém:

- `generator` (benchctl, commit, build, Go);
- `methodology` (workload, definições, tolerâncias, regras de repetição, limitações);
- `environment` (hardware de quem gerou o relatório);
- `runs[]` com `hardware`, `versions`, `config_hash`, `commit`, `dirty`,
  `resolved`, `metrics`, `metric_status` e `raw` (`run.json`, resumos dos shards,
  `resources.json` e medição de payload);
- `comparisons[]` com média, mínimo, máximo, desvio padrão, spread, tolerância,
  avaliação e resultado;
- `cost_model` com os insumos de volume (eventos/dia, TB/dia bruto, no fio e no
  payload de referência) e status `not_configured`;
- `warnings` de variância, recursos ausentes etc.

`report.md` repete a leitura humana, com tabela por execução, cobertura de
métricas, variância, metodologia, limitações e insumos de custo.

**Config hash:** SHA-256 sobre cenário, taxa, duração, shards, seeds, protocolo,
compressão, dataset/query set e parâmetros do gerador (mix, batch, payload).
Endpoint, host e `--start-time` ficam fora. Execuções com o mesmo workload em
máquinas diferentes compartilham o hash.

## AC3 — reprodutibilidade e variância

Tolerâncias documentadas no relatório:

| Métrica | Tolerância | Mínimo de execuções |
|---|---|---|
| events/s, bytes/s | ±5% | 2 |
| latência de envio p50/p95/p99 | ±10% | 3 |
| CPU média, RAM média | ±10% | 2 |
| compression ratio | ±5% | 2 |
| error rate | ±0,1 p.p. | 2 |

O ADR-0006 permite uma execução contínua de 1h no gate de throughput; nesse caso
a variância não é avaliada. Execuções que atravessam commits são listadas, mas
não julgadas como repetição; `dry-run` e `plan` também não entram na comparação.
Toda execução entra no relatório, inclusive `failed` e `blocked`.

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Testes do harness | `go test ./bench/...` | ok |
| Corrida | `go test -race ./bench/internal/resources ./bench/internal/report ./bench/internal/runner` | ok |
| Lint | `~/go/bin/golangci-lint run ./bench/...` | 0 issues |
| Suíte completa | `make ci` | ok |
| Smoke | `SMOKE_SKIP_CI=1 make smoke` | ok, incluindo plano + relatório |
| Throughput real 3×60s | `./bench/run.sh run throughput-100k --duration 60s --endpoint http://127.0.0.1:14318 --results-dir /tmp/opencode/bench-results-story23-final60s` | 99.959,9 / 99.966,6 / 99.967,0 eventos/s (−0,03% a −0,04%); 6.000.000 eventos cada; `status: ok` |
| Relatório | `./bench/run.sh report --results-dir /tmp/opencode/bench-results-story23-final60s --out /tmp/opencode/bench-report-story23-final60s` | 4 runs (3 ok + 1 plano), 1 grupo de comparação, **pass** |
| Variância observada (3×60s) | `report.json` | events/s 0,01%; p50 5,94%; p95 1,22%; p99 4,08%; CPU 7,03%; RAM 3,00%; compressão 0,003%; error rate 0 p.p. |
| Recursos (3×60s) | `report.json` | CPU média 6,24–6,70%; RAM 30,26–31,18%; loadgen child CPU 4,32–4,53%, RSS ~54 MiB; disco 441,1 GiB |
| Compressão | `report.json` | wire 6,70x; payload de referência 8,64x (132,4 gzip B/event); storage ainda não medido |
| Cauda curta (3×5s) | `./bench/run.sh report --results-dir /tmp/opencode/bench-results-story23-final` | `send_latency_p99_ms` 15,23% > 10% → **FAIL** e warning; demonstra que o relatório não esconde variância |
| CPU marginal (3×30s) | `./bench/run.sh report --results-dir /tmp/opencode/bench-results-story23-final30s` | `cpu_percent_mean` 10,21% > 10% → **FAIL** e warning; 60s estabilizou em 7,03% |
| Artefatos | `/tmp/opencode/bench-report-story23-final60s/report.md` e `.json` | 14.944 B e 159.180 B |

Os resultados ficaram em `/tmp/opencode/`, fora do repositório. As 3 execuções de
60s são evidência do mecanismo de coleta/relatório em uma máquina de
desenvolvimento; **não** são evidência do gate G1 (rampa, 1h e cluster T1 são da
Story 2.4). O receptor OTLP local foi um servidor HTTP mínimo que devolve 200,
para medir o caminho HTTP/gzip; não é o gateway do produto.

## Decisões e limitações

- **Não inventar métricas:** ingest lag, perda confirmada, consulta,
  cardinalidade e caos ficam `not_collected` com a story responsável.
- **Compressão:** `compression_ratio` é a razão no fio (HTTP) ou a razão
  determinística do payload; a compressão de armazenamento entra nas Stories
  3.1–3.4. A razão no fio (6,70x) difere da razão do payload isolado (8,64x)
  porque o workload intercala índices dos sinais; ambas estão no relatório.
- **Latência:** o relatório publica latência de envio do loadgen, não lag de
  ingestão; com múltiplos shards usa o pior shard para p50/p95/p99.
- **Recursos:** amostragem de host em Linux; CPU/RAM incluem o receptor local
  quando ele roda na mesma máquina. O RSS/CPU do loadgen é reportado à parte.
- **Custo:** Q-07 do ADR-0006 segue aberta para preço por TB/dia, réplica e
  horas de operação; o relatório publica apenas os insumos de volume.
- **Config hash:** uma alteração de documentação do cenário não muda o hash; uma
  alteração de workload muda.
- **Cauda:** execuções curtas (5s) mostraram p99 fora de ±10%; o relatório
  marcou FAIL. Para o gate, usar a execução de 1h da Story 2.4 e as regras do
  ADR-0006.

## Próximo

Story 2.4 — ambiente reproduzível e baseline publicado: provisionamento
idempotente do cluster T1, execução do baseline e publicação em `BENCHMARKS.md`
com commit, hardware e config.
