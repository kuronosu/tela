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
- **Used in:** `internal/gfx/flatten.go`, `internal/gfx/arc.go`
- **Notes:** Ported from `bezpath.rs` (`flatten`), `quadbez.rs` (`estimate_subdiv`, `determine_subdiv_t`, the parabola-integral approximations), `cubicbez.rs` (`to_quads`, `subsegment`) and `arc.rs` (`Arc::append_iter`, `sample_ellipse`). Diverges from kurbo in `Flatten`: it works on float64 internally and emits float32 points; it spends 90% of the tolerance as a safety margin; it splits cubics at inflections and sharp turns (near-cusps) before subdividing, because kurbo's own output exceeds the tolerance across strong inflections and at cusp tips; and it emits the turning point of collinear curves that fold back on themselves. Each ported file keeps the Kurbo Authors' copyright header.

## color.js (reference data only)

- **Source:** https://github.com/color-js/color.js (v0.7.1)
- **License:** MIT
- **Used in:** `internal/gfx/color.go` (matrices), `internal/gfx/color_test.go` (expected values)
- **Notes:** The Oklab / XYZ / linear sRGB matrices are the ones published in CSS Color 4 and used by color.js (recalculated for a consistent D65 white). The gamut-mapping algorithm is the one of CSS Color 4 section 13, written from the spec. The test expectations were generated with color.js (`toGamut({method: "css"})`); no color.js code was copied.

## Go standard library (math/atan.go) and Cephes

- **Source:** https://go.dev (`src/math/atan.go`, go1.27.2), itself based on the Cephes Mathematical Library by Stephen L. Moshier
- **License:** BSD-3-Clause (Copyright 2009 The Go Authors); Cephes: "may be used freely but it comes with no support or guarantee"
- **Used in:** `internal/gfx/raster/atan.go`
- **Notes:** `xatan` and `satan` ported with every product that feeds an addition wrapped in an explicit `float64` conversion, so that the compiler cannot fuse it into a multiply-add and the result is the same on every architecture (Go fuses on arm64, ppc64le, s390x, riscv64 and amd64 with `GOAMD64=v3`). `asin01` is new. The file keeps the Go copyright header and the Cephes notice.
