package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

var plans = []string{"free", "pro", "team", "enterprise"}

// radioPage is a radio group of buttons, each with a Label.
func radioPage(o aria.RadioGroupOptions[string]) func() tela.Node {
	return func() tela.Node { return useRadioPage(o) }
}

// useRadioPage is the body of radioPage, a custom hook so that a test can
// call it from its own component.
func useRadioPage(o aria.RadioGroupOptions[string]) tela.Node {
	o.Keys, o.AriaLabel = plans, "Plan"
	g := aria.UseRadioGroup(o)
	cls := "flex flex-col gap-1"
	if o.Orientation == "horizontal" {
		cls = "flex gap-2"
	}
	return Div(g.Props, Class(cls),
		Range(len(plans), func(i int) tela.Node {
			k := plans[i]
			return Div(Key(k), Class("flex items-center gap-2"),
				Button(g.RadioProps(k), Class("size-4 rounded-full border checked:size-5")),
				Label(For(g.RadioID(k)), Text(capital(k))))
		}))
}

func radio(name string) telatest.Locator { return telatest.ByRole("radio", name) }

func TestRadioGroupUncontrolled(t *testing.T) {
	var ev events
	o := aria.RadioGroupOptions[string]{OnValueChange: func(k string) { ev.add(k) }}
	app := telatest.MountFunc(t, radioPage(o), telatest.Size(300, 200))
	// Nothing is checked.
	for _, p := range plans {
		app.Expect(radio(capital(p)), telatest.Checked(false))
	}
	app.Click(radio("Pro"))
	app.Expect(radio("Pro"), telatest.Checked(true))
	app.Expect(radio("Free"), telatest.Checked(false))
	// A click on the checked radio changes nothing, and reports nothing.
	app.Click(radio("Pro"))
	app.Click(radio("Team"))
	app.Expect(radio("Team"), telatest.Checked(true))
	app.Expect(radio("Pro"), telatest.Checked(false))
	if got := ev.take(); got != "pro team" {
		t.Errorf("OnValueChange saw %q, want %q", got, "pro team")
	}
	// The label checks its radio.
	app.Click(telatest.ByText("Free"))
	app.Expect(radio("Free"), telatest.Checked(true))
}

func TestRadioGroupDefaultValue(t *testing.T) {
	app := telatest.MountFunc(t, radioPage(aria.RadioGroupOptions[string]{DefaultValue: new("team")}), telatest.Size(300, 200))
	app.Expect(radio("Team"), telatest.Checked(true))
	app.ExpectAria(`
		- radiogroup "Plan":
		  - radio "Free"
		  - text: Free
		  - radio "Pro"
		  - text: Pro
		  - radio "Team" [checked]
		  - text: Team
		  - radio "Enterprise"
		  - text: Enterprise
	`)
}

// Tab enters at the checked radio; with none checked, at the first one.
// The arrows move the focus and check, wrapping, skipping disabled radios.
func TestRadioGroupKeyboard(t *testing.T) {
	o := aria.RadioGroupOptions[string]{DisabledKeys: []string{"team"}}
	app := telatest.MountFunc(t, radioPage(o), telatest.Size(300, 200))
	app.Press("Tab")
	app.ExpectFocused(radio("Free"))
	app.Expect(radio("Free"), telatest.Checked(false)) // focusing does not check
	app.Press("ArrowDown")
	app.ExpectFocused(radio("Pro"))
	app.Expect(radio("Pro"), telatest.Checked(true))
	app.Press("ArrowDown") // team is disabled
	app.ExpectFocused(radio("Enterprise"))
	app.Expect(radio("Enterprise"), telatest.Checked(true))
	app.Press("ArrowRight") // all four arrows move, in a vertical group too
	app.ExpectFocused(radio("Free"))
	app.Expect(radio("Free"), telatest.Checked(true))
	app.Press("ArrowUp")
	app.ExpectFocused(radio("Enterprise"))
	app.Press("ArrowLeft")
	app.ExpectFocused(radio("Pro"))
	app.Expect(radio("Pro"), telatest.Checked(true))
	// Tab leaves the group, and Shift+Tab comes back to the checked radio.
	app.Press("Tab")
	app.Press("Shift+Tab")
	app.ExpectFocused(radio("Pro")) // the one checked is the tab stop
	app.Expect(radio("Team"), telatest.Disabled(true))
}

func TestRadioGroupTabEntersAtTheChecked(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Button(TestID("before"), Text("Before")), tela.C(radioPage(aria.RadioGroupOptions[string]{DefaultValue: new("team")})))
	}, telatest.Size(300, 200))
	app.Focus(telatest.ByTestID("before"))
	app.Press("Tab")
	app.ExpectFocused(radio("Team"))
	app.Press("Tab") // the group is one tab stop: Tab leaves it
	app.Press("Shift+Tab")
	app.ExpectFocused(radio("Team"))
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByTestID("before"))
}

func TestRadioGroupSpace(t *testing.T) {
	var ev events
	o := aria.RadioGroupOptions[string]{OnValueChange: func(k string) { ev.add(k) }}
	app := telatest.MountFunc(t, radioPage(o), telatest.Size(300, 200))
	app.Focus(radio("Team"))
	app.Press("Enter")
	app.Expect(radio("Team"), telatest.Checked(false))
	app.Press("Space")
	app.Expect(radio("Team"), telatest.Checked(true))
	if got := ev.take(); got != "team" {
		t.Errorf("OnValueChange saw %q, want team", got)
	}
}

// RTL swaps the horizontal arrows of a horizontal group; a vertical group
// keeps ArrowRight as next.
func TestRadioGroupRTL(t *testing.T) {
	o := aria.RadioGroupOptions[string]{Direction: "rtl", Orientation: "horizontal", DefaultValue: new("pro")}
	app := telatest.MountFunc(t, radioPage(o), telatest.Size(400, 100))
	app.Focus(radio("Pro"))
	app.Press("ArrowRight")
	app.ExpectFocused(radio("Free"))
	app.Press("ArrowLeft")
	app.ExpectFocused(radio("Pro"))
	app.Press("ArrowDown") // up and down are not mirrored
	app.ExpectFocused(radio("Team"))

	v := aria.RadioGroupOptions[string]{Direction: "rtl", DefaultValue: new("pro")}
	app = telatest.MountFunc(t, radioPage(v), telatest.Size(400, 200))
	app.Focus(radio("Pro"))
	app.Press("ArrowRight")
	app.ExpectFocused(radio("Team"))
}

func TestRadioGroupControlled(t *testing.T) {
	var asked []string
	var set tela.Setter[string]
	page := func() tela.Node {
		v, s := tela.UseState("pro")
		set = s
		return useRadioPage(aria.RadioGroupOptions[string]{Value: &v, OnValueChange: func(k string) { asked = append(asked, k); s.Set(k) }})
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Expect(radio("Pro"), telatest.Checked(true))
	app.Click(radio("Team"))
	app.Expect(radio("Team"), telatest.Checked(true))
	set.Set("free")
	app.Expect(radio("Free"), telatest.Checked(true))
	app.Expect(radio("Team"), telatest.Checked(false))
	if len(asked) != 1 || asked[0] != "team" {
		t.Errorf("OnValueChange saw %v, want [team]", asked)
	}
}

// A controlled group shows what the caller passes: a value it does not
// store is refused, and a value that is not a key checks nothing.
func TestRadioGroupControlledRefuses(t *testing.T) {
	var asked []string
	v := "pro"
	o := aria.RadioGroupOptions[string]{Value: &v, OnValueChange: func(k string) { asked = append(asked, k) }}
	app := telatest.MountFunc(t, radioPage(o), telatest.Size(300, 200))
	app.Click(radio("Team"))
	app.Expect(radio("Team"), telatest.Checked(false))
	app.Expect(radio("Pro"), telatest.Checked(true))
	app.Focus(radio("Pro"))
	app.Press("ArrowDown")
	app.ExpectFocused(radio("Team")) // the focus moves, the value does not
	app.Expect(radio("Pro"), telatest.Checked(true))
	if len(asked) != 2 || asked[0] != "team" || asked[1] != "team" {
		t.Errorf("OnValueChange saw %v, want [team team]", asked)
	}

	none := ""
	app = telatest.MountFunc(t, radioPage(aria.RadioGroupOptions[string]{Value: &none}), telatest.Size(300, 200))
	for _, p := range plans {
		app.Expect(radio(capital(p)), telatest.Checked(false))
	}
}

func TestRadioGroupDisabledAndReadOnly(t *testing.T) {
	var ev events
	cb := func(k string) { ev.add(k) }
	app := telatest.MountFunc(t, radioPage(aria.RadioGroupOptions[string]{Disabled: true, OnValueChange: cb}), telatest.Size(300, 200))
	for _, p := range plans {
		app.Expect(radio(capital(p)), telatest.Disabled(true))
	}
	if ev.take() != "" {
		t.Error("a disabled group reported a change")
	}

	app = telatest.MountFunc(t, radioPage(aria.RadioGroupOptions[string]{ReadOnly: true, DefaultValue: new("pro"), OnValueChange: cb}), telatest.Size(300, 200))
	app.Click(radio("Team"))
	app.Expect(radio("Pro"), telatest.Checked(true))
	app.Focus(radio("Pro"))
	app.Press("ArrowDown")
	app.ExpectFocused(radio("Team"))
	app.Press("Space")
	app.Expect(radio("Pro"), telatest.Checked(true))
	if ev.take() != "" {
		t.Error("a read-only group reported a change")
	}
}

// A radio made with a Div works too (the hook adds no element kind).
func TestRadioGroupDivRadios(t *testing.T) {
	page := func() tela.Node {
		g := aria.UseRadioGroup(aria.RadioGroupOptions[string]{Keys: plans, AriaLabel: "Plan"})
		return Div(g.Props, Class("flex flex-col"),
			Range(len(plans), func(i int) tela.Node {
				return Div(g.RadioProps(plans[i]), Key(plans[i]), Class("h-6 w-32"), Text(capital(plans[i])))
			}))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Click(radio("Pro"))
	app.Expect(radio("Pro"), telatest.Checked(true))
	app.Press("ArrowDown")
	app.ExpectFocused(radio("Team"))
	app.Expect(radio("Team"), telatest.Checked(true))
}

// A radio made with a Button in a Form is not the form's default button:
// Enter in a text field submits the form, and pressing a radio does not.
func TestRadioGroupInForm(t *testing.T) {
	var ev events
	page := func() tela.Node {
		g := aria.UseRadioGroup(aria.RadioGroupOptions[string]{Keys: plans, AriaLabel: "Plan", OnValueChange: func(k string) { ev.add("plan " + k) }})
		return Form(OnSubmit(func() { ev.add("submit") }),
			Div(g.Props, Class("flex flex-col"),
				Range(len(plans), func(i int) tela.Node {
					return Button(g.RadioProps(plans[i]), Key(plans[i]), Class("h-6 w-32"), Text(capital(plans[i])))
				})),
			Input(TestID("name"), Class("h-8 w-40")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	app.Click(radio("Pro"))
	if got := ev.take(); got != "plan pro" {
		t.Errorf("clicking a radio: %q, want %q (no submit)", got, "plan pro")
	}
	app.Focus(radio("Team"))
	app.Press("Space")
	app.Press("Enter")
	if got := ev.take(); got != "plan team" {
		t.Errorf("Space and Enter on a radio: %q, want %q (no submit)", got, "plan team")
	}
	app.Focus(telatest.ByTestID("name"))
	app.Press("Enter")
	if got := ev.take(); got != "submit" {
		t.Errorf("Enter in the field: %q, want submit (a radio is not the default button)", got)
	}
}

func TestRadioGroupAriaSnapshot(t *testing.T) {
	o := aria.RadioGroupOptions[string]{DefaultValue: new("pro"), DisabledKeys: []string{"enterprise"}, Required: true}
	app := telatest.MountFunc(t, radioPage(o), telatest.Size(300, 200))
	app.ExpectAria(`
		- radiogroup "Plan":
		  - radio "Free"
		  - text: Free
		  - radio "Pro" [checked]
		  - text: Pro
		  - radio "Team"
		  - text: Team
		  - radio "Enterprise" [disabled]
		  - text: Enterprise
	`)
}

// The group is named by the element with LabelProps.
func TestRadioGroupLabel(t *testing.T) {
	page := func() tela.Node {
		g := aria.UseRadioGroup(aria.RadioGroupOptions[string]{Keys: plans})
		return Div(
			Span(g.LabelProps, Text("Billing plan")),
			Div(g.Props, Range(len(plans), func(i int) tela.Node {
				return Button(g.RadioProps(plans[i]), Key(plans[i]), Text(capital(plans[i])))
			})))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.ExpectRole("radiogroup", "Billing plan")
}

func TestRadioGroupTouchAndVirtualPress(t *testing.T) {
	var ev events
	o := aria.RadioGroupOptions[string]{OnValueChange: func(k string) { ev.add(k) }}
	a := mountRaw(t, tela.C(radioPage(o)), 300, 200)
	b := a.tree.Semantics()
	node := findNode(b, "radio", "Team")
	if node == nil {
		t.Fatalf("no Team radio:\n%s", a.tree.AriaSnapshot())
	}
	r := node.Bounds
	a.tap(platform.PointerTouch, (r.X0+r.X1)/2, (r.Y0+r.Y1)/2)
	if got := ev.take(); got != "team" {
		t.Errorf("a tap: %q, want team", got)
	}
	a.clickNode("radio", "Pro")
	if got := ev.take(); got != "pro" {
		t.Errorf("the semantics click: %q, want pro", got)
	}
}

func TestRadioGroupOptionErrors(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		for _, want := range []string{"Orientation", `"horizontl"`, `did you mean "horizontal"`} {
			if !strings.Contains(msg, want) {
				t.Errorf("message %q lacks %q", msg, want)
			}
		}
	}()
	telatest.MountFunc(t, radioPage(aria.RadioGroupOptions[string]{Orientation: "horizontl"}), telatest.Size(100, 100))
}
