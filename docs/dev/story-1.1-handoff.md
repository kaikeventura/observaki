# Story 1.1 — Handoff e revisão

**Status:** implementação concluída; revisão dispensada explicitamente pelo dono
em 2026-10-31 (fluxo main-only). Story 1.1 marcada `done`.
**Branch:** `main` (fluxo do dono: merge `--no-ff` ao fim de cada sessão).
**Último commit de código:** `8654bd6` (evidência da T1.1.8).

## O que foi entregue

| Área | Caminho | Conteúdo |
|---|---|---|
| API Go | `cmd/observaki`, `internal/{httpserver,version,config}` | `version`, `serve`, `/healthz`, `/api/v1/health`, shutdown gracioso, config validada |
| Frontend | `web/` | SvelteKit 3 + Svelte 5 + TypeScript + Vite 8 + Tailwind 4; cartão de status da API; proxy `/api`; Vitest |
| Agente | `agent/` | workspace Rust, `rust-toolchain.toml` (stable + rustfmt/clippy), binário `observaki-agent --version` |
| Toolchain | `Makefile`, `scripts/` | `bootstrap`, `fmt`, `lint`, `test`, `build`, `dev`, `smoke`, `clean`, `ci`, `tools`, `check-toolchain` |
| Config | `.env.example`, `internal/config` | cinco variáveis com defaults e validação com nome/valor no erro |
| Docs | `README.md`, `docs/dev/local-setup.md` | quickstart, comandos, portas, troubleshooting, validação da T1.1.8 |

## Como validar

```bash
make bootstrap && make ci && make smoke
./bin/observaki version
curl -s localhost:8080/healthz
```

Esperado: `make smoke` termina com `Smoke: OK (healthz, api health, web build, version)`;
`version` imprime SHA/commit/data; os endpoints retornam `{"status":"ok"}`.
Evidência de clone limpo (bootstrap 1,5 s, ci 13,1 s, smoke 10,3 s) em
[`docs/dev/local-setup.md`](local-setup.md#validação-da-story-11-t118).

## Decisões de versão e toolchain

- **Módulo Go:** `github.com/observaki/observaki` (premissa da story); o remoto
  atual é `kaikeventura/observaki` — confirmar antes de publicar releases.
- **Go 1.27.2** no `go.mod`; `GOTOOLCHAIN=auto` resolve a toolchain.
- **Node 26.13.0** e pnpm 12.16.1 (via `corepack`); `check-toolchain` exige Node ≥ 26.
- **golangci-lint 2.15.0** precisa ser compilado com Go 1.27 (`make tools` fixa
  `GOTOOLCHAIN=go1.27.2`).
- **SvelteKit 3** não usa mais `svelte.config.js`: adapter e preprocess ficam no
  plugin `sveltekit()` do `vite.config.ts`.
- **Adapter estático + prerender** da página inicial; fallback SPA será
  adicionado quando houver rotas dinâmicas.
- **Fluxo Git:** commits diretos/merge `--no-ff` na `main`, por preferência do dono.

## Fora de escopo (não fazer nesta story)

- CI, SAST, SBOM e release assinada — Story 1.2.
- Licença, DCO, CoC, SECURITY e templates — Story 1.3.
- ADRs — Story 1.4.
- ClickHouse, VictoriaMetrics, PostgreSQL, NATS e Compose real — Story 5.1.
- Helm, UI de Explore/dashboards, auth e ingestão real.

## Handoff para a Story 1.2 (CI)

O CI deve chamar exatamente:

```bash
make ci      # bootstrap + lint + test + build
make smoke   # CI completo + smoke do binário em porta livre
```

Requisitos do runner:

- Linux com bash, `make`, `curl`; portas livres em `18080–18120` (smoke usa `/dev/tcp`).
- Go 1.27 (`GOTOOLCHAIN=auto`), Node 26 LTS, pnpm ≥ 10 via `corepack`,
  Rust stable + `rustfmt`/`clippy`, golangci-lint 2.x compilado com Go 1.27.
- Cache recomendado: módulos Go, store do pnpm, registry e `target/` do Cargo.
- `.env` não é necessário; os defaults da config valem no CI.
- Artefatos gerados: `bin/observaki`, `web/build/`, `agent/target/release/observaki-agent`.

Gaps conhecidos que a 1.2 pode fechar: nenhum workflow de CI existe ainda;
segredos de assinatura; matriz Linux/macOS; SBOM/SAST; release multi-arquitetura.

## Checklist de revisão

- [ ] Árvore e arquivos batem com a story 1.1 (desvios documentados: `svelte.config.js` removido pelo SvelteKit 3).
- [ ] Endpoints, shutdown gracioso e testes do servidor conferidos.
- [ ] Página, cartão de status, proxy `/api` e teste de renderização conferidos.
- [ ] Workspace Rust compila, testa e passa `fmt`/`clippy -D warnings`.
- [ ] `make bootstrap/ci/smoke/dev/clean` reproduzíveis.
- [ ] Config valida e falha citando variável e valor.
- [ ] Nenhum segredo, `.env`, build ou dado de benchmark versionado.
- [ ] Premissa do módulo Go e requisito de Node 26 aceitos.
