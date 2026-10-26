# ADR-030: `internal/text` loads TrueType outlines

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

`internal/text` loads TrueType outlines (glyf/gvar) and evaluates item variation stores (HVAR, MVAR, COLR) itself, ported from go-text with fusion-safe float64 arithmetic; go-text stays the parser and the shaper. CFF2 outlines and shaper advances still come from go-text

## Rationale

go-text accumulates variation deltas in float32, which Go fuses into multiply-adds on arm64 and amd64 v3: variable-font outlines differed across architectures, breaking ADR-027
