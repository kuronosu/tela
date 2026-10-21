package svg

import (
	"math"

	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gfx"
)

// Geometry: path data and the basic shapes, converted to gfx paths in
// user units. Arcs, circles, ellipses and rounded corners become cubic
// Beziers of at most 45 degrees each (error under 5e-6 of the radius,
// invisible at any zoom an icon is drawn at). The arithmetic is
// fusion-safe (ADR-041): every product feeding an addition is rounded
// explicitly, and the trigonometry comes from internal/fmath.

// parsePathData parses SVG path data (SVG 2, section 9.3.9). On an error
// the path keeps the segments before it, as browsers render it.
func parsePathData(s string) gfx.Path {
	var p gfx.Path
	var cur, start, ctrl gfx.Point // ctrl: last control point, for S and T
	var prev byte                  // previous command, for S and T
	var cmd byte
	var a [7]float64
	needArgs := false // a command letter was read and its arguments not yet
	i := skipSpace(s, 0)
	for i < len(s) {
		c := s[i]
		if isCommand(c) {
			if needArgs {
				return p // a command without arguments is an error
			}
			cmd = c
			needArgs = cmd != 'Z' && cmd != 'z'
			i = skipSpace(s, i+1)
			if p.IsEmpty() && cmd != 'M' && cmd != 'm' {
				return p
			}
			if cmd == 'Z' || cmd == 'z' {
				p.Close()
				cur, prev = start, cmd
				continue
			}
		} else if cmd == 0 || cmd == 'Z' || cmd == 'z' {
			return p
		}
		n := argCount(cmd)
		j := i
		ok := true
		for k := 0; k < n; k++ {
			if k > 0 {
				j = skipSep(s, j)
			}
			if (cmd == 'A' || cmd == 'a') && (k == 3 || k == 4) {
				if j < len(s) && (s[j] == '0' || s[j] == '1') {
					a[k] = float64(s[j] - '0')
					j++
					continue
				}
				ok = false
				break
			}
			var v float64
			v, j, ok = scanNumber(s, j)
			if !ok {
				break
			}
			a[k] = v
		}
		if !ok {
			return p
		}
		i = skipSpace(s, j)
		stop := false
		if i < len(s) && s[i] == ',' {
			// A comma separates numbers, never a number and a command:
			// the path ends after this segment.
			i = skipSpace(s, i+1)
			stop = i == len(s) || isCommand(s[i])
		}
		needArgs = false
		rel := cmd >= 'a'
		pt := func(x, y float64) gfx.Point {
			q := gfx.Pt(float32(x), float32(y))
			if rel {
				q = q.Add(cur)
			}
			return q
		}
		switch cmd {
		case 'M', 'm':
			cur = pt(a[0], a[1])
			start = cur
			p.MoveTo(cur)
			// Further pairs are implicit line-tos.
			if cmd == 'M' {
				cmd = 'L'
			} else {
				cmd = 'l'
			}
		case 'L', 'l':
			cur = pt(a[0], a[1])
			p.LineTo(cur)
		case 'H', 'h':
			x := float32(a[0])
			if rel {
				x += cur.X
			}
			cur = gfx.Pt(x, cur.Y)
			p.LineTo(cur)
		case 'V', 'v':
			y := float32(a[0])
			if rel {
				y += cur.Y
			}
			cur = gfx.Pt(cur.X, y)
			p.LineTo(cur)
		case 'C', 'c':
			c1, c2, e := pt(a[0], a[1]), pt(a[2], a[3]), pt(a[4], a[5])
			p.CubicTo(c1, c2, e)
			cur, ctrl = e, c2
		case 'S', 's':
			c1 := cur
			if prev == 'C' || prev == 'c' || prev == 'S' || prev == 's' {
				c1 = reflect(ctrl, cur)
			}
			c2, e := pt(a[0], a[1]), pt(a[2], a[3])
			p.CubicTo(c1, c2, e)
			cur, ctrl = e, c2
		case 'Q', 'q':
			c1, e := pt(a[0], a[1]), pt(a[2], a[3])
			p.QuadTo(c1, e)
			cur, ctrl = e, c1
		case 'T', 't':
			c1 := cur
			if prev == 'Q' || prev == 'q' || prev == 'T' || prev == 't' {
				c1 = reflect(ctrl, cur)
			}
			e := pt(a[0], a[1])
			p.QuadTo(c1, e)
			cur, ctrl = e, c1
		case 'A', 'a':
			e := pt(a[5], a[6])
			appendArc(&p, cur, a[0], a[1], a[2], a[3] != 0, a[4] != 0, e)
			cur = e
		}
		prev = cmd
		if stop {
			return p
		}
	}
	return p
}

func isCommand(c byte) bool {
	switch c {
	case 'M', 'm', 'Z', 'z', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't', 'A', 'a':
		return true
	}
	return false
}

func argCount(cmd byte) int {
	switch cmd {
	case 'M', 'm', 'L', 'l', 'T', 't':
		return 2
	case 'H', 'h', 'V', 'v':
		return 1
	case 'C', 'c':
		return 6
	case 'S', 's', 'Q', 'q':
		return 4
	case 'A', 'a':
		return 7
	}
	return 0
}

// reflect returns the reflection of c about p.
func reflect(c, p gfx.Point) gfx.Point {
	return gfx.Pt(float32(2*p.X)-c.X, float32(2*p.Y)-c.Y)
}

// appendArc appends the SVG elliptical arc from p0 to p1 with radii
// (rx, ry), x-axis rotation phi (degrees) and the large-arc and sweep
// flags, converted to center parameterization as in SVG 2 (appendix
// B.2.4 and B.2.5: out-of-range radii are scaled up, zero radii give a
// line, equal endpoints give nothing).
func appendArc(p *gfx.Path, p0 gfx.Point, rx, ry, phi float64, large, sweep bool, p1 gfx.Point) {
	if p0 == p1 {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		p.LineTo(p1)
		return
	}
	x0, y0, x1, y1 := float64(p0.X), float64(p0.Y), float64(p1.X), float64(p1.Y)
	sinPhi, cosPhi := sinCosDeg(float32(phi))
	sp, cp := float64(sinPhi), float64(cosPhi)
	dx, dy := (x0-x1)/2, (y0-y1)/2
	x1p := float64(cp*dx) + float64(sp*dy)
	y1p := float64(cp*dy) - float64(sp*dx)
	x1p2, y1p2 := float64(x1p*x1p), float64(y1p*y1p)
	if lam := x1p2/float64(rx*rx) + y1p2/float64(ry*ry); lam > 1 {
		s := math.Sqrt(lam)
		rx, ry = float64(rx*s), float64(ry*s)
	}
	rx2, ry2 := float64(rx*rx), float64(ry*ry)
	num := float64(rx2*ry2) - float64(rx2*y1p2) - float64(ry2*x1p2)
	den := float64(rx2*y1p2) + float64(ry2*x1p2)
	coef := 0.0
	if num > 0 && den > 0 {
		coef = math.Sqrt(num / den)
	}
	if large == sweep {
		coef = -coef
	}
	cxp := float64(float64(coef*rx)*y1p) / ry
	cyp := -float64(float64(coef*ry)*x1p) / rx
	cx := float64(cp*cxp) - float64(sp*cyp) + (x0+x1)/2
	cy := float64(sp*cxp) + float64(cp*cyp) + (y0+y1)/2
	t1 := fmath.Atan2((y1p-cyp)/ry, (x1p-cxp)/rx)
	t2 := fmath.Atan2((-y1p-cyp)/ry, (-x1p-cxp)/rx)
	dt := t2 - t1
	if sweep && dt < 0 {
		dt += 2 * math.Pi
	} else if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	}
	ellipseArc(p, cx, cy, rx, ry, sp, cp, t1, dt, p1)
}

// ellipseArc appends cubics approximating the arc of the ellipse with
// center (cx, cy), radii (rx, ry) and rotation (sp, cp) = (sin, cos) from
// angle t1 over the sweep dt, at most 45 degrees per cubic. The last point
// is end exactly.
func ellipseArc(p *gfx.Path, cx, cy, rx, ry, sp, cp, t1, dt float64, end gfx.Point) {
	n := int(math.Ceil(math.Abs(dt)/(math.Pi/4) - 1e-9))
	n = min(max(n, 1), 64)
	d := dt / float64(n)
	k := float64(4.0/3) * fmath.Tan(d/4)
	// at returns the point and the derivative (times k) at angle t.
	at := func(t float64) (px, py, dx, dy float64) {
		st, ct := fmath.Sincos(t)
		ex, ey := float64(rx*ct), float64(ry*st)    // point before rotation
		ddx, ddy := -float64(rx*st), float64(ry*ct) // derivative before rotation
		px = cx + float64(cp*ex) - float64(sp*ey)
		py = cy + float64(sp*ex) + float64(cp*ey)
		dx = float64(k * (float64(cp*ddx) - float64(sp*ddy)))
		dy = float64(k * (float64(sp*ddx) + float64(cp*ddy)))
		return
	}
	x0, y0, dx0, dy0 := at(t1)
	if cur, ok := p.CurrentPoint(); ok {
		x0, y0 = float64(cur.X), float64(cur.Y) // the exact start point
	}
	for i := 1; i <= n; i++ {
		x3, y3, dx3, dy3 := at(t1 + float64(d*float64(i)))
		e := gfx.Pt(float32(x3), float32(y3))
		if i == n {
			e = end
		}
		p.CubicTo(gfx.Pt(float32(x0+dx0), float32(y0+dy0)), gfx.Pt(float32(x3-dx3), float32(y3-dy3)), e)
		x0, y0, dx0, dy0 = x3, y3, dx3, dy3
	}
}

// ellipsePath returns the closed path of an ellipse, starting at
// (cx + rx, cy) and turning in the positive angle direction (clockwise on
// screen), as SVG 2 defines it for circle and ellipse (it matters for
// dashes).
func ellipsePath(cx, cy, rx, ry float32) gfx.Path {
	var p gfx.Path
	p.MoveTo(gfx.Pt(cx+rx, cy))
	ends := [4]gfx.Point{{X: cx, Y: cy + ry}, {X: cx - rx, Y: cy}, {X: cx, Y: cy - ry}, {X: cx + rx, Y: cy}}
	for q, e := range ends {
		ellipseArc(&p, float64(cx), float64(cy), float64(rx), float64(ry), 0, 1, float64(q)*(math.Pi/2), math.Pi/2, e)
	}
	p.Close()
	return p
}

// rectPath returns the path of a rect element: clockwise from (x + rx, y),
// with elliptical corners when rx and ry are both positive (already
// clamped to half the size).
func rectPath(x, y, w, h, rx, ry float32) gfx.Path {
	var p gfx.Path
	x1, y1 := x+w, y+h
	if rx <= 0 || ry <= 0 {
		p.MoveTo(gfx.Pt(x, y))
		p.LineTo(gfx.Pt(x1, y))
		p.LineTo(gfx.Pt(x1, y1))
		p.LineTo(gfx.Pt(x, y1))
		p.Close()
		return p
	}
	corner := func(cx, cy float32, t float64, end gfx.Point) {
		ellipseArc(&p, float64(cx), float64(cy), float64(rx), float64(ry), 0, 1, t, math.Pi/2, end)
	}
	// Straight sides are skipped when the corners meet (rx = w/2 or
	// ry = h/2), so that no zero-length segment reaches the stroker.
	line := func(q gfx.Point) {
		if cur, _ := p.CurrentPoint(); cur != q {
			p.LineTo(q)
		}
	}
	p.MoveTo(gfx.Pt(x+rx, y))
	line(gfx.Pt(x1-rx, y))
	corner(x1-rx, y+ry, -math.Pi/2, gfx.Pt(x1, y+ry))
	line(gfx.Pt(x1, y1-ry))
	corner(x1-rx, y1-ry, 0, gfx.Pt(x1-rx, y1))
	line(gfx.Pt(x+rx, y1))
	corner(x+rx, y1-ry, math.Pi/2, gfx.Pt(x, y1-ry))
	line(gfx.Pt(x, y+ry))
	corner(x+rx, y+ry, math.Pi, gfx.Pt(x+rx, y))
	p.Close()
	return p
}

// pointsPath returns the path of a polyline or polygon from its points
// attribute. An odd coordinate count drops the last one; a syntax error
// keeps the points before it.
func pointsPath(s string, closed bool) gfx.Path {
	v, _ := parseNumberList(s)
	var p gfx.Path
	if len(v) < 4 {
		return p
	}
	p.MoveTo(gfx.Pt(v[0], v[1]))
	for i := 2; i+1 < len(v); i += 2 {
		p.LineTo(gfx.Pt(v[i], v[i+1]))
	}
	if closed {
		p.Close()
	}
	return p
}
