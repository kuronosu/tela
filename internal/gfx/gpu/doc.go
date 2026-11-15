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
// The Encoder turns a display list into a Frame: passes of batches of
// instances, in a byte layout the shaders read, with the gradient ramps
// they look up. It knows no graphics API, has no build tag and is tested
// without a GPU. The geometry it computes on the CPU (snapping to the
// rasterizer's 1/256 pixel grid, inverse transforms, gradient
// coefficients) is in float64 and fusion-safe (ADR-041), then written as
// float32 relative to each instance's origin.
//
// A Device executes frames. The only one is Vulkan (vulkan.go, over
// internal/vk), compiled with the build tag telagpu on Linux and Windows
// (ADR-281). Without the tag, and on other systems, Open returns
// ErrUnavailable and nothing of Vulkan or purego is linked.
//
// # What is drawn
//
// One pipeline, the quad (shaders/quad.vert, quad.frag): an instance is a
// device rectangle whose fragment shader evaluates, per pixel, the
// coverage of a rectangle or rounded rectangle, the closed-form shadow of
// one (ADR-017), one anti-aliased clip, and a solid or gradient paint.
// FillRect, FillRRect, DrawShadow, PushTransform, PushClipRect and Marker
// are handled. A clip whose device edges fall on pixel boundaries is a
// scissor; another one is a shape whose coverage multiplies the
// instance's, and since an instance has one such slot, a second clip of
// this kind that still matters inside the first is not handled.
//
// Everything else (paths, strokes, glyphs, images, rounded and path clips,
// layers, backdrop filters) is not drawn yet and is never drawn wrong
// silently: Frame.Unsupported, and the Result of a Render, name what was
// left out.
//
// # How it follows the rasterizer
//
// A pixel's value is a function of its coordinates and of its instance
// only, and an instance does not depend on the region rendered, so a
// render of a region gives the pixels of a full render.
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
// Khronos validation layer and fail on anything it reports.
//
// # Shaders
//
// The shaders are GLSL sources compiled to SPIR-V by go generate
// (glslangValidator, a development tool); the .spv files are checked in
// and embedded. The set is fixed: nothing is generated at run time.
package gpu

//go:generate glslangValidator --quiet -V -o shaders/quad.vert.spv shaders/quad.vert
//go:generate glslangValidator --quiet -V -o shaders/quad.frag.spv shaders/quad.frag
