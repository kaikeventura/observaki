# Story 2.2 — Cenários de benchmark e datasets de consulta (handoff)

**Status:** implementado e validado; ver evidências abaixo e em
[`STATUS.md`](../../STATUS.md).
**Data:** 2026-11-01.
**Escopo:** os 12 cenários nomeados (throughput/query/cardinalidade/caos), o
runner `bench/run.sh`, o catálogo das 20 consultas e a documentação de hardware,
duração, métricas e saída esperada. A execução de query/cardinalidade/caos fica
bloqueada até as Stories 3.4/3.5/4.5, quando os backends existirem.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `bench/scenarios/*.env` | 12 cenários declarativos: 3 throughput, 3 query, 3 cardinalidade, 3 caos |
| `bench/internal/scenario` | Parser e validação estrita (chaves por família, unidades, catálogo de métricas) |
| `bench/internal/runner` | Execução, pré-requisitos, shards, agregação de taxa e `run.json` |
| `bench/cmd/benchctl` + `bench/run.sh` | `list`, `check`, `queries`, `run` com `--plan`/`--dry-run` |
| `bench/queries/dashboard.tsv` | 20 consultas representativas (logs, traces, métricas) |
| `bench/queries/README.md` | Contrato do catálogo e vínculo com o schema na Story 3.5 |
| `bench/README.md` | Matriz de cenários, datasets, execução, resultados e limitações |
| `bench/loadgen` | `--event-time-rate` para semear datasets rápidos com timestamps realistas |
| `Makefile` / `scripts/smoke.sh` | `bin/benchctl` no build e `benchctl check` no smoke |
| Testes | Parser/validação, catálogo, matemática dos datasets, runner, shards, plano, bloqueio e CLI |

## Cenários

| Família | Cenários | Parâmetro |
|---|---|---|
| Throughput | `throughput-100k`, `throughput-1m`, `throughput-5m` | 100k/1M/5M eventos/s; 5M em 5 shards |
| Query | `query-1b`, `query-10b`, `query-86b` | 1B/24h, 10B/7d, 86,4B/24h |
| Cardinalidade | `cardinality-100k`, `cardinality-1m`, `cardinality-10m` | 100k/1M/10M séries, 2 tenants |
| Caos | `chaos-clickhouse`, `chaos-nats`, `chaos-gateway` | falha + janela de 15m/5m/5m |

Cada arquivo declara hardware, duração, pré-requisitos, métricas e veredito. O
`benchctl check` também exige que os 12 nomes estejam presentes, para que a
matriz do AC1 não regrida.

## Datasets de consulta (Q-02 do ADR-0006)

O dataset é contado em eventos de telemetria e semeado com o gerador
determinístico. O `--event-time-rate` separa o avanço dos timestamps da taxa de
envio; assim o dataset cobre a janela de consulta mesmo sendo semeado mais
rápido que o tempo real.

| Dataset | Eventos | Janela | Event-time rate | Seed | Duração |
|---|---|---|---|---|---|
| 1B/24h | 1.000.000.000 | 24h | 11.574/s | 1M/s × 1 = 16m40s | 20m |
| 10B/7d | 10.000.000.000 | 7d | 16.534/s | 5M/s × 5 = 33m20s | 45m |
| 86B/24h | 86.400.000.000 | 24h | 1M/s | 5M/s × 5 = 4h48m | 5h |

O plano fixa `--start-time = agora - janela`; a contagem/distribuição é
determinística por seed. O catálogo tem 20 consultas com superfície, sinal,
engine, janela e datasets; o SQL/MetricsQL concreto é vinculado na Story 3.5,
sem inventar schema antes da 3.1/3.2.

## Runner e resultados

```bash
./bench/run.sh list
./bench/run.sh check
./bench/run.sh queries
./bench/run.sh run throughput-100k --plan
./bench/run.sh run throughput-100k --duration 10s --endpoint http://127.0.0.1:4318
./bench/run.sh run throughput-5m --duration 10s --dry-run
./bench/run.sh run query-1b --plan
```

Cada execução cria `bench/results/<UTC>-<cenário>/` com `run.json` (commit,
host, checks, comandos, resolução de overrides, shards e agregado),
`scenario.env` (cópia do contrato) e os artefatos brutos do gerador. Códigos de
saída: 0 ok/planejado, 1 falha, 3 bloqueado por story futura.

### Matriz de execução

| Família | `run` | `--plan`/`--dry-run` | Dependência |
|---|---|---|---|
| Throughput | executa e valida ±5% | plano ou loadgen sem rede | — |
| Query | bloqueia com `status: blocked` e exit 3 | grava plano de seed + consultas | 3.4/3.5 |
| Cardinalidade | bloqueia com `status: blocked` e exit 3 | grava plano do gerador de séries | 3.4 |
| Caos | bloqueia com `status: blocked` e exit 3 | grava carga + injeção de falha | 4.5 |

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Catálogo e contratos | `./bench/run.sh check` | 12 cenários válidos; 3 query sets com 20 consultas |
| Invariantes | `go test ./bench/...` | ok (parser, catálogo, matemática, runner, shards, CLI) |
| Lint | `golangci-lint run ./bench/...` | 0 issues |
| Suíte completa | `make ci` | ok |
| Smoke | `SMOKE_SKIP_CI=1 make smoke` | ok, incluindo `benchctl check` |
| Throughput real | `./bench/run.sh run throughput-100k --duration 10s --rate 100000 --endpoint http://127.0.0.1:14318` | 99.776,8/s (−0,22%), 1.000.000 eventos, `status: ok` |
| 5M/s sharded | `./bench/run.sh run throughput-5m --duration 10s --dry-run` | 4.999.569,8/s (−0,01%), 50.000.000 eventos, 5 shards, `status: ok` |
| Plano de query | `./bench/run.sh run query-1b --plan` | `--event-time-rate 11574`, seed 16m40s, 20 consultas × 3 |
| Bloqueio correto | `./bench/run.sh run query-1b` | `status: blocked`, exit 3, `run.json` gravado |

Os resultados acima ficaram em `/tmp/opencode/bench-results/` (fora do repo,
como esperado); `bench/results/` é ignorado pelo git.

## Decisões e limitações

- **Escopo de execução:** os cenários de query/cardinalidade/caos são contratos
  executáveis quando os backends chegarem; o runner valida pré-requisitos,
  registra o plano e falha fechado (exit 3), em vez de simular um resultado.
- **Hardware:** os mínimos estão declarados por cenário; o perfil T1 e o cluster
  de referência são da Story 2.4 (Q-01). Nada aqui é evidência do gate G1.
- **Rampa/aquecimento:** o gerador ainda executa taxa constante; a rampa do
  ADR-0006 será definida na Story 2.4.
- **Cardinalidade:** o gerador atual não varia séries além dos 32 conjuntos de
  atributos; o gerador de séries entra na Story 3.4.
- **5M/s:** usa cinco processos de 1M/s; um processo único satura esta máquina.
- **Sem schema inventado:** o catálogo de consultas é semântico; o SQL/MetricsQL
  é vinculado na Story 3.5.

## Próximo

Story 2.3 — coleta de métricas por cenário e gerador de relatório
(`report.md` + `report.json`, com hardware, versões, hash de config e resultados
brutos).
