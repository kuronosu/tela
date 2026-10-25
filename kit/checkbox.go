// Portions of this file follow shadcn/ui, checkbox.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// CheckboxStyle is the recipe of [Checkbox] (shadcn/ui's Checkbox).
type CheckboxStyle struct {
	// Root is the box: size, border, the checked colors, focus ring,
	// disabled and invalid. It has the class "peer", so a sibling after it
	// can use the peer-* variants. It is inline-flex with its content centered,
	// as a browser draws the content of a button (Tela's button centers it
	// horizontally only).
	Root string
	// Indicator holds the check mark and is only there while checked.
	Indicator string
	// Icon is the check mark.
	Icon string
}

func init() {
	defaultStyle.Checkbox = CheckboxStyle{
		Root: "peer inline-flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-input shadow-xs transition-shadow outline-none " +
			"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 " +
			"aria-invalid:border-destructive aria-invalid:ring-destructive/20 checked:border-primary checked:bg-primary checked:text-primary-foreground " +
			"dark:bg-input/30 dark:aria-invalid:ring-destructive/40 dark:checked:bg-primary",
		Indicator: "grid place-content-center text-current transition-none",
		Icon:      "size-3.5",
	}
}

// Checkbox is a box that is checked or not, shadcn/ui's Checkbox, on
// aria.UseCheckbox: a click or Space toggles it, Tab reaches it, and it
// has the checkbox role and aria-checked. Its state follows the kit's
// rule: give Checked (a pointer to your state) and OnCheckedChange to keep
// it yourself, or DefaultChecked to let the checkbox keep it.
//
//	agree, setAgree := tela.UseState(false)
//	kit.Checkbox{Checked: &agree, OnCheckedChange: setAgree.Set, AriaLabel: "Accept the terms"}
//
// A [Label] with the checkbox's ID names it and toggles it when clicked,
// and a [Field] ties its label to it with no ids to write:
//
//	kit.Field{Orientation: kit.Horizontal, Label: "Accept the terms",
//	    Children: kit.Checkbox{DefaultChecked: true}}
//
// The indeterminate state is not supported (aria has none yet).
type Checkbox struct {
	// Checked points to the state of a controlled checkbox; nil leaves it
	// to the checkbox, which starts at DefaultChecked.
	Checked *bool
	// DefaultChecked is the initial state of an uncontrolled checkbox.
	DefaultChecked bool
	// OnCheckedChange is called with the next state when the user toggles
	// the checkbox. For a controlled checkbox it is how the change is asked
	// for.
	OnCheckedChange func(bool)
	// Disabled makes the checkbox inert and dims it.
	Disabled bool
	// Required marks the checkbox as one the user must check
	// (aria-required).
	Required bool
	// Invalid marks the value as wrong: assistive technologies announce it
	// and the aria-invalid: classes (a red border and ring) apply. A
	// [Field] with an Error sets it on its control.
	Invalid bool
	// ID is the checkbox's id, for a [Label]'s For. Inside a [Field] it
	// defaults to the field's.
	ID string
	// AriaLabel names a checkbox that has no label of its own.
	AriaLabel string
	// Icon replaces the check mark (Lucide's check).
	Icon tela.Icon
	// Class is added after the recipe's classes; IndicatorClass and
	// IconClass do the same for their parts.
	Class          string
	IndicatorClass string
	IconClass      string
	// Props are more props for the checkbox element.
	Props tela.Child
}

// Build returns the checkbox element.
func (c Checkbox) Build() tela.Node {
	s := &UseStyle().Checkbox
	id, _, field := useChoiceField(c.ID, c.Invalid)
	box := aria.UseCheckbox(aria.ToggleOptions{Selected: c.Checked, DefaultSelected: c.DefaultChecked,
		OnChange: c.OnCheckedChange, Disabled: c.Disabled, ID: id})
	kids := []tela.Child{cls(s.Root, c.Class), box.Props, c.Props}
	kids = append(kids, field...)
	if c.Required {
		kids = append(kids, h.AriaRequired(true))
	}
	if c.AriaLabel != "" {
		kids = append(kids, h.AriaLabel(c.AriaLabel))
	}
	if box.Selected {
		kids = append(kids, h.Span(cls(s.Indicator, c.IndicatorClass), h.Icon(iconOr(c.Icon, iconCheck), cls(s.Icon, c.IconClass))))
	}
	return h.Button(kids...)
}
