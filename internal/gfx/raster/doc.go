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
// # Shadows and filters
//
// DrawShadow (shadow.go) follows CSS box-shadow, spread rule for radii
// included. Rounded-rectangle shadows are evaluated in closed form, after
// Evan Wallace: the blurred rectangle is a product of erf-based 1D
// integrals, and each corner subtracts the blurred region between its box
// and its ellipse, split where the arc's slope is 1 into a rectangle and two
// pieces whose boundary is never steep, integrated in closed form across and
// with Gauss-Legendre panels along. Every value is box-filtered over the
// pixel like the rasterizer's coverage, so Blur -> 0 converges to the
// anti-aliased fill; the error is a fraction of a level of 255. Under
// scale/translate transforms the 1D factors are tabulated per column and
// row; under rotations with a uniform scale the value is evaluated in local
// space per pixel; under skews the shape is rasterized and blurred.
//
// Layer filters (filter.go) run at PopLayer over a surface that extends the
// filters' reach beyond the region and beyond the clip active at PushLayer.
// Blur and DropShadow use the separable Gaussian of blur.go (three box blurs
// for device standard deviations of 2 and more, as SVG recommends, exact
// kernels below), whose support never exceeds the reach gfx.FilterOutset
// promises; ColorMatrix works on un-premultiplied values. The standard
// normal CDF and its integral come from interpolated tables (gauss.go).
//
// The accumulation is done in 64-bit integers, so it is associative: the
// value of a pixel does not depend on the order in which edges and cells are
// summed.
//
// # Text
//
// DrawGlyphRun (glyph.go) draws each glyph of a run in one of two ways.
//
// Cached, when the current transform is a positive scale and a translation
// and the device em size (the transform's scale times the run's Size,
// rounded to float32 like gfx.Affine.Mul) is at most 256 pixels along both
// axes: the device pen position is rounded to a quarter pixel in x and to a
// whole pixel in y (baseline snapping, as in Skia and browsers), and the
// glyph comes from a GlyphCache keyed by (Font.FontKey, glyph, device em
// size, quarter-pixel offset, FlattenTolerance). An outline glyph is an
// 8-bit coverage mask made by the path rasterizer from the em-space outline
// under Scale(em size) and Translate(offset/4, 0), with fillPath's
// tolerance; placing it at whole pixels changes nothing, so a cached glyph
// has exactly the pixels of filling its outline at the snapped position
// (TestGlyphCachedMatchesPaths). The mask is composited with the run's
// paint like any coverage: gradients are sampled at device pixel centers in
// the run's space, through the clip mask and onto the current layer. A
// color glyph (Font.ColorGlyph) is a premultiplied RGBA sprite, its
// DisplayList rendered by a nested Renderer onto a transparent surface at
// the same size and offset and trimmed to its painted pixels, keyed further
// by the foreground color's float32 bits and the color glyph nesting depth;
// it is composited with source-over through the clip like a layer.
//
// Direct, otherwise (rotations, skews, flips, larger sizes, pens beyond
// ±2^19 device pixels, glyphs whose em bounds exceed 1024 device pixels):
// the outline is filled with NonZero under the glyph's full transform
// (current transform, translation to the unsnapped pen, Scale(Size)), and a
// color glyph's DisplayList is visited inline under that transform. Inline
// commands composite one by one instead of as a group, which differs from
// a sprite only by rounding when they use source-over. Color glyphs nested
// more than three deep are not drawn, which stops fonts whose color glyphs
// draw themselves.
//
// Glyphs with empty Font.GlyphBounds are skipped, and so is each glyph
// whose bounds miss the clip, so a band only looks up the glyphs that cross
// it. A fully transparent solid paint hides outline glyphs, not color
// glyphs. The cache never changes the pixels: masks and sprites are a
// function of their key, the decision between cached and direct drawing
// depends on the transform, the size and the glyph only, and the pen
// positions are computed with exact products, so hits, misses, evictions,
// regions and band splits give the same pixels.
//
// The GlyphCache is shared across frames, Renderers and the band workers
// of a parallel Render (Options.GlyphCache; by default one cache of
// DefaultGlyphCacheBytes for the package). It is split into 16 shards with
// a mutex, a map and an LRU list each, and bounded by bytes per shard.
// Entries are immutable once added; a glyph rendered by two goroutines at
// once is added by the first and the second uses that entry. On a 1920x1080
// page, 2,000 glyphs of 14 px text take about 0.43 ms from a warm cache and
// 0.54 ms when every mask is made again, against 9.5 ms as paths
// (BenchmarkText1080p*, Intel Core Ultra 5 125H, one goroutine); blending
// the coverage, not the lookup, is most of the warm cost.
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
//   - A layer with filters renders its content over the region grown by the
//     filters' device reach (DisplayList.Visit grows the root-space region
//     by LayerReach likewise), and every pass of every filter runs over the
//     composited pixels grown by the total reach, so the values a pass reads
//     never depend on the region. A region therefore costs its area grown by
//     the reach: tiles under a large blur redo work their neighbors do too.
//   - Shadows cut their Gaussian at a fixed number of standard deviations
//     and choose their quadrature nodes from the geometry alone; the zone a
//     region evaluates only limits which pixels are computed.
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
// architecture. (The fusion may happen across statements: a product stored
// in a variable that is later added must be wrapped too.)
//
// The flattening itself (gfx.Path.Flatten), stroking (gfx.Path.Stroke) and
// the composition of transforms (gfx.Affine.Mul) live in package gfx.
//
// # Parallel rendering
//
// Render splits a frame into full-width horizontal bands rendered by
// several goroutines (Options.Workers; 0 means GOMAXPROCS). Each band is
// rendered as a region, so the frame is bit-identical to a sequential
// render for every worker count and band height. The goroutines claim
// bands from an atomic counter, so faster workers take more of them.
// Regions under 128x128 pixels, and regions that make a single band, are
// rendered on the calling goroutine.
//
// Bands rather than square tiles: a band reads and writes whole rows, and a
// filter layer costs extra only above and below it (band height plus twice
// the reach) instead of on four sides. The band height aims at four bands
// per worker, at least 16 rows. When filter layers cover much of the frame
// it grows, up to one band per worker, with their reach (see bandRowsFor).
//
// Every band repeats some work: it visits the whole list, flattens or
// strokes every path that crosses it, sets up every shadow that crosses
// it, and renders every filter layer it crosses grown by the reach. On
// 1920x1080 frames rendered band by band on one goroutine
// (BenchmarkBandOverhead), 16-row bands cost 35% more than one full render
// for 1000 rrects and 200 paths, 38% more for 100 cards with shadow-xl,
// nothing measurable for rotated shadows and gradients, and 205% more for a
// full-frame blur of standard deviation 8 (42% with 64-row bands, 200% with
// a standard deviation of 32). Frames dominated by filter layers with a
// large reach therefore scale worse than the rest (BenchmarkParallel on 18
// threads: about 1.6x for the cards under three blur-3xl circles, 3.3x for
// a full-frame blur of 8, against 5-6x for the same cards alone).
// Rendering such surfaces once per frame, shared read-only by the bands, is
// left to layer caching.
//
// # Concurrency
//
// A Renderer holds scratch buffers and must be used by one goroutine at a
// time. A parallel Render is safe because nothing mutable is shared:
//
//   - Each goroutine renders with a Renderer of its own: the calling
//     Renderer, or one of the worker Renderers it owns (created on first
//     use and reused for later frames; only that Renderer's Render starts
//     them). All scratch memory (edges, accumulation rows, masks, layer
//     and filter surfaces, blur planes, shadow tables, gradient ramps,
//     flattening and image caches, the Renderer that draws color glyph
//     sprites) lives in a Renderer.
//   - Each goroutine writes only the pixels of dst inside the bands it
//     claimed, and reads only those pixels of dst: source-over reads the
//     pixel it writes, and layers and filters work on private surfaces.
//     The bands are disjoint, so no pixel of dst is accessed by two
//     goroutines.
//   - What the goroutines share is read-only: the DisplayList, its paths,
//     paints, images and fonts (immutable by contract; fonts are safe for
//     concurrent use), the job description (written before the goroutines
//     start, read after they are waited for), the masks and sprites of the
//     glyph cache (immutable once added), and package tables built at init
//     (gaussTab, Gauss-Legendre nodes, color matrices). The band counter,
//     the panic slot and the glyph cache's maps and LRU lists are the only
//     shared writes; they are an atomic counter and mutexes.
//   - Render waits for every goroutine before it returns. A panic in any
//     band (an unimplemented feature, for example) stops the claiming of
//     new bands, and once every goroutine is done, Render panics again on
//     the calling goroutine with the original value. Worker goroutines are
//     started by each parallel Render and return as soon as they report
//     that they are done, so none keeps running after Render returns.
//
// The race detector needs cgo, so the tests check these rules by their
// effect instead: parallel renders with many worker counts and band
// heights, and several parallel Renderers sharing display lists, images
// and destination images, must match the sequential pixels exactly.
//
// DisplayLists are immutable and may be rendered by any number of
// Renderers at once. Render writes only the pixels inside the region, so
// different Renderers may also render disjoint regions of the same image
// concurrently.
//
// # Not implemented yet
//
// Blend modes other than SrcOver and BackdropFilter are phase 6 features:
// Render panics with "raster: blend mode <mode> is not implemented until
// phase 6" and "raster: BackdropFilter is not implemented until phase 6"
// when it meets them in the visited part of the list. A BackdropFilter reads
// pixels painted before it up to DisplayList.BackdropReach around its shape,
// which can lie in another band: parallel rendering will need bands grown
// by that reach (see renderParallel).
package raster
