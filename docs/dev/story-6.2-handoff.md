# Story 6.2 — Componente uPlot com 100k pontos

**Status:** done (2026-11-08)
**Escopo:** `epics.md` (P0-E6, Story 6.2); DESIGN.md (panel, densidade, estados) e
EXPERIENCE.md (teclado, tabela alternativa, nada só por cor)
**Dependências:** Story 6.1 (done)

## O que foi entregue

| AC | Resultado |
|---|---|
| **AC1 — Render** | `TimeSeriesChart` recebe 100.000 pontos e mede o primeiro frame em **11,4 ms** (o plot em si leva 2,0 ms) no Chrome headless a 1440×900 — bem abaixo do limite de 100 ms. O uPlot entra por `import()` dinâmico, então o chunk de 43,3 KB (16,8 KB gzip) só carrega na rota de dashboards. |
| **AC2 — Interação** | Hover, zoom e pan rodam a **60 fps** (9 rodadas: 58,7–60,7 fps, mediana 60,1) com eventos reais de mouse/wheel. O tooltip lê o índice pelo `posToIdx` do próprio uPlot e mostra `2026-11-08 01:38:08 · 914,71 req/s`; o valor e o timestamp ficam nos `data-*` para o teste conferir. Zoom por arraste, por wheel e por teclado; pan por shift+wheel, setas e botões. |
| **AC3 — Atualização** | O modo ao vivo acrescenta 1 ponto/s na janela de 100k (arrays fixos, `copyWithin`), chama `setData` no mesmo plot: `instances = 1`, o mesmo nó `<canvas>` sobrevive e o heap oscila entre 7 e 18 MiB em 10 min (40 amostras) sem crescimento monotônico. |

## Arquivos

```text
web/src/lib/chart/series.ts                    geração determinística, append em janela fixa, zoom/pan/clamp, sumarização, decimação min/max
web/src/lib/chart/format.ts                    timestamps, eixo, valores compactos, duração, faixa
web/src/lib/chart/theme.ts                     tokens do DESIGN lidos do CSS (canvas não usa classe)
web/src/lib/components/TimeSeriesChart.svelte  painel (44 px), plot, tooltip, teclado, botões, tabela, medições
web/src/lib/components/fixtures/fakeUplot.ts   dublê do uPlot para jsdom
web/src/routes/dashboards/+page.svelte         superfície do spike (monta o gráfico só no browser)
web/src/routes/dashboards/ChartSpike.svelte    série de 100k, live 1 Hz, pausa/retoma, append manual
web/scripts/chart-bench.mjs                    bench via CDP (sem Puppeteer): paint, fps, tooltip, live, heap
bench/frontend/README.md                       contrato do bench
bench/frontend/results/20261109T000523Z-local-100k/  report.md + chart-bench.json
```

## Decisões de implementação

- **Decimação min/max por pixel.** 100k pontos em ~1000 px são ~100 amostras por
  pixel. O path desenha ~1 ponto por pixel CSS (mín. e máx. de cada bucket, em
  ordem de tempo), então picos continuam visíveis e o frame cabe no orçamento.
  O dataset completo permanece em memória; o rodapé mostra "Drawn 1.078 of
  100.000" para não esconder a decisão. O caminho decimado sempre inclui o
  primeiro e o último ponto da janela, senão o auto-scale do uPlot (usado quando
  o live reseta a escala) encolhia o eixo em um bucket de cada lado.
- **Sem preenchimento de área.** No Chrome headless sem GPU o fill translúcido
  custava ~6 ms por frame e derrubava o zoom para ~49 fps. A linha mantém 60 fps;
  o DESIGN não exige área. Se o fill voltar, precisa de reavaliação no T1/GPU.
- **Interação não recalcula a tabela a cada frame.** `summarize` e `visibleStats`
  varrem os pontos visíveis; eles são adiados 120 ms depois do último gesto
  (`settledRange`), junto com o anúncio em `aria-live`. Sem isso o hover/zoom
  caía para ~45 fps.
- **Teclado e botões são o caminho alternativo ao arraste.** Setas fazem pan,
  `+`/`-` zoom, `0`/`Home` reset, `End` vai para os últimos 10%; os mesmos gestos
  existem como botões. O anúncio do novo intervalo é feito uma vez, ao assentar.
- **Tabela acessível com 12 buckets.** O gráfico nunca é a única fonte: a tabela
  mostra início/fim/mín/máx/média/último por bucket da faixa visível, com
  `caption` explicando a agregação. Tooltip é `aria-hidden`.
- **Medição embutida.** O painel expõe `data-chart-paint-ms`, `data-chart-updates`,
  `data-chart-instances`, `data-chart-drawn` e a faixa/dados; o bench lê esses
  atributos em vez de instrumentar o código por fora.
- **Sem dependência nova além do uPlot 1.6.32.** O bench fala CDP pelo WebSocket
  nativo do Node (sem Puppeteer/Playwright).

## Evidência

Comandos nesta máquina (Linux, Node 26.13, pnpm 12.16.1, Chrome 155):

```bash
make fmt
make lint            # golangci-lint 0, prettier ok, svelte-check 0/0, clippy ok
make test            # Go ok; web 12 arquivos / 72 testes; cargo 2 testes
make build
make smoke           # healthz, API, OpenAPI, OTLP, auth, quota, web build, loadgen...
make chart-bench CHART_BENCH_MINUTES=10
```

Bench publicado (`bench/frontend/results/20261109T000523Z-local-100k/`):

```text
first paint        11,4 ms (plot 2,0 ms)      limite 100 ms        pass
hover fps          60,1 / 60,1 / 60,1
zoom fps           60,1 / 58,7 / 60,7
pan fps            60,6 / 60,0 / 60,1         mediana 60,1         pass
idle / rasterProbe 60,5 / 60,1 fps
tooltip            01:38:08 -> 914,71 req/s ; 12:44:54 -> 1.096,55 req/s   pass
keyboard/arraste   zoom, pan, reset exato e drag-zoom                 pass
live               5 updates em 5,2 s, instances 1, canvas reusado     pass
heap 10 min        40 amostras, 7-18 MiB (GC), sem crescimento        pass
CLS da rota        0,0096
```

## Limitações e próximo passo

- Sem GPU neste host: os fps acima são de rasterização por software; com GPU o
  fill de área pode voltar, mas precisa de medição própria.
- Dados sintéticos; o Query Service real é Fase 1 (Story 8.3).
- O orçamento de bundle e o Lighthouse entram na Story 6.4; o chunk do uPlot é
  carregado sob demanda e hoje soma 16,8 KB gzip.
- Próximo passo: **Story 6.3 — tabela virtualizada de 10k linhas**.
