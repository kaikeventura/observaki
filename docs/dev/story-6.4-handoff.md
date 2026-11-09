# Story 6.4 — Orçamento de bundle no CI e Explore mock

**Status:** done (2026-11-09)
**Escopo:** `epics.md` (P0-E6, Story 6.4); DESIGN.md (query bar, signal tabs,
time range picker) e EXPERIENCE.md (filtros preservados e anunciados, atalhos,
URL reproduzível)
**Dependências:** Stories 6.1–6.3 (done)

## O que foi entregue

| AC | Resultado |
|---|---|
| **AC1 — Budget** | `web/scripts/bundle-budget.mjs` roda no fim de `make build` (e no CI): JS inicial **41,5 KB gzip** (< 180 KB) e rota de dashboards **74,4 KB** (< 250 KB). Um budget estourado sai com código 1 e imprime a rota; verificado com o limite rebaixado para 10 KB. |
| **AC2 — Lighthouse** | `web/scripts/lighthouse-check.mjs` (`make lighthouse`) mede `/explore` com perfil desktop de **50 Mbps / 40 ms RTT / 1x CPU** (1440×900): **LCP 390 ms** (< 1.500 ms) e **INP 81 ms** (< 200 ms); CLS 0,00007 e TBT 0. O INP vem de uma timespan run com interações reais (abas, autocomplete, serviço, range). |
| **AC3 — Mock Explore** | Abas Metrics/Logs/Traces preservam time range e filtro de serviço; a query é mantida quando pertence ao sinal e descartada com anúncio `aria-live` quando não pertence. O query bar autocompleta campos, valores, métricas e funções MetricsQL com `role="combobox"`, Arrow/Enter/Tab/Escape e ⌘Enter. Sinal, range, serviço e query vivem na URL e são restaurados no load. |

## Arquivos

```text
web/src/lib/explore/filters.ts          estado, URL, presets, vocabulário, sugestões, troca de sinal
web/src/lib/explore/filters.test.ts     11 testes (URL, presets, troca de sinal, autocomplete)
web/src/lib/explore/mock.ts             séries por métrica, filtros de log e preview determinístico de traces
web/src/lib/explore/mock.test.ts        7 testes (série, filtros, traces, status)
web/src/lib/components/QueryBar.svelte  combobox com autocomplete mock e ⌘Enter
web/src/lib/components/QueryBar.test.ts 3 testes (sugestões, teclado, anúncio)
web/src/lib/components/SignalTabs.svelte tablist com roving tabindex e setas/Home/End
web/src/lib/components/TraceTable.svelte preview nativa de até 50 traces
web/src/routes/explore/+page.svelte     mock Explore: query, filtros sticky, resultados por sinal, URL
web/src/routes/explore/page.test.ts     4 testes (render, persistência, drop/keep da query, autocomplete + URL)
web/src/lib/table/logs.ts               SERVICES → LOG_SERVICES exportado (fonte única dos serviços mock)
web/scripts/bundle-budget.mjs           gate de bundle (initial + dashboards, falha o build)
web/scripts/lighthouse-check.mjs        LCP (navigation) + INP (timespan) no /explore
Makefile                                alvos `bundle-budget` e `lighthouse`; budget no fim de `build`
.github/workflows/ci.yml                steps "Frontend bundle budget" e "Lighthouse"
web/package.json                        devDependencies lighthouse 13.6.0 + puppeteer-core 25.16.0
bench/frontend/README.md                contrato dos gates 6.4
bench/frontend/results/20261109T005535Z-local-explore/  report.md + bundle-budget.json + lighthouse.json
```

## Decisões de implementação

- **O que é "initial JS".** É o que o documento de entrada pré-carrega
  (`index.html`: runtime do SvelteKit, layout raiz e rota de entrada), não a
  soma de todos os chunks do build. A rota de dashboards usa o fecho completo,
  incluindo o `uPlot` importado dinamicamente, porque essa é a carga que o
  usuário realmente paga ao abrir a rota.
- **Budget no `make build`.** O AC pede que a regressão falhe o build; colocar
  o gate no fim de `build` faz `make ci` (e o smoke que roda `make ci`) falhar
  localmente e no CI. O step explícito no workflow deixa o número visível no
  log do job.
- **LCP e INP de fontes diferentes do Lighthouse, de propósito.** LCP sai de
  uma navigation run com throttling simulado (lantern) e INP de uma **timespan
  run** sem simulação, porque o Lighthouse só coleta INP em fluxo de usuário
  real (`supportedModes: ['timespan']`). O script dirige as interações pelo
  Puppeteer e o INP é o pior paint observado na janela.
- **`lighthouse` + `puppeteer-core` são devDependencies.** O AC nomeia o
  Lighthouse; ambos são ferramentas de CI (nada vai para o bundle). O critério
  de reversão está em `DECISIONS.md`: se o Lighthouse ficar instável no runner,
  o bench CDP próprio (padrão das Stories 6.2/6.3) mede os mesmos dois números.
- **Estado no URL, não em `$app/state`.** A página lê `location.search` no
  mount e grava com `history.replaceState`, então o mock continua testável em
  jsdom sem montar o router do SvelteKit e o link copiado reproduz a tela.
  Entradas inválidas (`signal=nope`, `range=1y`, serviço desconhecido) caem nos
  defaults em vez de quebrar a página.
- **Troca de sinal anuncia o que ficou e o que caiu.** `switchSignal` classifica
  a query pelo vocabulário do sinal (campos/funções que ela referencia):
  `severity:error` não vale para métricas, então é descartada com a mensagem
  explícita; `service:checkout` vale nos três sinais e é mantida. Nunca há drop
  silencioso.
- **Autocomplete por token.** O texto depois do último espaço é o token; as
  sugestões fazem prefix-match e substituem só esse token. Assim digitar
  `service:checkout sev` continua funcionando e o `Enter` aplica a opção ativa,
  enquanto `⌘Enter` sempre roda a query.
- **Sem dependência nova no produto.** A única mudança de dependência é de
  desenvolvimento (as duas ferramentas de CI); o app continua sem framework de
  UI além de Svelte/Tailwind/uPlot.

## Evidência

Comandos nesta máquina (Linux, Node 26.13.0, pnpm 12.16.1, Chrome 155):

```bash
make fmt
make lint            # ADR/OpenAPI ok, golangci-lint 0, prettier ok, svelte-check 0/0, clippy ok
make test            # Go ok; web 17 arquivos / 116 testes; cargo 2 testes
make build           # inclui o gate de bundle
make smoke           # SMOKE_SKIP_CI=1: health, OpenAPI, gateway, WAL, web build
make bundle-budget BUNDLE_BUDGET_OUT=bench/frontend/results/20261109T005535Z-local-explore/bundle-budget.json
make lighthouse LIGHTHOUSE_OUT=bench/frontend/results/20261109T005535Z-local-explore/lighthouse.json
```

Resultados publicados (`bench/frontend/results/20261109T005535Z-local-explore/`):

```text
initial JS          41,5 KB gzip (15 arquivos, 106,5 KB raw)   budget 180 KB  pass
dashboards JS       74,4 KB gzip (25 arquivos, 193,9 KB raw)   budget 250 KB  pass
gate negativo        budget 10 KB → exit 1 com a rota estourada  ok
Lighthouse LCP      390 ms (budget < 1.500 ms)                   pass
Lighthouse INP      81 ms (budget < 200 ms)                      pass
CLS / TBT           0,00007 / 0 ms                               —
transfer size       216,3 KB                                     —
```

## Limitações e próximo passo

- Não é G1: run local, dados sintéticos e Lighthouse de laboratório; LCP é
  simulado e INP é de uma sessão de interações, não de 28 dias de campo.
- A query não executa nada de verdade: o Query Service real é Fase 1
  (Story 8.3); o mock filtra logs por `severity:` e traces por `status:` para
  a tela responder ao que foi digitado.
- O painel de traces é uma preview de 50 linhas; waterfall, span detail,
  correlação e paginação keyset são Fase 1.
- O perfil do Lighthouse é fixo no script para ser comparável entre runs;
  mudá-lo exige atualizar o budget e registrar a decisão.
- Próximo passo: **Epic 7** (design partners) — a 6.4 fecha os spikes de
  frontend da Fase 0; o G1 continua dependente de P-07 e do baseline T1.
