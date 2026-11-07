# ADR-235: Windows notifications as WinRT toasts

- **Date:** 2026-11-07
- **Status:** Accepted. Amends ADR-179 (Win32 mapping) and ADR-217.

## Decision

- **Toasts first.** On Windows 10 1809 (build 17763) and later, `Notify` shows a WinRT toast through combase (ADR-230). Actions become buttons (5 at most) and `CloseNotification` withdraws the toast. There is no notification-area icon. The balloons stay as the fallback: before 1809, without WinRT (Windows 7 and 8, Wine), or when a toast fails to show.
- **AUMID registration, per user and lazy.** The first toast registers `HKCU\Software\Classes\AppUserModelId\<AUMID>` with `DisplayName` (the executable's name), `IconUri` (a cached PNG of the app's window or tray icon) and the marker `X-Tela-App`. It writes only what changed. A key without the marker is used as it is, and nothing is ever deleted. No Start menu shortcut is needed. `SetAppID` also sets the process's explicit AUMID, which groups the app's taskbar buttons under it.
- **Events.** Agile COM delegates (`IAgileObject`) run on WinRT's threads and post to the UI thread. Clicking the body is `NotificationActivated`, a button is `NotificationActionInvoked`, and a dismissal by the user or a failure is `NotificationClosed`. A timeout reports nothing, because the toast moves to the Action Center and can still be clicked while the app runs.
- **Activation scope.** There is no COM activator (`CustomActivator`). After the app exits, its toasts stay in the Action Center, and clicking one only dismisses it.

## Why

Toasts are the only Windows notifications that have buttons. The registry AUMID is what the Windows App SDK uses for unpackaged apps. Agile delegates avoid marshalling to the STA.
