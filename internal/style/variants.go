package style

import (
	"slices"
	"strings"
)

// StateFlags is a set of element states that variants test.
type StateFlags uint16

// The states the state and structural variants test.
const (
	StateHover StateFlags = 1 << iota
	StateActive
	StateFocus
	StateFocusVisible
	StateFocusWithin
	StateDisabled
	StateChecked
	StateFirst
	StateLast
	StateOnly
	StateOdd
	StateEven
	// StateReadOnly: an element the user cannot edit (CSS :read-only):
	// every element but an Input or Textarea that is neither ReadOnly nor
	// disabled.
	StateReadOnly
	// StatePlaceholderShown: a text field showing its placeholder
	// (:placeholder-shown).
	StatePlaceholderShown
	// StateInvalid: a text field that fails constraint validation
	// (:invalid).
	StateInvalid
	// StateEnabled is the absence of StateDisabled; the caller does not
	// set it, enabled: tests that StateDisabled is clear.
)

// Variant state names, in the order of the flags.
var stateVariants = []struct {
	name string
	flag StateFlags
}{
	{"hover", StateHover},
	{"active", StateActive},
	{"focus", StateFocus},
	{"focus-visible", StateFocusVisible},
	{"focus-within", StateFocusWithin},
	{"disabled", StateDisabled},
	{"checked", StateChecked},
	{"first", StateFirst},
	{"last", StateLast},
	{"only", StateOnly},
	{"odd", StateOdd},
	{"even", StateEven},
	{"read-only", StateReadOnly},
	{"placeholder-shown", StatePlaceholderShown},
	{"invalid", StateInvalid},
}

// groupable lists the states usable after group- and peer-.
var groupable = StateHover | StateActive | StateFocus | StateFocusVisible |
	StateFocusWithin | StateDisabled | StateChecked | StateReadOnly | StatePlaceholderShown | StateInvalid

// Pseudo is a set of pseudo-elements a rule styles instead of the element
// itself (the placeholder: and selection: variants).
type Pseudo uint8

const (
	// PseudoPlaceholder styles the placeholder text of a text field
	// (::placeholder): its text properties (ComputedStyle.Placeholder).
	PseudoPlaceholder Pseudo = 1 << iota
	// PseudoSelection styles the selected text (::selection, and that of
	// the descendants, as Tailwind's selection: does): its color and
	// background color (ComputedStyle.Selection).
	PseudoSelection
)

// VariantContext is what a Condition is evaluated against.
type VariantContext struct {
	// ViewportWidth is the window width in logical pixels (sm: to 2xl:).
	ViewportWidth float32
	Dark          bool
	ReducedMotion bool
	// State is the element's own state; Group the states of its
	// ancestors with the class "group" and Peer those of its preceding
	// siblings with the class "peer", combined (Tailwind's group-hover:
	// matches under any hovered group, peer-hover: after any hovered
	// peer).
	State, Group, Peer StateFlags
}

// Condition is the conjunction of the variants of a class: every part
// must hold. The zero Condition always holds.
type Condition struct {
	// State, Group and Peer are the states required on the element, on its
	// group ancestor and on its peer sibling.
	State, Group, Peer StateFlags
	// Enabled requires that StateDisabled is clear (enabled:).
	Enabled bool
	// Dark requires dark mode (dark:).
	Dark bool
	// MotionReduce requires reduced motion, MotionSafe its absence.
	MotionReduce, MotionSafe bool
	// MinWidth requires a viewport at least this wide, in pixels (the
	// largest of the breakpoint variants of the class).
	MinWidth float32
	// Pseudo makes the rule style a pseudo-element: it does not apply to
	// the element itself (Compute applies it to ComputedStyle.Placeholder
	// or ComputedStyle.Selection).
	Pseudo Pseudo
}

// IsZero reports whether the condition always holds.
func (c Condition) IsZero() bool { return c == Condition{} }

// Matches evaluates the condition.
func (c Condition) Matches(v VariantContext) bool {
	switch {
	case v.State&c.State != c.State,
		v.Group&c.Group != c.Group,
		v.Peer&c.Peer != c.Peer,
		c.Enabled && v.State&StateDisabled != 0,
		c.Dark && !v.Dark,
		c.MotionReduce && !v.ReducedMotion,
		c.MotionSafe && v.ReducedMotion,
		v.ViewportWidth < c.MinWidth:
		return false
	}
	return true
}

// and returns the conjunction of c and d.
func (c Condition) and(d Condition) Condition {
	c.State |= d.State
	c.Group |= d.Group
	c.Peer |= d.Peer
	c.Enabled = c.Enabled || d.Enabled
	c.Dark = c.Dark || d.Dark
	c.MotionReduce = c.MotionReduce || d.MotionReduce
	c.MotionSafe = c.MotionSafe || d.MotionSafe
	c.MinWidth = max(c.MinWidth, d.MinWidth)
	c.Pseudo |= d.Pseudo
	return c
}

// parseVariant parses one variant name ("hover", "group-focus", "md").
func parseVariant(th *Theme, name string) (Condition, error) {
	for _, sv := range stateVariants {
		if name == sv.name {
			return Condition{State: sv.flag}, nil
		}
	}
	switch name {
	case "enabled":
		return Condition{Enabled: true}, nil
	case "dark":
		return Condition{Dark: true}, nil
	case "motion-reduce":
		return Condition{MotionReduce: true}, nil
	case "motion-safe":
		return Condition{MotionSafe: true}, nil
	case "placeholder":
		return Condition{Pseudo: PseudoPlaceholder}, nil
	case "selection":
		return Condition{Pseudo: PseudoSelection}, nil
	}
	if l, ok := th.Breakpoints[name]; ok {
		return Condition{MinWidth: l.Resolve(th.RootFontSize, th.RootFontSize)}, nil
	}
	for _, prefix := range []string{"group-", "peer-"} {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if strings.Contains(rest, "/") {
			return Condition{}, &UnsupportedClassError{Class: name,
				Reason: "named groups and peers (" + prefix + "hover/name:) are not supported; use the plain " + strings.TrimSuffix(prefix, "-") + " class"}
		}
		for _, sv := range stateVariants {
			if rest == sv.name && sv.flag&groupable != 0 {
				if prefix == "group-" {
					return Condition{Group: sv.flag}, nil
				}
				return Condition{Peer: sv.flag}, nil
			}
		}
		return Condition{}, &UnknownClassError{Class: name, Suggestion: suggestVariant(th, name), Variant: true}
	}
	switch name {
	case "before", "after", "file", "marker", "first-line", "first-letter", "backdrop":
		return Condition{}, &UnsupportedClassError{Class: name,
			Reason: "pseudo-elements are not supported; use regular nodes"}
	case "print", "supports", "portrait", "landscape", "contrast-more", "forced-colors":
		return Condition{}, &UnsupportedClassError{Class: name,
			Reason: "the " + name + " variant is not supported"}
	case "rtl", "ltr":
		return Condition{}, &UnsupportedClassError{Class: name,
			Reason: "text direction variants are not supported yet; Tela lays out left to right"}
	}
	if strings.HasPrefix(name, "max-") || strings.HasPrefix(name, "min-") || strings.HasPrefix(name, "@") ||
		strings.HasPrefix(name, "aria-") || strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "has-") ||
		strings.HasPrefix(name, "not-") || strings.HasPrefix(name, "nth-") || strings.HasPrefix(name, "supports-") ||
		strings.HasPrefix(name, "*") || strings.HasPrefix(name, "[") {
		return Condition{}, &UnsupportedClassError{Class: name,
			Reason: "this variant is not supported; use explicit styles and the state, structural and environment variants (hover:, group-hover:, first:, dark:, sm:, motion-reduce:)"}
	}
	return Condition{}, &UnknownClassError{Class: name, Suggestion: suggestVariant(th, name), Variant: true}
}

// variantNames lists the variants for suggestions.
func variantNames(th *Theme) []string {
	var out []string
	for _, sv := range stateVariants {
		out = append(out, sv.name)
		if sv.flag&groupable != 0 {
			out = append(out, "group-"+sv.name, "peer-"+sv.name)
		}
	}
	out = append(out, "enabled", "dark", "motion-reduce", "motion-safe", "placeholder", "selection")
	out = append(out, th.Keys(NamespaceBreakpoint)...)
	slices.Sort(out)
	return out
}

func suggestVariant(th *Theme, name string) string {
	return closest(name, variantNames(th))
}
