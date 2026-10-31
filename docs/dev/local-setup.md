# Setup local — Observaki

Guia para sair de um clone vazio até a API e o frontend rodando em Linux,
com a toolchain validada. Tempo esperado: **≤ 10 minutos** com rede razoável.

Nesta fase (Fase 0 / Story 1.1) **não** sobem ClickHouse, VictoriaMetrics,
PostgreSQL nem NATS — isso é a Story 5.1. O `make dev` sobe apenas a API Go e
o dev server do SvelteKit.

## Pré-requisitos

| Ferramenta | Versão testada | Mínimo | Como obter |
|---|---|---|---|
| Go | 1.27.0 | 1.24 (o `go.mod` exige 1.27) | [go.dev/dl](https://go.dev/dl/) ou `GOTOOLCHAIN=auto` com Go ≥ 1.21 |
| Node.js | 26.13.0 LTS | 26 | `nvm install 26 && nvm use 26` |
| pnpm | 12.16.1 | 10 | `corepack enable pnpm` |
| Rust | 1.93.0 stable | 1.85 | `rustup update stable` |
| rustfmt / clippy | 1.93.0 | — | `rustup component add rustfmt clippy` |
| golangci-lint | 2.15.0 | 2.x | `make tools` |
| make, curl, git | — | recente | gerenciador de pacotes da distro |
| Docker Compose | v5.6.0 | v2 (opcional) | só a partir da Story 5.1 |

Verifique tudo de uma vez:

```bash
./scripts/check-toolchain.sh
```

O script falha com mensagem de correção para versão ausente ou antiga.
Docker Compose ausente é apenas aviso nesta fase.

## Quickstart (5 comandos)

```bash
make bootstrap   # valida a toolchain, instala dependências e cria .env
make test        # Go + Vitest + cargo test
make build       # binário Go + web/build + agente release
make dev         # API em :8080 e SvelteKit em :5173
make smoke       # em outro terminal: CI completo + smoke do binário
```

Abra <http://localhost:5173>: a página mostra o cartão **API status** com
“Operational” quando a API está respondendo.

## Comandos

| Comando | O que faz |
|---|---|
| `make bootstrap` | roda `check-toolchain`, baixa módulos Go, instala o `web/` com pnpm, faz `cargo fetch` e copia `.env.example` para `.env` se necessário |
| `make check-toolchain` | valida Go, Node, pnpm, Rust, golangci-lint e utilitários |
| `make tools` | instala o golangci-lint com a toolchain Go do projeto |
| `make fmt` | `go fmt`, Prettier (inclui Svelte) e `cargo fmt` |
| `make lint` | golangci-lint, Prettier `--check`, svelte-check e `clippy -D warnings` |
| `make test` | `go test ./...`, `vitest run` e `cargo test` |
| `make build` | `bin/observaki` com versão/commit/data via `-ldflags`, build estático do SvelteKit e agente em release |
| `make dev` | API Go + SvelteKit em paralelo, com trap que encerra os dois |
| `make smoke` | `make ci` e depois sobe `bin/observaki` em porta livre, validando `/healthz`, `/api/v1/health` e `web/build/index.html` |
| `make clean` | remove `bin/`, `web/build/`, `web/.svelte-kit/` e `agent/target/` |
| `make ci` | `bootstrap lint test build` |

## Portas e configuração

| Serviço | URL padrão | Variável |
|---|---|---|
| API Go | <http://localhost:8080> | `OBSERVAKI_HTTP_ADDR` |
| Health | `/healthz` e `/api/v1/health` | — |
| SvelteKit dev | <http://localhost:5173> | `WEB_PORT` |
| Proxy do frontend | `/api/*` → `http://localhost:8080` | `OBSERVAKI_API_URL` |

As demais variáveis (`OBSERVAKI_LOG_LEVEL`, `OBSERVAKI_ENV`,
`OBSERVAKI_DATABASE_URL`, `OBSERVAKI_WEB_DIR`) estão documentadas em
[`.env.example`](../../.env.example). O `make dev` carrega `.env`
automaticamente; valores inválidos fazem o processo falhar citando a variável
e o valor recusado.

## Verificação rápida

```bash
./bin/observaki version
curl -s localhost:8080/healthz
curl -s localhost:8080/api/v1/health
test -f web/build/index.html && echo "web build ok"
```

## Troubleshooting

### 1. `check-toolchain` falha com Node antigo

**Sintoma:** `Node v22.x is too old (>= 26 required)`.

**Correção:**

```bash
nvm install 26
nvm use 26
node --version   # v26.x
```

Se o `nvm use` reclamar de `prefix`/`globalconfig` no `~/.npmrc`, rode
`nvm use --delete-prefix 26`.

### 2. Go antigo ou `go.mod requires go >= 1.27.0`

**Sintoma:** erro ao rodar `go build`/`go test` com Go local antigo.

**Correção:** mantenha `GOTOOLCHAIN=auto` (default) para o Go baixar a
toolchain exigida pelo `go.mod`, ou instale Go 1.27+. Confirme com
`go env GOVERSION` dentro do repositório.

### 3. `pnpm: command not found`

**Sintoma:** `make bootstrap` para em `pnpm --dir web install`.

**Correção:**

```bash
corepack enable pnpm
pnpm --version
```

Se o `corepack enable` falhar com `EACCES` ao criar link em `/usr/bin`,
instale o shim no seu HOME:

```bash
corepack enable --install-directory "$HOME/.local/bin" pnpm
```

### 4. Porta ocupada (`bind: address already in use`)

**Sintoma:** `make dev` falha porque `:8080` ou `:5173` já está em uso.

**Correção:** descubra o processo e encerre, ou use outras portas:

```bash
ss -ltnp | grep -E ':8080|:5173'
OBSERVAKI_HTTP_ADDR=:18080 WEB_PORT=5174 make dev
```

### 5. `golangci-lint` recusa o projeto

**Sintoma:** `the Go language version (go1.26) used to build golangci-lint is
lower than the targeted Go version (1.27.0)`.

**Correção:** reinstale com a toolchain do projeto:

```bash
make tools
```

O alvo fixa `GOTOOLCHAIN=go1.27.0` de propósito.

### 6. `cargo`/`rustfmt`/`clippy` ausentes

**Sintoma:** `make build` ou `make lint` falha no agente.

**Correção:**

```bash
rustup update stable
rustup component add rustfmt clippy
```

### 7. Página abre, mas o cartão fica em “API unavailable”

**Causa:** a API Go não está rodando (ou está em outra porta).

**Correção:** confirme `curl localhost:8080/healthz`. Se mudou a porta no
`.env`, ajuste também `OBSERVAKI_API_URL` para o proxy do Vite.

### 8. Docker Compose ausente

**Sintoma:** aviso `Docker Compose v2 not found (not required until Story 5.1)`.

**Ação:** nenhuma nesta fase. O Compose real entra na Story 5.1.

## Limpeza

```bash
make clean
```

Remove artefatos de build; dependências instaladas (`node_modules/`,
`~/.cargo`, módulos Go) permanecem em cache para o próximo bootstrap.

## Validado por

| Data | Sistema | Ferramenta | Autor | Resultado |
|---|---|---|---|---|
| 2026-10-31 | Ubuntu 24.04.5 LTS (Linux 7.0) | Go 1.27.0, Node 26.13.0, pnpm 12.16.1, Rust 1.93.0, golangci-lint 2.15.0 | kaikeventura (autor do guia) | `make bootstrap/ci/smoke/dev/clean` ok |

A validação por uma pessoa que não escreveu o guia faz parte da T1.1.8.
