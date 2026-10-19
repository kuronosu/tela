package style

import (
	"errors"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// GradientKind selects linear, radial or conic.
type GradientKind uint8

const (
	GradientLinear GradientKind = iota
	GradientRadial
	GradientConic
)

// Corner is the corner of a "to bottom right" linear gradient direction.
type Corner uint8

const (
	// CornerNone means the direction is the Angle. It is the zero value.
	CornerNone Corner = iota
	CornerTopRight
	CornerBottomRight
	CornerBottomLeft
	CornerTopLeft
)

// RadialShape is circle or ellipse.
type RadialShape uint8

const (
	ShapeEllipse RadialShape = iota
	ShapeCircle
)

// RadialSize selects the extent of a radial gradient's ending shape.
type RadialSize uint8

const (
	SizeFarthestCorner RadialSize = iota
	SizeClosestSide
	SizeClosestCorner
	SizeFarthestSide
	// SizeExplicit uses Gradient.Radius.
	SizeExplicit
)

// StopPos is the position of a color stop. The zero value is automatic.
// Percentages refer to the gradient line (linear), the radius (radial) or
// the full turn (conic); pixels to the gradient line or radius.
type StopPos struct {
	Set bool
	LP
}

// ColorStop is one stop of a gradient.
type ColorStop struct {
	Color gfx.Color
	Pos   StopPos
}

// Gradient keeps the CSS meaning of a gradient image, which depends on the
// box it paints (an angle or corner keyword, percentages): Paint turns it
// into a gfx paint for a given rectangle (ADR-023).
type Gradient struct {
	Kind GradientKind

	// Angle is the direction of a linear gradient in degrees (0 is up, 90
	// is to the right) when Corner is CornerNone, and the start angle
	// ("from") of a conic gradient.
	Angle  float32
	Corner Corner

	// Radial shape, size and (for SizeExplicit) radii; a circle uses
	// Radius[0] only.
	Shape  RadialShape
	Size   RadialSize
	Radius [2]LP

	// Center is the position of a radial or conic gradient inside the
	// box, from its top-left corner. The default is 50% 50%.
	CenterX, CenterY LP

	// Interp is the color space of the interpolation, Oklab by default.
	Interp gfx.ColorSpace

	Stops []ColorStop
}

// newGradient returns a gradient of the kind with the CSS defaults: to
// bottom for linear, farthest-corner ellipse at the center for radial.
func newGradient(kind GradientKind) Gradient {
	g := Gradient{Kind: kind, CenterX: PctLP(50), CenterY: PctLP(50)}
	if kind == GradientLinear {
		g.Angle = 180
	}
	return g
}

// Paint returns the paint of the gradient for a box with the given
// rectangle, in the coordinates of that rectangle (the color of a point
// of the gradient is the same as CSS paints in a box of this size).
func (g *Gradient) Paint(box gfx.Rect) gfx.Paint {
	w, h := box.Width(), box.Height()
	switch g.Kind {
	case GradientRadial:
		return g.radial(box, w, h)
	case GradientConic:
		c := gfx.Point{X: box.X0 + g.CenterX.Resolve(w), Y: box.Y0 + g.CenterY.Resolve(h)}
		return gfx.ConicGradient{Center: c, StartAngle: degToRad(g.Angle), Stops: g.stops(1), Interp: g.Interp}
	}
	// Linear: the gradient line passes through the center of the box and
	// is long enough that the corners get the 0% and 100% colors (CSS
	// Images 3, section 3.1.1).
	var dx, dy float32
	if g.Corner != CornerNone {
		sx, sy := float32(1), float32(1) // "to bottom right"
		switch g.Corner {
		case CornerTopRight:
			sy = -1
		case CornerBottomLeft:
			sx = -1
		case CornerTopLeft:
			sx, sy = -1, -1
		}
		// The line is perpendicular to the diagonal through the two
		// other corners, so the 50% color runs along that diagonal.
		vx, vy := float32(sx*h), float32(sy*w)
		n := hypot32(vx, vy)
		if n == 0 {
			dx, dy = 0, 1
		} else {
			dx, dy = vx/n, vy/n
		}
	} else {
		dx, dy = angleDirection(g.Angle)
	}
	length := float32(abs32(float32(w*dx)) + abs32(float32(h*dy)))
	c := box.Center()
	hx, hy := float32(dx*length)/2, float32(dy*length)/2
	return gfx.LinearGradient{
		Start:  gfx.Point{X: c.X - hx, Y: c.Y - hy},
		End:    gfx.Point{X: c.X + hx, Y: c.Y + hy},
		Stops:  g.stops(length),
		Interp: g.Interp,
	}
}

// angleDirection returns the unit vector of a CSS gradient angle: 0deg
// points up (toward -Y) and 90deg to the right. Right angles are exact.
func angleDirection(deg float32) (dx, dy float32) {
	d := float32(math.Mod(float64(deg), 360))
	if d < 0 {
		d += 360
	}
	switch d {
	case 0:
		return 0, -1
	case 90:
		return 1, 0
	case 180:
		return 0, 1
	case 270:
		return -1, 0
	}
	rot := gfx.Rotate(degToRad(d)) // [cos sin -sin cos]
	return rot[1], -rot[0]
}

func (g *Gradient) radial(box gfx.Rect, w, h float32) gfx.Paint {
	cx, cy := g.CenterX.Resolve(w), g.CenterY.Resolve(h)
	// Distances from the center to the sides.
	left, right := abs32(cx), abs32(w-cx)
	top, bottom := abs32(cy), abs32(h-cy)
	closestX, farthestX := min(left, right), max(left, right)
	closestY, farthestY := min(top, bottom), max(top, bottom)
	var rx, ry float32
	switch {
	case g.Size == SizeExplicit:
		rx = g.Radius[0].Resolve(w)
		ry = rx
		if g.Shape == ShapeEllipse {
			ry = g.Radius[1].Resolve(h)
		}
	case g.Shape == ShapeCircle:
		var r float32
		switch g.Size {
		case SizeClosestSide:
			r = min(closestX, closestY)
		case SizeFarthestSide:
			r = max(farthestX, farthestY)
		case SizeClosestCorner:
			r = hypot32(closestX, closestY)
		default:
			r = hypot32(farthestX, farthestY)
		}
		rx, ry = r, r
	default: // ellipse
		switch g.Size {
		case SizeClosestSide:
			rx, ry = closestX, closestY
		case SizeFarthestSide:
			rx, ry = farthestX, farthestY
		case SizeClosestCorner:
			rx, ry = float32(closestX*math.Sqrt2), float32(closestY*math.Sqrt2)
		default:
			rx, ry = float32(farthestX*math.Sqrt2), float32(farthestY*math.Sqrt2)
		}
	}
	return gfx.RadialGradient{
		Center: gfx.Point{X: box.X0 + cx, Y: box.Y0 + cy},
		Radius: gfx.Point{X: rx, Y: ry},
		Stops:  g.stops(rx),
		Interp: g.Interp,
	}
}

// stops converts the color stops to gfx stops. Positions in pixels are
// divided by length, the length of the gradient line or radius.
func (g *Gradient) stops(length float32) []gfx.GradientStop {
	out := make([]gfx.GradientStop, len(g.Stops))
	for i, s := range g.Stops {
		if !s.Pos.Set {
			out[i] = gfx.AutoStop(s.Color)
			continue
		}
		off := s.Pos.Pct / 100
		if s.Pos.Px != 0 && length > 0 {
			off += s.Pos.Px / length
		}
		out[i] = gfx.GradientStop{Offset: off, Color: s.Color}
	}
	return out
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

// hypot32 returns sqrt(a*a + b*b). math.Hypot is not used: its pure-Go
// code may be fused differently per architecture (ADR-027). Products of
// float32 values are exact in float64, so this sum is the same everywhere.
func hypot32(a, b float32) float32 {
	x, y := float64(a), float64(b)
	return float32(math.Sqrt(float64(x*x) + float64(y*y)))
}

// BackgroundBox is a box of the CSS box model: border-box, padding-box,
// content-box or, for background-clip only, text.
type BackgroundBox uint8

const (
	BackgroundBorderBox BackgroundBox = iota
	BackgroundPaddingBox
	BackgroundContentBox
	// BackgroundText clips the background to the text (bg-clip-text).
	BackgroundText
)

// BackgroundSizeKind selects the meaning of a BackgroundSize.
type BackgroundSizeKind uint8

const (
	// SizeAuto is the image's natural size. It is the zero value.
	SizeAutoImage BackgroundSizeKind = iota
	SizeCover
	SizeContain
	// SizeLength uses W and H (a zero Dim is auto).
	SizeLength
)

// BackgroundSize is the CSS background-size.
type BackgroundSize struct {
	Kind BackgroundSizeKind
	W, H Dim
}

// BackgroundRepeat is the CSS background-repeat property.
type BackgroundRepeat uint8

const (
	RepeatBoth BackgroundRepeat = iota
	RepeatX
	RepeatY
	RepeatNone
	RepeatSpace
	RepeatRound
)

// BackgroundLayer is one background image layer: a gradient, an image
// named by url() (resolved by core through the assets registered with
// tela.Assets) or a decoded image (tela.BgImage).
type BackgroundLayer struct {
	// Gradient is the image when it is a gradient; nil for URL images.
	Gradient *Gradient
	// URL is the source of an image layer, as written in url().
	URL string
	// Image is a decoded image (tela.BgImage). Its dynamic type need not
	// be comparable: do not compare layers with ==.
	Image  image.Image
	Size   BackgroundSize
	PosX   LP // position of the image in the box; 0% is the top-left corner
	PosY   LP
	Repeat BackgroundRepeat
	Origin BackgroundBox
}

// ---------------------------------------------------------------------
// Parsing of gradient arguments in arbitrary values, such as
// bg-linear-[25deg,red_5%,yellow_60%] or bg-radial-[at_25%_25%].

// parseCornerOrAngle parses the direction argument of a linear gradient.
func parseLinearDirection(s string, g *Gradient) error {
	s = strings.TrimSpace(s)
	if rest, ok := strings.CutPrefix(s, "to "); ok {
		f := strings.Fields(rest)
		var v, hz string
		for _, w := range f {
			switch w {
			case "top", "bottom":
				v = w
			case "left", "right":
				hz = w
			default:
				return fmt.Errorf("invalid gradient direction %q; use e.g. \"to right\" or \"to bottom right\"", s)
			}
		}
		switch {
		case v == "" && hz == "":
			return fmt.Errorf("invalid gradient direction %q", s)
		case v == "" && hz == "right":
			g.Angle = 90
		case v == "" && hz == "left":
			g.Angle = 270
		case hz == "" && v == "top":
			g.Angle = 0
		case hz == "" && v == "bottom":
			g.Angle = 180
		case v == "top" && hz == "right":
			g.Corner = CornerTopRight
		case v == "bottom" && hz == "right":
			g.Corner = CornerBottomRight
		case v == "bottom" && hz == "left":
			g.Corner = CornerBottomLeft
		default:
			g.Corner = CornerTopLeft
		}
		return nil
	}
	a, ok := parseAngle(s)
	if !ok {
		return fmt.Errorf("invalid gradient angle %q; use e.g. \"45deg\" or \"to right\"", s)
	}
	g.Angle = a
	return nil
}

// parsePositionPair parses "50% 50%", "top left", "10px 20px", "center".
func parsePositionPair(fields []string) (x, y LP, err error) {
	x, y = PctLP(50), PctLP(50)
	if len(fields) == 0 || len(fields) > 2 {
		return x, y, fmt.Errorf("invalid position %q; use one or two values such as \"top left\" or \"25%% 75%%\"", strings.Join(fields, " "))
	}
	type axis int
	const (
		anyAxis axis = iota
		hor
		ver
	)
	conv := func(w string) (LP, axis, error) {
		switch w {
		case "left":
			return PctLP(0), hor, nil
		case "right":
			return PctLP(100), hor, nil
		case "top":
			return PctLP(0), ver, nil
		case "bottom":
			return PctLP(100), ver, nil
		case "center":
			return PctLP(50), anyAxis, nil
		}
		e, err := parseLengthExpr(w, true)
		if err != nil {
			return LP{}, anyAxis, fmt.Errorf("invalid position value %q", w)
		}
		lp, err := trackLP(e, 16)
		if err != nil {
			return LP{}, anyAxis, err
		}
		return lp, anyAxis, nil
	}
	a, aa, err := conv(fields[0])
	if err != nil {
		return x, y, err
	}
	if len(fields) == 1 {
		switch aa {
		case ver:
			return PctLP(50), a, nil
		}
		return a, PctLP(50), nil
	}
	b, ba, err := conv(fields[1])
	if err != nil {
		return x, y, err
	}
	if aa == ver || ba == hor { // "top left": swap
		if aa == hor || ba == ver {
			return x, y, fmt.Errorf("invalid position %q", strings.Join(fields, " "))
		}
		return b, a, nil
	}
	return a, b, nil
}

// parseRadialPrelude parses "circle", "ellipse closest-side at 25% 25%",
// "at top" and the like into g.
func parseRadialPrelude(s string, g *Gradient) error {
	f := strings.Fields(s)
	var at []string
	for i, w := range f {
		if w == "at" {
			at = f[i+1:]
			f = f[:i]
			break
		}
	}
	for _, w := range f {
		switch w {
		case "circle":
			g.Shape = ShapeCircle
		case "ellipse":
			g.Shape = ShapeEllipse
		case "closest-side":
			g.Size = SizeClosestSide
		case "farthest-side":
			g.Size = SizeFarthestSide
		case "closest-corner":
			g.Size = SizeClosestCorner
		case "farthest-corner":
			g.Size = SizeFarthestCorner
		default:
			e, err := parseLengthExpr(w, true)
			if err != nil {
				return fmt.Errorf("invalid radial gradient argument %q", w)
			}
			lp, err := trackLP(e, 16)
			if err != nil {
				return err
			}
			g.Size = SizeExplicit
			if g.Radius[0] == (LP{}) && !g.radiusFirstSet() {
				g.Radius[0] = lp
			} else {
				g.Radius[1] = lp
			}
		}
	}
	if g.Size == SizeExplicit && g.Radius[1] == (LP{}) {
		g.Radius[1] = g.Radius[0]
		if g.Shape == ShapeEllipse && g.Radius[0].Pct == 0 && g.Radius[0].Px != 0 {
			// A single length is a circle's radius.
			g.Shape = ShapeCircle
		}
	}
	if at != nil {
		x, y, err := parsePositionPair(at)
		if err != nil {
			return err
		}
		g.CenterX, g.CenterY = x, y
	}
	return nil
}

func (g *Gradient) radiusFirstSet() bool { return g.Radius[0] != LP{} }

// parseConicPrelude parses "from 45deg at 50% 50%".
func parseConicPrelude(s string, g *Gradient) error {
	f := strings.Fields(s)
	for i := 0; i < len(f); i++ {
		switch f[i] {
		case "from":
			if i+1 >= len(f) {
				return errors.New("\"from\" needs an angle: from 45deg")
			}
			a, ok := parseAngle(f[i+1])
			if !ok {
				return fmt.Errorf("invalid conic gradient angle %q", f[i+1])
			}
			g.Angle = a
			i++
		case "at":
			x, y, err := parsePositionPair(f[i+1:])
			if err != nil {
				return err
			}
			g.CenterX, g.CenterY = x, y
			return nil
		default:
			return fmt.Errorf("invalid conic gradient argument %q; use \"from <angle>\" and \"at <position>\"", f[i])
		}
	}
	return nil
}

// parseColorStop parses "red 10%", "#fff", "rgb(0 0 0 / 50%) 20% 40%". A
// stop with two positions yields two stops.
func parseColorStop(c *pctx, s string) ([]ColorStop, error) {
	f := fieldsTop(strings.TrimSpace(s))
	if len(f) == 0 {
		return nil, errors.New("empty color stop")
	}
	spec, ok, err := c.colorValue(f[0])
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("invalid color %q in the gradient", f[0])
	}
	if spec.current {
		return nil, errors.New("currentColor in a gradient is only supported through from-current, via-current and to-current")
	}
	var out []ColorStop
	for _, p := range f[1:] {
		e, err := parseLengthExpr(p, true)
		if err != nil {
			return nil, fmt.Errorf("invalid color stop position %q", p)
		}
		lp, err := trackLP(e, c.th.RootFontSize)
		if err != nil {
			return nil, err
		}
		out = append(out, ColorStop{spec.c, StopPos{Set: true, LP: lp}})
	}
	if len(out) == 0 {
		out = []ColorStop{{Color: spec.c}}
	}
	if len(out) > 2 {
		return nil, fmt.Errorf("a color stop takes at most two positions, got %q", s)
	}
	return out, nil
}

// parseGradientList parses the comma separated argument list of an
// arbitrary gradient class: an optional prelude followed by color stops.
func parseGradientList(c *pctx, kind GradientKind, inner string) (Gradient, error) {
	g := newGradient(kind)
	parts := splitTop(inner, ',')
	first := strings.TrimSpace(parts[0])
	hasPrelude := false
	if first != "" {
		switch kind {
		case GradientLinear:
			if strings.HasPrefix(first, "to ") || isAngleToken(first) {
				hasPrelude = true
				if err := parseLinearDirection(first, &g); err != nil {
					return g, err
				}
			}
		case GradientRadial:
			if isRadialPrelude(first) {
				hasPrelude = true
				if err := parseRadialPrelude(first, &g); err != nil {
					return g, err
				}
			}
		case GradientConic:
			if strings.HasPrefix(first, "from ") || strings.HasPrefix(first, "at ") {
				hasPrelude = true
				if err := parseConicPrelude(first, &g); err != nil {
					return g, err
				}
			}
		}
	}
	if hasPrelude {
		parts = parts[1:]
	}
	for _, p := range parts {
		stops, err := parseColorStop(c, p)
		if err != nil {
			return g, err
		}
		g.Stops = append(g.Stops, stops...)
	}
	if len(g.Stops) == 0 && hasPrelude {
		g.Stops = nil // prelude only: stops come from from-*, via-* and to-*
	}
	return g, nil
}

func isAngleToken(s string) bool {
	_, ok := parseAngle(s)
	return ok
}

func isRadialPrelude(s string) bool {
	w := strings.Fields(s)
	if len(w) == 0 {
		return false
	}
	switch w[0] {
	case "circle", "ellipse", "closest-side", "farthest-side", "closest-corner", "farthest-corner", "at":
		return true
	}
	if looksLikeLength(w[0]) && len(w) > 1 {
		return true
	}
	return false
}
