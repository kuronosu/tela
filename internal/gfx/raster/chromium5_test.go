package raster

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
)

// The composite and pattern scenes are also written as HTML whose oracle is
// Canvas: testdata/chromium/composite.html (globalCompositeOperation, with
// globalAlpha and under a rounded clip, a canvas per cell) and pattern.html
// (createPattern, setTransform, imageSmoothingEnabled). Chromium renders
// them into <name>_x1.png and <name>_x2.png with
//
//	chromium --headless --disable-gpu --hide-scrollbars \
//	  --force-device-scale-factor=1 --window-size=600,400 \
//	  --screenshot=composite_x1.png composite.html
//
// (Chromium 155.0.8059.84, Arch Linux; pattern.html at 600,200;
// --force-device-scale-factor=2 for _x2). Regenerate the HTML with
// TELA_WRITE_HTML=1 go test -run TestChromiumScenes5.

// compositeHTML is the composite scene drawn with Canvas.
func compositeHTML() string {
	var items []string
	for _, c := range gfxtest.CompositeCells() {
		op := c.Mode.String()
		switch c.Mode {
		case gfx.SrcOver:
			op = "source-over"
		case gfx.PlusLighter:
			op = "lighter"
		}
		items = append(items, fmt.Sprintf("[%g, %g, %q, %t]", c.X, c.Y, op, c.Clip))
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
canvas { position: absolute; width: 100px; height: 100px; }
</style></head><body>
<script>
const cells = [
%s
];
const dpr = window.devicePixelRatio;
function gradient(ctx, x0, y0, x1, y1, a, b) {
  const g = ctx.createLinearGradient(x0, y0, x1, y1);
  g.addColorStop(0, a);
  g.addColorStop(1, b);
  return g;
}
for (const [x, y, op, clip] of cells) {
  const cv = document.createElement("canvas");
  cv.style.left = x + "px";
  cv.style.top = y + "px";
  cv.width = cv.height = 100 * dpr;
  document.body.append(cv);
  const ctx = cv.getContext("2d");
  ctx.scale(dpr, dpr);
  ctx.fillStyle = gradient(ctx, 10, 10, 65, 10, %q, %q);
  ctx.fillRect(10, 10, 55, 55);
  ctx.fillStyle = %q;
  ctx.beginPath(); ctx.roundRect(14, 48, 44, 44, 22); ctx.fill();
  if (clip) {
    ctx.beginPath(); ctx.roundRect(8, 12, 80, 76, 26); ctx.clip();
    ctx.globalAlpha = 0.7;
  }
  ctx.globalCompositeOperation = op;
  ctx.translate(50, 50); ctx.rotate(%g); ctx.translate(-50, -50);
  ctx.fillStyle = gradient(ctx, 30, 30, 30, 76, %q, %q);
  ctx.fillRect(30, 30, 56, 46);
}
</script></body></html>
`, strings.Join(items, ",\n"), cssColor(red), cssColor(sky), cssColor(indigo.WithAlpha(0.5)),
		float64(gfxtest.CompositeAngle), cssColor(amber), cssColor(amber.WithAlpha(0.25)))
}

// patternCells returns the cells of the pattern scene compared with
// Chromium: all but the minified ones, which Chromium samples without mip
// levels whatever imageSmoothingQuality says (TestPatternMinified).
func patternCells() []gfxtest.PatternCell {
	var out []gfxtest.PatternCell
	for _, c := range gfxtest.PatternCells() {
		if !c.Big {
			out = append(out, c)
		}
	}
	return out
}

// patternHTML is the pattern scene drawn with Canvas.
func patternHTML() string {
	var items []string
	for _, c := range patternCells() {
		m := func(a gfx.Affine) string {
			return fmt.Sprintf("[%g, %g, %g, %g, %g, %g]", a[0], a[1], a[2], a[3], a[4], a[5])
		}
		items = append(items, fmt.Sprintf("[%q, %g, %g, %q, %t, %s, %s, %q]", c.Name, c.X, c.Y, c.Repeat, c.Nearest, m(c.Transform), m(c.Pattern), c.Shape))
	}
	tile := func(pix []color.NRGBA) string {
		var sb strings.Builder
		for i, p := range pix {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, "%d,%d,%d,%d", p.R, p.G, p.B, p.A)
		}
		return sb.String()
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
canvas { display: block; width: 600px; height: 200px; }
</style></head><body><canvas id="c"></canvas>
<script>
const cells = [
%s
];
function tile(w, pix) {
  const cv = document.createElement("canvas");
  cv.width = w; cv.height = pix.length / 4 / w;
  cv.getContext("2d").putImageData(new ImageData(new Uint8ClampedArray(pix), w), 0, 0);
  return cv;
}
const image = tile(12, [%s]);
const dpr = window.devicePixelRatio, cv = document.getElementById("c");
cv.width = 600 * dpr; cv.height = 200 * dpr;
const ctx = cv.getContext("2d");
// The bilinear weights of "low", the default, are coarse.
ctx.imageSmoothingQuality = "medium";
ctx.scale(dpr, dpr);
for (const [name, x, y, repeat, nearest, m, p, shape] of cells) {
  ctx.save();
  ctx.beginPath(); ctx.rect(x + 4, y + 4, 92, 92); ctx.clip();
  ctx.translate(x, y);
  ctx.transform(...m);
  ctx.imageSmoothingEnabled = !nearest;
  const pat = ctx.createPattern(image, repeat);
  pat.setTransform(new DOMMatrix(p));
  ctx.fillStyle = ctx.strokeStyle = pat;
  if (shape == "rect") {
    ctx.fillRect(-2000, -2000, 4000, 4000);
  } else if (shape == "disc") {
    ctx.beginPath(); ctx.roundRect(8, 7, 80, 80, 40); ctx.fill();
  } else {
    ctx.lineWidth = 22;
    ctx.beginPath(); ctx.arc(50, 50, 32, 0, 2 * Math.PI); ctx.stroke();
  }
  ctx.restore();
}
</script></body></html>
`, strings.Join(items, ",\n"), tile(gfxtest.PatternTilePixels))
}

// Bounds of the mean channel difference per cell: the larger of the two
// scales measured, plus 0.05. The maxima are 2 levels inside the cells (the
// rounding of gradients and of bilinear weights) and up to 67 on the
// anti-aliased edges of rotated rectangles, circles and rounded clips, where
// Skia's coverage differs from the exact area coverage (the means of the
// "pattern transform" and "stroke" cells, all edge, come from there).
var (
	compositeBounds = map[string]float64{
		"normal": 0.29, "plus-lighter": 0.29, "source-in": 0.12, "source-out": 0.11, "source-atop": 0.23,
		"destination-over": 0.27, "destination-in": 0.11, "destination-out": 0.21, "destination-atop": 0.18, "copy": 0.17,
		"xor": 0.26, "clip normal": 0.30, "clip plus-lighter": 0.33, "clip source-in": 0.21, "clip source-out": 0.20,
		"clip source-atop": 0.25, "clip destination-over": 0.28, "clip destination-in": 0.18, "clip destination-out": 0.22, "clip destination-atop": 0.25,
		"clip copy": 0.28, "clip xor": 0.28,
	}
	patternBounds = map[string]float64{
		"repeat": 0.07, "repeat-x": 0.10, "repeat-y": 0.12, "no-repeat": 0.12, "scaled": 0.21,
		"nearest": 0.05, "rotated": 0.21, "rotated no-repeat": 0.10, "pattern transform": 0.62, "stroke": 0.49,
	}
)

func TestChromiumScenes5(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	var composite, pattern []chromiumCell
	for _, c := range gfxtest.CompositeCells() {
		composite = append(composite, chromiumCell{c.Name(), gfx.RectXYWH(c.X, c.Y, 100, 100), compositeBounds[c.Name()]})
	}
	for _, c := range patternCells() {
		pattern = append(pattern, chromiumCell{c.Name, gfx.RectXYWH(c.X, c.Y, 100, 100), patternBounds[c.Name]})
	}
	pages := []chromiumPage{
		{"composite", scenes5[0], compositeHTML, composite},
		{"pattern", scenes5[1], patternHTML, pattern},
	}
	if os.Getenv("TELA_WRITE_HTML") == "1" {
		for _, p := range pages {
			if err := os.WriteFile(filepath.Join(dir, p.name+".html"), []byte(p.html()), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, p := range pages {
		hp := filepath.Join(dir, p.name+".html")
		html, err := os.ReadFile(hp)
		if err != nil {
			t.Fatal(err)
		}
		if string(html) != p.html() {
			t.Fatalf("%s is out of date with the scene; regenerate it with TELA_WRITE_HTML=1 and the screenshots with Chromium", hp)
		}
		for _, scale := range []float32{1, 2} {
			name := fmt.Sprintf("%s_x%g.png", p.name, scale)
			f, err := os.Open(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			ref, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			got := renderScene(p.scene, scale)
			for _, c := range p.cells {
				r := image.Rect(int(c.rect.X0*scale), int(c.rect.Y0*scale), int(c.rect.X1*scale), int(c.rect.Y1*scale))
				st := compareCell(got, ref, r)
				t.Logf("%s x%g %-22s mean %.3f max %3d", p.name, scale, c.name, st.mean, st.max)
				if c.maxMean == 0 {
					t.Errorf("%s: no bound recorded for cell %q", p.name, c.name)
				}
				if st.mean > c.maxMean {
					t.Errorf("%s x%g %s: mean channel difference %.3f, allowed %.2f (max %d)", p.name, scale, c.name, st.mean, c.maxMean, st.max)
				}
			}
		}
	}
}
