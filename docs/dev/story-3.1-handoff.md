# Story 3.1 — Schema ClickHouse para logs (handoff)

**Status:** AC1–AC3 concluídos com evidência local; **não é o Gate G1**.
**Data:** 2026-11-02 (carga em UTC de 2026-11-02).
**Escopo:** schema de logs, dataset sintético de 1B linhas, compressão,
consulta `q03` e TTL de 7/15 dias em um ClickHouse 26.8 single-node.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `internal/adapters/clickhouse/migrations/0001_otel_logs.sql` | DDL de `otel_logs`: partição/TTL por `ingested_at`, `ORDER BY` de tenant/projeto/serviço/tempo, codecs e índices |
| `internal/adapters/clickhouse/schema.go` | Migrações embarcadas (`embed`), ordenadas e validadas por versão |
| `internal/adapters/clickhouse/schema_test.go` | Teste de contrato do DDL: colunas, partição, TTL, ordem, índices e codecs |
| `bench/clickhouse/load.sql` | Gerador determinístico de 1B linhas em 24 h, ordenado no tempo |
| `bench/clickhouse/queries/q03_filtered_logs.sql` | Consulta representativa do catálogo (`q03`) |
| `bench/clickhouse/run.sh` | Container, schema, TTL, carga, métricas e relatório |
| `bench/clickhouse/config/bench.xml` | Perfil local de memória/CPU e limite de merge |
| `bench/clickhouse/README.md` | Uso, decisões, limitações e reprodução |
| `BENCHMARKS.md` | Seção publicada com os números e o veredito local |
| `bench/clickhouse/results/2026-11-02-local-1b/` | `report.md`, `report.json`, `runs.jsonl`, `ttl.json`, `run.log` |
| `Makefile` | Alvos `bench-clickhouse` (1B) e `bench-clickhouse-check` (10M) |

## AC1 — Schema

O DDL contém `tenant_id`, `project_id`, `timestamp`, `ingested_at`,
`observed_timestamp`, `severity`, `severity_number`, `service_name`, `body`,
`attributes`, `resource_attributes`, `trace_id`, `span_id`, `event_id` e
`retention_days`.

- **Engine:** `MergeTree`.
- **Partição:** `toDate(ingested_at)` — diária para manter o número de parts
  sob controle no spike. Em volume T1 (≈25,9B logs/dia) o writer pode trocar
  para `toStartOfHour(ingested_at)` depois de uma revisão de capacidade; o
  `ORDER BY` e o TTL não mudam.
- **Ordem:** `(tenant_id, project_id, service_name, timestamp)`.
- **TTL:** `ingested_at + INTERVAL retention_days DAY`; `ingested_at` é a
  autoridade (AD-29), não o timestamp do produtor.
- **Codecs:** `ZSTD(3)` no body, maps e IDs; `Delta(8) + ZSTD(3)` nos
  timestamps.
- **Índices:** `idx_body` (`tokenbf_v1`), `idx_trace` e `idx_span`
  (`bloom_filter`).
- **Teste:** `go test ./internal/adapters/clickhouse/...` valida o contrato do
  DDL sem precisar de ClickHouse; o teste roda no `make test`.

## AC2 — 1B linhas, compressão e consulta

**Dataset:** 1.000.000.000 linhas, 20 chunks de 50M, 24 h de event time
ordenado no tempo, 2 tenants × 4 projetos × 8 serviços, 90/7/3%
INFO/WARN/ERROR, corpo médio de 165,3 B e retenção de 7/15 dias por linha.
Cada chunk cobre ~1,2 h; a janela de 1 h da `q03` toca as parts do último
chunk, não as parts de todos os chunks.

**Carga:** 2026-11-02T18:07:20Z a 18:47:40Z (40 min 20 s; ~413k linhas/s),
em um host de 22 CPUs e 15,0 GiB de RAM, disco de 441,1 GiB (42,6 GiB livres
ao fim). Os merges de background da tabela principal foram parados durante a
carga para não estourar o disco; a tabela foi medida com 1.921 parts ativas.

| Métrica | Valor |
|---|---:|
| Linhas | 1.000.000.000 |
| Comprimido | 83.271.749.197 B (77,55 GiB) |
| Sem compressão | 580.989.960.496 B (541,09 GiB) |
| Compressão | **6,98x** (NFR-13 exige ≥ 5x) |
| Bytes comprimidos/linha | 83,3 B |
| Partes ativas | 1.921 |
| `q03` p50 | 248,8 ms |
| `q03` p95 | **263,0 ms** (alvo < 1.000 ms) |
| `q03` p99 | 298,5 ms |
| Linhas lidas (média) | 21.282.816 |
| Bytes lidos (média) | 426.995.180 B |
| Linhas retornadas | 100 |

A `q03` usa `tenant-0`, `project-0`, `checkout` e `ERROR` na janela
`2026-11-02 17:07:19` a `2026-11-02 18:07:19`; o relatório registra 20
repetições depois de um warm-up. O resultado é **pass** local.

## AC3 — Retenção

A tabela clone `otel_logs_ttl_test` recebeu cinco linhas:

| Bucket | Inseridas | Resultado |
|---|---:|---|
| 7 dias, 8 dias de idade | 1 | removida |
| 7 dias, 1 dia de idade | 1 | preservada |
| 15 dias, 16 dias de idade | 1 | removida |
| 15 dias, 8 dias de idade | 1 | preservada |
| 15 dias, 1 dia de idade | 1 | preservada |

Depois de `OPTIMIZE TABLE ... FINAL`, sobraram 3 linhas. O caso de 15 dias com
8 dias de idade é o teste de fronteira: ele precisa sobreviver porque só a
política de 7 dias expira aos 8 dias. **Estratégia escolhida:** TTL por linha
com `retention_days`; é mais simples que tabelas por bucket e cobre 7/15/30 no
mesmo schema. O valor é o do momento da ingestão; mudar a política afeta linhas
novas e a reconciliação por projeto/row policies continua na Story 3.5. O
fallback de tabelas por bucket permanece documentado no addendum e no
ADR-0005 (Q-02).

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Formatação | `make fmt` | ok |
| Lint | `make lint` | ok (Go, web, Rust, ADR) |
| Testes | `make test` | ok (inclui `internal/adapters/clickhouse`) |
| Build | `make build` | ok (Go, web, agente) |
| Smoke | `make smoke` | ok (healthz, API, web, loadgen, cenários, relatório) |
| Contrato do schema | `go test ./internal/adapters/clickhouse/...` | ok |
| Smoke de 10M | `./bench/clickhouse/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5 --stop-merges` | 6,52x–6,89x; `q03` p95 6,4 ms; TTL pass |
| Run de 1B | `./bench/clickhouse/run.sh --reuse --rows 1000000000 --chunk-rows 50000000 --runs 20 --stop-merges --base "2026-11-02 18:07:19"` | 6,98x; `q03` p95 263,0 ms; TTL pass |
| Relatório | [`bench/clickhouse/results/2026-11-02-local-1b/report.md`](../../bench/clickhouse/results/2026-11-02-local-1b/report.md) | `status: pass`, commit `494391a`, `dirty: false` |

## Decisões e limitações

- **Single-node local.** Não é o cluster T1; o baseline oficial continua
  bloqueado por P-07 e pelas Stories 3.2–3.5/4.1–4.5.
- **Dataset por SQL.** A carga não passa pelo gateway OTLP, NATS ou Query
  Service; mede o schema e o motor, não o caminho de ingestão durável.
- **Merges parados.** O host tem ~126 GiB livres e a tabela comprimida ocupa
  ~78 GiB; com merges de background, as parts inativas chegam a ~2,5× o
  tamanho ativo e o disco estoura. O run local para os merges da tabela
  principal e mede 1.921 parts; no T1 os merges rodam normal.
- **Cache quente.** O warm-up não entra nas 20 repetições; a `q03` lê a última
  hora do dataset e o cache de página do SO está quente.
- **Hardware menor que a referência.** O cenário `query-1b` do harness assume
  16 vCPU/64 GB; o host local tem 22 CPUs/15 GiB. O relatório publica o
  hardware real.
- **Sem tenancy completa.** Não há row policies, réplica, quotas, caos ou
  Query Service; isso é 3.3/3.5/4.5.
- **Não é G1.** O número oficial de 1M/s por 1 h, lag p95 < 10 s e zero perda
  continua dependendo do T1 e do caminho de ingestão.

## Próximo

1. **Story 3.2 — schema ClickHouse para traces**, que depende da 3.1.
2. O baseline T1 da Story 2.4 continua bloqueado por **P-07** (provedor,
   orçamento e credencial IAM dedicada) e pelas Stories 3.2–3.5/4.1–4.5.
3. A reconciliação de retenção por projeto, row policies e isolamento ficam
   para a Story 3.5; a estratégia de TTL por linha precisa ser reavaliada com
   mudança de política em dados já gravados.
