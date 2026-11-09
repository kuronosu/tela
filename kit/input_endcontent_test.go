package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// Lucide's eye and eye-off (ISC, THIRD_PARTY.md).
var (
	iconEye    = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/></svg>`}
	iconEyeOff = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49"/><path d="M14.084 14.158a3 3 0 0 1-4.242-4.242"/><path d="M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143"/><path d="m2 2 20 20"/></svg>`}
)

// revealInput is a password field with a button that shows the text.
type revealInput struct {
	Disabled bool
}

func (r revealInput) Build() tela.Node {
	shown, setShown := tela.UseState(false)
	typ, ic, label := "password", iconEye, "Show password"
	if shown {
		typ, ic, label = "text", iconEyeOff, "Hide password"
	}
	return kit.Input{Type: typ, DefaultValue: "correct horse", AriaLabel: "Password", Disabled: r.Disabled,
		Props: h.TestID("reveal-input"),
		EndContent: kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: ic, AriaLabel: label,
			Disabled: r.Disabled, OnClick: func() { setShown.Set(!shown) }, Props: h.TestID("reveal-button")}}
}

func endContentSheet() *sheet {
	c := func(name string, in kit.Input) cell {
		in.Props = h.TestID(name + "-input")
		return cell{name: name, width: 280, probes: []string{name + "-input"}, tela: in}
	}
	btn := func(ic tela.Icon) tela.Component {
		return kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: ic, AriaLabel: "Toggle"}
	}
	return &sheet{name: "input_endcontent", width: 640, cells: []cell{
		c("password", kit.Input{Type: "password", DefaultValue: "correct horse", EndContent: btn(iconEye)}),
		c("shown", kit.Input{DefaultValue: "correct horse", EndContent: btn(iconEyeOff)}),
		c("placeholder", kit.Input{Placeholder: "Password", Type: "password", EndContent: btn(iconEye)}),
		c("start", kit.Input{Icon: iconMail, Type: "email", DefaultValue: "m@example.com", EndContent: btn(iconX)}),
		c("disabled", kit.Input{Disabled: true, DefaultValue: "locked", EndContent: btn(iconEye)}),
		c("invalid", kit.Input{Invalid: true, DefaultValue: "not valid", EndContent: btn(iconEye)}),
	}}
}

func TestInputEndContentGolden(t *testing.T) { endContentSheet().golden(t) }

func TestInputEndContentRTLGolden(t *testing.T) {
	s := endContentSheet()
	s.name, s.rtl = "input_endcontent_rtl", true
	s.cells = s.cells[:4]
	s.golden(t)
}

// The button takes the focus and its own ring, and the field keeps its own.
func TestInputEndContentFocusGolden(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex w-72 flex-col gap-4 bg-background p-6 text-foreground"),
			revealInput{}, kit.Input{DefaultValue: "x", AriaLabel: "Other"})
	}, telatest.Size(320, 160), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground"))
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Show password"))
	app.GoldenScales("input_endcontent_focus", 1, 2)
}

func TestInputEndContentRevealToggle(t *testing.T) {
	app := telatest.Mount(t, revealInput{})
	app.ExpectCount(telatest.ByRole("button", "Show password"), 1)
	app.Click(telatest.ByRole("button", "Show password"))
	app.ExpectCount(telatest.ByRole("button", "Hide password"), 1)
	// The field keeps its text and is a plain textbox while shown.
	app.ExpectValue(telatest.ByRole("textbox", "Password"), "correct horse")
	app.Click(telatest.ByRole("button", "Hide password"))
	app.ExpectCount(telatest.ByRole("button", "Show password"), 1)
}

// A press on the button does not put the focus in the field, and a press on
// the field still does; the field leaves room for the button.
func TestInputEndContentLayout(t *testing.T) {
	app := telatest.Mount(t, h.Div(h.Class("w-72"), revealInput{}))
	in := app.Bounds(telatest.ByTestID("reveal-input"))
	btn := app.Bounds(telatest.ByTestID("reveal-button"))
	if btn.Width != 24 || btn.Height != 24 {
		t.Errorf("the button is %vx%v, want 24x24 (IconXs)", btn.Width, btn.Height)
	}
	if got := in.X + in.Width - (btn.X + btn.Width); got != 6 {
		t.Errorf("the button is %v px from the end, want 6 (end-1.5)", got)
	}
	if mid := in.Y + in.Height/2 - (btn.Y + btn.Height/2); mid != 0 {
		t.Errorf("the button is %v px off the field's middle", mid)
	}
	app.ExpectCount(telatest.ByClass("pe-9", "peer"), 1)
}

func TestInputEndContentMirrorsInRTL(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Dir("rtl"), h.Class("w-72"), revealInput{})
	}, telatest.Size(320, 80))
	in := app.Bounds(telatest.ByTestID("reveal-input"))
	btn := app.Bounds(telatest.ByTestID("reveal-button"))
	if got := btn.X - in.X; got != 6 {
		t.Errorf("under rtl the button is %v px from the left edge, want 6", got)
	}
}

func TestInputEndContentWithIconEndPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("EndContent with IconEnd did not panic in dev mode")
		}
	}()
	kit.Input{IconEnd: iconX, EndContent: kit.Badge{}}.Build()
}

// A key on the component works the same with a wrapper: the field keeps its
// text when its list is reordered.
type keyedInputs struct{}

func (keyedInputs) Build() tela.Node {
	order, setOrder := tela.UseState([]string{"a", "b"})
	return h.Div(
		h.Button(h.TestID("swap"), h.OnClick(func() { setOrder.Set([]string{"b", "a"}) }), h.Text("swap")),
		h.MapKeyed(order, func(s string) string { return s }, func(s string) tela.Node {
			return kit.Input{Icon: iconMail, AriaLabel: s, EndContent: kit.Badge{}}
		}))
}

func TestInputWithKeyKeepsTextAcrossReorder(t *testing.T) {
	app := telatest.Mount(t, keyedInputs{})
	app.Click(telatest.ByRole("textbox", "a"))
	app.Type("hello")
	app.Click(telatest.ByTestID("swap"))
	app.ExpectValue(telatest.ByRole("textbox", "a"), "hello")
	app.ExpectValue(telatest.ByRole("textbox", "b"), "")
}
