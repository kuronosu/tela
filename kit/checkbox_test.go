package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's Checkbox (new-york-v4/ui/checkbox.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnCheckbox          = "peer size-4 shrink-0 rounded-[4px] border border-input shadow-xs transition-shadow outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground dark:bg-input/30 dark:aria-invalid:ring-destructive/40 dark:data-[state=checked]:bg-primary"
	shadcnCheckboxIndicator = "grid place-content-center text-current transition-none"
)

// svgWithClass is how lucide-react renders an icon given a className.
func svgWithClass(ic tela.Icon, class string) string {
	return strings.Replace(ic.SVG, "<svg ", fmt.Sprintf(`<svg class="lucide %s" `, class), 1)
}

// radixAttrs are the attributes Radix renders on a checkbox, switch or
// radio button.
func radixAttrs(role string, checked, disabled, invalid bool) string {
	state := "unchecked"
	if checked {
		state = "checked"
	}
	a := fmt.Sprintf(`type="button" role=%q aria-checked="%t" data-state=%q`, role, checked, state)
	if disabled {
		a += ` disabled data-disabled=""`
	}
	if invalid {
		a += ` aria-invalid="true"`
	}
	return a
}

// checkboxHTML is the markup of shadcn's Checkbox.
func checkboxHTML(probe string, checked, disabled, invalid bool, class string) string {
	inner := ""
	if checked {
		inner = fmt.Sprintf(`<span data-state="checked" data-slot="checkbox-indicator" class="%s" style="pointer-events: none;">%s</span>`,
			shadcnCheckboxIndicator, svgWithClass(iconCheck, "size-3.5"))
	}
	return fmt.Sprintf(`<button data-probe=%q %s data-slot="checkbox" class="%s">%s</button>`,
		probe, radixAttrs("checkbox", checked, disabled, invalid), cn(shadcnCheckbox, class), inner)
}

func checkboxCell(name string, c kit.Checkbox) cell {
	checked := c.DefaultChecked
	c.Props = h.TestID(name + "-box")
	return cell{name: name, probes: []string{name + "-box"}, tela: c,
		html: checkboxHTML(name+"-box", checked, c.Disabled, c.Invalid, c.Class)}
}

// labelledCheckbox is a checkbox with its label in a horizontal Field, as
// shadcn's documentation writes it.
func labelledCheckbox(name, text string, c kit.Checkbox) cell {
	checked := c.DefaultChecked
	c.Props = h.TestID(name + "-box")
	id := "f-" + name
	box := strings.Replace(checkboxHTML(name+"-box", checked, c.Disabled, c.Invalid, ""), "<button ", fmt.Sprintf("<button id=%q ", id), 1)
	return cell{name: name, width: 260, probes: []string{name + "-box"},
		tela: kit.Field{Orientation: kit.Horizontal, Label: text, Children: c},
		html: fmt.Sprintf(`<div role="group" data-slot="field" data-orientation="horizontal" class="%s">%s<label data-slot="field-label" for=%q class="%s">%s</label></div>`,
			cn(shadcnFieldHorizontal), box, id, cn(shadcnLabel, shadcnFieldLabel), text)}
}

func checkboxSheet() *sheet {
	return &sheet{name: "checkbox", slot: 280, mean: [2]float64{3.5, 6}, cells: []cell{
		checkboxCell("unchecked", kit.Checkbox{}),
		checkboxCell("checked", kit.Checkbox{DefaultChecked: true}),
		checkboxCell("disabled", kit.Checkbox{Disabled: true}),
		checkboxCell("disabled-checked", kit.Checkbox{Disabled: true, DefaultChecked: true}),
		checkboxCell("invalid", kit.Checkbox{Invalid: true}),
		checkboxCell("invalid-checked", kit.Checkbox{Invalid: true, DefaultChecked: true}),
		checkboxCell("class", kit.Checkbox{DefaultChecked: true, Class: "size-6 rounded-full"}),
		labelledCheckbox("label", "Accept terms", kit.Checkbox{}),
		labelledCheckbox("label-checked", "Email me updates", kit.Checkbox{DefaultChecked: true}),
		labelledCheckbox("label-disabled", "Not available", kit.Checkbox{Disabled: true}),
	}}
}

func TestCheckboxGolden(t *testing.T)   { checkboxSheet().golden(t) }
func TestChromiumCheckbox(t *testing.T) { checkboxSheet().chromium(t) }

func checkbox(name string) telatest.Locator { return telatest.ByRole("checkbox", name) }

func TestCheckboxUncontrolled(t *testing.T) {
	var got []bool
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(kit.Checkbox{AriaLabel: "Agree", OnCheckedChange: func(b bool) { got = append(got, b) }},
			kit.Checkbox{AriaLabel: "Started on", DefaultChecked: true})
	})
	app.Expect(checkbox("Agree"), telatest.Checked(false))
	app.Expect(checkbox("Started on"), telatest.Checked(true))
	app.Click(checkbox("Agree"))
	app.Expect(checkbox("Agree"), telatest.Checked(true))
	app.Focus(checkbox("Agree"))
	app.Press("Space")
	app.Expect(checkbox("Agree"), telatest.Checked(false))
	app.Press("Enter") // a checkbox is not toggled by Enter, as in HTML
	app.Expect(checkbox("Agree"), telatest.Checked(false))
	if fmt.Sprint(got) != "[true false]" {
		t.Errorf("OnCheckedChange got %v, want [true false]", got)
	}
}

// controlledCheckbox keeps its state in the caller, which can refuse.
type controlledCheckbox struct{ refuse bool }

func (c controlledCheckbox) Build() tela.Node {
	on, setOn := tela.UseState(false)
	return h.Div(kit.Checkbox{AriaLabel: "Notify", Checked: &on, OnCheckedChange: func(b bool) {
		if !c.refuse {
			setOn.Set(b)
		}
	}}, h.Span(h.Class("sr-only"), h.TestID("state"), h.Text(fmt.Sprint(on))))
}

func TestCheckboxControlled(t *testing.T) {
	app := telatest.Mount(t, controlledCheckbox{})
	app.Click(checkbox("Notify"))
	app.Expect(checkbox("Notify"), telatest.Checked(true))
	app.ExpectText("true")
	refused := telatest.Mount(t, controlledCheckbox{refuse: true})
	refused.Click(checkbox("Notify"))
	refused.Expect(checkbox("Notify"), telatest.Checked(false))
}

func TestCheckboxDisabled(t *testing.T) {
	changed := false
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.Checkbox{AriaLabel: "Locked", Disabled: true, OnCheckedChange: func(bool) { changed = true }})
	})
	app.Expect(checkbox("Locked"), telatest.Disabled(true))
	b := app.Bounds(checkbox("Locked"))
	app.ClickAt(b.X+b.Width/2, b.Y+b.Height/2)
	app.Expect(checkbox("Locked"), telatest.Checked(false))
	if changed {
		t.Error("a disabled checkbox reported a change")
	}
}

func TestCheckboxInvalidAndRequired(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.Checkbox{AriaLabel: "Terms", Invalid: true, Required: true},
			kit.Field{Orientation: kit.Horizontal, Label: "Privacy", Error: "You must accept.", Children: kit.Checkbox{}})
	})
	app.Expect(checkbox("Terms"), telatest.Invalid(true))
	app.Expect(checkbox("Privacy"), telatest.Invalid(true))
	if snap := app.AriaSnapshot(); !strings.Contains(snap, "checkbox \"Terms\" [invalid]") {
		t.Errorf("the snapshot lacks the invalid checkbox:\n%s", snap)
	}
	if n := app.Count(telatest.ByClass("aria-invalid:border-destructive")); n != 2 {
		t.Errorf("%d checkboxes have the invalid classes, want 2", n)
	}
}

func TestCheckboxLabelAndField(t *testing.T) {
	var on bool
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.Checkbox{ID: "terms", OnCheckedChange: func(b bool) { on = b }},
			kit.Label{For: "terms", Children: h.Text("Accept terms")},
			kit.Field{Orientation: kit.Horizontal, Label: "Newsletter", Description: "Once a month.", Children: kit.Checkbox{}})
	})
	// The label names the checkbox and clicking it toggles it.
	app.Expect(checkbox("Accept terms"), telatest.Checked(false))
	app.Click(telatest.ByText("Accept terms"))
	app.Expect(checkbox("Accept terms"), telatest.Checked(true))
	if !on {
		t.Error("clicking the label did not toggle the checkbox")
	}
	// A field names it and describes it.
	app.Click(telatest.ByText("Newsletter"))
	app.Expect(checkbox("Newsletter"), telatest.Checked(true))
}

func TestCheckboxSlots(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.Checkbox{AriaLabel: "x", IndicatorClass: "opacity-90", Icon: iconX})
	})
	if n := app.Count(telatest.ByClass("opacity-90")); n != 0 {
		t.Errorf("an unchecked checkbox has %d indicators, want none", n)
	}
	app.Click(checkbox("x"))
	if n := app.Count(telatest.ByClass("opacity-90")); n != 1 {
		t.Errorf("a checked checkbox has %d indicators, want 1", n)
	}
}

func TestCheckboxAriaSnapshot(t *testing.T) {
	page := func() tela.Node {
		return h.Div(
			kit.Checkbox{AriaLabel: "Agree"},
			kit.Checkbox{AriaLabel: "Notify", DefaultChecked: true},
			kit.Checkbox{AriaLabel: "Locked", Disabled: true},
			kit.Checkbox{AriaLabel: "Terms", Invalid: true})
	}
	got, err := telatest.AriaSnapshot(tela.C(page))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`checkbox "Agree"`, `checkbox "Notify" [checked]`, `checkbox "Locked" [disabled]`, `checkbox "Terms" [invalid]`} {
		if !strings.Contains(got, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, got)
		}
	}
}

func ExampleCheckbox() {
	t := &exampleT{}
	defer t.close()
	agreed := false
	page := func() tela.Node {
		return h.Div(h.Class("flex items-center gap-3"),
			kit.Checkbox{ID: "terms", OnCheckedChange: func(b bool) { agreed = b }},
			kit.Label{For: "terms", Children: h.Text("Accept the terms")})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByText("Accept the terms")) // the label toggles the checkbox
	fmt.Println("agreed", agreed)
	// Output: agreed true
}
