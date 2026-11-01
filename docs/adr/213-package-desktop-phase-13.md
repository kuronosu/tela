# ADR-213: Package `desktop` for phase 13

- **Date:** 2026-11-01
- **Status:** Accepted

## Decision

The public API of ADR-209 on ADR-210's contract (amends ADR-181; details in doc comments).

- **Clipboard:** `ReadClipboard(done, types...)` (the well-known types offered, or the named ones; HTML names accepted), `WriteClipboard(*tela.DataTransfer)`, `ClipboardTypes`, `ReadClipboardText`, `WriteClipboardText`.
- **Shortcuts:** struct component `GlobalShortcut{ID, Keys, Description, OnPress, OnRelease, OnStatus}`; `ShortcutStatusEvent{ID, Status, Trigger, Err}`; `ParseKeys` (on `platform.ParseAccelerator`; bad Keys panic in dev mode, else `ShortcutFailed`). The latest mounted owner of an ID wins.
- **Launches:** `SingleInstance(app) tela.App` claims in a `core.OnRunStart` hook, after the backend exists and before the first window; a second process forwards and `Run` returns nil. Its root raises the main window (shown, `ActivateWithToken`). `UseLaunch(f)` gets the process's start once (the hooks mounted with the first one; none in telatest or a tela dev reload) and every forwarded `Launch{Args, Dir, Forwarded}`; `URLs()`, `Files()` (absolute, `file:` URIs as paths).
- **Registration:** `URLScheme`, `FileType`, `Register`, `Unregister`, `MakeDefault`, `RegistrationStatus`, options `Replace`, `ForApp(HandlerApp)`; the running app's backend, else `assoc.Native()`. Under tela dev, `Exec` must be given (`ErrDevExecutable`).
- **Secrets:** `SetSecret`, `Secret`, `DeleteSecret` (any goroutine, dev warning on the UI thread); `ErrSecretNotFound`, `ErrSecretCanceled` (new `platform.ErrSecretCanceled`).
- **Menus:** `PopupMenu(w, at, items, done)`, component `MenuBar{Menus, OnError}`; `MenuItem.Accelerator`; both report `ErrUnsupported` so kit can draw.
- **telatest:** `PressShortcut`, `ReleaseShortcut`, `SetShortcutStatus`, `GlobalShortcuts`, `InjectLaunch`, `StoredSecret`, `SetStoredSecret`, `FailSecrets`, `Clipboard`, `SetClipboard`, `PopupMenus`, `ChoosePopupItem`, `DismissPopupMenu`, `MenuBar`, `ChooseMenuItem`, `Registrations`, `HoldAssociation`, `SetDefaultHandler`; new `Unsupported` services.

## Rationale

React's model as in ADR-181: what lives while mounted is a component, the rest answers on the UI thread; blocking calls stay off it.
