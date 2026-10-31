# Revisão adversarial de UX — Observaki

**Data:** 2026-10-31  
**Escopo:** `DESIGN.md`, `EXPERIENCE.md`, `mockups/explore.html`, `mockups/dashboard.html`, `mockups/service-map.html`, `prd.md`  
**Método:** leitura cruzada PRD ↔ spines ↔ mockups, inspeção estática de HTML/CSS, cálculo de contraste WCAG 2.x (luminância relativa) e confronto de cada fluxo com as fases do PRD.  
**Limite:** os mockups são composições estáticas, sem JavaScript. Não foi possível testar comportamento real, leitor de tela ou performance. Os achados de interação são, portanto, sobre o que o artefato **prova ou deixa de provar**, não sobre um produto em execução.

---

## Veredito

**A direção visual é forte e plausível, mas o conjunto ainda não fecha como design de produto: o MVP não consegue executar o fluxo que o justifica, 6 das 9 superfícies da IA e todos os 13 estados críticos existem apenas como texto, e os três mockups falham em contraste, teclado, reflow e integridade de dados — é um belo painel de instrumentos que ainda não sobrevive a um incidente real.**

---

## O que está forte

- **Postura visual coerente.** Dark-first, superfícies discretas, cyan para interação, âmbar para warning e vermelho para erro formam um sistema legível à primeira vista. A densidade das tabelas é adequada ao público e a tipografia mono/tabular para valores, IDs e logs é a escolha certa.
- **Vocabulário de estado e voz.** A tabela de estados da `EXPERIENCE.md` (linhas 87–103) e a voz direta, sem “oops” e sem exclamações (linhas 52–65), são exatamente o tom de um produto de incidente. As mensagens de timeout, permissão e trace amostrado são boas no papel.
- **Intenção de correlação.** Query bar + signal tabs + drawer + waterfall + logs + métricas no mesmo contexto é a arquitetura de informação correta para “não trocar de ferramenta”. O drawer de detalhe em vez de modal e a ênfase no caminho crítico são decisões acertadas.
- **URL state e deep link.** A intenção de colocar tempo, filtros, sinal e span selecionado na URL (EXPERIENCE, linhas 39 e 125) é o que permite Slack → investigação → handoff. É a fundação certa; falta especificar o contrato.
- **Mockups visualmente calmos.** Os três arquivos são consistentes entre si na shell, na nav e na hierarquia de página. O problema não é a estética; é a cobertura, a veracidade dos dados e a operabilidade.

---

## Sumário de achados

| ID | Severidade | Achado | Onde dói |
|---|---|---|---|
| C1 | Crítico | O MVP não executa o UJ-1 que o justifica; o fluxo depende de fases 2/3 e de um rollback externo | Proposta de valor e aceitação do MVP |
| C2 | Crítico | Surface closure é afirmação, não cobertura: 3 de 9 superfícies têm mockup e os fluxos não passam pelas superfícies mapeadas | Implementação e journeys |
| C3 | Crítico | Query bar, chips de filtro e tabela contam três histórias diferentes | Confiança no diagnóstico |
| C4 | Crítico | Contraste e modo claro falham WCAG 2.2 AA antes de qualquer implementação | Acessibilidade e risco legal |
| C5 | Crítico | O mapa de serviços é uma imagem: desalinha em larguras reais, não tem hit area, zoom ou seleção | Centerpiece do UJ-1 |
| A1 | Alto | 13 estados críticos, zero estados desenhados | Incidente real |
| A2 | Alto | A hierarquia de ação serve autoria, não resposta a incidente | MTTA/MTTR |
| A3 | Alto | Tempo e contexto não sobrevivem à rolagem/navegação | Janela errada |
| A4 | Alto | O dashboard não prova visualmente a causalidade que promete | Decisão de rollback |
| A5 | Alto | Onboarding é a superfície menos desenhada, apesar de ser a métrica nº 1 | TTFV < 5 min |
| A6 | Alto | Teclado, semântica e alternativas de gráfico não são demonstráveis | Acessibilidade operacional |
| A7 | Alto | Responsivo é aspiracional; mobile “read-only” contradiz o único trabalho mobile | Tablet/zoom/mobile |
| A8 | Alto | Taxonomia de status/severidade inconsistente e sem thresholds | Priorização |
| A9 | Alto | Sampling, parcialidade e confiança estão escondidos ou superprometidos | Falso “completo” |
| A10 | Alto | Performance e densidade prometidas não são provadas pelos mockups | Sensação de “instantâneo” |
| M1 | Médio | Tokens de tipo, raio, spacing e peso derivam dos spines | Design system |
| M2 | Médio | Contratos de componente ausentes nos mockups | Reuso e aceitação |
| M3 | Médio | Comps estáticos não validam URL state, drafts, foco, ARIA ou loading | Fidelidade do artefato |
| M4 | Médio | Racional de design virou copy de UI nos rodapés | Ruído em incidente |
| M5 | Médio | Números, unidades e fusos inconsistentes | Comparabilidade |
| M6 | Médio | IA/nav contradiz o próprio spine (7 vs 9 superfícies, Incidents, Usage, topbar) | Navegação |
| M7 | Médio | RBAC/read-only não aparecem: viewer vê Edit/Share/Add panel | Segurança e permissão |
| M8 | Médio | URL com query, cache offline e token no snippet têm risco de privacidade | Vazamento de dados |
| M9 | Médio | Ações “mágicas” sem destino: Request access, background job, health page | Dead ends |
| M10 | Médio | Preview de alerta chama matches de “real” sem mecanismo | Confiança e ruído |
| B1 | Baixo | 71% + 28% = 99%; “exhaust in 3d” não fecha com 6,2x | Credibilidade |
| B2 | Baixo | Drift narrativo: 12 vs 14 min, 80% vs 88%, 18 vs 6 spans | Credibilidade |
| B3 | Baixo | Gradientes/sombras/1px outline contra “no gradients/no shadows/2px” | Consistência visual |
| B4 | Baixo | Ícones são glifos Unicode ambíguos, sem sistema | Escaneabilidade |
| B5 | Baixo | Sticky header não funciona com o scroll container atual | Tabelas longas |

---

## Achados Críticos

### C1 — O MVP não consegue executar o UJ-1 que justifica o produto

**Evidência**

- O Fluxo 1 (`EXPERIENCE.md`, linhas 163–168) termina com: métrica do pool no drawer com tendência de **1h**, **anotação de deploy** 12 minutos antes do alerta, **rollback** de Marina, “**She never left Observaki**” e “**links the trace to the incident**”.
- No PRD, eventos de mudança/deploy são **Fase 2** (`FR-6`, linhas 180–187; `FR-39`, linhas 526–533). SLOs e burn-rate são **Fase 2** (`FR-33`, linhas 465–472; `FR-34`, linhas 474–481). Timeline de incidente é **Fase 3** (`FR-38`, linhas 517–524).
- O PRD não define ação de deploy/rollback no produto; existe apenas change tracking. “Marina rolls back” é uma ação externa, mas o fluxo afirma que ela não saiu do Observaki.
- O Fluxo 4, marcado como Fase 1 (`EXPERIENCE.md`, linhas 197–207), usa **PagerDuty** (FR-31, Fase 2, linhas 446–453), **runbook** (FR-35, Fase 2, linhas 483–490) e “alert appears in Alerts list with an owner and a runbook” — owner/runbook também são Fase 2.
- Os mockups reforçam o vazamento: `dashboard.html` mostra **SLO burn** (linhas 169–176), **deploy/rollback annotations** (linhas 179–190), **owner** (linha 103) e itens de nav **Incidents** e **Usage & cost** (linhas 93–96).

**Impacto**

O produto não consegue demonstrar a própria promessa com o escopo do MVP. Ou o time antecipa quatro ou cinco features de Fase 2/3 para fazer o fluxo funcionar, ou o design partner executa um UJ-1 que termina em dead end. A frase “sem trocar de ferramenta” vira “diagnostica aqui, resolve em outro lugar” — e a narrativa de valor do PRD (§1, linhas 18–24; UJ-1, linhas 50–56) não se sustenta. Além disso, os mockups mostram 1,2M events/s, 180 serviços, owners e SLOs: o produto da Fase 3, não o ICP de 3–50 engenheiros do MVP (PRD §2.1).

**Correção**

1. Escolher explicitamente uma das duas rotas:
   - **(a)** mover para o MVP as dependências que o fluxo exige: change annotations, SLO burn, incident entity/link, PagerDuty/runbook/owner; ou
   - **(b)** reescrever Fluxo 1 e Fluxo 4 para o escopo Fase 1: sem deploy annotation, sem SLO, sem incident linking, sem rollback dentro do produto; mostrar handoff explícito para CI/CD e runbook externo, com link e contexto copiável.
2. Marcar cada superfície, fluxo, mockup e FR com a fase em que entra. Nenhum fluxo “Fase 1” pode depender de FR “Fase 2/3”.
3. Definir o que “nunca sair do Observaki” significa: diagnóstico + handoff com contexto, ou execução de remediação com permissão e auditoria. As duas são válidas; a ambigüidade não é.

**Critério de aceite**

Um roteiro de UJ-1 executado apenas com FRs marcados “Fase 1” chega de alerta a causa e a uma ação de handoff sem dead end. Se algum passo exigir Fase 2/3, o fluxo está marcado errado.

---

### C2 — Surface closure é afirmação, não cobertura

**Evidência**

- `EXPERIENCE.md` afirma “**every need in the PRD maps to a surface and a journey**” (linha 41) e lista 6 necessidades (linhas 43–50).
- A IA lista 9 superfícies (linhas 25–35); os mockups cobrem **3**: Explore, Dashboards e Services > Map. **Overview, Alerts, Incidents, Settings, Onboarding e Command palette não têm mockup.** Services só tem o mapa, não catálogo/detalhe.
- O Fluxo 1 **não passa por Services/Map**, apesar de a tabela de closure dizer “Overview → Explore → Services” (linha 45). O `service-map.html` fica órfão do fluxo principal.
- O Fluxo 3 (linhas 185–195) é mapeado para “Settings → Services → Alerts” (linha 47), mas o trabalho real é Settings > Usage, pipeline/redaction, quotas/cardinalidade, retenção e audit log. Nenhuma dessas superfícies está na IA.
- O Fluxo 4 é mapeado para “Alerts → Explore preview” (linha 48), mas começa “from the trace/metric panel” (linha 201). Alerts não tem mockup, lista, builder ou silences.
- “Find anything quickly → Command palette” (linha 49) não tem mockup e não é usado por nenhum fluxo. “Understand cost/retention → Settings → Usage” (linha 50) não tem superfície; os mockups mostram “Usage & cost” como item de nav de primeiro nível.
- O `dashboard.html` diz “Panels load independently; a failed panel shows its own retry state” (linha 193), mas nenhum painel falho é mostrado. O `service-map.html` diz “missing telemetry shows ‘no data’” (linha 198), mas nenhum nó “no data” existe.

**Impacto**

A tabela de closure dá uma falsa sensação de completude. Na implementação, as superfícies sem mockup serão inventadas por engenheiros, cada uma com um padrão diferente. As jornadas centrais — primeiro login, landing de alerta, criação de alerta, governança — não têm desenho. O design não é implementável como está; é um conjunto de três telas cercado por uma promessa de cobertura.

**Correção**

1. Substituir a tabela atual por uma **matriz de cobertura** com colunas: superfície, fase, owner, mockup, estados, componentes, FRs, jornada, status (“desenhado”, “wireframe”, “não no MVP”).
2. Produzir pelo menos wireframes de: **Overview** (landing de alerta), **Onboarding** (primeiro run), **Alerts** (lista + builder + silence), **Settings/Usage** (retenção, quota, custo, audit), **Command palette** e **Services** (catálogo + detalhe).
3. Corrigir a tabela de closure para apontar para passos exatos dos fluxos e para os mockups. Se um fluxo não passa pela superfície mapeada, a tabela está errada.
4. Marcar explicitamente o que é Fase 2/3 e não faz parte do MVP. “Incidents” e “Usage & cost” não podem aparecer como itens de primeiro nível no MVP sem estado “em breve” ou remoção.

**Critério de aceite**

Todo FR do MVP tem superfície e pelo menos um estado desenhado. Toda superfície listada na IA tem mockup ou status “não no MVP”. Nenhuma linha da tabela de closure aponta para uma jornada que não a usa.

---

### C3 — Query bar, chips e tabela contam três histórias diferentes

**Evidência**

- Os chips dizem `service.name = checkout` e `status = error` (`explore.html`, linhas 134–135).
- A query bar diz `WHERE service.name = "checkout" AND duration > 2000ms` (linha 143).
- A tabela mostra traces de **payment-authorizer, checkout, inventory e cart** (linhas 153–158), com status **ERROR, SLOW e OK**, e durações **1.984 ms, 1.902 ms e 1.441 ms** — abaixo do `> 2000ms` e fora do `status = error`.
- O contador diz “3.481 traces · 428 slow · 1,2% sampled” (linha 138), mas a lista tem 6 linhas e não há controle de paginação, apenas o rodapé “Showing 6 of 3.481 traces. Cursor pagination — no infinite scroll” (linha 161).
- Não há semântica de sincronização entre chips e SQL. Os chips são `<div class="btn">`, não têm “×” nem ação de remoção, embora o Fluxo 1 diga “she can remove them” (linha 164).
- A coluna “Service” não define se é **root service** ou **serviço do span mais lento**. A linha selecionada mostra `payment-authorizer` (linha 153), mas o drawer mostra root `checkout` (linha 170) e o filtro é `service.name = checkout`.
- `SLOW` não é um status da taxonomia do DESIGN (Healthy/Degraded/Critical/Muted/No data, linha 269); é uma condição de duração. O chip `status = error` não deveria incluir `SLOW`.

**Impacto**

Em um incidente, o respondedor não sabe qual conjunto está vendo. Pode concluir que o problema está em `payment-authorizer` quando o filtro é `checkout`; pode achar que removeu um filtro quando ele continua no SQL; pode achar que a lista está ordenada por duração e não está. A query bar é o controle mais importante do produto e, no mockup, ela mente. Isso destrói a confiança antes de qualquer bug de backend.

**Correção**

1. Uma única fonte de verdade: chips são uma **visão parseada** da query; editar chips atualiza o SQL e a URL; editar o SQL atualiza os chips. Mostrar “aplicado” vs “rascunho” e um botão “Run” explícito.
2. Definir a semântica do filtro de serviço: “traces que contêm o serviço” ou “traces cujo root é o serviço”. Rotular a coluna de acordo (“Root service” / “Slowest service”) e mostrar o root no drawer.
3. Definir `SLOW` como regra de duração (ex.: `duration > SLO`), não como status. Não permitir `status = error` com linhas `SLOW/OK`.
4. Adicionar remoção de chip, “Clear filters” e contagem de resultados **depois** dos filtros.
5. Reconciliar os dados do mockup com os filtros visíveis; usar um fixture único para todas as telas.
6. Adicionar testes de contrato: toda linha visível satisfaz os filtros visíveis; URL reproduz exatamente a mesma lista.

**Critério de aceite**

Para cada filtro visível, toda linha exibida o satisfaz ou está explicitamente marcada como exceção. Chips e SQL fazem round-trip. A URL copiada reproduz a mesma lista, na mesma ordem, com o mesmo span selecionado.

---

### C4 — Contraste e modo claro falham WCAG 2.2 AA antes de qualquer implementação

**Evidência (cálculo WCAG 2.x, luminância relativa)**

| Par | Ratio | Exigência | Resultado |
|---|---:|---:|---|
| `foreground` `#E6EDF7` / `background` `#080B10` | 16,72:1 | 4,5:1 | Passa |
| `foreground-muted` `#93A1B5` / `background` | 7,51:1 | 4,5:1 | Passa |
| `foreground-subtle` `#66758A` / `surface-1` `#0E131B` | **3,97:1** | 4,5:1 | **Falha** |
| `foreground-subtle` / `surface-2` `#141B26` | **3,69:1** | 4,5:1 | **Falha** |
| `foreground-subtle` / `background` | **4,20:1** | 4,5:1 | **Falha** |
| `primary-light` `#0891B2` / branco | **3,68:1** | 4,5:1 | **Falha (texto)** |
| `primary-light` / `background-light` `#F6F8FB` | **3,46:1** | 4,5:1 | **Falha (texto)** |
| `primary-foreground-light` `#FFFFFF` / `primary-light` | **3,68:1** | 4,5:1 | **Falha (botão)** |
| `foreground-subtle-light` `#7B8A9E` / branco | **3,52:1** | 4,5:1 | **Falha** |
| `foreground-subtle-light` / `background-light` | **3,31:1** | 4,5:1 | **Falha** |
| `border` `#253141` / `surface-1` | **1,41:1** | 3:1 (UI) | **Falha** |
| `border-strong` `#33445A` / `surface-1` | **1,88:1** | 3:1 (UI) | **Falha** |
| `border-light` `#D8E1EC` / branco | **1,32:1** | 3:1 (UI) | **Falha** |

- `foreground-subtle` é usado em metadados essenciais: cabeçalhos de tabela (`explore.html`, linha 61), eixos e legendas (linha 80), rodapés (linha 95), meta de painel (`dashboard.html`, linha 52), legendas (linha 60), cabeçalhos de tabela (linha 68), e status/legendas do mapa (`service-map.html`, linhas 50, 61, 67, 74, 78). São 9–11 px, portanto **não** se qualificam para a exceção de texto grande.
- Todos os mockups declaram `<html lang="pt-BR">` (`explore.html` linha 2; `dashboard.html` linha 2; `service-map.html` linha 2), mas toda a UI está em inglês. Isso viola WCAG 3.1.1 e faz o leitor de tela pronunciar “Checkout latency investigation” com regras de português.
- Não há `:focus`, `:focus-visible`, `prefers-reduced-motion` ou `@media` em nenhum mockup (grep nos três arquivos). O foco visível é uma promessa do spine (EXPERIENCE, linha 132), não um artefato.
- O modo claro é prometido (`DESIGN.md`, linha 213; PRD §8.6, linha 917), mas **nenhum mockup claro existe**. Os `:root` são só dark, não há `[data-theme]`/`prefers-color-scheme`, e o SVG do mapa usa cores hardcoded (`#33445A`, `#F87171`, `#66758A`, linhas 129–141), o que impede troca de tema sem reescrever.
- O botão primário claro usa `#FFFFFF` sobre `#0891B2` (3,68:1); o foco é `primary-light`, que sobre o próprio botão claro desaparece. O par escuro (`#041014` sobre `#22D3EE`, 10,67:1) passa; o claro não.

**Impacto**

A afirmação “WCAG 2.2 AA” (EXPERIENCE, linha 131; memlog, linha 12) é falsa no estado atual. Metadados que o respondedor mais precisa — hora, unidade, serviço, eixo, status — são justamente os que falham. Pessoas com baixa visão, daltonismo ou sensibilidade a contraste não conseguem operar; compras públicas/enterprise podem bloquear. O modo claro, se implementado como está, nasce reprovado.

**Correção**

1. Escurecer `foreground-subtle` no dark (ou clarear para ≥4,5:1) e `foreground-subtle-light` no light; testar todos os usos, não só o token.
2. No light, usar um primary mais escuro para texto (≥4,5:1) e um `primary-foreground` que contraste com o botão; `#FFFFFF` sobre `#0891B2` não serve.
3. Elevar bordas essenciais (inputs, search, project switcher, botões) para ≥3:1; se a borda é decorativa, não usar como única delimitação.
4. Trocar `lang` para `en` (ou o locale real da UI) e manter `lang` por parte quando houver conteúdo em outro idioma.
5. Criar mockups light dos três ecrãs, com tokens light completos para status pills, gráficos, annotations, logs e mapas; remover cores hardcoded dos SVGs.
6. Adicionar tema claro/escuro com persistência e preferência do sistema; testar ambos com axe e contraste.
7. Não usar “WCAG 2.2 AA” como claim até o audit passar; o PRD ainda diz 2.1 (linha 917) — reconciliar para 2.2 e cobrir os critérios novos.

**Critério de aceite**

Nenhum par texto/fundo essencial abaixo de 4,5:1; nenhuma borda essencial abaixo de 3:1; light e dark passam; `lang` correto; toggle de tema funcional; axe sem critical/serious.

---

### C5 — O mapa de serviços é uma imagem, não uma ferramenta

**Evidência**

- O layout é `1fr 360px` (`service-map.html`, linha 47), mas o SVG usa `viewBox="0 0 900 560"` com `preserveAspectRatio="none"` (linha 127) e os nós são posicionados em **px absolutos** (`left/top`, linhas 144–173). Em 1280 px, o painel do mapa fica em torno de 620 px; o SVG é escalado para ~0,69×, mas os nós não. Em 1440/1920, a escala é outra. Em qualquer largura ≠ 900 px, arestas e nós se desencontram.
- O nó `db.pool` começa em `left:620px` e tem 170 px de largura; em um painel de ~620 px ele é cortado. A aresta que deveria terminar nele usa `x=690` no viewBox, que escala para ~477 px — o arrowhead aponta para o vazio.
- A aresta vermelha de `db.pool` para “postgres” termina em `y=420` (linha 136), mas **não existe nó `postgres`**; só existe um label de texto em `x=705,y=350` (linha 141). O caminho crítico termina no nada.
- As arestas têm `stroke-width` 1,5–2,5 px e o SVG é `aria-hidden="true"` (linha 127); não há `role`, `tabindex`, hit path invisível ou handler. Não dá para clicar nem operar por teclado; o alvo fica muito abaixo dos 24 px mínimos da WCAG 2.5.8 e dos 44 px de toque do DESIGN.
- Não há controles de zoom/pan/reset, apesar de o DESIGN prometer “zoomable/pannable” (linha 267). Não há estado de nó selecionado; o painel lateral está fixo em `payment-authorizer` (linhas 178–196), independentemente do que o usuário clicar.
- O header diz “38 services · 112 edges” (linha 108), mas o mapa mostra **6 nós e 6 arestas**. Não há agregação, filtro, “show all” ou explicação de por que 32 serviços sumiram. Em um incidente, o culpado pode ser um dos invisíveis.
- A legenda mostra “Selected path” em cyan (linha 175), mas não há nenhum caminho cyan no mapa; o caminho crítico é vermelho (linhas 134–136). O rodapé afirma “Health is border + icon + label” (linha 194), mas os nós não têm ícone — só borda e texto.
- O nó tem `border-radius:8px` (linha 55), contra `{rounded.md}` = 6 px do token `service-node.radius` (`DESIGN.md`, linha 177); usa `box-shadow` e glow (linhas 55–56), contra “borders before shadows” e “nothing in the data plane should look raised” (`DESIGN.md`, linhas 239–241).

**Impacto**

O service map é a peça central do UJ-1 do PRD (linhas 53–54) e um dos três mockups. No estado atual, ele é uma ilustração bonita que **não funciona** em 1280 px, não é clicável, não é navegável por teclado e esconde 84% dos serviços. O respondedor pode clicar na aresta errada, não ver o serviço culpado ou não conseguir selecionar o nó. Isso é exatamente “bonito no papel, quebrado no incidente”.

**Correção**

1. Usar **um único sistema de coordenadas** para nós e arestas (SVG ou canvas), com um transform responsivo; nunca posicionar nós em px de CSS e arestas em unidades de viewBox.
2. Adicionar zoom/pan/reset, busca de serviço, minimapa ou foco+contexto, e um estado de seleção visível. O painel lateral deve seguir a seleção.
3. Criar hit areas de ≥24 px (ideal 44 px) para nós e arestas; suportar teclado (Tab/Enter/setas) e anunciar o nó/aresta selecionado.
4. Para 38/500 serviços, agregar por namespace/domínio, agrupar serviços saudáveis, permitir “show all” e indicar claramente “6 de 38” com filtro aplicado.
5. Adicionar o nó `postgres` ou remover a aresta; alinhar `db.pool` como pool vs banco; legendas de aresta (latência vs erro) e de caminho crítico.
6. Implementar e persistir layout incremental; novos nós não podem deslocar tudo sem aviso. “Layout” deve ter confirmação/undo.
7. Corrigir tokens: raio 6 px, sem sombra/glow em data plane, outline de caminho crítico de 2 px.

**Critério de aceite**

Em 1024/1280/1440/1920/200%, toda aresta termina no nó correto, nenhum nó é cortado, 38/500 serviços são navegáveis e o teclado seleciona nós/arestas. A legenda corresponde ao que está na tela.

---

## Achados Altos

### A1 — 13 estados críticos, zero estados desenhados

**Evidência**

- `EXPERIENCE.md` lista 13 estados (linhas 87–103): cold load, empty result, query error, query timeout, partial data, stale data, no permission, rate limited, degraded platform, offline, sampled trace, missing trace, dangerous action.
- Nenhum aparece nos mockups. Todos os três mostram apenas o happy path, com dados completos e ações funcionando.
- O `dashboard.html` afirma no rodapé que “a failed panel shows its own retry state” (linha 193), mas não há painel falho. O `service-map.html` afirma que “missing telemetry shows ‘no data’” (linha 198), mas não há nó sem dados. O `explore.html` afirma paginação (linha 161), mas não há controle de página.
- O estado de query error promete “previous result stays visible and is marked ‘stale’” (linha 93), mas não há estilo de stale, badge ou contraste. O de partial data promete “dotted underline + tooltip” (linha 95), mas tooltip não é acessível por teclado/touch e não há exemplo.

**Impacto**

Os momentos mais difíceis de um incidente são justamente os de ausência: query timeout, dado parcial, permissão negada, rate limit, trace amostrado. Sem desenho, a implementação cai no default que o spine proíbe: spinner, toast que desaparece, painel em branco, “Something went wrong”. O produto parece maduro no mockup e infantil no incidente.

**Correção**

Criar uma **galeria de estados** por superfície/componente, com: copy, layout, ação primária, ação secundária, foco, anúncio ARIA e critério de aceite. Priorizar: query timeout (com cancelar), partial trace (18 vs 6 spans), sampled data, no permission, rate limit, offline, failed panel, empty result, cold load. Usar fixtures de dados que reproduzam cada estado.

**Critério de aceite**

Cada um dos 13 estados tem um mock nomeado e um teste. Nenhum estado é apenas prosa.

---

### A2 — A hierarquia de ação serve autoria, não resposta a incidente

**Evidência**

- Em Explore, “Create alert” é o botão primário cyan (`explore.html`, linha 128); “Run” é texto muted de 11 px dentro da query bar (linha 144). A ação central da tela é visualmente secundária.
- No dashboard, “Add panel” é primário (linha 104); “Investigate”, no banner de incidente ativo, é secundário (linha 115).
- No service map, “Open Explore” é primário (linha 109); “Investigate” é secundário (linha 121).
- Não há badge/contador de alertas na nav (`explore.html`, linhas 108–119; `dashboard.html`, 87–98; `service-map.html`, 92–103). Um segundo alerta pode disparar sem o respondedor ver.
- Os banners não têm “Acknowledge”, “Silence”, “Runbook” ou severidade. O contrato de `Alert row / incident banner` exige “Acknowledge, silence and open investigation inline” (`EXPERIENCE.md`, linha 82). O dashboard só tem “Investigate”; o service map só tem “Investigate”.

**Impacto**

Às 3h da manhã, o olho é puxado para “Add panel” e “Create alert”, não para “Run” ou “Investigate”. O on-call não consegue dar ack/silence na tela em que aterrissa; um segundo alerta fica invisível; o MTTA piora. A interface foi desenhada para quem está construindo dashboards, não para quem está apagando incêndio.

**Correção**

1. Hierarquia por estado: em incidente, primário = **Investigate** ou **Run**; em edição, primário = **Save**; em onboarding, primário = **Send first signal**. “Create alert” e “Add panel” vão para secundário/overflow.
2. Adicionar **alert inbox global** com contador de firing/critical, acessível por teclado e `aria-live`.
3. Banner de incidente com: severidade, estado, serviço, duração, owner, **Ack**, **Silence**, **Runbook**, **Investigate**, e link “back to alert/incident”.
4. Não usar cyan de marca em ação de autoria quando há incidente ativo; reservar o primário para a ação de resposta.

**Critério de aceite**

Um on-call consegue ir de alerta a ack/silence/investigate sem sair da landing. O primário muda com o estado. Um novo alerta é anunciado e visível em qualquer superfície.

---

### A3 — Tempo e contexto não sobrevivem à rolagem/navegação

**Evidência**

- O time range está no toolbar da página (`explore.html`, linha 133; `dashboard.html`, linha 108; `service-map.html`, linha 114), não no topbar como o DESIGN define (linha 251). O container `.main` tem `overflow:auto` (`explore.html`, linha 37), então range, filtros e título rolam para fora. Depois de descer uma lista longa, o respondedor perde a janela.
- Os presets são diferentes em cada superfície: Explore 15m/1h/6h/24h/Custom; Dashboard 6h/24h/7d/Custom; Service map 15m/1h/6h/24h. Não há 5m, 7d/30d em todas, range absoluto, timezone ou live toggle.
- O dashboard diz “Last 6h” (linha 108), mas o painel de anotações diz “last 24h” (linha 180). O Fluxo 1 abre uma métrica com “1h trend” (linha 167) dentro de uma investigação de 15m. Não há regra de handoff entre superfícies.
- Só alguns horários têm “UTC” (banner do dashboard, linha 115; service map, linha 121; Explore, linha 126). A tabela de traces mostra `06:17:42` sem timezone (linhas 153–158); o log mostra `06:17:42.118` sem timezone (linha 177); o eixo do waterfall não tem timezone.
- O dashboard mostra “live · refreshed 8s ago · pause” (linha 112), com auto-refresh ligado por default, enquanto a `EXPERIENCE.md` diz que auto-refresh é opt-in (linha 96). “pause” é texto, não botão.

**Impacto**

O respondedor compara janelas diferentes sem saber: um p99 de 6h pode parecer calmo enquanto o incidente de 15m é grave; uma métrica de 1h dentro de uma investigação de 15m muda a escala; um painel de 24h dentro de um dashboard de 6h quebra a comparação. Sem timezone, o handoff entre fusos erra. Sem controle de pausa, o dado se move enquanto ele lê — exatamente o anti-pattern que o spine rejeita (linha 155).

**Correção**

1. **Time context bar global e sticky**, no topbar ou imediatamente abaixo: range resolvido, timezone, live/paused, refresh interval. Ela nunca rola para fora.
2. Contrato único de handoff: alerta define a janela; ao abrir Explore/Dashboard/Services, preservar a janela ou mostrar “janela alterada de 15m para 6h” com ação de voltar.
3. Permitir override por painel, mas com badge visível “24h” e opção “usar range global”. O default é global.
4. Padronizar presets (5m, 15m, 1h, 6h, 24h, 7d, 30d, custom) e range absoluto com timezone.
5. Auto-refresh opt-in, com botão de pausa real, intervalo e pausa quando a aba está oculta. Mostrar “stale há Xs” quando pausado.
6. Formatar todo horário absoluto com timezone; usar ISO/UTC no tooltip e o fuso do usuário no display.

**Critério de aceite**

O range está visível em qualquer posição de scroll. Todo painel usa o global ou mostra override. O Fluxo 1 preserva 15m ou rotula a mudança. A URL reproduz o range e o timezone.

---

### A4 — O dashboard não prova visualmente a causalidade que promete

**Evidência**

- O painel “Deploy & incident annotations” (`dashboard.html`, linhas 179–190) é separado dos painéis de p99/erro (linhas 144–150). Para correlacionar deploy e latência, o usuário precisa alinhar mentalmente dois gráficos.
- O gráfico de anotações não tem eixo Y, eixo X com horas, linha de threshold de 2s, sombreamento do incidente ou marcador de rollback. Tem uma linha que sobe monotonicamente da esquerda para a direita, uma linha de deploy em x=600 e o texto “deploy 05:58” (linha 186). A legenda diz “rollback 06:24” (linha 189), mas não há marcador de rollback no gráfico.
- O painel diz “last 24h” (linha 180), mas o dashboard diz “Last 6h” (linha 108); a linha do tempo não é a mesma.
- O banner do incidente é âmbar (`dashboard.html`, linha 115), mas o mesmo incidente é vermelho no service map (`service-map.html`, linha 121). A severidade muda de cor entre superfícies.
- Os números do SLO não reconciliam: “71%” consumido + “28% left” = 99% (linhas 172–174); “burn rate 6.2x · exhaust in 3d” não fecha com 71% consumido em uma janela de 30d (o restante de 29% a 6,2× daria ~1,4 dia, não 3).
- O painel de request rate mostra “+4,2%” em verde (linha 121), mas aumento de tráfego durante um incidente de saturação não é necessariamente bom; o DESIGN só permite cor semântica quando a direção “good” é conhecida (linha 261).
- Os stats não declaram se o valor é instantâneo ou agregado da janela: “p99 2,846” pode ser um p99 de 6h que esconde um pico de 5m, ou um valor atual. O delta “+1.402ms” não diz contra qual período foi comparado.

**Impacto**

A conclusão “o deploy causou o incidente” é afirmada, não mostrada. O respondedor não consegue apontar no gráfico onde o deploy entrou, onde o p99 inflectiu e onde o rollback recuperou — que é exatamente a decisão que ele precisa tomar. Um dashboard bonito que não sustenta a decisão é pior que uma tabela feia que sustenta.

**Correção**

1. Sobrepor deploy/rollback/incident markers **nos painéis de p99 e error rate**, com eixo X compartilhado. O painel separado pode existir como timeline, mas não substitui a sobreposição.
2. Adicionar eixo Y com unidade, eixo X com horas/timezone, linha de threshold do SLO, sombreamento da janela do incidente e crosshair com valores no hover/teclado.
3. Mostrar o rollback como marcador real e a recuperação pós-rollback; se o dado ainda não existe, dizer “aguardando dados pós-rollback”.
4. Reconciliar os números do SLO e do error budget; usar uma única definição de “consumido/restante”.
5. Unificar a cor de severidade do mesmo incidente em todas as superfícies.
6. Não colorir request rate de verde sem direção “good” definida; usar neutro.

**Critério de aceite**

Um leitor consegue, em um único gráfico, apontar deploy, início do incidente, p99 e rollback, com valores e horários. Os números do SLO fecham. A mesma severidade tem a mesma cor em Dashboard, Explore e Services.

---

### A5 — Onboarding é a superfície menos desenhada, apesar de ser a métrica nº 1

**Evidência**

- `SM-1` define TTFV p50 < 5 min e p90 < 15 min (PRD, linha 846). A IA tem “Onboarding” (EXPERIENCE, linha 35) e o Fluxo 2 é inteiro sobre isso (linhas 172–183).
- Não há mockup de onboarding. Não há tela de primeiro admin, criação de organização/projeto, defaults de retenção, criação/rotação de token, cópia do endpoint OTLP, abas de SDK, “waiting for first signal”, diagnóstico de “no data”, galeria de templates, convite de teammate ou share.
- O Fluxo 2 põe o token em um snippet Go (linha 177); não há aviso “não commite”, token com escopo ingest-only, rotação ou variável de ambiente.
- “Auto-detect the first trace” (linha 178) não tem mecanismo, timeout, polling ou feedback de espera. O “first trace toast” é transiente; se o usuário estiver em outra aba, perde.
- O diagnóstico de falha (linha 183) lista endpoint, token, clock skew, sampling e firewall, mas não tem mock, nem ordem de verificação, nem link exato.
- O “Create dashboard”/template gallery (linha 179) não tem mock; o “Share” (linha 180) não tem fluxo de permissão/convite.

**Impacto**

A promessa de 5 minutos é a métrica de sucesso do produto, mas a superfície que a entrega não existe. O primeiro run de um self-hosted é onde o usuário desiste; sem admin bootstrap e sem estado “nenhum dado chegou”, o produto começa com uma tela vazia e um token no clipboard. O TTFV não é projetável nem testável.

**Correção**

1. Desenhar o **first-run completo**: criar admin → organização → projeto → defaults de retenção/quota → token (escopo ingest, exibido uma vez, com rotação e aviso de segredo) → endpoint OTLP + abas Go/Python/Node/Collector → “aguardando primeiro sinal” com timeout e diagnóstico.
2. Criar um estado “no data yet” com checklist de causas (endpoint, token, clock skew, sampling, firewall), cada uma com link e ação de teste.
3. Template gallery com preview de painéis e “Create dashboard”; após criar, “Share” com convite de teammate e estado de permissão.
4. Persistir o checklist no servidor, não só no browser; reabrir em Help; notificar quando o primeiro sinal chegar mesmo se o usuário estiver em outra aba.
5. Não colocar token em snippet sem aviso; usar `OBSERVAKI_TOKEN` e mascarar.

**Critério de aceite**

Um usuário novo executa install → projeto → token → primeiro sinal → dashboard → share em <5 min sem documentação externa. O estado “nenhum dado” resolve as 5 causas mais comuns. O token não vaza no snippet.

---

### A6 — Teclado, semântica e alternativas de gráfico não são demonstráveis

**Evidência**

- Nenhum mockup tem `:focus`/`:focus-visible`; a busca por `@media`, `prefers-reduced-motion`, `role=`, `tabindex`, `aria-current`, `aria-live` retorna vazio (exceto `aria-label` nos SVGs e `aria-hidden` nas arestas).
- Os controles são elementos não interativos: segmented controls são `<span>` (`explore.html`, linhas 132–133), tabs são `<span>` (linha 167), “close” é `<span>` (linha 165), “Run” é `<span>` (linha 144), filtros são `<div class="btn">` (linhas 134–135), o project switcher é `<div>` (linha 102), a busca é `<div>` (linha 103), os nós do mapa são `<div>` (linhas 144–173), as arestas são SVG `aria-hidden` (linha 127).
- As tabelas têm `<th>` mas não `<caption>` nem `scope`; a nav ativa não tem `aria-current="page"`; não há skip link; não há landmark de busca.
- Os gráficos têm apenas `aria-label` (“Request rate trend”, `dashboard.html`, linha 122) sem `role="img"`, `<title>`, tabela alternativa ou resumo textual. O PRD §8.6 exige “todo gráfico canvas tem tabela alternativa e resumo textual” (linha 919); o EXPERIENCE repete (linha 133).
- O waterfall é uma sequência de `<div>`s sem semântica de lista/tabela, sem status por span e sem seleção por teclado. O `↑/↓` do EXPERIENCE (linha 79) não tem estado visual.
- O dashboard usa drag para reordenar/redimensionar painéis (EXPERIENCE, linha 123) sem alternativa de ponteiro único/teclado, o que falha WCAG 2.2 SC 2.5.7 (Dragging Movements).
- O focus ring é `{colors.focus-ring}` = primary; em um botão primary cyan, o ring sem offset desaparece. O mesmo vale no light (`primary-light` sobre `primary-light`).
- O mockup é estático; não há JS para validar foco preso no drawer/dialog, retorno de foco, `Esc`, `aria-live` da query ou do command palette.

**Impacto**

O claim “keyboard-first” (EXPERIENCE, linha 107) não é testável e, no artefato, é falso: os controles nem são focáveis. Leitores de tela não recebem a tabela de dados dos gráficos, a seleção de span ou o resultado da query. Pessoas com limitações motoras não conseguem reordenar painéis. O produto pode passar no visual e falhar na auditoria de acessibilidade.

**Correção**

1. Usar controles nativos (`button`, `input`, `select`) e padrões ARIA só quando necessário: `tablist/tab`, `aria-current`, `aria-live`, `role="status"`.
2. Adicionar `:focus-visible` com ring de 2 px e offset; garantir contraste do ring sobre botões primary.
3. Adicionar skip link, `<caption>`/`scope`, e uma tabela alternativa/resumo para cada gráfico.
4. Implementar seleção de span/linha por teclado com estado visível e anúncio.
5. Oferecer alternativa a drag: menu “Move up/down”, “Resize” por teclado ou diálogo de layout.
6. Prototipar em Storybook/Histoire e testar com teclado, leitor de tela e axe; documentar a ordem de foco do drawer/dialog.

**Critério de aceite**

Um roteiro keyboard-only completa UJ-1. O leitor de tela anuncia superfície, filtros, resultados, gráfico e seleção. Axe 0 critical/serious. Drag tem alternativa.

---

### A7 — Responsivo é aspiracional; mobile “read-only” contradiz seu único trabalho

**Evidência**

- Os três mockups têm `min-width:1200px` (`explore.html`, linha 21; `dashboard.html`, linha 19; `service-map.html`, linha 19) e nenhum `@media`. Em 1024 px, a página força scroll horizontal; em 200% de zoom sobre 1280 px, o viewport CSS vira 640 px e o scroll horizontal é garantido.
- A `EXPERIENCE.md` define 1024–1279 como nav de 64 px, drawer overlay e grid de 8 colunas (linha 145); o mockup mantém nav de 240 px e grid de 12 colunas. No grid de 1200 px que o `min-width` força, a tabela do Explore fica com ~482 px para 7 colunas e o mapa com ~542 px; em 1024, o restante é scroll horizontal.
- Em 768–1023, o spine promete nav sheet, tabelas com scroll horizontal e read-only (linha 146); não há mockup nem CSS.
- Em <768, o DESIGN diz “read-only mobile outside v1” (linha 235) e a EXPERIENCE diz “Read-only mobile outside v1; nav sheet; single column; charts simplified” (linha 147), mas a mesma seção diz que mobile existe para “check the alert and acknowledge” (linha 149). **Acknowledge é uma mutação**; se a UI é read-only, o único trabalho mobile é impossível. O mesmo vale para tablet read-only e ack/silence.
- Não há mockup mobile/tablet, nem estado read-only (banner, botões desabilitados, motivo).

**Impacto**

Tablet, laptop pequeno e zoom de 200% quebram. O mobile prometido não consegue fazer o que promete. O produto parece desktop-only, o que é aceitável — mas então o spine não pode prometer mobile/ack. A ambiguidade vai gerar retrabalho e falha de acessibilidade.

**Correção**

1. Implementar breakpoints reais com media/container queries: 64 px nav com tooltips, drawer overlay, grid 8 colunas, tabela com prioridade de colunas/scroll, nav sheet, mapa simplificado.
2. Definir mobile como **“ack + silence + investigação leve”** ou remover mobile do v1. Se read-only, remover “acknowledge” da promessa.
3. Criar estado read-only explícito: banner “Somente leitura”, ações desabilitadas com motivo, tooltip/aria-describedby.
4. Testar 320/768/1024/1280/1440 e 200% zoom; garantir que controles primários não exigem scroll horizontal.

**Critério de aceite**

A 200% de zoom, controles primários não têm scroll horizontal. Em 1024, o layout corresponde ao spec. Mobile consegue ack/silence ou o produto declara que não suporta mobile.

---

### A8 — Taxonomia de status/severidade inconsistente e sem thresholds

**Evidência**

- O DESIGN define os labels “Healthy”, “Degraded”, “Critical”, “Muted”, “No data” (linha 269). Os mockups usam **OK, SLOW, ERROR** (`explore.html`, linhas 153–158), **DEGRADED, SLOW, HEALTHY** (`dashboard.html`, linhas 160–163) e **SLOW, CRITICAL, HEALTHY** (`service-map.html`, linhas 145–172). Não há um vocabulário único.
- `DEGRADED` aparece com pill vermelho (`dashboard.html`, linha 160), mas o DESIGN diz que âmbar é “degraded” e vermelho é “critical/error” (linha 208). O mesmo status tem cores diferentes.
- Todos os serviços na tabela do Explore têm um dot vermelho (`explore.html`, linha 72, aplicado em todas as linhas 153–158), inclusive os `OK`. O dot não segue o status; ele “grita” erro em tudo.
- O mesmo incidente é âmbar no Dashboard (linha 115) e vermelho no Service map (linha 121). A severidade não é consistente entre superfícies.
- Nenhuma status pill tem ícone, embora o DESIGN exija label + ícone (linha 269); a distinção fica só por texto/cor.
- Não há thresholds para “healthy”, “degraded”, “critical” ou “slow”. O service map mostra `SLO 71%` (linha 187) sem dizer se é 71% de budget consumido, 71% de disponibilidade ou 71% de target; “Trace sample 1.2% · errors 100%” (linha 191) é ambíguo entre “100% dos erros são amostrados” e “100% de erro”.
- `wait 14` no nó `db.pool` (linha 162) não tem unidade (14 conexões? 14 ms? 14 waiters?).
- O banner do dashboard não mostra severidade; o do service map mostra “Critical path”, mas não “Critical” como status de alerta.

**Impacto**

Em um incidente, a cor e o texto precisam ser learnable em 2 segundos. Hoje, “degraded” é vermelho em uma tela e âmbar em outra; “OK” tem dot vermelho; “SLO 71%” pode significar sucesso ou catástrofe. O respondedor prioriza errado e perde confiança no mapa.

**Correção**

1. Uma taxonomia única, com definição e threshold por sinal: Healthy, Degraded, Critical, Muted, No data. “Slow” é uma condição de duração, não um status de saúde.
2. Mapear status → cor + ícone + label, consistente em todas as superfícies; “DEGRADED” = âmbar, “CRITICAL” = vermelho.
3. Remover o dot vermelho fixo ou fazê-lo seguir o status; se o dot é decoração, remover.
4. Unificar a severidade do incidente entre Dashboard, Explore e Services; usar a mesma pill/banner.
5. Definir thresholds no tooltip/legenda: p99, error rate, SLO, saturação.
6. Corrigir unidades e labels: “Budget consumed: 71%”, “Error traces sampled: 100%”, “Waiters: 14”, “Instance 1 of 3”.
7. Testar com simulação de deuteranopia/protanopia; não depender de cor.

**Critério de aceite**

A mesma entidade tem o mesmo status, cor e label em todas as superfícies. Todo status tem definição e threshold. Nenhum dot/ícone contradiz o texto. CVD test passa.

---

### A9 — Sampling, parcialidade e confiança estão escondidos ou superprometidos

**Evidência**

- `explore.html` mostra “1,2% sampled” em texto cinza de 11 px (linha 138), sem banner, sem dotted underline, sem marcação nas linhas afetadas. O DESIGN diz “sampling active” é âmbar (linha 208) e a EXPERIENCE diz “Never silently mix sampled and unsampled data” (linha 95).
- O trace meta diz “Spans 18” (linha 166), mas o waterfall renderiza **6 spans** (linhas 170–175), sem banner “partial trace”, sem “show all”, sem contagem “6 de 18”. O DESIGN exige “A trace with missing spans shows a ‘partial trace’ banner” (linha 265).
- O service map diz “38 services · 112 edges” (linha 108), mas mostra 6 nós; não há “6 de 38” nem explicação de agregação.
- A aba “Logs (4)” (linha 167) mostra **uma** linha de log (linha 177); não há lista, contagem ou “ver todos”.
- O Fluxo 1 failure diz “Trace sampled; logs and metrics are complete” (linha 170) e o estado “Sampled trace” diz “Metrics and logs are still complete” (linha 101). Isso é uma promessa forte: logs podem ser amostrados (FR-42, Fase 2), podem ter sido descartados por quota/redaction, ou podem ter expirado por retenção. “Complete” não é garantido.
- O estado “Missing trace” promete “Show correlated logs + service metrics” (linha 102), mas a correlação por serviço/janela é Fase 2 (FR-36, linhas 498–506); no MVP, sem `trace_id`, o fallback pode não existir.
- Não há “inspect” de painel para ver query, agregação, sampling e data source; o dashboard mostra números sem proveniência.

**Impacto**

O respondedor trata uma visão amostrada/parcial como completa, calcula o caminho crítico errado e conclui “o problema não está aqui” quando os spans que faltam são justamente os culpados. A frase “logs e métricas completos” pode ser falsa e destrói a confiança quando o usuário descobre. Em observabilidade, “parcial” precisa ser tão visível quanto “erro”.

**Correção**

1. Banner de parcialidade por sinal, com taxa/política de sampling, janela e valores afetados. Ex.: “Traces amostrados a 1,2%; erros preservados a 100%. Métricas RED vêm de todos os spans.”
2. Contagem explícita: “Mostrando 6 de 18 spans”, “Mostrando 6 de 38 serviços”, “4 logs neste trace”.
3. Marcar valores amostrados com ícone + texto acessível, não só dotted underline/tooltip.
4. Substituir “complete” por “disponível nesta janela” e link para sampling/retenção. Nunca prometer completude sem verificação.
5. Adicionar “Inspect” no painel: query, agregação, janela, fonte, sampling, última atualização.
6. Implementar fallback de missing trace só quando a FR de correlação por serviço/janela estiver disponível; no MVP, dizer “trace não disponível; veja logs do serviço na janela”.

**Critério de aceite**

Nenhuma visão parcial pode ser confundida com completa. Todo valor amostrado tem texto acessível. O usuário vê “6 de 18” e consegue expandir. “Complete” só aparece quando o dado é completo por contrato.

---

### A10 — Performance e densidade prometidas não são provadas pelos mockups

**Evidência**

- O PRD promete 100.000 pontos em <100 ms e 10.000 linhas a 60 fps (FR-27, linhas 404–411), 500 serviços em <1 s (FR-19, linha 325) e 10.000 logs/s (FR-13, linha 263).
- Os mockups mostram 6 traces, 6 nós, 4 linhas de tabela, 1 linha de log e SVGs estáticos. Não há virtualização, zoom/pan, skeletons, cancelamento, backpressure ou live tail.
- A tabela do Explore não tem `overflow`/`max-height`; o “sticky header” (`explore.html`, linha 61) está dentro de `.panel{overflow:hidden}` (linha 56), então o sticky fica relativo ao painel, que não rola, em vez do `.main` (linha 37), que rola. Em 10.000 linhas, o cabeçalho rola com o conteúdo.
- O dashboard tem 6 painéis, mas não mostra o comportamento de 12 painéis independentes, com falha parcial, skeletons ou reordenação.
- O service map tem 560 px de altura e 6 nós; não demonstra 38/500 nós, colisão, agregação ou performance.

**Impacto**

A promessa de “frontend que parece instantâneo” (PRD §1) e o orçamento de performance (JS <180 KB, LCP <1,5 s, INP <200 ms, linhas 873–874) não têm contrapartida visual. O time pode construir o layout bonito e descobrir no primeiro incidente que 10.000 linhas travam, o mapa de 500 serviços colide e o waterfall de 1.000 spans não renderiza. A densidade é boa no mockup; não é testada no volume real.

**Correção**

1. Criar fixtures de alta densidade: 10.000 traces, 500 serviços, 100.000 pontos, 10.000 logs/s e um trace de 1.000 spans.
2. Prototipar virtualização de tabela, uPlot com zoom/pan, agregação de mapa e cancelamento de query; medir INP/LCP.
3. Desenhar skeletons que correspondam ao layout final; mostrar “still loading” após 2 s com cancelar.
4. Implementar backpressure de live tail: pausar, agrupar, descartar antigos, indicar “N linhas/s”.
5. Testar sticky header no container correto e tabelas com scroll horizontal em 1024.

**Critério de aceite**

As fixtures de alta densidade atendem aos alvos do PRD sem travar; o layout não transborda; skeletons e cancelamento funcionam; sticky header permanece visível.

---

## Achados Médios

### M1 — Tokens de tipo, raio, spacing e peso derivam dos spines

**Evidência**

- `DESIGN.md` define `h3` 16 px (linhas 79–83), mas os títulos de painel têm 13 px (`explore.html`, linha 58; `dashboard.html`, linha 51; `service-map.html`, linha 50).
- `metric-value` é 24 px (DESIGN, linhas 142–145), mas o dashboard usa 26 px (linha 55).
- `data-table` usa `sans-sm` 13 px (DESIGN, linha 188), mas as tabelas usam 12 px (`explore.html`, linha 60; `dashboard.html`, linha 67).
- `caption` é 12 px (DESIGN, linhas 50–54), mas há textos de 10,5/11 px; `label` é 11 px (linhas 55–60), mas a UI usa 9/9,5/10 px; `mono-sm` é 12 px (linhas 89–93), mas o logbox usa 11 px (linha 89 do explore).
- Pesos 550/650 não existem no token set (400/500/600); aparecem em `.brand`, `.btn`, `.nav .label`, etc.
- `service-node.radius` é `{rounded.md}` = 6 px (DESIGN, linha 177), mas o nó usa 8 px (`service-map.html`, linha 55). O DESIGN proíbe misturar 8/12 px “porque ficou melhor” (linha 247).
- Gutters do dashboard são 14 px (`dashboard.html`, linha 47), não 16 px (DESIGN, linha 229); a escala de spacing não tem 14/18/22 px, mas o layout usa.
- O side panel do service map tem 360 px (`service-map.html`, linha 47), não os 420 px do drawer (DESIGN, linha 233).
- Linhas de tabela: DESIGN 40 px/32 px compact (linha 273); mockups ~34–36 px.
- O query bar tem um glow cyan (`explore.html`, linha 50) e o nó do mapa tem sombra/glow (`service-map.html`, linhas 55–56), contra “no neon glows” e “borders before shadows” (DESIGN, linhas 199, 239–241).

**Impacto**

O mockup é a referência visual que o time vai copiar. Se ele usa uma escala paralela, o design system nasce morto: cada componente terá “um ajuste de 1–2 px” e a densidade/consistência se perde. A promessa de “instrumento” depende justamente de disciplina tipográfica.

**Correção**

Escolher uma das duas rotas: (a) atualizar os tokens para a escala real usada (10/11/12/13/14/16/20/24/26, pesos 550/650, spacing 14/18/22, radius 8) ou (b) corrigir os mockups para os tokens. Adicionar lint de token/visual regression. Remover glow/sombra de data plane ou criar tokens explícitos de elevação.

**Critério de aceite**

Nenhum valor de tipo/raio/spacing/peso fora do token set; nenhum hex hardcoded em componente; visual regression compara mockup e implementação.

---

### M2 — Contratos de componente ausentes nos mockups

**Evidência**

- O DESIGN define: panel overflow com duplicate/inspect/export/remove (linha 259); alert badge (linhas 154–158, 269); log row com hover/copy/open trace/add filter (linha 263); command palette (linha 271); time range picker com timezone/live (linha 257); data table com sort/column visibility/checkbox/pagination (linha 273); empty state (linha 275).
- Os mockups não mostram: overflow de painel, alert badge, log row, command palette, time picker real, sort control, column visibility, checkbox de seleção, paginação, variáveis de dashboard como selectors, alert builder, silence dialog, share dialog.
- A tabela do Explore tem o texto “sorted by duration ↓” (linha 149), mas não um controle de sort; o DESIGN diz “sorting is explicit; no hidden default sorts” (linha 273).
- A paginação é só um rodapé (“Showing 6 of 3.481... Cursor pagination”, linha 161), sem “Load more”/page control, que o DESIGN exige (linha 273).
- Os chips de filtro não têm remoção; o painel não tem overflow; o gráfico não tem inspect.

**Impacto**

Sem contrato visual, cada engenheiro inventa o componente. A aceitação fica subjetiva (“parece bom”) e a consistência entre superfícies quebra. Componentes que o DESIGN declara “must not be decorative” (command palette, linha 271) não existem nem como wireframe.

**Correção**

Criar um catálogo de componentes com todas as variantes/estados: default, hover, focus, active, disabled, loading, error, empty, partial. Incluir pelo menos: panel + overflow, alert row/badge, log row, command palette, time picker, table header/sort/column visibility/pagination, filter chip, variable selector, alert builder step, silence dialog, share dialog.

**Critério de aceite**

Cada componente do DESIGN tem story e estado; a implementação usa o mesmo componente em todas as superfícies; o catálogo é a fonte de verdade visual.

---

### M3 — Comps estáticos não validam URL state, drafts, foco, ARIA ou loading

**Evidência**

- Não há JavaScript nos mockups. A query não roda, o filtro não remove, a tab não troca, o drawer não abre/fecha, o command palette não abre, o tempo não muda, a URL não atualiza.
- A `EXPERIENCE.md` promete drafts por projeto/superfície (linha 74), URL state (linha 39), foco preso/retornado (linha 132), `aria-live` (linha 133), `Esc` fecha a camada do topo (linha 132), `j/k` (linha 117), `g` + letra (linha 113). Nada disso é demonstrável.
- O estado “query error” promete resultado anterior marcado “stale” (linha 93); não há como testar a transição.
- O “Save view” e “Share” não têm feedback, confirmação ou estado.

**Impacto**

O artefato prova layout, não experiência. Decisões de interação críticas — o que acontece ao trocar de tab, ao falhar a query, ao fechar o drawer, ao trocar de projeto — ficam para a implementação, que é onde os bugs de contexto aparecem. A revisão de UX não consegue aprovar comportamentos que não existem.

**Correção**

Transformar os três mockups em protótipos navegáveis (mesmo que de baixa fidelidade) ou, no mínimo, adicionar uma matriz de comportamento por componente com transições, foco, ARIA e URL. Testar os 5 cenários: trocar sinal, falhar query, abrir/fechar drawer, trocar projeto, compartilhar link.

**Critério de aceite**

Os 5 cenários são executáveis no protótipo; URL muda e volta; foco retorna; draft persiste; estados são observáveis.

---

### M4 — Racional de design virou copy de UI nos rodapés

**Evidência**

- “Cursor pagination — no infinite scroll” (`explore.html`, linha 161) é uma decisão de implementação, não uma mensagem para o usuário.
- “Stable layout · positions do not change on refresh” (`service-map.html`, linha 118) e “Node position is stable across refreshes. Click an edge to filter traces by dependency. Health is border + icon + label, never color alone.” (linha 194) são notas de design.
- “Panels load independently; a failed panel shows its own retry state.” (`dashboard.html`, linha 193) é uma promessa de arquitetura.
- “Map derived from OTel spans; missing telemetry shows ‘no data’, not an error. Layout is explicit; auto-layout does not run on refresh.” (`service-map.html`, linha 198) é documentação.

**Impacto**

Em um incidente, o rodapé ocupa espaço e atenção com racional interno. Pior: parece que o produto está explicando a própria implementação porque não confia no comportamento. O usuário quer “Load more”, “Atualizado há 8s”, “6 de 38 serviços”, não “no infinite scroll”.

**Correção**

Mover essas frases para anotações de design fora da UI. Substituir por ações/estados: “Load more”, “Atualizado há 8s”, “6 de 38 serviços”, “Sem dados de telemetria”, “Amostrado 1,2%”. Se a frase não ajuda o usuário a decidir, não entra na tela.

**Critério de aceite**

Nenhum rodapé contém racional de design; todo texto na UI tem ação ou contexto de dados.

---

### M5 — Números, unidades e fusos inconsistentes

**Evidência**

- `2,39k` no service map (linha 186) usa vírgula decimal; o resto da UI usa `2,418`, `1.2M` (ponto decimal). Em UI inglesa, o correto é `2.39k`.
- Durações alternam `2,846 ms`, `2,846ms`, `2.8s`, `2.5s`, `441ms`; a mesma grandeza muda de unidade e precisão entre superfícies.
- O SLO mostra `71%` + `28% left` (99%) e `burn rate 6.2x · exhaust in 3d`; os números não fecham.
- O request rate headline é `2.418 req/s` (`dashboard.html`, linha 121), mas a soma das operações da tabela é `2.418 + 2.390 + 3.102 + 1.844 = 9.754 req/s` (linhas 160–163). A tabela mistura operações root e chamadas downstream sem dizer; o total do serviço e o total por operação não reconciliam.
- O deploy é `05:58` e o alerta `06:12` (14 min), mas o Fluxo 1 diz “12 minutes before” (linha 167). O waterfall mostra payment-authorizer 2.510/2.846 = 88%, mas o Fluxo 1 diz 80% (linha 165).
- “Spans 18” vs 6 renderizados; “38 services” vs 6 nós; “Logs (4)” vs 1 linha.
- Só alguns horários têm UTC; a tabela e o log não têm timezone.

**Impacto**

Números que não fecham destroem a confiança em um instrumento. O respondedor compara `2.8s` com `2,846ms` e precisa fazer conversão mental; o handoff entre fusos erra; o SLO parece inconsistente. Em um incidente, cada conversão é uma chance de erro.

**Correção**

Centralizar formatação: unidade canônica por grandeza (ms para latência, s para janelas, % para taxas), precisão consistente, timezone obrigatório, separador decimal do locale. Reconciliar todos os números dos fixtures. Adicionar testes de formatação.

**Critério de aceite**

A mesma grandeza tem a mesma unidade/precisão na mesma visão; todo horário absoluto tem timezone; os números do SLO fecham; fixtures são consistentes entre telas.

---

### M6 — IA/nav contradiz o próprio spine

**Evidência**

- `EXPERIENCE.md` diz “seven primary surfaces” (linha 37), mas a tabela lista 9 superfícies (incluindo command palette/onboarding) e os mockups mostram 8 links de nav (Overview, Explore, Dashboards, Services, Alerts, Incidents, Settings e Usage & cost), incluindo itens que a própria IA trata como links contextuais ou de fase futura (explore, linhas 108–119).
- “Incidents” é “Alert/Overview link (Fase 2)” na IA (linha 32), mas aparece como item de nav de primeiro nível em todos os mockups. No PRD, a timeline é Fase 3 (FR-38).
- “Usage & cost” é um item de nav nos mockups, mas a closure table diz “Settings → Usage” (linha 50) e a IA de Settings não inclui Usage (linha 33).
- O DESIGN diz que o topbar contém time range e refresh/pause (linha 251); os mockups colocam ambos no toolbar da página.
- O project switcher mostra só “Project checkout-prod” (`explore.html`, linha 102), sem organização ou ambiente; com múltiplas orgs (FR-45), o usuário não sabe em qual tenant está, e não há badge de “prod” antes de uma ação destrutiva.
- Os ícones da nav são glifos Unicode (`◈`, `⌁`, `▤`, `◇`, `◉`, `▥`, `⚙`, `▣`), não um icon set; em 64 px collapsed, ficam ambíguos e sem tooltip.

**Impacto**

O usuário não sabe se “Incidents” é uma superfície real, um link contextual ou uma fase futura. “Usage” está em dois lugares. O topbar não é o lugar global de tempo, então cada página reinventa. Em 64 px, os glifos não comunicam.

**Correção**

Reconciliar a IA: 7 ou 8 superfícies, com fases explícitas. Remover do MVP o que não existe; se “Incidents” e “Usage” ficam, definir suas telas e estados. Mover time range/refresh para o topbar ou assumir que é local e remover do contrato do topbar. Usar um icon set consistente com labels/tooltips.

**Critério de aceite**

A nav tem exatamente as superfícies da IA, com fase e permissão; o topbar corresponde ao DESIGN; ícones têm nome acessível e tooltip em collapsed.

---

### M7 — RBAC/read-only não aparecem: viewer vê Edit/Share/Add panel

**Evidência**

- O dashboard mostra “Edit”, “Share”, “Add panel” (linha 104) sem contexto de papel. O PRD define admin/editor/viewer (FR-47, linhas 611–618); viewer não pode criar dashboard nem alterar alerta.
- Explore mostra “Save view”, “Create alert” (linha 128) e “Create alert” no drawer (linha 178) sem disabled/hidden para viewer.
- Service map mostra “Layout”, “Filter”, “Open Explore” (linha 109) e “Open traces/Logs/Metrics” (linha 193) sem indicar o que é permitido.
- Não há banner “Somente leitura”, nem tooltip “você não tem permissão”, nem distinção entre “desabilitado por papel” e “desabilitado por estado”.
- Tablet/mobile read-only (EXPERIENCE, linhas 145–147) não tem indicação visual.

**Impacto**

Um viewer pode achar que pode editar e só descobrir no clique; um editor em tablet pode achar que a UI está quebrada; um admin não sabe por que uma ação está desabilitada. A falta de estado read-only é uma fonte clássica de tickets e de ações não intencionais.

**Correção**

Definir matriz de papéis × ações; esconder ou desabilitar com motivo; adicionar banner “Somente leitura” quando aplicável; usar `aria-disabled` + tooltip; testar como viewer/editor/admin e em tablet.

**Critério de aceite**

Viewer não vê ações destrutivas/editáveis habilitadas; toda ação desabilitada tem motivo acessível; read-only é visível.

---

### M8 — URL com query, cache offline e token no snippet têm risco de privacidade

**Evidência**

- A `EXPERIENCE.md` coloca query, filtros, tempo e span na URL (linha 39). Queries de logs/traces podem conter e-mail, customer ID, token ou PII em literais; “Copy link” (linha 125) copiaria isso para Slack/histórico/referrer.
- O estado offline promete “The last view is cached” (linha 100). Não há TTL, clear-on-logout, clear-on-permission-revocation ou indicação de idade do cache. Telemetria sensível ficaria no browser.
- O Fluxo 2 coloca o token do projeto em um snippet Go (linha 177); não há aviso de segredo, escopo ou rotação.
- “Copy link copies a permission-aware deep link” (linha 125) não define o que “permission-aware” significa: link assinado? escopo? expiração? Se o link concede acesso, é um token; se não, é só uma URL.
- “Shared link that the recipient cannot access shows the permission state” (linha 125) pode revelar a existência de projetos/recursos (enumeração).

**Impacto**

O produto é self-hosted e lida com PII; um link de investigação com `email = "cliente@..."` no path é um vazamento. Cache offline sem expiração mantém dados após logout/revogação. Token em snippet sem aviso acaba no Git. “Permission-aware” sem contrato é uma promessa de segurança vaga.

**Correção**

1. Definir o que vai na URL: IDs de recurso, não valores literais sensíveis; ou hash/encode com aviso. Adicionar “Copy link” com preview e aviso se houver PII.
2. Cache offline com TTL, criptografia/clear-on-logout, clear-on-permission-change e badge “visto há X min”.
3. Token de onboarding com escopo ingest-only, exibição única, rotação e aviso “não commite”; snippet usa env var.
4. Especificar share link: escopo, expiração, revogação, auditoria; não usar “permission-aware” sem definição.
5. Para recursos inacessíveis, decidir entre 403 explícito e 404 para evitar enumeração; documentar.

**Critério de aceite**

Nenhum link copiado contém PII sem aviso; cache expira/limpa; token não aparece em snippet sem aviso; share link tem escopo/expiração/revogação.

---

### M9 — Ações “mágicas” sem destino

**Evidência**

- “Request access” no estado no permission (EXPERIENCE, linha 97) não tem workflow, destino, admin notification ou tela.
- “Open as background job (if available)” no timeout (linha 94) não tem job, fila, tela de jobs ou notificação de conclusão.
- “Link to the health page” no degraded platform (linha 99) não tem health page na IA.
- “Save view” (explore, linha 128) existe, mas saved views são Fase 2 (FR-28).
- “Create alert” no drawer (linha 178) não diz se o alerta será do trace, do span ou da métrica.
- “Share” no dashboard (linha 104) não tem dialog/escopo.
- “Layout” no service map (linha 109) não diz o que faz, se reposiciona tudo ou se pode ser desfeito.

**Impacto**

O usuário clica e nada acontece, ou é levado a uma tela que não existe. Em um incidente, um dead end é pior que uma ação ausente. A promessa de “caminho rápido” vira frustração.

**Correção**

Para cada ação, definir destino, permissão, estado de loading/erro/sucesso e volta. Se não existe no MVP, remover ou rotular “Fase 2”. Criar as telas de health page, jobs e access request, ou remover as promessas.

**Critério de aceite**

Nenhum botão/link sem destino; toda ação tem estado de sucesso/erro; ações fora do MVP estão marcadas.

---

### M10 — Preview de alerta chama matches de “real” sem mecanismo

**Evidência**

- O Fluxo 4 diz “sees it would have fired twice (both real)” (linha 202) e “a real incident, not noise” (linha 204).
- O preview “test against last 24h” só pode mostrar que a condição teria sido satisfeita; não pode saber se o incidente era “real” sem julgamento humano ou labels de incidentes passados.
- O aviso de ruído usa “> 20 fires/day” (linha 207) como threshold arbitrário, sem distinguir severidade, serviço ou janela.
- Não há mockup do builder, do preview, do routing, do runbook ou do estado “unsaved changes”.

**Impacto**

O usuário confia em uma classificação automática de “real vs noise” que o produto não tem. O alerta pode ser salvo com falso positivo ou rejeitado por um threshold genérico. A promessa “não acordar ninguém às 3h” não é demonstrável.

**Correção**

Trocar “both real” por “would have fired 2 times; revise each match” com links para as janelas históricas. Deixar o usuário julgar e anotar. Definir thresholds de ruído por severidade/serviço e explicar o cálculo. Criar mock do builder com preview, routing, runbook e unsaved prompt.

**Critério de aceite**

O preview mostra matches com timestamps e links, sem afirmar “real”; o usuário pode inspecionar e ajustar; o ruído é explicado, não um número mágico.

---

## Achados Baixos

### B1 — 71% + 28% = 99%; “exhaust in 3d” não fecha com 6,2x

**Evidência:** `dashboard.html`, linhas 172–174.  
**Impacto:** um instrumento que não soma 100% perde credibilidade; “3d” pode ser 1,4d.  
**Correção:** usar 29% restante ou 71% consumido com o complemento correto; recalcular a projeção a partir do burn rate e da janela; mostrar a fórmula no tooltip.

### B2 — Drift narrativo: 12 vs 14 min, 80% vs 88%, 18 vs 6 spans

**Evidência:** Fluxo 1 (“12 minutes before”, “80%”, `EXPERIENCE.md`, linhas 165–167) vs dashboard (`deploy 05:58`, alerta 06:12) e waterfall (2.510/2.846 = 88%, 18 spans vs 6).  
**Impacto:** cada número divergente é uma dúvida sobre qual dado é o real.  
**Correção:** derivar todos os números de um fixture único; testes de consistência entre narrativa, mockup e dados.

### B3 — Gradientes/sombras/1px outline contra “no gradients/no shadows/2px”

**Evidência:** `service-map.html`, linhas 51–56 (radial/linear gradient, box-shadow, glow); `explore.html`, linha 88 (`outline:1px`); DESIGN, linhas 199, 239–241, 265.  
**Impacto:** quebra a linguagem “bordas antes de sombras” e a ênfase do caminho crítico.  
**Correção:** remover gradientes/sombras de data plane ou criar tokens; usar outline de 2 px no caminho crítico.

### B4 — Ícones são glifos Unicode ambíguos, sem sistema

**Evidência:** nav dos três mockups (`◈`, `⌁`, `▤`, `◇`, `◉`, `▥`, `⚙`, `▣`), topbar (`◔`, `?`).  
**Impacto:** em 64 px collapsed, “▤” vs “▥” é indistinguível; fallback de fonte pode mudar o glifo; leitor de tela pode ler “white square with upper left quadrant”.  
**Correção:** usar um icon set (Lucide/Phosphor) com `aria-hidden` + label; tooltip em collapsed; tamanho e stroke consistentes.

### B5 — Sticky header não funciona com o scroll container atual

**Evidência:** `explore.html`, linha 61 (`th{position:sticky;top:0}`) dentro de `.panel{overflow:hidden}` (linha 56) e de `.main{overflow:auto}` (linha 37), sem `max-height`/`overflow` no wrapper da tabela.  
**Impacto:** em listas longas, o cabeçalho rola com o conteúdo; o usuário perde a referência das colunas.  
**Correção:** colocar o scroll no wrapper da tabela (`overflow:auto; max-height`) ou remover o `overflow:hidden` do painel e tornar o header sticky em relação ao viewport; testar com 10.000 linhas.

---

## Matriz de cobertura de superfícies

| Superfície | Mockup | Estados desenhados | Jornada | Lacuna principal |
|---|---|---|---|---|
| Overview | **Não** | Nenhum | Fluxo 1 (entrada) | Landing de alerta, priorização, incident banner, SLO burn |
| Explore | Sim (só Traces) | Nenhum | Fluxos 1, 2, 4 | Logs, métricas, query error/timeout, parcial, sampling |
| Dashboards | Sim (view) | Nenhum | Fluxos 1, 2 | Edit/drag, variables, failed panel, empty, 12 painéis |
| Services | Só Map | Nenhum | Fluxo 1? (não usado) | Catálogo, detalhe, owner, tabs, traces/logs/metrics |
| Alerts | **Não** | Nenhum | Fluxo 4 | Lista, builder, silences, channels, SLOs, ack |
| Incidents | **Não** | Nenhum | Fluxo 1 (link) | Timeline, impacto, postmortem, fase |
| Settings | **Não** | Nenhum | Fluxo 3 | Projetos, retenção, auth, tokens, members, integrations, audit |
| Usage & cost | **Não** | Nenhum | Fluxo 3 | Ingestão, storage, cardinalidade, custo, quota |
| Onboarding | **Não** | Nenhum | Fluxo 2 | First-run, token, primeiro sinal, diagnóstico, template, share |
| Command palette | **Não** | Nenhum | Todos | Busca global, ações, preview, aria-live, permissão |

**Leitura:** 10 superfícies na prática (incluindo Usage e Onboarding), 3 com mockup, 0 com estados. A tabela de closure da `EXPERIENCE.md` cobre 6 necessidades, mas não cobre as 64 FRs do PRD.

---

## Checagem WCAG 2.2 AA (resumo)

| Critério | Evidência | Status |
|---|---|---|
| 1.4.3 Contraste (mínimo) | `foreground-subtle` 3,97/3,69; light primary 3,68; light subtle 3,31 | **Falha** |
| 1.4.11 Contraste não-textual | borders 1,41/1,88; light 1,24 | **Falha** |
| 1.4.10 Reflow | `min-width:1200px`, sem media queries; 200% zoom | **Falha** |
| 1.4.4 Resize text | tamanhos fixos 9–10 px, headers fixos | **Risco** |
| 2.4.7 Focus visible | nenhum `:focus-visible` nos mockups | **Falha** |
| 2.4.11 Focus not obscured | drawer overlay em 1024 sem spec de foco | **Não demonstrado** |
| 2.5.7 Dragging movements | dashboard drag sem alternativa | **Falha** |
| 2.5.8 Target size | arestas 1,5–2,5 px; close sem padding | **Falha** |
| 3.1.1 Language of page | `lang="pt-BR"` com UI inglesa | **Falha** |
| 1.3.1 Info and relationships | tabelas sem caption/scope; waterfall em divs | **Falha** |
| 1.1.1 Non-text content | gráficos só com aria-label, sem tabela/resumo | **Falha** |
| 1.4.1 Use of color | waterfall/edges por cor; dots | **Falha** |
| 2.2.2 Pause, stop, hide | auto-refresh default, “pause” não é botão | **Falha** |
| 4.1.2 Name, role, value | controles em span/div sem role | **Falha** |
| 4.1.3 Status messages | sem aria-live para query/alertas | **Falha** |
| 2.1.1 Keyboard | controles não focáveis | **Falha** |

**Nota:** o PRD §8.6 ainda declara WCAG **2.1** AA (linha 917), enquanto a EXPERIENCE e o memlog declaram **2.2** AA. Adotar 2.2 como contrato e atualizar o PRD; os critérios novos (2.4.11, 2.5.7, 2.5.8, 3.3.7, 3.3.8) não estão cobertos.

---

## O que parece bonito no papel e quebra no incidente

1. **O dashboard de “Golden Signals” não mostra o sinal de saturação.** O título promete golden signals, mas há latência, tráfego e erros; falta saturação (CPU, memória, pool, fila). Em um incidente de pool esgotado, a saturação é a causa; o dashboard mostra o efeito.
2. **A anotação de deploy está em um painel separado, sem eixo Y, eixo X ou marker de rollback.** A conclusão “o deploy causou” depende de alinhamento mental; o gráfico sobe monotonicamente e não mostra a recuperação.
3. **O mapa de serviços parece topologia, mas é uma imagem.** Em 1280 px, arestas e nós se desencontram; não há zoom, pan, seleção, hit area ou teclado; 32 dos 38 serviços não aparecem.
4. **O waterfall mostra 6 de 18 spans sem dizer que está incompleto.** O caminho crítico pode estar nos 12 spans que faltam; o outline de 1 px é quase invisível.
5. **A query bar mostra um SQL que contradiz os chips e a tabela.** O respondedor não sabe o que está filtrado; a fonte de verdade é ambígua.
6. **O “live” do dashboard atualiza sozinho, sem pausa real.** O dado se move enquanto ele lê; o spine diz que auto-refresh é opt-in.
7. **O botão “Create alert” é o mais forte da tela; “Run” é texto muted.** A interface prioriza autoria sobre diagnóstico.
8. **O banner de incidente não tem ack/silence/runbook/owner completo.** O on-call não consegue agir na landing; um segundo alerta não aparece.
9. **O SLO “71%” no painel lateral do serviço é ambíguo.** Pode ser 71% de budget consumido ou 71% de disponibilidade; a diferença é entre “atenção” e “catástrofe”.
10. **O sampling está em cinza de 11 px.** A visão parece completa, mas 98,8% dos traces não estão lá; o caminho crítico pode ser um viés.
11. **O rodapé explica “cursor pagination — no infinite scroll” em vez de dar “Load more”.** O usuário fica preso nos 6 primeiros traces.
12. **O service map mostra um caminho crítico para “postgres” que termina no vazio.** A topologia visual não corresponde ao modelo.
13. **O dashboard mostra request rate em verde.** Em um incidente de saturação, mais tráfego não é bom; a cor sugere o contrário.
14. **Os status pills não têm ícones e usam OK/SLOW/ERROR/DEGRADED/HEALTHY/CRITICAL.** A taxonomia não é aprendível em 2 segundos.
15. **O mockup é dark-only e sem foco/teclado.** A promessa de light mode e keyboard-first não é testável e, no estado atual, falha.

---

## Top 5 (para decisão)

1. **O MVP não consegue demonstrar o próprio UJ-1.** O fluxo central depende de deploy annotations, SLO, incident linking e rollback que estão em Fase 2/3 ou fora do produto; ou se antecipa escopo, ou se reescreve a narrativa.
2. **Surface closure é uma afirmação, não uma cobertura.** Seis superfícies e todos os 13 estados não têm mockup; os fluxos não passam pelas superfícies que a tabela de closure afirma.
3. **A query bar, os chips e a tabela contam três histórias diferentes.** O filtro visível não corresponde ao SQL nem às linhas; em um incidente, isso é uma conclusão errada esperando para acontecer.
4. **WCAG 2.2 AA falha antes da implementação.** Contraste de metadados (3,97/3,69), light primary (3,68), bordas (1,41), `lang` errado, sem foco, sem semântica, sem reflow e sem alternativas de gráfico.
5. **O mapa de serviços é uma imagem, não uma ferramenta.** Em 1280 px as arestas não encontram os nós, 32 de 38 serviços somem, e não há zoom, seleção, hit area ou teclado.

---

## Limitações e próximos passos

- Esta revisão é baseada em inspeção estática; não houve teste com usuários, leitores de tela ou dados reais. Os ratios de contraste foram calculados, mas a percepção real depende de fonte, tamanho e renderização.
- Recomenda-se, antes de aprovar o design: (1) um teste de usabilidade de 5 pessoas no Fluxo 1 com dados parcialmente faltantes; (2) um audit axe/teclado nos protótipos; (3) uma revisão de fase com o PM para decidir C1; (4) um fixture único de dados para todos os mockups.
- Os spines `DESIGN.md` e `EXPERIENCE.md` **não foram alterados**. Este arquivo é a revisão; as correções devem ser aplicadas pelos owners dos spines e dos mockups.

---

*Fim da revisão.*
