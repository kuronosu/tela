# ADR-037: Tailwind v4.3.3 `theme.css` and `preflight.css` are vendored in

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Tailwind v4.3.3 `theme.css` and `preflight.css` are vendored in `internal/style/gen/tailwind/` and `go generate` emits the default theme from them. The suggestion vocabulary is the set of utilities the parser supports, expanded with the generated theme keys, not a list taken from Tailwind (amends 018)

## Rationale

Tailwind defines its utilities in TypeScript, not CSS; generation stays offline and reproducible, and suggestions never name a class Tela cannot parse
