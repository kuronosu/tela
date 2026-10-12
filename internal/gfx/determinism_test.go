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

// trigDigest hashes the transforms and filters built from angles and
// lengths: Rotate, Skew, HueRotate and Point.Len.
func trigDigest() uint64 {
	h := fnv.New64a()
	put := func(vs ...float32) {
		for _, v := range vs {
			b := math.Float32bits(v)
			h.Write([]byte{byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24)})
		}
	}
	rng := rand.New(rand.NewPCG(12, 12))
	for i := 0; i < 20000; i++ {
		a := float32(rng.Float64()*20 - 10)
		r, k, m := Rotate(a), Skew(a/8, -a/9), HueRotate(a)
		put(r[:]...)
		put(k[:]...)
		put(m.M[:]...)
		put(Pt(a*37, float32(rng.Float64()*100)).Len())
	}
	return h.Sum64()
}

// wantTrigDigest is trigDigest() recorded on amd64 (GOAMD64=v1).
const wantTrigDigest uint64 = 0x226dafa4498089a0

func TestTrigDeterministic(t *testing.T) {
	if got := trigDigest(); got != wantTrigDigest {
		t.Errorf("trigDigest = %#x, want %#x (the same on every architecture)", got, wantTrigDigest)
	}
}

// colorDigest hashes the color conversions on random input, extended sRGB
// included: OKLCH, Oklab, ToOklab, ToOKLCH, the transfer functions and the
// Oklab matrices, as float32 results and as the float64 intermediates that
// reveal a fused multiply-add.
func colorDigest() uint64 {
	h := fnv.New64a()
	put32 := func(vs ...float32) {
		for _, v := range vs {
			b := math.Float32bits(v)
			h.Write([]byte{byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24)})
		}
	}
	put64 := func(vs ...float64) {
		for _, v := range vs {
			b := math.Float64bits(v)
			var buf [8]byte
			for i := range buf {
				buf[i] = byte(b >> (8 * i))
			}
			h.Write(buf[:])
		}
	}
	for _, m := range [][3][3]float64{linToLMS, lmsToLin} {
		for _, row := range m {
			put64(row[:]...)
		}
	}
	rng := rand.New(rand.NewPCG(13, 13))
	f := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	for i := 0; i < 20000; i++ {
		l, c, hue := f(-0.05, 1.05), f(-0.02, 0.45), f(-30, 400)
		k := OKLCH(l, c, hue, 1)
		put32(k.R, k.G, k.B)
		k = Oklab(l, f(-0.4, 0.4), f(-0.4, 0.4), 1)
		put32(k.R, k.G, k.B)
		x := Color{f(-0.3, 1.3), f(-0.3, 1.3), f(-0.3, 1.3), 1}
		a, b, d := x.ToOklab()
		put32(a, b, d)
		a, b, d = x.ToOKLCH()
		put32(a, b, d)
		v := f(-1.5, 1.5)
		put32(SRGBToLinear(v), LinearToSRGB(v))
		a, b, d = LinearSRGBToOklab(x.R, x.G, x.B)
		put32(a, b, d)
		a, b, d = OklabToLinearSRGB(l, f(-0.4, 0.4), f(-0.4, 0.4))
		put32(a, b, d)
		w := float64(v)
		put64(srgbToLinear(w), linearToSRGB(w))
		ll, la, lb := linToOklab(srgbToLinear(float64(x.R)), srgbToLinear(float64(x.G)), srgbToLinear(float64(x.B)))
		put64(ll, la, lb)
		put64(oklabToLin(ll, la, lb))
		put64(labToLCH(la, lb))
	}
	return h.Sum64()
}

// wantColorDigest is colorDigest() recorded on amd64 (GOAMD64=v1).
const wantColorDigest uint64 = 0x4fb3b15979b7f0a9

func TestColorDeterministic(t *testing.T) {
	if got := colorDigest(); got != wantColorDigest {
		t.Errorf("colorDigest = %#x, want %#x (the same on every architecture)", got, wantColorDigest)
	}
}
