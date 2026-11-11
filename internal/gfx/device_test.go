package gfx

import "testing"

func TestRoundDevice(t *testing.T) {
	// 1 + 23/1.5 logical px at x1.5 is 24.5 device px exactly, but the
	// float32 sum is a hair below it.
	hair := float64(float32(1)+float32(23.0/1.5)) * 1.5
	if !(hair < 24.5) {
		t.Fatalf("the case lost its float32 hair: %v", hair)
	}
	for _, c := range []struct{ v, want float64 }{
		{0, 0}, {0.49, 0}, {0.5, 1}, {1.498, 1}, {-0.5, 0}, {-0.51, -1},
		{24.498, 24}, {hair, 25}, {24.5, 25}, {24.5 - 1.0/1024, 25}, {24.5 - 2.0/1024, 24},
		{300000.4, 300001}, // 1/8 px tolerance far away
	} {
		if got := RoundDevice(c.v); got != c.want {
			t.Errorf("RoundDevice(%v) = %v, want %v", c.v, got, c.want)
		}
	}
}
