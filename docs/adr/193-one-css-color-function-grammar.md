# ADR-193: One CSS color-function grammar

- **Date:** 2026-10-29
- **Status:** Accepted

## Decision

Codebase review 2026-10-29, C17. `gfx.ParseRGBFunc` and `gfx.ParseHSLFunc` parse the arguments of `rgb()`, `rgba()`, `hsl()` and `hsla()` with the grammar of CSS Color 4, and `gfx` owns the HSL conversion. They return the color or a `*gfx.ColorFuncError` (the function, a `ColorFuncProblem` and the offending argument), so each caller words its own message. `internal/svg` (`fill`, `stroke`, `stop-color`) and `internal/style` (`bg-[...]`, `text-[...]`, gradients, shadows) both call them; `svg` keeps its named colors and `currentColor`, `style` keeps `oklch()`/`oklab()` and its Tailwind-specific messages.

The grammar: either commas (`r, g, b` and `r, g, b, a`; three numbers or three percentages in `rgb()`, percentages for saturation and lightness in `hsl()`, no `none`) or spaces with an optional `/ alpha` (numbers, percentages or `none`; `hsl()` also takes plain numbers of percent). Hues take `deg`, `grad`, `rad` and `turn`. Channels, saturation, lightness and alpha are clamped.

## Why

`svg` and `style` tokenized the same functions twice, and `style`'s tokenizer split on any of `, space /`, so it took what CSS rejects and refused `none`.

## What it changes

`style` is now strict. Rejected, with a message that names the fix: mixed separators (`rgb(1, 2 3)`, `rgb(1 2 3, 0.5)`, `rgb(1,2,3 / 0.5)`), commas with a `/`, an empty argument (`rgb(1,,3)`), legacy `hsl()` without `%` (`hsl(120,100,25)`; `hsl(120 100 25)` still works), and numbers mixed with percentages in legacy `rgb()`. Accepted now: `none` in the space syntax (`rgb(none 0 0 / none)`). HSL results are the same at 8 bit, except for one level where a channel is an exact tie (178.5): the float64 arithmetic of CSS Color 4 now decides. No class string in the repository used a rejected form. `oklch()`/`oklab()` keep their lax splitting.
