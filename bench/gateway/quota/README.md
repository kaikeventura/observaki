# Gateway quota isolation (Story 4.2)

Mede a latência de um tenant "quieto" sozinho e sob o tráfego de um tenant
"barulhento" que envia muito acima da própria quota. O objetivo é validar o
comportamento de admissão da Story 4.2: 429 + `Retry-After`, buckets
independentes por tenant e shedding **antes** do decode quando o
`Content-Length` permite estimar os eventos.

```bash
make build
./bench/gateway/quota/run.sh --smoke          # harness check (~15 s)
./bench/gateway/quota/run.sh --small-payload  # workload completo local
make bench-gateway-quota                       # idem
```

## O que o script faz

1. Cria um snapshot temporário com dois tenants e tokens aleatórios por run.
2. Sobe `bin/observaki gateway` com auth, audit e reload.
3. Mede o tenant quieto sozinho (baseline) e depois com o tenant barulhento
   enviando `--noisy-rate` (default 1M/s) contra uma quota de `--quota`
   (default 100k/s).
4. Confere no `/metrics` que houve rejeições 429 e que o tenant quieto não
   recebeu erro.
5. Publica `report.json` + `report.md` em `results/<timestamp>/`.

O critério de aceite da história é o p95 do tenant quieto variar menos de 10%.
O script mede o p95 **cliente** (ponta a ponta) e também o p95 de processamento
no gateway, por tenant. Este último exclui upload/descompressão do cliente e
ajuda a diagnosticar o efeito.

## Limitação local importante

O gateway e os dois geradores de carga rodam **na mesma máquina** no run
local. Em 1M eventos/s, o gerador barulhento consome banda de memória e CPU
suficientes para reduzir a frequência do host e contaminar a latência do
tenant quieto. Por isso o script separa:

- **status funcional** (pass): houve 429 e o tenant quieto não falhou;
- **isolamento** (pass/inconclusive): só é `pass` se o p95 do quieto variar
  <10%. Fora isso, o resultado local é `inconclusive` e a medição definitiva
  fica para o cluster T1 com load generators em hosts separados (bloqueado por
  P-07).

`--small-payload` reduz o payload (0/64/0 B) para o host conseguir sustentar
1M/s sem saturar a banda de memória. `--pin` fixa gateway/geradores em CPUs
disjuntas quando há ≥12 CPUs; ele não resolve contenção de memória, mas
documenta o experimento.

## Resultados

Os resultados ficam em `results/` (não versionados) e são resumidos em
[`BENCHMARKS.md`](../../../BENCHMARKS.md). Nenhum número local deste harness é
evidência do G1.
