package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// TestDisabledTrigger checks the Disabled field of the overlays that have a
// trigger slot: the trigger is a disabled button, a press on it opens
// nothing, and clearing the field enables it again.
func TestDisabledTrigger(t *testing.T) {
	trigger := kit.Button{Variant: kit.Outline, Children: h.Text("Open")}
	cases := []struct {
		name string
		make func(disabled bool) tela.Component
		role string
	}{
		{"Dialog", func(d bool) tela.Component {
			return kit.Dialog{Trigger: trigger, Title: h.Text("Title"), Disabled: d}
		}, "dialog"},
		{"AlertDialog", func(d bool) tela.Component {
			return kit.AlertDialog{Trigger: trigger, Title: h.Text("Title"), Disabled: d}
		}, "alertdialog"},
		{"Popover", func(d bool) tela.Component {
			return kit.Popover{Trigger: trigger, Children: h.Text("Body"), Disabled: d}
		}, "dialog"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			disabled := true
			var set tela.Setter[bool]
			app := telatest.MountFunc(t, func() tela.Node {
				d, s := tela.UseState(disabled)
				set = s
				return h.Div(h.Class("p-6"), c.make(d))
			}, telatest.Theme(kit.Theme()))
			open := telatest.ByRole("button", "Open")
			app.Expect(open, telatest.Disabled(true))
			// A disabled button has pointer-events-none: press where it is.
			r := app.Bounds(open)
			app.ClickAt(r.X+r.Width/2, r.Y+r.Height/2)
			app.ExpectCount(telatest.ByRole(c.role, ""), 0)

			set.Set(false)
			app.Expect(open, telatest.Disabled(false))
			app.Click(open)
			app.ExpectCount(telatest.ByRole(c.role, ""), 1)
		})
	}
}
