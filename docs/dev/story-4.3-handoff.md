# Story 4.3 — Batching, compressão, sharding e WAL NATS (handoff)

**Status:** AC2 e AC3 concluídos com evidência local; AC1 validado como proxy
local de 100k/s (p99 28,9 ms). O alvo de 1M/s por 1 h no cluster T1 **não** foi
executado; **não é o Gate G1**.
**Data:** 2026-11-03.
**Epic:** P0-E4 — Gateway OTLP e ingestão.
**ADs:** AD-1, AD-2, AD-3, AD-4, AD-9, AD-10, AD-18, AD-19, AD-20, AD-22,
AD-29, AD-31, AD-33; ADR-0003.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `api/proto/observaki/ingest/v1/envelope.proto` | Envelope protobuf v1 `observaki.ingest.v1.Envelope` com schema_version, identidade, config/epoch, sinal, shard, requests e `event_ids` alinhados |
| `internal/ingestpb/` | Código Go gerado com `protoc-gen-go v1.36.12` (protoc 33.0) |
| `internal/adapters/nats/` | Sink JetStream: batching por subject, zstd, publish síncrono com ack, retry, backpressure, streams por sinal, métricas, health e decode/validate |
| `internal/ingest/eventid.go` | `event_id` determinístico por evento (SHA-256 → UUID v5-shaped), após redaction |
| `internal/ingest/{sink,http,grpc,errors,metrics}.go` | `Batch.RequestID`, `ErrBackpressure`/`ErrSinkClosed`, 429 `wal_backpressure`, 503, `/readyz` e métricas do WAL |
| `internal/config/config.go` + `.env.example` | Configuração `OBSERVAKI_NATS_*` validada |
| `cmd/observaki/main.go` | `gateway` abre o WAL NATS e falha fechado se não conectar; remove o `DiscardSink` do caminho de produção |
| `bench/cmd/walcheck/` + `bin/walcheck` | Leitor sem ack que valida envelope, contagem, IDs únicos, compressão, subjects e shards |
| `bench/gateway/wal/` | Harness local com NATS Docker, dois tenants, dois load generators e relatório |
| `scripts/smoke.sh` | NATS temporário, métricas do WAL, `walcheck` e SIGTERM |
| `docs/dev/story-4.3-handoff.md` | Este documento |

## Contrato do WAL

### Envelope v1

- `schema_version = 1`, `message_id` (UUIDv7, `Nats-Msg-Id`), `tenant_id`,
  `project_id`, `config_version`, `config_generation`, `authz_epoch`, `signal`,
  `shard`, `created_at_unix_nano` e `requests[]`.
- Cada `Request` carrega `request_id`, o OTLP protobuf já redigido,
  `received_at_unix_nano` e um `event_id` por evento folha, na mesma ordem dos
  contadores (spans, log records ou data points). `ValidateEnvelope` rejeita
  contagem divergente.
- Campos novos usam números novos; o leitor ignora campos desconhecidos
  (teste com field 111) e rejeita `schema_version` desconhecido (AD-22).
- Compressão zstd (`Observaki-Encoding: zstd`, nível fastest, janela 1 MiB).
  Um envelope maior que o `max_payload` do NATS é dividido por request; um único
  request maior que o limite falha com 503 e não é confirmado.

### Subject e sharding

```text
observaki.ingest.v1.<tenant_b64url>.<project_b64url>.e<authz_epoch>.<signal>.<shard>
```

- Tenant/projeto são base64url porque o snapshot aceita `.`, `:` e `@`, que
  quebram tokens de subject NATS.
- O shard é `FNV-1a(tenant|project|signal|request_id) % shards`; cada request
  vai para um shard e o tenant se espalha sem estado compartilhado. Duplicatas
  de retry podem cair em outro shard e são absorvidas pelo `event_id`.
- Streams por sinal (`OBSERVAKI_TRACES`, `OBSERVAKI_LOGS`,
  `OBSERVAKI_METRICS`) com `LimitsPolicy`, `DiscardNew`, `MaxAge` (24h default)
  e `MaxBytes` (32 GiB por stream default; T1 deve configurar o sizing).
  `DiscardNew` garante que stream cheio vira backpressure, não perda silenciosa
  de dados já confirmados.
- O stream é criado se não existir; se existir, o subject esperado é validado e
  a configuração não é mutada por um gateway em runtime.

### Ack, dedupe e backpressure

- `Sink.Consume` só retorna depois de `js.PublishMsg` receber o ack persistido.
  O cliente OTLP nunca recebe 200 antes disso (AD-4).
- Fila limitada por `OBSERVAKI_NATS_MAX_PENDING_BYTES`; fila cheia → 429
  `wal_backpressure` com `Retry-After: 1`. NATS desconectado/erro de publish →
  503 `sink_unavailable`; `/readyz` responde 503 enquanto o WAL está fora.
- Retry de publish: 3 tentativas com backoff 50/100 ms e o mesmo `message_id`
  (`Nats-Msg-Id`) para a janela de duplicatas do JetStream.
- `event_id` é determinístico e calculado **depois** da redaction: reenvio do
  mesmo request produz os mesmos IDs; o writer deduplica por esse campo. O
  `Nats-Msg-Id` cobre apenas retry do mesmo publish, não o retry do SDK.
- Spans usam o caminho rápido `tenant|project|traces|trace_id|span_id`; spans
  malformados, logs e métricas usam um hash canônico de resource/scope/campos,
  com atributos ordenados.
- Redaction foi otimizada de regex com backtracking para um scanner linear de
  Luhn, preservando o contrato de mascarar cartões: o pipeline de um batch de
  500 eventos caiu de ~8,9 ms para ~1,8 ms no benchmark local.

## AC1 — Throughput

O AC1 pede 1M/s por 1 h na frota T1 com p99 de processamento < 50 ms. O T1
continua bloqueado por **P-07**; o run local é proxy, não G1.

**Run publicado local:** `bench/gateway/wal/results/2026-11-03-local-100k/`.

| Métrica | Valor |
|---|---:|
| Taxa ofertada (2 tenants) | 100.000 eventos/s |
| Duração | 10 s |
| Eventos aceitos | 1.000.000 |
| Eventos no WAL | 1.000.000 |
| `event_id` únicos | 1.000.000 |
| Duplicatas | 0 |
| Compressão do envelope | **5,46x** |
| Subjects usados | 192 |
| Shards usados | 32 |
| Maior subject | 0,95% das mensagens |
| p99 de processamento do gateway | **28,9 ms** (alvo < 50 ms) |
| p99 cliente | 54,5 ms |
| Falhas / rejeições | 0 / 0 |

**Run de 1M/s com payload mínimo:** 5.000.000 eventos em ~6,8 s (≈740k/s no
host compartilhado), 32 shards, p99 de processamento 25,4 ms, 5M IDs únicos,
compressão 3,13x. O payload de referência não sustenta 1M/s localmente com NATS,
gateway e load generators na mesma máquina; o run pontual de 5 s ficou em
≈243k/s e p99 69,8 ms, então **não** é usado como evidência de AC1.

## AC2 — Durabilidade

- `TestSinkSurvivesNATSServerRestart` grava 10 envelopes com storage de arquivo,
  para o servidor NATS, sobe um novo servidor no mesmo store dir e relê os 50
  eventos confirmados. Nenhum evento confirmado é perdido.
- `TestSinkIsIdempotentByEventID` envia o mesmo batch duas vezes; os `event_id`
  são idênticos nas duas tentativas, comprovando a chave que o writer usa para
  deduplicar.
- O `walcheck` do run publicado relê todas as mensagens sem ack e compara a
  contagem com o contador de eventos aceitos pelo gateway.
- O SIGTERM drena HTTP/gRPC, fecha o batcher, publica os grupos pendentes e só
  então fecha a conexão NATS; o smoke verifica o `/readyz`, o WAL e o exit 0.

## AC3 — Sharding

- Dois tenants no harness; cada um tem subjects próprios e usa 32 shards por
  sinal. O run publicado teve 192 subjects (2 tenants × 3 sinais × 32 shards) e
  o maior subject concentrou 0,95% das mensagens.
- `TestShardForSpreadsRequests` e os testes de subject cobrem o layout e a
  distribuição.
- Um tenant não consegue monopolizar um subject porque o shard é escolhido por
  request e os subjects de outro tenant são tokens distintos.

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Formatação | `make fmt` | ok |
| Lint | `make lint` | ok (0 issues) |
| Testes | `make test` | ok |
| Race | `go test -race ./internal/ingest ./internal/adapters/nats` | ok |
| Build | `make build` | ok (inclui `walcheck`) |
| Smoke | `SMOKE_SKIP_CI=1 make smoke` | ok; OTLP HTTP/gRPC, WAL, 401/403/audit, 429, SIGTERM |
| Harness smoke | `./bench/gateway/wal/run.sh --smoke` | pass; 10k eventos, 8 shards, p99 ~20 ms |
| Run publicado | `./bench/gateway/wal/run.sh --results-dir bench/gateway/wal/results/2026-11-03-local-100k` | pass; 1M eventos, 5,46x, p99 28,9 ms |
| 1M/s mínimo | `./bench/gateway/wal/run.sh --small-payload --rate 1000000 --duration 5s` | 5M eventos, 740k/s, p99 25,4 ms |
| Benchmark do pipeline | `go test ./internal/ingest -bench 'BenchmarkSynthesizeEventIDs\|BenchmarkMaskCardNumbers'` | ok |

## Decisões e limitações

1. **WAL default.** NATS JetStream é o WAL do gateway em todos os ambientes; o
   `DiscardSink` fica apenas como fixture de teste. O single-node do Compose
   (Story 5.1) pode usar NATS local; a decisão de buffer local vs NATS continua
   na Q-03 do ADR-0003.
2. **Retenção.** `MaxAge` 24h e `MaxBytes` 32 GiB por stream são defaults
   conservadores para desenvolvimento. O sizing de 15 min de outage a 1M/s
   (~120 GB) precisa ser configurado no T1; `DiscardNew` garante backpressure
   quando o teto é atingido.
3. **Ack e duplicatas.** O produto é at-least-once. O `Nats-Msg-Id` cobre retry
   de publish; a dedupe de retry do SDK é por `event_id` no writer (4.4). Não há
   promessa de exactly-once.
4. **Sem writer nesta história.** O envelope v1 e o `DecodeMessage`/
   `ValidateEnvelope` são o contrato para a 4.4; nenhum consumo/ack no NATS
   acontece ainda, então o stream retém tudo até os limites.
5. **Payload máximo.** O NATS precisa de `max_payload` compatível com o maior
   request comprimido; o harness usa 16 MiB no config do servidor. O Open
   valida o `max_payload` e divide grupos por request; um request individual
   acima do limite falha sem ack.
6. **Config do stream.** O gateway não faz `UpdateStream` em runtime; mudanças
   de retenção/replicas são operação explícita. Validar subjects evita que um
   stream antigo receba o WAL errado.
7. **Autenticação NATS.** URL com credenciais, TLS (`tls://`) e credenciais
   `.creds` são suportadas; mTLS/accounts por componente entram no deploy do
   T1/5.1 (AD-31).
8. **Local, não G1.** O host compartilhado limita o run de referência a 100k/s;
   a prova de 1M/s/1h, o outage de 15 min do ClickHouse e o caos são
   4.4/4.5 + T1 (P-07).

## Próximo

1. **Story 4.4 — writers ClickHouse/VM**: consumir os três streams, deduplicar
   por `event_id`, escrever em batch, ack só após flush e expor lag.
2. **Story 4.5 — caos + go/no-go**: NATS/ClickHouse fora, backpressure, zero
   perda confirmada e recomendação Go vs Rust/Collector.
3. **T1/P-07**: sizing de disco/replicas do WAL e o run de 1M/s por 1 h.
