package raster

import (
	"hash/fnv"
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// shadowDigest hashes the float64 values of the analytic shadow evaluation
// (separable, local and mask paths), of the Gaussian tables and kernels, and
// the pixels of the shadow and filter scenes. Pixels are 8-bit, which hides
// most last-bit differences; the float64 values reveal a product fused into
// a multiply-add on some architecture.
func shadowDigest() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(v float64) {
		b := math.Float64bits(v)
		for i := range buf {
			buf[i] = byte(b >> (8 * i))
		}
		h.Write(buf[:])
	}
	for x := -9.0; x <= 9; x += 0.0173 {
		put(normCDF(x))
		put(gaussI(x))
	}
	for _, s := range []float64{0.3, 1.7, 2, 5.5, 40} {
		for _, off := range []float64{0, 0.4, -3.6} {
			k, _ := newKernel(s, off, nil)
			for _, w := range k.w {
				put(float64(w))
			}
			put(float64(k.left))
			put(float64(k.right))
		}
	}
	for _, m := range []gfx.Affine{gfx.Identity, gfx.Scale(1.5, 1.5), gfx.RotateAbout(0.3, gfx.Pt(50, 50)), gfx.Skew(0.3, 0)} {
		for _, tc := range shadowCases {
			zone := image.Rect(0, 0, 120, 110)
			got, _ := evalShadow(tc.c, m, zone)
			for _, row := range got {
				for _, v := range row {
					put(v)
				}
			}
		}
	}
	for _, s := range scenes3[1:] {
		h.Write(renderScene(s, 1.5).Pix)
	}
	return h.Sum64()
}

// wantShadowDigest is shadowDigest() recorded on amd64 (GOAMD64=v1). Every
// architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static -run Deterministic ./internal/gfx/raster
const wantShadowDigest uint64 = 0xcee837a7313202cc

func TestShadowDeterministic(t *testing.T) {
	if got := shadowDigest(); got != wantShadowDigest {
		t.Errorf("shadowDigest = %#x, want %#x (the same on every architecture)", got, wantShadowDigest)
	}
}
