# ADR-243: Fonts load their outlines on demand from the file bytes (phase 17)

- **Date:** 2026-11-07
- **Status:** Accepted

## Decision

`internal/text` no longer parses glyf and gvar up front (refines ADR-030). Its tables are sub-slices of the font file found through the sfnt directory (WOFF and dfont still go through go-text's copies), so `ParseTypefaces` keeps references into `data`. glyf, loca, gvar and hmtx stay raw: a glyph and its variation data are parsed with go-text's per-glyph parsers when first needed and kept by the typeface. A face's per-glyph caches are allocated in pages of 128 glyphs.

## Why

Tela parsed every glyph of every embedded font a second time after go-text's `NewFont` and kept table copies: 26 MB of the 61 MB the fonts took in every app. Outlines, advances and bounds stay bit-identical.

## What remains

go-text's `NewFont`, 35 MB: glyf (17 MB) and gvar (9.6 MB) Tela does not use, COLR 5.3 MB, layout 1.6 MB. Hiding glyf/gvar from go-text would save 27 MB but drops the glyph extents of HarfBuzz's fallback mark positioning; that, and loading faces lazily (31 MB are faces hello never draws), go to the Font system phase.
