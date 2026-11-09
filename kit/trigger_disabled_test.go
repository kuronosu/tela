package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// TestDisabledTrigger checks the Disabled field of the overlays that have a
// trigger: the trigger is a disabled button, a press on it opens nothing,
// and clearing the field enables it again.
func TestDisabledTrigger(t *testing.T) {
	trigger := kit.Button{Variant: kit.Outline, Children: h.Text("Open")}
	menu := []kit.MenuItem[string]{{Key: "a", Label: "Item"}}
	cases := []struct {
		name   string
		make   func(disabled bool) tela.Component
		button string
		role   string
	}{
		{"Dialog", func(d bool) tela.Component {
			return kit.Dialog{Trigger: trigger, Title: h.Text("Title"), Disabled: d}
		}, "Open", "dialog"},
		{"AlertDialog", func(d bool) tela.Component {
			return kit.AlertDialog{Trigger: trigger, Title: h.Text("Title"), Disabled: d}
		}, "Open", "alertdialog"},
		{"Popover", func(d bool) tela.Component {
			return kit.Popover{Trigger: trigger, Children: h.Text("Body"), Disabled: d}
		}, "Open", "dialog"},
		{"DropdownMenu", func(d bool) tela.Component {
			return kit.DropdownMenu[string]{Trigger: trigger, Items: menu, Disabled: d}
		}, "Open", "menu"},
		{"Select", func(d bool) tela.Component {
			return kit.Select[string]{AriaLabel: "Fruit", Placeholder: "Open", Items: []kit.SelectItem[string]{{Value: "a", Label: "Apple"}}, Disabled: d}
		}, "Open Fruit", "listbox"},
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
			open := telatest.ByRole("button", c.button)
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

// A Disabled given to the trigger itself wins over the overlay's trigger
// props, which make the element an enabled button: the button stays
// disabled and opens nothing (it was lost before, ADR-145).
func TestTriggerOwnDisabledWins(t *testing.T) {
	trigger := kit.Button{Variant: kit.Outline, Disabled: true, Children: h.Text("Open")}
	menu := []kit.MenuItem[string]{{Key: "a", Label: "Item"}}
	cases := []struct {
		name string
		make func() tela.Component
		role string
	}{
		{"Dialog", func() tela.Component { return kit.Dialog{Trigger: trigger, Title: h.Text("Title")} }, "dialog"},
		{"Popover", func() tela.Component { return kit.Popover{Trigger: trigger, Children: h.Text("Body")} }, "dialog"},
		{"DropdownMenu", func() tela.Component { return kit.DropdownMenu[string]{Trigger: trigger, Items: menu} }, "menu"},
		{"Toggle", func() tela.Component {
			return kit.Tooltip{Content: h.Text("Bold"), Trigger: kit.DropdownMenu[string]{
				Trigger: kit.Toggle{Disabled: true, AriaLabel: "Open", Children: h.Text("B")}, Items: menu}}
		}, "menu"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := telatest.MountFunc(t, func() tela.Node { return h.Div(h.Class("p-6"), c.make()) }, telatest.Theme(kit.Theme()))
			open := telatest.ByRole("button", "Open")
			app.Expect(open, telatest.Disabled(true))
			r := app.Bounds(open)
			app.ClickAt(r.X+r.Width/2, r.Y+r.Height/2)
			app.ExpectCount(telatest.ByRole(c.role, ""), 0)
		})
	}
}
