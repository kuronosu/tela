# ADR-202: The theme's CSS variables inside arbitrary values

- **Date:** 2026-10-30
- **Status:** Accepted (amends §10, "Not implemented": `var()` is no longer unsupported for the theme's own variables)

## Decision

An arbitrary value may use the variables Tailwind v4 exposes for the theme: `var(--color-<name>)` (a palette color or a semantic token, custom ones included), `var(--radius)` (the `lg` radius, shadcn/ui's `--radius`), `var(--radius-<size>)` and `var(--spacing)`, with the optional fallback `var(--x, fallback)` (used when the theme lacks the name). It works wherever an arbitrary value does: colors (`bg-[var(--color-primary)]/50`), gradients and their stops, shadows, lengths and `calc()`/`min()`/`max()`/`clamp()` (`w-[calc(var(--spacing)*4)]`), `[clip-path:...]`, and Tailwind's shorthand `bg-(--color-primary)` and `bg-(color:--color-primary)`. Any other variable (an app's `--foo`, `--tw-*`) stays an error naming the supported ones; an unknown name in a supported family is an `InvalidClassError` with the closest name as suggestion (`tela lint` reports it). Fonts and the other namespaces are not exposed.

## Why

LLMs write `var(--color-primary)` inside gradients, rings and conic meters; the utilities (`from-primary`) cover only some of them.

## What it changes

`internal/style/themevars.go`: the parser substitutes the variables in the text of the value before parsing (a length inside `calc()` is grouped, an internal `tela-rgba()` color carries theme colors exactly). A semantic token has a light and a dark value, so a class that uses one is parsed twice and its op picks by the element's scheme (`schemeOp`), as `bg-primary` resolves in `Compute`: the compiled class stays shared by every scheme and a `Scheme` override. Palette colors, radii and spacing resolve once. Verified against Chromium with Tailwind 4.3.3 (`internal/core` `TestChromiumThemeVars`). With shadcn's `@theme inline`, Tailwind's own `var(--color-primary)` stays at its light value inside `.dark`; Tela follows the scheme, as the owner asked.
