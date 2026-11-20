# ADR-299: DevTools over `tela dev`

- **Date:** 2026-11-20
- **Status:** Accepted. Amends §17, ADR-111, ADR-112, ADR-174.

## Decision

Phase 28's inspecting and driving of a running app (the owner, 2026-11-20).

- **Only under `tela dev`.** A running app is one that `tela dev` supervises; release builds and plain `go run` never listen. The supervisor opens a **tools socket** (AF_UNIX, owner-only, like the remote socket) and writes its address to a discovery file in the user cache (`tela/dev/<pid>.json`: address, package, pid), removed at exit. Clients speak JSON-RPC 2.0, one message per line (the codec of `tela mcp`). With one session running, no address is needed.
- **The supervisor relays.** Calls about the tree go to the active child over the remote control channel (kinds `tela.dev.tools.*`, payloads in `internal/devloop`); window-level calls it answers itself: the window list, the presented frames (`Host.Frame`), what the host holds (trays, native menus, dialogs, notifications), and **input**, injected into the Host as events of the real window, so it takes the real path (focus, IME session, hover). Notifications (frame profiles, rebuilds, logs) flow back while a client subscribes.
- **Instrumentation in the child** (in every build, so `telatest` and `tela mcp` read it too, off until a client attaches or a tool turns it on; off it allocates nothing, and the hook points add about 64 KB to `examples/hello`; creation sites need dev mode): per element its component type, key, creation site, hooks (kind, call site, value in a short bounded form), build count and the **reasons** of its last builds (a hook's setter with its site, a context, a `UseValue` read, new props unequal or not comparable, a parent's build, a mount); per frame a **profile** in a ring of the last 600 frames: time per phase (input, animate, build, style, layout, paint, semantics, raster, present), elements built, boxes styled and laid out, damaged area; dev warnings and the app's log. **Pick mode** draws a highlight in the debug overlay (ADR-062) over the box under the pointer, and a click picks it without reaching the app.
- **`tela devtools`** is a Tela app inside the `tela` binary, built with `kit`: Elements (tree; selection's props, computed style, box model, hooks, why it rendered, source line), Accessibility (the semantics tree), Profiler (a timeline of frames by phase; a frame's breakdown and what it rebuilt). `tela dev --devtools` starts it beside the app.
- **`tela mcp --attach`** drives the session's real windows with the headless tools plus a `window` argument and `windows`; screenshots are the presented frames.

## Why

`tela dev` already owns the real windows, the clock and a control channel reserved for tools (ADR-112), so attaching costs no switch, no listener in apps and no new transport. A separate window keeps the app's layout untouched, as Flutter and React DevTools do. Injecting input at the host checks the same path a person's input takes.
