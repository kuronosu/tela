# ADR-189: Leaf packages below every layer

- **Date:** 2026-10-28
- **Status:** Accepted

## Decision

Amends ARCHITECTURE §5. A package under `internal/` that imports nothing from Tela (a leaf: `internal/fmath`, `internal/utf16x`, `internal/editdist`) may be imported by every layer, `platform` included. A leaf stays a leaf: once it imports a Tela package, the layer rules apply to it like any other.

## Rationale

The UTF-16 conversions were in `internal/platform/internal/utf16x` and again in `internal/text/edit`, and the edit distance was written again in `aria`, `semantics`, `core` and `telatest`: the layer rules gave helpers that need no other Tela code no shared home. `internal/fmath` was already used this way without a rule saying so.

## What it changes

`internal/utf16x` moves up from `internal/platform/internal` and replaces the `edit.UTF16*` functions; `internal/editdist` is new. ARCHITECTURE §5 gains one bullet.
