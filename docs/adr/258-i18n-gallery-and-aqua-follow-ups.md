# ADR-258: i18n, gallery and Aqua follow-ups

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-087, ADR-148, ADR-251, ADR-254.

## Decision

- **Bare `ar` follows CLDR 48** (owner): Latin digits, as ICU 78. `ar-EG`, `ar-SA` and the other Mashriq regions stay Arabic-Indic. The generator's `keepNative` exception is gone; the gallery's Arabic shows Latin digits (its Internationalization page keeps an `ar-EG` row).
- **`i18n.Replace(fsys, pattern)`** makes the files the only catalogs in one step (nothing changes on an error); **`i18n.Unload(tags...)`** removes the catalogs of some locales, or all. The explicit locale stays; a fallback whose catalog is gone is forgotten. Both tell `tela.LabelsChanged`.
- **The drawn message box's window title** follows the language (an effect of its view); a title the message sets stays.
- **`kit.SelectStyle.DefaultIcon`**: the glyph at the end of the trigger when the `Select` has no `Icon` (zero: the chevron down). Both Aqua designs set the double chevron; Tahoe's pop-up keeps it plain, with no accent tile.
- **Aqua contrast.** `destructive-text` is `#c4001a` in light (every Aqua look), and Sequoia's Destructive button takes Tahoe's dark tint and the token. In Tahoe's dark, `glass-strong` is a black 22% tint and `glass-selected` a gray (`rgba(99,99,102,.85)`), so the text of Cancel, an Outline toggle and a pressed toggle passes 4.5:1 on the sheet's saturated backdrop (`TestDestructiveTextContrast`, `TestGlassControlsContrast`).
- **`Regions.SearchField` and `SearchIcon` follow `Controls`**, not `Chrome`: the search field is an input.
- The gallery's stages are translated into Spanish and Arabic (about 300 messages); the captions stay English.

## Why

One rule for digits (ICU's), no state to unload by hand in tests or reloads, and no window left in the old language. A look's default icon needs a style field because an app cannot be asked to set `Icon` on each `Select`.
