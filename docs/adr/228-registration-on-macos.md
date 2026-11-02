# ADR-228: Registration of schemes and file types on macOS

- **Date:** 2026-11-02
- **Status:** Accepted. Amends ADR-209 and ADR-224.

## Decision

- **The app is its bundle.** `HandlerApp.Exec`, else the running executable, must be in `<Name>.app/Contents/MacOS/`, or every call returns `ErrUnsupported` naming `tela build` and `tela run`. The app ID must be the bundle's `CFBundleIdentifier`; a mismatch is an error naming both.
- **Info.plist, then Launch Services.** Marked entries (`TelaRegistered`): `CFBundleURLTypes` for a scheme; `CFBundleDocumentTypes` (Editor, `LSHandlerRank` Alternate) for the extension's UTI, with a `UTImportedTypeDeclarations` entry `<ID>.<ext>` when no app declares it. Other keys are kept; the file is written only on a change, never in a signed bundle; then `LSRegisterURL`.
- **Ranks.** Alternate never wins a default against an Owner or Default handler; the only candidate is the handler by the OS's rule. Nothing is taken, so `Replace` does nothing.
- **MakeDefault:** `LSSetDefaultHandlerForURLScheme`, `LSSetDefaultRoleHandlerForContentType`. Deprecated, but synchronous, any goroutine, no running app, no prompt; NSWorkspace's are asynchronous and may ask the user.
- **RegisterApp** registers the bundle, **UnregisterApp** writes nothing, **AppStatus** asks `LSCopyApplicationURLsForBundleIdentifier`.

## Why

Launch Services routes links and files only through what a bundle declares.
