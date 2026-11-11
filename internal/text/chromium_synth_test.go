package text

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The synthetic bold and oblique comparison draws one line in a font that
// has neither a bold nor an italic face (the test subset of Noto Sans CJK
// JP, a static Regular), here and in Chromium (canvas fillText with the
// font loaded through @font-face, whose descriptors declare one normal
// face of weight 400, so Blink synthesizes), in four styles and three
// sizes, and compares each cell's pixels and ink.
//
// The screenshot is committed. To regenerate it:
//
//	TELA_WRITE_HTML=1 go test -run TestChromiumSynthetic ./internal/text/
//	cd internal/text/testdata/chromium && chromium --headless --disable-gpu \
//	  --force-device-scale-factor=1 --window-size=1400,200 \
//	  --allow-file-access-from-files --virtual-time-budget=5000 \
//	  --hide-scrollbars --screenshot=synthetic_x1.png "file://$PWD/synthetic.html"
const (
	synthW, synthH = 1400, 200
	synthText      = "Hamburg 日本語"
)

type synthCell struct {
	size    float32
	weight  float32
	italic  bool
	x, y    float32 // pen position on the baseline
	cell    image.Rectangle
	control bool // the face itself, nothing synthesized
}

func synthCells() []synthCell {
	var cells []synthCell
	rows := []struct{ size, y, above, below float32 }{{14, 30, 16, 6}, {24, 76, 26, 10}, {40, 150, 42, 14}}
	styles := []struct {
		weight float32
		italic bool
	}{{400, false}, {700, false}, {400, true}, {700, true}}
	for _, r := range rows {
		for i, s := range styles {
			x := float32(20 + 340*i)
			cells = append(cells, synthCell{size: r.size, weight: s.weight, italic: s.italic, x: x, y: r.y,
				cell:    image.Rect(int(x)-6, int(r.y-r.above), int(x)+334, int(r.y+r.below)),
				control: s.weight == 400 && !s.italic})
		}
	}
	return cells
}

func (c synthCell) String() string {
	style := "normal"
	if c.italic {
		style = "italic"
	}
	return fmt.Sprintf("%g %s %gpx", c.weight, style, c.size)
}

func synthHTML() string {
	var items []string
	for _, c := range synthCells() {
		style := ""
		if c.italic {
			style = "italic "
		}
		items = append(items, fmt.Sprintf("[%q, %g, %g, %g]", style+fmt.Sprint(c.weight)+" "+fmt.Sprint(c.size)+"px TelaJP", c.x, c.y, c.size))
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
@font-face { font-family: "TelaJP"; src: url("../fonts/NotoSansJP-Subset.otf"); }
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
canvas { display: block; }
</style></head><body><canvas id="c" width="%d" height="%d"></canvas>
<script>
const items = [
%s
];
Promise.all(items.map(([font]) => document.fonts.load(font, %q))).then(() => {
  const ctx = document.getElementById("c").getContext("2d");
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, %d, %d);
  ctx.fillStyle = "#111827";
  ctx.textBaseline = "alphabetic";
  for (const [font, x, y] of items) {
    ctx.font = font;
    ctx.fillText(%q, x, y);
  }
});
</script></body></html>
`, synthW, synthH, strings.Join(items, ",\n"), synthText, synthW, synthH, synthText)
}

func renderSynthScene(t testing.TB) *image.RGBA {
	c := layoutCollection(t)
	b := gfx.NewBuilder()
	for _, cell := range synthCells() {
		st := TextStyle{Families: []string{"Noto Sans JP"}, Size: cell.size, Weight: cell.weight, Paint: gfx.RGB8(0x11, 0x18, 0x27)}
		if cell.italic {
			st.Style = StyleItalic
		}
		l := NewLayout(c, &Paragraph{Text: synthText, Default: st, Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}})
		l.BreakLines(1000)
		l.Paint(b, gfx.Pt(cell.x, cell.y-l.Line(0).Baseline))
	}
	img := image.NewRGBA(image.Rect(0, 0, synthW, synthH))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, b.Finish(), raster.Options{})
	return img
}

// ink returns the sum of the darkness of the pixels of r, in pixels.
func ink(img image.Image, r image.Rectangle) float64 {
	var s float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x+img.Bounds().Min.X, y+img.Bounds().Min.Y).RGBA()
			s += 1 - float64(cr+cg+cb)/3/65535
		}
	}
	return s
}

// Tolerances. Measured with Chromium 155 on Linux: mean channel
// differences of 0.66 to 1.01 for the cells drawn with the face as it is
// and of 0.90 to 1.14 for the synthesized ones (edge anti-aliasing: Skia
// samples, raster computes areas); ink within 1.6% of Chromium's for the
// former and 2.2% for the latter.
const (
	synthMaxMean = 1.6
	synthMaxInk  = 0.03
)

func TestChromiumSynthetic(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_WRITE_HTML") == "1" {
		if err := os.WriteFile(filepath.Join(dir, "synthetic.html"), []byte(synthHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	html, err := os.ReadFile(filepath.Join(dir, "synthetic.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(html) != synthHTML() {
		t.Fatal("testdata/chromium/synthetic.html is out of date; regenerate it with TELA_WRITE_HTML=1 and the screenshot with Chromium (see chromium_synth_test.go)")
	}
	fh, err := os.Open(filepath.Join(dir, "synthetic_x1.png"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := png.Decode(fh)
	fh.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := renderSynthScene(t)
	if p := os.Getenv("TELA_CHROMIUM_DUMP"); p != "" {
		f, _ := os.Create(p)
		png.Encode(f, got)
		f.Close()
	}
	for _, c := range synthCells() {
		st := compareRegion(got, ref, c.cell)
		gi, ri := ink(got, c.cell), ink(ref, c.cell)
		rel := (gi - ri) / ri
		t.Logf("%-18s max %3d, mean %.2f, ink %.1f vs %.1f (%+.1f%%)", c, st.max, st.mean, gi, ri, 100*rel)
		if st.mean > synthMaxMean {
			t.Errorf("%s: mean channel difference %.2f (allowed %.2f)", c, st.mean, synthMaxMean)
		}
		if rel > synthMaxInk || rel < -synthMaxInk {
			t.Errorf("%s: ink %.1f, Chromium %.1f (%+.1f%%, allowed ±%.0f%%)", c, gi, ri, 100*rel, 100*synthMaxInk)
		}
	}
}
