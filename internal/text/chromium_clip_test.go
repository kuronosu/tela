package text

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
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The background-clip: text comparison paints gradient backgrounds clipped
// to text the way core will (ADR-074): the backgrounds in a layer whose
// alpha mask is the element's glyph runs. Chromium renders the same divs
// (testdata/chromium/clip_text.html, fonts through @font-face) into
// clip_text_x1.png and clip_text_x2.png:
//
//	TELA_WRITE_HTML=1 go test -run TestChromiumClipText ./internal/text/
//	cd internal/text/testdata/chromium && chromium --headless --disable-gpu \
//	  --hide-scrollbars --allow-file-access-from-files --virtual-time-budget=5000 \
//	  --force-device-scale-factor=1 --window-size=560,200 \
//	  --screenshot=clip_text_x1.png "file://$PWD/clip_text.html"
//
// (and --force-device-scale-factor=2 for _x2).

type clipTextCell struct {
	text       string
	x, y, w, h float32
	size, lh   float32
	weight     float32
	bg         []gfx.Color
	color      gfx.Color // painted over the clipped background
}

var clipTextCells = []clipTextCell{
	{"Tela gradient", 16, 16, 520, 64, 56, 64, 800, []gfx.Color{gfx.RGB8(0x63, 0x66, 0xf1), gfx.RGB8(0xc0, 0x26, 0xd3), gfx.RGB8(0xf5, 0x9e, 0x0b)}, gfx.Transparent},
	{"Clip text 🎉 emoji", 16, 96, 360, 40, 32, 40, 600, []gfx.Color{gfx.RGB8(0x10, 0xb9, 0x81), gfx.RGB8(0x0e, 0xa5, 0xe9)}, gfx.Transparent},
	{"Over color", 16, 144, 360, 40, 32, 40, 700, []gfx.Color{gfx.RGB8(0xef, 0x44, 0x44), gfx.RGB8(0xf5, 0x9e, 0x0b)}, gfx.RGBA8(0x11, 0x18, 0x27, 77)},
}

func clipTextHTML() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<!doctype html>
<html><head><meta charset="utf-8"><style>
%s
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
div { position: absolute; box-sizing: border-box; white-space: nowrap; font-family: %s; }
</style></head><body>
`, chromiumFonts, `"Inter", "Noto Sans Arabic", "Noto Sans JP", "Noto Color Emoji"`)
	for _, c := range clipTextCells {
		var stops []string
		for _, s := range c.bg {
			p := s.Premul8()
			stops = append(stops, fmt.Sprintf("rgb(%d %d %d)", p.R, p.G, p.B))
		}
		col := c.color.NRGBA8()
		fmt.Fprintf(&sb, `<div style="left: %gpx; top: %gpx; width: %gpx; height: %gpx; font-size: %gpx; line-height: %gpx; font-weight: %g; background: linear-gradient(to right, %s); background-clip: text; color: rgb(%d %d %d / %.4f);">%s</div>
`, c.x, c.y, c.w, c.h, c.size, c.lh, c.weight, strings.Join(stops, ", "), col.R, col.G, col.B, float64(col.A)/255, c.text)
	}
	sb.WriteString("</body></html>\n")
	return sb.String()
}

func clipTextScene(t testing.TB) *gfx.DisplayList {
	b := gfx.NewBuilder()
	for _, c := range clipTextCells {
		st := TextStyle{Families: allFamilies, Size: c.size, Weight: c.weight, LineHeight: LineHeightPixels(c.lh), Paint: gfx.Black}
		l := NewLayout(layoutCollection(t), &Paragraph{Text: c.text, Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}, Default: st})
		l.BreakLines(float32(math.Inf(1)))
		origin := gfx.Pt(c.x, c.y)
		mb := gfx.NewBuilder()
		l.Paint(mb, origin)
		box := gfx.RectXYWH(c.x, c.y, c.w, c.h)
		b.PushMaskLayer(gfx.Mask{List: mb.Finish()}, 1, gfx.SrcOver)
		stops := make([]gfx.GradientStop, len(c.bg))
		for i, s := range c.bg {
			stops[i] = gfx.GradientStop{Offset: float32(i) / float32(len(c.bg)-1), Color: s}
		}
		b.FillRect(box, gfx.LinearGradient{Start: gfx.Pt(box.X0, box.Y0), End: gfx.Pt(box.X1, box.Y0), Stops: stops, Interp: gfx.InterpSRGB})
		b.PopLayer()
		if c.color.A > 0 {
			l.PaintWith(b, origin, 0, c.color)
		}
	}
	return b.Finish()
}

// The larger mean of scales 1 and 2 measured against Chromium 152, plus
// 0.3 (x1: 0.47, 2.84, 1.19; x2: 1.56, 3.48, 2.21). The masked
// backgrounds match; the differences are on glyph edges: anti-aliasing
// (Skia's against exact area coverage), the color emoji's edges, and at
// scale 2 Blink's baselines rounded in device pixels (ADR-048).
var clipTextBounds = []float64{1.9, 3.8, 2.5}

func TestChromiumClipText(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_WRITE_HTML") == "1" {
		if err := os.WriteFile(filepath.Join(dir, "clip_text.html"), []byte(clipTextHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	html, err := os.ReadFile(filepath.Join(dir, "clip_text.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(html) != clipTextHTML() {
		t.Fatal("testdata/chromium/clip_text.html is out of date; regenerate it with TELA_WRITE_HTML=1 and the screenshots with Chromium (see chromium_clip_test.go)")
	}
	dl := clipTextScene(t)
	for _, scale := range []float32{1, 2} {
		name := fmt.Sprintf("clip_text_x%g.png", scale)
		fh, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		ref, err := png.Decode(fh)
		fh.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := image.NewRGBA(ref.Bounds())
		raster.Clear(got, image.Rectangle{}, gfx.White)
		raster.Render(got, dl, raster.Options{Scale: scale})
		if d := os.Getenv("TELA_CHROMIUM_DUMP"); d != "" {
			if f, err := os.Create(filepath.Join(d, name)); err == nil {
				png.Encode(f, got)
				f.Close()
			}
		}
		for i, c := range clipTextCells {
			r := image.Rect(int(c.x*scale), int(c.y*scale), int((c.x+c.w)*scale), int((c.y+c.h)*scale))
			st := compareRegion(got, ref, r)
			t.Logf("x%g %q: mean %.3f, max %d, >32: %.2f%%", scale, c.text, st.mean, st.max, 100*st.far)
			if st.mean > clipTextBounds[i] {
				t.Errorf("x%g %q: mean channel difference %.3f, allowed %.2f", scale, c.text, st.mean, clipTextBounds[i])
			}
		}
	}
}
