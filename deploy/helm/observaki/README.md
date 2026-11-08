# Observaki Helm chart (alpha)

Instala o Observaki em um cluster Kubernetes de 3 nós: gateway OTLP, API/UI
(Query/control plane), writer, o Job de migrações e — no perfil padrão — as
quatro dependências (ClickHouse, VictoriaMetrics, PostgreSQL e NATS JetStream).

Este é o chart **alpha** da Story 5.2. Ele prova instalação, upgrade e rollback
em um cluster de 3 nós; o perfil de produção com dependências distribuídas usa
`dependencies.mode=external`. **Não é evidência de G1.**

## Arquitetura

```
                     OTLP gRPC :4317 / HTTP :4318
                                 │
                          ┌──────▼──────┐
                          │   gateway   │  auth, quota, redaction, RED
                          └──────┬──────┘
                                 │ WAL (JetStream)
                          ┌──────▼──────┐
                          │    NATS     │
                          └──────┬──────┘
                                 │ durable consumers
                 ┌───────────────┴───────────────┐
          ┌──────▼──────┐                 ┌──────▼─────────────┐
          │   writer    │── logs/traces ─▶│    ClickHouse      │
          │             │── metrics ─────▶│  VictoriaMetrics   │
          └─────────────┘                 └────────────────────┘
                                 │
                          ┌──────▼──────┐        ┌────────────┐
                          │  API + UI   │───────▶│ PostgreSQL │
                          └─────────────┘        └────────────┘
```

O mesmo binário/imagem roda `serve`, `gateway` e `writer`; a topologia muda
pelos valores, não por fork de código (AD-6). O chart respeita os invariantes:
ingestão só confirma depois do WAL (AD-18), RED é calculado no gateway antes do
sampling e gravado no VictoriaMetrics (AD-21), e o control plane PostgreSQL
fica separado do data plane (AD-4, AD-22).

## Prerequisites

| Requisito | Versão | Notas |
|---|---|---|
| Kubernetes | **1.35+** | `kubeVersion` do chart; validado em kind 1.36 |
| Helm | **4.3+** | usa hooks, `--wait` e `helm test` |
| StorageClass | default | PVCs RWO para ClickHouse, VM, PostgreSQL, NATS e o spool do writer |
| Nós | **3** | 1 control plane + 2 workers é o mínimo validado; o kind do smoke usa 3 nós |
| Recursos | ~4 vCPU / ~6 GiB livres | perfil de avaliação; veja a tabela de sizing |
| Rede | saída para o registry | as imagens de dependência são puxadas pelo cluster |

Para a validação local: `docker`, `kind` v0.33+, `kubectl` 1.35+ e `helm` 4.3+.
O `deploy/helm/smoke.sh` cria o cluster, instala, envia amostra, faz upgrade e
rollback.

## Instalação

```bash
helm install observaki ./deploy/helm/observaki \
  --namespace observaki --create-namespace \
  --wait --timeout 10m \
  --set image.repository=ghcr.io/kaikeventura/observaki \
  --set image.tag=v0.0.1-alpha \
  --set image.pullPolicy=IfNotPresent
```

O `--wait` só retorna quando os sete workloads (API, gateway, writer e as
quatro dependências) estão `Ready` e o Job de migração terminou com sucesso.
No smoke de 3 nós (kind v1.37, Helm 4.3.1) o install levou **47 s**, o upgrade
**22 s** e o rollback **17 s**; o relatório está em `deploy/helm/results/`.

Para usar uma imagem construída localmente (kind):

```bash
docker build -f deploy/compose/Dockerfile -t observaki:local .
kind load docker-image observaki:local --name observaki-helm
helm install observaki ./deploy/helm/observaki -n observaki --create-namespace \
  --wait --timeout 10m \
  --set image.repository=observaki --set image.tag=local
```

### Acessar

```bash
kubectl -n observaki port-forward svc/observaki-api 8080:80
# UI/API em http://127.0.0.1:8080

kubectl -n observaki port-forward svc/observaki-gateway 4318:4318
OBSERVAKI_OTLP_HTTP_URL=http://127.0.0.1:4318 ./deploy/compose/sample.sh
```

O Ingress do API/UI é opcional (`ingress.enabled=true`). O gateway OTLP
normalmente é consumido por coletores dentro do cluster, não por Ingress.

### Token de desenvolvimento

O perfil padrão (`environment=development`, `gateway.auth.devSnapshot=true`)
cria um Secret com o snapshot público `deploy/helm/observaki/files/ingest-auth-dev.json`
— tenant `tenant-dev`, projeto `project-dev`, token `observaki-local-dev-token`.
Ele é idêntico ao do Compose e **não pode** ser usado fora de um cluster de
avaliação. `deploy/helm/check.sh` falha se as duas cópias divergirem.

Em `staging`/`production` o chart falha fechado (AD-19): é obrigatório fornecer
`gateway.auth.existingSecret` com a snapshot assinada, `gateway.auth.publicKey`
(Ed25519) e `gateway.auth.auditFile`.

## Values

A referência completa, com comentários, é [`values.yaml`](observaki/values.yaml).
Os valores mais usados:

| Chave | Default | Descrição |
|---|---|---|
| `environment` | `development` | `development`, `test`, `staging` ou `production`; staging/produção ativam os guards de segurança |
| `image.repository` | `ghcr.io/kaikeventura/observaki` | imagem única dos três papéis |
| `image.tag` | `.Chart.AppVersion` | tag da imagem |
| `api.replicaCount` | `2` | réplicas do API/UI |
| `gateway.replicaCount` | `2` | réplicas do gateway OTLP |
| `writer.replicaCount` | `1` | réplicas do writer; cada pod recebe seu próprio PVC de spool |
| `gateway.limits.maxRequestBytes` | `10485760` | limite de request OTLP (10 MiB) |
| `gateway.limits.maxEvents` | `10000` | eventos por request |
| `gateway.wal.shards` | `32` | shards por tenant/projeto/epoch/sinal |
| `gateway.wal.streamMaxBytes` | `2147483648` | WAL por stream de sinal (2 GiB no perfil de avaliação) |
| `writer.retentionDays` | `15` | `retention_days` das linhas de logs/traces |
| `clickhouse.persistence.size` | `20Gi` | dados de logs/traces |
| `victoriametrics.persistence.size` | `10Gi` | métricas e RED |
| `postgres.persistence.size` | `2Gi` | control plane |
| `nats.persistence.size` | `20Gi` | JetStream WAL + DLQ |
| `writer.spool.size` | `10Gi` | spool local durável |
| `dependencies.mode` | `bundled` | `bundled` (avaliação) ou `external` (produção) |
| `postgresMigrations.enabled` | `true` | aplica as migrações do control plane no install/upgrade |
| `postgresMigrations.seedDev` | `true` | semeia a metadata de avaliação (ignorado fora de development/test) |
| `selfObservability.refreshInterval` | `5s` | cache da health overview |
| `selfObservability.timeout` | `3s` | timeout de cada probe de auto-observabilidade |
| `selfObservability.ingestLagThreshold` | `10s` | limiar de lag que deixa a ingestão `degraded` |
| `alerting.enabled` | `true` | avaliação das regras internas e outbox |
| `alerting.interval` | `15s` | período de avaliação |
| `alerting.deliveryInterval` | `5s` | dreno do outbox de notificações |

### Sizing

Defaults por réplica; o perfil padrão cabe em um cluster de 3 nós com
~4 vCPU e ~6 GiB livres (soma dos limits ~7 GiB, requests ~2,1 GiB):

| Componente | Réplicas | CPU request/limit | Memória request/limit | Storage |
|---|---|---|---|---|
| `api` | 2 | 50m / 500m | 64Mi / 256Mi | — |
| `gateway` | 2 | 100m / 1 | 128Mi / 512Mi | — |
| `writer` | 1 | 100m / 1 | 256Mi / 1Gi | 10Gi spool |
| `clickhouse` | 1 | 500m / 2 | 1Gi / 2Gi | 20Gi |
| `victoriametrics` | 1 | 100m / 2 | 256Mi / 1Gi | 10Gi |
| `postgres` | 1 | 50m / 1 | 128Mi / 512Mi | 2Gi |
| `nats` | 1 | 50m / 1 | 128Mi / 512Mi | 20Gi |
| `clickhouse-migrate` (Job) | 1 | 50m / 500m | 64Mi / 256Mi | — |
| `postgres-migrate` (Job) | 1 | 50m / 500m | 64Mi / 256Mi | — |

O ClickHouse tem `max_server_memory_usage` de 1,5 GiB dentro de um limite de
2 GiB; o NATS usa `max_file_store` de 16 GB (3 streams de 2 GiB + DLQ de 8 GiB)
num PVC de 20Gi. Ajuste requests/limits e os tamanhos de PVC conforme o volume;
o perfil T1 (`bench/env/t1.json`) dimensiona outro hardware.

### Dependências externas

```bash
helm install observaki ./deploy/helm/observaki -n observaki --create-namespace \
  --set dependencies.mode=external \
  --set external.clickhouse.url=http://clickhouse.example:8123 \
  --set external.clickhouse.nativeAddress=clickhouse.example:9000 \
  --set external.clickhouse.credentialsSecret=clickhouse-credentials \
  --set external.victoriametrics.url=http://victoriametrics.example:8428 \
  --set external.postgres.urlSecret=observaki-postgres \
  --set external.nats.url=nats://nats.example:4222
```

Contratos dos Secrets:

- `external.clickhouse.credentialsSecret`: chaves `user` e `password` (omitir
  só se o endpoint não exigir autenticação).
- `external.postgres.urlSecret`: chave `url` com o DSN completo.
- `external.clickhouse.nativeAddress`: `host:9000` usado pelo Job de migração.
  Se o operador do ClickHouse aplica o schema, use `migrations.enabled=false`.

Com `external`, o chart instala apenas API, gateway, writer e os Jobs de
migração; ClickHouse, VM, PostgreSQL e NATS ficam por sua conta (e devem ser
distribuídos/HA, como pede a spine).

### Migrações

Dois Jobs rodam como hooks `post-install,post-upgrade`:

- `observaki-migrate-<checksum>` aplica as migrações versionadas e idempotentes
  do ClickHouse em `internal/adapters/clickhouse/migrations` (copiadas para
  `observaki/files/migrations` e verificadas por `deploy/helm/check.sh`).
- `observaki-postgres-migrate-<checksum>` aplica as migrações do control plane
  embutidas no binário (`internal/adapters/postgres/migrations`) e, em
  `development`/`test`, semeia a metadata de avaliação
  (`postgresMigrations.seedDev`). Use `postgresMigrations.enabled=false` quando
  um operador externo gerencia o schema.

Não há passo manual. Rollback não reaplica migrações: o schema é aditivo, então
a versão anterior da aplicação continua compatível.

## Backup/restore da metadata

A imagem é distroless, então o backup é streamado pelo `kubectl exec` do pod da
API:

```bash
./deploy/helm/backup.sh backup --namespace observaki --release observaki \
  --file observaki-metadata.json
./deploy/helm/backup.sh restore --namespace observaki --release observaki \
  --file observaki-metadata.json --force
```

O arquivo contém organizações, usuários, projetos, dashboards, regras, canais,
silences, SLOs, tokens (apenas o hash) e auditoria. O restore é transacional e
exige `--force`. O procedimento completo e os limites estão em
[`docs/runbooks/backup-restore.md`](../../../docs/runbooks/backup-restore.md).

## Upgrade e rollback

```bash
# nova versão do chart e/ou da imagem
helm upgrade observaki ./deploy/helm/observaki -n observaki \
  --wait --timeout 10m \
  --set image.tag=v0.0.2-alpha

# voltar para a revisão anterior
helm rollback observaki -n observaki --wait --timeout 10m
helm history observaki -n observaki
```

Os PVCs não são recriados no upgrade nem no rollback; ClickHouse, VM,
PostgreSQL, NATS e o spool do writer mantêm os dados. O smoke empacota o chart
como `0.1.0-alpha.1.1`, faz upgrade dessa versão com uma imagem nova e então
rollback para a revisão anterior, medindo as contagens de logs e traces
antes/depois (`2 -> 4 -> 4`) para provar que nada foi perdido. O runbook completo
(pré-flight com backup, verificação e limites) está em
[`docs/runbooks/upgrade-rollback.md`](../../../docs/runbooks/upgrade-rollback.md).

## Auto-observabilidade e alertas

A API serve a página `/health` e o endpoint `GET /api/v1/health/overview` com
lag de ingestão, erro de query, uso de storage, componentes e o heartbeat do
alerting. O `helm test` inclui um probe desse endpoint. O alerting interno usa
as regras/canais do PostgreSQL; em `development`, o seed cria a regra
`Ingestion lag high` e um canal webhook local. Detalhes em
[`docs/runbooks/auto-observabilidade.md`](../../../docs/runbooks/auto-observabilidade.md).

## Verificação automática

```bash
make chart-check     # lint, render, guards e drift das cópias
make chart-smoke     # kind 3 nós: AC1, AC2 e AC3 (relatório em deploy/helm/results/)
```

`make chart-check` não precisa de cluster e roda no CI. `make chart-smoke`
requer Docker e rede, cria e destrói o cluster kind (use `--keep` para
inspecionar). O relatório inclui pods, PVCs, tempos de install/upgrade/rollback,
contagens no ClickHouse e o resultado de cada check.

## Troubleshooting

| Sintoma | Causa provável | Correção |
|---|---|---|
| Pods `Pending` por falta de PVC | nenhuma StorageClass default | instale uma StorageClass RWO ou defina `*.persistence.storageClass` |
| `clickhouse-migrate` falha com timeout | ClickHouse ainda subindo ou credencial divergente | `kubectl logs job -l app.kubernetes.io/component=migrate`; confira o Secret `observaki-clickhouse` |
| writer em `Init` "waiting for the OTLP gateway" | gateway sem NATS/streams | `kubectl logs deploy/observaki-gateway`; o writer espera o `/readyz` |
| `helm install` estoura o timeout | imagens ainda puxando ou PVC não bound | `kubectl get pods,pvc -n observaki`; aumente `--timeout` |
| gateway responde 401 | token errado | use `observaki-local-dev-token` no perfil de desenvolvimento |
| gateway 429 `wal_backpressure` | WAL cheio (NATS fora ou writer atrasado) | veja `kubectl logs statefulset/observaki-nats` e o lag do writer em `/stats` |
| upgrade falha no hook de migração | Job anterior ainda presente | o hook usa `before-hook-creation`; se um Job travou, `kubectl delete job -l app.kubernetes.io/component=migrate` e repita |
| pods reiniciam após rollback | imagem anterior não existe no registry | o smoke carrega as duas imagens; em produção mantenha N e N-1 publicadas |
| alerting `critical` com "evaluation is stale" | API sem PostgreSQL ou processo parado | veja `kubectl logs deploy/observaki-api`; a API tenta reconectar por 30 s no start e mostra o erro na health page |
| health overview retorna `503` | provider de self-observability não configurado (build antigo) | atualize a imagem; as variáveis `OBSERVAKI_SELF_*` são preenchidas pelo chart |

## Limitações desta versão alpha

- Dependências em `bundled` são **single-replica**: não há HA, replicação de
  ClickHouse/Keeper nem cluster NATS. Para produção use `external`.
- Não há Operator, CRDs, autoscaling nem backup **agendado** (há os comandos e
  scripts de backup/restore; o agendamento/object storage é Fase 3).
- O chart não instala Ingress controller, cert-manager nem política de rede.
- A query plane é Fase 1; a taxa de erro de query na health page conta o
  endpoint placeholder da API até o Query Service existir.
- Validado em kind (Linux). A validação em EKS/GKE e a matriz N-1/N-2 ficam
  para as próximas stories.

## Desinstalar

```bash
helm uninstall observaki -n observaki
# os PVCs de volumeClaimTemplates são mantidos de propósito
kubectl delete pvc -n observaki -l app.kubernetes.io/instance=observaki
kubectl delete namespace observaki
```
