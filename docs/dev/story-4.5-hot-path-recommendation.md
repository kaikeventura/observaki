# Story 4.5 — Recomendação do hot path: Go, Rust/Rotel ou OTel Collector

- **Story:** 4.5 — teste de caos e go/no-go da stack de ingestão.
- **Status:** recomendação **condicional**. Go continua sendo a escolha para o
  hot path do MVP; o gate T1 ainda não rodou.
- **Data:** 2026-11-04.
- **Vinculado a:** ADR-0002 (gateway em Go), ADR-0003 (WAL NATS JetStream),
  AD-4 (ack após durabilidade), AD-21 (RED no VictoriaMetrics), AD-22
  (contrato do envelope).
- **Evidência:** handoffs das Stories 4.1–4.4, `BENCHMARKS.md` e os runs de
  caos em `bench/chaos/results/`.

## Decisão

**Manter Go 1.27 no gateway e no writer do MVP.** Usar o OTel Collector na
borda do cliente como camada de compatibilidade, nunca como caminho central de
ingestão. Manter Rust/Rotel como fallback pré-aprovado, capaz de substituir o
gateway sem mudar OTLP, o envelope do WAL ou o contrato do writer.

A decisão é condicional porque todos os números de throughput medidos até aqui
são locais e dividem um único host com NATS, ClickHouse, VictoriaMetrics e os
load generators. O gate T1 (P-07) é a evidência que confirma ou reverte a
escolha.

## Por que Go

| Critério | Evidência |
|---|---|
| Throughput | Gateway + WAL aceitaram 100k eventos/s sustentados localmente (Story 4.3); 1M/s ofertado com payload mínimo atingiu ~740k/s em host compartilhado de 22 vCPUs, p99 25,4 ms |
| Latência | p99 de processamento do gateway 28,9 ms a 100k/s; lag p95 do writer 0,80 s a 20k/s (Story 4.4) |
| Durabilidade | Ack só depois do JetStream (4.3); writer só confirma no NATS depois do ClickHouse/VM (4.4) |
| Caos | ClickHouse fora 15 min, restart de NATS e `SIGKILL` do gateway na Story 4.5: zero perda para eventos confirmados, contagens exatas, 429 quando o WAL encheu |
| Operação | Um toolchain Go para gateway, writer, API e Query Service; binário distroless; `/healthz`, `/readyz`, `/metrics` |
| Contrato | OTLP gRPC/HTTP; nenhum SDK proprietário; envelope v1 inalterado se o gateway for trocado |
| Time | Time pequeno; uma linguagem reduz custo de review e on-call |

## Por que não OTel Collector como core

O addendum registra cerca de 10k eventos/s por instância do Collector numa
configuração típica. A 1M/s a frota seria de ~100 instâncias antes do headroom,
com uma superfície de configuração (processors, exporters, memory limiter)
difícil de raciocinar para tenancy, quotas e o contrato de ack do WAL. Ele
continua sendo a ferramenta certa na borda, onde o cliente já o opera, e como
escape de compatibilidade.

## Por que não Rust/Rotel agora

Rust tem a melhor eficiência por core e nenhuma pausa de GC, mas o ecossistema
OTel é menos maduro, o time carregaria um segundo hot path e os números atuais
em Go ainda não mostram uma falha que justifique a troca. O ADR-0002 já define
o fallback: se Go não sustentar 15–20k eventos/s por vCPU com o headroom
necessário, o gateway migra para Rust/Rotel atrás dos mesmos contratos de OTLP
e envelope.

## Condições

A recomendação só vale se todas estas condições forem verdadeiras no gate T1:

| # | Condição | Por quê | Status atual |
|---|---|---|---|
| C-01 | Gateway Go sustenta ≥ 15k eventos/s por vCPU no payload de referência | Regra de capacidade do ADR-0002; 4–6 gateways para 1M/s | **Pendente T1**; runs locais são proxy em host compartilhado |
| C-02 | 1M/s por 1 hora com lag p95 ponta a ponta < 10 s | G1, NFR-1/NFR-5 | **Pendente T1** (P-07) |
| C-03 | Zero perda para eventos confirmados com ClickHouse fora 15 min | AC1, ADR-0003 | Run local de 15 min **pass**; replicação e sizing T1 pendentes |
| C-04 | Retenção do WAL > outage + reparo, e WAL cheio libera espaço depois que os writers drenam | ADR-0003 item 5 | Corrigido na 4.5: streams de WAL agora usam WorkQueue (ack); run de capacidade recupera. Sizing T1 pendente |
| C-05 | Pausas de GC não quebram o p99 a 1M/s (`GOGC`/`GOMEMLIMIT` ajustados) | Risco de Go no ADR-0002 | p99 local 28,9 ms a 100k/s; T1 pendente |
| C-06 | Auth + quota adicionam < 10% ao p95 de request a 1M/s | Story 4.2 | +15,13% local e inconclusivo (host compartilhado); T1 pendente |
| C-07 | NATS R3 na topologia T1 e falha de nó não perdem mensagem confirmada | AC2, AD-23/ADR-0003 | Restart single-node **pass** local; R3 pendente |
| C-08 | Taxa de dreno do writer depois de 15 min de outage > backlog de 1M/s | Recuperação dentro do orçamento de lag | Dreno local pass a 5k/s; T1 pendente |
| C-09 | Redaction, quota e RED continuam antes do WAL sem regressão de p99 | AD-4, AD-21 | Pass funcional; latência T1 pendente |

Se C-01 ou C-02 falhar, aciona o fallback do ADR-0002: gateway Rust/Rotel com
os mesmos receptores OTLP, envelope v1 e WAL. Se C-07 falhar, reavaliar o NATS
(adaptador Kafka/Redpanda) antes de trocar a linguagem. Se C-03/C-04 falhar, o
sizing e a política de retenção do WAL são o primeiro conserto, não a
linguagem.

## Evidência da Story 4.5

- **Outage de storage:** ClickHouse parado por 15 minutos sob carga contínua.
  O WAL absorveu o backlog, o gateway ficou pronto, o writer retentou com
  backoff exponencial e, após a recuperação, as contagens e o
  `uniqExact(event_id)` do ClickHouse bateram com o writer. Ver
  `bench/chaos/results/2026-11-04-local-storage-15m/report.md`.
- **Falha de NATS e gateway:** o gateway retornou 5xx/503 enquanto o NATS estava
  fora, reconectou sozinho e o gateway reiniciado retomou; os consumidores
  duráveis sobreviveram ao restart do NATS. As duplicatas ficaram dentro do
  limite at-least-once e foram absorvidas pelo dedupe de `event_id`. Ver
  `bench/chaos/results/2026-11-04-local-all/report.md`.
- **Capacidade do WAL:** um stream minúsculo devolveu 429
  `wal_backpressure` com `Retry-After`; depois que o ClickHouse voltou e os
  writers deram ack, os streams WorkQueue liberaram espaço e a ingestão
  retomou. Ver `bench/chaos/results/2026-11-04-local-capacity/report.md`.

## O que mudaria a recomendação

1. O T1 mostrar Go abaixo de 15k eventos/s por vCPU depois de profiling e
   ajuste de GC.
2. p99 de request ou lag p95 do T1 perder o gate com o gateway a < 60% de CPU.
3. O dreno do writer não acompanhar um backlog de 15 min a 1M/s.
4. Uma exigência de segurança ou tenancy forçar um hot path isolado que Go não
   atenda na densidade necessária.

Até lá, trocar a linguagem é custo sem evidência.

## Consequências

- Gateway e writer continuam em Go; o time não mantém um segundo hot path.
- OTel Collector segue suportado na borda; o core não depende dele.
- O fallback é preservado pelos contratos OTLP-first e envelope v1.
- O gate T1 (P-07) é o ponto de decisão; a recomendação não é dispensa do
  benchmark do G1.
