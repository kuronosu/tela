// Package text is Tela's text layer (ARCHITECTURE section 11). This file
// documents its font layer: typefaces, face instances, metrics, font
// matching and fallback, and color glyphs. Paragraph layout (shaping,
// bidi, line breaking) builds on it.
//
// # Typefaces and faces
//
// A Typeface is one parsed font file (ParseTypefaces), immutable and shared.
// A Face is a Typeface at fixed variation coordinates (Typeface.Face with an
// Instance: weight, stretch, optical size, italic/oblique axes, explicit
// font-variation-settings). Faces are interned per typeface by their
// normalized coordinates, so equal requests return the same *Face and the
// same gfx FontKey; they live as long as their typeface (the embedded
// typefaces live for the whole process).
//
// Face implements gfx.Font. Glyph geometry is in em space: x right, y down,
// 1 em = 1 unit, origin at the pen position on the baseline (font units
// divided by unitsPerEm, y negated). Outlines, bounds, advances and color
// display lists are computed on first use and cached per face; every
// method is safe for concurrent use (the raster workers call them from
// several goroutines), except ShapingFace, which returns the go-text face
// used by the shaper and must be used from one goroutine at a time.
//
// Metrics (Face.Metrics) are in em units: ascent, descent and line gap
// following HarfBuzz's rule (OS/2 typo metrics when USE_TYPO_METRICS is set,
// else hhea), cap height, x-height, underline and strikeout, with MVAR
// deltas for variable fonts.
//
// # Matching and fallback
//
// A Collection maps family names, generic families and scripts to
// typefaces. Collection.Face and Collection.Match implement the CSS Fonts 4
// matching algorithm (font-stretch, then font-style, then font-weight, with
// variable fonts covering their axis ranges). Collection.Itemize splits
// text into runs of grapheme clusters that resolve to the same face:
// requested families first, then the per-script fallback families, then
// every other family in registration order with the embedded ones last;
// emoji presentation (VS16, Emoji_Presentation, ZWJ sequences, skin tones)
// tries the emoji families first and VS15 avoids color fonts.
//
// NewDefaultCollection registers the embedded fonts (Inter, JetBrains Mono,
// Noto Sans Arabic, Noto Color Emoji; see internal/assets/fonts) and the
// generic families sans-serif, system-ui, ui-sans-serif (Inter), monospace,
// ui-monospace (JetBrains Mono) and emoji (Noto Color Emoji). There are no
// system fonts here (ADR-008; optional discovery comes later behind the same
// API) and no synthetic bold or oblique: a request no face covers gets the
// closest face as CSS matching defines it.
//
// Optical size follows CSS font-optical-sizing: auto: opsz is the font size
// in CSS pixels, rounded to whole pixels by the Collection so that a
// typeface has at most one instance per pixel size (Inter's opsz axis spans
// 14 to 32, so at most 19 instances per weight and style). Weights and
// stretches are not quantized beyond the 2.14 fixed-point normalized
// coordinates of OpenType.
//
// # Determinism
//
// Glyph outlines must be bit-identical on every architecture (ADR-027).
// go-text applies glyph variations (gvar) and item variation store deltas
// (HVAR, MVAR, COLR) with float32 code of the form x += float32(d) * s,
// which Go fuses into multiply-adds on arm64 and on amd64 with
// GOAMD64=v3: Inter at wght 650, opsz 20 and Noto Sans Arabic at wght 650,
// wdth 80 give different outlines on amd64 and arm64 through go-text. This
// package therefore loads TrueType outlines and evaluates variation deltas
// itself (glyf.go, vardelta.go, ported from go-text), in float64 with every
// product that feeds an addition rounded explicitly, and computes every
// other float result the same way. TestOutlinesDeterministic and
// TestColorGlyphsDeterministic hold digests recorded on amd64; run them
// with GOARCH=arm64 go test -short -exec qemu-aarch64-static and with
// GOAMD64=v3.
//
// Remaining exceptions: CFF2 (variable CFF) outlines still come from
// go-text and may differ in their last bits (CFF outlines only add, so they
// are exact; no embedded font uses CFF2); COLR rotations and skews by
// angles that are not multiples of 90 degrees use math.Sincos and math.Tan
// (no embedded font uses them). The shaper's advances come from go-text and
// are not rounded; Face.Advance rounds variable advances to whole font
// units like HarfBuzz, and layout should prefer it.
//
// # Color glyphs
//
// ColorGlyph converts COLR version 0 and 1 glyphs to display lists in em
// space, with palette 0 of CPAL (palette index 0xFFFF is the foreground
// color). Lists are built once per glyph and shared by all foregrounds,
// unless the glyph uses the foreground color (then up to 32 lists are kept
// per glyph). The COLR clip box clips the glyph and is its GlyphBounds;
// without one, the bounds are those of the list, with unbounded fills
// limited to the font's bounding box (head).
//
// Conversion of the paint graph:
//
//   - PaintColrLayers, COLRv0 layers: drawn in order.
//   - PaintSolid: a fill of the clip area (or a FillPath of the enclosing
//     glyph).
//   - PaintGlyph: a FillPath of the outline when the child is a solid or a
//     gradient under any transforms (the outline is mapped into the
//     gradient's space), else PushClipPath of the outline around the child.
//   - PaintLinearGradient: gfx.LinearGradient from p0 to p3, the projection
//     of p1 on the line through p0 perpendicular to p0p2.
//   - PaintRadialGradient: concentric gradients (any radii) map exactly to
//     gfx.RadialGradient. Two-point gradients (the start circle not
//     concentric with the end circle; Noto Color Emoji has 17, all focal
//     with the start circle inside the end one) have no gfx equivalent and
//     are emulated by filled circles drawn from the far end of the color
//     line to the near one, banded so that neighbors differ by at most
//     about two levels (rings instead of disks when the line is
//     translucent). Exact for nested circles up to the banding; cones whose
//     circles are not nested are approximated.
//   - PaintSweepGradient: gfx.ConicGradient. COLR angles are counter-
//     clockwise in font space with the OpenType 1.9.1 bias ((value + 1) *
//     180 degrees), a point's angle is taken in [0, 360) degrees, and the
//     gfx gradient starts at the +x axis and turns clockwise on screen, so
//     its stops are the color line sampled from 360 degrees down to 0.
//   - Extend modes: pad is native; repeat and reflect are unrolled into
//     explicit stops over the area to cover (up to 256 periods, padded
//     beyond). Stops are sorted; offsets outside [0, 1] are supported.
//     Colors interpolate premultiplied in sRGB (gfx.InterpSRGB), like
//     Skia's COLRv1 rendering.
//   - Transforms (PaintTransform, Translate, Scale*, Rotate*, Skew*, and the
//     *AroundCenter forms): PushTransform with the transform conjugated by
//     the font-to-em mapping.
//   - PaintColrGlyph: the referenced glyph's graph, clipped to its clip box;
//     cycles are cut.
//   - PaintComposite: Src, Dest, SrcOver, DestOver and Clear are drawing
//     orders. SrcIn and DestIn with a solid mask are exact (a layer with the
//     mask's alpha as opacity); with any other mask, the mask's alpha is
//     approximated by the union of the glyph outlines it paints, used as a
//     clip (exact where the mask is opaque inside its outlines). SrcAtop and
//     DestAtop combine both. SrcOut, DestOut and Xor are approximated by the
//     source, the backdrop and source-over. The blend modes (multiply,
//     screen, overlay, darken, lighten, color-dodge, color-burn, hard-light,
//     soft-light, difference, exclusion, hue, saturation, color,
//     luminosity) and Plus map to gfx blend layers (an isolated group with
//     the backdrop, the source in a layer with the blend mode), but package
//     raster does not implement blend modes before phase 6, so until
//     exactBlend is turned on they are approximated with source-over
//     (approxBlend): soft-light, overlay and hard-light sources become
//     white or black by their lightness, other modes draw the source as it
//     is.
//   - Variable paints (PaintVar*, variable color stops and clip boxes): the
//     deltas are evaluated at the face's coordinates with the fusion-safe
//     item variation store code.
//
// Noto Color Emoji (3993 color glyphs, all with clip boxes) uses
// PaintColrLayers, PaintGlyph, PaintSolid, linear and radial gradients
// (pad only; no sweep gradients), PaintTransform, PaintTranslate,
// PaintScale, PaintScaleAroundCenter and PaintComposite in two modes:
// SrcIn in 296 glyphs (source a gradient or layers, mask a layer stack of
// glyphs or a translucent solid) and SoftLight in the 262 flags (shading).
// TestCOLRSurvey reproduces these numbers. Against Chromium, the golden
// emoji differ by a mean of 0.2 to 1.6 levels per channel (anti-aliasing),
// and the flags by 1.6 to 3.7 because of the soft-light approximation
// (TestChromiumEmoji).
//
// Bitmap (CBDT, sbix) and SVG glyphs are not supported: ColorGlyph returns
// nil for them and they draw their outline, if any.
package text
