# Story 1.1 — Bootstrap do mono-repo e ambiente de desenvolvimento

**Epic:** 1 — Fundação do repositório e governança
**Tamanho:** S (5,25 dev-days)
**Status:** ready-for-dev
**PRD:** FR-53 (base), NFR-8, NFR-15; AR-1, AR-8, AR-10, AR-12.

## Objetivo

Um contribuidor consegue clonar o repositório, rodar `make bootstrap`, `make test`, `make build` e `make dev` em menos de 15 minutos, com Go, SvelteKit e o placeholder do agente Rust funcionando.

## Premissas

- O módulo Go será `github.com/observaki/observaki`; ajustar se o org/repo definitivo for outro.
- O repositório começa vazio; CI, licença e ADRs são stories separadas (1.2, 1.3, 1.4).
- O ambiente de desenvolvimento **não** sobe ClickHouse/VictoriaMetrics/Postgres nesta story; isso é a Story 5.1. O `make dev` sobe apenas o binário Go e o dev server do SvelteKit.
- O agente Rust é um placeholder estrutural; a implementação real é Fase 2.

## Estrutura-alvo do repositório

```text
.
├── cmd/
│   └── observaki/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── httpserver/
│   │   └── server.go
│   └── version/
│       └── version.go
├── web/
│   ├── package.json
│   ├── pnpm-lock.yaml
│   ├── svelte.config.js
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── src/
│   │   ├── app.html
│   │   ├── app.css
│   │   ├── routes/
│   │   │   └── +page.svelte
│   │   └── lib/
│   └── static/
├── agent/
│   ├── Cargo.toml
│   ├── rust-toolchain.toml
│   └── crates/
│       └── observaki-agent/
│           ├── Cargo.toml
│           └── src/
│               └── main.rs
├── bench/
│   └── README.md
├── deploy/
│   └── README.md
├── docs/
│   ├── dev/
│   │   └── local-setup.md
│   └── adr/
│       └── .gitkeep
├── scripts/
│   ├── check-toolchain.sh
│   ├── dev.sh
│   └── smoke.sh
├── .editorconfig
├── .env.example
├── .gitignore
├── .golangci.yml
├── Makefile
├── README.md
├── go.mod
└── go.sum
```

## Grafo de dependências

```text
1.1.1 ─┬─> 1.1.2 ─┐
       ├─> 1.1.3 ─┼─> 1.1.5 ─> 1.1.6 ─> 1.1.7 ─> 1.1.8 ─> 1.1.9
       └─> 1.1.4 ─┘
```

## Tarefas

### T1.1.1 — Criar o esqueleto do mono-repo (0,5 dia)

**Descrição:** criar a árvore de diretórios, arquivos-raiz e o módulo Go.

**Passos:**
1. Criar os diretórios da estrutura-alvo (`cmd`, `internal`, `web`, `agent`, `bench`, `deploy`, `docs`, `scripts`).
2. Criar `.editorconfig` com indentação, charset e fim de linha.
3. Criar `.gitignore` cobrindo Go (`bin/`, `dist/`), Node (`node_modules/`, `.svelte-kit/`, `build/`), Rust (`target/`), env (`.env`), IDE (`.idea/`, `.vscode/`) e artefatos de benchmark.
4. Rodar `go mod init github.com/observaki/observaki`.
5. Criar `README.md` mínimo com nome, descrição de uma frase, pré-requisitos e link para `docs/dev/local-setup.md`.
6. Criar `bench/README.md` e `deploy/README.md` como placeholders com “a preencher nas stories 2.x/5.x”.
7. Criar `docs/adr/.gitkeep`.

**Arquivos:** `.editorconfig`, `.gitignore`, `go.mod`, `README.md`, `bench/README.md`, `deploy/README.md`, `docs/adr/.gitkeep`.

**Concluído quando:**
- `go mod tidy` roda sem erro.
- A árvore de diretórios existe e está versionada.
- O README aponta para o guia de setup local.

**Comando de verificação:**
```bash
go mod tidy
git status --short
```

---

### T1.1.2 — Criar o binário Go com `version` e `serve` (0,5 dia)

**Descrição:** criar o entrypoint do backend e o endpoint de saúde que será usado pelo smoke test.

**Passos:**
1. Criar `internal/version/version.go` com variáveis `Version`, `Commit`, `Date` preenchidas por `-ldflags`.
2. Criar `cmd/observaki/main.go` com subcomandos:
   - `observaki version` — imprime versão, commit e data.
   - `observaki serve` — sobe HTTP em `OBSERVAKI_HTTP_ADDR` (default `:8080`).
3. Criar `internal/httpserver/server.go` com:
   - `GET /healthz` → `200 {"status":"ok"}`.
   - `GET /api/v1/health` → `200 {"status":"ok","version":"..."}`.
   - graceful shutdown em SIGTERM/SIGINT.
4. Adicionar testes de handler em `internal/httpserver/server_test.go`.

**Arquivos:** `cmd/observaki/main.go`, `internal/version/version.go`, `internal/httpserver/server.go`, `internal/httpserver/server_test.go`.

**Concluído quando:**
- `go run ./cmd/observaki version` imprime a versão.
- `go run ./cmd/observaki serve` sobe e responde em `/healthz`.
- `go test ./internal/httpserver` passa.

**Comando de verificação:**
```bash
go run ./cmd/observaki version
go test ./internal/httpserver
```

---

### T1.1.3 — Criar o app SvelteKit mínimo (1 dia)

**Descrição:** criar o frontend com SvelteKit 3, Svelte 5, TypeScript, Vite 8 e Tailwind 4, com uma página inicial e proxy para a API Go.

**Passos:**
1. Criar `web/` com `package.json`, `svelte.config.js`, `vite.config.ts`, `tsconfig.json`.
2. Configurar scripts: `dev`, `build`, `preview`, `lint`, `check`, `test`, `format`.
3. Criar `src/app.html`, `src/app.css` (Tailwind) e `src/routes/+page.svelte` com o texto “Observaki” e um cartão de status que chama `/api/v1/health`.
4. Configurar proxy do Vite: `/api` → `http://localhost:8080`.
5. Adicionar Vitest + Testing Library (ou teste de componente mínimo) e um teste de renderização da página.
6. Garantir `pnpm build` gera `web/build/`.

**Arquivos:** `web/package.json`, `web/svelte.config.js`, `web/vite.config.ts`, `web/tsconfig.json`, `web/src/app.html`, `web/src/app.css`, `web/src/routes/+page.svelte`, `web/src/routes/page.test.ts`.

**Concluído quando:**
- `pnpm --dir web install` instala dependências.
- `pnpm --dir web check` passa.
- `pnpm --dir web test` passa.
- `pnpm --dir web build` gera o build.
- `pnpm --dir web dev` abre a página e o status da API aparece quando o Go está rodando.

**Comando de verificação:**
```bash
pnpm --dir web install
pnpm --dir web check
pnpm --dir web test
pnpm --dir web build
```

---

### T1.1.4 — Criar o workspace Rust placeholder do agente (0,5 dia)

**Descrição:** reservar a estrutura do agente Rust para evitar reestruturação na Fase 2.

**Passos:**
1. Criar `agent/Cargo.toml` como workspace com membro `crates/observaki-agent`.
2. Criar `agent/rust-toolchain.toml` com o canal stable e componentes `rustfmt`, `clippy`.
3. Criar `agent/crates/observaki-agent/Cargo.toml` e `src/main.rs` que imprime `observaki-agent <version>`.
4. Adicionar um teste unitário trivial e configurar `cargo fmt`/`clippy`.

**Arquivos:** `agent/Cargo.toml`, `agent/rust-toolchain.toml`, `agent/crates/observaki-agent/Cargo.toml`, `agent/crates/observaki-agent/src/main.rs`.

**Concluído quando:**
- `cargo build --manifest-path agent/Cargo.toml` passa.
- `cargo test --manifest-path agent/Cargo.toml` passa.
- `cargo run --manifest-path agent/Cargo.toml -p observaki-agent -- --version` imprime a versão.

**Comando de verificação:**
```bash
cargo build --manifest-path agent/Cargo.toml
cargo test --manifest-path agent/Cargo.toml
```

---

### T1.1.5 — Criar Makefile e scripts de toolchain (1 dia)

**Descrição:** padronizar bootstrap, lint, test, build, dev, smoke e clean.

**Passos:**
1. Criar `scripts/check-toolchain.sh` que valida:
   - Go ≥ 1.24
   - Node ≥ 26 LTS
   - pnpm ≥ 10
   - Rust ≥ 1.85
   - Docker Compose v2 (aviso, não bloqueia a Story 1.1)
   - `make`, `curl`
2. Criar `Makefile` com alvos:
   - `bootstrap`: check-toolchain, `go mod download`, `pnpm --dir web install`, `cargo fetch`, copia `.env.example` para `.env` se não existir.
   - `fmt`: `go fmt ./...`, `pnpm --dir web format`, `cargo fmt`.
   - `lint`: `golangci-lint run`, `pnpm --dir web lint`, `pnpm --dir web check`, `cargo clippy -- -D warnings`.
   - `test`: `go test ./...`, `pnpm --dir web test`, `cargo test`.
   - `build`: `go build -ldflags ... -o bin/observaki ./cmd/observaki`, `pnpm --dir web build`, `cargo build --release`.
   - `dev`: chama `scripts/dev.sh`.
   - `smoke`: chama `scripts/smoke.sh`.
   - `clean`: remove `bin/`, `web/build/`, `.svelte-kit/`, `target/`.
   - `ci`: `bootstrap lint test build`.
3. Criar `scripts/dev.sh` que sobe `go run ./cmd/observaki serve` e `pnpm --dir web dev` em paralelo, com trap para matar ambos.
4. Criar `scripts/smoke.sh` que roda `make ci`, inicia o binário em porta livre, chama `/healthz`, valida o build do web e encerra.

**Arquivos:** `Makefile`, `scripts/check-toolchain.sh`, `scripts/dev.sh`, `scripts/smoke.sh`.

**Concluído quando:**
- `make bootstrap` funciona em uma máquina limpa (Linux).
- `make ci` passa.
- `make dev` sobe Go + web e ambos respondem.
- `make smoke` passa.
- `make clean` remove os artefatos.

**Comando de verificação:**
```bash
make bootstrap
make ci
make smoke
```

---

### T1.1.6 — Configuração por ambiente (0,5 dia)

**Descrição:** centralizar configuração em variáveis de ambiente com defaults seguros.

**Passos:**
1. Criar `internal/config/config.go` com:
   - `OBSERVAKI_HTTP_ADDR` (default `:8080`)
   - `OBSERVAKI_LOG_LEVEL` (default `info`)
   - `OBSERVAKI_ENV` (default `development`)
   - `OBSERVAKI_DATABASE_URL` (opcional nesta story)
   - `OBSERVAKI_WEB_DIR` (default `web/build`)
2. Validar valores e retornar erro claro para configuração inválida.
3. Adicionar testes de parsing/defaults.
4. Criar `.env.example` documentando cada variável.
5. Carregar `.env` no `make dev` via `set -a; source .env; set +a` ou usar `godotenv` (decidir e documentar).

**Arquivos:** `internal/config/config.go`, `internal/config/config_test.go`, `.env.example`.

**Concluído quando:**
- `go test ./internal/config` passa.
- Variáveis inválidas retornam erro com nome e valor.
- `.env.example` está documentado no README.

**Comando de verificação:**
```bash
go test ./internal/config
```

---

### T1.1.7 — Documentação de setup local (0,5 dia)

**Descrição:** escrever o guia que uma pessoa nova segue sem ajuda.

**Passos:**
1. Criar `docs/dev/local-setup.md` com:
   - pré-requisitos e versões;
   - `make bootstrap`;
   - `make test`, `make lint`, `make build`;
   - `make dev` e portas;
   - `make smoke`;
   - troubleshooting (porta ocupada, versão errada, pnpm/cargo ausente).
2. Atualizar `README.md` com quickstart de 5 comandos.
3. Adicionar um checklist “validado por” com data e sistema operacional.

**Arquivos:** `docs/dev/local-setup.md`, `README.md`.

**Concluído quando:**
- Uma pessoa que não escreveu o guia consegue seguir do zero em Linux.
- Todos os comandos citados existem no Makefile.
- O troubleshooting cobre pelo menos 3 falhas comuns.

---

### T1.1.8 — Smoke test e validação final (0,5 dia)

**Descrição:** provar que a story atende aos critérios de aceitação em uma máquina limpa.

**Passos:**
1. Rodar `make bootstrap && make ci && make smoke` em uma máquina/container limpo.
2. Validar `./bin/observaki version` e `curl localhost:8080/healthz`.
3. Validar `pnpm --dir web build` e o conteúdo de `web/build/index.html`.
4. Registrar os resultados no PR (logs, versões, tempo de bootstrap).
5. Corrigir qualquer instrução quebrada.

**Concluído quando:**
- Todos os comandos passam sem intervenção manual.
- O tempo de bootstrap é ≤ 10 minutos em rede razoável.
- O PR contém evidência de execução.

---

### T1.1.9 — PR, revisão e handoff (0,25 dia)

**Descrição:** abrir o PR, descrever as decisões e passar o contexto para a Story 1.2.

**Passos:**
1. Abrir um único PR para a Story 1.1 (commits por tarefa).
2. Preencher descrição com:
   - o que foi criado;
   - como validar;
   - decisões de versão/toolchain;
   - o que ficou fora (CI, licença, ADRs, Compose real).
3. Pedir revisão de pelo menos 1 engenheiro.
4. Atualizar o status da story no backlog para `done`.
5. Deixar um comentário para a Story 1.2 com os comandos que o CI deve chamar (`make ci`, `make smoke`).

**Concluído quando:**
- PR aprovado e mergeado.
- CI verde.
- Story marcada como done.
- Handoff da Story 1.2 registrado.

---

## Sequenciamento sugerido

| Dia | Tarefas |
|---|---|
| 1 | T1.1.1; iniciar T1.1.2 e T1.1.3 em paralelo |
| 2 | T1.1.2, T1.1.3, T1.1.4 |
| 3 | T1.1.5, T1.1.6 |
| 4 | T1.1.7, T1.1.8 |
| 5 | T1.1.9 + buffer |

Com 2 engenheiros, o caminho crítico cai para ~3 dias: um faz Go/Makefile, outro faz SvelteKit/Rust.

## Definição de Done (Story 1.1)

- `make bootstrap` funciona em uma máquina limpa.
- `make ci` passa (lint, test, build).
- `make smoke` passa.
- `make dev` sobe Go + SvelteKit e a página mostra o status da API.
- `./bin/observaki version` imprime versão/commit/data.
- `web/build/` é gerado.
- `agent/` compila e testa.
- README + `docs/dev/local-setup.md` validados por outra pessoa.
- PR revisado e mergeado.
- Nenhum segredo ou artefato gerado commitado.

## Fora de escopo

- CI completo com SAST/SBOM/release assinada (Story 1.2).
- Licença, DCO, COC, SECURITY (Story 1.3).
- ADRs (Story 1.4).
- ClickHouse, VictoriaMetrics, PostgreSQL, NATS e Docker Compose real (Story 5.1).
- Helm (Story 5.2).
- UI real de Explore/dashboards (Story 6.1).
- Auth, ingestão real e queries (Stories 4.x/3.x).

## Riscos e mitigação

| Risco | Mitigação |
|---|---|
| Versões diferentes entre macOS/Linux | `scripts/check-toolchain.sh` e CI em ambos |
| SvelteKit gerar arquivos de demo desnecessários | Criar o mínimo manualmente e remover exemplos |
| Rust placeholder atrasar a story | Manter só workspace + binário “hello, version” |
| `make dev` conflitar portas | Variáveis `OBSERVAKI_HTTP_ADDR` e `WEB_PORT` |
| Módulo Go com org errada | Documentar a premissa e ajustar antes do merge |
| Bootstrap lento | Cache de `pnpm`/Go modules no CI; medir tempo |
