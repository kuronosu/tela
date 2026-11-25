package canvas

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/testutil/chromium"
	"github.com/kuronosu/tela/internal/text"
)

// The numbers of Chromium's canvas, kept as JSON: measureText, and whether
// points are in a path or in its line. Regenerate them with
//
//	TELA_CHROMIUM=1 go test -run TestChromiumNumbers ./internal/canvas/

// measured is one measureText.
type measured struct {
	family       string
	size, weight float64
	italic       bool
	spacing      float64
	text         string
	align        TextAlign
	baseline     TextBaseline
	rtl          bool
}

func (m *measured) String() string {
	return fmt.Sprintf("%s %g/%g %q %s %s spacing %g rtl %v italic %v", m.family, m.size, m.weight, m.text, alignNames[m.align], baselineNames[m.baseline], m.spacing, m.rtl, m.italic)
}

var numberFamilies = []string{"Inter", "JetBrains Mono", "Source Serif 4", "Noto Sans Arabic", "Noto Color Emoji"}

func measuredList() []measured {
	var out []measured
	texts := []string{"", " ", "  a b  ", "Hxgy", "AVATo", "ffi fl ->", "مرحبا", "😀", "a👩‍🚀b🎉", "abc مرحبا 12", "e\u0301Å", ".", "W", "j"}
	for _, b := range baselineList {
		for _, size := range []float64{16, 25} {
			out = append(out, measured{family: "Noto Sans JP", size: size, weight: 400, text: "日本", baseline: b})
		}
	}
	for _, f := range numberFamilies {
		for _, size := range []float64{13, 16, 22.5, 40} {
			for _, s := range texts {
				out = append(out, measured{family: f, size: size, weight: 400, text: s})
			}
			for _, b := range baselineList {
				out = append(out, measured{family: f, size: size, weight: 400, text: "Hx", baseline: b})
			}
		}
		for _, a := range []TextAlign{TextAlignStart, TextAlignEnd, TextAlignLeft, TextAlignRight, TextAlignCenter} {
			for _, rtl := range []bool{false, true} {
				for _, s := range []string{"Hxg", "مرحبا", "ab!"} {
					out = append(out, measured{family: f, size: 20, weight: 400, text: s, align: a, rtl: rtl})
				}
			}
		}
		for _, w := range []float64{100, 350, 700, 900} {
			out = append(out, measured{family: f, size: 24, weight: w, text: "Weight"})
		}
		out = append(out, measured{family: f, size: 24, weight: 400, italic: true, text: "fjord"})
		for _, sp := range []float64{3, -2, 0.4} {
			for _, s := range []string{"Hxg", "ffi", "a b", "مرحبا", "e\u0301a", ""} {
				out = append(out, measured{family: f, size: 24, weight: 400, text: s, spacing: sp})
			}
		}
	}
	return out
}

// The 12 numbers of a TextMetrics, in the order of metricNames.
var metricNames = []string{
	"width", "actualBoundingBoxLeft", "actualBoundingBoxRight", "actualBoundingBoxAscent", "actualBoundingBoxDescent",
	"fontBoundingBoxAscent", "fontBoundingBoxDescent", "emHeightAscent", "emHeightDescent",
	"hangingBaseline", "alphabeticBaseline", "ideographicBaseline",
}

func (m TextMetrics) list() [12]float64 {
	return [12]float64{
		m.Width, m.ActualBoundingBoxLeft, m.ActualBoundingBoxRight, m.ActualBoundingBoxAscent, m.ActualBoundingBoxDescent,
		m.FontBoundingBoxAscent, m.FontBoundingBoxDescent, m.EmHeightAscent, m.EmHeightDescent,
		m.HangingBaseline, m.AlphabeticBaseline, m.IdeographicBaseline,
	}
}

// hitShape is a path whose points are asked for, on a grid of hitGrid.
type hitShape struct {
	name string
	draw func(c *pen)
}

// hitGrid are the points asked, along each axis (a bit further down, see
// hitRow): none is on an edge of the shapes.
var hitGrid = func() (out []float64) {
	for v := -2.13; v < 102; v += 2.7 {
		out = append(out, math.Round(v*100)/100)
	}
	return out
}()

// hitRow returns the y of a row of the grid.
func hitRow(i int) float64 { return hitGrid[i] + 0.37 }

func hitShapes() []hitShape {
	var out []hitShape
	for _, page := range [][]cell{curveCells(), lineCells()} {
		for _, c := range page {
			if strings.HasPrefix(c.name, "strokeRect") || strings.HasPrefix(c.name, "far") || strings.HasPrefix(c.name, "huge") {
				continue // no path, and nothing more to ask
			}
			out = append(out, hitShape(c))
		}
	}
	return append(out,
		hitShape{"rings", func(c *pen) { c.LineWidth = 8; c.Arc(50, 50, 40, 0, 7); c.Arc(50, 50, 20, 0, 7) }},
		hitShape{"scaled line", func(c *pen) {
			c.LineWidth = 8
			c.Scale(2, 0.5)
			c.Arc(25, 100, 15, 0, 5)
		}},
		hitShape{"turned", func(c *pen) { c.LineWidth = 8; c.Translate(50, 50); c.Rotate(0.5); c.Rect(-30, -30, 60, 40) }},
		hitShape{"nothing", func(c *pen) {}},
		hitShape{"point", func(c *pen) { c.MoveTo(50, 50) }},
	)
}

// chromiumNumbers is the stored file.
type chromiumNumbers struct {
	Metrics [][12]float64
	// Hits has, for each shape, the answers for the points of the grid, row
	// by row: isPointInPath, isPointInPath("evenodd") and isPointInStroke,
	// as the bits of a digit.
	Hits []string
}

// hits asks c's path.
func hits(c *Canvas) string {
	var b strings.Builder
	for row := range hitGrid {
		y := hitRow(row)
		for _, x := range hitGrid {
			n := 0
			c.FillRule = gfx.NonZero
			if c.IsPointInPath(x, y) {
				n |= 1
			}
			c.FillRule = gfx.EvenOdd
			if c.IsPointInPath(x, y) {
				n |= 2
			}
			if c.IsPointInStroke(x, y) {
				n |= 4
			}
			b.WriteByte(byte('0' + n))
		}
	}
	return b.String()
}

func numbersHTML() string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n%s@font-face { font-family: \"Noto Sans JP\"; src: url(\"../../../text/testdata/fonts/NotoSansJP-Subset.otf\"); }\n</style></head><body><canvas id=\"canvas\"></canvas><pre id=\"out\"></pre>\n<script>\n", chromiumFonts)
	b.WriteString("const out = {Metrics: [], Hits: []}, grid = [" + nums(hitGrid...) + "], k = 1;\n")
	fmt.Fprintf(&b, "const names = %s;\n", func() string { j, _ := json.Marshal(metricNames); return string(j) }())
	b.WriteString(`const cv = document.getElementById("canvas");
cv.width = cv.height = 100;
let c = cv.getContext("2d"), g;
function measure(font, spacing, text, align, baseline, dir) {
  c.font = font; c.letterSpacing = spacing + "px"; c.textAlign = align; c.textBaseline = baseline; c.direction = dir;
  const m = c.measureText(text);
  out.Metrics.push(names.map(n => Math.round(m[n] * 1e4) / 1e4));
}
function hit(draw) {
  c.reset();
  draw();
  let s = "";
  for (const y of grid.map(y => y + 0.37)) for (const x of grid)
    s += (c.isPointInPath(x, y) ? 1 : 0) + (c.isPointInPath(x, y, "evenodd") ? 2 : 0) + (c.isPointInStroke(x, y) ? 4 : 0);
  out.Hits.push(s);
}
Promise.all([...document.fonts].map(f => f.load())).then(() => {
c.textRendering = "geometricPrecision";
`)
	for _, m := range measuredList() {
		dir := "ltr"
		if m.rtl {
			dir = "rtl"
		}
		fmt.Fprintf(&b, "measure(%s, %s, %s, %q, %q, %q);\n",
			jsString(cssFont(CanvasFont{Family: m.family, Size: m.size, Weight: m.weight, Italic: m.italic})),
			num(m.spacing), jsString(m.text), alignNames[m.align], baselineNames[m.baseline], dir)
	}
	for _, s := range hitShapes() {
		js := newScript(text.DirectionLTR)
		var r Recorder
		r.Record(textFrame(100, 100, 0, 1), func(c *Canvas) {
			p := &pen{Canvas: c, js: js}
			c.FillStyle, c.StrokeStyle = sky, indigo
			s.draw(p)
			p.sync()
		})
		fmt.Fprintf(&b, "hit(() => {\n%s});\n", js.String())
	}
	b.WriteString(`document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script></body></html>
`)
	return b.String()
}

func TestChromiumNumbers(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	page, _ := filepath.Abs(filepath.Join(dir, "numbers.html"))
	file := filepath.Join(dir, "numbers.json")
	html := numbersHTML()
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := os.WriteFile(page, []byte(html), 0o644); err != nil {
			t.Fatal(err)
		}
		var n chromiumNumbers
		chromium.DumpJSON(t, "file://"+page, 100, 100, 1, &n)
		b, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if stored, err := os.ReadFile(page); err != nil || string(stored) != html {
		t.Fatalf("%s is out of date: regenerate it with TELA_CHROMIUM=1", page)
	}
	var want chromiumNumbers
	if b, err := os.ReadFile(file); err != nil {
		t.Fatal(err)
	} else if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	t.Run("MeasureText", func(t *testing.T) { compareMetrics(t, want.Metrics) })
	t.Run("IsPointIn", func(t *testing.T) { compareHits(t, want.Hits) })
}

// The tolerances of the metrics.
const (
	// Inter and Source Serif have optical sizes, which Tela's faces have in
	// whole pixels (the 22.5 of the list), and go-text rounds the advances
	// of a variable font to font units: at most 1 in 1000 of an em a glyph.
	widthTolerance = 0.25
	// The box of what a text paints is that of its outlines, which Chromium
	// rounds outwards to pixels, glyph by glyph (the bounds of their masks;
	// on a Mac it does not).
	boxTolerance = 1.15
	// The rest is arithmetic.
	exactTolerance = 0.002
)

func compareMetrics(t *testing.T, want [][12]float64) {
	list := measuredList()
	if len(list) != len(want) {
		t.Fatalf("%d texts, Chromium measured %d", len(list), len(want))
	}
	fonts := numberFonts(t)
	var worst [12]float64
	for i, m := range list {
		var got TextMetrics
		f := textFrame(100, 100, text.DirectionLTR, 1)
		f.Fonts = fonts
		if m.rtl {
			f.Dir = text.DirectionRTL
		}
		var r Recorder
		r.Record(f, func(c *Canvas) {
			c.Font = CanvasFont{Family: m.family, Size: m.size, Weight: m.weight, Italic: m.italic}
			c.LetterSpacing, c.TextAlign, c.TextBaseline = m.spacing, m.align, m.baseline
			got = c.MeasureText(m.text)
		})
		// Tela is as the systems that are not Linux: see linuxShift.
		face := fonts.StyleFace(&text.TextStyle{Families: ParseFamilies(m.family), Size: float32(m.size), Weight: float32(m.weight), Stretch: 100})
		if fm := face.Metrics(); descentRoundsDown(fm, m.size) {
			got = m.hacked(got)
		}
		for k, v := range got.list() {
			d := math.Abs(v - want[i][k])
			tol := exactTolerance
			switch {
			case k == 0:
				tol = widthTolerance
			case k < 5:
				tol = boxTolerance
			case k == 7 || k == 8:
				continue // Chromium 155 has no emHeightAscent and emHeightDescent: the baselines tell
			}
			worst[k] = max(worst[k], d)
			if d > tol {
				t.Errorf("%v: %s %.4f, Chromium %.4f", &m, metricNames[k], v, want[i][k])
			}
		}
		// Chromium's box holds Tela's.
		for k := 1; k < 5; k++ {
			if v := got.list()[k]; m.text != "" && v > want[i][k]+widthTolerance {
				t.Errorf("%v: %s %.4f, Chromium's %.4f is smaller", &m, metricNames[k], v, want[i][k])
			}
		}
	}
	for k, n := range metricNames {
		t.Logf("%s: at most %.4f from Chromium's", n, worst[k])
	}
}

// numberFonts are the deterministic fonts and one with baselines (BASE).
func numberFonts(t *testing.T) *text.Collection {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "text", "testdata", "fonts", "NotoSansJP-Subset.otf"))
	if err != nil {
		t.Fatal(err)
	}
	c := stdfonts.NewCollection()
	if err := c.AddSource(&text.FontSource{Family: "Noto Sans JP", Data: [][]byte{data}}); err != nil {
		t.Fatal(err)
	}
	return c
}

// descentRoundsDown reports whether Blink moves the baseline of a font (see
// linuxShift).
func descentRoundsDown(fm text.Metrics, size float64) bool {
	d := float64(fm.Descent) * size
	return math.Floor(d+0.5) < d-1e-6 && float64(fm.Ascent)*size >= 1
}

// hacked is what g would be with the baseline a pixel higher in the font's
// box.
func (m *measured) hacked(g TextMetrics) TextMetrics {
	dy := -g.AlphabeticBaseline // of the point measured from, above the alphabetic baseline
	asc, hang, ideo := g.FontBoundingBoxAscent+dy, g.HangingBaseline+dy, g.IdeographicBaseline+dy
	newHang, newIdeo := hang, ideo
	if math.Abs(hang-hanging*asc) < 1e-9 { // the font does not tell
		newHang = hanging * (asc - 1)
	}
	if math.Abs(ideo+g.FontBoundingBoxDescent-dy) < 1e-9 {
		newIdeo = ideo - 1
	}
	up := 0.0
	switch m.baseline {
	case BaselineHanging:
		up = newHang - hang
	case BaselineIdeographic:
		up = newIdeo - ideo
	}
	g.FontBoundingBoxAscent -= 1 + up
	g.FontBoundingBoxDescent += 1 + up
	g.ActualBoundingBoxAscent -= up
	g.ActualBoundingBoxDescent += up
	g.HangingBaseline = newHang - dy - up
	g.IdeographicBaseline = newIdeo - dy - up
	g.AlphabeticBaseline -= up
	return g
}

func compareHits(t *testing.T, want []string) {
	shapes := hitShapes()
	if len(shapes) != len(want) {
		t.Fatalf("%d shapes, Chromium was asked for %d", len(shapes), len(want))
	}
	for i, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			recordText(100, 100, 0, func(c *Canvas) {
				c.FillStyle, c.StrokeStyle = sky, indigo
				s.draw(&pen{Canvas: c})
				got := hits(c)
				n := [3]int{}
				// near reports whether a point is close to the edge of what in
				// tells the inside of.
				near := func(in func(x, y float64) bool, x, y, tolerance float64) bool {
					for a := range 8 {
						v := rotation(float64(a) * math.Pi / 4)
						if in(x+tolerance*v.a, y+tolerance*v.b) != in(x, y) {
							return true
						}
					}
					return false
				}
				var first [3]string
				for j := range got {
					x, y := hitGrid[j%len(hitGrid)], hitRow(j/len(hitGrid))
					for k := range 3 {
						if (got[j]^want[i][j])>>k&1 != 0 {
							c.FillRule = []FillRule{gfx.NonZero, gfx.EvenOdd, gfx.NonZero}[k]
							if k == 2 && near(c.IsPointInStroke, x, y, strokeHitTolerance) || k != 2 && near(c.IsPointInPath, x, y, hitTolerance) {
								continue
							}
							if n[k] == 0 {
								first[k] = fmt.Sprintf("(%g, %g)", x, y)
							}
							n[k]++
						}
					}
				}
				for k, name := range []string{"IsPointInPath", "IsPointInPath, even-odd", "IsPointInStroke"} {
					if k == 2 && hitsKept[s.name] != "" {
						if n[k] == 0 {
							t.Error("the points of the line are the same as Chromium's: not a difference any more")
						}
						continue
					}
					if n[k] > 0 {
						t.Errorf("%s differs at %d of %d points, the first %s", name, n[k], len(got), first[k])
					}
				}
			})
		})
	}
}

// How far from the edge of a path and of a line Chromium may be: an arc is
// conics there and cubics here, and the line of a curve is made of a few
// parabolas, along which dashes are measured to half a pixel.
const (
	hitTolerance       = 0.01
	strokeHitTolerance = 0.5
)

// hitsKept are the lines whose points are others, and why: see
// chromiumBounds.
var hitsKept = map[string]string{
	"zero line":        "Chromium's isPointInStroke counts the dot that its stroke does not draw",
	"zero line square": "the same",
	"dot":              "the same",
	"wide circle":      "a hole in a line wider than the circle it is of",
	"dash dots square": "the squares of dashes of no length are upright",
	"dash longer":      "no corner where a dash as long as a closed line ends",
}
