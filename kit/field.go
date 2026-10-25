package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// FieldStyle is the recipe of [Field] (shadcn/ui's Field, FieldLabel,
// FieldContent, FieldDescription and FieldError).
type FieldStyle struct {
	Root string
	// Orientations are the arrangement of the control and its text:
	// Vertical (the label above the control) and Horizontal (the control
	// first, then the text).
	Orientations map[Orientation]string
	// HorizontalWithContent is added to Root when a Horizontal field has a
	// Content part (shadcn's has-[>[data-slot=field-content]]:).
	HorizontalWithContent string
	// Label is added to the [Label] recipe's classes for the field's label.
	Label string
	// HorizontalLabel is added to Label when a Horizontal field has no
	// Content part and the label sits next to the control.
	HorizontalLabel string
	// Content holds the label, description and error of a Horizontal
	// field that has a description or an error.
	Content     string
	Description string
	// DescriptionWithError is added to Description when the field shows an
	// error below it (shadcn's nth-last-2:).
	DescriptionWithError string
	// Invalid is added to Root when the field is invalid (shadcn's
	// data-[invalid=true]:): the label and the text turn red.
	Invalid string
	Error   string
}

func init() {
	defaultStyle.Field = FieldStyle{
		Root:    "flex w-full gap-3",
		Invalid: "text-destructive",
		Orientations: map[Orientation]string{
			Vertical:   "flex-col",
			Horizontal: "flex-row items-center",
		},
		HorizontalWithContent: "items-start",
		Label:                 "w-fit gap-2 leading-snug",
		HorizontalLabel:       "flex-auto",
		Content:               "flex flex-1 flex-col gap-1.5 leading-snug",
		Description:           "text-sm leading-normal font-normal text-muted-foreground",
		DescriptionWithError:  "-mt-1",
		Error:                 "text-sm font-normal text-destructive",
	}
}

// Field is a form control with its label, its help text and its error,
// shadcn/ui's Field. It does the wiring: the label points to the control
// (the control takes an id made with tela.UseID and the label's For is the
// same), and the control is described by the error when there is one, else
// by the description. The control is the Children, an [Input] or a
// [Textarea] (more controls of the kit take the id the same way as they
// arrive).
//
//	kit.Field{
//	    Label:       "Email",
//	    Description: "We never share it.",
//	    Error:       errs.Email, // empty: no error
//	    Children:    kit.Input{Type: "email", Value: &email, OnValueChange: setEmail.Set},
//	}
//
// The error shows as an alert below the control, and the field is invalid
// while it shows one: the control is marked h.AriaInvalid (a red border and
// ring from the recipe's aria-invalid: classes) and the label turns red.
type Field struct {
	// Label, Description and Error are plain text; empty draws nothing.
	Label, Description, Error string
	// Invalid makes the field invalid with no error message, such as when
	// the message is shown elsewhere. A field with an Error is invalid
	// already.
	Invalid bool
	// Orientation is Vertical (the zero value) or Horizontal.
	Orientation Orientation
	// Children is the control.
	Children tela.Component

	// Class is added after the recipe's Root classes. The other Class
	// fields do the same for their part.
	Class            string
	LabelClass       string
	ContentClass     string
	DescriptionClass string
	ErrorClass       string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the field element: a group holding the label, the control,
// the description and the error.
func (f Field) Build() tela.Node {
	s := &UseStyle().Field
	id := tela.UseID()
	descRef, errRef := tela.UseElementRef(), tela.UseElementRef()
	var describedBy tela.ElementRef
	switch {
	case f.Error != "":
		describedBy = errRef
	case f.Description != "":
		describedBy = descRef
	}
	vertical := f.Orientation != Horizontal
	invalid := f.Invalid || f.Error != ""

	hasContent := !vertical && (f.Description != "" || f.Error != "")
	var label, desc, errNode tela.Child
	if f.Label != "" {
		label = Label{For: id, Children: h.Text(f.Label),
			Class: joinClass(s.Label, when(!vertical && !hasContent, s.HorizontalLabel), f.LabelClass)}
	}
	if f.Description != "" {
		desc = h.P(cls(s.Description, when(f.Error != "", s.DescriptionWithError), f.DescriptionClass),
			h.Ref(descRef), h.Text(f.Description))
	}
	if f.Error != "" {
		errNode = h.Div(cls(s.Error, f.ErrorClass), h.Role("alert"), h.Ref(errRef), h.Text(f.Error))
	}

	var body []tela.Child
	if vertical {
		body = []tela.Child{label, f.Children, desc, errNode}
	} else {
		if hasContent {
			body = []tela.Child{f.Children, h.Div(cls(s.Content, f.ContentClass), label, desc, errNode)}
		} else {
			body = []tela.Child{f.Children, label}
		}
	}
	return h.Div(cls(s.Root, pick("Field", "Orientation", s.Orientations, orientationOr(f.Orientation, Vertical)),
		when(hasContent, s.HorizontalWithContent), when(invalid, s.Invalid), f.Class),
		h.Role("group"), f.Props,
		fieldCtx.Provider(fieldInfo{id: id, describedBy: describedBy, invalid: invalid, label: f.Label}, body...))
}
