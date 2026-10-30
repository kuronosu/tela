package gfx

import (
	"errors"
	"testing"
)

func TestParseOKLCHFunc(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Color
	}{
		{"0.637 0.237 25.331", OKLCH(0.637, 0.237, 25.331, 1)},
		{"  0.637   0.237 25.331  ", OKLCH(0.637, 0.237, 25.331, 1)},
		{"63.7% 0.237 25.331 / 0.5", OKLCH(0.637, 0.237, 25.331, 0.5)},
		{"63.7% 59.25% 25.331/50%", OKLCH(0.637, 0.237, 25.331, 0.5)},
		{"0.5 0.1 0.25turn", OKLCH(0.5, 0.1, 90, 1)},
		{"0.5 0.1 100grad", OKLCH(0.5, 0.1, 90, 1)},
		{"0.5 0.1 90deg", OKLCH(0.5, 0.1, 90, 1)},
		{"1 0 none", OKLCH(1, 0, 0, 1)},
		{"none none none / none", OKLCH(0, 0, 0, 0)},
		{"0.5 0.1 200 / 2", OKLCH(0.5, 0.1, 200, 1)},
	} {
		got, err := ParseOKLCHFunc(tt.in)
		if err != nil {
			t.Errorf("oklch(%s): %v", tt.in, err)
			continue
		}
		if !colorNear(got, tt.want.R, tt.want.G, tt.want.B, tt.want.A, 1.0/255) {
			t.Errorf("oklch(%s) = %v, want %v", tt.in, got, tt.want)
		}
	}
	for _, tt := range []struct {
		in   string
		want ColorFuncProblem
	}{
		{"0.5,0.1,200", ColorFuncCommas},
		{"0.5, 0.1, 200, 0.5", ColorFuncCommas},
		{"0.5 0.1, 200", ColorFuncCommas},
		{"0.5,0.1/200", ColorFuncCommas},
		{"0.5 0.1 200 0.5", ColorFuncArity},
		{"0.5 0.1", ColorFuncArity},
		{"", ColorFuncArity},
		{"0.5 0.1 200 / ", ColorFuncSlash},
		{"0.5 0.1 200 / 1 2", ColorFuncSlash},
		{"x 0.1 200", ColorFuncComponent},
		{"0.5 0.1x 200", ColorFuncComponent},
		{"0.5 0.1 red", ColorFuncHue},
		{"0.5 0.1 200 / x", ColorFuncAlpha},
	} {
		_, err := ParseOKLCHFunc(tt.in)
		var ce *ColorFuncError
		if !errors.As(err, &ce) {
			t.Errorf("oklch(%s): err = %v, want a ColorFuncError", tt.in, err)
			continue
		}
		if ce.Problem != tt.want || ce.Func != "oklch" || ce.Error() == "" {
			t.Errorf("oklch(%s): %+v, want problem %d", tt.in, ce, tt.want)
		}
	}
}

func TestParseOklabFunc(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Color
	}{
		{"0.7 0.1 -0.1", Oklab(0.7, 0.1, -0.1, 1)},
		{"70% 25% -25% / 0.5", Oklab(0.7, 0.1, -0.1, 0.5)},
		{"0.7 none none", Oklab(0.7, 0, 0, 1)},
		{"0.7 0.1 -0.1/none", Oklab(0.7, 0.1, -0.1, 0)},
	} {
		got, err := ParseOklabFunc(tt.in)
		if err != nil {
			t.Errorf("oklab(%s): %v", tt.in, err)
			continue
		}
		if !colorNear(got, tt.want.R, tt.want.G, tt.want.B, tt.want.A, 1.0/255) {
			t.Errorf("oklab(%s) = %v, want %v", tt.in, got, tt.want)
		}
	}
	for _, tt := range []struct {
		in   string
		want ColorFuncProblem
	}{
		{"0.7,0.1,-0.1", ColorFuncCommas},
		{"0.7 0.1 -0.1, 0.5", ColorFuncCommas},
		{"0.7 0.1 -0.1 0.5", ColorFuncArity},
		{"0.7 0.1", ColorFuncArity},
		{"0.7 0.1 -0.1 /", ColorFuncSlash},
		{"0.7 0.1 90deg", ColorFuncComponent},
		{"0.7 x -0.1", ColorFuncComponent},
		{"0.7 0.1 -0.1 / y", ColorFuncAlpha},
	} {
		_, err := ParseOklabFunc(tt.in)
		var ce *ColorFuncError
		if !errors.As(err, &ce) {
			t.Errorf("oklab(%s): err = %v, want a ColorFuncError", tt.in, err)
			continue
		}
		if ce.Problem != tt.want || ce.Func != "oklab" || ce.Error() == "" {
			t.Errorf("oklab(%s): %+v, want problem %d", tt.in, ce, tt.want)
		}
	}
}

// TestParseOKLCHFuncHugeHue: the hue wraps in float64, so a huge angle
// keeps the fraction that float32 would lose.
func TestParseOKLCHFuncHugeHue(t *testing.T) {
	for _, tt := range []struct{ in, same string }{
		{"0.6 0.2 1e9turn", "0.6 0.2 0"},
		{"0.6 0.2 99999999.25turn", "0.6 0.2 90"},
		{"0.6 0.2 -1e9turn", "0.6 0.2 0"},
		{"0.6 0.2 -90", "0.6 0.2 270"},
		{"0.6 0.2 -360", "0.6 0.2 0"},
		{"0.6 0.2 450", "0.6 0.2 90"},
		{"0.6 0.2 1e9", "0.6 0.2 280"},
	} {
		got, err := ParseOKLCHFunc(tt.in)
		if err != nil {
			t.Errorf("oklch(%s): %v", tt.in, err)
			continue
		}
		want, err := ParseOKLCHFunc(tt.same)
		if err != nil {
			t.Fatal(err)
		}
		if !colorNear(got, want.R, want.G, want.B, want.A, 1.0/255) {
			t.Errorf("oklch(%s) = %v, want oklch(%s) = %v", tt.in, got, tt.same, want)
		}
	}
}
