# Story 7.2 — Critérios de seleção, acordo e tratamento de dados

**Epic:** P0-E7 — Design partners e validação inicial
**Data:** 2026-11-08
**Status:** `in_progress` — artefatos e verificação executável entregues; assinatura do acordo e captura do baseline dependem de execução humana (P-01).

## Acceptance criteria

| AC | Resultado | Evidência |
|---|---|---|
| AC1 — Critérios | **Feito**: os cinco critérios (OTel, Kubernetes, ≥ 10k eventos/s, compartilhar dados, feedback semanal) viraram campos do tracker e uma triagem no validador; `signed` que falhe um critério falha o `make lint` | linha `screen:` do `partnerscheck`, seção *Selection screen* no relatório, `TestScreenRequiresAllFiveCriteria`, `TestSigningRequiresTheScreen` |
| AC2 — Acordo | **Parcial**: template com escopo, retenção, anonimização, cadência e saída, espelhando `data-handling.md` e com guarda de teste contra divergência; falta a assinatura dos 3 parceiros (P-01) | `docs/partners/agreement.md`, `TestAgreementCoversTheStory72Clauses`, `TestAgreementMatchesTheOutreachSummary` |
| AC3 — Baseline | **Parcial**: procedimento, formulário versionado e alvo `make partners-baseline ID=dp-001`; falta capturar o baseline real no onboarding (Story 7.3, P-01) | `docs/partners/baseline.md`, `partners/baseline.template.md`, `TestBaselineTemplateCoversAC3` |

## O que foi entregue

1. **Triagem executável (AC1)** — `internal/partners/selection.go`: `Criteria` (limiar de volume, padrão 10 000 eventos/s), `Screening` por candidato, `Screened` agregado por critério e `ScreenSummary` para o console. O validador agora rejeita `status=signed` que não passe nos cinco critérios, nomeando o que falta; `declined` e `call_done` continuam livres para reprovar (recusar é resultado normal do funil).
2. **Tracker com 25 colunas** — `can_share_data` e `can_weekly_feedback` (`yes`/`no`/`unknown`) entram no template, no exemplo fictício e em `RequiredColumns`. Um `partners/pipeline.csv` criado antes desta story precisa das duas colunas (ver *Migração* abaixo).
3. **Relatório redigido com a triagem** — nova seção *Selection screen* (elegíveis, bloqueados e falhas por critério) sem nome, empresa, contato ou nota; o restante do relatório da 7.1 ficou igual.
4. **Acordo do piloto (AC2)** — `docs/partners/agreement.md` em 9 cláusulas: escopo, duração/compromisso, tratamento de dados (retenção 7 dias, acesso, minimização, redação, anonimização, material público), cadência de feedback, baseline, propriedade/licença, saída, garantias e assinatura.
5. **Baseline do ambiente (AC3)** — `docs/partners/baseline.md` (o que capturar, quando, como medir, o que é público) e `partners/baseline.template.md` (ferramentas atuais, volume, custo, top 3 dores, expectativa); a cópia preenchida vai para `partners/baselines/<id>.md`, fora do git.
6. **Guardas de documentação** — `internal/partners/docs_test.go` falha quando o acordo deixa de cobrir uma cláusula, quando acordo/outreach/resumo divergem em retenção (7 dias), duração (8 semanas) ou cadência (30 min), quando `selection.md` cita um campo que o validador não exige, quando o template de baseline perde um bloco do AC3 ou quando aparece e-mail em `docs/partners/`.
7. **Privacidade** — `partners/baselines/` e `partners/agreements/` no `.gitignore`; só o relatório redigido é commitável.

## Evidência

```text
$ make partners-check
Partners pipeline: partners/pipeline.template.csv
rows: 0 | qualified: 0 | signed: 0 | declined: 0 | no_response: 0
screen: evaluated=0 eligible=0 blocked=0 min_volume=10000 | failing: uses_otel=0 ...
empty pipeline accepted (template only)

Partners pipeline: partners/examples/pipeline-example.csv
rows: 20 | qualified: 20 | signed: 3 | declined: 2 | no_response: 1
funnel: sourced=2 contacted=3 replied=4 call_scheduled=2 call_done=3 signed=3 declined=2 no_response=1
rates: reply 78% | call 57% | sign 38%
screen: evaluated=20 eligible=9 blocked=11 min_volume=10000 | failing: uses_otel=0 uses_kubernetes=2
        volume_events_per_second=7 can_share_data=5 can_weekly_feedback=7
AC1 qualified candidates: PASS (20/20)
AC3 calls happened: PASS (8/5)
AC3 partners signed: PASS (3/3)
```

Caminho negativo da triagem (parceiro assinado que não pode compartilhar dados):

```text
test.csv:2: dp-100: status "signed" requires the five Story 7.2 criteria; missing: can_share_data=yes
```

Baseline:

```text
$ make partners-baseline ID=dp-999
partners/baselines/dp-999.md ready (untracked; fill it in at onboarding start)
$ make partners-baseline ID=bad
ID must look like dp-001        # exit 1
```

Checks: `make fmt`, `make lint` (ADR/OpenAPI/partners + golangci 0 issues + prettier + svelte-check + clippy), `make test` (Go incl. `internal/partners`, web 116 testes, Rust 2), `make build` (bundle 41,5 KB / 74,4 KB) e `SMOKE_SKIP_CI=1 make smoke` verdes.

## Decisões tomadas nesta story

- **O critério vive no código, não no documento.** `can_share_data`/`can_weekly_feedback` são campos do tracker e a regra `signed ⇒ cinco critérios` é validação dura. Assim o AC1 não depende de memória na hora de assinar.
- **`planned` não passa na triagem.** A 7.1 aceita `planned` para pré-filtro; no piloto, "vamos adotar Kubernetes" não paga o custo de onboarding. `unknown` continua válido para registrar incerteza, mas não assina.
- **Reprovar não é erro antes da assinatura.** Só `signed` é bloqueado; `call_done` e `declined` podem falhar a triagem, porque é isso que a call existe para descobrir.
- **Limiar configurável.** `-min-volume` permite rodar cenários (ex.: piloto só com ≥ 50k/s) sem mudar código; o padrão de 10k/s vem do PRD.
- **O acordo vence o resumo.** `agreement.md` é a fonte; `data-handling.md` e o outreach precisam concordar com ele — e um teste falha quando divergem, fechando o risco registrado no handoff da 7.1.
- **Baseline fora do git.** Fatura e inventário de ferramentas são dados do parceiro; o template é versionado, a cópia preenchida não.

## Migração de um pipeline existente

Um `partners/pipeline.csv` criado na 7.1 não tem as duas colunas novas e o validador vai recusar o arquivo. Antes de rodar o gate:

```bash
# opção 1: recriar a partir do template e recolar as linhas
make partners-init   # se o arquivo ainda não existir
# opção 2: adicionar as colunas no fim do CSV, preenchendo yes/no/unknown
```

As colunas podem ficar em qualquer ordem (o parser usa o cabeçalho), mas precisam existir e estar preenchidas com `yes`, `no` ou `unknown`.

## O que falta para fechar a story

1. **AC2:** apresentar [`agreement.md`](../partners/agreement.md) aos candidatos que passarem na triagem, coletar assinatura/aceite e guardar a cópia em `partners/agreements/` (fora do git).
2. **AC1 real:** preencher `can_share_data` e `can_weekly_feedback` dos candidatos reais e conferir a linha `screen:` do `make partners-gate`.
3. **AC3:** na primeira call de onboarding, rodar `make partners-baseline ID=dp-00N` e preencher ferramentas, volume, custo e as 3 dores (Story 7.3 consome esse arquivo).
4. **P-01** em `DECISIONS.md` só fecha com os 3 acordos assinados.

## Riscos

- **Triagem tardia.** Se os campos só forem preenchidos depois da call, o parceiro pode ser recusado por algo que a pré-triagem pegaria de graça; a [`sourcing.md`](../partners/sourcing.md) já pede as cinco respostas antes de gastar a call.
- **Volume declarado ≠ volume real.** O baseline é o que confirma; se o parceiro ficar abaixo de 10k/s, o piloto continua válido para produto, mas não serve como evidência de escala no G1.
- **Acordo e resumo divergindo de novo.** Qualquer mudança de retenção (P-02) precisa entrar nos dois documentos na mesma sessão, senão o teste `TestAgreementMatchesTheOutreachSummary` falha o `make test`.
- **Assinatura sem leitura.** O bloco de assinatura pede confirmação explícita das cláusulas 1–8; aceite por e-mail vale, mas precisa citar retenção e saída.
