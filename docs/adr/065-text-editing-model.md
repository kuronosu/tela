# ADR-065: Text editing model

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Text editing model: `internal/text/edit` (on `text` only) holds the committed value, a `text.Selection` in displayed-text offsets, an IME composition and the undo history; the caller supplies the `*text.Layout` of `Editor.Display` (the value with the preedit inserted, one U+2022 per grapheme cluster in password mode) to geometric commands and rebuilds it only when `DisplayVersion` changes, so cursor moves never reshape. Commands are planned (`Plan`, no mutation, W3C Input Events Level 2 `InputType` and `Data`) and applied (`Apply`) so core fires a cancelable `beforeinput` in between; a plan applied after any other change is a no-op. Single-line fields turn inserted line breaks into spaces and strip them from programmatic values (Chromium paste, HTML sanitization); multi-line normalizes to LF; controls but tab are stripped from inserted text. `maxlength` counts UTF-16 code units (HTML) and truncates insertions on grapheme boundaries, at commit for compositions. Backspace deletes a code point except whole emoji sequences and a variation selector with its base (Blink's BackspaceStateMachine); Delete removes a grapheme cluster; password mode deletes whole clusters, disables copy/cut and hides word structure. Undo coalesces typing, Backspace runs and Delete runs until a selection change, another kind of edit, white space after a word (VS Code) or a pause over 500 ms on the caller's clock (ProseMirror); a committed composition, with the selection it replaced, is one step; `SetText` resets the history or records one step, and an equal value changes nothing. While composing every command is a no-op (Parley) and `FinishComposition` commits the preedit without changing the display

## Rationale

Browser behavior is what users compare against; plan/apply makes `beforeinput` cancelable without revert logic; keeping the selection in display offsets lets the goal column and drag unit survive and the layout be used as is; Chromium's single typing run per undo step was judged too coarse
