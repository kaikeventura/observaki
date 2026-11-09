# partners/ — dados do programa de design partners

Este diretório guarda o **formato** do pipeline (Story 7.1) e do baseline (Story 7.2). A documentação está em [`docs/partners/`](../docs/partners/README.md).

| Arquivo | Versionado | Conteúdo |
|---|---|---|
| `pipeline.template.csv` | sim | Cabeçalho do tracker (25 colunas), sem linhas |
| `baseline.template.md` | sim | Formulário de baseline, sem dados de parceiro |
| `examples/pipeline-example.csv` | sim | 20 candidatos **fictícios** (`*.example.com`) que exercitam o validador |
| `pipeline.csv` | **não** (`.gitignore`) | Lista real, com dados de contato |
| `pipeline-report.md` | sim, quando gerado | Contagens de funil e triagem redigidas, sem PII |
| `baselines/<id>.md` | **não** (`.gitignore`) | Baseline preenchido do parceiro |
| `agreements/` | **não** (`.gitignore`) | Cópias assinadas/aceites do acordo |

```bash
make partners-init                  # cria pipeline.csv a partir do template
make partners-check                 # valida template + exemplo (roda no make lint/CI)
make partners-gate                  # exige AC1/AC3 na lista real e grava o relatório redigido
make partners-baseline ID=dp-001    # cria baselines/dp-001.md a partir do template
```

Não commite contatos reais. O validador rejeita linha sem contato usável, status fora do funil, data fora de ordem, id duplicado e parceiro `signed` que falhe um dos cinco critérios da Story 7.2.
