package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

var formats = []string{"bold", "italic", "underline", "strike"}

func toggleGroupPage(o aria.ToggleGroupOptions[string]) func() tela.Node {
	return func() tela.Node { return useToggleGroupPage(o) }
}

func useToggleGroupPage(o aria.ToggleGroupOptions[string]) tela.Node {
	o.Keys, o.AriaLabel = formats, "Text style"
	g := aria.UseToggleGroup(o)
	cls := "flex"
	if o.Orientation == "vertical" {
		cls = "flex flex-col"
	}
	return Div(g.Props, Class(cls),
		Range(len(formats), func(i int) tela.Node {
			return Button(g.ItemProps(formats[i]), Key(formats[i]), Class("h-8 w-24 aria-pressed:w-28 aria-checked:w-28"), Text(capital(formats[i])))
		}))
}

func item(role, name string) telatest.Locator { return telatest.ByRole(role, name) }

func TestToggleGroupMultiple(t *testing.T) {
	var got []string
	o := aria.ToggleGroupOptions[string]{
		SelectionMode:     "multiple",
		OnSelectionChange: func(s aria.Selection[string]) { got = append(got, s.String()) },
	}
	app := telatest.MountFunc(t, toggleGroupPage(o), telatest.Size(500, 100))
	app.ExpectAria(`
		- toolbar "Text style":
		  - button "Bold"
		  - button "Italic"
		  - button "Underline"
		  - button "Strike"
	`)
	app.Click(item("button", "Bold"))
	app.Click(item("button", "Underline"))
	app.Expect(item("button", "Bold"), telatest.Pressed(true))
	app.Expect(item("button", "Underline"), telatest.Pressed(true))
	app.Expect(item("button", "Italic"), telatest.Pressed(false))
	app.Click(item("button", "Bold"))
	app.Expect(item("button", "Bold"), telatest.Pressed(false))
	if want := "{bold} {bold underline} {underline}"; strings.Join(got, " ") != want {
		t.Errorf("OnSelectionChange saw %v, want %q", got, want)
	}
	// Enter and Space toggle.
	app.Focus(item("button", "Strike"))
	app.Press("Enter")
	app.Expect(item("button", "Strike"), telatest.Pressed(true))
	app.Press("Space")
	app.Expect(item("button", "Strike"), telatest.Pressed(false))
	// An empty selection is fine in multiple mode.
	app.Click(item("button", "Underline"))
	app.Expect(item("button", "Underline"), telatest.Pressed(false))
}

func TestToggleGroupSingle(t *testing.T) {
	o := aria.ToggleGroupOptions[string]{DefaultSelected: aria.SelectionOf("italic")}
	app := telatest.MountFunc(t, toggleGroupPage(o), telatest.Size(500, 100))
	app.ExpectAria(`
		- radiogroup "Text style":
		  - radio "Bold"
		  - radio "Italic" [checked]
		  - radio "Underline"
		  - radio "Strike"
	`)
	app.Click(item("radio", "Bold"))
	app.Expect(item("radio", "Bold"), telatest.Checked(true))
	app.Expect(item("radio", "Italic"), telatest.Checked(false))
	// Pressing the one that is on turns it off.
	app.Click(item("radio", "Bold"))
	app.Expect(item("radio", "Bold"), telatest.Checked(false))
}

func TestToggleGroupDisallowEmpty(t *testing.T) {
	var changes int
	o := aria.ToggleGroupOptions[string]{
		DefaultSelected: aria.SelectionOf("bold"), DisallowEmptySelection: true,
		OnSelectionChange: func(aria.Selection[string]) { changes++ },
	}
	app := telatest.MountFunc(t, toggleGroupPage(o), telatest.Size(500, 100))
	app.Click(item("radio", "Bold"))
	app.Expect(item("radio", "Bold"), telatest.Checked(true))
	if changes != 0 {
		t.Errorf("%d changes, want none", changes)
	}
	app.Click(item("radio", "Italic"))
	app.Expect(item("radio", "Italic"), telatest.Checked(true))
	app.Expect(item("radio", "Bold"), telatest.Checked(false))

	m := aria.ToggleGroupOptions[string]{SelectionMode: "multiple", DefaultSelected: aria.SelectionOf("bold"), DisallowEmptySelection: true}
	app = telatest.MountFunc(t, toggleGroupPage(m), telatest.Size(500, 100))
	app.Click(item("button", "Bold"))
	app.Expect(item("button", "Bold"), telatest.Pressed(true))
	app.Click(item("button", "Italic"))
	app.Click(item("button", "Bold"))
	app.Expect(item("button", "Bold"), telatest.Pressed(false))
}

// The arrows move the focus without selecting, around the ends only with
// ShouldFocusWrap, skipping disabled items.
func TestToggleGroupKeyboard(t *testing.T) {
	o := aria.ToggleGroupOptions[string]{DisabledKeys: []string{"italic"}}
	app := telatest.MountFunc(t, toggleGroupPage(o), telatest.Size(500, 100))
	app.Press("Tab")
	app.ExpectFocused(item("radio", "Bold"))
	app.Press("ArrowRight") // italic is disabled
	app.ExpectFocused(item("radio", "Underline"))
	app.Expect(item("radio", "Underline"), telatest.Checked(false)) // moving does not select
	app.Press("ArrowDown")                                          // not the group's orientation
	app.ExpectFocused(item("radio", "Underline"))
	app.Press("End")
	app.ExpectFocused(item("radio", "Strike"))
	app.Press("ArrowRight") // no wrap
	app.ExpectFocused(item("radio", "Strike"))
	app.Press("Home")
	app.ExpectFocused(item("radio", "Bold"))
	app.Press("ArrowLeft")
	app.ExpectFocused(item("radio", "Bold"))

	w := aria.ToggleGroupOptions[string]{ShouldFocusWrap: true}
	app = telatest.MountFunc(t, toggleGroupPage(w), telatest.Size(500, 100))
	app.Focus(item("radio", "Strike"))
	app.Press("ArrowRight")
	app.ExpectFocused(item("radio", "Bold"))
	app.Press("ArrowLeft")
	app.ExpectFocused(item("radio", "Strike"))
}

func TestToggleGroupVerticalAndRTL(t *testing.T) {
	v := aria.ToggleGroupOptions[string]{Orientation: "vertical"}
	app := telatest.MountFunc(t, toggleGroupPage(v), telatest.Size(200, 400))
	app.Focus(item("radio", "Bold"))
	app.Press("ArrowRight") // not the orientation
	app.ExpectFocused(item("radio", "Bold"))
	app.Press("ArrowDown")
	app.ExpectFocused(item("radio", "Italic"))
	app.Press("ArrowUp")
	app.ExpectFocused(item("radio", "Bold"))

	r := aria.ToggleGroupOptions[string]{Direction: "rtl"}
	app = telatest.MountFunc(t, toggleGroupPage(r), telatest.Size(500, 100))
	app.Focus(item("radio", "Italic"))
	app.Press("ArrowLeft")
	app.ExpectFocused(item("radio", "Underline"))
	app.Press("ArrowRight")
	app.ExpectFocused(item("radio", "Italic"))
}

// The group is one tab stop, and Tab comes back to the item that had the
// focus.
func TestToggleGroupTabStop(t *testing.T) {
	page := func() tela.Node {
		return Div(Button(TestID("before"), Text("Before")), tela.C(toggleGroupPage(aria.ToggleGroupOptions[string]{})), Button(TestID("after"), Text("After")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(500, 100))
	app.Focus(telatest.ByTestID("before"))
	app.Press("Tab")
	app.ExpectFocused(item("radio", "Bold"))
	app.Press("ArrowRight")
	app.Press("ArrowRight")
	app.ExpectFocused(item("radio", "Underline"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("after"))
	app.Press("Shift+Tab")
	app.ExpectFocused(item("radio", "Underline"))

	// With a selection and no focus yet, Tab enters at the selected item.
	s := aria.ToggleGroupOptions[string]{DefaultSelected: aria.SelectionOf("strike")}
	app = telatest.MountFunc(t, toggleGroupPage(s), telatest.Size(500, 100))
	app.Press("Tab")
	app.ExpectFocused(item("radio", "Strike"))
}

func TestToggleGroupControlled(t *testing.T) {
	var asked []string
	var set tela.Setter[aria.Selection[string]]
	page := func() tela.Node {
		sel, s := tela.UseState(aria.SelectionOf("bold"))
		set = s
		return useToggleGroupPage(aria.ToggleGroupOptions[string]{
			SelectionMode: "multiple", Selected: sel,
			OnSelectionChange: func(n aria.Selection[string]) { asked = append(asked, n.String()); s.Set(n) },
		})
	}
	app := telatest.MountFunc(t, page, telatest.Size(500, 100))
	app.Expect(item("button", "Bold"), telatest.Pressed(true))
	app.Click(item("button", "Italic"))
	app.Expect(item("button", "Italic"), telatest.Pressed(true))
	set.Set(aria.SelectionOf("strike"))
	app.Expect(item("button", "Strike"), telatest.Pressed(true))
	app.Expect(item("button", "Bold"), telatest.Pressed(false))
	if fmt.Sprint(asked) != "[{bold italic}]" {
		t.Errorf("OnSelectionChange saw %v", asked)
	}
}

func TestToggleGroupControlledRefuses(t *testing.T) {
	var asked int
	o := aria.ToggleGroupOptions[string]{
		Selected:          aria.SelectionOf("bold"),
		OnSelectionChange: func(aria.Selection[string]) { asked++ },
	}
	app := telatest.MountFunc(t, toggleGroupPage(o), telatest.Size(500, 100))
	app.Click(item("radio", "Italic"))
	app.Expect(item("radio", "Bold"), telatest.Checked(true))
	app.Expect(item("radio", "Italic"), telatest.Checked(false))
	if asked != 1 {
		t.Errorf("%d requests, want 1", asked)
	}
	// An empty controlled selection shows nothing on.
	app = telatest.MountFunc(t, toggleGroupPage(aria.ToggleGroupOptions[string]{Selected: aria.SelectionOf[string]()}), telatest.Size(500, 100))
	app.Expect(item("radio", "Bold"), telatest.Checked(false))
}

func TestToggleGroupDisabled(t *testing.T) {
	var changes int
	o := aria.ToggleGroupOptions[string]{
		Disabled: true, OnSelectionChange: func(aria.Selection[string]) { changes++ },
	}
	app := telatest.MountFunc(t, toggleGroupPage(o), telatest.Size(500, 100))
	app.Expect(item("radio", "Bold"), telatest.Disabled(true))
	if changes != 0 {
		t.Error("a disabled group reported a change")
	}
	d := aria.ToggleGroupOptions[string]{DisabledKeys: []string{"bold"}}
	app = telatest.MountFunc(t, toggleGroupPage(d), telatest.Size(500, 100))
	app.Expect(item("radio", "Bold"), telatest.Disabled(true))
	app.Expect(item("radio", "Italic"), telatest.Disabled(false))
}

func TestToggleGroupOptionErrors(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		for _, want := range []string{"SelectionMode", `"multiple"`, `did you mean "multiple"`} {
			if !strings.Contains(msg, want) {
				t.Errorf("message %q lacks %q", msg, want)
			}
		}
	}()
	telatest.MountFunc(t, toggleGroupPage(aria.ToggleGroupOptions[string]{SelectionMode: "multipl"}), telatest.Size(100, 100))
}
