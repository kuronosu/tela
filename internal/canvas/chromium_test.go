package canvas

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/chromium"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// chromiumFonts are the deterministic fonts, for the pages in
// testdata/chromium.
const chromiumFonts = `@font-face { font-family: "Inter"; src: url("../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; }
@font-face { font-family: "Inter"; src: url("../../../assets/fonts/inter/InterVariable-Italic.ttf"); font-weight: 100 900; font-style: italic; }
@font-face { font-family: "JetBrains Mono"; src: url("../../../assets/fonts/jetbrainsmono/JetBrainsMono-wght.ttf"); font-weight: 100 800; }
@font-face { font-family: "JetBrains Mono"; src: url("../../../assets/fonts/jetbrainsmono/JetBrainsMono-Italic-wght.ttf"); font-weight: 100 800; font-style: italic; }
@font-face { font-family: "Source Serif 4"; src: url("../../../assets/fonts/sourceserif4/SourceSerif4-opsz-wght.ttf"); font-weight: 200 900; }
@font-face { font-family: "Source Serif 4"; src: url("../../../assets/fonts/sourceserif4/SourceSerif4-Italic-opsz-wght.ttf"); font-weight: 200 900; font-style: italic; }
@font-face { font-family: "Noto Sans Arabic"; src: url("../../../assets/fonts/notosansarabic/NotoSansArabic-wdth-wght.ttf"); font-weight: 100 900; font-stretch: 62.5% 100%; }
@font-face { font-family: "Noto Color Emoji"; src: url("../../../assets/fonts/notocoloremoji/Noto-COLRv1.ttf"); }
`

// chromiumFlags are the flags of the pages, besides the harness's.
var chromiumFlags = []string{"--allow-file-access-from-files", "--virtual-time-budget=10000"}

// chromiumScenes are the scenes compared with Chromium: those of the goldens
// and those made of cells.
func chromiumScenes() []scene {
	out := scenes()
	for i := range out {
		out[i].cells = sceneCells[out[i].name]
	}
	for _, p := range cellPages() {
		out = append(out, p.scene())
	}
	return out
}

// sceneHTML returns the page that draws s on a browser's canvas.
func sceneHTML(s *scene) string {
	js := newScript(s.dir)
	var r Recorder
	r.Record(textFrame(s.w, s.h, s.dir, 1), func(c *Canvas) { s.draw(&pen{Canvas: c, js: js}) })
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
%shtml, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
canvas { display: block; width: %gpx; height: %gpx; }
</style></head><body><canvas id="canvas"></canvas>
<script>
const loading = [];
function image(w, data) {
  const pix = Uint8ClampedArray.from(atob(data), c => c.charCodeAt(0));
  const cv = document.createElement("canvas");
  cv.width = w; cv.height = pix.length / 4 / w;
  cv.getContext("2d").putImageData(new ImageData(pix, w), 0, 0);
  const img = new Image();
  img.src = cv.toDataURL();
  loading.push(img.decode());
  return img;
}
%sconst k = window.devicePixelRatio, cv = document.getElementById("canvas");
cv.width = %g * k; cv.height = %g * k;
const c = cv.getContext("2d");
let g;
Promise.all([...loading, ...[...document.fonts].map(f => f.load())]).then(() => {
c.scale(k, k);
// Glyphs at fractions of a pixel and without hinting, as Tela's.
c.textRendering = "geometricPrecision";
%s});
</script></body></html>
`, chromiumFonts, s.w, s.h, js.decl.String(), s.w, s.h, js.String())
}

// cellDiff is how much a cell differs, per channel.
type cellDiff struct {
	mean float64
	max  int
	at   image.Point
}

func diffCell(got, want *image.RGBA, r image.Rectangle) cellDiff {
	var d cellDiff
	sum, n := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			a, b := got.Pix[got.PixOffset(x, y):], want.Pix[want.PixOffset(x, y):]
			for k := range 3 {
				v := int(a[k]) - int(b[k])
				if v < 0 {
					v = -v
				}
				if v > d.max {
					d.max, d.at = v, image.Pt(x, y)
				}
				sum += v
				n++
			}
		}
	}
	d.mean = float64(sum) / float64(n)
	return d
}

// TestChromiumCanvas compares the scenes with Chromium's canvas, cell by cell:
// the same numbers and fonts, at the scales 1 and 2, in a canvas whose backing
// store is scaled by the device scale (chromium_bounds_test.go says how much
// each one differs, and why). The references are in testdata/chromium, the
// pages and the screenshots of Chromium 155.0.8059.84, Arch Linux, without a
// GPU. Regenerate them with
//
//	TELA_CHROMIUM=1 go test -run TestChromiumCanvas ./internal/canvas/
//
// and write Tela's with TELA_CHROMIUM_DUMP=dir.
//
// What ADR-319 makes different on purpose does not show in them, since the
// pen (pen_test.go) sets it: the initial styles and font, the FillRule, and
// that a canvas is drawn from empty. What throws in a browser, as a negative
// radius, is in TestMethods.
func TestChromiumCanvas(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	used := map[string]bool{}
	for _, s := range chromiumScenes() {
		t.Run(s.name, func(t *testing.T) {
			page, _ := filepath.Abs(filepath.Join(dir, s.name+".html"))
			html := sceneHTML(&s)
			shot := func(scale int) string {
				p, _ := filepath.Abs(filepath.Join(dir, fmt.Sprintf("%s_x%d.png", s.name, scale)))
				return p
			}
			if os.Getenv("TELA_CHROMIUM") == "1" {
				if err := os.WriteFile(page, []byte(html), 0o644); err != nil {
					t.Fatal(err)
				}
				for _, k := range []int{1, 2} {
					chromium.Screenshot(t, "file://"+page, shot(k), int(s.w), int(s.h), k, chromiumFlags...)
					golden.WriteBestPNG(t, shot(k), golden.ReadRGBA(t, shot(k)))
				}
			}
			if stored, err := os.ReadFile(page); err != nil || string(stored) != html {
				t.Fatalf("%s is out of date with the scene: regenerate it with TELA_CHROMIUM=1", page)
			}
			var got, want [3]*image.RGBA
			for k := 1; k <= 2; k++ {
				got[k], want[k] = renderScene(&s, float32(k)), golden.ReadRGBA(t, shot(k))
				if got[k].Rect != want[k].Rect {
					t.Fatalf("x%d: %v, Chromium %v", k, got[k].Rect, want[k].Rect)
				}
				if d := os.Getenv("TELA_CHROMIUM_DUMP"); d != "" {
					golden.WritePNG(t, filepath.Join(d, fmt.Sprintf("%s_x%d.png", s.name, k)), got[k])
				}
			}
			var table strings.Builder
			for _, c := range s.cells {
				name := s.name + "/" + c.name
				b, ok := chromiumBounds[name]
				if used[name] {
					t.Errorf("two cells called %q", name)
				}
				used[name] = true
				usual := sceneBounds[s.name]
				if !ok {
					b = usual
				}
				same := true
				for k := 1; k <= 2; k++ {
					d := diffCell(got[k], want[k], image.Rect(int(c.rect.X0)*k, int(c.rect.Y0)*k, int(c.rect.X1)*k, int(c.rect.Y1)*k))
					fmt.Fprintf(&table, "%-44s x%d mean %6.3f max %3d at %v\n", name, k, d.mean, d.max, d.at)
					if d.mean > b.mean || d.max > b.max {
						t.Errorf("%s x%d: mean %.3f, allowed %.2f; max %d at %v, allowed %d: %s", name, k, d.mean, b.mean, d.max, d.at, b.max, b.why)
					}
					same = same && d.mean <= usual.mean && d.max <= usual.max
				}
				if b.kept && same {
					t.Errorf("%s: not a difference any more (%s)", name, b.why)
				}
			}
			t.Logf("\n%s", table.String())
		})
	}
	for name := range chromiumBounds {
		if !used[name] {
			t.Errorf("a bound for %q, which is not a cell", name)
		}
	}
}

// sceneCell is a part of a scene that is compared by itself.
type sceneCell struct {
	name string
	rect gfx.Rect
}

// grid returns cells of w by h from (x, y), left to right and down.
func grid(x, y, w, h float32, cols int, names ...string) []sceneCell {
	var out []sceneCell
	for i, n := range names {
		out = append(out, sceneCell{n, gfx.RectXYWH(x+float32(i%cols)*w, y+float32(i/cols)*h, w, h)})
	}
	return out
}

func join(lists ...[]sceneCell) []sceneCell {
	var out []sceneCell
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// sceneCells are the cells of the scenes of the goldens.
var sceneCells = map[string][]sceneCell{
	"paths": join(
		grid(0, 0, 100, 95, 4, "quadratics", "cubics", "arcTo", "ellipse", "roundRect", "roundRect 2", "roundRect 3", "roundRect 4"),
		grid(0, 180, 100, 120, 4, "arc", "half ellipses", "Path2D", "even-odd clip")),
	"dashes": join(
		grid(0, 5, 205, 20, 1, "20 10", "offset", "odd", "round", "dots", "squares", "whole", "invalid"),
		grid(205, 0, 105, 95, 1, "rectangle"), grid(310, 0, 90, 95, 1, "circle"), grid(205, 95, 195, 105, 1, "distorted")),
	"gradients": join(
		grid(0, 0, 105, 80, 1, "linear"), grid(105, 0, 95, 80, 1, "radial"), grid(200, 0, 100, 80, 1, "focal"), grid(300, 0, 100, 80, 1, "cone"),
		grid(0, 80, 100, 80, 1, "conic"), grid(100, 80, 100, 80, 1, "hard"), grid(200, 80, 200, 45, 1, "moved"), grid(200, 125, 200, 45, 1, "line"),
		grid(0, 170, 130, 80, 1, "radial skewed"), grid(130, 165, 100, 85, 1, "conic stretched"), grid(230, 170, 110, 80, 1, "conic mirrored"),
		grid(340, 170, 60, 80, 1, "faded"), grid(0, 250, 215, 70, 1, "text"), grid(215, 250, 185, 70, 1, "strokeText")),
	"patterns": join(
		grid(0, 0, 100, 75, 4, "repeat", "repeat-x", "repeat-y", "no-repeat"),
		grid(0, 75, 105, 80, 1, "shape"), grid(105, 75, 120, 100, 1, "turned"), grid(225, 75, 90, 80, 1, "transform"),
		grid(315, 75, 85, 85, 1, "stroke"), grid(0, 160, 290, 80, 1, "text"), grid(290, 160, 110, 80, 1, "scaled")),
	"text": join(
		grid(0, 5, 165, 22, 1, "start", "end", "left", "right", "center"),
		grid(165, 35, 315, 45, 1, "baselines"),
		grid(0, 130, 480, 30, 1, "scripts"), grid(0, 160, 130, 30, 1, "bold italic"), grid(130, 160, 225, 30, 1, "mono"), grid(355, 160, 125, 30, 1, "fallback"),
		grid(0, 190, 185, 30, 1, "wide"), grid(185, 190, 130, 30, 1, "maxWidth"), grid(315, 190, 165, 30, 1, "maxWidth right"),
		grid(0, 220, 165, 25, 1, "letterSpacing"), grid(165, 220, 150, 25, 1, "negative letterSpacing"),
		grid(0, 245, 175, 55, 1, "strokeText"), grid(175, 245, 125, 55, 1, "dashed strokeText"), grid(300, 245, 180, 55, 1, "both"),
		grid(0, 300, 180, 30, 1, "faded"), grid(180, 285, 140, 50, 1, "turned"), grid(330, 300, 120, 40, 1, "scaled"),
		grid(0, 340, 180, 80, 1, "boxes")),
	"text_rtl": grid(0, 5, 320, 28, 1, "start", "end", "left", "right", "center"),
	"images": join(
		grid(0, 0, 80, 65, 1, "natural"), grid(80, 0, 115, 65, 1, "stretched"), grid(200, 0, 75, 65, 1, "region"), grid(280, 0, 75, 65, 1, "past its edges"), grid(355, 0, 45, 65, 1, "negative"),
		grid(5, 65, 75, 75, 1, "smooth"), grid(80, 65, 75, 75, 1, "pixels"), grid(155, 65, 23, 20, 1, "minified"), grid(178, 70, 112, 70, 1, "faded"), grid(290, 65, 80, 75, 1, "turned"),
		grid(5, 145, 85, 60, 1, "mirrored"), grid(105, 145, 70, 60, 1, "clipped")),
	"shadows": grid(5, 0, 118, 100, 4,
		"rectangle", "rounded", "path", "stroke", "text", "image", "gradient", "turned", "translucent", "globalAlpha", "emoji", "clipped",
		"large blur", "outside", "destination-over", "pattern"),
	"operations": func() []sceneCell {
		var names []string
		for _, o := range operations {
			names = append(names, o.name)
		}
		return grid(0, 0, 80, 80, 6, names...)
	}(),
}
