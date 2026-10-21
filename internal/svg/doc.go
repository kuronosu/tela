// Package svg parses static SVG documents and draws them as gfx display
// list commands, for icons and SVG images (ADR-095).
//
// Parse decodes a document once (encoding/xml) and normalizes it into an
// immutable render tree: styles cascaded, lengths resolved, shapes turned
// into paths, use elements instantiated, gradients, clip paths and masks
// resolved for each element's bounding box. Draw then only walks the tree
// and records commands, so a document can be drawn many times, at any
// size and from any goroutine, cheaply. The per-draw inputs (Options:
// currentColor and author-CSS overrides on the root element, such as
// Tailwind's stroke-[1.5] or fill-current on an <svg>) do not need a new
// Parse.
//
// # Scope
//
// Supported: the svg root (width, height, viewBox, preserveAspectRatio,
// overflow); nested svg; path (the full path grammar), rect, circle,
// ellipse, line, polyline, polygon; g and a; defs, use (x, y, href and
// xlink:href) and symbol; linearGradient and radialGradient (units,
// gradientTransform, href inheritance, pad/reflect/repeat, fx/fy/fr);
// clipPath and mask; opacity, fill-opacity, stroke-opacity; fill, stroke
// and every stroke property; transform; display and visibility;
// presentation attributes and the style attribute; inheritance; CSS
// units; CSS Color 4 colors without the newer color functions.
//
// Ignored, like browsers ignore what they do not render: text, images,
// filters, patterns, markers, switch, foreignObject, <style> sheets,
// animation, scripts and external references, and unknown elements and
// attributes.
//
// Rendering follows Chromium where SVG leaves room (it is the pixel
// oracle of the tests): gradients interpolate unpremultiplied colors,
// invalid references to clip paths and masks are ignored, an
// objectBoundingBox gradient on a shape without width or height uses the
// paint's fallback, a document without viewBox but with a size is scaled
// to the box it is drawn into, and stroke-linejoin values Chromium does
// not support (miter-clip, arcs) are dropped.
package svg

import (
	"github.com/kuronosu/tela/internal/gfx"
)

// Doc is a parsed SVG document. It is immutable and safe for concurrent
// use.
type Doc struct {
	root node

	width, height float32 // intrinsic size; 0 when unknown
	viewBox       gfx.Rect
	hasViewBox    bool
	noRender      bool // a viewBox with a zero size, or display: none on the root
	aspect        aspect
	overflowVis   bool

	rootHasColor     bool
	rootColor        gfx.Color
	defaultRootColor gfx.Color
}

// Parse decodes and normalizes an SVG document. It returns an error for
// malformed XML, a root element that is not svg, and documents beyond the
// limits on size and nesting (ErrTooComplex). Unsupported content is
// skipped, not reported.
func Parse(data []byte) (doc *Doc, err error) {
	root, err := decode(data)
	if err != nil {
		return nil, err
	}
	c := &conv{ids: map[string]*element{}, active: map[*element]bool{}}
	indexIDs(root, c.ids)

	d := &Doc{aspect: defaultAspect}
	init := initialStyle()
	st := cascade(declsOf(root), &init, true)
	root.styled = &st
	rd := declsOf(root)
	if rd.has(pColor) && !rd.inherits(pColor) && !rd.color.current {
		d.rootHasColor, d.rootColor = true, rd.color.c
	}
	d.defaultRootColor = gfx.Black
	if d.rootHasColor {
		d.defaultRootColor = d.rootColor
	}
	c.defaultRootColor = d.defaultRootColor
	d.overflowVis = st.overflowVisible

	if v, ok := root.attr("width"); ok {
		if l, ok := parseLength(v); ok && l.isAbsolute() && l.v > 0 {
			d.width = l.resolve(axisX, viewport{}, st.fonts())
		}
	}
	if v, ok := root.attr("height"); ok {
		if l, ok := parseLength(v); ok && l.isAbsolute() && l.v > 0 {
			d.height = l.resolve(axisY, viewport{}, st.fonts())
		}
	}
	if v, ok := root.attr("viewBox"); ok {
		if vb, ok := parseViewBox(v); ok {
			d.viewBox, d.hasViewBox = vb, true
			d.noRender = vb.Empty()
		}
	}
	if v, ok := root.attr("preserveAspectRatio"); ok {
		if a, ok := parseAspect(v); ok {
			d.aspect = a
		}
	}
	// Percentages resolve against the viewBox, else the intrinsic size,
	// else the default object size (300 x 150).
	vp := viewport{300, 150}
	switch {
	case d.hasViewBox && !d.noRender:
		vp = viewport{d.viewBox.Width(), d.viewBox.Height()}
	default:
		if d.width > 0 {
			vp.w = d.width
		}
		if d.height > 0 {
			vp.h = d.height
		}
	}
	c.rootVP = vp
	if st.displayNone {
		d.noRender = true
	}
	if !d.noRender {
		n := node{kind: kindGroup, children: c.children(root, &st, vp)}
		if c.err != nil {
			return nil, c.err
		}
		if n, ok := c.effects(n, &st, vp); ok && len(n.children) > 0 {
			d.root = n
		} else {
			d.noRender = true
		}
		if c.err != nil {
			return nil, c.err
		}
	}
	return d, nil
}

// indexIDs maps ids to elements; the first element with an id wins, like
// getElementById.
func indexIDs(e *element, ids map[string]*element) {
	if id, ok := e.attr("id"); ok && id != "" {
		if _, dup := ids[id]; !dup {
			ids[id] = e
		}
	}
	for _, ch := range e.children {
		indexIDs(ch, ids)
	}
}

// Intrinsic returns the document's intrinsic width and height in px (0
// when the root's width or height is missing, a percentage or invalid)
// and its intrinsic aspect ratio width/height (from the viewBox, else
// from both sizes; 0 when unknown).
func (d *Doc) Intrinsic() (width, height, ratio float32) {
	if d.hasViewBox && d.viewBox.Width() > 0 && d.viewBox.Height() > 0 {
		ratio = d.viewBox.Width() / d.viewBox.Height()
	} else if d.width > 0 && d.height > 0 {
		ratio = d.width / d.height
	}
	return d.width, d.height, ratio
}

// Size returns the size to draw the document at when nothing else sets
// it: its width and height; a missing one from the other and the ratio;
// both missing, the viewBox's size; and the default object size 300 x 150
// (CSS) for what is still unknown.
func (d *Doc) Size() gfx.Size {
	w, h, ratio := d.Intrinsic()
	switch {
	case w > 0 && h > 0:
	case w > 0 && ratio > 0:
		h = w / ratio
	case h > 0 && ratio > 0:
		w = h * ratio
	case w == 0 && h == 0 && d.hasViewBox && d.viewBox.Width() > 0 && d.viewBox.Height() > 0:
		w, h = d.viewBox.Width(), d.viewBox.Height()
	}
	if !(w > 0) {
		w = 300
	}
	if !(h > 0) {
		h = 150
	}
	return gfx.Size{W: w, H: h}
}

// ViewBox returns the root's viewBox and whether it has a valid one.
func (d *Doc) ViewBox() (gfx.Rect, bool) { return d.viewBox, d.hasViewBox }

// Clips reports whether the document is clipped to the box it is drawn
// into: the root's overflow is not visible (the default; browsers always
// clip an SVG shown by <img>).
func (d *Doc) Clips() bool { return !d.overflowVis }

// UsesCurrentColor reports whether the drawing depends on the root's
// color (Options.CurrentColor or Root.Color) without overrides: some paint
// is currentColor where the color is inherited from the root.
func (d *Doc) UsesCurrentColor() bool { return d.root.deps&fromRootColor != 0 }

// Transform returns the transform from the document's user space to dst
// (logical px): the viewBox mapped by preserveAspectRatio; without a
// viewBox, the intrinsic size is treated as one (Chromium scales such
// documents too), and without either the user space is px at dst's
// origin.
func (d *Doc) Transform(dst gfx.Rect) gfx.Affine {
	switch {
	case d.hasViewBox && !d.viewBox.Empty():
		return viewBoxTransform(d.viewBox, dst, d.aspect)
	case d.width > 0 && d.height > 0:
		return viewBoxTransform(gfx.Rect{X1: d.width, Y1: d.height}, dst, d.aspect)
	}
	return gfx.Translate(dst.X0, dst.Y0)
}

// rootColor returns the color currentColor takes where it is inherited
// from the root.
func (d *Doc) rootColorFor(opts *Options) gfx.Color {
	switch {
	case opts.Root.HasColor:
		return opts.Root.Color
	case d.rootHasColor:
		return d.rootColor
	}
	return opts.CurrentColor
}

// Draw records the document into b, fitted into dst (logical px) by
// Transform and clipped to dst unless the root's overflow is visible.
func (d *Doc) Draw(b *gfx.Builder, dst gfx.Rect, opts Options) {
	if d.noRender || dst.Empty() || !dst.IsFinite() {
		return
	}
	dr := drawer{b: b, opts: &opts, rc: d.rootColorFor(&opts), defaultRC: d.defaultRootColor}
	clip := !d.overflowVis
	if clip {
		b.PushClipRect(dst)
	}
	b.PushTransform(d.Transform(dst))
	dr.node(&d.root)
	b.PopTransform()
	if clip {
		b.PopClip()
	}
}

// Record returns the document drawn in its user space (no viewport
// transform or clip), for callers that cache a list per Options and place
// it with DrawRecorded. The list is nil when nothing is drawn.
func (d *Doc) Record(opts Options) *gfx.DisplayList {
	if d.noRender {
		return nil
	}
	return record(&d.root, &opts, d.rootColorFor(&opts), d.defaultRootColor)
}

// DrawRecorded places a list from Record into b like Draw does.
func (d *Doc) DrawRecorded(b *gfx.Builder, dst gfx.Rect, list *gfx.DisplayList) {
	if list == nil || d.noRender || dst.Empty() || !dst.IsFinite() {
		return
	}
	clip := !d.overflowVis
	if clip {
		b.PushClipRect(dst)
	}
	b.PushTransform(d.Transform(dst))
	b.AppendList(list, nil)
	b.PopTransform()
	if clip {
		b.PopClip()
	}
}
