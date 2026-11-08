# Runbooks de operação (Fase 0)

Procedimentos mínimos para rodar um piloto com segurança. Eles cobrem o escopo
da Story 5.4; backup de telemetria (ClickHouse/VictoriaMetrics), DR trimestral e
operações de produção completas continuam no AD-25 e nas fases seguintes.

| Runbook | Quando usar |
|---|---|
| [`backup-restore.md`](backup-restore.md) | antes de um upgrade, de uma migração de metadata ou de uma operação destrutiva |
| [`upgrade-rollback.md`](upgrade-rollback.md) | para promover uma nova imagem/chart e voltar atrás |
| [`auto-observabilidade.md`](auto-observabilidade.md) | para ler a página de saúde, configurar um alerta interno e silenciar |

## Evidência

- `make operator-smoke` — backup/restore, health overview e alerta/dedupe/silence
  no Compose (`deploy/compose/operator-smoke.sh`).
- `make upgrade-drill` — upgrade e rollback versionados no Compose
  (`deploy/compose/upgrade-drill.sh`).
- `make compose-smoke`, `make quickstart-check`, `make chart-smoke` — as stories
  5.1, 5.2 e 5.3 continuam válidas.

Os relatórios ficam em `deploy/compose/results/<UTC>/` e
`deploy/helm/results/<UTC>/`. Nenhum desses drills é evidência de G1.
