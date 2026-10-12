// Package gfx holds Tela's basic graphics types and the DisplayList, the
// contract between the paint phase, the CPU rasterizer and any future GPU
// backend.
//
// gfx imports only the standard library and sits just above the platform
// layer (ARCHITECTURE section 5).
//
// # Coordinates
//
// Everything is float32 in logical pixels with the origin at the top-left and
// Y growing downward. Display scale is applied at raster time, never here.
// Rect is half-open: [X0, X1) x [Y0, Y1). Angles are radians, except hue in
// OKLCH, which follows CSS Color 4 and is in degrees. In this y-down system a
// positive rotation or sweep turns clockwise on screen.
//
// # Color
//
// Color is non-premultiplied sRGB with float32 components in [0, 1]. Colors
// are composited in sRGB space like browsers (ADR-007). Oklab and OKLCH
// constructors convert with the CSS Color 4 matrices and map out-of-gamut
// colors into sRGB with the CSS gamut-mapping algorithm. Gradients carry their
// interpolation color space, Oklab by default (ADR-023); interpolation is done
// on premultiplied colors (CSS Color 4, section 12.3).
//
// # Paths
//
// Path stores verbs and points compactly. Curves are flattened with kurbo's
// algorithm (Path.Flatten, or a reusable Flattener). Stroking is path to
// path (Path.Stroke, ADR-026): it returns a lines-only outline that a
// rasterizer fills with NonZero; StrokePath commands carry the style.
//
// Flattening, arcs, stroking and the Affine operations give the same points,
// bit for bit, on every architecture: products are rounded explicitly so
// that the compiler cannot fuse them into multiply-adds, and the elementary
// functions they need come from the deterministic ports in
// internal/fmath. (The color
// space conversions and Affine constructors such as Rotate still use the
// math package; their results are rounded to float32, which hides the
// last-bit differences in practice.)
//
// # The DisplayList
//
// A DisplayList is a flat, immutable slice of Command values: FillRect,
// FillRRect, FillPath, StrokePath, DrawImage, DrawGlyphRun, DrawShadow,
// BackdropFilter and the Push/Pop pairs for clips, transforms and layers. Push/Pop nest strictly
// as a single stack. Commands are plain structs so that a rasterizer can use a
// type switch.
//
// A Builder records the list. It validates nesting, copies every mutable
// input so the finished list is immutable and safe for concurrent readers, and
// computes a conservative root-space bounding rectangle for each command,
// which the rasterizer uses to cull commands per tile (DisplayList.Visit).
// Non-finite values never reach the list: the offending command, or the whole
// block for a push, is dropped. Dump and String give a deterministic text form
// for tests and tools.
//
// Semantics of the commands that need precision:
//
//   - Clip and transform shapes are given in the current, already transformed
//     coordinate space. PushTransform{T} makes current = current.Mul(T).
//   - PushLayer renders its content offscreen, applies the filters in order,
//     and composites with opacity and blend mode.
//   - DrawShadow follows CSS box-shadow: Blur is the CSS blur radius (the
//     Gaussian standard deviation is Blur/2), an outer shadow is painted only
//     outside the box and an inset shadow only inside it.
//   - Blur filters take a standard deviation, like CSS filter: blur().
//
// # Text
//
// DrawGlyphRun draws positioned glyphs of one Font at one size. Font is an
// interface implemented by package text (on go-text/typesetting), so that gfx
// and the rasterizers do not depend on a font library: it gives each glyph's
// outline and bounds in em space (1 em = 1 unit, y down, origin at the pen
// position on the baseline), and color glyphs as DisplayLists in em space
// that receive the foreground color. One em-space outline serves every
// size; rasterizers cache the rendered glyphs by Font.FontKey.
package gfx
