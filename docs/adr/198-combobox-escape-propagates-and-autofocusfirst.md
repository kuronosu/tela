# ADR-198: A combobox's Escape propagates when unused; `ComboboxOptions.AutoFocusFirst`

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

`aria.UseCombobox` consumes Escape (`PreventDefault`) only when it did something with it: it closed a list that can close (uncontrolled, or controlled with an `OnOpenChange` that hears the request) or it reverted the text. Otherwise the key goes on to the enclosing overlay or dialog. A controlled `Open` with no `OnOpenChange` (the always-open list of a command palette) therefore cannot swallow Escape. `ComboboxOptions.AutoFocusFirst` (new, default false) highlights the first item whenever the list is open and nothing is highlighted: on opening and after the text changes, so Enter chooses it with no arrow pressed (React Aria's Autocomplete does this unless `disableAutoFocusFirst`).

## Rationale

React Aria stops Escape only when the list was open or the text was reverted. Tela also stopped it for an open list that never closes, and let it through after reverting the text of a selected item. A single `DefaultFocusedKey` cannot name "the first match" of a list that is filtered while typing.

## What it changes

`aria/combobox.go` (`onKeyDown`, `sync`, the open step), `boolState.listens`. `examples/dashboard`'s palette drops its own `OnKeyDown` for Escape and Enter.
