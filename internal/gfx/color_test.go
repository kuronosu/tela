package gfx

import (
	"image/color"
	"math"
	"strings"
	"testing"
)

func colorNear(c Color, r, g, b, a, eps float32) bool {
	return near(c.R, r, eps) && near(c.G, g, eps) && near(c.B, b, eps) && near(c.A, a, eps)
}

func TestParseHex(t *testing.T) {
	tests := []struct {
		in   string
		want Color
	}{
		{"#fff", RGBA8(255, 255, 255, 255)},
		{"#000", RGBA8(0, 0, 0, 255)},
		{"#f80", RGBA8(0xff, 0x88, 0x00, 255)},
		{"#f808", RGBA8(0xff, 0x88, 0x00, 0x88)},
		{"#1e1e2e", RGBA8(0x1e, 0x1e, 0x2e, 255)},
		{"#1E1E2E", RGBA8(0x1e, 0x1e, 0x2e, 255)},
		{"#11223344", RGBA8(0x11, 0x22, 0x33, 0x44)},
		{"#ffffff00", RGBA8(255, 255, 255, 0)},
	}
	for _, tt := range tests {
		got, err := ParseHex(tt.in)
		if err != nil {
			t.Errorf("ParseHex(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseHex(%q) = %v want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseHexErrors(t *testing.T) {
	tests := []struct {
		in      string
		wantSub string
	}{
		{"", "must start with '#'"},
		{"fff", "must start with '#'"},
		{"#", "expected 3, 4, 6 or 8 hex digits"},
		{"#ff", "expected 3, 4, 6 or 8 hex digits"},
		{"#fffff", "expected 3, 4, 6 or 8 hex digits"},
		{"#fffffffff", "expected 3, 4, 6 or 8 hex digits"},
		{"#ggg", "'g' at position 1"},
		{"#12345z", "'z' at position 6"},
	}
	for _, tt := range tests {
		_, err := ParseHex(tt.in)
		if err == nil {
			t.Errorf("ParseHex(%q): expected an error", tt.in)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("ParseHex(%q) error %q does not contain %q", tt.in, err, tt.wantSub)
		}
		if !strings.Contains(err.Error(), tt.in) && tt.in != "" {
			t.Errorf("ParseHex(%q) error %q should quote the input", tt.in, err)
		}
	}
}

func TestMustParseHexPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParseHex(bad) did not panic")
		}
	}()
	MustParseHex("#zz")
}

func TestColorString(t *testing.T) {
	tests := []struct {
		c    Color
		want string
	}{
		{White, "#ffffff"}, {Black, "#000000"}, {RGBA8(0x1e, 0x1e, 0x2e, 0x80), "#1e1e2e80"},
		{Transparent, "#00000000"}, {Color{2, -1, 0.5, 1}, "#ff0080"},
	}
	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("String(%v) = %q want %q", [4]float32{tt.c.R, tt.c.G, tt.c.B, tt.c.A}, got, tt.want)
		}
	}
}

func TestAlphaHelpers(t *testing.T) {
	if got := White.MulAlpha(0.15); !colorNear(got, 1, 1, 1, 0.15, 1e-7) {
		t.Errorf("MulAlpha = %v", got)
	}
	if got := RGBA8(1, 2, 3, 128).MulAlpha(0.5); !near(got.A, 128.0/255/2, 1e-7) {
		t.Errorf("MulAlpha of translucent = %v", got.A)
	}
	if got := Black.WithAlpha(0.25); got.A != 0.25 || got.R != 0 {
		t.Errorf("WithAlpha = %v", got)
	}
	if !White.IsOpaque() || Transparent.IsOpaque() {
		t.Error("IsOpaque wrong")
	}
	if (Color{float32(math.NaN()), 0, 0, 1}).IsFinite() {
		t.Error("NaN color reported finite")
	}
	if got := (Color{float32(math.NaN()), 2, -1, 0.5}).Clamp(); got != (Color{0, 1, 0, 0.5}) {
		t.Errorf("Clamp = %v", got)
	}
}

func TestPremul8(t *testing.T) {
	tests := []struct {
		name string
		c    Color
		want color.RGBA
	}{
		{"opaque", RGBA8(10, 20, 30, 255), color.RGBA{10, 20, 30, 255}},
		{"transparent", Color{1, 1, 1, 0}, color.RGBA{0, 0, 0, 0}},
		{"half white", Color{1, 1, 1, 0.5}, color.RGBA{128, 128, 128, 128}},
		// 0.5*255 = 127.5 rounds half up.
		{"half black alpha", Color{0, 0, 0, 0.5}, color.RGBA{0, 0, 0, 128}},
		// 200/255 * 100/255 * 255 = 78.43 -> 78; premultiplying the rounded
		// bytes would also give 78 here, but 0.3*0.3*255 = 22.95 -> 23.
		{"exact float path", Color{0.3, 0.3, 0.3, 0.3}, color.RGBA{23, 23, 23, 77}},
		{"white 15%", White.MulAlpha(0.15), color.RGBA{38, 38, 38, 38}},
		{"clamped", Color{2, 2, 2, 2}, color.RGBA{255, 255, 255, 255}},
		{"negative", Color{-1, 0, 0, 1}, color.RGBA{0, 0, 0, 255}},
	}
	for _, tt := range tests {
		if got := tt.c.Premul8(); got != tt.want {
			t.Errorf("%s: Premul8 = %v want %v", tt.name, got, tt.want)
		}
	}
	// Invariant: premultiplied channels never exceed alpha.
	for a := 0; a <= 255; a += 5 {
		for v := 0; v <= 255; v += 5 {
			c := RGBA8(uint8(v), uint8(255-v), uint8(v/2), uint8(a))
			p := c.Premul8()
			if p.R > p.A || p.G > p.A || p.B > p.A {
				t.Fatalf("Premul8(%v) = %v violates channel <= alpha", c, p)
			}
		}
	}
	if got := RGBA8(10, 20, 30, 40).NRGBA8(); got != (color.NRGBA{10, 20, 30, 40}) {
		t.Errorf("NRGBA8 roundtrip = %v", got)
	}
	r, g, b, a := Color{1, 0.5, 0, 0.5}.Premultiplied()
	if r != 0.5 || g != 0.25 || b != 0 || a != 0.5 {
		t.Errorf("Premultiplied = %v %v %v %v", r, g, b, a)
	}
}

// Reference values: color.js 0.7.1 (https://github.com/color-js/color.js, the
// library behind colorjs.io, whose "css" gamut method implements CSS Color 4
// section 13) via new Color("oklch", [...]).toGamut({space: "srgb", method:
// "css"}).to("srgb").
// The first rows are Tailwind v4 palette entries (red-500 is #fb2c36).
var oklchRefs = []struct {
	l, c, h float64
	srgb    [3]float64
}{
	{0.637, 0.237, 25.331, [3]float64{0.982661, 0.171797, 0.213070}},
	{0.623, 0.214, 259.815, [3]float64{0.169333, 0.498049, 1.000000}},
	{0.7, 0.1, 180, [3]float64{0.292642, 0.700962, 0.630168}},
	{0.97, 0.014, 254.604, [3]float64{0.936940, 0.964035, 0.998872}},
	{0.985, 0, 0, [3]float64{0.980256, 0.980256, 0.980256}},
	{0.9, 0.4, 150, [3]float64{0.256812, 1.000000, 0.528226}},
	{0.5, 0.4, 30, [3]float64{0.765954, 0.000000, 0}},
	{0.9, 0.3, 300, [3]float64{0.901752, 0.825016, 1.000000}},
	{0.2, 0.3, 200, [3]float64{0, 0.111948, 0.123349}},
	{0.5, 0.3, 120, [3]float64{0.352613, 0.428735, 0}},
	{0.8, 0.2, 80, [3]float64{0.983077, 0.678082, 0}},
	{0.35, 0.25, 280, [3]float64{0.208979, 0.000000, 0.683101}},
	{1.2, 0.1, 0, [3]float64{1, 1, 1}},
	{-0.1, 0.1, 0, [3]float64{0, 0, 0}},
}

func TestOKLCHAgainstColorJS(t *testing.T) {
	for _, tt := range oklchRefs {
		got := OKLCH(float32(tt.l), float32(tt.c), float32(tt.h), 0.5)
		if !colorNear(got, float32(tt.srgb[0]), float32(tt.srgb[1]), float32(tt.srgb[2]), 0.5, 3e-4) {
			t.Errorf("OKLCH(%v, %v, %v) = (%.6f %.6f %.6f %.2f), want %v",
				tt.l, tt.c, tt.h, got.R, got.G, got.B, got.A, tt.srgb)
		}
	}
}

func TestOKLCHTailwindHex(t *testing.T) {
	// Tailwind v4 default theme: red-500 = oklch(63.7% 0.237 25.331),
	// published as #fb2c36.
	if got := OKLCH(0.637, 0.237, 25.331, 1).String(); got != "#fb2c36" {
		t.Errorf("red-500 = %s want #fb2c36", got)
	}
	// gray-50 = oklch(98.5% 0 0) = #fafafa.
	if got := OKLCH(0.985, 0, 0, 1).String(); got != "#fafafa" {
		t.Errorf("neutral-50 = %s want #fafafa", got)
	}
}

func TestOklabAgainstColorJS(t *testing.T) {
	tests := []struct {
		l, a, b float64
		srgb    [3]float64
	}{
		{0.4, 0.1, 0.05, [3]float64{0.473092, 0.162486, 0.145117}},
		{0.59686, 0.1009, 0.1192, [3]float64{0.776020, 0.363447, 0.025954}},
		{0.8, -0.1, 0.1, [3]float64{0.585189, 0.822983, 0.447514}},
		{0.3, -0.05, -0.1, [3]float64{0, 0.188649, 0.362729}},
		{0.95, 0.3, 0.3, [3]float64{1.000000, 0.896802, 0.827577}},
	}
	for _, tt := range tests {
		got := Oklab(float32(tt.l), float32(tt.a), float32(tt.b), 1)
		if !colorNear(got, float32(tt.srgb[0]), float32(tt.srgb[1]), float32(tt.srgb[2]), 1, 3e-4) {
			t.Errorf("Oklab(%v, %v, %v) = (%.6f %.6f %.6f), want %v", tt.l, tt.a, tt.b, got.R, got.G, got.B, tt.srgb)
		}
	}
}

func TestToOklab(t *testing.T) {
	// Reference: colorjs.io new Color("srgb", [...]).to("oklab" / "oklch").
	tests := []struct {
		srgb, lab, lch [3]float64
	}{
		{[3]float64{1, 0, 0}, [3]float64{0.627955, 0.224863, 0.125846}, [3]float64{0.627955, 0.257683, 29.233880}},
		{[3]float64{0, 1, 0}, [3]float64{0.866440, -0.233888, 0.179498}, [3]float64{0.866440, 0.294827, 142.495345}},
		{[3]float64{0, 0, 1}, [3]float64{0.452014, -0.032457, -0.311528}, [3]float64{0.452014, 0.313214, 264.052023}},
		{[3]float64{0.2, 0.4, 0.8}, [3]float64{0.532483, -0.022512, -0.166349}, [3]float64{0.532483, 0.167866, 262.293049}},
		{[3]float64{0.9, 0.5, 0.1}, [3]float64{0.697163, 0.086100, 0.135519}, [3]float64{0.697163, 0.160557, 57.570801}},
		{[3]float64{1, 1, 1}, [3]float64{1, 0, 0}, [3]float64{1, 0, 0}},
		{[3]float64{0, 0, 0}, [3]float64{0, 0, 0}, [3]float64{0, 0, 0}},
		{[3]float64{0.5, 0.5, 0.5}, [3]float64{0.598181, 0, 0}, [3]float64{0.598181, 0, 0}},
	}
	for _, tt := range tests {
		c := Color{float32(tt.srgb[0]), float32(tt.srgb[1]), float32(tt.srgb[2]), 1}
		l, a, b := c.ToOklab()
		if !near(l, float32(tt.lab[0]), 1e-4) || !near(a, float32(tt.lab[1]), 1e-4) || !near(b, float32(tt.lab[2]), 1e-4) {
			t.Errorf("ToOklab(%v) = %v %v %v want %v", tt.srgb, l, a, b, tt.lab)
		}
		l, ch, h := c.ToOKLCH()
		if !near(l, float32(tt.lch[0]), 1e-4) || !near(ch, float32(tt.lch[1]), 1e-4) || !near(h, float32(tt.lch[2]), 2e-3) {
			t.Errorf("ToOKLCH(%v) = %v %v %v want %v", tt.srgb, l, ch, h, tt.lch)
		}
	}
}

func TestOklabRoundTrip(t *testing.T) {
	for r := 0; r <= 255; r += 51 {
		for g := 0; g <= 255; g += 51 {
			for b := 0; b <= 255; b += 51 {
				c := RGB8(uint8(r), uint8(g), uint8(b))
				l, a, bb := c.ToOklab()
				back := Oklab(l, a, bb, 1)
				if !colorNear(back, c.R, c.G, c.B, 1, 2e-3) {
					t.Errorf("round trip of %s gave %s", c, back)
				}
				l, ch, h := c.ToOKLCH()
				back = OKLCH(l, ch, h, 1)
				if !colorNear(back, c.R, c.G, c.B, 2e-3, 2e-3) && !colorNear(back, c.R, c.G, c.B, 1, 2e-3) {
					t.Errorf("OKLCH round trip of %s gave %s", c, back)
				}
			}
		}
	}
}

func TestGamutMappingProperties(t *testing.T) {
	// Every output is inside the gamut, chroma never grows and lightness stays
	// close. (Hue is only guaranteed to within the JND, which at low lightness
	// is a large angle, so it is not asserted.)
	for l := float32(0.05); l < 1; l += 0.1 {
		for ch := float32(0); ch <= 0.5; ch += 0.05 {
			for h := float32(0); h < 360; h += 30 {
				c := OKLCH(l, ch, h, 1)
				if c.R < 0 || c.R > 1 || c.G < 0 || c.G > 1 || c.B < 0 || c.B > 1 || !c.IsFinite() {
					t.Fatalf("OKLCH(%v,%v,%v) = %v is out of gamut", l, ch, h, c)
				}
				gl, gc, _ := c.ToOKLCH()
				if gc > ch+0.01 {
					t.Errorf("OKLCH(%v,%v,%v): chroma grew to %v", l, ch, h, gc)
				}
				if math.Abs(float64(gl-l)) > 0.06 {
					t.Errorf("OKLCH(%v,%v,%v): lightness moved to %v", l, ch, h, gl)
				}
			}
		}
	}
	if got := OKLCH(1, 0.3, 40, 0.25); got != (Color{1, 1, 1, 0.25}) {
		t.Errorf("L=1 should give white keeping alpha, got %v", got)
	}
	if got := OKLCH(0, 0.3, 40, 1); got != Black {
		t.Errorf("L=0 should give black, got %v", got)
	}
	if got := OKLCH(float32(math.NaN()), 0.1, 0, 1); got != Black {
		t.Errorf("NaN lightness should give black, got %v", got)
	}
}

func TestTransferFunctions(t *testing.T) {
	for _, v := range []float32{0, 0.001, 0.04, 0.2, 0.5, 0.9, 1} {
		if got := LinearToSRGB(SRGBToLinear(v)); !near(got, v, 1e-6) {
			t.Errorf("transfer round trip of %v = %v", v, got)
		}
	}
	if got := SRGBToLinear(0.5); !near(got, 0.21404114, 1e-6) {
		t.Errorf("SRGBToLinear(0.5) = %v", got)
	}
	if got := SRGBToLinear(-0.5); !near(got, -0.21404114, 1e-6) {
		t.Errorf("SRGBToLinear(-0.5) = %v, should mirror", got)
	}
	l, a, b := LinearSRGBToOklab(1, 1, 1)
	r, g, bb := OklabToLinearSRGB(l, a, b)
	if !near(r, 1, 1e-5) || !near(g, 1, 1e-5) || !near(bb, 1, 1e-5) {
		t.Errorf("white did not round trip through Oklab: %v %v %v", r, g, bb)
	}
}
