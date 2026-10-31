# Fronteira open-core

> **Status: decisão de negócio pendente (P-03).** Este documento registra os
> princípios já confirmados e os **candidatos** a recursos enterprise. Ele não
> anuncia, promete ou agenda nenhum recurso pago. A lista abaixo é ilustrativa e
> pode mudar; nada aqui cria compromisso comercial.

- **Licença do núcleo:** Apache 2.0 — ver [`LICENSE`](../../LICENSE).
- **Decisão confirmada:** D-02 (Apache 2.0 no núcleo + open-core).
- **Pendência:** P-03 em [`DECISIONS.md`](../../_bmad-output/planning-artifacts/DECISIONS.md)
  — prazo: antes da Fase 1.
- **Invariante de arquitetura:** AD-11 (open-core com núcleo OSS completo) e
  AD-30 (entitlements, Fase 1/3).

## Princípios

1. **O núcleo de observabilidade é sempre open source.** Ingestão, armazenamento,
   consulta, dashboards, alertas e correlação rodam no binário OSS sem licença.
2. **Segurança básica não é paywall.** TLS, autenticação, RBAC básico, redaction
   e auditoria mínima ficam no Apache 2.0.
3. **Nenhum caminho de telemetria é paywalled.** O dado do cliente entra, é
   armazenado e pode ser consultado sem entitlement.
4. **Nada de "aberto hoje, fechado amanhã".** Um recurso já publicado sob
   Apache 2.0 permanece Apache 2.0. Mudanças de fronteira exigem aviso público,
   justificativa e período de transição.
5. **O binário OSS funciona offline e sem servidor de licença.** Um eventual
   entitlement enterprise falha fechado apenas no recurso enterprise; nunca
   desliga a plataforma inteira.
6. **A comunidade participa.** Discussão pública de roadmap e de mudanças de
   fronteira antes da decisão.

## Sempre aberto (Apache 2.0)

| Área | O que está incluído |
|---|---|
| Ingestão | Gateway OTLP (HTTP/gRPC), receiver, validação, batching e WAL |
| Armazenamento | Schemas, retenção global, row policies e writers ClickHouse/VictoriaMetrics |
| Consulta | Query Service, linguagem de consulta, agregações e correlação |
| Experiência | Dashboards, Explore, catálogo de serviços, estados e acessibilidade |
| Alertas | Regras, avaliação, notificações e canais básicos |
| Segurança | Auth básico, RBAC básico, TLS, redaction e auditoria mínima |
| Interfaces | API `/api/v1`, OpenAPI, CLI e agente |
| Operação | Docker Compose single-node, Helm chart, backup/restore e auto-observabilidade |
| Documentação | Guias, ADRs, benchmarks e material de onboarding |

## Candidatos a enterprise — **PENDENTE DE DECISÃO**

A tabela abaixo replica os candidatos do addendum do PRD. **Todos os itens estão
pendentes** e sujeitos a análise de negócio, comunidade e arquitetura. Nenhum
item está comprometido, priorizado ou agendado; vários podem acabar no OSS.

| Candidato | O que poderia incluir | Por que é candidato | Status |
|---|---|---|---|
| SSO/SCIM | SAML/OIDC corporativo, provisionamento SCIM, sync de grupos | Integrações normalmente exigidas por enterprise e com custo de suporte | **Pendente — P-03** |
| Auditoria avançada | Retenção longa, exportação, trilha por tenant e integrações SIEM | Além da auditoria mínima necessária para operar com segurança | **Pendente — P-03** |
| RBAC granular | Papéis customizados, permissões por recurso e delegação | Complexidade de produto e suporte; o RBAC básico permanece OSS | **Pendente — P-03** |
| Retenção por tenant de métricas | Retenção e downsampling por projeto no VictoriaMetrics | Depende de recursos enterprise do storage e de custo operacional | **Pendente — P-03** |
| Federação multi-cluster | Consulta e correlação entre clusters/regiões | Escala e operação de grandes organizações; não é necessária no MVP | **Pendente — P-03** |
| Operator/air-gapped gerenciado | Operator Kubernetes, pacote air-gapped e gestão centralizada | Empacotamento e suporte operacional especializado | **Pendente — P-03** |
| Suporte/SLA | Suporte comercial, SLA e canal dedicado | Serviço, não funcionalidade; não altera o código OSS | **Pendente — P-03** |
| Oferta gerenciada | Observaki hospedado e operado pelo projeto/empresa | Modelo de distribuição, não uma feature do binário | **Pendente — P-03** |

### O que não será paywalled

- Ingestão, armazenamento, consulta e retenção **global** do núcleo.
- Dashboards, alertas, correlação e API pública.
- Auth básico, RBAC básico, TLS, redaction e auditoria mínima.
- O modo single-node e o Helm chart do núcleo.
- O agente e os instrumentos de coleta.
- Correções de segurança e de performance do núcleo.

## Como a decisão será tomada

1. O dono da decisão é o fundador (P-03), com input de arquitetura, produto e
   comunidade.
2. A decisão precisa estar registrada em
   [`DECISIONS.md`](../../_bmad-output/planning-artifacts/DECISIONS.md) e, se
   alterar invariantes, em um AD novo ou emenda a AD-11/AD-30.
3. Critério de reversão de P-03: se a comunidade reagir mal, manter segurança
   básica e observabilidade no OSS e reduzir a lista enterprise.
4. Enquanto P-03 não for decidida, nenhum recurso já aberto pode ser removido do
   OSS, bloqueado por entitlement ou usado em comunicação comercial como recurso
   disponível.

## Como dar feedback

Abra uma issue no repositório com o rótulo conceitual `open-core` (quando o
rótulo existir) ou use o template de feature, explicando:

- qual recurso você precisa e por quê;
- se ele é essencial para adotar o Observaki;
- qual seria o impacto de ele ser pago;
- se você contribuiria com código ou financiamento.

Feedback da comunidade é parte da decisão, não um voto vinculante.

## Referências

- [PRD — licenciamento e governança](../../_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md)
- [Addendum — governança open source](../../_bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md)
- [ARCHITECTURE-SPINE — AD-11 e AD-30](../../_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md)
- [DECISIONS — D-02 e P-03](../../_bmad-output/planning-artifacts/DECISIONS.md)
