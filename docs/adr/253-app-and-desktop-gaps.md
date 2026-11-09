# ADR-253: App and desktop gaps

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-181, ADR-209, ADR-210, ADR-211, ADR-213, ADR-225, ADR-226 and ADR-228 (contract v9, protocol 13; numbers reconciled at merge).

## Decision

- **Close and quit in the tree:** `tela.UseCloseRequest(func(CloseRequestEvent))` and `UseQuitRequest(func(QuitRequestEvent))` run, in mount order, before `App.OnClose` / a Window's `OnClose` / `App.OnQuit`; `PreventDefault` skips the default (Tauri's onCloseRequested, Electron's close and before-quit).
- **Text drops:** an editable Input or Textarea accepts text/plain without handlers; the drop inserts at the drop point (`insertFromDrop`) unless `OnDrop` calls `PreventDefault`.
- **Saved positions:** `WindowOptions.Position` is fitted at creation to the work areas present (`platform.FitOnScreens`); headless `Options.Screens`, telatest `Monitors`.
- **Stores** watch their file while watched (`internal/watch`, moved from `cmd/tela`) and merge key by key: a change not yet written wins, else the file; writes read first.
- **Launches:** `LaunchEvent.Source` (forwarded, opened by the OS); `desktop.Launch.Source` (`LaunchStart`, `LaunchForwarded`, `LaunchOpened`) replaces `Forwarded`.
- **Early app events:** the Host holds app-scoped events until the main window's first commit; cocoa holds none.
- `desktop.AppMenuBar.NativeOnly`: only macOS's bar, no window bars.
- **Roles:** `platform.Association.Role` (`HandlerDefault`, Editor, Viewer, Shell, None), `desktop.Association.WithRole`: CFBundleTypeRole on macOS (None: rank None); None declares the type only on Linux and Windows.

## Why

State that decides lives in components; HTML's and Launch Services' rules are what users expect; one rule in core beats one per backend.
