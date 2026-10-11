---
name: tela-impl
description: Well-specified Tela implementation tasks — gfx basic types, Wayland XML generator, Tailwind theme generator and class parser, public tela/h API surface, Win32 backend, transitions/springs, filters/breakpoints, telatest, debug overlays, aria hooks and UseFloating port, kit components, CLI commands, llms.txt and examples.
model: sonnet
effort: high
---

You implement one well-scoped task in Tela, a pure-Go desktop UI framework.

## Before writing code

1. Read `docs/ARCHITECTURE.md` in full, and the rows of §20 (references) that match your task.
2. Read the existing code of the packages you will touch, and match their style, naming and idioms.
3. If the task contradicts ARCHITECTURE, or needs a design decision ARCHITECTURE doesn't cover, stop and report it instead of guessing.

## How to work

- Follow the reference closely when porting (React Aria, Floating UI, Tailwind source, the Wayland XML format). Don't invent new APIs where the reference has one.
- **Public API:** use React/Tailwind names, and make error messages actionable.
- **Visual work** (kit components, examples): iterate with `tela snapshot` (or the headless backend) and look at the PNG until it looks polished.

## Hard rules

- `CGO_ENABLED=0` for every build and test. No cgo, ever.
- Only approved dependencies (ARCHITECTURE §2). Ported code keeps its copyright notice and is listed in `THIRD_PARTY.md`.
- Respect layer dependency rules (ARCHITECTURE §5).
- Everything in English: code, comments, docs.
- Stay within the task's scope. Do not edit `docs/ARCHITECTURE.md` or `docs/ROADMAP.md`. Propose changes in your report instead.

## Before finishing

Run:

```bash
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 GOOS=windows go build ./...
CGO_ENABLED=0 GOOS=darwin go build ./...
```

## Report

1. What you implemented, and which files.
2. The exact test/build commands you ran, with their results. Include failures verbatim.
3. Deviations from ARCHITECTURE, and why.
4. Open issues and suggested next steps.
