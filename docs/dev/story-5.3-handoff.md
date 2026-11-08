# Story 5.3 — Quickstart, sample app e instrumentação (handoff)

**Status:** AC1, AC2 e AC3 concluídos com evidência local em Linux. **Não é G1.**
A validação em macOS continua pendente junto com o AC2 da Story 5.1.
**Data:** 2026-11-08 (UTC).
**Epic:** P0-E5 — Esqueleto de deploy, operação e API.
**ADs:** AD-6 (mesmas imagens/config), AD-18 (dedupe por `event_id`), AD-19
(snapshot de auth assinado), AD-21 (RED no gateway/VM, não no ClickHouse).

## Entregue

| Artefato | Conteúdo |
|---|---|
| `docs/quickstart.md` | Quickstart de 15 min: subir a stack, amostra, verificação em ClickHouse/VM, instrumentação Go/Python/Node.js, variáveis OTel padrão e troubleshooting |
| `examples/go/` | Módulo Go separado com OTel SDK v1.47; trace (2 spans), 2 logs e 2 métricas via OTLP/HTTP |
| `examples/python/` | `app.py` + `requirements.txt` com SDK 1.45.2; mesmos sinais |
| `examples/node/` | `app.js` + `package.json`/lockfile com SDK Node 0.301 / API 1.10; mesmos sinais |
| `examples/README.md` | Contrato comum (`OBSERVAKI_OTLP_ENDPOINT`, `OBSERVAKI_INGEST_TOKEN`, `OTEL_SERVICE_NAME`), como rodar e verificar |
| `examples/check.sh` | `--static` (default), `--build` e `--live` (roda as três apps e valida 2 logs, 2 spans e a métrica de cada uma) |
| `deploy/compose/quickstart-check.sh` | Reproduz o quickstart, mede o tempo até dados verificados, impõe o limite de 900 s e grava relatório |
| `Makefile` | Alvos `quickstart-check` (`QUICKSTART_ARGS=--fresh`) e `examples-check` |
| `.github/workflows/ci.yml` | `make examples-check` no job `quality` |
| `README.md`, `deploy/compose/README.md` | Links para o quickstart, apps de exemplo e novos alvos |

## Decisões de design

1. **Amostra sem dependências continua sendo o primeiro passo.** O
   `deploy/compose/sample.sh` usa só `curl`, então uma máquina com Docker
   consegue validar o caminho completo antes de instalar SDK algum. As apps em
   `examples/` são o passo seguinte, com os SDKs oficiais.
2. **Apps mínimas, sem framework.** Cada exemplo tem um único arquivo de
   entrada e emite o mesmo conjunto de sinais. O objetivo é mostrar endpoint,
   token, `service.name` e flush — não esconder a instrumentação atrás de
   auto-instrumentação.
3. **Go em módulo separado.** `examples/go/go.mod` evita arrastar o SDK OTel
   para o `go.mod` do produto; `make examples-check` cobre o módulo de exemplo e
   o CI roda a checagem estática.
4. **Node com `LoggerProvider` explícito.** No SDK Node 0.301 testado,
   `NodeSDK({ logRecordProcessors })` não propagou o provider global de logs
   para o logger da aplicação (`enabled()` retornava `false`). O exemplo cria o
   `LoggerProvider`, registra em `logs.setGlobalLoggerProvider` e faz flush no
   shutdown; traces e métricas seguem pelo `NodeSDK`.
5. **Sem token novo.** Os exemplos usam o token público de desenvolvimento do
   Compose (`observaki-local-dev-token`, só o SHA-256 fica no snapshot). A doc
   deixa explícito que ele não vale fora da avaliação local (AD-19).

## AC1 — Quickstart em menos de 15 minutos

Run `make quickstart-check QUICKSTART_ARGS=--fresh` em Linux 7.0.0-42 x86_64
(Docker 29.9.0, Compose 5.6.0), removendo volumes e a imagem local antes:

- `docker compose up -d --wait --build`: **29 s** até todos os healthchecks
  verdes (o cache de camadas base do Docker foi reaproveitado; a imagem da
  aplicação foi reconstruída).
- `deploy/compose/sample.sh`: logs, traces e métricas `accepted (HTTP 200)`.
- Dados verificados em `otel_logs`, `otel_traces` e
  `observaki_sample_requests_total` no VictoriaMetrics.
- **Total: 60 s** do `up` até os dados verificados (limite 900 s). Relatório em
  `deploy/compose/results/20261108T020129Z/quickstart-report.md`.

## AC2 — Instrumentação Go, Python e Node.js

Run `./examples/check.sh --live` contra a stack recém-criada:

| App | Saída | `otel_logs` | `otel_traces` | Métrica no VM |
|---|---|---|---|---|
| Go (OTel v1.47) | `sent 2 spans, 2 log records and 2 metric points` | 2 | 2 | presente |
| Python (SDK 1.45.2) | idem | 2 | 2 | presente |
| Node.js (SDK 0.301) | idem | 2 | 2 | presente |

O script usa nomes de serviço únicos por execução
(`observaki-quickstart-<lang>-<UTC>`), então a verificação não é contaminada por
runs anteriores. Cada app envia `Authorization: Bearer <token>` e usa o mesmo
endpoint base; os snippets mínimos de cada linguagem estão em
`docs/quickstart.md` e os arquivos completos em `examples/`.

## AC3 — Troubleshooting

`docs/quickstart.md` tem a tabela sintoma/causa/correção para endpoint errado,
token inválido/ausente, `cross_project_write`, quota/WAL, 413, 415, 503,
“nenhum dado”, latência do VictoriaMetrics, UI sem Explore, portas e migração.
Os dois casos mais comuns foram exercitados contra a stack:

- token inválido: `HTTP 401` com `reason=invalid_token` e `request_id`;
- endpoint errado (`:4399`): `curl: (7) Failed to connect` (documentado como
  stack parada/porta trocada).

## Comandos executados

```bash
./examples/check.sh                  # static: pass
./examples/check.sh --live           # Go, Python e Node: pass
make quickstart-check QUICKSTART_ARGS=--fresh  # 60 s, pass
make fmt                             # pass
make examples-check                  # pass
make smoke                           # make ci + smoke: pass
```

## Limitações e pendências

- **macOS** não foi validado nesta sessão; o AC2 da Story 5.1 continua parcial
  até a validação em macOS e a tela de Explore (Stories 6.1–6.4).
- Os exemplos usam OTLP/HTTP. O gRPC está documentado no quickstart, mas não
  tem app de exemplo dedicada.
- As apps são de avaliação: sem auto-instrumentação, sem TLS local e com o token
  público de desenvolvimento. Produção exige snapshot assinado (AD-19).
- `make quickstart-check` sem `QUICKSTART_ARGS=--fresh` reaproveita volumes e
  imagem; o modo `--fresh` apaga os dados locais de avaliação.
- A Story 5.3 não altera o caminho crítico de ingestão nem os benchmarks; o
  baseline T1 continua bloqueado por P-07.

## Próximo passo

**Story 5.4** — Backup/restore, upgrade/rollback e auto-observabilidade
(depende de 5.2 e 5.3).
