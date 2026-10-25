package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// LabelStyle is the recipe of [Label] (shadcn/ui's Label).
type LabelStyle struct {
	Root string
	// Disabled is added when the label is Disabled (shadcn's
	// group-data-[disabled=true]:).
	Disabled string
}

func init() {
	defaultStyle.Label = LabelStyle{
		Root:     "flex items-center gap-2 text-sm leading-none font-medium select-none peer-disabled:cursor-not-allowed peer-disabled:opacity-50",
		Disabled: "pointer-events-none opacity-50",
	}
}

// Label is the caption of a form control, shadcn/ui's Label. Clicking it
// focuses the control it labels, which also gets its accessible name from
// it. It labels the control whose ID is its For, or else the field or button
// inside it.
//
//	kit.Label{For: "email", Children: h.Text("Email")}
//	kit.Input{ID: "email", Type: "email"}
//
// A [Field] wires a label and its control with no ids to write.
type Label struct {
	// For is the ID of the control the label names (h.For).
	For string
	// Disabled dims the label and makes it ignore the pointer; a control
	// that is a sibling after a label with the class "peer" dims it with
	// peer-disabled: without it.
	Disabled bool
	// Children is the text.
	Children tela.Component
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the label element.
	Props tela.Child
}

// Build returns the label element.
func (l Label) Build() tela.Node {
	s := &UseStyle().Label
	kids := []tela.Child{cls(s.Root, when(l.Disabled, s.Disabled), l.Class), l.Props}
	if l.For != "" {
		kids = append(kids, h.For(l.For))
	}
	kids = append(kids, l.Children)
	return h.Label(kids...)
}
