# ADR-196: `tela snapshot` drops the `Scene` contract

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

`tela snapshot` no longer renders packages that define `func Scene(w, h float32) *gfx.DisplayList`, the phase-1 contract. A package is rendered from `Previews`, `Preview` or `App` only (ADR-059); `tela dev`, `tela inspect` and `tela mcp` already refused `Scene`. `examples/text`, its last user, goes (ADR-195): what it covers that the framework's tests do not moves to them, built with `h` elements.

## Rationale

`*gfx.DisplayList` lives in `internal/gfx`, so the contract only ever worked inside the Tela module: no application can use it, and removing it breaks no one, so it needs no deprecation cycle. It is the only reason the CLI's generated driver imports internal packages.

## What it changes

The `contractScene` path of `cmd/tela` (driver, snapshot, dev, mcp and their help texts) is deleted, and so are the `--dark` caveat for `Scene` packages and the tests that exercise it. A package that defines only `Scene` gets the usual "defines no Preview, Previews or App" error.
