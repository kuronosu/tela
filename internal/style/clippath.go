package style

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// ClipShape is the basic shape of a clip-path (CSS Shapes 1, section 3.1).
type ClipShape uint8

const (
	// ClipNone is clip-path: none. It is the zero value.
	ClipNone ClipShape = iota
	ClipInset
	ClipCircle
	ClipEllipse
	ClipPolygon
)

var clipShapeNames = [...]string{"none", "inset", "circle", "ellipse", "polygon"}

func (s ClipShape) String() string { return enumName(clipShapeNames[:], int(s)) }

// ShapeRadius is how a radius of circle() or ellipse() is given: a length
// (in ClipPath.Values) or a keyword.
type ShapeRadius uint8

const (
	// RadiusLength takes the radius from Values. It is the zero value.
	RadiusLength ShapeRadius = iota
	// RadiusClosestSide is the distance from the center to the closest
	// side of the reference box (the default).
	RadiusClosestSide
	// RadiusFarthestSide is the distance to the farthest side.
	RadiusFarthestSide
)

// ClipPath is the clip-path of a box: a basic shape relative to its border
// box (the default reference box), written as the arbitrary property
// [clip-path:circle(50%)] (ADR-079). The zero value is none.
//
// Values holds the shape's lengths in a fixed layout, so that two clip
// paths of the same shape and length interpolate value by value (a CSS
// transition between them):
//
//   - inset: top, right, bottom, left, then the corner radii top-left x
//     and y, top-right x and y, bottom-right x and y, bottom-left x and y
//     (12 values); percentages of the radii refer to the inset rectangle;
//   - circle: r, cx, cy (r used when Radius[0] is RadiusLength; a
//     percentage of r refers to sqrt(w² + h²) / sqrt(2));
//   - ellipse: rx, ry, cx, cy;
//   - polygon: x0, y0, x1, y1, ... with Rule.
//
// Horizontal percentages refer to the width of the border box and
// vertical ones to its height.
type ClipPath struct {
	Shape  ClipShape
	Values []LP
	// Radius are the radius keywords of circle() ([0]) and ellipse() (rx,
	// ry).
	Radius [2]ShapeRadius
	// Rule is the fill rule of a polygon.
	Rule gfx.FillRule
}

// IsNone reports clip-path: none.
func (c *ClipPath) IsNone() bool { return c.Shape == ClipNone }

// Equal reports whether c and d are the same clip path.
func (c *ClipPath) Equal(d *ClipPath) bool {
	if c.Shape != d.Shape || c.Radius != d.Radius || c.Rule != d.Rule || len(c.Values) != len(d.Values) {
		return false
	}
	for i := range c.Values {
		if c.Values[i] != d.Values[i] {
			return false
		}
	}
	return true
}

// Interpolable reports whether a transition between c and d interpolates
// its values (same shape, same number of values, same keywords and fill
// rule) rather than switching halfway.
func (c *ClipPath) Interpolable(d *ClipPath) bool {
	return c.Shape != ClipNone && c.Shape == d.Shape && c.Radius == d.Radius && c.Rule == d.Rule &&
		len(c.Values) == len(d.Values)
}

// ClipGeom is a clip path resolved against a reference box: a rounded
// rectangle (inset(), and circle() and ellipse() as rounded rectangles
// whose radii are half their sides) or a polygon.
type ClipGeom struct {
	RRect gfx.RRect
	// Poly is the polygon's vertices when IsPoly; Rule its fill rule.
	Poly   []gfx.Point
	Rule   gfx.FillRule
	IsPoly bool
}

// Resolve returns the shape of c for a reference box (the border box).
// It must not be called on a ClipNone.
func (c *ClipPath) Resolve(box gfx.Rect) ClipGeom {
	w, h := box.Width(), box.Height()
	v := func(i int, basis float32) float32 {
		if i < len(c.Values) {
			return c.Values[i].Resolve(basis)
		}
		return 0
	}
	switch c.Shape {
	case ClipInset:
		t, r, b, l := v(0, h), v(1, w), v(2, h), v(3, w)
		// A pair of insets larger than the box is reduced proportionally
		// to the box (CSS Shapes 1, inset()).
		if s := l + r; s > w && s > 0 {
			l, r = float32(float64(l)*float64(w)/float64(s)), float32(float64(r)*float64(w)/float64(s))
		}
		if s := t + b; s > h && s > 0 {
			t, b = float32(float64(t)*float64(h)/float64(s)), float32(float64(b)*float64(h)/float64(s))
		}
		rc := gfx.Rect{X0: box.X0 + l, Y0: box.Y0 + t, X1: box.X1 - r, Y1: box.Y1 - b}
		if rc.X1 < rc.X0 {
			rc.X1 = rc.X0
		}
		if rc.Y1 < rc.Y0 {
			rc.Y1 = rc.Y0
		}
		iw, ih := rc.Width(), rc.Height()
		var rd [4]gfx.Point
		for k := range rd {
			rd[k] = gfx.Point{X: max(v(4+2*k, iw), 0), Y: max(v(5+2*k, ih), 0)}
		}
		return ClipGeom{RRect: gfx.RRect{Rect: rc, Radii: rd}.Normalize()}
	case ClipCircle:
		cx, cy := box.X0+v(1, w), box.Y0+v(2, h)
		var rad float32
		switch c.Radius[0] {
		case RadiusLength:
			ref := float32(math.Sqrt(float64(float64(w)*float64(w))+float64(float64(h)*float64(h))) / math.Sqrt2)
			rad = v(0, ref)
		case RadiusFarthestSide:
			rad = max(abs32(cx-box.X0), abs32(box.X1-cx), abs32(cy-box.Y0), abs32(box.Y1-cy))
		default:
			rad = min(abs32(cx-box.X0), abs32(box.X1-cx), abs32(cy-box.Y0), abs32(box.Y1-cy))
		}
		return ClipGeom{RRect: ellipseRRect(cx, cy, rad, rad)}
	case ClipEllipse:
		cx, cy := box.X0+v(2, w), box.Y0+v(3, h)
		side := func(k ShapeRadius, i int, lo, hi, c, basis float32) float32 {
			switch k {
			case RadiusLength:
				return v(i, basis)
			case RadiusFarthestSide:
				return max(abs32(c-lo), abs32(hi-c))
			}
			return min(abs32(c-lo), abs32(hi-c))
		}
		rx := side(c.Radius[0], 0, box.X0, box.X1, cx, w)
		ry := side(c.Radius[1], 1, box.Y0, box.Y1, cy, h)
		return ClipGeom{RRect: ellipseRRect(cx, cy, rx, ry)}
	case ClipPolygon:
		pts := make([]gfx.Point, len(c.Values)/2)
		for i := range pts {
			pts[i] = gfx.Point{X: box.X0 + v(2*i, w), Y: box.Y0 + v(2*i+1, h)}
		}
		return ClipGeom{Poly: pts, Rule: c.Rule, IsPoly: true}
	}
	return ClipGeom{RRect: gfx.RRect{Rect: box}}
}

// ellipseRRect returns the ellipse of center (cx, cy) and radii rx, ry as
// a rounded rectangle; a negative radius is an empty shape.
func ellipseRRect(cx, cy, rx, ry float32) gfx.RRect {
	rx, ry = max(rx, 0), max(ry, 0)
	r := gfx.Point{X: rx, Y: ry}
	return gfx.RRect{Rect: gfx.Rect{X0: cx - rx, Y0: cy - ry, X1: cx + rx, Y1: cy + ry}, Radii: [4]gfx.Point{r, r, r, r}}.Normalize()
}

// Bounds returns the bounding rectangle of the shape.
func (g *ClipGeom) Bounds() gfx.Rect {
	if !g.IsPoly {
		return g.RRect.Rect
	}
	if len(g.Poly) == 0 {
		return gfx.Rect{}
	}
	r := gfx.Rect{X0: g.Poly[0].X, Y0: g.Poly[0].Y, X1: g.Poly[0].X, Y1: g.Poly[0].Y}
	for _, p := range g.Poly[1:] {
		r.X0, r.Y0 = min(r.X0, p.X), min(r.Y0, p.Y)
		r.X1, r.Y1 = max(r.X1, p.X), max(r.Y1, p.Y)
	}
	return r
}

// Path returns the polygon as a closed path.
func (g *ClipGeom) Path() gfx.Path {
	var p gfx.Path
	for i, q := range g.Poly {
		if i == 0 {
			p.MoveTo(q)
		} else {
			p.LineTo(q)
		}
	}
	if len(g.Poly) > 0 {
		p.Close()
	}
	return p
}

// ---------------------------------------------------------------------
// Parsing: [clip-path:<value>] (the only arbitrary property Tela accepts).

// clipLen is a parsed length of a clip path, evaluated when the style is
// computed (em, rem, vw, vh).
type clipLen = lenExpr

// parseClipPath parses the value of [clip-path:...], with underscores
// already turned into spaces.
func parseClipPath(v string) (Op, error) {
	v = strings.TrimSpace(v)
	if v == "none" {
		return op(func(e *env) { e.s.ClipPath = ClipPath{} }), nil
	}
	if strings.Contains(v, "var(") {
		return Op{}, unsupportedErr("CSS variables are not supported; write the shape with literal values")
	}
	// An optional reference box before or after the shape: only the
	// border box (the default) is supported.
	fields := fieldsTop(v)
	var fn string
	for _, f := range fields {
		switch {
		case f == "border-box":
		case strings.HasSuffix(f, "-box"):
			return Op{}, unsupportedErr(fmt.Sprintf("the reference box %s is not supported; clip paths are relative to the border box", f))
		case fn != "":
			return Op{}, fmt.Errorf("invalid clip path %q; use one shape such as circle(50%%)", v)
		default:
			fn = f
		}
	}
	i := strings.IndexByte(fn, '(')
	if i <= 0 || !strings.HasSuffix(fn, ")") {
		if strings.HasPrefix(fn, "url(") {
			return Op{}, unsupportedErr("SVG url() clip paths are not supported; use inset(), circle(), ellipse() or polygon()")
		}
		return Op{}, fmt.Errorf("invalid clip path %q; use inset(), circle(), ellipse() or polygon(), e.g. [clip-path:circle(50%%)]", v)
	}
	name, args := fn[:i], strings.TrimSpace(fn[i+1:len(fn)-1])
	var (
		shape  ClipShape
		exprs  []clipLen
		radius [2]ShapeRadius
		rule   gfx.FillRule
		err    error
	)
	switch name {
	case "inset":
		shape = ClipInset
		exprs, err = parseInsetArgs(args)
	case "circle":
		shape = ClipCircle
		exprs, radius, err = parseEllipseArgs(args, 1)
	case "ellipse":
		shape = ClipEllipse
		exprs, radius, err = parseEllipseArgs(args, 2)
	case "polygon":
		shape = ClipPolygon
		exprs, rule, err = parsePolygonArgs(args)
	case "path", "rect", "xywh", "shape", "url":
		return Op{}, unsupportedErr(name + "() clip paths are not supported; use inset(), circle(), ellipse() or polygon()")
	default:
		return Op{}, fmt.Errorf("unknown clip path shape %s(); use inset(), circle(), ellipse() or polygon()", name)
	}
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) {
		vals := make([]LP, len(exprs))
		for k := range exprs {
			vals[k] = e.lp(&exprs[k])
		}
		e.s.ClipPath = ClipPath{Shape: shape, Values: vals, Radius: radius, Rule: rule}
	}), nil
}

func clipLength(s string) (clipLen, error) {
	x, err := parseLengthExpr(s, true)
	if err != nil {
		return clipLen{}, fmt.Errorf("invalid length %q in the clip path", s)
	}
	return x, nil
}

// boxShorthand expands 1 to 4 values to top, right, bottom, left.
func boxShorthand[T any](v []T) [4]T {
	switch len(v) {
	case 1:
		return [4]T{v[0], v[0], v[0], v[0]}
	case 2:
		return [4]T{v[0], v[1], v[0], v[1]}
	case 3:
		return [4]T{v[0], v[1], v[2], v[1]}
	}
	return [4]T{v[0], v[1], v[2], v[3]}
}

// parseInsetArgs parses "<lp>{1,4} [round <border-radius>]".
func parseInsetArgs(args string) ([]clipLen, error) {
	f := fieldsTop(args)
	k := len(f)
	for i, w := range f {
		if w == "round" {
			k = i
			break
		}
	}
	if k == 0 || k > 4 {
		return nil, errors.New("inset() takes one to four lengths, then optionally round and the radii, e.g. inset(0_round_1rem)")
	}
	var ins []clipLen
	for _, w := range f[:k] {
		x, err := clipLength(w)
		if err != nil {
			return nil, err
		}
		ins = append(ins, x)
	}
	sides := boxShorthand(ins)
	out := append([]clipLen(nil), sides[0], sides[1], sides[2], sides[3])
	radii := make([]clipLen, 8)
	if k < len(f) {
		rf := f[k+1:]
		if len(rf) == 0 {
			return nil, errors.New("round needs one to four radii, e.g. inset(0_round_1rem)")
		}
		var hs, vs []clipLen
		cur := &hs
		for _, w := range rf {
			if w == "/" {
				if cur == &vs || len(hs) == 0 {
					return nil, errors.New("invalid radii after round")
				}
				cur = &vs
				continue
			}
			x, err := clipLength(w)
			if err != nil {
				return nil, err
			}
			*cur = append(*cur, x)
		}
		if len(hs) > 4 || len(vs) > 4 || (cur == &vs && len(vs) == 0) {
			return nil, errors.New("round takes one to four radii, optionally followed by / and one to four more")
		}
		if len(vs) == 0 {
			vs = hs
		}
		// Corners: top-left, top-right, bottom-right, bottom-left.
		h4, v4 := boxShorthand(hs), boxShorthand(vs)
		for c := range 4 {
			radii[2*c], radii[2*c+1] = h4[c], v4[c]
		}
	}
	return append(out, radii...), nil
}

// parseEllipseArgs parses "[<radius>{n}] [at <position>]" for circle()
// (n = 1) and ellipse() (n = 2).
func parseEllipseArgs(args string, n int) ([]clipLen, [2]ShapeRadius, error) {
	var kw [2]ShapeRadius
	f := fieldsTop(args)
	at := len(f)
	for i, w := range f {
		if w == "at" {
			at = i
			break
		}
	}
	rs := f[:at]
	if len(rs) != 0 && len(rs) != n {
		if n == 1 {
			return nil, kw, errors.New("circle() takes one radius, e.g. circle(50%_at_50%_50%)")
		}
		return nil, kw, errors.New("ellipse() takes two radii or none, e.g. ellipse(50%_30%_at_center)")
	}
	out := make([]clipLen, n+2)
	if len(rs) == 0 {
		kw = [2]ShapeRadius{RadiusClosestSide, RadiusClosestSide}
	}
	for i, w := range rs {
		switch w {
		case "closest-side":
			kw[i] = RadiusClosestSide
		case "farthest-side":
			kw[i] = RadiusFarthestSide
		default:
			x, err := clipLength(w)
			if err != nil {
				return nil, kw, err
			}
			out[i] = x
		}
	}
	if n == 1 {
		kw[1] = 0
	}
	out[n], out[n+1] = exprPct(50), exprPct(50)
	if at < len(f) {
		pos := f[at+1:]
		if len(pos) == 0 {
			return nil, kw, errors.New("at needs a position, e.g. circle(40%_at_top_left)")
		}
		x, y, err := parsePositionPair(pos)
		if err != nil {
			return nil, kw, err
		}
		out[n], out[n+1] = lenExpr{px: x.Px, pct: x.Pct}, lenExpr{px: y.Px, pct: y.Pct}
	}
	return out, kw, nil
}

// parsePolygonArgs parses "[nonzero | evenodd,] <lp> <lp>, ...".
func parsePolygonArgs(args string) ([]clipLen, gfx.FillRule, error) {
	items := splitTop(args, ',')
	rule := gfx.NonZero
	switch strings.TrimSpace(items[0]) {
	case "nonzero":
		items = items[1:]
	case "evenodd":
		rule, items = gfx.EvenOdd, items[1:]
	}
	if len(items) == 0 || (len(items) == 1 && strings.TrimSpace(items[0]) == "") {
		return nil, rule, errors.New("polygon() needs points, e.g. polygon(50%_0,100%_100%,0_100%)")
	}
	out := make([]clipLen, 0, 2*len(items))
	for _, it := range items {
		f := fieldsTop(strings.TrimSpace(it))
		if len(f) != 2 {
			return nil, rule, fmt.Errorf("polygon() point %q needs two lengths, x and y", strings.TrimSpace(it))
		}
		for _, w := range f {
			x, err := clipLength(w)
			if err != nil {
				return nil, rule, err
			}
			out = append(out, x)
		}
	}
	return out, rule, nil
}
