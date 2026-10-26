# ADR-052: Linux key translation is a pure-Go XKB subset in `internal/platform/internal/xkb`

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Linux key translation is a pure-Go XKB subset in `internal/platform/internal/xkb`: it compiles the resolved `xkb_v1` keymap a Wayland compositor sends (keycodes, types, compat interpretations for repeat flags and virtual modifiers, symbols) and resolves keys against the compositor's modifier masks without running XKB actions, with Ctrl masked (ADR-050) and libxkbcommon's Caps Lock transformation; it implements libX11 Compose files with libxkbcommon's lookup order and conflict rules. A key that breaks a compose sequence acts on its own (GTK/Qt); without a Compose file dead keys type their spacing accent. Repeat is client-side, folded into the poll timeout; non-repeating keys (modifiers) neither stop nor restart a repeat, whose later events report the current state (GTK/winit). On Wayland a key event waits for the `modifiers` event of the same read so `Mods` is the state after the event. libxkbcommon is the test oracle through a C program kept in `testdata` and never built; Compose files are not read under `go test` unless `TELA_SYSTEM_COMPOSE=1`. `include` statements and more than four groups are rejected

## Rationale

libxkbcommon needs cgo; matching its observable behavior keeps typing identical to other Linux apps, and the oracle checks 16 layouts and every keysym
