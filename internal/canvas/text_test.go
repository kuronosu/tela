package canvas

import (
	"fmt"
	"image"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/text"
)

// recordText runs draw on a canvas of w by h with fonts.
func recordText(w, h float64, dir text.Direction, draw func(c *Canvas)) *gfx.DisplayList {
	var r Recorder
	list, _ := r.Record(textFrame(w, h, dir, 1), draw)
	return list
}

// inkOf returns the bounds of the pixels that draw paints.
func inkOf(t *testing.T, w, h int, draw func(c *Canvas)) image.Rectangle {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	raster.Render(img, recordText(float64(w), float64(h), text.DirectionLTR, draw), raster.Options{})
	var out image.Rectangle
	for y := range h {
		for x := range w {
			if img.RGBAAt(x, y).A > 64 {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}

func runsOf(list *gfx.DisplayList) []gfx.DrawGlyphRun {
	var out []gfx.DrawGlyphRun
	for _, c := range list.All() {
		if r, ok := c.(gfx.DrawGlyphRun); ok {
			out = append(out, r)
		}
	}
	return out
}

func TestMeasureText(t *testing.T) {
	recordText(100, 50, text.DirectionLTR, func(c *Canvas) {
		near := func(what string, got, want float64) {
			t.Helper()
			if math.Abs(got-want) > 1e-3 {
				t.Errorf("%s is %g, want %g", what, got, want)
			}
		}
		m := c.MeasureText("Hxp")
		if m.Width < 25 || m.Width > 35 {
			t.Errorf("width %g", m.Width)
		}
		if w := c.widthOf("Hxp", 32); math.Abs(w/m.Width-2) > 0.15 { // the optical size
			t.Errorf("twice the size is %g wide, half of it %g", w, m.Width)
		}
		if m.ActualBoundingBoxAscent < 11 || m.ActualBoundingBoxAscent > 13 || m.ActualBoundingBoxDescent < 2.5 || m.ActualBoundingBoxDescent > 4 {
			t.Errorf("the ink is %g above and %g below", m.ActualBoundingBoxAscent, m.ActualBoundingBoxDescent)
		}
		if l, r := m.ActualBoundingBoxLeft, m.ActualBoundingBoxRight; l > 0 || l < -3 || r < m.Width-3 || r > m.Width {
			t.Errorf("the ink goes from %g to %g of %g", -l, r, m.Width)
		}
		near("the em square", m.EmHeightAscent+m.EmHeightDescent, 16)
		if m.FontBoundingBoxAscent < 14 || m.FontBoundingBoxDescent < 3 || m.AlphabeticBaseline != 0 || math.Signbit(m.AlphabeticBaseline) {
			t.Errorf("%+v", m)
		}
		near("the hanging baseline", m.HangingBaseline, 0.8*m.FontBoundingBoxAscent)
		near("the ideographic baseline", m.IdeographicBaseline, -m.FontBoundingBoxDescent)
		for b, above := range map[TextBaseline]float64{
			BaselineTop: m.EmHeightAscent, BaselineHanging: m.HangingBaseline, BaselineMiddle: (m.EmHeightAscent - m.EmHeightDescent) / 2,
			BaselineIdeographic: m.IdeographicBaseline, BaselineBottom: -m.EmHeightDescent,
		} {
			c.TextBaseline = b
			n := c.MeasureText("Hxp")
			near("the baseline", -n.AlphabeticBaseline, above)
			near("the ascent", n.ActualBoundingBoxAscent, m.ActualBoundingBoxAscent-above)
			near("the descent", n.ActualBoundingBoxDescent, m.ActualBoundingBoxDescent+above)
			near("the font's ascent", n.FontBoundingBoxAscent, m.FontBoundingBoxAscent-above)
			near("the em's descent", n.EmHeightDescent, m.EmHeightDescent+above)
		}
		c.TextBaseline = BaselineAlphabetic
		for a, left := range map[TextAlign]float64{TextAlignCenter: m.Width / 2, TextAlignRight: m.Width, TextAlignEnd: m.Width, TextAlignLeft: 0} {
			c.TextAlign = a
			n := c.MeasureText("Hxp")
			near("the left", n.ActualBoundingBoxLeft, m.ActualBoundingBoxLeft+left)
			near("the right", n.ActualBoundingBoxRight, m.ActualBoundingBoxRight-left)
			near("the width", n.Width, m.Width)
		}
		c.TextAlign = TextAlignStart
		if e := c.MeasureText(""); e.Width != 0 || e.ActualBoundingBoxRight != 0 || e.FontBoundingBoxAscent != m.FontBoundingBoxAscent {
			t.Errorf("no text: %+v", e)
		}
		// White space is kept, and is spaces.
		sp := c.MeasureText(" ").Width
		for _, s := range []string{"\t", "\n", "\r", "\f", "\v", " ", "\u0085"} {
			near(fmt.Sprintf("%q", s), c.MeasureText("a"+s+"b").Width, c.MeasureText("a b").Width)
		}
		near("three spaces", c.MeasureText("   ").Width, 3*sp)
		near("spaces around", c.MeasureText(" a ").Width, c.MeasureText("a").Width+2*sp)
		c.LetterSpacing = 5
		near("with letter spacing", c.MeasureText("iii").Width, 15+func() float64 { c.LetterSpacing = 0; return c.MeasureText("iii").Width }())
		c.LetterSpacing = nan
		near("with no spacing", c.MeasureText("iii").Width, func() float64 { c.LetterSpacing = 0; return c.MeasureText("iii").Width }())
		// What is not a font's size, weight or family is the element's.
		for _, f := range []CanvasFont{{}, {Size: -3}, {Size: nan}, {Size: inf}, {Weight: -1}, {Weight: 2000}, {Weight: nan}, {Family: "Inter, sans-serif", Size: 16, Weight: 400}} {
			c.Font = f
			near(fmt.Sprintf("with %+v", f), c.MeasureText("Hxp").Width, m.Width)
		}
		c.Font = CanvasFont{Family: "monospace"}
		if w := c.MeasureText("iii").Width; math.Abs(w-c.MeasureText("WWW").Width) > 1e-3 {
			t.Error("monospace is not")
		}
		c.Font = CanvasFont{Family: "nothing, 'JetBrains Mono', Inter"}
		near("the first family there is", c.MeasureText("iii").Width, func() float64 { c.Font.Family = "monospace"; return c.MeasureText("iii").Width }())
		c.Font = CanvasFont{Weight: 900}
		if w := c.MeasureText("Hxp").Width; w <= m.Width {
			t.Errorf("weight 900 is %g wide, weight 400 %g", w, m.Width)
		}
	})
	// The element's features: tabular numbers.
	var r Recorder
	f := textFrame(100, 50, 0, 1)
	ts := textStyle
	ts.Numeric = text.NumericTabular
	f.Text = &ts
	r.Record(f, func(c *Canvas) {
		if a, b := c.MeasureText("111").Width, c.MeasureText("888").Width; math.Abs(a-b) > 1e-3 {
			t.Errorf("tabular-nums: %g and %g", a, b)
		}
	})
}

// widthOf returns the width of s at a size.
func (c *Canvas) widthOf(s string, size float64) float64 {
	old := c.Font.Size
	defer func() { c.Font.Size = old }()
	c.Font.Size = size
	return c.MeasureText(s).Width
}

// Text is drawn where MeasureText says.
func TestTextPlaced(t *testing.T) {
	draw := func(c *Canvas) {
		c.Font.Size = 20
		c.TextAlign, c.TextBaseline = TextAlignRight, BaselineMiddle
		c.FillText("Hg", 70, 30)
	}
	var m TextMetrics
	recordText(100, 60, 0, func(c *Canvas) {
		draw(c)
		m = c.MeasureText("Hg")
	})
	want := image.Rect(int(math.Round(70-m.ActualBoundingBoxLeft)), int(math.Round(30-m.ActualBoundingBoxAscent)),
		int(math.Round(70+m.ActualBoundingBoxRight)), int(math.Round(30+m.ActualBoundingBoxDescent)))
	got := inkOf(t, 100, 60, draw)
	if d := func(a, b int) bool { return a-b > 1 || b-a > 1 }; d(got.Min.X, want.Min.X) || d(got.Min.Y, want.Min.Y) || d(got.Max.X, want.Max.X) || d(got.Max.Y, want.Max.Y) {
		t.Errorf("ink %v, measured %v", got, want)
	}
	// A translation, and the same in a transform that is recorded, agree.
	a := inkOf(t, 100, 60, func(c *Canvas) { c.Translate(10, 20); c.FillText("Hg", 5, 10) })
	b := inkOf(t, 100, 60, func(c *Canvas) { c.Transform(1, 1e-12, 0, 1, 10, 20); c.FillText("Hg", 5, 10) })
	if a != b {
		t.Errorf("%v and %v", a, b)
	}
}

func TestTextMaxWidth(t *testing.T) {
	const s = "Long words"
	var w float64
	recordText(200, 50, 0, func(c *Canvas) { w = c.MeasureText(s).Width })
	full := inkOf(t, 200, 50, func(c *Canvas) { c.FillText(s, 10, 30) })
	if same := inkOf(t, 200, 50, func(c *Canvas) { c.FillText(s, 10, 30, w+1) }); same != full {
		t.Errorf("a width that is enough: %v, want %v", same, full)
	}
	half := inkOf(t, 200, 50, func(c *Canvas) { c.FillText(s, 10, 30, w/2) })
	if half.Min.Y != full.Min.Y || half.Max.Y != full.Max.Y || half.Dx() < full.Dx()/2-2 || half.Dx() > full.Dx()/2+2 || half.Min.X < 9 {
		t.Errorf("half the width: %v, want half of %v", half, full)
	}
	right := inkOf(t, 200, 50, func(c *Canvas) { c.TextAlign = TextAlignRight; c.FillText(s, 150, 30, w/2) })
	if right.Max.X > 151 || right.Max.X < 147 || right.Dx() != half.Dx() && right.Dx() != half.Dx()+1 && right.Dx() != half.Dx()-1 {
		t.Errorf("condensed to the right: %v", right)
	}
	for _, m := range []float64{0, -5, nan, inf, math.Inf(-1)} {
		if list := recordText(200, 50, 0, func(c *Canvas) { c.FillText(s, 10, 30, m); c.StrokeText(s, 10, 30, m) }); list.Len() != 0 {
			t.Errorf("a width of %g draws", m)
		}
	}
	for _, v := range [][2]float64{{nan, 0}, {0, inf}} {
		if list := recordText(200, 50, 0, func(c *Canvas) { c.FillText(s, v[0], v[1]) }); list.Len() != 0 {
			t.Errorf("text at %v draws", v)
		}
	}
	// Gradients stay in place when the text is condensed, those that a
	// squeezed transform distorts too.
	for name, g := range map[string]func(c *Canvas) *CanvasGradient{
		"linear": func(c *Canvas) *CanvasGradient { return c.CreateLinearGradient(0, 0, 100, 0) },
		"conic":  func(c *Canvas) *CanvasGradient { return c.CreateConicGradient(-math.Pi/2, 50, 1000) },
	} {
		for _, stroke := range []bool{false, true} {
			img := image.NewRGBA(image.Rect(0, 0, 100, 50))
			raster.Render(img, recordText(100, 50, 0, func(c *Canvas) {
				s := g(c)
				s.AddColorStop(0.5, red)
				s.AddColorStop(0.5, green)
				if name == "conic" {
					s = g(c)
					s.AddColorStop(0.5, green)
					s.AddColorStop(0.5, red)
				}
				c.FillStyle, c.StrokeStyle = s, s
				c.Font = CanvasFont{Size: 60, Weight: 900}
				c.LineWidth = 12
				if stroke {
					c.StrokeText("MMMM", 0, 46, 100)
				} else {
					c.FillText("MMMM", 0, 46, 100)
				}
			}), raster.Options{})
			reds, greens := 0, 0
			for y := range 50 {
				for x := range 100 {
					switch p := img.RGBAAt(x, y); {
					case p == redPx && x >= 51, p == greenPx && x < 49:
						t.Fatalf("%s, stroke %v: pixel %d,%d is %v", name, stroke, x, y, p)
					case p == redPx:
						reds++
					case p == greenPx:
						greens++
					}
				}
			}
			if reds < 100 || greens < 100 {
				t.Errorf("%s, stroke %v: %d red pixels, %d green", name, stroke, reds, greens)
			}
		}
	}
}

// Text with color glyphs fades as a whole, and its outlines are the others'.
func TestTextColorGlyphs(t *testing.T) {
	list := recordText(200, 50, 0, func(c *Canvas) {
		c.GlobalAlpha = 0.5
		c.FillStyle = gfx.Color{R: 1, A: 0.5}
		c.FillText("a🎈", 10, 30)
	})
	layer, ok := list.At(1).(gfx.PushLayer)
	if !ok || layer.Opacity != 0.25 {
		t.Fatalf("not a layer:\n%s", list)
	}
	for _, r := range runsOf(list) {
		if r.Paint.(gfx.Color) != red {
			t.Errorf("painted with %v", r.Paint)
		}
	}
	if list := recordText(200, 50, 0, func(c *Canvas) { c.FillText("a🎈", 10, 30) }); list.Len() != 4 {
		t.Errorf("text that does not fade:\n%s", list)
	}
	list = recordText(200, 50, 0, func(c *Canvas) { c.StrokeText("a🎈b", 10, 30) })
	strokes := 0
	for _, cmd := range list.All() {
		if _, ok := cmd.(gfx.StrokePath); ok {
			strokes++
		}
	}
	if runs := runsOf(list); len(runs) != 1 || len(runs[0].Glyphs) != 1 || strokes != 1 {
		t.Errorf("strokeText with an emoji:\n%s", list)
	}
	// A shadow has the glyphs' shape, not their colors.
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	raster.Render(img, recordText(60, 60, 0, func(c *Canvas) {
		c.ShadowColor, c.ShadowOffsetX = green, 100
		c.Font.Size = 40
		c.FillText("😀", -92, 45)
	}), raster.Options{})
	if p := img.RGBAAt(30, 30); p != greenPx {
		t.Errorf("the shadow of an emoji: %v", p)
	}
}

// Texts are shaped once while they are drawn, and what is kept is bounded.
func TestTextCache(t *testing.T) {
	var r Recorder
	f := textFrame(100, 50, 0, 1)
	label := func(i int) string { return fmt.Sprintf("label %d", i) }
	draw := func(from, n int) func(c *Canvas) {
		return func(c *Canvas) {
			for i := range n {
				c.FillText(label(from+i), 5, 20)
			}
		}
	}
	r.Record(f, draw(0, 200))
	first := r.text.cur[textKey{text: label(7), family: f.Font.Family, size: 16, weight: 400, stretch: 100}]
	if first == nil || len(r.text.cur) != 200 {
		t.Fatalf("%d texts, label 7 %v", len(r.text.cur), first)
	}
	a, _ := r.Record(f, draw(0, 200))
	if got := r.text.cur[textKey{text: label(7), family: f.Font.Family, size: 16, weight: 400, stretch: 100}]; got != first || len(r.text.cur) != 200 {
		t.Error("shaped again")
	}
	for i := range 20 {
		r.Record(f, draw(i*300, 300))
		if n := len(r.text.cur) + len(r.text.old); n > 2*maxTexts {
			t.Fatalf("%d texts kept", n)
		}
	}
	long := strings.Repeat("word ", 5000)
	for i := range 8 {
		r.Record(f, func(c *Canvas) { c.MeasureText(long + label(i)) })
	}
	if n := len(r.text.cur) + len(r.text.old); n > 8 {
		t.Errorf("%d texts beside long ones", n)
	}
	// Other fonts, or the same resolved anew.
	r.Record(f, draw(0, 5))
	f.FontsGen++
	r.Record(f, draw(0, 1))
	if n := len(r.text.cur) + len(r.text.old); n != 1 {
		t.Errorf("%d texts after the fonts changed", n)
	}
	b, _ := r.Record(f, draw(0, 200))
	if a.Len() != b.Len() || !gfx.Equal(a.At(1), b.At(1)) {
		t.Error("the same text is drawn otherwise")
	}
	f.Fonts = stdfonts.NewCollection()
	r.Record(f, draw(0, 1))
	if n := len(r.text.cur) + len(r.text.old); n != 1 {
		t.Errorf("%d texts with another collection", n)
	}
	// Without any font there are no glyphs.
	f.Fonts = nil
	if list, _ := r.Record(f, draw(0, 1)); list.Len() != 0 {
		t.Errorf("without fonts:\n%s", list)
	}
}

func TestFamilies(t *testing.T) {
	for s, want := range map[string][]string{
		"":                         nil,
		"Inter":                    {"Inter"},
		" Open Sans , serif,":      {"Open Sans", "serif"},
		`"A, B", 'C "D"', sans`:    {"A, B", `C "D"`, "sans"},
		`"unclosed`:                {"unclosed"},
		`'x`:                       {"x"},
		`,, "", a`:                 {"a"},
		"ui-monospace,\tmonospace": {"ui-monospace", "monospace"},
	} {
		if got := ParseFamilies(s); !slices.Equal(got, want) {
			t.Errorf("%q: %q, want %q", s, got, want)
		}
	}
	for _, names := range [][]string{nil, {"Inter"}, {"A, B", `C "D"`, "It's"}, {"ui-sans-serif", "system-ui"}} {
		if got := ParseFamilies(JoinFamilies(names)); !slices.Equal(got, names) {
			t.Errorf("%q: %q", names, got)
		}
	}
}

// labels draws 200 texts that do not change, as the axes and the cells of a
// chart that moves.
func labels(c *Canvas) {
	for i := range 200 {
		c.FillText(labelTexts[i], float64(i%10)*40, 15+float64(i/10)*15)
	}
	c.FillRect(math.Mod(c.Time.Seconds(), 1), 0, 5, 5)
}

var labelTexts = func() (out [200]string) {
	for i := range out {
		out[i] = fmt.Sprintf("label %d", i)
	}
	return out
}()

// BenchmarkLabels200 is a draw of 200 labels that were drawn before.
func BenchmarkLabels200(b *testing.B) {
	var r Recorder
	f := textFrame(400, 300, 0, 1)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		f.Time = time.Duration(i) * time.Millisecond
		if list, _ := r.Record(f, labels); list.Len() < 200 {
			b.Fatalf("%d commands", list.Len())
		}
	}
}

// BenchmarkLabelsShape200 shapes them.
func BenchmarkLabelsShape200(b *testing.B) {
	var r Recorder
	f := textFrame(400, 300, 0, 1)
	b.ReportAllocs()
	for b.Loop() {
		f.FontsGen++
		r.Record(f, labels)
	}
}

// BenchmarkArcs2000 is a draw of 2000 particles, each a circle.
func BenchmarkArcs2000(b *testing.B) {
	var r Recorder
	f := frameOf(400, 300)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		f.Time = time.Duration(i) * time.Millisecond
		r.Record(f, func(c *Canvas) {
			for i := range 2000 {
				c.BeginPath()
				c.Arc(float64(i*37%400)+math.Mod(c.Time.Seconds(), 1), float64(i*53%300), 3, 0, 2*math.Pi)
				c.Fill()
			}
		})
	}
}

// Labels that were drawn cost their commands.
func TestLabelAllocations(t *testing.T) {
	var r Recorder
	f := textFrame(400, 300, 0, 1)
	r.Record(f, labels)
	// A run and its glyphs each.
	if got, limit := testing.AllocsPerRun(5, func() { r.Record(f, labels) }), 2*200.0+24; got > limit {
		t.Errorf("%g allocations for 200 labels, want at most %g", got, limit)
	}
}
