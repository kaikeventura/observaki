# Design partners — programa da Fase 0

**Objetivo (Epic P0-E7):** terminar a Fase 0 com **3 design partners comprometidos**, com dados reais e um ciclo de feedback semanal — não apenas conversas interessadas.

**Estado:** infraestrutura do pipeline (7.1) e os artefatos de seleção, acordo e baseline (7.2) estão prontos. A lista real de candidatos, as calls, as assinaturas e a captura de baseline dependem de execução humana (P-01 do `DECISIONS.md`); a meta de 5 calls e 3 assinaturas ainda não foi atingida.

## O que existe

| Artefato | O que é |
|---|---|
| [`pipeline.md`](pipeline.md) | Funil, campos do tracker, regras de atualização, SLAs e métricas semanais |
| [`sourcing.md`](sourcing.md) | Onde encontrar candidatos do ICP, cota semanal e pré-triagem |
| [`selection.md`](selection.md) | Os cinco critérios de seleção, quando triar, desqualificação e desempate |
| [`outreach.md`](outreach.md) | Templates de primeiro contato, follow-ups e roteiro da call (PT-BR e EN) |
| [`data-handling.md`](data-handling.md) | Resumo de tratamento de dados usado no outreach |
| [`agreement.md`](agreement.md) | Acordo do piloto: escopo, retenção, anonimização, cadência e saída |
| [`baseline.md`](baseline.md) | Como capturar ferramentas, volume, custo e as 3 dores do ambiente |
| [`partners/pipeline.template.csv`](../../partners/pipeline.template.csv) | Cabeçalho do tracker (versionado) |
| [`partners/baseline.template.md`](../../partners/baseline.template.md) | Formulário de baseline (versionado; a cópia preenchida fica local) |
| [`partners/examples/pipeline-example.csv`](../../partners/examples/pipeline-example.csv) | 20 linhas **fictícias** que exercitam o validador |
| [`docs/dev/story-7.1-handoff.md`](../dev/story-7.1-handoff.md) | Evidência e decisões do pipeline |
| [`docs/dev/story-7.2-handoff.md`](../dev/story-7.2-handoff.md) | Evidência e decisões de seleção, acordo e baseline |

## Comandos

```bash
make partners-init                 # cria partners/pipeline.csv a partir do template (não versionado)
make partners-check                # valida template + exemplo fictício (roda no make lint/CI)
make partners-gate                 # exige 20 qualificados, 5 calls e 3 assinados na lista real
make partners-baseline ID=dp-001   # cria partners/baselines/dp-001.md a partir do template
```

`make partners-gate` grava `partners/pipeline-report.md` — contagens de funil e da triagem, sem contato, e-mail ou nota. É o único artefato do pipeline que pode ser commitado quando os dados forem reais.

## Regra de privacidade (vinculante)

O tracker real (`partners/pipeline.csv`), as cópias do acordo (`partners/agreements/`) e os baselines preenchidos (`partners/baselines/`) contêm **dados do parceiro** e estão no `.gitignore`. Nunca commite e-mails, nomes, telefones, faturas ou notas de candidatos — nem em docs, issues ou mensagens de commit. O relatório redigido é a evidência pública. O teste `TestPartnerDocsCarryNoContacts` falha se um endereço de e-mail aparecer em `docs/partners/`.

## Acordo de compromisso

O que pedimos ao parceiro, já no primeiro contato (Story 7.1, AC2) e formalizado no [`agreement.md`](agreement.md) (Story 7.2, AC2):

- **8 semanas de piloto** alinhadas à Fase 0, com **1 call de 30 min por semana**.
- Ingestão de telemetria de um ambiente **não crítico** via OTel Collector, com volume declarado.
- Feedback escrito ou na call, com permissão para usar **números anonimizados** no benchmark público.
- Saída a qualquer momento: revogar o token, remover a stack e apagar os dados.

O que o parceiro recebe: acesso antecipado, linha direta com o fundador, influência no roadmap da Fase 1, software self-hosted Apache 2.0 sem lock-in e leitura do benchmark na escala dele.

**Não prometemos** SLA de produção, GA nem paridade com Datadog. O piloto roda com SLA reduzido e dados não críticos.
