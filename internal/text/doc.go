// Package text is Tela's text layer (ARCHITECTURE section 11): the font
// layer (typefaces, face instances, metrics, font matching and fallback,
// color glyphs) and, on top of it, paragraph layout (shaping, bidi, line
// breaking, alignment, truncation, painting); see "Paragraph layout".
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
// tries the emoji families first and VS15 avoids color fonts. Spaces
// resolve like any other character (a space between Arabic words takes the
// first requested family that has it, as in browsers); controls, line and
// paragraph separators and default ignorables keep the face before them.
//
// NewDefaultCollection registers the embedded fonts (Inter, JetBrains Mono,
// Noto Sans Arabic, Noto Color Emoji; see internal/assets/fonts) and the
// generic families sans-serif, system-ui, ui-sans-serif (Inter), monospace,
// ui-monospace (JetBrains Mono) and emoji (Noto Color Emoji). There are no
// system fonts here (ADR-008; see "System fonts") and no synthetic bold or
// oblique: a request no face covers gets the closest face as CSS matching
// defines it.
//
// # System fonts
//
// Collection.EnableSystemFonts opts in to the fonts installed on the machine
// (found by go-text's fontscan, which indexes the Linux, Windows and macOS
// font directories and caches the index on disk). Nothing enables it
// implicitly and the tests and snapshots never do (under "go test" the call
// is refused unless TELA_SYSTEM_FONTS=1 or the options name only explicit
// directories).
//
// System fonts serve named families that are not registered ("Segoe UI",
// "Noto Sans CJK JP") and, last of all in the fallback order, clusters that
// no requested, per-script, registered or embedded family covers (CJK,
// Hebrew, Devanagari, Thai...). Being last keeps default rendering of
// Latin, Arabic and emoji the same on every machine: the installed fonts
// only fill what the embedded ones cannot draw. A script is drawn by one
// family: the installed families that support it are ranked once per script
// (sans-serif designs before others, then the region of
// SystemFontOptions.Lang for CJK families, then the family that covers the
// fewest characters, then the name) and a cluster takes the first of them
// that covers it; clusters with no script of their own follow their run's.
//
// The scan and the parsing are lazy: the index loads at the first unknown
// family name or uncovered cluster, a font file is read and parsed when a
// face of it is first used (only the face CSS matching selects, from the
// index's family, style and weight), and a TrueType collection is read once.
// Installed fonts load through newTypeface, the code of ParseTypefaces, so
// they share the face, variation and determinism handling of the embedded
// ones: TrueType outlines through this package's port, CFF and CFF2 through
// go-text (ADR-030). Fonts without outlines (bitmap-only color fonts) are
// skipped. Output that uses them depends on the machine, by nature.
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
//     with the start circle inside the end one) map to gfx.FocalGradient
//     between two circles of the same family: those at the ends of the
//     color line (pad) or of the range of t over the area to cover
//     (repeat, reflect), moved to the circle of radius 0 where the line
//     reaches negative radii. Exact for every configuration, cones
//     included.
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
//     orders. SrcIn and DestIn are exact: with a solid mask, a layer with
//     the mask's alpha as opacity; with any other mask, a layer whose alpha
//     mask (gfx.Mask) is the mask paint recorded into a list of its own.
//     SrcAtop and DestAtop draw the backdrop (source) and then the source
//     (backdrop) masked by it, which differs from Porter-Duff atop where
//     both are translucent. SrcOut, DestOut and Xor are approximated by the
//     source, the backdrop and source-over. The blend modes (multiply,
//     screen, overlay, darken, lighten, color-dodge, color-burn, hard-light,
//     soft-light, difference, exclusion, hue, saturation, color,
//     luminosity) and Plus are exact gfx blend layers: an isolated group
//     with the backdrop, the source in a layer with the blend mode
//     (ADR-074).
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
// emoji and the flags differ by a mean of 0.2 to 1.7 levels per channel,
// mostly anti-aliasing (TestChromiumEmoji; the flags were 1.6 to 3.7
// before blend modes, masks and focal gradients were exact).
//
// Bitmap (CBDT, sbix) and SVG glyphs are not supported: ColorGlyph returns
// nil for them and they draw their outline, if any.
//
// # Paragraph layout
//
// NewLayout lays out a Paragraph (a UTF-8 string, styled spans by byte
// offsets, inline boxes and inlines) once; BreakLines breaks, aligns and
// positions it at any width without shaping again (Parley's model: shape
// once, break many times). Inline boxes are atomic placeholders (an
// inline-block, an icon) whose size the caller may change with SetBoxSize
// and SetBoxIntrinsic before breaking again; inlines are CSS inline boxes
// (spans, links) whose edges take space on the lines and whose fragments
// the caller decorates (inline.go, whose top comment is the reference).
// The pipeline:
//
//  1. Text preparation (prepare.go): text-transform with full case
//     mappings ("ß" to "SS", final sigma, Turkish dotted i, capitalize at
//     UAX #29 word starts), then CSS white-space processing: CR LF
//     normalized, collapsible spaces and tabs collapsed, removed around
//     newlines and at the start of the paragraph, newlines turned into
//     spaces (or removed between East Asian wide characters), tabs and
//     newlines preserved by pre, pre-wrap, pre-line and break-spaces.
//     Inline boxes become U+FFFC placeholders (bidi neutral, Common script).
//     Every processed rune keeps the source offset of the character that
//     produced it; removed characters belong to the rune before them, so
//     clusters map to contiguous, non-empty source byte ranges (boxes to an
//     empty range at their offset) and the source string is never copied
//     or rewritten.
//  2. Bidi (bidi.go): go-text's UAX #9 per bidi paragraph (split at
//     newlines), with a base direction from ParagraphStyle (auto: the first
//     strong character, P2 and P3). Rule L1 is applied per line, rule L2 on
//     the items of each line.
//  3. Scripts (script.go): UAX #24 with Common and Inherited characters
//     resolved from their neighbors and paired brackets.
//  4. Items and shaping (shape.go): runs of one bidi level, script and
//     shaping style (fonts, size, letter-spacing, features, language) are
//     split by Collection.Itemize and shaped with go-text's HarfBuzz at
//     units-per-em scale, so positions are whole font units, scaled to px.
//     Letter-spacing disables optional ligatures (liga, clig, dlig, hlig,
//     calt) as in Chromium; font-variant-numeric maps to OpenType features.
//  5. Clusters: HarfBuzz clusters merged until they end on grapheme and
//     source character boundaries, so ligatures, emoji sequences and
//     combining marks are single clusters that nothing splits. A cluster's
//     advance includes letter-spacing (after every cluster, none for
//     cursive scripts such as Arabic) and word-spacing (word separators).
//  6. Break opportunities (breaking.go): UAX #14 from go-text's segmenter,
//     forced breaks at preserved newlines, word-break (break-all adds
//     opportunities between letters, keep-all removes them), break-spaces
//     (after every space) and inline boxes (both sides, always).
//  7. Greedy breaking: content that fits stays; at the first cluster that
//     does not, the line ends at the last soft opportunity, else at the
//     last cluster boundary when overflow-wrap allows it, else the cluster
//     overflows. Trailing spaces hang (pre-wrap; only when they do not fit
//     before a forced break or the end) or are removed (collapsing modes),
//     and do not count for fitting or alignment. Tabs advance to multiples
//     of tab-size spaces of the block's font.
//  8. Lines (lines.go): items in visual order; line boxes per CSS 2.1
//     section 10.8: the strut (Default style), the span box of every style
//     on the line and, with line-height normal, the boxes of fallback fonts
//     (CSS Inline 3), plus inline boxes by vertical-align. Line boxes are
//     quantized in device pixels at the paragraph's display scale
//     (Paragraph.Scale, Layout.SetScale), as Blink does with the font
//     scaled by the device scale factor (ADR-159): ascent, descent and line
//     gap rounded to whole device pixels, the line height in LayoutUnits
//     (1/64 device px, the nearest), the leading halved in LayoutUnits
//     (truncated) and rounded down to whole device pixels; results are in
//     logical px (multiples of 1/scale). line-clamp keeps the
//     first lines; text-overflow and line-clamp truncate at the end edge,
//     hiding whole clusters in visual order, with an ellipsis in the
//     Default style.
//  9. Alignment (AlignTo): start, end, left, right, center and justify
//     (word separators, and the gaps around CJK characters as in Blink;
//     not the last line nor lines ending in a forced break). Overflowing
//     lines are start-aligned.
//
// Paint emits one gfx.DrawGlyphRun per run of one face, size and paint,
// and the decorations as rectangles (underline and overline below the
// glyphs, line-through above; positions and auto thicknesses from the
// style's first available font, so a decoration does not step between
// fallback fonts). PaintScaled and PaintLine snap underlines to the
// device pixels of a display scale as Blink does (a tenth of the font
// size, floored to whole device pixels, below the baseline by half of
// it). Inline boxes are not painted; BoxRect places them. Neither are
// inlines; InlineFragments reports their fragments.
//
// The read-only view (query.go) exposes lines, the runs of a line in
// visual order and clusters (source byte range, glyph range, x, advance,
// bidi level, grapheme count, flags), which is what cursor positioning,
// selection and hit-testing build on. Min- and max-content widths are
// computed by the same breaker at widths 0 and +Inf.
//
// Differences from Chromium, measured by TestChromiumLayout on 38
// paragraphs (Latin, Arabic, bidi, CJK, emoji, white-space modes,
// spacing, justification, inline boxes): line breaks, baselines and
// heights match, and cluster edges are within 0.06 px, except that line
// ends are not reshaped (Chromium drops the kerning between the last
// character of a line and what follows; up to 0.4 px), that UAX #14 allows
// breaks after "/" and "?" in URLs where Blink's ASCII pair table does not,
// and that capitalize uses the full case mapping of ligature characters.
// TestChromiumInline does the same on 22 paragraphs with inlines (edges,
// nesting, wrapping, empty inlines, bidi splits, right-to-left
// paragraphs, struts, truncation) and compares their fragments too:
// every fragment and cluster edge is within 0.06 px.
// TestChromiumBaselines compares one line of Inter in 13 sizes and 18 line
// heights at scales 1, 1.25, 1.5, 2 and 3: the baseline and the content
// area match to the device pixel and the line box within 1/64 device px,
// except 7 cases of 1170 where Blink's float32 line-height arithmetic
// truncates 4/3 of a 30 or 60 px device font below a whole pixel and its
// baseline is one device pixel higher.
// Not implemented: hyphenation, text-decoration-skip-ink, vertical-align on
// spans, synthetic bold and oblique, reshaping across emergency breaks
// (an Arabic word broken by overflow-wrap keeps its joining forms).
//
// Determinism: positions come from integers (font units) and px values
// computed with explicit float32 conversions; nominal advances come from
// Face.Advance (fusion-safe), not from go-text's float32 HVAR code. GPOS
// variation deltas are still go-text's float32 sums, rounded to whole font
// units; TestLayoutDeterministic checks a digest of a corpus of variable
// instances on every architecture.
//
// # Cursors, selection and hit-testing
//
// cursor.go, hittest.go, selection.go and navigation.go build editing
// geometry on the read-only view, after Parley's editing module and
// browser behavior. Everything is computed from the current line breaking
// (BreakLines, AlignTo) on demand; nothing is cached and nothing reshapes.
//
// A Cursor is a byte offset in the source text and an Affinity. Caret
// positions are the boundaries between clusters and, inside a ligature,
// between its grapheme clusters (the advance is split evenly). Offsets in
// between snap back to the start of their grapheme cluster, except in
// white space removed by collapsing, where they snap forward to the end of
// it (SnapCursor). One offset can have two visual positions: at a soft
// wrap (end of a line, start of the next), at a bidi level change (the
// trailing edge of the text before it, the leading edge of the text after
// it) and next to inline boxes, which have an empty source range (before
// or after them). Upstream picks the first of each pair, Downstream the
// second; the affinity is canonical (Downstream) wherever both look the
// same, so equal carets are equal cursors.
//
// Bidi caret rule: there is one caret per cursor, attached to the cluster
// its affinity designates (Parley's rule). Chromium places the caret of an
// offset at a level change by the visual order of its line fragments and
// may show it at the other end of a run from where the pointer was;
// HitTest always returns the cursor whose caret is where the pointer is.
// Callers that want a split caret draw both affinities.
//
// After a forced break, or after white space removed at a soft wrap,
// there is no caret on the line: the position belongs to the start of the
// next line, as in Chromium (no caret follows the collapsed space of a
// wrapped line, so End, a click past the line's end and arrow keys stop
// before that space; the same holds for collapsed white space at the end
// of the text, whose end is reached only by MoveDocumentEnd and
// SelectAll). Preserved white space that hangs (pre-wrap) is a
// sequence of caret positions past the line's width, also as in Chromium,
// but unlike Chromium the start of the next line remains reachable.
// Positions in text hidden by text-overflow or line-clamp show the caret
// at the end-side edge of the ellipsis, which behaves like an inline box
// that holds the hidden text; logical movement skips hidden text. Because
// inline boxes have no text, a box that starts or ends a wrapped line
// leaves one of its sides without a cursor, and a line that holds only
// boxes between two wraps has no caret position at all: navigation skips
// it and hit-testing returns the cursor before or after its boxes.
//
// HitTest maps a point to the nearest caret position; HitTestCluster
// tells which cluster, inline box or ellipsis is under a point.
// SelectionRects highlights a Selection (anchor and focus) with one
// rectangle per visual run per line, as tall as the line box, extending
// lines whose break the selection crosses by a space width (Chromium's
// painted selection). Move implements arrow keys (visual, Parley's model:
// each step crosses one cluster and attaches to it; and logical), word
// movement (UAX #29 words, skipping white space and punctuation like macOS
// and GTK; MoveNextWordStart for Windows), Home and End (logical ends of
// the visual line, Chromium's rule), vertical movement with a goal column,
// pages and the ends of the text; SelectWordAt, SelectLineAt,
// SelectParagraphAt and ExtendSelection cover double clicks, triple
// clicks and drags. TestChromiumCursor compares carets, hit-testing and
// selection rectangles with Chromium.
package text
