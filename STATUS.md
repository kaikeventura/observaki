---
project: Observaki
phase: 0
phase_name: Fundação e Benchmarks
epic: P0-E4
epic_name: Gateway OTLP e ingestão
story: "4.4"
story_name: Writers para ClickHouse e VictoriaMetrics
task: "AC1–AC3"
task_name: End-to-end, retry e observabilidade
status: done        # not_started | in_progress | blocked | done
updated: 2026-11-04
branch: main
remote: "git@github.com:kaikeventura/observaki.git"
last_commit: "0699eed"
next_action: "Story 4.5 — teste de caos (ClickHouse fora 15 min, falha de gateway/NATS) e go/no-go da stack de ingestão; o AC1 de 1M/s/1h e o sizing do WAL/spool ficam para o T1 (P-07)"
blocked_by: []
last_session: "Story 4.4: writers ClickHouse/VM com três consumidores duráveis, dedupe por event_id + geração/epoch, OnDone só após insert/DLQ, retry exponencial, DLQ NATS, INSERT de 17 colunas e remote write; run local de 20k/s por 20 s (400k eventos, lag p95 0,80 s, contagens exatas) e caos de 3 s/2 s (8 retries, zero perda, lag p95 7,97 s); make fmt/lint/test/build/smoke e bench-clickhouse-ingest-check verdes; T1 pendente; evidência em docs/dev/story-4.4-handoff.md."
---

# Observaki — Status de Desenvolvimento

> **Fonte de verdade do estado.** `epics.md` define o plano; este arquivo define **onde estamos**. Se houver conflito de escopo, o plano vence; se houver conflito de posição, este arquivo vence.

## Prompt para nova sessão (copie e cole)

**Arquivo pronto:** [`PROMPT.md`](PROMPT.md). Você pode simplesmente mandar: `Leia PROMPT.md e execute.`

```text
Leia AGENTS.md, project-context.md e STATUS.md.
Continue exatamente do next_action em STATUS.md.
Não replaneje, não pule gates e não invente escopo.
Ao terminar, atualize STATUS.md (frontmatter + checklist + session log) e pare com um relatório curto.
```

Atalho no OpenCode: `/continue`.

## Como o OpenCode deve usar este arquivo

**No início da sessão:**
1. Leia `AGENTS.md`, `project-context.md` e este arquivo.
2. Use `phase`, `epic`, `story`, `task` e `next_action` como posição atual.
3. Não recalcule o roadmap; não pule para outra fase.
4. Se `status: blocked`, leia `blocked_by` e pare.

**No fim da sessão:**
1. Atualize o frontmatter (`task`, `status`, `updated`, `next_action`, `blocked_by`, `last_session`).
2. Atualize a linha da story na tabela da Fase 0.
3. Adicione uma linha no **Session log** (append-only).
4. Registre evidência (commit/PR, comandos, resultado de teste/benchmark).
5. Pare. Não comece a próxima story sem uma nova sessão/ordem.

**Regra de ouro:** `done` só com evidência. Se não rodou teste/benchmark, não marque done.

## Agora

| Campo | Valor |
|---|---|
| Fase | **0 — Fundação e Benchmarks** |
| Epic | **P0-E4 — Gateway OTLP e ingestão** |
| Story | **4.4 — Writers para ClickHouse e VictoriaMetrics** |
| Task | **AC1–AC3 — End-to-end, retry e observabilidade** |
| Status | `done` |
| Branch | `main` |
| Remoto | `https://github.com/kaikeventura/observaki.git` |
| Último commit | `0699eed` |
| Próxima ação | Story 4.5 — teste de caos e go/no-go da stack de ingestão |
| Bloqueios | 2.4 (T1): P-07 e Story 4.5; o AC1 de 1M/s/1h, o caos de 15 min e o sizing do WAL/spool dependem do T1 |
| Última sessão | Story 4.4: writers CH/VM com consumidores duráveis, dedupe, retry, DLQ e métricas; 20k/s local (400k eventos, lag p95 0,80 s) e caos 3 s/2 s (8 retries, zero perda); T1 pendente |

**Ambiente local:** Node 26.13.0 instalado via `nvm` (alias `default` atualizado); pnpm 12.16.1 via `corepack` com shim em `~/.local/bin`; golangci-lint v2.15.0 em `~/go/bin` **compilado com `GOTOOLCHAIN=go1.27.2`** (a build padrão saiu com go1.26 e recusava o módulo). Rust 1.93 estável. `make bootstrap/lint/test/build/dev/smoke/clean` verificados nesta máquina. Em shells não interativos, rode `source ~/.nvm/nvm.sh && nvm use 26` antes de `make ci`.

**Nota de stack:** SvelteKit 3 removeu `svelte.config.js`; a configuração (adapter, preprocess) agora vai no plugin `sveltekit()` do `vite.config.ts`. O arquivo listado na story não existe mais.

## Fase 0 — checklist de stories

**Legenda:** `todo` · `doing` · `done` · `blocked`

| Epic | Story | Título | Status | Evidência / próximo passo |
|---|---|---|---|---|
| P0-E1 | 1.1 | Bootstrap do mono-repo | `done` | T1.1.1–T1.1.9 done; revisão dispensada pelo dono; evidência em `docs/dev/story-1.1-handoff.md` |
| P0-E1 | 1.2 | CI de qualidade e release alpha | `done` | CI/CodeQL verdes; `v0.0.1-alpha` com binários, SBOMs, checksums, bundles cosign e imagem assinada; falta habilitar branch protection (manual) |
| P0-E1 | 1.3 | Governança open source | `done` | LICENSE Apache 2.0, DCO workflow + script testado (sem sign-off e e-mail divergente falham), CONTRIBUTING/CoC/SECURITY, templates e `docs/governance/open-core.md`; `make ci`/`make smoke` e CI/CodeQL verdes em `24a16be` |
| P0-E1 | 1.4 | ADRs | `done` | Template + processo, seis ADRs **aceitas** por `kaikeventura` em 2026-10-31 (AC3), `scripts/check-adr.sh` no `make lint`; `make ci`/`make smoke` verdes |
| P0-E2 | 2.1 | Gerador de carga OTLP | `done` | `bench/loadgen` + `otlploadgen`; payload 37,8/199,0/499,2 B (132,3 B ponderado); 100k/1M 5 min e 5M sharded 5 min dentro de ±5%; 100k gzip/1M none HTTP; `make ci`/`make smoke` verdes; evidência em `docs/dev/story-2.1-handoff.md` |
| P0-E2 | 2.2 | Cenários de benchmark | `done` | 12 cenários + `bench/run.sh` + 20 consultas; `--event-time-rate`; 100k real (99.776,8/s) e 5M sharded (4.999.569,8/s) dentro de ±5%; query/cardinalidade/caos bloqueados até 3.4/3.5/4.5; `make ci`/`make smoke` verdes; evidência em `docs/dev/story-2.2-handoff.md` |
| P0-E2 | 2.3 | Relatório de benchmark | `done` | `resources.json` 1 Hz, `config_hash`/versões, `report.md`+`report.json`, cobertura `collected`/`not_collected` e variância; 3×60s reais a 99.964,5/s médio (spread 0,01%), p95 1,22%, CPU 7,03%, report pass; 5s/30s flagaram p99/CPU; `make ci`/`make smoke` verdes; evidência em `docs/dev/story-2.3-handoff.md` |
| P0-E2 | 2.4 | Ambiente reproduzível + baseline | `blocked` | Perfil T1, Terraform AWS de referência, custo/cleanup e guards de gasto/root; pré-baseline local 3×60s a 99.935,1/s (pass) em `BENCHMARKS.md`; falta apply/baseline T1 (P-07 + Stories 3.2–3.5/4.1–4.5); evidência em `docs/dev/story-2.4-handoff.md` |
| P0-E3 | 3.1 | Schema ClickHouse para logs | `done` | DDL com partição/TTL por `ingested_at`, ordem tenant/projeto/serviço/tempo e codecs; 1B linhas locais em 24 h, **6,98x**, `q03` p95 **263 ms**, TTL 7/15 pass; `make smoke` verde; não é G1; evidência em `docs/dev/story-3.1-handoff.md` e `bench/clickhouse/results/2026-11-02-local-1b/` |
| P0-E3 | 3.2 | Schema ClickHouse para traces | `done` | DDL com partição/TTL por `ingested_at`, ordem tenant/projeto/serviço/status/tempo, bloom filters e sem MV de RED; 1B spans locais em 24 h, **7,52x**, trace lookup p95 **328 ms**, serviço/status p95 **178 ms**, service map 9/9 e correlação log↔trace ok; `make smoke` verde; não é G1; evidência em `docs/dev/story-3.2-handoff.md` e `bench/clickhouse/traces/results/2026-11-02-local-1b/` |
| P0-E3 | 3.3 | Ingestão ClickHouse + dedupe | `done` | Writer RowBinary com spool/checkpoint, dedupe por `event_id` e token, DLQ e métricas; run local 400k/s por 60s, **24M eventos**, lag p95 **1,4s**, 18 parts, 0 `Too many parts`, dedupe 50k e recuperação 100k após crash; 1M/s/1h no T1 pendente (P-07/4.4); não é G1; evidência em `docs/dev/story-3.3-handoff.md` e `bench/clickhouse/ingest/results/2026-11-02-local-400k/` |
| P0-E3 | 3.4 | VictoriaMetrics + cardinalidade | `done` | 1M séries a 1M/s por 1h (999.738/s, p95 de carga 13ms, RSS 1,24GiB, 15,27GiB), 20 consultas em 24h (p95 799,5ms, 20/20, 7/7 checks) e admissão por tenant (100k rejeitadas no tenant-a, 0 no tenant-b, drift 4,82%); `m14` 1.305ms; não é G1; evidência em `docs/dev/story-3.4-handoff.md` e `bench/victoriametrics/results/2026-11-02-local-1m-1h/` |
| P0-E3 | 3.5 | Query/retenção/row policies | `done` | 16 SQL no dataset de 1B eventos (300M logs + 100M spans): p95 máx **431,94ms**, p99 <2x; 4 MetricsQL p95 **799,5ms**; 15/15 testes de isolamento e epoch (AD-20); TTL 7/15/30 e mutação de política em 10M linhas (**493ms**); 10B/7d projetado **617ms**; 86B/24h em rollup 1m (p95 máx **87,14ms**); não é G1; evidência em `docs/dev/story-3.5-handoff.md` e `bench/clickhouse/results/2026-11-02-local-1b-mix/` |
| P0-E4 | 4.1 | Gateway OTLP | `done` | OTLP gRPC :4317 e HTTP :4318 (protobuf/JSON/gzip) para traces/logs/métricas; limites 10 MiB/10k configuráveis; `partial_success`; redaction por chave/Luhn antes do sink com `trace_id` preservado; SIGTERM drena e sai 0; substituído pelo WAL NATS na 4.3; `make ci`/`make smoke` verdes; evidência em `docs/dev/story-4.1-handoff.md` |
| P0-E4 | 4.2 | Auth/quotas | `done` | Snapshot versionado com tokens SHA-256 e quota por tenant; 401/403 + audit hash chain; 429 com `Retry-After` e pré-admissão antes do decode no HTTP; `make smoke` cobre 401/403/audit/429; harness local com 26.703 requests limitados e p95 de processamento +15,13% em 1M/s (inconclusivo por host compartilhado); T1 pendente; evidência em `docs/dev/story-4.2-handoff.md` |
| P0-E4 | 4.3 | Batching/WAL NATS | `done` | Envelope protobuf v1 com zstd, batching por subject, 32 shards e `event_id` determinístico; WAL JetStream com ack só após publish, retry com `Nats-Msg-Id`, 429/503 e `/readyz`; streams por sinal com `DiscardNew`; `walcheck` relê sem ack; run local 100k/s por 10 s (1M eventos, **5,46x**, 32 shards, 0 duplicatas, p99 **28,9 ms**); 1M/s/1h no T1 pendente; não é G1; evidência em `docs/dev/story-4.3-handoff.md` e `bench/gateway/wal/results/2026-11-03-local-100k/` |
| P0-E4 | 4.4 | Writers CH/VM | `done` | Três consumidores duráveis JetStream; ack só após insert/DLQ do ClickHouse ou HTTP 2xx do VM; dedupe por `event_id` + geração/epoch; token estável por eventos; retry exponencial; DLQ `OBSERVAKI_DLQ`; INSERT de 17 colunas; `/metrics` e `/stats`; run local 20k/s por 20 s (400k eventos, lag p95 **0,80 s**, contagens exatas, 0 falhas) e caos 3 s/2 s (8 retries, zero perda, lag p95 7,97 s); `make fmt/lint/test/build/smoke` e `bench-clickhouse-ingest-check` verdes; **não é G1**; T1 pendente; evidência em `docs/dev/story-4.4-handoff.md` e `bench/pipeline/results/2026-11-03-local-20k/` |
| P0-E4 | 4.5 | Caos + go/no-go | `todo` | Depende de 4.3/4.4 |
| P0-E5 | 5.1 | Docker Compose | `todo` | Depende de 4.1/3.1/1.2 |
| P0-E5 | 5.2 | Helm chart | `todo` | Depende de 5.1 |
| P0-E5 | 5.3 | Quickstart + sample app | `todo` | Depende de 5.1 |
| P0-E5 | 5.4 | Backup/restore + auto-observabilidade | `todo` | Depende de 5.2/5.3 |
| P0-E5 | 5.5 | API mínima + OpenAPI | `todo` | Depende de 4.1/4.2 |
| P0-E6 | 6.1 | Shell SvelteKit | `todo` | Depende de 1.1 |
| P0-E6 | 6.2 | uPlot 100k pontos | `todo` | Depende de 6.1 |
| P0-E6 | 6.3 | Tabela virtualizada | `todo` | Depende de 6.1 |
| P0-E6 | 6.4 | Bundle CI + Explore mock | `todo` | Depende de 6.1-6.3 |
| P0-E7 | 7.1 | Pipeline de design partners | `todo` | Independente |
| P0-E7 | 7.2 | Critérios/acordo | `todo` | Depende de 7.1 |
| P0-E7 | 7.3 | Onboarding | `todo` | Depende de 7.2/5.3 |
| P0-E7 | 7.4 | Feedback + demo | `todo` | Depende de 7.3 |

## Gate G1 — checklist de saída da Fase 0

- [ ] 1M eventos/s por 1 hora no workload de referência (4.3: WAL local a 100k/s e 1M/s mínimo; T1 pendente)
- [ ] Lag p95 < 10s; zero perda para eventos confirmados (local 3.3: p95 1,4s e crash/replay de 100k sem perda; 4.3: ack só após JetStream e NATS restart sem perda; **4.4: lag p95 0,80s a 20k/s e 7,97s sob caos de 3s/2s, zero perda**; T1/caos de 15 min pendentes)
- [ ] p95 < 1s em 1B linhas/24h (logs local: `q03` p95 263 ms; traces local: trace lookup p95 328 ms; **3.5: 16 consultas SQL p95 máx 431,94 ms + 4 MetricsQL 799,5 ms no mix de 1B eventos**; T1/gateway pendentes)
- [ ] p95 < 3s em 10B linhas/7d (**3.5: projeção window-equivalent de 617 ms; raw 10B/T1 pendente**)
- [ ] Compressão ≥ 5x em logs e traces (logs local 6,98x; traces local 7,52x; 3.5: 7,51x/7,74x no mix; T1 pendente)
- [x] Métricas: 1M séries a 1M/s por 1h e 20 consultas p95 <1s em 24h (local: 999.738/s, p95 799,5ms; T1 pendente)
- [ ] Retenção/row policies/quotas validadas (3.5: TTL 7/15/30 e row policies locais pass, epoch AD-20 pass; 4.2: quota de ingestão local pass com 429/`Retry-After`, isolamento p95 <10% e quotas de consulta pendentes no T1; 4.3: `DiscardNew`/MaxAge/MaxBytes do WAL e 429 `wal_backpressure` locais pass; **4.4: linhas com `project_generation`/`authz_epoch` e dedupe escopado por epoch**)
- [ ] Compose single-node < 15 min; Helm em 3 nós; upgrade/rollback
- [ ] 100k pontos < 100ms; bundle inicial < 180 KB gzip
- [ ] 3 design partners com dados reais
- [x] ADRs aprovadas (storage, gateway, fila, frontend, tenancy, benchmark)
- [ ] Go/no-go registrado para cada spike (3.1: go local; 3.2: go local; 3.3: go local; 3.4: go local; 3.5: go local; 4.2: go funcional local, isolamento p95 inconclusivo; 4.3: go funcional local, AC1/T1 pendente; 4.4: go funcional local, retry/caos curto pass, AC1/T1 pendente; T1 pendente)
- [ ] `BENCHMARKS.md` publicado (spikes 3.1/3.2/3.3/3.4/3.5 e harness 4.2/4.3/4.4 locais publicados; baseline T1 pendente de P-07 e 4.5)

**Só depois de todos os itens:** atualizar `phase: 1` e começar o detalhamento da Fase 1.

## Session log (append-only)

| Data | Sessão | Fase/Story/Task | Resultado | Commit/PR | Próximo |
|---|---|---|---|---|---|
| 2026-10-31 | Planejamento | — | PRD, arquitetura, UX, backlogs e OpenCode setup concluídos | `6be6c2d` | T1.1.1 |
| 2026-10-31 | Story 1.1 / T1.1.1 | P0 / 1.1 / T1.1.1 | Esqueleto do mono-repo, `.editorconfig`, `.gitignore`, `go.mod` (Go 1.27.0), README + placeholders; `go mod tidy` ok | `ea625ad` | T1.1.2 |
| 2026-10-31 | Merge (ordem humana) | P0 / 1.1 / T1.1.1 | `feat/p0-s1.1-bootstrap` mergeada em `main` com `--no-ff`; branch preservada | `2ca771d` | T1.1.2 |
| 2026-10-31 | Story 1.1 / T1.1.2 | P0 / 1.1 / T1.1.2 | Binário Go (`version`, `serve`), `/healthz`, `/api/v1/health`, shutdown gracioso, testes; `go test ./...`, `go vet`, smoke com curl | `1924ae6` | T1.1.3 |
| 2026-10-31 | Merge (preferência do dono) | P0 / 1.1 / T1.1.2 | `feat/p0-s1.1-bootstrap` mergeada em `main` com `--no-ff`; branch preservada. Regra: toda sessão termina com merge na `main` | `a1c8df2` | T1.1.3 |
| 2026-10-31 | Story 1.1 / T1.1.3 | P0 / 1.1 / T1.1.3 | App SvelteKit 3 + Svelte 5 + TS + Vite 8 + Tailwind 4, tokens do DESIGN, cartão de status da API, proxy `/api`, Vitest/Testing Library (3 testes), `check` e build estático em `web/build/` | `99a024f` | T1.1.4 |
| 2026-10-31 | Story 1.1 / T1.1.4 | P0 / 1.1 / T1.1.4 | Workspace Rust (`agent/`), `rust-toolchain.toml` stable, binário `observaki-agent --version`, 2 testes; `cargo build/test/fmt/clippy -D warnings` verdes | `1f748c3` | T1.1.5 |
| 2026-10-31 | Story 1.1 / T1.1.5 | P0 / 1.1 / T1.1.5 | `Makefile` (bootstrap/fmt/lint/test/build/dev/smoke/clean/ci/tools), `check-toolchain.sh`, `dev.sh`, `smoke.sh`, `.golangci.yml`; todos os alvos validados, inclusive smoke e dev com proxy | `8fd0efb` | T1.1.6 |
| 2026-10-31 | Story 1.1 / T1.1.6 | P0 / 1.1 / T1.1.6 | `internal/config` com defaults/validação (HTTP addr, log level, env, database URL, web dir), 5 testes, `.env.example`, `main` usando a config e `dev.sh` carregando `.env` | `d014d55` | T1.1.7 |
| 2026-10-31 | Story 1.1 / T1.1.7 | P0 / 1.1 / T1.1.7 | `docs/dev/local-setup.md` completo (pré-requisitos, quickstart, comandos, portas, 8 troubleshootings, validado por), quickstart no README; dev.sh preserva overrides do shell | `1c80d46` | T1.1.8 |
| 2026-10-31 | Story 1.1 / T1.1.8 | P0 / 1.1 / T1.1.8 | Clone limpo em `/tmp` no commit `74bdc22`: bootstrap 1,5 s, `make ci` 13,1 s, `make smoke` 10,3 s; versão, healthz, health e `web/build/index.html` conferidos; evidência em `docs/dev/local-setup.md` | `8654bd6` | T1.1.9 |
| 2026-10-31 | Story 1.1 / T1.1.9 | P0 / 1.1 / T1.1.9 | Handoff, decisões, escopo e contrato de CI da Story 1.2 em `docs/dev/story-1.1-handoff.md`; **revisão humana pendente** (status `blocked`) | `ae4c081` | Revisão humana |
| 2026-10-31 | Workflow (pedido do dono) | — | Fluxo passa a ser **main-only**: `PROMPT.md`, `AGENTS.md`, `GIT-WORKFLOW.md` e comandos do OpenCode sem branch por story e sem PR; commit direto na `main` | `50f3180` | Revisão humana |
| 2026-10-31 | Story 1.1 / T1.1.9 | P0 / 1.1 / T1.1.9 | **Story 1.1 concluída**: revisão dispensada explicitamente pelo dono; handoff e checklist em `docs/dev/story-1.1-handoff.md`; próxima é a Story 1.2 | `8957f4b` | Story 1.2 |
| 2026-10-31 | Story 1.2 / T1.2.1–T1.2.4 | P0 / 1.2 / T1.2.5 | Workflows `ci.yml`, `codeql.yml`, `release.yml`, job security/SBOM (Syft), release com cosign keyless, `Dockerfile` distroless, `docs/ci.md` e `SMOKE_SKIP_CI`; `actionlint` e imagem local ok | `42f09af` | Primeiro run no GitHub |
| 2026-10-31 | Story 1.2 / T1.2.5 | P0 / 1.2 / T1.2.5 | CI e CodeQL verdes na `main`; `govulncheck` exigiu Go 1.27.2 (GO-2026-6613/6617) e `cargo audit` passou a rodar em `agent/`; tag `v0.0.1-alpha` publicou 4 binários, SBOMs, `SHA256SUMS`, bundles cosign e imagem multi-arch assinada | `925b534` + tag `v0.0.1-alpha` | Story 1.3 |
| 2026-10-31 | Story 1.3 | P0 / 1.3 / — | `LICENSE` Apache 2.0, `CONTRIBUTING`, `CODE_OF_CONDUCT`, `SECURITY`, templates de issue/PR, `docs/governance/open-core.md` (P-03 explícita) e workflow `DCO sign-off`; `scripts/check-dco.sh` validado com sign-off ausente/divergente/malformado (exit 1) e caso válido (exit 0); `actionlint`, schema YAML dos issue forms, `make ci` e `make smoke` ok; CI e CodeQL verdes em `24a16be` | `8746351` | Story 1.4 |
| 2026-10-31 | Story 1.4 | P0 / 1.4 / T1.4.1–T1.4.3 | Template e processo de ADR, seis ADRs (storage, gateway, fila, frontend, tenancy/retenção, benchmark), `scripts/check-adr.sh` (seções, status, revisão e índice) no `make lint`, `DECISIONS.md` atualizado; `make ci` e `make smoke` verdes; ADRs em `proposto` aguardando revisão humana (AC3); push não realizado a pedido do dono | `fa73372` | Revisão humana (AC3) |
| 2026-10-31 | Story 1.4 / AC3 | P0 / 1.4 / T1.4.3 | Seis ADRs promovidas para `aceito` após aprovação de `kaikeventura`; revisão registrada em cada ADR, índice/DECISIONS/gate atualizados; `make adr-check` 6 válidas; push não realizado a pedido do dono | `fae7e73` | Story 2.1 |
| 2026-10-31 | Story 2.1 | P0 / 2.1 / T2.1.1–T2.1.3 | Gerador OTLP determinístico (`bench/loadgen` + `otlploadgen`), payload 37,8/199,0/499,2 B (132,3 B ponderado), 100k/1M 5 min e 5M sharded 5 min dentro de ±5%, 100k gzip/1M none HTTP, métricas/backpressure; `make smoke` verde; evidência em `docs/dev/story-2.1-handoff.md` | `eaae22c` | Story 2.2 |
| 2026-11-01 | Story 2.2 | P0 / 2.2 / AC1–AC3 | 12 cenários declarativos (throughput/query/cardinalidade/caos), `benchctl` + `bench/run.sh`, catálogo de 20 consultas, `--event-time-rate` para datasets; 100k real a 99.776,8/s (−0,22%) e 5M sharded a 4.999.569,8/s (−0,01%); query/cardinalidade/caos validam pré-requisitos e registram plano/bloqueio; `make ci` e `make smoke` verdes; evidência em `docs/dev/story-2.2-handoff.md` | `46a76cd` | Story 2.3 |
| 2026-11-01 | Story 2.3 | P0 / 2.3 / AC1–AC3 | `resources.json` a 1 Hz (CPU/RAM/disco + rusage do loadgen), `config_hash`/versões/hardware no `run.json`, `compressed_bytes` no loadgen, `benchctl report` com `report.md`+`report.json`, cobertura de métricas por story e variância documentada; 3×60s HTTP/gzip reais a 99.959,9/99.966,6/99.967,0/s (−0,03% a −0,04%), report **pass** (throughput 0,01%, p95 1,22%, CPU 7,03%); 5s/30s falharam p99/CPU e foram reportados; `make ci`, `make smoke` e `go test -race` verdes; evidência em `docs/dev/story-2.3-handoff.md` | `d47ca9e` | Story 2.4 |
| 2026-11-01 | Story 2.4 | P0 / 2.4 / AC1–AC3 | Perfil T1 (`t1.json`: 6 CH/3 VM/6 gateway/4 NATS/2 loadgen; 1M/s; 168 h), Terraform AWS de referência validado, `cluster.sh` com `check/cost/plan/apply/status/destroy` e guards de gasto/root, modelo de custo (USD 41,79/h; 30.504,49/mês), `baseline.sh` (warm-up/ramp/medido) e `bench-sink`; pré-baseline local 3×60s a 99.932–99.939/s (0,007% spread; report **pass**), publicada em `BENCHMARKS.md`; `make ci`, `bench-env-check`, `go test -race` e `terraform validate` verdes; cluster T1 **não** provisionado; evidência em `docs/dev/story-2.4-handoff.md` | `5b6096d` | P-07 + 3.1/4.1 |
| 2026-11-02 | Story 3.1 | P0 / 3.1 / AC1–AC3 | Schema ClickHouse de logs (`0001_otel_logs.sql`) com partição/TTL por `ingested_at`, `ORDER BY` tenant/projeto/serviço/tempo, TTL por `retention_days` e codecs; teste de contrato no `make test`; dataset local de 1B linhas em 24 h (20×50M) em 40 min 20 s, **6,98x** de compressão, `q03` p95 **263 ms**/p99 299 ms, TTL 7/15 pass; merges parados no run local para caber no disco; `make fmt/lint/test/build/smoke` verdes; **não é G1**; evidência em `docs/dev/story-3.1-handoff.md` e `bench/clickhouse/results/2026-11-02-local-1b/` | `494391a` | Story 3.2 |
| 2026-11-02 | Story 3.2 | P0 / 3.2 / AC1–AC4 | Schema ClickHouse de traces (`0002_otel_traces.sql`) com partição/TTL por `ingested_at`, `ORDER BY` tenant/projeto/serviço/status/tempo, bloom filters em trace/span/parent, `set` de serviço e nenhuma MV de RED (AD-21); teste de contrato no `make test`; dataset local de 1B spans (100M traces de 10 spans, 20×50M) em 32 min 31 s, **7,52x** de compressão, trace lookup p95 **328 ms**, serviço/status/24h p95 **178 ms**, service map 9 nós/9 arestas e correlação log↔trace nos dois sentidos; merges parados no run local; `make fmt/lint/test/build/smoke` verdes; **não é G1**; evidência em `docs/dev/story-3.2-handoff.md` e `bench/clickhouse/traces/results/2026-11-02-local-1b/` | `03f01e7` | Story 3.3 |
| 2026-11-02 | Story 3.3 | P0 / 3.3 / AC1–AC3 | Writer ClickHouse (`internal/adapters/clickhouse/writer`) com batches RowBinary, spool local durável (`fsync`+checkpoint), dedupe por `event_id`, `insert_deduplication_token` estável, retry, DLQ e métricas; migration `0003_ingest_dedupe.sql`; `chloadgen` com `--dedupe-test`, `--recover`, `--crash-after-insert` e dry-run; `bench/clickhouse/ingest/run.sh` e alvos `make bench-clickhouse-ingest*`; run local de **400k/s por 60s** (24M eventos, lag p95 **1,4s**, 18 parts, 0 `Too many parts`, 0 erros de insert), dedupe de 50k e recuperação de 100k após crash; `make fmt/lint/test/build` e `make smoke` verdes; **não é G1**; 1M/s/1h no T1 pendente; evidência em `docs/dev/story-3.3-handoff.md` e `bench/clickhouse/ingest/results/2026-11-02-local-400k/` | `1c6af25` | Story 3.4 |
| 2026-11-02 | Diagramas de arquitetura | — | Pacote drawio de 6 páginas renderizado da spine (contexto C4 L1, containers C4 L2, deploy AWS multi-AZ, ingestão com fronteira de ack, consulta com contexto assinado, ERD do control plane), com o AD citado em cada nó/aresta e PNGs exportados; data plane segue self-hosted em EKS (AD-5/AD-12/AD-18) e a AWS entra como hospedagem; divergência `PANEL` do ERD da spine × AD-28 registrada em `docs/architecture/README.md`; nenhum AD novo, nenhuma story alterada | `58f4a68` | Story 3.4 |
| 2026-11-02 | Correção do ERD da spine | — | ERD do control plane alinhado a AD-28 e ao DDL canônico do addendum: `PANEL` removido, canais via `alert_rule_channel`, retenção só em `retention_policies`, `silences` e `audit_events` com o dono correto e `error_budgets` 1:1 com `slos`; nenhum AD novo e AD-28 inalterado (correção de seed, registrada no log de decisões da spine); `lint_spine.py` com **0 achados**; página 6 do drawio atualizada e reexportada | `f45c5dd` | Story 3.4 |
| 2026-11-02 | Story 3.4 | P0 / 3.4 / AC1–AC3 | Gerador remote write protobuf+snappy, catálogo determinístico de 1M séries (2 tenants), 20 consultas MetricsQL e proxy de admissão por tenant (AD-33); run local de 1M/s por 1h (**3,6B amostras**, 999.738/s, p95 de carga 13ms, RSS 1,24GiB, 15,27GiB), consultas p95 **799,5ms** (20/20, 7/7 checks; `m14` 1.305ms), tenant-a 100k séries rejeitadas, tenant-b 0, drift **4,82%**; `make fmt/lint/test/build`, `make smoke` e `make bench-victoriametrics-check` verdes; **não é G1**; evidência em `docs/dev/story-3.4-handoff.md` e `bench/victoriametrics/results/2026-11-02-local-1m-1h/` | `fc2f0c1` + `9369638` | Story 3.5 |
| 2026-11-02 | Story 3.5 | P0 / 3.5 / AC1–AC3 | Migração `0004_tenancy` (project_generation/authz_epoch, tenant_scope, row policies), 16 consultas SQL do catálogo, harness `bench/clickhouse/query` e rollups 1m; dataset de **1B eventos** (300M logs + 100M spans) com p95 máx **431,94ms** e p99 <2x, 4 MetricsQL p95 **799,5ms**, 15/15 testes de isolamento + epoch AD-20, TTL 7/15/30 e mutação de política em 10M linhas (**493ms**), 10B/7d projetado **617ms**, 86B/24h em rollup (p95 máx **87,14ms**); `make fmt/lint/test/build`, `make smoke` e `make bench-clickhouse-query-check` verdes; **não é G1**; evidência em `docs/dev/story-3.5-handoff.md` e `bench/clickhouse/results/2026-11-02-local-1b-mix/` | `fe3ce08` + `ff79762` + `2cf30b8` | Story 4.1 |
| 2026-11-02 | Story 4.1 | P0 / 4.1 / AC1–AC4 | Gateway OTLP (`internal/ingest`) com receptores gRPC :4317 e HTTP :4318 para traces/logs/métricas, protobuf/JSON/gzip, `partial_success` a partir do sink, limites configuráveis de 10 MiB/10k eventos, erros `google.rpc.Status` com `request_id` UUIDv7, redaction por chave e Luhn antes do sink preservando `trace_id`/`span_id`, `/healthz`, `/readyz`, `/metrics` e drain no SIGTERM; `DiscardSink` placeholder até 4.3; `go test ./...`, `go test -race`, `golangci-lint`, `make ci` e `make smoke` (OTLP HTTP/gRPC + SIGTERM) verdes; **não é G1**; evidência em `docs/dev/story-4.1-handoff.md` | `05b09e8` + `202a223` + `b03fc52` + `d6bc9b1` | Story 4.2 |
| 2026-11-03 | Story 4.2 | P0 / 4.2 / AC1–AC3 | Snapshot versionado (`config_version`/`generation`/`issued_at`/`expires_at`) com tokens SHA-256, escopos e quota por tenant; auth Bearer 401, cross-project 403 + audit JSONL com hash chain, quota 429 + `Retry-After` e `RetryInfo`; pré-admissão HTTP por `Content-Length`/EWMA antes do decode; `Batch` carrega tenant/projeto/geração; assinatura Ed25519 em staging/produção; `make ci`/`make smoke` verdes (401/403/audit/429); harness local com 26.703 requests limitados, p95 de processamento +15,13% em 1M/s ofertado (inconclusivo por host compartilhado; T1 pendente); **não é G1**; evidência em `docs/dev/story-4.2-handoff.md` e `bench/gateway/quota/results/20261103T030500Z/` | `dab5e98` | Story 4.3 |
| 2026-11-03 | Story 4.3 | P0 / 4.3 / AC1–AC3 | Envelope protobuf v1 (`observaki.ingest.v1`) com zstd, batching por subject, 32 shards por tenant/projeto/epoch/signal e `event_id` determinístico por evento; sink NATS JetStream com streams por sinal (`DiscardNew`/MaxAge/MaxBytes), ack só após publish, retry com `Nats-Msg-Id`, 429 `wal_backpressure`, 503 e `/readyz`; `walcheck` relê sem ack; smoke cobre OTLP HTTP/gRPC + WAL; run local 100k/s por 10 s (1M eventos, **5,46x**, 32 shards, 0 duplicatas, p99 **28,9 ms**) e 1M/s mínimo (5M eventos, ~740k/s, p99 25,4 ms); redaction Luhn passou de regex para scanner linear (~8,9 ms → ~1,8 ms por batch de 500); **não é G1**; 1M/s/1h no T1 pendente; evidência em `docs/dev/story-4.3-handoff.md` e `bench/gateway/wal/results/2026-11-03-local-100k/` | `794afd4` | Story 4.4 |
| 2026-11-04 | Story 4.4 | P0 / 4.4 / AC1–AC3 | Writers (`observaki writer`) com três consumidores duráveis JetStream, ack só após o insert confirmado no ClickHouse (ou DLQ) ou o HTTP 2xx do VictoriaMetrics, dedupe por `event_id` + geração/epoch, `insert_deduplication_token` derivado dos eventos, retry exponencial, DLQ `OBSERVAKI_DLQ`, INSERT de 17 colunas explícitas (inclui `project_generation`/`authz_epoch`) e adaptador remote write; run local 20k/s por 20 s (400k eventos, lag p95 **0,80 s**, 135.300 logs/132.000 traces exatos, 139.400 amostras no VM, 0 falhas) e caos de 3 s no CH/2 s no VM (8 retries, zero perda, lag p95 7,97 s); `make fmt/lint/test/build/smoke` e `bench-clickhouse-ingest-check` verdes; **não é G1**; T1/4.5 pendentes; evidência em `docs/dev/story-4.4-handoff.md` e `bench/pipeline/results/2026-11-03-local-20k/` | `0699eed` | Story 4.5 |

## Regras de atualização

- **Não marque `done` sem evidência**: teste, benchmark, commit/PR ou artefato verificável.
- **Uma story por sessão** (salvo acordo explícito). Ao terminar, pare e reporte.
- **Se bloquear**: `status: blocked`, preencha `blocked_by` com a pergunta/decisão e pare.
- **Se mudar de fase**: atualize `phase`, `phase_name`, `epic` e o gate; registre no session log.
- **Se uma decisão mudar**: atualize `DECISIONS.md` e o AD correspondente; não faça override silencioso.
- **Não apague histórico** do session log; apenas append.
- **Mantenha o arquivo curto**: uma linha por sessão, sem transcrição.
