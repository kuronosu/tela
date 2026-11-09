package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func forgotLink() tela.Component {
	return kit.Button{Variant: kit.Link, Size: kit.Sm, Class: "h-auto p-0 text-xs", Children: h.Text("Forgot password?")}
}

func fieldSlotsSheet() *sheet {
	c := func(name string, f kit.Field) cell {
		f.Props = h.TestID(name + "-field")
		return cell{name: name, width: 320, probes: []string{name + "-field"}, tela: f}
	}
	return &sheet{name: "field_slots", width: 720, cells: []cell{
		c("label-end", kit.Field{Label: "Password", LabelEnd: forgotLink(), Children: kit.Input{Type: "password", DefaultValue: "hunter22"}}),
		c("rich-label", kit.Field{Label: "Email", LabelContent: h.Span(h.Class("flex items-center gap-1"),
			h.Text("Email"), h.Span(h.Class("text-destructive"), h.Text("*"))), Children: kit.Input{Type: "email", Placeholder: "m@example.com"}}),
		c("description-end", kit.Field{Label: "Bio", Description: "Tell us a little about yourself.",
			DescriptionEnd: h.Span(h.Class("text-xs text-muted-foreground tabular-nums"), h.Text("24/160")),
			Children:       kit.Textarea{DefaultValue: "Designer and cat person."}}),
		c("rich-description", kit.Field{Label: "Website", DescriptionContent: h.Span(h.Text("Your public page, as in "), h.Span(h.Class("underline underline-offset-4"), h.Text("the guidelines")), h.Text(".")),
			Children: kit.Input{Placeholder: "https://"}}),
		c("all-slots", kit.Field{Label: "Username", LabelEnd: h.Span(h.Class("text-xs text-muted-foreground"), h.Text("Optional")),
			Description: "Letters and digits only.", DescriptionEnd: h.Span(h.Class("text-xs text-muted-foreground"), h.Text("3/20")),
			Children: kit.Input{DefaultValue: "ana"}}),
		c("error", kit.Field{Label: "Password", LabelEnd: forgotLink(), Description: "At least 8 characters.", Error: "Password is too short.",
			Children: kit.Input{Type: "password", DefaultValue: "short"}}),
		c("toggle-group", kit.Field{Label: "Alignment", Description: "How the text is laid out.",
			Children: kit.ToggleGroup[string]{Variant: kit.Outline, DefaultValue: new("left"),
				Items: []kit.ToggleGroupItem[string]{{Value: "left", Label: "Left"}, {Value: "center", Label: "Center"}, {Value: "right", Label: "Right"}}}}),
		c("radio-group", kit.Field{Label: "Plan", Children: kit.RadioGroup[string]{Items: plans, DefaultValue: new("pro")}}),
		c("horizontal", kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "Marketing emails", Description: "News and offers.",
			DescriptionEnd: h.Span(h.Class("text-xs text-muted-foreground"), h.Text("Weekly")),
			Children:       kit.Switch{DefaultChecked: true}}),
	}}
}

func TestFieldSlotsGolden(t *testing.T) { fieldSlotsSheet().golden(t) }

func TestFieldSlotsRTLGolden(t *testing.T) {
	s := fieldSlotsSheet()
	s.name, s.rtl = "field_slots_rtl", true
	s.cells = s.cells[:3]
	s.golden(t)
}

func TestFieldLabelEndIsAtTheEndOfTheLabelLine(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-80"),
			kit.Field{Label: "Password", LabelEnd: h.Span(h.Class("text-xs"), h.Text("Forgot password?"), h.TestID("end")),
				Children: kit.Input{Type: "password", Props: h.TestID("in")}})
	})
	in, end, label := app.Bounds(telatest.ByTestID("in")), app.Bounds(telatest.ByTestID("end")), app.Bounds(telatest.ByText("Password", telatest.Exact()))
	if r := end.X + end.Width; r < in.X+in.Width-0.01 || r > in.X+in.Width+0.01 {
		t.Errorf("the slot ends at %v, the field at %v", r, in.X+in.Width)
	}
	if label.X != in.X {
		t.Errorf("the label starts at %v, the field at %v", label.X, in.X)
	}
	// Level with the label, above the control.
	if d := (end.Y + end.Height/2) - (label.Y + label.Height/2); d < -1 || d > 1 {
		t.Errorf("the slot's center is %v px from the label's", d)
	}
	if end.Y+end.Height > in.Y {
		t.Error("the slot is not above the control")
	}
}

// The slot is outside the label: a press on it neither focuses the control
// nor is the control's name.
func TestFieldLabelEndIsOutsideTheLabel(t *testing.T) {
	var presses int
	app := telatest.Mount(t, kit.Field{Label: "Password",
		LabelEnd: kit.Button{Variant: kit.Link, OnClick: func() { presses++ }, Children: h.Text("Forgot password?")},
		Children: kit.Input{Type: "password"}})
	app.Expect(telatest.ByRole("textbox", "Password"), telatest.HasName("Password"))
	app.Click(telatest.ByRole("button", "Forgot password?"))
	if presses != 1 {
		t.Errorf("the slot's button was pressed %d times", presses)
	}
	app.ExpectFocused(telatest.ByRole("button", "Forgot password?"))
	// The label still focuses the control.
	app.Click(telatest.ByText("Password", telatest.Exact()))
	app.ExpectFocused(telatest.ByRole("textbox", "Password"))
}

func TestFieldRichLabelNamesTheControl(t *testing.T) {
	app := telatest.Mount(t, kit.Field{
		LabelContent: h.Span(h.Text("Email "), h.Span(h.Text("(required)"))),
		Children:     kit.Input{Type: "email"}})
	app.Expect(telatest.ByRole("textbox", "Email (required)"), telatest.HasName("Email (required)"))
	// Clicking the rich label focuses the control.
	app.Click(telatest.ByText("(required)"))
	app.ExpectFocused(telatest.ByRole("textbox", "Email (required)"))
}

func TestFieldRichDescriptionDescribesTheControl(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Website",
		DescriptionContent: h.Span(h.Text("Your public "), h.Strong(h.Text("page"))),
		DescriptionEnd:     h.Span(h.Text("0/20")),
		Children:           kit.Input{}})
	app.ExpectText("Your public")
	app.ExpectText("0/20")
	app.ExpectAria(`- group:
  - text: Website
  - textbox "Website"
  - paragraph:
    - text: Your public
    - strong: page
  - text: 0/20`)
}

// A Field with a group inside names the group by its label, with no
// AriaLabel and no FieldSet.
func TestFieldNamesAGroupInsideIt(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Alignment",
		Children: kit.ToggleGroup[string]{DefaultValue: new("left"),
			Items: []kit.ToggleGroupItem[string]{{Value: "left", Label: "Left"}, {Value: "right", Label: "Right"}}}})
	app.ExpectCount(telatest.ByRole("radiogroup", "Alignment"), 1)
	app = telatest.Mount(t, kit.Field{Label: "Plan", Children: kit.RadioGroup[string]{Items: plans}})
	app.ExpectCount(telatest.ByRole("radiogroup", "Plan"), 1)
	// Several controls of a group each keep their own name.
	app.ExpectCount(telatest.ByRole("radio", "Pro"), 1)
}

func TestFieldNamesAMultipleToggleGroup(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Format",
		Children: kit.ToggleGroup[string]{Multiple: true,
			Items: []kit.ToggleGroupItem[string]{{Value: "b", Label: "Bold"}, {Value: "i", Label: "Italic"}}}})
	app.ExpectCount(telatest.ByRole("toolbar", "Format"), 1)
}

// The group's own AriaLabel wins over the field's label, and a FieldSet's
// legend does not name a group in a Field that has a label.
func TestFieldGroupNameOrder(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Plan", Children: kit.RadioGroup[string]{Items: plans, AriaLabel: "Choose one"}})
	app.ExpectCount(telatest.ByRole("radiogroup", "Choose one"), 1)
	app = telatest.Mount(t, kit.FieldSet{Legend: "Billing", Children: kit.Field{Label: "Plan", Children: kit.RadioGroup[string]{Items: plans}}})
	app.ExpectCount(telatest.ByRole("radiogroup", "Plan"), 1)
	// A Field without a label leaves a FieldSet's legend out of it, as before.
	app = telatest.Mount(t, kit.FieldSet{Legend: "Billing", Children: kit.Field{Children: kit.RadioGroup[string]{Items: plans}}})
	app.ExpectCount(telatest.ByRole("radiogroup", "Billing"), 0)
}

// Nothing changes for a Field that has none of the new slots.
func TestFieldWithoutSlotsIsTheSameTree(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Name", Description: "As on your card.", Children: kit.Input{}})
	app.ExpectAria(`- group:
  - text: Name
  - textbox "Name"
  - paragraph: As on your card.`)
}
