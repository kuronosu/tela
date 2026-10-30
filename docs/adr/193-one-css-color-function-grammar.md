# ADR-193: One CSS color-function grammar

- **Date:** 2026-10-29
- **Status:** Accepted

## Decision

`gfx.ParseRGBFunc`, `ParseHSLFunc`, `ParseOKLCHFunc` and `ParseOklabFunc` parse the arguments of `rgb()`, `hsl()`, `oklch()` and `oklab()` (and `rgba()`/`hsla()`) with the CSS Color 4 grammar, and `gfx` owns the HSL conversion. They return the color or a `*gfx.ColorFuncError` (function, `ColorFuncProblem`, offending argument), so each caller words its own message. `internal/svg` calls the rgb and hsl parsers (`fill`, `stroke`, `stop-color`); `internal/style` calls all four (`bg-[...]`, `text-[...]`, gradients, shadows).

## Why

`svg` and `style` tokenized the same functions twice, and `style` split on any of `, space /`, so it took what CSS rejects and refused `none`.

## What it changes

`style` is strict: mixed separators, a bad `/` alpha and legacy `hsl()` without `%` are errors that name the fix, and `none` is accepted in the space syntax. `oklch()`/`oklab()` have only the space form. The rejected forms are listed in the `Parse*Func` doc comments and covered by tests; no class string in the repository used one.
