package canvas

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

// A pen is a Canvas that also writes what is drawn with it as the JavaScript
// that draws the same on a browser's canvas, so that a scene is written once
// (chromium_test.go). Its methods are the Canvas's; its fields are the
// Canvas's too, and those that changed since the last call are written as
// assignments before the next one.
//
// What a browser's canvas does not have is not written: DrawIcon. The script
// has the canvas's context as c, and k device pixels for each logical one:
// what a browser's canvas does not transform (the shadows' lengths, and
// setTransform) is scaled by it.
type pen struct {
	*Canvas
	js *script // nil: only draw
}

// script is the JavaScript of a canvas.
type script struct {
	bytes.Buffer
	last   penState
	saved  []penState
	images map[image.Image]string
	tiles  map[*CanvasPattern]string
	decl   bytes.Buffer // the images
	paths  int
}

// penState is what the browser's context has.
type penState struct {
	fill, stroke                 string
	lineWidth, miter, alpha      float64
	dashOffset                   float64
	shadowColor                  gfx.Color
	shadowBlur, shadowX, shadowY float64
	font, letterSpacing          string
	cap                          LineCap
	join                         LineJoin
	op                           CompositeOperation
	align                        TextAlign
	baseline                     TextBaseline
	smooth                       bool
}

// newScript returns the script of a canvas whose context is new.
func newScript(dir text.Direction) *script {
	s := &script{images: map[image.Image]string{}, tiles: map[*CanvasPattern]string{}}
	s.last = penState{
		fill: "c.%s = \"rgb(0 0 0 / 1)\"", stroke: "c.%s = \"rgb(0 0 0 / 1)\"", lineWidth: 1, miter: 10, alpha: 1,
		letterSpacing: "0px", smooth: true,
	}
	if dir == text.DirectionRTL {
		s.stmt("c.direction = \"rtl\"")
	}
	return s
}

func (s *script) stmt(format string, args ...any) {
	fmt.Fprintf(s, format, args...)
	s.WriteString(";\n")
}

func num(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func nums(v ...float64) string {
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = num(x)
	}
	return strings.Join(out, ", ")
}

// jsString returns s as a literal.
func jsString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func cssColor(c gfx.Color) string {
	c = c.Clamp()
	return fmt.Sprintf("rgb(%.4g %.4g %.4g / %.4g)", c.R*255, c.G*255, c.B*255, c.A)
}

// style returns the statements that set a style, with the property as %s.
func (s *script) style(st Style) string {
	var b strings.Builder
	switch st := st.(type) {
	case nil:
		return "c.%s = \"transparent\""
	case gfx.Color:
		return "c.%s = \"" + cssColor(st) + "\""
	case *CanvasGradient:
		b.WriteString("g = c.")
		switch {
		case st.kind == conicGradient:
			fmt.Fprintf(&b, "createConicGradient(%s)", nums(st.r0, st.x0, st.y0))
		case st.kind == radialGradient, st.r0 != 0, st.r1 != 0:
			fmt.Fprintf(&b, "createRadialGradient(%s)", nums(st.x0, st.y0, st.r0, st.x1, st.y1, st.r1))
		default:
			fmt.Fprintf(&b, "createLinearGradient(%s)", nums(st.x0, st.y0, st.x1, st.y1))
		}
		for _, stop := range st.stops {
			fmt.Fprintf(&b, "; g.addColorStop(%s, %q)", strconv.FormatFloat(float64(stop.Offset), 'g', -1, 32), cssColor(stop.Color))
		}
		return b.String() + "; c.%s = g"
	case *CanvasPattern:
		repeat := [...]string{gfx.RepeatBoth: "repeat", gfx.RepeatX: "repeat-x", gfx.RepeatY: "repeat-y", gfx.RepeatNone: "no-repeat"}[st.repeat]
		fmt.Fprintf(&b, "g = c.createPattern(%s, %q)", s.tiles[st], repeat)
		if st.m != identity {
			fmt.Fprintf(&b, "; g.setTransform(new DOMMatrix([%s]))", nums(st.m.a, st.m.b, st.m.c, st.m.d, st.m.e, st.m.f))
		}
		return b.String() + "; c.%s = g"
	}
	panic("pen: a style of another kind")
}

// image returns the name of img in the script: an element, as the images of
// a page are (drawn smaller, Chromium shrinks a canvas or an ImageBitmap with
// another filter, unless imageSmoothingQuality says better).
func (s *script) image(img image.Image) string {
	if n := s.images[img]; n != "" {
		return n
	}
	name := fmt.Sprintf("image%d", len(s.images))
	s.images[img] = name
	b := img.Bounds()
	var pix []byte
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			switch img := img.(type) {
			case *image.NRGBA:
				p := img.NRGBAAt(x, y)
				pix = append(pix, p.R, p.G, p.B, p.A)
			case *image.RGBA:
				p := img.RGBAAt(x, y)
				if p.A != 255 {
					panic("pen: a premultiplied image with transparency")
				}
				pix = append(pix, p.R, p.G, p.B, p.A)
			}
		}
	}
	fmt.Fprintf(&s.decl, "const %s = image(%d, %q);\n", name, b.Dx(), base64.StdEncoding.EncodeToString(pix))
	return name
}

// chromiumFamilies are the families that the generic ones are in the
// deterministic fonts (package stdfonts), and after them those that Tela
// falls back to.
func chromiumFamilies(family string) string {
	var out []string
	for _, f := range append(ParseFamilies(family), "Inter", "JetBrains Mono", "Noto Sans Arabic", "Noto Color Emoji") {
		switch f {
		case "sans-serif", "system-ui", "ui-sans-serif":
			f = "Inter"
		case "serif", "ui-serif":
			f = "Source Serif 4"
		case "monospace", "ui-monospace":
			f = "JetBrains Mono"
		}
		if f = "'" + f + "'"; !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return strings.Join(out, ", ")
}

// cssFont returns the browser's font for f, of an element whose font is that
// of textStyle.
func cssFont(f CanvasFont) string {
	f = fullFont(f)
	style := ""
	if f.Italic {
		style = "italic "
	}
	return fmt.Sprintf("%s%s %spx %s", style, num(f.Weight), num(f.Size), chromiumFamilies(f.Family))
}

// fullFont returns f of an element whose font is that of textStyle.
func fullFont(f CanvasFont) CanvasFont {
	if f.Family == "" {
		f.Family = JoinFamilies(textStyle.Families)
	}
	if f.Size == 0 {
		f.Size = float64(textStyle.Size)
	}
	if f.Weight == 0 {
		f.Weight = float64(textStyle.Weight)
	}
	return f
}

var (
	alignNames    = [...]string{"start", "end", "left", "right", "center"}
	baselineNames = [...]string{"alphabetic", "top", "hanging", "middle", "ideographic", "bottom"}
	capNames      = [...]string{gfx.CapButt: "butt", gfx.CapRound: "round", gfx.CapSquare: "square"}
	joinNames     = [...]string{gfx.JoinMiter: "miter", gfx.JoinRound: "round", gfx.JoinBevel: "bevel"}
)

// sync writes the fields that changed, and returns the script, nil for none.
func (c *pen) sync() *script {
	s := c.js
	if s == nil {
		return nil
	}
	now := penState{
		fill: s.style(c.FillStyle), stroke: s.style(c.StrokeStyle),
		lineWidth: c.LineWidth, miter: c.MiterLimit, alpha: c.GlobalAlpha, dashOffset: c.LineDashOffset,
		shadowColor: c.ShadowColor, shadowBlur: c.ShadowBlur, shadowX: c.ShadowOffsetX, shadowY: c.ShadowOffsetY,
		font: cssFont(c.Font), letterSpacing: num(c.LetterSpacing) + "px",
		cap: c.LineCap, join: c.LineJoin, op: c.GlobalCompositeOperation,
		align: c.TextAlign, baseline: c.TextBaseline, smooth: c.ImageSmoothing,
	}
	old := &s.last
	if now.fill != old.fill {
		s.stmt(now.fill, "fillStyle")
	}
	if now.stroke != old.stroke {
		s.stmt(now.stroke, "strokeStyle")
	}
	set := func(name string, now, old float64, unit string) {
		if now != old {
			s.stmt("c.%s = %s%s", name, num(now), unit)
		}
	}
	set("lineWidth", now.lineWidth, old.lineWidth, "")
	set("miterLimit", now.miter, old.miter, "")
	set("globalAlpha", now.alpha, old.alpha, "")
	set("lineDashOffset", now.dashOffset, old.dashOffset, "")
	set("shadowBlur", now.shadowBlur, old.shadowBlur, " * k")
	set("shadowOffsetX", now.shadowX, old.shadowX, " * k")
	set("shadowOffsetY", now.shadowY, old.shadowY, " * k")
	if now.shadowColor != old.shadowColor {
		s.stmt("c.shadowColor = %q", cssColor(now.shadowColor))
	}
	name := func(prop, now, old string) {
		if now != old {
			s.stmt("c.%s = %q", prop, now)
		}
	}
	name("font", now.font, old.font)
	name("letterSpacing", now.letterSpacing, old.letterSpacing)
	name("lineCap", capNames[now.cap], capNames[old.cap])
	name("lineJoin", joinNames[now.join], joinNames[old.join])
	name("globalCompositeOperation", now.op.String(), old.op.String())
	name("textAlign", alignNames[now.align], alignNames[old.align])
	name("textBaseline", baselineNames[now.baseline], baselineNames[old.baseline])
	if now.smooth != old.smooth {
		s.stmt("c.imageSmoothingEnabled = %t", now.smooth)
	}
	s.last = now
	return s
}

// call writes a call with numbers.
func (c *pen) call(method string, args ...float64) {
	if s := c.sync(); s != nil {
		s.stmt("c.%s(%s)", method, nums(args...))
	}
}

// turn adds the last argument of arc and ellipse.
func turn(ccw []bool, args ...float64) []float64 {
	if isCCW(ccw) {
		return append(args, 1)
	}
	return args
}

func (c *pen) rule() string {
	if c.FillRule == gfx.EvenOdd {
		return "\"evenodd\""
	}
	return ""
}

// withPath writes a Path2D, with the curves it has for its arcs, and the call
// of method with it.
func (c *pen) withPath(method string, p *Path2D, rule string) {
	s := c.sync()
	if s == nil {
		return
	}
	name := fmt.Sprintf("p%d", s.paths)
	s.paths++
	s.stmt("const %s = new Path2D()", name)
	pts := p.p.pts
	for _, v := range p.p.verbs {
		var args []float64
		for _, pt := range pts[:v.NumPoints()] {
			args = append(args, pt.x, pt.y)
		}
		pts = pts[v.NumPoints():]
		verb := [...]string{gfx.MoveTo: "moveTo", gfx.LineTo: "lineTo", gfx.QuadTo: "quadraticCurveTo", gfx.CubicTo: "bezierCurveTo", gfx.Close: "closePath"}[v]
		s.stmt("%s.%s(%s)", name, verb, nums(args...))
	}
	if rule != "" {
		rule = ", " + rule
	}
	s.stmt("c.%s(%s%s)", method, name, rule)
}

func (c *pen) Save() {
	if s := c.sync(); s != nil {
		s.stmt("c.save()")
		s.saved = append(s.saved, s.last)
	}
	c.Canvas.Save()
}

func (c *pen) Restore() {
	if s := c.sync(); s != nil {
		s.stmt("c.restore()")
		if n := len(s.saved); n > 0 {
			s.last, s.saved = s.saved[n-1], s.saved[:n-1]
		}
	}
	c.Canvas.Restore()
}

func (c *pen) Translate(x, y float64) { c.call("translate", x, y); c.Canvas.Translate(x, y) }
func (c *pen) Rotate(angle float64)   { c.call("rotate", angle); c.Canvas.Rotate(angle) }
func (c *pen) Scale(x, y float64)     { c.call("scale", x, y); c.Canvas.Scale(x, y) }

func (c *pen) Transform(m11, m12, m21, m22, dx, dy float64) {
	c.call("transform", m11, m12, m21, m22, dx, dy)
	c.Canvas.Transform(m11, m12, m21, m22, dx, dy)
}

func (c *pen) SetTransform(m11, m12, m21, m22, dx, dy float64) {
	if s := c.sync(); s != nil {
		s.stmt("c.setTransform(k, 0, 0, k, 0, 0); c.transform(%s)", nums(m11, m12, m21, m22, dx, dy))
	}
	c.Canvas.SetTransform(m11, m12, m21, m22, dx, dy)
}

func (c *pen) ResetTransform() {
	if s := c.sync(); s != nil {
		s.stmt("c.setTransform(k, 0, 0, k, 0, 0)")
	}
	c.Canvas.ResetTransform()
}

func (c *pen) BeginPath()          { c.call("beginPath"); c.Canvas.BeginPath() }
func (c *pen) ClosePath()          { c.call("closePath"); c.Canvas.ClosePath() }
func (c *pen) MoveTo(x, y float64) { c.call("moveTo", x, y); c.Canvas.MoveTo(x, y) }
func (c *pen) LineTo(x, y float64) { c.call("lineTo", x, y); c.Canvas.LineTo(x, y) }

func (c *pen) QuadraticCurveTo(cpx, cpy, x, y float64) {
	c.call("quadraticCurveTo", cpx, cpy, x, y)
	c.Canvas.QuadraticCurveTo(cpx, cpy, x, y)
}

func (c *pen) BezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y float64) {
	c.call("bezierCurveTo", cp1x, cp1y, cp2x, cp2y, x, y)
	c.Canvas.BezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y)
}

func (c *pen) ArcTo(x1, y1, x2, y2, radius float64) {
	c.call("arcTo", x1, y1, x2, y2, radius)
	c.Canvas.ArcTo(x1, y1, x2, y2, radius)
}

func (c *pen) Rect(x, y, width, height float64) {
	c.call("rect", x, y, width, height)
	c.Canvas.Rect(x, y, width, height)
}

func (c *pen) RoundRect(x, y, width, height float64, radii ...float64) {
	if s := c.sync(); s != nil {
		if len(radii) == 0 { // an empty list is an error
			s.stmt("c.roundRect(%s)", nums(x, y, width, height))
		} else {
			s.stmt("c.roundRect(%s, [%s])", nums(x, y, width, height), nums(radii...))
		}
	}
	c.Canvas.RoundRect(x, y, width, height, radii...)
}

func (c *pen) Arc(x, y, radius, startAngle, endAngle float64, ccw ...bool) {
	c.call("arc", turn(ccw, x, y, radius, startAngle, endAngle)...)
	c.Canvas.Arc(x, y, radius, startAngle, endAngle, ccw...)
}

func (c *pen) Ellipse(x, y, radiusX, radiusY, rotation, startAngle, endAngle float64, ccw ...bool) {
	c.call("ellipse", turn(ccw, x, y, radiusX, radiusY, rotation, startAngle, endAngle)...)
	c.Canvas.Ellipse(x, y, radiusX, radiusY, rotation, startAngle, endAngle, ccw...)
}

func (c *pen) Fill() {
	if s := c.sync(); s != nil {
		s.stmt("c.fill(%s)", c.rule())
	}
	c.Canvas.Fill()
}

func (c *pen) Stroke() { c.call("stroke"); c.Canvas.Stroke() }

func (c *pen) Clip() {
	if s := c.sync(); s != nil {
		s.stmt("c.clip(%s)", c.rule())
	}
	c.Canvas.Clip()
}

func (c *pen) FillPath(p *Path2D)   { c.withPath("fill", p, c.rule()); c.Canvas.FillPath(p) }
func (c *pen) StrokePath(p *Path2D) { c.withPath("stroke", p, ""); c.Canvas.StrokePath(p) }
func (c *pen) ClipPath(p *Path2D)   { c.withPath("clip", p, c.rule()); c.Canvas.ClipPath(p) }

func (c *pen) FillRect(x, y, width, height float64) {
	c.call("fillRect", x, y, width, height)
	c.Canvas.FillRect(x, y, width, height)
}

func (c *pen) StrokeRect(x, y, width, height float64) {
	c.call("strokeRect", x, y, width, height)
	c.Canvas.StrokeRect(x, y, width, height)
}

func (c *pen) ClearRect(x, y, width, height float64) {
	c.call("clearRect", x, y, width, height)
	c.Canvas.ClearRect(x, y, width, height)
}

func (c *pen) SetLineDash(segments ...float64) {
	if s := c.sync(); s != nil {
		s.stmt("c.setLineDash([%s])", nums(segments...))
	}
	c.Canvas.SetLineDash(segments...)
}

func (c *pen) write(method, s string, x, y float64, maxWidth []float64) {
	if js := c.sync(); js != nil {
		y += linuxShift(c.Font, c.TextBaseline)
		js.stmt("c.%s(%s, %s)", method, jsString(s), nums(append([]float64{x, y}, maxWidth...)...))
	}
}

// linuxShift is how much further down Chromium is told to write a text than
// Tela, for what it does on Linux, only there: with glyphs at fractions of a
// pixel (textRendering: geometricPrecision), the box of a font whose descent
// was rounded down is a pixel higher (FontMetrics::AscentDescentWithHacks),
// and with it the baselines that are parts of the box, which are those that
// the font does not tell.
func linuxShift(f CanvasFont, b TextBaseline) float64 {
	f = fullFont(f)
	ts := text.TextStyle{Families: ParseFamilies(f.Family), Size: float32(f.Size), Weight: float32(f.Weight), Stretch: 100}
	face := testFonts.StyleFace(&ts)
	if !descentRoundsDown(face.Metrics(), f.Size) {
		return 0
	}
	switch b {
	case BaselineHanging:
		if _, ok := face.Baseline("hang"); !ok {
			return hanging
		}
	case BaselineIdeographic:
		if _, ok := face.Baseline("ideo"); !ok {
			return 1
		}
	}
	return 0
}

func (c *pen) FillText(s string, x, y float64, maxWidth ...float64) {
	c.write("fillText", s, x, y, maxWidth)
	c.Canvas.FillText(s, x, y, maxWidth...)
}

func (c *pen) StrokeText(s string, x, y float64, maxWidth ...float64) {
	c.write("strokeText", s, x, y, maxWidth)
	c.Canvas.StrokeText(s, x, y, maxWidth...)
}

func (c *pen) place(img image.Image, args ...float64) {
	if s := c.sync(); s != nil {
		s.stmt("c.drawImage(%s, %s)", s.image(img), nums(args...))
	}
}

func (c *pen) DrawImage(img image.Image, x, y float64) {
	c.place(img, x, y)
	c.Canvas.DrawImage(img, x, y)
}

func (c *pen) DrawImageSize(img image.Image, x, y, width, height float64) {
	c.place(img, x, y, width, height)
	c.Canvas.DrawImageSize(img, x, y, width, height)
}

func (c *pen) DrawImageRegion(img image.Image, sx, sy, sWidth, sHeight, dx, dy, dWidth, dHeight float64) {
	c.place(img, sx, sy, sWidth, sHeight, dx, dy, dWidth, dHeight)
	c.Canvas.DrawImageRegion(img, sx, sy, sWidth, sHeight, dx, dy, dWidth, dHeight)
}

func (c *pen) CreatePattern(img image.Image, repeat PatternRepeat) *CanvasPattern {
	p := c.Canvas.CreatePattern(img, repeat)
	if c.js != nil {
		c.js.tiles[p] = c.js.image(img)
	}
	return p
}
