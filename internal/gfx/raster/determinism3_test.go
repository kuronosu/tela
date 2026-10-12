package raster

import (
	"hash/fnv"
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// textDigest hashes glyph masks and sprites rendered at many sizes and
// subpixel offsets, and the pixels of the text scene through the cache and
// through the path fallback. Masks are coverage on the 1/256 grid, so a
// product fused into a multiply-add (in the pen position, the em size or the
// flattening) shows up as a different mask.
func textDigest() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(v int) {
		for i := range buf {
			buf[i] = byte(uint64(v) >> (8 * i))
		}
		h.Write(buf[:])
	}
	r := NewRenderer()
	r.glyphs = NewGlyphCache(1 << 20)
	f := testFontA
	for _, size := range []float32{6.5, 9, 13.37, 14, 17.25, 31, 77.7} {
		for _, id := range []gfx.GlyphID{gO, gA, gS, gL, gI} {
			p, _ := f.GlyphOutline(id)
			for sub := 0; sub < glyphSubpixels; sub++ {
				rect, data := r.glyphMask(p, size, float32(size*1.25), float64(sub)/glyphSubpixels)
				put(rect.Min.X)
				put(rect.Min.Y)
				put(rect.Max.X)
				put(rect.Max.Y)
				h.Write(data)
			}
		}
		for sub := 0; sub < glyphSubpixels; sub++ {
			rect, pix, _ := r.glyphSprite(badgeList(f, rose), size, size, float64(sub)/glyphSubpixels)
			put(rect.Min.X)
			put(rect.Min.Y)
			h.Write(pix)
		}
	}
	// Pen positions: the snapping of apply's results.
	for i := 0; i < 2000; i++ {
		x := float32(float32(i)*0.3711) - 100 // rounded product: no fusion
		m := gfx.Affine{1.25, 0, 0, 1.5, 0.1, -0.37}
		X, Y := apply(&m, gfx.Pt(x, x*0.7))
		put(int(math.Floor(X*glyphSubpixels + 0.5)))
		put(int(math.Floor(Y + 0.5)))
	}
	s := textScenes[0]
	for _, scale := range []float32{1.5, 2} {
		h.Write(renderScene(s, scale).Pix)
		w, ht := int(float32(s.w)*scale), int(float32(s.h)*scale)
		pr := NewRenderer()
		pr.tune.glyphPaths = true
		img := image.NewRGBA(image.Rect(0, 0, w, ht))
		pr.Render(img, build(s.draw), Options{Scale: scale, Workers: 1})
		h.Write(img.Pix)
	}
	return h.Sum64()
}

// wantTextDigest is textDigest() recorded on amd64 (GOAMD64=v1). Every
// architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static -run Deterministic ./internal/gfx/raster
const wantTextDigest uint64 = 0x7c1c30e5e92a6d6d

func TestTextDeterministic(t *testing.T) {
	if got := textDigest(); got != wantTextDigest {
		t.Errorf("textDigest = %#x, want %#x (the same on every architecture)", got, wantTextDigest)
	}
}
