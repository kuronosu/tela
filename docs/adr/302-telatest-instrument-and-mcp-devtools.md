# ADR-302: `telatest.Instrument`, source lines and `tela mcp`'s DevTools tools

- **Date:** 2026-11-20
- **Status:** Accepted. Amends §17, ADR-174, ADR-299.

## Decision

- **`telatest`:** the mount option `Instrument()` turns on core's instrumentation (ADR-299) from the first build; `App.AriaSnapshotSources()`, `Components()`, `ComponentOf(node)` and `Frames(since)` read it, with the plain types `Component` (name, key, source, props, hooks, builds, `Renders` with their reasons), `Prop`, `Hook`, `Rendering` and `Frame` (time per phase by name, counts, damage). Without it nothing changes.
- **Source lines:** `tela inspect` and `tela mcp` print `[src=settings.go:42]` after each element's line and `"src"` in the layout JSON. A path inside the working directory is relative to it, else `<module>/<path>`, else the base name; an element a library built takes the nearest ancestor's site in the app's module (the line that wrote `kit.Button{…}`).
- **`tela mcp`** gains `state` (a component's props, hooks and why it rendered), `logs` (dev messages and the app's output since the last call), `perf` (time per phase of recent frames or of the last action, with a summary) and `prefer` (dark, reduced motion, high contrast, text scale live; locale live through `i18n`; direction by remounting). A module on an older Tela, found by `go list`, gets an "update tela" answer from them.

## Why

An agent fixing a screen needs the line to edit and why it rendered, as a person in DevTools does; the headless instance is where agents work, so the same data reaches it through the public `telatest` API.
