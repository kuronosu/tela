package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// menuPage is a menu button; the menu shows the focus strategy it opened
// with and focuses its first or last item accordingly, as UseMenu will.
func menuPage(o aria.MenuTriggerOptions, ev *events) func() tela.Node {
	return func() tela.Node {
		mt := aria.UseMenuTrigger(o)
		pop := aria.UsePopover(aria.PopoverOptions{
			Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close, Placement: "bottom-start", Modal: true,
		})
		return Div(Class("relative h-full"),
			Button(mt.TriggerProps, TestID("trigger"), Class("absolute left-4 top-4 h-8 w-24"), Text("Actions")),
			Portal(If(mt.Open,
				Div(pop.Props, mt.MenuProps, Role("menu"), Class("w-40 bg-white"),
					Textf("strategy: %q", mt.FocusStrategy),
					Button(TestID("first"), OnClick(func() { ev.add("first"); mt.Close() }), Class("block h-8 w-32"), Text("Edit")),
					Button(TestID("last"), Class("block h-8 w-32"), Text("Delete"))))))
	}
}

func TestMenuTriggerKeyboard(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuPage(aria.MenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	trigger := telatest.ByTestID("trigger")
	app.Focus(trigger)
	app.Expect(trigger, telatest.Expanded(false))

	for _, tc := range []struct{ key, strategy string }{
		{"ArrowDown", "first"},
		{"ArrowUp", "last"},
		{"Enter", "first"},
		{"Space", "first"},
	} {
		app.Press(tc.key)
		app.ExpectText(`strategy: "` + tc.strategy + `"`)
		app.Press("Escape")
		app.ExpectNoText("strategy")
		app.ExpectFocused(trigger) // the focus returns to the trigger
	}
}

func TestMenuTriggerMouseAndAria(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuPage(aria.MenuTriggerOptions{}, &ev), telatest.Size(400, 300))
	trigger := telatest.ByRole("button", "Actions")
	// The mouse opens it on the press, with no item focused.
	app.MouseMove(30, 30)
	app.MouseDown()
	app.ExpectText(`strategy: ""`)
	app.MouseUp()
	app.ExpectText(`strategy: ""`)
	app.ExpectAria(`
		- menu "Actions":
		  - text: "strategy: \"\""
		  - button "Edit"
		  - button "Delete"
	`)
	// Choosing an item closes the menu through Close.
	app.Click(telatest.ByTestID("first"))
	app.ExpectNoText("strategy")
	if got := ev.take(); got != "first" {
		t.Errorf("events %q", got)
	}
	app.Expect(trigger, telatest.Expanded(false))
}

// A long press opens the menu with the first item focused; a quick press
// is the button's own.
func TestMenuTriggerLongPress(t *testing.T) {
	var ev events
	o := aria.MenuTriggerOptions{Trigger: "longPress", OnPress: func(aria.PressEvent) { ev.add("press") }}
	app := telatest.MountFunc(t, menuPage(o, &ev), telatest.Size(400, 300), telatest.NoSettle())
	count := func() int { return app.Count(telatest.ByRole("menu", "Actions")) }

	// A quick press: the button's action, no menu.
	app.MouseMove(30, 30)
	app.MouseDown()
	app.Advance(100 * time.Millisecond)
	app.MouseUp()
	app.Advance(50 * time.Millisecond)
	if got := ev.take(); got != "press" || count() != 0 {
		t.Errorf("a quick press: events %q, menu open %v", got, count() == 1)
	}
	// Enter does not open it, Alt+ArrowDown does.
	app.Focus(telatest.ByTestID("trigger"))
	app.Press("Enter")
	app.Press("ArrowDown")
	if count() != 0 {
		t.Errorf("Enter or ArrowDown opened a long-press menu")
	}
	ev.take()
	app.Press("Alt+ArrowUp")
	app.ExpectText(`strategy: "last"`)
	app.Press("Escape")
	app.Advance(50 * time.Millisecond)

	// A long press opens it, and its release is not a press.
	app.MouseMove(30, 30)
	app.MouseDown()
	app.Advance(400 * time.Millisecond)
	if count() != 0 {
		t.Errorf("the menu opened before the long press")
	}
	app.Advance(200 * time.Millisecond)
	app.ExpectText(`strategy: "first"`)
	app.MouseUp()
	app.Advance(50 * time.Millisecond)
	if got := ev.take(); got != "" {
		t.Errorf("the release of a long press was a press: %q", got)
	}
	if count() != 1 {
		t.Errorf("the menu closed with the release")
	}
}

func TestMenuTriggerDisabled(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, menuPage(aria.MenuTriggerOptions{Disabled: true}, &ev), telatest.Size(400, 300))
	app.Expect(telatest.ByRole("button", "Actions"), telatest.Disabled(true))
	app.ClickAt(30, 30)
	app.ExpectNoText("strategy")
}
