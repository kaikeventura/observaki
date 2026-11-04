# Gateway batching, compression and NATS WAL (Story 4.3)

Mede o caminho gateway → envelope protobuf → zstd → NATS JetStream e verifica
o contrato de durabilidade: o gateway só responde depois do ack do WAL, o
`walcheck` relê todas as mensagens sem dar ack e confere contagem, `event_id`
únicos, compressão e distribuição por shard.

```bash
make build
./bench/gateway/wal/run.sh --smoke            # harness check (~10 s)
./bench/gateway/wal/run.sh                    # 100k/s local, 2 tenants
./bench/gateway/wal/run.sh --rate 1000000 --duration 5s --small-payload
make bench-gateway-wal                         # idem ao workload de 100k/s
```

## O que o script faz

1. Sobe um NATS 2.15.2 com JetStream em Docker, com `max_payload: 16 MiB` e
   store temporário.
2. Cria dois tenants (`tenant-a`, `tenant-b`) com tokens aleatórios e quota
   igual à taxa total ofertada.
3. Sobe `bin/observaki gateway` com `OBSERVAKI_NATS_*` (shards configuráveis,
   linger de 5 ms) e espera `/readyz`.
4. Roda dois `otlploadgen` em paralelo, cada um com metade da taxa; o workload
   mistura métricas/logs/traces e usa gzip.
5. Confere os contadores do gateway, espera a fila zerar e encerra com SIGTERM
   (drain + flush).
6. `bin/walcheck` lê os três streams sem ack, valida o envelope v1 e publica
   `wal.json`.

O critério local é: zero eventos falhos/rejeitados, contagem do WAL igual à
contagem aceita, zero `event_id` duplicado, pelo menos 2 shards/subjects usados,
p99 de processamento do gateway < 50 ms. O status `t1_status` continua
`pending`: o AC1 pede 1M/s por 1 hora no cluster T1 e o P-07/baseline seguem
bloqueados.

## Resultados locais

O run de 100k/s por 10 s (2026-11-03, host de 22 CPUs) aceitou 1.000.000 de
eventos, `walcheck` leu 1.000.000 eventos únicos, compressão **5,46x**, 32
shards usados, maior subject com **0,95%** das mensagens e p99 de processamento
de **28,9 ms**. O run de 1M/s com payload mínimo sustentou ~740k/s no host
compartilhado, com p99 de 25,4 ms e 5M eventos íntegros; o payload de referência
não sustenta 1M/s localmente com os load generators na mesma máquina. Nenhum
número local é evidência do G1.

## Limitações

- NATS, gateway e load generators compartilham o host; CPU e memória competem.
- O store do NATS é temporário e o stream tem `MaxBytes` de 8 GiB por sinal no harness local (default de
  produção 16 GiB; o T1 configura o sizing);
  um outage de 15 min a 1M/s precisa do sizing do T1.
- O `walcheck` usa um consumidor efêmero sem ack; ele não altera a retenção
  do stream. Desde a Story 4.5 o consumidor é `AckExplicit` (retenção
  WorkQueue) e não confirma nada, então as mensagens continuam no WAL; ele
  não pode rodar ao mesmo tempo que o writer (a WorkQueue aceita um único
  consumidor por stream).
- O writer ClickHouse/VM e o teste de caos são as Stories 4.4/4.5.
