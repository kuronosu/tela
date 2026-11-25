package canvas

import (
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/svg"
	"github.com/kuronosu/tela/internal/text"
)

// Text (the "text preparation algorithm" of the HTML Standard): one line,
// shaped by package text in the fonts of the element's tree, with the style
// of the element but for what the fields of the Canvas set. Nothing of a
// paragraph applies: white space is kept, and the text's transform,
// decoration and line height are the element's alone.
//
// What was shaped is kept (textCache): a draw that writes the same labels as
// the one before shapes none.

// CanvasFont is the font of the text of a Canvas. A zero field is the element's.
type CanvasFont struct {
	// Family is a font family, or a list of them as in CSS: "Inter",
	// "ui-monospace, monospace". A name with a comma is in quotes.
	Family string
	// Size is in logical pixels, and Weight from 1 to 1000.
	Size, Weight float64
	Italic       bool
}

// TextAlign is where in a text its x is: [Canvas.TextAlign].
type TextAlign uint8

// The alignments, the browser's.
const (
	TextAlignStart TextAlign = iota
	TextAlignEnd
	TextAlignLeft
	TextAlignRight
	TextAlignCenter
)

// TextBaseline is where in a text its y is: [Canvas.TextBaseline].
type TextBaseline uint8

// The baselines, the browser's.
const (
	BaselineAlphabetic TextBaseline = iota
	BaselineTop
	BaselineHanging
	BaselineMiddle
	BaselineIdeographic
	BaselineBottom
)

// TextMetrics are the sizes of a text, in the units of the canvas's transform
// (logical pixels without one): what [Canvas.MeasureText] returns. Each one
// is a distance from the point that the text would be drawn at: the fields
// of the two sides and those called Ascent grow away from it to the left or
// the right and up, those called Descent down, and the baselines are above it
// when positive.
type TextMetrics struct {
	// Width is the advance of the text, what the next text goes after.
	Width float64
	// The box of what the text paints, which may be larger or smaller than
	// its advance.
	ActualBoundingBoxLeft, ActualBoundingBoxRight     float64
	ActualBoundingBoxAscent, ActualBoundingBoxDescent float64
	// The ascent and the descent of the font, the same for every text.
	FontBoundingBoxAscent, FontBoundingBoxDescent float64
	// The em square of the font.
	EmHeightAscent, EmHeightDescent float64
	// The baselines of the font.
	HangingBaseline, AlphabeticBaseline, IdeographicBaseline float64
}

// FillText writes s on one line with FillStyle, in Font, at the point x, y,
// which TextAlign and TextBaseline place in it. Line breaks and tabs are
// spaces. A maxWidth condenses a text that is wider to that width; one that is
// not above 0 draws nothing.
func (c *Canvas) FillText(s string, x, y float64, maxWidth ...float64) {
	if r := c.use("FillText"); r != nil {
		c.drawText(r, s, x, y, maxWidth, false)
	}
}

// StrokeText draws the outlines of the characters of s with StrokeStyle and
// the line of Stroke, where FillText would write them. Characters of a color
// font (emoji) are drawn as FillText does.
func (c *Canvas) StrokeText(s string, x, y float64, maxWidth ...float64) {
	if r := c.use("StrokeText"); r != nil {
		c.drawText(r, s, x, y, maxWidth, true)
	}
}

// MeasureText returns the sizes of s as FillText would write it, with Font,
// LetterSpacing, TextAlign and TextBaseline as they are now.
func (c *Canvas) MeasureText(s string) TextMetrics {
	r := c.use("MeasureText")
	if r == nil {
		return TextMetrics{}
	}
	t := r.shape(c, s)
	dx, dy := c.anchor(r, t)
	m := TextMetrics{
		Width:                 t.width,
		FontBoundingBoxAscent: t.ascent - dy, FontBoundingBoxDescent: t.descent + dy,
		EmHeightAscent: t.emAscent - dy, EmHeightDescent: t.emDescent + dy,
		HangingBaseline: t.hanging - dy, AlphabeticBaseline: 0 - dy, IdeographicBaseline: t.ideographic - dy,
	}
	if ink := t.ink; !ink.Empty() {
		m.ActualBoundingBoxLeft, m.ActualBoundingBoxRight = dx-float64(ink.X0), float64(ink.X1)-dx
		m.ActualBoundingBoxAscent, m.ActualBoundingBoxDescent = -float64(ink.Y0)-dy, float64(ink.Y1)+dy
	}
	return m
}

// hanging is where the hanging baseline is, as a part of the ascent (as in
// Chromium), for a font that does not tell.
const hanging = 0.8

// anchor returns the point of t that is at the x, y given: to the right of
// its left end, and above its alphabetic baseline.
func (c *Canvas) anchor(r *Recorder, t *shaped) (dx, dy float64) {
	rtl := r.frame.Dir == text.DirectionRTL
	switch a := c.TextAlign; {
	case a == TextAlignCenter:
		dx = float64(t.width / 2)
	case a == TextAlignRight, a == TextAlignEnd && !rtl, a != TextAlignLeft && a != TextAlignEnd && rtl:
		dx = t.width
	}
	switch c.TextBaseline {
	case BaselineTop:
		dy = t.emAscent
	case BaselineHanging:
		dy = t.hanging
	case BaselineMiddle:
		dy = float64((t.emAscent - t.emDescent) / 2)
	case BaselineIdeographic:
		dy = t.ideographic
	case BaselineBottom:
		dy = -t.emDescent
	}
	return dx, dy
}

func (c *Canvas) drawText(r *Recorder, s string, x, y float64, maxWidth []float64, stroke bool) {
	if !finite(x, y) {
		return
	}
	limit := math.Inf(1)
	if len(maxWidth) > 0 {
		if limit = maxWidth[0]; !finite(limit) || limit <= 0 {
			return
		}
	}
	t := r.shape(c, s)
	dx, dy := c.anchor(r, t)
	d := drawing{kind: drawText, text: t}
	squeeze := t.width > limit
	if m := &r.m; !stroke && !squeeze && m.b == 0 && m.c == 0 && m.a == 1 && m.d == 1 {
		// Written where it is, in the canvas's coordinates.
		v := m.vec(x-dx, y+dy)
		d.at = gfx.Point{X: float32(v.x), Y: float32(v.y)}
		k, _ := r.ink(inkFill, c.FillStyle, t.alpha(c), m, c.ImageSmoothing)
		r.drawGlyphs(c, &d, k)
		return
	}
	// In coordinates of its own, with its left end at the origin.
	local := matrix{a: 1, d: 1, e: x - dx, f: y + dy}
	if squeeze {
		local.a = limit / t.width
		local.e = x - float64(dx*local.a)
	}
	d.under, d.m = true, r.m.mul(local)
	g, ok := local.invert()
	if _, inv := d.m.invert(); !ok || !inv {
		return
	}
	if stroke {
		r.strokeText(c, &d, &g)
		return
	}
	k, distorted := r.ink(inkFill, c.FillStyle, t.alpha(c), &g, c.ImageSmoothing)
	if distorted {
		k = r.masked(c, &d, inkFill, c.FillStyle)
	}
	r.drawGlyphs(c, &d, k)
}

// alpha returns the opacity of the paint of t: the canvas's, or 1 when t has
// color glyphs, which no paint fades: drawGlyphs does.
func (t *shaped) alpha(c *Canvas) float32 {
	if t.color {
		return 1
	}
	return c.alpha()
}

// drawGlyphs draws the text of d with ink k, of the opacity shaped.alpha.
func (r *Recorder) drawGlyphs(c *Canvas, d *drawing, k ink) {
	if d.kind == drawText {
		if d.text.color && k.paint != nil {
			// The opacity of the canvas and of the color is that of a layer,
			// for all of the text.
			k.alpha = float32(k.alpha * c.alpha())
			if k.solid && k.color.A < 1 {
				k.alpha = float32(k.alpha * k.color.A)
				k.color.A = 1
				k.paint = k.color
			}
		}
		if len(d.text.runs) == 0 || k.alpha == 0 {
			k = ink{}
		}
	}
	r.draw(c, d, k)
}

// strokeText draws the outlines of the text of d, and its color glyphs as
// they are. g maps the canvas's current coordinates to d's.
func (r *Recorder) strokeText(c *Canvas, d *drawing, g *matrix) {
	t := d.text
	if t.color {
		cd := *d
		if cd.text = t.colorPart(); len(cd.text.runs) > 0 {
			k, distorted := r.ink(inkStroke, c.StrokeStyle, 1, g, c.ImageSmoothing)
			if distorted {
				k = blackInk()
			}
			r.drawGlyphs(c, &cd, k)
		}
	}
	d.kind, d.path = drawStroke, t.outlines()
	if d.path.IsEmpty() && t.color {
		return
	}
	if d.path.IsEmpty() || !(c.LineWidth > 0) || math.IsInf(c.LineWidth, 0) {
		r.draw(c, d, ink{})
		return
	}
	d.line = r.line(c, 1, d.path)
	k, distorted := r.ink(inkStroke, c.StrokeStyle, c.alpha(), g, c.ImageSmoothing)
	switch {
	case distorted:
		k = r.masked(c, d, inkStroke, c.StrokeStyle)
	case !k.solid || k.color.A < 1:
		// Glyph by glyph, as Chromium does, so that what is translucent
		// shows where two of them overlap, as it does for FillText.
		d.parts = t.glyphs
	}
	r.draw(c, d, k)
}

// blackInk paints in black.
func blackInk() ink { return ink{paint: gfx.Black, alpha: 1, color: gfx.Black, solid: true} }

// masked makes d the paint of a style, which is not expressible in d's
// coordinates, through the shape of what d was. It returns the ink, in the
// canvas's current coordinates.
func (r *Recorder) masked(c *Canvas, d *drawing, which int, s Style) ink {
	k, _ := r.ink(which, s, c.alpha(), &identity, c.ImageSmoothing)
	r.b, r.aside = r.aside, r.b
	r.emit(d, blackInk())
	r.b, r.aside = r.aside, r.b
	mask := r.aside.Finish()
	inv, ok := r.m.invert()
	if mask.Len() == 0 || !ok {
		return ink{}
	}
	area := inv.affine().TransformRect(mask.Bounds())
	// (A hair larger: its edges must not show.)
	*d = drawing{kind: drawMask, mask: mask, area: area.Outset(area.Width()/1024 + area.Height()/1024 + 1)}
	return k
}

// ---------------------------------------------------------------------
// Shaping.

// shaped is a text that was shaped: its glyphs, from its left end at 0 and
// with its baseline at 0.
type shaped struct {
	runs  []gfx.DrawGlyphRun
	width float64
	ink   gfx.Rect // what they paint
	color bool     // some may be color glyphs
	// The font's: above and below the baseline, and the baselines above it.
	ascent, descent, emAscent, emDescent float64
	hanging, ideographic                 float64

	outline  *gfx.Path  // outlines, made once
	glyphs   []gfx.Path // with it: the outline of each glyph
	colorful *shaped    // colorPart
}

// textKey is what the shape of a text depends on.
type textKey struct {
	text, family          string
	size, weight, spacing float32
	word, stretch         float32
	style                 text.Style
	numeric               text.NumericVariant
	lang                  string
	dir                   text.Direction
}

// textCache keeps the texts that were shaped, and the icons recorded, for the
// draws to come: at most those of the last two generations, each of up to
// maxTexts. A Recorder is dropped when the window rests.
type textCache struct {
	fonts    *text.Collection
	gen      uint64
	cur, old map[textKey]*shaped
	bytes    int // of the texts in cur
	families map[string][]string
	icons    map[iconKey]*gfx.DisplayList
}

const (
	maxTexts     = 1024
	maxTextBytes = 64 << 10
	maxFamilies  = 64
	maxIcons     = 256
)

type iconKey struct {
	doc   *svg.Doc
	color gfx.Color
}

// begin starts a draw of frame f: what was shaped with other fonts is
// dropped.
func (tc *textCache) begin(f *Frame) {
	if tc.fonts != f.Fonts || tc.gen != f.FontsGen {
		*tc = textCache{fonts: f.Fonts, gen: f.FontsGen, families: tc.families, icons: tc.icons}
	}
}

// icon returns the list of doc with the color of opts.
func (tc *textCache) icon(doc *svg.Doc, opts svg.Options) *gfx.DisplayList {
	k := iconKey{doc, opts.CurrentColor}
	list, ok := tc.icons[k]
	if !ok {
		list = doc.Record(opts)
		if tc.icons == nil || len(tc.icons) >= maxIcons {
			tc.icons = map[iconKey]*gfx.DisplayList{}
		}
		tc.icons[k] = list
	}
	return list
}

// ParseFamilies returns the names of a CSS list of font families.
func ParseFamilies(s string) []string {
	var out []string
	for len(s) > 0 {
		s = strings.TrimLeft(s, " \t\n\r\f,")
		if s == "" {
			break
		}
		var name string
		if q := s[0]; q == '"' || q == '\'' {
			end := strings.IndexByte(s[1:], q)
			if end < 0 {
				end = len(s) - 1
			}
			name, s = s[1:1+end], s[min(end+2, len(s)):]
			if i := strings.IndexByte(s, ','); i >= 0 {
				s = s[i:]
			} else {
				s = ""
			}
		} else if i := strings.IndexByte(s, ','); i >= 0 {
			name, s = strings.TrimSpace(s[:i]), s[i:]
		} else {
			name, s = strings.TrimSpace(s), ""
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// JoinFamilies returns the list that ParseFamilies takes apart to names.
func JoinFamilies(names []string) string {
	var b strings.Builder
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		switch {
		case strings.Contains(n, `"`):
			b.WriteString("'" + n + "'")
		case strings.ContainsAny(n, ",'"):
			b.WriteString(`"` + n + `"`)
		default:
			b.WriteString(n)
		}
	}
	return b.String()
}

// prepare returns s with its white space as spaces ("replace all ASCII
// whitespace"), and with it the other characters that break lines, which
// Chromium does not break at either.
func prepare(s string) string {
	for _, c := range s {
		switch c {
		case '\t', '\n', '\v', '\f', '\r', 0x85, 0x2028, 0x2029:
			return strings.Map(func(c rune) rune {
				switch c {
				case '\t', '\n', '\v', '\f', '\r', 0x85, 0x2028, 0x2029:
					return ' '
				}
				return c
			}, s)
		}
	}
	return s
}

// shape returns the text s shaped in the font of now.
func (r *Recorder) shape(c *Canvas, s string) *shaped {
	f := &r.frame
	k := textKey{text: s, family: c.Font.Family, dir: f.Dir}
	if k.family == "" {
		k.family = f.Font.Family
	}
	var base text.TextStyle
	if f.Text != nil {
		base = *f.Text
	}
	k.size, k.weight, k.style = base.Size, base.Weight, base.Style
	if v := float32(c.Font.Size); v > 0 && !math.IsInf(float64(v), 0) {
		k.size = v
	}
	if v := c.Font.Weight; v >= 1 && v <= 1000 {
		k.weight = float32(v)
	}
	if c.Font.Italic && k.style == text.StyleNormal {
		k.style = text.StyleItalic
	}
	if v := float64(float32(c.LetterSpacing)); finite(v) {
		k.spacing = float32(v)
	}
	k.word, k.stretch, k.numeric, k.lang = base.WordSpacing, base.Stretch, base.Numeric, base.Language
	tc := &r.text
	if t := tc.cur[k]; t != nil {
		return t
	}
	t := tc.old[k]
	if t == nil {
		t = r.shapeText(k, &base)
	}
	if tc.cur == nil || len(tc.cur) >= maxTexts || tc.bytes > maxTextBytes {
		tc.old, tc.cur, tc.bytes = tc.cur, map[textKey]*shaped{}, 0
	}
	tc.cur[k] = t
	tc.bytes += len(k.text)
	return t
}

func (r *Recorder) shapeText(k textKey, base *text.TextStyle) *shaped {
	tc := &r.text
	ts := *base
	if k.family != "" {
		fam, ok := tc.families[k.family]
		if !ok {
			fam = ParseFamilies(k.family)
			if tc.families == nil || len(tc.families) >= maxFamilies {
				tc.families = map[string][]string{}
			}
			tc.families[k.family] = fam
		}
		ts.Families = fam
	}
	ts.Size, ts.Weight, ts.Style, ts.LetterSpacing = k.size, k.weight, k.style, k.spacing
	ts.Paint, ts.Decoration, ts.Transform, ts.LineHeight, ts.Hyphens = nil, text.TextDecoration{}, text.TransformNone, text.LineHeight{}, text.HyphensManual
	l := text.NewLayout(tc.fonts, &text.Paragraph{
		Text:    prepare(k.text),
		Style:   text.ParagraphStyle{Direction: k.dir, WhiteSpace: text.WhiteSpacePre},
		Default: ts,
	})
	t := &shaped{}
	ln := l.Line(0)
	t.width = float64(ln.Width) + float64(ln.Hanging)
	if tc.fonts != nil {
		if face := tc.fonts.StyleFace(&ts); face != nil {
			size := float64(ts.Size)
			if !(size > 0) {
				size = 16
			}
			// As in Blink (TextMetrics::Update, FontMetrics): the ascent and
			// the descent are whole pixels, and the em square in its units
			// of 1/64, so that texts are on the rows of Chromium's.
			m := face.Metrics()
			t.ascent, t.descent = math.Floor(float64(float64(m.Ascent)*size)+0.5), math.Floor(float64(float64(m.Descent)*size)+0.5)
			a, _ := face.EmBox()
			t.emAscent = math.Floor(float64(float64(float64(a)*size)*64)+0.5) / 64
			t.emDescent = size - t.emAscent
			t.hanging, t.ideographic = float64(t.ascent*hanging), -t.descent
			if v, ok := face.Baseline("hang"); ok {
				t.hanging = float64(float64(v) * size)
			}
			if v, ok := face.Baseline("ideo"); ok {
				t.ideographic = float64(float64(v) * size)
			}
		}
	}
	l.Paint(r.aside, gfx.Point{X: -ln.X, Y: -ln.Baseline})
	list := r.aside.Finish()
	t.ink = list.Bounds()
	for _, cmd := range list.All() {
		run, ok := cmd.(gfx.DrawGlyphRun)
		if !ok {
			continue
		}
		t.runs = append(t.runs, run)
		t.color = t.color || isColor(run.Font)
	}
	return t
}

// isColor reports whether the font may have color glyphs.
func isColor(f gfx.Font) bool {
	face, ok := f.(*text.Face)
	return !ok || face.Typeface().IsColor()
}

// outlines returns the outlines of the glyphs that are not color glyphs.
func (t *shaped) outlines() *gfx.Path {
	if t.outline != nil {
		return t.outline
	}
	whole := &gfx.Path{}
	for _, run := range t.runs {
		color := isColor(run.Font)
		for _, g := range run.Glyphs {
			if color && run.Font.ColorGlyph(g.ID, gfx.Black) != nil {
				continue
			}
			o, ok := run.Font.GlyphOutline(g.ID)
			if !ok {
				continue
			}
			p := &gfx.Path{}
			x, y := run.Origin.X+g.X, g.Y
			at := func(v gfx.Point) gfx.Point {
				return gfx.Point{X: x + float32(v.X*run.Size), Y: y + float32(v.Y*run.Size)}
			}
			for el := range o.Elements() {
				for _, p := range [2]*gfx.Path{p, whole} {
					switch el.Verb {
					case gfx.MoveTo:
						p.MoveTo(at(el.Pts[0]))
					case gfx.LineTo:
						p.LineTo(at(el.Pts[0]))
					case gfx.QuadTo:
						p.QuadTo(at(el.Pts[0]), at(el.Pts[1]))
					case gfx.CubicTo:
						p.CubicTo(at(el.Pts[0]), at(el.Pts[1]), at(el.Pts[2]))
					case gfx.Close:
						p.Close()
					}
				}
			}
			if !p.IsEmpty() {
				t.glyphs = append(t.glyphs, *p)
			}
		}
	}
	t.outline = whole
	return whole
}

// colorPart returns the color glyphs of t alone.
func (t *shaped) colorPart() *shaped {
	if t.colorful != nil {
		return t.colorful
	}
	out := &shaped{width: t.width, color: true}
	for _, run := range t.runs {
		if !isColor(run.Font) {
			continue
		}
		var gs []gfx.Glyph
		for _, g := range run.Glyphs {
			if run.Font.ColorGlyph(g.ID, gfx.Black) != nil {
				gs = append(gs, g)
			}
		}
		if len(gs) > 0 {
			run.Glyphs = gs
			out.runs = append(out.runs, run)
		}
	}
	t.colorful = out
	return out
}
