# ADR-0003 — Fila/WAL: NATS JetStream

- **Status:** aceito
- **Data:** 2026-10-31
- **Decisores:** Fundador + Arquitetura
- **Revisores:** kaikeventura (Fundador)
- **Consultados:** —
- **Relacionados:** AD-4, AD-18, AD-19, AD-20, AD-22, AD-31; D-04; S0.3; Stories 4.3–4.5
- **Substitui:** —
- **Substituído por:** —

## Contexto

O gateway só pode confirmar um evento depois que ele está durável (AD-4). O
WAL precisa absorver a indisponibilidade do ClickHouse e do VictoriaMetrics,
preservar ordem por partição quando necessário, permitir replay e não perder
eventos já confirmados ao cliente. O critério do spike S0.3 é 1M eventos/s com
**zero perda** durante 15 minutos de falha do ClickHouse.

O NATS JetStream é o default do addendum: binário Go, baixa latência,
at-least-once e operação simples. Kafka/Redpanda é opcional para clientes que
já operam o broker. Escrita direta no ClickHouse é fallback de single-node,
não do cluster.

A decisão precisa também cobrir o contrato interno (AD-22), a identidade de
tenant (AD-20), a autenticação entre serviços (AD-31) e o caminho de
configuração (AD-19). Sem isso, um rolling upgrade ou um replay pode gravar
tenant/evento errado.

## Decisão

**Adotar NATS JetStream como WAL padrão do data plane**, com os seguintes
contratos:

1. **Publicação durável antes do ack.** O gateway publica no stream e só
   responde ao produtor depois do ack persistido do JetStream. Em single-node
   sem NATS, é permitido um buffer local durável com o mesmo contrato de
   envelope e ack.
2. **Envelope versionado.** A mensagem usa protobuf com `schema_version`,
   campos reservados e compatibilidade N-1 (AD-22). O subject carrega
   `tenant_id`, `project_id`, `signal` e `project_generation`/`authz_epoch`
   (AD-20); o nome exato é decidido na Story 4.3.
3. **At-least-once, sem promessa de exactly-once.** Consumo pode reentregar.
   O dedupe usa `event_id` estável (sintetizado pelo gateway quando o produtor
   não fornece) e `Nats-Msg-Id` na publicação; a janela de dedupe é maior que
   o maior retry/outage. O writer é idempotente por `event_id`.
4. **Ack do writer só após durabilidade no store.** O writer confirma no NATS
   depois de `async_insert=0`/`wait_for_async_insert=1` ou confirmação de
   flush no ClickHouse, e de aceite no VictoriaMetrics. Rejeição permanente
   vai para DLQ com procedimento de replay; rejeição transitória volta para
   retry com backoff.
5. **Retenção do stream maior que a maior indisponibilidade + reparo.** O
   stream precisa cobrir 15 minutos de ClickHouse fora e o tempo de reparo
   declarado; o sizing é medido no S0.3 e registrado na Story 4.3.
6. **Backpressure explícito.** Stream cheio ou store lento gera 429 +
   `Retry-After` no gateway, não ack silencioso.
7. **Segurança.** NATS com accounts/credenciais por componente e mTLS entre
   serviços (AD-31); nenhum subject é acessível sem autorização.
8. **Kafka/Redpanda é adaptador opcional.** Um cliente que já opera Kafka pode
   usá-lo na ingestão/consumo, mas o default e o benchmark continuam NATS.

**Fora do escopo desta decisão:** o fator de replicação do cluster, o número
de streams/partições, o formato exato do DLQ e a topologia de single-node
(sem NATS vs. NATS local). Esses pontos são fechados nas Stories 4.3–4.5 e no
S0.3.

## Alternativas consideradas

| Alternativa | Prós | Contras | Por que não foi escolhida |
|---|---|---|---|
| **NATS JetStream** | Leve, baixa latência, at-least-once, binário Go, operação simples, bom encaixe com o resto da stack | Throughput por nó menor que Kafka; ecossistema menor; menos ferramentas de inspeção | **Escolhida** como default, condicionada ao S0.3 |
| Kafka/Redpanda | Throughput e durabilidade comprovados, ecossistema maduro | Peso operacional, JVM/recursos, tuning de partições; overkill para o default | Adaptador opcional para clientes que já têm Kafka |
| Escrita direta no ClickHouse | Menos componentes, menor latência | Sem buffer durável; outage do CH vira perda ou backpressure; sem replay | Apenas fallback de single-node, nunca o caminho do cluster |
| WAL local em disco como default | Zero dependência externa | Escala limitada, estado por réplica, operação de disco/rotação, sem HA | Fallback de single-node; não é o default do cluster |
| Pulsar | Multi-tenancy e tiering nativos | Complexidade operacional alta, ecossistema menor | Rejeitada |
| RabbitMQ | Simples para filas de trabalho | Não é feito para stream de telemetria de alto volume nem replay | Rejeitada |

## Consequências

### Positivas

- Um componente Go leve no caminho crítico, com operação mais simples que
  Kafka para o time pequeno.
- Ack durável antes da resposta ao produtor, com replay e DLQ explícitos.
- O mesmo contrato de envelope serve NATS, buffer local e eventual adaptador
  Kafka.
- O NATS também pode transportar o snapshot de configuração (AD-19), evitando
  um segundo barramento.

### Negativas

- Throughput por nó menor que Kafka; o cluster pode precisar de mais nós ou
  streams.
- At-least-once significa duplicatas visíveis se o dedupe falhar; não há
  exactly-once.
- O time precisa operar JetStream: storage, replicação, snapshots, upgrade e
  inspeção de DLQ.
- Single-node sem NATS cria um segundo caminho de durabilidade, com semântica
  de RPO diferente.

### Riscos aceitos e mitigações

| Risco | Probabilidade | Impacto | Mitigação | Dono |
|---|---|---|---|---|
| NATS não sustenta 1M/s com 15 min de CH fora | Média | Alto | S0.3 antes do compromisso; fallback WAL local + Kafka opcional | Arquitetura |
| Retenção do stream menor que o outage | Média | Alto | Dimensionar por outage + reparo; alerta de idade da mensagem mais antiga | Plataforma |
| Replay grava duplicata ou tenant errado | Média | Alto | `event_id` determinístico, dedupe, subject com geração/epoch, teste de replay | Ingestão |
| Upgrade quebra envelope antigo | Média | Médio | Protobuf N-1, dual-publish/dual-read, teste de contrato no CI (AD-22) | Plataforma |
| Single-node tem RPO diferente do cluster | Alta | Médio | Declarar RPO por topologia na matriz de capacidades; não prometer RPO 0 no single-node | Produto + Arquitetura |

## Status

`aceito` em 2026-10-31, com revisão registrada abaixo. NATS JetStream é o WAL
default; S0.3 prova o throughput e a durabilidade. Se falhar, o fallback é WAL
local + Kafka opcional, sem mudar o contrato de envelope.

**Evidência pendente para o G1 (não bloqueia a decisão):**

1. S0.3 provar 1M/s com zero perda em 15 minutos de falha do ClickHouse;
2. definir retenção do stream, replicação e DLQ na Story 4.3.

## Revisão

| Revisor | Papel | Data | Resultado | Referência |
|---|---|---|---|---|
| kaikeventura | Fundador | 2026-10-31 | aprovado | Revisão da sessão de 2026-10-31 |

## Questões abertas

| ID | Questão | Spike/story da Fase 0 | Dono | Prazo | Fallback |
|---|---|---|---|---|---|
| Q-01 | O JetStream sustenta 1M/s com zero perda em 15 min de ClickHouse fora? | S0.3 / Story 4.3, 4.5 | Benchmark | Semana 3 | WAL local em disco + retry; Kafka opcional em produção |
| Q-02 | Qual a retenção mínima do stream para cobrir outage + reparo? | S0.3 / Story 4.3 | Plataforma | Semana 3 | Aumentar disco/replicação; alerta de idade da fila |
| Q-03 | Single-node usa NATS local ou buffer em disco? | S0.3 / Story 5.1 | Arquitetura | Semana 4 | Buffer local com RPO declarado; NATS no Compose se o custo couber |
| Q-04 | Qual o fator de replicação do stream no cluster? | S0.3 / Story 2.4 | Benchmark | Fim da Fase 0 | R3 conservador com headroom; revisar após o primeiro parceiro |
| Q-05 | O contrato N-1 cobre replay após upgrade? | Story 4.3 / AD-22 | Plataforma | Semana 4 | Dual-read no writer e janela de compatibilidade estendida |
| Q-06 | Há demanda real por adaptador Kafka no MVP? | Story 4.3 / Epic 7 | Produto | Semana 4 | Manter interface de fila e adiar o adaptador para a Fase 1 |

## Referências

- PRD: FR-4, FR-5, FR-52; NFR-1, NFR-5; §10.1.
- Addendum: §4.2 (fila), §6 (benchmark), §7 (spike S0.3).
- Arquitetura: AD-4, AD-18, AD-19, AD-20, AD-22, AD-31; fluxo de ingestão.
- Revisão adversarial: `review-architecture.md`, achados C-03, C-04, A-01, M-01.
- Decisões: D-04 (stack).

## Histórico

| Data | Mudança | Autor |
|---|---|---|
| 2026-10-31 | Criação (Story 1.4) | OpenCode |
