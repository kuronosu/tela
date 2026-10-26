# ADR-004: Internally three trees

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Internally three trees (Node, Element, Box) and a phased pipeline with dirty flags, like Flutter

## Rationale

Cheap rebuilds, 0% idle CPU, bounded relayout/repaint
