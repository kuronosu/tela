package gfx

import (
	"errors"
	"math"
	"testing"
)

func TestParseColorFuncAccepts(t *testing.T) {
	tests := []struct {
		fn, in string
		want   Color
	}{
		{"rgb", "255 0 0", RGBA8(255, 0, 0, 255)},
		{"rgb", "255, 0, 0", RGBA8(255, 0, 0, 255)},
		{"rgb", "  1 , 2 , 3  ", RGBA8(1, 2, 3, 255)},
		{"rgb", "0,0,255,.5", Color{B: 1, A: 0.5}},
		{"rgb", "0 128 255 / 50%", Color{G: 128.0 / 255, B: 1, A: 0.5}},
		{"rgb", "0 0 0/0.6", Color{A: 0.6}},
		{"rgb", "100%, 50%, 0%", Color{R: 1, G: 0.5, A: 1}},
		{"rgb", "300, -1, 0", RGBA8(255, 0, 0, 255)},
		{"rgb", "none 0 0", RGBA8(0, 0, 0, 255)},
		{"rgb", "10% 20 30%", Color{R: 0.1, G: 20.0 / 255, B: 0.3, A: 1}},
		{"rgb", "1 2 3 / none", Color{R: 1.0 / 255, G: 2.0 / 255, B: 3.0 / 255}},
		{"rgb", "1e1 2 3", Color{R: 10.0 / 255, G: 2.0 / 255, B: 3.0 / 255, A: 1}},
		{"rgb", "1 2 3 / 2", Color{R: 1.0 / 255, G: 2.0 / 255, B: 3.0 / 255, A: 1}},
		{"hsl", "120, 100%, 25%", RGBA8(0, 127, 0, 255)},
		{"hsl", "120 100% 25%", RGBA8(0, 127, 0, 255)},
		{"hsl", "0, 100%, 50%, 0.3", Color{R: 1, A: 0.3}},
		{"hsl", "0.5turn 100% 50%", Color{G: 1, B: 1, A: 1}},
		{"hsl", "240deg, 100%, 50%", Color{B: 1, A: 1}},
		{"hsl", "300 100 50", Color{R: 1, B: 1, A: 1}},
		{"hsl", "none 100% 50%", Color{R: 1, A: 1}},
		{"hsl", "-120 100% 50%", Color{B: 1, A: 1}},
		{"hsl", "120 100% 50% / 25%", Color{G: 1, A: 0.25}},
	}
	for _, tt := range tests {
		var got Color
		var err error
		if tt.fn == "rgb" {
			got, err = ParseRGBFunc(tt.in)
		} else {
			got, err = ParseHSLFunc(tt.in)
		}
		if err != nil {
			t.Errorf("%s(%s): %v", tt.fn, tt.in, err)
			continue
		}
		if !colorNear(got, tt.want.R, tt.want.G, tt.want.B, tt.want.A, 1.0/255) {
			t.Errorf("%s(%s) = %v, want %v", tt.fn, tt.in, got, tt.want)
		}
	}
}

func TestParseColorFuncRejects(t *testing.T) {
	tests := []struct {
		fn, in string
		want   ColorFuncProblem
	}{
		{"rgb", "1,2", ColorFuncArity},
		{"rgb", "1 2", ColorFuncArity},
		{"rgb", "1 2 3 4", ColorFuncArity},
		{"rgb", "1,2,3,4,5", ColorFuncArity},
		{"rgb", "", ColorFuncArity},
		{"rgb", "1, 2 3", ColorFuncSyntax},
		{"rgb", "1,2,3 / 0.5", ColorFuncSyntax},
		{"rgb", "1 2 3, 0.5", ColorFuncSyntax},
		{"rgb", "1,,3", ColorFuncSyntax},
		{"rgb", "1,2,3,", ColorFuncSyntax},
		{"rgb", "1 2 3 / ", ColorFuncSlash},
		{"rgb", "1 2 3 / 4 5", ColorFuncSlash},
		{"hsl", "120 100% 50% /", ColorFuncSlash},
		{"rgb", "a b c", ColorFuncComponent},
		{"rgb", "none, 0, 0", ColorFuncComponent},
		{"rgb", "1 2 inf", ColorFuncComponent},
		{"rgb", "1 2 0x10", ColorFuncComponent},
		{"rgb", "1e 2 3", ColorFuncComponent},
		{"rgb", "1 2 3 / x", ColorFuncAlpha},
		{"rgb", "1, 2, 3, none", ColorFuncAlpha},
		{"rgb", "1, 2%, 3", ColorFuncPercent},
		{"hsl", "1", ColorFuncArity},
		{"hsl", "120,100,25", ColorFuncPercent},
		{"hsl", "120, 100%, 25", ColorFuncPercent},
		{"hsl", "red 100% 50%", ColorFuncHue},
		{"hsl", "none, 100%, 50%", ColorFuncHue},
		{"hsl", "120 x 50%", ColorFuncComponent},
		{"hsl", "120 100% 50% / y", ColorFuncAlpha},
	}
	for _, tt := range tests {
		var err error
		if tt.fn == "rgb" {
			_, err = ParseRGBFunc(tt.in)
		} else {
			_, err = ParseHSLFunc(tt.in)
		}
		var ce *ColorFuncError
		if !errors.As(err, &ce) {
			t.Errorf("%s(%s): err = %v, want a ColorFuncError", tt.fn, tt.in, err)
			continue
		}
		if ce.Problem != tt.want || ce.Func != tt.fn {
			t.Errorf("%s(%s): %+v, want problem %d", tt.fn, tt.in, ce, tt.want)
		}
		if ce.Error() == "" {
			t.Errorf("%s(%s): empty message", tt.fn, tt.in)
		}
	}
}

// refHSLToRGB is the float32 formulation of the conversion that style used
// before ADR-193: hue in degrees, saturation and lightness in [0, 1].
func refHSLToRGB(h, s, l float32) (r, g, b float32) {
	h = float32(math.Mod(float64(h), 360))
	if h < 0 {
		h += 360
	}
	c := float32((1 - float32(math.Abs(float64(2*l-1)))) * s)
	hp := h / 60
	x := float32(c * (1 - float32(math.Abs(math.Mod(float64(hp), 2)-1))))
	var r1, g1, b1 float32
	switch {
	case hp < 1:
		r1, g1, b1 = c, x, 0
	case hp < 2:
		r1, g1, b1 = x, c, 0
	case hp < 3:
		r1, g1, b1 = 0, c, x
	case hp < 4:
		r1, g1, b1 = 0, x, c
	case hp < 5:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}
	m := l - c/2
	return r1 + m, g1 + m, b1 + m
}

// TestHSLMatchesTheOldStyleConversion checks that ParseHSLFunc gives the
// same 8-bit colors as style's former float32 conversion on a grid of whole
// degrees and percentages. They can only differ by one level where the
// exact channel is a tie between two levels (178.5): there the last bit of
// either arithmetic decides, and float64 is the one CSS specifies.
func TestHSLMatchesTheOldStyleConversion(t *testing.T) {
	to8 := func(v float32) int { return int(math.Round(float64(min(max(v, 0), 1)) * 255)) }
	same := func(got, old float32) bool {
		d := to8(got) - to8(old)
		if d == 0 {
			return true
		}
		frac := math.Abs(float64(min(max(old, 0), 1))*255 - math.Floor(float64(min(max(old, 0), 1))*255) - 0.5)
		return (d == 1 || d == -1) && frac < 1e-3
	}
	n := 0
	for h := 0; h < 360; h += 3 {
		for s := 0; s <= 100; s += 5 {
			for l := 0; l <= 100; l += 5 {
				args := []string{itoa(h), itoa(s) + "%", itoa(l) + "%"}
				got, err := ParseHSLFunc(args[0] + " " + args[1] + " " + args[2])
				if err != nil {
					t.Fatal(err)
				}
				r, g, b := refHSLToRGB(float32(h), float32(s)/100, float32(l)/100)
				if !same(got.R, r) || !same(got.G, g) || !same(got.B, b) {
					t.Errorf("hsl(%d %d%% %d%%) = %d,%d,%d; old %d,%d,%d", h, s, l,
						to8(got.R), to8(got.G), to8(got.B), to8(r), to8(g), to8(b))
					n++
					if n > 10 {
						return
					}
				}
			}
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for ; v > 0; v /= 10 {
		b = append([]byte{byte('0' + v%10)}, b...)
	}
	return string(b)
}
