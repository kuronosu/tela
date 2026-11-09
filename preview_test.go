package tela_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// session is the state a screen reads from context.
type session struct{ User string }

var sessionCtx = tela.CreateContext(session{User: "guest"})

// inbox is a screen with local state (the open tab, seeded by a field) and
// state from a context (the user, seeded by a Wrap).
type inbox struct{ InitialTab string }

func (m inbox) Build() tela.Node {
	tab, _ := tela.UseState(m.InitialTab)
	s := tela.UseContext(sessionCtx)
	return Div(Textf("%s sees the %s tab", s.User, tab))
}

var inboxPreviews = tela.Previews{
	"open":   {Root: inbox{InitialTab: "open"}},
	"closed": {Root: inbox{InitialTab: "closed"}},
	"mocked": {
		Root: inbox{InitialTab: "open"},
		Wrap: func(n tela.Node) tela.Node { return sessionCtx.Provider(session{User: "Ana"}, n) },
	},
}

func ExamplePreview() {
	// A Preview is a Component: mount it as `tela snapshot` does.
	for _, name := range inboxPreviews.Names() {
		yaml, _ := telatest.AriaSnapshot(inboxPreviews[name])
		fmt.Printf("%s: %s\n", name, strings.TrimPrefix(yaml, "- text: "))
	}
	// Output:
	// closed: guest sees the closed tab
	// mocked: Ana sees the open tab
	// open: guest sees the open tab
}

// A Root may be a node, such as tela.C(fn), as well as a struct component.
func TestPreviewRootKinds(t *testing.T) {
	fn := func() tela.Node {
		n, _ := tela.UseState(4)
		return P(Textf("n=%d", n))
	}
	for name, p := range map[string]tela.Preview{
		"C":      {Root: tela.C(fn)},
		"CP":     {Root: tela.CP(func(n int) tela.Node { return P(Textf("n=%d", n)) }, 4)},
		"struct": {Root: inbox{InitialTab: "t"}},
		"node":   {Root: P(Text("n=4"))},
		"wrapped": {Root: tela.C(fn), Wrap: func(n tela.Node) tela.Node {
			return Div(n)
		}},
	} {
		yaml, err := telatest.AriaSnapshot(p)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if name != "struct" && !strings.Contains(yaml, "n=4") {
			t.Errorf("%s: snapshot %q lacks the text", name, yaml)
		}
	}
}

// Wrap receives the root as a node and its result is what is mounted.
func TestPreviewWrap(t *testing.T) {
	calls := 0
	p := tela.Preview{
		Root: inbox{InitialTab: "x"},
		Wrap: func(n tela.Node) tela.Node {
			calls++
			return Div(Text("wrapped"), n)
		},
	}
	yaml, err := telatest.AriaSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Error("Wrap was not called")
	}
	if !strings.Contains(yaml, "wrapped") || !strings.Contains(yaml, "guest sees the x tab") {
		t.Errorf("snapshot:\n%s", yaml)
	}
}

// swatches is a box in the theme's brand color and one that the dark:
// variant colors.
type swatches struct{}

func (swatches) Build() tela.Node {
	return Div(Class("flex"),
		Div(TestID("token"), Class("size-8 bg-brand")),
		Div(TestID("scheme"), Class("size-8 bg-rose-500 dark:bg-emerald-500")))
}

// TestPreviewThemeAndScheme: a Preview's Theme and ColorScheme (ADR-250)
// apply where telatest mounts it, after the options, as `tela snapshot`
// applies them after the package's App; a preview without them keeps the
// options'.
func TestPreviewThemeAndScheme(t *testing.T) {
	brand := tela.Theme{Colors: map[string]tela.SchemeColor{"brand": {Light: tela.Hex("#ff0000"), Dark: tela.Hex("#0000ff")}}}
	other := &tela.Theme{Colors: map[string]tela.SchemeColor{"brand": {Light: tela.Hex("#00ff00"), Dark: tela.Hex("#ffff00")}}}
	app := tela.App{ColorScheme: tela.Dark, Theme: &brand}
	red, blue, green := color.RGBA{0xff, 0, 0, 0xff}, color.RGBA{0, 0, 0xff, 0xff}, color.RGBA{0, 0xff, 0, 0xff}
	rose, emerald := color.RGBA{0xff, 0x20, 0x56, 0xff}, color.RGBA{0x00, 0xbc, 0x7d, 0xff}
	for _, c := range []struct {
		name         string
		p            tela.Preview
		token, boxes color.RGBA
	}{
		{"none", tela.Preview{Root: swatches{}}, blue, emerald},
		{"light", tela.Preview{Root: swatches{}, ColorScheme: tela.Light}, red, rose},
		{"theme", tela.Preview{Root: swatches{}, Theme: other, ColorScheme: tela.Light}, green, rose},
	} {
		// The App's settings as options, as the CLI gives them.
		a := telatest.Mount(t, c.p, telatest.FromApp(app), telatest.Size(100, 40))
		at := func(id string) color.RGBA {
			b := a.Bounds(telatest.ByTestID(id))
			return a.PixelAt(b.X+b.Width/2, b.Y+b.Height/2)
		}
		if got := at("token"); got != c.token {
			t.Errorf("%s: brand %v, want %v", c.name, got, c.token)
		}
		if got := at("scheme"); got != c.boxes {
			t.Errorf("%s: scheme box %v, want %v", c.name, got, c.boxes)
		}
		// The headless entry points too.
		img, err := telatest.Render(c.p, telatest.FromApp(app), telatest.Size(100, 40))
		if err != nil {
			t.Fatal(err)
		}
		if got := img.RGBAAt(48, 16); got != c.boxes {
			t.Errorf("%s: Render: scheme box %v, want %v", c.name, got, c.boxes)
		}
	}
	// The preview wins over an option given after the App's.
	a := telatest.Mount(t, tela.Preview{Root: swatches{}, ColorScheme: tela.Light, Theme: other}, telatest.ColorScheme(tela.Dark), telatest.Size(100, 40))
	b := a.Bounds(telatest.ByTestID("scheme"))
	if got := a.PixelAt(b.X+b.Width/2, b.Y+b.Height/2); got != rose {
		t.Errorf("an explicit option: scheme box %v, want the preview's light rose", got)
	}
}

// A preview without a Root is a mistake with a fix.
func TestPreviewNeedsRoot(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "needs a Root") || !strings.Contains(msg, "tela.C(Settings)") {
			t.Errorf("panic message %q", msg)
		}
	}()
	tela.Preview{}.Build()
}
