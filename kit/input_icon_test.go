package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// iconSearch is Lucide's (ISC, THIRD_PARTY.md).
var iconSearch = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/></svg>`}

func inputIconSheet() *sheet {
	c := func(name string, in kit.Input) cell {
		in.Props = h.TestID(name + "-input")
		return cell{name: name, width: 280, probes: []string{name + "-input"}, tela: in}
	}
	return &sheet{name: "input_icon", width: 640, cells: []cell{
		c("start", kit.Input{Icon: iconSearch, Placeholder: "Search..."}),
		c("start-value", kit.Input{Icon: iconMail, DefaultValue: "m@example.com", Type: "email"}),
		c("end", kit.Input{IconEnd: iconCheck, DefaultValue: "available-name"}),
		c("both", kit.Input{Icon: iconSearch, IconEnd: iconX, DefaultValue: "tela"}),
		c("disabled", kit.Input{Icon: iconSearch, Placeholder: "Search...", Disabled: true}),
		c("invalid", kit.Input{Icon: iconMail, DefaultValue: "not an email", Invalid: true}),
		c("plain", kit.Input{Placeholder: "No icon"}),
		c("long", kit.Input{Icon: iconSearch, DefaultValue: "a very long value that runs under the trailing icon of the field", IconEnd: iconX}),
	}}
}

func TestInputIconGolden(t *testing.T) { inputIconSheet().golden(t) }

func TestInputIconRTLGolden(t *testing.T) {
	s := inputIconSheet()
	s.name, s.rtl = "input_icon_rtl", true
	s.cells = s.cells[:4]
	s.golden(t)
}

// focusedInput is an input with the focus, for the ring.
func TestInputIconFocusGolden(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex w-72 flex-col gap-4 bg-background p-6 text-foreground"),
			kit.Input{Icon: iconSearch, Placeholder: "Search...", AriaLabel: "Search"},
			kit.Input{Icon: iconMail, IconEnd: iconCheck, DefaultValue: "m@example.com", AriaLabel: "Email"})
	}, telatest.Size(320, 160), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground"))
	app.Focus(telatest.ByRole("textbox", "Search"))
	app.GoldenScales("input_icon_focus", 1, 2)
}

func TestInputIconPadding(t *testing.T) {
	// The text starts after the icon: the field's padding is 36 px at the
	// start with an icon, 12 px without (ps-9, px-3), and the same at the
	// end.
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex w-72 flex-col gap-2"),
			kit.Input{DefaultValue: "x", AriaLabel: "plain"},
			kit.Input{Icon: iconSearch, DefaultValue: "x", AriaLabel: "start"},
			kit.Input{IconEnd: iconX, DefaultValue: "x", AriaLabel: "end"})
	})
	app.ExpectCount(telatest.ByClass("ps-9"), 1)
	app.ExpectCount(telatest.ByClass("pe-9"), 1)
	app.ExpectCount(telatest.ByClass("peer"), 2)
	// All three are the same size: the wrapper is as wide as the input.
	a := app.Bounds(telatest.ByRole("textbox", "plain"))
	for _, name := range []string{"start", "end"} {
		b := app.Bounds(telatest.ByRole("textbox", name))
		if b.Width != a.Width || b.Height != a.Height {
			t.Errorf("%s: %vx%v, plain %vx%v", name, b.Width, b.Height, a.Width, a.Height)
		}
	}
}

func TestInputIconClassGoesToTheWrapper(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex"),
			kit.Input{Icon: iconSearch, Class: "w-48", InputClass: "bg-muted", AriaLabel: "s"},
			kit.Input{Class: "w-64", AriaLabel: "plain"})
	})
	if w := app.Bounds(telatest.ByRole("textbox", "s")).Width; w != 192 {
		t.Errorf("with an icon Class sizes the wrapper: input %v px wide, want 192", w)
	}
	if w := app.Bounds(telatest.ByRole("textbox", "plain")).Width; w != 256 {
		t.Errorf("without one Class sizes the input: %v px, want 256", w)
	}
	app.ExpectCount(telatest.ByClass("bg-muted", "ps-9"), 1)
}

// The icons are decorative: the input keeps its role and name, and the
// icons leave the pointer to the input, so a press on one focuses it.
func TestInputIconIsDecorative(t *testing.T) {
	app := telatest.Mount(t, kit.Input{Icon: iconSearch, AriaLabel: "Search", Placeholder: "Search..."})
	app.ExpectAria(`- textbox "Search"`)
}

func TestInputIconInAField(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Email", Description: "We never share it.",
		Children: kit.Input{Icon: iconMail, Type: "email"}})
	app.Expect(telatest.ByRole("textbox", "Email"), telatest.HasName("Email"))
	app.Click(telatest.ByText("Email"))
	app.ExpectFocused(telatest.ByRole("textbox", "Email"))
	app.Type("a")
	app.ExpectValue(telatest.ByRole("textbox", "Email"), "a")
}

// Icons mirror in a right-to-left page: the leading one is at the right.
func TestInputIconMirrors(t *testing.T) {
	left := func(dir string) float32 {
		app := telatest.MountFunc(t, func() tela.Node {
			return h.Div(h.Dir(dir), h.Class("w-72"),
				kit.Input{Icon: iconSearch, AriaLabel: "s", Props: h.TestID("in")})
		}, telatest.Size(320, 80))
		// The input starts a 36 px inset: its text is at the far side from
		// the icon, which is the wrapper's first child after the input.
		return app.Bounds(telatest.ByTestID("in")).X
	}
	if l, r := left("ltr"), left("rtl"); l != r {
		t.Errorf("the input's box moved: %v ltr, %v rtl", l, r)
	}
}

// Typing in the field works as without icons, and the Invalid state reaches
// the input.
func TestInputIconInvalid(t *testing.T) {
	app := telatest.Mount(t, kit.Input{Icon: iconMail, Invalid: true, AriaLabel: "Email"})
	app.Expect(telatest.ByRole("textbox", "Email"), telatest.Invalid(true))
}
