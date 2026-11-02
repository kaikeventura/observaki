# Story 3.2 — Schema ClickHouse para traces (handoff)

**Status:** AC1–AC4 concluídos com evidência local; **não é o Gate G1**.
**Data:** 2026-11-02 (carga em UTC de 2026-11-02).
**Escopo:** schema de traces, dataset sintético de 1B spans, busca por
`trace_id`, busca por serviço + status em 24 h, compressão, service map básico
e correlação log↔trace em um ClickHouse 26.8 single-node.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `internal/adapters/clickhouse/migrations/0002_otel_traces.sql` | DDL de `otel_traces`: partição/TTL por `ingested_at`, ordem tenant/projeto/serviço/status/tempo, codecs, índices de trace/span/parent e `set` de serviço; sem MV de RED (AD-21) |
| `internal/adapters/clickhouse/schema_test.go` | Teste de contrato do DDL de traces: colunas, partição, TTL, ordem, índices, codecs e ausência de `span_metrics`/MV |
| `bench/clickhouse/traces/load.sql` | Gerador determinístico de 1B spans em 24 h, 10 spans por trace, 9 serviços e arestas pai/filho |
| `bench/clickhouse/traces/queries/` | `q_trace_by_id`, `q_service_status_24h`, nós/arestas do service map, correlação log↔trace e IDs de suporte |
| `bench/clickhouse/traces/run.sh` | Container, schema, carga, medições, service map, correlação, checagem AD-21 e relatório |
| `bench/clickhouse/traces/README.md` | Uso, decisões, limitações e reprodução |
| `Makefile` | Alvos `bench-clickhouse-traces` (1B) e `bench-clickhouse-traces-check` (10M) |
| `bench/clickhouse/traces/results/2026-11-02-local-1b/` | `report.md`, `report.json`, `runs.jsonl`, `service_map_*.json`, `correlation.json`, `red_check.json`, `run.log` |

## AC1 — Schema

O DDL contém `trace_id`, `span_id`, `parent_span_id`, `service_name`,
`operation_name`, `duration_ns`, `status_code`, `attributes`, `events`,
`links`, `event_id`, `ingested_at` e `retention_days`.

- **Engine:** `MergeTree`.
- **Partição:** `toDate(ingested_at)`.
- **Ordem:** `(tenant_id, project_id, service_name, status_code, timestamp)`.
- **TTL:** `ingested_at + INTERVAL retention_days DAY`; `ingested_at` é a
  autoridade (AD-29).
- **Índices:** `bloom_filter` em `trace_id`, `span_id` e `parent_span_id`;
  `set(1000)` em `service_name`.
- **RED:** nenhuma MV de `span_metrics`; a checagem do run confirma zero
  tabelas RED/MV no ClickHouse (AD-21).
- **Teste:** `go test ./internal/adapters/clickhouse/...` valida o contrato sem
  precisar de ClickHouse; roda no `make test`.

## AC2 — Consulta por trace

**Dataset:** 1.000.000.000 spans, 100.000.000 traces de 10 spans, 20 chunks de
50M, 24 h de event time terminando em `2026-11-02 18:00:00`, 2 tenants × 4
projetos × 9 serviços × 10 operações. A carga levou 32 min 31 s
(19:05:20–19:37:51 UTC; ~512 mil spans/s). Os merges de background foram
parados para caber no disco; a tabela foi medida com 1.701 parts ativas.

A trace usada é a de índice 50.000.000 (`tenant-0`/`project-0`),
`80d31d3a6bc766b73806cb2d982d1548`.

| Métrica | Valor |
|---|---:|
| Repetições (após warm-up) | 20 |
| p50 | 308,595 ms |
| p95 | **328,211 ms** (alvo < 1.000 ms) |
| p99 | 332,394 ms |
| Linhas lidas (média) | 6.430.720 |
| Bytes lidos (média) | 238.158.732 B |
| Spans retornados | 10 |

O `bloom_filter` em `trace_id` é o caminho de acesso; a consulta retorna a
trace completa. **Pass local.**

## AC3 — Consulta por serviço e compressão

A consulta `q_service_status_24h` filtra `checkout` + `ERROR` na janela de 24 h
e retorna 100 spans.

| Métrica | Valor |
|---|---:|
| Repetições (após warm-up) | 20 |
| p50 | 169,854 ms |
| p95 | **177,826 ms** (alvo < 2.000 ms) |
| p99 | 180,500 ms |
| Linhas lidas (média) | 13.935.188 |
| Bytes lidos (média) | 279.060.732 B |
| Spans retornados | 100 |

**Compressão:**

| Métrica | Valor |
|---|---:|
| Comprimido | 60.538.124.506 B (56,38 GiB) |
| Sem compressão | 455.244.613.129 B (423,98 GiB) |
| Compressão | **7,52x** (NFR-13 exige ≥ 5x) |
| Bytes comprimidos/span | 60,5 B |
| Partes ativas | 1.701 |

**Pass local** nos dois critérios (p95 < 2 s e compressão ≥ 5x).

## AC4 — Correlação e service map

**Correlação:** um log de `otel_logs` foi inserido com o `trace_id` e o
`span_id` raiz da trace de índice 50.000.000. A navegação log→trace retornou os
10 spans (incluindo o span raiz) e a navegação trace→log retornou o log
`corr-log-1`. **Pass nos dois sentidos.**

**Service map:** janela de 60 min terminando em `2026-11-02 18:00:00`. Os nós
vêm de uma agregação exata dos spans; as arestas usam uma amostra
determinística de 1% das traces (todo o trace entra ou fica fora) e as
contagens são escaladas por 100.

| Métrica | Valor |
|---|---:|
| Nós | 9 (frontend, checkout, payments, inventory, shipping, email, auth, search, fraud) |
| Arestas | 9 (ex.: frontend→checkout, checkout→payments, payments→email, shipping→inventory) |
| Tempo dos nós | 207,6 ms |
| Tempo das arestas | 596,8 ms |
| Amostra | 1% das traces; contagens estimadas |

O mapa é derivado dos spans brutos para validação do spike. Em produção, o RED
e o service map vêm do gateway e do VictoriaMetrics (AD-21); nenhuma MV de
`span_metrics`/RED existe no ClickHouse (checagem do run: 0 tabelas, 0 MVs).

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Formatação | `make fmt` | ok |
| Lint | `make lint` | ok (Go, web, Rust, ADR) |
| Testes | `make test` | ok (inclui `internal/adapters/clickhouse`) |
| Build | `make build` | ok (Go, web, agente) |
| Smoke | `make smoke` | ok (healthz, API, web, loadgen, cenários, relatório) |
| Contrato do schema | `go test ./internal/adapters/clickhouse/...` | ok |
| Smoke de 10M | `./bench/clickhouse/traces/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5 --optimize` | 7,40x; trace lookup p95 17,8 ms; serviço p95 6,3 ms; service map 9 nós/9 arestas; correlação ok |
| Run de 1B | `./bench/clickhouse/traces/run.sh --rows 1000000000 --chunk-rows 50000000 --runs 20 --stop-merges --base "2026-11-02 18:00:00"` | 7,52x; trace lookup p95 328,2 ms; serviço p95 177,8 ms; service map 9/9; correlação ok; relatório `status: pass`, commit `f6d6c41`, `dirty: false` |
| Relatório | [`bench/clickhouse/traces/results/2026-11-02-local-1b/report.md`](../../bench/clickhouse/traces/results/2026-11-02-local-1b/report.md) | `status: pass`; hardware real, 1.701 parts, merges parados |

## Decisões e limitações

- **Single-node local.** Não é o cluster T1; o baseline oficial continua
  bloqueado por P-07 e pelas Stories 3.2–3.5/4.1–4.5.
- **Dataset por SQL.** A carga não passa pelo gateway OTLP, NATS ou Query
  Service; mede o schema e o motor, não o caminho de ingestão durável.
- **Service map de spike.** As arestas usam uma amostra determinística de 1%
  das traces e contagens estimadas; o service map de produção vem do RED do
  gateway no VictoriaMetrics (AD-21), não de uma varredura de spans.
- **Merges parados.** O run local de 1B para os merges da tabela principal para
  caber no disco e mede a tabela com 1.701 parts não consolidadas; no T1 os
  merges rodam normal. A carga de 1B levou 32 min 31 s e deixou 66,6 GiB
  livres no host.
- **Cache quente.** O warm-up não entra nas 20 repetições; a busca por trace usa
  uma trace conhecida e a página está quente.
- **Hardware menor que a referência.** O host local tem 22 CPUs/15 GiB; o
  cenário `query-1b` assume 16 vCPU/64 GB. O relatório publica o hardware real.
- **Sem tenancy completa.** Não há row policies, réplica, quotas, caos ou Query
  Service; isso é 3.3/3.5/4.5.
- **Não é G1.** O número oficial de 1M/s por 1 h, lag p95 < 10 s e zero perda
  continua dependendo do T1 e do caminho de ingestão.

## Próximo

1. **Story 3.3 — caminho de ingestão ClickHouse com dedupe.**
2. O baseline T1 da Story 2.4 continua bloqueado por **P-07** e pelas Stories
   3.2–3.5/4.1–4.5.
3. A reconciliação de retenção por projeto, row policies e isolamento ficam
   para a Story 3.5.
