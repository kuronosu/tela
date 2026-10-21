package svg

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// svgDigest hashes the display lists of every gallery cell, drawn into a
// box that does not match its size (viewBox mapping, gradient and clip
// conversions), and random arcs, ellipses and rounded rectangles. The
// lists print every float32 exactly, so a product fused into a
// multiply-add on some architecture changes the digest (ADR-027, ADR-041).
func svgDigest(t testing.TB) uint64 {
	h := fnv.New64a()
	for _, c := range galleryCells(t, mainGallery) {
		d, err := Parse(c.source(t))
		if err != nil {
			t.Fatal(err)
		}
		b := gfx.NewBuilder()
		d.Draw(b, gfx.RectXYWH(3.3, 7.1, 97.7, 61.9), c.opts)
		h.Write([]byte(b.Finish().String()))
	}
	rng := rand.New(rand.NewPCG(95, 95))
	f := func() float64 { return float64(float32(rng.Float64()*200 - 100)) }
	for i := 0; i < 500; i++ {
		var p gfx.Path
		p.MoveTo(gfx.Pt(float32(f()), float32(f())))
		appendArc(&p, gfx.Pt(float32(f()), float32(f())), f(), f(), f()*3, i%2 == 0, i%3 == 0, gfx.Pt(float32(f()), float32(f())))
		q := ellipsePath(float32(f()), float32(f()), float32(f()/2+51), float32(f()/3+40))
		r := rectPath(float32(f()), float32(f()), 80, 60, float32(f()/10+11), float32(f()/20+6))
		fmt.Fprint(h, pathString(p), pathString(q), pathString(r))
		m, _ := parseTransform(fmt.Sprintf("rotate(%g %g %g) skewX(%g) scale(%g)", f(), f(), f(), f()/3, f()/50))
		fmt.Fprint(h, m)
	}
	return h.Sum64()
}

// wantSVGDigest is svgDigest() recorded on amd64 (GOAMD64=v1). Every
// architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static -run Deterministic ./internal/svg
const wantSVGDigest uint64 = 0x81a9762d927c47fe

func TestSVGDeterministic(t *testing.T) {
	if got := svgDigest(t); got != wantSVGDigest {
		t.Errorf("svgDigest = %#x, want %#x (the same on every architecture)", got, wantSVGDigest)
	}
}
