# Critérios de seleção de design partner

**Story 7.2, AC1.** A Story 7.1 filtrava o ICP antes do contato; esta triagem decide **quem entra no piloto**. Os cinco critérios são verificáveis no tracker e o validador rejeita um parceiro assinado que falhe qualquer um deles (`make partners-check`).

## Os cinco critérios

| # | Critério | Campo do tracker | Valor que passa | Como confirmar |
|---|---|---|---|---|
| 1 | Usa OpenTelemetry | `uses_otel` | `yes` (o produto é OTel-native) | Collector/SDK em uso, ou adoção decidida na call; `planned` não basta |
| 2 | Roda Kubernetes | `uses_kubernetes` | `yes` (EKS, GKE, k3s, OpenShift) | Namespace/Helm chart existente; `planned` não basta |
| 3 | Volume ≥ 10k eventos/s | `volume_events_per_second` | inteiro ≥ 10000 | Número do ambiente atual ou do Collector; `unknown` não basta |
| 4 | Pode compartilhar dados | `can_share_data` | `yes` | Aceite explícito de rodar o piloto com telemetria de um ambiente não crítico |
| 5 | Pode dar feedback semanal | `can_weekly_feedback` | `yes` | Pessoa nomeada com 30 min/semana por 8 semanas, ou feedback escrito combinado |

`can_share_data` e `can_weekly_feedback` aceitam `yes`, `no` ou `unknown`; `yes` é o único valor que passa na triagem. Registrar `unknown` é melhor do que inventar — mas o candidato não pode ser assinado antes de virar `yes`.

## Quando a triagem acontece

| Estágio | O que já se sabe | Triagem |
|---|---|---|
| `sourced` / `contacted` | stack pública, hipótese de volume | pré-triagem da [`sourcing.md`](sourcing.md): OTel, K8s, volume aproximado |
| `replied` / `call_scheduled` | respostas de uma linha | completar os cinco campos com o que o candidato declarou |
| `call_done` | call de 30 min feita | fechar volume, dados e feedback na call; quem falhar vira `declined` |
| `signed` | acordo assinado | **os cinco critérios precisam estar `yes`/≥ 10k**, senão o validador falha |

Uma linha `declined` ou `call_done` pode falhar a triagem sem erro: recusar por volume, por dados ou por tempo é resultado normal do funil. O que não pode é **assinar** quem não passa.

## O que fazer com cada reprovação

| Reprovado em | Ação | Registro |
|---|---|---|
| sem OTel e sem plano | desqualificar antes do contato | `notes` com o motivo, 90 dias sem novo contato |
| sem Kubernetes e sem plano | idem | idem |
| volume < 10k/s | manter em `declined` salvo outro sinal forte de dor | `notes` com o volume real |
| não pode compartilhar dados | oferecer avaliação sem ingestão (benchmark + quickstart); sem piloto | `declined` com o motivo |
| sem tempo para feedback | propor feedback assíncrono a cada 2 semanas; se recusar, `declined` | `notes` |

## Desempate

Se mais candidatos passarem do que as 3 vagas do piloto, priorize nesta ordem:

1. **Dor declarada** com custo atual alto (fatura de APM, MTTR, ferramentas espalhadas).
2. **Volume** mais próximo de 100k/s — exercita mais a stack sem sair do suporte da Fase 0.
3. **Diversidade de stack** (Go, Node.js, Python, Java) para cobrir os SDKs da Fase 1.
4. **Facilidade de acesso** à pessoa que opera a stack (não só ao decisor).

## Verificação executável

```bash
make partners-check    # template + exemplo fictício, incluindo a triagem
make partners-gate     # lista real: AC1 (≥ 20 qualificados), triagem e AC3 (5 calls, 3 assinados)
go run ./cmd/partnerscheck -file partners/pipeline.csv -min-volume 10000
```

A saída do gate traz uma linha `screen:` com elegíveis, bloqueados e quantos falharam em cada critério, e o relatório redigido ganha a seção **Selection screen** — só contagens, sem nomes. O limiar é configurável com `-min-volume` para testar cenários (por exemplo, um piloto só com ≥ 50k/s).
