# ADR-304: The DevTools window's data and the scheme with no system preference

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-299, ADR-112.

## Decision

- **Tools socket, protocol 2.** `elements {diff: true, version}` answers with what changed since a version of the list (`core.Tree.ElementsSince`: the elements new or changed by value, the whole order as IDs when it changed; the last 3 lists kept per tree, an unknown version gets the whole list). `inspect {depth}` stops at that many levels of children, counting the rest in `more` (DevTools ask for the box alone). A client whose queue is full gets `dropped {events: {event: count}}` before any newer notification, and resyncs with the calls it has.
- **No color scheme preference.** Platform contract v17 adds `SystemSettings.LightMode`: with neither it nor `DarkMode` the system states no preference (the freedesktop portal's 0; Windows and macOS always state one). Remote protocol 18 carries it. Core keeps a process default for `System` with no preference, light unless `core.SetNoPreferenceScheme` sets it; it is not public API. `tela devtools` sets dark, Chrome's and Flutter's DevTools default, and its toolbar toggles the scheme.

## Why

Refetching a 10,000-element tree after every build and a big root's whole layout on selection made the DevTools slow on big apps; versions keep the child stateless per client. Disconnecting or coalescing a slow client loses less than a resync repairs. A light-only default made DevTools light on a desktop with no setting, which `DarkMode` alone cannot tell from a light preference.
