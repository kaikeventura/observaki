# Story 6.4 — bundle budget and Lighthouse on `/explore`

Run: 2026-11-09T00:55Z · local Linux, Chrome 155.0.8059.39 (headless), Node 26.13.0,
pnpm 12.16.1 · synthetic mock data.

Commands:

```bash
make bundle-budget BUNDLE_BUDGET_OUT=bench/frontend/results/20261109T005535Z-local-explore/bundle-budget.json
make lighthouse LIGHTHOUSE_OUT=bench/frontend/results/20261109T005535Z-local-explore/lighthouse.json
```

## AC1 — bundle budget

Production build (`pnpm --dir web build`, adapter-static). "Initial" is what
`index.html` preloads (SvelteKit runtime + root layout + entry route). A route
budget walks every chunk the route can pull in, including the dynamically
imported uPlot chunk.

| Route | JS gzip | JS raw | Files | CSS gzip | Budget | Result |
|---|---|---|---|---|---|---|
| initial (`index.html`) | **41.5 KB** | 106.5 KB | 15 | 5.7 KB | < 180 KB | pass |
| dashboards | **74.4 KB** | 193.9 KB | 25 | 6.4 KB | < 250 KB | pass |
| explore (reported) | 74.4 KB | 193.9 KB | 25 | 6.4 KB | no budget | pass |

The budget runs at the end of `make build`, so a regression fails `make ci`
locally and the CI quality job (explicit `make bundle-budget` step). Verified
that a lowered budget exits 1 with `bundle budget exceeded`.

## AC2 — Lighthouse on Explore

Reference laptop profile: desktop form factor, 1440×900, 50 Mbps / 40 ms RTT,
1x CPU. LCP from a Lighthouse navigation run with simulated throttling; INP from
a Lighthouse **timespan** run (INP is only collected in unsimulated user flows)
while the script drives real interactions: metrics → logs → traces tab changes,
autocomplete typing (`serv` → arrow → enter), service filter change and a range
change.

| Metric | Measured | Budget | Result |
|---|---|---|---|
| LCP | **390 ms** | < 1 500 ms | pass |
| INP | **81 ms** | < 200 ms | pass |
| CLS | 0.00007 | — | — |
| TBT | 0 ms | — | — |
| Transfer size | 216.3 KB | — | — |

## AC3 — mock Explore

The acceptance flow was exercised by the Lighthouse timespan itself and by
`web/src/routes/explore/page.test.ts`:

- Metrics/Logs/Traces tabs keep the sticky time range and the service filter;
  a query that does not apply to the new signal is dropped with an explicit
  `aria-live` announcement ("Dropped the query … because it does not apply to
  metrics"), a valid one is kept.
- The query bar autocompletes mock fields, values, metric names and MetricsQL
  functions with combobox semantics (Arrow/Enter/Tab/Escape, ⌘Enter runs).
- Signal, range, service and query live in the URL; the page restores them on
  load and rewrites it with `history.replaceState`.

## Limitations

- Not G1 evidence: local machine, synthetic data, no field/CrUX data. Lighthouse
  LCP is simulated (lantern) and INP is a lab timespan, not 28-day field data.
- The real Query Service and URL-driven query results are Phase 1 (Story 8.3).
- The trace panel is a preview table capped at 50 rows; the waterfall and span
  detail are Phase 1.
