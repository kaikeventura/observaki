# Observaki single-node (Docker Compose)

Sobe a stack completa — API/UI, gateway OTLP, writer, ClickHouse,
VictoriaMetrics, PostgreSQL e NATS JetStream — com um comando, a partir do
código do repositório. É o ambiente de avaliação da Story 5.1, não um deployment
endurecido e **não** é evidência de G1.

## Subir

```bash
docker compose up -d --build --wait   # ou: make compose-up
```

O primeiro `up` compila a imagem (SvelteKit + binário Go) e leva alguns minutos;
os seguintes reaproveitam a imagem. `--wait` só retorna quando todos os
healthchecks estão verdes. O smoke mede esse tempo e falha se passar de 10 min.

Abra <http://localhost:8080>. A API responde em `/healthz` e
`/api/v1/health`; o gateway em `:4318/readyz`; o writer em `:8082/readyz`.

## Serviços e portas

| Serviço | Imagem | Porta (127.0.0.1) | Volume | Healthcheck |
|---|---|---|---|---|
| `observaki-api` | `observaki:local` (build local) | 8080 | — | `observaki healthcheck /healthz` |
| `observaki-gateway` | `observaki:local` | 4317 (gRPC), 4318 (HTTP) | `ingest-auth.json` (ro) | `/readyz` |
| `observaki-writer` | `observaki:local` | 8082 | `writer-spool` | `/readyz` |
| `clickhouse` | `clickhouse/clickhouse-server:26.8` | 8123 | `clickhouse-data` | `SELECT 1` |
| `clickhouse-migrate` | `clickhouse/clickhouse-server:26.8` | — | migrações (ro) | sai 0 |
| `victoriametrics` | `victoriametrics/victoria-metrics:v1.148.0` | 8428 | `victoriametrics-data` | `/health` |
| `postgres` | `postgres:18.6-alpine` | 5432 | `postgres-data` | `pg_isready` |
| `nats` | `nats:2.15.2-alpine` | 4222, 8222 (monitor) | `nats-data` | `/healthz` |

Todas as portas publicadas ficam presas em `127.0.0.1`: a stack não fica exposta
na rede.

## Dados de exemplo

```bash
./deploy/compose/sample.sh          # uma rodada: 2 logs, 2 spans, 2 métricas
./deploy/compose/sample.sh --loop 5 # a cada 5 s
```

O script fala OTLP/HTTP JSON com o token de desenvolvimento, então exercita o
caminho gateway → WAL NATS → writer → ClickHouse/VictoriaMetrics. Para o
caminho de 15 minutos com instrumentação em Go/Python/Node.js e troubleshooting,
veja [`docs/quickstart.md`](../../docs/quickstart.md) e as apps em
[`examples/`](../../examples/README.md).

Para conferir:

```bash
docker compose exec -T clickhouse clickhouse-client --database observaki \
  --query "SELECT service_name, count() FROM otel_logs GROUP BY service_name"
curl -s --get http://127.0.0.1:8428/api/v1/query_range \
  --data-urlencode 'query=observaki_sample_requests_total' \
  --data-urlencode "start=$(( $(date +%s) - 600 ))" \
  --data-urlencode "end=$(date +%s)" --data-urlencode 'step=30'
```

O VictoriaMetrics só devolve amostras mais antigas que o `-search.latencyOffset`
(30 s por padrão) em consultas instantâneas; por isso o smoke usa `query_range`
com polling em vez de `query`.

O cartão da landing page cobre a saúde da API; a tela de Explore (métricas,
logs e traces navegáveis) chega nas Stories 6.1–6.4. O sample app com
instruções por linguagem está em [`examples/`](../../examples/README.md) e o
quickstart em [`docs/quickstart.md`](../../docs/quickstart.md).

## Ciclo de vida e dados

- `docker compose down` mantém os volumes nomeados; `up` de novo restaura o
  estado sem passo manual.
- `docker compose down -v` apaga os volumes (ClickHouse, VM, PostgreSQL, NATS,
  spool do writer) e volta ao estado inicial.
- `clickhouse-migrate` reaplica as migrações versionadas embutidas
  (`internal/adapters/clickhouse/migrations`) em todo `up`. Elas são aditivas e
  idempotentes, então um volume existente não é reescrito e um volume novo já
  nasce com o schema.

## Verificação automática

```bash
make compose-smoke          # ou: ./deploy/compose/smoke.sh
make quickstart-check       # ou: ./deploy/compose/quickstart-check.sh --fresh
make examples-check         # sintaxe/formatação das apps de exemplo
./examples/check.sh --live  # roda as três apps contra a stack e valida os dados
```

O smoke cobre os três ACs da Story 5.1 e grava o relatório em
`deploy/compose/results/<UTC>/report.md`:

1. **AC1** — `up` até todos os healthchecks verdes, tempo abaixo de 10 min,
   API/UI e `/readyz` respondendo.
2. **AC2** — amostra enviada e contabilizada em `otel_logs`, `otel_traces` e
   `observaki_sample_requests_total` no VictoriaMetrics.
3. **AC3** — `down` + `up`, contagens preservadas e migração automática.

Requer `docker`, `docker compose` v2+, `curl` e `jq`.

## Token de desenvolvimento

O gateway exige um snapshot de auth versionado (AD-19). O arquivo
[`config/ingest-auth.json`](config/ingest-auth.json) é público de propósito:

- tenant `tenant-dev`, projeto `project-dev`;
- token `observaki-local-dev-token` (só o SHA-256 fica no arquivo);
- quota de 100k eventos/s, `expires_at` distante para a stack não quebrar;
- assinatura Ed25519 dispensada porque o ambiente é `development`.

Fora de uma avaliação local, gere o snapshot pelo control plane e assine em
staging/produção — nunca reutilize este arquivo.

## Limites desta stack

- ClickHouse com 4 GiB de `max_server_memory_usage`, 8 threads e merges
  limitados; WAL de 2 GiB por sinal e 16 GB de `max_file_store` no JetStream.
- Retenção do VictoriaMetrics de 1 mês; retenção de logs/traces vem da coluna
  `retention_days` (default 15 na `writer`).
- PostgreSQL sobe com credenciais locais (`observaki`/`observaki-local-dev`) e
  ainda não tem schema de control plane — ele é criado nas stories de API/control
  plane.
- O build da imagem usa o `web/pnpm-lock.yaml`; rode `pnpm --dir web install`
  depois de mudar dependências para o lockfile ficar em dia.

## Troubleshooting

| Sintoma | Causa provável | Correção |
|---|---|---|
| `port is already allocated` | outra stack/benchmark usando 8080/4318/8123/8428 | pare o outro processo ou ajuste as portas no `compose.yaml` |
| `observaki-writer` reiniciando | streams do WAL ausentes (gateway não subiu) | `docker compose logs observaki-gateway`; o writer só inicia depois do gateway saudável |
| UI responde 404 `ui_not_installed` | imagem antiga sem `OBSERVAKI_WEB_DIR` | `docker compose build observaki-api` (ou `up --build`) |
| Nenhum dado no ClickHouse | amostra não enviada ou token errado | rode `deploy/compose/sample.sh` e veja o status HTTP |
| `clickhouse-migrate` falhando | migração aplicada com usuário sem privilégio | remova o volume (`down -v`) e suba de novo; em produção as migrações rodam com admin |
| Primeiro `up` muito lento | build de Node + Go | a imagem fica em cache; `--no-build` no smoke seguinte |

## Escopo e pendências

- Validado em Linux; a validação em macOS depende de Docker Desktop e ainda não
  foi executada (ver `docs/dev/story-5.1-handoff.md`).
- Helm chart, backup/restore e auto-observabilidade são as Stories 5.2 e 5.4.
- O quickstart de 15 minutos e as apps de exemplo (Go, Python e Node.js) são a
  Story 5.3: [`docs/quickstart.md`](../../docs/quickstart.md) e
  [`examples/`](../../examples/README.md).
