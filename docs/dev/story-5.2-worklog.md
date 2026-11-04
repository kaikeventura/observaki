# Story 5.2 — Helm chart alpha (worklog)

**Status:** em andamento — implementação pronta e validada parcialmente; falta
fechar AC2 (rollback) com o relatório do smoke.
**Data:** 2026-11-04 (UTC).
**Epic:** P0-E5 — Esqueleto de deploy, operação e API.
**ADs:** AD-4 (control/data plane), AD-6 (mesmas imagens/config em Compose e
Kubernetes), AD-18 (durabilidade/ack), AD-19 (snapshot de auth assinado),
AD-21 (RED no gateway/VM), AD-22 (Postgres control plane), AD-25 (Helm 4.3+ /
Kubernetes 1.35+).

## Onde paramos

O chart está escrito em [`deploy/helm/observaki`](../../deploy/helm/observaki)
e o `make chart-check` está verde. A instalação em um cluster kind de 3 nós
(v1.37) chegou ao fim, a amostra OTLP gravou **2 logs e 2 spans** no ClickHouse
e o `helm test` passou. O upgrade para a imagem `observaki:local-v2` rolou API,
gateway e writer, mas a sessão foi interrompida durante o upgrade; por isso o
Helm marcou a revisão 2 como `failed: context canceled` e o **rollback não foi
executado**. O cluster kind foi destruído para liberar recursos.

Nada no chart depende desse estado: `make chart-smoke` recria o cluster e é
idempotente (`helm upgrade --install`).

## Entregue

| Artefato | Conteúdo |
|---|---|
| `deploy/helm/observaki/` | Chart `apiVersion: v2`, `version: 0.1.0-alpha.1`, `appVersion: v0.0.1-alpha`, `kubeVersion >=1.35` |
| `templates/` | API/UI, gateway, writer (StatefulSet com spool), Job de migração (hook pós-install/pós-upgrade), PDBs, Ingress opcional, `helm test` e `NOTES.txt` |
| `templates/{clickhouse,nats,victoriametrics,postgres}.yaml` | Dependências single-replica em `dependencies.mode=bundled` (perfil de avaliação) |
| `values.yaml` | Documenta CPU/memória/storage de cada componente; `dependencies.mode=external` para ClickHouse/VM/PostgreSQL/NATS gerenciados |
| `files/migrations/*.sql` | Cópias verificadas das migrações canônicas de `internal/adapters/clickhouse/migrations` |
| `files/ingest-auth-dev.json` | Snapshot público de desenvolvimento (idêntico ao Compose) |
| `deploy/helm/kind.yaml` | Cluster kind de 3 nós (1 control plane + 2 workers) |
| `deploy/helm/check.sh` | Drift das cópias, `helm lint --strict`, render bundled/external, guards de produção e dry-run |
| `deploy/helm/smoke.sh` | AC1/AC2/AC3 em kind com relatório em `deploy/helm/results/<UTC>/report.md` |
| `deploy/helm/sync-files.sh` | Sincroniza as cópias embutidas no chart |
| `Makefile` | `chart-check` e `chart-smoke` |
| `.github/workflows/ci.yml` | Instala Helm 4.3.1 e roda `make chart-check` no job `quality` |

## Decisões de design

1. **Dependências empacotadas por padrão, externas por opção.** O AC1 exige que
   “as dependências fiquem prontas”; o perfil padrão sobe ClickHouse, VM,
   PostgreSQL e NATS single-replica (equivalente ao Compose). `dependencies.mode=external`
   remove esses StatefulSets e aponta para serviços distribuídos. A HA das
   dependências não é escopo do alpha e está documentada como limitação.
2. **Um hook de migração, não um init container.** O hook
   `post-install,post-upgrade` roda depois que os workloads estão `Ready` (com
   `--wait`) e aplica as migrações idempotentes. Um init container que esperasse
   o schema criaria deadlock com `--wait` (o hook nunca rodaria).
3. **Writer espera o gateway.** O writer tem um init container que faz polling
   no `/readyz` do gateway; as streams do WAL pertencem ao gateway e o writer
   falha fechado sem elas. Isso evita crash-loop no primeiro install.
4. **Segredos de desenvolvimento explícitos e guards.** O perfil padrão usa o
   token/senhas públicas do Compose e `environment=development`. Em
   `staging`/`production` o render falha se faltar snapshot assinado, chave
   Ed25519, audit log ou Secrets de dependência (AD-19).
5. **Mesma imagem para os três papéis** (`serve`, `gateway`, `writer`), sem fork
   por topologia (AD-6).
6. **Chart em `deploy/helm/observaki`, não na raiz.** O AC ilustra
   `helm install observaki ./chart`; como o Helm sempre recebe um caminho, o
   comando documentado é `helm install observaki ./deploy/helm/observaki`,
   consistente com `deploy/compose` (o `compose.yaml` fica na raiz apenas porque
   `docker compose up` não tem `-f`).
7. **Migrations e snapshot são cópias com verificação de drift** porque o Helm
   não lê arquivos fora do diretório do chart; `check.sh` falha se divergirem.

## Evidência já coletada (sessão interrompida)

Cluster kind `observaki-helm` (3 nós, Kubernetes v1.37.0, StorageClass
`standard` local-path):

- `helm upgrade --install ... --wait --timeout 10m`: revisão 1 `deployed`;
  todos os 7 workloads `Running/Ready`; 5 PVCs `Bound` (20Gi CH, 20Gi NATS,
  10Gi VM, 10Gi spool, 2Gi PG); Job `observaki-migrate-161220efd3` `Completed`.
- `deploy/compose/sample.sh` via port-forward do gateway: `otel_logs = 2`,
  `otel_traces = 2`, métrica no VictoriaMetrics (checks do script passaram).
- `helm test observaki`: `observaki-test-api` e `observaki-test-gateway`
  `Succeeded`.
- Upgrade para `observaki:local-v2`: os pods de API, gateway e writer foram
  recriados com a nova imagem; a sessão foi abortada antes do `helm rollback`.

Os logs dessa execução foram descartados junto com o cluster kind (não havia
relatório completo); a próxima execução do `make chart-smoke` gera um novo
diretório em `deploy/helm/results/`.

## Para retomar

```bash
export PATH="$HOME/.local/bin:$PATH"   # helm, kubectl e kind instalados pelo dono
make chart-check                        # deve continuar verde
make chart-smoke                        # cria o kind, instala, amostra, upgrade e rollback
# relatório: deploy/helm/results/<UTC>/report.md
```

Ao terminar: revisar `report.md`, commitar o diretório de resultados, fechar
AC1/AC2/AC3 no `STATUS.md`, rodar `make fmt/lint/test/build/smoke` e atualizar
`docs/dev/story-5.2-handoff.md` (ainda não existe).

## Gotchas encontrados

1. **Números grandes viram notação científica.** O parser YAML do Helm trata
   inteiros grandes como `float64` e `%v` imprime `1.048576e+07`; o gateway e o
   ClickHouse recusaram os valores. Todos os campos grandes em `values.yaml`
   agora são strings citadas (`maxRequestBytes`, `streamMaxBytes`,
   `batchMaxBytes`, `maxServerMemoryBytes`, `maxPayload`, etc.). O
   `chart-check` falha se notação científica reaparecer no render? Ainda não —
   considerar adicionar esse grep ao `check.sh`.
2. **`kubectl apply --dry-run=client` exige um cluster acessível** (descoberta
   de API), mesmo com `--validate=false`; o `check.sh` só roda esse passo quando
   há cluster.
3. **Não usar `include` do próprio template para checksum** — recursão
   infinita. Os checksums de ConfigMap são derivados dos valores.
4. **Ordem do `--wait`:** hooks `post-install` rodam depois dos recursos
   ficarem `Ready`; por isso o writer não pode esperar o schema (ver decisão 2).
5. **Kind usa `kindest/node:v1.37.0` por padrão** e já traz a StorageClass
   `standard` (local-path), suficiente para os PVCs do chart.

## Pendências / riscos

- Fechar AC2 com o relatório do smoke (rollback + contagens preservadas).
- Validar em um cluster real (EKS/GKE) e a matriz Kubernetes N-1/N-2 fica para
  depois do alpha.
- Dependências empacotadas não são HA; produção exige `dependencies.mode=external`.
- Sem Operator/CRDs, autoscaling, backup e Ingress controller (Stories 5.3/5.4
  e Fase 3).
- O chart não instala política de rede; o gateway fica acessível no cluster.
