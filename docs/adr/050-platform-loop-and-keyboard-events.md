# ADR-050: Platform loop and keyboard events

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Platform loop and keyboard events. Backends share `internal/platform/internal/loop` (FIFO queue, coalesced wake, quit checks, ConfigureEvent application, frame-due rule with an optional display-signal gate, pacing deadline, re-entrancy guard for Win32 `WndProc`); each backend keeps only its OS I/O. `KeyEvent` follows winit's three fields: `Code` is a `KeyCode` named by the W3C `code` values (`MetaLeft`, not `SuperLeft`; unidentified keys carry the native scancode), `Key` is a W3C `key` string reflecting every modifier but Ctrl, `Text` ignores Ctrl and never holds control characters (Enter, Tab, Backspace, Escape and Delete give "" and are handled through `Key`), empty on release. Text is inserted only without Ctrl or Super; AltGr is never reported as Ctrl or Alt. `Modifiers` stays Shift/Ctrl/Alt/Super (no lock states). On focus loss every backend sends synthetic releases of held keys before `FocusEvent{false}`; keys held at focus gain produce no presses and their releases are dropped. Backends generate repeats; `Mods` is the state after the event. Linux backends map evdev codes through one shared table. The full contract is in `internal/platform/doc.go`

## Rationale

The loop was duplicated in three backends and two more were coming; DOM names are what LLMs and `telatest` users know; synthetic presses on refocus fire shortcuts, control characters in `Text` cause insertion bugs, and lock bits break `Mods == ModCtrl`
