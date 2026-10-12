package fmath

import (
	"hash/fnv"
	"math"
	"testing"
)

func TestErfAccuracy(t *testing.T) {
	check := func(x float64) {
		if d := ulps(Erf(x), math.Erf(x)); d > 2 { // math.Exp is assembly on amd64
			t.Errorf("Erf(%v) = %v, math.Erf = %v (%d ulps)", x, Erf(x), math.Erf(x), d)
		}
		if d := ulps(Erfc(x), math.Erfc(x)); d > 4 {
			t.Errorf("Erfc(%v) = %v, math.Erfc = %v (%d ulps)", x, Erfc(x), math.Erfc(x), d)
		}
	}
	for x := -30.0; x <= 30; x += 0.00731 {
		check(x)
	}
	for e := -320; e <= 2; e++ {
		check(math.Pow(10, float64(e)))
		check(-math.Pow(10, float64(e)))
	}
	for _, x := range []float64{0, 0.25, 0.84375, 1.25, 1 / 0.35, 6, 28, math.Inf(1)} {
		check(x)
		check(-x)
	}
	if !math.IsNaN(Erf(math.NaN())) || !math.IsNaN(Erfc(math.NaN())) {
		t.Error("NaN")
	}
}

// erfDigest hashes the bits of Erf and Erfc on a fixed set of arguments.
func erfDigest() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(v float64) {
		b := math.Float64bits(v)
		for i := range buf {
			buf[i] = byte(b >> (8 * i))
		}
		h.Write(buf[:])
	}
	for x := -30.0; x <= 30; x += 0.00731 {
		put(Erf(x))
		put(Erfc(x))
	}
	return h.Sum64()
}

// wantErfDigest is erfDigest() recorded on amd64 (GOAMD64=v1). Every
// architecture must reproduce it; check with
//
//	GOARCH=arm64 go test -exec qemu-aarch64-static ./internal/fmath
const wantErfDigest uint64 = 0x927fb98d2dbf2db1

func TestErfDeterministic(t *testing.T) {
	if got := erfDigest(); got != wantErfDigest {
		t.Errorf("erfDigest = %#x, want %#x (the same on every architecture)", got, wantErfDigest)
	}
}
