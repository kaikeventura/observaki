# Backup e restore da metadata

**Escopo:** o control plane em PostgreSQL — organizações, usuários, projetos,
políticas de retenção, dashboards, canais, regras de alerta, silences, SLOs,
orçamentos de erro, tokens e auditoria. A telemetria (ClickHouse e
VictoriaMetrics) tem ciclo próprio e **não** entra neste backup (AD-3, AD-25).

**AC1 da Story 5.4:** restaurar projetos, dashboards, regras de alerta e
usuários em menos de 30 minutos. O drill local restaura em segundos.

## Modelo

- Formato `format_version: 1`, JSON indentado, com `created_at`, `app_version`,
  `schema_version` e uma tabela por entidade em ordem de dependência.
- O backup carrega apenas o **hash** dos tokens (`token_hash`); o token de
  ingestão em claro nunca é gravado.
- O restore é transacional e substitui toda a metadata de uma vez. Se qualquer
  linha falhar, o banco anterior permanece intacto.
- O restore truncate também o estado derivado de alerting (`alert_states`,
  `alert_outbox`); o avaliador recalcula no próximo ciclo.
- O restore reajusta a sequência de `audit_events` e recria o heartbeat de
  alerting.

## Compose

Pré-requisito: stack saudável (`docker compose up -d --wait`).

```bash
# backup (streaming do container distroless)
docker compose exec -T observaki-api /observaki backup --output - \
  > observaki-metadata-$(date -u +%Y%m%dT%H%M%SZ).json

# conferir sem restaurar
jq '.format_version, .schema_version, .counts' observaki-metadata-*.json

# restore (obrigatório --force: substitui a metadata atual)
docker compose exec -T observaki-api /observaki restore --input - --force \
  < observaki-metadata-*.json
```

Verificação:

```bash
docker compose exec -T postgres psql -U observaki -d observaki -tAc "
  SELECT (SELECT count(*) FROM users),
         (SELECT count(*) FROM projects),
         (SELECT count(*) FROM dashboards),
         (SELECT count(*) FROM alert_rules);"
```

Os quatro números precisam bater com os do backup. Abra
`http://localhost:8080/health` e confirme que o alerting voltou a avaliar
(heartbeat recente).

## Helm

```bash
./deploy/helm/backup.sh backup --namespace observaki --release observaki \
  --file observaki-metadata.json

./deploy/helm/backup.sh restore --namespace observaki --release observaki \
  --file observaki-metadata.json --force
```

O script encontra o pod da API pelo label
`app.kubernetes.io/instance=<release>,app.kubernetes.io/component=api` e usa
`kubectl exec` (a imagem é distroless, sem shell). O backup é streamado pelo
stdout; não há volume compartilhado.

## Rotina recomendada

1. Antes de qualquer upgrade, faça o backup e guarde o `app_version` e o
   `schema_version`.
2. Guarde o arquivo fora do cluster (object storage com criptografia e
   retenção). O backup contém hashes de token e URLs de webhook: trate como
   segredo.
3. Teste o restore em um namespace/stack descartável pelo menos uma vez por
   release. `make operator-smoke` executa o ciclo completo no Compose.
4. Registre o tempo de restore no ticket/relatório. O alvo é < 30 min; o drill
   local mede poucos segundos com a metadata de avaliação.

## Falhas comuns

| Sintoma | Causa | Correção |
|---|---|---|
| `target already has metadata; pass --force` | restore sem `--force` | repita com `--force` se a substituição for intencional |
| `backup is missing table ...` | arquivo editado/truncado | gere um backup novo; o restore valida todas as tabelas |
| `unsupported backup format` | backup de outra versão incompatível | use o binário que gerou o backup ou migre o documento |
| `migration ... changed after it was applied` | SQL de migração alterado | nunca edite uma migração aplicada; crie uma nova |
| restore conclui mas o alerting mostra erro | job de migração do PostgreSQL não rodou | rode `observaki migrate` (o Compose/Helm roda automaticamente no próximo `up`/`upgrade`) |

## Evidência

`deploy/compose/operator-smoke.sh` mede backup, mutação, restore e a
comparação das contagens; o relatório fica em
`deploy/compose/results/<UTC>/operator-report.md`.
