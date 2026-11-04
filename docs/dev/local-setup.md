# Setup local — Observaki

Guia para sair de um clone vazio até a API e o frontend rodando em Linux,
com a toolchain validada. Tempo esperado: **≤ 10 minutos** com rede razoável.

O `make dev` sobe apenas a API Go e o dev server do SvelteKit. A stack
completa — API/UI, gateway, writer, ClickHouse, VictoriaMetrics, PostgreSQL e
NATS — sobe com `make compose-up` (Story 5.1), documentada em
[`deploy/compose/README.md`](../../deploy/compose/README.md). O
`observaki gateway` exige um NATS JetStream; o exemplo abaixo sobe um container
local descartável.

## Pré-requisitos

| Ferramenta | Versão testada | Mínimo | Como obter |
|---|---|---|---|
| Go | 1.27.2 | 1.27.2 (o `go.mod` exige 1.27.2) | [go.dev/dl](https://go.dev/dl/) ou `GOTOOLCHAIN=auto` com Go ≥ 1.21 |
| Node.js | 26.13.0 LTS | 26 | `nvm install 26 && nvm use 26` |
| pnpm | 12.16.1 | 10 | `corepack enable pnpm` |
| Rust | 1.93.0 stable | 1.85 | `rustup update stable` |
| rustfmt / clippy | 1.93.0 | — | `rustup component add rustfmt clippy` |
| golangci-lint | 2.15.0 | 2.x | `make tools` |
| make, curl, git | — | recente | gerenciador de pacotes da distro |
| Docker Compose | v5.6.0 | v2 | `make compose-up` (Story 5.1); opcional para `make dev` |

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

## Gateway OTLP e tokens

O comando `gateway` **exige** um snapshot de configuração com tokens de
ingestão; não existe modo anônimo no binário. O snapshot é um JSON versionado
(AD-19) com `tenant_id`, `project_id`, `ingest_quota` e o hash SHA-256 de cada
token. O token em texto claro nunca é gravado no arquivo.

```bash
TOKEN="$(openssl rand -hex 24)"
HASH="$(printf '%s' "$TOKEN" | sha256sum | cut -d' ' -f1)"
mkdir -p /tmp/observaki-dev

cat >/tmp/observaki-dev/ingest-auth.json <<EOF
{
  "config_version": "dev-1",
  "generation": 1,
  "issued_at": "2026-01-01T00:00:00Z",
  "expires_at": "2030-01-01T00:00:00Z",
  "tenants": [{
    "tenant_id": "tenant-dev",
    "ingest_quota": {"events_per_second": 100000, "burst_events": 100000},
    "projects": [{
      "project_id": "project-dev",
      "tokens": [{"token_id": "token-dev", "sha256": "${HASH}", "scopes": ["ingest"]}]
    }]
  }]
}
EOF

OBSERVAKI_INGEST_AUTH_FILE=/tmp/observaki-dev/ingest-auth.json \
OBSERVAKI_INGEST_AUDIT_FILE=/tmp/observaki-dev/audit.jsonl \
./bin/observaki gateway
```

Antes, suba um NATS com JetStream (o gateway falha fechado se o WAL não
conectar):

```bash
docker run -d --rm --name observaki-nats -p 4222:4222 \
  nats:2.15.2-alpine -js -sd /data
```

Envie telemetria com o token no header `Authorization: Bearer`:

```bash
./bin/otlploadgen --protocol http --endpoint http://127.0.0.1:4318 \
  --rate 100 --duration 5s --batch-size 10 --mix 34,33,33 \
  --metrics-addr "" --header "Authorization=Bearer ${TOKEN}"
```

Comportamento:

- token ausente, inválido ou expirado → **401** com `WWW-Authenticate: Bearer`;
- token de um projeto tentando escrever em outro (header
  `X-Observaki-Project-Id` ou `X-Observaki-Tenant-Id` divergente) → **403**
  `cross_project_write`, registrado no audit;
- tenant acima de `ingest_quota.events_per_second` → **429** com
  `Retry-After`; o excesso é descartado antes da descompressão/decode quando o
  `Content-Length` permite estimar os eventos;
- WAL cheio ou fila acima de `OBSERVAKI_NATS_MAX_PENDING_BYTES` → **429**
  `wal_backpressure`; NATS indisponível → **503** `sink_unavailable` e
  `/readyz` falha enquanto o WAL estiver desconectado;
- o gateway só responde depois do ack durável do JetStream; o envelope protobuf
  v1 é comprimido com zstd e o subject carrega tenant/projeto/epoch/signal/shard
  (tenant/projeto em base64url);
- `/metrics` expõe `observaki_gateway_auth_failures_total`,
  `observaki_gateway_quota_rejected_requests_total`,
  `observaki_gateway_wal_published_events_total`,
  `observaki_gateway_wal_pending_bytes`,
  `observaki_gateway_wal_publish_duration_seconds`,
  `observaki_gateway_config_age_seconds` e o p95/p99 por tenant.
- `bin/walcheck` lê o WAL sem dar ack e confere mensagens, eventos, IDs únicos,
  compressão e distribuição por shard.

O snapshot é relido a cada `OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL` (default
`30s`). Se `expires_at` passar, auth e quota falham fechadas com **503**. Em
`staging`/`production`, `OBSERVAKI_INGEST_AUTH_PUBLIC_KEY` (Ed25519 em base64)
e `OBSERVAKI_INGEST_AUDIT_FILE` são obrigatórios; a assinatura destacada fica
em `<arquivo>.sig`. O audit é JSONL append-only com hash chain, verificado na
abertura.

Os tokens, o snapshot e o audit de desenvolvimento ficam em `/tmp` e nunca no
repositório. O `make smoke` gera arquivos temporários equivalentes e valida
401, 403, audit e 429.

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

### 2. Go antigo ou `go.mod requires go >= 1.27.2`

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
lower than the targeted Go version (1.27.2)`.

**Correção:** reinstale com a toolchain do projeto:

```bash
make tools
```

O alvo fixa `GOTOOLCHAIN=go1.27.2` de propósito.

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

**Ação:** só é necessário para a stack single-node (`make compose-up`). Para
`make dev` e `make ci`, o aviso pode ser ignorado.

## Limpeza

```bash
make clean
```

Remove artefatos de build; dependências instaladas (`node_modules/`,
`~/.cargo`, módulos Go) permanecem em cache para o próximo bootstrap.

## Validação da Story 1.1 (T1.1.8)

Clone limpo em `/tmp` no commit `74bdc22`, sem `.env`, `bin/`, `node_modules`
ou builds. Todos os comandos rodaram sem intervenção manual:

| Comando | Resultado | Tempo |
|---|---|---|
| `make bootstrap` | ok; `.env` criado a partir de `.env.example` | 1,5 s |
| `make ci` | lint + test + build verdes | 13,1 s |
| `make smoke` | `Smoke: OK (healthz, api health, web build, version)` | 10,3 s |

Verificações pontuais:

- `./bin/observaki version` → `observaki 74bdc22 (commit 74bdc22, built 2026-10-31T21:04:05Z)`
- `GET /healthz` → `200` `{"status":"ok"}`
- `GET /api/v1/health` → `200` `{"status":"ok","version":"74bdc22"}`
- `web/build/index.html` → 2,6 kB, com `<title>Observaki</title>` e o markup da página.

**Ambiente:** Ubuntu 24.04.5 LTS (Linux 7.0.0-38-generic); Go 1.27.2,
Node 26.13.0, pnpm 12.16.1, Rust 1.93.0, golangci-lint 2.15.0,
Docker Compose 5.6.0.

**Ressalva:** o clone era limpo, mas os caches de toolchain (módulos Go,
store do pnpm e registry do Cargo) estavam aquecidos; um bootstrap com rede
fria não foi medido. O limite de 10 minutos pressupõe rede razoável.

## Validado por

| Data | Sistema | Ferramentas | Autor | Resultado |
|---|---|---|---|---|
| 2026-10-31 | Ubuntu 24.04.5 LTS (Linux 7.0) | Go 1.27.2, Node 26.13.0, pnpm 12.16.1, Rust 1.93.0, golangci-lint 2.15.0 | kaikeventura (autor do guia) | `make bootstrap/ci/smoke/dev/clean` ok |
| 2026-10-31 | Ubuntu 24.04.5 LTS, clone limpo em `/tmp` | mesmas versões | kaikeventura | `make bootstrap && make ci && make smoke` ok, `.env` criado automaticamente |

A revisão independente do PR e o handoff para a Story 1.2 fazem parte da
T1.1.9.
