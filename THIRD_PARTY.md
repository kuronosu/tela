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

## resvg / usvg

- **Source:** https://github.com/linebender/resvg (`crates/usvg/src/parser`, commit 5192641, design reference)
- **License:** Apache-2.0 OR MIT, Copyright 2018 the Resvg Authors
- **Used in:** `internal/svg/`
- **Notes:** Design reference only, for the normalization rules of static SVG (`use` and `symbol` instancing and recursion checks, the attributes and stops gradients take from their `href` chains, units, invalid geometry and references). No source was copied: the parser, the cascade and the converters are written from scratch against SVG 2 and CSS, and follow Chromium (the test oracle) where the two differ. The test SVGs in `internal/svg/testdata/chromium/cells` are written for Tela, not taken from resvg's test suite. Listed here as a courtesy to the project.

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
- **Notes:** The Oklab / XYZ / linear sRGB matrices are the ones published in CSS Color 4 and used by color.js (recalculated for a consistent D65 white). Colors are not gamut-mapped (ADR-046). The test expectations were generated with color.js (unclipped conversions for colors; `a.mix(b, t, {space, premultiplied: true, outputSpace: "srgb"})` for gradient colors, run with node outside the repository); no color.js code was copied.

## Go standard library (math) and Cephes / FreeBSD msun

- **Source:** https://go.dev (`src/math/atan.go`, `atan2.go`, `log.go`, `exp.go`, `cbrt.go`, `sin.go`, `sincos.go`, `erf.go`, go1.27.2). `atan.go` and `sin.go` are based on the Cephes Mathematical Library by Stephen L. Moshier; `log.go` and `exp.go` on FreeBSD's `e_log.c` and `e_exp.c`, `erf.go` on FreeBSD's `s_erf.c`, and `cbrt.go` on fdlibm's `s_cbrt.c` (Sun Microsystems)
- **License:** BSD-3-Clause (Copyright The Go Authors); Cephes: "may be used freely but it comes with no support or guarantee"; Sun Microsystems: "Permission to use, copy, modify, and distribute this software is freely granted, provided that this notice is preserved"
- **Used in:** `internal/fmath/` (`atan.go`, `explog.go`, `cbrt.go`, `sincos.go`, `erf.go`)
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

## Tailwind CSS (theme and Preflight)

- **Source:** https://github.com/tailwindlabs/tailwindcss, npm package `tailwindcss@4.3.3` (`theme.css`, `preflight.css`)
- **License:** MIT, Copyright (c) Tailwind Labs, Inc.
- **Used in:** `internal/style/gen/tailwind/` (vendored unchanged, with `LICENSE`), and the Go theme generated from it in `internal/style`
- **Notes:** The generator reads the theme's CSS custom properties (colors, spacing, radii, shadows, type scale...) and converts OKLCH colors to sRGB. Preflight is the reference for the primitives' default styles (ADR-038). Utility class names follow Tailwind's documentation; no Tailwind code is ported.

## Taffy (layout algorithms and test fixtures)

- **Source:** https://github.com/DioxusLabs/taffy (tag v0.14.0, commit 77f3856)
- **License:** MIT, Copyright (c) 2018 Visly Inc. and Taffy contributors
- **Used in:** `internal/layout/testdata/taffy/` (fixtures `block`, `flex`, `grid`, `blockflex`, `blockgrid`, `gridflex`, `leaf`, and `gentest/test_base_style.css`, `gentest/test_helper.js`, vendored unchanged, with `LICENSE`)
- **Notes:** The fixtures are HTML pages derived from Web Platform Tests. Expected layouts are measured from them with headless Chromium (ADR-039, ADR-073). `test_base_style.css` embeds the Ahem test font (public domain, https://github.com/Kozea/Ahem). Two grid fixtures set a style from a `<script>`; the measuring page inserts fixtures with `innerHTML`, which does not run scripts, so both Chromium and the fixture parser see them without it.
- **Ported code:** `internal/layout/{cache,compute,leaf,align,block,flex,absolute,round}.go` are ports of Taffy's `src/tree/cache.rs`, `src/tree/layout.rs`, `src/compute/mod.rs` (dispatch and `round_layout`), `src/compute/leaf.rs`, `src/compute/common/{alignment,scrollable_overflow,sizing_keyword}.rs`, `src/compute/block.rs` and `src/compute/flexbox.rs` (absolute layout from both), with Taffy's MIT notice and source paths in each file. Left out: margin collapsing, floats, `flex-wrap: balance`, `box-sizing: content-box`. `calc()` of a length and a percentage (`layout.UnitCalc`) is Tela's own. Deviations to match Chromium (ADR-043) are listed in `internal/layout/doc.go`; two fixes over Taffy: leaves are measured with content-box known sizes in size queries, and cached full layouts measured during baseline queries are dropped.
- **Ported code (grid, ADR-073):** `internal/layout/{grid,grid_tracks,grid_placement,grid_item,grid_sizing,grid_align}.go` port `src/compute/grid/` (`mod.rs`, `explicit_grid.rs`, `implicit_grid.rs`, `placement.rs`, `track_sizing.rs`, `alignment.rs`, `types/{coordinates,grid_track,grid_track_counts,grid_item,cell_occupancy,named}.rs`), with Taffy's MIT notice in each file. Left out: subgrid, masonry, layout containment, detailed grid information, `box-sizing: content-box`. Deviations to match Chromium (fit-content sizing of auto-width grids, growth limits of tracks with zero-size items, distribution beyond limits, automatic minimum sizes, a row estimate for aspect-ratio items, named lines per CSS Grid 1 section 8.3, Tela's containing blocks for absolutely positioned children, the container baseline) are listed in `internal/layout/doc.go`.

## Chromium / Blink (reference behavior)

- **Source:** https://chromium.googlesource.com/chromium/src (`third_party/blink/renderer/core/paint/`, behavior of Chromium 152)
- **License:** BSD-3-Clause, Copyright The Chromium Authors
- **Used in:** `internal/core/decorate.go`, `internal/core/paint.go`, `internal/gfx/raster/backdrop.go`
- **Notes:** No code is copied. The dash-gap fitting of dashed and dotted borders re-expresses the formula of Blink's `SelectBestDashGap`; pixel snapping, the 1px shadow clip inset, background bleed avoidance and backdrop-filter edge mirroring reproduce Chromium's observable output (ADR-045).


## winit (design reference)

- **Source:** https://github.com/rust-windowing/winit (v0.30.13)
- **License:** Apache-2.0
- **Used in:** `internal/platform/key.go`, `internal/platform/keycode.go`, `internal/platform/internal/evdev/`, `internal/platform/internal/keyboard/`, `internal/platform/x11/xi2.go`, `internal/platform/x11/keyboard.go`
- **Notes:** Design reference only; no source was copied. The keyboard model follows winit's `KeyEvent` (physical key, logical key and text as separate fields; a `PhysicalKey` that keeps the native scancode of unidentified keys; logical key and text ignoring Ctrl) and its synthetic releases on focus loss. The X11 backend's XInput2 handling follows winit's `platform_impl/linux/x11/event_processor.rs` (scroll valuator deltas divided by the increment, dropping emulated wheel buttons, resetting scroll baselines on enter) and its use of XKB detectable auto-repeat. The evdev table in `internal/platform/internal/evdev` covers the keys of winit's `scancode_to_physicalkey` (`platform_impl/linux/common/xkb/keymap.rs`) and adds more, checked against `linux/input-event-codes.h` and the USB HID usages of the kernel's `hid-input.c`. Differences from winit are listed in `internal/platform/doc.go` ("Keyboard") and `key.go`. Listed here as a courtesy to the project.

## W3C UI Events key and code values (identifiers)

- **Source:** https://www.w3.org/TR/uievents-code/ and https://www.w3.org/TR/uievents-key/
- **License:** W3C Software and Document License
- **Used in:** `internal/platform/keycode.go` (`KeyCode` names), `internal/platform/key.go` (named `Key` values)
- **Notes:** Only the value names are used, as identifiers and `String()` results, so that they match the DOM's `KeyboardEvent.code` and `KeyboardEvent.key`. No specification text is reproduced; the comments are Tela's own.

## libxkbcommon (keysym tables; reference behavior)

- **Source:** https://github.com/xkbcommon/libxkbcommon (1.13.2, as installed by the distribution: `/usr/include/xkbcommon/xkbcommon-keysyms.h`)
- **License:** MIT (libxkbcommon); the keysym header carries the X11 notices of The Open Group (1987, 1994, 1998) and Digital Equipment Corporation (1987), reproduced in the generated file
- **Used in:** `internal/platform/internal/xkb/keysyms_table.go` (generated by `gen_keysyms.go`), `internal/platform/internal/xkb/` (behavior), `internal/platform/internal/xkb/testdata/oracle/oracle.c` (expected results)
- **Notes:** The keysym names, their canonical names and the Unicode mapping of the legacy keysyms are generated from the header's machine-readable `#define` lines and comments (the mapping matches libxkbcommon's `xkb_keysym_to_utf32`, which comes from Markus Kuhn's public-domain `keysym2ucs.c`, except for two angle brackets that libxkbcommon maps to their canonical equivalents; the test checks every keysym against the library). No libxkbcommon code was copied: the keymap compiler and state lookup are written from scratch following libxkbcommon's documented and observed behavior (automatic key types, interpretation matching order, virtual modifier mapping, Caps Lock transformation, Compose file conflict rules), with libxkbcommon 1.13.2 as the test oracle through the C program in `testdata/oracle` (a development tool, never built by Go). Parts of libxkbcommon's source were read to match its behavior (`FindAutomaticType` in `src/xkbcomp/symbols.c`, `xkb_keysym_from_name` in `src/keysym.c`).

## xkeyboard-config (test keymaps)

- **Source:** https://gitlab.freedesktop.org/xkeyboard-config/xkeyboard-config (2.48), compiled with libxkbcommon 1.13.2 (`xkbcli compile-keymap --output-format 1 --layout … [--variant …] [--options …]`)
- **License:** xkeyboard-config's COPYING (MIT/X11-style permissive notices of its many contributors)
- **Used in:** `internal/platform/internal/xkb/testdata/keymaps/*.xkb.gz` (tests only)
- **Notes:** Resolved keymaps, gzip-compressed, as a Wayland compositor sends them; the exact commands are in `internal/platform/internal/xkb/testdata/README`.

## libX11 Compose file (test fixture subset)

- **Source:** https://gitlab.freedesktop.org/xorg/lib/libx11 (1.8.13, `nls/en_US.UTF-8/Compose`)
- **License:** MIT AND X11
- **Used in:** `internal/platform/internal/xkb/testdata/compose/locale/en_US.UTF-8/Compose` (tests only)
- **Notes:** About 35 lines of the file, plus lines written for the parser tests; `compose.dir` and `locale.alias` next to it are short hand-written files in the format of libX11's.

## XGB (vendored X11 client)

- **Source:** https://github.com/jezek/xgb (v1.3.1), a fork of BurntSushi's xgb, itself a fork of Google's X Go Binding
- **License:** BSD-3-Clause, Copyright (c) 2009 The XGB Authors (`internal/platform/x11/internal/xgb/LICENSE`, with `AUTHORS`, `CONTRIBUTORS` and `README` from the module)
- **Used in:** `internal/platform/x11/internal/xgb/` (the core package: `auth.go`, `conn.go`, `cookie.go`, `doc.go`, `help.go`, `sync.go`, `xgb.go`), `internal/platform/x11/internal/xgb/xproto/xproto.go` and `internal/platform/x11/internal/xgb/bigreq/bigreq.go` (ADR-025)
- **Notes:** Copied unchanged except for: a copyright header on each Go file (the originals carry the license only in `LICENSE`); the import paths of `xproto` and `bigreq`, rewritten to the vendored location; and Tela's patch to the reader in `xgb.go`, marked "Tela patch": (1) a GenericEvent (code 35) is read whole, the 32-byte header plus the extra 4-byte units its length field announces, and delivered as `RawEvent` (v1.3.1 read every event as 32 bytes, so an XInput2 or Present event desynchronized the connection); (2) an event without a registered constructor (an extension without a Go package, such as XKB) is delivered as `RawEvent` instead of being logged and dropped; (3) an error without a registered constructor (XInput2's BadDevice, XKB errors) becomes a `RawError` and is matched to its request's cookie (v1.3.1 skipped it, so the request's `Reply` blocked forever). `RawEvent`, `RawError` and `GenericEventCode` are in the new file `raw.go`, tested by `raw_test.go`. The unreachable `panic` at the end of `readAuthority` (`auth.go`) is removed so that `go vet` passes. The other xgb extension packages, `xgbgen`, the examples and xgb's own tests are not vendored. `doc.go` is kept as upstream wrote it (not gofmt-formatted).

## X.Org server (reference behavior)

- **Source:** https://gitlab.freedesktop.org/xorg/xserver (`xkb/xkbUtils.c`: `XkbGetCoreMap`; `xkb/XKBMisc.c`: `XkbKeyTypesForCoreSymbols`; `dix/getevents.c` and `Xi/` for XInput2 smooth-scrolling behavior) and the X protocol headers (`XI2proto.h`, `XKBproto.h` from xorgproto, `xinput.xml`, `xkb.xml` from xcb-proto)
- **License:** MIT/X11 (X.Org and contributors' notices)
- **Used in:** `internal/platform/internal/xkb/core.go` and `core_test.go`, `internal/platform/x11/xi2.go`, `internal/platform/x11/xkbext.go`
- **Notes:** No code is copied. `NewCoreKeymap` reads core keysym lists in the order `XkbGetCoreMap` writes them and applies XKB's single-keysym rule; `core_test.go` re-expresses `XkbGetCoreMap`'s layout to export test keymaps as a server does. The XInput2 and XKB request and event layouts are written from the protocol headers.

## Playwright (ported code; reference behavior)

- **Source:** https://github.com/microsoft/playwright (v1.61.1: `packages/injected/src/ariaSnapshot.ts`, `packages/injected/src/roleUtils.ts`, `packages/isomorphic/yaml.ts`)
- **License:** Apache-2.0, Copyright (c) Microsoft Corporation
- **Used in:** `internal/semantics/snapshot.go` (ported, with the notice), `internal/semantics/roles.go` and `internal/core/semantics.go` (reference behavior)
- **Notes:** `snapshot.go` ports the aria snapshot's tree normalization and YAML rendering (`generateAriaTree`'s text handling, `normalizeStringChildren`, `normalizeGenericRoles`, `renderAriaTree`'s keys, `yamlEscapeKeyIfNeeded`, `yamlEscapeValueIfNeeded`, `yamlStringNeedsQuotes`, `normalizeWhiteSpace`) to Go, with JavaScript's white space, `JSON.stringify` and `Number()` semantics written out. `roles.go` lists the ARIA role vocabulary and role groups as `roleUtils.ts` does; `core/semantics.go` follows its role, name (`getTextAlternativeInternal`) and state rules without copying code. The default mode only: no refs, AI mode, regex templates or snapshot matching. Test expectations in `internal/core/semantics_test.go` and `internal/semantics/snapshot_test.go` were compared with Playwright 1.61.1 running Chromium 149. `telatest` follows Playwright's locator and actionability behavior without copying code: `getByText` (`elementMatchesText`: the deepest element whose normalized text matches, case-insensitive substring or exact), `getByRole` (hidden and implicit generic elements excluded), strict mode, `locator.click`'s checks (visible, enabled, receives events), `keyboard.press` syntax and the US layout key definitions, `toMatchAriaSnapshot` and the auto-wait of `expect`.

## Ebitengine and winit (Win32 backend)

- **Source:** https://github.com/hajimehoshi/ebiten (no-cgo Windows code) and https://github.com/rust-windowing/winit (Windows backend)
- **License:** Apache-2.0
- **Used in:** `internal/platform/win32/`
- **Notes:** Design references only (syscall-based window class, message loop and DPI setup; mouse capture, leave tracking and DPI quirks). No source was copied; the backend is written against the Win32 documentation. Listed as a courtesy. The keyboard (`scancode.go`, `vkey.go`, `keyboard.go`, `layout.go`) follows winit 0.30's Windows keyboard (`keyboard.rs`, `keyboard_layout.rs`): the scan code to W3C code table and the virtual-key to named-key table are the same facts (and the same W3C and Firefox sources) written out again in Go, and the techniques (character messages complete the key, a peek at the queue, the fake left Ctrl before AltGr, `ToUnicodeEx` on the layout) are winit's. No Rust code was translated.

## Motion (springs)

- **Source:** https://github.com/motiondivision/motion (formerly framer-motion; `packages/motion-dom/src/animation/generators/spring/`: `index.ts`, `find.ts`)
- **License:** MIT, Copyright (c) 2018 Framer B.V.
- **Used in:** `internal/anim/spring.go`
- **Notes:** The spring model (stiffness, damping, mass, initial velocity), its closed-form solution for under-damped, critically damped and over-damped springs, the rest thresholds (`restSpeed` and `restDelta` defaults for small and large distances) and the `duration`/`bounce` parametrization (`findSpring`: Newton iteration on the envelope, damping ratio clamped to 0.05-1, duration to 0.01-10 s) are ported to Go. Tela works in seconds, also returns the velocity, uses `internal/fmath` and explicit rounding of products (ADR-041), and finds the settle time by sampling instead of Motion's `calcGeneratorDuration`. The file keeps Motion's copyright notice. The explicit animations (`internal/anim/motion.go`, `internal/core/valueanim.go`, `internal/core/presence.go`, ADR-080) follow Motion's behavior without copying code: the spring defaults (stiffness 100, damping 10; 800 ms and bounce 0.3 for a spring defined by its duration, from `springDefaults` in `generators/spring.ts`), physics options winning over duration/bounce, velocity by a 5 ms finite difference for tweens (`utils/velocity.ts`), retargeting from the current position and velocity, and `AnimatePresence`'s merge of exiting children at their previous indices.

## WebKit UnitBezier / Chromium CubicBezier (reference behavior)

- **Source:** https://github.com/WebKit/WebKit (`Source/WebCore/platform/graphics/UnitBezier.h`) and https://chromium.googlesource.com/chromium/src (`ui/gfx/geometry/cubic_bezier.cc`)
- **License:** BSD-2-Clause (Apple) and BSD-3-Clause (Chromium Authors)
- **Used in:** `internal/anim/curve.go`
- **Notes:** No code is copied. The solver (Newton's method on x(t) with bisection as the fallback, then y(t)) is the one both engines use for CSS `cubic-bezier()`, written from the CSS Easing Functions Level 1 formulas with Tela's explicit rounding. `steps()` follows the same specification.

## Unicode CLDR (vendored data)

- **Source:** https://github.com/unicode-org/cldr-json (tag 48.0.0, packages `cldr-dates-full` `ca-gregorian.json` and `cldr-misc-full` `listPatterns.json`, per locale, and `cldr-core` `supplemental/weekData.json`)
- **License:** Unicode License v3, Copyright (c) 1991-2025 Unicode, Inc. (`i18n/internal/gen/cldr/LICENSE`, the repository's `LICENSE`)
- **Used in:** `i18n/internal/gen/cldr/<locale>.json` (113 locales, trimmed to the parts the generator reads: month and weekday names in their format and stand-alone forms, AM/PM markers, era names, date, time and date-time patterns, the yMMMM available format (a month and a year, ADR-134), standard list patterns, under their original JSON paths), `i18n/internal/gen/cldr/supplemental/weekData.json` (only `supplemental.weekData.firstDay`, ADR-131) and the table generated from them, `i18n/dates_gen.go`
- **Notes:** The files are CLDR data reduced to a subset and merged (one file per locale holds both source files' subtrees); `go run ./internal/gen -fetch` from `i18n` rebuilds them from the tag above, `go generate ./i18n` regenerates the table. The generated file keeps a copyright and license notice in its header.

## golang.org/x/text (dependency)

- **Source:** https://go.googlesource.com/text (v0.43.0)
- **License:** BSD-3-Clause, Copyright 2009 The Go Authors
- **Used in:** `i18n` (`language`, `feature/plural`, `number`, `message`), a module dependency, not vendored code (ARCHITECTURE §2). Its CLDR 32 tables give the plural rules and number formats; date patterns do not come from it (it keeps them private).

## Floating UI (ported code; test oracle)

- **Source:** https://github.com/floating-ui/floating-ui (`@floating-ui/core` 1.7.5 and `@floating-ui/utils` 0.2.11)
- **License:** MIT, Copyright (c) 2021-present Floating UI contributors
- **Used in:** `aria/floating_position.go` (`computePosition`, `computeCoordsFromPlacement`, `detectOverflow` and the placement helpers of `@floating-ui/utils`), `aria/floating_middleware.go` (`offset`, `flip`, `shift` with `limitShift`, `size`, `arrow`, `hide`, `autoPlacement`); test data in `aria/testdata/floatingui/`
- **Notes:** Ported function by function from the published `@floating-ui/core` sources, keeping their names and structure. It diverges where Tela's engine does the work: it is synchronous and works on the rectangles of a frame (`core.FloatInput`) instead of a platform object, always with the fixed strategy and float32 window coordinates; `detectOverflow` takes the clipping rectangle of the floating element and of the reference from the engine; the `size` middleware does not measure the element again (it records the limits that its `apply` function asks for, and the engine lays out again and calls the position function again, ADR-099); the `arrow` middleware uses the floating element's border box for its client size; the `inline` middleware, `boundary`, `rootBoundary`, `elementContext` options, per-side padding and derivable options are not ported. `aria/testdata/floatingui/gen.mjs` runs the real `@floating-ui/core` over seeded random rectangles, placements and middleware stacks and writes `cases.json`, which `aria/floating_oracle_test.go` compares with the port; the package is not vendored (`npm install` in that directory fetches it).

  ```text
  MIT License

  Copyright (c) 2021-present Floating UI contributors

  Permission is hereby granted, free of charge, to any person obtaining a copy
  of this software and associated documentation files (the "Software"), to deal
  in the Software without restriction, including without limitation the rights
  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
  copies of the Software, and to permit persons to whom the Software is
  furnished to do so, subject to the following conditions:

  The above copyright notice and this permission notice shall be included in all
  copies or substantial portions of the Software.

  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
  SOFTWARE.
  ```

## React Aria (ported behavior)

- **Source:** https://github.com/adobe/react-spectrum (`@react-aria/interactions`: `usePress`, `useHover`; `@react-aria/focus`: `useFocusRing`, `FocusScope`, `useFocusManager`; `@react-aria/button`: `useButton`, `useToggleButton`; `@react-aria/checkbox`, `@react-aria/switch`, `@react-aria/toggle` and `@react-stately/toggle`: `useCheckbox`, `useSwitch`, `useToggle`, `useToggleState`; `@react-aria/overlays`: `useOverlay`, `useModalOverlay`, `usePopover`, `useOverlayTrigger`, `ariaHideOutside`; `@react-aria/dialog`: `useDialog`; `@react-aria/menu`: `useMenuTrigger`; `@react-aria/tooltip`: `useTooltipTrigger`, `useTooltip`; `@react-stately/overlays`, `@react-stately/menu` and `@react-stately/tooltip`: `useOverlayTriggerState`, `useMenuTriggerState`, `useTooltipTriggerState`; `@react-aria/selection`: `useSelectableCollection`, `useSelectableList`, `useSelectableItem`, `useTypeSelect`, `ListKeyboardDelegate`; `@react-stately/selection` and `@react-stately/list`: `SelectionManager`, `Selection`, `useListState`; `@react-aria/listbox`: `useListBox`, `useOption`, `useListBoxSection`; `@react-aria/menu`: `useMenu`, `useMenuItem`, `useMenuSection`; `@react-stately/tree`: `useTreeState`; `@react-aria/tabs` and `@react-stately/tabs`: `useTabList`, `useTab`, `useTabPanel`, `useTabListState`; `@react-aria/select` and `@react-stately/select`: `useSelect`, `useSelectState`; `@react-aria/combobox` and `@react-stately/combobox`: `useComboBox`, `useComboBoxState`; `@react-aria/i18n`: `useFilter`; `@react-aria/calendar` and `@react-stately/calendar`: `useCalendar`, `useRangeCalendar`, `useCalendarGrid`, `useCalendarCell`, `useCalendarState`, `useRangeCalendarState`; `@react-aria/radio` and `@react-stately/radio`: `useRadioGroup`, `useRadio`, `useRadioGroupState`; `@react-aria/button`, `@react-aria/toolbar` and `@react-stately/toggle`: `useToggleButtonGroup`, `useToggleButtonGroupItem`, `useToolbar`, `useToggleGroupState`; `@react-aria/slider` and `@react-stately/slider`: `useSlider`, `useSliderThumb`, `useSliderState`; `@react-aria/progress` and `@react-aria/meter`: `useProgressBar`, `useMeter`; `@react-aria/color` and `@react-stately/color`: `useColorArea`, `useColorSlider`, `useColorField`, `useColorFieldState`, `useColorPickerState`)
- **License:** Apache-2.0, Copyright 2020 Adobe. Licensed under the Apache License, Version 2.0 (http://www.apache.org/licenses/LICENSE-2.0)
- **Used in:** `aria/press.go`, `aria/hover.go`, `aria/focus.go`, `aria/focusscope.go`, `aria/button.go`, `aria/checkbox.go`, `aria/overlay.go`, `aria/dialog.go`, `aria/popover.go`, `aria/menutrigger.go`, `aria/tooltip.go`, `aria/collection.go`, `aria/selection.go`, `aria/listbox.go`, `aria/menu.go`, `aria/menubar.go`, `aria/contextmenu.go`, `aria/tabs.go`, `aria/color.go`, `aria/colorstate.go`, `aria/colorarea.go`, `aria/colorslider.go`, `aria/colorfield.go`, `aria/calendar.go` (each file names the hooks it follows in its header)
- **Notes:** The rules and the event sequences are ported, not the source: React Aria works on DOM events and Tela's handlers receive its own (pointer events with the engine's capture and `Bounds` instead of document listeners and `getBoundingClientRect`; key and click events; `ElementRef` instead of refs to DOM nodes). It diverges where the owner or the engine decided: options structs and `Props` (merged with `core.MergeProps`) instead of prop getters; the state of toggles and overlays is integrated, controlled when its pointer (`Selected`, `Open`) is non-nil (no react-stately layer); Enter presses on the key down and Space on the key up, where React Aria completes both on the key up; a virtual press is a click with no preceding pointer release; link handling, text-selection control or `useFocusVisible` global listeners; the focus manager takes `FocusOptions` and has no `accept` filter; no indeterminate state; overlays are one hook for modal and non-modal (no underlay props; the engine's overlay stack and `inert` do `ariaHideOutside`), the tooltip is Escape-only on the overlay stack and has no separate `useTooltip`, and presses outside a popover's trigger are told apart by the trigger's bounds. The collections (list box, menu, tabs) keep one `collState` per hook call and one `ElementRef` per key instead of a collection object and a hook per item: selection is the immutable `aria.Selection`, with React Aria's "all"; the item press is a pointer down that records the pointer type plus a click (selection on release, as React Aria's `onPress`), and Enter and Space are handled by the collection's key handler on the focused key; typeahead is case-insensitive prefix matching without a collator, its timer on the frame clock; page keys count items by the stride of those that are rendered; menus do not have submenus, `closeOnSelect` per item or press-drag-release selection (`allowsDifferentPressOrigin`). The select and the combobox compose the menu trigger, the popover and the collection engine; the select has no hidden native select and chooses an item on the click (no `shouldSelectOnPressUp`); the combobox's filters fold case and diacritics (NFD without combining marks, with `golang.org/x/text/unicode/norm`) instead of using a collator, its effect is the `sync` method run after every build, and its popover is not dismissed by a press outside (the blur commits). The radio group and the toggle group (`aria/rover.go`) keep one `rover` per hook call and one `ElementRef` per key, check or toggle on the click (no hidden input, no form value), and move the focus with the group's own arrow handler; the slider has no hidden range input, keeps its state in the hook and repaints through a Value while dragging (`aria/slider.go`), with react-stately's `snapValueToStep` and `roundToStepPrecision`, neighbor limits and closest-thumb rule ported; progress and meter percentages go from 0 to 100; the menu bar (`aria/menubar.go`) and the context menu trigger (`aria/contextmenu.go`) compose the menu trigger rules with the WAI-ARIA Authoring Practices menubar pattern and the behavior of Radix UI's Menubar and ContextMenu (MIT, no code copied), one press handler and one `presser` per trigger key. The color hooks (`aria/color*.go`) keep one `ColorState` (an `HSB` that keeps the hue and, for black, the saturation) shared by the area, the sliders and the field; the area has one `slider` thumb with a value text for both channels, where React Aria has two hidden range inputs, and the color slider is built on `UseSlider`; the hex field commits on blur and Enter and cancels characters that cannot be part of a code before they are typed. The tests are written for Tela, not taken from React Aria's suite.

## Lucide (icons, redistributed as data)

- **Source:** https://lucide.dev (`lucide-static` 1.61.0 from npm, https://github.com/lucide-icons/lucide; tarball integrity `sha512-yLAiQB3MWUJEstsEDAwenSA3m7vr1tdLvgnC16Q/XMxRRma0DWwMs8M2bWlvgShrIs1m2mWMJ+0AiahqQO4GrA==`)
- **License:** ISC, Copyright (c) 2026 Lucide Icons and Contributors; the icons Lucide derived from Feather (listed in the license file) are MIT, Copyright (c) 2013-present Cole Bemis
- **Used in:** `icons/lucide/` (the separate module `github.com/kuronosu/tela/icons/lucide`): `icons_gen.go` holds the SVG drawings as Go string data, and `LICENSE` is Lucide's license file, unchanged, with the Feather notice; and, also a subset (27 icons copied from the same `lucide-static` 1.61.0 files, unaltered), in `examples/aria/icons/` (the `*.svg` files, `icons_gen.go` generated from them by `tela icons gen`, and the same `LICENSE`), because the root module cannot import `icons/lucide`
- **Notes:** The SVG files are minified by `tela icons gen` (comments, class attributes and white space removed; the minified document is checked to draw the same display lists as the original) and wrapped in one `tela.Icon` variable each; the drawings are not altered. The license text is copied into the header of `icons_gen.go`. The root module (`github.com/kuronosu/tela`) contains none of it, except the icons kit draws by default, copied unaltered (every icon slot of the kit can be replaced; the kit cannot import `icons/lucide`): check and circle in `kit/choice.go` (checkbox, radio group), arrow-up-down, arrow-up and arrow-down in `kit/table.go` (the sort arrows of `kit.Table`, ADR-139); and, as test fixtures, six icons (mail, chevron-right, arrow-right, circle-alert, check, x) in `kit/icons_test.go`, three (bold, italic, underline) in `kit/icons_toggle_test.go` and the table's arrows in `kit/table_test.go`.

## shadcn/ui (class strings and markup; test oracle)

- **Source:** https://github.com/shadcn-ui/ui, `apps/v4/registry/new-york-v4/ui/*.tsx` and the theming CSS of `apps/v4`, at commit `617134d5810bba521803b3ed787190bacbb80b35` (branch main, 2026-10-24). Every component of `kit` follows this one commit (`kit.ShadcnCommit`)
- **License:** MIT, Copyright (c) 2023 shadcn
- **Used in:** `kit/` (the class strings of the recipes in `button.go`, `badge.go`, `card.go`, `avatar.go`, `label.go`, `input.go`, `textarea.go`, `separator.go`, `skeleton.go`, `field.go`, `checkbox.go`, `switch.go`, `radiogroup.go`, `toggle.go`, `togglegroup.go`, `slider.go`, `progress.go`), and the tests' copies of shadcn's own strings and markup in `kit/*_test.go`, `kit/harness_test.go` (the theme CSS) and `kit/testdata/chromium/`
- **Notes:** The class strings are ported, translated for what Tela does not have (`data-[state=...]:` to `checked:` or `aria-*:`, `[&_svg]:` to classes on the icon slots, `has-[>svg]:` to a Go condition, `border` to `border border-border`; `kit/doc.go` and ADR-130 list every deviation). The recipes are data in Go, not shadcn's cva code; the components are written for Tela's hooks. The pages in `kit/testdata/chromium/` are shadcn's markup rendered by headless Chromium with Tailwind CSS 4.3.3 (MIT), as the oracle of the conformance tests.

  ```
  MIT License

  Copyright (c) 2023 shadcn

  Permission is hereby granted, free of charge, to any person obtaining a copy
  of this software and associated documentation files (the "Software"), to deal
  in the Software without restriction, including without limitation the rights
  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
  copies of the Software, and to permit persons to whom the Software is
  furnished to do so, subject to the following conditions:

  The above copyright notice and this permission notice shall be included in all
  copies or substantial portions of the Software.

  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
  SOFTWARE.
  ```

## Sonner (default styles; test oracle for the toast stack)

- **Source:** https://github.com/emilkowalski/sonner (sonner 2.0.7, the version shadcn/ui's `sonner.tsx` at the pinned commit depends on, `^2.0.0`)
- **License:** MIT, Copyright (c) 2023 Emil Kowalski
- **Used in:** `kit/toast.go` (the sizes, gaps, stacking rule, timings and the dark-theme colors of `kit.Toaster`'s recipe are sonner's default stylesheet's, ported as classes), and `kit/testdata/sonner/` (`styles.css`, sonner's stylesheet unchanged, and the DOM it renders in a React page, captured with `capture.jsx`, as the Chromium conformance page of `kit/toast_test.go`)
- **Notes:** The stack's behavior (the newest toast in front, the others `translateY(gap * n) scale(1 - 0.05 n)` behind it, fanned out by their measured heights while the pointer is over them, timers paused meanwhile) follows sonner's `Toaster`; the code is written for Tela. Swipe to dismiss, promise toasts and rich colors are not ported.

  ```
  MIT License

  Copyright (c) 2023 Emil Kowalski

  Permission is hereby granted, free of charge, to any person obtaining a copy
  of this software and associated documentation files (the "Software"), to deal
  in the Software without restriction, including without limitation the rights
  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
  copies of the Software, and to permit persons to whom the Software is
  furnished to do so, subject to the following conditions:

  The above copyright notice and this permission notice shall be included in all
  copies or substantial portions of the Software.

  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
  SOFTWARE.
  ```

## shadcn/ui Select, Tabs and Sonner wrapper; Lucide icons in `kit`

- **Source:** shadcn/ui `select.tsx`, `tabs.tsx` and `sonner.tsx` at the commit of the shadcn/ui section above; Lucide (`lucide-static` 1.61.0, see the Lucide section)
- **License:** MIT (shadcn/ui); ISC (Lucide), Copyright (c) 2026 Lucide Icons and Contributors
- **Used in:** `kit/select.go`, `kit/tabs.go`, `kit/toast.go` (the class strings of the recipes) and their tests
- **Notes:** `kit/select.go` draws chevron-down and check, and `kit/toast.go` circle-check, info, triangle-alert, octagon-x, loader-circle and x, as unexported `tela.Icon` literals copied unaltered from the files `icons/lucide` holds (ADR-129, K7); each is replaceable by a field of the component. The ISC notice is in `icons/lucide/LICENSE`.
