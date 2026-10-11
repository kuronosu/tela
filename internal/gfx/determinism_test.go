package gfx

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"testing"
)

// flattenDigest hashes the float64 intermediate results of the flattening
// and arc code on random input. Output points are float32, which hides
// most last-bit differences, so the intermediates are what reveals a
// product fused into a multiply-add on some architecture.
func flattenDigest() uint64 {
	h := fnv.New64a()
	put := func(vs ...float64) {
		for _, v := range vs {
			b := math.Float64bits(v)
			var buf [8]byte
			for i := range buf {
				buf[i] = byte(b >> (8 * i))
			}
			h.Write(buf[:])
		}
	}
	rng := rand.New(rand.NewPCG(11, 11))
	f := func() float64 { return float64(float32(rng.Float64()*200 - 100)) }
	for i := 0; i < 2000; i++ {
		c := [4][2]float64{{f(), f()}, {f(), f()}, {f(), f()}, {f(), f()}}
		t := rng.Float64()
		p := evalCubic2(c, t)
		d := cubicDeriv(c, t)
		put(p[0], p[1], d[0], d[1])
		q := cubicSubQuad(c, t/2, t)
		put(q[1][0], q[1][1], q[2][0], q[2][1])
		s := cubicSubsegment(c, t/3, t)
		put(s[1][0], s[2][1])
		infl, n := cubicInflections(c)
		put(infl[0], infl[1], float64(n))
		st, ok := sharpTurn(c, 5)
		put(st, float64(cubicToQuadCount(c, 0.01)))
		if ok {
			put(1)
		}
		params := estimateSubdiv(q, 0.3)
		put(params.a0, params.a2, params.u0, params.uscale, params.val, params.fold)
		put(determineSubdivT(&params, t), approxParabolaIntegral(f()), approxParabolaInvIntegral(f()))
		e := sampleEllipse(f(), f(), f(), f())
		put(e[0], e[1])
		pt := evalQuad2(q, t)
		put(float64(pt.X), float64(pt.Y))
	}
	// And the float32 outputs of whole operations.
	var path Path
	for i := 0; i < 50; i++ {
		path.MoveTo(Pt(float32(f()), float32(f())))
		path.CubicTo(Pt(float32(f()), float32(f())), Pt(float32(f()), float32(f())), Pt(float32(f()), float32(f())))
		path.AddArc(Arc{Center: Pt(float32(f()), float32(f())), Radii: Pt(float32(f()), float32(f())), StartAngle: float32(f()), SweepAngle: float32(f() / 20)}, 0)
		path.AddRRect(NewRRect(RectXYWH(float32(f()), float32(f()), 50, 40), float32(rng.Float64()*20)))
	}
	path.Flatten(0.1, func(_ Verb, p Point) { put(float64(p.X), float64(p.Y)) })
	st := NewStrokeStyle(4)
	st.Join = JoinRound
	st.Dashes = []float32{7, 3}
	for el := range path.Stroke(st, 0.05).Elements() {
		put(float64(el.Pts[0].X), float64(el.Pts[0].Y))
	}
	return h.Sum64()
}

// wantFlattenDigest is flattenDigest() recorded on amd64 (GOAMD64=v1, no
// fused multiply-adds). Every architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static -run Deterministic ./internal/gfx
const wantFlattenDigest uint64 = 0x4c0174baab0dd912

func TestFlattenDeterministic(t *testing.T) {
	if got := flattenDigest(); got != wantFlattenDigest {
		t.Errorf("flattenDigest = %#x, want %#x (the same on every architecture)", got, wantFlattenDigest)
	}
}
