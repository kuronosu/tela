package main

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Phase 3 asks for the Counter of ARCHITECTURE section 1 to render like its
// HTML + Tailwind equivalent. These tests compare Tela's layout and pixels
// with Chromium's (see chromium_test.go for the reference files).

// laidOut is Counter rendered by Tela, like `tela snapshot` and `tela
// inspect` do it.
type laidOut struct {
	root core.InspectNode
	list *gfx.DisplayList
	img  *image.RGBA
}

func renderCounter(scale float32) laidOut {
	tree := core.NewTree(core.Config{})
	tree.SetViewport(gfx.Size{W: counterW, H: counterH})
	tree.SetScale(scale)
	tree.Mount(core.Component(Preview))
	dl, _ := tree.Frame()
	return laidOut{tree.Inspect(), dl, tree.Rasterize(dl, scale)}
}

// parts returns the card, the heading and the button of the layout tree.
func (l laidOut) parts(t testing.TB) (card, h1, button core.InspectNode) {
	t.Helper()
	if len(l.root.Children) != 1 {
		t.Fatalf("root has %d children, want the card", len(l.root.Children))
	}
	card = l.root.Children[0]
	if card.Kind != "div" || len(card.Children) != 2 || card.Children[0].Kind != "h1" || card.Children[1].Kind != "button" {
		t.Fatalf("unexpected tree: card %q with %d children", card.Kind, len(card.Children))
	}
	return card, card.Children[0], card.Children[1]
}

// glyphRuns returns the baseline origin of every glyph run whose baseline is
// inside rect, in drawing order.
func (l laidOut) glyphRuns(rect gfx.Rect) []gfx.Point {
	var out []gfx.Point
	for _, c := range l.list.All() {
		if run, ok := c.(gfx.DrawGlyphRun); ok && run.Origin.Y >= rect.Y0 && run.Origin.Y <= rect.Y1 {
			out = append(out, run.Origin)
		}
	}
	return out
}

// TestCounterHTML checks that counter.html is what chromium_test.go
// generates and that its classes are Counter's, so that the reference page
// is the same tree.
func TestCounterHTML(t *testing.T) {
	got, err := os.ReadFile(filepath.Join(chromiumDir, "counter.html"))
	if err != nil || string(got) != counterHTML() {
		t.Fatal("testdata/chromium/counter.html is out of date; regenerate with TELA_CHROMIUM=1 (see chromium_test.go)")
	}
	layout, err := os.ReadFile(filepath.Join(chromiumDir, "layout.html"))
	if err != nil || string(layout) != layoutHTML() {
		t.Fatal("testdata/chromium/layout.html is out of date; regenerate with TELA_CHROMIUM=1 (see chromium_test.go)")
	}
	card, h1, button := renderCounter(1).parts(t)
	var want []string
	for _, n := range []core.InspectNode{card, h1, button} {
		want = append(want, n.Style.Classes)
	}
	var classes []string
	for _, m := range regexp.MustCompile(`class="([^"]*)"`).FindAllStringSubmatch(counterMarkup, -1) {
		classes = append(classes, m[1])
	}
	if strings.Join(classes, "|") != strings.Join(want, "|") {
		t.Errorf("classes in the HTML %q, in Counter %q", classes, want)
	}
	for _, text := range []string{"Count: 0", "Increment"} {
		if !strings.Contains(counterMarkup, ">"+text+"<") {
			t.Errorf("the HTML has no %q", text)
		}
	}
}

// edgeTolerance is the largest difference, in CSS px, accepted between an
// edge of Chromium's layout and Tela's, baselines included. Measured: at
// most 0.008 (Chromium snaps to 1/64 px, LayoutUnit). At scale 2 both
// round the font's ascent and descent in device pixels (ADR-159; the
// heading's ascent is 93 device px, 46.5 CSS px): the heading's baseline
// is at 81.5 CSS px and the button's at 141.5 in both. Before ADR-159 Tela
// quantized line metrics in logical pixels and both texts were one device
// pixel off.
const edgeTolerance = 0.02

// edges are the left, top, right and bottom of a box, or the left, baseline
// and right of a text range.
type edges []float64

func ltrb(r [4]float64) edges { return edges{r[0], r[1], r[0] + r[2], r[1] + r[3]} }

func rectEdges(r gfx.Rect) edges {
	return edges{float64(r.X0), float64(r.Y0), float64(r.X1), float64(r.Y1)}
}

func (e edges) String() string {
	parts := make([]string, len(e))
	for i, v := range e {
		parts[i] = fmt.Sprintf("%8.3f", v)
	}
	return strings.Join(parts, " ")
}

func TestCounterLayoutChromium(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(chromiumDir, "layout.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1, see chromium_test.go)", err)
	}
	var ref chromiumFile
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("edges in CSS px: Chromium / Tela / difference\n")
	for i, c := range []chromiumLayout{ref.X1, ref.X2} {
		scale := float32(i + 1)
		l := renderCounter(scale)
		card, h1, button := l.parts(t)
		// The text range: its left edge is where the line starts and its
		// right edge is the box's, whose width is the text's advance (both
		// are flex items that shrink to their content, and the button's
		// padding is left out). The baseline is the top of Chromium's range
		// plus the quantized font ascent, and the origin of Tela's glyph
		// run.
		textOf := func(n core.InspectNode, padLeft, padRight float64) (left, baseline, right float64) {
			runs := l.glyphRuns(n.Rect)
			if len(runs) != 1 {
				t.Fatalf("x%g: %d glyph runs in the %s, want 1", scale, len(runs), n.Kind)
			}
			return float64(runs[0].X), float64(runs[0].Y), float64(n.Rect.X1) - padRight
		}
		hl, hb, hr := textOf(h1, 0, 0)
		bl, bb, br := textOf(button, 0, float64(button.Padding.Right))
		type pair struct {
			name, edges string
			want, got   edges
		}
		pairs := []pair{
			{"div", "left top right bottom", ltrb(c.Card), rectEdges(card.Rect)},
			{"h1", "left top right bottom", ltrb(c.H1), rectEdges(h1.Rect)},
			{"h1 text", "left baseline right", edges{c.Text[0], c.Text[1] + c.Ascent, c.Text[0] + c.Text[2]}, edges{hl, hb, hr}},
			{"button", "left top right bottom", ltrb(c.Button), rectEdges(button.Rect)},
			{"btn text", "left baseline right", edges{c.ButtonText[0], c.ButtonText[1] + c.ButtonAscent, c.ButtonText[0] + c.ButtonText[2]}, edges{bl, bb, br}},
		}
		for _, p := range pairs {
			worst := 0.0
			for e := range p.want {
				d := math.Abs(p.want[e] - p.got[e])
				worst = math.Max(worst, d)
				tol := edgeTolerance
				if d > tol {
					t.Errorf("x%g %s (%s): Tela %s, Chromium %s: an edge differs by %.3f px, more than %g", scale, p.name, p.edges, p.got, p.want, d, tol)
				}
			}
			fmt.Fprintf(&sb, "x%g %-8s %-22s Chromium %s | Tela %s | max %.4f\n", scale, p.name, p.edges, p.want, p.got, worst)
		}
	}
	t.Log(sb.String())
}

// A region is a set of pixels of the viewport, defined on logical
// coordinates so that both scales measure the same parts of the image.
type region struct {
	name string
	in   func(x, y float64) bool // the center of a pixel, in logical px
}

// cardRadius is rounded-3xl (1.5rem).
const cardRadius = 24

// edgeBand is the half-width, in logical px, of the band around the card's
// outline that counts as its edge.
const edgeBand = 2

// regionsOf returns the regions of the Counter's picture, disjoint and
// covering the whole image, from Tela's layout: the card's edge band (where
// the outline is anti-aliased), the heading and the button (text and its
// surroundings in their boxes), the card's body (the gradient), the shadow
// below the card and the plain canvas.
func regionsOf(card, h1, button gfx.Rect) []region {
	cx, cy := float64(card.X0+card.X1)/2, float64(card.Y0+card.Y1)/2
	hx, hy := float64(card.X1-card.X0)/2, float64(card.Y1-card.Y0)/2
	sd := func(x, y float64) float64 { // signed distance to the card's outline
		qx, qy := math.Abs(x-cx)-(hx-cardRadius), math.Abs(y-cy)-(hy-cardRadius)
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - cardRadius
	}
	inside := func(r gfx.Rect, grow float64) func(x, y float64) bool {
		return func(x, y float64) bool {
			return x >= float64(r.X0)-grow && x < float64(r.X1)+grow && y >= float64(r.Y0)-grow && y < float64(r.Y1)+grow
		}
	}
	inH1, inButton := inside(h1, 1), inside(button, 1)
	return []region{
		{"heading", func(x, y float64) bool { return inH1(x, y) && sd(x, y) < -edgeBand }},
		{"button", func(x, y float64) bool { return inButton(x, y) && sd(x, y) < -edgeBand }},
		{"card edge", func(x, y float64) bool { return math.Abs(sd(x, y)) <= edgeBand }},
		{"card body", func(x, y float64) bool {
			return sd(x, y) < -edgeBand && !inH1(x, y) && !inButton(x, y)
		}},
		{"shadow", func(x, y float64) bool { return sd(x, y) > edgeBand && y < float64(card.Y1)+100 }},
		{"canvas", func(x, y float64) bool { return sd(x, y) > edgeBand && y >= float64(card.Y1)+100 }},
	}
}

// pixelDiff is the difference between two images over a region, in channel
// levels (0-255) of R, G and B.
type pixelDiff struct {
	max    int     // largest channel difference
	mean   float64 // mean channel difference
	over16 int     // pixels with a channel difference above 16
	pixels int
}

func diffImages(a, b *image.RGBA, scale float64, in func(x, y float64) bool) pixelDiff {
	var d pixelDiff
	sum := 0
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			if in != nil && !in((float64(x)+0.5)/scale, (float64(y)+0.5)/scale) {
				continue
			}
			pa, pb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			m := 0
			for _, v := range [3]int{int(pa.R) - int(pb.R), int(pa.G) - int(pb.G), int(pa.B) - int(pb.B)} {
				v = max(v, -v)
				sum += v
				m = max(m, v)
			}
			d.max = max(d.max, m)
			if m > 16 {
				d.over16++
			}
			d.pixels++
		}
	}
	if d.pixels > 0 {
		d.mean = float64(sum) / float64(3*d.pixels)
	}
	return d
}

// pixelBound is the largest difference accepted in a region, with why.
type pixelBound struct {
	max  int
	mean float64
	why  string
}

// pixelBounds are set from what TestCounterPixelsChromium measured
// (Chromium 152, Tailwind 4.3.3), a little above it so that a regression
// shows. Measured at x1 and x2: whole image max 182/186 mean 0.229/0.213;
// heading 182/151, 2.888/1.050; button 59/148, 0.935/1.045; card edge 32/50,
// 0.342/0.327; card body 2/2, 0.259/0.259; shadow 1/1, 0.500/0.415; canvas 0
// (the whole image at x2 was measured before ADR-159, which put the texts'
// baselines at x2 where Blink's are: heading 186, 1.887 and button 169, 2.235
// before it).
var pixelBounds = map[string]pixelBound{
	"whole": {200, 0.3, "the sum of the regions below"},
	"heading": {200, 3.2, "text: Skia's glyph anti-aliasing differs from ours on glyph edges (the diff image shows a thin " +
		"outline of every glyph)"},
	"button": {170, 1.3, "text, as the heading"},
	"card edge": {60, 0.45, "Skia's rounded-rectangle coverage is not the exact area coverage of our rasterizer " +
		"(single anti-aliased pixels on the corner arcs)"},
	"card body": {4, 0.35, "the gradient: stops are extended sRGB, interpolated and clipped per pixel like Chromium; " +
		"under the backdrop-blurred translucent button too"},
	"shadow": {3, 0.6, "shadow-2xl: the Gaussian blur of the two layers agrees within a few levels (the diff image shows " +
		"banding from rounding)"},
	"canvas": {0, 0, "nothing is drawn there: exactly white"},
}

// TestCounterPixelsChromium compares Tela's render with Chromium's
// screenshots at scale 1 and 2, over the whole image and by region.
func TestCounterPixelsChromium(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("difference from Chromium in channel levels (0-255)\n")
	fmt.Fprintf(&sb, "%-10s %2s %9s %6s %7s %7s\n", "region", "", "pixels", "max", "mean", ">16")
	for _, scale := range []int{1, 2} {
		want := golden.ReadRGBA(t, filepath.Join(chromiumDir, fmt.Sprintf("counter_x%d.png", scale)))
		l := renderCounter(float32(scale))
		got := l.img
		if got.Rect != want.Rect {
			t.Fatalf("x%d: Tela's image is %v, Chromium's %v", scale, got.Rect, want.Rect)
		}
		card, h1, button := l.parts(t)
		type row struct {
			name string
			d    pixelDiff
		}
		rows := []row{{"whole", diffImages(got, want, float64(scale), nil)}}
		covered := 0
		for _, r := range regionsOf(card.Rect, h1.Rect, button.Rect) {
			d := diffImages(got, want, float64(scale), r.in)
			covered += d.pixels
			rows = append(rows, row{r.name, d})
		}
		if covered != rows[0].d.pixels {
			t.Errorf("x%d: the regions cover %d pixels of %d", scale, covered, rows[0].d.pixels)
		}
		failed := false
		for _, r := range rows {
			fmt.Fprintf(&sb, "%-10s x%d %9d %6d %7.3f %7d\n", r.name, scale, r.d.pixels, r.d.max, r.d.mean, r.d.over16)
			b, ok := pixelBounds[r.name]
			if !ok {
				continue
			}
			if r.d.max > b.max || r.d.mean > b.mean {
				failed = true
				t.Errorf("x%d %s: max %d mean %.3f, want at most %d and %.3f (%s)", scale, r.name, r.d.max, r.d.mean, b.max, b.mean, b.why)
			}
		}
		if failed || os.Getenv("TELA_COUNTER_DUMP") == "1" {
			dumpDiff(t, scale, got, want)
		}
	}
	t.Log(sb.String())
}

// dumpDiff writes Tela's image and the difference with Chromium's (every
// channel difference times 8, in gray) to testdata, where the golden helper
// puts its failures too (git ignores both).
func dumpDiff(t *testing.T, scale int, got, want *image.RGBA) {
	d := image.NewRGBA(got.Rect)
	for i := 0; i < len(d.Pix); i += 4 {
		m := 0
		for c := range 3 {
			v := int(got.Pix[i+c]) - int(want.Pix[i+c])
			m = max(m, v, -v)
		}
		v := uint8(min(255, 8*m))
		d.Pix[i], d.Pix[i+1], d.Pix[i+2], d.Pix[i+3] = v, v, v, 255
	}
	golden.WritePNG(t, fmt.Sprintf("testdata/chromium_x%d_actual.png", scale), got)
	golden.WritePNG(t, fmt.Sprintf("testdata/chromium_x%d_diff.png", scale), d)
	t.Logf("x%d: wrote testdata/chromium_x%d_actual.png and testdata/chromium_x%d_diff.png", scale, scale, scale)
}

func TestCounterGolden(t *testing.T) {
	// 1.25 and 1.5: fractional scales (Wayland wp_fractional_scale_v1,
	// Windows 125 % and 150 %).
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			golden.Image(t, fmt.Sprintf("counter_x%g", scale), renderCounter(scale).img)
		})
	}
}
