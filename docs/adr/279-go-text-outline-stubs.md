# ADR-279: go-text sees a glyf of glyph headers, and no gvar (phase 24)

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-243 ("What remains").

## Decision

go-text's `font.NewFont` gets a glyf of glyph headers (each glyph's bounding box, zero contours, 12 bytes) and a matching loca instead of the font's own, unless the font has kerx; for a variable font only when it also has HVAR and GPOS lookups, and then without gvar. A variable font without them keeps both tables. A Hebrew run at a variable instance of a font whose GPOS has no `hebr` script is shaped with a second parse that has the font's own tables, made once, when such a run first needs it.

## Why

As Tela calls go-text (horizontal text, ppem never set, outlines loaded by Tela), go-text reads glyf and gvar only for glyph extents (HarfBuzz's fallback mark positioning, used only when GPOS is not applied), variable advances without HVAR, kerx control points and vertical origins. Extents at the default instance are the header's box, so they stay identical; at other instances they are needed only where the Hebrew shaper disables a GPOS without `hebr`. The eight files of `fonts/standard`: 42 → 17 MiB of heap, 54 → 13 ms to parse; Adwaita Sans 4.3 → 1.2 MiB, JetBrains Mono Nerd Font 8.4 → 2.3 MiB.

## What it changes

Shaping output, advances and default-instance extents are unchanged (`TestGoTextOutlineStubs`, `TestHebrewRunsShapeWithOutlines`). A glyf with a malformed glyph, which go-text dropped whole, now gives go-text the headers' extents. Code that asks go-text for TrueType outlines, contour points or non-default extents must parse the font without the stubs (`goTextLoader(src, goTextOutlines{})`, as `Typeface.goTextFullFont` does).
