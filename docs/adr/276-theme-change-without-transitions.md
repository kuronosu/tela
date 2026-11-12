# ADR-276: A theme or scheme change applies without transitions

- **Date:** 2026-11-11
- **Status:** Accepted

## Decision

Phase 24, CPU performance. `tela.SetColorScheme`, `tela.SetTheme`, a change of the system's dark mode (or high contrast) that the app follows, and a text scale change start no transitions: the frame that applies them styles with `Tree.anim.instant` (set by `noteThemeChange` in `applyEnv` and `useParser`, cleared by `afterFrame` once a style pass saw it), so `mayTransition` is false and `updateTransitions` starts nothing. A transition already running whose target the switch changes is removed and the box takes the new target at once ("jumps to its end with the new tokens"); one whose target is unchanged keeps going. Later changes, a hover right after the switch included, transition as usual; `animate-*` keyframes, `UseSpring`/`Animate`, layout animations and `Presence` are not transitions and are untouched. Opt-out: `tela.App.ThemeTransitions` (`core.Config.ThemeTransitions`, `telatest.ThemeTransitions()`, read by `telatest.FromApp`) keeps the fade. A subtree's `h.Scheme` change does not count: it is an app-made prop change, like toggling a class, and the author controls it; no global switch happens, so no frames of the whole window are at stake.

## Why

Browsers fade such a switch, and so did Tela (ADR-076): `examples/dashboard`'s board cards (`transition` for their hover) made a theme switch about 28 frames of ~150 ms that rasterize the cards under the glass (p50 3.7 to 19 ms at x1). Web libraries solve this with next-themes' `disableTransitionOnChange`, which is the default users expect from a theme toggle. The option is a positive `App` field (zero value is the default, like `IgnoreTextScale`) named for what it keeps, in CSS vocabulary (transitions).

## What it changes

Default behavior of switches (a Changed line, no API break); ADR-076's "Transitions follow CSS" gains this exception (ARCHITECTURE section 12, Implicit transitions: "except on a theme or scheme change (ADR-276)").
