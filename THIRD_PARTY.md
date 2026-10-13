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

## Taffy (layout test fixtures)

- **Source:** https://github.com/DioxusLabs/taffy (tag v0.14.0, commit 77f3856)
- **License:** MIT, Copyright (c) 2018 Visly Inc. and Taffy contributors
- **Used in:** `internal/layout/testdata/taffy/` (fixtures `block`, `flex`, `blockflex`, `leaf`, and `gentest/test_base_style.css`, `gentest/test_helper.js`, vendored unchanged, with `LICENSE`)
- **Notes:** The fixtures are HTML pages derived from Web Platform Tests. Expected layouts are measured from them with headless Chromium (ADR-039). `test_base_style.css` embeds the Ahem test font (public domain, https://github.com/Kozea/Ahem).
- **Ported code:** `internal/layout/{cache,compute,leaf,align,block,flex,absolute,round}.go` are ports of Taffy's `src/tree/cache.rs`, `src/tree/layout.rs`, `src/compute/mod.rs` (dispatch and `round_layout`), `src/compute/leaf.rs`, `src/compute/common/{alignment,scrollable_overflow,sizing_keyword}.rs`, `src/compute/block.rs` and `src/compute/flexbox.rs` (absolute layout from both), with Taffy's MIT notice and source paths in each file. Left out: margin collapsing, floats, grid, `flex-wrap: balance`, `box-sizing: content-box`. `calc()` of a length and a percentage (`layout.UnitCalc`) is Tela's own. Deviations to match Chromium (ADR-043) are listed in `internal/layout/doc.go`; two fixes over Taffy: leaves are measured with content-box known sizes in size queries, and cached full layouts measured during baseline queries are dropped.

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
- **Notes:** Design references only (syscall-based window class, message loop and DPI setup; mouse capture, leave tracking and DPI quirks). No source was copied; the backend is written against the Win32 documentation. Listed as a courtesy.
