# Story 2.4 — Ambiente reproduzível e baseline publicado (handoff)

**Status:** scaffolding implementado e validado; **execução T1 bloqueada**.
**Data:** 2026-11-01 (execuções em UTC de 2026-11-02).
**Escopo entregue:** perfil T1, provisionamento idempotente de referência,
estimativa de custo, limpeza, protocolo de baseline e publicação de uma
pré-baseline local. O cluster T1 não foi provisionado e o baseline T1 não foi
executado; os motivos estão em "Bloqueios".

## Entregue

| Artefato | Conteúdo |
|---|---|
| `bench/env/t1.json` | Perfil T1 provider-neutral: 6 ClickHouse (6–12), 3 VictoriaMetrics, 6 gateways (4–6), 4 NATS, 2 loadgens; 1M/s, 134 B/evento, retenção 168 h, réplica 2 |
| `bench/env/prices.aws-us-east-1.json` | Snapshot manual de preços On-Demand de `us-east-1`, com data, fonte e exclusões |
| `bench/internal/envprofile` | Carga/validação do perfil, capacidade, preços, estimativa de custo e renderização de `tfvars`; testes de contrato e de custo |
| `bench/env/terraform` | VPC/subnet/IGW/SG, 21 instâncias, 20 volumes gp3, key pair, outputs de IPs/IDs; provider AWS travado em `>=5,<7`; `terraform validate` verde |
| `bench/env/cluster.sh` | `check`, `cost`, `plan`, `apply`, `status`, `output`, `destroy`/`cleanup`; sincroniza o perfil, exige `OBSERVAKI_BENCH_ALLOW_SPEND=1`, recusa root, confirma e verifica o state vazio |
| `bench/env/baseline.sh` | Protocolo warm-up (10 min) + ramp (5 degraus de 1 min) + medido (1 h); `--plan` e `--dry-run` |
| `bench/cmd/bench-sink` | Receptor OTLP/HTTP descartável para validar o harness local sem gateway |
| `benchctl env` | `show`, `check`, `cost`, `tfvars`; `--json` para automação |
| `BENCHMARKS.md` | Pré-baseline publicada + perfil/custo T1 + protocolo + bloqueios |
| `bench/baselines/2026-11-01-local-100k/` | `report.md` + `report.json` da pré-baseline |
| `bench/README.md`, `bench/env/README.md`, `deploy/README.md` | Uso, segurança, custo, limitações e separação do Compose do produto |
| `Makefile` | `bin/bench-sink` no build e alvo `bench-env-check` |
| Correção | O relatório usava `string(Signal)` e publicava `"\u0000"` como nome do sinal; agora usa `Signal.String()` (métricas/logs/traces), com teste |

## AC1 — provisionamento

**Implementado:** perfil versionado, módulo Terraform de referência, script
idempotente (Terraform + `profile.auto.tfvars` regenerado), guards de gasto e de
credencial root, outputs de inventário e `destroy` com verificação.

**Validado sem gasto:**

- `terraform fmt -check -recursive` e `terraform validate` verdes com AWS
  provider 6.71.0.
- `benchctl env check` valida 6/3/6/4/2, capacidade e preços.
- `benchctl env tfvars` é determinístico e não inclui `admin_cidr`/chave SSH.
- `cluster.sh apply` sem `OBSERVAKI_BENCH_ALLOW_SPEND=1` recusa.
- `cluster.sh apply` com a credencial root atual recusa antes de qualquer
  chamada de criação.

**Não validado:** `terraform apply` real, segundo apply no-op, criação dos 21
nós e verificação de que o cluster aceita o workload. Isso exige aprovação de
gasto e uma credencial IAM dedicada.

## AC2 — baseline

**Publicado:** `BENCHMARKS.md` com a pré-baseline local e a seção T1 pendente.
A pré-baseline tem commit, hardware, config hash, seed, workload, resultados,
variância e veredito. O JSON bruto está versionado.

**Pré-baseline (commit `5b6096d`, 3 × 60 s, `throughput-100k`):**

- 99.932,3 / 99.934,1 / 99.939,0 eventos/s; média 99.935,1 (−0,065% do alvo);
- dispersão de 0,007% em eventos/s; p95 de envio 39,02 ms; p99 42,75 ms;
- CPU média 9,96%; RAM média 30,67%; compressão no fio 6,70x; payload 8,65x
  (132,27 gzip B/evento);
- zero erros, descartes, backpressure e pacing lag;
- relatório **pass** em todas as métricas comparadas.

**Não executado:** baseline T1 de 1M/s por 1 h. Depende de provedor/orçamento
(P-07), gateway (4.1), schema/ingestão ClickHouse (3.1–3.3), VictoriaMetrics
(3.4), Query Service (3.5) e caos (4.5). O `baseline.sh` está pronto, mas a
rampa contínua dentro de uma única execução do gerador ainda não existe; as
fases separadas são registradas no relatório.

## AC3 — custo e limpeza

**Implementado:** `cluster.sh cost` e `destroy` usam o mesmo modelo; o destroy
calcula as horas desde o `apply`, imprime o custo do run, destrói, verifica o
state e remove os arquivos locais. O `benchctl env cost --json` é a fonte para
automação.

**Estimativa do perfil T1 (snapshot AWS us-east-1, 2026-11-01):**

| Item | Valor |
|---|---:|
| Por hora | USD 41,79 |
| Run de 4 h | USD 167,15 |
| Mês (730 h) | USD 30.504,49 |
| Por TB/dia | USD 86,62 (11,58 TB/dia) |

**Achado:** o disco do ClickHouse domina o custo (6 × 32 TiB para 7 dias). O
addendum cita USD 300–800/nó/mês com 4 TB; essa conta cobre ~1 dia de 1M/s com
réplica 2, não 7. A diferença precisa entrar na decisão de retenção (P-02) e no
orçamento do benchmark.

**Não validado:** destroy real e verificação de recursos na AWS. O caminho foi
testado apenas com `terraform validate` e com os guards; não houve `apply`.

## Evidência de execução

| Verificação | Comando | Resultado |
|---|---|---|
| Suíte completa | `make ci` | ok (lint, testes, build web/Rust/Go) |
| Testes com corrida | `go test -race ./bench/internal/envprofile ./bench/cmd/benchctl ./bench/cmd/bench-sink` | ok |
| Perfil/capacidade | `./bin/benchctl env check` | ok; 192/36/2 TiB; 1,44M gateway; 1,92M loadgen |
| Custo | `./bin/benchctl env cost --hours 4` | USD 41,79/h; 167,15/4h; 30.504,49/mês |
| Terraform | `terraform fmt -check -recursive && terraform init -backend=false && terraform validate` | ok |
| Guard de gasto | `./bench/env/cluster.sh apply` | recusa sem `OBSERVAKI_BENCH_ALLOW_SPEND=1` |
| Guard de root | `OBSERVAKI_BENCH_ALLOW_SPEND=1 ./bench/env/cluster.sh apply` | recusa credencial root |
| Protocolo dry-run | `./bench/env/baseline.sh ... --dry-run` | 4 fases gravadas; relatório gerado |
| Pré-baseline real | `./bench/run.sh run throughput-100k --duration 60s ...` ×3 | 99.932–99.939/s; `status: ok` |
| Relatório | `./bench/run.sh report ...` | 3 runs, 1 grupo, **pass** |

Artefatos locais: `/tmp/opencode/bench-results-story24-final-20261102T014908Z`
e `/tmp/opencode/bench-report-story24-final` (fora do repositório). Os
consolidados publicados estão em `bench/baselines/2026-11-01-local-100k/`.

## Decisões e limitações

- **Perfil T1:** fecha a parte técnica da Q-01 do ADR-0006. A aprovação de
  provedor/orçamento/credencial continua pendente (P-07); o `apply` não é
  automático.
- **Provedor:** AWS é a implementação de referência porque o ambiente já tem
  Terraform e AWS CLI. O perfil e o contrato de capacidade são provider-neutral;
  Hetzner/OVH exigem um adapter Terraform e uma tabela de preços, não um novo
  perfil.
- **Single-AZ:** mede performance e custo do benchmark; não é o sizing de HA de
  produção. Cross-AZ mudaria latência e custo.
- **Retenção:** 168 h no perfil. Um run curto pode usar menos disco, mas a
  diferença tem de ser declarada; o custo de 7 dias é o número conservador.
- **Root AWS:** a conta atual tem credencial root. O script recusa; uma
  credencial IAM dedicada é pré-requisito.
- **Pré-baseline:** o sink local não é o gateway; CPU/RAM incluem o sink e o
  desktop; 60 s é curto; não há lag, perda, armazenamento ou consulta.
- **Rampa:** o protocolo separa warm-up/ramp/medido, mas cada fase é taxa
  constante. A rampa contínua no gerador fica para a execução T1.

## Bloqueios

1. **P-07:** aprovar provedor, teto de gasto e credencial IAM dedicada. Sem
   isso, `cluster.sh apply` permanece recusado por design.
2. **Stories 3.1–3.5 e 4.1–4.5:** sem gateway, storage, Query Service e caos não
   existe baseline T1 de ponta a ponta nem métricas de lag/perda/consulta.
3. **Rampa contínua:** implementar/validar no gerador antes de declarar o
   protocolo completo do ADR-0006.

## Próximo

Com P-07 aprovado e 3.1/4.1 prontos: preencher `terraform.tfvars` com uma
credencial IAM, rodar `cluster.sh check/cost/plan/apply`, executar
`baseline.sh --scenario throughput-1m`, publicar o resultado em `BENCHMARKS.md`
com o JSON e destruir o cluster com `cluster.sh destroy`. O baseline T1
substitui a seção "pendente"; a pré-baseline local permanece como evidência do
mecanismo.
