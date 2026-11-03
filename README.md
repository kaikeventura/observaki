# Observaki

**Observabilidade open source, self-hosted e OTel-native:** métricas, logs, traces, dashboards, alertas e correlação em uma única plataforma — com foco em performance, baixo footprint e operação por times pequenos.

> **Status:** Fase 0 (fundação e benchmarks) em andamento. O estado corrente fica em [`STATUS.md`](STATUS.md).

## Pré-requisitos

| Ferramenta | Versão | Observação |
|---|---|---|
| Go | 1.27 | o `go.mod` fixa a versão e o toolchain é resolvido automaticamente |
| Node.js | 26 LTS | frontend SvelteKit |
| pnpm | 10 | gerenciador de pacotes do `web/` |
| Rust | 1.85 (stable) | agente, com `rustfmt` e `clippy` |
| Docker Compose | v2 | opcional nesta fase (Story 5.1) |
| make, curl | recente | orquestração e smoke test |

## Quickstart (5 comandos)

Guia completo (portas, configuração e troubleshooting): [`docs/dev/local-setup.md`](docs/dev/local-setup.md).

```bash
make bootstrap   # valida a toolchain, instala dependências e cria .env
make test        # go test + vitest + cargo test
make build       # binário Go, load generator, build do SvelteKit + agente
make dev         # API Go (:8080) + SvelteKit (:5173)
make smoke       # em outro terminal: CI completo + smoke do binário
```

Abra <http://localhost:5173> — o cartão **API status** fica “Operational”
quando a API responde. `make lint` exige o golangci-lint, instalável com
`make tools`.

Para validar o gerador de carga OTLP da Fase 0:

```bash
make build
./bin/otlploadgen --check-payload
```

O workload de referência e as limitações de taxa estão em
[`bench/README.md`](bench/README.md).

## Gateway OTLP (Fase 0)

O comando `gateway` sobe os receptores OTLP/gRPC (`:4317`) e OTLP/HTTP
(`:4318`) para traces, logs e métricas. Ele exige um snapshot versionado com
tokens por tenant/projeto e quotas, aplica os limites de requisição (10 MiB e
10k eventos por padrão), autentica/limita o tráfego antes do sink e redige
campos de cartão.

```bash
make build
# crie o snapshot com o hash SHA-256 do token (veja docs/dev/local-setup.md)
# e suba um NATS com JetStream (docker run ... nats:2.15.2-alpine -js)
OBSERVAKI_INGEST_AUTH_FILE=/tmp/observaki-dev/ingest-auth.json \
OBSERVAKI_INGEST_AUDIT_FILE=/tmp/observaki-dev/audit.jsonl \
OBSERVAKI_NATS_URL=nats://127.0.0.1:4222 \
  ./bin/observaki gateway
./bin/otlploadgen --protocol http --endpoint http://127.0.0.1:4318 \
  --rate 100 --duration 5s --batch-size 10 --mix 34,33,33 --metrics-addr "" \
  --header "Authorization=Bearer ${TOKEN}"
```

Token inválido ou expirado retorna 401; tentativa de escrever em outro
projeto retorna 403 e vai para o audit; excesso de quota retorna 429 com
`Retry-After`. O gateway publica no WAL NATS JetStream (envelope protobuf v1
comprimido com zstd, shards por tenant/projeto/epoch/signal) e só confirma
depois do ack durável; fila cheia retorna 429 e NATS indisponível retorna 503.
Duplicatas são deduplicadas por `event_id`; o writer ClickHouse/VM é a Story
4.4. Detalhes em [`docs/dev/story-4.3-handoff.md`](docs/dev/story-4.3-handoff.md).

O spike local de armazenamento de logs (Story 3.1) deixa um container e ~78 GiB
em `/tmp/opencode/observaki-ch31`. A limpeza do container e dos dados está em
[`bench/clickhouse/README.md`](bench/clickhouse/README.md#limpeza-importante).

## Estrutura do repositório

```text
cmd/observaki/     # binário Go
internal/          # domínio, ports, adapters, ingest, query, alerting
web/               # SvelteKit 3 SPA
agent/             # agente Rust (Fase 2)
bench/             # harness de benchmark
deploy/            # Compose, Helm, jobs
docs/              # ADRs, guias
```

Módulo Go: `github.com/observaki/observaki` (premissa da Story 1.1 — ajustar
caso o org ou o nome definitivo do repositório sejam outros).

## Como o desenvolvimento funciona

Cada sessão do OpenCode é independente. O agente lê `AGENTS.md`, `project-context.md` e `STATUS.md`, continua do `next_action` e atualiza o estado ao terminar.

Prompt de retomada: [`PROMPT.md`](PROMPT.md). Basta mandar:

```text
Leia PROMPT.md e execute.
```

## Planejamento

- [`PROGRAM-PLAN.md`](_bmad-output/planning-artifacts/PROGRAM-PLAN.md) — fases, gates, staffing, releases.
- [`PRD`](_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md) + [`addendum`](_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md) — produto, requisitos, stack, benchmarks.
- [`ARCHITECTURE-SPINE.md`](_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md) — 35 invariantes (AD-1..AD-35).
- [`DESIGN.md`](_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/DESIGN.md) + [`EXPERIENCE.md`](_bmad-output/planning-artifacts/ux-designs/ux-observaki-2026-10-31/EXPERIENCE.md) — UX/UI.
- [`epics.md`](_bmad-output/planning-artifacts/epics.md) — Fase 0 detalhada.
- [`DECISIONS.md`](_bmad-output/planning-artifacts/DECISIONS.md) — decisões e pendências.

## Gates

- **G1:** 1M eventos/s, p95 < 1s em 1B linhas, compressão ≥ 5x, 3 design partners.
- **G2:** UJ-1 ponta a ponta, 3 pilotos, soak de 7 dias.
- **G3:** 1M/s em produção assistida, 10 parceiros.
- **G4:** 10M/s, 30 dias de soak, 3 tenants, GA 1.0.

Nenhuma fase começa sem o gate anterior. Detalhes em [`PROGRAM-PLAN.md`](_bmad-output/planning-artifacts/PROGRAM-PLAN.md).

## Configuração

O binário lê a configuração de variáveis de ambiente; os defaults seguros
estão documentados em [`.env.example`](.env.example):

| Variável | Default | Descrição |
|---|---|---|
| `OBSERVAKI_HTTP_ADDR` | `:8080` | endereço de escuta da API |
| `OBSERVAKI_LOG_LEVEL` | `info` | `debug`, `info`, `warn` ou `error` |
| `OBSERVAKI_ENV` | `development` | `development`, `test`, `staging` ou `production` |
| `OBSERVAKI_DATABASE_URL` | — | PostgreSQL do control plane (opcional na Fase 0) |
| `OBSERVAKI_WEB_DIR` | `web/build` | build estático servido pelo binário |
| `OBSERVAKI_OTLP_GRPC_ADDR` | `:4317` | endereço do receptor OTLP/gRPC |
| `OBSERVAKI_OTLP_HTTP_ADDR` | `:4318` | endereço do receptor OTLP/HTTP |
| `OBSERVAKI_INGEST_MAX_REQUEST_BYTES` | `10485760` | limite do corpo após descompressão |
| `OBSERVAKI_INGEST_MAX_EVENTS` | `10000` | limite de spans/logs/data points por request |
| `OBSERVAKI_INGEST_AUTH_FILE` | — | snapshot de tokens/quotas (obrigatório no `gateway`) |
| `OBSERVAKI_INGEST_AUTH_PUBLIC_KEY` | — | chave Ed25519 do snapshot (obrigatória em staging/produção) |
| `OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL` | `30s` | intervalo de releitura do snapshot |
| `OBSERVAKI_INGEST_AUDIT_FILE` | — | audit JSONL append-only (obrigatório em staging/produção) |
| `OBSERVAKI_REDACT_ATTRIBUTE_KEYS` | `credit_card,card_number,card.number,cc_number,pan,cvv,cvc` | chaves mascaradas pela redação |
| `OBSERVAKI_REDACT_MASK` | `[REDACTED]` | valor que substitui o campo sensível |
| `OBSERVAKI_NATS_URL` | `nats://127.0.0.1:4222` | WAL NATS JetStream (obrigatório no `gateway`) |
| `OBSERVAKI_NATS_CREDENTIALS_FILE` | — | credenciais NATS (`.creds`) |
| `OBSERVAKI_NATS_SUBJECT_PREFIX` | `observaki.ingest.v1` | prefixo dos subjects do WAL |
| `OBSERVAKI_NATS_STREAM_PREFIX` | `OBSERVAKI` | prefixo dos streams por sinal |
| `OBSERVAKI_NATS_SHARDS` | `32` | shards por tenant/projeto/epoch/signal |
| `OBSERVAKI_NATS_STREAM_REPLICAS` | `1` | réplicas do stream (3 no cluster) |
| `OBSERVAKI_NATS_STREAM_MAX_AGE` | `24h` | janela de retenção do WAL |
| `OBSERVAKI_NATS_STREAM_MAX_BYTES` | `34359738368` | teto de bytes por stream |
| `OBSERVAKI_NATS_STREAM_STORAGE` | `file` | `file` ou `memory` |
| `OBSERVAKI_NATS_BATCH_MAX_EVENTS` | `100000` | eventos por envelope antes do flush |
| `OBSERVAKI_NATS_BATCH_MAX_BYTES` | `4194304` | bytes por envelope antes do flush |
| `OBSERVAKI_NATS_BATCH_LINGER` | `5ms` | espera máxima para fechar um envelope |
| `OBSERVAKI_NATS_PUBLISH_TIMEOUT` | `5s` | timeout de cada publish no JetStream |
| `OBSERVAKI_NATS_MAX_PENDING_BYTES` | `268435456` | fila máxima antes de 429 |
| `OBSERVAKI_NATS_PUBLISH_WORKERS` | `8` | publishers concorrentes |

`make bootstrap` cria `.env` a partir de `.env.example` e `make dev` carrega o
arquivo antes de subir a API e o SvelteKit. Valores inválidos falham com o nome
da variável e o valor recusado.

## Contribuindo

Contribuições são bem-vindas. Leia [`CONTRIBUTING.md`](CONTRIBUTING.md) para
setup, padrões, RFC e o **DCO** (todo commit precisa de `git commit -s`). Ao
participar, você concorda com o [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

Vulnerabilidades devem ser reportadas em privado, conforme o
[`SECURITY.md`](SECURITY.md) — nunca em issue pública.

## Licença

O núcleo é [Apache 2.0](LICENSE). O modelo open-core e a lista de candidatos a
recursos enterprise — ainda **pendente de decisão de negócio** (P-03) — estão em
[`docs/governance/open-core.md`](docs/governance/open-core.md). A fronteira
exata também é acompanhada em [`DECISIONS.md`](_bmad-output/planning-artifacts/DECISIONS.md).
