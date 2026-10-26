# ADR-040: A minimal `UseState`

- **Date:** 2026-10-12
- **Status:** Superseded by ADR-051 (in part or in full; see it)

## Decision

A minimal `UseState` (hook slots on the Element; the setter is safe from any goroutine and schedules a rebuild) ships in phase 3; the other hooks, `tela.C`/`tela.CP` and hook-order checks stay in phase 4. Event props such as `OnClick` are accepted from phase 3 and dispatched from phase 4

## Rationale

The phase-3 acceptance example, §1's `Counter`, compiles and renders as written
