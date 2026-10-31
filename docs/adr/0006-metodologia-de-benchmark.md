# ADR-0006 — Metodologia de benchmark

- **Status:** proposto
- **Data:** 2026-10-31
- **Decisores:** Fundador + Benchmark
- **Revisores:** pendente
- **Consultados:** Arquitetura + Produto
- **Relacionados:** AD-14, AD-16, AD-29; NFR-1, NFR-2, NFR-13; AR-2, AR-3; S0.1–S0.6; Stories 2.1–2.4
- **Substitui:** —
- **Substituído por:** —

## Contexto

O Gate G1 decide se a stack entra na Fase 1. A decisão não pode depender de
números de vendor, de uma execução escolhida a dedo ou de hardware não
documentado. O benchmark precisa ser reproduzível, comparável entre releases
e capaz de provar (ou refutar) as metas:

- 1M eventos/s por 1 hora no workload de referência;
- lag p95 < 10 s e zero perda para eventos confirmados;
- p95 < 1 s em 1B linhas/24h e < 3 s em 10B linhas/7d;
- compressão ≥ 5x em logs e traces;
- 100k pontos < 100 ms e bundle inicial < 180 KB gzip no frontend;
- custo de infraestrutura por TB/dia dentro do alvo.

O workload de referência é 60% métricas (~40 B), 30% logs (~200 B) e 10%
traces (~500 B), com média de ~134 B comprimidos; 1M/s ≈ 11,6 TB/dia. O
harness vive em `bench/` e o relatório em `BENCHMARKS.md` + JSON.

## Decisão

**Adotar um harness reproduzível em `bench/` e um protocolo único de medição,
relatório e go/no-go.**

### Workload e cenários

1. **Gerador determinístico.** `telemetrygen` + gerador Go próprio, com seed
   fixa, mix 60/30/10 e tamanhos de payload 40 B / 200 B / 500 B (1 KB como
   stress). A taxa é configurável em 100k, 1M e 5M eventos/s.
2. **Throughput:** aquecimento, rampa e 1 hora em regime para o gate de 1M/s.
   Reportar taxa sustentada, lag p50/p95/p99, erros, drops e eventos
   confirmados perdidos.
3. **Consulta:** 20 consultas representativas de dashboard, incluindo janelas
   de 1B linhas/24h, 10B/7d e um plano para 86B/24h com rollups. Reportar
   p50/p95/p99 e o plano de execução.
4. **Cardinalidade:** 100k, 1M e 10M séries; teste de dois tenants para provar
   que o p95 do vizinho não varia mais de 10%.
5. **Caos:** kill do ClickHouse, NATS, gateway e nó; 15 minutos de ClickHouse
   fora; zero perda para eventos confirmados; medir recuperação, lag e DLQ.
6. **Frontend:** 100k pontos < 100 ms, bundle inicial < 180 KB gzip,
   dashboard < 250 KB, LCP/INP e axe; medir por rota, não só a média do app.
7. **Retenção/compressão:** medir razão comprimido/bruto por sinal, TTL
   efetivo e custo por TB/dia com o fator de réplica declarado.

### Regras de medição

8. **Ambiente fixo e declarado:** hardware (CPU, RAM, disco, rede), versões
   exatas de SO, Go, ClickHouse, VictoriaMetrics, NATS, PostgreSQL e
   ferramentas, configuração completa e commit.
9. **Sem cherry-picking.** O relatório inclui todas as execuções, inclusive as
   que falharam, e explica outliers. Números de vendor só entram com
   reprodução própria.
10. **Distribuição, não média.** Publicar p50/p95/p99, taxa de erro, lag,
    throughput e dispersão entre execuções. Latência de consulta e cenários de
    falha têm no mínimo 3 execuções; throughput de 1 hora pode ser uma
    execução contínua se as métricas de estabilidade forem publicadas.
11. **Estado separado.** Aquecimento e cold start são reportados à parte; não
    entram no número de regime.
12. **Relatório versionado.** `BENCHMARKS.md` (leitura humana) + JSON (dados
    brutos) com commit, hardware, config, seed, resultados e veredito
    go/no-go. Todo release publica o relatório; benchmark de release é
    comparado com o anterior.
13. **Regressão é bug P1.** Uma regressão de throughput, latência, compressão
    ou bundle bloqueia o release e exige benchmark antes/depois (AD-14).
14. **Custo junto com performance.** O relatório inclui custo por TB/dia,
    horas de operação e premissas de TCO; performance sem custo não decide o
    gate.

### Go/no-go do G1

| Critério | Meta | Evidência |
|---|---|---|
| Throughput | 1M eventos/s por 1 h | Relatório de throughput |
| Lag | p95 < 10 s; p99 < 30 s | Métricas do gateway/WAL |
| Perda | Zero para eventos confirmados | Teste de caos + contagem |
| Consulta 1B/24h | p95 < 1 s | 20 consultas representativas |
| Consulta 10B/7d | p95 < 3 s | 20 consultas representativas |
| Compressão | ≥ 5x em logs e traces | Razão comprimido/bruto |
| Retenção/isolamento | TTL por projeto e row policies validados | S0.8 |
| Frontend | 100k pontos < 100 ms; bundle < 180 KB | S0.5 |
| Custo | Dentro do alvo de TCO | Modelo de custo do relatório |

**Se qualquer critério falhar, o gate permanece fechado.** O relatório registra
o fallback acionado (schema, Rust, WAL local/Kafka, React, etc.) e a nova
tentativa; não se arredonda número para “quase passou”.

## Alternativas consideradas

| Alternativa | Prós | Contras | Por que não foi escolhida |
|---|---|---|---|
| **Harness próprio + telemetrygen + relatório versionado** | Reprodutível, comparável, cobre throughput/consulta/caos/custo | Custo de construção e de hardware; exige disciplina | **Escolhida** |
| Benchmarks de vendor | Rápidos, prontos, bons para marketing | Hardware/config não controlados; sem commit; não comparáveis | Só como referência, nunca como evidência do gate |
| Carga sintética sem consultas | Simples | Prova ingestão, não prova a experiência de consulta | Rejeitada; consulta é metade do produto |
| Uma execução única | Barato | Não mostra variância nem regressão | Rejeitada; distribuição e pelo menos 3 execuções para latência |
| Medir só o happy path | Bonito no relatório | Esconde perda, lag e falha de recuperação | Rejeitada; caos é obrigatório |
| Benchmark contínuo em produção de cliente | Realista | Não reproduzível, risco de privacidade e de impacto no cliente | Complementar, não substitui o gate |

## Consequências

### Positivas

- O gate tem evidência reproduzível, com commit e configuração.
- Regressões de performance são detectadas antes do release.
- O relatório serve de base para sizing, TCO e conversa com design partners.
- O mesmo harness compara Go vs Rust, NATS vs fallback e schema A vs B.

### Negativas

- Exige hardware dedicado/alugado e tempo de engenharia.
- O relatório precisa ser mantido e atualizado a cada release.
- Benchmark sintético não captura toda a cauda longa de produção.
- A disciplina de publicar falhas e outliers desacelera narrativas de marketing.

### Riscos aceitos e mitigações

| Risco | Probabilidade | Impacto | Mitigação | Dono |
|---|---|---|---|---|
| Hardware não representa o T1 | Média | Alto | Declarar perfil e mapear para T-shirt sizes; validar com design partner | Benchmark |
| Número publicado é irreprodutível | Média | Alto | Commit, config, seed e JSON brutos; reprodução em clone limpo | Benchmark |
| Custo do benchmark estoura o orçamento | Média | Médio | Reutilizar máquinas, começar em single-node e escalar só o necessário | Fundador |
| Gate passa com workload irreal | Média | Alto | Workload 60/30/10 e 134 B ±10%; consultas reais de dashboard | Produto + Benchmark |
| Relatório vira marketing sem contexto | Baixa | Médio | Publicar limitações, hardware e data; proibir número sem reprodução | Fundador |

## Status

`proposto`. Para `aceito`, faltam:

1. Stories 2.1–2.4 implementarem o harness, os cenários e o relatório;
2. uma execução completa de referência (baseline) com hardware documentado;
3. revisão humana registrada (AC3 da Story 1.4).

## Revisão

| Revisor | Papel | Data | Resultado | Referência |
|---|---|---|---|---|
| pendente | — | — | pendente | — |

## Questões abertas

| ID | Questão | Spike/story da Fase 0 | Dono | Prazo | Fallback |
|---|---|---|---|---|---|
| Q-01 | Qual o perfil de hardware e o cluster de referência (6–12 CH, 3 VM, 4–6 gateways)? | Story 2.4 | Benchmark | Semana 3 | Cluster mínimo com headroom; documentar a diferença |
| Q-02 | Como gerar os datasets de 1B/10B/86B linhas de forma reproduzível? | Story 2.2 | Benchmark | Semana 3 | Dados sintéticos por seed + retenção simulada |
| Q-03 | Qual o conjunto de 20 consultas representativas? | Story 2.2 | Produto + Benchmark | Semana 2 | Consultas do mock de Explore/Dashboard |
| Q-04 | Como medir latência com variância e outliers sem mascarar a cauda? | Story 2.3 | Benchmark | Semana 3 | p50/p95/p99 + dispersão; publicar todas as execuções |
| Q-05 | Qual o RPO/RTO real do single-node no teste de caos? | Story 4.5 / Story 2.4 | Arquitetura | Semana 4 | Declarar RPO diferente por topologia; não prometer RPO 0 |
| Q-06 | Como medir bundle/LCP/INP de forma estável no CI? | S0.5 / Story 6.4 | Frontend | Semana 4 | Budget.json + Lighthouse em runner dedicado |
| Q-07 | Qual o modelo de custo por TB/dia e horas de operação? | Story 2.3 | Fundador + Benchmark | Semana 4 | TCO conservador com premissas publicadas |

## Referências

- PRD: NFR-1, NFR-2, NFR-13; SM-3, SM-4, SM-6, SM-7; §10.1, §10.3.
- Addendum: §6 (harness e critérios), §7 (spikes), §10 (TCO).
- Arquitetura: AD-14, AD-16, AD-29.
- Revisão adversarial: `review-architecture.md`, achados A-11, A-16.
- Gate: `STATUS.md` (G1), `PROGRAM-PLAN.md`.

## Histórico

| Data | Mudança | Autor |
|---|---|---|
| 2026-10-31 | Criação (Story 1.4) | OpenCode |
