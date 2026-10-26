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

// shadcn/ui's FieldSet and FieldLegend (new-york-v4/ui/field.tsx at
// kit.ShadcnCommit), verbatim.
const (
	shadcnFieldSet    = "flex flex-col gap-6 has-[>[data-slot=checkbox-group]]:gap-3 has-[>[data-slot=radio-group]]:gap-3"
	shadcnFieldLegend = "mb-3 font-medium data-[variant=legend]:text-base data-[variant=label]:text-sm"
)

// switchRowCell is a horizontal Field with the control after the text:
// shadcn's `<Field orientation="horizontal"><FieldContent>...</FieldContent>
// <Switch /></Field>` (the content is only there with a description).
func switchRowCell(name, label, desc string, on bool) cell {
	return cell{name: name, width: 320, probes: []string{name + "-field", name + "-switch"},
		tela: switchRow(name, label, desc, on, "").tela, html: switchRowHTML(name, label, desc, on)}
}

// switchRow is the Tela side of a switch row.
func switchRow(name, label, desc string, on bool, class string) cell {
	return cell{tela: kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: label, Description: desc,
		Props: h.TestID(name + "-field"), Class: class,
		Children: kit.Switch{DefaultChecked: on, Props: h.TestID(name + "-switch")}}}
}

// switchRowHTML is shadcn's markup of a switch row.
func switchRowHTML(name, label, desc string, on bool) string {
	id := "sr-" + name
	labelHTML := fmt.Sprintf(`<label data-slot="field-label" for=%q class="%s">%s</label>`, id, cn(shadcnLabel, shadcnFieldLabel), label)
	text := labelHTML
	if desc != "" {
		text = fmt.Sprintf(`<div data-slot="field-content" class="%s">%s<p data-slot="field-description" class="%s">%s</p></div>`,
			shadcnFieldContent, labelHTML, cn(shadcnFieldDesc), desc)
	}
	return fmt.Sprintf(`<div role="group" data-slot="field" data-orientation="horizontal" data-probe="%s-field" class="%s">%s%s</div>`,
		name, cn(shadcnFieldHorizontal), text, switchHTML(name+"-switch", id, on, false, ""))
}

// fieldSetCell is a FieldSet whose Tela side is f and whose markup is
// shadcn's FieldSet, FieldLegend, FieldDescription and the children's
// markup, in order.
func fieldSetCell(name string, f kit.FieldSet, children string, probes ...string) cell {
	f.Props = h.TestID(name + "-set")
	f.Class = "w-full"
	variant := "legend"
	if f.LegendVariant == kit.LegendVariantLabel {
		variant = "label"
	}
	var b strings.Builder
	if f.Legend != "" {
		fmt.Fprintf(&b, `<legend data-slot="field-legend" data-variant=%q class="%s">%s</legend>`, variant, cn(shadcnFieldLegend), f.Legend)
	}
	if f.Description != "" {
		fmt.Fprintf(&b, `<p data-slot="field-description" class="%s">%s</p>`, cn(shadcnFieldDesc), f.Description)
	}
	b.WriteString(children)
	return cell{name: name, width: 340, probes: append([]string{name + "-set"}, probes...), tela: f,
		html: fmt.Sprintf(`<fieldset data-slot="field-set" data-probe="%s-set" class="%s">%s</fieldset>`, name, cn(shadcnFieldSet, "w-full"), b.String())}
}

// addressCell is a FieldSet with a legend, a description and two Fields.
func addressCell() cell {
	street := fieldCell("street", kit.Field{Label: "Street"}, kit.Input{Placeholder: "123 Main St"})
	city := fieldCell("city", kit.Field{Label: "City"}, kit.Input{Placeholder: "Springfield"})
	return fieldSetCell("address", kit.FieldSet{Legend: "Address", Description: "Where we send your invoices.",
		Children: h.Fragment(street.tela, city.tela)}, street.html+city.html,
		"street-field", "street-input", "city-field", "city-input")
}

// planRadioCell is a FieldSet holding a RadioGroup, named by the legend.
func planRadioCell(name string, variant kit.LegendVariant, desc string) cell {
	value := "pro"
	group := rgCell(name, rgSpec{items: plans, value: value})
	return fieldSetCell(name, kit.FieldSet{Legend: "Plan", LegendVariant: variant, Description: desc,
		Children: kit.RadioGroup[string]{Items: plans, DefaultValue: &value, Props: h.TestID(name + "-group")}},
		group.html, name+"-group")
}

// notificationsCell is a label-variant FieldSet of two switch rows.
func notificationsCell() cell {
	a, b := switchRow("push", "Push notifications", "", true, ""), switchRow("mail", "Email digest", "Once a week.", false, "")
	return fieldSetCell("notifications", kit.FieldSet{Legend: "Notifications", LegendVariant: kit.LegendVariantLabel,
		Children: h.Fragment(a.tela, b.tela)},
		switchRowHTML("push", "Push notifications", "", true)+switchRowHTML("mail", "Email digest", "Once a week.", false),
		"push-field", "push-switch", "mail-field", "mail-switch")
}

// fieldSetSheet has the Field with the control after the text and the
// FieldSet. The Chromium reference is shadcn's markup with a native
// fieldset and legend.
func fieldSetSheet() *sheet {
	return &sheet{name: "fieldset", cells: []cell{
		switchRowCell("row", "Marketing emails", "News about new features and offers.", true),
		switchRowCell("row-label", "Airplane mode", "", false),
		addressCell(),
		notificationsCell(),
		planRadioCell("plan", kit.LegendVariantLegend, "Choose how you are billed."),
		planRadioCell("plan-label", kit.LegendVariantLabel, ""),
	}}
}

func TestFieldSetGolden(t *testing.T)   { fieldSetSheet().golden(t) }
func TestChromiumFieldSet(t *testing.T) { fieldSetSheet().chromium(t) }

// With ControlAfter the label and the description come first and the
// control last, while the label still names the control and a click on it
// presses it. A vertical field ignores it.
func TestFieldControlAfterOrder(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex w-[320px] flex-col gap-4 p-4"),
			kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "After", Description: "Text first.",
				Children: kit.Switch{Props: h.TestID("after")}},
			kit.Field{Orientation: kit.Horizontal, Label: "Before", Description: "Control first.",
				Children: kit.Switch{Props: h.TestID("before")}},
			kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "Plain",
				Children: kit.Switch{Props: h.TestID("plain")}},
			kit.Field{ControlAfter: true, Label: "Vertical", Children: kit.Input{Props: h.TestID("vertical")}})
	}, telatest.Theme(kit.Theme()), telatest.Size(400, 400))

	text, sw := app.Bounds(telatest.ByText("After")), app.Bounds(telatest.ByTestID("after"))
	if !(text.X < sw.X) || sw.X < 250 {
		t.Errorf("ControlAfter: text at x=%g, switch at x=%g, want the text first and the switch at the end of the row", text.X, sw.X)
	}
	text, sw = app.Bounds(telatest.ByText("Before")), app.Bounds(telatest.ByTestID("before"))
	if !(sw.X < text.X) {
		t.Errorf("default: switch at x=%g, text at x=%g, want the switch first", sw.X, text.X)
	}
	text, sw = app.Bounds(telatest.ByText("Plain")), app.Bounds(telatest.ByTestID("plain"))
	if !(text.X < sw.X) {
		t.Errorf("ControlAfter without a description: text at x=%g, switch at x=%g, want the text first", text.X, sw.X)
	}
	label, input := app.Bounds(telatest.ByText("Vertical")), app.Bounds(telatest.ByTestID("vertical"))
	if !(label.Y < input.Y) {
		t.Errorf("a vertical field with ControlAfter: label at y=%g, input at y=%g, want the label above", label.Y, input.Y)
	}

	// The label still points at the control.
	app.Click(telatest.ByText("After"))
	app.Expect(switchRole("After"), telatest.Checked(true))
	app.Click(telatest.ByText("Plain"))
	app.Expect(switchRole("Plain"), telatest.Checked(true))
}

// An error or Invalid with ControlAfter still marks the control invalid
// and describes it, and the error comes after the label and the
// description, inside the text.
func TestFieldControlAfterInvalidAndError(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex w-[320px] flex-col gap-4 p-4"),
			kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "Terms", Description: "Required.", Error: "Accept to continue.",
				Children: kit.Switch{}},
			kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "Updates", Invalid: true,
				Children: kit.Switch{}})
	}, telatest.Theme(kit.Theme()), telatest.Size(400, 400))
	app.ExpectCount(telatest.ByRole("alert", ""), 1)
	snap := app.AriaSnapshot()
	for _, want := range []string{`switch "Terms" [invalid]`, `switch "Updates" [invalid]`} {
		if !strings.Contains(snap, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, snap)
		}
	}
	if i, j := strings.Index(snap, "Accept to continue."), strings.Index(snap, `switch "Terms"`); i < 0 || j < i {
		t.Errorf("the error is not before its switch in the reading order:\n%s", snap)
	}
}

// ControlAfter mirrors under right to left: the text is at the right edge
// and the switch at the left, and it is the reverse of the default order.
func TestFieldControlAfterRTL(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[320px] p-4"), h.Dir("rtl"),
			kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "Marketing emails", Description: "News.",
				Children: kit.Switch{Props: h.TestID("sw")}})
	}, telatest.Theme(kit.Theme()), telatest.Size(400, 200))
	text, sw := app.Bounds(telatest.ByText("Marketing emails")), app.Bounds(telatest.ByTestID("sw"))
	if !(sw.X < text.X) {
		t.Errorf("rtl: switch at x=%g, text at x=%g, want the switch at the left, after the text in reading order", sw.X, text.X)
	}
	if text.X+text.Width < 290 || sw.X > 40 {
		t.Errorf("rtl: the text ends at x=%g and the switch starts at x=%g, want the text at the right edge of the row and the switch at the left", text.X+text.Width, sw.X)
	}
}

// A FieldSet is a group named by its legend and described by its
// description, with the parts in order.
func TestFieldSetAriaSnapshot(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[360px] p-4"),
			kit.FieldSet{Legend: "Address", Description: "Where we send your invoices.",
				Children: h.Fragment(
					kit.Field{Label: "Street", Children: kit.Input{}},
					kit.Field{Label: "City", Children: kit.Input{}})})
	}, telatest.Theme(kit.Theme()), telatest.Size(400, 400))
	app.ExpectAria(`
- group "Address":
  - text: Address
  - paragraph: Where we send your invoices.
  - group:
    - text: Street
    - textbox "Street"
  - group:
    - text: City
    - textbox "City"
`)
}

// A RadioGroup or a ToggleGroup in a FieldSet is named by the legend, with
// no AriaLabel and no dev warning; an AriaLabel of its own wins.
func TestFieldSetNamesItsGroups(t *testing.T) {
	var logs []string
	app := telatest.Mount(logTB{t, &logs}, h.Div(h.Class("flex w-[360px] flex-col gap-6 p-4"),
		kit.FieldSet{Legend: "Plan", Description: "Choose one.",
			Children: kit.RadioGroup[string]{Items: plans, DefaultValue: new("pro")}},
		kit.FieldSet{Legend: "Density", LegendVariant: kit.LegendVariantLabel,
			Children: kit.ToggleGroup[string]{DisallowEmptySelection: true, DefaultValue: new("a"), Items: []kit.ToggleGroupItem[string]{
				{Value: "a", Label: "Compact"}, {Value: "b", Label: "Roomy"}}}},
		kit.FieldSet{Legend: "Billing",
			Children: kit.RadioGroup[string]{Items: plans, AriaLabel: "Billing period"}},
	), telatest.Theme(kit.Theme()), telatest.Size(400, 700))
	app.ExpectRole("group", "Plan")
	app.ExpectRole("radiogroup", "Plan")
	app.ExpectRole("radiogroup", "Density")
	app.ExpectRole("group", "Billing")
	app.ExpectRole("radiogroup", "Billing period")
	app.ExpectCount(telatest.ByRole("radiogroup", ""), 3)
	for _, l := range logs {
		t.Errorf("dev message: %s", l)
	}
}

// A group inside a Field inside a FieldSet is not named by the legend: the
// Field is the nearer caption.
func TestFieldSetLegendDoesNotReachThroughAField(t *testing.T) {
	app := telatest.Mount(t, kit.FieldSet{Legend: "Wrapper",
		Children: kit.Field{Label: "Plan", Children: kit.RadioGroup[string]{Items: plans, AriaLabel: "Plan"}}},
		telatest.Theme(kit.Theme()), telatest.Size(400, 300))
	app.ExpectRole("radiogroup", "Plan")
	app.ExpectCount(telatest.ByRole("radiogroup", "Wrapper"), 0)

	app = telatest.Mount(t, kit.FieldSet{Legend: "Wrapper",
		Children: kit.Field{Label: "Plan", Children: kit.RadioGroup[string]{Items: plans}}},
		telatest.Theme(kit.Theme()), telatest.Size(400, 300))
	app.ExpectCount(telatest.ByRole("radiogroup", "Wrapper"), 0)
}

// The legend of a label variant is the size of a field's label, and the
// default one is larger; the description is pulled up under a legend only
// of the default variant.
func TestFieldSetLegendVariants(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex w-[360px] flex-col gap-6 p-4"),
			kit.FieldSet{Legend: "Title", Description: "Under a title.", Children: kit.Input{}},
			kit.FieldSet{Legend: "Caption", LegendVariant: kit.LegendVariantLabel, Description: "Under a caption.", Children: kit.Input{}})
	}, telatest.Theme(kit.Theme()), telatest.Size(400, 400))
	title, caption := app.Bounds(telatest.ByText("Title", telatest.Exact())), app.Bounds(telatest.ByText("Caption", telatest.Exact()))
	if title.Height <= caption.Height {
		t.Errorf("the legend is %g px tall and the label variant %g, want the legend taller (text-base against text-sm)", title.Height, caption.Height)
	}
	d1, d2 := app.Bounds(telatest.ByText("Under a title.")), app.Bounds(telatest.ByText("Under a caption."))
	if g1, g2 := d1.Y-(title.Y+title.Height), d2.Y-(caption.Y+caption.Height); g1 >= g2 {
		t.Errorf("the gap under the legend is %g px, and under the label variant %g, want the legend's smaller (-mt-1.5)", g1, g2)
	}
}

func TestFieldSetUndefinedLegendVariant(t *testing.T) {
	_, err := telatest.Render(kit.FieldSet{Legend: "x", LegendVariant: "labell"})
	if err == nil || !strings.Contains(err.Error(), `FieldSet.LegendVariant "labell"`) || !strings.Contains(err.Error(), `did you mean "label"?`) {
		t.Errorf("error %v, want the variant and the closest", err)
	}
	if _, err := telatest.Render(kit.FieldSet{Legend: "x", LegendVariant: "labell"}, telatest.NoDev()); err != nil {
		t.Errorf("without dev mode: %v", err)
	}
}

func ExampleFieldSet() {
	t := &exampleT{}
	defer t.close()
	plan := "free"
	page := func() tela.Node {
		return h.Div(h.Class("p-6"),
			kit.FieldSet{Legend: "Plan", LegendVariant: kit.LegendVariantLabel,
				Children: kit.RadioGroup[string]{Items: plans, Value: &plan, OnValueChange: func(k string) { plan = k }}})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("radio", "Pro"))
	fmt.Println("plan", plan)
	// Output: plan pro
}
