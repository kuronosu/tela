# ADR-312: `tela check` and `tela verify`, and the rules of what is on screen

- **Date:** 2026-11-22
- **Status:** Accepted. Amends §17, ADR-306, ADR-174.

## Decision

Six rules join `telatest`'s audit (ADR-306), each a heuristic that reports what a designer would agree is a bug: `color-contrast` (WCAG 1.4.3: 4.5:1, 3:1 for large text, on the colors the frame composites: alpha, opacity, translucent backgrounds; not text over an image, gradient, blur or filter, nor disabled text, nor text faded to 60% or less, the inactive look), `click-events-have-key-events`, `text-clipped` (glyphs cut by `overflow-hidden` or by the window's edge; not truncation on purpose, scroll containers, `sr-only`, transforms), `text-overlap`, `target-overlap` (flow content only: overlays, portals and scrolled-out content are not judged) and `focus-visible` (Tab through the screen, at most 100 stops; an element whose surroundings do not change shows no indicator; text fields, whose caret is the indicator, are not judged). The first two are default; the other four run when named (`WithRules`, or `WithTags("layout")` for the first three). `core.Tree.AuditTexts`, `AuditFlows` and `Blur` give `telatest` the laid-out text, its clips and colors (not public).

`tela check <pkg>` is the fast pass: the default preview in light and dark, every rule. `tela verify <pkg>` runs every preview (or `--preview`) in light, dark, `--widths` (default 360, 768, 1280), text scale `--text-scale` (default 2, WCAG 1.4.4), `rtl` and one other language (`--lang`, else the first other locale of a package that links `tela/i18n`; a variant that does not apply is listed under `skipped`). The focus rule runs in light and dark only. Both take `--rules`, `--disable` (rule IDs) and `-json`; they mount through the build overlay as `tela snapshot` does. A finding is the violation of ADR-306 (rule, impact, message, fix, aria line, `file:line`, bounds) with the previews and variants it appears in, merged. The text output is `file:line: rule (impact): message`, then `element:`, `fix:` and `in:` lines; exit 0 with nothing found, 1 with findings, 2 on a usage error or a package that cannot run. `tela mcp` gets the tools `check` and `verify` (own process, the source as it is).

## Why

An agent that writes UI cannot see cut-off text or low contrast from the aria tree alone; the rules turn the layout and the frame into findings it can fix, by line. Two commands keep the everyday check fast and the matrix explicit.

## What it changes

`telatest.Rules()` lists the new rules; ADR-306's default `ExpectAccessible` now also checks contrast and click handlers. `cmd/tela` gains `check`, `verify` and two mcp tools; kit's audit disables `color-contrast` for the Avatar's fallback (4.34:1, a theme finding).
