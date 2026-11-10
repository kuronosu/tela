# ADR-267: Small limitations go to a non-blocking list, not to a new phase

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-218.

## Decision

- The roadmap gets a **Known limitations** list, next to Live checks. A limitation found while working goes there when it is small and no planned phase covers its subject. It blocks nothing: a phase that works in the area, or an application that needs it, picks it up and moves it into its own tasks.
- A limitation still becomes a task of a planned phase when that phase's subject covers it (layout, performance, accessibility, the dev loop…), and a bug of the work itself is fixed before the phase closes.
- A new phase is opened only by the owner, for a subject of its own, not to collect what the previous phase left.
- Desktop touch input moves to Deferred until a real need appears.

## Rationale

Phases 20, 21 and 22 were each the follow-ups of the one before, and each close produced another follow-up phase: the chain did not converge, and the planned phases (performance, GPU, packaging, DevTools) did not move.

## What it changes

`docs/ROADMAP.md` (the list, phase 22's follow-ups moved into it or into Dev loop and testing, touch input deferred) and the end-of-session rule in `AGENTS.md`.
