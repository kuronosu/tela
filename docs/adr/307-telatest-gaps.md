# ADR-307: telatest's gaps

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-086, ADR-221, ADR-237, ADR-259, ADR-291, ADR-298.

## Decision

- **Modifiers:** `telatest.Modifiers("Shift", "ControlOrMeta"...)` is an option of `Click`, `ClickAt`, `DoubleClick` and of the drags (it already served drag and drop), like Playwright's `click({modifiers})`: every pointer event carries them, no key events are sent (a lone Alt must not arm a menu bar). `ClickOption` and `DragOption` become interfaces.
- **Flavor:** the keys a menu role shows (Copy's `Ctrl+C` or `Super+C`) and a `GlobalShortcut`'s `CmdOrCtrl` follow the window's flavor (`PlatformKeys`), through `platform.ParseAcceleratorFor`; `ParseAccelerator` keeps the host's.
- **Tray, range:** `TrayIcon.Template`; `HasRange(now, min, max)` over aria-valuenow, -min and -max. `Stats.LaidOut` (boxes laid out) already existed: `Stats.Layouts` counts layout phases, empty ones too, and its doc says to assert "no relayout" on `LaidOut`.
- **Starts and defaults:** the option `StartLaunch(desktop.Launch{Args, Dir, AtLogin})` gives the app its own start per app (`core.Options.StartLaunch`, read by `UseLaunch` before the process's); `App.AskForFileDefaults` plays Windows (`MakeDefault` of a file type returns `DefaultAsked`) and `DefaultsAsked` lists the requests.
- **Waits:** between two failed tries a wait moves the clock to the app's next deadline when nothing else is due, not one frame, so a wait for a long timer takes a few tries.
- **`UseAsync`:** the tree records the runs in flight (`Tree.AsyncRuns`, in `Tree.Pending` too). `telatest` settles by waiting for them in real time; a run still going after the `Timeout` fails the test naming it, and `NoSettle` leaves runs alone (loading-state tests). `Render`, `AriaSnapshot` and `LayoutJSON` (`tela snapshot`, `inspect`) wait up to the `Timeout`, then draw the loading state with a dev warning naming the run (a CLI has no test to fail). The cocoa reference run mounts with `Mount`.
- **Blur on an empty area:** no change; it stopped reproducing at merge 2e8de0b4 (ADR-277), and `TestPressOnEmptyAreaBlurs` keeps it.

## Why

Each was a test an app could not write, or a capture that depended on timing.
