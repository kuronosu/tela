# ADR-226: The app menu bar, menu roles and the title bar overlay in the public API

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-213, ADR-215 and ADR-182.

## Decision

The public names of ADR-224 on ADR-225's contract (details in doc comments).

- **`desktop.AppMenuBar{Menus, AppItems, OnError}`**, a component declared once anywhere. Where `Desktop.SetAppMenu` works (macOS) it is the bar of every window; where it does not and windows have bars (Windows) it is the bar of every framed window without its own `MenuBar`, later windows included; elsewhere `OnError` gets `ErrUnsupported`. The later of two wins, with a dev warning; a window's own `MenuBar` wins on its window.
- **`desktop.Role`** (`RoleUndo` to `RoleHelpMenu`, Electron's names) and `MenuItem.Role`: default English labels, standard items for the edit and window menus. An item's own `OnSelect` replaces the role. Where the platform reports a role item, `desktop` performs it: minimize, zoom, close on the window (the key window for the app bar), the edit roles as the primary-modifier shortcut sent to the focused element (`core.Desktop.PressKey`).
- **`kit.Menubar`**: `AppMenu`, `AppItems`, `OnAppSelect`; `MenubarMenu.Role`. A native app bar renders `desktop.AppMenuBar`, else the bar is drawn where declared.
- **`kit.TitleBar`** keeps free the room of a non-empty `WindowState.TitleBarControls` and draws no controls; `kit.WindowControls` draws nothing then.
- **telatest:** `MountApp` applies `TitleBarOverlay` and `OnQuit`; `RequestQuit`, `SetTitleBarControls`, `AppMenuBar`, `AppMenuItems`, `ChooseAppMenuItem`, options `TitleBarControls` and `WithUnsupported`, `Unsupported(AppMenu)`. Headless has the app bar by default and kit draws unless `PreferNativeMenus`, so kit's goldens stay.

## Why

Electron's names are the vocabulary LLMs know; kit gets one switch, as in ADR-215. `desktop` performing roles gives one behavior everywhere.

## What it changes

`desktop` (the drawn message box no longer dot-imports `h`, which has a `Role`), `internal/core`, `kit`, `telatest`, `docs/llms.txt`, the wiki.
