# ADR-0005 — Tenancy e retenção

- **Status:** aceito
- **Data:** 2026-10-31
- **Decisores:** Fundador + Arquitetura
- **Revisores:** kaikeventura (Fundador)
- **Consultados:** Produto + Segurança
- **Relacionados:** AD-2, AD-3, AD-17, AD-19, AD-20, AD-28, AD-29, AD-33, AD-35; P-02, P-03; S0.4, S0.8; Stories 3.5, 4.2
- **Substitui:** —
- **Substituído por:** —

## Contexto

O Observaki é self-hosted, mas não é single-user. Organizações têm projetos,
times, quotas, permissões e retenção próprias. O isolamento precisa valer em
todos os caminhos: ingestão, storage, consulta, cache, alertas, exportação e
exclusão. Um header forjado, um cache sem epoch ou um path errado no
VictoriaMetrics viram vazamento cross-tenant.

As restrições que governam a decisão:

- `tenant_id`/`project_id` em toda linha e consulta; nunca confiar em header
  do cliente (AD-2, AD-3).
- `project_id` imutável, com geração/epoch em todas as chaves; move e delete
  precisam de contrato (AD-20).
- O Query Service injeta o predicado de tenant e o ClickHouse aplica row
  policies; o VictoriaMetrics não tem row policy nativa e depende de `vmauth`
  e path autenticado (AD-3).
- Retenção por projeto para logs/traces; métricas globais no MVP; mudança sem
  restart (FR-40).
- `ingested_at` é a autoridade de TTL, não o timestamp do cliente (AD-29).
- Auditoria append-only e verificável (AD-35).
- Default de retenção (7/15/30 dias) ainda é pendência P-02; retenção de
  métricas por projeto/tenant é candidata enterprise (P-03).

## Decisão

### Identidade e isolamento

1. **Tenant = organização; projeto = unidade de isolamento, quota, permissão e
   retenção.** Toda telemetria carrega `(tenant_id, project_id, signal,
   event_id)`; logs/traces carregam `trace_id`/`span_id` quando existirem.
2. **`project_id` é imutável e autoritativo.** `project_generation` e
   `authz_epoch` acompanham a identidade em linha do ClickHouse, path do
   VictoriaMetrics, subject do NATS, chave de cache, token e auditoria.
3. **Enforcement em profundidade:**
   - gateway resolve o token em tenant/projeto antes do pipeline;
   - API autentica e emite contexto assinado curto (AD-17); o Query Service
     valida a assinatura e injeta o predicado — nunca confia em `X-Tenant`;
   - ClickHouse usa row policies e bloqueia table functions perigosas para o
     role de usuário;
   - VictoriaMetrics é acessado via `vmauth`, com path/tenant autenticado;
   - cache inclui tenant, projeto, `authz_epoch` e versão de schema.
4. **`project move` nunca é in-place.** É implementado como “novo projeto +
   cópia + delete” com epoch novo e auditoria; os dados antigos não mudam de
   dono por UPDATE. Se o custo não couber na Fase 1, a alternativa é proibir
   move no MVP, mas isso exige emenda explícita ao FR-45.
5. **Delete é job assíncrono com tombstone**, cobre hot e cold, é auditado e
   não depende só de TTL. A meta de 24 h do FR-44 é da Fase 3; o MVP declara o
   que consegue cumprir.
6. **RBAC básico** (admin/editor/viewer por projeto) no MVP; RBAC granular é
   candidato enterprise (P-03), sem paywall no caminho de telemetria.

### Retenção

7. **O PostgreSQL guarda o desejo, não o TTL efetivo.** `retention_policies`
   (AD-28) é a fonte de verdade; um reconciliador único aplica a política no
   ClickHouse/VM. O data plane aplica o último snapshot conhecido e expõe
   `config_age_seconds` (AD-19).
8. **Logs e traces:** retenção por projeto; `ingested_at` define partição/TTL;
   timestamps do cliente são preservados com flag de skew e rejeitados ou
   clampados fora de ±24 h (configurável) (AD-29).
9. **Métricas:** retenção global da instância no MVP. Retenção por
   projeto/tenant de métricas fica para a Fase 2/P-03, via instâncias ou
   recurso validado no backend; a UI precisa deixar claro o que é global.
10. **Mudança de retenção não exige restart** e é auditada. A UI mostra o
    valor efetivo e a última reconciliação, não só o desejo.
11. **Default de retenção:** 7, 15 ou 30 dias permanece P-02; até a decisão, o
    default conservador é 15 dias para logs/traces (valor do DDL de referência
    do addendum), explicitamente registrado como provisório.

### Decisão do spike da Fase 0 (Story 3.5)

14. **Estratégia escolhida: TTL por linha (`retention_days`) em uma única
    tabela.** O spike de 2026-11-02 validou 7/15/30 dias com um probe de TTL
    (`project-0` 7d, `project-1` 15d, `project-2` 30d) e mediu a mutação de
    troca de política em 10M linhas (~0,49 s). O custo é o merge de TTL: uma
    part com retenções mistas reescreve também as linhas vivas quando a
    primeira linha expira. O fallback de tabelas por bucket 7/15/30 continua
    documentado e é a saída se o custo de merge no T1 (10B/86B) estourar o
    orçamento. A estratégia por bucket evita a reescrita de linhas vivas, mas
    triplica o schema, o roteamento do gateway e o número de parts; por isso
    não é o default do MVP.
15. **Consulta e rollups.** As 20 consultas de dashboard rodaram no dataset de
    1B eventos (300M logs + 100M spans + 600M amostras de métricas): 16 no
    ClickHouse com p95 < 1 s e 4 no VictoriaMetrics com p95 799,5 ms. O tier
    de 10B/7d é uma projeção linear window-equivalent (1,43x o de 1B/24h,
    617 ms projetados) e o de 86B/24h foi medido em rollups de 1 minuto
    (9 consultas agregadas, p95 máximo 87 ms). O raw 10B/86B continua sendo
    item do T1/G1; os rollups não substituem a medição raw.
16. **Isolamento.** Row policies por tenant com `project_generation` e
    `authz_epoch` foram validadas no ClickHouse; table functions perigosas e
    tabelas de sistema sensíveis foram bloqueadas. O Query Service ainda não
    existe, então o predicado injetado pelo QS (AD-3/AD-17) é simulado pelo
    usuário de benchmark e continua sendo requisito da Fase 1.

### Auditoria e testes

12. **`audit_events` append-only com hash chain**, permissões restritas,
    exportação contínua e auditoria de acesso à auditoria (AD-35).
13. **Testes de isolamento são obrigatórios no CI**: row policy no ClickHouse,
    path do VictoriaMetrics, cache, consulta do Query Service, gateway,
    move/delete e noisy neighbor. Dois tenants com p95 do vizinho < 10%
    (AD-33).

## Alternativas consideradas

| Alternativa | Prós | Contras | Por que não foi escolhida |
|---|---|---|---|
| **Schema compartilhado com `tenant_id`/`project_id` + row policies** | Eficiente, um cluster, operação simples, bom para o ICP | Exige disciplina em todo caminho; row policy não cobre VM; risco de vazamento se o predicado falhar | **Escolhida** para o MVP |
| Banco/cluster por tenant | Isolamento físico forte | Custo e operação proibitivos para times pequenos; não escala para muitos tenants | Rejeitada no MVP; possível oferta enterprise futura |
| Instância de VictoriaMetrics por tenant | Isolamento de métricas | Muita RAM/operação; overhead por tenant | Rejeitada |
| `project move` in-place (UPDATE de tenant) | Simples na UI | Reescreve dados históricos, quebra cache/tokens e arrisca vazamento | Rejeitada; usar novo projeto + cópia + delete |
| TTL pelo timestamp do cliente | Fácil de implementar | Relógio errado ou malicioso infla custo ou apaga dados | Rejeitada; `ingested_at` é autoridade |
| Colunas `retention_*` na tabela `projects` | Menos uma tabela | Duplica o dono da retenção e conflita com AD-28 | Rejeitada; usar `retention_policies` |
| Delete só por TTL | Nenhum job novo | Não cumpre exclusão em 24 h nem auditoria; dados ficam até o TTL | Rejeitada; tombstone + job assíncrono |
| Confiar no header de tenant da API | Menos validação no QS | API comprometida ou chamada interna forjada atravessa tenants | Rejeitada; contexto assinado (AD-17) |

## Consequências

### Positivas

- Um cluster serve muitos projetos com custo previsível para o ICP.
- O isolamento é testável em cada camada, não só na UI.
- Retenção por projeto para logs/traces atende o caso de uso principal sem
  esperar a Fase 3.
- Move/delete têm contrato explícito, com epoch e auditoria.
- A separação entre desejo (PG) e TTL efetivo evita dois donos.

### Negativas

- Complexidade em todos os caminhos: gateway, QS, CH, VM, cache e NATS.
- Move é caro (cópia + delete) e pode ser lento em projetos grandes.
- Delete full hot/cold é Fase 3; o MVP não promete 24 h.
- O VictoriaMetrics não tem row policy; a barreira é o `vmauth`/path, que
  precisa de teste dedicado.
- A retenção de métricas por projeto não existe no MVP; a UI precisa explicar.
- Testes de isolamento e noisy neighbor aumentam o tempo de CI.

### Riscos aceitos e mitigações

| Risco | Probabilidade | Impacto | Mitigação | Dono |
|---|---|---|---|---|
| Vazamento cross-tenant por predicado/cache/path | Baixa | Crítico | Contexto assinado, row policy, vmauth, cache com epoch, testes de isolamento no CI | Segurança + Arquitetura |
| `project move` perde ou duplica dados | Média | Alto | Cópia idempotente por `event_id`, tombstone, auditoria, teste de move | Plataforma |
| TTL por projeto não é viável | Média | Médio | S0.8; fallback tabelas por bucket | Arquitetura |
| Retenção efetiva diverge do desejo no PG | Média | Médio | Reconciliador único, `config_age_seconds`, UI mostra efetivo e última reconciliação | Plataforma |
| Noisy neighbor de cardinalidade | Média | Alto | Admissão antes do ack (AD-33), quotas, teste de dois tenants | Arquitetura |
| Default de retenção provisório vira definitivo por inércia | Média | Médio | P-02 com prazo na Semana 2; ADR atualizado quando decidido | Produto |

## Status

`aceito` em 2026-10-31, com revisão registrada abaixo. A decisão fixa
identidade, isolamento em profundidade e o mecanismo de retenção. S0.8 e S0.4
provam row policies, TTL por projeto e isolamento no VM; P-02 define o default
de retenção e P-03 decide a retenção de métricas por projeto.

**Evidência pendente para o G1 (não bloqueia a decisão):**

1. S0.8 provar row policies, TTL por projeto e isolamento no ClickHouse/VM;
2. P-02 decidir o default de retenção (7/15/30);
3. definir o escopo de move/delete no MVP e o SLA da Fase 3.

## Revisão

| Revisor | Papel | Data | Resultado | Referência |
|---|---|---|---|---|
| kaikeventura | Fundador | 2026-10-31 | aprovado | Revisão da sessão de 2026-10-31 |

## Questões abertas

| ID | Questão | Spike/story da Fase 0 | Dono | Prazo | Fallback |
|---|---|---|---|---|---|
| Q-01 | Row policies + predicado do QS isolam logs/traces sem bypass por MVs/views? | S0.8 / Story 3.5 | Segurança | Semana 4 | Role dedicado, views com `SQL SECURITY INVOKER`, teste de ataque. **Parcial (Story 3.5):** row policies + `authz_epoch` validados no ClickHouse; o predicado do QS entra na Fase 1. |
| Q-02 | TTL por projeto é viável por coluna ou exige tabelas por bucket? | S0.8 / Story 3.5 | Arquitetura | Semana 4 | **Fechada (Story 3.5):** TTL por linha validado para 7/15/30; fallback de tabelas por bucket documentado para o T1. |
| Q-03 | Qual o default de retenção: 7, 15 ou 30 dias? | P-02 / Story 3.5 | Produto | Semana 2 | 15 dias provisório, documentado na UI |
| Q-04 | O MVP suporta `project move` ou proíbe com emenda ao FR-45? | Story 3.5 / Story 5.5 | Produto + Arquitetura | Semana 4 | Proibir move no MVP e registrar a emenda |
| Q-05 | O path/vmauth do VictoriaMetrics isola tenants de forma testável? | S0.4 / Story 3.4–3.5 | Segurança | Semana 3 | Proxy dedicado com tenant no path e teste de query vazia |
| Q-06 | Delete hot/cold em 24 h (FR-44) entra na Fase 3? | Story 3.5 / Fase 3 | Produto | Antes da Fase 3 | Declarar SLA do MVP e manter tombstone + auditoria |
| Q-07 | Retenção de métricas por projeto fica no OSS ou no enterprise (P-03)? | S0.4 / Fase 2 | Fundador | Antes da Fase 2 | Manter global no OSS e documentar a limitação |
| Q-08 | Qual o teste de cache que prova invalidação por `authz_epoch`? | Story 3.5 / Story 5.5 | Plataforma | Semana 4 | Invalidar todo o cache em mudança de epoch; teste de revogação. **Parcial (Story 3.5):** epoch invalida linhas no ClickHouse; o cache entra na Story 5.5. |

## Referências

- PRD: FR-40, FR-44, FR-45, FR-46, FR-47, FR-49; NFR-6, NFR-7; §4.9, §11.
- Addendum: §5 (modelo de dados), §7 (spike S0.8), §9 (segurança).
- Arquitetura: AD-2, AD-3, AD-17, AD-19, AD-20, AD-28, AD-29, AD-33, AD-35.
- Revisão adversarial: `review-architecture.md`, achados C-04, C-05, A-10, A-12, A-15, A-16.
- Decisões: P-02 (retenção default), P-03 (open-core/retention de métricas).

## Histórico

| Data | Mudança | Autor |
|---|---|---|
| 2026-10-31 | Criação (Story 1.4) | OpenCode |
| 2026-11-02 | Decisão do spike 3.5: TTL por linha para 7/15/30, rollups 1m para 86B/24h, Q-02 fechada | OpenCode |
