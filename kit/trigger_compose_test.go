package kit_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

var tipLoc = telatest.ByRole("tooltip", "")

// mountTrigger mounts a component at the top left of a window, with a
// focusable button before it for the keyboard.
func mountTrigger(t *testing.T, c tela.Component) *telatest.App {
	t.Helper()
	return telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex items-start gap-4 p-16"), h.Button(h.Text("Before")), c)
	}, telatest.Size(480, 360), telatest.Theme(kit.Theme()), telatest.NoSettle())
}

// One button with two overlays: a tooltip that explains it and the menu it
// opens, nested either way (Radix's asChild on both triggers).
func TestTooltipAndMenuOnOneTrigger(t *testing.T) {
	items := []kit.MenuItem[string]{{Key: "rename", Label: "Rename"}, {Key: "delete", Label: "Delete"}}
	button := kit.Button{Variant: kit.Outline, Children: h.Text("Actions")}
	for name, c := range map[string]tela.Component{
		"tooltip outside": kit.Tooltip{Content: h.Text("More actions"),
			Trigger: kit.DropdownMenu[string]{Trigger: button, Items: items}},
		"menu outside": kit.DropdownMenu[string]{Items: items,
			Trigger: kit.Tooltip{Content: h.Text("More actions"), Trigger: button}},
	} {
		t.Run(name, func(t *testing.T) {
			app := mountTrigger(t, c)
			actions := telatest.ByRole("button", "Actions")
			app.Hover(actions)
			app.ExpectCount(tipLoc, 1)
			app.Expect(actions, telatest.HasPopup("menu"), telatest.Expanded(false))
			// The press closes the tooltip and opens the menu.
			app.Click(actions)
			app.Advance(time.Second)
			app.ExpectCount(tipLoc, 0)
			app.ExpectCount(telatest.ByRole("menu", "Actions"), 1)
			app.Press("Escape")
			app.ExpectCount(telatest.ByRole("menu", "Actions"), 0)
			app.ExpectFocused(actions)
			// The keyboard: Tab focuses it with its tooltip, Enter opens the
			// menu on its first item.
			app.Focus(telatest.ByRole("button", "Before"))
			app.MouseMove(470, 350)
			app.Advance(time.Second)
			app.Press("Tab")
			app.ExpectFocused(actions)
			app.ExpectCount(tipLoc, 1)
			app.Press("Enter")
			app.ExpectFocused(telatest.ByRole("menuitem", "Rename"))
			app.ExpectCount(tipLoc, 0)
		})
	}
}

// A dialog's trigger with a tooltip.
func TestTooltipOnDialogTrigger(t *testing.T) {
	app := mountTrigger(t, kit.Tooltip{Content: h.Text("Edit your profile"),
		Trigger: kit.Dialog{Trigger: kit.Button{Children: h.Text("Edit")}, Title: h.Text("Profile")}})
	edit := telatest.ByRole("button", "Edit")
	app.Hover(edit)
	app.ExpectCount(tipLoc, 1)
	app.Click(edit)
	app.ExpectCount(telatest.ByRole("dialog", "Profile"), 1)
}

// Toggle, Checkbox, Switch and Slider are Triggers: they keep working as
// controls with a tooltip on them.
func TestControlsAsTooltipTriggers(t *testing.T) {
	cases := []struct {
		name    string
		trigger kit.Trigger
		loc     telatest.Locator
		after   telatest.State
	}{
		{"Toggle", kit.Toggle{AriaLabel: "Bold", Children: h.Text("B")}, telatest.ByRole("button", "Bold"), telatest.Pressed(true)},
		{"Checkbox", kit.Checkbox{AriaLabel: "Accept"}, telatest.ByRole("checkbox", "Accept"), telatest.Checked(true)},
		{"Switch", kit.Switch{AriaLabel: "Wi-Fi"}, telatest.ByRole("switch", "Wi-Fi"), telatest.Checked(true)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := mountTrigger(t, kit.Tooltip{Content: h.Text("Explains it"), Trigger: c.trigger})
			app.Hover(c.loc)
			app.ExpectCount(tipLoc, 1)
			app.Click(c.loc)
			app.Expect(c.loc, c.after)
			app.ExpectCount(tipLoc, 0)
			// The tooltip describes its trigger.
			app.Focus(telatest.ByRole("button", "Before"))
			app.Press("Tab")
			app.ExpectCount(tipLoc, 1)
		})
	}
}

// A slider's tooltip opens for the pointer over the slider and for the
// keyboard focus on its thumb, inside the trigger.
func TestSliderAsTooltipTrigger(t *testing.T) {
	app := mountTrigger(t, kit.Tooltip{Content: h.Text("Volume"),
		Trigger: kit.Slider{AriaLabel: "Volume", DefaultValue: new(50.0), Class: "w-48"}})
	thumb := telatest.ByRole("slider", "Volume")
	app.Hover(thumb)
	app.ExpectCount(tipLoc, 1)
	app.MouseMove(470, 350)
	app.Advance(time.Second)
	app.ExpectCount(tipLoc, 0)
	app.Focus(telatest.ByRole("button", "Before"))
	app.Press("Tab")
	app.ExpectFocused(thumb)
	app.ExpectCount(tipLoc, 1)
	app.Press("ArrowRight")
	app.Expect(thumb, telatest.HasValue("51"))
}

// kit adds no tab stop to a context menu's area (Radix's model): Tab goes
// from the button before it to the one inside it, and an area with nothing
// focusable is skipped.
func TestContextMenuAreaIsNotATabStop(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex flex-col gap-4 p-6"),
			h.Button(h.Text("Before")),
			kit.ContextMenu[string]{AriaLabel: "Canvas menu", Class: "h-24 w-48 border",
				Items: []kit.MenuItem[string]{{Key: "a", Label: "Item"}}, Children: h.Text("Right click here")},
			h.Button(h.Text("After")))
	}, telatest.Theme(kit.Theme()))
	app.Focus(telatest.ByRole("button", "Before"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "After"))
}
