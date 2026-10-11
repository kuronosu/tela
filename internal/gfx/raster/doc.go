// Package raster is Tela's CPU rasterizer: it renders a gfx.DisplayList into
// an *image.RGBA. It is the reference backend for goldens and headless
// rendering (ADR-002).
//
// # Pixels
//
// The destination holds premultiplied RGBA8 in sRGB (Go's image.RGBA
// convention). Device pixel (x, y) is the pixel of dst at image coordinates
// (x, y) and covers the square [x, x+1) x [y, y+1); the display list's root
// origin maps to device point (0, 0) and root coordinates are multiplied by
// Options.Scale. Rendering into dst.SubImage(r) is therefore the same as
// rendering into dst with Region r.
//
// Every draw composites with source-over directly on the sRGB-encoded,
// premultiplied 8-bit values, without linearization (ADR-007): with source
// color s (premultiplied), coverage c in [0, 1] and destination d,
//
//	out = s*c + d*(1 - s.a*c)
//
// computed exactly in integers and rounded once to nearest. Coverage
// multiplies the source, so an opaque color at full coverage is copied as is.
//
// # Coverage
//
// Shapes are anti-aliased with exact area coverage: the coverage of a pixel
// is the area of the pixel covered by the shape (a box filter).
//
//   - Paths are flattened into lines with gfx.Path.Flatten at a device-space
//     tolerance (FlattenTolerance), mapped to device space, snapped to a
//     1/256 pixel grid, and rasterized by signed-area accumulation (the
//     approach of font-rs and vello_cpu): each line adds, per row and per
//     cell, the signed area to its right to an accumulation row whose prefix
//     sum is the winding-weighted coverage a. NonZero uses min(1, |a|) and
//     EvenOdd uses |a - 2*round(a/2)|. Both are exact where edges do not
//     cross inside a pixel; where they do, the accumulated value mixes the
//     windings of the sub-pixel regions (as in every area-accumulation
//     rasterizer) and EvenOdd can differ slightly from the true area.
//   - Axis-aligned rectangles (under transforms that keep them axis-aligned)
//     take a fast path whose coverage, the product of the horizontal and
//     vertical overlaps on the same 1/256 grid, is bit-identical to the path
//     rasterizer's result for the same rectangle.
//   - Rounded rectangles under scale/translate transforms take a fast path:
//     rectangle coverage minus, for pixels cut by a corner arc, the exact
//     area of the pixel outside the corner ellipse (closed form, see
//     corner.addOutside), for any normalized radii, elliptical and per
//     corner. Under rotation or skew, and for rectangles reaching beyond
//     ±2^20 device pixels, rounded rectangles are filled as paths.
//   - Strokes (StrokePath) are turned into outlines by gfx.Path.Stroke in
//     local space, with a tolerance of FlattenTolerance divided by the
//     transform's largest scale, and filled with NonZero. The outline
//     self-overlaps; where a winding-2 region meets the outside inside one
//     pixel, min(1, |a|) overestimates that pixel (measured: one pixel of a
//     tight curve, 72 levels; see TestStrokeCoverage).
//
// # Paints, clips and layers
//
// A paint is a shader that produces premultiplied colors per device pixel,
// sampled at the pixel center and mapped back to the command's local space
// with the inverse transform: solid colors, linear, radial and conic
// gradients (gradient.go: CSS geometry, pad spread, premultiplied
// interpolation in Oklab or sRGB through 256-entry ramps per stop segment,
// exact hard stops) and images (image.go: nearest or bilinear sampling,
// clamped to the Src rectangle).
//
// Clips (clip.go) narrow the writable rectangle and, unless they are
// pixel-aligned rectangles, add an 8-bit coverage mask that multiplies the
// coverage of everything drawn inside them; nested clips multiply their
// masks. Masks cover only the region ∩ the clip bounds. Layers render their
// content into an offscreen buffer (region ∩ layer bounds) and composite it
// at PopLayer with source-over, the layer's opacity and the clip that was
// active at PushLayer. Mask and layer buffers are pooled by the Renderer.
//
// The accumulation is done in 64-bit integers, so it is associative: the
// value of a pixel does not depend on the order in which edges and cells are
// summed.
//
// # Regions and tiling
//
// Options.Region restricts drawing to a rectangle of device pixels, and the
// pixels inside it are bit-identical to those of a full render. This is the
// foundation of tiled, parallel rendering: every per-pixel quantity is a
// function of the absolute pixel coordinates and the geometry only, never of
// where the region starts.
//
//   - Geometry is clipped to a fixed, region-independent box (±2^20 device
//     pixels) before it is snapped to the fixed-point grid; parts left and
//     right of the box are replaced by vertical lines on its boundary, which
//     keeps the winding numbers of the pixels inside it.
//   - For each row, the pieces of a line that fall left of the region are
//     not walked cell by cell: their exact (integer) contribution, the
//     signed height they span, is added to the first accumulation cell.
//     Cell boundary crossings are computed in closed form from the line, not
//     incrementally, so the split point is the same value a full walk would
//     reach.
//   - Commands are culled with DisplayList.Visit using the region mapped to
//     root space and grown by one device pixel, so a command that can touch
//     a pixel of the region is never skipped.
//   - Paths reaching far outside the region have their curves culled: a
//     curve whose control box lies entirely outside the region (with a
//     margin) is replaced by its chord, which gives exactly the same pixels
//     inside the region (see Renderer.flattenCulled). This bounds the work
//     for huge paths without making the result depend on the region.
//
// # Determinism
//
// The same display list gives the same pixels on every platform. Integer
// arithmetic is exact; the floating-point code is written so that results do
// not depend on the CPU. The Go specification allows a compiler to fuse
// x*y + z into one fused multiply-add, which skips the rounding of the
// product, and Go does so on arm64, ppc64le, s390x, riscv64 and amd64 with
// GOAMD64=v3. An explicit conversion such as float64(x*y) forces the
// rounding and prevents the fusion, so this package wraps every product that
// feeds an addition in such a conversion, unless the product is exact anyway
// (a float32 times a float32 computed in float64, or a multiplication by a
// power of two). Products returned by a function count too: after inlining,
// the caller's addition can absorb them. math.Atan, math.Asin, math.Pow and
// math.Cbrt are avoided for the same reason (their pure-Go code can be
// fused, and some math functions are assembly on one architecture only):
// internal/gfx/internal/fmath holds fusion-safe ports. Determinism tests
// record digests of the float64 intermediates on amd64; run them with
// GOARCH=arm64 go test -exec qemu-aarch64-static to check another
// architecture.
//
// The flattening itself (gfx.Path.Flatten), stroking (gfx.Path.Stroke) and
// the composition of transforms (gfx.Affine.Mul) live in package gfx.
//
// # Concurrency
//
// A Renderer holds scratch buffers and must be used by one goroutine at a
// time; use one Renderer per raster worker. DisplayLists are immutable and
// may be rendered by any number of Renderers at once. Render writes only the
// pixels inside the region, so workers may render disjoint regions of the
// same image concurrently.
//
// # Not implemented yet
//
// DrawShadow and layer filters (Blur, DropShadow, ColorMatrix) come with task
// T6, parallel tiling with T9: Render panics with "raster: DrawShadow is not
// implemented yet" or "raster: layer filters are not implemented yet" when
// it meets them in the visited part of the list. Blend modes other than
// SrcOver and BackdropFilter are phase 6 features and panic with "raster:
// blend mode <mode> is not implemented until phase 6" and "raster:
// BackdropFilter is not implemented until phase 6".
package raster
