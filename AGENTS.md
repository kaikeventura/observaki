# AGENTS.md — Observaki

Este repositório é o **Observaki**, uma plataforma de observabilidade open source, self-hosted e OTel-native. O planejamento está completo; o desenvolvimento começa pela **Fase 0** e só avança com os gates.

## Estado atual

- **Fase 0 — não iniciada.** Nenhum código de produto foi escrito.
- O estado corrente vive em `STATUS.md` (fonte de verdade de onde estamos).
- O prompt de retomada pronto para novas sessões está em `PROMPT.md`.
- O primeiro trabalho é a **Story 1.1** (`_bmad-output/planning-artifacts/stories/story-1.1-tasks.md`).
- O backlog da Fase 0 está em `_bmad-output/planning-artifacts/epics.md`.
- O plano executivo está em `_bmad-output/planning-artifacts/PROGRAM-PLAN.md`.
- A arquitetura binding está em `_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md` (AD-1..AD-35).
- O design/UX binding está em `_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/DESIGN.md` e `EXPERIENCE.md`.

## Regras não negociáveis

1. **Não pule fases.** Fase 1 só começa depois do Gate G1 (1M/s, p95 < 1s em 1B linhas, compressão ≥ 5x). Fases 2-4 só depois dos gates anteriores.
2. **Não invente escopo.** Se a tarefa não estiver em uma story/FR, pare e pergunte. Non-goals do PRD são vinculantes.
3. **AD vence código.** Se uma implementação contradizer um AD, pare e proponha um AD novo/emenda; nunca faça override silencioso.
4. **Toda leitura de telemetria passa pelo Query Service.** Nunca conecte UI/API/alertas direto em ClickHouse ou VictoriaMetrics.
5. **Tenancy em profundidade.** Toda linha/consulta carrega `tenant_id`/`project_id`; nunca confie em header do cliente. Testes de isolamento são obrigatórios.
6. **RED tem um dono:** gateway calcula antes do sampling e grava no VictoriaMetrics. Não crie RED no ClickHouse.
7. **Durabilidade:** ack só depois do WAL/flush; dedupe por `event_id`; rejeições permanentes vão para DLQ. Não prometa exactly-once.
8. **Sem segredos no repo.** Tokens, senhas e chaves nunca em código, logs, fixtures ou commits.
9. **Performance é contrato.** Mudanças no caminho crítico exigem benchmark antes/depois; regressão é bug P1.
10. **Acessibilidade é definition of done.** WCAG 2.2 AA, teclado, foco visível, tabela alternativa para gráficos, estado nunca só por cor.

## Ordem de leitura antes de codar

1. `_bmad-output/planning-artifacts/README.md`
2. `_bmad-output/planning-artifacts/PROGRAM-PLAN.md`
3. `_bmad-output/planning-artifacts/epics.md`
4. A story/tarefa atual
5. `ARCHITECTURE-SPINE.md` (ADs relevantes)
6. `DESIGN.md` / `EXPERIENCE.md` (se houver UI)
7. `DECISIONS.md` (decisões confirmadas e pendências)

## Protocolo de sessão (obrigatório)

Cada sessão do OpenCode é independente. O estado precisa sobreviver na `STATUS.md`.

**Ao iniciar:**
1. Leia `STATUS.md` antes de qualquer outra ação.
2. Use `phase`, `epic`, `story`, `task` e `next_action` como posição atual.
3. Não recalcule o roadmap; não pule para outra fase.
4. Se `status: blocked`, leia `blocked_by` e pare.

**Ao encerrar:**
1. Atualize o frontmatter de `STATUS.md` (`task`, `status`, `updated`, `next_action`, `blocked_by`, `last_session`).
2. Atualize a linha da story na tabela da Fase 0.
3. Adicione uma linha no **Session log** (append-only).
4. Registre evidência (commit/PR, comandos, teste/benchmark).
5. Pare. Não emende a próxima story sem nova ordem.

**Regra:** `done` só com evidência. Sem teste/benchmark, não marque done.

## Fluxo de trabalho

Para cada story:

1. Leia a story inteira e os ADs/UX relevantes.
2. Verifique dependências e se a story está “ready”.
3. Implemente **apenas** a story atual; não faça refactors fora do escopo.
4. Rode `make fmt`, `make lint`, `make test`, `make build` (quando o scaffold existir).
5. Rode o smoke/benchmark aplicável.
6. Atualize a documentação/ADR/memlog se a decisão mudou.
7. Registre evidência no PR: comandos, resultados, screenshots quando UI.
8. Atualize `STATUS.md` com o resultado e o próximo passo.
9. Faça commit com Conventional Commits + DCO (`git commit -s`) e push da branch. Se o push falhar por auth, mantenha o commit local e reporte.
10. Pare e reporte; não emende a próxima story sem confirmação.

## Git

O fluxo completo está em `GIT-WORKFLOW.md`. Regras mínimas:
- branch por story: `feat/p0-s1.1-bootstrap`;
- commits pequenos, Conventional Commits, `-s` (DCO);
- nunca `force-push` em `main`;
- nunca commitar segredo, `.env`, build ou dados de benchmark;
- ao fim da sessão: checks → `STATUS.md` → commit → push → registrar SHA/PR.

## Comandos (após o scaffold da Story 1.1)

```bash
make bootstrap     # instala/valida toolchain
make fmt           # gofmt + prettier + cargo fmt
make lint          # golangci-lint + svelte-check + clippy
make test          # go test + vitest + cargo test
make build         # binário Go + build SvelteKit + agente
make dev           # Go + SvelteKit em paralelo
make smoke         # smoke test do binário/health
make ci            # bootstrap + lint + test + build
```

Enquanto o scaffold não existir, use os comandos da Story 1.1. Não crie um Makefile alternativo.

## Convenções de código

- **Go:** `cmd/observaki`, `internal/...`; pacotes lowercase; interfaces em `ports`, adapters em `adapters`; erros estruturados; contexto explícito; sem panic em request path.
- **Svelte/TS:** SvelteKit 3, Svelte 5 runes, TypeScript strict; componentes em PascalCase; rotas kebab-case; sem `any` sem justificativa; sem fetch direto em componente (use `lib/api`).
- **Rust (agente, Fase 2):** workspace em `agent/`; `rustfmt` + `clippy -D warnings`; sem `unsafe` sem ADR.
- **SQL:** migrações versionadas; `snake_case`; índices nomeados; TTL por `ingested_at`; sem `SELECT *` em produção.
- **API:** proto é canônico; OpenAPI gerado; `/api/v1`; erros `{code,message,details,request_id}`; paginação keyset.
- **Commits:** Conventional Commits em inglês; DCO (`Signed-off-by`); PR pequeno.
- **Docs:** produto em Português-BR; código/UI em inglês; ADRs em `docs/adr/`.

## Bancos e dados

- ClickHouse: logs/traces/eventos; `MergeTree` com partição/TTL por `ingested_at`; `event_id` para dedupe; sem RED.
- VictoriaMetrics: métricas e RED; acesso via `vmauth`; retenção global no MVP.
- PostgreSQL: control plane; migrações versionadas; `audit_events` append-only.
- NATS JetStream: WAL; stream retém outage + reparo; DLQ para rejeições permanentes.
- Nunca rode migração/destroy em produção sem confirmação explícita.

## UI/UX

- Dark-first, desktop-first, keyboard-first; tokens de `DESIGN.md`; sem cores hardcoded fora dos tokens.
- Toda tela tem estados: loading, empty, error, timeout, partial, no permission, rate limit, degraded.
- Toda ação destrutiva tem confirmação com nome do recurso; `Esc` cancela; foco começa em cancelar.
- Toda query tem time range global sticky, timezone e URL reproduzível.
- Nada de hover-only; drag tem alternativa por teclado/menu.
- Não use “AI magic” que esconde a query; mostre a consulta e a proveniência.

## Quando parar

Pare e peça decisão humana quando:

- a story não tem AC testável ou conflita com um AD;
- é necessário mudar contrato público, licença, tenancy ou retenção;
- um benchmark falha ou regride;
- a solução exige uma dependência nova relevante;
- há dúvida sobre segurança, PII ou isolamento;
- o trabalho sair do escopo da fase/story.

## Comandos OpenCode úteis

- `/continue` — continua exatamente do `next_action` em `STATUS.md`.
- `/status` — mostra fase/story/task/próximo passo, sem editar.
- `/handoff` — encerra a sessão e atualiza o `STATUS.md` com evidência e próximo passo.
- `/next-story` — pega a próxima story pronta da Fase 0 e implementa só ela.
- `/phase0` — continua a Fase 0 na ordem, sem pular gates.
- `/gate-check` — verifica os critérios do gate atual, sem implementar.
- `/benchmark` — roda o benchmark e atualiza o relatório, sem mudar código de produto.

## Links

- PRD: `_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md`
- Addendum: `_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md`
- Architecture: `_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md`
- UX: `_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/`
- Phase 0 backlog: `_bmad-output/planning-artifacts/epics.md`
- Phase 1 backlog: `_bmad-output/planning-artifacts/epics-phase-1.md`
- Phase 2-4: `_bmad-output/planning-artifacts/epics-phase-2-4.md`
- Decisions: `_bmad-output/planning-artifacts/DECISIONS.md`
