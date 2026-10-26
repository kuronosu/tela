# ADR-175: A `func() tela.Node` Preview is a component

- **Date:** 2026-10-26
- **Status:** Accepted

## Decision

When a package's `Preview` is a `func() tela.Node`, `tela snapshot`, `tela inspect`, `tela dev` and `tela mcp` mount `tela.C(Preview)` instead of calling `Preview()` and mounting its result (amends ADR-092). Its hooks then belong to a component of its own, as in `tela.App{Root: tela.C(Screen)}`. A `Preview` that returns a `tela.Component` is still called and its result mounted, and what a `func() tela.Node` Preview returns is unchanged: one that returns `tela.C(Screen)` mounts the same screen inside one more boundary, with the same pixels.

## Rationale

`func Preview() tela.Node` reads like a component, and writing `UseState` in it is the first thing an agent does; called as a plain function it panicked with "UseState called outside a component build", found while checking `tela mcp` with a fresh package. Wrapping removes the trap without changing any screen that worked.
