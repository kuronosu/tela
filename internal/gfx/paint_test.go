package gfx

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func stopOffsets(s []GradientStop) []float32 {
	out := make([]float32, len(s))
	for i := range s {
		out[i] = s[i].Offset
	}
	return out
}

func equalOffsets(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !near(a[i], b[i], 1e-6) {
			return false
		}
	}
	return true
}

func TestNormalizeStops(t *testing.T) {
	c := []Color{Black, White, RGB8(255, 0, 0), RGB8(0, 255, 0), RGB8(0, 0, 255)}
	auto := func(i int) GradientStop { return AutoStop(c[i]) }
	at := func(o float32, i int) GradientStop { return GradientStop{o, c[i]} }
	tests := []struct {
		name string
		in   []GradientStop
		want []float32
	}{
		{"explicit", []GradientStop{at(0, 0), at(0.3, 1), at(1, 2)}, []float32{0, 0.3, 1}},
		{"all auto, two stops", []GradientStop{auto(0), auto(1)}, []float32{0, 1}},
		{"all auto, three stops", []GradientStop{auto(0), auto(1), auto(2)}, []float32{0, 0.5, 1}},
		{"all auto, five stops", []GradientStop{auto(0), auto(1), auto(2), auto(3), auto(4)}, []float32{0, 0.25, 0.5, 0.75, 1}},
		{"auto between explicit", []GradientStop{at(0.2, 0), auto(1), auto(2), at(0.8, 3)}, []float32{0.2, 0.4, 0.6, 0.8}},
		{"auto last gets 1", []GradientStop{at(0.5, 0), auto(1)}, []float32{0.5, 1}},
		{"auto first gets 0", []GradientStop{auto(0), at(0.5, 1)}, []float32{0, 0.5}},
		{"clamped to [0,1]", []GradientStop{at(-0.5, 0), at(1.5, 1)}, []float32{0, 1}},
		{"non-decreasing", []GradientStop{at(0.5, 0), at(0.2, 1), at(0.8, 2)}, []float32{0.5, 0.5, 0.8}},
		{"hard stop", []GradientStop{at(0, 0), at(0.5, 1), at(0.5, 2), at(1, 3)}, []float32{0, 0.5, 0.5, 1}},
		{"auto after raised stop", []GradientStop{at(0.6, 0), at(0.3, 1), auto(2), at(1, 3)}, []float32{0.6, 0.6, 0.8, 1}},
		{"single", []GradientStop{auto(0)}, []float32{0}},
	}
	for _, tt := range tests {
		in := append([]GradientStop(nil), tt.in...)
		got := NormalizeStops(tt.in)
		if !equalOffsets(stopOffsets(got), tt.want) {
			t.Errorf("%s: offsets = %v want %v", tt.name, stopOffsets(got), tt.want)
		}
		for i := range got {
			if got[i].Color != tt.in[i].Color {
				t.Errorf("%s: color %d changed", tt.name, i)
			}
			// The input must not be modified.
			if !(in[i].Offset == tt.in[i].Offset || (math.IsNaN(float64(in[i].Offset)) && math.IsNaN(float64(tt.in[i].Offset)))) {
				t.Errorf("%s: input was modified", tt.name)
			}
		}
		for i := 1; i < len(got); i++ {
			if got[i].Offset < got[i-1].Offset {
				t.Errorf("%s: output not monotonic: %v", tt.name, stopOffsets(got))
			}
		}
	}
	if NormalizeStops(nil) != nil || NormalizeStops([]GradientStop{}) != nil {
		t.Error("no stops should give nil")
	}
}

func TestPaintInterface(t *testing.T) {
	var _ Paint = White
	var _ Paint = LinearGradient{}
	var _ Paint = RadialGradient{}
	var _ Paint = ConicGradient{}
	if InterpOklab != 0 {
		t.Error("Oklab must be the zero value of ColorSpace (ADR-023)")
	}
	if InterpOklab.String() != "oklab" || InterpSRGB.String() != "srgb" {
		t.Error("ColorSpace.String wrong")
	}
	var lg LinearGradient
	if lg.Interp != InterpOklab {
		t.Error("a zero-value gradient must interpolate in Oklab")
	}
}

func TestSanitizePaint(t *testing.T) {
	nan := float32(math.NaN())
	stops := []GradientStop{AutoStop(RGB8(255, 0, 0)), AutoStop(Color{0, 0, 3, 1})}
	tests := []struct {
		name string
		p    Paint
		ok   bool
	}{
		{"solid", RGB8(1, 2, 3), true},
		{"transparent solid", Transparent, false},
		{"nan solid", Color{nan, 0, 0, 1}, false},
		{"linear", LinearGradient{End: Pt(10, 0), Stops: stops}, true},
		{"linear nan", LinearGradient{End: Pt(nan, 0), Stops: stops}, false},
		{"linear no stops", LinearGradient{End: Pt(10, 0)}, false},
		{"linear transparent stops", LinearGradient{End: Pt(10, 0), Stops: []GradientStop{{0, Transparent}, {1, Transparent}}}, false},
		{"radial", RadialGradient{Radius: Pt(5, 5), Stops: stops}, true},
		{"radial inf", RadialGradient{Radius: Pt(float32(math.Inf(1)), 5), Stops: stops}, false},
		{"conic", ConicGradient{Stops: stops, StartAngle: 1}, true},
		{"conic nan angle", ConicGradient{Stops: stops, StartAngle: nan}, false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		got, ok := sanitizePaint(tt.p)
		if ok != tt.ok {
			t.Errorf("%s: ok = %v", tt.name, ok)
		}
		if ok && got == nil {
			t.Errorf("%s: nil paint returned", tt.name)
		}
	}
	// The sanitized gradient owns its stops (no aliasing) and has them
	// normalized and clamped.
	in := LinearGradient{End: Pt(1, 0), Stops: stops}
	got, _ := sanitizePaint(in)
	g := got.(LinearGradient)
	if &g.Stops[0] == &in.Stops[0] {
		t.Error("stops are shared with the caller")
	}
	if g.Stops[0].Offset != 0 || g.Stops[1].Offset != 1 || g.Stops[1].Color.B != 1 {
		t.Errorf("stops not normalized/clamped: %+v", g.Stops)
	}
	if !math.IsNaN(float64(in.Stops[0].Offset)) {
		t.Error("the caller's stops were modified")
	}
}

func TestFilterConstructors(t *testing.T) {
	apply := func(m ColorMatrix, c Color) Color {
		v := [5]float32{c.R, c.G, c.B, c.A, 1}
		var o [4]float32
		for r := 0; r < 4; r++ {
			for k := 0; k < 5; k++ {
				o[r] += m.M[r*5+k] * v[k]
			}
		}
		return Color{o[0], o[1], o[2], o[3]}.Clamp()
	}
	px := Color{0.8, 0.4, 0.2, 0.6}
	tests := []struct {
		name string
		m    ColorMatrix
		in   Color
		want Color
	}{
		{"brightness 1", Brightness(1), px, px},
		{"brightness 0.5", Brightness(0.5), px, Color{0.4, 0.2, 0.1, 0.6}},
		{"brightness 2 clamps", Brightness(2), px, Color{1, 0.8, 0.4, 0.6}},
		{"brightness negative", Brightness(-1), px, Color{0, 0, 0, 0.6}},
		{"contrast 1", Contrast(1), px, px},
		{"contrast 0", Contrast(0), px, Color{0.5, 0.5, 0.5, 0.6}},
		{"contrast 2", Contrast(2), Color{0.6, 0.5, 0.25, 1}, Color{0.7, 0.5, 0, 1}},
		{"grayscale 0", Grayscale(0), px, px},
		{"grayscale 1 white", Grayscale(1), White, White},
		{"grayscale 1 red", Grayscale(1), Color{1, 0, 0, 1}, Color{0.2126, 0.2126, 0.2126, 1}},
		{"grayscale clamps >1", Grayscale(5), Color{1, 0, 0, 1}, Color{0.2126, 0.2126, 0.2126, 1}},
		{"saturate 1", Saturate(1), px, px},
		{"saturate 0 = luminance", Saturate(0), Color{1, 0, 0, 1}, Color{0.213, 0.213, 0.213, 1}},
		{"sepia 0", Sepia(0), px, px},
		{"sepia 1 white", Sepia(1), White, Color{1, 1, 0.937, 1}},
		{"invert 0", Invert(0), px, px},
		{"invert 1", Invert(1), px, Color{0.2, 0.6, 0.8, 0.6}},
		{"invert 0.5 is flat gray", Invert(0.5), px, Color{0.5, 0.5, 0.5, 0.6}},
		{"hue-rotate 0", HueRotate(0), px, px},
		{"hue-rotate 360", HueRotate(2 * math.Pi), Color{0.3, 0.6, 0.9, 1}, Color{0.3, 0.6, 0.9, 1}},
		{"hue-rotate keeps gray", HueRotate(1.234), Color{0.5, 0.5, 0.5, 1}, Color{0.5, 0.5, 0.5, 1}},
	}
	for _, tt := range tests {
		// Sepia(1) white: R = 0.393+0.769+0.189 = 1.351 -> clamped to 1.
		if got := apply(tt.m, tt.in); !colorNear(got, tt.want.R, tt.want.G, tt.want.B, tt.want.A, 2e-3) {
			t.Errorf("%s: got %+v want %+v", tt.name, got, tt.want)
		}
	}
	// hue-rotate(180deg) of pure red from the Filter Effects matrix:
	// r' = 0.213 + (-1)*0.787 = -0.574 -> 0, g' = 0.213+0.213 = 0.426, b' = 0.426.
	got := apply(HueRotate(math.Pi), Color{1, 0, 0, 1})
	if !colorNear(got, 0, 0.426, 0.426, 1, 1e-3) {
		t.Errorf("hue-rotate(180) of red = %+v", got)
	}
	if !IdentityMatrix.IsIdentity() || Brightness(1) != IdentityMatrix {
		t.Error("identity detection wrong")
	}
	if Saturate(1).Mul(IdentityMatrix) != Saturate(1) {
		t.Error("Mul with identity changed the matrix")
	}
}

func TestMatrixMulComposition(t *testing.T) {
	apply := func(m ColorMatrix, v [4]float32) [4]float32 {
		in := [5]float32{v[0], v[1], v[2], v[3], 1}
		var o [4]float32
		for r := 0; r < 4; r++ {
			for k := 0; k < 5; k++ {
				o[r] += m.M[r*5+k] * in[k]
			}
		}
		return o
	}
	m := Contrast(0.7)
	n := Invert(0.8)
	v := [4]float32{0.3, 0.6, 0.9, 0.5}
	want := apply(n, apply(m, v))
	got := apply(m.Mul(n), v)
	for i := range want {
		if !near(got[i], want[i], 1e-5) {
			t.Fatalf("Mul composition: got %v want %v", got, want)
		}
	}
}

func TestFilterOutsets(t *testing.T) {
	tests := []struct {
		f    Filter
		want float32
	}{
		{Blur{4}, 12}, {Blur{0}, 0}, {Blur{-3}, 0},
		{DropShadow{Offset: Pt(10, -3), StdDev: 2}, float32(math.Sqrt(109)) + 6},
		{DropShadow{Offset: Pt(-20, 0), StdDev: 0}, 20},
		{DropShadow{Offset: Pt(3, -4), StdDev: 1}, 8},
		{Brightness(2), 0},
	}
	for _, tt := range tests {
		if got := FilterOutset(tt.f); got != tt.want {
			t.Errorf("FilterOutset(%v) = %v want %v", tt.f, got, tt.want)
		}
	}
	if got := FiltersOutset([]Filter{Blur{1}, Blur{2}, Sepia(1)}); got != 9 {
		t.Errorf("FiltersOutset = %v", got)
	}
}

func TestSanitizeFilters(t *testing.T) {
	nan := float32(math.NaN())
	out, ok := sanitizeFilters([]Filter{Blur{0}, Blur{3}, DropShadow{StdDev: -1, Color: Color{2, 0, 0, 1}}})
	if !ok || len(out) != 2 {
		t.Fatalf("got %v %v", out, ok)
	}
	if ds := out[1].(DropShadow); ds.StdDev != 0 || ds.Color.R != 1 {
		t.Errorf("drop shadow not clamped: %+v", ds)
	}
	for _, bad := range [][]Filter{{Blur{nan}}, {DropShadow{Offset: Pt(nan, 0)}}, {ColorMatrix{M: [20]float32{nan}}}, {nil}} {
		if _, ok := sanitizeFilters(bad); ok {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestBlendMode(t *testing.T) {
	if SrcOver != 0 || SrcOver.String() != "normal" || Multiply.String() != "multiply" || ColorBlend.String() != "color" || PlusLighter.String() != "plus-lighter" {
		t.Error("BlendMode names wrong")
	}
	for b := SrcOver; b <= PlusLighter; b++ {
		if b.String() == "" || b.String() == "BlendMode(?)" {
			t.Errorf("blend %d has no name", b)
		}
	}
	if !Multiply.IsSeparable() || !PlusLighter.IsSeparable() || Hue.IsSeparable() || Luminosity.IsSeparable() {
		t.Error("IsSeparable wrong")
	}
}

func TestImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(2, 3, 4, 5)) // non-zero origin
	src.SetRGBA(2, 3, color.RGBA{10, 20, 30, 255})
	src.SetRGBA(3, 4, color.RGBA{1, 2, 3, 4})
	img := NewImage(src)
	if img.Width() != 2 || img.Height() != 2 || img.Stride() != 8 || img.Bounds() != (Rect{0, 0, 2, 2}) {
		t.Fatalf("dims: %d %d %d %v", img.Width(), img.Height(), img.Stride(), img.Bounds())
	}
	pix := img.Pix()
	if len(pix) != 16 || pix[0] != 10 || pix[3] != 255 || pix[8+4] != 1 || pix[8+7] != 4 {
		t.Errorf("pixels = %v", pix)
	}
	// Copy: mutating the source must not change the Image.
	src.SetRGBA(2, 3, color.RGBA{99, 99, 99, 99})
	if img.Pix()[0] != 10 {
		t.Error("NewImage did not copy")
	}
	if NewImage(src).ID() == img.ID() || img.ID() == 0 {
		t.Error("IDs must be unique and non-zero")
	}
	// Conversion from a non-premultiplied image.
	n := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	n.SetNRGBA(0, 0, color.NRGBA{200, 100, 50, 128})
	conv := NewImage(n)
	if p := conv.Pix(); p[0] != 100 || p[3] != 128 {
		t.Errorf("premultiplied conversion = %v", p)
	}
	if p := NewImagePremul(1, 1, []byte{1, 2, 3, 4}); p.Pix()[2] != 3 {
		t.Error("NewImagePremul")
	}
	if SampleLinear != 0 || SampleLinear.String() != "linear" || SampleNearest.String() != "nearest" {
		t.Error("Sampling wrong")
	}
	for name, f := range map[string]func(){
		"nil":       func() { NewImage(nil) },
		"bad bytes": func() { NewImagePremul(2, 2, make([]byte, 3)) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
}
