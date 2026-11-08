# Quickstart — primeiros 15 minutos

Sobe o Observaki single-node, envia a primeira telemetria (métricas, logs e
traces) e confirma que os dados chegaram ao armazenamento. É o caminho mais
curto para avaliar o produto sem ler a documentação inteira.

**Tempo esperado:** 5–10 minutos em uma máquina com rede razoável; o limite do
check automatizado é 15 minutos. O primeiro `up` compila a imagem e é a parte
mais lenta; os próximos reaproveitam o cache.

> Esta stack é para avaliação local. Ela usa credenciais de desenvolvimento
> públicas e **não** é um deployment endurecido. Para Kubernetes, veja o
> [Helm chart](../deploy/helm/observaki/README.md).

## O que você vai ver

| Sinal | Onde fica | Como conferir |
|---|---|---|
| Logs | ClickHouse `observaki.otel_logs` | query SQL no passo 4 |
| Traces | ClickHouse `observaki.otel_traces` | query SQL no passo 4 |
| Métricas | VictoriaMetrics | query MetricsQL no passo 4 |

A tela de Explore (métricas/logs/traces navegáveis) chega nas Stories 6.1–6.4.
Até lá, a landing page mostra a saúde da API e os dados são verificados no
armazenamento, como abaixo.

## Pré-requisitos

| Ferramenta | Versão | Observação |
|---|---|---|
| Docker Engine | 24+ | com o daemon rodando |
| Docker Compose | v2+ | `docker compose version` |
| git | recente | para clonar o repositório |
| curl | recente | usado pela amostra sem dependências |
| RAM | ≥ 8 GB | a stack sobe ClickHouse, VM, PostgreSQL e NATS |
| Disco | ≥ 10 GB livres | volumes de dados |

Portas usadas em `127.0.0.1`: `8080` (API/UI), `4317`/`4318` (OTLP), `8082`
(writer), `8123` (ClickHouse), `8428` (VictoriaMetrics), `5432` (PostgreSQL),
`4222`/`8222` (NATS). Se alguma estiver ocupada, veja o
[troubleshooting](#troubleshooting).

## 1. Suba a stack

```bash
git clone https://github.com/kaikeventura/observaki.git
cd observaki
docker compose up -d --build --wait
```

`--wait` só retorna quando todos os healthchecks estão verdes. A saída termina
com os containers `healthy`; o `clickhouse-migrate` sai com código 0 depois de
aplicar as migrações idempotentes.

Se preferir o atalho do Makefile:

```bash
make compose-up
```

## 2. Confirme a saúde

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:4318/readyz
curl -s http://127.0.0.1:8082/readyz
```

Abra <http://127.0.0.1:8080>: o cartão **API status** deve mostrar
“Operational”. O gateway (`:4318`) e o writer (`:8082`) respondem `200` em
`/readyz` quando o WAL e o storage estão prontos.

## 3. Envie a primeira telemetria

### Opção A — amostra sem dependências (recomendada para o primeiro teste)

```bash
./deploy/compose/sample.sh
```

O script fala OTLP/HTTP JSON com o token de desenvolvimento e envia **2 logs,
2 spans e 2 métricas**. Saída esperada:

```text
logs: accepted (HTTP 200)
traces: accepted (HTTP 200)
metrics: accepted (HTTP 200)
```

O caminho exercitado é gateway → WAL NATS JetStream → writer →
ClickHouse/VictoriaMetrics.

### Opção B — app de exemplo na sua linguagem

As apps em [`examples/`](../examples/README.md) usam os SDKs oficiais e enviam
o mesmo conjunto de sinais. Escolha uma:

```bash
# Go (o módulo de exemplo tem go.mod próprio)
cd examples/go && go run . && cd ../..

# Python
cd examples/python
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt
python app.py
cd ../..

# Node.js
cd examples/node
npm install
npm start
cd ../..
```

Todas leem `OBSERVAKI_OTLP_ENDPOINT` (default `http://127.0.0.1:4318`) e
`OBSERVAKI_INGEST_TOKEN` (default `observaki-local-dev-token`), e usam
`OTEL_SERVICE_NAME` para o `service.name`. O `examples/check.sh --live` roda as
três e valida os dados.

## 4. Confirme os dados

Logs e traces:

```bash
docker compose exec -T clickhouse clickhouse-client --database observaki --query \
  "SELECT service_name, count() FROM otel_logs
   WHERE service_name LIKE 'observaki-%' GROUP BY service_name ORDER BY service_name"

docker compose exec -T clickhouse clickhouse-client --database observaki --query \
  "SELECT service_name, count() FROM otel_traces
   WHERE service_name LIKE 'observaki-%' GROUP BY service_name ORDER BY service_name"
```

Métricas (o VictoriaMetrics só devolve amostras mais antigas que o
`-search.latencyOffset` de 30 s em consultas instantâneas; por isso usamos
`query_range` com uma janela de 10 minutos):

```bash
curl -s --get http://127.0.0.1:8428/api/v1/query_range \
  --data-urlencode 'query=observaki_sample_requests_total{project_id="project-dev"}' \
  --data-urlencode "start=$(( $(date +%s) - 600 ))" \
  --data-urlencode "end=$(date +%s)" \
  --data-urlencode 'step=30'
```

O resultado é um JSON com `"result": [...]` não vazio. O projeto é
`project-dev`, o tenant é `tenant-dev`.

## 5. Encerre ou limpe

```bash
docker compose down       # para a stack e mantém os volumes
docker compose down -v    # apaga os volumes (ClickHouse, VM, PostgreSQL, NATS, spool)
```

`down` + `up` preserva os dados e reaplica as migrações sem passo manual.

---

## Instrumentação da sua aplicação

O gateway aceita **OTLP/HTTP** (`http://127.0.0.1:4318`) e **OTLP/gRPC**
(`127.0.0.1:4317`) para traces, logs e métricas. Toda requisição precisa do
token do projeto:

| Item | Valor local |
|---|---|
| Endpoint HTTP | `http://127.0.0.1:4318` |
| Endpoint gRPC | `http://127.0.0.1:4317` |
| Header | `Authorization: Bearer observaki-local-dev-token` |
| Caminhos HTTP | `/v1/traces`, `/v1/logs`, `/v1/metrics` |
| Tenant / projeto | `tenant-dev` / `project-dev` |

O token local é público de propósito e só vale para esta stack. Em
staging/produção, gere o snapshot assinado no control plane (AD-19) e configure
o SDK com o token real. Nunca comite tokens.

### Go

```go
import (
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

exporter, err := otlptracehttp.New(ctx,
	otlptracehttp.WithEndpoint("127.0.0.1:4318"),
	otlptracehttp.WithInsecure(), // use HTTPS em produção
	otlptracehttp.WithHeaders(map[string]string{
		"Authorization": "Bearer observaki-local-dev-token",
	}),
)
if err != nil {
	return err
}
provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
defer provider.Shutdown(ctx)
```

O exemplo completo (traces + logs + métricas, com flush e código de saída) está
em [`examples/go/main.go`](../examples/go/main.go).

### Python

```python
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter

exporter = OTLPSpanExporter(
    endpoint="http://127.0.0.1:4318/v1/traces",
    headers={"Authorization": "Bearer observaki-local-dev-token"},
)
```

O exemplo completo está em [`examples/python/app.py`](../examples/python/app.py);
as dependências fixadas ficam em `requirements.txt`.

### Node.js

```js
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";

const exporter = new OTLPTraceExporter({
	url: "http://127.0.0.1:4318/v1/traces",
	headers: { Authorization: "Bearer observaki-local-dev-token" },
});
```

O exemplo completo está em [`examples/node/app.js`](../examples/node/app.js).
Logs usam `@opentelemetry/exporter-logs-otlp-http` e métricas
`@opentelemetry/exporter-metrics-otlp-http` com o mesmo endpoint/header.

### Auto-instrumentação e variáveis padrão

SDKs e agentes que seguem a spec OpenTelemetry leem estas variáveis:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
export OTEL_EXPORTER_OTLP_HEADERS='Authorization=Bearer%20observaki-local-dev-token'
export OTEL_SERVICE_NAME=minha-app
```

O valor do header é URL-encoded (`%20` no lugar do espaço), como a spec exige.
Para gRPC, use `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4317` e o mesmo
header (o SDK envia como metadata `authorization`). Se o seu SDK separar por
sinal, use `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`,
`OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` e `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` com a
URL completa (`/v1/traces`, etc.).

O gateway limita cada requisição a **10 MiB** e **10k eventos** por padrão
(`OBSERVAKI_INGEST_MAX_REQUEST_BYTES`, `OBSERVAKI_INGEST_MAX_EVENTS`). Ajuste o
batch do SDK se a sua aplicação gera lotes maiores.

---

## Troubleshooting

| Sintoma | Causa provável | Correção |
|---|---|---|
| `connection refused` em `:4318` ou `:4317` | stack parada, porta trocada ou endpoint sem `http://` | `docker compose ps`; use `http://127.0.0.1:4318`; confira se outra stack não está usando a porta |
| `HTTP 401` com `invalid_token`/`missing_token` | token ausente, errado ou de outro ambiente | envie `Authorization: Bearer observaki-local-dev-token`; confira o snapshot em `deploy/compose/config/ingest-auth.json` |
| `HTTP 403` com `cross_project_write` | header `x-observaki-project-id` aponta para outro projeto | remova o header; o projeto vem do token |
| `HTTP 429` com `quota_exceeded` | quota do tenant estourada | respeite o `Retry-After`; reduza a taxa; a quota local é 100k eventos/s |
| `HTTP 429` com `wal_backpressure` | WAL NATS cheio (writer lento ou storage fora) | verifique `:8082/readyz` e os logs do writer; aguarde o dreno ou aumente `OBSERVAKI_NATS_STREAM_MAX_BYTES` |
| `HTTP 413` com `request_too_large` | lote acima de 10 MiB ou 10k eventos | reduza `batch_size`/`max_export_batch_size` no SDK |
| `HTTP 415` | `Content-Type` errado | use um SDK OTLP; em curl, `application/json` ou `application/x-protobuf` |
| `HTTP 503` com `sink_unavailable` | NATS indisponível | `docker compose ps nats`; `docker compose logs observaki-gateway` |
| `HTTP 503` com `config_stale` | snapshot de auth expirado | regenere o snapshot (staging/produção) ou reinicie a stack local |
| Nenhum dado no ClickHouse | writer ainda subindo, token errado, lote rejeitado ou dedupe | veja `:8082/readyz`, `docker compose logs observaki-writer` e a saída do `sample.sh`; spans repetidos com o mesmo `trace_id`/`span_id` são deduplicados |
| Métrica não aparece no VictoriaMetrics | janela de 30 s do `-search.latencyOffset` | use `query_range` com janela de 10 min e aguarde ~30 s |
| UI sem tela de Explore | Stories 6.1–6.4 ainda não entregues | verifique os dados pelas queries do passo 4 |
| `port is already allocated` | outra stack/benchmark usando a porta | pare o outro processo ou ajuste as portas no `compose.yaml` |
| `observaki-writer` reiniciando | gateway não criou os streams do WAL | `docker compose logs observaki-gateway`; o writer só inicia depois do gateway saudável |
| `clickhouse-migrate` falhando | volume/migração inconsistente | `docker compose down -v` e suba de novo (apaga dados locais) |
| UI responde 404 `ui_not_installed` | imagem antiga sem o build do SvelteKit | `docker compose build observaki-api` e `up` de novo |
| Primeiro `up` muito lento | build de Node + Go | a imagem fica em cache; use `--no-build` no próximo `up` |

### “Enviei mas não vejo nada” — checklist

1. A amostra respondeu `accepted (HTTP 200)`? Se não, o problema é auth,
   endpoint ou limite — veja a linha correspondente acima.
2. `curl -s http://127.0.0.1:8082/readyz` responde `200`? Se não, o writer está
   fora ou o WAL está sem os streams; veja `docker compose logs observaki-writer`.
3. O `service.name` que você consultou é o mesmo que o SDK enviou? Use
   `SELECT DISTINCT service_name FROM otel_logs` para conferir.
4. Para traces, cada span precisa de `trace_id`/`span_id` novos; reenviar o
   mesmo span é deduplicado por `event_id` (AD-18), não é perda.
5. Para métricas, aguarde 30 s e use `query_range` (o `query` instantâneo
   ignora amostras recentes por causa do `-search.latencyOffset`).
6. Se o gateway devolveu 429, o evento **não** foi confirmado; o SDK deve
   retentar com backoff. O WAL só confirma depois do publish durável.

### Ver os logs de cada componente

```bash
docker compose logs -f observaki-gateway
docker compose logs -f observaki-writer
docker compose logs -f clickhouse
```

## Próximos passos

- Instrumentação completa por linguagem: [`examples/README.md`](../examples/README.md).
- Stack single-node, portas e limites: [`deploy/compose/README.md`](../deploy/compose/README.md).
- Kubernetes/Helm: [`deploy/helm/observaki/README.md`](../deploy/helm/observaki/README.md).
- Desenvolvimento local da API/UI: [`docs/dev/local-setup.md`](dev/local-setup.md).
