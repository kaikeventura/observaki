# Story 3.5 — Consulta, retenção e row policies (handoff)

**Status:** AC1–AC3 validados em run local single-node; **não é o Gate G1**.
**Data:** 2026-11-02 (run em UTC de 2026-11-03).
**Commit:** harness da Story 3.5 (working tree no momento do run; o commit de
publicação é o desta sessão).
**Escopo:** as 20 consultas de dashboard no dataset de 1B eventos, isolamento
por tenant com row policies e `authz_epoch`, retenção 7/15/30 e o tier de
86B/24h em rollups de 1 minuto.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `internal/adapters/clickhouse/migrations/0004_tenancy.sql` | `project_generation`/`authz_epoch` nas tabelas, `tenant_scope`, role `observaki_query` e row policies por tenant |
| `internal/adapters/clickhouse/schema_test.go` | Teste de contrato da migração de tenancy |
| `bench/clickhouse/queries/q01–q15,q20` | 16 consultas SQL do catálogo `bench/queries/dashboard.tsv` |
| `bench/clickhouse/queries/catalog.tsv` | Catálogo id/signal/engine/window/target/file |
| `bench/clickhouse/query/load_logs.sql`, `load_traces.sql` | Geradores do dataset de 1B eventos (300M logs + 100M spans), com pod/host, timeout, retenção 7/15/30 e epoch |
| `bench/clickhouse/query/run.sh` | Container, carga, consultas, isolamento, retenção, rollup, métricas e relatório |
| `bench/clickhouse/query/rollup/` | 9 consultas agregadas em rollup de 1 minuto para o tier 86B/24h |
| `bench/clickhouse/query/metrics_catalog.tsv` | Mapeamento q16–q19 para as consultas MetricsQL da Story 3.4 |
| `bench/clickhouse/query/README.md` | Uso, decisões e limitações |
| `bench/clickhouse/results/2026-11-02-local-1b-mix/` | `report.md`, `report.json`, `query_runs.jsonl`, `query_summary.json`, `rollup.json`, `tenancy.jsonl`, `epoch.json`, `retention.json`, `metrics_queries.json`, `storage_*.tsv`, `run.log` |
| `Makefile` | `bench-clickhouse-query`, `bench-clickhouse-query-check` |
| `docs/adr/0005-tenancy-e-retencao.md` | Decisão de TTL por linha; Q-02 fechada; Q-01/Q-08 parciais |
| `DECISIONS.md`, `BENCHMARKS.md`, `STATUS.md` | Registro da decisão, números e posição |

## AC1 — Latência

**Dataset:** 1B eventos no mix do cenário `query-1b` (60% métricas, 30% logs,
10% traces): **300M logs + 100M spans + 600M amostras de métricas**. O lado de
métricas usa o run publicado da Story 3.4 (1M séries, 1M/s, 1 h), que é maior
que o mix de 1B.

| Sinal | Linhas | Comprimido | Razão | B/linha |
|---|---:|---:|---:|---:|
| Logs | 300.000.005 (5 probes de epoch) | 23,52 GiB | 7,51x | 84,2 |
| Spans | 100.000.000 | 5,67 GiB | 7,74x | 60,9 |

**16 consultas SQL**, 1 warm-up + 20 execuções, como `obs_tenant_0` (role
`observaki_query`):

| Consulta | p95 | p99 | Linhas lidas | Linhas retornadas |
|---|---:|---:|---:|---:|
| q01 | 164,35 ms | 165,12 ms | 39.272.454 | 48 |
| q02 | 250,70 ms | 251,61 ms | 39.272.454 | 8 |
| q03 | 187,11 ms | 209,13 ms | 10.797.062 | 100 |
| q04 | 254,49 ms | 257,58 ms | 39.272.454 | 8 |
| q05 | **431,94 ms** | 448,42 ms | 10.780.678 | 16 |
| q06 | 100,92 ms | 148,45 ms | 2.038.585 | 100 |
| q07 | 195,63 ms | 203,48 ms | 25.059.334 | 9 |
| q08 | 65,19 ms | 65,73 ms | 4.186.118 | 1 |
| q09 | 102,73 ms | 103,61 ms | 25.059.334 | 9 |
| q10 | 54,84 ms | 56,23 ms | 15.745.030 | 2 |
| q11 | 14,07 ms | 14,42 ms | 81.926 | 100 |
| q12 | 52,20 ms | 63,29 ms | 589.830 | 10 |
| q13 | 179,46 ms | 185,96 ms | 31.490.054 | 9 |
| q14 | 206,49 ms | 212,20 ms | 25.059.334 | 9 |
| q15 | 52,16 ms | 53,83 ms | 16.395 | 1 |
| q20 | 56,05 ms | 61,00 ms | 15.384.582 | 54 |

**4 consultas MetricsQL** (q16–q19), do run da Story 3.4:

| Consulta | p95 | Séries | Status |
|---|---:|---:|---|
| q16 (m01) | 757,04 ms | 200 | pass |
| q17 (m05) | 799,50 ms | 200 | pass |
| q18 (m06) | 659,67 ms | 1 | pass |
| q19 (m20) | 688,76 ms | 1 | pass |

**Tiers maiores:** o 10B/7d é uma projeção linear window-equivalent (10/7 =
1,43x o de 1B/24h): **617,06 ms** (alvo < 3 s). O 86B/24h foi medido no rollup
de 1 minuto: 9 consultas agregadas, p95 máximo **87,14 ms** (q07). O raw
10B/86B não foi materializado; continua item do T1/G1.

## AC2 — Isolamento

**Mecanismo:** row policies no ClickHouse com `tenant_id`,
`project_generation` e `authz_epoch` lidos de `tenant_scope`; o usuário do QS
(`obs_tenant_0`) tem apenas a role `observaki_query`. A política em
`tenant_scope` esconde as linhas dos outros usuários. Table functions perigosas
e tabelas de sistema sensíveis são bloqueadas por least privilege.

| Teste | Esperado | Resultado |
|---|---|---|
| `cross_tenant_logs` | 0 | pass |
| `cross_tenant_traces` | 0 | pass |
| `subquery_bypass` | 0 | pass |
| `or_bypass` | 0 | pass |
| `join_bypass` | 0 | pass |
| `url`, `file`, `s3`, `remote`, `mysql`, `postgresql` | denied | pass |
| `system.users`, `system.processes` | denied | pass |
| `INSERT`, `ALTER` | denied | pass |

**Epoch (AD-20):** 149.981.236 linhas do tenant-0 visíveis com epoch 1; após o
bump do epoch, 0 linhas antigas; uma linha nova com o epoch novo aparece (1).
O teste restaura o epoch ao final. O cache com `authz_epoch` continua na
Story 5.5 (Q-08 parcial).

## AC3 — Retenção

**Estratégia:** TTL por linha (`retention_days`) em uma tabela, partição diária
por `ingested_at`. O probe insere 6.000 linhas com idades 6/14/29 e 8/16/31
dias e políticas 7/15/30; após `OPTIMIZE ... FINAL`, sobrevivem exatamente
1.000 linhas de cada projeto (6d/7d, 14d/15d, 29d/30d); as expiradas somem.

**Troca de política:** 10M linhas de 8 dias com retenção 30, mutação para 7
com `mutations_sync=1`: **493 ms**, 10.000.000 → 0 linhas. O custo é o merge
de TTL (reescreve linhas vivas de uma part com retenções mistas). O fallback
de tabelas por bucket 7/15/30 evita a reescrita, mas triplica schema,
roteamento e parts; fica documentado para o T1.

## Comandos

```bash
# smoke (10M logs + 1M spans, 5 execuções)
make bench-clickhouse-query-check

# run publicado (300M logs + 100M spans, 20 execuções, rollup 86B)
make bench-clickhouse-query

# repetir só a medição/rollup sobre o dataset carregado
./bench/clickhouse/query/run.sh --skip-load --base "2026-11-03 01:23:09" \
  --runs 20 --rollup --results-dir bench/clickhouse/results/2026-11-02-local-1b-mix
```

## Limitações e pendências

- Single-node local; sem réplica, NATS, gateway, Query Service ou vmauth. O
  usuário de benchmark simula o backstop de row policy do QS; o predicado
  injetado pelo QS (AD-3/AD-17) ainda não existe.
- Dataset sintético gerado por SQL, não pelo caminho OTLP real.
- O raw 10B/86B não foi materializado; o 10B/7d é projeção e o 86B/24h é
  rollup. O baseline T1 continua bloqueado por **P-07** e pelas Stories
  4.1–4.5.
- O run local parou merges de background para caber no disco; as tabelas foram
  medidas com parts não consolidadas.
- A `q03`/`q05` usam cache de página quente após o warm-up; a primeira execução
  não entra na estatística.
- O default de retenção 7/15/30 continua **P-02** (produto); o ADR-0005 usa 15
  dias provisório.
- Próxima story da Fase 0: **4.1 — Skeleton do gateway OTLP**.
