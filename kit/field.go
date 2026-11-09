// Portions of this file follow shadcn/ui, field.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

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
	// first, then the text; the text first with Field.ControlAfter).
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
	// LabelRow holds the label and the LabelEnd slot, DescriptionRow the
	// description and the DescriptionEnd slot: the text first and the slot
	// at the end of the line. They exist only when the slot is given.
	LabelRow, DescriptionRow string
	// DescriptionInRow is added to Description in its row.
	DescriptionInRow string
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
		LabelRow:              "flex w-full items-center justify-between gap-2",
		DescriptionRow:        "flex w-full items-baseline justify-between gap-2",
		DescriptionInRow:      "min-w-0 flex-1",
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
//
// A Horizontal field puts the control before its text, as shadcn does for
// a checkbox or a radio. A switch row has the text first: set ControlAfter.
//
//	kit.Field{
//	    Orientation:  kit.Horizontal,
//	    ControlAfter: true,
//	    Label:        "Marketing emails",
//	    Description:  "News about new features and offers.",
//	    Class:        "items-center gap-6",
//	    Children:     kit.Switch{Checked: &on, OnCheckedChange: setOn.Set},
//	}
//
// The label and the description are text, and take rich content too:
// LabelContent and DescriptionContent are drawn in place of the text (the
// text still names the control for assistive technologies that need a
// string), and LabelEnd and DescriptionEnd are slots at the end of the label's
// line and of the description's, such as a link or a counter:
//
//	kit.Field{
//	    Label:    "Password",
//	    LabelEnd: kit.Button{Variant: kit.Link, Size: kit.Sm, Class: "h-auto p-0", Children: h.Text("Forgot password?")},
//	    Children: kit.Input{Type: "password"},
//	}
//
// The error takes rich content too: ErrorContent is drawn in place of Error.
//
//	kit.Field{Label: "Username", Children: kit.Input{},
//	    ErrorContent: h.Span(h.Text("Taken. "), h.A(h.Class("underline"), h.Text("See suggestions")))}
//
// A group of controls (a [RadioGroup], a [ToggleGroup]) in a Field is named
// by the field's label: the group is labelled by it (aria-labelledby), so it
// needs no AriaLabel. To caption a group that has more than the one control
// a label points to, with its own legend and description, use a [FieldSet].
type Field struct {
	// Label, Description and Error are plain text; empty draws nothing.
	Label, Description, Error string
	// LabelContent and DescriptionContent are drawn in place of Label and
	// Description, for text that is not plain: a bold word, a link, an
	// icon. Label and Description should still say it in words: they name
	// a control that has no label element of its own (a Select's button).
	// Either one makes the label or the description exist.
	LabelContent, DescriptionContent tela.Component
	// LabelEnd is drawn at the end of the label's line, level with it and
	// outside the label (pressing it does not focus the control), such as
	// "Forgot password?". DescriptionEnd does the same by the description,
	// such as a character counter.
	LabelEnd, DescriptionEnd tela.Component
	// ErrorContent is drawn in place of Error, for a message that is not
	// plain text: a link, a list of what is wrong. It makes the field
	// invalid, shows as an alert below the control and describes it like
	// Error; with both, ErrorContent is drawn. Error should still say it in
	// words where a string is wanted.
	ErrorContent tela.Component
	// Invalid makes the field invalid with no error message, such as when
	// the message is shown elsewhere. A field with an Error is invalid
	// already.
	Invalid bool
	// Orientation is Vertical (the zero value) or Horizontal.
	Orientation Orientation
	// ControlAfter puts the control after the label and the description, as
	// in shadcn's switch rows (FieldContent first, then the Switch). It only
	// affects a Horizontal field: a Vertical one is always the label, the
	// control, the description and the error. The DOM order, and so the
	// reading order of a screen reader, is the visual one; the label still
	// points to the control, and the error and the invalid state still reach
	// it.
	ControlAfter bool
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
	descRef, errRef, labelRef := tela.UseElementRef(), tela.UseElementRef(), tela.UseElementRef()
	hasLabel := f.Label != "" || f.LabelContent != nil
	hasDesc := f.Description != "" || f.DescriptionContent != nil
	hasErr := f.Error != "" || f.ErrorContent != nil
	var describedBy tela.ElementRef
	switch {
	case hasErr:
		describedBy = errRef
	case hasDesc:
		describedBy = descRef
	}
	vertical := f.Orientation != Horizontal
	invalid := f.Invalid || hasErr
	// A group of controls inside the field is named by the label.
	var groupName tela.ElementRef
	if hasLabel {
		groupName = labelRef
	}

	hasContent := !vertical && (hasDesc || hasErr)
	var label, desc, errNode tela.Child
	if hasLabel {
		var text tela.Component = f.LabelContent
		if text == nil {
			text = h.Text(f.Label)
		}
		horizontalLabel := when(!vertical && !hasContent, s.HorizontalLabel)
		label = Label{For: id, Children: text, Props: h.Ref(labelRef),
			Class: joinClass(s.Label, when(f.LabelEnd == nil, horizontalLabel), f.LabelClass)}
		if f.LabelEnd != nil {
			label = h.Div(cls(s.LabelRow, horizontalLabel), label, f.LabelEnd)
		}
	}
	if hasDesc {
		var text tela.Component = f.DescriptionContent
		if text == nil {
			text = h.Text(f.Description)
		}
		withError := when(hasErr, s.DescriptionWithError)
		if f.DescriptionEnd != nil {
			desc = h.Div(cls(s.DescriptionRow, withError),
				h.P(cls(s.Description, s.DescriptionInRow, f.DescriptionClass), h.Ref(descRef), text), f.DescriptionEnd)
		} else {
			desc = h.P(cls(s.Description, withError, f.DescriptionClass), h.Ref(descRef), text)
		}
	}
	if hasErr {
		var text tela.Component = f.ErrorContent
		if text == nil {
			text = h.Text(f.Error)
		}
		errNode = h.Div(cls(s.Error, f.ErrorClass), h.Role("alert"), h.Ref(errRef), text)
	}

	var body []tela.Child
	if vertical {
		body = []tela.Child{label, f.Children, desc, errNode}
	} else {
		text := label
		if hasContent {
			text = h.Div(cls(s.Content, f.ContentClass), label, desc, errNode)
		}
		if f.ControlAfter {
			body = []tela.Child{text, f.Children}
		} else {
			body = []tela.Child{f.Children, text}
		}
	}
	return h.Div(cls(s.Root, pick("Field", "Orientation", s.Orientations, orientationOr(f.Orientation, Vertical)),
		when(hasContent, s.HorizontalWithContent), when(invalid, s.Invalid), f.Class),
		h.Role("group"), f.Props,
		legendCtx.Provider(groupName,
			fieldCtx.Provider(fieldInfo{id: id, describedBy: describedBy, invalid: invalid, label: f.Label}, body...)))
}
