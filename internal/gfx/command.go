package gfx

// Command is one entry of a DisplayList. The interface is sealed: the
// concrete types below are the complete set, so a rasterizer can handle them
// with a type switch on the values (not pointers):
//
//	switch c := cmd.(type) {
//	case gfx.FillRect: ...
//	}
//
// Draw commands paint with the transform and clip that are current at that
// point of the list. Shapes, paints and clip regions are expressed in the
// current (transformed) coordinate space: PushTransform changes the meaning of
// every coordinate that follows until its PopTransform. All clips, transforms
// and layers are properly nested as one stack (see Builder).
//
// Commands in a list owned by a DisplayList are immutable; their slices
// (path points, gradient stops, dashes, filters, glyphs) must not be
// modified.
type Command interface {
	isCommand()
	// Name returns the command name used by Dump, such as "FillRect".
	Name() string
}

// FillRect fills a rectangle (anti-aliased when not pixel aligned).
type FillRect struct {
	Rect  Rect
	Paint Paint
}

// FillRRect fills a rounded rectangle. RRect is normalized.
type FillRRect struct {
	RRect RRect
	Paint Paint
}

// FillPath fills the interior of Path according to Rule. Open subpaths are
// closed implicitly.
type FillPath struct {
	Path  Path
	Rule  FillRule
	Paint Paint
}

// StrokePath strokes the outline of Path with the given style.
type StrokePath struct {
	Path  Path
	Style StrokeStyle
	Paint Paint
}

// LineJoin selects how two stroked segments are joined.
type LineJoin uint8

const (
	// JoinMiter extends the outer edges to a sharp corner, unless the miter
	// is longer than MiterLimit, in which case the join is a bevel. It is the
	// zero value (the SVG and CSS default).
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

// String returns "miter", "round" or "bevel".
func (j LineJoin) String() string {
	switch j {
	case JoinRound:
		return "round"
	case JoinBevel:
		return "bevel"
	}
	return "miter"
}

// LineCap selects the ends of open subpaths and dashes.
type LineCap uint8

const (
	// CapButt ends the stroke flush with the endpoint. It is the zero value.
	CapButt LineCap = iota
	CapRound
	// CapSquare extends the stroke by half its width.
	CapSquare
)

// String returns "butt", "round" or "square".
func (c LineCap) String() string {
	switch c {
	case CapRound:
		return "round"
	case CapSquare:
		return "square"
	}
	return "butt"
}

// DefaultMiterLimit is the SVG/CSS default miter limit.
const DefaultMiterLimit = 4

// StrokeStyle describes how a path is stroked. The zero value is not useful
// (zero width); build one with NewStrokeStyle, which sets the SVG/CSS
// defaults: miter join, butt caps, miter limit 4, no dashes.
type StrokeStyle struct {
	// Width is the stroke width in the local coordinate space. A width of 0
	// or less (or NaN) draws nothing: the Builder drops the command.
	Width float32
	// MiterLimit is the maximum ratio of miter length to Width for
	// JoinMiter. 0 means DefaultMiterLimit; values in (0, 1) are raised to 1
	// (the SVG minimum).
	MiterLimit float32
	Join       LineJoin
	Cap        LineCap
	// Dashes lists alternating dash and gap lengths, starting with a dash.
	// A nil or empty slice means a solid line. The Builder normalizes the
	// slice: an odd-length list is repeated twice to give an even length, as
	// in SVG; a list with a negative or non-finite entry, or whose total is
	// zero, is dropped (solid line).
	Dashes []float32
	// DashOffset is the distance into the dash pattern where the stroke
	// starts. It may be negative.
	DashOffset float32
}

// NewStrokeStyle returns a StrokeStyle of the given width with the SVG/CSS
// defaults.
func NewStrokeStyle(width float32) StrokeStyle {
	return StrokeStyle{Width: width, MiterLimit: DefaultMiterLimit}
}

// DrawImage draws the Src region of Image, scaled to fill Dst, with the given
// sampling. Src is in image pixels; an empty Src (the zero Rect) selects the
// whole image (the Builder fills it in). Parts of Src outside the image are
// clipped away together with the corresponding part of Dst. Dst is in the
// local coordinate space.
type DrawImage struct {
	Image    *Image
	Src, Dst Rect
	Sampling Sampling
}

// DrawShadow paints a CSS box-shadow for the box RRect. RRect is the box's
// border box exactly as written in CSS, without the shadow's offset or spread
// applied.
//
// Outer shadow (Inset false): the shadow shape is RRect translated by Offset
// and grown by Spread on every side (negative Spread shrinks it; corner radii
// follow the CSS spread rules, handled by the rasterizer), filled with Color
// and blurred with a Gaussian of standard deviation Blur/2. The result is
// painted only OUTSIDE RRect: CSS clips an outer shadow to the area outside
// the border box.
//
// Inset shadow (Inset true): the region inside RRect that lies outside the
// shape RRect translated by Offset and shrunk by Spread is filled with Color
// and blurred likewise. The result is painted only INSIDE RRect.
//
// Blur is the CSS blur radius (the Gaussian standard deviation is Blur/2); a
// Blur of 0 or less gives a sharp shadow. Spread may be negative.
type DrawShadow struct {
	RRect  RRect
	Offset Point
	Blur   float32
	Spread float32
	Color  Color
	Inset  bool
}

// PushClipRect intersects the clip with Rect (in the current coordinate
// space). Edges that are not pixel aligned are anti-aliased. Ended by
// PopClip.
type PushClipRect struct{ Rect Rect }

// PushClipRRect intersects the clip with a rounded rectangle (anti-aliased).
// Ended by PopClip.
type PushClipRRect struct{ RRect RRect }

// PushClipPath intersects the clip with the interior of Path under Rule
// (anti-aliased). Ended by PopClip.
type PushClipPath struct {
	Path Path
	Rule FillRule
}

// PopClip restores the clip that was current before the matching PushClip*.
type PopClip struct{}

// PushTransform pre-multiplies the current transform with Affine: points
// given after it are first transformed by Affine and then by the transform
// that was current before (current = current.Mul(Affine)). Ended by
// PopTransform.
type PushTransform struct{ Affine Affine }

// PopTransform restores the transform that was current before the matching
// PushTransform.
type PopTransform struct{}

// PushLayer starts a group: everything up to the matching PopLayer is
// rendered to an offscreen buffer (isolated: blend modes inside blend only
// with the group), then Filters are applied in order, the result is
// multiplied by Mask, and it is composited onto what is below with Opacity
// and Blend. The group is bounded by the clip that is current at PushLayer
// time (and by the mask's bounds).
//
// This is the order of CSS (filter, then clip and mask, then opacity and
// mix-blend-mode): Opacity and Mask scale the group's premultiplied pixels,
// Blend then composites them with the backdrop (W3C Compositing and
// Blending Level 1, on sRGB-encoded values, ADR-007), and the coverage of
// the clip interpolates between the backdrop and that result.
//
// Opacity is in [0, 1] (the Builder clamps). A layer with Opacity 1, SrcOver,
// no Filters and no Mask is still an isolated group.
type PushLayer struct {
	Opacity float32
	Blend   BlendMode
	Filters []Filter
	// Mask, when its List is not nil, multiplies the group's pixels by the
	// mask's coverage; see Mask.
	Mask Mask
}

// MaskMode selects how the pixels of a Mask become coverage.
type MaskMode uint8

const (
	// MaskAlpha takes the mask's alpha (CSS mask-mode: alpha). It is the
	// zero value.
	MaskAlpha MaskMode = iota
	// MaskLuminance takes the luminance of the mask's colors times their
	// alpha (CSS mask-mode: luminance, SVG <mask>): 0.2126 R + 0.7152 G +
	// 0.0722 B of the premultiplied sRGB-encoded values, like Chromium.
	MaskLuminance
)

// String returns "alpha" or "luminance".
func (m MaskMode) String() string {
	switch m {
	case MaskAlpha:
		return "alpha"
	case MaskLuminance:
		return "luminance"
	}
	return "MaskMode(?)"
}

// Mask is painted content used as the mask of a layer (PushLayer.Mask,
// Builder.PushMaskLayer): List is rendered as an isolated group onto a
// transparent surface, in the local coordinate space of the PushLayer (its
// root space is the space where the layer is pushed), and each pixel's
// coverage is its alpha or its luminance (Mode). The group's pixels are
// multiplied by that coverage: where the mask paints nothing, the layer
// shows nothing.
//
// Uses: the source of a COLR SrcIn composite masked by its backdrop, and
// CSS background-clip: text (the backgrounds in a layer masked by the
// element's glyph runs), CSS mask-image. Unlike a clip, a mask applies to
// the group as a whole, so overlapping content inside it is not blended
// twice at the mask's edges.
//
// Rasterizers render the mask list whole (it is not culled by the
// region), not clipped by the clips around the layer, over the pixels the
// layer composites. It is a backdrop root: backdrop filters in it read the
// mask's own pixels.
type Mask struct {
	List *DisplayList
	Mode MaskMode
}

// PopLayer composites the group started by the matching PushLayer.
type PopLayer struct{}

// BackdropFilter applies Filters to what has already been painted behind
// Shape, within Shape (CSS backdrop-filter), then paints the result over the
// same area. Shape is in the current coordinate space and acts as a rounded
// clip. Like Chromium, the filters read only the pixels inside the bounds
// of Shape: beyond them the backdrop is extended by mirroring it, so no
// color from outside the element bleeds in. A filter that spreads pixels
// (blur, drop shadow) reads the whole bounds; see DisplayList.BackdropReach.
// Inside a layer the backdrop is the layer's own content (a layer is a
// backdrop root).
type BackdropFilter struct {
	Shape   RRect
	Filters []Filter
}

// Marker is a placeholder: it paints nothing and stands for content
// recorded in another list (a layer), which Builder.AppendList replaces it
// with through its callback. Rasterizers ignore it. Its bounds are the clip
// active where it was recorded, so the blocks around it are kept.
type Marker struct{ ID uint32 }

func (FillRect) isCommand()       {}
func (FillRRect) isCommand()      {}
func (FillPath) isCommand()       {}
func (StrokePath) isCommand()     {}
func (DrawImage) isCommand()      {}
func (DrawShadow) isCommand()     {}
func (PushClipRect) isCommand()   {}
func (PushClipRRect) isCommand()  {}
func (PushClipPath) isCommand()   {}
func (PopClip) isCommand()        {}
func (PushTransform) isCommand()  {}
func (PopTransform) isCommand()   {}
func (PushLayer) isCommand()      {}
func (PopLayer) isCommand()       {}
func (BackdropFilter) isCommand() {}
func (Marker) isCommand()         {}

// Name implementations.
func (FillRect) Name() string       { return "FillRect" }
func (FillRRect) Name() string      { return "FillRRect" }
func (FillPath) Name() string       { return "FillPath" }
func (StrokePath) Name() string     { return "StrokePath" }
func (DrawImage) Name() string      { return "DrawImage" }
func (DrawShadow) Name() string     { return "DrawShadow" }
func (PushClipRect) Name() string   { return "PushClipRect" }
func (PushClipRRect) Name() string  { return "PushClipRRect" }
func (PushClipPath) Name() string   { return "PushClipPath" }
func (PopClip) Name() string        { return "PopClip" }
func (PushTransform) Name() string  { return "PushTransform" }
func (PopTransform) Name() string   { return "PopTransform" }
func (PushLayer) Name() string      { return "PushLayer" }
func (PopLayer) Name() string       { return "PopLayer" }
func (BackdropFilter) Name() string { return "BackdropFilter" }
func (Marker) Name() string         { return "Marker" }

// stackKind identifies the kind of push/pop pair.
type stackKind uint8

const (
	kindNone stackKind = iota
	kindClip
	kindTransform
	kindLayer
)

func (k stackKind) String() string {
	switch k {
	case kindClip:
		return "clip"
	case kindTransform:
		return "transform"
	case kindLayer:
		return "layer"
	}
	return "none"
}

// pushKind returns the stack kind of a push command and whether c is a push.
func pushKind(c Command) (stackKind, bool) {
	switch c.(type) {
	case PushClipRect, PushClipRRect, PushClipPath:
		return kindClip, true
	case PushTransform:
		return kindTransform, true
	case PushLayer:
		return kindLayer, true
	}
	return kindNone, false
}

// popKind returns the stack kind of a pop command and whether c is a pop.
func popKind(c Command) (stackKind, bool) {
	switch c.(type) {
	case PopClip:
		return kindClip, true
	case PopTransform:
		return kindTransform, true
	case PopLayer:
		return kindLayer, true
	}
	return kindNone, false
}
