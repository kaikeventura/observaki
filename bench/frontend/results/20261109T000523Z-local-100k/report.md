# Story 6.2 — uPlot com 100k pontos (run local, 2026-11-08)

**Máquina:** Linux, Chrome 155.0.8059.39 headless (`--headless=new`), viewport
1440×900, sem GPU dedicada. Node 26.13.0.
**Servidor:** `bin/observaki serve` com `OBSERVAKI_WEB_DIR=web/build` em
`127.0.0.1:8099`.
**Comando:**

```bash
node web/scripts/chart-bench.mjs --minutes 10 \
  --out bench/frontend/results/20261109T000523Z-local-100k/chart-bench.json
```

## Resultado

| Critério | Medido | Limite | Passa |
|---|---|---|---|
| AC1 — primeiro frame com 100k pontos | **11,4 ms** (plot 2,0 ms) | < 100 ms | ✅ |
| AC2 — fps de hover (mousemove a 60 Hz) | 60,1 / 60,1 / 60,1 | ~60 | ✅ |
| AC2 — fps de zoom (wheel a ~15 Hz) | 60,1 / 58,7 / 60,7 | ~60 | ✅ |
| AC2 — fps de pan (shift+wheel a ~15 Hz) | 60,6 / 60,0 / 60,1 | ~60 | ✅ |
| AC2 — tooltip | 01:38:08 → 914,71 req/s e 12:44:54 → 1.096,55 req/s, dentro da faixa | correto | ✅ |
| AC2 — zoom por teclado / arraste | faixa encolhe nos dois; reset volta exato à faixa dos dados | correto | ✅ |
| AC3 — atualização ao vivo | 5 atualizações em 5,2 s; `instances = 1`; mesmo nó `<canvas>` | sem recriar | ✅ |
| AC3 — heap em 10 min (40 amostras a cada 15 s) | 7–18 MiB, dente-de-serra do GC; sem crescimento monotônico | estável | ✅ |

Contexto de performance:

- `idleFps` (rAF sem eventos) = 60,5; `rasterProbe` (mesmo loop com o canvas
  oculto) = 60,1 → o custo por evento sintético é desprezível e o desenho
  acompanha o vsync.
- Carga inicial da rota: `domContentLoaded` em 105,7 ms, 10,5 KB transferidos
  (o chunk do uPlot é carregado sob demanda: 43,3 KB / 16,8 KB gzip).
- CLS da rota: 0,0096 (limite "bom" < 0,1).
- Pontos desenhados: 1.078 de 100.000 (decimação min/max, ~1 ponto por pixel
  CSS). O dataset completo continua na memória e o rodapé do painel mostra a
  contagem.

## O que não está coberto

- Sem GPU: a rasterização é por software. O preenchimento de área custava ~6 ms
  por frame nesse cenário e foi retirado do spike; a linha mantém 60 fps.
- Dados sintéticos determinísticos, não o Query Service (Fase 1).
- Sem Lighthouse nem orçamento de bundle no CI: Story 6.4.

O JSON completo (`chart-bench.json`) tem as 40 amostras de heap, as 9 rodadas de
interação e os estados antes/depois de cada gesto.
