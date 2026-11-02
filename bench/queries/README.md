# bench/queries — catálogo de consultas representativas

`dashboard.tsv` é o contrato das 20 consultas de dashboard exigidas pelo
ADR-0006 (Q-03). Ele é independente de engine e de schema: descreve a pergunta,
a superfície do produto, o sinal, a janela e os datasets em que a consulta se
aplica. O SQL do ClickHouse é vinculado na Story 3.5, quando o schema
(3.1/3.2) e o Query Service existirem. O MetricsQL do VictoriaMetrics está
vinculado na Story 3.4 em
[`bench/victoriametrics/queries/catalog.tsv`](../victoriametrics/queries/catalog.tsv).

## Formato

Colunas separadas por tabulação:

| Coluna | Valores | Uso |
|---|---|---|
| `id` | `q01`–`q20` | Identificador estável no relatório |
| `surface` | `Explore/Logs`, `Explore/Traces`, `Dashboard`, `Services`, `Explorer/Metrics` | Origem no produto |
| `signal` | `logs`, `traces`, `metrics` | Motor de destino |
| `engine` | `sql`, `metricsql` | ClickHouse ou VictoriaMetrics |
| `window` | `scenario`, `15m`, `1h`, `24h`, `7d`, `point` | `scenario` usa a janela do dataset |
| `datasets` | `1b`, `10b`, `86b` | Datasets em que a consulta roda |
| `description` | texto | Semântica da consulta |

## Cobertura

- **Logs:** volume por serviço, taxa de erro, busca filtrada, top-10, volume por
  host/pod, busca textual de incidente e correlação por `trace_id`.
- **Traces:** p95/p99, taxa de erro, distribuição por status, spans lentos,
  busca por `trace_id`, mapa de serviços, histograma e tendência.
- **Métricas:** rate, taxa de erro, p95 via `histogram_quantile` e top-10 por
  cardinalidade.

## Regras

1. Toda consulta declara a janela e os datasets; nenhuma depende de dados de
   outro tenant.
2. `point` significa busca por identificador (`trace_id`), não varredura.
3. `scenario` usa a janela do cenário (24h ou 7d); a consulta precisa produzir
   o mesmo resultado semântico nos três datasets.
4. O benchmark roda cada consulta pelo menos 3 vezes; p95 e p99 são reportados
   separadamente (ADR-0006, regra 10).
5. Mudanças no catálogo exigem atualização do relatório e justificativa; não se
   remove uma consulta para melhorar a média.
6. O catálogo MetricsQL cobre as perguntas de métricas (`q16`–`q19`) e as
   consultas de rate/erro/latência/cardinalidade; `m14`/`m15` são visões de
   operador e as demais são escopadas por `tenant_id`/`project_id`.
