# Outreach (Story 7.1, AC2)

Regras: ≤ 150 palavras, um pedido claro, sem anexo e sem promessa de feature. Todo contato explica **valor**, **compromisso** e **tratamento de dados**. Nada de “revolucionário”, “AI magic” ou benchmark inventado.

## Template A — e-mail (PT-BR)

**Assunto:** 30 min sobre observabilidade self-hosted — piloto com 3 times

> Olá {nome},
>
> Sou o Kaike e estou construindo o Observaki: uma plataforma de observabilidade open source (Apache 2.0), self-hosted e OTel-native — métricas, logs e traces no mesmo lugar, sem tirar dado do seu cluster.
>
> Vi que o time de {empresa} roda {stack}. Estou selecionando 3 times para um piloto de 8 semanas e queria te convidar.
>
> **O que peço:** 1 call de 30 min por semana durante 8 semanas; telemetria de um ambiente não crítico via OTel Collector (o volume você escolhe); feedback honesto, inclusive quando algo não funcionar.
>
> **O que você recebe:** acesso antecipado, linha direta comigo, influência no roadmap da Fase 1 e os números do benchmark na escala do seu time. Sem lock-in: Apache 2.0, rodando na sua infraestrutura.
>
> **Sobre dados:** a stack fica no seu cluster e eu não tenho acesso por padrão. Retenção padrão de 7 dias no piloto, ajustável. Amostras compartilhadas saem anonimizadas e só usamos números agregados em material público. Você sai quando quiser: revoga o token, derruba a stack e apaga os volumes.
>
> Sendo transparente: é alpha, sem SLA de produção e sem paridade com Datadog. O piloto é para ambiente não crítico.
>
> Faz sentido conversar 30 min? Se sim, me manda dois horários.

## Template B — e-mail (EN)

**Subject:** 30 min on self-hosted observability — 8-week pilot with 3 teams

> Hi {name},
>
> I'm Kaike, building Observaki: an open source (Apache 2.0), self-hosted, OTel-native observability platform for metrics, logs and traces — your data never leaves your cluster.
>
> I saw {company} runs {stack}. I'm selecting 3 teams for an 8-week pilot and wanted to invite you.
>
> **What I ask:** one 30-min call per week for 8 weeks; telemetry from a non-critical environment through the OTel Collector (volume is your call); honest feedback, including when it breaks.
>
> **What you get:** early access, a direct line to me, influence over the Phase 1 roadmap, and benchmark numbers at your scale. No lock-in: Apache 2.0, running in your own infrastructure.
>
> **Data handling:** the stack stays in your cluster and I have no access by default. Default retention is 7 days during the pilot, adjustable. Shared samples are anonymized and only aggregated numbers appear publicly. You can leave anytime by revoking the token, tearing down the stack and deleting the volumes.
>
> To be clear: this is alpha, with no production SLA and no Datadog parity. The pilot targets a non-critical environment.
>
> Worth 30 minutes? Send me two slots and I'll book it.

## LinkedIn / Slack (curto)

> Olá {nome}, tudo bem? Construo o Observaki, uma alternativa open source e self-hosted para métricas, logs e traces via OTel. Estou montando um piloto de 8 semanas com 3 times (1 call de 30 min por semana, dados de ambiente não crítico, retenção de 7 dias, saída livre). Topa uma conversa de 30 min?

## Follow-ups

| Quando | Mensagem |
|---|---|
| D+5 | “Passando para saber se faz sentido. Posso mandar o resultado do benchmark local (1M séries/s) e o quickstart de 15 min, se ajudar a avaliar.” |
| D+12 | “Última tentativa por aqui. Se a call não couber agora, posso mandar 5 perguntas por escrito e você responde quando der — o piloto pode começar depois.” |
| D+14 | Encerrar: mover para `no_response` com `last_contact_at`; sem novo contato por 90 dias. |

## Roteiro da call (30 min)

| Tempo | Bloco |
|---|---|
| 0–5 | Contexto: quem está na call, o que rodam hoje, tamanho do time |
| 5–10 | Dor: custo, MTTR, ferramentas espalhadas, o que já tentaram |
| 10–20 | Demo honesta: stack single-node, ingestão OTLP e a tela de Explore (mock na Fase 0) |
| 20–25 | Compromisso: 8 semanas, 30 min/semana, dados não críticos, retenção, saída |
| 25–30 | Próximo passo: acordo (Story 7.2) e onboarding (Story 7.3), com data |

**Perguntas de descoberta:** qual o volume de eventos/s? Qual a retenção atual? Quanto custa hoje? Qual o MTTR de um incidente típico? Quem usa a ferramenta no dia a dia? O que faria você trocar?

## Objeções

| Objeção | Resposta |
|---|---|
| “Dado não pode sair.” | Não sai: é self-hosted, a stack roda no seu cluster e eu não tenho acesso por padrão. |
| “É maduro?” | Não é GA — é alpha, para ambiente não crítico, com 8 semanas e saída livre. É justamente por isso que o feedback vale. |
| “Já usamos Datadog.” | Não peço troca. Rodo o mesmo volume no benchmark e você compara custo e fluxo de diagnóstico. |
| “Sem tempo.” | 30 min por semana, ou feedback assíncrono por escrito; o formato é seu. |
| “Quanto custa?” | O piloto é gratuito e o núcleo é Apache 2.0. Sem lock-in; open-core vem depois. |
| “Precisa de SSO/HIPAA?” | Fora do escopo da Fase 1. Registro como requisito e a gente conversa sobre a Fase 2/3. |

**Ao final de toda call:** atualizar `status`, `call_at` ou `decision_at` e `next_action`/`next_action_due` no tracker. Sem isso, a call não conta como evidência.
