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
		{"0.5,0.1,200", ColorFuncSyntax},
		{"0.5, 0.1, 200, 0.5", ColorFuncSyntax},
		{"0.5 0.1, 200", ColorFuncSyntax},
		{"0.5,0.1/200", ColorFuncSyntax},
		{"0.5 0.1 200 0.5", ColorFuncArity},
		{"0.5 0.1", ColorFuncArity},
		{"", ColorFuncArity},
		{"0.5 0.1 200 / ", ColorFuncSyntax},
		{"0.5 0.1 200 / 1 2", ColorFuncSyntax},
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
		{"0.7,0.1,-0.1", ColorFuncSyntax},
		{"0.7 0.1 -0.1, 0.5", ColorFuncSyntax},
		{"0.7 0.1 -0.1 0.5", ColorFuncArity},
		{"0.7 0.1", ColorFuncArity},
		{"0.7 0.1 -0.1 /", ColorFuncSyntax},
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
