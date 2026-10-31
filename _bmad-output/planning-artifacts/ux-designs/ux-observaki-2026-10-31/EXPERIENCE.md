---
name: Observaki
status: final
sources:
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/prd.md
  - _bmad-output/planning-artifacts/prds/prd-observaki-2026-10-31/addendum.md
  - _bmad-output/planning-artifacts/architecture/architecture-observaki-2026-10-31/ARCHITECTURE-SPINE.md
updated: 2026-10-31
---

# Observaki — Experience Spine

## Foundation

**Desktop-first responsive web.** Observaki is used at a desk, often while a service is failing; the primary surface is a 1280px+ browser, with keyboard as a first-class input. The app is a SvelteKit 3 SPA embedded in the Go binary, using shadcn-svelte/bits-ui behavior and Tailwind 4. `DESIGN.md` is the visual identity contract; this spine owns information architecture, behavior, states, interactions, accessibility and journeys.

**UI system:** shadcn-svelte/bits-ui primitives, extended with Observaki-specific components (query bar, trace waterfall, service map, log row, alert builder). The component library owns focus trapping, ARIA and keyboard semantics; Observaki owns density, data formatting, URL state and the investigation workflow.

**Tenancy:** a user can belong to multiple organizations/projects; the project switcher is global. All query state is scoped to the active project; switching projects preserves the surface but resets context that does not exist in the destination (with a visible notice).

**Form factor:** web only in v1. Mobile is read-only outside v1. There is no native app.

## Information Architecture

| Surface | Reached from | Purpose | Phase | Mockup |
|---|---|---|---|---|
| **Overview** | Nav rail / `g o` | Health snapshot, active alerts, recent changes | 1 | spec only |
| **Explore** | Nav rail / `g e`, command palette | Query metrics/logs/traces, correlate, inspect details | 1 | `mockups/explore.html` |
| **Dashboards** | Nav rail / `g d` | Browse, view, create and edit dashboards | 1 | `mockups/dashboard.html` |
| **Services** | Nav rail / `g s`, service links | Service catalog, service detail, map, traces, logs, metrics | 1 (map basic) | `mockups/service-map.html` |
| **Alerts** | Nav rail / `g a` | Firing alerts, rules, silences, channels | 1 | spec only |
| **Settings** | User menu / `g ,` | Projects, retention, auth, tokens, members, usage | 1 | spec only |
| **Onboarding** | First login, Help menu | Quickstart checklist, instrumentation, first dashboard | 1 | `mockups/onboarding.html` |
| **States gallery** | Any surface (reference) | Empty/error/timeout/partial/rate limit/no permission | 1 | `mockups/states.html` |
| **Command palette** | `⌘K` / `Ctrl+K`, `/` | Global search and actions | 1 | spec only |
| **Incidents** | Alert/Overview link | Incident timeline, impact, postmortem | 2 | not in MVP |
| **Usage & cost** | Settings | Ingestion/storage/cost per project | 2 | spec only (inside Settings) |

**Navigation model:** fixed left nav rail (240px, 64px collapsed) with six Phase 1 surfaces: Overview, Explore, Dashboards, Services, Alerts, Settings. Incidents and Usage are **not** primary nav in the MVP; Incidents appears in Phase 2 and Usage lives under Settings. The top bar holds the project switcher, command trigger, a **sticky time-context bar** (range, timezone, live/pause), and user menu. A detail drawer (420px) opens from the right for traces, logs, alerts and service metadata; it never covers the query bar. Modals are reserved for destructive confirmation and creation flows; detail inspection uses the drawer.

**URL state:** every investigation is deep-linkable. Time range, signal, filters, query, selected trace/span and dashboard variables live in the URL. Copying the URL reproduces the view (subject to permissions). Back/forward restores the prior investigation state.

**Surface closure (Phase 1):** every MVP need maps to a surface, a journey, and a state. Surfaces without a mock are specified here and will be built from the spine tables.

| Need | Surface | Journey | Mockup | States |
|---|---|---|---|---|
| Find the cause of an alert | Overview → Explore → Services | Flow 1 | Explore, Services, States | loading, empty, timeout, partial, sampled |
| Instrument a new service | Onboarding → Explore | Flow 2 | Onboarding, Explore | no data, error, token expired |
| Create/tune an alert | Alerts → Explore preview | Flow 4 | Alerts (spec), Explore | invalid query, noisy preview, permission |
| View service health | Services → service detail | Flow 1, Flow 4 | Services | no data, degraded, critical |
| Find anything quickly | Command palette | All flows | spec only | no matches, recent, keyboard |
| Understand cost/retention | Settings → Usage (Phase 2) | Flow 5 | spec only | no data, quota, permission |

**Out of Phase 1:** Incidents timeline, SLO burn, deploy annotations, owner catalog, cost estimator, cross-service fallback. These are Phase 2/3 and must not appear as required steps in Phase 1 flows.

## Voice and Tone

**Direct, calm, technical.** Observaki talks like a senior SRE beside you: short sentences, concrete nouns, no hype. It never shouts, never celebrates a routine action, and never blames the user.

| Do | Don't |
|---|---|
| “No data for this service in the last 15 minutes.” | “Oops! Looks like there's nothing here 😢” |
| “Query timed out after 30s. Narrow the time range or add a filter.” | “Something went wrong.” |
| “3 traces match. Showing the slowest first.” | “We found some results for you!” |
| “This trace was sampled. Logs are still available.” | “Trace not found.” |
| “Alert resolved at 14:32 UTC.” | “Great job! The incident is over 🎉” |
| “You don't have permission to view this project.” | “Access denied.” (with no next step) |

**Numbers and units are part of the voice:** always show the unit (`ms`, `s`, `%`, `req/s`, `GB`), the timezone for absolute times, and the time window for a rate. Use “p99”, not “99th percentile” in dense UI; spell it out in tooltips.

## Component Patterns

*Behavioral rules. Visual specs live in `DESIGN.md.Components`.*

| Component | Use | Behavioral rules |
|---|---|---|
| **Command palette** | Global | `⌘K`/`Ctrl+K` opens; fuzzy search across surfaces, services, dashboards, saved queries and actions. `Enter` executes; `→` previews; `Esc` closes. Results announce via `aria-live`. Recent items first when the query is empty. |
| **Query bar** | Explore, alert preview | Mono input with syntax highlight and autocomplete. Drafts persist per project and surface. `⌘Enter` runs. Errors appear inline with caret position and a fix suggestion. The bar never loses content on tab/signal switch. |
| **Signal tabs** | Explore | Metrics / Logs / Traces. Switching preserves time range, service and search terms where valid; it announces what was preserved and what was dropped. Active tab has a 2px underline, not a filled pill. |
| **Time range picker** | All data surfaces | A **global, sticky** control below the top bar: presets + absolute range; timezone visible; live/pause. Changing range updates URL and all panels; per-panel override is explicit and badged. A custom range shows start/end in the user's timezone; “UTC” is always available. |
| **Panel** | Dashboards, Overview | Header has title, unit/legend, overflow (duplicate, inspect, export, remove). Empty state inside the panel; error state with retry; loading skeleton matches final layout. No toast-only errors. |
| **Log row** | Explore > Logs, service detail | Dense mono row: timestamp, severity, service, message, trace link. Click opens the detail drawer; `Enter` opens; `j/k` moves selection. Hover reveals copy/filter/open-trace on desktop; touch reveals on tap. Error rows use a red left border, not a full red background. |
| **Trace waterfall** | Explore > Traces, service detail | Shared time axis; click selects a span; `↑/↓` moves; `Enter` opens detail. Critical path outlined, not recolored. Missing spans show a “partial trace” banner. Zoom/pan is local to the chart; reset is explicit. |
| **Service map** | Services > Map | Stable node positions across refreshes; zoom/pan; click node filters service; click edge filters traces. Health encoded by border + icon + label, never color alone. A “layout” action is explicit, not automatic. |
| **Alert rule builder** | Alerts > New rule | Three steps: signal/query → condition → routing. Live preview of the query and a “test against last 24h” action. Save is explicit; unsaved changes prompt on navigation. The builder can be opened from a log/trace/metric with context prefilled. |
| **Alert row / incident banner** | Alerts, Overview | Severity + state + service + duration. Acknowledge, silence and open investigation are inline. “Silence” requires a duration and reason; it is visible in the alert detail. The banner never covers the query bar. Health taxonomy is fixed: Healthy / Degraded / Critical / Muted / No data; “slow” is a duration condition, not a health status. |
| **Data table** | Logs, traces, services, alerts | Sticky header; explicit sort; column visibility persisted; cursor pagination, never infinite scroll. Selection checkbox has a 44px hit target. Row click opens drawer; `Enter` opens; `j/k` moves. |
| **Empty / error state** | All surfaces | One sentence of what is missing + one primary action + a “learn” link. Error includes request ID and retry. No illustrations, no exclamation marks. |
| **Onboarding checklist** | First login | Four steps: install → send first signal → open first dashboard → create first alert. Each step links to the exact docs/surface; progress persists. Dismissible; re-open from Help. |

## State Patterns

| State | Surface | Treatment |
|---|---|---|
| **Cold load** | Overview, Explore, Dashboards | Skeleton rows/panels that match the final layout. No spinner-only screens. Resolve within 2s or show a “still loading” hint with cancel. |
| **Empty result** | Explore | “No <signal> match this filter.” + “Clear filters” + “Widen time range”. Keep the query bar visible. |
| **Query error** | Explore | Inline error below the bar with syntax caret, request ID, and a fix suggestion. The previous result stays visible and is marked “stale”. |
| **Query timeout** | Explore | “Query timed out after 30s. Narrow the time range or add a filter.” + “Cancel” + “Open as background job” (if available). |
| **Partial data** | Explore, Dashboards | A banner says which signal/panel is partial or sampled; the affected values are marked with a dotted underline and a tooltip. Never silently mix sampled and unsampled data. |
| **Stale data** | Dashboards, Overview | Timestamp + “paused”/“stale” pill; manual refresh. Auto-refresh is opt-in, with an interval control; it pauses when the tab is hidden. |
| **No permission** | Any | Surface hidden in nav if the user has no access; direct URL shows a clear “You don't have permission to view this project” + “Request access”/“Switch project”. No fake empty state. |
| **Rate limited** | Ingest/Explore | “Too many requests. Retry in 30s.” + countdown; the UI disables the action instead of spamming. |
| **Degraded platform** | Global | A thin banner: “Ingestion is delayed. Queries may be incomplete.” with a link to the health page. The banner is dismissible but returns if the state persists. |
| **Offline** | Global | “You're offline. The last view is cached; actions are disabled.” No optimistic writes. |
| **Sampled trace** | Trace detail | Banner: “This trace was sampled out. Logs and metrics for this service are available in this window, but not guaranteed complete.” Link to the service metrics. |
| **Missing trace** | Log detail | “Trace not found for this log. It may have been sampled or expired.” In Phase 1, show logs for the same service/window; cross-service fallback is Phase 2. |
| **Dangerous action** | Settings, dashboards | Confirmation dialog with the exact resource name and consequence; destructive button is red; `Esc` cancels; focus starts on cancel. |

## Interaction Primitives

**Keyboard-first, mouse-supported.** The primary audience is engineers; the keyboard surface is part of the product.

| Shortcut | Action |
|---|---|
| `⌘K` / `Ctrl+K` | Command palette |
| `/` | Focus the query/search bar in the current surface |
| `g o` / `g e` / `g d` / `g s` / `g a` / `g ,` | Go to Overview / Explore / Dashboards / Services / Alerts / Settings |
| `⌘Enter` / `Ctrl+Enter` | Run query |
| `[` / `]` | Shrink / expand time range (preset step) |
| `t` | Cycle signal tab (metrics → logs → traces) |
| `j` / `k` | Move selection down / up in tables and lists |
| `Enter` | Open selected row / execute selected command |
| `Esc` | Close drawer/dialog, exit edit mode, clear palette |
| `?` | Keyboard shortcuts sheet |
| `r` | Refresh current surface (when not editing) |

**Mouse:** click to select, drag to reorder dashboard panels (edit mode only), drag to resize panels. No drag on mobile. Every drag action has a keyboard/menu alternative (“Move up/down”, “Resize”, “Layout”) so WCAG 2.5.7 is satisfied. Hover reveals row actions on desktop; touch reveals via tap. Hover is never the only way to discover an action.

**URL and sharing:** the current view (time range, query, filters, selected entity) is encoded in the URL. “Copy link” copies a permission-aware deep link. A shared link that the recipient cannot access shows the permission state, not a generic 404.

**No modal stacks.** Detail inspection uses a drawer; creation/confirmation uses a single dialog. A dialog never opens another dialog; a drawer can open a dialog only for destructive confirmation.

## Accessibility Floor

- **WCAG 2.2 AA** across the desktop web surface; keyboard-only operation for every task. See [`contrast-check.md`](contrast-check.md) for the current token ratios.
- **Focus visible** on every interactive element using `{colors.focus-ring}`; focus order matches reading order; `Esc` closes the topmost layer.
- **Screen reader:** page surface announced on navigation (“Explore, metrics, project checkout”); live regions for query status, alert state and command palette results; tables have captions and headers; charts have a table alternative and a text summary.
- **Color is never the only signal:** status pills include text and icon; charts use dash patterns and labels; error rows have a left border and severity text.
- **Motion:** respect `prefers-reduced-motion`; no auto-playing animations; pulsing only for firing alerts and disabled under reduced motion.
- **Zoom:** 200% zoom without loss of function; no horizontal scroll for primary controls at 200%.
- **Targets:** 44px minimum hit area for touch-capable surfaces; 32px minimum for desktop dense controls.
- **Content:** no time-limited interactions; auto-refresh is opt-in and pausable.

## Responsive & Platform

| Breakpoint | Behavior |
|---|---|
| `≥1280px` | Full app shell; nav rail 240px; Explore split panes; right drawer 420px. |
| `1024–1279px` | Nav rail collapses to 64px icons; right drawer overlays; dashboard grid 8 columns. |
| `768–1023px` | Read-only; nav becomes a sheet; tables horizontal scroll; no drag/edit. |
| `<768px` | Read-only mobile outside v1; nav sheet; single column; charts simplified. |

The product is optimized for desktop/laptop. Mobile is **read-only outside v1**; it is not an investigation or acknowledgement surface. If a future version supports mobile ack/silence, that is a new decision with its own permission and audit design.

## Inspiration & Anti-patterns

**Lifted from Linear:** keyboard-first discipline, command palette, dense but calm surfaces, status vocabulary. **Lifted from Datadog:** correlation across signals and the “one investigation” flow. **Lifted from Grafana:** familiarity of Explore, time range and dashboards. **Lifted from Sentry/HyperDX:** trace waterfall and issue detail patterns.

**Rejected:** rainbow dashboards (color as decoration), modal stacks (context loss), infinite scroll (lost position), auto-refresh without control (data moves while reading), hover-only actions (inaccessible), “AI magic” that hides the query (trust), and celebratory animations (noise during incidents).

## Key Flows

### Flow 1 — Marina finds the checkout bottleneck (UJ-1, Phase 1)

**Persona + context:** Marina, SRE at a logistics company, owns 180 services. It is 06:12; checkout p99 is above 2s.

1. Marina gets a Slack alert with a deep link. She clicks it; Observaki opens **Overview** filtered to the alert’s project and the alert’s 15m window. The alert banner shows “Checkout p99 > 2s · 5m · Critical” with **Acknowledge**, **Silence** and **Investigate**.
2. She clicks “Investigate”; **Explore** opens on Traces with `service.name = checkout` and `duration > 2000ms`, same 15m range. The query bar and the chips show the same filter; she can remove a chip or edit the query.
3. The trace list is sorted by duration. She opens the slowest trace; the **waterfall** shows `payment-authorizer` consuming most of the time. The critical path is outlined; the DB span is red.
4. She clicks the DB span; the **detail drawer** shows attributes and a “Logs for this trace” tab. The log line says `connection pool exhausted`; she clicks “Open metric” on the pool metric. The metric panel uses the same global time range and shows the pool saturation trend.
5. **Climax:** Marina has the trace, the log, the metric and the query in one drawer. She copies a **context bundle** (trace URL, log query, metric query, service, window) into the incident channel and opens the pool runbook in the team’s existing runbook system. She doesn’t need Observaki to be the remediation console; she needs it to end the diagnosis with evidence, not a dead end.
6. **Resolution:** she creates an alert rule from the metric panel (“pool > 90% for 5m”) and links the trace in the alert’s description. The next on-call starts with the evidence bundle.

**Failure:** if the trace was sampled, the drawer shows “Trace sampled; logs and metrics for this service are available in the same window” and keeps the log/metric path. If the log has no `trace_id`, it falls back to the service + window and says so. If the query times out, the previous result stays visible marked “stale” with a retry.

*Phase 2 extension (not required for MVP): deploy/change annotations, SLO burn, incident entity, and owner catalog are added to the same flow. Until then, the flow ends with an explicit handoff to CI/CD and the runbook.*

### Flow 2 — Ravi gets his first signal in five minutes (UJ-2)

**Persona + context:** Ravi, backend engineer at a 40-person startup, has Observaki running in Docker Compose and a Go service in staging.

1. Ravi opens Observaki for the first time. **Onboarding** shows four steps: install (done), send first signal, open first dashboard, create first alert.
2. He clicks “Send first signal”; a side panel shows the OTLP endpoint, a project token (copy once), and tabs for Go/Python/Node/Collector. He copies the Go snippet.
3. He adds the exporter, deploys to staging, and returns. The checklist auto-detects the first trace and marks the step complete. **Explore** opens on the new service with a “first trace” toast (one-time, dismissible).
4. He clicks “Create dashboard”; the template gallery offers “Go service”, “Kubernetes”, “Postgres”. He picks Go service, sees latency/error/throughput, and saves it.
5. **Climax:** the dashboard URL is shareable; he pastes it in the team channel. A teammate opens it and sees the same data. Ravi realizes he didn't need a observability team to get here.
6. **Resolution:** the checklist marks “first alert” as the next step; Ravi creates a p99 alert from the dashboard panel.

**Failure:** if no data arrives, the checklist shows a diagnostic panel: endpoint, token, clock skew, sampling, firewall. It links to the exact troubleshooting doc, not a generic “contact support”.

### Flow 3 — Ana rolls out to 40 teams (UJ-3, Fase 3)

**Persona + context:** Ana leads platform at a 1,200-engineer company; she is replacing part of Datadog without losing governance.

1. Ana installs the Helm chart on a 3-node cluster, configures OIDC, and maps groups to roles.
2. She creates organizations/projects per domain, sets retention and quotas, and publishes the agent DaemonSet.
3. **Settings > Usage** shows ingestion, storage, series and estimated cost per project. A team exceeds its quota; the UI shows the top labels causing cardinality and offers a drop/aggregate rule.
4. **Climax:** the 40 teams self-serve dashboards and alerts; Ana sees cost per tenant and blocks the noisy project without affecting the others. The platform is a product, not a ticket queue.
5. **Resolution:** she schedules a retention change and reviews the audit log; the next rollout wave is a project template, not a manual setup.

**Failure:** if a tenant sends a cardinality spike, the guardrail contains it, the UI explains which labels caused it, and the other tenants' p95 stays within 10%. Ana can raise the quota or apply the suggested aggregation.

### Flow 4 — On-call creates an alert that does not wake anyone at 3am (Phase 1)

**Persona + context:** Diego, on-call backend engineer, just fixed a latency issue and wants to catch it earlier next time.

1. From the trace/metric panel, Diego clicks “Create alert”. The **Alert rule builder** opens with the query and service prefilled.
2. He chooses “p99 > 800ms for 10m”, adds a “test against last 24h” preview, and sees it would have fired twice (both real).
3. He routes to Slack `#oncall` and a webhook for the incident tool; adds a description with a runbook URL and a silence for the maintenance window. PagerDuty/Opsgenie native channels are Phase 2.
4. **Climax:** the preview shows the alert would have fired at 02:14 and resolved at 02:31 — a real incident, not noise. Diego saves it; the alert appears in the Alerts list with severity, channel and the preview evidence.
5. **Resolution:** the next alert includes the trace/log links and the runbook URL in the description, so the responder starts with context.

**Failure:** if the query is invalid, the builder blocks save and shows the exact error; if the alert would be too noisy (> 20 fires/day in the preview), it warns and suggests a longer `for` or a narrower filter.

## Surface → Component → Token Map

| Surface | Key components | Tokens |
|---|---|---|
| Overview | Incident banner, metric cards, alert list, SLO burn | `{components.app-shell}`, `{components.status-pill}`, `{components.metric-value}` |
| Explore | Query bar, signal tabs, time range, results table, drawer | `{components.query-bar}`, `{components.signal-tab}`, `{components.data-table}` |
| Dashboards | Panel grid, panel header, variables, empty/error | `{components.panel}`, `{components.panel-header}`, `{components.empty-state}` |
| Services | Service map, service header, tabs, trace list | `{components.service-node}`, `{components.trace-waterfall}` |
| Alerts | Rule builder, alert rows, silences, channels | `{components.alert-badge}`, `{components.status-pill}` |
| Settings | Forms, tables, danger zone | `{components.data-table}`, `{components.panel}` |
| Global | Command palette, top bar, nav rail | `{components.command-palette}`, `{components.nav-rail}` |

## Mockups

- [`mockups/explore.html`](mockups/explore.html) — Explore > Traces with query bar, time range, trace list and detail drawer.
- [`mockups/dashboard.html`](mockups/dashboard.html) — service dashboard with Phase 1 panels and alert banner.
- [`mockups/service-map.html`](mockups/service-map.html) — service map with a critical path and alert banner.
- [`mockups/onboarding.html`](mockups/onboarding.html) — first run: checklist, token, waiting for first signal, template gallery.
- [`mockups/states.html`](mockups/states.html) — empty, error, timeout, partial, sampled, no permission, rate limit, degraded, offline, failed panel.

*Spines win on conflict with any mockup. The light theme is specified in `DESIGN.md` tokens; a light mock is a Phase 1 implementation artifact, not a separate design.*
