# Upgrade e rollback

**AC2 da Story 5.4:** o procedimento é reproduzível e o rollback volta à versão
anterior em menos de 15 minutos.

## Princípios

- A imagem é única (`serve`, `gateway`, `writer`, `migrate`, `backup`,
  `restore`); a tag fixa a versão. O mesmo contrato vale para Compose e Helm.
- As migrações de ClickHouse e PostgreSQL são **aditivas e idempotentes**. O
  `migrate` recusa uma migração cujo checksum mudou depois de aplicada.
- O envelope do WAL é protobuf versionado com compatibilidade N-1 (AD-22); um
  writer novo lê o envelope antigo durante a janela de upgrade.
- Nunca faça rollback de dados por cima de uma migração destrutiva. Na Fase 0
  nenhuma migração remove coluna/tabela; se isso mudar, o rollback vira
  forward-fix.

## Pré-flight (5 min)

1. **Backup da metadata** — siga [`backup-restore.md`](backup-restore.md).
2. Registre a versão atual:
   ```bash
   curl -fsS http://localhost:8080/api/v1/health | jq -r .version
   docker compose ps
   ```
3. Abra a página de saúde e confirme que não há degradação pendente
   (`http://localhost:8080/health`).
4. Verifique espaço em disco para a nova imagem e para o spool do writer.
5. Leia as notas da release; confirme que não há migração destrutiva.

## Compose

```bash
# upgrade para a tag nova (sem rebuild, usando a imagem publicada)
OBSERVAKI_IMAGE=ghcr.io/kaikeventura/observaki:<nova-tag> \
  docker compose up -d --wait --no-build

# verificar
curl -fsS http://localhost:8080/api/v1/health | jq -r .version
curl -fsS http://localhost:8080/api/v1/health/overview | jq '.status, .components'
docker compose ps

# rollback para a tag anterior
OBSERVAKI_IMAGE=ghcr.io/kaikeventura/observaki:<tag-anterior> \
  docker compose up -d --wait --no-build
```

O `up` recria API/gateway/writer e reexecuta os jobs `clickhouse-migrate` e
`postgres-migrate` (idempotentes). Os volumes não são tocados.

### Drill reproduzível

```bash
./deploy/compose/upgrade-drill.sh            # constrói as tags A e B
./deploy/compose/upgrade-drill.sh --no-build # reusa as tags
```

O drill constrói a mesma árvore duas vezes (`VERSION=5.4-drill-a` e
`5.4-drill-b`), sobe A, envia uma amostra, faz upgrade para B, rollback para A e
verifica versão, contagem de metadata e linhas no ClickHouse em cada passo. O
relatório em `deploy/compose/results/<UTC>/upgrade-report.md` registra os
tempos; o limite de rollback é 900 s.

## Helm

```bash
helm upgrade observaki deploy/helm/observaki \
  --namespace observaki --atomic --wait --timeout 10m \
  --set image.tag=<nova-tag>

helm status observaki -n observaki
kubectl -n observaki get jobs
kubectl -n observaki get pods
curl -fsS http://localhost:8080/api/v1/health/overview | jq .status

# rollback para a revisão anterior
helm rollback observaki -n observaki --wait --timeout 10m
```

- O Job `postgres-migrate` e o Job `clickhouse-migrate` rodam como hooks
  `post-install,post-upgrade` antes da liberação.
- `--atomic` desfaz automaticamente um upgrade que não fique `Ready`.
- O `helm test` cobre `/healthz`, `/readyz` do gateway e a health overview.

Evidência da Story 5.2 (kind de 3 nós): instalação em 47 s, upgrade em 22 s,
rollback em 17 s com dados preservados (2 → 4 → 4). O runbook acima é o mesmo
procedimento, com a versão fixada na tag.

## Verificação pós-upgrade

1. `GET /api/v1/health` devolve a versão nova.
2. `GET /api/v1/health/overview` está `healthy` (ou `no_data` sem tráfego) e os
   componentes estão verdes.
3. `helm test`/`compose ps` sem jobs falhando.
4. Uma amostra OTLP nova aparece no ClickHouse/VictoriaMetrics.
5. A página de saúde mostra lag, storage e o heartbeat de alerting recente.

## Quando parar

- Migração com checksum divergente, erro de constraint ou perda de dados: pare,
  restaure a metadata e trate como incidente.
- Banco de dados indisponível durante o upgrade: não force o rollback até o
  cluster estabilizar; a API falha fechada no alerting e a página mostra
  `critical`.
- Upgrade de chart/imagem que exija mudança de contrato público: siga a política
  de versionamento e registre um AD antes.
