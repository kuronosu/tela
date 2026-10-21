package core

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

func token(name string, dark bool) gfx.Color {
	c, _ := style.DefaultTheme().TokenColor(name, dark)
	return c
}

func bgOf(tr *Tree, k string) gfx.Color { return keyed(tr, k).box.cs.BackgroundColor }

// TestSchemeScoping: a Scheme prop gives its subtree a scheme of its own
// for tokens and dark:, a nested one wins, the app's scheme no longer
// reaches inside, and a change of the prop restyles the subtree.
func TestSchemeScoping(t *testing.T) {
	var setPanel Setter[ColorScheme]
	app := func() Node {
		scheme, set := UseState(ColorSchemeDark)
		setPanel = set
		return div(Key("root"), Class("bg-background"),
			div(Key("panel"), SchemeProp(scheme), Class("bg-card dark:text-white"),
				div(Key("inner"), Class("bg-background border border-border"),
					div(Key("light"), SchemeProp(ColorSchemeLight), Class("bg-card dark:bg-red-500")),
				),
			),
		)
	}
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 200, H: 200})
	tr.Mount(Component(app))
	tr.Frame()
	check := func(when, k string, want gfx.Color) {
		t.Helper()
		if got := bgOf(tr, k); got != want {
			t.Errorf("%s: %s background %v, want %v", when, k, got, want)
		}
	}
	check("light app", "root", token("background", false))
	check("light app", "panel", token("card", true))
	check("light app", "inner", token("background", true))
	check("light app", "light", token("card", false))
	if c := keyed(tr, "inner").box.cs.Border.Top.Color; c != token("border", true) {
		t.Errorf("inner border %v, want the dark border token", c)
	}
	if c := keyed(tr, "inner").box.cs.Text.Color; c != gfx.White {
		t.Errorf("dark:text-white of the panel not inherited: %v", c)
	}

	tr.SetColorScheme(ColorSchemeDark)
	tr.Frame()
	check("dark app", "root", token("background", true))
	check("dark app", "panel", token("card", true))
	check("dark app", "light", token("card", false))

	tr.SetColorScheme(ColorSchemeLight)
	setPanel.Set(ColorSchemeLight)
	tr.Frame()
	check("panel light", "panel", token("card", false))
	check("panel light", "inner", token("background", false))
	if c := keyed(tr, "inner").box.cs.Text.Color; c != gfx.Black {
		t.Errorf("panel light: inner text %v, want black", c)
	}

	// System removes the override: the subtree follows the app.
	setPanel.Set(ColorSchemeSystem)
	tr.SetColorScheme(ColorSchemeDark)
	tr.Frame()
	check("panel follows the dark app", "inner", token("background", true))
}

// TestSchemeReaders: UseColorScheme and UseTheme read the scheme at their
// position; the app's scheme rebuilds the readers it reaches, a Scheme
// prop change the readers below it, and nothing else builds.
func TestSchemeReaders(t *testing.T) {
	type read struct {
		scheme ColorScheme
		bg     gfx.Color
	}
	reads := map[string][]read{}
	reader := func(name string) func() Node {
		return func() Node {
			s := UseColorScheme()
			th := UseTheme()
			reads[name] = append(reads[name], read{s, th.Color("background")})
			return div()
		}
	}
	outer := func(int) Node { return reader("outer")() }
	inner := func(int) Node { return reader("inner")() }
	var setScheme Setter[ColorScheme]
	app := func() Node {
		s, set := UseState(ColorSchemeDark)
		setScheme = set
		return div(ComponentProps(outer, 0), div(SchemeProp(s), div(ComponentProps(inner, 0))))
	}
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	tr.Mount(Component(app))
	tr.Frame()
	last := func(name string) read { r := reads[name]; return r[len(r)-1] }
	expect := func(when, name string, builds int, scheme ColorScheme) {
		t.Helper()
		if len(reads[name]) != builds {
			t.Errorf("%s: %s built %d times, want %d", when, name, len(reads[name]), builds)
		}
		r := last(name)
		if r.scheme != scheme || r.bg != token("background", scheme == ColorSchemeDark) {
			t.Errorf("%s: %s read %v %v, want %v", when, name, r.scheme, r.bg, scheme)
		}
	}
	expect("mount", "outer", 1, ColorSchemeLight)
	expect("mount", "inner", 1, ColorSchemeDark)

	tr.SetColorScheme(ColorSchemeDark)
	tr.Frame()
	expect("app dark", "outer", 2, ColorSchemeDark)
	expect("app dark", "inner", 1, ColorSchemeDark) // scoped: not rebuilt

	setScheme.Set(ColorSchemeLight)
	tr.Frame()
	expect("panel light", "inner", 2, ColorSchemeLight)
	setScheme.Set(ColorSchemeSystem)
	tr.Frame()
	expect("panel removed", "inner", 3, ColorSchemeDark)
	tr.SetColorScheme(ColorSchemeLight)
	tr.Frame()
	expect("unscoped again", "inner", 4, ColorSchemeLight)
	expect("unscoped again", "outer", 3, ColorSchemeLight)
}

// TestThemeConfig: a Theme's colors merge over the default tokens, its
// radius sets the scale and its fonts the families; a theme swap
// recompiles the classes and rebuilds UseTheme readers.
func TestThemeConfig(t *testing.T) {
	brand := SchemeColor{Light: gfx.RGB8(124, 58, 237), Dark: gfx.RGB8(167, 139, 250)}
	var got []gfx.Color
	reader := func() Node {
		th := UseTheme()
		got = append(got, th.Color("brand"))
		if th.Radius() != 10 && len(got) == 1 {
			t.Errorf("Radius() = %v, want 10", th.Radius())
		}
		return div(Key("b"), Class("bg-brand/50 text-primary rounded-lg font-mono"))
	}
	th := &Theme{Colors: map[string]SchemeColor{"brand": brand}, Radius: 10, FontMono: "Fira Code"}
	tr := NewTree(Config{Fonts: testFonts, Theme: th, ClassAliases: map[string]string{"x": "p-1"}})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	tr.Mount(Component(reader))
	tr.Frame()
	cs := keyed(tr, "b").box.cs
	if cs.BackgroundColor != brand.Light.MulAlpha(0.5) || cs.Text.Color != token("primary", false) {
		t.Errorf("bg %v text %v", cs.BackgroundColor, cs.Text.Color)
	}
	if cs.Radius.TopLeft.Px != 10 {
		t.Errorf("rounded-lg = %v, want 10", cs.Radius.TopLeft)
	}
	if !slices.Equal(cs.Text.FontFamily, []string{"Fira Code", "JetBrains Mono"}) {
		t.Errorf("font-mono = %v", cs.Text.FontFamily)
	}
	other := SchemeColor{Light: gfx.RGB8(1, 2, 3), Dark: gfx.RGB8(4, 5, 6)}
	tr.SetTheme(Theme{Colors: map[string]SchemeColor{"brand": other}})
	tr.Frame()
	cs = keyed(tr, "b").box.cs
	if cs.BackgroundColor != other.Light.MulAlpha(0.5) {
		t.Errorf("after SetTheme: bg %v", cs.BackgroundColor)
	}
	if cs.Radius.TopLeft.Px != 8 {
		t.Errorf("after SetTheme without a radius: rounded-lg %v, want Tailwind's 8", cs.Radius.TopLeft)
	}
	if len(got) != 2 || got[1] != other.Light {
		t.Errorf("UseTheme reads %v", got)
	}
	if tr.parser.Aliases()["x"] != "p-1" {
		t.Error("SetTheme dropped the class aliases")
	}
}

// TestThemeDevReports: an unknown token is reported with the closest one,
// an alias cycle and an error inside an alias name the alias, and
// ThemeValues.Color panics in dev mode.
func TestThemeDevReports(t *testing.T) {
	var logs []string
	app := func() Node {
		return div(div(Class("bg-primry")), div(Class("loop")), div(Class("hover:bad")))
	}
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { logs = append(logs, m) },
		ClassAliases: map[string]string{"loop": "a", "a": "loop", "bad": "text-mutd"}})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	tr.Mount(Component(app))
	tr.Frame()
	all := strings.Join(logs, "\n")
	for _, want := range []string{`unknown class "bg-primry"; did you mean "bg-primary"?`,
		`class aliases form a cycle: loop → a → loop`,
		`unknown class "hover:text-mutd"; did you mean "hover:text-muted"? (in the class alias "bad")`} {
		if !strings.Contains(all, want) {
			t.Errorf("no %q in the log:\n%s", want, all)
		}
	}

	panicked := ""
	reader := func() Node {
		defer func() {
			if r := recover(); r != nil {
				panicked, _ = r.(string)
			}
		}()
		UseTheme().Color("primry")
		return div()
	}
	tr2 := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(string) {}})
	tr2.Mount(Component(reader))
	tr2.Frame()
	if !strings.Contains(panicked, `did you mean "primary"?`) {
		t.Errorf("Color panic = %q", panicked)
	}
}

func TestThemeConfigPanics(t *testing.T) {
	for name, cfg := range map[string]Config{
		"palette name": {Theme: &Theme{Colors: map[string]SchemeColor{"red-500": {}}}},
		"upper case":   {Theme: &Theme{Colors: map[string]SchemeColor{"Brand": {}}}},
		"alias key":    {ClassAliases: map[string]string{"hover:x": "p-1"}},
	} {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s: NewTree did not panic", name)
				}
			}()
			NewTree(cfg)
		}()
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Scheme") {
			t.Errorf("Scheme(7) panic = %v", r)
		}
	}()
	SchemeProp(7)
}

// TestSchemeChangeTransitionsTokens: a scheme change moves token colors
// through transition-colors, and a Scheme prop change does too.
func TestSchemeChangeTransitionsTokens(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("size-10 bg-background text-foreground transition-colors duration-200 ease-linear"))), 100, 100)
	x.settle()
	if x.bg("b") != token("background", false) {
		t.Fatalf("light: %v", x.bg("b"))
	}
	x.b.SetSettings(platform.SystemSettings{DarkMode: true})
	x.b.Drain(x.h)
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) < 2 {
		t.Fatalf("want transitions of the background and the text color: %+v", ba)
	}
	x.advanceTo(ba.trans[0].Start + 100*time.Millisecond)
	mid := x.bg("b")
	if mid == token("background", false) || mid == token("background", true) || mid.R < 0.3 || mid.R > 0.7 {
		t.Errorf("half way: %v", mid)
	}
	x.settle()
	if x.bg("b") != token("background", true) {
		t.Errorf("dark: %v", x.bg("b"))
	}
}

func TestSchemePropTransitions(t *testing.T) {
	var set Setter[ColorScheme]
	app := func() Node {
		s, st := UseState(ColorSchemeSystem)
		set = st
		return div(SchemeProp(s), div(Key("b"), Class("size-10 bg-card transition-colors duration-200 ease-linear")))
	}
	x := newInput(t, Component(app), 100, 100)
	x.settle()
	set.Set(ColorSchemeDark)
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no transition after the Scheme prop changed: %+v", ba)
	}
	x.settle()
	if x.bg("b") != token("card", true) {
		t.Errorf("end: %v", x.bg("b"))
	}
}
