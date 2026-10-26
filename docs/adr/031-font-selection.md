# ADR-031: Font selection

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Font selection: CSS Fonts 4 matching (stretch, style, weight; a variable font covers its axis range); fallback per grapheme cluster: requested families, then the per-script list, then user-registered families, then embedded fonts; emoji presentation (VS16, Emoji_Presentation, ZWJ and modifier sequences) prefers the emoji font, VS15 avoids color fonts (Unicode 17 emoji properties vendored). `opsz` follows the font size rounded to whole CSS px. CJK is not embedded: system fonts at runtime, a subset font for goldens

## Rationale

Browser behavior an LLM expects; per-cluster resolution keeps emoji sequences and combining marks in one face; whole-px optical size bounds the number of instances; a CJK font would add 16+ MB per weight
