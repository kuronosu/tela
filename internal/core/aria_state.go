package core

import (
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// AriaRequired sets whether a field must be filled (aria-required): a
// textbox, combobox, listbox, radio group, spin button or tree. It sets
// the node's required state and matches the aria-required: variants.
func AriaRequired(required bool) Prop {
	return ariaProp("AriaRequired", func(a *ariaProps) { a.required = semantics.Of(required) })
}

// AriaReadOnly sets whether a widget the user can focus is not editable
// (aria-readonly): a textbox, checkbox, combobox, grid, listbox, radio
// group, slider or spin button. It sets the node's read-only state, like
// a ReadOnly text field's, and matches the aria-readonly: variants.
func AriaReadOnly(readOnly bool) Prop {
	return ariaProp("AriaReadOnly", func(a *ariaProps) { a.readOnly = semantics.Of(readOnly) })
}

// AriaBusy sets whether the element, and what it contains, is being
// updated (aria-busy): a region that loads. It sets the node's busy state
// and matches the aria-busy: variants.
func AriaBusy(busy bool) Prop {
	return ariaProp("AriaBusy", func(a *ariaProps) { a.busy = semantics.Of(busy) })
}

// AriaInvalid sets whether the value of a field fails validation
// (aria-invalid): a textbox, combobox, listbox, checkbox, radio group,
// slider, spin button or grid cell. It sets the node's invalid state (true
// or false: it wins over a text field's own validity, as in Chromium's
// accessibility tree) and matches the aria-invalid: variants; Playwright's
// aria snapshot prints it as [invalid] on the roles that support it.
func AriaInvalid(invalid bool) Prop {
	return ariaProp("AriaInvalid", func(a *ariaProps) { a.invalid = semantics.Of(invalid) })
}

var sortValues = []string{"ascending", "descending", "other", "none"}

// AriaSort sets how the rows of a table or grid are sorted by a column or
// row header (aria-sort): "ascending", "descending", "other" or "none"
// (the default), ASCII case-insensitive. Unknown values panic in dev mode
// with the closest valid one and are ignored otherwise. It sets the node's
// Sort, and "ascending" and "descending" match the aria-[sort=ascending]:
// and aria-[sort=descending]: variants; the aria snapshot does not print
// it (Playwright's does not either).
func AriaSort(sort string) Prop {
	v, ok := ariaToken("AriaSort", sort, sortValues)
	if v == "none" {
		v = ""
	}
	return ariaProp("AriaSort", func(a *ariaProps) {
		if ok {
			a.sort = v
		}
	})
}

var currentValues = []string{"page", "step", "location", "date", "time", "true", "false"}

// AriaCurrent marks the element that is the current item of a set
// (aria-current): "page" (the link of the page being shown), "step",
// "location", "date", "time", or "true" for any other. "false" and ""
// mean none (the default). Tokens are ASCII case-insensitive; unknown ones
// panic in dev mode with the closest valid one and are ignored otherwise.
// It sets the node's Current, and each token matches its own
// aria-[current=page]: variant (also step, location, date, time and true,
// with the group- and peer- forms); Tailwind has no bare aria-current:.
// Playwright's aria snapshot does not print it.
func AriaCurrent(current string) Prop {
	v := ""
	ok := true
	if strings.TrimSpace(current) != "" {
		v, ok = ariaToken("AriaCurrent", current, currentValues)
	}
	if v == "false" {
		v = ""
	}
	return ariaProp("AriaCurrent", func(a *ariaProps) {
		if ok {
			a.current = v
		}
	})
}

// ariaStates returns the states that a's props set to true: what the
// aria-busy:, aria-checked:... variants (and their group- and peer-
// forms) test, as Tailwind's [aria-x="true"] selectors do (and
// aria-[sort=ascending]:, aria-[sort=descending]: and aria-[current=page]:
// with its siblings), and what
// checked: tests (an element whose AriaChecked is true: Tela has no
// native checkbox, ADR-100). A mixed checkbox or pressed button is not
// "true".
func ariaStates(a *ariaProps) style.StateFlags {
	if a == nil {
		return 0
	}
	var s style.StateFlags
	for _, f := range [...]struct {
		on   bool
		flag style.StateFlags
	}{
		{a.busy == semantics.ToggleTrue, style.StateAriaBusy},
		{a.checked == semantics.ToggleTrue, style.StateAriaChecked | style.StateChecked},
		{a.disabled == semantics.ToggleTrue, style.StateAriaDisabled},
		{a.expanded == semantics.ToggleTrue, style.StateAriaExpanded},
		{a.hiddenSet && a.hidden, style.StateAriaHidden},
		{a.invalid == semantics.ToggleTrue, style.StateAriaInvalid},
		{a.pressed == semantics.ToggleTrue, style.StateAriaPressed},
		{a.readOnly == semantics.ToggleTrue, style.StateAriaReadOnly},
		{a.required == semantics.ToggleTrue, style.StateAriaRequired},
		{a.selected == semantics.ToggleTrue, style.StateAriaSelected},
		{a.sort == "ascending", style.StateAriaSortAscending},
		{a.sort == "descending", style.StateAriaSortDescending},
		{a.current == "page", style.StateAriaCurrentPage},
		{a.current == "step", style.StateAriaCurrentStep},
		{a.current == "location", style.StateAriaCurrentLocation},
		{a.current == "date", style.StateAriaCurrentDate},
		{a.current == "time", style.StateAriaCurrentTime},
		{a.current == "true", style.StateAriaCurrentTrue},
	} {
		if f.on {
			s |= f.flag
		}
	}
	return s
}

// setAriaStates moves the aria states of primitive el from old to new,
// restyling, as setState does for each, only the elements whose classes
// test a state that changed: el, the descendants of a group and the
// following siblings of a peer.
func (t *Tree) setAriaStates(el *Element, old, new style.StateFlags) {
	changed := old ^ new
	for f := style.StateFlags(1); changed != 0; f <<= 1 {
		if changed&f != 0 {
			t.setState(el, f, new&f != 0)
			changed &^= f
		}
	}
}
