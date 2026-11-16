// Package gpu renders a gfx.DisplayList on the GPU, with the meaning the
// CPU rasterizer (internal/gfx/raster) gives it (ADR-282): the same
// options, premultiplied RGBA8 holding sRGB-encoded values, source-over on
// those values (ADR-007), into a target that keeps its pixels between
// frames, so a render of a region leaves the rest as it was. The
// rasterizer stays the reference: goldens are CPU-only, and this package's
// tests compare its output with the rasterizer's within a tolerance.
//
// # Two halves
//
// The Encoder turns a display list into a Frame: passes over targets, each
// of batches of instances, in a byte layout the shaders read, with the
// gradient ramps they look up, the kernels of their filters and the pixels
// new in the textures they read. It knows no
// graphics API, has no build tag and is tested without a GPU. The geometry
// it computes on the CPU (snapping to the rasterizer's 1/256 pixel grid,
// inverse transforms, gradient coefficients) is in float64 and fusion-safe
// (ADR-041), then written as float32 relative to each instance's origin.
//
// A Device executes frames. The only one is Vulkan (vulkan.go, over
// internal/vk), compiled with the build tag telagpu on Linux and Windows
// (ADR-281). Without the tag, and on other systems, Open returns
// ErrUnavailable and nothing of Vulkan or purego is linked.
//
// # What is drawn
//
// Every command of a display list is drawn. Most of it by one pipeline,
// the quad (shaders/quad.vert, quad.frag): an instance is a device
// rectangle whose fragment shader evaluates, per pixel, a coverage, the
// clips, and a paint.
//
//   - FillRect, FillRRect and DrawShadow: the coverage of a rectangle or
//     rounded rectangle, and the closed-form shadow of one (ADR-017),
//     computed by the shader.
//   - FillPath and StrokePath: coverage rasterized on the CPU by the
//     rasterizer's own code (raster.Masker) and read from the coverage
//     atlas (cover.go).
//   - DrawGlyphRun: the rasterizer's cached masks and sprites, under its
//     own keys, read from the atlases; the glyphs it does not cache are
//     paths, and display lists encoded in place (glyph.go).
//   - DrawImage: the image's texture, sampled as the rasterizer samples
//     its pixels (image.go).
//   - Paints: a solid color, or any gradient, on all of the above.
//   - PushClipRect, PushClipRRect and PushClipPath, nested in any way: the
//     scissor, one shape in the instance, and one clip mask in the
//     coverage atlas that holds the product of the others (clip.go).
//   - PushTransform and Marker.
//   - PushLayer and PopLayer: the layer is drawn into an offscreen target
//     and composited by an instance that paints its pixels, with its
//     opacity, its mask and its blend mode, through the clip around it
//     (layer.go). Its filters, and those of a BackdropFilter, are steps of
//     a second pipeline, the filter (shaders/filter.frag), between
//     offscreen targets (filter.go).
//
// What a device cannot draw is never drawn wrong silently:
// Frame.Unsupported, and the Result of a Render, name it. That is only
// what is larger than the device's textures (an image, or a layer grown
// for its filters, beyond 16384 pixels on the devices of today).
//
// # Targets and passes
//
// A frame is a list of passes, executed in order. A pass draws into the
// destination or into an offscreen target of a pool the Encoder keeps on
// the device (target.go), with the pixels of other targets as textures;
// a pass never reads the target it draws into. A pass that does not clear
// goes on over the pixels its target holds: the pass of a surface ends
// where a layer or a backdrop filter starts and goes on after it, and
// where an atlas is emptied in the middle of a frame.
//
// A layer is a pass into a target as large as the pixels it composites
// (grown by the reach of its filters, as the rasterizer's surface is),
// cleared first, encoded without the clip around the layer; its mask is
// its list drawn into another target; a blend mode other than source-over
// reads the pixels under the layer from a copy, since a fragment cannot
// read the target it writes, and its shader computes the W3C formula on
// 8-bit values, with the rasterizer's integers where it has them. A
// backdrop filter ends the pass of its surface, filters the surface's
// pixels into a target (mirrored at the shape's bounds, like the
// rasterizer's) and paints the result through the shape when the pass
// goes on.
//
// A blur is a kernel per axis whose weights are data: the rasterizer's
// own kernel for small deviations, and for larger ones blocks of pixels,
// blurred and interpolated back, as the rasterizer's large blurs are
// (filter.go says how the two differ, and by how much). Texels are read
// by their whole coordinates and never filtered by the device, so the
// pixels of a filter do not depend on the target it runs in.
//
// A region that backdrop filters read beyond is grown as the rasterizer
// grows it (raster.BackdropRegion) and rendered into an offscreen target
// that starts from the destination's pixels there, or from the clear
// color, and only the region is copied to the destination: the pixels
// beyond the region are the previous frame's final ones, not the backdrop
// a filter in the middle of the list is to read.
//
// # Pixels of a target
//
// A target that holds a window's frames (core's GPU frame, ADR-281's
// readback) is also copied from and inside. Neither goes through the
// Encoder, and each call is one submission that is waited for:
//
//   - ReadPixels copies rectangles into an image: the damage of a frame in
//     one call. The pixels cross a buffer in cached host memory (the
//     write-combined memory some drivers list first is read at a fifteenth
//     of the speed) and are copied again into the image;
//   - Shift moves a rectangle inside the target, as a scroll does to the
//     pixels of a framebuffer, through a surface the device keeps for it
//     (a copy of an image onto itself may not overlap), which Trim gives
//     up.
//
// A device that reports that it is lost stays lost: every call returns
// ErrDeviceLost from then on, Close still destroys what it holds, and the
// caller draws with the CPU rasterizer.
//
// # Resources
//
// Frames draw with textures that outlive them. The Encoder decides what
// they hold; the device only stores pixels.
//
// The offscreen targets are a pool (target.go): textures in a few sizes,
// that a layer drawn frame after frame finds again, listed by every frame
// (Frame.Targets) and bounded like the rest. Their pixels live for a
// frame.
//
// The coverage atlas is an array texture of 8-bit pages (atlas.go) packed
// in shelves. It holds glyph masks, the coverage of paths and strokes, and
// clip masks, each found again by what makes its pixels (the rasterizer's
// glyph key; a path's content, its transform up to whole pixels, its rule
// or stroke style; the chain of a mask's clips). A picture larger than
// half a page is cut into tiles, and a tile that is empty or entirely
// covered is only noted. The color atlas is the same for the RGBA sprites
// of color glyphs. An image has a texture of its own, named by its ID.
//
// A frame uploads only what is new: Frame.Data and Pass.Uploads for the
// atlases, Frame.Images for the images. A frame that draws what earlier
// frames drew uploads nothing. Since a device mirrors what one Encoder
// decided, frames must be drawn in the order they were encoded:
// Frame.Res numbers the changes, a device refuses a frame it cannot follow
// (ErrResourcesLost), and the Renderer then starts over with a frame that
// uploads all it needs. One Renderer serves every target of a device.
//
// Memory is bounded (Limits) and follows use, like the rasterizer's
// scratch (raster/trim.go). When an atlas is full, the pictures the frame
// being encoded does not use are evicted, the oldest first. When the
// frame alone fills it, the frame is flushed: the pass ends with the
// instances encoded so far, which read the atlas as it is, and a new pass
// goes on in the emptied atlas with uploads of its own, so the pixels are
// right whatever the limits. EndFrame, called once per frame, gives up the
// pictures, images, offscreen targets and cached shapes that no frame
// used for idleFrames frames, and the pages left empty; Trim gives up
// everything, on the
// device too, for a window that rests. Without either an Encoder keeps
// what it has until its limits make it evict.
//
// # How it follows the rasterizer
//
// A pixel's value is a function of its coordinates and of its instance
// only, and an instance does not depend on the region rendered (nor do the
// pictures it reads: a path's coverage and a clip's mask are rasterized by
// whole tiles, never cut to the region; a layer's target is as large as
// the region needs, but its pixels are functions of their device
// coordinates), so a render of a region gives the pixels of a full
// render, bit for bit (TestRegion), through layers, filters and backdrop
// filters too.
//
// Coverage is the area of the pixel inside the shape, rounded to 8 bits
// like the rasterizer's, and multiplied like it by the clip's. Under
// transforms that keep rectangles axis-aligned (flips and quarter turns
// included) the shape is in device space on the rasterizer's 1/256 pixel
// grid: a rectangle's area is the product of two overlaps, exactly the
// rasterizer's value, and a corner's ellipse takes away the area outside
// it, integrated by Gauss-Legendre quadrature of a form without
// cancellation (the rasterizer has it in closed form, in float64). Under
// rotations and skews, where the rasterizer fills a path, a rectangle is
// a parallelogram whose area inside the pixel is exact (the sum the
// rasterizer accumulates per edge), and a corner's ellipse is its tangent
// near the pixel bent by its curvature. The rasterizer flattens those
// corners to polygons within raster.FlattenTolerance (0.1 pixel), so the
// pixels along them differ by up to that much coverage; elsewhere the two
// agree within a level or two.
//
// A solid paint is the rasterizer's premultiplied 8-bit color. A gradient
// computes its parameter at the pixel center with the rasterizer's
// geometry, in float32, and looks its color up in the ramps the
// rasterizer bakes (raster.GradientRamp), entry by entry, so stops,
// Oklab interpolation and hard stops are its own. A pixel whose center is
// exactly on a discontinuity (a hard stop, a conic seam, a cone's edge)
// can take the other side.
//
// A shadow is raster/shadow.go's formula: the blurred rectangle as a
// product of two box-filtered erf integrals, minus the blurred region
// between each corner's box and its ellipse, split where the arc's slope
// is 1 and integrated by Gauss-Legendre panels along. The rasterizer
// tabulates this per row and column; the shader evaluates it per pixel,
// over the part of each piece within the Gaussian's reach. Under a scale
// and a translation the Gaussian is evaluated in device space, under any
// other transform in the command's local space; the rasterizer does the
// latter only for rotations with a uniform scale, and blurs a mask
// otherwise, so shadows under skews differ (see shadow.go).
//
// The coverage of paths, strokes, glyphs and clip masks is the
// rasterizer's, bit for bit: the same flattening at raster.FlattenTolerance,
// the same grid, the same rules, the same stroke outlines, the same glyph
// cache. A path is rasterized without the whole pixels of its translation
// and placed at them, which moves a picture and not its values. Nested
// masks multiply with the rasterizer's rounding; a clip carried by the
// instance multiplies in another order, a level at most.
//
// An image is sampled with the rasterizer's arithmetic: the pixel that
// contains the point, or four pixels with weights quantized to 1/256 and
// summed in integers, clamped to the source rectangle. The point is
// computed in float32: the weights can differ by 1/256, and a pixel center
// exactly on the boundary of two pixels of an image sampled by its nearest
// pixel can take the other one.
//
// A layer is composited on 8-bit values: its opacity times its mask's
// coverage times the clip's, each product rounded like the rasterizer's,
// and for the blend modes the rasterizer's exact integer formulas (its
// float64 ones in float32 for color-dodge, color-burn, soft-light and the
// non-separable modes, a level of difference in places). A color matrix
// is the rasterizer's arithmetic in float32; a drop shadow's composite
// its integers. Blurs are within a level or two of the rasterizer's (see
// filter.go), which a color matrix after them scales.
//
// Blending is the device's: source-over on the encoded values, which the
// API only requires at the precision of the target. Measured (TestBlend):
// Mesa on Intel blends at high precision, within a level of the
// rasterizer's exact rounding on a few pixels; NVIDIA rounds the source
// to 8 bits first, a level of difference on about half the pixels of a
// translucent fill.
//
// # Tests
//
// The encoder's tests run everywhere. The tests of the device need the
// tag and skip without Vulkan; they run on every physical device.
// TestCompare renders the rasterizer's shared golden scenes
// (internal/gfx/gfxtest) and this package's at scales 1, 1.5 and 2 and
// compares the pixels with the rasterizer's under the tolerance of
// ADR-282, which lives in compare_test.go with the scenes that need
// another one and why. With TELA_GPU_DUMP=dir it writes the two images
// and their difference; with TELA_VK_VALIDATE=1 the tests run under the
// Khronos validation layer and fail on anything it reports, a texture left
// alive included. The logic of the resources (packing, eviction, keys,
// flushes, clip stacks) is tested without a device, by applying the
// uploads of frames to atlases in memory and checking what every instance
// reads (resources_test.go); so are the passes of layers and filters,
// their targets and what they read (layer_test.go), and the kernels of the
// blurs against the rasterizer's (TestBlurKernels).
//
// # Shaders
//
// The shaders are GLSL sources compiled to SPIR-V by go generate
// (glslangValidator, a development tool); the .spv files are checked in
// and embedded. The set is fixed: nothing is generated at run time.
package gpu

//go:generate glslangValidator --quiet -V -o shaders/quad.vert.spv shaders/quad.vert
//go:generate glslangValidator --quiet -V -o shaders/quad.frag.spv shaders/quad.frag
//go:generate glslangValidator --quiet -V -o shaders/filter.frag.spv shaders/filter.frag
