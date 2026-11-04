# deploy/ — artefatos de implantação

- [`compose/`](compose/README.md) — stack single-node com Docker Compose
  (Story 5.1): Dockerfile multi-stage, configuração do ClickHouse/NATS, snapshot
  de auth de desenvolvimento, script de migração, sample app e smoke de AC1/AC3.
  O `compose.yaml` fica na raiz do repositório para `docker compose up` funcionar
  sem `-f`.
- Helm chart alpha (Story 5.2), jobs de backup/restore e auto-observabilidade
  (Story 5.4) ainda não existem.

O cluster de **benchmark** T1 (Story 2.4) não é o Compose do produto: ele vive
em [`bench/env/`](../bench/env/README.md) e provisiona apenas hosts, rede e
discos para medir a stack.

- Escopo e critérios: `_bmad-output/planning-artifacts/epics.md` (Epic P0-E5).
- O ambiente de desenvolvimento desta fase (`make dev`) **não** sobe
  ClickHouse, VictoriaMetrics, PostgreSQL ou NATS; use
  `make compose-up` para a stack completa.

Segredos não entram no repositório: use `.env` (ignorado) ou variáveis do
ambiente do cluster. O token de `deploy/compose/config/ingest-auth.json` é
público de propósito e só vale para a stack local.
