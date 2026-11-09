# ADR-263: Kit, animation and telatest follow-ups (phase 22)

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-248, ADR-250, ADR-256, ADR-136 and §10, §12 (play state).

## Decision

- **`kit.Input`:** the end padding follows the measured width of `EndContent` (`pe-[Npx]`, N = inset + width + 6 px gap, after the recipe's `pe-9`, which stays the floor; a frame after the content appears); `IconEnd` and `EndContent` combine (the content sits before the icon, `InputStyle.ActionAfterIcon`), no dev panic. An `IconXs` button keeps shadcn's 12 px icon; the 16 px of `InputGroupButton` is `IconClass: "size-4"` (no new API).
- **`kit.Alert.Action`:** shadcn's `pe-18` holds an action up to 60 px; a wider one makes the room its width plus 12 px (`end-3`) and 8 px.
- **Animations:** `paused`, `running` (tw-animate-css) and `[animation-play-state:…]` (`AnimProps.Paused`): a paused animation holds its time, asks for no frames and continues from it. A changed duration keeps the time the animation has run, so the progress changes, as in Chromium (CSS Animations: the current time is kept); a finished animation the new timing outlasts runs again.
- **`BackgroundGradientFn`** is a layer over the classes' images (bg-none for alone), replacing ADR-250/ADR-256's "topmost replaced" (Breaking).
- **Progress `ValueFn`** keeps its indicator a deferred layer: it is a few commands and a marker, a new value records only it (`TestProgressValueFnLayerCostsLittle`); dropping it needs a build, and giving `Value` instead of `ValueFn` does it.
- **`App.Root`:** only a `Preview` that is the root counts; the theme and scheme belong to the window and are set before anything builds.
- **telatest:** `Click` already fails on `pointer-events-none` (Playwright's "does not receive pointer events"); the message now names the cause. `ByClass` and `ByTestID` match a decorative `h.Icon` (`semantics.Node.Decorative`, hidden but with bounds).
- **Chromium references of Tela-only pieces:** the same structure with the recipe's classes (engine check, not design); the design reference is shadcn's nearest piece, named in the doc comment; pieces whose size Tela measures (Input fit, Alert room) have goldens and geometry tests, no Chromium cell.

## Why

The gaps found in phase 21; each follows shadcn and Chromium where they have an answer.
