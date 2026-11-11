package text

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Synthetic bold and oblique (ADR-269). When CSS font matching picks a
// face that lacks the requested weight or slope, browsers draw it
// emboldened or slanted rather than as it is (CSS font-synthesis: auto,
// the default, which Tailwind has no utility for). Tela follows Chromium
// on Linux, Blink with Skia:
//
//   - Bold is synthesized when the request is at least 600 (Blink's
//     kBoldThreshold) and the face's heaviest weight is below 600 (its
//     wght axis maximum for a variable font): the rule of
//     CSSSegmentedFontFace for @font-face fonts, which Tela's app fonts
//     are. The glyph outline is filled together with its stroke (Skia's
//     stroke-and-fill fake bold, SkScalerContextRec::useStrokeForFakeBold)
//     of width size × s, where s goes linearly from 1/24 at a font size of
//     9 px to 1/32 at 36 px and stays there beyond (Skia's
//     kStdFakeBoldInterpKeys and Values), with miter joins (limit 4): each
//     edge moves out by half of it. The size is the CSS font size rounded to
//     whole pixels (the optical size's rounding). Advances do not change,
//     as in Blink, so text lays out exactly as with the face itself.
//   - Oblique is synthesized when italic or oblique is requested and the
//     face is upright without an ital or slnt axis that gives it: the
//     outline is skewed by -1/4 (x' = x - y/4 in Tela's y-down em space,
//     about 14 degrees; Blink's SkFont::setSkewX(-SK_Scalar1/4)), after
//     the emboldening.
//
// Color glyphs (COLR, CBDT, sbix) are never synthesized. A synthetic face
// is another Face (another FontKey, so the rasterizers cache its glyphs
// apart) over the same typeface and coordinates: it shapes, measures and
// reports metrics as its base face.

// syntheticBoldMin is the requested weight from which a face lighter than
// it is emboldened (Blink's kBoldThreshold).
const syntheticBoldMin = 600

// obliqueSkew is the horizontal skew of synthetic oblique: x' = x +
// obliqueSkew*y in em space (y down).
const obliqueSkew = -0.25

// synthTolerance is the flattening tolerance of the stroke of an
// emboldened outline, in em: 0.025 device pixels at 200 px.
const synthTolerance = 1.0 / 8192

// fakeBoldWidth returns the stroke width of synthetic bold in em for a font
// of size CSS pixels (Skia's interpolation; see the comment above).
func fakeBoldWidth(size float32) float32 {
	const k0, k1 = 9, 36
	const v0, v1 = 1.0 / 24, 1.0 / 32
	s := float64(size)
	if !(s > 0) || math.IsInf(s, 0) {
		s = 16
	}
	switch {
	case s <= k0:
		return v0
	case s >= k1:
		return v1
	}
	t := (s - k0) / (k1 - k0)
	return float32(v0 + float64(t*(v1-v0)))
}

// synthesis returns what instance synthesizes for q on tf at size: the
// stroke width of synthetic bold in em (0 for none) and whether it slants
// the outlines. inst is the instance the face has.
func synthesis(tf *Typeface, q Query, inst Instance, size float32) (bold float32, oblique bool) {
	if tf.IsColor() {
		return 0, false
	}
	if _, hi := tf.WeightRange(); q.weight() >= syntheticBoldMin && hi < syntheticBoldMin {
		bold = fakeBoldWidth(size)
	}
	oblique = q.Style != StyleNormal && tf.style == StyleNormal && !inst.Italic && !inst.Oblique
	return bold, oblique
}

// syntheticPath returns the outline p (em space, NonZero) emboldened by a
// stroke of width bold (0: not emboldened) and skewed when oblique.
func syntheticPath(p gfx.Path, bold float32, oblique bool) gfx.Path {
	if p.IsEmpty() {
		return p
	}
	out := p
	if bold > 0 {
		out = embolden(p, float64(bold)/2)
	}
	if oblique {
		out = out.Transform(gfx.Affine{1, 0, obliqueSkew, 1, 0, 0})
	}
	return out
}

// pt2 is a float64 point. Its arithmetic converts every product
// explicitly so that the compiler cannot fuse multiply-adds (ADR-027).
type pt2 struct{ x, y float64 }

func (a pt2) add(b pt2) pt2     { return pt2{a.x + b.x, a.y + b.y} }
func (a pt2) sub(b pt2) pt2     { return pt2{a.x - b.x, a.y - b.y} }
func (a pt2) mul(s float64) pt2 { return pt2{float64(a.x * s), float64(a.y * s)} }
func dotp(a, b pt2) float64     { return float64(a.x*b.x) + float64(a.y*b.y) }
func crossp(a, b pt2) float64   { return float64(a.x*b.y) - float64(a.y*b.x) }
func (a pt2) point() gfx.Point  { return gfx.Point{X: float32(a.x), Y: float32(a.y)} }
func (a pt2) unit() (pt2, float64) {
	l := math.Sqrt(dotp(a, a))
	if !(l > 0) {
		return pt2{}, 0
	}
	return pt2{a.x / l, a.y / l}, l
}

// contours flattens p into closed polygons, without repeated points.
func contours(p gfx.Path) [][]pt2 {
	var out [][]pt2
	var cur []pt2
	end := func() {
		if n := len(cur); n > 1 && cur[n-1] == cur[0] {
			cur = cur[:n-1]
		}
		if len(cur) >= 3 {
			out = append(out, cur)
		}
		cur = nil
	}
	p.Flatten(synthTolerance, func(v gfx.Verb, q gfx.Point) {
		pt := pt2{float64(q.X), float64(q.Y)}
		switch v {
		case gfx.MoveTo:
			end()
			cur = []pt2{pt}
		case gfx.LineTo:
			if len(cur) == 0 || cur[len(cur)-1] != pt {
				cur = append(cur, pt)
			}
		case gfx.Close:
			end()
		}
	})
	end()
	return out
}

// polyArea returns twice the signed area of a closed polygon.
func polyArea(c []pt2) float64 {
	var a float64
	for i := range c {
		a += crossp(c[i], c[(i+1)%len(c)])
	}
	return a
}

// emboldenMiter is the miter limit of synthetic bold's joins (Skia's
// default SkPaint miter limit, the SVG and CSS default).
const emboldenMiter = 4

// embolden returns the outline p (NonZero) grown by h on every side: each
// contour of the flattened outline moves its edges h away from the filled
// side, its vertices to where the moved edges meet, so that its corners
// stay sharp, as the outline of Skia's stroke-and-fill fake bold (the
// union of the fill and a stroke of width 2h with miter joins): outer
// contours grow and holes shrink. Convex corners sharper than the miter
// limit are beveled; the shift of concave ones is bounded by the same
// limit. The result does not overlap itself, unlike a fill plus a stroke,
// whose overlapping edges an area-coverage rasterizer would count twice
// in partly covered pixels (the edge would look heavier than Skia's,
// which samples).
func embolden(p gfx.Path, h float64) gfx.Path {
	cs := contours(p)
	var total float64
	for _, c := range cs {
		total += polyArea(c)
	}
	if total == 0 {
		return p
	}
	// With NonZero filling and consistent winding, the filled side of
	// every edge (holes included) is on the left of the direction of
	// travel when the total area is positive, on the right otherwise.
	side := 1.0
	if total < 0 {
		side = -1
	}
	const minK = 2.0 / (emboldenMiter * emboldenMiter) // 1 + cos of the sharpest turn mitered
	var out gfx.Path
	for _, c := range cs {
		n := len(c)
		first := true
		emit := func(q pt2) {
			if first {
				out.MoveTo(q.point())
				first = false
			} else {
				out.LineTo(q.point())
			}
		}
		for i := range n {
			prev, cur, next := c[(i+n-1)%n], c[i], c[(i+1)%n]
			d1, _ := cur.sub(prev).unit()
			d2, _ := next.sub(cur).unit()
			// The normals away from the filled side.
			n1 := pt2{d1.y, -d1.x}.mul(side)
			n2 := pt2{d2.y, -d2.x}.mul(side)
			k := 1 + dotp(n1, n2) // 2 cos²(turn/2)
			convex := crossp(d1, d2)*side > 0
			switch {
			case convex && k < minK:
				// Beyond the miter limit: a bevel.
				emit(cur.add(n1.mul(h)))
				emit(cur.add(n2.mul(h)))
			case k < minK:
				// A concave spike: the meeting point is far away; bound
				// the shift by the miter limit.
				m, l := n1.add(n2).unit()
				if l == 0 {
					m = n1
				}
				emit(cur.add(m.mul(h * emboldenMiter)))
			default:
				emit(cur.add(n1.add(n2).mul(h / k)))
			}
		}
		out.Close()
	}
	return out
}
