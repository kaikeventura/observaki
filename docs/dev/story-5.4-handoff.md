# Story 5.4 — backup/restore, upgrade/rollback e auto-observabilidade

**Status:** done (com as limitações abaixo).
**Data:** 2026-11-08.
**Stories relacionadas:** 5.2 (Helm), 5.3 (quickstart), 4.2 (auth), 4.4/4.5
(writer/caos).

## Entregue

### AC1 — backup/restore da metadata

- Schema canônico do control plane em PostgreSQL (`internal/adapters/postgres/migrations/0001_control_plane.sql`)
  com organizações, usuários, projetos/geração, retenção, dashboards, canais,
  regras, memberships, tokens, silences, SLOs, error budgets e auditoria.
- Migrações embutidas e idempotentes, com advisory lock e checksum: uma migração
  alterada é recusada (`observaki migrate`).
- Seed de avaliação idempotente e restrito a `development`/`test` (org/projeto/
  usuário/dashboard/canal/regra/silence/SLO/token).
- `observaki backup --output -` exporta JSON versionado (`format_version: 1`) com
  `created_at`, `app_version`, `schema_version`, todas as tabelas e contagens.
- `observaki restore --input - --force` valida o documento, adquire o lock,
  trunca a metadata e reinsere tudo em **uma transação** com
  `json_populate_recordset`; falha deixa o estado anterior intacto. A sequência
  de `audit_events` e o heartbeat de alerting são reajustados.
- Estado derivado de alerting (`alert_states`, `alert_outbox`) fica fora do
  backup e é recalculado.
- `deploy/helm/backup.sh` faz backup/restore via `kubectl exec` (imagem
  distroless).

Evidência: `deploy/compose/results/20261108T030610Z/operator-report.md` —
14 tabelas / 15 linhas, backup e restore sub-segundo, contagens idênticas
(`1|1|1|1|1|1|1|1|1`), projeto e dashboard restaurados, token em claro ausente.

### AC2 — upgrade/rollback

- `deploy/compose/upgrade-drill.sh` constrói duas imagens da mesma árvore com
  `VERSION` diferente, sobe A, envia amostra, atualiza para B, faz rollback para
  A e verifica versão da API, metadata e linhas no ClickHouse em cada passo.
- Runbooks em `docs/runbooks/upgrade-rollback.md` (Compose e Helm) com pré-flight
  de backup, `helm upgrade --atomic`, `helm rollback`, verificação e limites.
- Chart: novos hooks `postgres-migrate` + `clickhouse-migrate` e o teste
  `helm test` da health overview.

Evidência: `deploy/compose/results/20261108T030652Z/upgrade-report.md` —
start 13 s, upgrade 13 s, **rollback 14 s** (limite 15 min) com metadata
`1|1|1|1|Development overview` e 22 logs preservados. Chart smoke:
`deploy/helm/results/20261108T030211Z/report.md` — **27/27 checks**, install
45 s, upgrade/rollback com dados `2 → 4 → 4`.

### AC3 — auto-observabilidade

- `internal/selfobs` agrega endpoints de auto-observabilidade sem violar AD-3:
  writer `/stats` + `/storage`, gateway `/metrics` + `/readyz`, NATS monitor
  `/healthz`, ping do PostgreSQL e os contadores da API.
- `GET /api/v1/health/overview` retorna ingestão (lag p95, pending, DLQ,
  confirmados, WAL), query (requests/erros/taxa), storage (bytes/parts CH,
  bytes/séries VM), componentes, alerting e `summary`.
- Valores têm `observed_at`; quando uma fonte cai, o último valor conhecido é
  preservado e o componente fica `critical` (AD-24).
- Writer expõe `GET /storage` com cache de 5 s; o probe consulta
  `system.parts` e o TSDB do VictoriaMetrics.
- API expõe `/metrics` (`observaki_api_requests_total`, contadores de query,
  uptime) e a página `/health` do SvelteKit (cards, tabela de componentes,
  estados acessíveis, polling de 10 s, erro com retry e último snapshot).
- A taxa de erro de query usa a classe `/api/v1/query*` da API até o Query
  Service (Fase 1); o placeholder retorna `501 not_implemented`.

Evidência: relatório do operator smoke — overview `healthy`, 8 componentes
verdes, lag p95 0,57 s, 18 eventos confirmados, CH 52.234 B/8 parts, VM 11.769 B/
59 séries, query 100% (4×501 do drill), heartbeat recente. UI: 5/5 testes
vitest e `svelte-check` 0/0.

### AC4 — alerta mínimo

- Avaliador interno no `serve` (`internal/alerting`): regras `signal=metrics`
  com métricas internas (`ingest_lag_p95_seconds`, `ingest_pending_messages`,
  `ingest_dlq_messages`, `query_error_rate`, `writer_connected`,
  `gateway_connected`, `component_down`), operadores `>`, `>=`, `<`, `<=` e
  `for_seconds`.
- Máquina de estados `inactive → pending → firing → resolved`, com
  `state_version` por transição.
- Outbox transacional em PostgreSQL (`alert_outbox`), chave de idempotência
  `rule|fingerprint|state_version|channel`, entrega at-least-once com
  `SKIP LOCKED`, retry exponencial 5 s→5 min e header `Idempotency-Key`.
- Silences por matcher (`rule_id`, `rule_name`, `severity`, `project_id`);
  suprimem firing e resolução, mas o estado continua mudando.
- Canais `webhook` e `slack` na Fase 0; outros kinds registram erro controlado.
- Dead-man's switch visível: heartbeat em `alerting_heartbeat`, idade na health
  overview e `critical` acima de 3 intervalos.

Evidência: relatório do operator smoke — 1 notificação entregue, contagem
estável em 1 após 8 s (dedupe), 2 linhas suprimidas no outbox (resolução +
re-firing) e nenhuma entrega adicional durante a silence. Payload e
`Idempotency-Key` no `webhooks.jsonl`.

## Decisões e desvios

- **`github.com/jackc/pgx/v5` v5.12.0** (MIT, Go puro) é o driver do PostgreSQL
  do control plane (AD-7/AD-28). É a única dependência nova; não há ORM.
- O DDL segue o addendum com duas extensões documentadas:
  - tabela `users` (diretório local; OIDC/SCIM continua o IdP na Fase 3);
  - `memberships` com `id` e `UNIQUE NULLS NOT DISTINCT (user_id, org_id,
    project_id)` para permitir membership de organização com `project_id` nulo
    (o `PRIMARY KEY` do addendum tornaria a coluna `NOT NULL`).
  - `alert_states`, `alert_outbox` e `alerting_heartbeat` implementam AD-32 e
    não fazem parte do backup lógico.
- A health overview **não** consulta ClickHouse/VM (AD-3); o writer é o dono do
  probe de storage. O storage é reportado em bytes/parts/séries, não como
  telemetria de tenant.
- A query error rate é a classe da API até o Query Service; o card fica `idle`
  sem tráfego e não afeta o status geral na Fase 0.
- A API tenta conectar ao PostgreSQL por até 30 s no start; depois roda
  degradada e sem panic (o coletor recebe um pinger nil-safe). Um restart
  recupera a conexão.
- SvelteKit 3 removeu `$lib`; o projeto passou a usar `#lib` (subpath import em
  `package.json`, `paths` no `tsconfig` e alias no Vitest).

## Limitações / pendências

- AC1 foi medido com a metadata de avaliação (15 linhas). O limite de 30 min
  cobre metadata grande; ainda não há teste com milhares de dashboards/regras.
- Backup de telemetria (ClickHouse/VM), PITR, object storage e agendamento não
  fazem parte da Fase 0 (AD-25/Fase 3).
- O alerting cobre só métricas internas; sem compilador PromQL/logs/traces, sem
  e-mail/PagerDuty/Opsgenie, sem UI de regras/silences. O dead-man é visível,
  não um alerta externo independente.
- Scheduler single-writer; o outbox evita entrega dupla em HA, mas a liderança
  do scheduler fica para a Fase 1.
- A página de saúde não tem histórico; mostra o snapshot atual + último valor
  conhecido.
- T1/P-07 continua bloqueando o baseline de 1M/s por 1 h e o AC1 original da
  4.5; nada nesta story é G1.

## Verificação executada

```text
make fmt                      # verde
make lint                     # golangci-lint 0 issues, prettier, svelte-check 0/0, clippy
make test                     # Go + vitest 5/5 + cargo 2/2
make build                    # binário, loadgens, web/build (health.html), agente
make smoke                    # CI + smoke completo: OK
make chart-check              # 13 checks + skips opcionais: pass
make chart-smoke              # kind 3 nós: 27/27, install 45s, upgrade/rollback com dados 2→4→4
make operator-smoke           # AC1+AC3+AC4: pass (relatório acima)
make upgrade-drill            # AC2: 13s/14s, metadata e 22 logs preservados
OBSERVAKI_POSTGRES_TEST_URL=... go test ./internal/adapters/postgres -run Integration
                              # backup/restore + alerting store: pass
```

## Próximo passo

Story 5.5 — API mínima versionada e OpenAPI (`epics.md`), dependente de 4.1/4.2.
Ela pode usar o schema e o seed desta story para os endpoints de projetos,
tokens e a consulta de amostra.
