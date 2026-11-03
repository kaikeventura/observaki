# bench/clickhouse/query — Story 3.5 query, retention and row-policy spike

Este diretório mede as 20 consultas de dashboard, o isolamento por tenant e a
retenção por projeto no dataset de 1B eventos do cenário `query-1b` (60%
métricas, 30% logs, 10% traces). O run local usa **300M logs + 100M spans** e
reaproveita as 4 consultas MetricsQL da Story 3.4; o total é 1B eventos.

> **Não é evidência do G1.** O run é single-node e local. O raw 10B/86B, o
> Query Service e o cluster T1 continuam pendentes de P-07 e das Stories
> 4.1–4.5.

## O que roda

| Etapa | O que faz |
|---|---|
| Dataset | 300M logs + 100M spans em 24 h, 2 tenants × 4 projetos, retenção 7/15/30 |
| Consultas | 16 SQL de [`../queries/catalog.tsv`](../queries/catalog.tsv) como `obs_tenant_0` (role `observaki_query`), 1 warm-up + 20 repetições; 4 MetricsQL da Story 3.4 |
| Isolamento | Row policies por `tenant_id` + `project_generation` + `authz_epoch`; cross-tenant, subquery, `OR 1=1`, join, table functions, `system.users`/`system.processes`, escrita e DDL |
| Retenção | TTL 7/15/30 com idades 6/14/29 e 8/16/31 dias; mutação de política em 10M linhas |
| 86B/24h | Rollups de 1 minuto (logs e traces) com as mesmas row policies; 9 consultas agregadas em 20 repetições |
| Relatório | `report.md`, `report.json`, `query_runs.jsonl`, `rollup.json`, `tenancy.jsonl`, `epoch.json`, `retention.json` |

## Uso

```bash
# smoke (≈1 min)
make bench-clickhouse-query-check

# run publicado (≈20 min de carga + consultas)
make bench-clickhouse-query

# repetir só a medição e os rollups sobre o dataset já carregado
./bench/clickhouse/query/run.sh --skip-load --base "2026-11-03 01:23:09" \
  --runs 20 --rollup --results-dir bench/clickhouse/results/2026-11-02-local-1b-mix
```

O container `observaki-ch35` e o diretório `/tmp/opencode/observaki-ch35` são
preservados para `--skip-load`/`--reuse`. Use `--cleanup` para remover o
container; o diretório de dados não é apagado.

## Consultas

As 16 consultas SQL cobrem `q01`–`q15` e `q20` do catálogo
[`bench/queries/dashboard.tsv`](../../queries/dashboard.tsv). As `q16`–`q19`
são MetricsQL e vêm do run publicado da Story 3.4
([`../../victoriametrics/results/2026-11-02-local-1m-1h/`](../../victoriametrics/results/2026-11-02-local-1m-1h/)).
Cada consulta recebe o predicado de tenant/projeto que o Query Service injeta
em produção (AD-3/AD-17); a row policy é o backstop.

## Retenção

A estratégia escolhida é **TTL por linha (`retention_days`) em uma tabela**.
O probe valida 7/15/30 e a mutação de troca de política em 10M linhas. O custo
é o merge de TTL: uma part com retenções mistas reescreve também as linhas
vivas quando a primeira expira. O fallback de tabelas por bucket 7/15/30 evita
a reescrita de linhas vivas, mas triplica schema, roteamento e parts; por isso
não é o default. A decisão está em
[`docs/adr/0005-tenancy-e-retencao.md`](../../../docs/adr/0005-tenancy-e-retencao.md).

## Rollups 86B/24h

As consultas agregadas usam rollups de 1 minuto; as pontuais/seletivas
(`q03`, `q06`, `q11`, `q12`, `q13`, `q15`) ficam nas tabelas raw com bloom
filters. O rollup de 1B tem a mesma cardinalidade do rollup de 86B/24h
(dimensões × 1440 minutos); só os contadores escalam. O raw 10B/86B continua
sendo item do T1/G1.

## Limitações

- Single-node local; sem réplica, NATS, gateway ou Query Service. O usuário
  `obs_tenant_0` simula o backstop de row policy do QS.
- Dataset gerado por SQL, não pelo caminho OTLP real.
- O tier de 10B/7d é uma projeção window-equivalent; o de 86B/24h é medido no
  rollup, não no raw. Nenhum dos dois substitui a medição T1.
- O hardware local é menor que a referência `query-1b` (16 vCPU/64 GB); o
  relatório publica CPU, RAM e disco reais.
