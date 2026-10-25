// Portions of this file follow React Aria's useRadioGroup and useRadio and
// react-stately's useRadioGroupState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/radio and
// packages/@react-stately/radio), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// RadioGroupOptions configures [UseRadioGroup]. K is the type of the
// radios' keys: a string, an int, an enum; any comparable value.
//
// The value is uncontrolled when Value is nil (the hook keeps it, starting
// at DefaultValue, or with nothing checked) and controlled when Value
// points to the caller's state: the group shows it and only asks
// OnValueChange for the next one, so a change the caller does not store is
// refused. OnValueChange is also called for an uncontrolled group. A value
// that is not one of Keys (a controlled "" for none) checks no radio.
//
//	// Uncontrolled, starting at "pro".
//	aria.RadioGroupOptions[string]{Keys: plans, DefaultValue: new("pro")}
//	// Controlled.
//	plan, setPlan := tela.UseState("pro")
//	aria.RadioGroupOptions[string]{Keys: plans, Value: &plan, OnValueChange: setPlan.Set}
type RadioGroupOptions[K comparable] struct {
	// Keys are the keys of the radios, in the order they are shown. The
	// list must be given on every build.
	Keys []K
	// DisabledKeys are the radios that can not be focused or checked.
	// They are skipped by the arrow keys, and get Disabled and
	// AriaDisabled.
	DisabledKeys []K
	// Value points to the checked key of a controlled group; nil leaves
	// the value to the hook.
	Value *K
	// DefaultValue is the checked key of an uncontrolled group at the
	// start; nil checks nothing.
	DefaultValue *K
	// OnValueChange is called with the next key when the user checks a
	// radio: for a controlled group it is how the change is asked for, for
	// an uncontrolled one a notification. It is not called for the radio
	// that already is checked.
	OnValueChange func(K)
	// Disabled disables every radio.
	Disabled bool
	// ReadOnly keeps the radios focusable but not changeable
	// (aria-readonly on the group).
	ReadOnly bool
	// Required marks the group as one the user must answer
	// (aria-required).
	Required bool
	// Orientation is "vertical" (the default, as React Aria's) or
	// "horizontal". It is aria-orientation, and decides whether a
	// right-to-left Direction swaps ArrowLeft and ArrowRight: all four
	// arrows move the focus in either orientation.
	Orientation string
	// Direction is "ltr" (the default) or "rtl".
	Direction string
	// AriaLabel names the group; else it is named by the element with
	// LabelProps, or by AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the group, when it is not
	// the one with LabelProps.
	AriaLabelledBy tela.ElementRef
	// Ref is the group element's ref, for a hook that is used together
	// with others on one element; the zero value makes the hook create
	// its own.
	Ref tela.ElementRef
}

// RadioGroupResult is what [UseRadioGroup] returns.
type RadioGroupResult[K comparable] struct {
	// Props go to the group's element: the "radiogroup" role,
	// aria-orientation, its name, the required, read-only and disabled
	// states and the ref.
	Props tela.Prop
	// LabelProps go to the element that labels the group (a span or a
	// Label): its text names the group, unless AriaLabel is set.
	LabelProps tela.Prop
	// Value is the checked key, valid when HasValue is true. It can be a
	// key that is not one of Keys when a controlled group says so.
	Value K
	// HasValue is whether the group has a value.
	HasValue bool
	// Ref is the ref attached to the group by Props.
	Ref tela.ElementRef

	g *radioGroup[K]
}

// radioGroup is the state a [RadioGroupResult] reads.
type radioGroup[K comparable] struct {
	r      *rover[K]
	val    *choice[K]
	cfg    RadioGroupOptions[K]
	rtl    bool
	prefix string
}

// RadioProps returns the props of the radio with key k (React Aria's
// useRadio): the "radio" role, aria-checked, an ID for a Label, the ref, a
// tab stop on the checked radio (the first enabled one while none is
// checked; the arrow keys move between the others), the disabled state and
// the handlers: a click, tap or Space checks it, and the arrow keys move
// the focus to the next radio and check it.
//
// Render the radio with a Button: a Label(For(RadioID(k))) then names it
// and checks it when it is clicked. A Div works too, but a Label only
// names it. The `checked:` variant (and `aria-checked:`) match the checked
// radio.
func (r RadioGroupResult[K]) RadioProps(k K) tela.Prop {
	g := r.g
	selected := g.isSelected(k)
	disabled := g.r.isDisabled(k)
	ps := []tela.Prop{
		h.Ref(g.r.refFor(k)), h.Role("radio"), h.ID(r.RadioID(k)), h.AriaChecked(selected),
		// A radio in a Form is not its default button.
		core.DefaultButtonType("button"),
	}
	if disabled {
		return core.MergeProps(append(ps, h.Disabled(true), h.AriaDisabled(true))...)
	}
	if g.tabStop(k) {
		ps = append(ps, h.TabIndex(0))
	} else {
		ps = append(ps, h.TabIndex(-1))
	}
	ps = append(ps,
		h.OnClick(func(e tela.MouseEvent) {
			if e.Button == tela.MouseButtonLeft {
				g.check(k)
			}
		}),
		h.OnKeyDown(func(e tela.KeyboardEvent) { g.onKeyDown(k, e) }),
		h.OnFocus(func() { g.r.onFocus(k) }),
	)
	return core.MergeProps(ps...)
}

// IsSelected reports whether the radio with key k is the checked one.
func (r RadioGroupResult[K]) IsSelected(k K) bool { return r.g.isSelected(k) }

// RadioRef returns the ref that RadioProps(k) attaches to the radio, to
// share with another hook on its element.
func (r RadioGroupResult[K]) RadioRef(k K) tela.ElementRef { return r.g.r.refFor(k) }

// RadioID returns the id that RadioProps(k) gives the radio, for a
// Label's For.
func (r RadioGroupResult[K]) RadioID(k K) string { return fmt.Sprintf("%s-%v", r.g.prefix, k) }

// UseRadioGroup holds the state of a radio group and returns the props of
// the group and its radios (React Aria's useRadioGroup and useRadio with
// react-stately's useRadioGroupState). At most one radio is checked; the
// caller renders one element per key.
//
//	plan := aria.UseRadioGroup(aria.RadioGroupOptions[string]{Keys: plans, DefaultValue: new("free"), AriaLabel: "Plan"})
//	return Div(plan.Props, Class("grid gap-2"),
//	    h.Range(len(plans), func(i int) tela.Node {
//	        k := plans[i]
//	        return Div(Key(k), Class("flex items-center gap-2"),
//	            Button(plan.RadioProps(k), Class("size-4 rounded-full border checked:bg-primary")),
//	            Label(For(plan.RadioID(k)), Text(k)))
//	    }))
//
// The arrow keys (all four, in either orientation) move the focus to the
// next enabled radio, going around the ends, and check it; Space checks
// the focused radio; Tab enters the group at the checked radio. A press
// checks a radio. Right-to-left Direction swaps the horizontal arrows.
func UseRadioGroup[K comparable](o RadioGroupOptions[K]) RadioGroupResult[K] {
	const hook = "UseRadioGroup: RadioGroupOptions."
	orient := checkChoice(hook, "Orientation", o.Orientation, "vertical", "vertical", "horizontal")
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	r := useRover(o.Keys, o.DisabledKeys, o.Disabled)
	id := tela.UseID()
	val := useChoice(o.Value, o.DefaultValue, o.OnValueChange)
	label := tela.UseElementRef()
	own := tela.UseElementRef()
	ref := o.Ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	g := &radioGroup[K]{r: r, val: val, cfg: o, rtl: dir == "rtl", prefix: id}
	name := h.AriaLabelledBy(label)
	switch {
	case o.AriaLabel != "":
		name = h.AriaLabel(o.AriaLabel)
	case o.AriaLabelledBy != (tela.ElementRef{}):
		name = h.AriaLabelledBy(o.AriaLabelledBy)
	}
	ps := []tela.Prop{h.Ref(ref), h.Role("radiogroup"), h.AriaOrientation(orient), name}
	if o.Required {
		ps = append(ps, h.AriaRequired(true))
	}
	if o.ReadOnly {
		ps = append(ps, h.AriaReadOnly(true))
	}
	if o.Disabled {
		ps = append(ps, h.AriaDisabled(true))
	}
	cur, has := val.value()
	return RadioGroupResult[K]{
		Props: core.MergeProps(ps...), LabelProps: h.Ref(label),
		Value: cur, HasValue: has, Ref: ref, g: g,
	}
}

func (g *radioGroup[K]) isSelected(k K) bool {
	cur, has := g.val.value()
	return has && cur == k
}

// tabStop is whether the radio with key k is the one Tab reaches: the
// checked one, or the first enabled one while no enabled radio is checked.
func (g *radioGroup[K]) tabStop(k K) bool {
	if cur, has := g.val.value(); has && g.r.enabled(cur) {
		return cur == k
	}
	first, ok := g.r.first()
	return ok && first == k
}

// check asks for the radio with key k as the value, unless the group or
// the radio does not allow it.
func (g *radioGroup[K]) check(k K) {
	if g.cfg.ReadOnly || !g.r.enabled(k) {
		return
	}
	g.val.request(k)
}

func (g *radioGroup[K]) onKeyDown(k K, e tela.KeyboardEvent) {
	// Only the radio itself handles keys: a key typed in a field inside it
	// is the field's.
	if !g.r.refFor(k).Focused() {
		return
	}
	horizontal := g.cfg.Orientation == "horizontal"
	if d := arrowDirection(e.Key, horizontal, g.rtl, true); d != 0 {
		e.PreventDefault()
		if nk, ok := g.r.next(k, d > 0, true); ok && nk != k {
			// Focus first, so that the radio scrolls into view; then check.
			g.r.refFor(nk).Focus()
			g.check(nk)
		}
		return
	}
	switch e.Key {
	case " ":
		e.PreventDefault()
		if !e.Repeat {
			g.check(k)
		}
	case tela.KeyEnter:
		e.PreventDefault() // a radio does not activate with Enter, and neither does its Button
	}
}

// choice is the value of a control that has at most one chosen key,
// controlled by the caller (Value points to its state) or kept by the hook.
// It is boolState's rule for a key: the handlers read and write it
// directly, so a second request in the same event sees the first.
type choice[K comparable] struct {
	cur        K
	has        bool
	controlled bool
	onChange   func(K)
	setCur     tela.Setter[optional[K]]
}

// optional is a key that may be missing, comparable so that it can be the
// state of UseState.
type optional[K comparable] struct {
	k  K
	ok bool
}

func (c *choice[K]) value() (K, bool) { return c.cur, c.has }

// request asks for k: an uncontrolled value takes it, a controlled one only
// reports it. It does nothing when k already is the value.
func (c *choice[K]) request(k K) {
	if c.has && c.cur == k {
		return
	}
	if !c.controlled {
		c.cur, c.has = k, true // a second request in the same event sees it
		c.setCur.Set(optional[K]{k, true})
	}
	if c.onChange != nil {
		c.onChange(k)
	}
}

// useChoice returns the choice of this build.
func useChoice[K comparable](value, def *K, onChange func(K)) *choice[K] {
	var start optional[K]
	if def != nil {
		start = optional[K]{*def, true}
	}
	internal, setInternal := tela.UseState(start)
	c := &tela.UseRef(choice[K]{}).Current
	cur := internal
	if value != nil {
		cur = optional[K]{*value, true}
	}
	c.cur, c.has, c.controlled, c.onChange, c.setCur = cur.k, cur.ok, value != nil, onChange, setInternal
	return c
}
