package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// Tests of the owner's API review (ADR-154, ADR-156).

func intTabs() []kit.Tab[int] {
	return []kit.Tab[int]{
		{Value: 0, Label: "Zero"},
		{Value: 1, Label: "One"},
		{Value: 2, Label: "Two"},
	}
}

// A zero key is a real tab: an unset DefaultValue starts on the first
// enabled tab (here the second, the zero one being disabled), new(0)
// selects the zero key.
func TestTabsZeroKey(t *testing.T) {
	ts := intTabs()
	ts[0].Disabled = true
	app := telatest.Mount(t, kit.Tabs[int]{Tabs: ts})
	selectedTab(app, "One")

	app = telatest.Mount(t, kit.Tabs[int]{Tabs: intTabs(), DefaultValue: new(0)})
	selectedTab(app, "Zero")

	app = telatest.Mount(t, kit.Tabs[int]{Tabs: intTabs(), DefaultValue: new(2)})
	selectedTab(app, "Two")

	// A default that is not a tab falls back to the first enabled one.
	app = telatest.Mount(t, kit.Tabs[int]{Tabs: ts, DefaultValue: new(9)})
	selectedTab(app, "One")
}

func intItems() []kit.SelectItem[int] {
	return []kit.SelectItem[int]{
		{Value: 0, Label: "None"},
		{Value: 1, Label: "Low"},
		{Value: 2, Label: "High"},
	}
}

// An unset DefaultValue shows the placeholder even when an option is the
// zero key; new(0) chooses that option.
func TestSelectZeroKey(t *testing.T) {
	app := telatest.Mount(t, kit.Select[int]{Items: intItems(), Placeholder: "Priority", AriaLabel: "Level"})
	app.Expect(telatest.ByRole("button", "Priority Level"), telatest.Expanded(false))

	app = telatest.Mount(t, kit.Select[int]{Items: intItems(), Placeholder: "Priority", AriaLabel: "Level", DefaultValue: new(0)})
	app.Expect(telatest.ByRole("button", "None Level"), telatest.Expanded(false))

	app = telatest.Mount(t, kit.Select[int]{Items: intItems(), Placeholder: "Priority", AriaLabel: "Level", DefaultValue: new(2)})
	app.Expect(telatest.ByRole("button", "High Level"), telatest.Expanded(false))
}

// A radio group of a menu has nothing chosen without a DefaultValue, even
// when an item has the zero key; new(0) chooses it.
func TestMenuRadioZeroKey(t *testing.T) {
	items := func(def *int) []kit.MenuItem[int] {
		return []kit.MenuItem[int]{{Kind: kit.MenuRadioGroup, Key: 100, DefaultValue: def, Items: []kit.MenuItem[int]{
			{Key: 0, Label: "Off"}, {Key: 1, Label: "On"},
		}}}
	}
	app := telatest.Mount(t, kit.DropdownMenu[int]{DefaultOpen: true, Trigger: kit.Button{Children: h.Text("Menu")}, Items: items(nil)})
	app.Expect(telatest.ByRole("menuitemradio", "Off"), telatest.Checked(false))
	app.Expect(telatest.ByRole("menuitemradio", "On"), telatest.Checked(false))

	app = telatest.Mount(t, kit.DropdownMenu[int]{DefaultOpen: true, Trigger: kit.Button{Children: h.Text("Menu")}, Items: items(new(0))})
	app.Expect(telatest.ByRole("menuitemradio", "Off"), telatest.Checked(true))
	app.Expect(telatest.ByRole("menuitemradio", "On"), telatest.Checked(false))
}

// AriaLabel names an Input and a Textarea that have no label of their own.
func TestInputAndTextareaAriaLabel(t *testing.T) {
	app := telatest.Mount(t, h.Div(kit.Input{AriaLabel: "Search", Type: "search"}, kit.Textarea{AriaLabel: "Message"}))
	app.ExpectCount(telatest.ByRole("searchbox", "Search"), 1)
	app.ExpectCount(telatest.ByRole("textbox", "Message"), 1)
}

// A ColorPicker in a Field: the label points at the hex field, which takes
// the description and the invalid state, and the picker is a group named by
// the label. Dev mode would report a label that points at nothing.
func TestColorPickerInAField(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Brand color", Description: "Used for links.", Error: "Pick one.",
		Children: kit.ColorPicker{DefaultValue: new(tela.Hex("#6366f1"))}})
	app.ExpectCount(telatest.ByRole("group", "Brand color"), 1)
	hex := telatest.ByRole("textbox", "Hex")
	app.Expect(hex, telatest.HasValue("#6366f1"), telatest.Invalid(true))
	app.Click(telatest.ByText("Brand color"))
	app.ExpectFocused(hex)

	// Outside a Field, ID and AriaLabel do the same by hand.
	app = telatest.Mount(t, h.Div(kit.Label{For: "tint", Children: h.Text("Tint")},
		kit.ColorPicker{ID: "tint", AriaLabel: "Tint picker"}))
	app.ExpectCount(telatest.ByRole("group", "Tint picker"), 1)
	app.Click(telatest.ByText("Tint"))
	app.ExpectFocused(telatest.ByRole("textbox", "Hex"))

	// A picker with no name is not a group of its own.
	app = telatest.Mount(t, kit.ColorPicker{})
	app.ExpectAria(`
        - group "Color area":
          - slider "Color area"
        - group "Hue":
          - slider "Hue"
        - textbox "Hex": "#000000"`)
}

// A ColorInput in a Field: the label points at the button, which is named
// by it followed by the color, and the panel is named by it too.
func TestColorInputInAField(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Brand color", Description: "Used for links.",
		Children: kit.ColorInput{DefaultValue: new(tela.Hex("#6366f1"))}})
	btn := telatest.ByRole("button", "Brand color, #6366f1")
	app.Expect(btn, telatest.HasPopup("dialog"))
	// A press on the label presses the button, as a label does in HTML.
	app.Click(telatest.ByText("Brand color"))
	app.ExpectCount(telatest.ByRole("dialog", "Brand color"), 1)
	app.Press("Escape")
	app.ExpectFocused(btn)

	// Outside a Field: ID and AriaLabel.
	app = telatest.Mount(t, h.Div(kit.Label{For: "fill", Children: h.Text("Fill")},
		kit.ColorInput{ID: "fill", AriaLabel: "Fill color", DefaultValue: new(tela.Hex("#6366f1"))}))
	app.Click(telatest.ByText("Fill"))
	app.ExpectCount(telatest.ByRole("dialog", "Fill color"), 1)

	// Without a name the button keeps its code, and the panel its default.
	app = telatest.Mount(t, kit.ColorInput{DefaultValue: new(tela.Hex("#6366f1"))})
	app.Click(telatest.ByRole("button", "#6366f1"))
	app.ExpectCount(telatest.ByRole("dialog", "Color picker"), 1)
}

// A nil DefaultValue starts a color picker at opaque black, and a
// transparent black is a color it can start with.
func TestColorPickerDefaultValuePointer(t *testing.T) {
	app := telatest.Mount(t, kit.ColorPicker{})
	hex := telatest.ByRole("textbox", "Hex")
	app.Expect(hex, telatest.HasValue("#000000"))

	app = telatest.Mount(t, kit.ColorPicker{DefaultValue: new(tela.Hex("#ff0000"))})
	app.Expect(hex, telatest.HasValue("#ff0000"))
}
