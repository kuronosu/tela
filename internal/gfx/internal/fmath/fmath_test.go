package fmath

import (
	"hash/fnv"
	"math"
	"testing"
)

// ulps returns the distance between a and b in units in the last place.
func ulps(a, b float64) uint64 {
	ia, ib := int64(math.Float64bits(a)), int64(math.Float64bits(b))
	if ia < 0 {
		ia = math.MinInt64 - ia
	}
	if ib < 0 {
		ib = math.MinInt64 - ib
	}
	if ia > ib {
		return uint64(ia - ib)
	}
	return uint64(ib - ia)
}

// inputs returns a spread of positive arguments: dense in [0, 4], plus
// powers of ten.
func inputs() []float64 {
	var xs []float64
	for i := 1; i <= 4000; i++ {
		xs = append(xs, float64(i)*0.001)
	}
	for e := -30; e <= 30; e++ {
		xs = append(xs, math.Pow10(e), 3*math.Pow10(e))
	}
	return xs
}

func TestAccuracy(t *testing.T) {
	for _, x := range inputs() {
		if d := ulps(Log(x), math.Log(x)); d > 1 {
			t.Errorf("Log(%v) = %v, math.Log = %v (%d ulps)", x, Log(x), math.Log(x), d)
		}
		if d := ulps(Cbrt(x), math.Cbrt(x)); d > 1 {
			t.Errorf("Cbrt(%v) = %v, math.Cbrt = %v (%d ulps)", x, Cbrt(x), math.Cbrt(x), d)
		}
		if d := ulps(Cbrt(-x), math.Cbrt(-x)); d > 1 {
			t.Errorf("Cbrt(%v) = %v, math.Cbrt = %v (%d ulps)", -x, Cbrt(-x), math.Cbrt(-x), d)
		}
		for _, y := range []float64{2.4, 1 / 2.4, -0.5, 3} {
			got, want := Pow(x, y), math.Pow(x, y)
			if math.IsInf(want, 0) || want == 0 {
				continue
			}
			if math.Abs(got-want) > 1e-14*math.Abs(want) && x <= 4 && x >= 1e-6 {
				t.Errorf("Pow(%v, %v) = %v, math.Pow = %v", x, y, got, want)
			}
		}
	}
	for x := -700.0; x <= 700; x += 0.37 {
		if d := ulps(Exp(x), math.Exp(x)); d > 2 { // math.Exp is assembly on amd64
			t.Errorf("Exp(%v) = %v, math.Exp = %v (%d ulps)", x, Exp(x), math.Exp(x), d)
		}
	}
	for i := 0; i < 720; i++ {
		a := float64(i) * math.Pi / 360
		for _, r := range []float64{1e-3, 1, 1e6} {
			y, x := r*math.Sin(a), r*math.Cos(a)
			if d := ulps(Atan2(y, x), math.Atan2(y, x)); d > 2 && math.Abs(Atan2(y, x)-math.Atan2(y, x)) > 1e-15 {
				t.Errorf("Atan2(%v, %v) = %v, math.Atan2 = %v", y, x, Atan2(y, x), math.Atan2(y, x))
			}
		}
	}
	for _, c := range [][3]float64{{0, 1, 0}, {0, -1, math.Pi}, {1, 0, math.Pi / 2}, {-1, 0, -math.Pi / 2}, {0, 0, 0}} {
		if got := Atan2(c[0], c[1]); got != c[2] {
			t.Errorf("Atan2(%v, %v) = %v, want %v", c[0], c[1], got, c[2])
		}
	}
	if Log(0) != math.Inf(-1) || !math.IsNaN(Log(-1)) || Exp(1000) != math.Inf(1) || Exp(-1000) != 0 {
		t.Error("special cases")
	}
}

// digest hashes the bits of every function on a fixed set of arguments.
func digest() uint64 {
	h := fnv.New64a()
	put := func(v float64) {
		b := math.Float64bits(v)
		var buf [8]byte
		for i := range buf {
			buf[i] = byte(b >> (8 * i))
		}
		h.Write(buf[:])
	}
	for _, x := range inputs() {
		put(Log(x))
		put(Cbrt(x))
		put(Pow(x, 2.4))
		put(Pow(x, 1/2.4))
		put(Atan2(x, 1-x))
		put(Asin01(x / 4))
		s, c := Sincos(x * 3)
		put(s)
		put(c)
	}
	for x := -700.0; x <= 700; x += 0.37 {
		put(Exp(x))
	}
	return h.Sum64()
}

// wantDigest is digest() recorded on amd64 (GOAMD64=v1, no fused
// multiply-adds). Every architecture must reproduce it: a mismatch means a
// product is being fused somewhere. Check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static ./internal/gfx/internal/fmath
const wantDigest uint64 = 0x133470d881c168b6

// On amd64 without FMA, math.Sincos is the same pure-Go code, so the port
// agrees bit for bit there; elsewhere within an ulp.
func TestSincos(t *testing.T) {
	for x := -20.0; x <= 20; x += 0.0137 {
		s, c := Sincos(x)
		ws, wc := math.Sincos(x)
		if ulps(s, ws) > 1 || ulps(c, wc) > 1 {
			t.Errorf("Sincos(%v) = %v, %v; math.Sincos = %v, %v", x, s, c, ws, wc)
		}
	}
	if s, c := Sincos(1e12); math.Abs(s*s+c*c-1) > 1e-12 {
		t.Errorf("Sincos(1e12) = %v, %v: not on the unit circle", s, c)
	}
}

func TestDeterministic(t *testing.T) {
	if got := digest(); got != wantDigest {
		t.Errorf("digest = %#x, want %#x (the same on every architecture)", got, wantDigest)
	}
}
