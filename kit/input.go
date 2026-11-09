// Portions of this file follow shadcn/ui, input.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// InputStyle is the recipe of [Input] (shadcn/ui's Input).
type InputStyle struct {
	Root string
	// Wrapper is the element around an input that has an Icon or an
	// IconEnd; it carries the input's Class and is as wide as the input
	// would be.
	Wrapper string
	// Icon is the icon, level with the text and at the start (IconStart)
	// or at the end (IconEnd) of the field; both are muted and let the
	// pointer through. IconDisabled and IconInvalid are added to them
	// after.
	Icon, IconStart, IconEnd string
	// IconDisabled dims the icon of a disabled field and IconInvalid
	// colors the one of an invalid field.
	IconDisabled, IconInvalid string
	// PadStart and PadEnd are added to the input to make room for the
	// icons (an icon, its inset and a gap).
	PadStart, PadEnd string
	// Action is the box around an EndContent: at the end of the field,
	// level with the text, taking pointer events (shadcn's
	// InputGroupAddon with an InputGroupButton, which sits 0.45rem into
	// the field's padding). ActionDisabled is added to it after, for a
	// disabled field.
	Action, ActionDisabled string
}

func init() {
	defaultStyle.Input = InputStyle{
		Root: "h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none " +
			"selection:bg-primary selection:text-primary-foreground placeholder:text-muted-foreground " +
			"disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30 " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 " +
			"aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
		Wrapper:        "relative flex w-full min-w-0",
		Icon:           "pointer-events-none absolute top-1/2 size-4 -translate-y-1/2 text-muted-foreground",
		IconStart:      "start-3",
		IconEnd:        "end-3",
		IconDisabled:   "peer-disabled:opacity-50",
		IconInvalid:    "peer-aria-invalid:text-destructive",
		PadStart:       "ps-9",
		PadEnd:         "pe-9",
		Action:         "absolute end-1.5 top-1/2 flex -translate-y-1/2 items-center gap-1 text-muted-foreground",
		ActionDisabled: "peer-disabled:opacity-50",
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
//
// EndContent puts a component inside the field at its end, where IconEnd
// would be, and it takes pointer events: a button, a unit label, a spinner.
// A password field with a button that shows what was typed:
//
//	shown, setShown := tela.UseState(false)
//	typ := "password"
//	if shown {
//	    typ = "text"
//	}
//	kit.Input{Type: typ, AriaLabel: "Password",
//	    EndContent: kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: lucide.Eye,
//	        AriaLabel: "Show password", OnClick: func() { setShown.Set(!shown) }}}
//
// (shadcn's InputGroup with an InputGroupButton). The box is as tall as its
// content and 6 px from the edge; the field keeps 36 px of padding for it,
// the room of an icon-sized button (kit.IconXs, 24 px): for a wider piece of
// content add a padding to InputClass ("pe-16"). A Disabled field dims it;
// disable a button in it yourself. EndContent and IconEnd take the same
// place: giving both panics in dev mode.
//
// A key for the field in a list goes on the component, tela.WithKey(kit.Input{...}, id),
// which is the same with and without a wrapper; a Key in Props keys the
// input element inside the component only.
//
// Icon puts an icon inside the field at its start (a magnifier in a search
// box, an envelope in an email field) and IconEnd one at its end. Both are
// decorative, mirror under a right-to-left direction, and leave the text
// room; the focus ring stays on the field. The icons need a wrapper, so
// with one Class goes on the wrapper (the width of the field, "max-w-sm")
// and InputClass on the input:
//
//	kit.Input{Icon: lucide.Search, Placeholder: "Search...", Class: "max-w-sm"}
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
	ID string
	// AriaLabel names a field that has no label of its own, such as a
	// search box; a [Field]'s Label or a [Label] pointing to the field
	// names it otherwise.
	AriaLabel string
	Disabled  bool
	ReadOnly  bool
	// Invalid marks the value as wrong: assistive technologies announce it
	// and the recipe's aria-invalid: classes (a red border and ring) apply.
	// A [Field] with an Error sets it on its control.
	Invalid   bool
	AutoFocus bool
	// MaxLength limits what the user can type, in UTF-16 code units; 0 is
	// no limit.
	MaxLength int
	// Icon is drawn inside the field at its start and IconEnd at its end.
	Icon, IconEnd tela.Icon
	// EndContent is drawn inside the field at its end, over the input's
	// padding; see above.
	EndContent tela.Component
	// Class is added after the recipe's classes. With an Icon, IconEnd or
	// EndContent it is for the wrapper that holds the input and the icons, and
	// InputClass is added to the input.
	Class      string
	InputClass string
	// Props are more props for the input element.
	Props tela.Child
}

// Build returns the input element.
func (i Input) Build() tela.Node {
	s := &UseStyle().Input
	if devMode() && i.EndContent != nil && i.IconEnd != (tela.Icon{}) {
		panic("kit: Input has both IconEnd and EndContent, which take the same place: keep one (put the icon in the EndContent)")
	}
	hasEnd := i.IconEnd != (tela.Icon{}) || i.EndContent != nil
	hasIcons := i.Icon != (tela.Icon{}) || hasEnd
	inputClass := joinClass(s.Root, i.Class, i.InputClass)
	if hasIcons {
		inputClass = joinClass(s.Root, "peer", when(i.Icon != (tela.Icon{}), s.PadStart), when(hasEnd, s.PadEnd), i.InputClass)
	}
	kids := []tela.Child{h.Class(inputClass), i.Props}
	if i.Type != "" {
		kids = append(kids, h.Type(i.Type))
	}
	kids = append(kids, useTextField(textFieldProps{
		value: i.Value, defaultValue: i.DefaultValue, onChange: i.OnValueChange,
		placeholder: i.Placeholder, name: i.Name, id: i.ID, ariaLabel: i.AriaLabel,
		disabled: i.Disabled, readOnly: i.ReadOnly, invalid: i.Invalid, autoFocus: i.AutoFocus, maxLength: i.MaxLength,
	})...)
	if !hasIcons {
		return h.Input(kids...)
	}
	// The icons follow the input so that peer- variants see its state.
	var action tela.Child
	if i.EndContent != nil {
		action = h.Div(cls(s.Action, s.ActionDisabled), i.EndContent)
	}
	return h.Div(cls(s.Wrapper, i.Class), h.Input(kids...),
		icon(i.Icon, s.Icon, s.IconStart, s.IconDisabled, s.IconInvalid),
		icon(i.IconEnd, s.Icon, s.IconEnd, s.IconDisabled, s.IconInvalid), action)
}
