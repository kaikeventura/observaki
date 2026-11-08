# Story 6.1 — Shell SvelteKit, design tokens e navegação

**Status:** done (2026-11-08)
**Escopo:** `epics.md` (P0-E6, Story 6.1); FR de UI do MVP; `DESIGN.md` e `EXPERIENCE.md`
**Dependências:** Story 1.1 (done)

## O que foi entregue

| AC | Resultado |
|---|---|
| **AC1 — Shell** | O app abre com o shell: nav rail de 240 px (Overview pelo wordmark, **Explore**, **Dashboards**, **Alerts**, **Settings**), top bar de 56 px com o **project switcher** e um bloco de referência (**State gallery**, **System health**). O switcher lista dois projetos mock (`Acme / Production`, `Acme / Staging`), persiste a escolha em `localStorage` (`observaki.project`), anuncia a troca em `aria-live` e é restaurado após a hidratação. |
| **AC2 — Estados** | Componente `StateMessage` com sete estados (`loading`, `empty`, `error`, `timeout`, `partial`, `rate_limited`, `no_permission`), cópia acionável (mensagem + ação primária + secundária) e um frame reservado de `15rem` compartilhado por todos. A rota `/states` permite disparar cada estado. Medição no Chrome headless: **240 px** em todos os sete estados e **CLS 0,00032** na sessão inteira (limite "bom" < 0,1). |
| **AC3 — Acessibilidade** | Ordem de tabulação começa no skip link ("Skip to main content" → `#main-content`) e passa por todos os controles; **10 tab stops** verificados, todos com `outline: 2px solid rgb(34, 211, 238)` (`:focus-visible`), sem scroll horizontal em 1440×900. Todo controle é nativo (`a`/`button`), sem `tabindex="-1"`. `prefers-reduced-motion` desliga as animações do skeleton. |

## Arquivos

```text
web/src/app.css                              tokens do DESIGN (cores, tipografia, radius) + reduced motion
web/src/lib/projects.ts                      projetos mock, store, persistência e restore
web/src/lib/navigation.ts                    nav primária/referência + isActivePath
web/src/lib/states.ts                        cópia e frame dos estados (15rem)
web/src/lib/components/AppShell.svelte       shell: skip link, nav rail, top bar, main
web/src/lib/components/NavRail.svelte        nav rail (240px, item ativo com aria-current)
web/src/lib/components/NavIcon.svelte        ícones inline (sem dependência nova)
web/src/lib/components/ProjectSwitcher.svelte  switcher (fora do padrão: Esc/clique fora, aria-expanded)
web/src/lib/components/StatusPill.svelte     pílula com texto + marcador (nunca só cor)
web/src/lib/components/StateMessage.svelte   estados com frame fixo e ações
web/src/lib/components/SurfacePlaceholder.svelte  placeholder das superfícies da Fase 1
web/src/routes/+layout.svelte                AppShell + import do app.css + foco no main
web/src/routes/+page.svelte                  Overview (status da API + launcher)
web/src/routes/{explore,dashboards,alerts}/+page.svelte  superfícies com empty state
web/src/routes/settings/+page.svelte         projeto ativo + estado da API (loading/error/retry)
web/src/routes/states/+page.svelte           galeria de estados
web/src/routes/+error.svelte                 404/erro dentro do shell
internal/httpserver/static.go                resolve <route>.html e <route>/index.html
```

## Decisões de implementação

- **Rotas prerenderizadas em vez de SPA puro.** O `adapter-static` passou a gerar
  `build/<rota>.html`; o handler estático do Go resolve `<route>.html` e
  `<route>/index.html` antes do fallback `index.html`. Assim cada URL entrega o
  markup, o `<title>` e o CSS corretos no primeiro paint (sem flash de página
  sem estilo, que acontecia com o CSS carregado via JS), e o roteador do
  SvelteKit continua assumindo depois da hidratação. Sem mudança de AD: a API
  continua sem consultar ClickHouse/VM (AD-3) e a UI segue embutida no binário.
- **Bug latente corrigido: o `app.css` nunca era importado.** Desde a Story 1.1 o
  arquivo de tokens existia mas não era referenciado por layout nenhum, então os
  utilitários do Tailwind não eram emitidos no build. O import agora está no
  `+layout.svelte` raiz e o CSS entra no `<head>` de todas as rotas.
- **Restauração do projeto após a hidratação.** O store inicia sempre em
  `Acme / Production` (igual ao HTML prerenderizado) e aplica o valor do
  `localStorage` no `onMount`, evitando mismatch de hidratação.
- **Projetos mock explícitos.** O menu declara "Mock projects for the Phase 0
  shell"; a lista real vem de `GET /api/v1/projects` na Fase 1.
- **Sem dependências novas.** Ícones são SVG inline e os testes usam o que já
  existia (Vitest + Testing Library).

## Evidência

Comandos executados nesta máquina (Linux, Node 26.13, pnpm 12.16.1):

```bash
make fmt
make lint            # golangci-lint 0 issues, prettier ok, svelte-check 0/0, clippy ok
make test            # Go ok; web 8 arquivos / 27 testes; cargo 2 testes
make build
make smoke           # OK: healthz, API, OpenAPI, gateway OTLP/HTTP/gRPC, auth, quota,
                     #     web build, version, loadgen, chloadgen, bench scenarios
```

Verificação no Chrome headless contra o binário Go servindo `web/build`
(`OBSERVAKI_WEB_DIR=web/build`, `http://127.0.0.1:8099`):

```text
GET /, /explore, /dashboards, /alerts, /settings, /states, /health -> 200 text/html
<title> correto por rota (ex.: "Explore · Observaki"); 7 links na nav primária
tab stops: 10 (skip link -> #main-content, brand, 4 superfícies, 2 referências,
           project switcher, health)
outlines: 2px solid rgb(34, 211, 238) em todos; scroll horizontal: false
state gallery: loading/empty/error/timeout/partial/rate_limited/no_permission
               = 240 px cada; CLS acumulado 0,00032
project switch: Production -> Staging, localStorage observaki.project=project-staging,
               anúncio "Switched to Acme Staging."
```

Screenshots locais (não versionados): `/tmp/opencode/shell-explore.png` e
`/tmp/opencode/shell-states.png`.

## Limitações e próximo passo

- As superfícies Explore/Dashboards/Alerts são placeholders com empty state; o
  Explore mock e o orçamento de bundle ficam na Story 6.4, e o chart na 6.2.
- Nav rail colapsada (64 px) do breakpoint 1024–1279 e tema claro não entraram
  nesta story (dark-first, desktop-first); ficam para a UI da Fase 1.
- Galeria de estados não codifica o estado na URL (é superfície de referência).
- Sem gate automatizado de axe/contraste no CI ainda; a evidência acima é uma
  execução manual no Chrome headless. A Story 6.4 adiciona o budget de bundle.
- Próximo passo: **Story 6.2 — componente uPlot com 100k pontos**.
