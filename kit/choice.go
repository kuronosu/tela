package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// This file holds what the choice controls share ([Checkbox], [Switch],
// [RadioGroup]): the wiring to an enclosing [Field], and the Lucide icons
// they draw.

// useChoiceField returns the id a choice control takes, and the props that
// tie it to the enclosing [Field] when there is one: the control's own id
// when given, else the field's, so that the field's label names it;
// aria-describedby for the field's error or description; and aria-invalid,
// which it reports as well.
// It is a hook (it reads the field's context).
func useChoiceField(id string, invalid bool) (string, bool, []tela.Child) {
	fc := tela.UseContext(fieldCtx)
	if id == "" {
		id = fc.id
	}
	var kids []tela.Child
	if fc.describedBy != (tela.ElementRef{}) {
		kids = append(kids, h.AriaDescribedBy(fc.describedBy))
	}
	invalid = invalid || fc.invalid
	if invalid {
		kids = append(kids, h.AriaInvalid(true))
	}
	return id, invalid, kids
}

// The icons the choice controls draw, from Lucide (ISC, THIRD_PARTY.md), as
// `tela icons gen` writes them: the kit cannot import icons/lucide, a
// separate module. Every icon slot can be replaced with another tela.Icon.
var (
	iconCheck  = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`}
	iconCircle = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/></svg>`}
)

// iconOr returns ic, or def when ic is the zero Icon.
func iconOr(ic, def tela.Icon) tela.Icon {
	if ic == (tela.Icon{}) {
		return def
	}
	return ic
}
