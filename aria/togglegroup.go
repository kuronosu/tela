// Portions of this file follow React Aria's useToggleButtonGroup,
// useToggleButtonGroupItem and useToolbar, and react-stately's
// useToggleGroupState (https://github.com/adobe/react-spectrum,
// packages/@react-aria/button, packages/@react-aria/toolbar and
// packages/@react-stately/toggle), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// ToggleGroupOptions configures [UseToggleGroup]: a row of toggle buttons
// with one value, such as a segmented control or a formatting toolbar. The
// items are keyed like those of a list box: see [ListBoxOptions] for Keys
// and DisabledKeys.
//
// The selection is a [Selection], uncontrolled when Selected is the zero
// Selection (the hook keeps it, starting at DefaultSelected) and controlled
// when Selected is given: the group shows it and only asks
// OnSelectionChange for the next one. OnSelectionChange is also called for
// an uncontrolled group.
type ToggleGroupOptions[K comparable] struct {
	// Keys are the keys of the items, in the order they are shown. The
	// list must be given on every build.
	Keys []K
	// DisabledKeys are the items that can not be focused or toggled.
	DisabledKeys []K
	// Disabled disables every item.
	Disabled bool
	// SelectionMode is "single" (the default: at most one item is on, and
	// the group is a radio group of toggle buttons) or "multiple" (any
	// number is on, and the group is a toolbar).
	SelectionMode string
	// Selected is the selection of a controlled group; the zero Selection
	// leaves it to the hook. In "single" mode its first key is the one that
	// is on.
	Selected Selection[K]
	// DefaultSelected is the selection of an uncontrolled group at the
	// start.
	DefaultSelected Selection[K]
	// OnSelectionChange is called with the next selection when the user
	// toggles an item. An equal selection is not reported.
	OnSelectionChange func(Selection[K])
	// DisallowEmptySelection keeps one item on: toggling the only item that
	// is on does nothing.
	DisallowEmptySelection bool
	// Orientation is "horizontal" (the default; ArrowLeft and ArrowRight
	// move the focus) or "vertical" (ArrowUp and ArrowDown).
	Orientation string
	// Direction is "ltr" (the default) or "rtl", which swaps the
	// horizontal arrows.
	Direction string
	// ShouldFocusWrap makes the arrow keys go around the ends of the group.
	ShouldFocusWrap bool
	// AriaLabel names the group; else it is named by AriaLabelledBy.
	AriaLabel string
	// AriaLabelledBy is the element that names the group.
	AriaLabelledBy tela.ElementRef
	// Ref is the group element's ref, for a hook that is used together
	// with others on one element; the zero value makes the hook create
	// its own.
	Ref tela.ElementRef
}

// ToggleGroupResult is what [UseToggleGroup] returns.
type ToggleGroupResult[K comparable] struct {
	// Props go to the group's element: the role ("radiogroup" for a single
	// selection, "toolbar" for multiple), aria-orientation, its name and
	// the ref.
	Props tela.Prop
	// Selected is the current selection, controlled or not.
	Selected Selection[K]
	// Ref is the ref attached to the group by Props.
	Ref tela.ElementRef

	g *toggleGroup[K]
}

type toggleGroup[K comparable] struct {
	r          *rover[K]
	sel        *selChoice[K]
	cfg        ToggleGroupOptions[K]
	horizontal bool
	rtl        bool
	single     bool
}

// ItemProps returns the props of the item with key k (React Aria's
// useToggleButtonGroupItem): in a single-selection group the "radio" role
// with aria-checked, in a multiple-selection one the "button" role with
// aria-pressed (so the `aria-pressed:` or `checked:` variants style the
// items that are on); the ref, the disabled state, a tab stop on the item
// that had the focus last (else the first one that is on, else the first
// enabled one), and the handlers: a click, a tap, Enter or Space toggles
// the item, and the arrow keys of the group's orientation move the focus
// without toggling.
//
// Render the item with a Button, or with any element that should behave as
// one.
func (r ToggleGroupResult[K]) ItemProps(k K) tela.Prop {
	g := r.g
	on := g.sel.has(k)
	ps := []tela.Prop{h.Ref(g.r.refFor(k)), core.DefaultButtonType("button")}
	if g.single {
		ps = append(ps, h.Role("radio"), h.AriaChecked(on))
	} else {
		ps = append(ps, h.Role("button"), h.AriaPressed(on))
	}
	if g.r.isDisabled(k) {
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
				g.toggle(k)
			}
		}),
		h.OnKeyDown(func(e tela.KeyboardEvent) { g.onKeyDown(k, e) }),
		h.OnFocus(func() { g.r.onFocus(k) }),
	)
	return core.MergeProps(ps...)
}

// IsSelected reports whether the item with key k is on.
func (r ToggleGroupResult[K]) IsSelected(k K) bool { return r.g.sel.has(k) }

// ItemRef returns the ref that ItemProps(k) attaches to the item, to share
// with another hook on its element.
func (r ToggleGroupResult[K]) ItemRef(k K) tela.ElementRef { return r.g.r.refFor(k) }

// UseToggleGroup holds the state of a group of toggle buttons and returns
// the props of the group and its items (React Aria's useToggleButtonGroup
// and useToggleButtonGroupItem). It is the segmented control of a kit:
//
//	view := aria.UseToggleGroup(aria.ToggleGroupOptions[string]{
//	    Keys: []string{"list", "grid"}, DefaultSelected: aria.SelectionOf("list"),
//	    DisallowEmptySelection: true, AriaLabel: "View",
//	})
//	return Div(view.Props, Class("inline-flex rounded-md border"),
//	    h.Range(len(modes), func(i int) tela.Node {
//	        k := modes[i]
//	        return Button(view.ItemProps(k), Key(k), Class("px-3 py-1 aria-checked:bg-accent"), Text(k))
//	    }))
//
// The group is a single tab stop, and the arrows move the focus between
// its items without toggling them, skipping the disabled ones; Home and End
// jump. With SelectionMode "single" pressing the item that is on turns it
// off, unless DisallowEmptySelection.
func UseToggleGroup[K comparable](o ToggleGroupOptions[K]) ToggleGroupResult[K] {
	const hook = "UseToggleGroup: ToggleGroupOptions."
	mode := checkChoice(hook, "SelectionMode", o.SelectionMode, "single", "single", "multiple")
	orient := checkChoice(hook, "Orientation", o.Orientation, "horizontal", "horizontal", "vertical")
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	r := useRover(o.Keys, o.DisabledKeys, o.Disabled)
	own := tela.UseElementRef()
	ref := o.Ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	single := mode == "single"
	sel := useSelChoice(r, o.Selected, o.DefaultSelected, o.OnSelectionChange, single)
	g := &toggleGroup[K]{r: r, sel: sel, cfg: o, horizontal: orient == "horizontal", rtl: dir == "rtl", single: single}
	role := "toolbar"
	if single {
		role = "radiogroup"
	}
	ps := []tela.Prop{h.Ref(ref), h.Role(role), h.AriaOrientation(orient)}
	switch {
	case o.AriaLabel != "":
		ps = append(ps, h.AriaLabel(o.AriaLabel))
	case o.AriaLabelledBy != (tela.ElementRef{}):
		ps = append(ps, h.AriaLabelledBy(o.AriaLabelledBy))
	}
	if o.Disabled {
		ps = append(ps, h.AriaDisabled(true))
	}
	return ToggleGroupResult[K]{Props: core.MergeProps(ps...), Selected: sel.cur, Ref: ref, g: g}
}

// tabStop is whether the item with key k is the one Tab reaches.
func (g *toggleGroup[K]) tabStop(k K) bool {
	if g.r.hasFocused && g.r.enabled(g.r.focused) {
		return g.r.focused == k
	}
	for _, key := range g.r.keys {
		if g.sel.has(key) && g.r.enabled(key) {
			return key == k
		}
	}
	first, ok := g.r.firstKey()
	return ok && first == k
}

// toggle flips the item with key k, as the selection mode and
// DisallowEmptySelection allow.
func (g *toggleGroup[K]) toggle(k K) {
	if !g.r.enabled(k) {
		return
	}
	on := g.sel.has(k)
	var next Selection[K]
	switch {
	case g.single && on:
		next = SelectionOf[K]()
	case g.single:
		next = SelectionOf(k)
	default:
		cur := g.sel.cur.Resolve(g.r.keys)
		next = cur.Toggle(k)
	}
	if g.cfg.DisallowEmptySelection && next.IsEmpty() {
		return
	}
	g.sel.request(next)
}

func (g *toggleGroup[K]) onKeyDown(k K, e tela.KeyboardEvent) {
	// Only the item itself handles keys: a key typed in a field inside it
	// is the field's.
	if !g.r.refFor(k).Focused() {
		return
	}
	if d := arrowDirection(e.Key, g.horizontal, g.rtl, false); d != 0 {
		e.PreventDefault()
		if nk, ok := g.r.next(k, d > 0, g.cfg.ShouldFocusWrap); ok && nk != k {
			g.r.refFor(nk).Focus()
		}
		return
	}
	switch e.Key {
	case tela.KeyHome, tela.KeyEnd:
		e.PreventDefault()
		nk, ok := g.r.firstKey()
		if e.Key == tela.KeyEnd {
			nk, ok = g.r.lastKey()
		}
		if ok && nk != k {
			g.r.refFor(nk).Focus()
		}
	case tela.KeyEnter, " ":
		e.PreventDefault()
		if !e.Repeat {
			g.toggle(k)
		}
	}
}

// selChoice is the selection of a group of toggles, controlled by the
// caller (Selected is given) or kept by the hook.
type selChoice[K comparable] struct {
	r          *rover[K]
	cur        Selection[K]
	controlled bool
	single     bool
	onChange   func(Selection[K])
	// kept is the uncontrolled selection; it lives in the struct (the
	// handlers read and write it directly, as boolState does) and the
	// rover's bump asks for a build.
	kept    Selection[K]
	started bool
}

func (s *selChoice[K]) has(k K) bool { return s.cur.Has(k) && s.r.has(k) }

// request asks for the next selection: an uncontrolled group takes it, a
// controlled one only reports it. An equal selection is ignored.
func (s *selChoice[K]) request(next Selection[K]) {
	if s.cur.Equal(next) {
		return
	}
	if !s.controlled {
		s.kept, s.cur = next, next // a second request in the same event sees it
		s.r.invalidate()
	}
	if s.onChange != nil {
		s.onChange(next)
	}
}

func useSelChoice[K comparable](r *rover[K], selected, def Selection[K], onChange func(Selection[K]), single bool) *selChoice[K] {
	s := tela.UseOnce(func() *selChoice[K] { return &selChoice[K]{r: r} })
	if !s.started {
		s.started = true
		s.kept = def
		if !s.kept.IsSet() {
			s.kept = SelectionOf[K]()
		}
	}
	s.single, s.onChange = single, onChange
	s.controlled = selected.IsSet()
	s.cur = s.kept
	if s.controlled {
		s.cur = selected
	}
	if single && s.cur.Len() > 1 {
		k, _ := s.cur.First()
		s.cur = SelectionOf(k) // a single selection keeps its first key
	}
	return s
}
