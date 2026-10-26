# ADR-059: `telatest` and a public headless entry point

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

`telatest` and a public headless entry point (amends ADR-047; implements §17). `telatest.Mount(testing.TB, tela.Node, ...Option)` runs a tree in a headless window (manual 60 Hz clock, focused window, `tela.Post` delivery, deterministic `UseId` prefix, unmount on cleanup). Locators (`ByRole`, `ByText`, `ByClass`, `Nth`, `Filter`) resolve against the semantics tree and follow Playwright (strict; hidden nodes and implicit generics excluded; `getByText` deepest match, case-insensitive substring unless `Exact`). Actions send real pointer and key events after Playwright-style actionability checks (visible, enabled, scrolled into view, receives events at its center), each followed by a settle: frames run, advancing the clock, until the app is quiet; an app that never goes quiet (a clock goroutine) is logged once and not waited for, rather than failing the test. Held-pointer timers (long press, scrollbar fade) advance only with `Advance`, delayed clicks or while an expectation polls. Expectations retry with a 5 s wall-clock timeout; goldens are exact by default (`Tolerance` opts in). `Render`, `AriaSnapshot` and `LayoutJSON` are the public headless entry point: the CLI's generated runner uses them, so `tela snapshot`/`inspect` work on any module requiring Tela (the phase-1 `Scene` contract still needs the tela module)

## Rationale

One public path to render, inspect and drive a tree, shared by tests, the CLI and the future `tela mcp`; Playwright's model is what users and LLMs know. The extras beyond §17 and the settle policy were chosen while the owner was away (to review)
