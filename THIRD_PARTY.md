# Third-party code

Code ported from other projects and vendored data files. Ported files keep
their original copyright notice. Add one section per source project.

<!--
## <Project>

- **Source:** <URL> (<version, tag or commit>)
- **License:** <SPDX identifier>
- **Used in:** <repo paths>
- **Notes:** <what was ported or vendored, and how it diverges>
-->

## Wayland protocol XML

- **Source:** https://gitlab.freedesktop.org/wayland/wayland (wayland 1.26.0, `protocol/wayland.xml`) and https://gitlab.freedesktop.org/wayland/wayland-protocols (wayland-protocols 1.49)
- **License:** MIT (X11-style; each file carries its own `<copyright>` block)
- **Used in:** `internal/platform/wayland/protocols/xml/` (vendored unchanged), and the Go bindings generated from them in `internal/platform/wayland/protocols/*.go`
- **Notes:** Files: `wayland.xml`, `xdg-shell.xml`, `viewporter.xml`, `fractional-scale-v1.xml`, `xdg-decoration-unstable-v1.xml`, `cursor-shape-v1.xml`, `tablet-v2.xml` (needed by cursor-shape) and `text-input-unstable-v3.xml`. Every generated Go file reproduces its protocol's copyright text in its header, as `wayland-scanner` does.

## go-wayland

- **Source:** https://github.com/rajveermalviya/go-wayland (design reference, shallow clone of `main` at the time of writing)
- **License:** BSD-2-Clause, Copyright 2021 go-wayland authors
- **Used in:** `internal/platform/wayland/wlgen/` and `internal/platform/wayland/wire/`
- **Notes:** Design reference only (XML-driven generator, one typed proxy per interface, request/event split). No source was copied: the generator and the wire runtime are written from scratch against `wayland.dtd` and libwayland's wire format and differ in API (typed event structs and handler setters, descriptor tables, zombie handling, explicit non-blocking `Flush`/`Read`/`Dispatch`). Listed here as a courtesy to the project.

## kurbo

- **Source:** https://github.com/linebender/kurbo (v0.13.1, commit 9366cda)
- **License:** Apache-2.0 OR MIT
- **Used in:** `internal/gfx/flatten.go`, `internal/gfx/arc.go`, `internal/gfx/stroke.go`, `internal/gfx/dash.go`
- **Notes:** Ported from `bezpath.rs` (`flatten`), `quadbez.rs` (`estimate_subdiv`, `determine_subdiv_t`, the parabola-integral approximations), `cubicbez.rs` (`to_quads`, `subsegment`) and `arc.rs` (`Arc::append_iter`, `sample_ellipse`). Diverges from kurbo in `Flatten`: it works on float64 internally and emits float32 points; it spends 90% of the tolerance as a safety margin; it splits cubics at inflections and sharp turns (near-cusps) before subdividing, because kurbo's own output exceeds the tolerance across strong inflections and at cusp tips; and it emits the turning point of collinear curves that fold back on themselves. Each ported file keeps the Kurbo Authors' copyright header.
- **Notes (stroking):** `Path.Stroke` follows `stroke.rs` (`stroke`, `StrokeCtx::do_join`, `finish`, `finish_closed`, `round_cap`, `square_cap`: forward and backward offset paths, miter point and limit test, caps) and the `PathSeg::tangents` rule of `bezpath.rs`; dashing follows `stroke.rs` (`dash`, `DashIterator`: offset normalization, per-subpath restart, joining the dash that runs through the start of a closed subpath with the first one). Diverges from kurbo: curves are flattened (with `Flatten`'s algorithm) and stroked as polylines with round joins between their pieces, instead of fitting cubics to the offset curve (`offset_cubic`, `fit_to_bezpath_opt`); the first or last piece of a curve at a butt/square cap or a miter/bevel join is skewed onto the true end normal, splitting the curve near that end when needed; inner joins go through the center point unless the inner miter point is provably covered by both adjacent pieces, so the NonZero winding never drops to zero inside the stroke; round joins and caps are emitted as polygons (by angle bisection) and the output contains only lines; the tolerance is respected everywhere (kurbo uses a fixed 1e-3 for joins and caps); zero-length subpaths and zero-length dashes draw SVG dots with round or square caps; arc lengths use adaptive Gauss-Legendre quadrature instead of kurbo's `arclen`/`inv_arclen`; dashes are emitted in path order (no stash reordering); a pattern that would produce more than 65536 dashes is ignored.

## color.js (reference data only)

- **Source:** https://github.com/color-js/color.js (v0.7.1)
- **License:** MIT
- **Used in:** `internal/gfx/color.go` and `internal/gfx/raster/gradient.go` (matrices), `internal/gfx/color_test.go` and `internal/gfx/raster/gradient_test.go` (expected values)
- **Notes:** The Oklab / XYZ / linear sRGB matrices are the ones published in CSS Color 4 and used by color.js (recalculated for a consistent D65 white). The gamut-mapping algorithm is the one of CSS Color 4 section 13, written from the spec. The test expectations were generated with color.js (`toGamut({method: "css"})` for colors; `a.mix(b, t, {space, premultiplied: true, outputSpace: "srgb"})` for gradient colors, run with node outside the repository); no color.js code was copied.

## Go standard library (math) and Cephes / FreeBSD msun

- **Source:** https://go.dev (`src/math/atan.go`, `atan2.go`, `log.go`, `exp.go`, `cbrt.go`, `sin.go`, `sincos.go`, `erf.go`, go1.27.2). `atan.go` and `sin.go` are based on the Cephes Mathematical Library by Stephen L. Moshier; `log.go` and `exp.go` on FreeBSD's `e_log.c` and `e_exp.c`, `erf.go` on FreeBSD's `s_erf.c`, and `cbrt.go` on fdlibm's `s_cbrt.c` (Sun Microsystems)
- **License:** BSD-3-Clause (Copyright The Go Authors); Cephes: "may be used freely but it comes with no support or guarantee"; Sun Microsystems: "Permission to use, copy, modify, and distribute this software is freely granted, provided that this notice is preserved"
- **Used in:** `internal/gfx/internal/fmath/` (`atan.go`, `explog.go`, `cbrt.go`, `sincos.go`, `erf.go`)
- **Notes:** Ported with every product that feeds an addition wrapped in an explicit `float64` conversion, so that the compiler cannot fuse it into a multiply-add and the result is the same on every architecture (Go fuses on arm64, ppc64le, s390x, riscv64 and amd64 with `GOAMD64=v3`; `math.Exp` and `math.Log` are also assembly on some architectures). Special cases are reduced to what Tela needs; `Sincos` reduces arguments of 2^29 and more with `math.Mod` instead of Payne-Hanek. `Asin01`, `Pow` (as `Exp(y*Log(x))`) are new. `Erf` and `Erfc` share their rational approximations through small helper functions (same coefficients and branches as Go's). `atan.go` moved here from `internal/gfx/raster`. Each file keeps the Go copyright header and the Cephes or Sun notice. The file keeps the Go copyright header and the Cephes notice.

## Inter (font)

- **Source:** https://github.com/rsms/inter (release v4.1, `Inter-4.1.zip`, sha256 `9883fdd4a49d4fb66bd8177ba6625ef9a64aa45899767dde3d36aa425756b11e`)
- **License:** OFL-1.1, Copyright 2016 The Inter Project Authors (`internal/assets/fonts/inter/OFL.txt`, the release's `LICENSE.txt`)
- **Used in:** `internal/assets/fonts/inter/InterVariable.ttf` (sha256 `4989b125924991b90d05b2d16e0e388c48f7d5bb8b30539bbf9c755278d0ccaf`) and `InterVariable-Italic.ttf` (sha256 `d6f1f6a172d9e588438db9f986fd5cfad7b30f644374080a8a9d4d91e344586f`)
- **Notes:** Vendored unchanged from the zip root; default sans-serif. Variable axes: `opsz`, `wght`.

## JetBrains Mono (font)

- **Source:** https://github.com/JetBrains/JetBrainsMono (release v2.304, `JetBrainsMono-2.304.zip`, sha256 `6f6376c6ed2960ea8a963cd7387ec9d76e3f629125bc33d1fdcd7eb7012f7bbf`)
- **License:** OFL-1.1, Copyright 2020 The JetBrains Mono Project Authors (`internal/assets/fonts/jetbrainsmono/OFL.txt`, the release's `OFL.txt`)
- **Used in:** `internal/assets/fonts/jetbrainsmono/JetBrainsMono-wght.ttf` (sha256 `662a196d58f1183bf2d77428b6d5283fe3f45161ab021bea4036bc98e5cac016`) and `JetBrainsMono-Italic-wght.ttf` (sha256 `f115aaa12113718c02ce72864fe6823b87241bc23d3e44cf1220155f861063f2`)
- **Notes:** Vendored unchanged from `fonts/variable/`; renamed from `JetBrainsMono[wght].ttf` and `JetBrainsMono-Italic[wght].ttf` to avoid brackets in `go:embed` paths. Default monospace. Variable axis: `wght`.

## Noto Sans Arabic (font)

- **Source:** https://github.com/google/fonts, `ofl/notosansarabic/` at commit `8b0a1d0f5983c89bc2b93f1b5fb55f9e252744b5` (font version 2.012; upstream https://github.com/notofonts/arabic)
- **License:** OFL-1.1, Copyright 2022 The Noto Project Authors (`internal/assets/fonts/notosansarabic/OFL.txt`)
- **Used in:** `internal/assets/fonts/notosansarabic/NotoSansArabic-wdth-wght.ttf` (sha256 `63111b5b2e074dd48cc67692e0a2726d86ee94c1c37fe8598257b7b4e87e869e`)
- **Notes:** Vendored unchanged; renamed from `NotoSansArabic[wdth,wght].ttf` to avoid brackets and a comma in `go:embed` paths. Arabic script fallback. Variable axes: `wdth`, `wght`.

## Noto Color Emoji (font)

- **Source:** https://github.com/googlefonts/noto-emoji (tag v2.051, commit `8998f5dd683424a73e2314a8c1f1e359c19e8742`, `fonts/Noto-COLRv1.ttf`)
- **License:** OFL-1.1 (`internal/assets/fonts/notocoloremoji/OFL.txt`, the repository's `LICENSE`), Copyright 2022 Google Inc.
- **Used in:** `internal/assets/fonts/notocoloremoji/Noto-COLRv1.ttf` (sha256 `0ae57fe58645638523ba35f388d93739d292539a9acb84df5700c81b1e1a28d2`)
- **Notes:** Vendored unchanged. This is the COLRv1 (vector) build; the CBDT bitmap build is not used. The `fonts/` directory no longer exists on the repository's main branch, so the tag is pinned. Newer tags (such as v2026-09-24-unicode18_0) publish only the 3D fonts as release assets.

## Noto Sans CJK JP (font, test-only subset)

- **Source:** https://github.com/notofonts/noto-cjk (release Sans2.004, `06_NotoSansCJKjp.zip` sha256 `bf5b26feb83ee02533d4eb4a127bd90ed9d42bdd7ca7cb6e4030b2f21ef55cc5`, file `NotoSansCJKjp-Regular.otf` sha256 `68a3fc98800b2a27b371f2fb79991daf3633bd89309d4ffaa6946fd587f375b5`)
- **License:** OFL-1.1, Copyright 2014-2021 Adobe (`internal/text/testdata/fonts/OFL.txt`, the release's `LICENSE`)
- **Used in:** `internal/text/testdata/fonts/NotoSansJP-Subset.otf` (sha256 `cc7759815817d9e0c4b875680bfaa3ee73373bfaa4bffc4661e5f65c6d9747c9`). Tests only; never embedded in a non-test package.
- **Notes:** Subset made with fontTools `pyftsubset` (a development tool, not a dependency) to ASCII, CJK Symbols and Punctuation, Hiragana, Katakana, Halfwidth and Fullwidth Forms and the characters of `cjk_sample.txt`; OpenType layout features and the `name` table kept, hinting dropped. The exact command is in `internal/text/testdata/fonts/README.md`. The full Pan-CJK JP file is the source instead of `SubsetOTF/JP/` because the latter lacks Simplified-only characters.

## go-text/typesetting (ported code)

- **Source:** https://github.com/go-text/typesetting (ported from v0.3.5; `go.mod` has since moved to commit `e49e2bd`, whose versions of these functions only add input validation, such as rejecting gvar point numbers out of range, which the port handles by skipping them, and small refactors; the ports were not updated)
- **License:** Unlicense OR BSD-3-Clause, Copyright 2021 The go-text authors (used under BSD-3-Clause)
- **Used in:** `internal/text/glyf.go`, `internal/text/vardelta.go`
- **Notes:** go-text is also an approved dependency; these files port parts of it so that variable outlines and metrics are bit-identical across architectures (ADR-027). Ported from `font/glyphs.go` (`getPointsForGlyphRec`, `transformPoints`: TrueType simple and composite glyphs with phantom points), `font/variations.go` (`parseGlyphVariationSerializedData`, `parsePointNumbers`, `unpackDeltas`, `calculateScalar`, `applyDeltasToPoints`, `inferDelta`: gvar deltas with IUP, itself adapted from HarfBuzz's `hb-ot-var-gvar-table.hh`), `font/renderer.go` (`buildSegments`: contours to quadratic paths) and `font/opentype/tables/xvar_src.go` (`ItemVarStore.GetDelta`, `RegionAxisCoordinates.evaluate`). Diverges from go-text: arithmetic is float64 with every product that feeds an addition rounded explicitly (go-text uses float32 that Go fuses into multiply-adds on arm64); coordinate differences are taken in `int` (go-text subtracts `int16` values, which can overflow); invalid variation regions are ignored as in HarfBuzz; contours are emitted directly as em-space `gfx.Path`s and start at their first on-curve point. Each ported file keeps the go-text copyright notice.

## Unicode emoji data

- **Source:** https://www.unicode.org/Public/17.0.0/ucd/emoji/emoji-data.txt (Unicode 17.0.0)
- **License:** Unicode License v3 (https://www.unicode.org/license.txt)
- **Used in:** `internal/text/emoji_tables.go`
- **Notes:** The `Emoji_Presentation`, `Emoji` and `Emoji_Modifier_Base` properties, converted to `unicode.RangeTable`s (adjacent ranges merged). The generated file names its source; regenerating it means re-running the same conversion on a newer `emoji-data.txt`.

## Parley (design reference)

- **Source:** https://github.com/linebender/parley (main branch, commit `46fcad3`, October 2026)
- **License:** Apache-2.0 OR MIT
- **Used in:** `internal/text/layout.go`, `prepare.go`, `shape.go`, `breaking.go`, `lines.go`, `paint.go`, `query.go`, `cursor.go`, `hittest.go`, `selection.go`, `navigation.go`
- **Notes:** Design reference only; no source was copied. Tela's paragraph layout follows Parley's model: shape a paragraph once, then break lines many times from per-cluster data; greedy line breaking with regular and emergency opportunities and hanging white space; alignment with start-aligned overflowing lines and justification of word separators; line boxes with ascent and descent rounded to whole pixels and the half-leading rounded down (Parley's `quantize`). Tela differs in its input model (byte-offset spans, white-space processing with a source map), in using go-text instead of HarfRust and ICU data, in its truncation (text-overflow and line-clamp) and in Blink's CJK justification opportunities. Listed here as a courtesy to the project.
- **Notes (editing):** Cursors, hit-testing, selection and navigation follow Parley's `editing/cursor.rs` and `editing/selection.rs` (commit `46fcad3`): a cursor is a byte index plus an affinity that attaches it to the cluster before or after it; a caret is at the trailing edge of the upstream cluster or the leading edge of the downstream one; hit-testing picks the nearer edge of the cluster under the point with the affinity that shows the caret there; visual movement crosses one cluster per step and attaches to it; selections keep an anchor, a focus, a goal column for vertical movement and the unit (word, line) of double and triple clicks for drags. Tela's code is written from scratch on its own layout data and differs: ligatures split their advance by grapheme cluster inside one cluster; offsets snap out of collapsed white space; affinities are canonical; positions after forced breaks and collapsed spaces at soft wraps belong to the next line (Chromium's rule); hidden text (text-overflow, line-clamp) maps to the ellipsis; lines without caret positions are skipped; selection rectangles extend crossed line breaks by a space for soft wraps too (Chromium's painting); word movement uses UAX #29 words and skips punctuation.
