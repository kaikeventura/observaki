# ADR-0004 — Frontend: SvelteKit + uPlot

- **Status:** aceito
- **Data:** 2026-10-31
- **Decisores:** Fundador + Frontend
- **Revisores:** kaikeventura (Fundador)
- **Consultados:** Produto + Design
- **Relacionados:** AD-8, AD-14, AD-34; D-04, D-06, P-04; S0.5; Stories 6.1–6.4
- **Substitui:** —
- **Substituído por:** —

## Contexto

A UI do Observaki é uma superfície densa de dados: Explore, dashboards,
waterfall de traces, tabelas virtualizadas e gráficos de séries temporais com
até 100k pontos. O orçamento é rígido (NFR-3):

- JS inicial < 180 KB gzip; dashboard < 250 KB;
- LCP < 1,5 s e INP < 200 ms;
- 100k pontos < 100 ms;
- WCAG 2.2 AA, navegação por teclado, foco visível e tabela alternativa para
  todo gráfico (NFR-10).

O frontend é uma SPA/build estático embutido no binário Go via `embed` (AD-8),
consome a API versionada e nunca acessa ClickHouse, VictoriaMetrics ou
PostgreSQL. O modo dev usa Vite proxy para a API.

O time do MVP é pequeno e a contratação é uma variável real. A escolha precisa
ser boa para UI densa, ter runtime pequeno e permitir um fallback claro se o
time contratado for majoritariamente React. A pendência P-04 (SvelteKit vs
React) continua aberta até a Semana 1.

## Decisão

**Adotar SvelteKit 3.1 + Svelte 5.57 + TypeScript strict + Vite 8 + Tailwind
4.3**, com uPlot 1.6.32 para séries temporais e CodeMirror 6 para o editor de
consulta.

Regras que decorrem da decisão:

1. **SPA estática embutida.** O build vai para `web/build` e é servido pelo
   binário Go (AD-8). Não há SSR obrigatório nem deploy separado.
2. **Configuração no plugin.** SvelteKit 3 removeu `svelte.config.js`; adapter
   e preprocess ficam no plugin `sveltekit()` do `vite.config.ts`.
3. **Acesso a dados centralizado.** Componentes não fazem `fetch` direto;
   usam `lib/api` com tipos gerados/contratados. Nada de `any` sem
   justificativa.
4. **Gráficos:** uPlot para séries temporais; ECharts apenas para
   heatmaps/gráficos complexos, carregado sob demanda; Chart.js e D3 fora do
   core. Todo canvas tem tabela alternativa e resumo textual.
5. **Segurança (AD-34):** CSP restritiva; sessão em cookie HttpOnly/Secure/
   SameSite ou token em memória com CSRF token; asset hashing e cache headers;
   timeouts de SSE/WS documentados.
6. **Orçamento por rota.** O CI mede o bundle inicial e o dashboard, roda
   Lighthouse/axe e falha em regressão (AD-14). O orçamento não é “média do
   app”: é por rota.
7. **Acessibilidade é definition of done.** Teclado, foco visível, contraste
   AA, estados nunca só por cor e tabela alternativa para gráficos.
8. **Fallback explícito.** Se P-04 concluir que o time é React-heavy, a
   alternativa é React 19 + Vite + TanStack + uPlot, com o mesmo orçamento e
   os mesmos contratos de API. A troca exige um ADR substituto, não um fork
   silencioso.

**Fora do escopo desta decisão:** design visual (DESIGN.md), arquitetura de
informação e fluxos (EXPERIENCE.md), biblioteca exata de componentes
(shadcn-svelte/bits-ui) e estratégia de internacionalização.

## Alternativas consideradas

| Alternativa | Prós | Contras | Por que não foi escolhida |
|---|---|---|---|
| **SvelteKit + Svelte 5 + uPlot** | Runtime pequeno, reatividade simples, ótimo para UI densa, build estático, uPlot leve e rápido | Ecossistema menor que React; contratação; menos bibliotecas prontas | **Escolhida**, condicionada ao S0.5 e à confirmação de P-04 |
| React 19 + Vite + TanStack + uPlot | Maior ecossistema, contratação fácil, muitas bibliotecas | Runtime maior; mais decisões de estado; bundle inicial maior | Fallback se o time for React-heavy (P-04) |
| Vue 3 + Vite | Meio-termo, bom ecossistema | Runtime maior que Svelte; menos momentum em observabilidade | Rejeitada |
| ECharts como gráfico principal | Rica, muitos tipos de gráfico | ~1 MB; pesada para 100k pontos e para o orçamento inicial | Apenas gráficos complexos sob demanda |
| Chart.js | Boa DX, tamanho moderado | Menos adequada a 100k pontos; canvas/SVG com custo | Rejeitada para o core |
| D3 puro | Máxima flexibilidade | Muito código customizado; acessibilidade e performance ficam por conta do time | Rejeitada para o core |
| Plugin Grafana | Ecossistema pronto | Não é um produto próprio; lock-in e arquitetura alheia | Rejeitada |

## Consequências

### Positivas

- Runtime pequeno e build estático, alinhados ao orçamento de 180 KB e ao
  embed no binário Go.
- uPlot entrega 100k pontos com custo baixo de CPU e memória.
- Svelte 5 runes simplifica estado local sem uma biblioteca pesada.
- O fallback React é tecnicamente viável sem mudar a API nem o contrato de
  dados.

### Negativas

- Ecossistema menor que React; algumas bibliotecas exigem adaptação.
- Contratação de Svelte é mais difícil em alguns mercados.
- uPlot é canvas: acessibilidade e tabela alternativa exigem trabalho
  deliberado, não vêm de graça.
- SvelteKit 3 ainda é novo; mudanças de configuração (como a remoção do
  `svelte.config.js`) exigem acompanhamento.

### Riscos aceitos e mitigações

| Risco | Probabilidade | Impacto | Mitigação | Dono |
|---|---|---|---|---|
| Bundle inicial estoura 180 KB | Média | Alto | S0.5 mede por rota; code splitting, lazy ECharts, auditoria no CI | Frontend |
| 100k pontos não ficam < 100 ms | Baixa | Alto | uPlot + downsampling no servidor; benchmark no S0.5 | Frontend + Query |
| Time contratado é React-heavy | Média | Alto | P-04 na Semana 1; fallback React 19 documentado | Fundador |
| uPlot não atende acessibilidade | Média | Alto | Tabela alternativa obrigatória, resumo textual, testes axe | Design + Frontend |
| SSE/live tail cortado por proxy | Média | Médio | Timeouts documentados, heartbeats e teste de longa duração | Plataforma |

## Status

`aceito` em 2026-10-31, com revisão registrada abaixo. SvelteKit + uPlot é a
escolha default; S0.5 prova o orçamento de bundle e de 100k pontos. O fallback
React 19 continua documentado caso o time contratado seja React-heavy (P-04).

**Evidência pendente para o G1 (não bloqueia a decisão):**

1. S0.5 provar 100k pontos < 100 ms e bundle < 180 KB gzip;
2. P-04 confirmar o time ou acionar o fallback React.

## Revisão

| Revisor | Papel | Data | Resultado | Referência |
|---|---|---|---|---|
| kaikeventura | Fundador | 2026-10-31 | aprovado | Revisão da sessão de 2026-10-31 |

## Questões abertas

| ID | Questão | Spike/story da Fase 0 | Dono | Prazo | Fallback |
|---|---|---|---|---|---|
| Q-01 | SvelteKit + uPlot entregam 100k pontos < 100 ms e bundle < 180 KB? | S0.5 / Story 6.2, 6.4 | Frontend | Semana 3 | React 19 + uPlot; ECharts lazy; downsampling no servidor |
| Q-02 | O time confirma SvelteKit ou aciona o fallback React? | P-04 / Story 6.1 | Fundador | Semana 1 | React 19 + Vite + TanStack + uPlot, mesmo orçamento |
| Q-03 | Qual o timeout de SSE/WS no ingress e no proxy de dev? | Story 6.1 / Story 5.1 | Plataforma | Semana 3 | Heartbeat e reconexão com backoff; documentar limite |
| Q-04 | Como medir axe/Lighthouse e orçamento por rota no CI? | Story 6.4 | Frontend | Semana 4 | Job dedicado com budget.json e falha em regressão |
| Q-05 | Qual a matriz de browsers suportada no MVP? | Story 6.4 / NFR-11 | Produto | Semana 4 | Últimas duas versões de Chrome/Firefox/Safari desktop |

## Referências

- PRD: FR-23, FR-24, FR-27; NFR-3, NFR-10, NFR-11; §4.7.
- Addendum: §4.6 (frontend), §4.7 (gráficos), §7 (spike S0.5).
- Arquitetura: AD-8, AD-14, AD-34.
- UX: `DESIGN.md`, `EXPERIENCE.md`.
- Revisão adversarial: `review-architecture.md`, achado M-02.
- Decisões: D-04 (stack), D-06 (UX), P-04 (SvelteKit vs React).

## Histórico

| Data | Mudança | Autor |
|---|---|---|
| 2026-10-31 | Criação (Story 1.4) | OpenCode |
