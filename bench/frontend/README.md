# bench/frontend — gráfico com 100k pontos e tabela com 10k linhas

Este diretório mede os spikes de frontend da Fase 0:

- **Story 6.2:** o componente `TimeSeriesChart` (uPlot 1.6.32) com **100.000
  pontos**, interação a 60 fps e atualização ao vivo a 1 Hz sem recriar o plot.
- **Story 6.3:** o componente `VirtualLogTable` com **10.000 linhas**, somente a
  janela visível no DOM, scroll a 60 fps, ordenação, redimensionamento de
  colunas, seleção com painel de detalhe e estados de vazio/erro com retry.

> **Não é evidência do G1.** O run é local, em Chrome headless (rasterização por
> software, sem GPU) e usa dados sintéticos determinísticos. O critério do gate
> é "100k pontos < 100 ms; bundle inicial < 180 KB gzip" — o bundle fecha na
> Story 6.4.

## Gráfico (Story 6.2)

### O que roda

| Etapa | O que faz |
|---|---|
| Servidor | `bin/observaki serve` com `OBSERVAKI_WEB_DIR=web/build` em `127.0.0.1:8099` |
| Navegador | Google Chrome headless (`--headless=new`), viewport 1440×900, `--enable-precise-memory-info` |
| Protocolo | Chrome DevTools via WebSocket nativo do Node (sem Puppeteer/Playwright) |
| AC1 | Lê `data-chart-paint-ms` do primeiro frame após o plot de 100k pontos (orçamento < 100 ms) |
| AC2 | Mede fps de `requestAnimationFrame` durante hover (mousemove a 60 Hz), zoom (wheel a ~15 Hz) e pan (shift+wheel a ~15 Hz); valida o tooltip contra os dados e o zoom por arraste e por teclado |
| AC3 | Liga o modo ao vivo (1 Hz), confere que `data-chart-instances` continua 1, que o mesmo nó `<canvas>` sobrevive e amostra o heap JS a cada 15 s por 10 min |
| Relatório | JSON em `results/<timestamp>-local-100k/chart-bench.json` |

O componente usa **decimação min/max** por bucket de pixel para desenhar (~1
ponto por pixel CSS). O dataset continua com 100k pontos na memória; o que muda
é quantos pontos entram no path do canvas. O rodapé do painel mostra "Drawn N of
100.000" para deixar isso explícito.

## Uso do gráfico

```bash
# build do binário + web e run completo (10 min de soak)
make chart-bench

# run curto (sem soak), para iterar
make chart-bench CHART_BENCH_MINUTES=0

# contra um servidor já no ar
node web/scripts/chart-bench.mjs --url http://127.0.0.1:8099/dashboards --minutes 0

# gravando o relatório publicado
node web/scripts/chart-bench.mjs --minutes 10 \
  --out bench/frontend/results/20261108T210527Z-local-100k/chart-bench.json
```

O script sobe e derruba o servidor e o Chrome; sai com código 1 se algum
critério de `pass` falhar.

## Critérios do gráfico

| Check | Regra |
|---|---|
| `firstPaintUnder100ms` | primeiro frame com o plot < 100 ms |
| `interactionNear60fps` | o menor fps entre as 9 rodadas (3 de hover, 3 de zoom, 3 de pan) ≥ 55 |
| `tooltipCorrect` | timestamp cresce com o cursor, fica dentro da faixa visível e o valor é positivo |
| `keyboardZoom` / `keyboardPan` / `keyboardReset` / `dragZoom` | a faixa muda na direção esperada; reset volta exatamente à faixa dos dados |
| `liveReusesPlot` | `instances == 1`, o nó do canvas é o mesmo e `updates` cresce |
| `memoryStable` | crescimento do heap < 8 MiB e amplitude < 16 MiB depois do warm-up |

## Limitações do gráfico

- Sem GPU: o Chrome headless desta máquina rasteriza em software. O fill de área
  custava ~6 ms por frame e foi removido do spike; a linha mantém 60 fps.
- O medidor de fps é `requestAnimationFrame` na própria página; não é um trace do
  compositor. O probe com o canvas oculto separa o custo do desenho do custo do
  evento sintético.
- Ainda não há gate no CI: o orçamento de bundle e o Lighthouse entram na Story
  6.4.

## Tabela virtualizada (Story 6.3)

### O que roda

| Etapa | O que faz |
|---|---|
| Servidor | `bin/observaki serve` com `OBSERVAKI_WEB_DIR=web/build` em `127.0.0.1:8099` |
| Navegador | Google Chrome headless (`--headless=new`), viewport 1440×900 |
| Protocolo | Chrome DevTools via WebSocket nativo do Node (sem Puppeteer/Playwright) |
| AC1 | Confere `data-table-total` (10.000) e `data-table-rendered`; mede fps de `requestAnimationFrame` enquanto envia **wheel real** pelo CDP e amostra o número de `<tr data-row-id>` a cada frame |
| AC2 | Ordena por duração (asc/desc) e confere a ordem; redimensiona a coluna Service por teclado e por arraste real; seleciona uma linha, confere o painel, fecha com `Esc`, move com `j` e abre com `Enter`; confere que larguras, ordenação e seleção sobrevivem |
| AC3 | Aciona os cenários Empty e Error, confere o texto/ação de retry e o retorno às 10k linhas |
| Relatório | JSON em `results/<timestamp>-local-10k/table-bench.json` |

A tabela renderiza **somente a janela visível + 6 linhas de overscan** (19 no
topo, no máximo 25 durante o scroll) e reserva o resto com linhas espaçadoras.
As alturas de linha são fixas (40 px), então a janela é calculada por
`scrollTop / rowHeight`; o cabeçalho é sticky. A tabela é nativa
(`table`/`thead`/`tbody`) com `aria-sort`, `aria-current`, caption e uma região
`aria-live` para seleção, ordenação e resize.

### Uso da tabela

```bash
# build do binário + web e run completo (~10 s)
make table-bench

# gravando o relatório publicado
make table-bench TABLE_BENCH_OUT=bench/frontend/results/<timestamp>-local-10k/table-bench.json
```

O script sobe e derruba o servidor e o Chrome; sai com código 1 se algum
critério de `pass` falhar.

### Critérios da tabela

| Check | Regra |
|---|---|
| `virtualized` | 10.000 linhas no total, 1–59 linhas no DOM e `rendered == domRows` |
| `scrollNear60fps` | o menor fps das 3 rodadas de wheel ≥ 55 |
| `domStaysBounded` | nenhuma amostra durante o scroll passa de 59 linhas no DOM |
| `sortWorks` | `duration:asc` e `duration:desc` com as durações na ordem certa |
| `resizeByKeyboard` / `resizeByPointer` | a coluna Service cresce e `aria-valuenow` acompanha |
| `resizePreserved` / `sortPreserved` | larguras e ordenação sobrevivem à seleção |
| `selectionOpensDetail` | o painel abre com o id da linha selecionada |
| `escapeKeepsSelection` | `Esc` fecha o painel e mantém a seleção |
| `keyboardMovesSelection` / `enterOpensDetail` | `j` move uma linha e `Enter` abre o painel |
| `emptyStateRetries` / `errorStateRetries` | o estado aparece e o retry volta às 10k linhas |

### Limitações da tabela

- Dados sintéticos determinísticos; o Query Service real é Fase 1 (Story 8.3).
- O CLS de carga é **0**. O JSON também publica o CLS acumulado da sessão
  sintética (0,21 no run publicado): quase todo ele vem da troca de linhas
  durante o scroll programático por wheel, que o Chrome contabiliza como
  layout shift da tabela virtualizada. Não é uma regressão de carregamento e
  não faz parte dos ACs da 6.3; o CLS oficial da rota entra na 6.4 junto com o
  Lighthouse.
- O painel de detalhe aparece ao lado da tabela em ≥1024 px; em telas menores
  ele empilha abaixo, ainda dentro do mesmo frame.

## Orçamento de bundle e Lighthouse (Story 6.4)

Este diretório também publica o gate de frontend da Fase 0. O build de produção
é medido de duas formas independentes:

| Gate | Script | Critério |
|---|---|---|
| Bundle | `web/scripts/bundle-budget.mjs` (`make bundle-budget`) | JS inicial < **180 KB gzip** e rota de dashboards < **250 KB gzip**; falha o build |
| Lighthouse | `web/scripts/lighthouse-check.mjs` (`make lighthouse`) | `/explore` com 50 Mbps / 40 ms RTT, desktop 1x CPU: **LCP < 1,5 s** e **INP < 200 ms** |

### O que roda

| Etapa | O que faz |
|---|---|
| Build | `pnpm --dir web build` gera `web/build` com PRerender estático; `make build` chama o budget no fim |
| Budget | Lê o `<link rel="modulepreload">`/`<script>` de `index.html` (entrada) e de `dashboards.html`, percorre os imports de cada chunk (inclusive `import()` dinâmico) e soma o gzip nível 9 |
| Servidor | `bin/observaki serve` com `OBSERVAKI_WEB_DIR=web/build` em `127.0.0.1:8099` |
| Navegador | Google Chrome headless via `puppeteer-core` (devDependency) |
| LCP | Lighthouse **navigation** run (`throttlingMethod=simulate`, lantern) |
| INP | Lighthouse **timespan** run (`throttlingMethod=provided`), porque o INP só é coletado sem simulação; o script dirige troca de abas, autocomplete, filtro de serviço e time range |
| Relatório | `bundle-budget.json` + `lighthouse.json` em `results/<timestamp>-local-explore/` |

```bash
# gate local (roda dentro de `make build` e no CI)
make bundle-budget

# Lighthouse contra o build atual (sobe e derruba servidor e Chrome)
make lighthouse

# publicando o relatório
make bundle-budget BUNDLE_BUDGET_OUT=bench/frontend/results/<ts>-local-explore/bundle-budget.json
make lighthouse LIGHTHOUSE_OUT=bench/frontend/results/<ts>-local-explore/lighthouse.json
```

### Critérios

| Check | Regra |
|---|---|
| `results[].pass` (initial, dashboards) | JS gzip dentro do budget; o script sai 1 e imprime qual rota estourou |
| `checks.lcpUnderBudget` | LCP da navigation run < 1.500 ms |
| `checks.inpUnderBudget` | INP da timespan run < 200 ms (e observado; senão falha) |

### Limitações

- Não é evidência do G1: máquina local, dados sintéticos e Lighthouse de
  laboratório (o LCP é simulado e o INP é de uma sessão de interações, não de
  28 dias de campo).
- O perfil é fixo no script (50 Mbps, 40 ms RTT, 1x CPU, 1440×900) para o
  número ser comparável entre runs; mudar o perfil exige atualizar o budget e
  registrar a decisão.
- `lighthouse` e `puppeteer-core` são dependências de desenvolvimento; nenhuma
  delas entra no bundle.
