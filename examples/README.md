# Sample apps (Story 5.3)

Três apps mínimas — Go, Python e Node.js — que enviam **1 trace (2 spans),
2 logs e 2 métricas** para o Observaki via OTLP/HTTP. Elas existem para o
quickstart de [`docs/quickstart.md`](../docs/quickstart.md) e para validar a
instrumentação de cada linguagem contra o gateway.

Todas usam o mesmo contrato:

| Variável | Default | O que é |
|---|---|---|
| `OBSERVAKI_OTLP_ENDPOINT` | `http://127.0.0.1:4318` | base OTLP/HTTP do gateway (sem `/v1/...`) |
| `OBSERVAKI_INGEST_TOKEN` | `observaki-local-dev-token` | bearer token do snapshot de auth |
| `OTEL_SERVICE_NAME` | `observaki-quickstart` | vira `service.name` e a coluna `service_name` |

> O token default é público de propósito e só existe na stack local
> (`deploy/compose/config/ingest-auth.json`). Fora da avaliação local, gere um
> snapshot assinado no control plane (AD-19) e nunca reutilize esse token.

O gateway espera `Authorization: Bearer <token>`; os exemplos já enviam esse
header. Cada app sai com código 0 só depois de dar flush em traces, logs e
métricas; um 401/403/429/413 do gateway aparece no stderr e no código de saída.

## Rodar

Com a stack de `docker compose up -d --build --wait` saudável:

```bash
# Go
cd examples/go && go run .

# Python
cd examples/python
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt
python app.py

# Node.js
cd examples/node
npm install
npm start
```

Para usar outro endpoint/token:

```bash
OBSERVAKI_OTLP_ENDPOINT=http://127.0.0.1:4318 \
OBSERVAKI_INGEST_TOKEN=observaki-local-dev-token \
OTEL_SERVICE_NAME=minha-app go run .
```

## Verificar

```bash
docker compose exec -T clickhouse clickhouse-client --database observaki --query \
  "SELECT service_name, count() FROM otel_logs
   WHERE service_name LIKE 'observaki-quickstart%' GROUP BY service_name"

docker compose exec -T clickhouse clickhouse-client --database observaki --query \
  "SELECT service_name, count() FROM otel_traces
   WHERE service_name LIKE 'observaki-quickstart%' GROUP BY service_name"
```

As métricas ficam no VictoriaMetrics com `project_id="project-dev"` e
`service_name="observaki-quickstart"`:

```bash
curl -s --get http://127.0.0.1:8428/api/v1/query_range \
  --data-urlencode 'query=observaki_quickstart_requests_total{project_id="project-dev"}' \
  --data-urlencode "start=$(( $(date +%s) - 600 ))" \
  --data-urlencode "end=$(date +%s)" --data-urlencode 'step=30'
```

## Checagem automática

```bash
./examples/check.sh             # sintaxe/formatação, sem rede
./examples/check.sh --build     # compila/instala as dependências
./examples/check.sh --live      # roda as três apps contra a stack e verifica os dados
```

O `--live` usa `OBSERVAKI_OTLP_ENDPOINT`/`OBSERVAKI_INGEST_TOKEN` do ambiente e
exige a stack saudável. Ele instala as dependências Python/Node em diretórios
temporários e não altera o repositório.

## Estrutura

| Caminho | Conteúdo |
|---|---|
| [`go/`](go/) | módulo Go separado (`go.opentelemetry.io/otel` v1.47) |
| [`python/`](python/) | `app.py` + `requirements.txt` (SDK 1.45.2) |
| [`node/`](node/) | `package.json` + `app.js` (SDK Node 0.301 / API 1.10) |

Cada app é deliberadamente pequena: sem framework HTTP, sem banco e sem
auto-instrumentação. O objetivo é mostrar o endpoint, o token e o flush; para
uma aplicação real, use a auto-instrumentação da sua linguagem e mantenha o
mesmo endpoint/header.
