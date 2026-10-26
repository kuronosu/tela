# ADR-072: Phase-5 review fixes

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Phase-5 review fixes (amends ADR-050, ADR-067, ADR-069). **Alt chords type nothing** on Windows and Linux: a text field inserts a key's `Text` only without Ctrl, Alt and Meta (Option still types on macOS; AltGr is never Alt). **Alt codes on Windows:** a press with Alt and no `WM_SYSCHAR` (the numpad digits of an Alt code) has no `Text`; the `WM_CHAR` that `TranslateMessage` posts with the release of Alt after a numpad key pressed with Alt is delivered after that release as a press and release of their own with an unidentified Code (0x10000 plus the first code unit, as `VK_PACKET` text) and the character as `Key` and `Text`, without Alt in `Mods`. **IME deletions:** `beforeinput` for an IME's delete-surrounding fires before the deletion (deleteContentBackward, deleteContentForward or deleteContent; `IsComposing` during a composition) and `PreventDefault` does not cancel it. **IDs:** a duplicate `Id` names the first mounted element in tree order (HTML's `getElementById`; it was the last one mounted), and the others take over when it unmounts; mounted elements are indexed by `Id` and Labels by `For`. **Editor:** switching to single-line always resets the undo history

## Rationale

Chromium inserts nothing for Alt chords (Windows sends them as `WM_SYSCHAR`, GTK treats them as accelerators) and types the Alt-code `WM_CHAR` as a char event; the KeyEvent contract keeps `Text` on presses, so the character gets its own press rather than riding on the Alt release. Chromium dispatches `beforeinput` for IME deletions and deletes whatever its result (the IME cannot be told its edit was refused). The ID index fixes a lookup lost when the element that registered an ID last unmounted, and the For index makes accessible names O(labels) instead of a tree walk per control (200 labeled inputs over 10k elements: `AriaSnapshot` 2.4 ms → 1.3 ms). Undo steps can hold line breaks even when the value has none
