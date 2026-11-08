# ADR-246: Accessibility design (phase 19)

- **Date:** 2026-11-08
- **Status:** Accepted

## Decision

Amends ADR-058; §2, §15, §16, §20.

- **Push model (AccessKit's).** Each OS bridge keeps its own immutable copy of a window's tree and answers the assistive technology's (AT's) queries from it, on whatever thread they arrive; core pushes updates; the AT's requests come back to the UI thread as events. A query never waits for the UI thread.
- **Platform contract v5.** `internal/platform` defines a flat accessibility tree (it cannot import `semantics`, which sits above `gfx`): `AccessNode` (ID, ARIA role string, name, description, value, range, states, toggles, actions, bounds in window logical px, child IDs, text field data, live region, key shortcuts, level, position in set, table counts) and `AccessUpdate{Nodes, Root, Focus, Announcements}`, where `Nodes` holds the new and changed nodes and a node no longer reachable from `Root` is gone. `Window.SetAccessibility(AccessUpdate)` copies it. Two window events: `AccessibilityEvent{Active}` (an AT started or stopped looking) and `AccessActionEvent{Node, Action, …}` (default action, focus, set value, increment, decrement, expand, collapse, scroll into view, scroll, set text selection, show menu). Backends without a bridge ignore updates and never report `Active`; headless records them; remote (protocol 9) forwards both ways.
- **Core.** While a window's bridge is active, after each commit core builds the semantics tree and sends the nodes that differ from the last ones sent, the focus, and the announcements of live regions whose text changed (`aria-live`, the implicit live roles `alert`, `status`, `log`; `aria-atomic`). Nothing runs while no AT is active, so idle stays at 0% CPU. A full build per commit is the start; incremental `needsSemantics` marks are a CPU performance task.
- **Bridges.** Linux: AT-SPI2 over D-Bus with godbus (`internal/platform/internal/atspi`, shared by Wayland and X11): the a11y bus from `org.a11y.Bus`, embedded in the registry, interfaces Accessible, Application, Component, Action, Value, Text, EditableText, Selection, events per at-spi2-core's XML. Windows: UI Automation providers as Go COM servers (`comserver_windows.go`) answering `WM_GETOBJECT`, with the Invoke, Toggle, Value, RangeValue, ExpandCollapse, SelectionItem, Selection, ScrollItem and Text patterns and `UiaRaiseNotificationEvent` for announcements. macOS: Objective-C classes made at run time, one `NSAccessibilityElement` per node under the content view, and `NSAccessibilityAnnouncementRequestedNotification`.
- **Roles.** ARIA roles map to each platform's roles by W3C Core-AAM 1.2 and HTML-AAM, as Chromium and AccessKit do. `Input` of type password is protected (AT-SPI password text, UIA `IsPassword`, `AXSecureTextField`).
- **New public props** (React's `aria-*` names): `h.AriaLive`, `h.AriaAtomic`, `h.AriaKeyShortcuts`, `h.AriaPosInSet`, `h.AriaSetSize`, and the deferred name `h.AriaLabelFn(func() string)`, read when semantics are built, so a `UseValue` change rebuilds nothing.
- **Dependencies.** Windows adds `uiautomationcore.dll` (the provider API) and `oleaut32.dll` (VARIANT, BSTR, SAFEARRAY) to §2.
- **References** (§20): AccessKit (MIT/Apache) for the tree model and the three adapters, W3C Core-AAM 1.2 and HTML-AAM for role and state mappings, at-spi2-core's interface XML, Chromium's accessibility code for quirks.

## Why

The AT's queries arrive on D-Bus goroutines, COM threads and AppKit callbacks; answering from a snapshot avoids deadlocks with a busy UI thread, and AccessKit shows the model works for all three. Announcements computed in core behave the same on every platform and are testable headless.

## What it changes

Screen readers can read and operate Tela apps. Tests can check what a bridge would receive without an AT.
