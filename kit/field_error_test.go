package kit_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func fieldErrorSheet() *sheet {
	c := func(name string, f kit.Field) cell {
		f.Props = h.TestID(name + "-field")
		return cell{name: name, width: 320, probes: []string{name + "-field"}, tela: f}
	}
	return &sheet{name: "field_error", width: 720, cells: []cell{
		c("plain", kit.Field{Label: "Username", Error: "That name is taken.", Children: kit.Input{DefaultValue: "ana"}}),
		c("content", kit.Field{Label: "Username",
			ErrorContent: h.Span(h.Text("That name is taken. "), h.Span(h.Class("underline underline-offset-4"), h.Text("See suggestions"))),
			Children:     kit.Input{DefaultValue: "ana"}}),
		c("list", kit.Field{Label: "Password", Description: "Pick a strong one.",
			ErrorContent: h.Div(h.Class("flex flex-col"), h.Div(h.Text("At least 8 characters")), h.Div(h.Text("One digit"))),
			Children:     kit.Input{Type: "password", DefaultValue: "abc"}}),
		c("horizontal", kit.Field{Orientation: kit.Horizontal, Label: "Accept the terms",
			ErrorContent: h.Span(h.Text("You must accept to continue.")),
			Children:     kit.Checkbox{}}),
	}}
}

func TestFieldErrorContentGolden(t *testing.T) { fieldErrorSheet().golden(t) }

func TestFieldErrorContentRTLGolden(t *testing.T) {
	s := fieldErrorSheet()
	s.name, s.rtl = "field_error_rtl", true
	s.cells = s.cells[:3]
	s.golden(t)
}

// ErrorContent is an error like Error: the field is invalid, the message is
// an alert, and it describes the control.
func TestFieldErrorContentIsAnError(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Username", Description: "Public name.",
		ErrorContent: h.Span(h.Text("Taken. "), h.A(h.Text("Suggestions"))),
		Children:     kit.Input{}})
	in := telatest.ByRole("textbox", "Username")
	app.Expect(in, telatest.Invalid(true))
	app.ExpectCount(telatest.ByRole("alert", ""), 1)
	// With both, the content is drawn and Error is not.
	app = telatest.Mount(t, kit.Field{Label: "Username", Error: "plain", ErrorContent: h.Text("rich"), Children: kit.Input{}})
	app.ExpectCount(telatest.ByText("rich"), 1)
	app.ExpectCount(telatest.ByText("plain"), 0)
}

// A Field's label above a group points its For at no control: the group is
// labelled by it instead, and dev mode must not warn that it names nothing.
func TestFieldAroundAGroupDoesNotWarn(t *testing.T) {
	for name, group := range map[string]func() tela.Component{
		"radiogroup": func() tela.Component {
			return kit.RadioGroup[string]{Items: plans, DefaultValue: new("pro")}
		},
		"togglegroup": func() tela.Component {
			return kit.ToggleGroup[string]{DefaultValue: new("a"), Items: []kit.ToggleGroupItem[string]{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}}
		},
	} {
		var logs []string
		telatest.Mount(logTB{t, &logs}, kit.Field{Label: "Choice", Children: group()}, telatest.Dev())
		for _, l := range logs {
			if strings.Contains(l, "names no control") || strings.Contains(l, "labels no control") {
				t.Errorf("%s in a Field: %s", name, l)
			}
		}
	}
	// A label that names nothing still warns.
	var logs []string
	telatest.Mount(logTB{t, &logs}, h.Div(kit.Label{For: "nope", Children: h.Text("Orphan")}), telatest.Dev())
	found := false
	for _, l := range logs {
		found = found || strings.Contains(l, "names no control")
	}
	if !found {
		t.Errorf("an orphan label: no warning in %q", logs)
	}
}
