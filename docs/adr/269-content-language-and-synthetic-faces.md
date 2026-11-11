# ADR-269: Content language, CJK fallback, synthetic bold and oblique

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-031, ADR-087 and ADR-244; §10, §11.

## Decision

- **`h.Lang(tag)`** (owner): HTML's `lang`, BCP 47, inherited like `h.Dir`. Default: `tela.SetDefaultLang`, kept by `i18n` at its locale (`i18n.Provider` through `core.LangProvider`, no box), else the user's language (`text.SystemLanguage`, also `i18n`'s source; unknown in tests).
- Text shapes with it (locl) and its region orders CJK fallback.
- System fallback ranks by weight distance before region; a paragraph keeps one family per script (Han with kana).
- Chromium's synthetic bold (≥ 600 on a face below 600; Skia's stroke width, as an outline offset; advances unchanged) and oblique (skew −1/4). Not for color fonts; no `font-synthesis` utilities.

## Why

A Thin-only family drew Windows 11 CJK text beside another family. An offset outline avoids double edge coverage: ink within 2.2% of Chromium.

## What it changes

Single-face fonts draw bold and italic synthesized; `i18n` apps shape in their locale.
