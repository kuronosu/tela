# ADR-231: X11 IME through XIM

- **Date:** 2026-11-07
- **Status:** Accepted. Amends ADR-068 (X11 is no longer a no-op).

## Decision

- X11 has a pure-Go, asynchronous XIM client: codec and COMPOUND_TEXT in `x11/internal/xim`, state machine in `x11/ime.go`. XMODIFIERS names the input method; none (or `@im=none`) means no IME. Servers are found and refound through `XIM_SERVERS`; one that goes away ends the sessions and typing goes on without it.
- Transport 0.0. Encodings: `UTF8_STRING`, `UTF-8`, else COMPOUND_TEXT, whose CJK sets decode through `golang.org/x/text` (about 0.4 MB).
- Styles: on-the-spot (Tela draws the preedit), else over-the-spot (fcitx5's default), else root. One input context per window, focused for a focused non-password field (a password field has no IME, like GTK); spot = bottom-left of the cursor area.
- Keys go to the server while focused; keys it sends back take the normal keyboard path, so Compose runs once. Synchronous messages get `XIM_SYNC_REPLY`. Commits and preedit updates of one read are one `IMEEvent`, flushed before any sent-back key.
- No trigger keys (dynamic flow), authentication or string conversion.

## Rationale

XIM is what every X11 input method serves; libX11 needs cgo and blocks on replies. ibus's XIM speaks only COMPOUND_TEXT. Without a `done` event, one read is the atomic unit.
