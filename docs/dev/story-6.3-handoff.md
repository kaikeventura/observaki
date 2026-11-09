# Story 6.3 — Tabela virtualizada de 10k linhas

**Status:** done (2026-11-08)
**Escopo:** `epics.md` (P0-E6, Story 6.3); DESIGN.md (data-table: cabeçalho
sticky, linhas de 40 px, números à direita, seleção com acento) e EXPERIENCE.md
(j/k, Enter, Esc, foco visível, estado nunca só por cor)
**Dependências:** Story 6.1 (done)

## O que foi entregue

| AC | Resultado |
|---|---|
| **AC1 — Render** | A tabela recebe **10.000 linhas** e mantém **19 linhas no DOM** no topo, no máximo **25** durante o scroll (janela visível + 6 de overscan); o resto é altura reservada. O scroll com **wheel real** rodou a **60,2–60,7 fps** em 3 rodadas (mediana 60,5), com pior frame de 16,8 ms. |
| **AC2 — Colunas e seleção** | Ordenação explícita por qualquer coluna (duração asc/desc conferida nos dados), resize da coluna Service por teclado (150 → 182 px) e por arraste (182 → 230 px), com `aria-valuenow` acompanhando. Larguras e ordenação sobrevivem à seleção; a linha `log-000759` abre o painel de detalhe, `Esc` fecha mantendo a seleção, `j` move para `log-007726` e `Enter` reabre. |
| **AC3 — Estados** | Resultado vazio (“No logs match this filter.”) e erro (“The query failed.” + request id) usam o mesmo frame de altura reservada, com **Retry query** que volta às 10.000 linhas. |

## Arquivos

```text
web/src/lib/table/logs.ts                       geração determinística de 10k logs, colunas, sort, janela virtual, formatação
web/src/lib/table/logs.test.ts                  determinismo, sort/estabilidade, janela e limites de largura
web/src/lib/components/VirtualLogTable.svelte   tabela nativa, sticky header, virtualização, resize, seleção, detalhe, estados
web/src/lib/components/VirtualLogTable.test.ts  8 testes (DOM limitado, scroll, sort, resize, teclado, vazio/erro)
web/src/routes/explore/+page.svelte             spike no Explore com cenários Loaded/Empty/Error
web/src/routes/explore/page.test.ts             3 testes da rota (10k, vazio, erro/retry)
web/scripts/table-bench.mjs                     bench CDP sem Puppeteer: wheel, FPS, sort, resize, seleção, estados
web/vitest-setup.ts                             stub de ResizeObserver para o bind:clientHeight do jsdom
bench/frontend/README.md                        contrato dos dois benches de frontend
bench/frontend/results/20261109T004040Z-local-10k/  report.md + table-bench.json
```

## Decisões de implementação

- **Virtualização por altura fixa.** Linhas de 40 px (DESIGN comfortable) e
  janela calculada por `scrollTop / rowHeight`, com 6 linhas de overscan. O
  `tbody` tem uma linha espaçadora no topo e outra no fim para preservar a
  altura total (400.041 px) sem renderizar as 10k linhas. A tabela continua
  nativa (`table`/`thead`/`tbody`/`th`) e o cabeçalho é `sticky`.
- **Uma atualização por frame.** Eventos de scroll são coalescidos com
  `requestAnimationFrame`; a cada frame o Svelte troca só as linhas que entram
  e saem. O bench amostra o número de `<tr data-row-id>` em cada frame para
  provar que o DOM não cresce.
- **Sort explícito e estável.** A coluna ordenada expõe `aria-sort`; o primeiro
  clique começa ascendente (exceto Time, que começa descendente) e o segundo
  inverte. O desempate é o `id`, então a ordem é determinística.
- **Resize por pointer e por teclado.** A alça tem `role="separator"`,
  `aria-valuemin/max/now` e `tabindex`; setas ajustam 8 px (32 px com Shift) e
  `Home` volta à largura padrão. O arraste usa `pointermove` na janela, fora do
  caminho de render.
- **Seleção e detalhe sem modal.** Clique seleciona e abre; `j`/`k` movem sem
  abrir; `Enter` abre; `Esc` fecha e devolve o foco ao scroller. O painel é um
  `aside` rotulado, com IDs completos e “Copy” do trace ID. A linha usa
  `aria-current`, fundo `surface-3` e acento de 2 px — nunca só cor.
- **Estados no mesmo frame.** Vazio/erro substituem o corpo da tabela dentro de
  um contêiner com a mesma altura reservada (544 px + 32 px), então trocar de
  estado não desloca a seção de notas. O CLS de carga ficou em **0**.
- **Sem dependência nova.** O bench fala CDP pelo WebSocket nativo do Node. O
  único ajuste de infraestrutura é o stub de `ResizeObserver` no jsdom, usado
  pelo `bind:clientHeight` do Svelte 5.

## Evidência

Comandos nesta máquina (Linux, Node 26.13, pnpm 12.16.1, Chrome 155):

```bash
make fmt
make lint            # golangci-lint 0, prettier ok, svelte-check 0/0, clippy ok
make test            # Go ok; web 14 arquivos / 94 testes; cargo 2 testes
make build
make smoke
make table-bench TABLE_BENCH_OUT=bench/frontend/results/20261109T004040Z-local-10k/table-bench.json
```

Bench publicado (`bench/frontend/results/20261109T004040Z-local-10k/`):

```text
linhas totais / DOM    10.000 / 19 (máx. 25 no scroll)      pass
scroll wheel fps       60,7 / 60,2 / 60,5   mediana 60,5    pass
pior frame             16,8 ms                              pass
sort                   duration:asc 120→121 / desc 4500→4440  pass
resize                 teclado 150→182; arraste 182→230    pass
estado preservado      largura 230 e sort desc na seleção  pass
seleção/detalhe        log-000759; Esc mantém; j/Enter ok  pass
vazio e erro           retry volta às 10.000 linhas        pass
CLS de carga           0                                    —
```

## Limitações e próximo passo

- Dados sintéticos; o Query Service real é Fase 1 (Story 8.3).
- O CLS acumulado da sessão sintética é 0,216, quase todo da troca de linhas
  durante o scroll virtualizado. O CLS de carga é 0; o Lighthouse da rota entra
  na 6.4.
- Em telas <1024 px o painel de detalhe empilha abaixo da tabela.
- Próximo passo: **Story 6.4 — orçamento de bundle no CI e Explore mock**
  (depende de 6.1–6.3): abas de sinal, time range/serviço persistentes e
  autocomplete no query bar.
