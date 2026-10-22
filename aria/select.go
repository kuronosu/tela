// Portions of this file follow React Aria's useSelect and
// react-stately's useSelectState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/select and
// packages/@react-stately/select), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"cmp"
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// SelectOptions configures [UseSelect]. The items are keyed like those of
// a list box: see [ListBoxOptions] for Keys, TextValue, DisabledKeys and
// the controlled and uncontrolled Selected, DefaultSelected and
// OnSelectionChange (a select holds one selected key).
type SelectOptions[K comparable] struct {
	Keys []K
	// TextValue is the text of an item: it is what the select shows when
	// the item is selected (SelectResult.Value) and what typeahead matches.
	// The default is fmt.Sprint of the key.
	TextValue         func(K) string
	DisabledKeys      []K
	Selected          Selection[K]
	DefaultSelected   Selection[K]
	OnSelectionChange func(Selection[K])
	// Open points to the open state of a controlled select; nil leaves it
	// to the hook, starting at DefaultOpen. OnOpenChange is called with the
	// next state, as a request for a controlled select and as a
	// notification for an uncontrolled one.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)
	// Placeholder is what SelectResult.Value says while nothing is
	// selected.
	Placeholder string
	// Disabled makes the trigger inert and disabled.
	Disabled bool
	// Placement is where the popover goes around the trigger, one of
	// Floating UI's 12 placements; "bottom-start" by default. Offset is the
	// distance in pixels, and Floating are more options of [UseFloating] (a
	// [Size] that limits the height, say), as in [PopoverOptions].
	Placement string
	Offset    float32
	Floating  []FloatingOption
	// MatchWidth makes the popover as wide as the trigger (see
	// SelectResult.PopoverProps).
	MatchWidth bool
	// Modal makes the rest of the window inert while the list is open, as
	// React Aria's select does; see [PopoverOptions]. The default is a
	// non-modal popover.
	Modal bool
	// DisallowTypeAhead turns off typeahead, on the trigger and in the
	// list.
	DisallowTypeAhead bool
	// AriaLabel names the select when there is no label element; with
	// LabelProps the label element names it. The trigger's name is the
	// value followed by the name, as in React Aria ("Apple Favorite fruit").
	AriaLabel string
	// Ref is the trigger's ref, for a hook used together with others on the
	// same element; the zero value makes the hook create its own.
	Ref tela.ElementRef
}

// SelectResult is what [UseSelect] returns.
type SelectResult[K comparable] struct {
	// TriggerProps go to the trigger, a Button or any element: the press
	// that opens the list, the keys that open it (Enter, Space, ArrowDown
	// and ArrowUp) and change the selection while it is closed (typeahead
	// and ArrowLeft and ArrowRight), the "button" role, a tab stop,
	// aria-haspopup="listbox", aria-expanded, aria-controls while open
	// and aria-labelledby (the value and the label).
	TriggerProps tela.Prop
	// ValueProps go to the element inside the trigger that shows the
	// selected text (or the placeholder): the trigger is named by it.
	ValueProps tela.Prop
	// LabelProps go to the label element: it names the select, and a click
	// on it focuses the trigger.
	LabelProps tela.Prop
	// PopoverProps go to the popover's element, which holds the list
	// box: it is fixed to the window and placed next to the trigger in the
	// frame that shows it, it is on the overlay stack (Escape and a press
	// outside close it) and a focus scope gives the focus back to the
	// trigger when it unmounts. With MatchWidth it is as wide as the
	// trigger. Render it in a Portal, only while Open.
	PopoverProps tela.Prop
	// ArrowProps go to an arrow element inside the popover.
	ArrowProps tela.Prop
	// ListBox is the list: its Props go to the element that holds the
	// items (it scrolls) and ItemProps(k) to each item, as in [UseListBox].
	// The items are rendered inside the popover, only while Open.
	ListBox ListBoxResult[K]
	// Value is the text of the selected item, or the Placeholder.
	Value string
	// Selected is the current selection: [Selection.First] is the
	// selected key.
	Selected Selection[K]
	// Open is whether the list is open; SetOpen asks for a state, Toggle for
	// the other one and Close for closed.
	Open    bool
	SetOpen func(open bool)
	Toggle  func()
	Close   func()
	// Pressed is true while the trigger is pressed.
	Pressed bool
	// Placement is the popover's placement after flipping (see
	// [PopoverResult]).
	Placement string
	// TriggerRef and PopoverRef are the refs of the trigger and the
	// popover.
	TriggerRef, PopoverRef tela.ElementRef
}

// UseSelect holds the state of a select and returns the props of its
// trigger, its popover and its list (React Aria's useSelect with
// useSelectState): a button that shows the selected item and opens a list
// box next to it. A press, Enter, Space and ArrowDown open the list (the
// last also with ArrowUp), the selected item or else the first (the last
// for ArrowUp) has the focus, choosing an item selects it, closes the list
// and gives the focus back to the trigger, and Escape closes it. On the
// closed trigger typing text selects the next item that starts with it and
// the left and right arrows select the previous and next item.
//
//	sel := aria.UseSelect(aria.SelectOptions[string]{
//	    Keys: fruit, TextValue: capital, Placeholder: "Pick a fruit", MatchWidth: true,
//	})
//	return Fragment(
//	    Span(sel.LabelProps, Text("Favorite fruit")),
//	    Button(sel.TriggerProps, Class("flex w-48 justify-between rounded-md border px-3 py-2"),
//	        Span(sel.ValueProps, Text(sel.Value)), Icon(chevronDown)),
//	    Portal(If(sel.Open,
//	        Div(sel.PopoverProps, Class("rounded-md border bg-popover p-1 shadow-md"),
//	            Div(sel.ListBox.Props, Class("max-h-60 overflow-auto"),
//	                Range(len(fruit), func(i int) tela.Node {
//	                    k := fruit[i]
//	                    return Div(sel.ListBox.ItemProps(k), Key(k),
//	                        Class("rounded px-2 py-1.5 focus-visible:bg-accent aria-selected:font-medium"),
//	                        Text(capital(k)))
//	                }))))))
//
// In the aria snapshot the closed select is a button named by its value
// and its label (`button "Banana Favorite fruit": Banana`), and the open
// list, a listbox named by the label, sits next to it.
//
// The list lives in the component that calls the hook (its items are
// rendered by the caller only while open): the hook focuses the selected
// item when the list opens. There is no hidden native select, so a Form
// does not submit the value: read SelectResult.Selected in OnSubmit.
func UseSelect[K comparable](o SelectOptions[K]) SelectResult[K] {
	placement := cmp.Or(o.Placement, "bottom-start")
	text := o.TextValue
	if text == nil {
		text = func(k K) string { return fmt.Sprint(k) }
	}
	listRef, valueRef, labelRef := tela.UseElementRef(), tela.UseElementRef(), tela.UseElementRef()
	// took is whether the key being handled is a space of the typeahead
	// search, which the trigger must not take as "open".
	took := &tela.UseRef(false).Current
	mt := UseMenuTrigger(MenuTriggerOptions{
		Type: "listbox", Open: o.Open, DefaultOpen: o.DefaultOpen, OnOpenChange: o.OnOpenChange,
		Disabled: o.Disabled, Ref: o.Ref, MenuRef: listRef,
		skipKey: func(tela.KeyboardEvent) bool { return *took },
	})
	c := useCollection(collConfig[K]{
		kind: kindListBox, role: "listbox",
		keys: o.Keys, textValue: text, disabled: o.DisabledKeys,
		mode: "single", behavior: "toggle", disallowEmpty: true,
		selected: o.Selected, defaultSelected: o.DefaultSelected, onSelectionChange: o.OnSelectionChange,
		orientation: "vertical", disableType: o.DisallowTypeAhead, focusOnHover: true,
		label: o.AriaLabel, labelledBy: labelRef, ref: listRef,
		onChosen: func(K, bool) { mt.Close() },
	})
	// The list decides what has the focus when it opens and forgets it when
	// it closes.
	was := &tela.UseRef(false).Current
	switch {
	case mt.Open && !*was:
		c.focusOnOpen(mt.FocusStrategy)
	case !mt.Open && *was:
		c.closed()
	}
	*was = mt.Open
	// The focused item takes the focus when it mounts, which does not scroll
	// the list: bring it into view once the list is shown (a layout effect
	// would read the extents of the scroll container before the engine
	// updates them).
	tela.UseEffect(func() func() {
		if mt.Open && c.hasFocused && c.has(c.focused) {
			c.refFor(c.focused).ScrollIntoView()
		}
		return nil
	}, mt.Open)

	// The closed trigger: typeahead and the horizontal arrows change the
	// selection (React Aria's useSelect).
	onKeyDown := func(e tela.KeyboardEvent) {
		*took = false
		if o.Disabled || !mt.Ref.Focused() {
			return
		}
		cur, hasCur := c.sel.First()
		hasCur = hasCur && c.has(cur)
		var (
			k  K
			ok bool
		)
		switch e.Key {
		case tela.KeyArrowLeft, tela.KeyArrowRight:
			e.PreventDefault()
			switch {
			case !hasCur:
				k, ok = c.firstKey()
			case e.Key == tela.KeyArrowLeft:
				k, ok = c.keyAbove(cur)
			default:
				k, ok = c.keyBelow(cur)
			}
		default:
			k, ok, *took = c.typeAhead(e, cur, hasCur)
		}
		if ok {
			c.replaceSelection(k)
		}
	}
	ps := []tela.Prop{h.OnKeyDown(onKeyDown), mt.TriggerProps}
	if o.AriaLabel != "" {
		ps = append(ps, h.AriaLabel(o.AriaLabel), core.AriaLabelledBy(valueRef, labelRef, mt.Ref))
	} else {
		ps = append(ps, core.AriaLabelledBy(valueRef, labelRef))
	}

	size := tela.UseSize(mt.Ref)
	pop := UsePopover(PopoverOptions{
		Trigger: mt.Ref, Open: mt.Open, OnClose: mt.Close,
		Placement: placement, Offset: o.Offset, Floating: o.Floating, Modal: o.Modal,
	})
	popover := []tela.Prop{pop.Props, core.Scope(core.ScopeOptions{Restore: true}),
		// Tab leaves the list as it leaves a native select: the focus goes to
		// the trigger, from where the key moves on, and the list closes.
		h.OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == tela.KeyTab && !e.Ctrl && !e.Alt && !e.Meta {
				mt.Ref.Focus()
				mt.Close()
			}
		})}
	if o.MatchWidth {
		popover = append(popover, h.Style(tela.WidthFn(tela.MapValue(size, func(s tela.Size) float32 { return s.Width }))))
	}

	value := o.Placeholder
	if k, ok := c.sel.First(); ok && c.has(k) {
		value = text(k)
	}
	return SelectResult[K]{
		TriggerProps: core.MergeProps(ps...),
		ValueProps:   h.Ref(valueRef),
		LabelProps: core.MergeProps(h.Ref(labelRef), h.OnClick(func() {
			if !o.Disabled {
				mt.Ref.Focus()
			}
		})),
		PopoverProps: core.MergeProps(popover...),
		ArrowProps:   pop.ArrowProps,
		ListBox:      ListBoxResult[K]{Collection: Collection[K]{c}, Props: c.containerProps(), Selected: c.sel, Ref: c.ref},
		Value:        value,
		Selected:     c.sel,
		Open:         mt.Open,
		SetOpen:      mt.SetOpen,
		Toggle:       mt.Toggle,
		Close:        mt.Close,
		Pressed:      mt.Pressed,
		Placement:    pop.Placement,
		TriggerRef:   mt.Ref,
		PopoverRef:   pop.Ref,
	}
}
