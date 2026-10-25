package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// TextareaStyle is the recipe of [Textarea] (shadcn/ui's Textarea).
type TextareaStyle struct {
	Root string
}

func init() {
	defaultStyle.Textarea = TextareaStyle{
		Root: "flex field-sizing-content min-h-16 w-full rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-[color,box-shadow] outline-none " +
			"placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 " +
			"disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 md:text-sm dark:bg-input/30 dark:aria-invalid:ring-destructive/40",
	}
}

// Textarea is a multi-line text field, shadcn/ui's Textarea. It grows with
// its text (field-sizing-content) from a minimum height of 64 px, and
// scrolls once a max-h-* class in Class caps it. Its value follows the
// same rule as [Input]'s.
//
//	kit.Textarea{Placeholder: "Write a message", Value: &msg, OnValueChange: setMsg.Set}
type Textarea struct {
	// Value points to the text of a controlled field; nil leaves the text
	// to the field, which starts with DefaultValue.
	Value        *string
	DefaultValue string
	// OnValueChange is called with the new text after every edit.
	OnValueChange func(string)
	Placeholder   string
	Name          string
	// ID is the field's id, for a [Label]'s For. Inside a [Field] it
	// defaults to the field's.
	ID string
	// Rows is the number of lines the field is tall (h.Rows); 0 leaves the
	// recipe's minimum height and the text to size it.
	Rows     int
	Disabled bool
	ReadOnly bool
	// Invalid marks the value as wrong (see [Input]); a [Field] with an
	// Error sets it.
	Invalid   bool
	AutoFocus bool
	MaxLength int
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the textarea element.
	Props tela.Child
}

// Build returns the textarea element.
func (t Textarea) Build() tela.Node {
	s := &UseStyle().Textarea
	kids := []tela.Child{cls(s.Root, t.Class), t.Props}
	if t.Rows > 0 {
		kids = append(kids, h.Rows(t.Rows))
	}
	kids = append(kids, useTextField(textFieldProps{
		value: t.Value, defaultValue: t.DefaultValue, onChange: t.OnValueChange,
		placeholder: t.Placeholder, name: t.Name, id: t.ID,
		disabled: t.Disabled, readOnly: t.ReadOnly, invalid: t.Invalid, autoFocus: t.AutoFocus, maxLength: t.MaxLength,
	})...)
	return h.Textarea(kids...)
}
