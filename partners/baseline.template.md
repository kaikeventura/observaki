# Baseline do ambiente — {parceiro} ({id})

> Story 7.2, AC3. Preenchido **no início do onboarding**, junto com o parceiro, antes de interpretar qualquer número do piloto. Cópia preenchida fica em `partners/baselines/{id}.md` (fora do git; contém dados do parceiro). Só números agregados entram em material público.

## Contexto

| Campo | Valor |
|---|---|
| Parceiro | {empresa} |
| Id do tracker | {id} |
| Data da captura | {data} |
| Quem respondeu | {nome, cargo} |
| Ambiente observado | {não crítico / staging / pré-produção} |
| Nº de serviços e hosts | {n} |
| Time de plataforma | {n} engenheiros |

## Ferramentas atuais

| Sinal | Ferramenta | Versão/plano | Observações |
|---|---|---|---|
| Métricas | {Datadog, Prometheus, CloudWatch…} | {plano} | {retenção, cardinalidade} |
| Logs | {…} | {…} | {retenção, volume} |
| Traces | {…} | {…} | {amostragem} |
| Alertas / on-call | {…} | {…} | {ferramenta, cobertura} |
| Dashboards / BI | {…} | {…} | {quem usa} |
| OpenTelemetry | {Collector, SDKs, nenhum} | {versão} | {quem mantém} |

## Volume

| Medida | Valor | Como foi obtido |
|---|---|---|
| Eventos/s (pico) | {n} | {métrica do Collector/vendor, janela} |
| Eventos/s (média 7d) | {n} | {métrica, janela} |
| Logs (GiB/dia) | {n} | {…} |
| Spans (milhões/dia) | {n} | {…} |
| Séries ativas | {n} | {…} |
| Retenção atual por sinal | {dias} | {política vigente} |

## Custo

| Item | Valor mensal | Fonte |
|---|---|---|
| APM/observabilidade | {USD} | {fatura, mês} |
| Armazenamento/retenção | {USD} | {fatura} |
| Licenças e add-ons | {USD} | {fatura} |
| Esforço de operação (h/mês) | {h} | {estimativa do time} |
| **Total** | **{USD}** | — |

## Top 3 dores

1. **{dor}** — impacto: {custo/tempo/risco}; como é medida hoje: {métrica ou relato}.
2. **{dor}** — impacto: {…}.
3. **{dor}** — impacto: {…}.

## Expectativa do piloto

| Campo | Valor |
|---|---|
| O que faria o piloto valer a pena | {…} |
| Métrica de sucesso acordada | {p95 de query, custo, MTTR, tempo de onboarding} |
| Sinais que entram primeiro | {logs / traces / métricas} |
| Restrições de dados | {PII, retenção, região} |

## Como medir o baseline

1. **Volume:** leia o `otelcol_receiver_accepted_*` do Collector do parceiro (ou o painel do vendor) na mesma janela de 7 dias; sem Collector, use a média do vendor e anote a fonte.
2. **Custo:** pegue a fatura do mês anterior; se o valor for rateado por time, registre a regra do rateio.
3. **Dores:** saem da call de onboarding, não de suposição; peça um exemplo de incidente recente com tempo de diagnóstico.
4. **Retenção:** confirme a política vigente — o piloto usa 7 dias por padrão ([`data-handling.md`](data-handling.md)).

## O que vai para o público

- Agregados: eventos/s, p95, compressão e custo/1M eventos — sem nome de empresa, host, serviço ou pessoa.
- Comparativo de custo só com aprovação escrita; caso contrário, publicamos a faixa agregada.
- Nada do tracker, do baseline ou de faturas entra no repositório.
