package svg

import (
	"math"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gfx"
)

// Value parsing: numbers, lengths, lists, transforms, viewBox and
// preserveAspectRatio. Every parser rejects what browsers reject; the
// caller then ignores the attribute or declaration, as if it were absent.

// maxAbs is the largest magnitude accepted for a number. Larger values,
// which float32 geometry could not use anyway, make the value invalid.
const maxAbs = 1e9

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// skipSpace returns the index of the first non-space byte at or after i.
func skipSpace(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}

// skipSep skips white space and at most one comma (the separator of SVG
// number lists and path data).
func skipSep(s string, i int) int {
	i = skipSpace(s, i)
	if i < len(s) && s[i] == ',' {
		i = skipSpace(s, i+1)
	}
	return i
}

// scanNumber scans an SVG/CSS number at s[i:]: an optional sign, digits
// with an optional fraction (or a fraction alone), and an optional
// exponent, which is taken only when digits follow the 'e' (so that "1em"
// is the number 1 and the unit em). It returns the value, the index after
// it and whether a number was there and is finite and within maxAbs.
// Compact forms such as "1.5.5" scan as 1.5 then .5.
func scanNumber(s string, i int) (float64, int, bool) {
	start := i
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		j := i + 1
		frac := 0
		for j < len(s) && isDigit(s[j]) {
			j++
			frac++
		}
		if frac > 0 || digits > 0 {
			i = j
			digits += frac
		}
	}
	if digits == 0 {
		return 0, start, false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			for j < len(s) && isDigit(s[j]) {
				j++
			}
			i = j
		}
	}
	v, err := strconv.ParseFloat(s[start:i], 64)
	if err != nil && !isRangeErr(err) {
		return 0, start, false
	}
	if math.IsNaN(v) || math.Abs(v) > maxAbs {
		return 0, i, false
	}
	return v, i, true
}

func isRangeErr(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

// parseNumber parses a whole attribute value as one number, with optional
// surrounding white space.
func parseNumber(s string) (float64, bool) {
	i := skipSpace(s, 0)
	v, j, ok := scanNumber(s, i)
	if !ok || skipSpace(s, j) != len(s) {
		return 0, false
	}
	return v, true
}

// parseAlpha parses an opacity: a number or a percentage, clamped to
// [0, 1] (CSS Color 4 <alpha-value>).
func parseAlpha(s string) (float32, bool) {
	s = strings.TrimSpace(s)
	pct := strings.HasSuffix(s, "%")
	if pct {
		s = s[:len(s)-1]
	}
	v, ok := parseNumber(s)
	if !ok {
		return 0, false
	}
	if pct {
		v /= 100
	}
	return float32(min(max(v, 0), 1)), true
}

// unit is a length unit.
type unit uint8

const (
	uNone unit = iota // user units (px)
	uPx
	uPercent
	uEm
	uEx
	uRem
	uMm
	uCm
	uIn
	uPt
	uPc
)

// length is a number with a unit, as written.
type length struct {
	v float32
	u unit
}

var units = map[string]unit{
	"px": uPx, "%": uPercent, "em": uEm, "ex": uEx, "rem": uRem,
	"mm": uMm, "cm": uCm, "in": uIn, "pt": uPt, "pc": uPc,
}

// scanLength scans a length at s[i:].
func scanLength(s string, i int) (length, int, bool) {
	v, j, ok := scanNumber(s, i)
	if !ok {
		return length{}, i, false
	}
	k := j
	for k < len(s) && (s[k] == '%' || s[k] >= 'a' && s[k] <= 'z' || s[k] >= 'A' && s[k] <= 'Z') {
		k++
	}
	if k == j {
		return length{float32(v), uNone}, j, true
	}
	u, ok := units[strings.ToLower(s[j:k])]
	if !ok {
		return length{}, i, false
	}
	return length{float32(v), u}, k, true
}

// parseLength parses a whole value as one length.
func parseLength(s string) (length, bool) {
	i := skipSpace(s, 0)
	l, j, ok := scanLength(s, i)
	if !ok || skipSpace(s, j) != len(s) {
		return length{}, false
	}
	return l, true
}

// Absolute units in px at 96 dpi (CSS Values 4).
const (
	pxPerIn = 96
	pxPerCm = 96 / 2.54
	pxPerMm = 96 / 25.4
	pxPerPt = 96.0 / 72
	pxPerPc = 96.0 / 6
)

// defaultFontSize is the font size of the root (medium), for em, ex and
// rem.
const defaultFontSize = 16

// axis selects what a percentage refers to.
type axis uint8

const (
	axisX    axis = iota // the viewport width
	axisY                // the viewport height
	axisDiag             // sqrt((w² + h²) / 2)
)

// viewport is the size of the nearest viewport, for percentages.
type viewport struct{ w, h float32 }

func (vp viewport) diag() float32 {
	w, h := float64(vp.w), float64(vp.h)
	return float32(math.Sqrt((float64(w*w) + float64(h*h)) / 2))
}

// fonts are the font sizes that font-relative units refer to: the
// element's (em; ex is taken as half of it, like resvg) and the root
// element's (rem: in an SVG document the root is the svg element).
type fonts struct{ em, rem float32 }

// resolve converts l to user units.
func (l length) resolve(ax axis, vp viewport, f fonts) float32 {
	fontSize := f.em
	switch l.u {
	case uNone, uPx:
		return l.v
	case uPercent:
		var ref float32
		switch ax {
		case axisX:
			ref = vp.w
		case axisY:
			ref = vp.h
		default:
			ref = vp.diag()
		}
		return float32(float64(l.v) * float64(ref) / 100)
	case uEm:
		return float32(l.v * fontSize)
	case uEx:
		return float32(l.v*fontSize) / 2
	case uRem:
		return float32(l.v * f.rem)
	case uMm:
		return float32(float64(l.v) * pxPerMm)
	case uCm:
		return float32(float64(l.v) * pxPerCm)
	case uIn:
		return l.v * pxPerIn
	case uPt:
		return float32(float64(l.v) * pxPerPt)
	case uPc:
		return l.v * pxPerPc
	}
	return l.v
}

// isAbsolute reports whether l does not depend on a viewport (intrinsic
// sizes: percentages are not).
func (l length) isAbsolute() bool { return l.u != uPercent }

// parseNumberList parses numbers separated by white space and/or one
// comma. ok is false if the list is malformed; numbers parsed before the
// error are returned (points and path data keep them).
func parseNumberList(s string) ([]float32, bool) {
	var out []float32
	i := skipSpace(s, 0)
	for i < len(s) {
		v, j, ok := scanNumber(s, i)
		if !ok {
			return out, false
		}
		out = append(out, float32(v))
		k := skipSpace(s, j)
		if k < len(s) && s[k] == ',' {
			k = skipSpace(s, k+1)
			if k == len(s) {
				return out, false // trailing comma
			}
		}
		i = k
	}
	return out, true
}

// parseViewBox parses "min-x min-y width height". A negative width or
// height invalidates it; a zero one is returned (it disables rendering).
func parseViewBox(s string) (gfx.Rect, bool) {
	v, ok := parseNumberList(s)
	if !ok || len(v) != 4 || v[2] < 0 || v[3] < 0 {
		return gfx.Rect{}, false
	}
	return gfx.Rect{X0: v[0], Y0: v[1], X1: v[0] + v[2], Y1: v[1] + v[3]}, true
}

// aspect is a parsed preserveAspectRatio.
type aspect struct {
	none   bool  // align none: stretch
	ax, ay uint8 // 0 min, 1 mid, 2 max
	slice  bool
}

var defaultAspect = aspect{ax: 1, ay: 1}

func parseAspect(s string) (aspect, bool) {
	f := strings.Fields(s)
	if len(f) > 0 && f[0] == "defer" { // SVG 1.1, ignored for svg elements
		f = f[1:]
	}
	if len(f) == 0 || len(f) > 2 {
		return aspect{}, false
	}
	var a aspect
	if f[0] == "none" {
		a.none = true
	} else {
		if len(f[0]) != 8 || f[0][0] != 'x' || f[0][4] != 'Y' {
			return aspect{}, false
		}
		var ok1, ok2 bool
		a.ax, ok1 = alignOf(f[0][1:4])
		a.ay, ok2 = alignOf(f[0][5:8])
		if !ok1 || !ok2 {
			return aspect{}, false
		}
	}
	if len(f) == 2 {
		switch f[1] {
		case "meet":
		case "slice":
			a.slice = true
		default:
			return aspect{}, false
		}
	}
	return a, true
}

func alignOf(s string) (uint8, bool) {
	switch s {
	case "Min":
		return 0, true
	case "Mid":
		return 1, true
	case "Max":
		return 2, true
	}
	return 0, false
}

// viewBoxTransform maps the viewBox vb into the viewport rectangle dst
// following a (SVG 2, section 8.2). vb and dst have positive sizes. The
// arithmetic is fusion-safe (ADR-041).
func viewBoxTransform(vb, dst gfx.Rect, a aspect) gfx.Affine {
	vw, vh := float64(vb.Width()), float64(vb.Height())
	dw, dh := float64(dst.Width()), float64(dst.Height())
	sx, sy := dw/vw, dh/vh
	if !a.none {
		s := min(sx, sy)
		if a.slice {
			s = max(sx, sy)
		}
		sx, sy = s, s
	}
	tx := float64(dst.X0) - float64(float64(vb.X0)*sx)
	ty := float64(dst.Y0) - float64(float64(vb.Y0)*sy)
	if !a.none {
		ex := dw - float64(vw*sx) // free space
		ey := dh - float64(vh*sy)
		tx += float64(ex*float64(a.ax)) / 2
		ty += float64(ey*float64(a.ay)) / 2
	}
	return gfx.Affine{float32(sx), 0, 0, float32(sy), float32(tx), float32(ty)}
}

// parseTransform parses an SVG transform list. An invalid list is
// rejected as a whole (browsers then use no transform).
func parseTransform(s string) (gfx.Affine, bool) {
	m := gfx.Identity
	i := skipSpace(s, 0)
	for i < len(s) {
		j := i
		for j < len(s) && (s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z') {
			j++
		}
		name := s[i:j]
		j = skipSpace(s, j)
		if j >= len(s) || s[j] != '(' {
			return gfx.Identity, false
		}
		end := strings.IndexByte(s[j:], ')')
		if end < 0 {
			return gfx.Identity, false
		}
		args, ok := parseNumberList(s[j+1 : j+end])
		if !ok {
			return gfx.Identity, false
		}
		t, ok := transformOf(name, args)
		if !ok {
			return gfx.Identity, false
		}
		m = m.Mul(t)
		i = skipSep(s, j+end+1)
	}
	return m, true
}

func transformOf(name string, a []float32) (gfx.Affine, bool) {
	n := len(a)
	switch name {
	case "matrix":
		if n == 6 {
			return gfx.Affine{a[0], a[1], a[2], a[3], a[4], a[5]}, true
		}
	case "translate":
		switch n {
		case 1:
			return gfx.Translate(a[0], 0), true
		case 2:
			return gfx.Translate(a[0], a[1]), true
		}
	case "scale":
		switch n {
		case 1:
			return gfx.Scale(a[0], a[0]), true
		case 2:
			return gfx.Scale(a[0], a[1]), true
		}
	case "rotate":
		switch n {
		case 1:
			return rotateDeg(a[0]), true
		case 3:
			c := gfx.Pt(a[1], a[2])
			return gfx.Translate(c.X, c.Y).Mul(rotateDeg(a[0])).Mul(gfx.Translate(-c.X, -c.Y)), true
		}
	case "skewX":
		if n == 1 {
			return gfx.Affine{1, 0, tanDeg(a[0]), 1, 0, 0}, true
		}
	case "skewY":
		if n == 1 {
			return gfx.Affine{1, tanDeg(a[0]), 0, 1, 0, 0}, true
		}
	}
	return gfx.Identity, false
}

// sinCosDeg returns the sine and cosine of an angle in degrees, exact for
// multiples of 90 degrees (like Skia), so that quarter turns keep
// axis-aligned transforms axis-aligned.
func sinCosDeg(deg float32) (s, c float32) {
	d := math.Mod(float64(deg), 360)
	if d < 0 {
		d += 360
	}
	switch d {
	case 0:
		return 0, 1
	case 90:
		return 1, 0
	case 180:
		return 0, -1
	case 270:
		return -1, 0
	}
	sf, cf := fmath.Sincos(d * (math.Pi / 180))
	return float32(sf), float32(cf)
}

func rotateDeg(deg float32) gfx.Affine {
	s, c := sinCosDeg(deg)
	return gfx.Affine{c, s, -s, c, 0, 0}
}

func tanDeg(deg float32) float32 {
	return float32(fmath.Tan(float64(deg) * (math.Pi / 180)))
}

// parseIRI returns the id of a local reference "#id" (href).
func parseIRI(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '#' {
		return "", false
	}
	return s[1:], true
}

// scanFuncIRI scans "url(#id)" (quotes allowed) at the start of s and
// returns the id, the rest of s and whether the reference is a local one.
// A url() to another document is valid syntax but references nothing:
// id is "" and ok is true.
func scanFuncIRI(s string) (id, rest string, ok bool) {
	if len(s) < 4 || !strings.EqualFold(s[:4], "url(") {
		return "", s, false
	}
	end := strings.IndexByte(s, ')')
	if end < 0 {
		return "", s, false
	}
	inner := strings.TrimSpace(s[4:end])
	if len(inner) >= 2 && (inner[0] == '"' || inner[0] == '\'') && inner[len(inner)-1] == inner[0] {
		inner = strings.TrimSpace(inner[1 : len(inner)-1])
	}
	rest = strings.TrimSpace(s[end+1:])
	if strings.HasPrefix(inner, "#") && len(inner) > 1 {
		return inner[1:], rest, true
	}
	return "", rest, true
}
