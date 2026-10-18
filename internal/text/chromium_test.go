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

// The Chromium comparison renders goldenEmoji with Noto Color Emoji at 64
// and 20 px on integer pen positions, here and in Chromium (canvas
// fillText with the same font file loaded through @font-face), and
// compares the pixels of each emoji's cell.
//
// The screenshot is committed. To regenerate it (Chromium supports COLRv1):
//
//	TELA_WRITE_HTML=1 go test -run TestChromiumEmoji ./internal/text/
//	cd internal/text/testdata/chromium && chromium --headless --disable-gpu \
//	  --force-device-scale-factor=1 --window-size=1140,150 \
//	  --allow-file-access-from-files --virtual-time-budget=5000 \
//	  --screenshot=emoji_x1.png "file://$PWD/emoji.html"
const (
	chromiumW, chromiumH = 1140, 150
	chromiumBig          = 64
	chromiumSmall        = 20
	chromiumBigStep      = 80
	chromiumSmallStep    = 28
	chromiumBigY         = 80
	chromiumSmallY       = 130
)

type emojiCell struct {
	text string
	size float32
	x, y float32
	cell image.Rectangle
}

// chromiumEmoji adds flags with mid-tone colors, where soft-light shading
// matters most, to goldenEmoji.
var chromiumEmoji = append(append([]string(nil), goldenEmoji...), "🇧🇷", "🇩🇪")

func chromiumCells() []emojiCell {
	var cells []emojiCell
	for i, e := range chromiumEmoji {
		x := float32(16 + chromiumBigStep*i)
		cells = append(cells, emojiCell{e, chromiumBig, x, chromiumBigY,
			image.Rect(int(x)-6, chromiumBigY-64, int(x)+chromiumBigStep-6, chromiumBigY+20)})
	}
	for i, e := range chromiumEmoji {
		x := float32(16 + chromiumSmallStep*i)
		cells = append(cells, emojiCell{e, chromiumSmall, x, chromiumSmallY,
			image.Rect(int(x)-3, chromiumSmallY-20, int(x)+chromiumSmallStep-3, chromiumSmallY+8)})
	}
	return cells
}

func chromiumHTML() string {
	var items []string
	for _, c := range chromiumCells() {
		items = append(items, fmt.Sprintf("[%q, %g, %g, %g]", c.text, c.size, c.x, c.y))
	}
	return fmt.Sprintf(`<!doctype html>
<html><head><meta charset="utf-8"><style>
@font-face { font-family: "TelaEmoji"; src: url("../../../assets/fonts/notocoloremoji/Noto-COLRv1.ttf"); }
html, body { margin: 0; padding: 0; overflow: hidden; background: #fff; }
canvas { display: block; }
</style></head><body><canvas id="c" width="%d" height="%d"></canvas>
<script>
const items = [
%s
];
document.fonts.load("64px TelaEmoji", "\u{1F600}").then(() => {
  const ctx = document.getElementById("c").getContext("2d");
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, %d, %d);
  ctx.fillStyle = "#111827";
  ctx.textBaseline = "alphabetic";
  for (const [t, size, x, y] of items) {
    ctx.font = size + "px TelaEmoji";
    ctx.fillText(t, x, y);
  }
});
</script></body></html>
`, chromiumW, chromiumH, strings.Join(items, ",\n"), chromiumW, chromiumH)
}

func renderChromiumScene(t testing.TB) *image.RGBA {
	f := NewDefaultCollection().Face(Query{Families: []string{"emoji"}}, 64)
	b := gfx.NewBuilder()
	for _, c := range chromiumCells() {
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: f, Size: c.size, Origin: gfx.Pt(c.x, c.y),
			Glyphs: []gfx.Glyph{{ID: shapedGlyph(t, f, c.text)}}, Paint: gfx.RGB8(0x11, 0x18, 0x27)})
	}
	img := image.NewRGBA(image.Rect(0, 0, chromiumW, chromiumH))
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, b.Finish(), raster.Options{})
	return img
}

// Tolerances: the mean channel difference of an emoji cell, and the share
// of its channels that differ by more than 32 levels (anti-aliasing of edges
// and of hard color boundaries inside the emoji differ between Skia and
// raster). Measured with exact blend modes, mask layers and focal gradients
// (ADR-074): at most 1.69 (🇧🇷 at 20 px; flags use soft-light), against up
// to 3.74 when soft-light, non-solid SrcIn masks and focal gradients were
// approximated.
const (
	chromiumMaxMean = 2.0
	chromiumMaxFar  = 0.02
)

func TestChromiumEmoji(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_WRITE_HTML") == "1" {
		if err := os.WriteFile(filepath.Join(dir, "emoji.html"), []byte(chromiumHTML()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	html, err := os.ReadFile(filepath.Join(dir, "emoji.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(html) != chromiumHTML() {
		t.Fatal("testdata/chromium/emoji.html is out of date; regenerate it with TELA_WRITE_HTML=1 and the screenshot with Chromium (see chromium_test.go)")
	}
	fh, err := os.Open(filepath.Join(dir, "emoji_x1.png"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := png.Decode(fh)
	fh.Close()
	if err != nil {
		t.Fatal(err)
	}
	got := renderChromiumScene(t)
	if os.Getenv("TELA_CHROMIUM_DUMP") != "" {
		f, _ := os.Create(os.Getenv("TELA_CHROMIUM_DUMP"))
		png.Encode(f, got)
		f.Close()
	}
	for _, c := range chromiumCells() {
		st := compareRegion(got, ref, c.cell)
		t.Logf("%s %2gpx: max %3d, mean %.2f, >32: %.2f%%", c.text, c.size, st.max, st.mean, 100*st.far)
		limit := chromiumMaxMean
		if st.mean > limit || st.far > chromiumMaxFar {
			t.Errorf("%s at %gpx: mean channel difference %.2f (allowed %.2f), %.2f%% of the channels off by more than 32 (allowed %.0f%%)",
				c.text, c.size, st.mean, limit, 100*st.far, 100*chromiumMaxFar)
		}
	}
}

type regionStats struct {
	max  int
	mean float64
	far  float64 // share of channels differing by more than 32
}

func compareRegion(a *image.RGBA, b image.Image, r image.Rectangle) regionStats {
	var st regionStats
	var sum, n, far float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := a.PixOffset(x, y)
			cr, cg, cb, _ := b.At(x+b.Bounds().Min.X, y+b.Bounds().Min.Y).RGBA()
			for k, v := range [3]uint32{cr >> 8, cg >> 8, cb >> 8} {
				d := int(a.Pix[i+k]) - int(v)
				if d < 0 {
					d = -d
				}
				st.max = max(st.max, d)
				sum += float64(d)
				n++
				if d > 32 {
					far++
				}
			}
		}
	}
	st.mean, st.far = sum/n, far/n
	return st
}
