# Observaki — Contrast Check (DESIGN tokens)

**Method:** WCAG 2.x relative luminance; text ≥ 4.5:1, large text ≥ 3:1, interactive UI boundary/focus ≥ 3:1.
**Scope:** dark and light tokens used by the Phase 1 mockups. Re-run in CI against the implemented tokens; this file is the design-time reference.

| Pair | Ratio | Requirement | Result |
|---|---:|---:|---|
| `foreground` `#E6EDF7` / `background` `#080B10` | 16.72 | 4.5 | Pass |
| `foreground-muted` `#93A1B5` / `background` | 7.51 | 4.5 | Pass |
| `foreground-subtle` `#8696AB` / `surface-1` `#0E131B` | 6.18 | 4.5 | Pass |
| `foreground-subtle` / `surface-2` `#141B26` | 5.73 | 4.5 | Pass |
| `foreground-subtle` / `background` | 6.54 | 4.5 | Pass |
| `primary` `#22D3EE` / `background` | 10.90 | 4.5 | Pass |
| `primary-foreground` `#041014` / `primary` | 10.67 | 4.5 | Pass |
| `border-strong` `#50647D` / `surface-1` | 3.07 | 3.0 | Pass (interactive) |
| `foreground-light` `#0B1220` / `background-light` `#F6F8FB` | 17.60 | 4.5 | Pass |
| `foreground-muted-light` `#5A6B80` / `background-light` | 5.13 | 4.5 | Pass |
| `foreground-subtle-light` `#5B6B80` / `background-light` | 5.11 | 4.5 | Pass |
| `primary-light` `#0E7490` / `background-light` | 5.04 | 4.5 | Pass |
| `primary-foreground-light` `#FFFFFF` / `primary-light` | 5.36 | 4.5 | Pass (button) |
| `border-light` `#8494A8` / `#FFFFFF` | 3.10 | 3.0 | Pass (interactive) |

**Rules:**
- `foreground-subtle` is the floor for essential metadata; if a value matters during an incident, use `foreground` or `foreground-muted`.
- `border` is decorative; interactive controls use `border-strong`/`border-light`.
- Status colors always carry text + icon; never color alone.
- Every theme change runs axe + this script in CI.
