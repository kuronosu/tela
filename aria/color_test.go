package aria_test

import (
	"math"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
)

func closeTo(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestHSBKnownValues(t *testing.T) {
	for _, tc := range []struct {
		hex     string
		h, s, b float64
	}{
		{"#ff0000", 0, 100, 100},
		{"#00ff00", 120, 100, 100},
		{"#0000ff", 240, 100, 100},
		{"#ffff00", 60, 100, 100},
		{"#00ffff", 180, 100, 100},
		{"#ff00ff", 300, 100, 100},
		{"#ffffff", 0, 0, 100},
		{"#000000", 0, 0, 0},
		{"#808080", 0, 0, 50.196},
		{"#336699", 210, 66.667, 60},
		{"#6366f1", 238.73, 58.92, 94.51},
	} {
		got := aria.ToHSB(tela.Hex(tc.hex))
		if !closeTo(got.H, tc.h, 0.1) || !closeTo(got.S, tc.s, 0.01) || !closeTo(got.B, tc.b, 0.05) {
			t.Errorf("ToHSB(%s) = %+v, want %g %g %g", tc.hex, got, tc.h, tc.s, tc.b)
		}
		if got.A != 1 {
			t.Errorf("ToHSB(%s).A = %g, want 1", tc.hex, got.A)
		}
		if back := aria.FormatHexColor(got.Color(), false); back != tc.hex {
			t.Errorf("HSB(%s).Color() = %s", tc.hex, back)
		}
	}
	// Specific constructions.
	if c := (aria.HSB{H: 360, S: 100, B: 100, A: 1}).Color(); c != tela.Hex("#ff0000") {
		t.Errorf("hue 360 is red, got %v", c)
	}
	if c := (aria.HSB{H: -120, S: 100, B: 100, A: 1}).Color(); aria.FormatHexColor(c, false) != "#0000ff" {
		t.Errorf("hue -120 is blue, got %v", c)
	}
	if c := (aria.HSB{H: 10, S: 400, B: -3, A: 9}).Color(); c != (tela.Color{A: 1}) {
		t.Errorf("out of range values are clamped, got %v", c)
	}
}

func TestHSLKnownValues(t *testing.T) {
	for _, tc := range []struct {
		hex     string
		h, s, l float64
	}{
		{"#ff0000", 0, 100, 50},
		{"#ffffff", 0, 0, 100},
		{"#000000", 0, 0, 0},
		{"#336699", 210, 50, 40},
		{"#bfbf40", 60, 49.8, 50},
	} {
		got := aria.ToHSL(tela.Hex(tc.hex))
		if !closeTo(got.H, tc.h, 0.1) || !closeTo(got.S, tc.s, 0.05) || !closeTo(got.L, tc.l, 0.05) {
			t.Errorf("ToHSL(%s) = %+v, want %g %g %g", tc.hex, got, tc.h, tc.s, tc.l)
		}
		if back := aria.FormatHexColor(got.Color(), false); back != tc.hex {
			t.Errorf("HSL(%s).Color() = %s", tc.hex, back)
		}
	}
	if c := (aria.HSL{H: 0, S: 100, L: 50, A: 0.5}).Color(); c != (tela.Color{R: 1, A: 0.5}) {
		t.Errorf("hsl(0 100 50 / 0.5) = %v", c)
	}
	hsb := aria.HSL{H: 210, S: 50, L: 40, A: 1}.HSB()
	if !closeTo(hsb.S, 66.667, 0.01) || !closeTo(hsb.B, 60, 0.01) {
		t.Errorf("HSL to HSB: %+v", hsb)
	}
}

// Every color of a coarse grid of RGB survives HSB and HSL and back, to
// the 8 bits of its code.
func TestColorRoundTrips(t *testing.T) {
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 17 {
				c := tela.RGB(uint8(r), uint8(g), uint8(b))
				want := aria.FormatHexColor(c, false)
				hsb := aria.ToHSB(c)
				if got := aria.FormatHexColor(hsb.Color(), false); got != want {
					t.Fatalf("%s -> HSB %+v -> %s", want, hsb, got)
				}
				hsl := aria.ToHSL(c)
				if got := aria.FormatHexColor(hsl.Color(), false); got != want {
					t.Fatalf("%s -> HSL %+v -> %s", want, hsl, got)
				}
				if got := aria.FormatHexColor(hsl.HSB().Color(), false); got != want {
					t.Fatalf("%s -> HSL -> HSB -> %s", want, got)
				}
				if again := aria.ToHSB(hsb.Color()); !closeTo(again.H, hsb.H, 1e-3) || !closeTo(again.S, hsb.S, 1e-3) || !closeTo(again.B, hsb.B, 1e-3) {
					t.Fatalf("%s: HSB %+v is %+v after a round trip", want, hsb, again)
				}
			}
		}
	}
}

// Deterministic: the same input is the same bits.
func TestColorConversionsAreDeterministic(t *testing.T) {
	for range 3 {
		if got := (aria.HSB{H: 238.7, S: 58.5, B: 94.5, A: 0.3}).Color(); got != (aria.HSB{H: 238.7, S: 58.5, B: 94.5, A: 0.3}).Color() {
			t.Fatal("HSB.Color is not deterministic")
		}
	}
}

func TestOutOfGamutColorsAreClamped(t *testing.T) {
	h := aria.ToHSB(tela.Color{R: 1.2, G: -0.1, B: 0.5, A: 1})
	if h.S < 0 || h.S > 100 || h.B < 0 || h.B > 100 {
		t.Errorf("out of range: %+v", h)
	}
	if got := aria.ToHSB(tela.Color{R: float32(math.NaN()), A: 1}); got.B != 0 {
		t.Errorf("NaN is 0, got %+v", got)
	}
}

func TestParseHexColor(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want tela.Color
	}{
		{"#fff", tela.White},
		{"#000", tela.Black},
		{"fff", tela.White},
		{"#1E1E2E", tela.Hex("#1e1e2e")},
		{"  #1e1e2e ", tela.Hex("#1e1e2e")},
		{"1e1e2e", tela.Hex("#1e1e2e")},
		{"#f008", tela.Hex("#ff000088")},
		{"#ff000080", tela.Hex("#ff000080")},
	} {
		got, err := aria.ParseHexColor(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseHexColor(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"", "empty"},
		{"#", "empty"},
		{"#12", "got 2"},
		{"#12345", "got 5"},
		{"#123456789", "got 9"},
		{"#12345g", `'g' is not a hex digit`},
		{"red", "got 3"}, // "red" has the length of a code but "r" is not a digit
	} {
		_, err := aria.ParseHexColor(tc.in)
		if err == nil {
			t.Errorf("ParseHexColor(%q): no error", tc.in)
			continue
		}
		if tc.in == "red" {
			if !strings.Contains(err.Error(), "not a hex digit") {
				t.Errorf("ParseHexColor(red): %v", err)
			}
			continue
		}
		if !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "aria: invalid hex color") {
			t.Errorf("ParseHexColor(%q): %q lacks %q", tc.in, err, tc.want)
		}
	}
}

func TestFormatHexColor(t *testing.T) {
	c := tela.Hex("#6366f1")
	if got := aria.FormatHexColor(c, false); got != "#6366f1" {
		t.Errorf("got %s", got)
	}
	if got := aria.FormatHexColor(c.WithAlpha(0.5), true); got != "#6366f180" {
		t.Errorf("got %s", got)
	}
	if got := aria.FormatHexColor(c, true); got != "#6366f1ff" {
		t.Errorf("an opaque color with alpha: %s", got)
	}
	if got := aria.FormatHexColor(c.WithAlpha(0.5), false); got != "#6366f1" {
		t.Errorf("got %s", got)
	}
	if got := aria.FormatHexColor(tela.Color{R: 2, G: -1, A: 1}, false); got != "#ff0000" {
		t.Errorf("out of gamut clamps: %s", got)
	}
}
