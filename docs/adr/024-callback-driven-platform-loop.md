# ADR-024: Callback-driven platform loop

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Callback-driven platform loop: `Backend.Run(Handler)` delivers events on the UI thread instead of the app draining `Window.Events()`; interface fixed in §15

## Rationale

Cocoa owns the main loop and Win32 modal move/resize loops only run `WndProc`; callbacks are the one model that works on all four OSes (winit's `ApplicationHandler`)
