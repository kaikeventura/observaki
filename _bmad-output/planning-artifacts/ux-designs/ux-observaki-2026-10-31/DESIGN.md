---
name: Observaki
description: Plataforma de observabilidade open source, self-hosted e OTel-native. UI data-dense, dark-first, feita para investigação durante incidentes.
colors:
  background: '#080B10'
  background-light: '#F6F8FB'
  surface-1: '#0E131B'
  surface-1-light: '#FFFFFF'
  surface-2: '#141B26'
  surface-2-light: '#F1F4F8'
  surface-3: '#1B2432'
  surface-3-light: '#E8EDF4'
  border: '#2E3B4D'
  border-light: '#8494A8'
  border-strong: '#50647D'
  foreground: '#E6EDF7'
  foreground-light: '#0B1220'
  foreground-muted: '#93A1B5'
  foreground-muted-light: '#5A6B80'
  foreground-subtle: '#8696AB'
  foreground-subtle-light: '#5B6B80'
  primary: '#22D3EE'
  primary-light: '#0E7490'
  primary-foreground: '#041014'
  primary-foreground-light: '#FFFFFF'
  primary-hover: '#67E8F9'
  primary-hover-light: '#155E75'
  warning: '#F5A524'
  warning-light: '#B45309'
  warning-foreground: '#1A1206'
  success: '#34D399'
  success-light: '#047857'
  danger: '#F87171'
  danger-light: '#B91C1C'
  info: '#60A5FA'
  info-light: '#1D4ED8'
  focus-ring: '#22D3EE'
  focus-ring-light: '#0891B2'
typography:
  sans:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 14px
    fontWeight: '400'
    lineHeight: '1.5'
  sans-sm:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 13px
    fontWeight: '400'
    lineHeight: '1.45'
  caption:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 12px
    fontWeight: '500'
    lineHeight: '1.4'
  label:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 11px
    fontWeight: '600'
    lineHeight: '1.2'
    letterSpacing: '0.04em'
  display:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 28px
    fontWeight: '600'
    lineHeight: '1.15'
    letterSpacing: '-0.02em'
  h1:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 24px
    fontWeight: '600'
    lineHeight: '1.2'
    letterSpacing: '-0.015em'
  h2:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 20px
    fontWeight: '600'
    lineHeight: '1.25'
    letterSpacing: '-0.01em'
  h3:
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif'
    fontSize: 16px
    fontWeight: '600'
    lineHeight: '1.3'
  mono:
    fontFamily: 'JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, monospace'
    fontSize: 13px
    fontWeight: '400'
    lineHeight: '1.5'
  mono-sm:
    fontFamily: 'JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, monospace'
    fontSize: 12px
    fontWeight: '400'
    lineHeight: '1.45'
rounded:
  sm: 4px
  md: 6px
  lg: 10px
  xl: 14px
  full: 9999px
spacing:
  '0': 0px
  '1': 4px
  '2': 8px
  '3': 12px
  '4': 16px
  '5': 20px
  '6': 24px
  '8': 32px
  '10': 40px
  '12': 48px
  '16': 64px
components:
  app-shell:
    background: '{colors.background}'
    foreground: '{colors.foreground}'
    navWidth: 240px
    topbarHeight: 56px
  nav-rail:
    background: '{colors.surface-1}'
    border: '{colors.border}'
    activeBackground: '{colors.surface-3}'
    activeForeground: '{colors.primary}'
  panel:
    background: '{colors.surface-1}'
    border: '{colors.border}'
    radius: '{rounded.lg}'
    padding: '{spacing.4}'
  panel-header:
    foreground: '{colors.foreground}'
    muted: '{colors.foreground-muted}'
    height: 44px
  query-bar:
    background: '{colors.surface-2}'
    border: '{colors.border}'
    focusBorder: '{colors.primary}'
    radius: '{rounded.md}'
    font: '{typography.mono}'
  signal-tab:
    foreground: '{colors.foreground-muted}'
    activeForeground: '{colors.foreground}'
    activeBorder: '{colors.primary}'
  metric-value:
    foreground: '{colors.foreground}'
    font: '{typography.mono}'
    size: 24px
  status-pill:
    healthyBackground: 'rgba(52, 211, 153, 0.12)'
    healthyForeground: '{colors.success}'
    warningBackground: 'rgba(245, 165, 36, 0.14)'
    warningForeground: '{colors.warning}'
    criticalBackground: 'rgba(248, 113, 113, 0.14)'
    criticalForeground: '{colors.danger}'
    radius: '{rounded.full}'
  alert-badge:
    critical: '{colors.danger}'
    warning: '{colors.warning}'
    info: '{colors.info}'
    text: '{colors.background}'
  log-row:
    background: '{colors.surface-1}'
    alternateBackground: '{colors.surface-2}'
    foreground: '{colors.foreground}'
    muted: '{colors.foreground-muted}'
    errorForeground: '{colors.danger}'
    font: '{typography.mono-sm}'
  trace-waterfall:
    bar: '{colors.primary}'
    barError: '{colors.danger}'
    barMuted: '{colors.foreground-subtle}'
    grid: '{colors.border}'
  service-node:
    background: '{colors.surface-2}'
    border: '{colors.border}'
    healthy: '{colors.success}'
    warning: '{colors.warning}'
    critical: '{colors.danger}'
    radius: '{rounded.md}'
  command-palette:
    background: '{colors.surface-2}'
    border: '{colors.border-strong}'
    selectedBackground: '{colors.surface-3}'
    selectedForeground: '{colors.primary}'
    radius: '{rounded.lg}'
  data-table:
    headerBackground: '{colors.surface-2}'
    rowHover: '{colors.surface-2}'
    border: '{colors.border}'
    font: '{typography.sans-sm}'
  empty-state:
    foreground: '{colors.foreground-muted}'
    icon: '{colors.foreground-subtle}'
    action: '{colors.primary}'
---

## Brand & Style

**Observaki is an instrument panel, not a dashboard toy.** It is built for the moment when a service is down and a human needs to find the cause before the next alert fires. The visual posture is calm, dense and precise: dark-first because on-call happens at 3am, light mode available for daylight review, and color reserved for meaning rather than decoration.

The interface should disappear into the data. Chrome recedes: thin borders, quiet surfaces, no gradients, no illustrations, no celebratory animation. The one moment of visual energy is the **signal**: cyan for interactive focus, amber for warning, red for error, green for healthy. If a pixel is not helping someone read a number, a trace or a log line, it is not doing its job.

Observaki inherits **shadcn-svelte/bits-ui** behavior and Tailwind 4 tokens. This DESIGN.md defines the brand-layer delta: the dark-first palette, the data typography, the density rules and the component anatomy specific to observability. The component library owns focus behavior, keyboard semantics and ARIA; this contract owns the visual language.

## Colors

**Dark is the default.** `{colors.background}` is near-black with a blue undertone, not pure black; it reduces halation around dense text and makes the cyan/amber/red signals readable for long sessions. `{colors.surface-1}` is the panel field, `{colors.surface-2}` is the inset/query field, and `{colors.surface-3}` is hover/selected state. `{colors.border}` is deliberately low-contrast and decorative; interactive containers (inputs, search, buttons, project switcher) use `{colors.border-strong}` at ≥3:1. Never use a decorative border as the only affordance.

**Cyan (`{colors.primary}`)** means “this is interactive or currently selected”: primary buttons, active nav, focus ring, query bar focus, selected trace span. It is never used for “success” or “healthy” — that is green’s job.

**Amber (`{colors.warning}`)** means “degraded, pending or at risk”: warning alerts, SLO burn near budget, stale data, sampling active. **Red (`{colors.danger}`)** means “error, destructive or critical”: error spans, critical alerts, delete actions. **Green (`{colors.success}`)** means “healthy/resolved”: healthy services, resolved alerts, successful deploys. **Blue (`{colors.info}`)** is informational and change markers (deploy annotations).

**Never encode state by color alone.** Every status pill carries a label and an icon; charts use shape/dash patterns as well as color; alerts show severity text. The palette is checked for deuteranopia/protanopia contrast.

Light mode mirrors the roles with darker, higher-contrast values (`{colors.primary-light}`, `{colors.warning-light}`, etc.) on `{colors.background-light}`. Surfaces stay white/neutral; the data colors do not get “pastel”. Interactive borders use `{colors.border-light}` at ≥3:1; panel separators may be lighter but never the only affordance.

**Contrast floor:** body text ≥4.5:1, large text ≥3:1, interactive borders/focus ≥3:1. `{colors.foreground-subtle}` is the lightest text token allowed for essential metadata; if a value matters in an incident, it does not use the subtle token. The light primary is darker (`{colors.primary-light}`) so white text remains readable; the dark primary uses dark text (`{colors.primary-foreground}`). Every theme change is verified with axe and a contrast script in CI.

**Avoid:** purple/blue gradients, neon glows, rainbow dashboards, red for non-error emphasis, amber for decoration, cyan for success. Color is a signal, not a theme.

## Typography

**Geist/Inter for the interface; JetBrains Mono for the data.** The sans face carries labels, navigation, prose and buttons. The mono face carries query text, metric values, IDs, timestamps, log bodies and trace attributes. Monospace is not “code decoration”; it is how users compare values vertically and spot a differing digit.

The scale is compact: `{typography.display}` for the Overview headline only, `{typography.h1}` for page titles, `{typography.h2}` for panel group titles, `{typography.h3}` for panel titles, `{typography.sans}` for body, `{typography.sans-sm}` for dense tables, `{typography.caption}` for metadata, `{typography.label}` for uppercase field labels. `{typography.mono}` is the default for any value a user might copy.

**Numbers are tabular.** Metric values, durations, counts and percentages use `font-variant-numeric: tabular-nums` and right-align in tables. A metric that changes width as it updates is a bug. Long IDs truncate in the middle with a copy affordance, never with an ellipsis that hides the distinguishing suffix.

**Line length:** prose max 72ch; logs and queries never wrap by default (horizontal scroll), with a “wrap” toggle. Do not set body copy in mono, and do not set query text in sans.

## Layout & Spacing

**Desktop-first, data-first.** The app shell is a 240px nav rail + 56px top bar + fluid content. The content grid is 12 columns with 16px gutters; data surfaces can ignore the max-width and fill the viewport. The baseline is 4px (`{spacing.1}`); the standard rhythm is 8/12/16/24/32.

**Density modes:** Comfortable (default, 40px table rows) and Compact (32px rows, 12px body) for on-call power users. Density is a per-user setting, persisted; it affects tables, logs and trace lists, not marketing pages (there are none).

**Explore is a split pane:** query bar on top, results below; optional left facet rail (240px) and right detail drawer (420px). The time range picker is always visible; changing it updates the URL so a view is shareable. Dashboards use a 12-column grid with 16px gutters and a 320px minimum panel width.

**Breakpoints:** `≥1280px` full (nav rail + split panes); `1024–1279px` compact (nav rail collapses to 64px icons, right drawer overlays); `768–1023px` read-only (no drag/edit, tables horizontal scroll); `<768px` read-only mobile outside v1. Observability is not a phone-first product; do not pretend otherwise.

## Elevation & Depth

**Borders before shadows.** Panels are separated by `{colors.border}`, not by drop shadows. Elevation is reserved for things that actually float: command palette, popovers, dropdowns, dialogs and the detail drawer. Those use a single soft shadow and `{colors.border-strong}`; there are no stacked shadows and no “card hover lift” in data surfaces.

Selected rows use `{colors.surface-3}` and a 2px left accent in `{colors.primary}`, not a shadow. Modals dim the background with a 60% scrim; only one modal stack is allowed. Nothing in the data plane should look “raised” — the data is the hierarchy.

## Shapes

**Sharp enough to feel like a tool, soft enough to be modern.** Controls and inputs use `{rounded.md}` (6px); panels and drawers use `{rounded.lg}` (10px); dialogs/command palette use `{rounded.xl}` (14px); tags and status pills use `{rounded.full}`. Badges use `{rounded.sm}` (4px). There are no pill-shaped buttons and no fully round avatars in data tables.

The corner language is consistent: if it accepts input, 6px; if it contains data, 10px; if it floats, 14px. Do not mix 8px and 12px “because it looked better”.

## Components

**App shell.** `{components.app-shell}` background, nav rail `{components.nav-rail}` with a 240px width (64px collapsed), top bar 56px. The top bar holds project switcher, global search/command trigger, time range (on data surfaces), refresh/pause, and user menu. The nav rail holds Overview, Explore, Dashboards, Services, Alerts, Incidents, Settings; active item uses cyan text + surface-3 background + 2px left accent.

**Query bar.** `{components.query-bar}` is a mono input with syntax highlighting, autocomplete and a visible “Run” affordance. It is the hero control of Explore. It supports a simple filter mode and a SQL/MetricsQL mode; the mode switch is a segmented control, not a hidden menu. Errors appear inline below the bar with a caret position and a “fix” suggestion. The bar never loses content on navigation; drafts are preserved per project.

**Signal tabs.** Metrics / Logs / Traces tab strip at the top of Explore. Active tab uses a 2px cyan underline and foreground text; inactive uses muted text. Switching tabs preserves time range, service filter and search terms where semantically valid; it does not silently drop filters.

**Time range picker.** A single control with relative presets (5m, 15m, 1h, 6h, 24h, 7d, 30d) and an absolute range. It shows the resolved timezone and a “live” toggle. Changing the range updates the URL. The control is keyboard-operable and announces the selected range.

**Panel.** `{components.panel}` with a 44px header: title, optional unit/legend, overflow menu (duplicate, inspect, export, remove). Body is the chart/table/stat. A panel with no data shows the empty-state pattern, not a blank rectangle. A panel that failed shows an inline error with retry, not a toast that disappears.

**Metric value.** `{components.metric-value}` uses mono, tabular numbers, 24px for the hero stat, 13px for secondary. Trend delta uses an arrow + signed value; color follows semantic direction only when the metric has a known “good” direction (e.g., error rate up is bad; availability up is good). Unknown direction is neutral.

**Log row.** `{components.log-row}` is a dense mono row: timestamp (12px muted), severity pill, service, message, trace link. Error rows use a red left border and red severity text, not a full red background. Hover reveals “copy”, “open trace”, “add filter” on desktop; touch users tap to reveal. Long messages truncate with a wrap toggle; the row expands to a detail drawer, not a modal.

**Trace waterfall.** `{components.trace-waterfall}` uses horizontal bars on a shared time axis. Healthy spans are cyan/neutral; error spans are red; the critical path is emphasized with a 2px outline, not a different color. The time axis has grid lines in `{colors.border}`. Clicking a span selects it and opens the detail drawer; keyboard up/down moves selection. A trace with missing spans shows a “partial trace” banner with sampling explanation.

**Service node.** `{components.service-node}` is a compact card with service name, request rate, error rate and p99. Border color encodes health (green/amber/red) plus a status icon. Edges show direction and latency; clicking an edge filters traces. The map is zoomable/pannable; it never auto-layouts on every refresh (stable positions matter for memory).

**Alert badge / status pill.** `{components.status-pill}` always includes text: “Healthy”, “Degraded”, “Critical”, “Muted”, “No data”. The badge is not the only signal; the row also carries the numeric value and time. Critical uses red, warning amber, info blue, healthy green. A pulsing animation is allowed only for “firing” and must respect `prefers-reduced-motion`.

**Command palette.** `{components.command-palette}` opens with `⌘K`/`Ctrl+K`; fuzzy search across surfaces, services, dashboards, saved queries and actions. The selected row uses surface-3 + cyan text; `Enter` executes, `→` previews, `Esc` closes. Results announce via `aria-live`. The palette is the fastest path to any surface; it must not be a decorative search box.

**Data table.** `{components.data-table}` uses a sticky header, 40px rows (32px compact), right-aligned numbers, left-aligned text, and a 1px border between rows. Sorting is explicit; no hidden default sorts. Column visibility is a menu, persisted per user. Selection uses a checkbox with a 44px hit target. The table never paginates with infinite scroll; use cursor pagination with a visible “Load more” or page control.

**Empty state.** `{components.empty-state}` is one sentence of what is missing + one primary action + a secondary “learn” link. No illustrations, no exclamation marks, no “Oops”. Example: “No traces match this filter. Widen the time range or remove the status filter.” with “Clear filters”.

## Do's and Don'ts

| Do | Don't |
|---|---|
| Use color for meaning: cyan=interactive, amber=warning, red=error, green=healthy | Use color decoratively or as the only state indicator |
| Keep data surfaces dense and scannable | Add marketing whitespace, hero illustrations or oversized cards |
| Use mono + tabular numbers for values, IDs, queries and logs | Set body copy in mono or use proportional numbers for metrics |
| Use borders and surface steps for hierarchy | Stack drop shadows or “lift” cards on hover |
| Make every surface keyboard-reachable; show focus | Rely on hover-only actions or hidden gestures |
| Put the time range and filters in the URL | Reset context silently when switching signal or project |
| Show partial/stale/empty/error states explicitly | Leave a blank panel or a toast that disappears |
| Use one modal stack and a side drawer for details | Stack dialogs or hide critical context behind a modal |
| Respect `prefers-reduced-motion` | Animate charts, pulses or page transitions by default |
| Write direct, calm microcopy | Use exclamation marks, “Oops”, emoji or anthropomorphic errors |
