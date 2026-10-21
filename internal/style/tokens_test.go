package style

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

var (
	lightCx = &Context{Viewport: gfx.Size{W: 1280, H: 800}}
	darkCx  = &Context{Viewport: gfx.Size{W: 1280, H: 800}, Dark: true}
)

func tok(name string, dark bool) gfx.Color {
	c, ok := DefaultTheme().TokenColor(name, dark)
	if !ok {
		panic("no token " + name)
	}
	return c
}

func TestDefaultTokens(t *testing.T) {
	th := DefaultTheme()
	if got := th.TokenNames(); !slices.Equal(got, slices.Sorted(slices.Values(DefaultTokenNames))) {
		t.Fatalf("TokenNames = %v, want %v", got, DefaultTokenNames)
	}
	if err := th.CheckTokens(); err != nil {
		t.Fatal(err)
	}
	// oklch(1 0 0) is white, oklch(0.145 0 0) is #0a0a0a (shadcn/ui's
	// neutral-950 equivalent), within float32 rounding.
	if c := tok("background", false); !colorNear(c, gfx.White, 1e-5) {
		t.Errorf("background light = %v, want white", c)
	}
	if c := tok("background", true); !colorNear(c, gfx.Color{R: 0.0393, G: 0.0393, B: 0.0393, A: 1}, 2e-3) {
		t.Errorf("background dark = %v, want about #0a0a0a", c)
	}
	if c := tok("border", true); !near(c.A, 0.1, 1e-6) || !near(c.R, 1, 1e-5) {
		t.Errorf("border dark = %v, want white at 10%%", c)
	}
	// The default theme keeps Tailwind's radius scale.
	if th.Radius["lg"] != Rem(0.5) || th.Radius["sm"] != Rem(0.25) {
		t.Errorf("radius scale changed: lg %v sm %v", th.Radius["lg"], th.Radius["sm"])
	}
	// Clone copies the tokens.
	c := th.Clone()
	c.Tokens["primary"] = SchemeColor{}
	if th.Tokens["primary"] == (SchemeColor{}) {
		t.Error("Clone shares the token map")
	}
}

// TestTokenUtilities: every color utility takes a token, resolved for the
// scheme, with the opacity modifier.
func TestTokenUtilities(t *testing.T) {
	type check func(s *ComputedStyle) gfx.Color
	tests := []struct {
		class string
		token string
		alpha float32
		get   check
	}{
		{"bg-primary", "primary", 1, func(s *ComputedStyle) gfx.Color { return s.BackgroundColor }},
		{"bg-primary/50", "primary", 0.5, func(s *ComputedStyle) gfx.Color { return s.BackgroundColor }},
		{"bg-primary/[0.25]", "primary", 0.25, func(s *ComputedStyle) gfx.Color { return s.BackgroundColor }},
		{"text-muted-foreground", "muted-foreground", 1, func(s *ComputedStyle) gfx.Color { return s.Text.Color }},
		{"border border-border", "border", 1, func(s *ComputedStyle) gfx.Color { return s.Border.Top.Color }},
		{"border-x-2 border-x-input", "input", 1, func(s *ComputedStyle) gfx.Color { return s.Border.Left.Color }},
		{"border-t border-t-destructive/40", "destructive", 0.4, func(s *ComputedStyle) gfx.Color { return s.Border.Top.Color }},
		{"outline-2 outline-ring/50", "ring", 0.5, func(s *ComputedStyle) gfx.Color { return s.Outline.Color }},
		{"ring-2 ring-ring", "ring", 1, func(s *ComputedStyle) gfx.Color { return s.BoxShadows[len(s.BoxShadows)-1].Color }},
		{"inset-ring-2 inset-ring-accent", "accent", 1, func(s *ComputedStyle) gfx.Color { return s.BoxShadows[0].Color }},
		{"ring-2 ring-offset-2 ring-offset-background", "background", 1, func(s *ComputedStyle) gfx.Color { return s.BoxShadows[0].Color }},
		{"shadow-lg shadow-primary/20", "primary", 0.2, func(s *ComputedStyle) gfx.Color { return s.BoxShadows[0].Color }},
		{"inset-shadow-sm inset-shadow-card", "card", 1, func(s *ComputedStyle) gfx.Color { return s.BoxShadows[0].Color }},
		{"drop-shadow-md drop-shadow-chart-1", "chart-1", 1, func(s *ComputedStyle) gfx.Color { return s.Filter.DropShadow[0].Color }},
		{"text-shadow-md text-shadow-popover", "popover", 1, func(s *ComputedStyle) gfx.Color { return s.Text.Shadow[0].Color }},
		{"underline decoration-secondary-foreground", "secondary-foreground", 1, func(s *ComputedStyle) gfx.Color { return s.Text.Decoration.Color }},
		{"caret-primary", "primary", 1, func(s *ComputedStyle) gfx.Color { return s.Caret.Color }},
		{"bg-linear-to-r from-chart-1 to-chart-2", "chart-1", 1, func(s *ComputedStyle) gfx.Color { return s.Backgrounds[0].Gradient.Stops[0].Color }},
		{"bg-linear-to-r from-chart-1 via-chart-3/60 to-chart-2", "chart-3", 0.6, func(s *ComputedStyle) gfx.Color { return s.Backgrounds[0].Gradient.Stops[1].Color }},
		{"bg-linear-to-r from-chart-1 to-chart-2", "chart-2", 1, func(s *ComputedStyle) gfx.Color { return s.Backgrounds[0].Gradient.Stops[1].Color }},
	}
	for _, tt := range tests {
		t.Run(tt.class, func(t *testing.T) {
			for _, cx := range []*Context{lightCx, darkCx} {
				s := styleIn(t, cx, KindDiv, nil, tt.class)
				want := tok(tt.token, cx.Dark).MulAlpha(tt.alpha)
				if got := tt.get(s); !colorNear(got, want, 1e-6) {
					t.Errorf("dark=%v: got %v, want %v", cx.Dark, got, want)
				}
			}
		})
	}
}

// TestTokenPseudoAndVariants: tokens under variants, in pseudo-elements
// and in a variant that tests the scheme.
func TestTokenPseudoAndVariants(t *testing.T) {
	cl := []*Classes{mustCompile(t, "bg-card hover:bg-accent dark:hover:bg-primary/80 placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground")}
	for _, dark := range []bool{false, true} {
		cx := &Context{Dark: dark}
		s := Compute(cx, Input{Kind: KindInput, Classes: cl})
		if want := tok("card", dark); s.BackgroundColor != want {
			t.Errorf("dark=%v rest: %v, want %v", dark, s.BackgroundColor, want)
		}
		h := Compute(cx, Input{Kind: KindInput, Classes: cl, State: StateHover})
		want := tok("accent", dark)
		if dark {
			want = tok("primary", true).MulAlpha(0.8)
		}
		if !colorNear(h.BackgroundColor, want, 1e-6) {
			t.Errorf("dark=%v hover: %v, want %v", dark, h.BackgroundColor, want)
		}
		if s.Placeholder == nil || s.Placeholder.Color != tok("muted-foreground", dark) {
			t.Errorf("dark=%v placeholder: %+v", dark, s.Placeholder)
		}
		if s.Selection == nil || s.Selection.Background != tok("primary", dark) || s.Selection.Color != tok("primary-foreground", dark) {
			t.Errorf("dark=%v selection: %+v", dark, s.Selection)
		}
	}
}

// TestSchemeOverride: Input.Scheme overrides Context.Dark for the element
// and its descendants, for dark: and for tokens; a nested override wins.
func TestSchemeOverride(t *testing.T) {
	cls := "bg-background dark:text-white"
	root := styleIn(t, lightCx, KindDiv, nil, cls)
	if root.Scheme != SchemeInherit || root.BackgroundColor != tok("background", false) || root.Text.Color != gfx.Black {
		t.Fatalf("light root: scheme %v bg %v text %v", root.Scheme, root.BackgroundColor, root.Text.Color)
	}
	compute := func(cx *Context, parent *ComputedStyle, s Scheme) *ComputedStyle {
		return Compute(cx, Input{Kind: KindDiv, Parent: parent, Scheme: s, Classes: []*Classes{mustCompile(t, cls)}})
	}
	panel := compute(lightCx, root, SchemeDark)
	if panel.Scheme != SchemeDark || panel.BackgroundColor != tok("background", true) || panel.Text.Color != gfx.White {
		t.Errorf("dark panel: scheme %v bg %v text %v", panel.Scheme, panel.BackgroundColor, panel.Text.Color)
	}
	child := compute(lightCx, panel, SchemeInherit)
	if child.Scheme != SchemeDark || child.BackgroundColor != tok("background", true) {
		t.Errorf("child of the dark panel: scheme %v bg %v", child.Scheme, child.BackgroundColor)
	}
	nested := compute(lightCx, child, SchemeLight)
	if nested.Scheme != SchemeLight || nested.BackgroundColor != tok("background", false) || nested.Text.Color != gfx.White {
		// The text color is inherited from the dark panel (no dark: here).
		t.Errorf("nested light: scheme %v bg %v text %v", nested.Scheme, nested.BackgroundColor, nested.Text.Color)
	}
	// In a dark app, a light override.
	lp := compute(darkCx, styleIn(t, darkCx, KindDiv, nil, cls), SchemeLight)
	if lp.BackgroundColor != tok("background", false) || lp.IsDark(darkCx) {
		t.Errorf("light panel in a dark app: bg %v dark %v", lp.BackgroundColor, lp.IsDark(darkCx))
	}
}

func TestTokenErrors(t *testing.T) {
	c := Compile("bg-primry text-muted-foregrond")
	var errs ClassErrors
	if !errors.As(c.Err, &errs) || len(errs) != 2 {
		t.Fatalf("errors = %v", c.Err)
	}
	for i, want := range []string{"bg-primary", "text-muted-foreground"} {
		var u *UnknownClassError
		if !errors.As(errs[i], &u) || u.Suggestion != want {
			t.Errorf("error %d = %v, want a suggestion of %q", i, errs[i], want)
		}
	}
	// Tokens cannot go into arbitrary values (resolved at parse time).
	for _, cls := range []string{"shadow-[0_0_4px_primary]", "bg-[linear-gradient(primary,red-500)]"} {
		c := Compile(cls)
		if c.Err == nil || !strings.Contains(c.Err.Error(), "token") {
			t.Errorf("%s: err = %v, want a message about tokens", cls, c.Err)
		}
	}
	// Palette names still work there.
	mustCompile(t, "shadow-[0_0_4px_red-500]")
	// fill-, stroke-, divide- and accent- stay unsupported with a token.
	for _, cls := range []string{"fill-primary", "stroke-primary", "divide-border", "accent-primary"} {
		var un *UnsupportedClassError
		if c := Compile(cls); !errors.As(c.Err, &un) {
			t.Errorf("%s: err = %v, want unsupported", cls, c.Err)
		}
	}
}

func TestCheckTokens(t *testing.T) {
	th := DefaultTheme()
	th.Tokens["brand-2"] = SchemeColor{}
	if err := th.CheckTokens(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"Brand", "red-500", "white", "current", "-x", "a b", "", "x-"} {
		th := DefaultTheme()
		th.Tokens[bad] = SchemeColor{}
		if err := th.CheckTokens(); err == nil {
			t.Errorf("CheckTokens accepts %q", bad)
		}
	}
}

func TestSetRadius(t *testing.T) {
	th := DefaultTheme()
	before := th.Radius["xs"]
	th.SetRadius(10)
	want := map[string]float32{"sm": 6, "md": 8, "lg": 10, "xl": 14, "2xl": 18, "3xl": 22, "4xl": 26}
	for k, v := range want {
		if got := th.Radius[k]; got.Unit != UnitPx || !near(got.Value, v, 1e-5) {
			t.Errorf("rounded-%s = %v, want %gpx", k, got, v)
		}
	}
	if th.Radius["xs"] != before || th.Radius["DEFAULT"] != Rem(0.25) {
		t.Errorf("xs or DEFAULT changed: %v %v", th.Radius["xs"], th.Radius["DEFAULT"])
	}
	p := NewParser(th)
	s := Compute(&Context{Theme: p.Theme()}, Input{Kind: KindDiv, Classes: []*Classes{p.Compile("rounded-md")}})
	if s.Radius.TopLeft.Px != 8 {
		t.Errorf("rounded-md with radius 10 = %v", s.Radius.TopLeft)
	}
	th2 := DefaultTheme()
	th2.SetRadius(0)
	if th2.Radius["lg"] != Rem(0.5) {
		t.Error("SetRadius(0) changed the scale")
	}
}

func aliasParser(t *testing.T, aliases map[string]string) *Parser {
	t.Helper()
	p, err := NewParserAliases(DefaultTheme(), aliases)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAliases(t *testing.T) {
	p := aliasParser(t, map[string]string{
		"text-muted": "text-sm text-muted-foreground",
		"card-title": "text-lg font-semibold dark:text-white",
		"card":       "card-title rounded-xl bg-card",
		"btn":        "group px-4",
	})
	compute := func(cls string, dark bool, state StateFlags) *ComputedStyle {
		c := p.Compile(cls)
		if c.Err != nil {
			t.Fatalf("%q: %v", cls, c.Err)
		}
		return Compute(&Context{Theme: p.Theme(), Dark: dark}, Input{Kind: KindDiv, Classes: []*Classes{c}, State: state})
	}
	// An alias wins over the utility of the same name (text-muted is also
	// the muted token as a text color).
	s := compute("text-muted", false, 0)
	if s.Text.Color != tok("muted-foreground", false) || s.Text.FontSize != 14 {
		t.Errorf("text-muted: color %v size %v", s.Text.Color, s.Text.FontSize)
	}
	// It expands where it appears: later classes win, earlier ones lose.
	if s := compute("text-muted text-red-500", false, 0); s.Text.Color != DefaultTheme().Colors["red-500"] {
		t.Errorf("a later class lost to the alias: %v", s.Text.Color)
	}
	if s := compute("text-xl text-muted", false, 0); s.Text.FontSize != 14 {
		t.Errorf("an earlier class won over the alias: %v", s.Text.FontSize)
	}
	// Variants apply to every class of the expansion, nested aliases too.
	rest := compute("hover:card", true, 0)
	if rest.Text.FontSize != 16 || rest.BackgroundColor.A != 0 {
		t.Errorf("hover:card at rest: size %v bg %v", rest.Text.FontSize, rest.BackgroundColor)
	}
	hov := compute("hover:card", true, StateHover)
	if hov.Text.FontSize != 18 || hov.Text.FontWeight != 600 || hov.Text.Color != gfx.White || hov.BackgroundColor != tok("card", true) {
		t.Errorf("hover:card hovered: %+v bg %v", hov.Text, hov.BackgroundColor)
	}
	if lh := compute("hover:card", false, StateHover); lh.Text.Color == gfx.White {
		t.Error("dark:text-white of the alias applied in the light scheme")
	}
	// The rules name the expanded classes.
	var names []string
	for _, r := range p.Compile("hover:card-title").Rules {
		names = append(names, r.Class)
	}
	if !slices.Contains(names, "hover:dark:text-white") || !slices.Contains(names, "hover:text-lg") {
		t.Errorf("rule classes = %v", names)
	}
	// The group marker of an alias without variants.
	if c := p.Compile("btn"); !c.Group {
		t.Error("btn does not mark a group")
	}
	// Aliases are in the vocabulary.
	if got := p.Suggest("text-mutd"); got != "text-muted" {
		t.Errorf("Suggest(text-mutd) = %q", got)
	}
}

func TestAliasErrors(t *testing.T) {
	p := aliasParser(t, map[string]string{
		"a":   "b p-2",
		"b":   "c",
		"c":   "a",
		"me":  "me",
		"bad": "text-mutd p-4",
	})
	c := p.Compile("a")
	var ie *InvalidClassError
	if !errors.As(c.Err, &ie) || !strings.Contains(ie.Reason, "a → b → c → a") {
		t.Fatalf("cycle error = %v", c.Err)
	}
	var ae *AliasError
	if !errors.As(c.Err, &ae) || ae.Alias != "a" {
		t.Errorf("cycle error not attributed to the alias: %v", c.Err)
	}
	if len(c.Rules) != 1 { // p-2 still applies
		t.Errorf("rules = %v", c.Rules)
	}
	if c := p.Compile("me"); c.Err == nil || !strings.Contains(c.Err.Error(), "me → me") {
		t.Errorf("self cycle = %v", c.Err)
	}
	c = p.Compile("bad")
	var u *UnknownClassError
	if !errors.As(c.Err, &u) || u.Class != "text-mutd" || !strings.Contains(c.Err.Error(), `in the class alias "bad"`) {
		t.Errorf("unknown class in an alias = %v", c.Err)
	}
	// An unknown variant on an alias is reported once.
	c = p.Compile("hovr:bad")
	var errs ClassErrors
	if !errors.As(c.Err, &errs) || len(errs) != 1 {
		t.Errorf("hovr:bad errors = %v", c.Err)
	}
	for _, bad := range []map[string]string{{"": "p-2"}, {"hover:x": "p-2"}, {"a b": "p-2"}, {"-x": "p-2"}, {"x[1]": "p-2"}} {
		if _, err := NewParserAliases(DefaultTheme(), bad); err == nil {
			t.Errorf("NewParserAliases accepts %v", bad)
		}
	}
}
