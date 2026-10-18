package raster

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// compositingDigest hashes the float64 intermediates of the blend modes
// computed in floating point and of the focal gradient's parameter on
// random input, and the pixels of the blend, mask and focal scenes. The
// intermediates reveal a product fused into a multiply-add on some
// architecture before 8-bit rounding hides it.
func compositingDigest() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(vs ...float64) {
		for _, v := range vs {
			b := math.Float64bits(v)
			for i := range buf {
				buf[i] = byte(b >> (8 * i))
			}
			h.Write(buf[:])
		}
	}
	rng := rand.New(rand.NewPCG(74, 6))
	for i := 0; i < 3000; i++ {
		var cs, cb [3]float64
		for k := range 3 {
			cs[k], cb[k] = rng.Float64(), rng.Float64()
		}
		put(lum(cs), sat(cb))
		hue := setLum(setSat(cs, sat(cb)), lum(cb))
		lm := setLum(cb, lum(cs))
		put(hue[0], hue[1], hue[2], lm[0], lm[1], lm[2])
		s, d := randPremul(rng), randPremul(rng)
		if s[3] != 0 && d[3] != 0 {
			for _, m := range []gfx.BlendMode{gfx.ColorDodge, gfx.ColorBurn, gfx.SoftLight, gfx.Hue, gfx.Luminosity} {
				var o [4]uint8
				blendFloat(m, &s, &d, &o)
				h.Write(o[:3])
			}
		}
	}
	r := NewRenderer()
	for _, c := range append(focalCells(), focalCell{fx: 3.7, fy: -1.3, fr: 0.5, cx: 0.25, cy: 0.5, r: 9.75, stops: []gfx.Color{rose, sky}}) {
		m := gfx.RotateAbout(0.3, gfx.Pt(20, 20)).Mul(gfx.Scale(1.25, 0.8))
		r.ctm = m
		inv, _ := invert(m)
		g := r.focalShader(c.paint(), inv)
		gs := g.(*gradShader)
		put(gs.pa, gs.pb, gs.pc, gs.qa, gs.qb, gs.qc, gs.a, gs.r0dr)
		for k := 0; k < 200; k++ {
			t, ok := gs.focalT(float64(rng.Float64()*300)-150, float64(rng.Float64()*300)-150) // rounded: no fusion
			if !ok {
				t = -1e300
			}
			put(t)
		}
	}
	for _, s := range scenes4 {
		for _, scale := range []float32{1, 1.5} {
			h.Write(renderScene(s, scale).Pix)
		}
	}
	return h.Sum64()
}

// wantCompositingDigest is compositingDigest() recorded on amd64
// (GOAMD64=v1). Every architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static -run Deterministic ./internal/gfx/raster
const wantCompositingDigest uint64 = 0x438d1932b8b56c86

func TestCompositingDeterministic(t *testing.T) {
	if got := compositingDigest(); got != wantCompositingDigest {
		t.Errorf("compositingDigest = %#x, want %#x (the same on every architecture)", got, wantCompositingDigest)
	}
}
