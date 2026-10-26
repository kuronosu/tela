# ADR-043: `internal/layout` implements §9's protocol with Taffy's request model

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

`internal/layout` implements §9's protocol with Taffy's request model: known dimensions plus available space per axis (definite, min-content, max-content), as a size query or a full layout, cached per node by the whole request; the parent sets the location. Relayout boundaries are boxes with a px width and height and non-visible overflow in both axes. `absolute` boxes resolve against the nearest positioned ancestor (CSS), not the parent (Taffy). `overflow: auto` reserves no scrollbar space (overlay scrollbars); `scroll` does. Where Taffy and Chromium differ, layout follows Chromium (flex container fit-content sizing, CSS 2 §10.3.7 for absolute boxes, and others listed in `internal/layout/doc.go`)

## Rationale

Taffy's model is what CSS flex and block need; Flutter's min/max constraints cannot express min-/max-content requests. Tight constraints of a flex item come from its content, so they cannot stop relayout. Chromium is the reference users compare against
