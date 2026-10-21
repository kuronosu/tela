package svg

import (
	"github.com/kuronosu/tela/internal/gfx"
)

// Clip paths and masks (CSS Masking 1, as Chromium renders SVG).
//
// A clipPath is the union of its children's geometry (fill area under
// clip-rule; strokes, paints and opacity do not count), each in its own
// transform, mapped by the clipPath's transform and, for
// clipPathUnits="objectBoundingBox", by the bounding box of the clipped
// element. Like Blink, a clip path with one child becomes a path clip
// (PushClipPath); several children, or children with their own clip-path,
// become an alpha mask layer whose list fills each child in opaque white.
// A clip-path on the clipPath element intersects (parent).
//
// A mask is its content painted in the referencing element's user space
// (or its bounding box for maskContentUnits="objectBoundingBox"), clipped
// to the mask region (x, y, width, height in maskUnits, by default the
// bounding box grown by 10% on every side), used as luminance coverage,
// or alpha with mask-type: alpha (an ADR-074 mask layer).
//
// References to missing elements, or to elements of another kind, are
// ignored (the element is drawn without clip or mask), and so are
// references that would recurse. A clip path or mask that can show
// nothing (no children, an empty region, objectBoundingBox units on an
// element without width or height) hides the element.

// clipRes is a clip path resolved for one element.
type clipRes struct {
	isPath bool
	path   gfx.Path // isPath: in the element's local space
	rule   gfx.FillRule
	list   *gfx.DisplayList // otherwise: an alpha mask
	parent *clipRes         // the clipPath's own clip-path
}

// clipPiece is one child of a clipPath.
type clipPiece struct {
	path gfx.Path
	rule gfx.FillRule
	m    gfx.Affine
	clip *clipRes
}

// bboxTransform maps the unit square to b.
func bboxTransform(b gfx.Rect) gfx.Affine {
	return gfx.Affine{b.Width(), 0, 0, b.Height(), b.X0, b.Y0}
}

func validBBox(b gfx.Rect, ok bool) bool { return ok && b.Width() > 0 && b.Height() > 0 }

// clipFor resolves clip-path: url(#id) for an element with bounding box
// bbox. show is false when the element must not be drawn.
func (c *conv) clipFor(id string, bbox gfx.Rect, hasBBox bool) (res *clipRes, show bool) {
	e := c.lookup(id)
	if e == nil || e.tag != "clipPath" || c.active[e] {
		return nil, true
	}
	c.active[e] = true
	defer delete(c.active, e)
	st := c.styleOf(e)
	m := gfx.Identity
	if v, ok := e.attr("transform"); ok {
		if t, ok := parseTransform(v); ok {
			m = t
		}
	}
	if v, _ := e.attr("clipPathUnits"); v == "objectBoundingBox" {
		if !validBBox(bbox, hasBBox) {
			return nil, false
		}
		m = m.Mul(bboxTransform(bbox))
	}
	var pieces []clipPiece
	for _, ch := range e.children {
		cst := c.styleOf(ch)
		if cst.displayNone || cst.hidden {
			continue
		}
		t := gfx.Identity
		if v, ok := ch.attr("transform"); ok {
			if pt, ok := parseTransform(v); ok {
				t = pt
			}
		}
		var path gfx.Path
		rule := cst.clipRule
		switch ch.tag {
		case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
			path, _ = c.geometry(ch, cst, c.rootVP)
		case "use":
			// A use in a clipPath may reference a shape directly.
			href, _ := ch.href()
			tid, ok := parseIRI(href)
			target := c.lookup(tid)
			if !ok || target == nil {
				continue
			}
			switch target.tag {
			case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
			default:
				continue
			}
			tst := cascade(declsOf(target), cst, false)
			if tst.displayNone || tst.hidden {
				continue
			}
			x := c.lengthAttr(ch, "x", axisX, c.rootVP, cst, 0)
			y := c.lengthAttr(ch, "y", axisY, c.rootVP, cst, 0)
			t = t.Mul(gfx.Translate(x, y))
			if v, ok := target.attr("transform"); ok {
				if pt, ok := parseTransform(v); ok {
					t = t.Mul(pt)
				}
			}
			path, _ = c.geometry(target, &tst, c.rootVP)
			rule = tst.clipRule
		default:
			continue
		}
		if path.IsEmpty() || !path.IsFinite() {
			continue
		}
		if !c.countPoints(path) {
			return nil, false
		}
		p := clipPiece{path: path, rule: rule, m: m.Mul(t)}
		if cst.hasClipPath {
			cl, show := c.clipFor(cst.clipPath, path.Bounds(), true)
			if !show {
				continue
			}
			p.clip = cl
		}
		pieces = append(pieces, p)
	}
	if len(pieces) == 0 {
		return nil, false
	}
	res = &clipRes{}
	if st.hasClipPath {
		parent, show := c.clipFor(st.clipPath, bbox, hasBBox)
		if !show {
			return nil, false
		}
		res.parent = parent
	}
	if len(pieces) == 1 && pieces[0].clip == nil {
		p := pieces[0]
		res.isPath, res.path, res.rule = true, p.path, p.rule
		if p.m != gfx.Identity {
			res.path = p.path.Transform(p.m)
		}
		return res, true
	}
	b := gfx.NewBuilder()
	for _, p := range pieces {
		hasT := p.m != gfx.Identity
		if hasT {
			b.PushTransform(p.m)
		}
		pushClip(b, p.clip)
		b.FillPath(p.path, p.rule, gfx.White)
		popClip(b, p.clip)
		if hasT {
			b.PopTransform()
		}
	}
	res.list = b.Finish()
	if res.list.Len() == 0 {
		return nil, false
	}
	return res, true
}

// pushClip pushes the clip chain of cl (parents first).
func pushClip(b *gfx.Builder, cl *clipRes) {
	if cl == nil {
		return
	}
	pushClip(b, cl.parent)
	if cl.isPath {
		b.PushClipPath(cl.path, cl.rule)
	} else {
		b.PushMaskLayer(gfx.Mask{List: cl.list, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
	}
}

// popClip closes what pushClip opened.
func popClip(b *gfx.Builder, cl *clipRes) {
	if cl == nil {
		return
	}
	if cl.isPath {
		b.PopClip()
	} else {
		b.PopLayer()
	}
	popClip(b, cl.parent)
}

// maskRes is a mask resolved for one element.
type maskRes struct {
	content node             // the region clip and content transform around the children
	list    *gfx.DisplayList // content recorded with the default Options
	mode    gfx.MaskMode
	deps    uint8 // root dependencies of the content
}

// maskFor resolves mask: url(#id) for an element with bounding box bbox.
// show is false when the element must not be drawn.
func (c *conv) maskFor(id string, bbox gfx.Rect, hasBBox bool, vp viewport) (res *maskRes, show bool) {
	e := c.lookup(id)
	if e == nil || e.tag != "mask" || c.active[e] {
		return nil, true
	}
	c.active[e] = true
	defer delete(c.active, e)
	st := c.styleOf(e)
	region := func(name string, ax axis, def float32) float32 {
		v, ok := e.attr(name)
		l, valid := parseLength(v)
		if !ok || !valid {
			l = length{def, uPercent}
		}
		return l.resolve(ax, vp, st.fonts())
	}
	var r gfx.Rect
	if v, _ := e.attr("maskUnits"); v == "userSpaceOnUse" {
		x, y := region("x", axisX, -10), region("y", axisY, -10)
		r = gfx.RectXYWH(x, y, region("width", axisX, 120), region("height", axisY, 120))
	} else {
		if !validBBox(bbox, hasBBox) {
			return nil, false
		}
		frac := func(name string, def float32) float32 {
			v, ok := e.attr(name)
			l, valid := parseLength(v)
			if !ok || !valid {
				return def / 100
			}
			if l.u == uPercent {
				return l.v / 100
			}
			return l.v
		}
		x, y, w, h := frac("x", -10), frac("y", -10), frac("width", 120), frac("height", 120)
		r = gfx.RectXYWH(bbox.X0+float32(x*bbox.Width()), bbox.Y0+float32(y*bbox.Height()),
			float32(w*bbox.Width()), float32(h*bbox.Height()))
	}
	if r.Empty() {
		return nil, false
	}
	inner := node{kind: kindGroup, opacity: 1}
	if v, _ := e.attr("maskContentUnits"); v == "objectBoundingBox" {
		if !validBBox(bbox, hasBBox) {
			return nil, false
		}
		inner.t, inner.hasT = bboxTransform(bbox), true
	}
	inner.children = c.children(e, st, vp)
	if len(inner.children) == 0 {
		return nil, false
	}
	inner.deps = depsOf(&inner)
	res = &maskRes{
		content: node{kind: kindGroup, clipRect: r, hasClipRect: true, opacity: 1, children: []node{inner}, deps: inner.deps},
		mode:    gfx.MaskLuminance,
		deps:    inner.deps,
	}
	if st.maskTypeAlpha {
		res.mode = gfx.MaskAlpha
	}
	res.list = record(&res.content, &Options{}, c.defaultRootColor, c.defaultRootColor)
	if res.list.Len() == 0 {
		return nil, false
	}
	return res, true
}
