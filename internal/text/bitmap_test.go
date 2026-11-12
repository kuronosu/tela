package text

import (
	"fmt"
	"image"
	"math"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

const (
	familyCBDT = "Bitmap Emoji"
	familySbix = "Tela Sbix Test"
)

func bitmapTypeface(t testing.TB, file, family string) *Typeface {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	tfs, err := ParseTypefaces(data, family)
	if err != nil {
		t.Fatal(err)
	}
	return tfs[0]
}

// bitmapCollection is the default collection with the CBDT subset as the
// emoji family and the sbix test font registered.
func bitmapCollection(t testing.TB) *Collection {
	t.Helper()
	c := NewDefaultCollection()
	for _, f := range []struct{ file, family string }{{cbdtTestFile, familyCBDT}, {sbixTestFile, familySbix}} {
		data, err := os.ReadFile(f.file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Register(data, f.family); err != nil {
			t.Fatal(err)
		}
	}
	c.SetGeneric("emoji", familyCBDT)
	return c
}

// imageOf returns the image of the single DrawImage of a bitmap glyph's
// list, and its destination.
func imageOf(t *testing.T, l *gfx.DisplayList) (*gfx.Image, gfx.Rect) {
	t.Helper()
	if l == nil || l.Len() != 1 {
		t.Fatalf("list %v: want one DrawImage", l)
	}
	c, ok := l.At(0).(gfx.DrawImage)
	if !ok {
		t.Fatalf("command %s, want DrawImage", l.At(0).Name())
	}
	return c.Image, c.Dst
}

func TestCBDTFace(t *testing.T) {
	tf := bitmapTypeface(t, cbdtTestFile, familyCBDT)
	if !tf.IsColor() {
		t.Error("a CBDT font is not a color font")
	}
	f := tf.Face(Instance{})
	m := f.Metrics()
	// hhea of Noto Color Emoji: 1900 and -500 units of 2048.
	if m.Ascent != float32(1900.0/2048) || m.Descent != float32(500.0/2048) {
		t.Errorf("ascent %g descent %g", m.Ascent, m.Descent)
	}
	g, ok := f.GlyphIndex(0x1F600)
	if !ok {
		t.Fatal("no glyph for U+1F600")
	}
	if got, want := f.Advance(g), float32(2550.0/2048); got != want {
		t.Errorf("advance %g, want %g", got, want)
	}
	// Noto's strike: 109 ppem, images 136 by 128 pixels, 101 above the
	// baseline.
	want := gfx.Rect{X0: 0, Y0: float32(-101.0 / 109), X1: float32(136.0 / 109), Y1: float32(27.0 / 109)}
	if b := f.GlyphBounds(g); b != want {
		t.Errorf("bounds %v, want %v", b, want)
	}
	img, dst := imageOf(t, f.ColorGlyph(g, gfx.Black))
	if img.Width() != 136 || img.Height() != 128 || dst != want {
		t.Errorf("ColorGlyph: %dx%d image at %v", img.Width(), img.Height(), dst)
	}
	// 16 px: k = 16/109; the level of 34 by 32 pixels is the smallest that
	// is not smaller than the 20 by 19 pixels drawn.
	img, dst = imageOf(t, f.ColorGlyphAt(g, gfx.Black, 16))
	if img.Width() != 34 || img.Height() != 32 || dst != want {
		t.Errorf("at 16 px: %dx%d image at %v", img.Width(), img.Height(), dst)
	}
	// The text color does not matter, and the decoded image is shared.
	img2, _ := imageOf(t, f.ColorGlyphAt(g, gfx.RGB8(255, 0, 0), 16))
	if img2 != img {
		t.Error("the image is decoded again for another foreground")
	}
	// The space has no bitmap: it is an outline glyph without an outline.
	sp, _ := f.GlyphIndex(' ')
	if f.ColorGlyph(sp, gfx.Black) != nil || !f.GlyphBounds(sp).Empty() {
		t.Error("the space has a bitmap")
	}
	if f.Advance(sp) <= 0 {
		t.Error("the space has no advance")
	}
}

func TestSbixFace(t *testing.T) {
	tf := bitmapTypeface(t, sbixTestFile, familySbix)
	if !tf.IsColor() {
		t.Error("an sbix font is not a color font")
	}
	f := tf.Face(Instance{})
	gid := func(r rune) gfx.GlyphID {
		g, ok := f.GlyphIndex(r)
		if !ok {
			t.Fatalf("no glyph for %U", r)
		}
		return g
	}
	disc, dupe, jpg, big := gid(0x1F600), gid(0x1F601), gid(0x1F602), gid(0x1F603)
	small := gfx.Rect{X0: 0, Y0: -0.8, X1: 1, Y1: 0.2}                               // 20 px strike, offset -4
	large := gfx.Rect{X0: 0, Y0: float32(-52.0 / 64), X1: 1, Y1: float32(12.0 / 64)} // 64 px strike, offset -12
	if b, want := f.GlyphBounds(disc), small.Union(large); b != want {
		t.Errorf("bounds %v, want %v", b, want)
	}
	for _, tc := range []struct {
		g          gfx.GlyphID
		ppem       float32
		size       int
		dst        gfx.Rect
		r, gr, blu uint8 // the color of a pixel of the disc's edge
	}{
		{disc, 12, 20, small, 0xE5, 0x39, 0x35}, // the smallest strike at least as large
		{disc, 20, 20, small, 0xE5, 0x39, 0x35},
		{disc, 21, 32, large, 0x1E, 0x88, 0xE5}, // 64 px strike, half size
		{disc, 64, 64, large, 0x1E, 0x88, 0xE5},
		{disc, 200, 64, large, 0x1E, 0x88, 0xE5}, // else the largest
		{disc, 0, 64, large, 0x1E, 0x88, 0xE5},
		{dupe, 16, 20, small, 0xE5, 0x39, 0x35},
		// Only the 64 px strike has it: drawn from it at every size.
		{big, 10, 12, gfx.Rect{X0: 4.0 / 64, Y0: -0.75, X1: 52.0 / 64, Y1: 0}, 0x2E, 0x7D, 0x32},
	} {
		img, dst := imageOf(t, f.ColorGlyphAt(tc.g, gfx.Black, tc.ppem))
		if img.Width() != tc.size || dst != tc.dst {
			t.Errorf("glyph %d at %g px: %dx%d image at %v, want %d px at %v", tc.g, tc.ppem, img.Width(), img.Height(), dst, tc.size, tc.dst)
			continue
		}
		// A pixel half way between the center and the edge, on the
		// horizontal axis (the center is white).
		o := (img.Height()/2)*img.Stride() + 4*(img.Width()/4)
		p := img.Pix()[o : o+4]
		if absDiff(p[0], tc.r) > 2 || absDiff(p[1], tc.gr) > 2 || absDiff(p[2], tc.blu) > 2 || p[3] != 255 {
			t.Errorf("glyph %d at %g px: pixel %v, want %02x%02x%02x", tc.g, tc.ppem, p, tc.r, tc.gr, tc.blu)
		}
	}
	// A 'dupe' shares the image it refers to.
	a, _ := imageOf(t, f.ColorGlyphAt(disc, gfx.Black, 20))
	b, _ := imageOf(t, f.ColorGlyphAt(dupe, gfx.Black, 20))
	if a != b {
		t.Error("a dupe glyph decodes its own image")
	}
	// JPEG images are not drawn: no color glyph, no bounds.
	if f.ColorGlyph(jpg, gfx.Black) != nil || f.ColorGlyphAt(jpg, gfx.Black, 20) != nil || !f.GlyphBounds(jpg).Empty() {
		t.Error("a jpg glyph is drawn")
	}
}

// TestBitmapEmojiPresentation checks that a bitmap emoji font is a color
// font for font selection (ADR-031).
func TestBitmapEmojiPresentation(t *testing.T) {
	c := bitmapCollection(t)
	sans := Query{Families: []string{"sans-serif"}}
	for _, tc := range []struct{ text, want string }{
		{"a😀b", "a=Inter|😀=" + familyCBDT + "|b=Inter"},
		{"❤", "❤=Inter"},
		{"❤️", "❤️=" + familyCBDT},
		{"❤︎", "❤︎=Inter"},
		{"🔥", "🔥=" + familyCBDT},
	} {
		if got := runsOf(c, tc.text, sans); got != tc.want {
			t.Errorf("Itemize(%q):\n got %s\nwant %s", tc.text, got, tc.want)
		}
	}
	// VS15 avoids color fonts even when they come first.
	q := Query{Families: []string{familyCBDT, "Inter"}}
	if got, want := runsOf(c, "❤︎", q), "❤︎=Inter"; got != want {
		t.Errorf("Itemize(❤ VS15) = %s, want %s", got, want)
	}
	if got, want := runsOf(c, "❤", q), "❤="+familyCBDT; got != want {
		t.Errorf("Itemize(❤) = %s, want %s", got, want)
	}
}

// TestBitmapCacheBound checks that the decoded images stay within the
// cache's budget and that eviction does not change the images drawn.
func TestBitmapCacheBound(t *testing.T) {
	tf := bitmapTypeface(t, cbdtTestFile, familyCBDT)
	f := tf.Face(Instance{})
	saved := bitmapImages
	defer func() { bitmapImages = saved }()
	// Room for two images of 136 by 128 pixels and their mip levels.
	bitmapImages = newBitmapCache(2 * (136*128*4*4/3 + 256))
	var first []byte
	for round := range 2 {
		for i, r := range cbdtTestRunes {
			g, _ := f.GlyphIndex(r)
			img, _ := imageOf(t, f.ColorGlyphAt(g, gfx.Black, 0))
			if n, bytes := bitmapImages.len(); n > 2 || bytes > bitmapImages.limit {
				t.Fatalf("%d images, %d bytes: over the budget of %d", n, bytes, bitmapImages.limit)
			}
			if i == 0 {
				if round == 0 {
					first = append([]byte(nil), img.Pix()...)
				} else if string(img.Pix()) != string(first) {
					t.Error("an image decoded again differs")
				}
			}
		}
	}
}

// The decoded-image cache does not keep a dropped collection's typefaces
// alive, and their images leave the cache when they are collected.
func TestBitmapCacheReleasesTypefaces(t *testing.T) {
	saved := bitmapImages
	defer func() { bitmapImages = saved }()
	bitmapImages = newBitmapCache(bitmapCacheBytes)

	ref := func() weak.Pointer[Typeface] {
		data, err := os.ReadFile(cbdtTestFile)
		if err != nil {
			t.Fatal(err)
		}
		c := NewCollection()
		if _, err := c.Register(data, familyCBDT); err != nil {
			t.Fatal(err)
		}
		f := c.Face(Query{Families: []string{familyCBDT}}, 24)
		for _, r := range cbdtTestRunes {
			g, _ := f.GlyphIndex(r)
			imageOf(t, f.ColorGlyphAt(g, gfx.Black, 24))
		}
		return weak.Make(f.Typeface())
	}()
	if n, _ := bitmapImages.len(); n == 0 {
		t.Fatal("no image cached")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		n, bytes := bitmapImages.len()
		if ref.Value() == nil && n == 0 && bytes == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after the collection is dropped: typeface alive %v, %d images (%d bytes) cached",
				ref.Value() != nil, n, bytes)
		}
		time.Sleep(10 * time.Millisecond)
	}
	bitmapImages.mu.Lock()
	defer bitmapImages.mu.Unlock()
	if len(bitmapImages.watched) != 0 {
		t.Errorf("%d typefaces still watched", len(bitmapImages.watched))
	}
}

// bitmapScene draws emoji from the CBDT subset in paragraphs of Inter at
// several sizes, the sbix test font's strikes, translucent and rotated
// bitmap emoji, and the COLR emoji of the same characters for comparison.
func bitmapScene(t testing.TB) (*gfx.DisplayList, int, int) {
	c := bitmapCollection(t)
	b := gfx.NewBuilder()
	const W = 520
	ink := gfx.RGB8(0x11, 0x18, 0x27)
	y := float32(12)
	para := func(size float32, text string, paint gfx.Paint, fams ...string) {
		p := &Paragraph{Text: text, Default: TextStyle{Families: fams, Size: size, Paint: paint}}
		l := NewLayout(c, p)
		l.BreakLines(W)
		// The baseline guide shows that emoji sit on the text's baseline.
		bl := y + l.lines[0].baseline
		b.FillRect(gfx.Rect{X0: 12, Y0: bl, X1: 12 + l.Width(), Y1: bl + 0.5}, gfx.RGBA8(0xdc, 0x26, 0x26, 0x80))
		l.Paint(b, gfx.Pt(12, y))
		y += l.Height() + 6
	}
	const text = "Emoji 😀😍🔥🎉❤️👍🌍🤔 end"
	for _, size := range []float32{12, 16, 20, 24, 32, 48} {
		para(size, text, ink, "Inter", "emoji")
	}
	para(24, text, gfx.RGBA8(0x11, 0x18, 0x27, 0x80), "Inter", "emoji")
	para(24, text, ink, "Inter", FamilyNotoColorEmoji)

	// The sbix strikes: 20 px (red) up to 20 device pixels, 64 px (blue)
	// above; the green square only exists at 64 px.
	sb := c.Face(Query{Families: []string{familySbix}}, 16)
	x := float32(12)
	y += 52
	for _, size := range []float32{10, 16, 20, 24, 48} {
		var gs []gfx.Glyph
		for i, r := range []rune{0x1F600, 0x1F601, 0x1F603} {
			g, _ := sb.GlyphIndex(r)
			gs = append(gs, gfx.Glyph{ID: g, X: float32(i) * size * 1.1})
		}
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: sb, Size: size, Origin: gfx.Pt(x, y), Glyphs: gs, Paint: ink})
		x += size*3.3 + 12
	}
	y += 16

	// Rotated: drawn directly, at the em's device size.
	em := c.Face(Query{Families: []string{"emoji"}}, 40)
	g, _ := em.GlyphIndex(0x1F389)
	for i, a := range []float32{-0.4, 0, 0.4} {
		b.PushTransform(gfx.Translate(float32(40+i*70), y+40).Mul(gfx.Rotate(a)))
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: em, Size: 40, Origin: gfx.Pt(-25, 15), Glyphs: []gfx.Glyph{{ID: g}}, Paint: ink})
		b.PopTransform()
	}
	y += 90
	return b.Finish(), W + 24, int(math.Ceil(float64(y)))
}

func TestGoldenBitmapEmoji(t *testing.T) {
	dl, w, h := bitmapScene(t)
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, int(float32(w)*scale), int(float32(h)*scale)))
			raster.Clear(img, image.Rectangle{}, gfx.White)
			raster.Render(img, dl, raster.Options{Scale: scale})
			golden.Image(t, fmt.Sprintf("bitmap_emoji_x%g", scale), img)
		})
	}
}

// TestConcurrentBitmapGlyphs loads the bitmaps of a cold typeface from
// many goroutines, as raster workers do, and checks that they agree.
func TestConcurrentBitmapGlyphs(t *testing.T) {
	for _, file := range []string{cbdtTestFile, sbixTestFile} {
		tf := bitmapTypeface(t, file, "")
		f := tf.Face(Instance{})
		n := tf.NumGlyphs()
		type result struct {
			bounds gfx.Rect
			pix    string
		}
		var wg sync.WaitGroup
		results := make([][]result, 8)
		for w := range results {
			wg.Go(func() {
				for i := range 3 * n {
					g := gfx.GlyphID((i + w) % n)
					r := result{bounds: f.GlyphBounds(g)}
					if l := f.ColorGlyphAt(g, gfx.Black, float32(8+(i%5)*12)); l != nil && l.Len() == 1 {
						r.pix = string(l.At(0).(gfx.DrawImage).Image.Pix())
					}
					if i < n {
						results[w] = append(results[w], r)
					}
				}
			})
		}
		wg.Wait()
		for w := 1; w < len(results); w++ {
			for i, r := range results[w] {
				g := (i + w) % n
				ref := results[0][g]
				if r.bounds != ref.bounds || (i%5 == g%5 && r.pix != ref.pix) {
					t.Errorf("%s glyph %d: goroutines disagree", file, g)
				}
			}
		}
	}
}
