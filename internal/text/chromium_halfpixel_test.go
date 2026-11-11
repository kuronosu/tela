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

// The half-pixel baseline comparison draws one line of Inter at every
// font size of halfPixelSizes, with line-height: normal, at offsets of 0
// to 3 px from a row's top, at display scales 1.25, 1.5 and 1.75, here
// (text.Layout at the scale, painted and rasterized) and in Chromium
// (absolutely positioned divs, --force-device-scale-factor). At these
// scales an odd offset puts many baselines on an exact half device pixel:
// Blink's text origin is in LayoutUnits there, and Skia rounds a glyph's
// y to whole device pixels half up; Tela's baseline is a float32 sum that
// can be a hair below the half, which raster rounded down, one row
// higher (ADR-159's known case, fixed by ADR-273). The test compares the
// vertical ink centroid of each cell, which moves by a whole row when the
// baseline does and by a fraction when only anti-aliasing differs.
// Chromium's screenshots are committed in testdata/chromium/halfpixel; to
// regenerate them:
//
//	TELA_CHROMIUM=1 go test -run TestChromiumHalfPixelBaselines ./internal/text/

var halfPixelSizes = []float32{11, 12, 13, 14, 15, 16, 17, 18, 20, 24}

var halfPixelScales = []float32{1.25, 1.5, 1.75}

const (
	halfPixelRow = 40  // CSS px per size
	halfPixelCol = 100 // CSS px per offset
	halfPixelW   = 400
	halfPixelH   = 400 // whole device pixels at every scale
)

func halfPixelHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(`@font-face { font-family: "Inter"; src: url("../../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }`)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }\n")
	b.WriteString("div { position: absolute; font-family: \"Inter\"; white-space: nowrap; color: #000; }\n")
	b.WriteString("</style></head><body>\n")
	for row, s := range halfPixelSizes {
		for off := range 4 {
			fmt.Fprintf(&b, "<div style=\"left: %dpx; top: %dpx; font-size: %gpx\">Hxgy</div>\n", off*halfPixelCol+8, row*halfPixelRow+off, s)
		}
	}
	b.WriteString("</body></html>\n")
	return b.String()
}

// halfPixelRender renders the cells at scale k.
func halfPixelRender(t testing.TB, k float32) *image.RGBA {
	b := gfx.NewBuilder()
	for row, s := range halfPixelSizes {
		st := TextStyle{Families: []string{"Inter"}, Size: s, Weight: 400, Paint: gfx.Black}
		for off := range 4 {
			l := NewLayout(layoutCollection(t), &Paragraph{Text: "Hxgy", Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}, Default: st, Scale: k})
			l.BreakLines(float32(math.Inf(1)))
			l.PaintScaled(b, gfx.Pt(float32(off*halfPixelCol+8), float32(row*halfPixelRow+off)), k)
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, int(halfPixelW*k), int(halfPixelH*k)))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, b.Finish(), raster.Options{Scale: k})
	return img
}

// inkCentroid returns the mean row of the darkness of r in img.
func inkCentroid(img *image.RGBA, r image.Rectangle) float64 {
	var sum, n float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			d := 255 - float64(img.RGBAAt(x, y).G)
			sum += d * float64(y)
			n += d
		}
	}
	if n == 0 {
		return math.NaN()
	}
	return sum / n
}

func TestChromiumHalfPixelBaselines(t *testing.T) {
	dir := filepath.Join("testdata", "chromium", "halfpixel")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		page, _ := filepath.Abs(filepath.Join(dir, "page.html"))
		if err := os.WriteFile(page, []byte(halfPixelHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, k := range halfPixelScales {
			shot, _ := filepath.Abs(filepath.Join(dir, fmt.Sprintf("x%g.png", k)))
			chromium.ScreenshotAt(t, "file://"+page, shot, halfPixelW, halfPixelH, float64(k), "--allow-file-access-from-files", "--virtual-time-budget=10000")
			golden.WriteBestPNG(t, shot, golden.ReadRGBA(t, shot))
		}
	}
	if h, err := os.ReadFile(filepath.Join(dir, "page.html")); err != nil || string(h) != halfPixelHTML() {
		t.Fatalf("%s/page.html is out of date; regenerate with TELA_CHROMIUM=1", dir)
	}
	for _, k := range halfPixelScales {
		ref := golden.ReadRGBA(t, filepath.Join(dir, fmt.Sprintf("x%g.png", k)))
		got := halfPixelRender(t, k)
		if d := os.Getenv("TELA_CHROMIUM_DUMP"); d != "" {
			golden.WritePNG(t, filepath.Join(d, fmt.Sprintf("halfpixel_x%g.png", k)), got)
		}
		if got.Rect != ref.Rect {
			t.Fatalf("x%g: render %v, Chromium %v", k, got.Rect, ref.Rect)
		}
		worst := 0.0
		for row, s := range halfPixelSizes {
			for off := range 4 {
				x0, y0 := float64(off*halfPixelCol), float64(row*halfPixelRow)
				r := image.Rect(int(x0*float64(k)), int(y0*float64(k)), int((x0+halfPixelCol)*float64(k)), int((y0+halfPixelRow)*float64(k)))
				d := inkCentroid(got, r) - inkCentroid(ref, r)
				worst = max(worst, math.Abs(d))
				// A baseline one row off moved the centroid by 0.63 to
				// 1.23 device px (before ADR-273); glyph rendering moves
				// it by at most 0.41 (20 px at x1.25: Skia draws the H's
				// crossbar one row lower).
				if math.Abs(d) > 0.5 {
					t.Errorf("x%g %gpx at +%dpx: ink %.2f device px from Chromium's (a baseline one row off)", k, s, off, d)
				}
			}
		}
		t.Logf("x%g: largest ink centroid difference %.3f device px", k, worst)
	}
}
