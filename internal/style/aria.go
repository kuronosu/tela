package style

import "strings"

// AriaRow is one row of the aria table: an ARIA attribute Tela has a prop
// for, and either its true state (Token empty: the boolean attributes,
// whose "true" the named variants aria-busy: and the arbitrary
// aria-[busy=true]: test) or one of its value tokens (the token-valued
// attributes sort and current).
//
// The table is the one place that lists them: style derives the aria
// variants, their error hint and their did-you-mean vocabulary from it,
// core derives the validation lists of AriaSort and AriaCurrent and the
// state bits of the props (ADR-097, ADR-131, ADR-171), and telatest the
// lists of its Sort and Current. A new token or attribute is a row (and a
// StateFlags constant when a variant tests it).
type AriaRow struct {
	// Attr is the ARIA attribute without the "aria-" prefix, as an
	// attribute selector spells it: "busy", "readonly", "sort", "current".
	Attr string
	// Prop is the name of the prop that sets it: "AriaBusy".
	Prop string
	// Token is the value token of a token-valued attribute; empty for a
	// boolean one.
	Token string
	// State is the flag the variants test; 0 for a token that no variant
	// tests ("other" and "none" of aria-sort, "false" of aria-current).
	State StateFlags
	// Unset reports that the token means the attribute is not set (the
	// ARIA default): "none" for aria-sort, "false" for aria-current.
	Unset bool
}

// ariaTable lists the aria attributes the variants test, in the order of
// the state flags (the order of the hint).
var ariaTable = []AriaRow{
	{Attr: "busy", Prop: "AriaBusy", State: StateAriaBusy},
	{Attr: "checked", Prop: "AriaChecked", State: StateAriaChecked},
	{Attr: "disabled", Prop: "AriaDisabled", State: StateAriaDisabled},
	{Attr: "expanded", Prop: "AriaExpanded", State: StateAriaExpanded},
	{Attr: "hidden", Prop: "AriaHidden", State: StateAriaHidden},
	{Attr: "invalid", Prop: "AriaInvalid", State: StateAriaInvalid},
	{Attr: "pressed", Prop: "AriaPressed", State: StateAriaPressed},
	{Attr: "readonly", Prop: "AriaReadOnly", State: StateAriaReadOnly},
	{Attr: "required", Prop: "AriaRequired", State: StateAriaRequired},
	{Attr: "selected", Prop: "AriaSelected", State: StateAriaSelected},

	{Attr: "sort", Prop: "AriaSort", Token: "ascending", State: StateAriaSortAscending},
	{Attr: "sort", Prop: "AriaSort", Token: "descending", State: StateAriaSortDescending},
	{Attr: "sort", Prop: "AriaSort", Token: "other"},
	{Attr: "sort", Prop: "AriaSort", Token: "none", Unset: true},

	{Attr: "current", Prop: "AriaCurrent", Token: "page", State: StateAriaCurrentPage},
	{Attr: "current", Prop: "AriaCurrent", Token: "step", State: StateAriaCurrentStep},
	{Attr: "current", Prop: "AriaCurrent", Token: "location", State: StateAriaCurrentLocation},
	{Attr: "current", Prop: "AriaCurrent", Token: "date", State: StateAriaCurrentDate},
	{Attr: "current", Prop: "AriaCurrent", Token: "time", State: StateAriaCurrentTime},
	{Attr: "current", Prop: "AriaCurrent", Token: "true", State: StateAriaCurrentTrue},
	{Attr: "current", Prop: "AriaCurrent", Token: "false", Unset: true},
}

// AriaStates is the set of the aria-* states: the flags of the aria table.
var AriaStates = func() (s StateFlags) {
	for _, r := range ariaTable {
		s |= r.State
	}
	return s
}()

// AriaTokens returns the value tokens of the token-valued attribute attr
// ("sort", "current") in table order, and nil for any other. The caller
// must not modify it.
func AriaTokens(attr string) []string {
	return ariaTokens[attr]
}

var ariaTokens = func() map[string][]string {
	m := map[string][]string{}
	for _, r := range ariaTable {
		if r.Token != "" {
			m[r.Attr] = append(m[r.Attr], r.Token)
		}
	}
	return m
}()

// AriaTokenState returns the flag the variants test for token of the
// attribute attr, or 0 when none does (or the token is unknown).
func AriaTokenState(attr, token string) StateFlags {
	if token == "" {
		return 0
	}
	for _, r := range ariaTable {
		if r.Attr == attr && r.Token == token {
			return r.State
		}
	}
	return 0
}

// AriaTokenUnset reports whether token of attr means the attribute is not
// set: "none" for sort, "false" for current.
func AriaTokenUnset(attr, token string) bool {
	if token == "" {
		return false
	}
	for _, r := range ariaTable {
		if r.Attr == attr && r.Token == token {
			return r.Unset
		}
	}
	return false
}

// AriaBoolState returns the flag of the boolean attribute attr ("busy",
// "readonly"...), or 0 for any other.
func AriaBoolState(attr string) StateFlags {
	for _, r := range ariaTable {
		if r.Attr == attr && r.Token == "" {
			return r.State
		}
	}
	return 0
}

// ariaNamedVariants are the entries of stateVariants of the boolean
// attributes: aria-busy: and its siblings.
func ariaNamedVariants() []stateVariant {
	var out []stateVariant
	for _, r := range ariaTable {
		if r.Token == "" {
			out = append(out, stateVariant{"aria-" + r.Attr, r.State})
		}
	}
	return out
}

// ariaVariantHint lists the aria variants for messages.
var ariaVariantHint = func() string {
	var props, named, sorts, currents []string
	for _, r := range ariaTable {
		switch {
		case r.Token == "":
			props = append(props, r.Prop)
			named = append(named, "aria-"+r.Attr+":")
		case r.State == 0:
		case r.Attr == "sort":
			sorts = append(sorts, "aria-[sort="+r.Token+"]:")
		case r.Attr == "current":
			currents = append(currents, "aria-[current="+r.Token+"]:")
		}
	}
	return "the aria variants test the props " + andList(props) + " when they are true: " +
		strings.Join(named, ", ") + ", AriaSort with " + andList(sorts) +
		", and AriaCurrent with " + andList(currents) +
		" (also group-aria-checked: and peer-aria-[sort=ascending]:)"
}()

// andList joins "a, b and c".
func andList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
