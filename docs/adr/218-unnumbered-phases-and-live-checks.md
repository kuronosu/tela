# ADR-218: Unnumbered future phases, limitations as phase tasks, live checks that do not block

- **Date:** 2026-11-01
- **Status:** Accepted. Amends ADR-165.

## Decision

- **Future phases have no number.** The roadmap lists them in order; a phase takes the next number when it starts (the next one is 14). Code, docs and comments name a future phase by its subject ("the macOS backend"), not by a number.
- **One responsibility per phase.** The old phase 14 splits into the macOS backend (with the host-platform flavor first), macOS desktop integration, and Linux and Windows parity (with the X11 IME).
- **Every open limitation is a task of the phase that fixes it.** New phases take what no phase did: Linux and Windows parity, API and kit gaps, layout, text and engine fidelity, CPU performance, the dev loop and testing. The roadmap keeps only a short list of limitations that are by design or outside Tela.
- **Live checks do not block.** A check that needs hardware or a desktop the project does not have goes to "Live checks", one list per platform. A phase closes on its CI and local criteria. macOS gets a single session on a real Mac (rented or borrowed) whenever the owner can arrange it. It gates v1.0's macOS support, not a phase. The macOS phases close on `macos-latest` CI.

## Rationale

Phase 14 mixed four responsibilities: core's input model, the X11 IME, a new backend and desktop parity. Later phases each held a real-Mac check, and the owner has no Mac and wants one unscheduled hardware test. Because future phases were numbered, every change of order meant renumbering them in comments. The limitations had grown into one long list, with no order and no phase to fix them.

## What it changes

The structure of `docs/ROADMAP.md`, the end-of-session rule in `AGENTS.md`, and the comments that named phases 14–17 by number.
