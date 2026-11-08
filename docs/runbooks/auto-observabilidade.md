# Auto-observabilidade e alertas internos

A stack se observa pelo mesmo caminho que observa o cliente, mas com uma
fronteira explícita: a API **não** abre conexão com ClickHouse ou
VictoriaMetrics (AD-3). Ela lê os endpoints de saúde/métricas dos próprios
serviços (AD-10/AD-24).

## Página de saúde

- UI: `http://localhost:8080/health` (rota `/health` do SvelteKit).
- API: `GET /api/v1/health/overview` (JSON, `Cache-Control: no-store`).
- Atualização: a cada 10 s na UI; o coletor da API tem cache de 5 s
  (`OBSERVAKI_SELF_REFRESH_INTERVAL`).

O painel mostra:

| Seção | Fonte | Conteúdo |
|---|---|---|
| Ingestão | writer `/stats` | lag p95 por sinal, mensagens pendentes, DLQ, eventos confirmados, conexão do WAL |
| Query | contadores da própria API | requisições, erros 5xx, erros 4xx e taxa desde o start |
| Storage | writer `/storage` | bytes e parts do ClickHouse; bytes e séries do VictoriaMetrics |
| Componentes | `/readyz`, `/healthz`, ping do PostgreSQL, storage | status, latência e detalhe por componente |
| Alerting | PostgreSQL | heartbeat, regras firing/pending, fila de notificações e último erro |

Estados: `Healthy`, `Degraded`, `Critical`, `No data`, `Idle`, `Not configured`
e `Disabled`. Cor nunca é o único sinal: cada pill tem marcador e texto. Quando
uma fonte cai, o painel mantém o **último valor conhecido com o horário de
observação** e marca o componente como `critical` (AD-24).

### Query error rate na Fase 0

O Query Service chega na Fase 1. Até lá, a classe `query` conta as requisições da
API sob `/api/v1/query*`; o endpoint placeholder devolve `501 not_implemented`.
Assim a taxa é real e o painel já está ligado no lugar certo. Quando o Query
Service entrar, o mesmo contador passa a medir a consulta de verdade.

## Endpoints Prometheus

| Serviço | Endpoint | Famílias principais |
|---|---|---|
| API | `:8080/metrics` | `observaki_api_requests_total{class,code}`, `observaki_api_query_*`, `observaki_api_up` |
| Gateway | `:4318/metrics` | `observaki_gateway_wal_*`, requests/eventos/auth/quota |
| Writer | `:8082/metrics` | lag, accepted/written/deduped/failed, pending e DLQ por sinal/tenant |
| Writer | `:8082/stats` | snapshot JSON consumido pela health page |
| Writer | `:8082/storage` | uso de storage com cache de 5 s |

## Alerta interno mínimo (AC4)

O avaliador roda no processo `serve`, a cada
`OBSERVAKI_ALERTING_INTERVAL` (default 15 s; o Compose usa 5 s). Ele lê as
regras habilitadas de `alert_rules`, avalia um conjunto fechado de métricas
internas e persiste o estado em `alert_states`.

### Métricas suportadas na Fase 0

`ingest_lag_p95_seconds`, `ingest_pending_messages`,
`ingest_dlq_messages`, `query_error_rate`, `writer_connected`,
`gateway_connected` e `component_down` (com `condition.component`).

Operadores `>`, `>=`, `<`, `<=`. O `for_seconds` controla a janela de
confirmação. Regras de logs/traces/PromQL ficam para a Fase 1.

### Máquina de estados

- `inactive` → `pending` no primeiro breach; → `firing` depois de
  `for_seconds`; → `resolved` quando a condição volta ao normal.
- Cada transição incrementa `state_version` e grava uma linha no outbox
  transacional (`alert_outbox`) com a chave
  `<rule>|<fingerprint>|<state_version>|<channel>`. O notifier deduplica pela
  chave; uma avaliação seguinte não reenvia o mesmo estado.
- A entrega é at-least-once, com retry exponencial (5 s → 5 min), header
  `Idempotency-Key` e timeout de 5 s.
- `silences` casam por labels (`rule_id`, `rule_name`, `severity`,
  `project_id`) dentro da janela. Uma silence suprime firing **e** resolução,
  mas o estado continua mudando (visível na página de saúde).
- Canais suportados na Fase 0: `webhook` (payload JSON) e `slack`
  (`{"text": ...}`). E-mail/PagerDuty/Opsgenie ficam para a Fase 1 e registram
  erro de entrega.

### Dead-man's switch

O heartbeat fica em `alerting_heartbeat`. A health overview mostra a idade da
última avaliação; acima de 3 intervalos o alerting aparece `critical`
("evaluation is stale"). Um dead-man externo independente continua pendente
para produção (AD-24).

### Criar regra e silence (Fase 0, via SQL)

```sql
INSERT INTO alert_rules
  (id, project_id, name, signal, query, condition, for_seconds, severity, enabled)
VALUES
  ('00000000-0000-4000-8000-0000000000c1',
   '00000000-0000-4000-8000-000000000002',
   'Ingestion lag high', 'metrics', 'ingest_lag_p95_seconds',
   '{"op":">","threshold":10}'::jsonb, 60, 'warning', true);

INSERT INTO alert_rule_channels (rule_id, channel_id)
VALUES ('00000000-0000-4000-8000-0000000000c1',
        '00000000-0000-4000-8000-000000000005');

INSERT INTO silences (id, project_id, matchers, starts_at, ends_at, reason)
VALUES ('00000000-0000-4000-8000-0000000000c2',
        '00000000-0000-4000-8000-000000000002',
        '{"rule_name":"Ingestion lag high"}'::jsonb,
        now(), now() + interval '1 hour', 'manutenção');
```

A UI/editor de regras e a API de metadata chegam na Story 5.5/Fase 1.

## Drill

```bash
make operator-smoke   # Compose: AC1 + AC3 + AC4 com webhook local
make upgrade-drill    # Compose: AC2
```

O `operator-smoke` inicia `bin/webhooksink` na porta 19090, cria uma regra
determinística, verifica a entrega, a dedupe por 8 s, a silence na resolução e
no re-fire, e limpa a regra no final. O relatório fica em
`deploy/compose/results/<UTC>/operator-report.md` e o JSONL bruto em
`webhooks.jsonl`.

## Limitações conhecidas

- Sem compilador de regras para PromQL/logs/traces e sem canais de e-mail/PagerDuty.
- A query error rate é da API até o Query Service existir.
- Sem paginação/ordenação na página de saúde (o volume é pequeno na Fase 0).
- O dead-man é visível, não um alerta externo independente.
- Single-node: o alerting roda em um processo; em HA o `SKIP LOCKED` do outbox
  evita entrega dupla, mas o scheduler deve permanecer single-writer até a
  Fase 1.
