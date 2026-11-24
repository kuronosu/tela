package gfx

import (
	"math"
	"testing"
)

// The premultiplied ramp between the stops is close to the one between
// colors that are not premultiplied.
func TestUnpremultipliedStops(t *testing.T) {
	a, b := Color{1, 0, 0.2, 1}, Color{0, 1, 0.2, 0}
	out := UnpremultipliedStops([]GradientStop{{0.25, a}, {0.75, b}, {0.75, a}, {1, Color{0, 0, 1, 1}}})
	if n := len(out); n != 12+3 || out[0] != (GradientStop{0.25, a}) || out[n-1].Offset != 1 {
		t.Fatalf("%d stops: %v", n, out)
	}
	worst := 0.0
	for i := 0; out[i+1].Offset <= 0.75 && out[i+1].Offset > out[i].Offset; i++ {
		for k := range 9 {
			u := float64(k) / 8
			o := float64(out[i].Offset) + float64(out[i+1].Offset-out[i].Offset)*u
			v := (o - 0.25) / 0.5
			r0, _, _, _ := out[i].Color.Premultiplied()
			r1, _, _, _ := out[i+1].Color.Premultiplied()
			worst = max(worst, math.Abs(float64(r0)+float64(r1-r0)*u-(1-v)*(1-v)))
		}
	}
	if worst > 0.51/255 {
		t.Errorf("%g levels away", worst*255)
	}
	if same := UnpremultipliedStops([]GradientStop{{0, Color{1, 0, 0, 1}}, {1, Color{1, 0, 0, 0}}}); len(same) != 2 {
		t.Errorf("one color, two alphas: %v", same)
	}
	if none := UnpremultipliedStops(nil); len(none) != 0 {
		t.Errorf("%v", none)
	}
}
