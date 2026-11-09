package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func buttonLoadingSheet() *sheet {
	c := func(name string, b kit.Button) cell {
		b.Props = h.TestID(name + "-button")
		return cell{name: name, probes: []string{name + "-button"}, tela: b}
	}
	return &sheet{name: "button_loading", slot: 176, cells: []cell{
		c("text", kit.Button{Loading: true, Children: h.Text("Save changes")}),
		c("text-idle", kit.Button{Children: h.Text("Save changes")}),
		c("icon", kit.Button{Loading: true, Icon: iconMail, Children: h.Text("Login with email")}),
		c("icon-idle", kit.Button{Icon: iconMail, Children: h.Text("Login with email")}),
		c("icon-end", kit.Button{Loading: true, Variant: kit.Secondary, IconEnd: iconArrowRight, Children: h.Text("Continue")}),
		c("outline", kit.Button{Loading: true, Variant: kit.Outline, Children: h.Text("Submit")}),
		c("destructive", kit.Button{Loading: true, Variant: kit.Destructive, Icon: iconCircleAlert, Children: h.Text("Delete")}),
		c("ghost", kit.Button{Loading: true, Variant: kit.Ghost, Children: h.Text("Refresh")}),
		c("sm", kit.Button{Loading: true, Size: kit.Sm, Variant: kit.Outline, Children: h.Text("Small")}),
		c("lg", kit.Button{Loading: true, Size: kit.Lg, Children: h.Text("Large")}),
		c("icon-only", kit.Button{Loading: true, Variant: kit.Outline, Size: kit.Icon, Icon: iconChevronRight, AriaLabel: "Next"}),
		c("link", kit.Button{Loading: true, Variant: kit.Link, Children: h.Text("Resend email")}),
	}}
}

func TestButtonLoadingGolden(t *testing.T) { buttonLoadingSheet().golden(t) }

func rtlButtonLoadingSheet() *sheet {
	s := buttonLoadingSheet()
	s.name, s.rtl = "button_loading_rtl", true
	s.cells = s.cells[2:5]
	return s
}

func TestButtonLoadingRTLGolden(t *testing.T) { rtlButtonLoadingSheet().golden(t) }

// A loading button is as wide and as tall as the idle one, however it shows
// the spinner: nothing around it moves.
func TestButtonLoadingKeepsItsSize(t *testing.T) {
	for name, b := range map[string]kit.Button{
		"text":     {Children: h.Text("Save changes")},
		"icon":     {Icon: iconMail, Children: h.Text("Login with email")},
		"icon-end": {IconEnd: iconArrowRight, Children: h.Text("Continue")},
		"outline":  {Variant: kit.Outline, Children: h.Text("Submit")},
		"sm":       {Size: kit.Sm, Children: h.Text("Small")},
		"icon-btn": {Size: kit.Icon, Icon: iconChevronRight, AriaLabel: "Next"},
	} {
		idle := b
		idle.Props = h.TestID("b")
		busy := b
		busy.Loading, busy.Props = true, h.TestID("b")
		a, c := telatest.Mount(t, idle), telatest.Mount(t, busy)
		ra, rc := a.Bounds(telatest.ByTestID("b")), c.Bounds(telatest.ByTestID("b"))
		if ra.Width != rc.Width || ra.Height != rc.Height {
			t.Errorf("%s: idle %vx%v, loading %vx%v", name, ra.Width, ra.Height, rc.Width, rc.Height)
		}
	}
}

func TestButtonLoadingIgnoresPresses(t *testing.T) {
	var clicks int
	press := func() { clicks++ }
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.Button{Loading: true, OnClick: press, Children: h.Text("Save")},
			kit.Button{OnClick: press, Children: h.Text("Idle")})
	})
	// The pointer goes through a loading button, as through a disabled one.
	r := app.Bounds(telatest.ByRole("button", "Save"))
	app.ClickAt(r.X+r.Width/2, r.Y+r.Height/2)
	if clicks != 0 {
		t.Errorf("a loading button's click ran OnClick %d times", clicks)
	}
	app.Focus(telatest.ByRole("button", "Save"))
	app.ExpectFocused(telatest.ByRole("button", "Save")) // it stays in the tab order
	app.Press("Enter")
	app.Press("Space")
	if clicks != 0 {
		t.Errorf("a loading button's key press ran OnClick %d times", clicks)
	}
	app.Click(telatest.ByRole("button", "Idle"))
	if clicks != 1 {
		t.Errorf("an idle button's click ran OnClick %d times", clicks)
	}
}

// The text of a loading button still names it.
func TestButtonLoadingKeepsItsName(t *testing.T) {
	for _, b := range []kit.Button{
		{Children: h.Text("Save")},
		{Icon: iconMail, Children: h.Text("Save")},
	} {
		b.Loading = true
		app := telatest.Mount(t, b)
		app.Expect(telatest.ByRole("button", "Save"), telatest.HasName("Save"))
	}
}

func TestButtonLoadingIsBusy(t *testing.T) {
	app := telatest.Mount(t, kit.Button{Loading: true, Class: "aria-busy:w-40", Children: h.Text("Save"), Props: h.TestID("b")})
	if w := app.Bounds(telatest.ByTestID("b")).Width; w != 160 {
		t.Errorf("a loading button is not aria-busy: width %v", w)
	}
	app = telatest.Mount(t, kit.Button{Class: "aria-busy:w-40", Children: h.Text("Save"), Props: h.TestID("b")})
	if w := app.Bounds(telatest.ByTestID("b")).Width; w == 160 {
		t.Error("an idle button is aria-busy")
	}
}

// A loading submit button does not submit its form.
func TestButtonLoadingDoesNotSubmit(t *testing.T) {
	var submits int
	for _, loading := range []bool{true, false} {
		submits = 0
		app := telatest.MountFunc(t, func() tela.Node {
			return h.Form(h.OnSubmit(func(tela.SubmitEvent) { submits++ }),
				kit.Button{Type: "submit", Loading: loading, Children: h.Text("Send")})
		})
		app.Focus(telatest.ByRole("button", "Send"))
		app.Press("Enter")
		app.Press("Space")
		if want := map[bool]int{true: 0, false: 2}[loading]; submits != want {
			t.Errorf("Loading %v: %d submits, want %d", loading, submits, want)
		}
	}
}

func TestButtonLoadingIconSlot(t *testing.T) {
	// LoadingIcon replaces the spinner; the recipe's classes keep it
	// turning.
	app := telatest.Mount(t, kit.Button{Loading: true, LoadingIcon: iconCheck, Children: h.Text("Save"), Props: h.TestID("b")})
	app.ExpectCount(telatest.ByRole("button", "Save"), 1)
}
