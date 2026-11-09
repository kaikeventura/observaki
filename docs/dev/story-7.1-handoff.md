# Story 7.1 — Pipeline de recrutamento de design partners

**Epic:** P0-E7 — Design partners e validação inicial
**Data:** 2026-11-08
**Status:** `in_progress` — infraestrutura entregue; AC1 (lista real) e AC3 (5 calls, 3 assinados) dependem de execução humana (P-01).

## Acceptance criteria

| AC | Resultado | Evidência |
|---|---|---|
| AC1 — ≥ 20 candidatos qualificados com contato, stack, volume e status | **Parcial**: contrato do tracker, validador e 20 linhas fictícias que passam os três checks; a lista real de 20 qualificados ainda não existe | `make partners-check` (example: 20/20) |
| AC2 — outreach com valor, compromisso e tratamento de dados; pipeline rastreia resposta, call e decisão | **Feito**: templates PT-BR/EN, DM, follow-ups, roteiro de call e objeções; o tracker exige `reply_at`/`call_at`/`decision_at` por estágio | `docs/partners/outreach.md`, `internal/partners` |
| AC3 — ≥ 5 calls e 3 partners assinados | **Pendente**: só o exemplo fictício atinge 8 calls e 3 assinaturas | `make partners-gate` na lista real |

## O que foi entregue

1. **Tracker CSV com contrato executável** (`partners/pipeline.template.csv`): 23 colunas cobrindo contato, stack, adoção de OTel/Kubernetes, volume, origem, estágio do funil e as datas de resposta, call, decisão e assinatura.
2. **Validador Go** (`internal/partners`, `cmd/partnerscheck`): parse do CSV, validação por linha, coerência de funil (um estágio só existe com a evidência correspondente), ordem cronológica, ids únicos, contato usável, volume numérico ou `unknown`, e avaliação de AC1/AC3 (`-min-candidates`, `-min-calls`, `-min-signed`).
3. **Relatório redigido** (`partners/pipeline-report.md`): apenas contagens de funil e taxas; nenhum nome, e-mail ou nota. É o artefato que pode ser commitado.
4. **Exemplo fictício** (`partners/examples/pipeline-example.csv`): 20 linhas com domínios `*.example.com` e nomes inventados, cobrindo todos os oito estágios, para exercitar o validador no CI.
5. **Privacidade por design**: `partners/pipeline.csv` está no `.gitignore`; contatos reais nunca entram no repositório.
6. **Documentação**: `docs/partners/` com funil/campos/SLAs, sourcing por canal com cota semanal, outreach PT-BR/EN com follow-ups e roteiro de call, e o resumo de tratamento de dados que o acordo da Story 7.2 precisa espelhar.
7. **Automação**: `make partners-check` no `make lint` (portanto no CI), `make partners-init` e `make partners-gate`.

## Evidência

```text
$ make partners-check
Partners pipeline: partners/pipeline.template.csv
rows: 0 | qualified: 0 | ...
empty pipeline accepted (template only)

Partners pipeline: partners/examples/pipeline-example.csv
rows: 20 | qualified: 20 | signed: 3 | declined: 2 | no_response: 1
funnel: sourced=2 contacted=3 replied=4 call_scheduled=2 call_done=3 signed=3 declined=2 no_response=1
rates: reply 78% | call 57% | sign 38%
AC1 qualified candidates: PASS (20/20)
AC3 calls happened: PASS (8/5)
AC3 partners signed: PASS (3/3)
```

Caminhos negativos exercitados:

```text
$ go run ./cmd/partnerscheck -file /tmp/partners-ten.csv     # 10 linhas
AC1 qualified candidates: FAIL (10/20)
partnerscheck: 1 acceptance check(s) failed                  # exit 1

$ go run ./cmd/partnerscheck -file /tmp/partners-broken.csv  # contato removido
partners-broken.csv:12: dp-011: contact_name is empty (AC1 requires a contact)
partners-broken.csv:12: dp-011: no usable contact: ...       # exit 1
```

Testes: `go test ./...` verde, incluindo `internal/partners` (validação por linha, funil, taxas, duplicidade de id, ICP e ausência de PII no relatório). `make fmt`, `make lint`, `make build` (bundle 41,5 KB / 74,4 KB, dentro do budget) e `SMOKE_SKIP_CI=1 make smoke` verdes.

## Decisões tomadas nesta story

- **Contatos fora do git.** O tracker real é local e ignorado; só o relatório agregado é versionado. O repo é Apache 2.0 e pode virar público — e-mail de pessoa é PII, não metadado de projeto.
- **CSV em vez de planilha proprietária.** Editável em qualquer ferramenta, diffável e validável no CI sem dependência nova.
- **Coerência de funil é regra, não convenção.** `replied` sem `reply_at`, `signed` sem `signed_at` ou `call_at` antes de `reply_at` falham o `make lint`. Isso impede inflar a meta sem evidência.
- **Qualificação da 7.1 é só o pré-filtro.** O validador exige OTel e Kubernetes em `yes`/`planned` e volume declarado; os cinco critérios completos e o acordo continuam na Story 7.2.
- **`unknown` é valor válido para volume.** Melhor registrar a incerteza do que inventar número; a 7.2 filtra o ≥ 10k eventos/s na call.

## O que falta para fechar a story

1. **AC1:** rodar `make partners-init`, preencher ≥ 20 candidatos reais com stack, volume e contato (usar [`docs/partners/sourcing.md`](../partners/sourcing.md)), e rodar `make partners-gate` até AC1 passar.
2. **AC2:** enviar o outreach usando [`docs/partners/outreach.md`](../partners/outreach.md) e registrar `reply_at`, `call_at` e `decision_at` no tracker (o validador rejeita estágio sem evidência).
3. **AC3:** chegar a ≥ 5 calls feitas e 3 assinaturas; o `make partners-gate` passa a sair 0.
4. **Commitar** o `partners/pipeline-report.md` gerado (sem PII) como evidência do gate e atualizar `STATUS.md`.
5. **P-01** em `DECISIONS.md` continua aberta até os 3 nomes estarem assinados.

## Riscos

- **Pipeline vazio é o risco real do gate G1.** 20 qualificados com taxa de resposta de 30–50% e conversão de call para assinatura de ~30% produzem perto de 3 parceiros; repor a lista semanalmente não é opcional.
- **Promessa de produto.** O outreach fala em alpha, sem SLA e sem paridade com Datadog. Qualquer promessa maior precisa de decisão de produto antes do contato.
- **Divergência entre resumo de dados e acordo.** `docs/partners/data-handling.md` precisa ser a base do acordo da 7.2; se a 7.2 mudar retenção ou anonimização, o outreach é corrigido antes do próximo convite.
- **PII em anexos e issues.** Relatório redigido é o único artefato público; capturas de tela do tracker também contam como PII.
