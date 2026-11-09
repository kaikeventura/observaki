# Baseline do ambiente do parceiro (Story 7.2, AC3)

O baseline é o **retrato do antes**: ferramentas, volume, custo e as 3 principais dores. Sem ele, o ganho do piloto vira opinião. A captura acontece no início do onboarding (Story 7.3), junto com o parceiro, e serve de insumo para o benchmark e para a priorização da Fase 1.

## O que capturar

| Bloco | Conteúdo | Por quê |
|---|---|---|
| **Ferramentas atuais** | vendor por sinal, plano, retenção, quem usa | comparação honesta e sem "AI magic" |
| **Volume** | eventos/s (pico e média 7d), GiB/dia de logs, spans/dia, séries ativas | escala real do piloto |
| **Custo** | fatura mensal de APM/observabilidade, armazenamento, esforço de operação | base para custo/1M eventos |
| **Top 3 dores** | dor, impacto, como é medida hoje | prioriza o roadmap e a demo |

O formulário versionado é [`partners/baseline.template.md`](../../partners/baseline.template.md). A cópia preenchida fica em `partners/baselines/{id}.md`, **fora do repositório** (contém dados do parceiro).

## Procedimento

```bash
make partners-baseline ID=dp-001   # cria partners/baselines/dp-001.md a partir do template
```

1. **Antes do primeiro dado:** capture o baseline na mesma call em que o token é criado — se o piloto começar antes, o "antes" se perde.
2. **Fonte de cada número:** anote de onde veio (métrica do Collector, painel do vendor, fatura). Baseline sem fonte não é evidência.
3. **Dores com exemplo:** peça um incidente recente e o tempo que levou para diagnosticar; dor sem exemplo costuma ser preferência de ferramenta.
4. **Retenção:** confirme a política vigente; o piloto usa 7 dias por padrão, conforme [`data-handling.md`](data-handling.md) e a cláusula 3 do [`agreement.md`](agreement.md).
5. **Revisão:** repita a medição no fim das 8 semanas com o mesmo método; a diferença é o resultado do piloto (Story 7.4).

## O que é público

Só agregados: eventos/s, p95, compressão, custo/1M eventos e variação percentual. Nome de empresa, serviço, host, fatura ou pessoa não entram no `BENCHMARKS.md` nem no relatório do pipeline sem aprovação escrita ([`agreement.md`](agreement.md), cláusula 3).

## Ligação com os outros artefatos

| Artefato | Papel |
|---|---|
| [`selection.md`](selection.md) | decide quem entra (inclui a declaração de volume) |
| [`agreement.md`](agreement.md) | cláusula 5 fixa a captura do baseline |
| [`pipeline.md`](pipeline.md) | `notes` guarda o ponteiro para o baseline; os números não vão para o CSV |
| `docs/dev/story-7.3-handoff.md` | onboarding consome este baseline (próxima story) |
