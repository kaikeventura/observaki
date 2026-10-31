# Architecture Decision Records (ADRs) — Observaki

Este diretório guarda as decisões de arquitetura que vinculam o produto. Um ADR
explica **por que** uma escolha foi feita, quais alternativas foram descartadas
e o que a decisão custa. O código pode mudar; o ADR também, mas nunca em
silêncio.

- **Template:** [`template.md`](template.md).
- **Invariantes de arquitetura:** [`ARCHITECTURE-SPINE.md`](../../_bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md)
  (AD-1..AD-35). Um ADR não substitui um invariante: ele registra e detalha a
  decisão que o aplica.
- **Registro executivo:** [`DECISIONS.md`](../../_bmad-output/planning-artifacts/DECISIONS.md)
  (D-xx confirmadas, P-xx pendentes, índice de ADs).
- **Processo de RFC:** [`CONTRIBUTING.md`](../../CONTRIBUTING.md#processo-de-rfc).

## Quando escrever um ADR

Escreva um ADR quando a mudança for difícil de reverter ou afetar mais de um
componente. Exemplos: storage, fila/WAL, gateway, frontend, tenancy, retenção,
contrato público, segurança, licença ou metodologia de benchmark.

Não use ADR para:

- tarefa de implementação, refactor local ou detalhe reversível;
- decisão que já está em um invariante sem mudança de interpretação;
- rascunho de ideia sem decisão (use issue/RFC).

Se a mudança contradiz um AD ou um invariante, **pare**: proponha um ADR novo
ou uma emenda. Nunca faça override silencioso.

## Ciclo de vida

| Status | Significado | Quem pode definir |
|---|---|---|
| `proposto` | Decisão escrita, ainda sem revisão. Pode sustentar spikes, não a Fase 1. | Autor |
| `em revisão` | Revisor designado está analisando. | Autor |
| `aceito` | Revisada por pelo menos um outro engenheiro registrado na seção **Revisão**. | Revisor + autor |
| `substituído` | Há um ADR mais novo que a substitui; o histórico permanece. | Autor do novo ADR |
| `rejeitado` | A alternativa foi analisada e descartada; o registro evita retrabalho. | Autor + revisor |

Regras:

1. **Numeração imutável.** O número (`NNNN`) é sequencial e nunca é
   reutilizado. Um ADR substituído continua no repositório com o link para o
   substituto.
2. **Um ADR, uma decisão.** Se a discussão virar três decisões independentes,
   divida em três ADRs.
3. **Revisão humana obrigatória.** `aceito` exige pelo menos um revisor que não
   seja o autor, com nome, data e resultado na seção **Revisão**. O autor pode
   ser o decisor, mas não o único revisor.
4. **Questões abertas têm destino.** Toda linha de **Questões abertas** aponta
   para uma spike (`S0.x`) ou story da Fase 0, com dono, prazo e fallback.
5. **Português-BR.** Produto e documentação em português; código, API e UI em
   inglês.
6. **Evidência.** Decisão que depende de benchmark cita o cenário, o commit e o
   relatório; enquanto o benchmark não existir, o status fica `proposto`.
7. **Mudança de decisão confirmada:** atualize o ADR (emenda) ou crie um
   substituto e atualize `DECISIONS.md`. Não renumere.

## Como criar um ADR

```bash
# 1. escolha o próximo número livre
ls docs/adr | sort

# 2. copie o template
cp docs/adr/template.md docs/adr/0007-meu-tema.md

# 3. preencha e valide
make adr-check
```

O `make adr-check` verifica seções obrigatórias, status válido, revisão
registrada para ADRs `aceitos`, questões abertas ligadas a spike/story e se
todos os arquivos aparecem no índice abaixo. O alvo roda no `make lint` e,
portanto, no CI.

## Como revisar

Checklist mínimo do revisor:

- [ ] O contexto explica a força que exige a decisão, com números e links?
- [ ] A decisão é verificável e diz o que está fora do escopo?
- [ ] As alternativas incluem “não fazer nada” e o motivo real da rejeição?
- [ ] As consequências negativas e os riscos têm dono e mitigação?
- [ ] A decisão conflita com algum AD-1..AD-35 ou com um non-goal do PRD?
- [ ] As questões abertas têm spike/story, dono, prazo e fallback?
- [ ] O status é compatível com a evidência (sem benchmark, sem `aceito`)?
- [ ] A decisão toca tenancy, retenção, auth, storage, RED ou upgrade? Então o
      AD correspondente foi citado?

Registre o resultado na seção **Revisão** com data e link. “Aprovado com
ressalvas” exige que as ressalvas virem questões abertas ou um follow-up
explícito.

## Índice

Os seis ADRs obrigatórios da Fase 0 (AR-12) estão abaixo. Todos nascem
`proposto`; a aprovação é item do Gate G1.

| ADR | Título | Status | Invariantes | Spike da Fase 0 |
|---|---|---|---|---|
| [0001](0001-storage-clickhouse-victoriametrics.md) | Armazenamento: ClickHouse + VictoriaMetrics | `proposto` | AD-5, AD-9, AD-21, AD-29 | S0.1, S0.4, S0.8 |
| [0002](0002-gateway-go.md) | Gateway de ingestão em Go | `proposto` | AD-1, AD-4, AD-6, AD-17, AD-21 | S0.2, S0.7 |
| [0003](0003-fila-nats-jetstream.md) | Fila/WAL: NATS JetStream | `proposto` | AD-4, AD-18, AD-19, AD-22 | S0.3 |
| [0004](0004-frontend-sveltekit-uplot.md) | Frontend: SvelteKit + uPlot | `proposto` | AD-8, AD-14, AD-34 | S0.5 |
| [0005](0005-tenancy-e-retencao.md) | Tenancy e retenção | `proposto` | AD-2, AD-3, AD-20, AD-28, AD-29, AD-35 | S0.8, S0.4 |
| [0006](0006-metodologia-de-benchmark.md) | Metodologia de benchmark | `proposto` | AD-14, AD-16, AD-29 | S0.1–S0.6 |

### ADRs futuros

Decisões já previstas no spine e ainda sem ADR próprio entram nas próximas
fases, sem pressa e sem renumerar:

| Tema | Invariantes | Fase |
|---|---|---|
| Contexto de authz e protocolo de consulta | AD-17, AD-23 | 1 |
| Contrato interno protobuf e upgrade N-1 | AD-22 | 1 |
| Config plane e entitlements | AD-19, AD-30 | 1 |
| Meta-observabilidade e alerting outbox | AD-24, AD-32 | 1/2 |
| Envelope operacional, backup/DR | AD-25 | 1/3 |
| Federação multi-cluster | — | 3/4 |

## Aprovação no Gate G1

O Gate G1 exige as seis ADRs **aprovadas** (ver `STATUS.md`). Enquanto o status
for `proposto` ou `em revisão`:

- o item do gate permanece aberto;
- a Fase 1 não começa;
- um spike pode concluir e propor emenda, mas não pode promover o ADR sozinho.

## Histórico

| Data | Mudança | Autor |
|---|---|---|
| 2026-10-31 | Criação do template, processo e seis ADRs obrigatórios (Story 1.4) | OpenCode |
