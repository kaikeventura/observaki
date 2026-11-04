# Story 5.1 — Docker Compose single-node (handoff)

**Status:** AC1 e AC3 com evidência local em Linux; AC2 parcial — a amostra
chega e é verificada no ClickHouse/VictoriaMetrics, mas a UI ainda não tem tela
de Explore (Stories 6.1–6.4) e a validação em macOS não foi executada. O sample
app definitivo e o quickstart são a Story 5.3; o Helm é a 5.2. **Não é G1.**
**Data:** 2026-11-04 (UTC).
**Commit:** feature + docs de status desta sessão.
**Epic:** P0-E5 — Esqueleto de deploy, operação e API.
**ADs:** AD-4 (control/data plane), AD-18 (durabilidade/ack), AD-19 (snapshot de
auth), AD-22 (Postgres control plane), AD-31 (OpenAPI/API), ADR-0002, ADR-0003,
ADR-0004 (SvelteKit).

## Entregue

| Artefato | Conteúdo |
|---|---|
| `compose.yaml` (raiz) | API/UI, gateway, writer, ClickHouse 26.8, VictoriaMetrics 1.148, PostgreSQL 18.6 e NATS 2.15 com healthchecks, volumes nomeados e portas presas em `127.0.0.1` |
| `deploy/compose/Dockerfile` | Imagem multi-stage: `node:26-alpine` compila o SvelteKit, `golang:1.27-alpine` compila o binário, runtime distroless `nonroot` com `/srv/web` e spool do writer |
| `deploy/compose/config/clickhouse.xml` | `listen_host`, logger, teto de memória de 4 GiB, limites de merge/threads para single-node |
| `deploy/compose/config/nats.conf` | JetStream em `/data/js`, `max_payload` 16 MiB (o envelope do WAL chega a 4 MiB), `max_file_store` 16 GB |
| `deploy/compose/config/ingest-auth.json` | Snapshot de auth de desenvolvimento (`tenant-dev`/`project-dev`, quota 100k/s, token público) |
| `deploy/compose/scripts/clickhouse-migrate.sh` | Serviço one-shot que aplica as migrações versionadas embutidas de forma idempotente a cada `up` |
| `deploy/compose/sample.sh` | Amostra OTLP/HTTP com métricas, logs e traces (IDs de trace/span novos por rodada; spans são deduplicados por `trace_id`/`span_id`) |
| `deploy/compose/smoke.sh` | Verificação de AC1/AC2/AC3 com relatório em `deploy/compose/results/<UTC>/report.md` |
| `internal/httpserver` + `static.go` | A API Go passa a servir `OBSERVAKI_WEB_DIR` com fallback de SPA, cache imutável para `/_app/immutable/`, 404 JSON em `/api/*` e 405 em métodos não suportados |
| `internal/healthcheck` + `observaki healthcheck` | Sonda HTTP usada como healthcheck dos containers (a imagem distroless não tem shell, curl ou wget) |
| `.dockerignore`, `Makefile`, `.env.example`, `deploy/README.md`, `README.md`, `docs/dev/local-setup.md` | Build context enxuto, alvos `compose-check`/`compose-up`/`compose-down`/`compose-smoke` e documentação |

## Decisões de empacotamento

1. **Um `compose.yaml` na raiz.** O AC1 pede literalmente `docker compose up`,
   então o arquivo fica na raiz (auto-descoberta) e os assets de build ficam em
   `deploy/compose/`.
2. **Imagem construída da fonte.** `docker compose up` não exige Go/Node na
   máquina: o Dockerfile compila UI e binário. O `Dockerfile` da raiz continua
   sendo o artefato de release (binários pré-construídos).
3. **Healthcheck próprio.** A imagem final é distroless; `observaki healthcheck
   --url ...` faz a sonda de `/healthz` (API), `/readyz` (gateway e writer).
4. **Token público de desenvolvimento.** O snapshot em
   `deploy/compose/config/ingest-auth.json` é público de propósito e vale só
   para a stack local; AD-19 continua exigindo snapshot assinado em
   staging/produção. `expires_at` fica distante para a stack não quebrar.
5. **Migração automática e idempotente.** `clickhouse-migrate` roda em todo
   `up`; as migrações são aditivas (`IF NOT EXISTS`), então um volume existente
   não é reescrito e um volume novo já nasce com o schema — sem passo manual
   (AC3).
6. **PostgreSQL sem schema de control plane.** O serviço sobe com healthcheck e
   volume, mas as tabelas de control plane chegam com as stories de API/config
   plane; não há migração de Postgres nesta fase.
7. **WAL dimensionado para single-node.** 2 GiB por stream de sinal e 16 GB de
   `max_file_store` (o default de 16 GiB por stream é o perfil T1). O DLQ
   continua com os 8 GiB/7 dias do writer.

## AC1 — Subida

Run `deploy/compose/results/20261104T032219Z/report.md` (Linux 7.0.0-38,
Docker 29.9.0, Compose 5.6.0), a partir de **sem imagem e sem volumes**:

- `docker compose up -d --build --wait`: **28 s** até todos os healthchecks
  verdes (limite do AC: 10 min; o smoke falha acima de 600 s).
- `clickhouse`, `victoriametrics`, `postgres`, `nats`, `observaki-api`,
  `observaki-gateway` e `observaki-writer` ficam `running/healthy`;
  `clickhouse-migrate` termina com exit 0.
- `http://127.0.0.1:8080/` serve o `index.html` do SvelteKit e
  `/api/v1/health` responde `{"status":"ok"}`; `:4318/readyz` e `:8082/readyz`
  respondem 200.

## AC2 — Dados de exemplo (parcial)

- `deploy/compose/sample.sh` enviou 2 logs, 2 spans e 2 pontos de métrica e
  recebeu HTTP 200 nos três sinais.
- `otel_logs` foi de 0 → 2 e `otel_traces` de 0 → 2 para `project-dev`;
  `observaki_sample_requests_total{project_id="project-dev"}` aparece no
  VictoriaMetrics (a consulta aguarda o `-search.latencyOffset` de 30 s).
- **Lacuna consciente:** a landing page atual só mostra o cartão de saúde da
  API. Ver métricas, logs e traces navegáveis é o Explore das Stories 6.1–6.4;
  a Story 5.3 entrega o sample app e o quickstart com instrumentação por
  linguagem. Por isso o AC2 fica registrado como parcial, com o caminho
  de dados provado ponta a ponta.
- **macOS não validado.** Os scripts usam apenas `bash`, `curl`, `jq`, `awk` e
  `od` (portáveis), mas a execução em Docker Desktop ficou pendente.

## AC3 — Ciclo de vida

- `docker compose down` (4 s) e `docker compose up -d --wait` (18 s) mantiveram
  os volumes nomeados: as linhas de logs e traces continuaram e a série do
  VictoriaMetrics permaneceu.
- `clickhouse-migrate` reaplicou as quatro migrações sem erro e sem intervenção
  manual; o smoke falha se o container não sair com 0.
- `docker compose down -v` é o reset documentado (apaga ClickHouse, VM,
  PostgreSQL, NATS e o spool do writer).

## Comandos executados

```bash
make fmt
make lint          # golangci-lint, prettier, svelte-check, clippy, ADR check
make test          # go test + vitest + cargo test
make build         # binário + loadgen + web/build + agente
make smoke         # CI completo + smoke do binário (healthz, OTLP, WAL, quota)
docker compose config --quiet
docker compose build --no-cache      # 15,4 s (cache do BuildKit já aquecido)
./deploy/compose/smoke.sh            # 28 s até healthy, AC2 e AC3 pass
```

## Riscos e pendências

1. **Sem view de dados na UI (AC2).** Depende do Explore (6.1–6.4); até lá o
   smoke valida storage e API, não a experiência visual.
2. **macOS não validado.** Rodar `./deploy/compose/smoke.sh` em Docker Desktop
   e anexar o relatório à story.
3. **Sem hardening de produção.** ClickHouse e PostgreSQL usam credenciais de
   desenvolvimento, e o token de ingest é público; o Compose não substitui o
   Helm (5.2) nem o perfil T1.
4. **Primeiro `up` depende de rede** para baixar `node:26-alpine`,
   `golang:1.27-alpine`, imagens de CH/VM/NATS/Postgres e dependências
   (pnpm/go modules). O build medido foi 15,4 s com caches do host; um host
   frio deve ficar na casa de poucos minutos, ainda abaixo dos 10 min do AC1.
5. **PostgreSQL ainda sem schema.** O control plane (projetos, usuários,
   dashboards, alertas) chega nas próximas stories; hoje o serviço só comprova
   que a dependência sobe e persiste.
