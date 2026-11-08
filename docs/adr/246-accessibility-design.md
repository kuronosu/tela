# ADR-246: Accessibility design (phase 19)

- **Date:** 2026-11-08
- **Status:** Accepted

## Decision

Amends ADR-058; §2, §15, §16, §20.

- **Push model (AccessKit's).** Each OS bridge keeps an immutable copy of a window's tree and answers the assistive technology's (AT's) queries from it on any thread; core pushes updates; the AT's requests come back to the UI thread as events.
- **Platform contract v5.** A flat tree in `internal/platform` (`AccessNode`, `AccessUpdate`, `AccessTree`), `Window.SetAccessibility`, and the events `AccessibilityEvent{Active}` and `AccessActionEvent`. Backends without a bridge ignore updates; headless records them; remote (protocol 9) forwards both ways.
- **Core** sends nothing while no AT is active (idle stays at 0% CPU); while one is, it sends after each commit the changed nodes, the focus and live-region announcements.
- **Bridges.** AT-SPI2 over D-Bus with godbus for Wayland and X11; UI Automation providers as Go COM servers on Windows; `NSAccessibilityElement`s made at run time on macOS. Roles and states follow W3C Core-AAM 1.2 and HTML-AAM.
- **New props:** `h.AriaLive`, `AriaAtomic`, `AriaKeyShortcuts`, `AriaPosInSet`, `AriaSetSize`, and the deferred name `h.AriaLabelFn`.
- **Dependencies:** `uiautomationcore.dll` and `oleaut32.dll` on Windows (§2). References in §20.

## Why

The AT's queries arrive on D-Bus goroutines, COM threads and AppKit callbacks; answering from a snapshot never waits for a busy UI thread, and AccessKit shows the model works on all three. Announcements computed in core behave the same everywhere and are testable headless.

## What it changes

Screen readers read and operate Tela apps. Tests check what a bridge would receive without an AT.
