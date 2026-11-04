# Story 4.5 — Teste de caos e go/no-go da stack de ingestão (handoff)

**Status:** AC1 e AC2 com evidência local (proxy de 5k/s; ClickHouse fora por
15 min, restart de NATS e `SIGKILL` do gateway); AC3 concluído com recomendação
condicional. O alvo de 1M/s por 1 h, o R3 do NATS e o sizing do WAL no T1
**não** foram executados; **não é o Gate G1**.
**Data:** 2026-11-04 (UTC).
**Commit:** `0c5244c` (feature) + docs de status.
**Epic:** P0-E4 — Gateway OTLP e ingestão.
**ADs:** AD-4, AD-5, AD-9, AD-18, AD-20, AD-21, AD-22, AD-29, AD-31, AD-33;
ADR-0002, ADR-0003.

## Entregue

| Artefato | Conteúdo |
|---|---|
| `bench/chaos/run.sh` | Harness de caos com cenários `storage`, `nats`, `gateway`, `capacity` e `all`; timeline a 1 Hz, eventos de falha, verificação de perda e relatório |
| `bench/chaos/README.md` | Uso, contrato de verificação, resultados publicados e limitações |
| `internal/adapters/nats/sink.go` | Streams de sinal em retenção **WorkQueue** (ack remove a mensagem), `DiscardNew` para backpressure, classificação de stream cheio como `ErrBackpressure` (429) e aviso de stream legado |
| `internal/adapters/nats/sink_test.go` | Teste da retenção WorkQueue, do backpressure por stream cheio e leitura com consumidor `AckExplicit` |
| `bench/cmd/walcheck/main.go` | Leitura sem ack com consumidor `AckExplicit` (WorkQueue) e `MaxAckPending`/`AckWait` para inspeção |
| `Makefile` | Alvos `bench-chaos` (suíte local) e `bench-chaos-check` (smoke) |
| `docs/dev/story-4.5-hot-path-recommendation.md` | Recomendação AC3: Go no MVP, Collector na borda, fallback Rust/Rotel com condições |
| `docs/adr/0002`, `docs/adr/0003`, `DECISIONS.md` | Evidência da 4.5, retenção WorkQueue e Q-05 respondida |

## Achados de design

1. **WAL cheio não liberava espaço.** Com `LimitsPolicy`, uma mensagem já
   confirmada continuava ocupando o orçamento de `MaxBytes`; com `DiscardNew`,
   o stream cheio rejeitava toda publicação nova mesmo depois de o ClickHouse
   voltar, até o `MaxAge` (24 h) liberar espaço. O run `capacity` provou o
   problema: o gateway ficava em 429 indefinidamente. A correção é retenção
   **WorkQueue** nos streams de sinal: o ack do consumidor durável remove a
   mensagem, o writer drena, o espaço é liberado e a ingestão retoma. O DLQ
   continua em `LimitsPolicy` (retenção de 7 dias/8 GiB para auditoria).
2. **Stream cheio é backpressure, não indisponibilidade.** O erro
   `maximum bytes exceeded`/`maximum messages exceeded` do JetStream passou a
   ser classificado como `ingest.ErrBackpressure`; o gateway responde 429 +
   `Retry-After` (`wal_backpressure`), conforme ADR-0002/ADR-0003. Antes
   retornava 503 e o produtor não sabia que o WAL estava no limite.
3. **Métricas sobrevivem ao outage do ClickHouse.** O writer de métricas usa o
   VictoriaMetrics e continua drenando o stream de métricas enquanto logs e
   traces ficam parados no WAL. O outage do ClickHouse não interrompe RED nem
   métricas.
4. **Contadores do gateway reiniciam no restart.** O harness soma o scrape de
   cada instância antes do `SIGKILL` ao scrape final; sem isso, o run `all`
   subestimava os eventos publicados no WAL.
5. **Upgrade de stream legado.** `ensureStreams` reutiliza o stream existente e
   emite um `WARN` quando a retenção não é WorkQueue. Recriar o stream é uma
   operação de upgrade da Fase 1; o handoff não faz migração automática.

## AC1 — Falha de storage

Run `bench/chaos/results/2026-11-04-local-storage-15m/`:

- 5k eventos/s em dois tenants, 60 s de pré-carga, **ClickHouse parado por
  15 min**, 240 s de pós-recuperação.
- O gateway continuou `/readyz` 200 e publicando no WAL; o writer manteve as
  mensagens não confirmadas e retentou com backoff exponencial.
- Após a recuperação: os logs batem exatamente com o `written`; os traces
  armazenados ficam 200 linhas abaixo do `written` (bloco reenviado
  deduplicado pelo ClickHouse) mas o `uniqExact(event_id)` é **igual aos traces
  confirmados** (1.980.000), provando zero perda; zero falha no writer; zero
  evento confirmado perdido.

| Métrica | Valor |
|---|---:|
| Eventos confirmados pelo cliente | 6.000.000 |
| Eventos publicados no WAL (todas as instâncias) | 6.005.000 |
| Backpressure 429 no gateway | 0 |
| Pico de mensagens/bytes no WAL | 15.043 / 1.220.441.849 B (1,14 GiB) |
| Retries do writer | 122 |
| Writer aceito / escrito / deduplicado / falho | 6.005.200 / 6.002.500 / 2.700 / 0 |
| Logs / traces no ClickHouse | 1.980.825 (únicos 1.980.825) / 1.980.625 (únicos **1.980.000**) |
| Amostras no VictoriaMetrics | 2.021.632 (64 séries) |
| Traces confirmados | 1.980.000 |
| Duplicatas no armazenamento | 625 linhas (0,0158%; limite 1%) |
| Lag p95 na janela de pós-recuperação | 736,49 s (observado; backlog na janela) |
| Recuperação (fim da falha → dreno completo) | 241 s |
| Exit codes gateway/writer | 0 / 0 |

O AC1 do T1 pede 1M/s constante; este run é proxy de durabilidade e
contabilidade, não de throughput. O sizing de 15 min a 1M/s (~22 GB comprimidos
no WAL) continua pendente do T1 (P-07).

## AC2 — Falha de gateway/NATS

Run `bench/chaos/results/2026-11-04-local-all/`:

- 3 min de ClickHouse fora, restart do NATS por 30 s e `SIGKILL` do gateway por
  15 s na mesma carga contínua.
- O gateway amostrou `/readyz` 503 durante o NATS fora e voltou a 200; após o
  restart, o novo processo reabriu e retomou. O writer reconectou e os
  consumidores duráveis sobreviveram; mensagens reentregues foram absorvidas
  pelo dedupe de `event_id`.
- Contagens exatas e zero perda para os eventos confirmados.

| Métrica | Valor |
|---|---:|
| Eventos confirmados pelo cliente | 1.948.600 |
| Eventos publicados no WAL (todas as instâncias) | 1.953.400 |
| Amostras de `/readyz` não-200 no outage de NATS | 28 |
| Amostras de `/readyz` não-200 no outage do gateway | 14 |
| Writer aceito / escrito / deduplicado / falho | 1.953.600 / 1.950.275 / 3.325 / 0 |
| Retries do writer | 32 |
| Logs / traces no ClickHouse | 643.775 / 642.950 (únicos 642.950) |
| Amostras no VictoriaMetrics | 657.626 (64 séries) |
| Duplicatas no armazenamento | 0 |
| Recuperação (última falha → dreno) | 120 s |
| Exit codes gateway/writer | 0 / 0 |

**Limite de duplicatas documentado:** o produto é at-least-once. Uma mensagem
cujo ack se perdeu é reentregue; o `event_id` determinístico e o
`insert_deduplication_token` a absorvem dentro da janela de dedupe em memória
(default 500k; 1M no harness) e da janela de 10.000 blocos do ClickHouse. O
replay idêntico do harness incrementa `writer_deduped_events_total` e não muda
as contagens do storage. Fora da janela, um replay pode duplicar; um índice
durável é Fase 1 (FR-4).

## AC3 — Recomendação

`docs/dev/story-4.5-hot-path-recommendation.md`:

- **Go** permanece no hot path do MVP (gateway e writer), condicionado a
  C-01..C-09.
- **OTel Collector** só na borda; não é o core a 1M/s.
- **Rust/Rotel** é o fallback pré-aprovado se o T1 falhar 15k eventos/s por
  vCPU ou o p95/lag; o contrato OTLP e o envelope v1 não mudam.
- As condições objetivas (C-01..C-09), o que dispara o fallback e o que não
  dispara estão no documento.

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Formatação | `make fmt` | ok |
| Lint | `make lint` | ok (0 issues) |
| Testes | `make test` | ok |
| Build | `make build` | ok |
| Smoke | `SMOKE_SKIP_CI=1 make smoke` | ok |
| Chaos smoke | `make bench-chaos-check` | pass (storage + NATS + gateway) |
| Chaos storage 15 min | `./bench/chaos/run.sh --scenario storage --outage 15m --post 240s` | pass (6M confirmados, zero perda, 625 duplicatas = 0,0158%) |
| Chaos all | `./bench/chaos/run.sh --scenario all --outage 3m --post 120s` | pass (NATS 28 e gateway 14 amostras não-200; dreno 120 s) |
| Chaos capacity | `./bench/chaos/run.sh --scenario capacity` | pass (1.552 respostas 429, zero perda, retomada +573.600 eventos) |
| Pipeline 4.4 com WorkQueue | `make bench-pipeline-check` | pass (25k eventos, lag p95 8,01 s, zero perda) |
| WAL 4.3 com `walcheck` | `make bench-gateway-wal-check` | pass (4,91x, 42 subjects, 0 duplicatas) |

## Decisões e limitações

1. **Sem T1.** O host local não sustenta 1M/s com gateway, NATS, ClickHouse,
   VictoriaMetrics e load generators juntos. 1M/s por 1 h, R3 do NATS, outage
   de 15 min a 1M/s e o sizing do WAL/spool são T1 (P-07).
2. **Single-node.** O restart de NATS usa o mesmo volume em disco; não há
   eleição de líder nem quórum. A falha de um nó em R3 é a condição C-07.
3. **Cliente sem retry.** O load generator conta request falho como `failed`;
   um cliente real pode retentar e o `event_id` absorve a duplicata. O harness
   valida o replay idêntico separadamente.
4. **Lag durante o outage.** O AC1 não exige lag durante a falha; a janela de
   lag é reportada na pós-recuperação. O gate de p95 < 10 s vale para operação
   normal e para o T1.
5. **Upgrade de stream.** Streams criados antes da 4.5 com `LimitsPolicy` são
   reutilizados com `WARN`; recriar exige drenar o WAL e é uma tarefa de
   upgrade da Fase 1.
6. **`walcheck` x writer.** WorkQueue aceita um único consumidor por stream; o
   `walcheck` não pode rodar junto com o writer em produção. Ele é uma
   ferramenta de inspeção de janela de manutenção.
7. **Sem exactly-once.** O produto é at-least-once; a recomendação e o G1 não
   prometem exactly-once.

## Próximo

1. **Story 5.1 — Docker Compose single-node** (próxima da Fase 0).
2. **T1/P-07:** 1M/s por 1 h, R3 do NATS, sizing do WAL/spool, outage de 15 min
   a 1M/s, C-06 (auth/quota < 10%) e C-08 (dreno).
3. **Fase 1:** índice durável de dedupe (FR-4), Query Service, vmauth e upgrade
   dos streams legados.
