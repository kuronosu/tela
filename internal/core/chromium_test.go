package core

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/chromium"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The box gallery (gallery_test.go) is compared with Chromium's rendering
// of the same page written in HTML and styled by Tailwind's compiled CSS.
// testdata/chromium holds the page (gallery.html), its CSS (gallery.css,
// compiled by the Tailwind CLI v4.3.3 with Inter as the sans font) and
// Chromium's screenshots at device scale factors 1 and 2
// (gallery_x1.png, gallery_x2.png). Regenerate them with
//
//	TELA_CHROMIUM=1 TELA_TAILWIND=/path/to/node_modules/.bin/tailwindcss \
//	  go test -run TestChromiumGallery ./internal/core/
//
// which needs chromium in PATH and the Tailwind CLI (TELA_TAILWIND, or
// tailwindcss in PATH, installed in a node_modules directory). Set
// TELA_CHROMIUM_DUMP to a directory to get Tela's renders and amplified
// difference images there.

// chromiumFonts loads the embedded fonts in the page, relative to
// testdata/chromium.
const chromiumFonts = `@font-face { font-family: "Inter"; src: url("../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }
@font-face { font-family: "Inter"; src: url("../../../assets/fonts/inter/InterVariable-Italic.ttf"); font-weight: 100 900; font-style: italic; }
`

func regenerateGallery(t *testing.T, dir string) {
	htmlPath, err := filepath.Abs(filepath.Join(dir, "gallery.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(galleryHTML()), 0o644); err != nil {
		t.Fatal(err)
	}
	chromium.Tailwind(t, htmlPath, filepath.Join(dir, "gallery.css"),
		"@theme { --font-sans: \"Inter\"; }\n"+chromiumFonts)
	w, h := gallerySize()
	for _, scale := range []int{1, 2} {
		shot, err := filepath.Abs(filepath.Join(dir, fmt.Sprintf("gallery_x%d.png", scale)))
		if err != nil {
			t.Fatal(err)
		}
		chromium.Screenshot(t, "file://"+htmlPath, shot, int(w), int(h), scale)
		// Chromium compresses fast; keep the committed file smaller.
		golden.WriteBestPNG(t, shot, golden.ReadRGBA(t, shot))
	}
}

// boxDiff is the difference between two renders over one cell.
type boxDiff struct {
	max  int     // largest channel difference
	mean float64 // mean channel difference (RGB)
	over int     // pixels with a channel difference above 16
}

func diffRegion(a, b *image.RGBA, r image.Rectangle) boxDiff {
	var d boxDiff
	sum, n := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pa, pb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			m := 0
			for _, v := range [3]int{int(pa.R) - int(pb.R), int(pa.G) - int(pb.G), int(pa.B) - int(pb.B)} {
				v = max(v, -v)
				sum += v
				m = max(m, v)
			}
			d.max = max(d.max, m)
			if m > 16 {
				d.over++
			}
			n++
		}
	}
	d.mean = float64(sum) / float64(3*n)
	return d
}

// chromiumBound is the largest difference accepted for a cell, with the
// reason it is larger than the default.
type chromiumBound struct {
	max  int
	mean float64
	why  string
}

// defaultChromiumBound applies to the cells not listed in chromiumBounds.
// Measured (Chromium 152): flat areas, shadows, blurs, backdrop filters
// and transforms agree within 1-5 levels away from the edges of shapes,
// and the mean difference of a cell stays under 0.4. The largest
// differences are single anti-aliased pixels on curves: Skia's rounded
// rectangle and circle coverage is not the exact area coverage of our
// rasterizer (up to 71 levels on a rounded-full edge at x2), and the
// diagonal seam where border sides of different colors meet (63 levels).
var defaultChromiumBound = chromiumBound{max: 80, mean: 0.5}

// gradientWhy explains the gradient cells, which have tighter bounds than
// the default to keep the color pipeline honest.
const gradientWhy = "stops are extended sRGB, interpolated unclipped and clipped per pixel like Chromium; " +
	"the interior agrees within a few levels, the max is the anti-aliased edge of the rounded shape"

// chromiumBounds lists the cells whose bound differs from the default.
var chromiumBounds = map[string]chromiumBound{
	"dashed": {110, 0.7, "dash phase along the corner arcs: Skia dashes its own conic arcs, measured exactly"},
	"dotted": {110, 0.7, "dot positions along the circle (Skia's conics and arc lengths)"},
	"ring-offset": {80, 0.75, "Skia's circle anti-aliasing on three concentric edges two pixels apart " +
		"(the box, the offset and the ring) adds up on every edge pixel"},
	"linear":       {50, 0.25, gradientWhy},
	"linear-via":   {45, 0.25, gradientWhy},
	"linear-angle": {4, 0.2, gradientWhy + " (no rounded edge here)"},
	"radial":       {70, 0.45, gradientWhy},
	"conic":        {75, 0.45, gradientWhy},
	"counter": {200, 0.4, "text: Skia's glyph anti-aliasing differs from ours on glyph edges (up to 189 levels); at x2 " +
		"Blink quantizes line metrics in device pixels, so baselines move by one device pixel"},
}

func TestChromiumGallery(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateGallery(t, dir)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "gallery.html")); err != nil || string(h) != galleryHTML() {
		t.Fatal("testdata/chromium/gallery.html is out of date: the gallery changed; regenerate with TELA_CHROMIUM=1")
	}
	names, rects := galleryRects()
	w, h := gallerySize()
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	diffs := make([][2]boxDiff, len(names))
	for si, scale := range []int{1, 2} {
		want := golden.ReadRGBA(t, filepath.Join(dir, fmt.Sprintf("gallery_x%d.png", scale)))
		_, got := Render(galleryRoot().node(), RenderOptions{
			Config: Config{Fonts: testFonts, RootClass: "bg-white"},
			Width:  w, Height: h, Scale: float32(scale),
		})
		if got.Rect != want.Rect {
			t.Fatalf("x%d: render %v, Chromium %v", scale, got.Rect, want.Rect)
		}
		if dump != "" {
			golden.WritePNG(t, filepath.Join(dump, fmt.Sprintf("gallery_x%d_tela.png", scale)), got)
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
			golden.WritePNG(t, filepath.Join(dump, fmt.Sprintf("gallery_x%d_diff.png", scale)), d)
		}
		for i := range names {
			r := rects[i]
			diffs[i][si] = diffRegion(got, want, image.Rect(r[0]*scale, r[1]*scale, r[2]*scale, r[3]*scale))
		}
	}
	var sb strings.Builder
	sb.WriteString("cell                    x1 max   mean  >16 | x2 max   mean  >16\n")
	for i, name := range names {
		d1, d2 := diffs[i][0], diffs[i][1]
		fmt.Fprintf(&sb, "%-22s %6d %6.3f %4d | %6d %6.3f %4d\n", name, d1.max, d1.mean, d1.over, d2.max, d2.mean, d2.over)
		b, known := chromiumBounds[name]
		if !known {
			b = defaultChromiumBound
		}
		for si, d := range diffs[i] {
			if d.max > b.max || d.mean > b.mean {
				why := ""
				if known {
					why = " (" + b.why + ")"
				}
				t.Errorf("x%d %s: max %d mean %.3f, want at most %d and %.2f%s", si+1, name, d.max, d.mean, b.max, b.mean, why)
			}
		}
	}
	t.Log("difference from Chromium per cell (channel levels):\n" + sb.String())
}
