# Simulação do programa de design partners

**Para quê:** treinar o funil inteiro (tracker → validador → triagem → gate → relatório) sem candidatos reais. O dono declarou em 2026-11-08 que não há pessoas disponíveis para testar, então vale o critério de reversão do **P-01**: usar dados sintéticos e **adiar** o gate de produto do G1.

**O que a simulação prova:** o formato do tracker, a coerência de funil, a triagem dos cinco critérios, os cálculos de taxa, o relatório redigido e o gate — tudo rodando pelo mesmo código que a lista real usa.

**O que ela não prova:** que existe demanda, que o ICP responde, que alguém assina e que a stack do parceiro aguenta o volume. **AC1 (20 candidatos reais) e AC3 (5 calls, 3 assinados) continuam abertos**, e o item "3 design partners com dados reais" do G1 continua desmarcado.

## Como rodar

```bash
make partners-simulate                       # 20 candidatos, seed 7, data de hoje
make partners-simulate CANDIDATES=30 SEED=11 # outra coorte, resultado reprodutível
go run ./cmd/partnerssim -out /tmp/opencode/sim -as-of 2026-12-01
```

Saída em `partners/simulation/` (fora do git):

| Arquivo | Conteúdo |
|---|---|
| `pipeline.csv` | 20 linhas fictícias no formato real do tracker, ids `dp-sim-NNN`, domínios `.example.invalid` |
| `report.md` | relatório redigido do funil e da triagem, com banner de simulação |
| `README.md` | aviso de que o diretório é descartável e não é evidência |

O comando sai com código 1 se o funil simulado não atingir os alvos (20 qualificados, 5 calls, 3 assinados) — é o mesmo cálculo do gate real.

## A coorte gerada

Determinística (mesma seed, mesmo CSV) e desenhada para exercitar os dois lados da triagem:

| Estágio | Linhas | Observação |
|---|---:|---|
| `signed` | 3 | todos elegíveis (`yes`, volume ≥ 10k) |
| `call_done` | 3 | um com 6k/s, que a triagem reprova e a decisão pendente ainda aceita |
| `call_scheduled` | 2 | um com Kubernetes `planned` e volume `unknown` |
| `replied` | 4 | respostas de dados/feedback ainda `unknown` |
| `contacted` | 3 | um abaixo de 10k/s |
| `sourced` | 2 | volume `unknown`, exercitando o pré-filtro da 7.1 |
| `declined` | 2 | um por volume, um por não poder compartilhar dados |
| `no_response` | 1 | 3 tentativas sem resposta |

As datas ficam dentro das 8 semanas anteriores à data de referência; só uma call `call_scheduled` aponta para o futuro, como no uso real.

## Guarda anti-fraude

Uma coorte sintética nunca pode virar evidência:

```text
$ go run ./cmd/partnerscheck -file partners/simulation/pipeline.csv
partnerscheck: 20 simulated row(s) in partners/simulation/pipeline.csv (first: dp-sim-001);
synthetic data is not AC evidence — run the simulation with cmd/partnerssim or pass -simulation explicitly
# exit 1
```

O detector (`partners.IsSimulated`) marca como simulada qualquer linha com id `dp-sim-*`, `source` de simulação, domínio `.example.*`/`.invalid`, ou texto "fictício"/"simulado" em empresa, contato ou nota. Consequências:

- `make partners-gate` (lista real) **falha** se alguém colar linhas simuladas no tracker.
- `make partners-check` (CI) passa `-simulation` para o exemplo fictício versionado, e a saída fica prefixada com `SIMULATION`.
- O relatório gerado em modo simulação começa com o banner **SIMULAÇÃO — dados fictícios**, que não é removível por acidente ao commitar.

## Quando a lista real existir

1. `make partners-init` e preencher `partners/pipeline.csv` com pessoas reais.
2. Manter `partners/simulation/` apagado (ou ignorado, como já está).
3. `make partners-gate` — sem `-simulation` — e commitar só `partners/pipeline-report.md`.
4. Registrar a evidência no `STATUS.md`; a simulação não substitui esse passo.

## Limites e próximos usos

- A simulação **não** exercita baseline nem assinatura com valores fakes: esses artefatos (`make partners-baseline ID=dp-001`, `docs/partners/agreement.md`) são preenchidos com dados reais no onboarding (Story 7.3).
- Serve como demonstração do produto para a Story 7.4 (mostrar o funil e o relatório a um parceiro sem expor dados de ninguém).
- Se um dia houver candidatos reais, a coorte simulada continua útil como teste de regressão do validador.
