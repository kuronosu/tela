// Package aria provides unstyled behavior hooks for building components:
// the interaction and accessibility that a button, a checkbox or a dialog
// needs, without a look. It follows React Aria (Adobe's behavior hooks,
// Apache-2.0): the same names, the same rules for pointers, keys and
// focus, translated to Tela's hooks and props. The styled components of
// package kit are built on it; use it directly to build your own.
//
// # The pattern: options in, Props out
//
// A hook takes an options struct and returns a struct. Its Props field is
// one tela.Prop that carries everything the hook needs on the element:
// event handlers, the ref, the role and the aria states. Give it to the
// element, and style the element from the other fields of the result:
//
//	press := aria.UsePress(aria.PressOptions{
//	    OnPress:  func(e aria.PressEvent) { save() },
//	    Disabled: busy,
//	})
//	return Button(press.Props, Class("rounded-md px-3 py-1.5"),
//	    If(press.Pressed, Class("scale-95")), Text("Save"))
//
// Props merges several props into one (core's MergeProps): the handlers of
// one event all run, in the order given, and the others (a class, a role)
// follow the same rules as props given one after the other. So hook props
// combine with your own props and with each other:
//
//	ref := tela.UseElementRef() // see "One element, several hooks" below
//	btn := aria.UseButton(aria.ButtonOptions{Ref: ref})
//	hover := aria.UseHover(aria.HoverOptions{})
//	ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
//	return Button(btn.Props, hover.Props, ring.Props,
//	    Class("rounded-md px-3 py-1.5 transition"),
//	    If(hover.Hovered, Class("bg-accent")),
//	    If(ring.FocusVisible, Class("ring-2 ring-primary")), ...)
//
// The hooks are ordinary custom hooks (ADR-085): call them at the top
// level of a component, in the same order on every build.
//
// # One element, several hooks: the ref
//
// A few hooks need the element itself (press measures it to know when the
// pointer leaves, the focus ring asks the engine who has the focus) and
// attach a ref to it. An element has one ref, so when several hooks serve
// one element they must share it: make it once with tela.UseElementRef and
// give it to each hook as its Ref option. The hooks that need no ref
// (UseHover, UseOverlay) take none.
//
//	ref := tela.UseElementRef()
//	btn := aria.UseButton(aria.ButtonOptions{Ref: ref, OnPress: func(aria.PressEvent) { save() }})
//	ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
//	return Button(btn.Props, ring.Props, ...)
//
// Two different refs on one element are a mistake that dev mode reports
// with a panic that names both call sites and this fix (outside dev mode
// the last one wins). That includes h.Ref(other) next to the props of a
// hook: use the Ref field of the hook's result instead, or pass the ref to
// the hook. The same ref twice is fine.
//
// # Controlled and uncontrolled state
//
// A hook that holds state keeps it itself by default (uncontrolled), and
// lets the caller keep it (controlled) when the caller passes a pointer to
// the value, next to the callback that asks for changes. The callback is
// also called for an uncontrolled control, to follow it:
//
//	// Uncontrolled: DefaultSelected is the initial state.
//	box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})
//	// Uncontrolled and notified: read the result's Selected, or listen.
//	box = aria.UseCheckbox(aria.ToggleOptions{OnChange: func(on bool) { save(on) }})
//	// Controlled: the caller keeps the state.
//	on, setOn := tela.UseState(false)
//	sw := aria.UseSwitch(aria.ToggleOptions{Selected: &on, OnChange: setOn.Set})
//
// A controlled control shows what the caller passes: a change that the
// caller does not store is refused, as with any controlled field. The open
// state of the overlay hooks (OverlayTriggerOptions, MenuTriggerOptions and
// TooltipTriggerOptions) follows the same rule with Open, DefaultOpen and
// OnOpenChange.
//
// # Hooks
//
// Interactions: [UsePress] (mouse, touch, pen and keyboard as one press),
// [UseHover] (mouse and pen) and [UseFocusRing] (the focus, and whether to
// show it). Focus: [UseFocusScope] (contain, restore and auto-focus the
// focus, and a [FocusManager] that moves it). Controls: [UseButton]
// (which can submit or reset its Form), [UseToggleButton], [UseCheckbox]
// and [UseSwitch], which add the role, the aria state, the tab stop and
// the toggle state to a press. Positioning: [UseFloating].
//
// Overlays: [UseOverlay] (Escape and a press outside close the top
// overlay; a modal one makes the rest of the window inert), [UseDialog]
// (role, name and focus), [UsePopover] (a floating overlay next to its
// trigger), [UseOverlayTrigger] (the open state and the trigger's props),
// [UseMenuTrigger] (a trigger that opens with press, long press or keys)
// and [UseTooltipTrigger] (hover and focus with React Aria's delays and
// warm-up).
//
// Collections, keyed lists of items with a focused key and a selection:
// [UseListBox], [UseMenu] (the menu of [UseMenuTrigger]), [UseTabs],
// [UseSelect] (a button that opens a list box) and [UseCombobox] (a text
// field that filters one, with the [Contains] and [StartsWith] filters),
// with the [Selection] type for what is selected.
//
// # Overlays
//
// An overlay is rendered by its owner, in a Portal and only while it is
// open; the hooks give the props. The trigger hook holds the state, the
// popover positions and dismisses, the dialog names and focuses, and they
// share the overlay element's ref:
//
//	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
//	pop := aria.UsePopover(aria.PopoverOptions{
//	    Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close,
//	    Placement: "bottom-start", Offset: 4,
//	})
//	dlg := aria.UseDialog(aria.DialogOptions{Ref: trig.OverlayRef, AriaLabel: "Filters"})
//	return Fragment(
//	    Button(trig.TriggerProps, Text("Filters")),
//	    Portal(Presence(If(trig.Open,
//	        Div(pop.Props, dlg.Props, Exit("opacity-0"),
//	            Class("rounded-md border bg-popover p-4 transition starting:opacity-0"), ...)))))
//
// To let an overlay fade out, render it inside a Presence with If and give
// it Exit classes and a transition (and the starting: variant for the
// entrance), as above. An exiting overlay is inert and off the overlay
// stack at once, so the page below works while it fades, a second Escape
// reaches the next overlay, and the hooks' Open value is already false.
//
// Presses and Escape reach only the top overlay. What is opened from an
// overlay is inside it (a popover in a dialog, a tooltip on a button in a
// popover), in a Portal or not.
//
// # Collections
//
// A list box, a menu and a tab list are collections: the caller gives the
// keys of the items (any comparable type) on every build, and renders one
// element per key with the props of ItemProps(key). The hook keeps the
// focused key and the selection, moves the focus with the arrow keys,
// Home, End, PageUp, PageDown and typeahead, and selects with the pointer
// and the keyboard as React Aria does.
//
//	lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: names, SelectionMode: "multiple"})
//	return Div(lb.Props, Range(len(names), func(i int) tela.Node {
//	    return Div(lb.ItemProps(names[i]), Key(names[i]),
//	        Class("aria-selected:bg-accent focus-visible:ring-2"), Text(names[i]))
//	}))
//
// The selection is a [Selection], uncontrolled by default and controlled
// when the options hold a non-zero one:
//
//	sel, setSel := tela.UseState(aria.SelectionOf("b"))
//	aria.ListBoxOptions[string]{Keys: names, SelectionMode: "multiple",
//	    Selected: sel, OnSelectionChange: setSel.Set}
//
// The focus is roving (the focused item's element has it) unless
// ShouldUseVirtualFocus keeps it on the collection and marks the focused
// item with aria-activedescendant. Give the items of a long list to a
// h.VirtualList: keys that are not rendered can still be focused.
//
// A select and a combobox put a list box in a popover next to a trigger or
// an input. They return the props of each of their elements; the popover
// goes in a Portal, only while Open, and the items are rendered from
// ListBox.ItemProps as in a list box:
//
//	sel := aria.UseSelect(aria.SelectOptions[string]{Keys: fruit, Placeholder: "Pick one"})
//	cb := aria.UseCombobox(aria.ComboboxOptions[string]{Items: fruit, AriaLabel: "Fruit"})
//
// # Testing and agents
//
// The roles and states the hooks set are the ones the aria snapshot shows:
// a checkbox made with [UseCheckbox] prints as `checkbox "Name" [checked]`,
// and telatest finds it with ByRole("checkbox", "Name").
package aria
