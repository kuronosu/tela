package style

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// bgColor is the background color of a Div with the classes in the context.
func bgColor(t *testing.T, cx *Context, classes string) gfx.Color {
	t.Helper()
	return styleIn(t, cx, KindDiv, nil, classes).BackgroundColor
}

// wpx is the width of s in px, 0 when it is not a length.
func wpx(s *ComputedStyle) float32 {
	v, _ := s.Width.Resolve(0)
	return v
}

// TestThemeVarColors: var(--color-x) resolves like the class bg-x does,
// for the scheme of the element, with an opacity modifier.
func TestThemeVarColors(t *testing.T) {
	th := DefaultTheme()
	tests := []struct {
		class string
		want  func(dark bool) gfx.Color
	}{
		{"bg-[var(--color-primary)]", func(d bool) gfx.Color { return tok("primary", d) }},
		{"bg-(--color-primary)", func(d bool) gfx.Color { return tok("primary", d) }},
		{"bg-(color:--color-border)", func(d bool) gfx.Color { return tok("border", d) }},
		{"bg-[color:var(--color-muted)]", func(d bool) gfx.Color { return tok("muted", d) }},
		{"bg-[var(--color-primary)]/50", func(d bool) gfx.Color { return tok("primary", d).MulAlpha(0.5) }},
		{"bg-[var(--color-red-500)]", func(bool) gfx.Color { return th.Colors["red-500"] }},
		{"bg-[var(--color-red-500)]/25", func(bool) gfx.Color { return th.Colors["red-500"].MulAlpha(0.25) }},
		{"bg-[var(--color-nope,#336699)]", func(bool) gfx.Color { return gfx.RGBA8(0x33, 0x66, 0x99, 255) }},
		{"bg-[var(--color-primary,#336699)]", func(d bool) gfx.Color { return tok("primary", d) }},
		{"dark:bg-[var(--color-primary)]", func(d bool) gfx.Color {
			if d {
				return tok("primary", true)
			}
			return gfx.Transparent
		}},
	}
	for _, tt := range tests {
		for _, dark := range []bool{false, true} {
			cx := lightCx
			if dark {
				cx = darkCx
			}
			got, want := bgColor(t, cx, tt.class), tt.want(dark)
			if !colorNear(got, want, 1e-6) {
				t.Errorf("%s dark=%v: got %v, want %v", tt.class, dark, got, want)
			}
		}
	}
}

// TestThemeVarSameAsClass: a variable and the utility class give the same
// style, whatever the scheme, also under a Scheme override.
func TestThemeVarSameAsClass(t *testing.T) {
	pairs := [][2]string{
		{"bg-[var(--color-card)]", "bg-card"},
		{"border-[var(--color-border)]", "border-border"},
		{"text-[var(--color-muted-foreground)]", "text-muted-foreground"},
	}
	for _, p := range pairs {
		for _, cx := range []*Context{lightCx, darkCx} {
			a := styleIn(t, cx, KindDiv, nil, p[0]+" border")
			b := styleIn(t, cx, KindDiv, nil, p[1]+" border")
			if a.BackgroundColor != b.BackgroundColor || a.Border.Top.Color != b.Border.Top.Color || a.Text.Color != b.Text.Color {
				t.Errorf("%s != %s (dark=%v)", p[0], p[1], cx.Dark)
			}
		}
	}
	// A Scheme override wins over the context.
	in := Input{Kind: KindDiv, Classes: []*Classes{mustCompile(t, "bg-[var(--color-primary)]")}, Scheme: SchemeDark}
	if got := Compute(lightCx, in).BackgroundColor; got != tok("primary", true) {
		t.Errorf("SchemeDark in a light app: %v, want %v", got, tok("primary", true))
	}
	in.Scheme = SchemeLight
	if got := Compute(darkCx, in).BackgroundColor; got != tok("primary", false) {
		t.Errorf("SchemeLight in a dark app: %v, want %v", got, tok("primary", false))
	}
}

// TestThemeVarCustomToken: a token the app adds to the theme is a variable
// too.
func TestThemeVarCustomToken(t *testing.T) {
	th := DefaultTheme()
	light, dark := gfx.RGBA8(0xff, 0xd7, 0x00, 255), gfx.RGBA8(0xb8, 0x86, 0x0b, 255)
	th.Tokens["gold"] = SchemeColor{Light: light, Dark: dark}
	th.Colors["brand"] = gfx.RGBA8(1, 2, 3, 255)
	p := NewParser(th)
	c := p.Compile("bg-[var(--color-gold)] border-[var(--color-brand)]")
	if c.Err != nil {
		t.Fatal(c.Err)
	}
	for _, d := range []bool{false, true} {
		s := Compute(&Context{Theme: th, Dark: d}, Input{Kind: KindDiv, Classes: []*Classes{c}})
		want := light
		if d {
			want = dark
		}
		if s.BackgroundColor != want {
			t.Errorf("dark=%v: gold = %v, want %v", d, s.BackgroundColor, want)
		}
		if s.Border.Top.Color != th.Colors["brand"] {
			t.Errorf("dark=%v: brand = %v", d, s.Border.Top.Color)
		}
	}
}

// TestThemeVarGradient: tokens inside gradient functions, their stops and
// the from-*, via-* and to-* utilities follow the scheme.
func TestThemeVarGradient(t *testing.T) {
	for _, dark := range []bool{false, true} {
		cx := lightCx
		if dark {
			cx = darkCx
		}
		stops := func(classes string) []ColorStop {
			s := styleIn(t, cx, KindDiv, nil, classes)
			if len(s.Backgrounds) != 1 || s.Backgrounds[0].Gradient == nil {
				t.Fatalf("%s: no gradient in %+v", classes, s.Backgrounds)
			}
			return s.Backgrounds[0].Gradient.Stops
		}
		primary := tok("primary", dark)

		g := stops("bg-[conic-gradient(var(--color-primary)_72%,transparent_0)]")
		if len(g) != 2 || g[0].Color != primary || g[0].Pos.Pct != 72 || g[1].Color != gfx.Transparent {
			t.Errorf("dark=%v conic stops = %+v", dark, g)
		}
		g = stops("bg-[linear-gradient(to_right,var(--color-primary),var(--color-accent))]")
		if len(g) != 2 || g[0].Color != primary || g[1].Color != tok("accent", dark) {
			t.Errorf("dark=%v linear stops = %+v", dark, g)
		}
		g = stops("bg-linear-to-r from-[var(--color-primary)] to-[var(--color-red-500)]")
		if len(g) != 2 || g[0].Color != primary || g[1].Color != DefaultTheme().Colors["red-500"] {
			t.Errorf("dark=%v from/to stops = %+v", dark, g)
		}
	}
}

// TestThemeVarLengths: --spacing and --radius in lengths and calc().
func TestThemeVarLengths(t *testing.T) {
	cx := testCx
	tests := []struct {
		class string
		get   func(s *ComputedStyle) float32
		want  float32
	}{
		{"w-[var(--spacing)]", wpx, 4},
		{"w-[calc(var(--spacing)*4)]", wpx, 16},
		{"w-[calc(var(--spacing)_*_10)]", wpx, 40},
		{"w-[calc(var(--spacing)*-1+100px)]", wpx, 96},
		{"w-[min(var(--spacing)*50,100px)]", wpx, 100},
		{"rounded-[var(--radius)]", func(s *ComputedStyle) float32 { return s.Radius.TopLeft.Px }, 8},
		{"rounded-[calc(var(--radius)-2px)]", func(s *ComputedStyle) float32 { return s.Radius.TopLeft.Px }, 6},
		{"rounded-[var(--radius-xl)]", func(s *ComputedStyle) float32 { return s.Radius.TopLeft.Px }, 12},
		{"rounded-[var(--radius-nope,3px)]", func(s *ComputedStyle) float32 { return s.Radius.TopLeft.Px }, 3},
		{"rounded-[var(--radius-nope,var(--radius-sm))]", func(s *ComputedStyle) float32 { return s.Radius.TopLeft.Px }, 4},
	}
	for _, tt := range tests {
		s := styleIn(t, cx, KindDiv, nil, tt.class)
		if got := tt.get(s); !near(got, tt.want, 1e-4) {
			t.Errorf("%s = %v, want %v", tt.class, got, tt.want)
		}
	}

	// The theme's own values: a custom base radius and spacing.
	th2 := DefaultTheme()
	th2.SetRadius(10)
	th2.Spacing = Px(5)
	p := NewParser(th2)
	c := p.Compile("rounded-[var(--radius)] w-[calc(var(--spacing)*3)]")
	if c.Err != nil {
		t.Fatal(c.Err)
	}
	s := Compute(&Context{Theme: th2}, Input{Kind: KindDiv, Classes: []*Classes{c}})
	if s.Radius.TopLeft.Px != 10 || wpx(s) != 15 {
		t.Errorf("custom theme: radius %v, width %v; want 10 and 15", s.Radius.TopLeft.Px, wpx(s))
	}
}

// TestThemeVarShadow: a ring color given as a variable.
func TestThemeVarShadow(t *testing.T) {
	for _, dark := range []bool{false, true} {
		cx := lightCx
		if dark {
			cx = darkCx
		}
		s := styleIn(t, cx, KindDiv, nil, "shadow-[0_0_0_2px_var(--color-ring)]")
		if len(s.BoxShadows) != 1 || s.BoxShadows[0].Spread != 2 || s.BoxShadows[0].Color != tok("ring", dark) {
			t.Errorf("dark=%v: shadow = %+v, want a 2px ring of %v", dark, s.BoxShadows, tok("ring", dark))
		}
	}
}

// TestThemeVarErrors: the variables of an app are rejected with a clear
// message; an unknown name of the theme suggests the closest.
func TestThemeVarErrors(t *testing.T) {
	unsupported := []string{
		"bg-[var(--foo)]", "w-[var(--my-width)]", "bg-[var(--foo,#fff)]", "w-(--my-width)", "w-[calc(var(--gap)*2)]",
		"[clip-path:circle(var(--r))]", "bg-[var(--tw-gradient-from)]",
	}
	for _, class := range unsupported {
		c := Compile(class)
		var u *UnsupportedClassError
		if !errors.As(c.Err, &u) || !strings.Contains(u.Reason, "CSS variables") || !strings.Contains(u.Reason, "var(--color-<token>)") {
			t.Errorf("%s: Err = %v, want an UnsupportedClassError about CSS variables", class, c.Err)
		}
	}

	unknown := []struct{ class, suggestion string }{
		{"bg-[var(--color-primry)]", "bg-[var(--color-primary)]"},
		{"border-[var(--color-bordr)]", "border-[var(--color-border)]"},
		{"bg-(--color-primry)", "bg-(--color-primary)"},
		{"rounded-[var(--radius-xxl)]", "rounded-[var(--radius-2xl)]"},
		{"bg-[conic-gradient(var(--color-primry)_72%,transparent_0)]", "bg-[conic-gradient(var(--color-primary)_72%,transparent_0)]"},
	}
	for _, tt := range unknown {
		c := Compile(tt.class)
		var ie *InvalidClassError
		if !errors.As(c.Err, &ie) {
			t.Errorf("%s: Err = %v, want an InvalidClassError", tt.class, c.Err)
			continue
		}
		if ie.Suggestion != tt.suggestion || !strings.Contains(ie.Reason, "unknown theme") {
			t.Errorf("%s: reason %q, suggestion %q; want suggestion %q", tt.class, ie.Reason, ie.Suggestion, tt.suggestion)
		}
	}
	// No suggestion when nothing is close; still unknown.
	c := Compile("bg-[var(--color-zzzzzzzz)]")
	var ie *InvalidClassError
	if !errors.As(c.Err, &ie) || ie.Suggestion != "" {
		t.Errorf("far name: Err = %v", c.Err)
	}
	// An opacity modifier cannot be a variable.
	if c := Compile("bg-primary/(--x)"); c.Err == nil {
		t.Error("bg-primary/(--x) compiled")
	}
}

// TestThemeVarStaysSymbolic: a class with a theme variable that does not
// depend on the scheme is parsed once; the compiled class is shared by
// both schemes.
func TestThemeVarNoSchemeNoSplit(t *testing.T) {
	c := &pctx{th: DefaultTheme()}
	if _, err := c.substituteVars("calc(var(--spacing)*4)"); err != nil || c.schemeVar {
		t.Errorf("spacing: err %v schemeVar %v", err, c.schemeVar)
	}
	if _, err := c.substituteVars("var(--color-red-500)"); err != nil || c.schemeVar {
		t.Errorf("palette: err %v schemeVar %v", err, c.schemeVar)
	}
	if _, err := c.substituteVars("var(--color-primary)"); err != nil || !c.schemeVar {
		t.Errorf("token: err %v schemeVar %v", err, c.schemeVar)
	}
}

func TestSubstituteVarsText(t *testing.T) {
	c := &pctx{th: DefaultTheme()}
	tests := []struct{ in, want string }{
		{"calc(var(--spacing)*4)", "calc((0.25rem)*4)"},
		{"var(--spacing)", "0.25rem"},
		{"var(--radius-lg)", "0.5rem"},
		{"circle(var(--spacing))", "circle(0.25rem)"},
		{"no variables", "no variables"},
		{"somevar(--x)", "somevar(--x)"},
		{"var(--radius-nope, 4px)", "4px"},
		{"var( --spacing )", "0.25rem"},
	}
	for _, tt := range tests {
		got, err := c.substituteVars(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("substituteVars(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"var(--color-primary", "var(spacing)", "var(--radius-nope)", "var(--other)"} {
		if got, err := c.substituteVars(in); err == nil {
			t.Errorf("substituteVars(%q) = %q, want an error", in, got)
		}
	}
}

// TestThemeVarRobust: odd input never panics.
func TestThemeVarRobust(t *testing.T) {
	for _, class := range []string{
		"bg-[var()]", "bg-[var(,)]", "bg-[var(--)]", "bg-[var(--color-)]", "bg-[var(--color-primary,)]",
		"bg-[var(--color-primary]", "bg-[var(var(--color-primary))]", "w-[calc(var(--spacing)]", "bg-()", "bg-(--)",
		"bg-[var(--color-a,var(--color-b,var(--color-c)))]", "rounded-[var(--radius-)]",
	} {
		_ = Compile(class)
	}
}
