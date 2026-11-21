# ADR-310: Debuggable `tela dev` builds, and desktop services across reloads

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-116.

## Decision

- **`tela dev --debug`:** dev builds keep DWARF and the symbol table (no `-ldflags=-s -w`) and build every package with `-gcflags=all=-N -l` (Delve's advice), unless `GOFLAGS` sets those flags, which stay the user's (ADR-116's rule; `tela dev` warns). Each reload is a new process, so every start prints its process ID and `dlv attach <pid>`. Without the flag nothing changes.

## Why

A debugger is the one tool the fast dev build ruled out; a flag keeps ADR-116's build speed as the default and costs one full compile of the dependencies when asked for.
