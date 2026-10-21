package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// checkboxPage is a checkbox with a Label, in a window where the box is
// 16 px wide, or 32 px with checked:.
func checkboxPage(o func() aria.ToggleOptions, hook func(aria.ToggleOptions) aria.ToggleResult) func() tela.Node {
	return func() tela.Node { return useCheckboxPage(o, hook) }
}

// useCheckboxPage is the body of checkboxPage, a custom hook so that a
// test can call it from its own component.
func useCheckboxPage(o func() aria.ToggleOptions, hook func(aria.ToggleOptions) aria.ToggleResult) tela.Node {
	box := hook(o())
	return Div(Class("flex items-center gap-2"),
		Button(box.Props, TestID("box"), Class("h-4 w-4 checked:w-8 rounded border")),
		Label(For(box.ID), Class("text-sm"), Text("Notifications")))
}

func TestCheckboxUncontrolled(t *testing.T) {
	app := telatest.MountFunc(t, checkboxPage(func() aria.ToggleOptions { return aria.ToggleOptions{DefaultSelected: true} }, aria.UseCheckbox),
		telatest.Size(300, 100))
	box := telatest.ByRole("checkbox", "Notifications")
	app.Expect(box, telatest.Checked(true))
	if w := boxWidth(app); w != 32 {
		t.Errorf("selected width %g, want 32 (checked:)", w)
	}
	app.Click(box)
	app.Expect(box, telatest.Checked(false))
	if w := boxWidth(app); w != 16 {
		t.Errorf("unselected width %g, want 16", w)
	}
	// Space toggles; Enter does not.
	app.Focus(box)
	app.Press("Space")
	app.Expect(box, telatest.Checked(true))
	app.Press("Enter")
	app.Expect(box, telatest.Checked(true))
	app.Press("Space")
	app.Expect(box, telatest.Checked(false))
}

// A click on the Label toggles the control it labels, and the label names
// it.
func TestCheckboxLabel(t *testing.T) {
	app := telatest.MountFunc(t, checkboxPage(func() aria.ToggleOptions { return aria.ToggleOptions{} }, aria.UseCheckbox),
		telatest.Size(300, 100))
	box := telatest.ByRole("checkbox", "Notifications")
	app.Expect(box, telatest.Checked(false))
	app.Click(telatest.ByText("Notifications"))
	app.Expect(box, telatest.Checked(true))
}

func TestCheckboxControlled(t *testing.T) {
	var asked []bool
	var on bool
	var setOn tela.Setter[bool]
	page := func() tela.Node {
		v, set := tela.UseState(false)
		on, setOn = v, set
		return useCheckboxPage(func() aria.ToggleOptions {
			return aria.ToggleOptions{Selected: v, OnChange: func(next bool) { asked = append(asked, next); set.Set(next) }}
		}, aria.UseCheckbox)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	box := telatest.ByRole("checkbox", "Notifications")
	app.Click(box)
	app.Expect(box, telatest.Checked(true))
	app.Click(box)
	app.Expect(box, telatest.Checked(false))
	if len(asked) != 2 || !asked[0] || asked[1] || on {
		t.Errorf("OnChange got %v, state %v", asked, on)
	}
	// The caller sets it from elsewhere: the control follows.
	setOn.Set(true)
	app.Expect(box, telatest.Checked(true))
}

// A controlled control shows what the caller passes: a change the caller
// does not store is refused.
func TestCheckboxControlledRefusesChanges(t *testing.T) {
	var asked []bool
	page := checkboxPage(func() aria.ToggleOptions {
		return aria.ToggleOptions{Selected: true, OnChange: func(next bool) { asked = append(asked, next) }}
	}, aria.UseCheckbox)
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	box := telatest.ByRole("checkbox", "Notifications")
	app.Click(box)
	app.Expect(box, telatest.Checked(true))
	if len(asked) != 1 || asked[0] {
		t.Errorf("OnChange got %v, want [false]", asked)
	}
}

func TestCheckboxDisabledAndReadOnly(t *testing.T) {
	var asked []bool
	for name, o := range map[string]aria.ToggleOptions{
		"disabled": {Disabled: true},
		"readonly": {ReadOnly: true},
	} {
		t.Run(name, func(t *testing.T) {
			o := o
			o.OnChange = func(v bool) { asked = append(asked, v) }
			app := telatest.MountFunc(t, checkboxPage(func() aria.ToggleOptions { return o }, aria.UseCheckbox), telatest.Size(300, 100))
			box := telatest.ByRole("checkbox", "Notifications")
			if name == "disabled" {
				app.Expect(box, telatest.Disabled(true))
				return
			}
			app.Focus(box)
			app.Press("Space")
			app.Click(box)
			app.Expect(box, telatest.Checked(false))
		})
	}
	if len(asked) != 0 {
		t.Errorf("OnChange got %v", asked)
	}
}

func TestSwitch(t *testing.T) {
	var on bool
	page := func() tela.Node {
		v, set := tela.UseState(false)
		on = v
		return useCheckboxPage(func() aria.ToggleOptions {
			return aria.ToggleOptions{Selected: v, OnChange: set.Set}
		}, aria.UseSwitch)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	sw := telatest.ByRole("switch", "Notifications")
	app.Click(sw)
	app.Expect(sw, telatest.Checked(true))
	if !on {
		t.Error("the caller's state did not follow")
	}
	app.Focus(sw)
	app.Press("Space")
	app.Expect(sw, telatest.Checked(false))
}

func TestToggleButton(t *testing.T) {
	page := func() tela.Node {
		bold, setBold := tela.UseState(false)
		tb := aria.UseToggleButton(aria.ToggleOptions{Selected: bold, OnChange: setBold.Set})
		return Button(tb.Props, Class("px-2 w-[40px] aria-pressed:w-[80px] h-8"), If(tb.Pressed, Class("h-10")), TestID("b"), Text("Bold"))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	b := telatest.ByRole("button", "Bold")
	app.Expect(b, telatest.Pressed(false))
	app.Click(b)
	app.Expect(b, telatest.Pressed(true))
	if w := app.Bounds(telatest.ByTestID("b")).Width; w != 80 {
		t.Errorf("aria-pressed: width %g, want 80", w)
	}
	app.Press("Enter")
	app.Expect(b, telatest.Pressed(false))
	app.Press("Space")
	app.Expect(b, telatest.Pressed(true))
}

// The aria snapshot of the controls, as Playwright prints them.
func TestToggleControlsAriaSnapshot(t *testing.T) {
	page := func() tela.Node {
		cb := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})
		off := aria.UseCheckbox(aria.ToggleOptions{})
		sw := aria.UseSwitch(aria.ToggleOptions{DefaultSelected: true})
		dis := aria.UseSwitch(aria.ToggleOptions{Disabled: true})
		tb := aria.UseToggleButton(aria.ToggleOptions{DefaultSelected: true})
		btn := aria.UseButton(aria.ButtonOptions{})
		dbtn := aria.UseButton(aria.ButtonOptions{Disabled: true})
		return Div(Class("flex flex-col"),
			Button(cb.Props, AriaLabel("Notifications")),
			Button(off.Props, AriaLabel("Marketing")),
			Button(sw.Props, AriaLabel("Wi-Fi")),
			Button(dis.Props, AriaLabel("Bluetooth")),
			Button(tb.Props, Text("Bold")),
			Div(btn.Props, Text("Save")),
			Div(dbtn.Props, Text("Delete")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	want := `- checkbox "Notifications" [checked]
- checkbox "Marketing"
- switch "Wi-Fi" [checked]
- switch "Bluetooth" [disabled]
- button "Bold" [pressed]
- button "Save"
- button "Delete" [disabled]`
	if got := app.AriaSnapshot(); got != want {
		t.Errorf("snapshot:\n%s\nwant:\n%s", got, want)
	}
}

// UseButton on a Div: role, tab stop, press with the keyboard, and the
// Type keeps a Button from submitting its form.
func TestButton(t *testing.T) {
	var ev events
	page := func() tela.Node {
		btn := aria.UseButton(aria.ButtonOptions{OnPress: func(e aria.PressEvent) { ev.add("press:" + string(e.PointerType)) }})
		return Form(OnSubmit(func() { ev.add("submit") }),
			Div(btn.Props, TestID("div"), Class("h-8 w-20"), Text("Save")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.Click(telatest.ByRole("button", "Save"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("div"))
	app.Press("Enter")
	app.Press("Space")
	if got := ev.take(); got != "press:mouse press:keyboard press:keyboard" {
		t.Errorf("presses: %q", got)
	}
}

func TestButtonInFormDoesNotSubmit(t *testing.T) {
	var ev events
	page := func() tela.Node {
		btn := aria.UseButton(aria.ButtonOptions{OnPress: func(aria.PressEvent) { ev.add("press") }})
		return Form(OnSubmit(func() { ev.add("submit") }),
			Button(btn.Props, Text("Save")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.Click(telatest.ByRole("button", "Save"))
	app.Focus(telatest.ByRole("button", "Save"))
	app.Press("Enter")
	app.Press("Space")
	if got := ev.take(); got != "press press press" {
		t.Errorf("a Button with UseButton in a Form: %q", got)
	}
}
