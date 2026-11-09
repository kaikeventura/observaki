# bench/frontend — componente de gráfico com 100k pontos

Este diretório mede a Story 6.2: o componente `TimeSeriesChart` (uPlot 1.6.32)
com **100.000 pontos** renderizados, interação a 60 fps e atualização ao vivo a
1 Hz sem recriar o plot.

> **Não é evidência do G1.** O run é local, em Chrome headless (rasterização por
> software, sem GPU) e usa dados sintéticos determinísticos. O critério do gate
> é "100k pontos < 100 ms; bundle inicial < 180 KB gzip" — o bundle fecha na
> Story 6.4.

## O que roda

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

## Uso

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

## Critérios

| Check | Regra |
|---|---|
| `firstPaintUnder100ms` | primeiro frame com o plot < 100 ms |
| `interactionNear60fps` | o menor fps entre as 9 rodadas (3 de hover, 3 de zoom, 3 de pan) ≥ 55 |
| `tooltipCorrect` | timestamp cresce com o cursor, fica dentro da faixa visível e o valor é positivo |
| `keyboardZoom` / `keyboardPan` / `keyboardReset` / `dragZoom` | a faixa muda na direção esperada; reset volta exatamente à faixa dos dados |
| `liveReusesPlot` | `instances == 1`, o nó do canvas é o mesmo e `updates` cresce |
| `memoryStable` | crescimento do heap < 8 MiB e amplitude < 16 MiB depois do warm-up |

## Limitações

- Sem GPU: o Chrome headless desta máquina rasteriza em software. O fill de área
  custava ~6 ms por frame e foi removido do spike; a linha mantém 60 fps.
- O medidor de fps é `requestAnimationFrame` na própria página; não é um trace do
  compositor. O probe com o canvas oculto separa o custo do desenho do custo do
  evento sintético.
- Ainda não há gate no CI: o orçamento de bundle e o Lighthouse entram na Story
  6.4.
