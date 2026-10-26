# ADR-068: Platform text input and IME

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Platform text input and IME (implements §11 and §15's IME line). `Window.SetTextInput(*TextInputState)` describes the focused field: cursor area (logical px, window coordinates; the zero rect is unknown), `TextPurpose` (normal, password, email, number, phone, URL, search) and multiline flag, optional `SurroundingText` (committed text without the preedit, cursor and anchor); nil means no field and turns the IME off; nil then a state restarts the session (another field); `ResetTextInput` tells the IME the app finished or removed the composition itself. Backends keep a copy, ignore equal states and send at most one update per loop iteration. The IME's output is one `IMEEvent` per atomic update (text-input-v3 `done`, one `WM_IME_COMPOSITION`, one `NSTextInputClient` call), applied as delete surrounding bytes → commit → new preedit with its cursor (both -1: hidden), delivered only when it changes something; an empty preedit without a commit cancels a composition. `IMESessionEvent{Active}` reports the session (focus, a state, the OS attaching the IME); after it ends a composition gets nothing more. All offsets are UTF-8 bytes; Win32 and macOS convert from UTF-16 (`internal/platform/internal/utf16x`). Keys the IME consumes never arrive; `KeyProcess` keys and `KeyEvent.Text` during a composition are not inserted. Wayland: `zwp_text_input_v3` version 1, one per seat; content-type changes restart the session (disable, enable); change cause "other" on enable and reset (GTK); stale `done` serials are applied as the protocol says and the state is never held back waiting for a matching serial; surrounding text over 4000 bytes is cut to a window around the selection. X11 (phase 8) and Win32 (later in phase 5) implement the methods as no-ops for now

## Rationale

One atomic batch maps text-input-v3, IMM32 and NSTextInputClient without reordering and makes the `internal/text/edit` mapping exact (ADR-065); a level-triggered state (the app says what the field is, the backend diffs) keeps text-input-v3's commit counting out of `core`; holding state for matching serials stalls with IMEs that do not answer every commit
