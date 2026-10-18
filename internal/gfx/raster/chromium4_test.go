package raster

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// The blend, masks and focal scenes (scenes4_test.go) are also written as
// HTML: testdata/chromium/blend.html (isolation, mix-blend-mode, opacity,
// gradients with out-of-gamut colors), masks.html (mask-image with alpha
// and luminance masks, nested, under a rounded clip and a rotation, with
// opacity and a blend mode) and focal.html (canvas createRadialGradient).
// Chromium renders them into <name>_x1.png and <name>_x2.png with
//
//	chromium --headless --disable-gpu --hide-scrollbars \
//	  --force-device-scale-factor=1 --window-size=600,300 \
//	  --screenshot=blend_x1.png blend.html
//
// (Chromium 152.0.7977.82, Arch Linux; masks.html at 480,260;
// --force-device-scale-factor=2 for _x2). Regenerate the HTML with
// TELA_WRITE_HTML=1 go test -run TestChromiumScenes4.
//
// TestChromiumScenes4 compares every cell and records the measured mean
// and maximum channel differences; it fails when a mean grows beyond the
// bound measured when the screenshots were taken (plus a margin).

type chromiumCell struct {
	name string
	rect gfx.Rect // logical
	// maxMean is the allowed mean channel difference.
	maxMean float64
}

type chromiumPage struct {
	name  string
	scene scene
	html  func() string
	cells []chromiumCell
}

func chromiumPages() []chromiumPage {
	var blend []chromiumCell
	for _, c := range blendCells() {
		name := c.mode.String()
		if c.clip {
			name = "clip " + name
		}
		blend = append(blend, chromiumCell{name, gfx.RectXYWH(c.x, c.y, 100, 100), blendBounds[name]})
	}
	var masks []chromiumCell
	for _, c := range maskCells {
		masks = append(masks, chromiumCell{c.name, gfx.RectXYWH(c.x, c.y, 160, 130), maskBounds[c.name]})
	}
	var focal []chromiumCell
	for _, c := range focalCells() {
		focal = append(focal, chromiumCell{c.name, gfx.RectXYWH(c.x, c.y, 150, 150), focalBounds[c.name]})
	}
	return []chromiumPage{
		{"blend", scenes4[0], blendHTML, blend},
		{"masks", scenes4[1], masksHTML, masks},
		{"focal", scenes4[2], focalHTML, focal},
	}
}

// Bounds of the mean channel difference per cell: the larger of the two
// scales measured against Chromium 152, plus 0.05. The maxima are a few
// levels inside the cells (gradient rounding; color-dodge, whose result
// changes fastest near Cs = 1, up to 16) and up to 44 on anti-aliased
// rounded and rotated edges, where Skia's coverage differs from the exact
// area coverage.
var (
	blendBounds = map[string]float64{
		"normal": 0.36, "multiply": 0.40, "screen": 0.37, "overlay": 0.41, "darken": 0.37, "lighten": 0.41,
		"color-dodge": 0.46, "color-burn": 0.38, "hard-light": 0.51, "soft-light": 0.34, "difference": 0.55,
		"exclusion": 0.51, "hue": 0.39, "saturation": 0.37, "color": 0.38, "luminosity": 0.38,
		"plus-lighter": 0.38, "clip difference": 0.62,
	}
	maskBounds = map[string]float64{
		"alpha linear": 0.50, "luminance": 0.52, "radial": 0.46, "nested": 0.47, "clip+rotate": 0.54, "opacity+multiply": 0.50,
	}
	focalBounds = map[string]float64{
		"focal": 0.19, "focal radius": 0.20, "focal on edge": 0.18, "cone": 0.08, "concentric": 0.17, "strip": 0.09,
		"ellipse": 0.14, "shrinking": 0.20,
	}
)

func TestChromiumScenes4(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_WRITE_HTML") == "1" {
		for _, p := range chromiumPages() {
			if err := os.WriteFile(filepath.Join(dir, p.name+".html"), []byte(p.html()), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, p := range chromiumPages() {
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
			if d := os.Getenv("TELA_CHROMIUM_DUMP"); d != "" {
				if f, err := os.Create(filepath.Join(d, name)); err == nil {
					png.Encode(f, got)
					f.Close()
				}
			}
			for _, c := range p.cells {
				r := image.Rect(int(c.rect.X0*scale), int(c.rect.Y0*scale), int(c.rect.X1*scale), int(c.rect.Y1*scale))
				st := compareCell(got, ref, r)
				t.Logf("%s x%g %-18s mean %.3f max %3d", p.name, scale, c.name, st.mean, st.max)
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

type cellStats struct {
	mean float64
	max  int
}

// compareCell compares the RGB channels of two opaque images over r.
func compareCell(a *image.RGBA, b image.Image, r image.Rectangle) cellStats {
	var st cellStats
	var sum, n float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := a.PixOffset(x, y)
			cr, cg, cb, _ := b.At(x+b.Bounds().Min.X, y+b.Bounds().Min.Y).RGBA()
			for k, v := range [3]uint32{cr >> 8, cg >> 8, cb >> 8} {
				d := iabs(int(a.Pix[i+k]) - int(v))
				st.max = max(st.max, d)
				sum += float64(d)
				n++
			}
		}
	}
	st.mean = sum / n
	return st
}
