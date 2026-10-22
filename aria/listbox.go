// Portions of this file follow React Aria's useListBox, useOption,
// useListBoxSection and react-stately's useListState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/listbox
// and packages/@react-stately/list), Copyright 2020 Adobe, licensed under
// the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// checkChoice returns v, or def when v is empty. Another value panics in
// dev mode, naming the option, the choices and the closest one, and counts
// as def otherwise.
func checkChoice(hook, option, v, def string, choices ...string) string {
	if v == "" {
		return def
	}
	for _, c := range choices {
		if v == c {
			return v
		}
	}
	if core.DevNow() {
		panic(fmt.Sprintf("aria: %s %s %q: unknown value; use one of %s%s",
			hook, option, v, `"`+strings.Join(choices, `", "`)+`"`, didYouMean(v, choices)))
	}
	return def
}

// ListBoxOptions configures [UseListBox]. K is the type of the items'
// keys: a string, an int, an ID type; any comparable value.
type ListBoxOptions[K comparable] struct {
	// Keys are the keys of the items, in the order they are shown. The
	// list must be given on every build; a key that goes takes its
	// selection with it only when the caller drops it (the focus moves to
	// the nearest item).
	Keys []K
	// TextValue is the text of an item, for typeahead (typing "ba" focuses
	// the first item whose text starts with it, ignoring case). Without
	// it the text of an item is its key as fmt.Sprint prints it, so
	// typeahead works for string keys; give TextValue when the keys are
	// IDs, or DisallowTypeAhead to turn typeahead off. While a search is
	// going (it expires after one second without keys) a Space is a
	// character of it and selects nothing, as in React Aria.
	TextValue func(K) string
	// DisabledKeys are the items that can not be focused, selected or
	// acted on. They are skipped by the keyboard, and get Disabled and
	// AriaDisabled, so both the `disabled:` and the `aria-disabled:`
	// variants match.
	DisabledKeys []K
	// SelectionMode is "none" (the default: the list has actions, not a
	// selection), "single" or "multiple".
	SelectionMode string
	// SelectionBehavior is "toggle" (the default: a click or Space toggles
	// the item, so several can be selected with plain clicks) or
	// "replace" (a click selects only that item, Ctrl+click toggles it,
	// Shift+click selects a range, and moving the focus with the arrow
	// keys moves the selection with it).
	SelectionBehavior string
	// Selected is the selection of a controlled list, with
	// OnSelectionChange asking for the next one. The zero Selection leaves
	// the selection to the hook, which starts at DefaultSelected. See
	// [Selection].
	Selected          Selection[K]
	DefaultSelected   Selection[K]
	OnSelectionChange func(Selection[K])
	// DisallowEmptySelection keeps one item selected once there is one.
	DisallowEmptySelection bool
	// DisallowSelectAll turns off Ctrl+A (Cmd+A on a Mac) in a multiple
	// selection.
	DisallowSelectAll bool
	// OnAction is called when an item is activated: with Enter, with a
	// tap, or with a click when nothing is selected (SelectionBehavior
	// "toggle") or the list does not select (SelectionMode "none"). With
	// SelectionBehavior "replace" and a selection, a click selects and a
	// double click or Enter runs the action.
	OnAction func(K)
	// Orientation is "vertical" (the default; ArrowUp and ArrowDown move
	// the focus) or "horizontal" (ArrowLeft and ArrowRight).
	Orientation string
	// Direction is "ltr" (the default) or "rtl": the arrow keys of a
	// horizontal list swap in a right-to-left one.
	Direction string
	// ShouldFocusWrap makes ArrowDown on the last item focus the first,
	// and ArrowUp on the first the last.
	ShouldFocusWrap bool
	// ShouldUseVirtualFocus keeps the focus on the list itself and
	// marks the focused item with aria-activedescendant, as a combobox's
	// popup does: items are not focusable, and IsFocused(k) says which one
	// is highlighted.
	ShouldUseVirtualFocus bool
	// ExternalFocus is ShouldUseVirtualFocus for a list that never has
	// the focus: another element does (a combobox's input), and calls
	// [Collection.HandleKeyDown] from its key handler and puts
	// [Collection.ActiveDescendant] on itself. The list is not a tab stop
	// and pressing an item does not move the focus away from that element.
	// The keys of a text field stay its own: HandleKeyDown handles the
	// arrow keys, PageUp, PageDown and Enter, and there is no typeahead.
	ExternalFocus bool
	// ShouldFocusOnHover focuses an item when the mouse moves over it.
	ShouldFocusOnHover bool
	// DisallowTypeAhead turns off typeahead.
	DisallowTypeAhead bool
	// EscapeKeyBehavior is "clearSelection" (the default: Escape clears
	// the selection, and is not passed on while there is one) or "none".
	EscapeKeyBehavior string
	// AutoFocus focuses the list when it mounts: the first selected item
	// if there is one, else the item FocusStrategy says, else the list.
	AutoFocus bool
	// FocusStrategy is FocusFirst or FocusLast: it implies AutoFocus.
	FocusStrategy string
	// AriaLabel and AriaLabelledBy name the list.
	AriaLabel      string
	AriaLabelledBy tela.ElementRef
	// Ref is the list's ref, for a hook that is used together with others
	// on one element (see the package documentation) and for a
	// VirtualList: the list scrolls its items into view through it. The
	// zero value makes the hook create its own.
	Ref tela.ElementRef
}

// ListBoxResult is what [UseListBox] returns.
type ListBoxResult[K comparable] struct {
	Collection[K]
	// Props go to the list's element: the "listbox" role, aria-multiselectable,
	// the ref, the key handler and, with a roving focus, a tab stop while
	// no item is focused. Give them to the element that scrolls (or to a
	// h.VirtualList).
	Props tela.Prop
	// Selected is the current selection (for a single selection, its
	// [Selection.First] is the selected key).
	Selected Selection[K]
	// Ref is the ref attached to the list by Props.
	Ref tela.ElementRef
}

// UseListBox holds the state of a list box and returns the props of its
// element and its options (React Aria's useListBox with useListState,
// useSelectableList and useOption): a keyed collection with a focused
// key, a selection, and the keyboard and pointer rules of a listbox.
//
//	var fruit = []string{"apple", "banana", "cherry"}
//
//	lb := aria.UseListBox(aria.ListBoxOptions[string]{
//	    Keys: fruit, TextValue: strings.ToUpper, SelectionMode: "single",
//	    AriaLabel: "Fruit",
//	})
//	return Div(lb.Props, Class("w-48 rounded-md border p-1"),
//	    h.Range(len(fruit), func(i int) tela.Node {
//	        k := fruit[i]
//	        return Div(lb.ItemProps(k), Key(k),
//	            Class("rounded px-2 py-1 aria-selected:bg-primary aria-selected:text-primary-foreground focus-visible:ring-2"),
//	            Text(k))
//	    }))
//
// The keyboard: ArrowUp and ArrowDown (ArrowLeft and ArrowRight for a
// horizontal list) move the focus, skipping disabled items, with
// ShouldFocusWrap going around the ends; Home and End jump; PageUp and
// PageDown move a viewport of items; typing text focuses the next item
// that starts with it (the string expires after a second); Space and
// Enter select or act; with a multiple selection Shift+ArrowDown extends
// the range, Ctrl+A selects everything and Ctrl+ArrowDown moves without
// selecting (with SelectionBehavior "replace"). A click selects, Shift+click
// extends, Ctrl+click toggles.
//
// Style items with the `aria-selected:` and `aria-disabled:` variants, with
// `focus-visible:` (the element that has the focus), or in Go from
// IsSelected, IsFocused and IsDisabled. The list uses a roving focus: the
// focused item's element has the focus of the window and is the list's only
// tab stop. Render the items as Divs or Lis, not Buttons.
//
// # Selection
//
// The selection is a [Selection]. A list keeps it itself by default:
//
//	aria.ListBoxOptions[string]{Keys: fruit, SelectionMode: "multiple",
//	    DefaultSelected: aria.SelectionOf("banana")}
//
// and is controlled by passing the selection, next to the callback that
// asks for changes (OnSelectionChange is also called for an uncontrolled
// list):
//
//	sel, setSel := tela.UseState(aria.SelectionOf("banana"))
//	aria.ListBoxOptions[string]{Keys: fruit, SelectionMode: "multiple",
//	    Selected: sel, OnSelectionChange: setSel.Set}
//
// Ctrl+A passes [All]; resolve it with [Selection.Resolve] to get keys.
//
// # Long lists
//
// A h.VirtualList works as the list: give it Props and render its items
// with ItemProps. Keys that are not rendered can be focused: the list
// scrolls to their index and the item takes the focus when it mounts.
// PageUp and PageDown measure the rendered items. Typeahead and Ctrl+A
// work on the whole list.
//
//	h.VirtualList(len(keys), func(int) float32 { return 32 }, func(i int) tela.Node {
//	    return Div(lb.ItemProps(keys[i]), Key(keys[i]), Class("h-8"), Text(keys[i]))
//	}, lb.Props, Class("h-64"))
//
// # Sections
//
// Section returns the props of a group of items and of its heading, which
// names the group: the keys stay one flat list.
func UseListBox[K comparable](o ListBoxOptions[K]) ListBoxResult[K] {
	const hook = "UseListBox: ListBoxOptions."
	mode := checkChoice(hook, "SelectionMode", o.SelectionMode, "none", "none", "single", "multiple")
	behavior := checkChoice(hook, "SelectionBehavior", o.SelectionBehavior, "toggle", "toggle", "replace")
	orient := checkChoice(hook, "Orientation", o.Orientation, "vertical", "vertical", "horizontal")
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	esc := checkChoice(hook, "EscapeKeyBehavior", o.EscapeKeyBehavior, "clearSelection", "clearSelection", "none")
	strategy := checkChoice(hook, "FocusStrategy", o.FocusStrategy, "", FocusFirst, FocusLast)
	c := useCollection(collConfig[K]{
		kind: kindListBox, role: "listbox",
		keys: o.Keys, textValue: o.TextValue, disabled: o.DisabledKeys,
		mode: mode, behavior: behavior,
		selected: o.Selected, defaultSelected: o.DefaultSelected, onSelectionChange: o.OnSelectionChange,
		disallowEmpty: o.DisallowEmptySelection, disallowSelectAll: o.DisallowSelectAll,
		wrap: o.ShouldFocusWrap, orientation: orient, rtl: dir == "rtl",
		disableType: o.DisallowTypeAhead || o.ExternalFocus, escapeClears: esc == "clearSelection",
		virtualFocus: o.ShouldUseVirtualFocus || o.ExternalFocus, external: o.ExternalFocus,
		focusOnHover: o.ShouldFocusOnHover,
		autoFocus:    o.AutoFocus, strategy: strategy,
		onAction: o.OnAction,
		label:    o.AriaLabel, labelledBy: o.AriaLabelledBy, ref: o.Ref,
	})
	return ListBoxResult[K]{Collection: Collection[K]{c}, Props: c.containerProps(), Selected: c.sel, Ref: c.ref}
}

// Collection is what the collection hooks share: the state of the items
// (selected, focused, disabled), the props of an item and the way to move
// the focus. It is embedded in the results of [UseListBox], [UseMenu] and
// [UseTabs].
type Collection[K comparable] struct{ c *collState[K] }

// ItemProps returns the props of the item with key k: its role (option,
// menuitem... by the hook), the aria state (aria-selected, aria-checked,
// aria-disabled), the ref, a tab stop (0 on the focused item, -1 on the
// others, none on a disabled one) and the pointer and focus handlers. Give
// every item of Keys its props, and the element a key of its own. An item
// element holds one ref: do not attach another (use [Collection.ItemRef]).
func (r Collection[K]) ItemProps(k K) tela.Prop { return r.c.itemProps(k) }

// ItemRef returns the ref that ItemProps attaches to the item with key k,
// for measuring or focusing it; it has no element while the item is not
// rendered.
func (r Collection[K]) ItemRef(k K) tela.ElementRef { return r.c.refFor(k) }

// IsSelected reports whether the item is selected (with [All], every item
// that is not disabled).
func (r Collection[K]) IsSelected(k K) bool { return r.c.isSelected(k) }

// IsDisabled reports whether the item is in DisabledKeys.
func (r Collection[K]) IsDisabled(k K) bool { return r.c.isDisabled(k) }

// IsFocused reports whether the item is the focused one while the focus is
// in the collection: with a roving focus, the element that has the focus;
// with a virtual focus, the highlighted item.
func (r Collection[K]) IsFocused(k K) bool {
	c := r.c
	return c.hasFocused && c.focused == k && (c.within || c.cfg.external)
}

// IsFocusVisible is IsFocused for a focus that came with the keyboard: the
// item that should show a focus ring.
func (r Collection[K]) IsFocusVisible(k K) bool {
	return r.IsFocused(k) && (r.c.visible || r.c.cfg.external)
}

// Focused returns the focused key, and whether there is one (it is
// remembered after the focus leaves the collection, since it is the item
// that takes the focus when it comes back).
func (r Collection[K]) Focused() (K, bool) { return r.c.focused, r.c.hasFocused }

// FocusKey focuses the item with key k from a handler or an effect (a
// disabled or unknown key is ignored). With a roving focus the item's
// element takes the focus, scrolling a virtual list to it first when it is
// not rendered; with a virtual focus it becomes the highlighted item.
func (r Collection[K]) FocusKey(k K) {
	if r.c.has(k) && !r.c.isDisabled(k) {
		r.c.within = true
		r.c.moveFocus(k)
	}
}

// Focus focuses the first item ([FocusFirst]), the last ([FocusLast]), or
// the first selected one (the first when none is selected) for any other
// strategy, like the focus a Tab gives the collection.
func (r Collection[K]) Focus(strategy string) {
	c := r.c
	var (
		k  K
		ok bool
	)
	switch strategy {
	case FocusFirst:
		k, ok = c.firstKey()
	case FocusLast:
		k, ok = c.lastKey()
	default:
		if k, ok = c.firstSelected(); !ok {
			k, ok = c.firstKey()
		}
	}
	if ok {
		r.FocusKey(k)
	}
}

// ClearFocus forgets the focused key: no item is highlighted.
func (r Collection[K]) ClearFocus() { r.c.clearFocus() }

// HandleKeyDown handles a key as the list's own handler does (navigation,
// selection, typeahead, Enter and Space on the focused item). Props already
// carries it; this is for another element that has the focus while the list
// is shown, as a combobox's input does with ExternalFocus.
func (r Collection[K]) HandleKeyDown(e tela.KeyboardEvent) {
	r.c.syncFocus()
	r.c.handleKey(e)
}

// ActiveDescendant returns the aria-activedescendant prop that points to
// the focused item, for the element that has the focus when the focus is
// virtual and external; it is empty while no item is focused.
func (r Collection[K]) ActiveDescendant() tela.Prop {
	c := r.c
	if c.hasFocused && c.has(c.focused) {
		return h.AriaActiveDescendant(c.refFor(c.focused))
	}
	return core.MergeProps()
}

// SectionResult is what [Collection.Section] returns.
type SectionResult struct {
	// Props go to the element that groups the items: the "group" role,
	// named by the heading.
	Props tela.Prop
	// HeadingProps go to the heading's element, which only names the group
	// (role "presentation").
	HeadingProps tela.Prop
}

// Section returns the props of a group of items and of its heading
// (React Aria's useListBoxSection and useMenuSection). name identifies the
// section within the list; sections do not change the keys, which stay one
// flat list.
//
//	fruit := lb.Section("fruit")
//	Div(fruit.Props,
//	    Div(fruit.HeadingProps, Class("text-xs text-muted-foreground"), Text("Fruit")),
//	    Div(lb.ItemProps("apple"), Key("apple"), Text("Apple")))
func (r Collection[K]) Section(name string) SectionResult {
	c := r.c
	s, ok := c.sections[name]
	if !ok {
		s = &sectionRefs{group: core.NewElementRef(), heading: core.NewElementRef()}
		c.sections[name] = s
	}
	return SectionResult{
		Props:        core.MergeProps(h.Ref(s.group), h.Role("group"), h.AriaLabelledBy(s.heading)),
		HeadingProps: core.MergeProps(h.Ref(s.heading), h.Role("presentation")),
	}
}
