package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Intercept is asked with the point before the menu opens: true takes the
// opening over (no menu, no OnOpenChange), false lets the menu open in the
// same event. An open menu is moved without asking.
func TestContextMenuIntercept(t *testing.T) {
	type call struct{ x, y float32 }
	var calls []call
	var changes []bool
	handle := true
	page := func() tela.Node {
		cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{
			OnOpenChange: func(open bool) { changes = append(changes, open) },
			Intercept: func(x, y float32) bool {
				calls = append(calls, call{x, y})
				return handle
			},
		})
		return Div(cm.TriggerProps, Class("h-40 w-40"), Text("area"),
			Portal(If(cm.Open, Div(Role("menu"), AriaLabel("m"), Class("h-10 w-20"), Text("menu")))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	menu := telatest.ByRole("menu", "m")

	app.ClickAt(30, 40, telatest.Right())
	if len(calls) != 1 || calls[0] != (call{30, 40}) {
		t.Fatalf("calls %v, want one at (30, 40)", calls)
	}
	if app.Count(menu) != 0 || len(changes) != 0 {
		t.Errorf("a handled opening opened the menu: count %d, changes %v", app.Count(menu), changes)
	}

	handle = false
	app.ClickAt(50, 60, telatest.Right())
	app.ExpectCount(menu, 1)
	if len(calls) != 2 || calls[1] != (call{50, 60}) || len(changes) != 1 || !changes[0] {
		t.Errorf("calls %v, changes %v", calls, changes)
	}
	// This menu has no popover to dismiss it, so it is still open: a right
	// click on the area moves it, which is not an opening and is not asked.
	app.ClickAt(20, 20, telatest.Right())
	if len(calls) != 2 {
		t.Errorf("a move of the open menu was asked about: calls %v", calls)
	}
}
