# bench/chaos — caos na stack de ingestão

Este diretório mede a Story 4.5. Ele roda o pipeline completo
(`gateway → NATS JetStream WAL → writers → ClickHouse/VictoriaMetrics`) sob
carga OTLP contínua e injeta uma falha:

| Cenário | Falha injetada | O que prova |
|---|---|---|
| `storage` | ClickHouse parado pelo `--outage` (default 15m) | O WAL absorve o backlog; o writer retenta; após a recuperação, zero evento confirmado se perde |
| `nats` | NATS parado pelo `--nats-outage` e reiniciado | O gateway retorna 5xx enquanto desconectado, reconecta sozinho e retoma; mensagens reentregues são deduplicadas por `event_id` |
| `gateway` | Gateway morto com `SIGKILL` e reiniciado | Eventos confirmados ficam no WAL; a ingestão volta quando o novo processo abre |
| `capacity` | WAL com `MaxBytes` minúsculo e ClickHouse fora | `DiscardNew` rejeita envelopes e o gateway responde **429 + `Retry-After`** (`wal_backpressure`), não 500 |
| `all` | `storage`, depois `nats`, depois `gateway` na mesma carga | Recuperação encadeada com a mesma contabilidade |

> **Não é evidência do G1.** O AC1 pede 1M/s constante com ClickHouse fora por
> 15 minutos na frota T1. O T1 continua bloqueado por **P-07**. Os runs locais
> são proxy de comportamento e durabilidade, não de throughput.

## Uso

```bash
make build

# smoke (~1 min): storage + NATS + gateway com janelas de segundos
make bench-chaos-check

# suíte local: 15 min de ClickHouse fora + falha de NATS + falha de gateway
make bench-chaos

# cenário isolado
./bench/chaos/run.sh --scenario storage --outage 15m --post 180s
./bench/chaos/run.sh --scenario nats --nats-outage 30s
./bench/chaos/run.sh --scenario capacity --capacity-bytes 1048576
```

Opções úteis: `--rate`, `--pre`, `--outage`, `--nats-outage`,
`--gateway-outage`, `--settle`, `--post`, `--capacity-bytes`, `--no-dedupe`,
`--results-dir`. `KEEP_CONTAINERS=1` preserva os containers e `KEEP_WORK=1`
preserva spool, store do NATS e snapshots para diagnóstico.

## Contrato de verificação

O harness só fecha `status: pass` quando todas as checagens abaixo passam:

- eventos confirmados pelo cliente (`sent`) chegaram ao WAL
  (`wal_published >= sent`);
- o writer consumiu os eventos do WAL (`accepted >= sent`);
- a contabilidade fecha (`accepted == written + deduped + failed + skipped`);
- `failed == 0` no writer;
- `written >= sent`: nenhum evento confirmado se perdeu;
- por sinal: `uniqExact(event_id)` no ClickHouse `>=` confirmados (logs e
  traces) e amostras no VictoriaMetrics `>=` 99% dos confirmados de métricas;
- os logs batem exatamente com o `written`; os traces não excedem o `written`
  (o ClickHouse pode deduplicar um bloco reenviado) e a taxa de duplicatas
  fica `<= 1%` (limite documentado do at-least-once);
- retries, backpressure 429, `/readyz` 503→200 e retomada do dreno aparecem no
  `timeline.jsonl` e nos eventos;
- cenário `capacity`: `observaki_gateway_wal_backpressure_total > 0`, o cliente
  viu rejeições e o WAL voltou a aceitar depois do dreno;
- replay idêntico (mesmo seed e `start-time`) incrementa
  `writer_deduped_events_total`;
- gateway/writer saem com código 0; cenários `nats`/`gateway` mostram `/readyz`
  diferente de 200 durante a falha e de volta a 200 no fim.

O lag p95 da janela recente é reportado, mas **não reprova** cenário com
outage de storage: 15 min de backlog mantêm a janela de 4.096 amostras com
lag alto por construção. O gate de p95 < 10 s é medido no run estável da 4.4
e no T1.

O `timeline.jsonl` amostra a cada 2 s: `/readyz`, eventos publicados no WAL,
backpressure, mensagens/bytes por stream do JetStream, accepted/written/
deduped/failed/retries, fila do consumidor e lag. O `events.jsonl` registra
cada início e fim de falha com o deslocamento em segundos.

## Resultados publicados

| Run | Cenário | Status | Resultado |
|---|---|---|---|
| [`2026-11-04-local-storage-15m/`](results/2026-11-04-local-storage-15m/) | storage (15m) | pass | 5k/s por 20 min; 6M confirmados, zero perda, 625 duplicatas (0,0158%), recuperação 241 s |
| [`2026-11-04-local-all/`](results/2026-11-04-local-all/) | all | pass | storage + NATS + gateway; 1,95M confirmados, zero perda, 0 duplicatas, recuperação 120 s |
| [`2026-11-04-local-capacity/`](results/2026-11-04-local-capacity/) | capacity | pass | WAL de 1 MiB, 1.552 respostas 429, zero perda e retomada (+573.600 eventos no pós) |

## Limitações

- Single-node: sem réplica de NATS, sem vários gateways, sem vmauth ou Query
  Service. O caos de 15 min a 1M/s e o sizing do WAL/spool são T1 (P-07).
- O NATS é reiniciado com o mesmo volume em disco; não há eleição de líder nem
  quórum Raft para exercitar.
- O load generator não retenta um request que falhou; ele conta como
  `failed`. Um cliente real pode retentar e o `event_id` determinístico absorve
  a duplicata. O harness mede o limite inferior (zero retry do cliente) e
  valida o replay idêntico separadamente.
- O produto é **at-least-once**. Duplicatas só são absorvidas dentro da janela
  de dedupe em memória e do `insert_deduplication_token` do ClickHouse. Um
  índice durável cross-restart é Fase 1 (FR-4).
