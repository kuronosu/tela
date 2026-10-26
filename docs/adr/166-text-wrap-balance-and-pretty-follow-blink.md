# ADR-166: `text-wrap: balance` and `pretty` follow Blink

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

`text-balance` and `text-pretty` set a new inherited paragraph property, text-wrap-style (`text.ParagraphStyle.WrapStyle`); `text-wrap` and `text-nowrap` reset it, `whitespace-*` keeps it (v0.10.4; engine only, no API change). `internal/text/wrapstyle.go`, ported from Blink, then moves the greedy line breaks without reshaping. **Balance:** Blink's score line breaker (squared free space of every line, Minikin's line penalty, an orphan penalty) over at most 6 lines up to the first forced break or the line clamp; failing that, and without forced breaks, bisection of the available width in 1 px steps. Nothing when a line overflows or breaks inside a word. **Pretty:** the same search on the last 4 lines of each part when its last line is one short word. The line count, the full-width alignment and the intrinsic sizes stay those of greedy breaking. Computed in logical px: Chromium's breaks are the same at scales 1, 1.5 and 2.

## Rationale

Tailwind and shadcn/ui markup balances headings, quotes and tooltips; until now they wrapped greedily (ADR-137 noted it). Against Chromium 154 on 33 paragraphs (`TestChromiumWrapStyle`) every line break is equal and line edges are within 0.037 px. Paragraphs without the property pay one field check.
