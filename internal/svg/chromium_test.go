package svg

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The Chromium oracle (ADR-095). Every file of testdata/chromium/cells is
// a 100x100 SVG document covering a group of features; the gallery page
// (testdata/chromium/gallery.html) shows each one with <img> in a grid,
// plus inline <svg> cells whose root gets CSS from the page (color,
// stroke-width, fill), which Tela receives as Options. With TELA_CHROMIUM=1
// the test rewrites the page and takes the screenshots with headless
// Chromium (TELA_CHROMIUM_BIN, default "chromium"):
//
//	chromium --headless --disable-gpu --hide-scrollbars --force-device-scale-factor=N \
//	  --window-size=W,H --screenshot=xN.png gallery.html
//
// at scales 1 and 2 (Chromium 154.0.8037.97, Arch Linux, for the committed
// screenshots). TestChromiumSVG renders the same cells with Draw and the
// CPU rasterizer and compares each cell: the mean difference per RGB
// channel, and the largest difference away from anti-aliased edges
// (pixels whose 3x3 neighborhood varies by less than 24 levels in both
// images). The bounds are the measured values plus a margin.

const (
	galleryCols  = 6
	galleryPitch = 110
	galleryPad   = 10
	cellSize     = 100
)

// galleryCell is one cell of the gallery.
type galleryCell struct {
	name string
	// inline cells: the svg markup (parsed by Tela as is), the CSS the
	// page applies to its root and the wrapper's color, and the Options
	// that express them.
	inline, css, color string
	opts               Options
}

var inlineCells = []galleryCell{
	{
		name:   "inline-current-color",
		inline: `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M8 12l3 3 5-6"/><rect x="2" y="2" width="4" height="4" fill="currentColor" stroke="none" color="#f59e0b"/></svg>`,
		color:  "#7c3aed",
		opts:   Options{CurrentColor: gfx.MustParseHex("#7c3aed")},
	},
	{
		name:   "inline-stroke-override",
		inline: `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" color="#dc2626" stroke-linecap="round"><path d="M4 6h16M4 12h16"/><path d="M4 18h16" stroke-width="3"/><circle cx="12" cy="12" r="10" stroke-width="inherit"/></svg>`,
		css:    "stroke-width: 0.75; color: #0d9488",
		color:  "#000000",
		opts:   Options{Root: Root{StrokeWidth: 0.75, HasStrokeWidth: true, Color: gfx.MustParseHex("#0d9488"), HasColor: true}},
	},
	{
		name:   "inline-fill-override",
		inline: `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 10 10" fill="#2563eb" stroke="#000" stroke-width=".5"><rect x="1" y="1" width="3.5" height="3.5"/><rect x="5.5" y="1" width="3.5" height="3.5" fill="#16a34a"/><g><rect x="1" y="5.5" width="3.5" height="3.5"/><rect x="5.5" y="5.5" width="3.5" height="3.5" stroke="#f43f5e"/></g></svg>`,
		css:    "fill: currentColor; stroke: none; color: #ea580c",
		color:  "#000000",
		opts: Options{Root: Root{Fill: Paint{Kind: PaintCurrentColor}, Stroke: Paint{Kind: PaintNone},
			Color: gfx.MustParseHex("#ea580c"), HasColor: true}},
	},
	{
		name:   "inline-fill-color",
		inline: `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 10 10" stroke="currentColor"><rect x="1" y="1" width="8" height="3.5"/><rect x="1" y="5.5" width="8" height="3.5" fill="none" stroke-dasharray="1"/></svg>`,
		css:    "fill: #a855f7; stroke: #0369a1",
		color:  "#000000",
		opts:   Options{Root: Root{Fill: Paint{Kind: PaintColor, Color: gfx.MustParseHex("#a855f7")}, Stroke: Paint{Kind: PaintColor, Color: gfx.MustParseHex("#0369a1")}}},
	},
}

// galleryCells lists the file cells (sorted) and then the inline cells.
func galleryCells(t testing.TB) []galleryCell {
	entries, err := os.ReadDir(filepath.Join("testdata", "chromium", "cells"))
	if err != nil {
		t.Fatal(err)
	}
	var cells []galleryCell
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".svg"); ok {
			cells = append(cells, galleryCell{name: name, opts: Options{CurrentColor: gfx.Black}})
		}
	}
	return append(cells, inlineCells...)
}

func (c galleryCell) source(t testing.TB) []byte {
	if c.inline != "" {
		return []byte(c.inline)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "chromium", "cells", c.name+".svg"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func cellOrigin(i int) (x, y int) {
	return galleryPad + (i%galleryCols)*galleryPitch, galleryPad + (i/galleryCols)*galleryPitch
}

func gallerySize(n int) (w, h int) {
	rows := (n + galleryCols - 1) / galleryCols
	return 2*galleryPad + galleryCols*galleryPitch - (galleryPitch - cellSize), 2*galleryPad + rows*galleryPitch - (galleryPitch - cellSize)
}

func galleryHTML(cells []galleryCell) string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	sb.WriteString("html, body { margin: 0; background: #fff; }\n.cell { position: absolute; width: 100px; height: 100px; }\n.cell > svg { display: block; }\n")
	for i, c := range cells {
		if c.css != "" {
			fmt.Fprintf(&sb, "#c%d > svg { %s; }\n", i, c.css)
		}
	}
	sb.WriteString("</style></head><body>\n")
	for i, c := range cells {
		x, y := cellOrigin(i)
		if c.inline != "" {
			fmt.Fprintf(&sb, "<div class=\"cell\" id=\"c%d\" style=\"left: %dpx; top: %dpx; color: %s\">%s</div>\n", i, x, y, c.color, c.inline)
		} else {
			fmt.Fprintf(&sb, "<img class=\"cell\" src=\"cells/%s.svg\" style=\"left: %dpx; top: %dpx\">\n", c.name, x, y)
		}
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

// renderGallery draws every cell with Draw and rasterizes the page.
func renderGallery(t testing.TB, cells []galleryCell, scale float32) *image.RGBA {
	w, h := gallerySize(len(cells))
	b := gfx.NewBuilder()
	for i, c := range cells {
		doc, err := Parse(c.source(t))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		x, y := cellOrigin(i)
		doc.Draw(b, gfx.RectXYWH(float32(x), float32(y), cellSize, cellSize), c.opts)
	}
	img := image.NewRGBA(image.Rect(0, 0, int(float32(w)*scale), int(float32(h)*scale)))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, b.Finish(), raster.Options{Scale: scale})
	return img
}

// cellBound is the largest accepted mean and interior maximum of a cell.
type cellBound struct {
	mean float64
	imax int
}

// defaultBound applies to the cells not in cellBounds. Measured with
// Chromium 154: flat areas and gradient interiors agree within 2 levels
// (gradients: ramp rounding; opacity layers: one rounding level), and
// whole cells within 0.6 levels on average. The maxima (up to 203 levels,
// reported but not bounded) are single pixels on anti-aliased edges and
// dash ends: Skia's coverage of curves and thin strokes is not the exact
// area coverage of the rasterizer, and Skia measures dash positions on
// curves from its own flattening.
var defaultBound = cellBound{mean: 0.6, imax: 3}

// cellBounds holds the cells measured above the default (the larger of
// the two scales plus 0.05), with the reason.
var cellBounds = map[string]cellBound{
	// Thin strokes and dash ends on circles at scale 1: anti-aliasing.
	"grad-stroke":            {0.85, 3},
	"shapes":                 {0.80, 3},
	"stroke-dash":            {0.93, 3},
	"stroke-joins":           {0.73, 3},
	"inline-stroke-override": {1.06, 3},
	// A layer at opacity 0.6 over the whole cell: one rounding level on
	// every pixel, plus dash ends.
	"root-style": {0.72, 3},
	// A corner pixel of a circle clipped by its viewport: Skia's coverage
	// of the clip edge.
	"nested-svg": {0.6, 7},
}

type cellStats struct {
	mean       float64
	max, imax  int
	edgePixels int
}

// compareCell compares the RGB channels of a and b over r.
func compareCell(a, b *image.RGBA, r image.Rectangle) cellStats {
	var st cellStats
	var sum, n float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pa, pb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			m := 0
			for _, d := range [3]int{int(pa.R) - int(pb.R), int(pa.G) - int(pb.G), int(pa.B) - int(pb.B)} {
				d = max(d, -d)
				sum += float64(d)
				m = max(m, d)
			}
			n += 3
			st.max = max(st.max, m)
			if isEdge(a, x, y, r) || isEdge(b, x, y, r) {
				st.edgePixels++
				continue
			}
			st.imax = max(st.imax, m)
		}
	}
	st.mean = sum / n
	return st
}

// isEdge reports whether a pixel's 3x3 neighborhood (inside r) varies by
// 24 levels or more in some channel: an anti-aliased edge or a hard stop.
func isEdge(img *image.RGBA, x, y int, r image.Rectangle) bool {
	c := img.RGBAAt(x, y)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			p := image.Pt(x+dx, y+dy)
			if !p.In(r) {
				continue
			}
			q := img.RGBAAt(p.X, p.Y)
			for _, d := range [3]int{int(c.R) - int(q.R), int(c.G) - int(q.G), int(c.B) - int(q.B)} {
				if d >= 24 || d <= -24 {
					return true
				}
			}
		}
	}
	return false
}

func readPNG(t testing.TB, path string) *image.RGBA {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

func writePNG(t testing.TB, path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// regenerate writes the page and takes the Chromium screenshots.
func regenerate(t *testing.T, cells []galleryCell) {
	dir, err := filepath.Abs(filepath.Join("testdata", "chromium"))
	if err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "gallery.html")
	if err := os.WriteFile(page, []byte(galleryHTML(cells)), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := os.Getenv("TELA_CHROMIUM_BIN")
	if bin == "" {
		bin = "chromium"
	}
	w, h := gallerySize(len(cells))
	for _, scale := range []int{1, 2} {
		out := filepath.Join(dir, fmt.Sprintf("x%d.png", scale))
		cmd := exec.Command(bin, "--headless", "--disable-gpu", "--hide-scrollbars",
			fmt.Sprintf("--force-device-scale-factor=%d", scale), fmt.Sprintf("--window-size=%d,%d", w, h),
			"--screenshot="+out, "file://"+page)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", bin, err, b)
		}
	}
}

func TestChromiumSVG(t *testing.T) {
	cells := galleryCells(t)
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerate(t, cells)
	}
	dir := filepath.Join("testdata", "chromium")
	if h, err := os.ReadFile(filepath.Join(dir, "gallery.html")); err != nil || string(h) != galleryHTML(cells) {
		t.Fatalf("%s/gallery.html is out of date with the cells; regenerate it and the screenshots with TELA_CHROMIUM=1", dir)
	}
	dump := os.Getenv("TELA_CHROMIUM_DUMP")
	var report strings.Builder
	report.WriteString("cell                      x1 mean imax  max | x2 mean imax  max\n")
	stats := make([][2]cellStats, len(cells))
	for si, scale := range []int{1, 2} {
		got := renderGallery(t, cells, float32(scale))
		ref := readPNG(t, filepath.Join(dir, fmt.Sprintf("x%d.png", scale)))
		if ref.Rect != got.Rect {
			t.Fatalf("x%d: render %v, Chromium %v", scale, got.Rect, ref.Rect)
		}
		if dump != "" {
			writePNG(t, filepath.Join(dump, fmt.Sprintf("svg_x%d_tela.png", scale)), got)
		}
		for i := range cells {
			x, y := cellOrigin(i)
			r := image.Rect(x*scale, y*scale, (x+cellSize)*scale, (y+cellSize)*scale)
			stats[i][si] = compareCell(got, ref, r)
		}
	}
	for i, c := range cells {
		b, ok := cellBounds[c.name]
		if !ok {
			b = defaultBound
		}
		s1, s2 := stats[i][0], stats[i][1]
		fmt.Fprintf(&report, "%-24s %7.3f %4d %4d | %7.3f %4d %4d\n", c.name, s1.mean, s1.imax, s1.max, s2.mean, s2.imax, s2.max)
		for si, s := range stats[i] {
			if s.mean > b.mean || s.imax > b.imax {
				t.Errorf("x%d %s: mean %.3f interior max %d (max %d), bound %.2f %d", si+1, c.name, s.mean, s.imax, s.max, b.mean, b.imax)
			}
		}
	}
	t.Log("\n" + report.String())
}

// TestGoldenGallery pins Tela's own rendering of the gallery, bit for bit.
func TestGoldenGallery(t *testing.T) {
	cells := galleryCells(t)
	for _, scale := range []float32{1, 2} {
		name := fmt.Sprintf("gallery_x%g", scale)
		t.Run(name, func(t *testing.T) {
			golden.Image(t, name, renderGallery(t, cells, scale))
		})
	}
}
