package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// InputStyle is the recipe of [Input] (shadcn/ui's Input).
type InputStyle struct {
	Root string
}

func init() {
	defaultStyle.Input = InputStyle{
		Root: "h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none " +
			"selection:bg-primary selection:text-primary-foreground placeholder:text-muted-foreground " +
			"disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30 " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
	}
}

// Input is a single-line text field, shadcn/ui's Input. Its value follows
// the controlled and uncontrolled rule of the whole kit: give Value (a
// pointer to the state) and OnValueChange to keep the value yourself, or
// DefaultValue to let the field keep it.
//
//	name, setName := tela.UseState("")
//	kit.Input{Placeholder: "Name", Value: &name, OnValueChange: setName.Set}
//
//	kit.Input{Type: "email", DefaultValue: "ana@example.com", Name: "email"}
//
// Inside a [Field] the input takes the field's ID, so the field's label
// names it, and is described by the field's description or error.
type Input struct {
	// Value points to the text of a controlled field; nil leaves the text
	// to the field, which starts with DefaultValue.
	Value *string
	// DefaultValue is the initial text of an uncontrolled field.
	DefaultValue string
	// OnValueChange is called with the new text after every edit. For a
	// controlled field it is how the change is asked for.
	OnValueChange func(string)
	// Type is "text" (the default), "password", "email", "search", "url",
	// "tel" or "number" (h.Type).
	Type        string
	Placeholder string
	// Name names the field in its h.Form.
	Name string
	// ID is the field's id, for a [Label]'s For. Inside a [Field] it
	// defaults to the field's.
	ID        string
	Disabled  bool
	ReadOnly  bool
	AutoFocus bool
	// MaxLength limits what the user can type, in UTF-16 code units; 0 is
	// no limit.
	MaxLength int
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the input element.
	Props tela.Child
}

// Build returns the input element.
func (i Input) Build() tela.Node {
	s := &UseStyle().Input
	kids := []tela.Child{h.Class(s.Root, i.Class), i.Props}
	if i.Type != "" {
		kids = append(kids, h.Type(i.Type))
	}
	kids = append(kids, useTextField(textFieldProps{
		value: i.Value, defaultValue: i.DefaultValue, onChange: i.OnValueChange,
		placeholder: i.Placeholder, name: i.Name, id: i.ID,
		disabled: i.Disabled, readOnly: i.ReadOnly, autoFocus: i.AutoFocus, maxLength: i.MaxLength,
	})...)
	return h.Input(kids...)
}
