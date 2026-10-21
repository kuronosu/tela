package style

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
)

// StateFlags is a set of element states that variants test.
type StateFlags uint32

// The states the state and structural variants test.
const (
	StateHover StateFlags = 1 << iota
	StateActive
	StateFocus
	StateFocusVisible
	StateFocusWithin
	StateDisabled
	// StateChecked: an element whose AriaChecked is true (Tela has no native
	// checkbox; ADR-100), as the aria-checked: variant, which tests the
	// attribute selector.
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

	// The aria-* variants (ADR-097) match the attribute selectors
	// [aria-x="true"] of Tailwind: the element's own Aria… prop set to
	// true, not the state it may have implicitly (a Button's Disabled
	// does not match aria-disabled:).
	StateAriaBusy
	StateAriaChecked
	StateAriaDisabled
	StateAriaExpanded
	StateAriaHidden
	StateAriaPressed
	StateAriaReadOnly
	StateAriaRequired
	StateAriaSelected
)

// AriaStates is the set of the aria-* states.
const AriaStates = StateAriaBusy | StateAriaChecked | StateAriaDisabled | StateAriaExpanded |
	StateAriaHidden | StateAriaPressed | StateAriaReadOnly | StateAriaRequired | StateAriaSelected

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
	{"aria-busy", StateAriaBusy},
	{"aria-checked", StateAriaChecked},
	{"aria-disabled", StateAriaDisabled},
	{"aria-expanded", StateAriaExpanded},
	{"aria-hidden", StateAriaHidden},
	{"aria-pressed", StateAriaPressed},
	{"aria-readonly", StateAriaReadOnly},
	{"aria-required", StateAriaRequired},
	{"aria-selected", StateAriaSelected},
}

// groupable lists the states usable after group- and peer-.
var groupable = StateHover | StateActive | StateFocus | StateFocusVisible |
	StateFocusWithin | StateDisabled | StateChecked | StateReadOnly | StatePlaceholderShown | StateInvalid |
	AriaStates

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
	// ViewportWidth is the window width in logical pixels (sm: to 2xl:,
	// max-sm: to max-2xl:, min-[…]: and max-[…]:).
	ViewportWidth float32
	Dark          bool
	ReducedMotion bool
	// Starting selects the starting: variants: the style is the starting
	// style of an element that was not rendered before (CSS
	// @starting-style), which its transitions start from.
	Starting bool
	// RTL selects the rtl: variants and deselects the ltr: ones: the
	// element's direction is right to left.
	RTL bool
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
	// Starting requires the starting style (starting:, CSS
	// @starting-style): the rule applies only to the style an element's
	// first transitions start from.
	Starting bool
	// RTL requires a right-to-left element (rtl:), LTR a left-to-right
	// one (ltr:). Tailwind matches them on the nearest dir attribute; in
	// Tela the direction comes only from dir (h.Dir), so they test the
	// element's inherited direction.
	RTL, LTR bool
	// MinWidth requires a viewport at least this wide, in pixels (the
	// largest of the sm: to 2xl: and min-[…]: variants of the class).
	MinWidth float32
	// MaxWidth, with HasMaxWidth, requires a viewport narrower than this,
	// in pixels (Tailwind's @media (width < …): max-sm: to max-2xl: and
	// max-[…]:; the smallest of the class's variants). md:max-xl: gives
	// both limits.
	MaxWidth    float32
	HasMaxWidth bool
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
		c.Starting && !v.Starting,
		c.RTL && !v.RTL,
		c.LTR && v.RTL,
		v.ViewportWidth < c.MinWidth,
		c.HasMaxWidth && !(v.ViewportWidth < c.MaxWidth):
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
	c.Starting = c.Starting || d.Starting
	c.RTL = c.RTL || d.RTL
	c.LTR = c.LTR || d.LTR
	c.MinWidth = max(c.MinWidth, d.MinWidth)
	if d.HasMaxWidth {
		if c.HasMaxWidth {
			c.MaxWidth = min(c.MaxWidth, d.MaxWidth)
		} else {
			c.MaxWidth, c.HasMaxWidth = d.MaxWidth, true
		}
	}
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
	case "starting":
		return Condition{Starting: true}, nil
	case "rtl":
		return Condition{RTL: true}, nil
	case "ltr":
		return Condition{LTR: true}, nil
	case "placeholder":
		return Condition{Pseudo: PseudoPlaceholder}, nil
	case "selection":
		return Condition{Pseudo: PseudoSelection}, nil
	}
	if l, ok := th.Breakpoints[name]; ok {
		return Condition{MinWidth: l.Resolve(th.RootFontSize, th.RootFontSize)}, nil
	}
	if c, ok, err := parseWidthVariant(th, name); ok {
		return c, err
	}
	if c, ok, err := parseAriaVariant(name); ok {
		return c, err
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
		if strings.HasPrefix(rest, "aria-[") {
			c, ok, err := parseAriaVariant(rest)
			if err != nil || !ok {
				return Condition{}, withClass(err, name)
			}
			if prefix == "group-" {
				return Condition{Group: c.State}, nil
			}
			return Condition{Peer: c.State}, nil
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
	}
	if strings.HasPrefix(name, "@") ||
		strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "has-") ||
		strings.HasPrefix(name, "not-") || strings.HasPrefix(name, "nth-") || strings.HasPrefix(name, "supports-") ||
		strings.HasPrefix(name, "*") || strings.HasPrefix(name, "[") {
		return Condition{}, &UnsupportedClassError{Class: name,
			Reason: "this variant is not supported; use explicit styles and the state, structural, aria and environment variants (hover:, group-hover:, first:, aria-expanded:, dark:, sm:, max-md:, motion-reduce:)"}
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
	out = append(out, "enabled", "dark", "motion-reduce", "motion-safe", "starting", "rtl", "ltr", "placeholder", "selection")
	for _, bp := range th.Keys(NamespaceBreakpoint) {
		out = append(out, bp, "max-"+bp, "min-"+bp)
	}
	slices.Sort(out)
	return out
}

func suggestVariant(th *Theme, name string) string {
	return closest(name, variantNames(th))
}

// parseWidthVariant parses the width variants besides the breakpoints
// themselves: max-sm: to max-2xl: (the window is narrower than the
// breakpoint: Tailwind v4's @media (width < …)), min-sm: to min-2xl: (the
// same as sm:), and the arbitrary min-[600px]: and max-[40rem]: (a length
// in px, rem or em). ok is false when name is not one of them.
func parseWidthVariant(th *Theme, name string) (c Condition, ok bool, err error) {
	var isMax bool
	var arg string
	switch {
	case strings.HasPrefix(name, "max-"):
		isMax, arg = true, name[len("max-"):]
	case strings.HasPrefix(name, "min-"):
		arg = name[len("min-"):]
	default:
		return Condition{}, false, nil
	}
	set := func(px float32) Condition {
		if isMax {
			return Condition{MaxWidth: px, HasMaxWidth: true}
		}
		return Condition{MinWidth: px}
	}
	if inner, isArb := strings.CutPrefix(arg, "["); isArb && strings.HasSuffix(inner, "]") {
		px, err := parseWidthLength(th, strings.TrimSuffix(inner, "]"))
		if err != nil {
			return Condition{}, true, &InvalidClassError{Class: name, Reason: err.Error()}
		}
		return set(px), true, nil
	}
	if l, found := th.Breakpoints[arg]; found {
		return set(l.Resolve(th.RootFontSize, th.RootFontSize)), true, nil
	}
	if strings.HasPrefix(arg, "[") {
		return Condition{}, true, &InvalidClassError{Class: name, Reason: "the arbitrary width variant needs a closing bracket, as in " + name[:4] + "[600px]"}
	}
	return Condition{}, true, &UnknownClassError{Class: name, Suggestion: suggestVariant(th, name), Variant: true}
}

// parseWidthLength parses the length of min-[…] and max-[…]: a number
// followed by px, rem or em (media queries resolve em and rem against the
// initial font size, th.RootFontSize).
func parseWidthLength(th *Theme, s string) (float32, error) {
	const hint = "the width must be a length in px, rem or em, such as the variants min-[600px] and max-[40rem]"
	var unit float32
	var num string
	switch {
	case strings.HasSuffix(s, "px"):
		unit, num = 1, strings.TrimSuffix(s, "px")
	case strings.HasSuffix(s, "rem"):
		unit, num = th.RootFontSize, strings.TrimSuffix(s, "rem")
	case strings.HasSuffix(s, "em"):
		unit, num = th.RootFontSize, strings.TrimSuffix(s, "em")
	default:
		return 0, errors.New(hint)
	}
	v, err := strconv.ParseFloat(num, 32)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, errors.New(hint)
	}
	return float32(v) * unit, nil
}

// ariaVariantHint lists the aria variants for messages.
const ariaVariantHint = "the aria variants test the props AriaBusy, AriaChecked, AriaDisabled, AriaExpanded, AriaHidden, AriaPressed, AriaReadOnly, AriaRequired and AriaSelected when they are true: aria-busy:, aria-checked:, aria-disabled:, aria-expanded:, aria-hidden:, aria-pressed:, aria-readonly:, aria-required:, aria-selected: (also group-aria-checked: and peer-aria-checked:)"

// parseAriaVariant parses the arbitrary aria variant aria-[checked=true]:
// (the same as aria-checked:) for the aria states Tela has props for. The
// named ones (aria-checked:) are in stateVariants. ok is false when name
// is not an arbitrary aria variant.
func parseAriaVariant(name string) (c Condition, ok bool, err error) {
	inner, isArb := strings.CutPrefix(name, "aria-[")
	if !isArb || !strings.HasSuffix(inner, "]") {
		return Condition{}, false, nil
	}
	inner = strings.TrimSuffix(inner, "]")
	attr, value, hasValue := strings.Cut(inner, "=")
	for _, sv := range stateVariants {
		if sv.name == "aria-"+attr && sv.flag&AriaStates != 0 && hasValue && value == "true" {
			return Condition{State: sv.flag}, true, nil
		}
	}
	return Condition{}, true, &UnsupportedClassError{Class: name,
		Reason: "Tela has no prop for the attribute selector [" + inner + "]; " + ariaVariantHint +
			" (aria-[checked=true]: is aria-checked:)"}
}

// withClass gives an error found in a part of a variant the whole variant
// name.
func withClass(err error, name string) error {
	var un *UnsupportedClassError
	if errors.As(err, &un) {
		return &UnsupportedClassError{Class: name, Reason: un.Reason}
	}
	return err
}
