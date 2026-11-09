# Pipeline de design partners (Story 7.1)

O tracker é um CSV de uma linha por candidato. Fonte de verdade local: `partners/pipeline.csv` (não versionado, contém contatos). O validador `cmd/partnerscheck` impõe o contrato e falha quando um estado de funil não tem a evidência correspondente.

## Funil e evidência obrigatória

| Status | Significado | Evidência exigida |
|---|---|---|
| `sourced` | Identificado, ainda não contatado | nenhuma data (só `next_action`) |
| `contacted` | Primeiro contato enviado | `first_contact_at`, `last_contact_at` |
| `replied` | Candidato respondeu | `reply_at` |
| `call_scheduled` | Call marcada | `reply_at`, `call_at` (data futura permitida) |
| `call_done` | Call feita, decisão pendente | `reply_at`, `call_at` |
| `signed` | Acordo assinado | `reply_at`, `call_at`, `decision_at`, `signed_at` |
| `declined` | Recusou | `reply_at`, `call_at`, `decision_at` (sem `signed_at`) |
| `no_response` | 3 tentativas, sem resposta | `first_contact_at`, `last_contact_at` |

Datas em `YYYY-MM-DD` e em ordem: `first_contact_at` → `reply_at` → `call_at` → `decision_at` → `signed_at`. O validador rejeita fora de ordem, status fora do funil, id duplicado e linha sem contato usável.

## Campos

| Campo | Regra |
|---|---|
| `id` | `dp-NNN`, único e imutável |
| `company` / `website` | Nome e site da empresa |
| `segment` | `startup`, `scaleup`, `platform-team` ou `other` |
| `contact_name` / `contact_role` | Pessoa de contato e cargo (obrigatório) |
| `contact_email` | E-mail válido **ou** um canal alternativo abaixo |
| `contact_channel` | `email`, `linkedin`, `slack`, `community`, `referral`, `other` |
| `stack` | Sinais e ferramentas atuais (`Go, EKS, OTel Collector, Datadog`) |
| `uses_otel` / `uses_kubernetes` | `yes`, `no`, `planned`, `unknown` |
| `volume_events_per_second` | Inteiro ≥ 0 ou `unknown` (a triagem da 7.2 exige ≥ 10000) |
| `can_share_data` / `can_weekly_feedback` | `yes`, `no`, `unknown` (Story 7.2; só `yes` passa na triagem) |
| `status` | Estágio do funil acima |
| `next_action` / `next_action_due` | Próximo passo e data (sempre preenchidos) |
| `notes` | Contexto curto; sem dado pessoal sensível |

Um candidato é **qualificado** (AC1 da 7.1) quando tem contato usável, stack, volume declarado, `uses_otel` em `yes`/`planned` e `uses_kubernetes` em `yes`/`planned`. A **triagem completa** (AC1 da 7.2) olha os cinco critérios e só deixa assinar quem tem `uses_otel=yes`, `uses_kubernetes=yes`, volume ≥ 10000, `can_share_data=yes` e `can_weekly_feedback=yes` — ver [`selection.md`](selection.md).

## Cadência

| Quando | O que fazer |
|---|---|
| Segunda | Atualizar o tracker: enviados, respostas, calls, decisões; repor a cota de `sourced` |
| Quarta | Primeiro contato com os novos qualificados |
| Sexta | Revisar números da semana e destravar follow-ups vencidos |

**SLAs:** primeiro follow-up em 5 dias úteis; segunda tentativa em 12 dias; encerrar como `no_response` em 14 dias ou 3 tentativas.

## Métricas semanais

Saída de `make partners-gate` (relatório redigido):

- **Cobertura:** qualificados na lista (meta AC1 ≥ 20).
- **Atividade:** contatados na semana, taxa de resposta (`replied / contacted`).
- **Conversão:** calls feitas (`call_done + signed + declined`), taxa de call (`calls / replied`), taxa de assinatura (`signed / calls`).
- **Meta AC3:** ≥ 5 calls e ≥ 3 assinados até o fim da Fase 0.

## Mapa AC → artefato

| AC | Como é satisfeito | Evidência |
|---|---|---|
| AC1 — lista (7.1) | `partners/pipeline.csv` com ≥ 20 qualificados, validado pelo `partnerscheck` | `make partners-gate` (relatório redigido) |
| AC2 — outreach (7.1) | Templates em [`outreach.md`](outreach.md) com valor, compromisso e tratamento de dados; datas de resposta/call/decisão no tracker | Exemplo versionado + relatório |
| AC3 — meta (7.1) | 5 calls e 3 assinados | `make partners-gate` |
| AC1 — critérios (7.2) | cinco critérios em [`selection.md`](selection.md); `signed` sem triagem falha o validador | linha `screen:` do gate + seção *Selection screen* no relatório |
| AC2 — acordo (7.2) | [`agreement.md`](agreement.md) espelhando [`data-handling.md`](data-handling.md) | teste `TestAgreementMatchesTheOutreachSummary` |
| AC3 — baseline (7.2) | [`baseline.md`](baseline.md) + [`partners/baseline.template.md`](../../partners/baseline.template.md) | teste `TestBaselineTemplateCoversAC3` |

## Comandos

```bash
make partners-init                                   # cria a lista local
make partners-check                                  # template + exemplo fictício (CI)
make partners-gate                                   # AC1/AC3 na lista real + relatório
make partners-baseline ID=dp-001                     # cria partners/baselines/dp-001.md (local)
go run ./cmd/partnerscheck -file partners/pipeline.csv -min-candidates 20 -min-calls 5 -min-signed 3
go run ./cmd/partnerscheck -file partners/pipeline.csv -min-volume 50000   # cenário mais exigente
```
