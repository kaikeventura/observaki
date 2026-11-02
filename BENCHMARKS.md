# Observaki — Benchmarks

> **Status (2026-11-02):** pré-baseline local do harness e spikes locais de
> armazenamento de logs (1B linhas) e traces (1B spans) publicados; baseline T1
> **não executado**. Nenhum número deste arquivo é evidência do Gate G1 até a
> execução no cluster T1 com gateway, armazenamento e o commit registrados.

Este arquivo segue o ADR-0006: todo número publicado declara commit, hardware,
configuração, seed, duração e veredito; falhas e limitações ficam visíveis; e
benchmark de fornecedor não entra sem reprodução própria.

## Resumo

| Item | Situação |
|---|---|
| Pré-baseline local do harness (100k/s, 3 × 60 s) | ✅ publicada; mecanismo de geração, transporte, coleta e relatório validado |
| Schema ClickHouse de logs (1B linhas, local) | ✅ publicado; 6,98x, `q03` p95 263 ms e TTL 7/15 ok; **não é G1** |
| Schema ClickHouse de traces (1B spans, local) | ✅ publicado; 7,52x, trace lookup p95 328 ms, serviço/status p95 178 ms e correlação ok; **não é G1** |
| Baseline T1 (1M/s, 1 h, 6 ClickHouse + 3 VictoriaMetrics + 6 gateways) | ⛔ bloqueado: provedor/orçamento e Stories 3.2–3.5/4.1–4.5 |
| Perfil T1 e capacidade | ✅ definidos e validados (`bench/env/t1.json`) |
| Provisionamento idempotente + destroy + custo | ✅ implementado e validado sem `apply`; execução bloqueada por gasto |
| Gate G1 | ⛔ aberto |

## Pré-baseline local do harness

**O que é:** três execuções de 60 s do `throughput-100k` contra um receptor
OTLP/HTTP local que apenas drena o corpo e devolve 200. Mede o gerador, o
transporte HTTP/gzip, a coleta de recursos e o gerador de relatório. **Não é o
gateway do produto**, não mede ingestão durável, lag, perda, armazenamento ou
consulta. É a evidência de que o mecanismo da Story 2.3 funciona de ponta a ponta
e de que o protocolo da Story 2.4 é executável.

### Identificação

| Campo | Valor |
|---|---|
| Commit do harness | `5b6096d` (limpo, sem alterações não commitadas) |
| Data (UTC) | 2026-11-02T01:49:08Z a 2026-11-02T01:52:08Z |
| Cenário | `throughput-100k`, 100.000 eventos/s, HTTP, gzip |
| Duração | 3 × 60 s (60,08 / 60,11 / 60,10 s) |
| Workload | 60% métricas, 30% logs, 10% traces; 134 B/evento comprimidos |
| Seeds | 1 (um shard) |
| Config hash | `sha256:f6b7069c2aabbb4011b1e06efa1011f2b1e6edf41d6f65a61d31cb2ab59d1978` |
| Endpoint | `http://127.0.0.1:4318` (bench-sink local) |
| Artefatos | [`bench/baselines/2026-11-01-local-100k/report.md`](bench/baselines/2026-11-01-local-100k/report.md) e [`report.json`](bench/baselines/2026-11-01-local-100k/report.json) |

### Hardware

| Campo | Valor |
|---|---|
| Host | `kabuntu-zenbook` (máquina de desenvolvimento, não é o T1) |
| CPU | Intel Core Ultra 7 155H, 22 CPUs lógicas |
| RAM | 15,0 GiB (16.092.491.776 B) |
| Disco | 441,1 GiB no total; 147,0 GiB livres no início/fim |
| Kernel | `7.0.0-38-generic` (linux/amd64) |
| Go | go1.27.2 |
| Binários | `benchctl`/`otlploadgen` `v0.0.1-alpha-17-g5b6096d` |

O receptor local roda na mesma máquina; CPU e RAM do host incluem o sink e
qualquer carga do desktop. O CPU/RSS do processo gerador é reportado à parte.

### Resultados

| Métrica | Execução 1 | Execução 2 | Execução 3 | Média / dispersão |
|---|---:|---:|---:|---:|
| Eventos/s | 99.932,3 | 99.934,1 | 99.939,0 | 99.935,1; 0,007% |
| Delta vs. alvo | −0,07% | −0,07% | −0,06% | −0,065% |
| Bytes/s (protobuf) | 109,0 MiB/s | 109,0 MiB/s | 109,0 MiB/s | 109,0 MiB/s |
| Bytes/s no fio (gzip) | 16,3 MiB/s | 16,3 MiB/s | 16,3 MiB/s | 16,3 MiB/s |
| Latência de envio p50 | 11,50 ms | 12,20 ms | 12,09 ms | 11,93 ms |
| Latência de envio p95 | 38,66 ms | 39,19 ms | 39,21 ms | 39,02 ms |
| Latência de envio p99 | 42,25 ms | 42,98 ms | 43,01 ms | 42,75 ms |
| CPU do host (média) | 9,64% | 10,14% | 10,11% | 9,96% |
| RAM do host (média) | 30,95% | 30,46% | 30,60% | 30,67% |
| Compressão no fio | 6,70x | 6,70x | 6,70x | 6,70x |
| Compressão do payload | 8,65x | 8,65x | 8,64x | 8,65x; 132,27 gzip B/evento |
| Erros | 0 | 0 | 0 | 0 |
| Eventos descartados | 0 | 0 | 0 | 0 |
| Backpressure | 0 | 0 | 0 | 0 |
| Pacing lag | 0 | 0 | 0 | 0 |

**Variância observada vs. tolerância documentada:**

| Métrica | Dispersão | Tolerância | Resultado |
|---|---:|---:|---|
| Eventos/s, bytes/s | 0,007% | ±5% | pass |
| Latência p50 | 5,93% | ±10% | pass |
| Latência p95 | 1,41% | ±10% | pass |
| Latência p99 | 1,77% | ±10% | pass |
| CPU média | 4,94% | ±10% | pass |
| RAM média | 1,60% | ±10% | pass |
| Compressão | 0,042% | ±5% | pass |
| Error rate | 0 p.p. | ±0,1 p.p. | pass |

### Veredito

**Pass para o mecanismo do harness.** As três execuções ficaram dentro da
tolerância documentada; a taxa ficou 0,06–0,07% abaixo do alvo; não houve erro,
descarte, backpressure ou pacing lag. O relatório marca as métricas de gateway,
lag de ingestão, perda confirmada, consulta e armazenamento como
`not_collected`, com a story responsável. Isso é intencional: o relatório não
inventa números.

**Não é um resultado do G1.** O receptor é um sink local; o hardware é um
laptop; a duração é de 60 s; não há ClickHouse, VictoriaMetrics, NATS ou
gateway. O G1 exige 1M/s por 1 h no cluster T1, lag p95 < 10 s e zero perda
confirmada.

## Story 3.1 — schema ClickHouse de logs (local, 1B)

**O que é:** o spike de schema da Story 3.1 em um ClickHouse 26.8 single-node
local, com 1B de linhas sintéticas em 24 h. Mede compressão, a consulta `q03`
(tenant + projeto + serviço + severidade + janela de 1 h + `LIMIT 100`) e a
expiração dos buckets de retenção de 7 e 15 dias. **Não é o gateway, não é o
cluster T1 e não é evidência do Gate G1.**

### Identificação

| Campo | Valor |
|---|---|
| Commit do schema/harness | `494391a` (limpo, sem alterações não commitadas) |
| Carga (UTC) | 2026-11-02T18:07:20Z a 2026-11-02T18:47:40Z (40 min 20 s; ~413k linhas/s) |
| Dataset | 1.000.000.000 linhas; 20 chunks × 50.000.000; 24 h ordenadas no tempo |
| Dimensões | 2 tenants × 4 projetos × 8 serviços; 90% INFO / 7% WARN / 3% ERROR |
| Corpo | 165,3 B de média; IDs de trace/span aleatórios |
| Retenção | 7 e 15 dias por linha (`retention_days`) |
| Hardware | `kabuntu-zenbook`, 22 CPUs, 15,0 GiB de RAM, disco de 441,1 GiB (42,6 GiB livres ao fim) |
| ClickHouse | 26.8.36.3 (`clickhouse/clickhouse-server:26.8`) |
| Schema | [`0001_otel_logs.sql`](internal/adapters/clickhouse/migrations/0001_otel_logs.sql), sha256 `2daca91e22cdbfe3cfb367fb8f8241e9839464653f2a0125f3352c35e7197e05` |
| Artefatos | [`report.md`](bench/clickhouse/results/2026-11-02-local-1b/report.md), [`report.json`](bench/clickhouse/results/2026-11-02-local-1b/report.json), [`handoff`](docs/dev/story-3.1-handoff.md) |

### Resultados

| Métrica | Valor |
|---|---:|
| Compressão | **6,98x** (77,55 GiB comprimidos / 541,09 GiB sem compressão) |
| Bytes comprimidos/linha | 83,3 B |
| Partes ativas | 1.921 (merges de background parados no run local) |
| `q03` p50 | 248,8 ms |
| `q03` p95 | **263,0 ms** (alvo < 1.000 ms) |
| `q03` p99 | 298,5 ms |
| Linhas lidas (média) | 21.282.816 |
| Bytes lidos (média) | 426.995.180 B |
| Linhas retornadas | 100 |
| TTL 7/15 | **pass**: 5 inseridas, 3 sobreviveram; expiradas de 7 e 15 dias removidas; a linha de 15 dias com 8 dias de idade sobreviveu |

### Veredito

**Pass local para o schema.** A compressão ficou 39,6% acima do alvo de 5x, a
`q03` ficou 3,8x abaixo do alvo de 1 s e os dois buckets de TTL removeram apenas
o que estava expirado. O run usou SQL para gerar o dataset, parou os merges de
background para caber no disco local e mediu a tabela com 1.921 partes; esses
fatos estão nas limitações do relatório.

**Não é um resultado do G1.** Não há gateway, NATS, Query Service, row
policies, réplica, caos nem o hardware T1. O baseline T1 continua bloqueado por
P-07 e pelas Stories 3.2–3.5/4.1–4.5.

## Story 3.2 — schema ClickHouse de traces (local, 1B)

**O que é:** o spike de schema da Story 3.2 em um ClickHouse 26.8 single-node
local, com 1B de spans sintéticos (100M traces de 10 spans) em 24 h. Mede a
busca de uma trace completa por `trace_id`, a busca por serviço + status em
24 h, a compressão, um service map básico e a correlação log↔trace. **Não é o
gateway, não é o cluster T1 e não é evidência do Gate G1.**

### Identificação

| Campo | Valor |
|---|---|
| Commit do schema/harness | `f6d6c41` (limpo, sem alterações não commitadas) |
| Carga (UTC) | 2026-11-02T19:05:20Z a 19:37:51Z (32 min 31 s; ~512 mil spans/s) |
| Dataset | 1.000.000.000 spans; 100.000.000 traces de 10 spans; 20 chunks × 50M; 24 h ordenadas no tempo |
| Dimensões | 2 tenants × 4 projetos × 9 serviços × 10 operações; 97/3% OK/ERROR em spans não-raiz e 99/1% em raízes; 10% com event e 5% com link |
| IDs | CityHash64, aleatórios na aparência e determinísticos |
| Retenção | 7 e 15 dias por linha (`retention_days`) |
| Hardware | `kabuntu-zenbook`, 22 CPUs, 15,0 GiB de RAM, disco de 441,1 GiB (66,6 GiB livres ao fim) |
| ClickHouse | 26.8.36.3 (`clickhouse/clickhouse-server:26.8`) |
| Schema | [`0002_otel_traces.sql`](internal/adapters/clickhouse/migrations/0002_otel_traces.sql), sha256 `ad5fe2074531bceeed37a01dadb787215931f1e99994923b6a29f3fc5faaeab3` |
| Artefatos | [`report.md`](bench/clickhouse/traces/results/2026-11-02-local-1b/report.md), [`report.json`](bench/clickhouse/traces/results/2026-11-02-local-1b/report.json), [`handoff`](docs/dev/story-3.2-handoff.md) |

### Resultados

| Métrica | Valor |
|---|---:|
| Compressão | **7,52x** (56,38 GiB comprimidos / 423,98 GiB sem compressão) |
| Bytes comprimidos/span | 60,5 B |
| Partes ativas | 1.701 (merges de background parados no run local) |
| Trace lookup p50 | 308,6 ms |
| Trace lookup p95 | **328,2 ms** (alvo < 1.000 ms) |
| Trace lookup p99 | 332,4 ms |
| Trace lookup linhas lidas | 6.430.720 |
| Trace lookup spans retornados | 10 |
| Serviço + status + 24h p50 | 169,9 ms |
| Serviço + status + 24h p95 | **177,8 ms** (alvo < 2.000 ms) |
| Serviço + status + 24h p99 | 180,5 ms |
| Serviço + status + 24h linhas lidas | 13.935.188 |
| Serviço + status + 24h spans retornados | 100 |
| Service map | 9 nós, 9 arestas; nós 207,6 ms, arestas 596,8 ms (amostra de 1% das traces) |
| Correlação log→trace | ok (10 spans) |
| Correlação trace→log | ok (1 log) |
| RED no ClickHouse | 0 tabelas, 0 MVs (AD-21) |

### Veredito

**Pass local para o schema.** A compressão ficou 50,4% acima do alvo de 5x, a
busca por trace ficou 3,0x abaixo do alvo de 1 s e a busca por serviço/status
ficou 11,2x abaixo do alvo de 2 s. O service map e a correlação funcionaram nos
dois sentidos. O run usou SQL para gerar o dataset, parou os merges de
background para caber no disco local e mediu a tabela com 1.701 partes; esses
fatos estão nas limitações do relatório.

**Não é um resultado do G1.** Não há gateway, NATS, Query Service, row
policies, réplica, caos nem o hardware T1. O baseline T1 continua bloqueado por
P-07 e pelas Stories 3.2–3.5/4.1–4.5.

## Cluster de referência T1 (pendente)

O perfil está em [`bench/env/t1.json`](bench/env/t1.json) e a documentação de
provisionamento em [`bench/env/README.md`](bench/env/README.md).

| Papel | Nós | Instância | Disco | Capacidade |
|---|---:|---|---|---:|
| ClickHouse | 6 (6–12) | 16 vCPU / 64 GiB | 2 × 16 TiB gp3, 500 MB/s, 16k IOPS | 192 TiB raw / 96 TiB lógicos; 121 TiB exigidos pela retenção de 7 dias; +58,7% |
| VictoriaMetrics | 3 | 16 vCPU / 64 GiB | 1 × 12 TiB gp3, 500 MB/s, 16k IOPS | 36 TiB raw / 18 TiB lógicos; 26,4 TiB exigidos; +36,3% |
| Gateway | 6 (4–6) | 16 vCPU / 32 GiB | root 100 GiB | 1,44M eventos/s a 15k eventos/s/vCPU |
| NATS JetStream | 4 | 8 vCPU / 32 GiB | 1 × 512 GiB gp3 | 2 TiB raw / 1 TiB lógico; ≥1 h de outage; +127,9% |
| Loadgen | 2 | 16 vCPU / 32 GiB | root 100 GiB | 1,92M eventos/s a 60k eventos/s/vCPU |

Premissas: 1M eventos/s, 60% métricas (~40 B), 30% logs (~200 B), 10% traces
(~500 B), 134 B/evento comprimidos, retenção de 168 h, réplica 2 no ClickHouse,
2 no VictoriaMetrics e 2 no NATS, single-AZ. O perfil **não** inclui PostgreSQL,
object storage, backup, suporte, NAT gateway ou tráfego de saída.

### Custo estimado

Snapshot On-Demand de `us-east-1` em 2026-11-01
([`bench/env/prices.aws-us-east-1.json`](bench/env/prices.aws-us-east-1.json)),
apenas para decisão; não é uma fatura.

| Item | Valor |
|---|---:|
| Custo por hora | USD 41,79 |
| Custo de um run de 4 h | USD 167,15 |
| Custo mensal (730 h) | USD 30.504,49 |
| Custo por TB/dia | USD 86,62 (11,58 TB/dia) |

O disco do ClickHouse domina o custo: 6 × 32 TiB gp3 para 7 dias de retenção.
O addendum do PRD cita ~USD 300–800 por nó/mês com 4 TB; essa conta cobre
aproximadamente **um dia** de 1M/s com réplica 2, não sete. A diferença é um
achado do benchmark, não um erro de arredondamento. Um run de 1 h usa o mesmo
cluster, mas pode ser reduzido com um perfil de retenção menor — a diferença
tem de ser declarada no relatório.

### Provisionamento e limpeza

```bash
make build
cp bench/env/terraform/terraform.tfvars.example bench/env/terraform/terraform.tfvars
# edite admin_cidr e ssh_public_key

./bench/env/cluster.sh check
./bench/env/cluster.sh cost --hours 4
./bench/env/cluster.sh plan
OBSERVAKI_BENCH_ALLOW_SPEND=1 ./bench/env/cluster.sh apply
# ... execute o baseline ...
./bench/env/cluster.sh destroy
```

O `apply` é idempotente via Terraform, exige `OBSERVAKI_BENCH_ALLOW_SPEND=1`,
recusa credenciais root da AWS e pede confirmação digitada. O `destroy` calcula
as horas desde o `apply`, imprime o custo estimado do run, destrói os recursos e
verifica que o state ficou vazio. O `terraform.tfvars` guarda só `admin_cidr`,
`ssh_public_key` e tags; o shape vem do perfil.

### Protocolo do baseline

```bash
./bench/env/baseline.sh --scenario throughput-1m \
  --endpoint http://<gateway-privado>:4318 \
  --results-dir /var/lib/observaki-bench/results
```

1. **warm-up:** 1M/s por 10 min, não medido.
2. **ramp:** 10%, 25%, 50%, 75% e 100% da taxa, 1 min por degrau, não medido.
3. **medido:** 1M/s por 1 h. É o número do gate.

O relatório inclui as três fases; warm-up e ramp têm `--rate`/`--duration`
diferentes e não entram na comparação de variância. O script pode ser validado
sem cluster com `--dry-run`.

### O que falta para publicar o baseline T1

1. **Aprovação de provedor, orçamento e credencial dedicada** (P-07 em
   `DECISIONS.md`). O perfil de referência é AWS; outra nuvem exige um adapter
   Terraform e uma tabela de preços, mas não muda o perfil.
2. **Gateway OTLP (Story 4.1)** para receber o workload e calcular RED.
3. **Ingestão ClickHouse (Story 3.3)** para medir lag e perda confirmada; o
   schema de logs (Story 3.1) já tem spike local com 1B linhas.
4. **VictoriaMetrics e cardinalidade (Story 3.4)** para as métricas e o RED.
5. **Query Service (Story 3.5)** para as 20 consultas de dashboard.
6. **Automação de caos (Story 4.5)** para o teste de falha e DLQ.
7. **Validação do protocolo de rampa no cluster:** o `baseline.sh` executa fases
   separadas de taxa constante; a rampa contínua dentro de uma única execução
   ainda não está no gerador. O número do gate só será publicado quando o
   protocolo completo for executado e as fases ficarem registradas.

## Reprodutibilidade

### Pré-baseline local

```bash
make build
./bin/bench-sink --addr 127.0.0.1:4318 &
for i in 1 2 3; do
  ./bench/run.sh run throughput-100k --duration 60s \
    --endpoint http://127.0.0.1:4318 \
    --results-dir /tmp/observaki-bench/results
done
./bench/run.sh report --results-dir /tmp/observaki-bench/results \
  --out /tmp/observaki-bench/report
```

O `bench-sink` é um receptor descartável: aceita qualquer POST, drena o corpo e
devolve 200. Ele não é o gateway e não deve ser usado como um.

### Cluster T1

```bash
./bin/benchctl env check
./bin/benchctl env cost --hours 4
./bench/env/cluster.sh plan
OBSERVAKI_BENCH_ALLOW_SPEND=1 ./bench/env/cluster.sh apply
./bench/env/baseline.sh --scenario throughput-1m \
  --endpoint http://<gateway-privado>:4318
./bench/env/cluster.sh destroy
```

## Limitações e regras de publicação

- Números de vendor não entram sem reprodução própria.
- Toda execução entra no relatório, inclusive `failed` e `blocked`; só
  `status: ok` e `mode: run` entram na comparação.
- O baseline T1 só é publicado com commit, hardware, versões, config hash,
  seed, custo e veredito.
- Regressão de throughput, latência, compressão ou bundle é bug P1.
- O custo do cluster é parte do resultado: performance sem custo não decide o
  gate.
- O relatório do baseline T1 substituirá a seção "pendente" acima; a
  pré-baseline local permanece como evidência do mecanismo, não como G1.
