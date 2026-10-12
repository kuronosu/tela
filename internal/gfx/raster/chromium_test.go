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

// The Tailwind shadow scene (drawTailwindScene) is also written as plain
// HTML/CSS, testdata/chromium/tailwind_shadows.html, and rendered by
// Chromium into testdata/chromium/tailwind_shadows_x1.png and _x2.png with
//
//	chromium --headless --disable-gpu --hide-scrollbars \
//	  --force-device-scale-factor=1 --window-size=640,600 \
//	  --screenshot=tailwind_shadows_x1.png tailwind_shadows.html
//
// (Chromium 152.0.7977.82, Arch Linux; --force-device-scale-factor=2 for
// _x2). TestChromiumShadows compares
// those screenshots with our render. Regenerate the HTML with
// TELA_WRITE_HTML=1 go test -run TestChromiumShadows.

// cssColor formats a color as CSS rgb() with fractional channels.
func cssColor(c gfx.Color) string {
	return fmt.Sprintf("rgb(%.3f %.3f %.3f / %.3f)", c.R*255, c.G*255, c.B*255, c.A)
}

func cssShadows(layers []cssShadow) string {
	var parts []string
	for _, l := range layers {
		s := fmt.Sprintf("%gpx %gpx %gpx %gpx %s", l.x, l.y, l.blur, l.spread, cssColor(l.color))
		if l.inset {
			s = "inset " + s
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// tailwindHTML returns the HTML of the Tailwind shadow scene.
func tailwindHTML() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	sb.WriteString("html, body { margin: 0; padding: 0; overflow: hidden; }\n")
	fmt.Fprintf(&sb, "body { width: 640px; height: 600px; background: %s; position: relative; }\n", cssColor(twGray100))
	sb.WriteString("div { position: absolute; box-sizing: border-box; }\n</style></head><body>\n")
	for _, c := range twCards() {
		r := c.box.Rect
		rad := c.box.Radii
		style := fmt.Sprintf("left: %gpx; top: %gpx; width: %gpx; height: %gpx; background: %s; "+
			"border-radius: %gpx %gpx %gpx %gpx / %gpx %gpx %gpx %gpx;",
			r.X0, r.Y0, r.Width(), r.Height(), cssColor(c.bgColor),
			rad[0].X, rad[1].X, rad[2].X, rad[3].X, rad[0].Y, rad[1].Y, rad[2].Y, rad[3].Y)
		layers := append(append([]cssShadow(nil), c.inner...), c.outer...)
		if len(layers) > 0 {
			style += " box-shadow: " + cssShadows(layers) + ";"
		}
		if c.rotate != 0 {
			style += fmt.Sprintf(" transform: rotate(%gdeg);", c.rotate)
		}
		fmt.Fprintf(&sb, "<div title=%q style=%q></div>\n", c.name, style)
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

// Tolerances against Chromium. Away from the anti-aliased edges of the
// cards and of the hard (unblurred) shadows (two pixels around them), the
// shadows agree within chromiumTolerance. On the edges, two known
// differences add up to chromiumEdgeTolerance: Skia's anti-aliasing of
// rounded corners differs from our exact area coverage, and Blink does not
// clip an outer shadow when the box's background is opaque (the background
// covers it), while DrawShadow always clips with the box's coverage, so a
// partly covered edge pixel shows the background through both the shadow
// and the fill (conflation).
const (
	chromiumTolerance     = 3
	chromiumEdgeTolerance = 32
	chromiumMaxMean       = 0.15
)

func TestChromiumShadows(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_WRITE_HTML") == "1" {
		if err := os.WriteFile(filepath.Join(dir, "tailwind_shadows.html"), []byte(tailwindHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	html, err := os.ReadFile(filepath.Join(dir, "tailwind_shadows.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(html) != tailwindHTML() {
		t.Fatalf("testdata/chromium/tailwind_shadows.html is out of date with the scene; regenerate it with TELA_WRITE_HTML=1 and the screenshots with Chromium")
	}
	for _, scale := range []float32{1, 2} {
		name := fmt.Sprintf("tailwind_shadows_x%g.png", scale)
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		ref, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := renderScene(scenes3[0], scale)
		// The anti-aliased edges of the cards and of the hard shadows,
		// grown by two pixels (Skia's anti-aliasing ramps are wider).
		edge := make([]bool, got.Rect.Dx()*got.Rect.Dy())
		mark := func(draw func(b *gfx.Builder)) {
			img := image.NewRGBA(got.Rect)
			Render(img, build(draw), Options{Scale: scale})
			for i := range edge {
				if a := img.Pix[4*i+3]; a != 0 && a != 255 {
					edge[i] = true
				}
			}
		}
		for _, c := range twCards() {
			c := c
			mark(func(b *gfx.Builder) {
				if c.rotate != 0 {
					b.PushTransform(gfx.RotateAbout(float32(float64(c.rotate)*math.Pi/180), c.box.Rect.Center()))
				}
				b.FillRRect(c.box, gfx.Black)
				if c.rotate != 0 {
					b.PopTransform()
				}
			})
			for _, l := range append(append([]cssShadow(nil), c.inner...), c.outer...) {
				if l.blur == 0 {
					shape, _ := shadowShape(gfx.DrawShadow{RRect: c.box, Offset: gfx.Pt(l.x, l.y), Spread: l.spread, Inset: l.inset})
					mark(func(b *gfx.Builder) { b.FillRRect(shape, gfx.Black) })
				}
			}
		}
		w := got.Rect.Dx()
		onEdge := func(x, y int) bool {
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if p := image.Pt(x+dx, y+dy); p.In(got.Rect) && edge[p.Y*w+p.X] {
						return true
					}
				}
			}
			return false
		}
		st := compareOpaque(got, ref, onEdge)
		t.Logf("%s: max channel difference %d at %v away from card edges, %d at %v on them; mean %.3f",
			name, st.worst, st.at, st.edgeWorst, st.edgeAt, st.mean)
		if st.worst > chromiumTolerance || st.edgeWorst > chromiumEdgeTolerance || st.mean > chromiumMaxMean {
			t.Errorf("%s: max channel difference %d at %v (allowed %d), on edges %d at %v (allowed %d), mean %.3f (allowed %.2f)",
				name, st.worst, st.at, chromiumTolerance, st.edgeWorst, st.edgeAt, chromiumEdgeTolerance, st.mean, chromiumMaxMean)
		}
	}
}

type diffStats struct {
	worst, edgeWorst int
	at, edgeAt       image.Point
	mean             float64
}

// compareOpaque compares two opaque images per RGB channel, separately for
// the pixels where onEdge is true.
func compareOpaque(a *image.RGBA, b image.Image, onEdge func(x, y int) bool) diffStats {
	var st diffStats
	var sum, n float64
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			i := a.PixOffset(x, y)
			r, g, bb, _ := b.At(x+b.Bounds().Min.X, y+b.Bounds().Min.Y).RGBA()
			edge := onEdge(x, y)
			for c, v := range [3]uint32{r >> 8, g >> 8, bb >> 8} {
				d := iabs(int(a.Pix[i+c]) - int(v))
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
