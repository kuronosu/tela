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
