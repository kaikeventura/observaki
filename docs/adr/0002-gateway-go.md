# ADR-0002 — Gateway de ingestão em Go

- **Status:** proposto
- **Data:** 2026-10-31
- **Decisores:** Fundador + Arquitetura
- **Revisores:** pendente
- **Consultados:** —
- **Relacionados:** AD-1, AD-4, AD-6, AD-17, AD-21, AD-31; D-04; S0.2, S0.7; Stories 4.1–4.5
- **Substitui:** —
- **Substituído por:** —

## Contexto

O gateway é a porta de entrada do data plane. Ele recebe OTLP gRPC (4317) e
OTLP HTTP (4318), resolve o token em `tenant_id`/`project_id`, aplica quota,
redaction e pipeline, calcula RED **antes** do sampling e publica no WAL. Só
depois do ack durável ele responde ao produtor (AD-4). Ele também não pode
depender do PostgreSQL no hot path (AD-1).

As restrições de escala são explícitas:

- 1M eventos/s no benchmark do MVP, com 4–6 gateways de 16 vCPU e headroom;
- critério do spike S0.2: 15–20k eventos/s por vCPU;
- < 8 GB de RAM por gateway (NFR-9);
- lag p95 < 10 s e zero perda para eventos confirmados (NFR-1, NFR-5);
- OTel Collector é compatibilidade de borda, não o core: o addendum registra
  ~10k eventos/s por instância em configuração típica e uma frota grande em
  escala.

A escolha da linguagem afeta throughput, pausas de GC, custo de contratação e
a velocidade de entrega. O spine não fixa a linguagem do gateway; este ADR
fixa Go e o critério de fallback.

## Decisão

**Implementar o gateway em Go 1.27**, como serviço stateless do data plane
(`internal/ingest`), com múltiplas réplicas na mesma imagem do produto
(AD-12).

Regras que decorrem da decisão:

1. **Contrato OTLP-first.** O gateway aceita OTLP gRPC/HTTP e Prometheus remote
   write; nenhum SDK proprietário é obrigatório. OTel Collector pode ficar na
   borda do cliente.
2. **Stateless.** O estado de durabilidade vive no WAL (ADR-0003) e no buffer
   local durável do single-node. Réplicas podem ser reiniciadas e balanceadas
   sem sticky session.
3. **Identidade antes do pipeline.** O token resolve tenant/projeto antes de
   qualquer processamento; nenhum writer grava sem os dois (AD-2). O gateway
   não confia em header de tenant do cliente.
4. **RED antes do sampling.** Span metrics/RED são calculados no gateway e
   gravados apenas no VictoriaMetrics (AD-21). Nenhum sampling pode acontecer
   antes desse cálculo.
5. **Ack só depois do WAL.** O gateway responde 200/partial_success apenas
   após o ack durável do NATS JetStream ou do buffer local. Backpressure usa
   429 + `Retry-After`; rejeições permanentes vão para DLQ (ADR-0003).
6. **Sem PostgreSQL no hot path.** Quotas, pipeline e auth usam o snapshot de
   configuração (AD-19) e cache de chaves; o gateway falha fechado para
   segurança/quota quando o snapshot está velho.
7. **Observabilidade do próprio gateway.** `/healthz`, `/readyz`, métricas
   Prometheus e traces OTel (AD-10); `config_age_seconds` exposto (AD-19).

**Fora do escopo desta decisão:** a linguagem do agente (Rust, Fase 2), o
empacotamento em um ou mais binários e o desenho exato dos processors de
pipeline (Stories 4.1–4.3).

## Alternativas consideradas

| Alternativa | Prós | Contras | Por que não foi escolhida |
|---|---|---|---|
| **Go** | Concorrência simples, bibliotecas OTel maduras, baixo overhead, entrega rápida, mesma linguagem do resto do backend | GC e pausas em picos extremos; menos eficiente por core que Rust | **Escolhida**, condicionada ao S0.2 |
| Rust (gateway nativo ou Rotel) | Máxima eficiência por core, sem GC, binário pequeno | Custo de desenvolvimento maior, ecossistema OTel menos maduro, contratação | Fallback do hot path se Go não atingir 15–20k/s por vCPU; agente da Fase 2 já usa Rust |
| OTel Collector como core | Compatibilidade total, processors prontos, comunidade | Throughput por instância menor, frota grande em 1M/s, configuração complexa | Usado na borda, não como gateway de alto volume |
| Java/JVM | Ecossistema e ferramentas | Footprint, pausas de GC, operação mais pesada | Rejeitada |
| Node.js | Entrega rápida | Throughput e uso de CPU piores no hot path | Rejeitada |

## Consequências

### Positivas

- Uma linguagem e um toolchain para gateway, API, Query Service e writers;
  bibliotecas OTel, NATS e protobuf são maduras em Go.
- Contratação e revisão mais simples para o time pequeno do MVP.
- Binário estático, imagem distroless e deploy previsível.
- O fallback para Rust é possível sem mudar o contrato OTLP nem o envelope
  interno (AD-22).

### Negativas

- GC e alocação por evento exigem tuning e benchmark; p99 é mais sensível que
  em Rust.
- Eficiência por core menor; o cluster precisa de mais réplicas ou vCPU.
- Se o fallback for acionado, o time mantém dois hot paths por um período.

### Riscos aceitos e mitigações

| Risco | Probabilidade | Impacto | Mitigação | Dono |
|---|---|---|---|---|
| Go não atinge 15–20k/s por vCPU | Média | Alto | S0.2 mede antes do compromisso; fallback Rust/Rotel sem mudar OTLP | Arquitetura |
| Pausas de GC estouram p99 de ingestão | Média | Médio | Tuning de `GOGC`/`GOMEMLIMIT`, pooling, benchmark de 1 h; alerta de p99 | Gateway |
| Gateway vira gargalo de RED/cardinalidade | Média | Médio | Medir custo do cálculo de RED no S0.2; admissão de cardinalidade (AD-33) | Arquitetura |
| Snapshot de config velho bloqueia tráfego | Baixa | Alto | Cache local, staleness máxima e fail-closed só para segurança/quota (AD-19) | Plataforma |

## Status

`proposto`. Para `aceito`, faltam:

1. S0.2 provar 15–20k eventos/s por vCPU e 1M/s com 4–6 gateways;
2. S0.7 provar que o cache de auth/JWKS não adiciona dependência do
   PostgreSQL no hot path;
3. revisão humana registrada (AC3 da Story 1.4).

## Revisão

| Revisor | Papel | Data | Resultado | Referência |
|---|---|---|---|---|
| pendente | — | — | pendente | — |

## Questões abertas

| ID | Questão | Spike/story da Fase 0 | Dono | Prazo | Fallback |
|---|---|---|---|---|---|
| Q-01 | Go atinge 15–20k eventos/s por vCPU com o workload 60/30/10? | S0.2 / Story 4.1–4.4 | Benchmark | Semana 3 | Migrar hot path para Rust/Rotel mantendo OTLP |
| Q-02 | O cálculo de RED antes do sampling cabe no orçamento de CPU e memória? | S0.2 / Story 4.1, 4.4 | Arquitetura | Semana 3 | Pré-agregar no writer ou reduzir labels; nunca amostrar antes do RED |
| Q-03 | O cache de auth/JWKS mantém o gateway independente do PostgreSQL? | S0.7 / Story 4.2 | Plataforma | Semana 3 | Cache local com TTL curto e fail-closed para decisões novas |
| Q-04 | O gateway cabe em < 8 GB de RAM por réplica no pico? | S0.2 / Story 4.1 | Benchmark | Semana 3 | Reduzir batch, pooling, revisar limite de concorrência |
| Q-05 | Qual o critério objetivo para acionar o fallback Rust? | S0.2 / Story 4.5 | Arquitetura | Semana 4 | Falha de throughput ou p99 acima da meta por 2 execuções consecutivas |

## Referências

- PRD: FR-1, FR-4, FR-5, FR-46, FR-50, FR-52; NFR-1, NFR-5, NFR-6, NFR-9; §10.1.
- Addendum: §4.1 (gateway), §6 (harness), §7 (spike S0.2/S0.7).
- Arquitetura: AD-1, AD-4, AD-6, AD-17, AD-21, AD-31; fluxo de ingestão.
- Revisão adversarial: `review-architecture.md`, achados C-01, C-03, C-04, A-01.
- Decisões: D-04 (stack).

## Histórico

| Data | Mudança | Autor |
|---|---|---|
| 2026-10-31 | Criação (Story 1.4) | OpenCode |
