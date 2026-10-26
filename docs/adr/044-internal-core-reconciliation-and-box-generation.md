# ADR-044: `internal/core` reconciliation and box generation

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

`internal/core` reconciliation and box generation: a zero Node (`If(false, …)`) is a hole that keeps later positions; each fragment (`Map`, `List`, `Fragment`) is its own key scope; `display: none` generates no box; a block whose content is all inline is one paragraph (one `text.Layout`), mixed content gets anonymous paragraph boxes, and in phase 3 inline elements contribute only their text style; inline-block boxes are block-level with fit-content width (text-align imitated with auto margins); `space-x/y` become margins on block children and gap in flex containers, like Tailwind; a `UseState` setter with an equal comparable value does not rebuild; builds are serialized by a process-wide mutex that guards the current Element

## Rationale

Keeps positions and state stable under conditional rendering and growing lists; matches browser box generation for the content phase 3 renders; the bail-out avoids useless frames
