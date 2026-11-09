# Sourcing de design partners

**ICP:** startups e times pequenos (3–50 engenheiros), com Kubernetes, OpenTelemetry adotado ou planejado, volume ≥ 10k eventos/s e abertura para compartilhar dados não críticos e dar feedback semanal.

## Onde procurar

| Canal | Sinal que vale qualificar | Esforço |
|---|---|---|
| Rede do fundador e indicações | Ex-colegas em times de plataforma; melhor taxa de resposta | Baixo |
| CNCF Slack (`#opentelemetry`, `#kubernetes-users`, `#prometheus`) | Quem pede ajuda com Collector, cardinalidade ou custo | Baixo |
| Meetups de Kubernetes/DevOps (São Paulo e remotos) | Times migrando de APM proprietário | Médio |
| LinkedIn | Cargos `Head of Platform`, `SRE Manager`, `Staff SRE` em empresas de 20–200 pessoas | Médio |
| Comunidades BR (Telegram/Slack de DevOps) | Relatos de custo de Datadog/New Relic | Baixo |
| Job posts | Vaga de observabilidade com “migrar”, “reduzir custo”, “OTel” | Baixo |
| Aceleradoras e comunidades de startups | Startups em série A/B com time de plataforma próprio | Médio |

Não compre listas, não faça scraping de e-mails e não use contato pessoal sem contexto profissional. Cada linha precisa de uma razão concreta para o convite.

## Cota semanal

| Etapa | Meta por semana |
|---|---|
| Novos `sourced` qualificados | 10 |
| Primeiros contatos (`contacted`) | 5 |
| Follow-ups | todos os vencidos |
| Calls | 2 |
| Atualizar decisões e `next_action` | todas as linhas |

Com 20 qualificados e taxa de resposta realista (30–50%), a meta de 5 calls e 3 assinaturas exige repor a lista toda semana — não pare em 20 linhas.

## Pré-triagem (antes de gastar uma call)

Responda com o que já é público ou com uma pergunta de uma linha:

1. Usa OpenTelemetry (ou tem plano com data)?
2. Roda Kubernetes (EKS, GKE, k3s, OpenShift)?
3. Qual o volume aproximado de eventos/s hoje?
4. Pode enviar telemetria de um ambiente não crítico?
5. Quem participa de uma call de 30 min por semana durante 8 semanas?

Marque `uses_otel`, `uses_kubernetes` e `volume_events_per_second` no tracker mesmo quando a resposta for `unknown` — o campo vazio é o que o validador rejeita.

## Desqualificação rápida

- Sem Kubernetes e sem plano de adoção.
- Sem OTel e sem intenção de adotar (o produto é OTel-native).
- Volume irrelevante (< 1k eventos/s) e sem outro sinal forte de dor.
- Compras/procurement obrigatório para um piloto de 8 semanas.
- Exige SSO/SCIM, HIPAA ou residência de dados fora do escopo da Fase 1.
- Só quer desconto em ferramenta existente, sem trocar de stack.

Registre a recusa em `notes` e mova para `declined` — o motivo alimenta a priorização da Story 7.4.

## Ritual

- **Segunda:** 10 novos `sourced`, com stack e volume já preenchidos.
- **Quarta:** 5 primeiros contatos usando [`outreach.md`](outreach.md).
- **Sexta:** follow-ups vencidos e atualização do relatório (`make partners-gate`).
