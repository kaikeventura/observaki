# Story 3.4 — VictoriaMetrics, MetricsQL e cardinalidade (handoff)

**Status:** AC1–AC3 validados em run local single-node; **não é o Gate G1**.
**Data:** 2026-11-02 (run em UTC de 2026-11-02).
**Commit:** `fc2f0c1` (harness) + o commit de docs/resultados desta sessão.
**Escopo:** ingestão de 1M amostras/s com 1M séries ativas, 20 consultas
MetricsQL em janelas de 24 h e admissão de cardinalidade por tenant com
isolamento do vizinho.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `bench/internal/promrw/` | Encoder/decoder do Prometheus remote write (protobuf + snappy), com testes de round-trip e encoding incremental |
| `bench/internal/vmbench/` | Catálogo determinístico de 1M séries (2 tenants × 500k), cliente MetricsQL, catálogo das 20 consultas e percentis |
| `bench/cmd/vmloadgen/` | Gerador remote write com pacing por worker, relatório JSON e modo probe para cardinalidade |
| `bench/cmd/vmquerybench/` | Runner das 20 consultas, validação de `rate`/`increase`/`histogram_quantile`/agregação e consulta única para o vizinho |
| `bench/cmd/vmadmission/` | Proxy de admissão por tenant (AD-33), erro `429 cardinality_limit_exceeded`, `/metrics` e `/stats` |
| `bench/victoriametrics/run.sh` | Container, correção, carga, consultas, cardinalidade, coleta e relatório |
| `bench/victoriametrics/queries/catalog.tsv` | 20 consultas MetricsQL representativas, escopadas por tenant/projeto |
| `bench/victoriametrics/results/2026-11-02-local-1m-1h/` | `report.md`, `report.json`, `load.json`, `queries.json`, `verify.json`, `cardinality-*`, `neighbor-*`, `vm.json`, `resources.tsv`, `run.log` |
| `Makefile` / `scripts/smoke.sh` | `bench-victoriametrics`, `bench-victoriametrics-check` e smoke dos binários |
| `bench/victoriametrics/README.md` | Uso, contrato, decisões e limitações |

## AC1 — Ingestão

**Run local:** 1M séries ativas, 1M amostras/s por 1 h, 16 workers, 5.000
séries por request, remote write protobuf + snappy.

| Métrica | Valor |
|---|---:|
| Amostras enviadas | 3.600.000.000 |
| Throughput | **999.737,99/s** (99,974% de 1M/s) |
| Erros / rejeições | 0 / 0 |
| Latência de request p50 / p95 / p99 | 7,13 / **13,09** / 21,28 ms |
| Séries ativas após a carga | 1.000.006 (1M do catálogo + 6 da verificação) |
| Séries ativas após a cardinalidade | 1.200.006 |
| RSS máximo / médio | 1,24 GiB / 0,98 GiB (1.331.040.256 / 1.049.283.197 B) |
| Tamanho do diretório | 15,27 GiB (16.390.650.204 B) |
| Linhas inseridas (`promremotewrite`) | 3.603.199.726 |

**Sobre a memória:** o perfil T1 tem 16 vCPU e 64 GiB. O orçamento adotado é
32 GiB (50% do nó, seguindo a recomendação do VictoriaMetrics de deixar 50% de
RAM livre). O run local publica o RSS real; ele não é o hardware de referência.

## AC2 — Consulta

**20 consultas MetricsQL** em janelas de 24 h, step de 5 m, `nocache=1`,
5 execuções medidas + 1 warm-up por consulta. As consultas de dashboard
filtram `tenant_id`/`project_id`; `m14`/`m15` são visões de operador.

| Métrica | Valor |
|---|---:|
| p50 | 573,95 ms |
| p95 | **799,50 ms** (alvo < 1.000 ms) |
| p99 | 1.216,71 ms |
| Máximo | 1.305,19 ms |
| Erros | 0 |
| Consultas com resultado | 20/20 |
| Consulta mais lenta (m14, visão de operador) | p95 1.305,19 ms |

**Correção:** `vmquerybench verify` grava um dataset determinístico e valida
`rate`, `increase`, `histogram_quantile` (p50/p95) e agregação por label. Os
7 checks passaram no run.

## AC3 — Cardinalidade

**Mecanismo:** `vmadmission` aplica o limite por tenant antes do
VictoriaMetrics (AD-33). Um request que ultrapassaria o limite é rejeitado por
inteiro com `429` e `code=cardinality_limit_exceeded`; o proxy expõe
`observaki_cardinality_*`.

| Verificação | Valor |
|---|---:|
| Limite tenant-a | 150.000 |
| Séries ativas tenant-a | 150.000 |
| Séries rejeitadas tenant-a | **100.000** (1.004 requests, 100.400 amostras) |
| Limite tenant-b | 100.000 |
| Séries ativas tenant-b | 50.000 |
| Séries rejeitadas tenant-b | **0** |
| p95 do vizinho (baseline) | 584,49 ms |
| p95 do vizinho (durante) | 612,64 ms |
| Drift do vizinho | **4,82%** (alvo < 10%) |

O teste usa a métrica de probe `observaki_cardinality_probe_total` para que o
proxy comece a contagem em zero, sem depender do dataset principal.

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Formatação | `make fmt` | ok |
| Lint | `make lint` | ok (0 issues; ADR, Go, web, Rust) |
| Testes | `make test` | ok (Go, web, Rust; inclui `promrw`, `vmbench`, `vmloadgen`, `vmquerybench` e `vmadmission`) |
| Build | `make build` | ok (Go, `vmloadgen`, `vmquerybench`, `vmadmission`, web e agente) |
| Smoke | `SMOKE_SKIP_CI=1 make smoke` | ok (healthz, API, web, `vmloadgen`/`vmquerybench`/`vmadmission`, cenários e relatório) |
| Smoke do spike | `make bench-victoriametrics-check` | pass (100k séries; 100k/s por 10 s; drift 7,32%) |
| Run publicado | `./bench/victoriametrics/run.sh --reset --results-dir .../2026-11-02-local-1m-1h` | pass (1M séries, 1M/s, 1 h) |
| Relatório | `bench/victoriametrics/results/2026-11-02-local-1m-1h/report.md` | status `pass` |

## Decisões e limitações

- **Single-node local.** Sem réplica, vmauth, gateway OTLP, NATS, Query Service,
  row policies ou caos. O `vmadmission` é o spike do AD-33; o gateway de
  produção (Stories 4.1/4.2) é o dono da admissão.
- **Remote write canônico.** O caminho é protobuf + snappy, o mesmo que o
  writer de métricas usará na Story 4.4. Não há import JSON/textual.
- **Consultas com `nocache=1`.** O p95 mede o caminho bruto, sem o rollup cache
  do VictoriaMetrics. É conservador em relação a um dashboard com cache.
- **Distribuição sintética.** As famílias de aplicação foram dimensionadas para
  que as consultas de dashboard não varram 1M séries; as métricas de container
  completam o orçamento de 1M séries ativas sem inflar as consultas. O mix é
  determinístico e documentado em `bench/internal/vmbench/series.go`.
- **Admissão por request inteiro.** Um request que ultrapassaria o limite é
  rejeitado por completo. O proxy não faz admissão parcial nem agregação.
- **Contagem em memória.** O proxy reinicia a contagem de séries do zero; o
  catálogo durável por tenant é trabalho da Fase 1 (FR-4).
- **Não é G1.** O baseline T1 continua bloqueado por **P-07** e pelas Stories
  4.1/4.3/4.5.

## Próximo

1. **Story 3.5 — benchmark de consulta, retenção e row policies.**
2. O baseline T1 da Story 2.4 continua bloqueado por **P-07** e pelas Stories
   3.5/4.1–4.5.
