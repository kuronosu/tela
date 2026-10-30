# ADR-201: `kit.Plain` title bar and window controls

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

New variant `kit.Plain` for `kit.TitleBar` and `kit.WindowControls` (amends ADR-182), and a `Look kit.Variant` field on both. `Plain` keeps the platform's arrangement and glyphs (`Look` picks another one, empty is `runtime.GOOS`'s): the same heights, title at the start (Windows) or centered (GNOME, macOS), controls at the end in minimize, maximize, close order, or as three dots at the start in Aqua's order on macOS. It draws nothing of its own: the bar has no background, border or outline (windowed, maximized or inactive), so the app's surface shows (`Class: "bg-sidebar"` gives it one). The controls are ghost buttons that follow theme tokens: `text-muted-foreground`, `hover:bg-accent`, a destructive hover on close (small neutral dots on macOS, destructive on close). Still `h.WindowDrag` on the bar and `h.WindowControl` on the buttons; unfocused windows dim.

Recipes: `TitleBarStyle.Variants[Plain]` (applied after the look's) and `Inactive[Plain]`; `WindowControlsStyle` maps get `plain-windows`, `plain-gnome` and `plain-macos` keys.

## Why

In apps with a design of their own the platform look dominates and was overridden with `Class: "border-0 bg-sidebar"`. `Look` exists so goldens and the gallery are the same on every OS (as the named looks are).

## What it changes

`kit/titlebar.go`, `kit/windowcontrols.go`, their tests, the gallery's Title Bar page, `examples/notes` (uses `Plain`), the wiki and `docs/llms.txt`. Dashboard has no `TitleBar`.
