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
O tempo medido no smoke de 3 nós está em `deploy/helm/results/`.

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

Com `external`, o chart instala apenas API, gateway, writer e o Job de
migração; ClickHouse, VM, PostgreSQL e NATS ficam por sua conta (e devem ser
distribuídos/HA, como pede a spine).

### Migrações

O Job `observaki-migrate-<checksum>` roda como hook `post-install,post-upgrade`
e aplica as migrações versionadas e idempotentes de
`internal/adapters/clickhouse/migrations` (copiadas para
`observaki/files/migrations` e verificadas por `deploy/helm/check.sh`). Não há
passo manual. Rollback não reaplica migrações: o schema é aditivo, então a
versão anterior da aplicação continua compatível.

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
PostgreSQL, NATS e o spool do writer mantêm os dados. O smoke mede as contagens
de logs e traces antes/depois do upgrade e do rollback para provar que nada foi
perdido.

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

## Limitações desta versão alpha

- Dependências em `bundled` são **single-replica**: não há HA, replicação de
  ClickHouse/Keeper nem cluster NATS. Para produção use `external`.
- Não há Operator, CRDs, autoscaling nem backup automatizado (Stories 5.3/5.4 e
  Fase 3).
- O PostgreSQL ainda não tem schema de control plane; o API valida o DSN e as
  tabelas chegam com as stories de API/config plane.
- A UI ainda é a landing page com o cartão de saúde; o Explore é 6.1–6.4.
- O chart não instala Ingress controller, cert-manager nem política de rede.
- Validado em kind (Linux). A validação em EKS/GKE e a matriz N-1/N-2 ficam
  para as próximas stories.

## Desinstalar

```bash
helm uninstall observaki -n observaki
# os PVCs de volumeClaimTemplates são mantidos de propósito
kubectl delete pvc -n observaki -l app.kubernetes.io/instance=observaki
kubectl delete namespace observaki
```
