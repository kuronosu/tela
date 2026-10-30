# ADR-197: Platform looks in a nested `looks/` module

- **Date:** 2026-10-30
- **Status:** Accepted

## Decision

The platform looks of `examples/native` move to a nested module `github.com/kuronosu/tela/looks` (its own `go.mod`, like `icons/lucide`, ADR-102). Package `looks` defines `Look{Name, Theme tela.Theme, Style *kit.Style, Regions}`, where `Regions` holds the classes of the window regions kit has no component for (toolbar, sidebar, content, status bar, details pane), and `Detect()`, which picks the look of the running desktop (Windows → win11; Linux with KDE in `XDG_CURRENT_DESKTOP` → breeze, otherwise adwaita; macOS → aqua from phase 14, adwaita until then). One package per look (`looks/win11`, `looks/adwaita`, `looks/breeze`) exports `Look()`. A gallery in the module (`looks/gallery`, `package main`) shows kit's controls in each look and replaces the explorer, which goes with `examples/native`.

## Rationale

The looks are recipes, not an example: an app wants to import one. Keeping them out of `kit` keeps kit platform-neutral and lets the looks change at their own pace; a nested module keeps them out of the core module's API and compatibility promise. The explorer's data (columns, date formats, status text) is specific to a file manager and stays out.

## What it changes

CI gets a job for the module next to `icons/lucide`'s. The goldens of the three looks (light and dark) move to the module's tests. Phase 14 adds `looks/aqua` and checks the gallery on macOS. The core module cannot import `looks`, so no example uses it.
