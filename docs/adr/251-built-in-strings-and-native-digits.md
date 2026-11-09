# ADR-251: Built-in strings and native digits

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-087, ADR-148, ADR-178, ADR-181, ADR-225, ADR-226 and ADR-238 (platform contract v8, remote protocol 12).

## Decision

- **One hook for every text Tela shows itself.** Each has a fixed ID and an English default (`tela.titlebar.close`, `tela.messagebox.ok`, `tela.toast.close`, `tela.menu.quit`, `tela.panel.format`...; `tela.Labels()` lists them). `tela.Label(id, args...)`, `tela.UseLabels()` (a hook), `tela.SetLabelSource(src)` and `tela.LabelsChanged()` live in `tela` over a registry in `internal/core`, below `desktop` and `i18n`. Importing `i18n` installs the catalogs as the source: a message whose key is the ID translates it (`{"tela": {"toast": {"close": "..."}}}`), and `SetLocale` or `Load` call `LabelsChanged`. `i18n.Formatter.Label` gives a locale's text, so kit follows `i18n.Provider`.
- **Consumers.** The fallback title bar, the drawn message box (buttons, default titles) and the role items' labels (`Role.Label`; `AppMenuBar`, `MenuBar` and `Tray` rebuild) use `UseLabels`. `kit.Toaster` gains `AriaLabel` and `CloseLabel` (field, else label).
- **Platform backends get plain strings.** `Desktop.SetStrings(platform.Strings)` carries only the IDs the source answered, `{name}` kept for the backend to fill; core sends it at start and after every `LabelsChanged`. Cocoa rebuilds its menu bar (app menu, Edit, Window, role labels) and uses the texts in NSAlert buttons and the save panel's "Format:"; Win32 renames a message box's buttons through a `WH_CBT` hook; the rest ignore it. Absent IDs keep the OS's text. Remote protocol 12 forwards it.
- **Tela ships no translations**: English defaults only; apps (the gallery) supply Spanish and Arabic in their catalogs.
- **Digits.** Numbers and dates use the locale's CLDR 48 default numbering system (`numbering_gen.go`, from the vendored `defaultNumberingSystems.json`), found by region, script, language: `ar-EG`, `ar-SA`, `ckb`, `sd` Arabic-Indic, `ar-AE`, `ar-MA` Latin. A tag's `-u-nu-` wins. Bare `ar` stays Arabic-Indic although CLDR 48 made it Latin (generator `keepNative`, one line).

## Why

One ID namespace means the title bar, native boxes and kit all follow the app's language with no per-widget plumbing, and layers stay one-way. Sending only translated IDs leaves untranslated native boxes in the user's OS language. Keeping bare `ar` avoids changing every Arabic golden and the gallery.
