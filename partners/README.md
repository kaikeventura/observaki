# partners/ — dados do programa de design partners

Este diretório guarda apenas o **formato** do pipeline de recrutamento (Story 7.1). A documentação está em [`docs/partners/`](../docs/partners/README.md).

| Arquivo | Versionado | Conteúdo |
|---|---|---|
| `pipeline.template.csv` | sim | Cabeçalho do tracker, sem linhas |
| `examples/pipeline-example.csv` | sim | 20 candidatos **fictícios** (`*.example.com`) que exercitam o validador |
| `pipeline.csv` | **não** (`.gitignore`) | Lista real, com dados de contato |
| `pipeline-report.md` | sim, quando gerado | Contagens de funil redigidas, sem PII |

```bash
make partners-init    # cria pipeline.csv a partir do template
make partners-check   # valida template + exemplo (roda no make lint/CI)
make partners-gate    # exige AC1/AC3 na lista real e grava o relatório redigido
```

Não commite contatos reais. O validador rejeita linha sem contato usável, status fora do funil, data fora de ordem e id duplicado.
