# bench/env/ — ambiente reproduzível do benchmark

Este diretório define o **cluster de referência T1** e o protocolo do baseline.
O perfil é versionado e provider-neutral; a implementação de referência usa
AWS + Terraform. Nada aqui é provisionado automaticamente pelo CI e nenhum
`apply` acontece sem uma decisão explícita de gasto.

> **Status (2026-11-01):** scaffolding validado (`benchctl env check`,
> `terraform validate`, custo, scripts e protocolo). O cluster **não foi
> provisionado** e o baseline T1 **não foi executado**. O bloqueio é a aprovação
> de provedor/orçamento e a existência do gateway (Story 4.1) e do schema
> ClickHouse (Story 3.1). O baseline local publicado em
> [`BENCHMARKS.md`](../../BENCHMARKS.md) é uma pré-baseline do harness, não
> evidência do G1.

## Perfil T1 (Q-01)

Fonte de verdade: [`t1.json`](t1.json). O perfil fecha a parte **técnica** da
questão Q-01 do ADR-0006 com o **mínimo** que atende ao contrato do G1 sem
esconder o custo. A aprovação de provedor, orçamento e credencial continua
pendente (P-07 em `DECISIONS.md`).

| Papel | Nós | Instância de referência | Disco | Para quê |
|---|---:|---|---|---|
| ClickHouse | 6 (6–12) | 16 vCPU / 64 GiB (`m6i.4xlarge`) | 2 × 16 TiB gp3, 500 MB/s, 16k IOPS | logs/traces, 7 dias, réplica 2 |
| VictoriaMetrics | 3 | 16 vCPU / 64 GiB (`m6i.4xlarge`) | 1 × 12 TiB gp3, 500 MB/s, 16k IOPS | métricas e RED, réplica 2 |
| Gateway | 6 (4–6) | 16 vCPU / 32 GiB (`c6i.4xlarge`) | root 100 GiB | OTLP, RED e cliente do WAL |
| NATS JetStream | 4 | 8 vCPU / 32 GiB (`m6i.2xlarge`) | 1 × 512 GiB gp3 | WAL, réplica 2, ≥1 h de outage |
| Loadgen | 2 | 16 vCPU / 32 GiB (`c6i.4xlarge`) | root 100 GiB | 5 shards do `throughput-5m` |

Premissas do workload: 1M eventos/s, 60% métricas (~40 B), 30% logs (~200 B),
10% traces (~500 B), 134 B/evento comprimidos, retenção de 168 h, single-AZ.
A capacidade calculada e validada por `benchctl env check` é:

- ClickHouse: 192 TiB raw / 96 TiB lógicos; 121 TiB exigidos pela retenção;
  +58% de headroom.
- VictoriaMetrics: 36 TiB raw / 18 TiB lógicos; 26,4 TiB exigidos; +36%.
- NATS: 2 TiB raw / 1 TiB lógico; 0,88 TiB exigidos para 1 h; +128%.
- Gateways: 1,44M eventos/s de capacidade a 15k eventos/s/vCPU.
- Loadgen: 1,92M eventos/s de capacidade a 60k eventos/s/vCPU.

**O que o perfil não é:** não é um sizing de produção multi-AZ, não inclui
PostgreSQL, object storage, backup, suporte, NAT gateway ou tráfego de saída.
O custo de HA é uma decisão separada e não entra no número do benchmark.

## Pré-requisitos

- Terraform >= 1.6 e AWS CLI v2.
- Credencial AWS **dedicada** (usuário/role de benchmark). Root é recusado pelo
  `cluster.sh apply`.
- Quota de vCPU On-Demand suficiente para 21 instâncias (320 vCPUs no perfil,
  mais o headroom da conta). A quota default de contas novas pode ser menor.
- Quota de EBS para ~40 TiB de gp3 e permissão para criar VPC, subnet, IGW,
  security group, key pair, EBS e EC2.
- Chave SSH pública e um CIDR de administração (`/32` de preferência).
- `make build` para gerar `bin/benchctl`.

## Provisionamento

```bash
make build
cp bench/env/terraform/terraform.tfvars.example bench/env/terraform/terraform.tfvars
$EDITOR bench/env/terraform/terraform.tfvars   # admin_cidr e ssh_public_key

./bench/env/cluster.sh check      # valida perfil, preços, capacidade e custo
./bench/env/cluster.sh cost       # estimativa para 4 h (ajuste com --hours)
./bench/env/cluster.sh plan       # terraform init + plan; não gasta
OBSERVAKI_BENCH_ALLOW_SPEND=1 ./bench/env/cluster.sh apply
./bench/env/cluster.sh status
./bench/env/cluster.sh output     # JSON com IPs, IDs e volumes
```

O `apply` imprime a estimativa, exige `OBSERVAKI_BENCH_ALLOW_SPEND=1`, recusa
root e pede confirmação digitada (`apply`) quando `--yes` não é passado. O
`terraform.tfvars` guarda apenas `admin_cidr`, `ssh_public_key` e tags; região,
zona, contagens, tipos e discos vêm de `t1.json` via
`benchctl env tfvars` → `profile.auto.tfvars`.

### Idempotência

- O estado é local (`bench/env/terraform/terraform.tfstate`, ignorado pelo git).
- `cluster.sh plan` gera um plano; `apply` refaz o plano antes de aplicar.
- Um segundo `apply` sem mudanças é um no-op do Terraform.
- `profile.auto.tfvars` é regenerado a cada execução; o perfil é a única fonte
  de verdade do shape.

### Limpeza e custo

```bash
./bench/env/cluster.sh destroy          # pede confirmação
./bench/env/cluster.sh destroy --yes    # automação
```

O `destroy` calcula as horas desde o `apply`, imprime a estimativa do run com o
mesmo modelo de custo, executa `terraform destroy` e verifica que o state ficou
vazio. Se o state não estiver vazio, sai com erro.

## Protocolo do baseline

[`baseline.sh`](baseline.sh) implementa o protocolo do ADR-0006 sem esconder o
aquecimento:

1. **warm-up:** taxa cheia por 10 min, não medido.
2. **ramp:** 10%, 25%, 50%, 75% e 100% da taxa, 1 min por degrau, não medido.
3. **medido:** taxa cheia por 1 h. É o número do gate.

```bash
# dry-run local, sem tráfego, para validar a orquestração
./bench/env/baseline.sh --scenario throughput-100k --rate 100000 \
  --duration 2s --warmup 1s --ramp-step 1s --ramp-steps 2 --dry-run

# T1 (depois do apply e das Stories 3.1/4.1)
./bench/env/baseline.sh --scenario throughput-1m \
  --endpoint http://<gateway-privado>:4318 \
  --results-dir /var/lib/observaki-bench/results
```

O relatório inclui warm-up, ramp e a execução medida; as fases de aquecimento e
rampa têm `--rate`/`--duration` diferentes e não entram na comparação de
variância. Use a última execução (duração cheia) como número do gate.

## Custo (Q-07)

[`prices.aws-us-east-1.json`](prices.aws-us-east-1.json) é um snapshot manual de
preços On-Demand de `us-east-1`, com data e fonte. O estimador soma instância,
EBS gp3 (root + dados), throughput acima de 125 MB/s, IOPS acima de 3.000 e IPv4
público por nó. Ele **não** inclui tráfego, snapshots, backup, suporte ou
impostos.

```bash
./bin/benchctl env cost --hours 4
./bin/benchctl env cost --hours 4 --json
```

O número é uma estimativa para decisão, não uma fatura. Antes de um `apply`, o
dono precisa aprovar provedor, orçamento e credencial. Para outro provedor,
basta um novo `prices.*.json` e um adapter Terraform; o perfil e o contrato de
capacidade não mudam.

## O que fica para as próximas stories

- **Instalação e configuração dos componentes:** ClickHouse/VM/NATS reais e o
  gateway não são instalados por este módulo. Os discos são anexados, mas não
  formatados; o passo de instalação (Stories 3.1–3.4 e 4.1–4.4) é quem define
  filesystem, config e versões.
- **Baseline T1:** depende de 3.1/4.1 e da aprovação de gasto.
- **Multi-AZ/HA:** fora do escopo do benchmark single-AZ; o custo e o RPO/RTO
  de produção são decisões separadas (Q-05).
- **Preços ao vivo:** o snapshot é revisável; não há chamada à Price List API.
