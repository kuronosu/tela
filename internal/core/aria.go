package core

import (
	"fmt"
	"math"

	"github.com/kuronosu/tela/internal/semantics"
)

// The props of the semantics tree (semantics.go): React's role and aria-*
// attributes, and Img's alt text. They change nothing on screen.

// ariaProps are the semantics props of a primitive node, allocated by the
// first one applied.
type ariaProps struct {
	role        string // as given (a space-separated fallback list)
	label       string
	hasLabel    bool
	labelledBy  *elemRef
	describedBy *elemRef
	desc        string
	hidden      bool
	hiddenSet   bool
	checked     semantics.Toggle
	pressed     semantics.Toggle
	selected    semantics.Toggle
	expanded    semantics.Toggle
	w           *ariaWidget // the widget props (ariawidget.go)
	disabled    semantics.Toggle
	required    semantics.Toggle // AriaRequired, AriaReadOnly and AriaBusy (aria_state.go)
	readOnly    semantics.Toggle
	busy        semantics.Toggle
	level       int32
	rng         semantics.Range
	valueText   string
	hasText     bool
	alt         string
	hasAlt      bool
}

// aria returns the semantics props of d, allocating them.
func (d *desc) ariaProps() *ariaProps {
	if d.aria == nil {
		d.aria = &ariaProps{}
	}
	return d.aria
}

func ariaProp(name string, f func(a *ariaProps)) Prop {
	return Prop{name: name, apply: func(d *desc) { f(d.ariaProps()) }}
}

// Role sets the ARIA role of a primitive (WAI-ARIA 1.2), overriding its
// implicit one: Role("switch"), Role("dialog"). A space-separated list
// takes its first valid role, as in HTML. In dev mode an unknown role
// panics with the closest valid one; otherwise unknown roles are ignored
// and the implicit role applies. "none" and "presentation" remove the
// element's own role unless it is focusable or labelled (ARIA's
// presentational role conflict resolution).
func Role(role string) Prop {
	if devNow() {
		if _, bad := semantics.ParseRole(role); bad != "" {
			msg := fmt.Sprintf("tela: Role(%q): unknown ARIA role %q", role, bad)
			if s := semantics.SuggestRole(bad); s != "" {
				msg += fmt.Sprintf("; did you mean %q?", s)
			} else {
				msg += "; see the WAI-ARIA 1.2 roles (button, checkbox, dialog, heading, link, switch, tab...)"
			}
			panic(msg)
		}
	}
	return ariaProp("Role", func(a *ariaProps) { a.role = role })
}

// AriaLabel sets the accessible name of a primitive (aria-label). It wins
// over the name from content and Alt; AriaLabelledBy wins over it.
func AriaLabel(label string) Prop {
	return ariaProp("AriaLabel", func(a *ariaProps) { a.label, a.hasLabel = label, true })
}

// AriaLabelledBy names a primitive with the text of the element r is
// attached to (aria-labelledby): its AriaLabel, or its content.
func AriaLabelledBy(r ElementRef) Prop {
	if r.r == nil {
		panic("tela: AriaLabelledBy(zero ElementRef): pass an ElementRef from tela.UseElementRef attached with Ref to the labelling element")
	}
	return ariaProp("AriaLabelledBy", func(a *ariaProps) { a.labelledBy = r.r })
}

// AriaDescription sets the accessible description (aria-description).
func AriaDescription(s string) Prop {
	return ariaProp("AriaDescription", func(a *ariaProps) { a.desc = s })
}

// AriaDescribedBy describes a primitive with the text of the element r is
// attached to (aria-describedby), the way a tooltip describes its
// trigger; AriaDescription wins over it.
func AriaDescribedBy(r ElementRef) Prop {
	if r.r == nil {
		panic("tela: AriaDescribedBy(zero ElementRef): pass an ElementRef from tela.UseElementRef attached with Ref to the describing element")
	}
	return ariaProp("AriaDescribedBy", func(a *ariaProps) { a.describedBy = r.r })
}

// AriaHidden hides a primitive and its subtree from the semantics tree
// when true (aria-hidden), without changing what is painted.
func AriaHidden(hidden bool) Prop {
	return ariaProp("AriaHidden", func(a *ariaProps) { a.hidden, a.hiddenSet = hidden, true })
}

// AriaChecked sets the checked state of a checkbox, radio, switch or
// option (aria-checked).
func AriaChecked(checked bool) Prop {
	return ariaProp("AriaChecked", func(a *ariaProps) { a.checked = semantics.Of(checked) })
}

// AriaPressed sets the pressed state of a toggle button (aria-pressed).
func AriaPressed(pressed bool) Prop {
	return ariaProp("AriaPressed", func(a *ariaProps) { a.pressed = semantics.Of(pressed) })
}

// AriaSelected sets the selected state of a tab, option, row or cell
// (aria-selected).
func AriaSelected(selected bool) Prop {
	return ariaProp("AriaSelected", func(a *ariaProps) { a.selected = semantics.Of(selected) })
}

// AriaExpanded sets whether the element a button, link or combobox
// controls is expanded (aria-expanded).
func AriaExpanded(expanded bool) Prop {
	return ariaProp("AriaExpanded", func(a *ariaProps) { a.expanded = semantics.Of(expanded) })
}

// AriaDisabled marks a primitive and its descendants disabled in the
// semantics tree (aria-disabled) without making them inert, unlike
// Disabled.
func AriaDisabled(disabled bool) Prop {
	return ariaProp("AriaDisabled", func(a *ariaProps) { a.disabled = semantics.Of(disabled) })
}

// AriaLevel sets the level of a heading (for Role("heading"); H1 to H6
// keep their own), list item, row or tree item (aria-level). Levels below
// 1 are ignored.
func AriaLevel(level int) Prop {
	l := int32(max(min(level, math.MaxInt32), 0))
	return ariaProp("AriaLevel", func(a *ariaProps) { a.level = l })
}

// AriaValueNow sets the current value of a range widget (aria-valuenow).
func AriaValueNow(v float64) Prop {
	return ariaProp("AriaValueNow", func(a *ariaProps) { a.rng.Now, a.rng.HasNow = v, true })
}

// AriaValueMin sets the minimum value of a range widget (aria-valuemin).
func AriaValueMin(v float64) Prop {
	return ariaProp("AriaValueMin", func(a *ariaProps) { a.rng.Min, a.rng.HasMin = v, true })
}

// AriaValueMax sets the maximum value of a range widget (aria-valuemax).
func AriaValueMax(v float64) Prop {
	return ariaProp("AriaValueMax", func(a *ariaProps) { a.rng.Max, a.rng.HasMax = v, true })
}

// AriaValueText sets the human-readable value of a range widget
// (aria-valuetext): "50%", "Medium".
func AriaValueText(s string) Prop {
	return ariaProp("AriaValueText", func(a *ariaProps) { a.valueText, a.hasText = s, true })
}

// Alt sets the alternative text of an Img, its accessible name. Alt("")
// marks a decorative image, which the semantics tree leaves out (role
// presentation).
func Alt(text string) Prop {
	return ariaProp("Alt", func(a *ariaProps) { a.alt, a.hasAlt = text, true })
}
