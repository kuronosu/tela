package text

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/chromium"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The skip-ink comparison draws underlined and overlined words whose
// glyphs cross the line ("gyp", "jq", the ring of "Å") at a few sizes and
// thicknesses, here and in Chromium (absolutely positioned divs, Inter
// through @font-face), at scales 1 and 2. Chromium's default
// text-decoration-skip-ink: auto leaves gaps around the glyphs
// (skipink.go). The test compares the pixels of each cell and, in the rows
// of the line, the columns where the line is drawn. The screenshots are
// committed in testdata/chromium/skipink; to regenerate them:
//
//	TELA_CHROMIUM=1 go test -run TestChromiumSkipInk ./internal/text/

type skipInkCell struct {
	text       string
	x, y       float32
	size       float32
	lines      Decoration
	thick      float32 // 0: auto
	css        string  // the decoration in CSS
	underlined bool
}

var skipInkCells = []skipInkCell{
	{"gyp jq", 8, 4, 14, DecorationUnderline, 0, "underline", true},
	{"gyp jq", 8, 30, 24, DecorationUnderline, 0, "underline", true},
	{"gyp jq", 8, 70, 40, DecorationUnderline, 0, "underline", true},
	{"gyp jq", 168, 4, 14, DecorationUnderline, 3, "underline; text-decoration-thickness: 3px", true},
	{"gyp jq", 168, 30, 24, DecorationUnderline, 2, "underline; text-decoration-thickness: 2px", true},
	{"Ågyp", 168, 70, 40, DecorationOverline, 0, "overline", false},
	{"Typography", 8, 124, 18, DecorationUnderline, 0, "underline", true},
	{"quick jumping", 168, 124, 18, DecorationUnderline, 0, "underline", true},
}

const skipInkW, skipInkH = 320, 156

func skipInkHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(`@font-face { font-family: "Inter"; src: url("../../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }`)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }\n")
	b.WriteString("div { position: absolute; font-family: \"Inter\"; white-space: nowrap; color: #000; }\n")
	b.WriteString("</style></head><body>\n")
	for _, c := range skipInkCells {
		fmt.Fprintf(&b, "<div style=\"left: %gpx; top: %gpx; font-size: %gpx; text-decoration: %s\">%s</div>\n", c.x, c.y, c.size, c.css, c.text)
	}
	b.WriteString("</body></html>\n")
	return b.String()
}

// skipInkRender renders the cells at scale k, and returns the device row
// through the middle of each cell's line.
func skipInkRender(t testing.TB, k float32) (*image.RGBA, []int) {
	b := gfx.NewBuilder()
	var rows []int
	for _, c := range skipInkCells {
		st := TextStyle{Families: []string{"Inter"}, Size: c.size, Weight: 400, Paint: gfx.Black,
			Decoration: TextDecoration{Lines: c.lines, Thickness: c.thick}}
		l := NewLayout(layoutCollection(t), &Paragraph{Text: c.text, Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}, Default: st, Scale: k})
		l.BreakLines(float32(math.Inf(1)))
		l.PaintScaled(b, gfx.Pt(c.x, c.y), k)
		by := c.y + l.lines[0].baseline
		top, h := snappedUnderline(&st, c.size, by, k)
		if c.lines == DecorationOverline {
			top, h = snappedOverline(&st, c.size, l.sinfo[0].ascent, by, k)
		}
		rows = append(rows, int(math.Floor(float64((by+top+h/2)*k))))
	}
	img := image.NewRGBA(image.Rect(0, 0, int(skipInkW*k), int(skipInkH*k)))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, b.Finish(), raster.Options{Scale: k})
	return img, rows
}

// lineMismatches returns the number of columns of cell r's device row
// where one image is dark (the line or a glyph) and the other is not: on
// the row through the middle of a decoration line, a gap at another place,
// or no gap, counts every column it moves.
func lineMismatches(got, ref *image.RGBA, r image.Rectangle, row int) (n int) {
	dark := func(img *image.RGBA, x, y int) bool { return img.RGBAAt(x, y).G < 128 }
	for x := r.Min.X; x < r.Max.X; x++ {
		if dark(got, x, row) != dark(ref, x, row) {
			n++
		}
	}
	return n
}

// skipInkCellRect returns the device rectangle of cell i: from its top-left
// corner to the next cell's at scale k.
func skipInkCellRect(i int, k float32) image.Rectangle {
	c := skipInkCells[i]
	// Overlines are drawn above the line box: 6 px of a cell's top
	// belong to the one above.
	h := float32(skipInkH) - c.y
	for _, d := range skipInkCells {
		if d.y > c.y {
			h = min(h, d.y-c.y-6)
		}
	}
	return image.Rect(int((c.x-8)*k), int((c.y-6)*k), int((c.x+152)*k), int((c.y+h)*k)).Intersect(image.Rect(0, 0, int(skipInkW*k), int(skipInkH*k)))
}

func TestChromiumSkipInk(t *testing.T) {
	dir := filepath.Join("testdata", "chromium", "skipink")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		page, _ := filepath.Abs(filepath.Join(dir, "page.html"))
		if err := os.WriteFile(page, []byte(skipInkHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, k := range []float32{1, 2} {
			shot, _ := filepath.Abs(filepath.Join(dir, fmt.Sprintf("x%g.png", k)))
			chromium.ScreenshotAt(t, "file://"+page, shot, skipInkW, skipInkH, float64(k), "--allow-file-access-from-files", "--virtual-time-budget=10000")
			golden.WriteBestPNG(t, shot, golden.ReadRGBA(t, shot))
		}
	}
	if h, err := os.ReadFile(filepath.Join(dir, "page.html")); err != nil || string(h) != skipInkHTML() {
		t.Fatalf("%s/page.html is out of date; regenerate with TELA_CHROMIUM=1", dir)
	}
	for _, k := range []float32{1, 2} {
		ref := golden.ReadRGBA(t, filepath.Join(dir, fmt.Sprintf("x%g.png", k)))
		got, rows := skipInkRender(t, k)
		if d := os.Getenv("TELA_CHROMIUM_DUMP"); d != "" {
			golden.WritePNG(t, filepath.Join(d, fmt.Sprintf("skipink_x%g.png", k)), got)
		}
		if got.Rect != ref.Rect {
			t.Fatalf("x%g: render %v, Chromium %v", k, got.Rect, ref.Rect)
		}
		for i, c := range skipInkCells {
			r := skipInkCellRect(i, k)
			st := compareRegion(got, ref, r)
			row := rows[i]
			n := lineMismatches(got, ref, r, row)
			t.Logf("x%g %q %s: mean %.3f, max %d; line row %d: %d columns differ", k, c.text, c.css, st.mean, st.max, row, n)
			// Glyph anti-aliasing (Skia hints Inter's horizontal stems,
			// "Typography" at x2: 2.9) is what differs.
			if st.mean > 3 {
				t.Errorf("x%g %q %s: mean channel difference %.3f, allowed 3", k, c.text, c.css, st.mean)
			}
			// Without skip-ink 12 to 103 columns differed. What remains is
			// a gap edge on another device pixel now and then (Chromium
			// positions glyphs, and so their intercepts, on its own
			// grid, and hints Inter's stems).
			if n > 4*int(k) {
				t.Errorf("x%g %q %s: %d columns of the line row differ from Chromium's", k, c.text, c.css, n)
			}
		}
	}
}
