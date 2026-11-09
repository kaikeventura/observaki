# Story 6.3 — Tabela virtualizada com 10k linhas (run local, 2026-11-08)

**Máquina:** Linux, Chrome 155.0.8059.39 headless (`--headless=new`), viewport
1440×900, sem GPU dedicada. Node 26.13.0.
**Servidor:** `bin/observaki serve` com `OBSERVAKI_WEB_DIR=web/build` em
`127.0.0.1:8099`.
**Comando:**

```bash
make table-bench \
  TABLE_BENCH_OUT=bench/frontend/results/20261109T004040Z-local-10k/table-bench.json
```

## Resultado

| Critério | Medido | Limite | Passa |
|---|---|---|---|
| AC1 — linhas no DOM | **19 no topo**, **25 no máximo** durante o scroll (10.000 no total) | só a janela visível | ✅ |
| AC1 — fps de scroll (wheel real, 3 rodadas) | 60,7 / 60,2 / 60,5 (mediana **60,5**) | ~60 | ✅ |
| AC1 — pior frame durante o scroll | 16,8 ms | ~16,7 ms (60 fps) | ✅ |
| AC2 — ordenação por duração | `duration:asc` (120 → 121 ms) e `duration:desc` (4500 → 4440 ms) | ordem explícita | ✅ |
| AC2 — resize da coluna Service | teclado 150 → **182 px** (`aria-valuenow` 182); arraste 182 → **230 px** | pointer + teclado | ✅ |
| AC2 — estado preservado | largura 230 px e `duration:desc` sobrevivem à seleção; seleção sobrevive ao `Esc` | preservado | ✅ |
| AC2 — seleção e detalhe | `log-000759` abre o painel com time, level, service, duração, mensagem, trace/span e row id; `j` move para `log-007726` e `Enter` reabre | abre/fecha | ✅ |
| AC3 — vazio | “No logs match this filter.” + **Retry query**; retry volta às 10.000 linhas | estado + retry | ✅ |
| AC3 — erro | “The query failed.” + `Request ID req_01J8Z3K4` + **Retry query**; retry volta às 10.000 linhas | estado + retry | ✅ |

Contexto de performance:

- `idleFps` (rAF sem wheel) = 60,7; o scroll com wheel real mantém 60,2–60,7,
  então o custo do diff de ~19–25 linhas por frame cabe no vsync.
- Carga da rota: `domContentLoaded` em 94,5 ms e 12,0 KB transferidos; o heap
  inicial ficou em 14,4 MB (10k linhas de log sintéticas em memória).
- `scrollHeight` da tabela = 400.041 px (10.000 × 40 px + 41 px do cabeçalho);
  o scroller tem 544 px de altura e a janela é calculada por
  `scrollTop / 40`.
- CLS de carga = **0**. O CLS acumulado da sessão sintética ficou em **0,216**
  (0,210 depois do scroll e +0,006 no resize): o Chrome contabiliza a troca de
  linhas durante o scroll virtualizado como layout shift. Não é uma métrica de
  carregamento e não faz parte dos ACs da 6.3; o Lighthouse da rota entra na 6.4.
- Bundle: a rota `/explore` gera um nó de 22,82 KB (8,35 KB gzip) mais os chunks
  compartilhados; o orçamento inicial de 180 KB gzip é medido na Story 6.4.

## O que não está coberto

- Dados sintéticos determinísticos (10.000 logs, ~6 h), não o Query Service
  (Fase 1, Story 8.3).
- O scroll é medido com wheel real pelo CDP, mas o medidor de fps é
  `requestAnimationFrame` na página; não é um trace do compositor.
- Sem Lighthouse nem gate de bundle no CI: Story 6.4.
- O painel de detalhe é lateral em ≥1024 px e empilha abaixo em telas menores.

O JSON completo (`table-bench.json`) tem as três rodadas de scroll, as larguras
antes/depois de cada gesto, os textos dos estados e os registros de layout shift.
