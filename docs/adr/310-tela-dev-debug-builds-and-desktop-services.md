# ADR-310: Debuggable `tela dev` builds, and desktop services across reloads

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-116, ADR-184, ADR-187, ADR-214.

## Decision

- **`tela dev --debug`:** dev builds keep DWARF and the symbol table (no `-ldflags=-s -w`) and build every package with `-gcflags=all=-N -l` (Delve's advice), unless `GOFLAGS` sets those flags, which stay the user's (ADR-116's rule; `tela dev` warns). Each reload is a new process, so every start prints its process ID and `dlv attach <pid>`. Without the flag nothing changes.
- **Desktop services across reloads (remote protocol 19):** the host waits (at most 1 s) for the effect the child sets inside `OnDrop`, the drop's final effect; a shortcut taken, refused, revoked or failed is registered again by the next build; a real window with no window of the new build at its promotion waits `HostOptions.ReloadGrace` (1 s) for one it opens later, then closes; a file dialog or message box asked for at a build's start is taken over (or answered with its answer) by the next build's start when it asks for the same one; a starting build under a suspended window gets its frame at once, under one that gets no frames after 100 ms; the real window's `PresentsAlpha` reaches the child, so Windows 11 and macOS backdrops show through.

## Why

A debugger is the one tool the fast dev build ruled out; a flag keeps ADR-116's build speed as the default and costs one full compile of the dependencies when asked for. A reload must look like nothing happened: the host is the one process that sees both builds, so it holds what the child cannot (the drop, the open dialog, the real window), and a grace bounds what a build that no longer asks keeps.
