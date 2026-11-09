# ADR-249: Transparent windows

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-179, ADR-187 and ADR-246 (protocol 10).

## Decision

- **API:** `tela.App.Transparent` and `tela.Window.Transparent`, fixed at open, require `Frameless` (Run panics without it). The root keeps no background (ADR-187's transparent canvas), so unpainted pixels stay {0,0,0,0} through the persistent framebuffer and damage; the app pads the root so its card's `shadow-*` fits. `WindowState.Transparent` says whether the OS shows the alpha.
- **Contract v6:** `WindowOptions.Transparent`, `WindowState.Transparent` (false and opaque where the OS cannot: X11 without a compositing manager, followed live), `platform.InputMask`. The OS draws no frame, shadow or corners; no resize edges.
- **Click-through:** pixels under alpha 64 (`InputAlpha`, above Tailwind shadows) pass the pointer below. Each backend computes the mask from the frames it presents, rescanning only the damage: Wayland input region, X11 SHAPE input, Win32 layered window (`WS_EX_TRANSPARENT` toggled with a cursor poll), Cocoa `ignoresMouseEvents` the same way.
- **Tests:** headless fakes the mask (`NoCompositing`), telatest's `Transparent()` goldens keep alpha; `tela snapshot` too.

## Why

The presented alpha is what the user sees, so input matches it; a threshold keeps shadows click-through and translucent surfaces clickable. Layered windows are the per-pixel path Windows passes input through.
