# ADR-256: Kit and deferred-form follow-ups (phase 21)

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-248, ADR-250, ADR-136 and §10 (arbitrary properties).

## Decision

- **`kit.Input.EndContent`** (a component at the end of the field, shadcn's InputGroup button: over the input's end padding, `end-1.5`, pointer events on; `IconEnd` and it together panic in dev). A key goes on the component (`tela.WithKey`), the same with and without a wrapper: a `Key` in `Props` keys the inner input only.
- **`Separator.CaptionAlign`** (`CaptionStart`, `CaptionCenter`, `CaptionEnd`; a short line on the near side, logical). **`Alert.Action`** (top end corner, shadcn's AlertAction; the alert keeps `pe-18`). **`Field.ErrorContent`** (rich error: invalid, alert, describes the control).
- **Field around a group:** core's label check accepts a Label that an element names with `aria-labelledby` (a group, not a control), so a `Field` around a `RadioGroup` no longer warns.
- **Animation longhands:** `duration-*`, `ease-*` also set the animation's duration and timing (as `delay-*`); `animation-duration-*`, `repeat-*`, `direction-*`, `fill-mode-*`; arbitrary `[animation-duration:…]`, `-timing-function`, `-iteration-count`, `-direction`, `-fill-mode`. They replace the shorthand's values (`ComputedStyle.AnimProps`), also while it runs, without a restart.
- **`aria.ProgressBarOptions.ValueFn` and `MeterOptions.ValueFn`** (read when painted and when semantics are built, never in the build; the result has `PercentFn`, `ValueTextFn`); `kit.Progress` uses them.
- **`BackgroundGradientFn`** is the topmost image layer: it replaces the classes' topmost layer and keeps those below.
- **A `Preview` as `App.Root`** applies its Theme and ColorScheme in `Run` and `Main`, over the App's.

## Why

Gaps found writing shadcn-style code; the deferred forms keep one shape; Chromium sheets (alert, alert action, badge tones, button loading, input icon, end content, separator caption and align, avatar group, field slots and error, table sort) guard the phase 20 pieces.
