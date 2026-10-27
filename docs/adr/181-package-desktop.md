# ADR-181: Package `desktop`

- **Date:** 2026-10-27
- **Status:** Accepted

## Decision

The public API of ADR-178's OS services on ADR-179's contract; doc comments hold the details.

- **Dialogs:** `OpenFile`, `OpenFiles`, `SaveFile`, `PickFolder(done, opts...) error` (`Title`, `Directory`, `DefaultName`, `Filter`, `Parent`). `done` runs once, later, on the UI thread; cancel is `""`/nil with a nil error; an error return means nothing was shown. Parent: the given window, else the one whose handler or effect runs, else the focused, else the main window.
- **Message box:** `ShowMessage(Message{Kind, Title, Text, Buttons, Parent}, done) error`, the platform's kinds and button sets. Native where it exists, else drawn by `desktop` in a modal child window with `h` and theme tokens (shadcn AlertDialog look, GNOME button order, Enter = default, Escape/close = `DismissAnswer`, height fitted to the content). `MessagePreview` renders it.
- **Notifications:** `Notify(Notification{…, OnClick, OnClose, Actions[{Label, OnClick}]}) (ShownNotification, error)`.
- **Tray:** struct component `Tray{Icon, Tooltip, Menu, OnClick, OnError}`, shown while mounted, updated only on change (icons by identity); `MenuItem` has per-item `OnSelect` and no ID (tests name items by label).
- **Paths and store:** `ConfigDir`…`LogDir` per OS, created 0700, named by `SetAppID`, else App.AppID, else the executable; `TELA_DESKTOP_DIR` moves them. `Store` (JSON, one per path, coalesced atomic writes, corrupt file kept), `Settings()`, `UseSetting[T](key, def) (T, tela.Setter[T])`: a real state setter mirrored to the store by an effect and back by a watcher.
- **Placement:** `LoadPlacement`, `SavePlacement`, `UseSavePlacement(key)` (on change, debounced 0.5 s, and on unmount), `SavedWindow{Key, Window}`, `PersistApp(app)`; `windows.json` in StateDir, restore token included.
- Missing services return errors wrapping `ErrUnsupported`.
- **Core** (`internal/core/desktop.go`): `RunningDesktop`, `DesktopOf`, `RunningAppID`, `Desktop.Window/Listen/Local/Warn` (`Host.HandleEvent` delivers app-scoped events to `Listen`), `WatchPlacement`, `OpenWindow` (a child window opened by code, closed with its parent's tree), `OnRunEnd` (Run flushes the stores).
- **telatest** (`desktop.go`): `TempDesktopDirs`, `Unsupported`, `AnswerFileDialog`, `AnswerMessage`, `Notifications`, `ClickNotification…`, `Tray`, `ClickTrayItem(path...)`.

## Rationale

Electron's and Tauri's services in React's model: a declarative tray, a setting as state, imperative dialogs answering on the UI thread. Opening a window by code lets an imperative call draw a box without a component in the tree.
