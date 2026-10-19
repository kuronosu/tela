package raster

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// The Tailwind blur presets (blur-xs ... blur-3xl, backdrop-blur-xs ...
// backdrop-blur-3xl, drop-shadow-xs ... drop-shadow-2xl) drawn as a scene
// and written as HTML, testdata/chromium/blur.html, which Chromium renders
// into blur_x1.png and blur_x2.png with
//
//	chromium --headless --disable-gpu --hide-scrollbars \
//	  --force-device-scale-factor=1 --window-size=1400,600 \
//	  --screenshot=blur_x1.png blur.html
//
// (Chromium 152.0.7977.82, Arch Linux; --force-device-scale-factor=2 for
// _x2). Regenerate the HTML with TELA_WRITE_HTML=1 go test -run
// TestChromiumBlur.
//
// TestChromiumBlur compares every cell, separately on and away from the
// anti-aliased edges of the unblurred shapes (the glass panels and the
// shapes under drop shadows, grown by two pixels), where Skia's coverage
// differs from the exact area coverage (ADR-028).

// blurPresets are Tailwind v4's blur scale (blur-* and backdrop-blur-*):
// the CSS blur() radius, which is the standard deviation.
var blurPresets = []struct {
	name string
	px   float32
}{{"xs", 4}, {"sm", 8}, {"md", 12}, {"lg", 16}, {"xl", 24}, {"2xl", 40}, {"3xl", 64}}

// dropPresets are Tailwind v4's drop-shadow-* presets (offset y, blur
// radius, black alpha), and a colored, opaque 2xl that makes differences
// visible. Chromium 152 blurs drop-shadow() with a standard deviation equal
// to the blur radius (Filter Effects 1: stdDeviation="[radius]"), not half
// of it as box-shadow does, and so does this scene.
var dropPresets = []struct {
	name  string
	y, r  float32
	color gfx.Color
}{
	{"xs", 1, 1, gfx.Black.WithAlpha(0.05)},
	{"sm", 1, 2, gfx.Black.WithAlpha(0.15)},
	{"md", 3, 3, gfx.Black.WithAlpha(0.12)},
	{"lg", 4, 4, gfx.Black.WithAlpha(0.15)},
	{"xl", 9, 7, gfx.Black.WithAlpha(0.1)},
	{"2xl", 25, 25, gfx.Black.WithAlpha(0.15)},
	{"2xl indigo", 25, 25, indigo},
}

const blurCell = 200 // cell pitch; cells are 196x196

// blurCellRect returns the rectangle of cell i of row.
func blurCellRect(row, i int) gfx.Rect {
	return gfx.RectXYWH(float32(blurCell*i+2), float32(blurCell*row+2), blurCell-4, blurCell-4)
}

// drawBlurScene draws the three rows: layer blurs of a disc and a
// translucent rounded square, glass panels over stripes, drop shadows of
// a white rounded square and a rotated amber one.
func drawBlurScene(b *gfx.Builder) {
	for i, p := range blurPresets {
		c := blurCellRect(0, i)
		b.PushClipRect(c)
		b.FillRect(c, slate100)
		b.PushLayer(1, gfx.SrcOver, gfx.Blur{StdDev: p.px})
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(c.X0+30, c.Y0+30, 96, 96), 48), indigo)
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(c.X0+80, c.Y0+80, 80, 80), 16), rose.WithAlpha(0.8))
		b.PopLayer()
		b.PopClip()
	}
	for i, p := range blurPresets {
		c := blurCellRect(1, i)
		b.PushClipRect(c)
		drawStripes(b, c)
		panel := glassPanel(c)
		b.BackdropFilter(panel, gfx.Blur{StdDev: p.px})
		b.FillRRect(panel, gfx.White.WithAlpha(0.2))
		b.PopClip()
	}
	for i, p := range dropPresets {
		c := blurCellRect(2, i)
		b.PushClipRect(c)
		b.FillRect(c, slate100)
		b.PushLayer(1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(0, p.y), StdDev: p.r, Color: p.color})
		drawDropContent(b, c)
		b.PopLayer()
		b.PopClip()
	}
}

var slate100 = gfx.MustParseHex("#f1f5f9")

// drawStripes fills a cell with 16 px vertical stripes of two colors and
// a dark disc: hard edges under the glass.
func drawStripes(b *gfx.Builder, c gfx.Rect) {
	b.FillRect(c, amber)
	for x := c.X0; x < c.X1; x += 32 {
		b.FillRect(gfx.Rect{X0: x, Y0: c.Y0, X1: min(x+16, c.X1), Y1: c.Y1}, sky)
	}
	b.FillRRect(gfx.NewRRect(gfx.RectXYWH(c.X0+56, c.Y0+56, 84, 84), 42), slate)
}

// glassPanel is the panel of a backdrop cell.
func glassPanel(c gfx.Rect) gfx.RRect {
	return gfx.NewRRect(gfx.RectXYWH(c.X0+30, c.Y0+30, 136, 136), 12)
}

// dropSquare and dropDiamond are the shapes of a drop-shadow cell.
func dropSquare(c gfx.Rect) gfx.RRect {
	return gfx.NewRRect(gfx.RectXYWH(c.X0+28, c.Y0+28, 80, 80), 12)
}

func dropDiamond(c gfx.Rect) gfx.Rect { return gfx.RectXYWH(c.X0+96, c.Y0+70, 60, 60) }

func drawDropContent(b *gfx.Builder, c gfx.Rect) {
	b.FillRRect(dropSquare(c), gfx.White)
	d := dropDiamond(c)
	b.PushTransform(gfx.RotateAbout(math.Pi/4, d.Center()))
	b.FillRect(d, amber)
	b.PopTransform()
}

func blurHTML() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	sb.WriteString("html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }\n")
	sb.WriteString("div { position: absolute; box-sizing: border-box; }\n</style></head><body>\n")
	cell := func(c gfx.Rect, bg string) string {
		return fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; height: %gpx; overflow: hidden; background: %s;",
			c.X0, c.Y0, c.Width(), c.Height(), bg)
	}
	at := func(x, y, w, h float32) string {
		return fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; height: %gpx;", x, y, w, h)
	}
	for i, p := range blurPresets {
		c := blurCellRect(0, i)
		fmt.Fprintf(&sb, "<div title=\"blur-%s\" style=%q><div style=%q>", p.name, cell(c, cssColor(slate100)),
			at(0, 0, c.Width(), c.Height())+fmt.Sprintf(" filter: blur(%gpx);", p.px))
		fmt.Fprintf(&sb, "<div style=%q></div>", at(30, 30, 96, 96)+" border-radius: 48px; background: "+cssColor(indigo)+";")
		fmt.Fprintf(&sb, "<div style=%q></div>", at(80, 80, 80, 80)+" border-radius: 16px; background: "+cssColor(rose.WithAlpha(0.8))+";")
		sb.WriteString("</div></div>\n")
	}
	for i, p := range blurPresets {
		c := blurCellRect(1, i)
		fmt.Fprintf(&sb, "<div title=\"backdrop-blur-%s\" style=%q>", p.name, cell(c, cssColor(amber)))
		for x := float32(0); x < c.Width(); x += 32 {
			fmt.Fprintf(&sb, "<div style=%q></div>", at(x, 0, min(16, c.Width()-x), c.Height())+" background: "+cssColor(sky)+";")
		}
		fmt.Fprintf(&sb, "<div style=%q></div>", at(56, 56, 84, 84)+" border-radius: 42px; background: "+cssColor(slate)+";")
		fmt.Fprintf(&sb, "<div style=%q></div>", at(30, 30, 136, 136)+
			fmt.Sprintf(" border-radius: 12px; background: %s; backdrop-filter: blur(%gpx);", cssColor(gfx.White.WithAlpha(0.2)), p.px))
		sb.WriteString("</div>\n")
	}
	for i, p := range dropPresets {
		c := blurCellRect(2, i)
		fmt.Fprintf(&sb, "<div title=\"drop-shadow-%s\" style=%q><div style=%q>", p.name, cell(c, cssColor(slate100)),
			at(0, 0, c.Width(), c.Height())+fmt.Sprintf(" filter: drop-shadow(0 %gpx %gpx %s);", p.y, p.r, cssColor(p.color)))
		fmt.Fprintf(&sb, "<div style=%q></div>", at(28, 28, 80, 80)+" border-radius: 12px; background: #fff;")
		fmt.Fprintf(&sb, "<div style=%q></div>", at(96, 70, 60, 60)+" transform: rotate(45deg); background: "+cssColor(amber)+";")
		sb.WriteString("</div></div>\n")
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

// blurEdges marks the device pixels within two pixels of an anti-aliased
// edge of an unblurred shape: the glass panels and the drop-shadow
// content.
func blurEdges(w, h int, scale float32) []bool {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	edge := make([]bool, w*h)
	// Each shape is marked on its own: where the diamond overlaps the
	// square, its edge is still an edge.
	var shapes []func(b *gfx.Builder)
	for i := range blurPresets {
		shapes = append(shapes, func(b *gfx.Builder) { b.FillRRect(glassPanel(blurCellRect(1, i)), gfx.Black) })
	}
	for i := range dropPresets {
		c := blurCellRect(2, i)
		shapes = append(shapes, func(b *gfx.Builder) { b.FillRRect(dropSquare(c), gfx.Black) }, func(b *gfx.Builder) {
			d := dropDiamond(c)
			b.PushTransform(gfx.RotateAbout(math.Pi/4, d.Center()))
			b.FillRect(d, gfx.Black)
			b.PopTransform()
		})
	}
	for _, draw := range shapes {
		clear(img.Pix)
		Render(img, build(draw), Options{Scale: scale})
		for y := range h {
			for x := range w {
				if a := img.Pix[4*(y*w+x)+3]; a == 0 || a == 255 {
					continue
				}
				for dy := -2; dy <= 2; dy++ {
					for dx := -2; dx <= 2; dx++ {
						if p := image.Pt(x+dx, y+dy); p.In(img.Rect) {
							edge[p.Y*w+p.X] = true
						}
					}
				}
			}
		}
	}
	return edge
}

// blurBound is the largest accepted difference of a cell from Chromium:
// the maximum channel difference away from anti-aliased edges, on them,
// and the mean.
type blurBound struct {
	max, edge int
	mean      float64
}

// blurBounds are the bounds per cell, measured against Chromium 152: 3
// levels away from the edges (ADR-028), the measured edge maximum plus 2
// (Skia's anti-aliasing of the glass panels' rounded corners and of the
// drop-shadow content's edges), the larger mean of the two scales plus
// 0.05. The downsampled blurs (standard deviations from about 21 device
// pixels, ADR-078) gave the same maxima as the full-resolution ones and
// means within 0.002.
var blurBounds = map[string]blurBound{
	"blur-xs":                {3, 0, 0.18},
	"blur-sm":                {3, 0, 0.25},
	"blur-md":                {3, 0, 0.31},
	"blur-lg":                {3, 0, 0.36},
	"blur-xl":                {3, 0, 0.40},
	"blur-2xl":               {3, 0, 0.41},
	"blur-3xl":               {3, 0, 0.44},
	"backdrop-blur-xs":       {3, 27, 0.27},
	"backdrop-blur-sm":       {3, 30, 0.27},
	"backdrop-blur-md":       {3, 29, 0.27},
	"backdrop-blur-lg":       {3, 26, 0.28},
	"backdrop-blur-xl":       {3, 25, 0.27},
	"backdrop-blur-2xl":      {3, 21, 0.27},
	"backdrop-blur-3xl":      {3, 22, 0.19},
	"drop-shadow-xs":         {3, 24, 0.07},
	"drop-shadow-sm":         {3, 24, 0.08},
	"drop-shadow-md":         {3, 24, 0.09},
	"drop-shadow-lg":         {3, 24, 0.09},
	"drop-shadow-xl":         {3, 24, 0.10},
	"drop-shadow-2xl":        {3, 24, 0.16},
	"drop-shadow-2xl indigo": {3, 25, 0.25},
}

func TestChromiumBlur(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_WRITE_HTML") == "1" {
		if err := os.WriteFile(filepath.Join(dir, "blur.html"), []byte(blurHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	html, err := os.ReadFile(filepath.Join(dir, "blur.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(html) != blurHTML() {
		t.Fatalf("testdata/chromium/blur.html is out of date with the scene; regenerate it with TELA_WRITE_HTML=1 and the screenshots with Chromium")
	}
	s := scene{"blur_presets", 1400, 600, drawBlurScene}
	for _, scale := range []float32{1, 2} {
		name := fmt.Sprintf("blur_x%g.png", scale)
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		ref, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := renderScene(s, scale)
		if d := os.Getenv("TELA_CHROMIUM_DUMP"); d != "" {
			if f, err := os.Create(filepath.Join(d, name)); err == nil {
				png.Encode(f, got)
				f.Close()
			}
		}
		w := got.Rect.Dx()
		edge := blurEdges(w, got.Rect.Dy(), scale)
		cells := []struct {
			name string
			r    gfx.Rect
		}{}
		for i, p := range blurPresets {
			cells = append(cells, struct {
				name string
				r    gfx.Rect
			}{"blur-" + p.name, blurCellRect(0, i)})
		}
		for i, p := range blurPresets {
			cells = append(cells, struct {
				name string
				r    gfx.Rect
			}{"backdrop-blur-" + p.name, blurCellRect(1, i)})
		}
		for i, p := range dropPresets {
			cells = append(cells, struct {
				name string
				r    gfx.Rect
			}{"drop-shadow-" + p.name, blurCellRect(2, i)})
		}
		for _, c := range cells {
			r := image.Rect(int(c.r.X0*scale), int(c.r.Y0*scale), int(c.r.X1*scale), int(c.r.Y1*scale))
			st := compareBlurCell(got, ref, r, func(x, y int) bool { return edge[y*w+x] })
			t.Logf("x%g %-24s max %3d at %v, edges %3d, mean %.3f", scale, c.name, st.worst, st.at, st.edgeWorst, st.mean)
			b, ok := blurBounds[c.name]
			if !ok {
				t.Errorf("no bound recorded for cell %q", c.name)
				continue
			}
			if st.worst > b.max || st.edgeWorst > b.edge || st.mean > b.mean {
				t.Errorf("x%g %s: max %d (allowed %d), on edges %d (allowed %d), mean %.3f (allowed %.2f)",
					scale, c.name, st.worst, b.max, st.edgeWorst, b.edge, st.mean, b.mean)
			}
		}
	}
}

// compareBlurCell compares the RGB channels of got and ref (same bounds,
// origin 0) over r, separately where onEdge is true.
func compareBlurCell(got *image.RGBA, ref image.Image, r image.Rectangle, onEdge func(x, y int) bool) diffStats {
	var st diffStats
	var sum, n float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := got.PixOffset(x, y)
			cr, cg, cb, _ := ref.At(x, y).RGBA()
			edge := onEdge(x, y)
			for k, v := range [3]uint32{cr >> 8, cg >> 8, cb >> 8} {
				d := iabs(int(got.Pix[i+k]) - int(v))
				sum += float64(d)
				n++
				switch {
				case edge && d > st.edgeWorst:
					st.edgeWorst, st.edgeAt = d, image.Pt(x, y)
				case !edge && d > st.worst:
					st.worst, st.at = d, image.Pt(x, y)
				}
			}
		}
	}
	st.mean = sum / n
	return st
}
