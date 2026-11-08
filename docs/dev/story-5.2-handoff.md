# Story 5.2 — Helm chart alpha (handoff)

**Status:** AC1, AC2 e AC3 concluídos com evidência local em Linux (kind v1.37,
Helm 4.3.1). **Não é G1.** O chart é alpha e as dependências empacotadas não são
HA; produção usa `dependencies.mode=external`.
**Data:** 2026-11-08 (UTC).
**Commit:** feature + docs de status desta sessão.
**Epic:** P0-E5 — Esqueleto de deploy, operação e API.
**ADs:** AD-4 (control/data plane), AD-6 (mesmas imagens/config em Compose e
Kubernetes), AD-18 (durabilidade/ack), AD-19 (snapshot de auth assinado),
AD-21 (RED no gateway/VM), AD-22 (Postgres control plane), AD-25 (Helm 4.3+ /
Kubernetes 1.35+).

## Entregue

| Artefato | Conteúdo |
|---|---|
| `deploy/helm/observaki/` | Chart `apiVersion: v2`, `version: 0.1.0-alpha.1`, `appVersion: v0.0.1-alpha`, `kubeVersion >=1.35` |
| `templates/` | API/UI, gateway OTLP, writer (StatefulSet com spool), Job de migração (`post-install,post-upgrade`), PDBs, Ingress opcional, probes, `helm test` e `NOTES.txt` |
| `templates/{clickhouse,nats,victoriametrics,postgres}.yaml` | Dependências single-replica no perfil `bundled` (avaliação) |
| `values.yaml` | CPU/memória/storage documentados; `dependencies.mode=external` para serviços distribuídos; guards de produção (AD-19) |
| `files/migrations/*.sql` | Cópias verificadas das migrações canônicas de `internal/adapters/clickhouse/migrations` |
| `files/ingest-auth-dev.json` | Snapshot público de desenvolvimento (idêntico ao Compose) |
| `deploy/helm/kind.yaml` | Cluster kind de 3 nós (1 control plane + 2 workers) |
| `deploy/helm/check.sh` | Drift das cópias, `helm lint --strict`, render bundled/external, guards, notação científica e dry-run |
| `deploy/helm/smoke.sh` | AC1/AC2/AC3 em kind com relatório em `deploy/helm/results/<UTC>/report.md` |
| `deploy/helm/sync-files.sh` | Sincroniza as cópias embutidas no chart |
| `Makefile` | Alvos `chart-check` e `chart-smoke` |
| `.github/workflows/ci.yml` | Instala Helm 4.3.1 e roda `make chart-check` no job `quality` |
| `README.md`, `deploy/README.md`, chart README | Quickstart, arquitetura, prerequisitos, values, sizing, upgrade/rollback, limitações e troubleshooting |

## Decisões de design

1. **Dependências empacotadas por padrão, externas por opção.** O AC1 exige que
   as dependências fiquem prontas; o perfil padrão sobe ClickHouse, VM,
   PostgreSQL e NATS single-replica (equivalente ao Compose). `dependencies.mode=external`
   não instala StatefulSets de dependência e aponta para serviços externos.
   HA das dependências não é escopo do alpha.
2. **Migração como hook, não init container.** O hook
   `post-install,post-upgrade` roda depois que os workloads ficam `Ready` (com
   `--wait`) e aplica as migrações idempotentes. Um init container esperando o
   schema criaria deadlock com `--wait` (o hook nunca rodaria). Rollback não
   reaplica migrações; o schema é aditivo e a versão anterior continua
   compatível.
3. **Writer espera o gateway.** Um init container faz polling no `/readyz` do
   gateway; as streams do WAL pertencem ao gateway e o writer falha fechado sem
   elas. Isso evita crash-loop no primeiro install.
4. **Segredos de desenvolvimento explícitos e guards.** O perfil padrão usa o
   token/senhas públicas do Compose e `environment=development`. Em
   `staging`/`production` o render falha sem snapshot assinado, chave Ed25519,
   audit log ou Secrets de dependência (AD-19).
5. **Mesma imagem para os três papéis** (`serve`, `gateway`, `writer`), sem fork
   por topologia (AD-6).
6. **Chart em `deploy/helm/observaki`.** O AC ilustra `helm install observaki
   ./chart`; como o Helm sempre recebe um caminho, o comando documentado é
   `helm install observaki ./deploy/helm/observaki`, consistente com
   `deploy/compose` (o `compose.yaml` fica na raiz só porque `docker compose up`
   não aceita `-f`).
7. **Cópias com verificação de drift.** Helm não lê arquivos fora do diretório
   do chart; as migrações e o snapshot de auth são copiados para
   `observaki/files/` e `check.sh` falha se divergirem das fontes.

## AC1 — Instalação

Run `deploy/helm/results/20261108T012922Z/report.md` (Linux 7.0.0-42, kind
v0.33.0 com Kubernetes v1.37.0, Helm v4.3.1, imagem `observaki:local`
construída do checkout), a partir de um cluster vazio:

- `helm upgrade --install observaki deploy/helm/observaki -n observaki
  --create-namespace --wait --timeout 10m`: **47 s** até todos os workloads
  prontos (limite do smoke: 600 s).
- `deployment/observaki-api` 2/2, `deployment/observaki-gateway` 2/2,
  `statefulset/observaki-writer` 1/1, `clickhouse` 1/1, `victoriametrics` 1/1,
  `postgres` 1/1, `nats` 1/1; Job `observaki-migrate-*` `Completed`.
- **3 nós Ready** e **5 PVCs Bound** (CH 20Gi, NATS 20Gi, VM 10Gi, spool 10Gi,
  PG 2Gi) na StorageClass `standard` do kind.
- API `/healthz` e gateway `/readyz` respondem 200 via Service; a amostra OTLP
  grava **2 logs e 2 spans** no ClickHouse e a métrica aparece no
  VictoriaMetrics; `helm test` passa (`observaki-test-api` e
  `observaki-test-gateway`).
- O values documenta CPU/memória/storage de cada componente (tabela de sizing
  no README do chart).

## AC2 — Upgrade e rollback

- O smoke empacota o chart como **0.1.0-alpha.1.1** (`helm package --version`),
  faz upgrade dessa versão com a imagem `observaki:local-v2` e depois
  `helm rollback` para a revisão anterior.
- Revisões: **1** (`observaki-0.1.0-alpha.1`, `observaki:local`) →
  **2** (`observaki-0.1.0-alpha.1.1`, `observaki:local-v2`) →
  **3** rollback para **1** (`observaki-0.1.0-alpha.1`, `observaki:local`).
- Upgrade em **22 s** e rollback em **17 s**; API e gateway voltam a responder
  depois de cada rollout.
- Dados preservados: logs/traces **2 → 4 → 4**; a amostra pós-upgrade prova que
  a imagem nova também escreve, e os PVCs continuam os mesmos após o rollback.
- A imagem dos pods volta de `observaki:local-v2` para `observaki:local` e a
  versão do chart volta para `0.1.0-alpha.1`.

## AC3 — Documentação

O [`README` do chart](../../deploy/helm/observaki/README.md) cobre
**Prerequisites**, arquitetura, instalação, acesso, **Values**, **Sizing**
(CPU/memória/storage por componente), dependências externas, migrações,
**Upgrade** e rollback, verificação automática, **Troubleshooting**,
limitações e desinstalação. O smoke falha se alguma dessas seções faltar.

## Comandos executados

```bash
make chart-check
make chart-smoke            # 27 checks, 0 failures; relatório em deploy/helm/results/20261108T012922Z/
make fmt
make smoke                  # make ci (bootstrap/lint/test/build) + smoke do binário
```

`make smoke` cobre `make ci`: golangci-lint, prettier, svelte-check, clippy, ADR
check, Go/Vitest/Cargo tests, build do binário + UI + agente e o smoke do
binário (healthz, OTLP HTTP/gRPC, auth 401/403 + audit, quota 429, métricas,
SIGTERM, loadgen, `chloadgen` e cenários/report do benchmark).

## Riscos e pendências

1. **Dependências empacotadas não são HA.** ClickHouse, VM, PostgreSQL e NATS
   rodam single-replica no perfil de avaliação; produção exige
   `dependencies.mode=external` com serviços distribuídos.
2. **Modo `external` validado só por render.** O `check.sh` cobre render e
   guards, mas não há teste contra um ClickHouse/VM/NATS/PostgreSQL externos
   reais; a validação em EKS/GKE fica para depois do alpha.
3. **Matriz de versões.** Validado em kind v1.37 + Helm 4.3.1. A matriz
   Kubernetes N-1/N-2 e outros provedores continuam abertos.
4. **Rollback não reaplica migrações.** O schema é aditivo, então a versão
   anterior funciona; uma migração destrutiva exigiria um runbook próprio.
5. **Sem Operator/CRDs, autoscaling, backup ou Ingress controller.** São as
   Stories 5.3/5.4 e a Fase 3. O chart também não instala NetworkPolicy.
6. **UI ainda é a landing page.** O Explore é 6.1–6.4; o AC1 valida a stack e
   a API, não a experiência de exploração.
7. **PostgreSQL sem schema de control plane.** O API valida o DSN; as tabelas
   chegam com as stories de API/config plane.
8. **Rede necessária.** O cluster precisa puxar as imagens de dependência; o
   smoke carrega apenas as imagens `observaki:*` no kind.
9. **Credenciais de desenvolvimento públicas.** O token de ingest e as senhas
   de CH/PG em `values.yaml` valem só para avaliação; os guards barram
   `staging`/`production` sem Secrets próprios.
