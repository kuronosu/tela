package style

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// Expected values are copied from gen/tailwind/theme.css (Tailwind v4.3.3).

func TestSpacingAndRoot(t *testing.T) {
	th := DefaultTheme()
	if th.RootFontSize != 16 {
		t.Errorf("RootFontSize = %v, want 16", th.RootFontSize)
	}
	if th.Spacing != Rem(0.25) { // --spacing: 0.25rem
		t.Errorf("Spacing = %v, want 0.25rem", th.Spacing)
	}
	if got := th.Spacing.Resolve(th.RootFontSize, 0) * 4; got != 16 { // p-4
		t.Errorf("p-4 = %v px, want 16", got)
	}
}

func TestBreakpointsAndContainers(t *testing.T) {
	th := DefaultTheme()
	wantBP := map[string]Length{
		"sm": Rem(40), "md": Rem(48), "lg": Rem(64), "xl": Rem(80), "2xl": Rem(96),
	}
	if !reflect.DeepEqual(th.Breakpoints, wantBP) {
		t.Errorf("Breakpoints = %v, want %v", th.Breakpoints, wantBP)
	}
	if got := th.Breakpoints["md"].Resolve(th.RootFontSize, 0); got != 768 {
		t.Errorf("breakpoint-md = %v px, want 768", got)
	}
	for k, want := range map[string]Length{"3xs": Rem(16), "md": Rem(28), "7xl": Rem(80)} {
		if got := th.Containers[k]; got != want {
			t.Errorf("container-%s = %v, want %v", k, got, want)
		}
	}
	if len(th.Containers) != 13 {
		t.Errorf("len(Containers) = %d, want 13", len(th.Containers))
	}
}

func TestTextScale(t *testing.T) {
	th := DefaultTheme()
	// --text-5xl: 3rem; --text-5xl--line-height: 1
	if got := th.Text["5xl"]; got != (TextSize{Size: Rem(3), LineHeight: 1}) {
		t.Errorf("text-5xl = %+v", got)
	}
	// --text-sm: 0.875rem; --text-sm--line-height: calc(1.25 / 0.875)
	if got := th.Text["sm"]; got.Size != Rem(0.875) || got.LineHeight != float32(1.25/0.875) {
		t.Errorf("text-sm = %+v", got)
	}
	if got := th.Text["base"]; got != (TextSize{Size: Rem(1), LineHeight: 1.5}) {
		t.Errorf("text-base = %+v", got)
	}
	// At the default root size every line height is a whole number of px
	// (text-xs is 12px / 16px, text-sm 14px / 20px, text-xl 20px / 28px).
	want := map[string][2]float32{
		"xs": {12, 16}, "sm": {14, 20}, "base": {16, 24}, "lg": {18, 28}, "xl": {20, 28},
		"2xl": {24, 32}, "3xl": {30, 36}, "4xl": {36, 40}, "5xl": {48, 48},
		"6xl": {60, 60}, "7xl": {72, 72}, "8xl": {96, 96}, "9xl": {128, 128},
	}
	for k, w := range want {
		ts := th.Text[k]
		size := ts.Size.Resolve(16, 0)
		line := float32(size * ts.LineHeight)
		if size != w[0] || math.Abs(float64(line-w[1])) > 1e-4 {
			t.Errorf("text-%s = %v px / %v px, want %v px / %v px", k, size, line, w[0], w[1])
		}
	}
	if len(th.Text) != len(want) {
		t.Errorf("len(Text) = %d, want %d", len(th.Text), len(want))
	}
}

func TestTypographyScales(t *testing.T) {
	th := DefaultTheme()
	if got := th.Tracking["tight"]; got != Em(-0.025) { // --tracking-tight
		t.Errorf("tracking-tight = %v", got)
	}
	if got := th.Tracking["widest"]; got != Em(0.1) {
		t.Errorf("tracking-widest = %v", got)
	}
	if got := th.Leading["relaxed"]; got != 1.625 {
		t.Errorf("leading-relaxed = %v", got)
	}
	if got := th.FontWeights["semibold"]; got != 600 {
		t.Errorf("font-weight-semibold = %v", got)
	}
	if len(th.FontWeights) != 9 {
		t.Errorf("len(FontWeights) = %d, want 9", len(th.FontWeights))
	}
}

func TestRadius(t *testing.T) {
	th := DefaultTheme()
	if got := th.Radius["3xl"]; got != Rem(1.5) { // --radius-3xl: 1.5rem
		t.Errorf("radius-3xl = %v", got)
	}
	if got := th.Radius["DEFAULT"]; got != Rem(0.25) { // deprecated --radius
		t.Errorf("rounded = %v", got)
	}
}

func TestShadows(t *testing.T) {
	th := DefaultTheme()
	black := func(a float32) gfx.Color { return gfx.Color{A: a} }
	// --shadow-2xl: 0 25px 50px -12px rgb(0 0 0 / 0.25)
	want := []Shadow{{OffsetY: 25, Blur: 50, Spread: -12, Color: black(0.25)}}
	if got := th.Shadows["2xl"]; !reflect.DeepEqual(got, want) {
		t.Errorf("shadow-2xl = %+v, want %+v", got, want)
	}
	// --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)
	want = []Shadow{
		{OffsetY: 4, Blur: 6, Spread: -1, Color: black(0.1)},
		{OffsetY: 2, Blur: 4, Spread: -2, Color: black(0.1)},
	}
	if got := th.Shadows["md"]; !reflect.DeepEqual(got, want) {
		t.Errorf("shadow-md = %+v, want %+v", got, want)
	}
	// --shadow-2xs: 0 1px rgb(0 0 0 / 0.05)
	want = []Shadow{{OffsetY: 1, Color: black(0.05)}}
	if got := th.Shadows["2xs"]; !reflect.DeepEqual(got, want) {
		t.Errorf("shadow-2xs = %+v, want %+v", got, want)
	}
	// --inset-shadow-xs: inset 0 1px 1px rgb(0 0 0 / 0.05)
	want = []Shadow{{OffsetY: 1, Blur: 1, Color: black(0.05), Inset: true}}
	if got := th.InsetShadows["xs"]; !reflect.DeepEqual(got, want) {
		t.Errorf("inset-shadow-xs = %+v, want %+v", got, want)
	}
	for k, layers := range th.InsetShadows {
		for _, s := range layers {
			if !s.Inset {
				t.Errorf("inset-shadow-%s has a layer that is not inset", k)
			}
		}
	}
	// --drop-shadow-xl: 0 9px 7px rgb(0 0 0 / 0.1)
	want = []Shadow{{OffsetY: 9, Blur: 7, Color: black(0.1)}}
	if got := th.DropShadows["xl"]; !reflect.DeepEqual(got, want) {
		t.Errorf("drop-shadow-xl = %+v, want %+v", got, want)
	}
	// --text-shadow-sm has three layers.
	if got := th.TextShadows["sm"]; len(got) != 3 || got[2].OffsetY != 2 || got[2].Blur != 2 || got[2].Color.A != 0.075 {
		t.Errorf("text-shadow-sm = %+v", got)
	}
	// Deprecated --shadow-inner: inset 0 2px 4px 0 rgb(0 0 0 / 0.05)
	want = []Shadow{{OffsetY: 2, Blur: 4, Color: black(0.05), Inset: true}}
	if got := th.Shadows["inner"]; !reflect.DeepEqual(got, want) {
		t.Errorf("shadow-inner = %+v, want %+v", got, want)
	}
	// Deprecated --shadow equals shadow-sm.
	if !reflect.DeepEqual(th.Shadows["DEFAULT"], th.Shadows["sm"]) {
		t.Errorf("shadow = %+v, want shadow-sm %+v", th.Shadows["DEFAULT"], th.Shadows["sm"])
	}
}

func TestBlurPerspectiveAspect(t *testing.T) {
	th := DefaultTheme()
	if got := th.Blur["xl"]; got != Px(24) {
		t.Errorf("blur-xl = %v", got)
	}
	if got := th.Blur["DEFAULT"]; got != Px(8) {
		t.Errorf("blur = %v", got)
	}
	if got := th.Perspective["midrange"]; got != Px(800) {
		t.Errorf("perspective-midrange = %v", got)
	}
	if got := th.Aspect["video"]; got != float32(16.0/9.0) {
		t.Errorf("aspect-video = %v", got)
	}
}

func TestEaseAndTransition(t *testing.T) {
	th := DefaultTheme()
	// --ease-in-out: cubic-bezier(0.4, 0, 0.2, 1)
	if got := th.Ease["in-out"]; got != (CubicBezier{0.4, 0, 0.2, 1}) {
		t.Errorf("ease-in-out = %+v", got)
	}
	if got := th.Ease["in"]; got != (CubicBezier{0.4, 0, 1, 1}) {
		t.Errorf("ease-in = %+v", got)
	}
	if got := th.Ease["out"]; got != (CubicBezier{0, 0, 0.2, 1}) {
		t.Errorf("ease-out = %+v", got)
	}
	want := Transition{Duration: 150 * time.Millisecond, Timing: CubicBezier{0.4, 0, 0.2, 1}}
	if th.DefaultTransition != want {
		t.Errorf("DefaultTransition = %+v, want %+v", th.DefaultTransition, want)
	}
	if got := th.Animate["spin"]; got != "spin 1s linear infinite" {
		t.Errorf("animate-spin = %q", got)
	}
}

func TestFonts(t *testing.T) {
	th := DefaultTheme()
	want := map[string][]string{
		"sans":  {"Inter"},
		"mono":  {"JetBrains Mono"},
		"serif": {"serif"},
	}
	if !reflect.DeepEqual(th.Fonts, want) {
		t.Errorf("Fonts = %v, want %v", th.Fonts, want)
	}
	if th.DefaultFont != "sans" || th.DefaultMonoFont != "mono" {
		t.Errorf("default fonts = %q, %q", th.DefaultFont, th.DefaultMonoFont)
	}
}

func TestColors(t *testing.T) {
	th := DefaultTheme()
	if th.Colors["white"] != gfx.White || th.Colors["black"] != gfx.Black {
		t.Errorf("white = %v, black = %v", th.Colors["white"], th.Colors["black"])
	}
	// Chroma 0 colors are exact grays (#fafafa, #737373, #0a0a0a).
	for name, want := range map[string]gfx.Color{
		"neutral-50":  gfx.RGB8(0xfa, 0xfa, 0xfa),
		"neutral-500": gfx.RGB8(0x73, 0x73, 0x73),
		"neutral-950": gfx.RGB8(0x0a, 0x0a, 0x0a),
	} {
		got := th.Colors[name]
		if got.NRGBA8() != want.NRGBA8() || got.A != 1 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	// Palette colors equal the gfx conversion of the CSS source, extended
	// sRGB for the out-of-gamut ones (blue-500, fuchsia-600).
	cases := map[string][3]float32{ // oklch(L% C H)
		"red-500":     {0.637, 0.237, 25.331},
		"blue-500":    {0.623, 0.214, 259.815},
		"fuchsia-50":  {0.977, 0.017, 320.058},
		"fuchsia-600": {0.591, 0.293, 322.896},
		"slate-900":   {0.208, 0.042, 265.755},
	}
	for name, v := range cases {
		want := gfx.OKLCH(v[0], v[1], v[2], 1)
		if got := th.Colors[name]; got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	// red-500 is #fb2c36 in Tailwind's sRGB fallback.
	if got := th.Colors["red-500"].NRGBA8(); got.R != 0xfb || got.G != 0x2c || got.B != 0x36 {
		t.Errorf("red-500 = %v, want #fb2c36", got)
	}
	// 26 families of 11 shades plus black and white.
	if len(th.Colors) != 26*11+2 {
		t.Errorf("len(Colors) = %d, want %d", len(th.Colors), 26*11+2)
	}
	// Colors are opaque extended sRGB: Tailwind's out-of-gamut colors keep
	// components outside [0, 1], but not far outside.
	extended := 0
	for name, c := range th.Colors {
		if !c.IsFinite() || c.A != 1 || min(c.R, c.G, c.B) < -0.5 || max(c.R, c.G, c.B) > 1.5 {
			t.Errorf("%s = %+v is not an opaque, plausible extended sRGB color", name, c)
		}
		if c != c.Clamp() {
			extended++
		}
	}
	if extended == 0 {
		t.Error("no palette color is outside sRGB; the theme should keep extended components")
	}
}

func TestDefaultThemeIsIndependent(t *testing.T) {
	a := DefaultTheme()
	a.Colors["white"] = gfx.Color{R: 1}
	a.Shadows["2xl"][0].Blur = 1
	a.Fonts["sans"][0] = "Comic Sans"
	delete(a.Radius, "3xl")
	b := DefaultTheme()
	if b.Colors["white"] != gfx.White || b.Shadows["2xl"][0].Blur != 50 ||
		b.Fonts["sans"][0] != "Inter" || b.Radius["3xl"] != Rem(1.5) {
		t.Error("modifying one DefaultTheme() result changed another")
	}
}

func TestCloneIsDeep(t *testing.T) {
	a := DefaultTheme()
	b := a.Clone()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Clone is not equal to the original")
	}
	// Every map field must be a distinct map, and every slice element
	// distinct memory. Mutate through reflection so that a field added
	// to Theme without updating Clone is caught.
	av, bv := reflect.ValueOf(a).Elem(), reflect.ValueOf(b).Elem()
	for i := range av.NumField() {
		f := av.Type().Field(i)
		if f.Type.Kind() != reflect.Map {
			continue
		}
		if av.Field(i).Pointer() == bv.Field(i).Pointer() {
			t.Errorf("Clone shares the %s map", f.Name)
		}
		if f.Type.Elem().Kind() == reflect.Slice {
			iter := av.Field(i).MapRange()
			for iter.Next() {
				other := bv.Field(i).MapIndex(iter.Key())
				if iter.Value().Len() > 0 && iter.Value().Pointer() == other.Pointer() {
					t.Errorf("Clone shares a slice of %s[%v]", f.Name, iter.Key())
				}
			}
		}
	}
}

func TestKeys(t *testing.T) {
	th := DefaultTheme()
	for _, ns := range Namespaces() {
		keys := th.Keys(ns)
		if len(keys) == 0 {
			t.Errorf("Keys(%v) is empty", ns)
		}
		if !slices.IsSorted(keys) {
			t.Errorf("Keys(%v) is not sorted: %v", ns, keys)
		}
		if strings.HasPrefix(ns.String(), "Namespace(") {
			t.Errorf("namespace %d has no name", ns)
		}
	}
	if got := th.Keys(NamespaceBreakpoint); !slices.Equal(got, []string{"2xl", "lg", "md", "sm", "xl"}) {
		t.Errorf("Keys(breakpoint) = %v", got)
	}
	if got := th.Keys(NamespaceEase); !slices.Equal(got, []string{"in", "in-out", "out"}) {
		t.Errorf("Keys(ease) = %v", got)
	}
	if !slices.Contains(th.Keys(NamespaceColor), "fuchsia-600") {
		t.Error("Keys(color) lacks fuchsia-600")
	}
	if got := th.Keys(Namespace(200)); got != nil {
		t.Errorf("Keys(unknown) = %v, want nil", got)
	}
	if NamespaceFontWeight.String() != "font-weight" {
		t.Errorf("NamespaceFontWeight = %q", NamespaceFontWeight)
	}
}

func TestLength(t *testing.T) {
	for _, tc := range []struct {
		l    Length
		want float32
		str  string
	}{
		{Px(3), 3, "3px"},
		{Rem(1.5), 24, "1.5rem"},
		{Em(0.5), 7, "0.5em"},
	} {
		if got := tc.l.Resolve(16, 14); got != tc.want {
			t.Errorf("%v.Resolve(16, 14) = %v, want %v", tc.l, got, tc.want)
		}
		if got := tc.l.String(); got != tc.str {
			t.Errorf("String() = %q, want %q", got, tc.str)
		}
	}
	if got := (Length{1, Unit(9)}).Unit.String(); got != "Unit(9)" {
		t.Errorf("unknown unit = %q", got)
	}
	// A custom root font size changes every rem value.
	if got := Rem(2).Resolve(20, 0); got != 40 {
		t.Errorf("Rem(2) at 20px root = %v", got)
	}
}

func TestSpacingSuggestions(t *testing.T) {
	if !slices.Contains(SpacingSuggestions, "px") || !slices.Contains(SpacingSuggestions, "96") {
		t.Errorf("SpacingSuggestions = %v", SpacingSuggestions)
	}
}
