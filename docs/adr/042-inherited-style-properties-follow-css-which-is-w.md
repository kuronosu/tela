# ADR-042: Inherited style properties follow CSS, which is wider than §10's list

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Inherited style properties follow CSS, which is wider than §10's list: besides the text properties, `cursor`, `visibility`, `pointer-events`, `user-select`, `word-break`, `overflow-wrap`, `tab-size`, `font-variant-numeric` and `text-shadow` inherit. A `style.Parser` is bound to one theme (it resolves theme colors at parse time); swapping themes uses another parser. `ch` is approximated as 0.613 em (Inter's "0")

## Rationale

Without CSS inheritance `pointer-events-none` on a container and a `Button`'s cursor over its label would not work; theme-bound parsing keeps compiled classes cheap
