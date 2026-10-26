# ADR-047: Public API of phase 3

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Public API of phase 3: `tela.Node`, `Child`, `Prop`, `Color`, `StyleProp`, `Shadow`, `Gradient` and `GradientStop` are aliases of internal types; the text-color prop is `tela.TextColor` (`Color` is the type); `tela.Previews` is `map[string]func() Node` and the default preview is the first name in byte order; `h` exports exactly its pinned list. `tela snapshot`/`inspect` detect `Previews`, then `Preview()`, then the phase-1 `Scene` by parsing the package; `inspect` defaults to the layout tree (JSON: `rect` as x/y/width/height, zero edges omitted) until `--aria` arrives in phase 4. The generated runner imports `internal/core`, so the CLI only works for packages inside the tela module until a public headless entry point (`telatest`) exists

## Rationale

Shortest literals for users; aliases keep one type per concept without exposing internal import paths; the CLI limitation is accepted until `telatest`
