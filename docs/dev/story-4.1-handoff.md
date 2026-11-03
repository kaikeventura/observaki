# Story 4.1 — Skeleton do gateway OTLP (handoff)

**Status:** AC1–AC4 validados em execução local; **não é o Gate G1**.
**Data:** 2026-11-02.
**Escopo:** receptores OTLP/gRPC e OTLP/HTTP para traces, logs e métricas,
limites de request configuráveis, `partial_success`, shutdown gracioso e
redação antes do sink.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `internal/ingest/gateway.go` | `Gateway` stateless, listeners gRPC/HTTP, `/healthz`, `/readyz`, `/metrics`, shutdown gracioso com drain |
| `internal/ingest/grpc.go` | Serviços OTLP `TraceService`, `LogsService` e `MetricsService`, gzip e limites |
| `internal/ingest/http.go` | `POST /v1/traces`, `/v1/logs` e `/v1/metrics`; protobuf e JSON OTLP; gzip; erros `google.rpc.Status` |
| `internal/ingest/sink.go` | Porta `Sink`, `Batch`, `Result` e `DiscardSink` (placeholder até a Story 4.3) |
| `internal/ingest/limits.go` | Contagem de spans, log records e data points por sinal |
| `internal/ingest/redact.go` | Redação por chave e por padrão de cartão (Luhn), em logs, traces e métricas, antes do sink |
| `internal/ingest/errors.go` | Envelope `{code,message,details,request_id}` com `ErrorInfo` |
| `internal/ingest/metrics.go` | Contadores Prometheus de requests, eventos, rejeições, erros e in-flight |
| `internal/id/uuid.go` | UUIDv7 para `request_id` (AD-15) |
| `cmd/observaki/main.go` | Comando `observaki gateway` |
| `internal/config/config.go` | Endereços OTLP, limites e regras de redação via ambiente |
| `scripts/smoke.sh` | Smoke OTLP HTTP/gRPC, `/metrics`, SIGTERM e exit 0 |
| `README.md`, `.env.example` | Uso, configuração e limitações do gateway |

## AC1 — Recepção

- gRPC em `:4317` (`OBSERVAKI_OTLP_GRPC_ADDR`) e HTTP em `:4318`
  (`OBSERVAKI_OTLP_HTTP_ADDR`), com os três sinais.
- OTLP/HTTP aceita `application/x-protobuf` e `application/json`; no JSON,
  `traceId`/`spanId`/`parentSpanId` são convertidos de hex para o mapping
  Protobuf (AD-16). Campos desconhecidos são ignorados, como o OTLP exige.
- Gzip aceito em gRPC e HTTP. Sem zstd nesta story.
- `partial_success` é montado a partir do `Result` do sink
  (`Accepted`/`Rejected`/`Reason`) para spans, log records e data points.

**Evidência:** `TestGRPCAcceptsAllSignals`, `TestHTTPAcceptsAllSignals`,
`TestHTTPJSONWithHexIDs`, `TestHTTPGzip`, `TestGRPCPartialSuccess`,
`TestHTTPPartialSuccess` e `TestGRPCGzip`. No smoke, o `otlploadgen` enviou o
mix 34/33/33 por HTTP/gzip e gRPC/gzip; o `/metrics` registrou 7 requests e 20
eventos por sinal em cada protocolo.

## AC2 — Limites

- Default 10 MiB (`OBSERVAKI_INGEST_MAX_REQUEST_BYTES`) e 10.000 eventos
  (`OBSERVAKI_INGEST_MAX_EVENTS`), ambos configuráveis e validados na carga da
  configuração.
- HTTP: `413 Content Too Large` com `google.rpc.Status` e mensagem
  `request body exceeds the N-byte limit` ou `request has N events, limit is M`.
  O limite de tamanho vale depois da descompressão (proteção contra gzip bomb).
- gRPC: `RESOURCE_EXHAUSTED` para mensagem acima do limite (`MaxRecvMsgSize`) e
  para contagem de eventos acima do limite, com mensagem explicando o valor.
- `request_id` vai no header `X-Request-Id` e no `ErrorInfo`/JSON.

**Evidência:** `TestHTTPRequestSizeLimit`, `TestHTTPEventLimit`,
`TestHTTPGzipBombIsLimitedAfterDecompression`, `TestGRPCRequestSizeLimit`,
`TestGRPCEventLimit` e os testes de configuração em `internal/config`.

## AC3 — Operação

- SIGTERM/SIGINT cancelam o `Run`: `/readyz` passa a 503, o HTTP usa
  `Shutdown` e o gRPC usa `GracefulStop`, drenando requests em voo; o sink é
  fechado se implementar `io.Closer`.
- O timeout de drain é `15s` (`DefaultShutdownTimeout`), configurável no
  `ingest.Config`.
- `/healthz` é liveness; `/readyz` é readiness; `/metrics` expõe
  `observaki_gateway_requests_total`, `observaki_gateway_events_total`,
  `observaki_gateway_rejected_events_total`, `observaki_gateway_errors_total`,
  `observaki_gateway_in_flight_requests` e `observaki_gateway_up`.

**Evidência:** `TestGatewayGracefulShutdownDrainsHTTP`,
`TestGatewayGracefulShutdownDrainsGRPC`, `TestGatewayShutdownIsIdempotent` e o
smoke, que envia OTLP, mata o processo com `SIGTERM` e exige exit 0.

## AC4 — Redaction

- A regra padrão mascara as chaves `credit_card`, `card_number`, `card.number`,
  `cc_number`, `pan`, `cvv` e `cvc` (case-insensitive; `-` e `.` equivalem a
  `_`). A lista e a máscara são configuráveis
  (`OBSERVAKI_REDACT_ATTRIBUTE_KEYS`, `OBSERVAKI_REDACT_MASK`).
- Números de cartão válidos por Luhn também são mascarados no corpo do log e
  em valores string, inclusive aninhados em arrays/KV lists. Números comuns
  (ordem, timestamp) não são tocados.
- A redação percorre resource, scope, log record, span, span events, links,
  data points e exemplars.
- `trace_id`/`span_id` são preservados, mantendo a correlação log↔trace.
- A redação roda antes de `Sink.Consume`; o `DiscardSink` é o stand-in do WAL
  até a Story 4.3.

**Evidência:** `TestRedactLogsMasksCreditCard`,
`TestRedactConfigurableMaskAndKeys`, `TestRedactTracesAndMetrics`,
`TestRedactDoesNotTouchNonCardNumbers`, `TestLuhnValid` e
`TestGRPCRedactionPreservesTraceCorrelation`.

## Configuração

| Variável | Default | Descrição |
|---|---|---|
| `OBSERVAKI_OTLP_GRPC_ADDR` | `:4317` | receptor OTLP/gRPC |
| `OBSERVAKI_OTLP_HTTP_ADDR` | `:4318` | receptor OTLP/HTTP |
| `OBSERVAKI_INGEST_MAX_REQUEST_BYTES` | `10485760` | limite do corpo após descompressão |
| `OBSERVAKI_INGEST_MAX_EVENTS` | `10000` | limite de eventos por request |
| `OBSERVAKI_REDACT_ATTRIBUTE_KEYS` | lista de campos de cartão | chaves mascaradas |
| `OBSERVAKI_REDACT_MASK` | `[REDACTED]` | valor substituto |

Execução local:

```bash
make build
./bin/observaki gateway
./bin/otlploadgen --protocol http --endpoint http://127.0.0.1:4318 \
  --rate 100 --duration 5s --mix 34,33,33 --metrics-addr ""
```

## Validação executada

| Comando | Resultado |
|---|---|
| `go test ./...` | pass |
| `go test -race ./internal/ingest ./internal/config ./internal/id` | pass (2 execuções) |
| `golangci-lint run ./...` | 0 issues |
| `make ci` | pass (bootstrap, lint, test, build) |
| `SMOKE_SKIP_CI=1 make smoke` | pass, incluindo gateway OTLP HTTP/gRPC, `/metrics` e SIGTERM |
| `otlploadgen` manual (HTTP/gzip e gRPC/gzip) | 7 requests e 20 eventos por sinal/protocolo; exit 0 |

## Limitações e pendências

- **Sem auth/tenant (Story 4.2):** `Batch.TenantID`/`ProjectID` ficam vazios;
  nenhum token é validado. Não usar fora de desenvolvimento.
- **Sem WAL/durabilidade (Story 4.3):** o sink é `DiscardSink`; o ACK não
  significa persistência. O warning aparece no startup. Em
  `OBSERVAKI_ENV=production`, o comando `gateway` recusa iniciar com o
  `DiscardSink` (fail-closed) até a Story 4.3. O WAL NATS, dedupe por
  `event_id`, DLQ e backpressure 429 entram na 4.3.
- **Sem writers, RED ou cardinalidade (Stories 4.4 e AD-33):** o gateway não
  calcula span metrics/RED e não grava em ClickHouse/VictoriaMetrics.
- **Sem self-tracing OTel (AD-10):** `/healthz`, `/readyz` e `/metrics` existem,
  mas o gateway não exporta traces de si mesmo; o SDK OTel é uma dependência
  nova a decidir na story de auto-observabilidade.
- **Sem zstd, profiles, HTTP/2 h2c, quotas/rate limiting:** fora do escopo desta
  story.
- **`partial_success` real:** a plumbing está pronta e testada com sinks que
  rejeitam; com o `DiscardSink` nada é rejeitado até 4.3/4.4.
- **Limite de tamanho no gRPC:** é aplicado pelo `grpc-go` antes do handler, com
  `RESOURCE_EXHAUSTED`; por isso não carrega o `ErrorInfo`/`request_id` do
  Observaki. O limite por contagem de eventos carrega.
- **`serve` não sobe o gateway:** use `observaki gateway`; o Compose da Story 5.1
  vai orquestrar os dois processos.

## Mapa de ADs

| AD | Como a story trata |
|---|---|
| AD-1 | Gateway é data plane; não abre conexão com PostgreSQL |
| AD-2/AD-3 | `Batch` reserva `tenant_id`/`project_id`; resolução do token fica na 4.2 |
| AD-4 | OTLP gRPC/HTTP é o caminho; ack após o sink (discard é placeholder) |
| AD-10 | `/healthz`, `/readyz` e `/metrics`; self-traces pendentes |
| AD-15 | `request_id` UUIDv7; logs JSON; erros com `code/message/details/request_id` |
| AD-16 | Contratos OTLP cobertos por testes de gRPC, HTTP, gzip, JSON e partial success |
| AD-21 | RED não é calculado nesta story |
| AD-29 | `ingested_at` e skew ficam para a ingestão/storage |
