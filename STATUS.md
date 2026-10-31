---
project: Observaki
phase: 0
phase_name: Fundação e Benchmarks
epic: P0-E1
epic_name: Fundação do repositório e governança
story: "1.1"
story_name: Bootstrap do mono-repo e ambiente de desenvolvimento
task: T1.1.4
task_name: Criar o workspace Rust placeholder do agente
status: in_progress        # not_started | in_progress | blocked | done
updated: 2026-10-31
branch: main
remote: "git@github.com:kaikeventura/observaki.git"
last_commit: "99a024f"
next_action: "T1.1.4 — criar agent/ como workspace Rust (rust-toolchain.toml + crates/observaki-agent) com --version, build e testes"
blocked_by: []
last_session: "T1.1.3 concluída: app SvelteKit 3 com cartão de status da API, proxy /api, testes e build estático."
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
| Story | **1.1 — Bootstrap do mono-repo e ambiente de desenvolvimento** |
| Task | **T1.1.4 — Criar o workspace Rust placeholder do agente** |
| Status | `in_progress` |
| Branch | `main` |
| Remoto | `https://github.com/kaikeventura/observaki.git` |
| Último commit | `99a024f` |
| Próxima ação | Criar `agent/` (workspace Rust + `rust-toolchain.toml` + binário `observaki-agent` com `--version`), build e testes |
| Bloqueios | Nenhum |
| Última sessão | T1.1.3 concluída (app SvelteKit mínimo) |

**Ambiente local (gaps para as próximas tarefas):** Node é v22 (alvo 26 LTS); pnpm 12.16.1 via `corepack`; a T1.1.5 vai exigir Node ≥ 26 no `check-toolchain.sh`. Go 1.22.2 instalado, mas o toolchain 1.27.0 é baixado automaticamente pelo `go.mod` (validado).

**Nota de stack:** SvelteKit 3 removeu `svelte.config.js`; a configuração (adapter, preprocess) agora vai no plugin `sveltekit()` do `vite.config.ts`. O arquivo listado na story não existe mais.

## Fase 0 — checklist de stories

**Legenda:** `todo` · `doing` · `done` · `blocked`

| Epic | Story | Título | Status | Evidência / próximo passo |
|---|---|---|---|---|
| P0-E1 | 1.1 | Bootstrap do mono-repo | `doing` | T1.1.1 (`ea625ad`), T1.1.2 (`1924ae6`), T1.1.3 (`99a024f`) done; próximo: T1.1.4 |
| P0-E1 | 1.2 | CI de qualidade e release alpha | `todo` | Depende de 1.1 |
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

## Regras de atualização

- **Não marque `done` sem evidência**: teste, benchmark, commit/PR ou artefato verificável.
- **Uma story por sessão** (salvo acordo explícito). Ao terminar, pare e reporte.
- **Se bloquear**: `status: blocked`, preencha `blocked_by` com a pergunta/decisão e pare.
- **Se mudar de fase**: atualize `phase`, `phase_name`, `epic` e o gate; registre no session log.
- **Se uma decisão mudar**: atualize `DECISIONS.md` e o AD correspondente; não faça override silencioso.
- **Não apague histórico** do session log; apenas append.
- **Mantenha o arquivo curto**: uma linha por sessão, sem transcrição.
