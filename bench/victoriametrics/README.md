# bench/victoriametrics — VictoriaMetrics, MetricsQL e cardinalidade

Este diretório mede a Story 3.4: ingestão de 1M amostras/s com 1M séries
ativas, 20 consultas MetricsQL representativas em janelas de 24 h e admissão
de cardinalidade por tenant com isolamento do vizinho.

> **Não é evidência do G1.** O run padrão é single-node e local. O baseline T1
> continua bloqueado por **P-07** e pelas Stories 4.1/4.3/4.5 (gateway, WAL,
> Query Service e caos). O perfil T1 tem 16 vCPU e 64 GiB; o host local é
> menor em RAM e não é o hardware de referência.

## O que roda

| Etapa | O que faz |
|---|---|
| Container | `victoriametrics/victoria-metrics:v1.148.0` com `-retentionPeriod=30d`, `-memory.allowedBytes=4GiB`, `-search.maxSamplesPerQuery=10B` e `-search.maxUniqueTimeseries=2M` |
| Correção | `vmquerybench verify` grava um dataset determinístico e valida `rate`, `increase`, `histogram_quantile` e agregação por label |
| Carga | `vmloadgen` envia Prometheus remote write (protobuf + snappy) a 1M amostras/s por 1 h, 16 workers e 5.000 séries por request |
| Consulta | `vmquerybench run` executa as 20 consultas de [`queries/catalog.tsv`](queries/catalog.tsv) em janelas de 24 h, com `nocache=1` |
| Cardinalidade | `vmadmission` aplica limite por tenant antes do VictoriaMetrics; tenant-a estoura o limite, tenant-b permanece abaixo e tem o p95 do vizinho comparado antes/depois |
| Coleta | RSS do processo, linhas inseridas, tamanho do diretório, séries ativas, erros e contadores do proxy |
| Relatório | `report.md` e `report.json` em `results/<timestamp>/` |

## Uso

```bash
# smoke local (≈1 min; 100k séries, 100k/s por 10 s)
make bench-victoriametrics-check
# ou
./bench/victoriametrics/run.sh --smoke

# run publicado da story (1M séries, 1M/s por 1 h; ≈1 h 10 min)
make bench-victoriametrics
# ou
./bench/victoriametrics/run.sh --reset \
  --results-dir "$PWD/bench/victoriametrics/results/2026-11-02-local-1m-1h"

# repetir só as consultas sobre um dataset já carregado
./bench/victoriametrics/run.sh --skip-load --skip-cardinality

# repetir só a cardinalidade
./bench/victoriametrics/run.sh --skip-load --skip-queries

# validar só a correção
./bin/vmquerybench verify --vm-url http://127.0.0.1:18428
```

O script preserva o container ao final para inspeção. Use `--cleanup` para
removê-lo; o diretório `/tmp/opencode/observaki-vm34` não é apagado. `--reset`
remove o container e os dados antes de começar. Para apagar os dados depois:

```bash
docker rm -f observaki-vm34
docker run --rm --user root -v /tmp/opencode/observaki-vm34:/data \
  --entrypoint /bin/sh victoriametrics/victoria-metrics:v1.148.0 \
  -c 'rm -rf /data/*'
```

## Contrato da carga

- **Séries determinísticas.** `bench/internal/vmbench` gera 500k séries por
  tenant com famílias de métricas de aplicação e infraestrutura. O total é 1M
  para dois tenants. Cada série tem `tenant_id` e `project_id`; os labels são
  ordenados e pré-codificados uma vez.
- **1 amostra/s por série.** Com 1M séries e 1M amostras/s, cada série recebe
  uma amostra por segundo. O gerador usa `time.Now()` no momento do batch e
  faz pacing por worker; o relatório publica a taxa alcançada.
- **Remote write canônico.** O payload é um `WriteRequest` protobuf com
  labels/samples, comprimido com snappy. O caminho é o mesmo que o gateway
  usará na Story 4.4; não há JSON nem import textual.
- **Distribuição das famílias.** `http_requests_total`, histogramas HTTP,
  `span_requests_total`, histogramas de span, métricas de processo, gRPC,
  filas e métricas de container. As consultas de dashboard filtram
  `tenant_id`/`project_id` como o Query Service fará em produção
  (AD-2/AD-17); só `m14`/`m15` são visões de operador.
- **Cache desligado nas consultas.** O run usa `nocache=1` para medir o
  caminho bruto, sem o rollup cache do VictoriaMetrics. É uma medida
  conservadora; um dashboard com cache tende a ser mais rápido.
- **Cardinalidade antes do ack (AD-33).** O proxy `vmadmission` conta séries
  únicas por tenant, rejeita o request inteiro com `429` e
  `code=cardinality_limit_exceeded` quando o limite seria ultrapassado, e
  expõe `observaki_cardinality_*` em `/metrics`. O gateway de produção
  (Stories 4.1/4.2) é o dono dessa decisão; este proxy é o spike.
- **Namespace de probe.** O teste de cardinalidade usa a métrica
  `observaki_cardinality_probe_total` para começar a contagem do proxy em zero
  e não depender do dataset principal.

## Resultado local

O run publicado fica em
[`results/2026-11-02-local-1m-1h/`](results/2026-11-02-local-1m-1h/).
O smoke de validação fica em
[`results/smoke-final/`](results/smoke-final/) (100k séries; artefato de
validação, não é o número publicado).

## Limitações

- Single-node local; sem réplica, vmauth, gateway OTLP, NATS, Query Service,
  row policies ou caos. O `vmadmission` é um proxy de spike, não o gateway.
- O dataset é sintético e determinístico; não passa pelo caminho OTLP real.
- O host local tem menos RAM que o perfil T1 (16 vCPU/64 GiB). O relatório
  publica o RSS real e o compara com o orçamento de 32 GiB (50% do nó T1).
- As consultas usam `nocache=1`; o número não inclui o ganho do rollup cache.
- O limite de cardinalidade é aplicado por request inteiro: um request que
  ultrapassaria o limite é rejeitado por completo, não parcialmente.
- O proxy conta séries aceitas e rejeitadas em memória; um restart zera a
  contagem. O control plane da Fase 1 precisa do catálogo durável.
