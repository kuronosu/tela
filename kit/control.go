package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// This file holds what the form controls share: the context a [Field]
// gives its control, and the props the text fields have in common.

// fieldInfo is what a [Field] tells its control: the id its label points
// to and the element that describes it.
type fieldInfo struct {
	id          string
	describedBy tela.ElementRef
}

// fieldCtx carries the enclosing Field's wiring to the control below it.
// Outside a Field it is the zero fieldInfo: the control keeps its own id
// and has no description.
var fieldCtx = tela.CreateContext(fieldInfo{})

// textFieldProps are the props Input and Textarea share.
type textFieldProps struct {
	value        *string
	defaultValue string
	onChange     func(string)
	placeholder  string
	name, id     string
	disabled     bool
	readOnly     bool
	autoFocus    bool
	maxLength    int
}

// useTextField returns the props of a text field: its value, handlers and
// states, with the id and description of the enclosing [Field] when there
// is one. It is a hook (it reads the field's context).
func useTextField(p textFieldProps) []tela.Child {
	var kids []tela.Child
	switch {
	case p.value != nil:
		kids = append(kids, h.Value(*p.value))
	case p.defaultValue != "":
		kids = append(kids, h.DefaultValue(p.defaultValue))
	}
	if p.onChange != nil {
		on := p.onChange
		kids = append(kids, h.OnChange(func(e tela.ChangeEvent) { on(e.Value) }))
	}
	if p.placeholder != "" {
		kids = append(kids, h.Placeholder(p.placeholder))
	}
	if p.name != "" {
		kids = append(kids, h.Name(p.name))
	}
	fc := tela.UseContext(fieldCtx)
	id := p.id
	if id == "" {
		id = fc.id
	}
	if id != "" {
		kids = append(kids, h.ID(id))
	}
	if fc.describedBy != (tela.ElementRef{}) {
		kids = append(kids, h.AriaDescribedBy(fc.describedBy))
	}
	if p.disabled {
		kids = append(kids, h.Disabled(true))
	}
	if p.readOnly {
		kids = append(kids, h.ReadOnly(true))
	}
	if p.autoFocus {
		kids = append(kids, h.AutoFocus(true))
	}
	if p.maxLength > 0 {
		kids = append(kids, h.MaxLength(p.maxLength))
	}
	return kids
}
