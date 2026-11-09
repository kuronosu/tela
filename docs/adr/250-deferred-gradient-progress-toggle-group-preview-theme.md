# ADR-250: Deferred gradient, Progress and ToggleGroup; a preview's theme

- **Date:** 2026-11-08
- **Status:** Accepted. Amends ADR-082, ADR-092, ADR-136 and ADR-246.

## Decision

- **`tela.BackgroundGradientFn(func() Gradient)`:** a paint-phase prop like `BgFn`. At style time it leaves one image layer without an image (the size, position, repeat and origin classes apply); paint puts the gradient in, stops copied. The recording compares it by content (`paintIn.grad`), so a change records only the list that paints the box (ADR-204).
- **`h.AriaValueNowFn`, `h.AriaValueTextFn`:** like `AriaLabelFn`, called when the semantics tree is built; a Value they read marks it stale, no frame. NaN is no value; static and Fn props replace each other.
- **`kit.Progress.ValueFn`:** read in a `TranslateFn` of the bar's own width and in those two props: a tween or a download moves the bar and its value without a build.
- **`kit.ToggleGroup`:** the deferred form is the value. `ValueFn` and `SelectedFn` are read in the group's build, so a choice in a `tela.Value` rebuilds the group (its items' `aria-checked` and look are build-time), not its holder. `Indicator` is drawn in every item that is on; a `LayoutID` slides it. Recipe part `ItemWithIndicator`.
- **`tela.Preview.Theme`, `ColorScheme`:** applied by telatest after the options, so they beat the App's and `--dark` in `snapshot`, `inspect`, `mcp` (which say so); `tela dev` sets them on the App.

## Why

Same `…Fn` shape; a selection cannot be painted, so ToggleGroup defers the read (Compose's lambda); Preview keeps the CLI and a test's `Mount` the same.
