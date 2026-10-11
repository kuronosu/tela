package fmath

import (
	"math"
	"testing"
)

// asinInputs exercise every branch of satan.
var asinInputs = []float64{0, 1e-9, 0.1, 0.3, 0.5, 0.55, 0.7071067811865476, 0.8, 0.9, 0.92, 0.99, 0.999999, 1}

// asinBits are the results of Asin01 on asinInputs, recorded on amd64 (no
// fused multiply-add). Every architecture must reproduce them bit for bit;
// a mismatch means a product is being fused somewhere.
var asinBits = []uint64{
	0x0000000000000000, 0x3e112e0be826d695, 0x3fb9a49276037884, 0x3fd380159e14f6ff,
	0x3fe0c152382d7366, 0x3fe2a2ba538032f0, 0x3fe921fb54442d19, 0x3fedac670561bb50,
	0x3ff1ea93705fa172, 0x3ff2b07529b1748a, 0x3ff6de3c6f33d51d, 0x3ff91c306b2c13ac,
	0x3ff921fb54442d18,
}

func TestAsinAccuracy(t *testing.T) {
	for _, x := range asinInputs {
		// math.Asin computes 1-x*x with cancellation and loses digits near
		// 1 (Asin01(0.999999) matches C's libm, math.Asin is 7.8e-15 off).
		got, want := Asin01(x), math.Asin(x)
		if math.Abs(got-want) > 1e-14 {
			t.Errorf("Asin01(%v) = %v, math.Asin = %v", x, got, want)
		}
	}
	for x := 0.0; x <= 50; x += 0.01 {
		if got, want := satan(x), math.Atan(x); math.Abs(got-want) > 4e-16 {
			t.Fatalf("satan(%v) = %v, math.Atan = %v", x, got, want)
		}
	}
}

func TestAsinDeterministic(t *testing.T) {
	if len(asinBits) != len(asinInputs) {
		var s string
		for _, x := range asinInputs {
			s += "\n\t0x" + hex64(math.Float64bits(Asin01(x))) + ","
		}
		t.Fatalf("asinBits not recorded; values on this machine:%s", s)
	}
	for i, x := range asinInputs {
		if got := math.Float64bits(Asin01(x)); got != asinBits[i] {
			t.Errorf("Asin01(%v) = %#x, want %#x (same on every architecture)", x, got, asinBits[i])
		}
	}
}

func hex64(v uint64) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		b[i] = digits[v&15]
		v >>= 4
	}
	return string(b)
}
