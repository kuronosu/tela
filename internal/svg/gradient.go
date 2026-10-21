package svg

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Gradients (SVG 2, section 14.2, as Chromium paints them).
//
// A gradient is resolved for each shape that uses it, since
// objectBoundingBox coordinates depend on the shape's bounding box. Its
// attributes and stops follow the href chain (attributes from the nearest
// element that has them; coordinates only from gradients of the same
// kind; the stops of the first element that has any). The gradient space
// G (the bounding-box map, then gradientTransform) is folded into the
// paint whenever gfx can express the result in user space: always for
// linear gradients, and for radial ones when G maps circles to circles
// (or, without a focal point, to axis-aligned ellipses). Otherwise the
// shape is drawn in gradient space (see gradRes).
//
// Chromium interpolates SVG gradient stops on unpremultiplied colors, in
// sRGB. gfx interpolates premultiplied colors, so a segment whose two
// stops differ in both alpha and color gets intermediate stops (finishStops)
// that keep the premultiplied piecewise-linear ramp within half an 8-bit
// level of the unpremultiplied one.

// gradRes is a gradient resolved for one shape and paint.
type gradRes struct {
	// paint is in user space, or in gradient space when hasSpace.
	paint    gfx.Paint
	space    gfx.Affine // gradient space to user space
	hasSpace bool
	fillPath gfx.Path // the shape's path in gradient space (hasSpace)
	// tmpl is set when a stop's color is the root's currentColor: the
	// drawer rebuilds the stops with the color of its Options.
	tmpl []stopTmpl
}

// stopTmpl is a gradient stop before its color is final.
type stopTmpl struct {
	off     float32
	c       gfx.Color // with stop-opacity and the paint's opacity in A
	current bool      // c is the root's currentColor (c.A holds the opacity)
}

// gradChain returns the gradient e and the gradients it links to with
// href, in order, without cycles.
func (c *conv) gradChain(e *element) []*element {
	chain := []*element{e}
	for len(chain) < maxRefChain {
		href, ok := chain[len(chain)-1].href()
		if !ok {
			break
		}
		id, ok := parseIRI(href)
		if !ok {
			break
		}
		next := c.lookup(id)
		if next == nil || (next.tag != "linearGradient" && next.tag != "radialGradient") {
			break
		}
		for _, g := range chain {
			if g == next {
				return chain
			}
		}
		chain = append(chain, next)
	}
	return chain
}

// gradAttr returns attribute name from the first element of chain that
// has it; sameKind restricts the search to gradients of chain[0]'s kind.
func gradAttr(chain []*element, name string, sameKind bool) (string, bool) {
	for _, g := range chain {
		if sameKind && g.tag != chain[0].tag {
			continue
		}
		if v, ok := g.attr(name); ok {
			return v, true
		}
	}
	return "", false
}

// gradient resolves the gradient element g for sh. ok is false when the
// gradient is invalid for this shape (an objectBoundingBox gradient on a
// shape without width or height, or a singular transform), in which case
// the paint's fallback applies.
func (c *conv) gradient(g *element, sh *shape, alpha float32, vp viewport) (paintRes, bool) {
	chain := c.gradChain(g)
	// Stops.
	var tmpl []stopTmpl
	dynamic := false
	for _, ge := range chain {
		for _, s := range ge.children {
			if s.tag != "stop" {
				continue
			}
			st := c.styleOf(s)
			off := float32(0)
			if v, ok := s.attr("offset"); ok {
				if a, ok := parseAlpha(v); ok { // a number or a percentage, clamped
					off = a
				}
			}
			if n := len(tmpl); n > 0 {
				off = max(off, tmpl[n-1].off)
			}
			t := stopTmpl{off: off, c: st.stopColor.c}
			t.c.A *= st.stopOpacity * alpha
			if st.stopColor.current {
				if st.fromRoot&fromRootColor != 0 {
					t.current, dynamic = true, true
					t.c = gfx.Color{A: st.stopOpacity * alpha}
				} else {
					t.c = st.color
					t.c.A *= st.stopOpacity * alpha
				}
			}
			tmpl = append(tmpl, t)
		}
		if len(tmpl) > 0 {
			break
		}
	}
	switch len(tmpl) {
	case 0:
		return paintRes{kind: paintNone}, true // no stops: none
	case 1:
		return c.stopPaint(tmpl[0], alpha), true
	}

	bboxUnits := true
	if v, ok := gradAttr(chain, "gradientUnits", false); ok && v == "userSpaceOnUse" {
		bboxUnits = false
	}
	G := gfx.Identity
	if bboxUnits {
		b := sh.bbox
		if !(b.Width() > 0 && b.Height() > 0) {
			return paintRes{}, false
		}
		G = gfx.Affine{b.Width(), 0, 0, b.Height(), b.X0, b.Y0}
	}
	if v, ok := gradAttr(chain, "gradientTransform", false); ok {
		if t, ok := parseTransform(v); ok {
			G = G.Mul(t)
		}
	}
	if !G.IsFinite() || G.Det() == 0 {
		return paintRes{}, false
	}
	spread := gfx.SpreadPad
	if v, ok := gradAttr(chain, "spreadMethod", false); ok {
		switch v {
		case "reflect":
			spread = gfx.SpreadReflect
		case "repeat":
			spread = gfx.SpreadRepeat
		}
	}
	fontSize := c.styleOf(g).fonts()
	coord := func(name string, ax axis, def length) float32 {
		l := def
		if v, ok := gradAttr(chain, name, true); ok {
			if pl, ok := parseLength(v); ok {
				l = pl
			}
		}
		if bboxUnits {
			if l.u == uPercent {
				return l.v / 100
			}
			return l.resolve(ax, vp, fontSize) // a fraction, as a number
		}
		return l.resolve(ax, vp, fontSize)
	}
	stops := finishStops(tmpl, c.defaultRootColor)
	res := gradRes{}
	if dynamic {
		res.tmpl = tmpl
	}

	if g.tag == "linearGradient" {
		p1 := gfx.Pt(coord("x1", axisX, length{0, uPercent}), coord("y1", axisY, length{0, uPercent}))
		p2 := gfx.Pt(coord("x2", axisX, length{100, uPercent}), coord("y2", axisY, length{0, uPercent}))
		start, end, ok := linearInUserSpace(G, p1, p2)
		if !ok {
			// Equal end points: the last stop's color.
			return c.stopPaint(tmpl[len(tmpl)-1], alpha), true
		}
		res.paint = gfx.LinearGradient{Start: start, End: end, Stops: stops, Interp: gfx.InterpSRGB, Spread: spread}
		return paintRes{kind: paintURL, grad: &res, alpha: alpha}, true
	}

	cx := coord("cx", axisX, length{50, uPercent})
	cy := coord("cy", axisY, length{50, uPercent})
	r := coord("r", axisDiag, length{50, uPercent})
	fx, fy := cx, cy
	if v, ok := gradAttr(chain, "fx", true); ok {
		if _, ok := parseLength(v); ok {
			fx = coord("fx", axisX, length{})
		}
	}
	if v, ok := gradAttr(chain, "fy", true); ok {
		if _, ok := parseLength(v); ok {
			fy = coord("fy", axisY, length{})
		}
	}
	fr := coord("fr", axisDiag, length{0, uPercent})
	if !(r > 0) {
		// r = 0 paints the last stop's color; a negative r is an error.
		if r == 0 {
			return c.stopPaint(tmpl[len(tmpl)-1], alpha), true
		}
		return paintRes{}, false
	}
	fr = max(fr, 0)
	focal := fx != cx || fy != cy || fr != 0
	gradSpace := func(center, f gfx.Point, rad, frad float32) gfx.Paint {
		if !focal {
			return gfx.RadialGradient{Center: center, Radius: gfx.Pt(rad, rad), Stops: stops, Interp: gfx.InterpSRGB, Spread: spread}
		}
		return gfx.FocalGradient{Focal: f, FocalRadius: frad, Center: center, Radius: rad, Stops: stops, Interp: gfx.InterpSRGB, Spread: spread}
	}
	a, b, cc, d := G[0], G[1], G[2], G[3]
	switch {
	case (a == d && b == -cc) || (a == -d && b == cc):
		// A similarity: circles stay circles.
		s := float32(math.Sqrt(math.Abs(float64(G.Det()))))
		res.paint = gradSpace(G.Apply(gfx.Pt(cx, cy)), G.Apply(gfx.Pt(fx, fy)), float32(r*s), float32(fr*s))
	case b == 0 && cc == 0 && !focal:
		res.paint = gfx.RadialGradient{Center: G.Apply(gfx.Pt(cx, cy)), Radius: gfx.Pt(float32(r*abs32(a)), float32(r*abs32(d))),
			Stops: stops, Interp: gfx.InterpSRGB, Spread: spread}
	default:
		inv, ok := G.Invert()
		if !ok {
			return paintRes{}, false
		}
		res.paint = gradSpace(gfx.Pt(cx, cy), gfx.Pt(fx, fy), r, fr)
		res.space, res.hasSpace = G, true
		res.fillPath = sh.path.Transform(inv)
	}
	return paintRes{kind: paintURL, grad: &res, alpha: alpha}, true
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

// stopPaint returns the solid paint of a single stop. A stop with the
// root's currentColor is painted with the default root color: single-stop
// gradients do not follow Options.CurrentColor.
func (c *conv) stopPaint(t stopTmpl, alpha float32) paintRes {
	col := t.c
	if t.current {
		col = c.defaultRootColor
		col.A *= t.c.A
	}
	// The stop already carries the paint's opacity.
	return paintRes{kind: paintColor, color: col, alpha: 1}
}

// linearInUserSpace returns the start and end points, in user space, of
// the linear gradient from p1 to p2 in gradient space G. The gradient
// parameter is an affine function t(q) = k·(q - p1) of the gradient-space
// point q, with k = (p2 - p1) / |p2 - p1|²; in user space it is
// t(u) = (L⁻ᵀ k)·(u - G(p1)), L being G's linear part, which gfx
// expresses as the gradient from G(p1) to G(p1) + k'/|k'|². ok is false
// when p1 = p2.
func linearInUserSpace(G gfx.Affine, p1, p2 gfx.Point) (start, end gfx.Point, ok bool) {
	dx, dy := float64(p2.X)-float64(p1.X), float64(p2.Y)-float64(p1.Y)
	l2 := float64(dx*dx) + float64(dy*dy)
	if !(l2 > 0) {
		return gfx.Point{}, gfx.Point{}, false
	}
	kx, ky := dx/l2, dy/l2
	a, b, c, d := float64(G[0]), float64(G[1]), float64(G[2]), float64(G[3])
	det := float64(a*d) - float64(b*c)
	ux := (float64(d*kx) - float64(b*ky)) / det
	uy := (float64(a*ky) - float64(c*kx)) / det
	u2 := float64(ux*ux) + float64(uy*uy)
	if !(u2 > 0) || math.IsInf(u2, 0) {
		return gfx.Point{}, gfx.Point{}, false
	}
	start = G.Apply(p1)
	end = gfx.Pt(float32(float64(start.X)+ux/u2), float32(float64(start.Y)+uy/u2))
	return start, end, true
}

// finishStops returns the gfx stops of tmpl, with currentColor stops
// taking root, and intermediate stops that emulate unpremultiplied
// interpolation (see the package comment above).
func finishStops(tmpl []stopTmpl, root gfx.Color) []gfx.GradientStop {
	col := func(t stopTmpl) gfx.Color {
		if t.current {
			c := root
			c.A *= t.c.A
			return c
		}
		return t.c
	}
	out := make([]gfx.GradientStop, 0, len(tmpl))
	for i, t := range tmpl {
		c0 := col(t)
		out = append(out, gfx.GradientStop{Offset: t.off, Color: c0})
		if i+1 == len(tmpl) {
			break
		}
		n := tmpl[i+1]
		c1 := col(n)
		da := math.Abs(float64(c1.A - c0.A))
		dc := max(math.Abs(float64(c1.R-c0.R)), math.Abs(float64(c1.G-c0.G)), math.Abs(float64(c1.B-c0.B)))
		if !(n.off > t.off) || da == 0 || dc == 0 || c0.A <= 0 && c1.A <= 0 {
			continue
		}
		// The premultiplied value a(u)·c(u) is quadratic in u; k linear
		// pieces stay within da·dc/(4k²) of it: half a level for
		// k² ≥ 127.5·da·dc.
		k := int(math.Ceil(math.Sqrt(127.5 * float64(da*dc))))
		k = min(max(k, 1), 16)
		for j := 1; j < k; j++ {
			u := float32(j) / float32(k)
			out = append(out, gfx.GradientStop{
				Offset: t.off + float32((n.off-t.off)*u),
				Color: gfx.Color{
					R: c0.R + float32((c1.R-c0.R)*u),
					G: c0.G + float32((c1.G-c0.G)*u),
					B: c0.B + float32((c1.B-c0.B)*u),
					A: c0.A + float32((c1.A-c0.A)*u),
				},
			})
		}
	}
	return out
}
