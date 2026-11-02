# ADR-226: The app menu bar, menu roles and the title bar overlay in the public API

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-213, ADR-215 and ADR-182.

## Decision

ADR-224's names on ADR-225's contract (details in doc comments).

- **`desktop.AppMenuBar{Menus, AppItems, OnError}`**, declared once: macOS's bar for every window; on Windows every framed window's bar unless it has its own `MenuBar`; elsewhere `ErrUnsupported`.
- **`desktop.Role`** and `MenuItem.Role` (Electron's names): default labels, standard Edit and Window menus; an item's own `OnSelect` replaces the role; where the OS does not perform a role, `desktop` does (window roles on the window, edit roles as the shortcut sent to the focused element).
- **kit:** `Menubar.AppMenu`, `AppItems`, `OnAppSelect`, `MenubarMenu.Role`; `TitleBar` keeps the reported buttons' side free and draws no controls.
- **telatest:** `RequestQuit`, `SetTitleBarControls`, `AppMenuBar`, `ChooseAppMenuItem`, options `TitleBarControls` and `WithUnsupported`; kit stays drawn unless `PreferNativeMenus`.

## Rationale

Electron's names are what LLMs know; kit gets one switch, as in ADR-215; `desktop` performing roles gives one behavior everywhere.
