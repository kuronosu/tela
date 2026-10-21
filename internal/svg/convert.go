package svg

import (
	"github.com/kuronosu/tela/internal/gfx"
)

// Normalization: the source elements become an immutable render tree of
// nodes (groups and shapes) in which everything static is resolved once:
// styles, geometry, use instances, gradients for each shape's bounding
// box, clip paths and masks. What can change between draws (currentColor
// and the root overrides of Options) is kept as the root-dependency bits
// of style (fromRoot*) and resolved by the drawer.

type nodeKind uint8

const (
	kindGroup nodeKind = iota
	kindShape
)

// node is a group or a shape of the render tree.
type node struct {
	kind nodeKind
	// t maps the node's local space to its parent's (hasT).
	t    gfx.Affine
	hasT bool
	// clipRect is a viewport clip in the local space (nested svg, symbol,
	// mask region).
	clipRect    gfx.Rect
	hasClipRect bool
	opacity     float32
	clip        *clipRes
	mask        *maskRes
	children    []node // groups
	sh          *shape // shapes
	deps        uint8  // root dependencies of the subtree (fromRoot* bits)
}

// shape is a filled and stroked path.
type shape struct {
	path         gfx.Path
	bbox         gfx.Rect // fill bounding box, in the shape's local space
	fill, stroke paintRes
	rule         gfx.FillRule
	stroke0      gfx.StrokeStyle // Width resolved; may be 0
	hidden       bool            // visibility: hidden
	fromRoot     uint8           // root-dependency bits of the style
	color        gfx.Color       // computed color, unless fromRootColor
}

// paintRes is a fill or stroke paint resolved for a shape.
type paintRes struct {
	kind  paintKind // paintURL: grad holds the gradient
	color gfx.Color // paintColor
	alpha float32   // fill-opacity or stroke-opacity
	grad  *gradRes
}

// conv holds the state of one normalization.
type conv struct {
	ids              map[string]*element
	nodes, points    int
	err              error
	active           map[*element]bool // elements being instantiated or resolved (cycles)
	useDepth         int
	defaultRootColor gfx.Color
	rootVP           viewport
}

func (c *conv) count() bool {
	if c.nodes++; c.nodes > maxNodes {
		c.err = ErrTooComplex
	}
	return c.err == nil
}

// countPoints charges the points of a path to the document's budget.
func (c *conv) countPoints(p gfx.Path) bool {
	if c.points += len(p.Points()); c.points > maxPoints {
		c.err = ErrTooComplex
	}
	return c.err == nil
}

// styleOf returns the computed style of e at its place in the source tree
// (the context of gradient stops, clip paths and masks).
func (c *conv) styleOf(e *element) *style {
	if e.styled != nil {
		return e.styled
	}
	var s style
	if e.parent == nil {
		init := initialStyle()
		s = cascade(declsOf(e), &init, true)
	} else {
		s = cascade(declsOf(e), c.styleOf(e.parent), false)
	}
	e.styled = &s
	return &s
}

// lookup returns the element with the given id.
func (c *conv) lookup(id string) *element {
	if id == "" {
		return nil
	}
	return c.ids[id]
}

// children converts the rendered children of e.
func (c *conv) children(e *element, st *style, vp viewport) []node {
	var out []node
	for _, ch := range e.children {
		if c.err != nil {
			return nil
		}
		if n, ok := c.convert(ch, st, vp); ok {
			out = append(out, n)
		}
	}
	return out
}

// convert converts a rendered element (a container, a shape or use) with
// parent style parent. ok is false when it renders nothing.
func (c *conv) convert(e *element, parent *style, vp viewport) (node, bool) {
	if c.err != nil {
		return node{}, false
	}
	st := cascade(declsOf(e), parent, false)
	if st.displayNone {
		return node{}, false
	}
	var n node
	switch e.tag {
	case "g", "a":
		n = node{kind: kindGroup, children: c.children(e, &st, vp)}
		if len(n.children) == 0 {
			return node{}, false
		}
	case "svg":
		var ok bool
		if n, ok = c.viewportNode(e, e, &st, vp, nil); !ok {
			return node{}, false
		}
	case "use":
		var ok bool
		if n, ok = c.use(e, &st, vp); !ok {
			return node{}, false
		}
	case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
		var ok bool
		if n, ok = c.shapeNode(e, &st, vp); !ok {
			return node{}, false
		}
	default:
		return node{}, false
	}
	if e.tag != "svg" && e.tag != "use" {
		if v, ok := e.attr("transform"); ok {
			if t, ok := parseTransform(v); ok && t != gfx.Identity {
				n.t, n.hasT = t, true
			}
		}
	}
	if !c.count() {
		return node{}, false
	}
	return c.effects(n, &st, vp)
}

// effects sets the opacity, clip path and mask of n from st. ok is false
// when the clip or the mask hides everything.
func (c *conv) effects(n node, st *style, vp viewport) (node, bool) {
	n.opacity = st.opacity
	if st.hasClipPath || st.hasMask {
		bbox, hasBBox := bboxOf(&n)
		if st.hasClipPath {
			clip, show := c.clipFor(st.clipPath, bbox, hasBBox)
			if !show {
				return node{}, false
			}
			n.clip = clip
		}
		if st.hasMask {
			m, show := c.maskFor(st.mask, bbox, hasBBox, vp)
			if !show {
				return node{}, false
			}
			n.mask = m
		}
	}
	n.deps = depsOf(&n)
	return n, true
}

// depsOf returns the root dependencies of n's subtree.
func depsOf(n *node) uint8 {
	var d uint8
	if n.kind == kindShape {
		sh := n.sh
		d = sh.fromRoot & (fromRootFill | fromRootStroke | fromRootStrokeWidth)
		if sh.fromRoot&fromRootColor != 0 && (usesColor(&sh.fill) || usesColor(&sh.stroke)) {
			d |= fromRootColor
		}
		if (sh.fill.grad != nil && sh.fill.grad.tmpl != nil) || (sh.stroke.grad != nil && sh.stroke.grad.tmpl != nil) {
			d |= fromRootColor
		}
	}
	for i := range n.children {
		d |= n.children[i].deps
	}
	if n.mask != nil {
		d |= n.mask.deps
	}
	return d
}

func usesColor(p *paintRes) bool { return p.kind == paintCurrent }

// viewportNode converts a nested svg element, or the symbol (or svg)
// referenced by a use element (src is then the use, e the target): a
// viewport at x, y of size width x height, clipped unless overflow is
// visible, whose content is mapped from the viewBox.
func (c *conv) viewportNode(src, e *element, st *style, vp viewport, use *element) (node, bool) {
	x := c.lengthAttr(src, "x", axisX, vp, st, 0)
	y := c.lengthAttr(src, "y", axisY, vp, st, 0)
	w := c.lengthAttrPct(src, "width", axisX, vp, st)
	h := c.lengthAttrPct(src, "height", axisY, vp, st)
	if use != nil && e.tag == "svg" {
		// The use's width and height override the svg's.
		if _, ok := use.attr("width"); ok {
			w = c.lengthAttrPct(use, "width", axisX, vp, st)
		}
		if _, ok := use.attr("height"); ok {
			h = c.lengthAttrPct(use, "height", axisY, vp, st)
		}
	}
	if !(w > 0 && h > 0) {
		return node{}, false
	}
	inner := viewport{w, h}
	content := node{kind: kindGroup, opacity: 1}
	if v, ok := e.attr("viewBox"); ok {
		if vb, ok := parseViewBox(v); ok {
			if vb.Empty() {
				return node{}, false // a zero-size viewBox disables rendering
			}
			a := defaultAspect
			if pa, ok := e.attr("preserveAspectRatio"); ok {
				if pa, ok := parseAspect(pa); ok {
					a = pa
				}
			}
			content.t, content.hasT = viewBoxTransform(vb, gfx.Rect{X1: w, Y1: h}, a), true
			inner = viewport{vb.Width(), vb.Height()}
		}
	}
	content.children = c.children(e, st, inner)
	if len(content.children) == 0 || !c.count() {
		return node{}, false
	}
	content.deps = depsOf(&content)
	n := node{kind: kindGroup, children: []node{content}}
	if x != 0 || y != 0 {
		n.t, n.hasT = gfx.Translate(x, y), true
	}
	if !st.overflowVisible {
		n.clipRect, n.hasClipRect = gfx.Rect{X1: w, Y1: h}, true
	}
	return n, true
}

// use converts a use element: an instance of the referenced element in
// the use's context, translated by x, y. References to missing elements,
// to an ancestor, or cycles render nothing (as in browsers).
func (c *conv) use(e *element, st *style, vp viewport) (node, bool) {
	href, _ := e.href()
	id, ok := parseIRI(href)
	if !ok {
		return node{}, false
	}
	target := c.lookup(id)
	if target == nil || c.active[target] || c.useDepth >= maxRefChain {
		return node{}, false
	}
	for a := e; a != nil; a = a.parent {
		if a == target {
			return node{}, false
		}
	}
	c.active[target] = true
	c.useDepth++
	defer func() {
		delete(c.active, target)
		c.useDepth--
	}()
	var inst node
	switch target.tag {
	case "symbol":
		tst := cascade(declsOf(target), st, false)
		if tst.displayNone {
			return node{}, false
		}
		inst, ok = c.viewportNode(e, target, &tst, vp, e)
		if !ok {
			return node{}, false
		}
		// x and y belong to the use node below.
		if inst.hasT {
			inst.t, inst.hasT = gfx.Identity, false
		}
		if inst, ok = c.effects(inst, &tst, vp); !ok {
			return node{}, false
		}
	case "svg":
		tst := cascade(declsOf(target), st, false)
		if tst.displayNone {
			return node{}, false
		}
		inst, ok = c.viewportNode(target, target, &tst, vp, e)
		if !ok {
			return node{}, false
		}
		if inst, ok = c.effects(inst, &tst, vp); !ok {
			return node{}, false
		}
	default:
		inst, ok = c.convert(target, st, vp)
		if !ok {
			return node{}, false
		}
	}
	n := node{kind: kindGroup, children: []node{inst}}
	t := gfx.Identity
	if v, ok := e.attr("transform"); ok {
		if pt, ok := parseTransform(v); ok {
			t = pt
		}
	}
	x := c.lengthAttr(e, "x", axisX, vp, st, 0)
	y := c.lengthAttr(e, "y", axisY, vp, st, 0)
	if x != 0 || y != 0 {
		t = t.Mul(gfx.Translate(x, y))
	}
	if t != gfx.Identity {
		n.t, n.hasT = t, true
	}
	return n, true
}

// lengthAttr resolves a length attribute, or returns def when it is
// missing or invalid.
func (c *conv) lengthAttr(e *element, name string, ax axis, vp viewport, st *style, def float32) float32 {
	v, ok := e.attr(name)
	if !ok {
		return def
	}
	l, ok := parseLength(v)
	if !ok {
		return def
	}
	return l.resolve(ax, vp, st.fonts())
}

// lengthAttrPct is lengthAttr with 100% as the default (width and height
// of viewports).
func (c *conv) lengthAttrPct(e *element, name string, ax axis, vp viewport, st *style) float32 {
	def := vp.w
	if ax == axisY {
		def = vp.h
	}
	return c.lengthAttr(e, name, ax, vp, st, def)
}

// shapeNode converts a basic shape or a path.
func (c *conv) shapeNode(e *element, st *style, vp viewport) (node, bool) {
	path, ok := c.geometry(e, st, vp)
	if !ok || path.IsEmpty() || !path.IsFinite() || !c.countPoints(path) {
		return node{}, false
	}
	sh := &shape{
		path:     path,
		bbox:     path.Bounds(),
		rule:     st.fillRule,
		hidden:   st.hidden,
		fromRoot: st.fromRoot,
		color:    st.color,
	}
	sh.fill = c.paint(&st.fill, st.fillOpacity, sh, vp)
	sh.stroke = c.paint(&st.stroke, st.strokeOpacity, sh, vp)
	sh.stroke0 = strokeStyle(st, vp)
	return node{kind: kindShape, sh: sh}, true
}

// geometry returns the path of a shape element in user units.
func (c *conv) geometry(e *element, st *style, vp viewport) (gfx.Path, bool) {
	l := func(name string, ax axis) float32 { return c.lengthAttr(e, name, ax, vp, st, 0) }
	switch e.tag {
	case "path", "polyline", "polygon":
		// Parsed once per element: use instances share the path.
		if e.path == nil {
			var p gfx.Path
			if e.tag == "path" {
				d, _ := e.attr("d")
				p = parsePathData(d)
			} else {
				pts, _ := e.attr("points")
				p = pointsPath(pts, e.tag == "polygon")
			}
			e.path = &p
		}
		return *e.path, true
	case "rect":
		w, h := l("width", axisX), l("height", axisY)
		if !(w > 0 && h > 0) {
			return gfx.Path{}, false
		}
		rx, okx := c.autoLength(e, "rx", axisX, vp, st)
		ry, oky := c.autoLength(e, "ry", axisY, vp, st)
		switch {
		case !okx && !oky:
			rx, ry = 0, 0
		case !okx:
			rx = ry
		case !oky:
			ry = rx
		}
		rx, ry = min(rx, w/2), min(ry, h/2)
		return rectPath(l("x", axisX), l("y", axisY), w, h, rx, ry), true
	case "circle":
		r := l("r", axisDiag)
		if !(r > 0) {
			return gfx.Path{}, false
		}
		return ellipsePath(l("cx", axisX), l("cy", axisY), r, r), true
	case "ellipse":
		rx, okx := c.autoLength(e, "rx", axisX, vp, st)
		ry, oky := c.autoLength(e, "ry", axisY, vp, st)
		switch {
		case !okx && !oky:
			return gfx.Path{}, false
		case !okx:
			rx = ry
		case !oky:
			ry = rx
		}
		if !(rx > 0 && ry > 0) {
			return gfx.Path{}, false
		}
		return ellipsePath(l("cx", axisX), l("cy", axisY), rx, ry), true
	case "line":
		var p gfx.Path
		p.MoveTo(gfx.Pt(l("x1", axisX), l("y1", axisY)))
		p.LineTo(gfx.Pt(l("x2", axisX), l("y2", axisY)))
		return p, true
	}
	return gfx.Path{}, false
}

// autoLength resolves rx or ry: ok is false for auto (missing, invalid or
// negative).
func (c *conv) autoLength(e *element, name string, ax axis, vp viewport, st *style) (float32, bool) {
	v, ok := e.attr(name)
	if !ok {
		return 0, false
	}
	l, ok := parseLength(v)
	if !ok || l.v < 0 {
		return 0, false
	}
	return l.resolve(ax, vp, st.fonts()), true
}

// strokeStyle resolves the stroke properties of st.
func strokeStyle(st *style, vp viewport) gfx.StrokeStyle {
	s := gfx.StrokeStyle{
		Width:      st.strokeWidth.resolve(axisDiag, vp, st.fonts()),
		MiterLimit: st.miterlimit,
		Join:       st.linejoin,
		Cap:        st.linecap,
	}
	if len(st.dashes) > 0 {
		ds := make([]float32, len(st.dashes))
		var sum float32
		for i, l := range st.dashes {
			ds[i] = l.resolve(axisDiag, vp, st.fonts())
			sum += ds[i]
		}
		if sum > 0 {
			s.Dashes = ds
			s.DashOffset = st.dashOffset.resolve(axisDiag, vp, st.fonts())
		}
	}
	return s
}

// paint resolves a declared paint for sh.
func (c *conv) paint(pd *paintDecl, alpha float32, sh *shape, vp viewport) paintRes {
	switch pd.kind {
	case paintNone:
		return paintRes{kind: paintNone, alpha: alpha}
	case paintColor:
		return paintRes{kind: paintColor, color: pd.color, alpha: alpha}
	case paintCurrent:
		return paintRes{kind: paintCurrent, alpha: alpha}
	case paintURL:
		if g := c.lookup(pd.id); g != nil && (g.tag == "linearGradient" || g.tag == "radialGradient") {
			if r, ok := c.gradient(g, sh, alpha, vp); ok {
				return r
			}
		}
		if pd.hasFB {
			return paintRes{kind: pd.fallback, color: pd.fbColor, alpha: alpha}
		}
	}
	// none keeps its opacity: a root override may replace it.
	return paintRes{kind: paintNone, alpha: alpha}
}

// bboxOf returns the fill bounding box of n in its local space (before
// n.t). ok is false when n has no geometry.
func bboxOf(n *node) (gfx.Rect, bool) {
	var acc bboxAcc
	acc.add(n, gfx.Identity, false)
	return acc.r, acc.ok
}

type bboxAcc struct {
	r  gfx.Rect
	ok bool
}

func (a *bboxAcc) add(n *node, m gfx.Affine, hasM bool) {
	if n.kind == kindShape {
		r := n.sh.bbox
		if hasM {
			r = n.sh.path.Transform(m).Bounds()
		}
		if !a.ok {
			a.r, a.ok = r, true
			return
		}
		a.r = gfx.Rect{X0: min(a.r.X0, r.X0), Y0: min(a.r.Y0, r.Y0), X1: max(a.r.X1, r.X1), Y1: max(a.r.Y1, r.Y1)}
		return
	}
	for i := range n.children {
		ch := &n.children[i]
		cm, has := m, hasM
		if ch.hasT {
			cm, has = m.Mul(ch.t), true
		}
		a.add(ch, cm, has)
	}
}
