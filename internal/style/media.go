package style

import "github.com/kuronosu/tela/internal/gfx"

// Images, icons and masks (ADR-098): the SVG paint properties, object-fit
// and object-position, and mask layers.

// SVGPaintKind is the kind of an SVG paint (fill, stroke).
type SVGPaintKind uint8

const (
	// SVGPaintUnset leaves the document's own paint. It is the zero value.
	SVGPaintUnset SVGPaintKind = iota
	// SVGPaintNone paints nothing (fill-none, stroke-none).
	SVGPaintNone
	// SVGPaintColor paints Color.
	SVGPaintColor
	// SVGPaintCurrent paints the currentColor of the element that shows
	// the document (fill-current), faded by Color.A: like CSS, the
	// keyword is inherited, not the color, so a descendant with another
	// text color uses its own.
	SVGPaintCurrent
)

// SVGPaint is the value of fill or stroke.
type SVGPaint struct {
	Kind  SVGPaintKind
	Color gfx.Color
}

// Resolve returns the paint's color for the element's text color cur
// (meaningful for SVGPaintColor and SVGPaintCurrent).
func (p SVGPaint) Resolve(cur gfx.Color) gfx.Color {
	if p.Kind == SVGPaintCurrent {
		return cur.MulAlpha(p.Color.A)
	}
	return p.Color
}

// ObjectFit is the CSS object-fit property of a replaced element (Img).
type ObjectFit uint8

const (
	// ObjectFill stretches the image to the content box. It is the zero
	// value, the CSS initial value.
	ObjectFill ObjectFit = iota
	ObjectContain
	ObjectCover
	ObjectNone
	ObjectScaleDown
)

var objectFitNames = [...]string{"fill", "contain", "cover", "none", "scale-down"}

func (f ObjectFit) String() string { return enumName(objectFitNames[:], int(f)) }

// MaskMode is the CSS mask-mode of the mask layers.
type MaskMode uint8

const (
	// MaskMatchSource is the initial value: alpha for images and
	// gradients (luminance only for SVG <mask> elements, which Tela's
	// masks never reference).
	MaskMatchSource MaskMode = iota
	MaskModeAlpha
	MaskModeLuminance
)

// MaskComposite is the CSS mask-composite operator between a mask layer
// and the layers below it.
type MaskComposite uint8

const (
	// MaskAdd (source-over) is the initial value.
	MaskAdd MaskComposite = iota
	MaskSubtract
	MaskIntersect
	MaskExclude
)

// Mask is the CSS mask of an element: image layers (topmost first) sized,
// positioned and tiled like background layers (Origin is mask-origin,
// border-box by default), clipped to Clip unless NoClip, turned into
// coverage by Mode and combined by Composite. An element with layers is
// a stacking context. A nil Layers is no mask.
type Mask struct {
	Layers    []BackgroundLayer
	Mode      MaskMode
	Composite MaskComposite
	Clip      BackgroundBox
	NoClip    bool
}

// IsNone reports whether there is no mask.
func (m *Mask) IsNone() bool { return len(m.Layers) == 0 }
