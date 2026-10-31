---
project: Observaki
phase: 0
phase_name: Fundação e Benchmarks
epic: P0-E1
epic_name: Fundação do repositório e governança
story: "1.2"
story_name: CI de qualidade e release alpha assinada
task: T1.2.5
task_name: Validação e evidência
status: in_progress        # not_started | in_progress | blocked | done
updated: 2026-10-31
branch: main
remote: "git@github.com:kaikeventura/observaki.git"
last_commit: "8957f4b"
next_action: "Empurrar o CI para a main, acompanhar o primeiro run no GitHub Actions e, se verde, criar a tag v0.0.1-alpha"
blocked_by: []
last_session: "Story 1.2 implementada (CI, CodeQL, security/SBOM, release assinada e Dockerfile); validação local ok; falta o run real no GitHub e a branch protection."
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
| Epic | **P0-E1 — Fundação do repositório e governança** |
| Story | **1.2 — CI de qualidade e release alpha assinada** |
| Task | **T1.2.5 — Validação e evidência** |
| Status | `in_progress` |
| Branch | `main` |
| Remoto | `https://github.com/kaikeventura/observaki.git` |
| Último commit | `8957f4b` |
| Próxima ação | Push da `main`, acompanhar o primeiro run do GitHub Actions e, se verde, criar a tag `v0.0.1-alpha` |
| Bloqueios | Branch protection é configuração manual no GitHub (documentada em `docs/ci.md`) |
| Última sessão | Story 1.2 implementada; validação local ok; falta run real |

**Ambiente local:** Node 26.13.0 instalado via `nvm` (alias `default` atualizado); pnpm 12.16.1 via `corepack` com shim em `~/.local/bin`; golangci-lint v2.15.0 em `~/go/bin` **compilado com `GOTOOLCHAIN=go1.27.0`** (a build padrão saiu com go1.26 e recusava o módulo). Rust 1.93 estável. `make bootstrap/lint/test/build/dev/smoke/clean` verificados nesta máquina.

**Nota de stack:** SvelteKit 3 removeu `svelte.config.js`; a configuração (adapter, preprocess) agora vai no plugin `sveltekit()` do `vite.config.ts`. O arquivo listado na story não existe mais.

## Fase 0 — checklist de stories

**Legenda:** `todo` · `doing` · `done` · `blocked`

| Epic | Story | Título | Status | Evidência / próximo passo |
|---|---|---|---|---|
| P0-E1 | 1.1 | Bootstrap do mono-repo | `done` | T1.1.1–T1.1.9 done; revisão dispensada pelo dono; evidência em `docs/dev/story-1.1-handoff.md` |
| P0-E1 | 1.2 | CI de qualidade e release alpha | `doing` | T1.2.1–T1.2.4 implementadas; T1.2.5: primeiro run no GitHub e tag `v0.0.1-alpha` |
| P0-E1 | 1.3 | Governança open source | `todo` | Depende de 1.1 |
| P0-E1 | 1.4 | ADRs | `todo` | Depende de 1.1 |
| P0-E2 | 2.1 | Gerador de carga OTLP | `todo` | Depende de 1.1 |
| P0-E2 | 2.2 | Cenários de benchmark | `todo` | Depende de 2.1 |
| P0-E2 | 2.3 | Relatório de benchmark | `todo` | Depende de 2.1/2.2 |
| P0-E2 | 2.4 | Ambiente reproduzível + baseline | `todo` | Depende de 2.2/2.3 |
| P0-E3 | 3.1 | Schema ClickHouse para logs | `todo` | Depende de 2.2 |
| P0-E3 | 3.2 | Schema ClickHouse para traces | `todo` | Depende de 3.1 |
| P0-E3 | 3.3 | Ingestão ClickHouse + dedupe | `todo` | Depende de 3.1/3.2 |
| P0-E3 | 3.4 | VictoriaMetrics + cardinalidade | `todo` | Depende de 3.1 |
| P0-E3 | 3.5 | Query/retenção/row policies | `todo` | Depende de 3.1-3.4 |
| P0-E4 | 4.1 | Gateway OTLP | `todo` | Depende de 1.1 |
| P0-E4 | 4.2 | Auth/quotas | `todo` | Depende de 4.1 |
| P0-E4 | 4.3 | Batching/WAL NATS | `todo` | Depende de 4.1/4.2 |
| P0-E4 | 4.4 | Writers CH/VM | `todo` | Depende de 4.3/3.3/3.4 |
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

- [ ] 1M eventos/s por 1 hora no workload de referência
- [ ] Lag p95 < 10s; zero perda para eventos confirmados
- [ ] p95 < 1s em 1B linhas/24h
- [ ] p95 < 3s em 10B linhas/7d
- [ ] Compressão ≥ 5x em logs e traces
- [ ] Retenção/row policies/quotas validadas
- [ ] Compose single-node < 15 min; Helm em 3 nós; upgrade/rollback
- [ ] 100k pontos < 100ms; bundle inicial < 180 KB gzip
- [ ] 3 design partners com dados reais
- [ ] ADRs aprovadas (storage, gateway, fila, frontend, tenancy, benchmark)
- [ ] Go/no-go registrado para cada spike
- [ ] `BENCHMARKS.md` publicado

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
| 2026-10-31 | Story 1.2 / T1.2.1–T1.2.4 | P0 / 1.2 / T1.2.5 | Workflows `ci.yml`, `codeql.yml`, `release.yml`, job security/SBOM (Syft), release com cosign keyless, `Dockerfile` distroless, `docs/ci.md` e `SMOKE_SKIP_CI`; `actionlint` e imagem local ok | `pendente` | Primeiro run no GitHub |

## Regras de atualização

- **Não marque `done` sem evidência**: teste, benchmark, commit/PR ou artefato verificável.
- **Uma story por sessão** (salvo acordo explícito). Ao terminar, pare e reporte.
- **Se bloquear**: `status: blocked`, preencha `blocked_by` com a pergunta/decisão e pare.
- **Se mudar de fase**: atualize `phase`, `phase_name`, `epic` e o gate; registre no session log.
- **Se uma decisão mudar**: atualize `DECISIONS.md` e o AD correspondente; não faça override silencioso.
- **Não apague histórico** do session log; apenas append.
- **Mantenha o arquivo curto**: uma linha por sessão, sem transcrição.
