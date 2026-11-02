# deploy/ — artefatos de implantação

Placeholder. Docker Compose, Helm chart, jobs de backup/restore e
auto-observabilidade são criados nas Stories 5.1 a 5.4 (Fase 0).

O cluster de **benchmark** T1 (Story 2.4) não é o Compose do produto: ele vive
em [`bench/env/`](../bench/env/README.md) e provisiona apenas hosts, rede e
discos para medir a stack.

- Escopo e critérios: `_bmad-output/planning-artifacts/epics.md` (Epic P0-E5).
- O ambiente de desenvolvimento desta fase (`make dev`) **não** sobe
  ClickHouse, VictoriaMetrics, PostgreSQL ou NATS; isso é a Story 5.1.

Segredos não entram no repositório: use `.env` (ignorado) ou variáveis do
ambiente do cluster.
