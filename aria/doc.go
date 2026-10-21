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
//	hover := aria.UseHover(aria.HoverOptions{})
//	ring := aria.UseFocusRing(aria.FocusRingOptions{})
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
// (UseHover) take none. Without the shared ref each hook attaches its own
// and only the last one stays attached.
//
//	ref := tela.UseElementRef()
//	btn := aria.UseButton(aria.ButtonOptions{Ref: ref, OnPress: func(aria.PressEvent) { save() }})
//	ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
//	return Button(btn.Props, ring.Props, ...)
//
// Do not add h.Ref(other) to an element that has hook props: it replaces
// the hook's ref. Use the Ref field of the hook's result instead.
//
// # Controlled and uncontrolled state
//
// A hook that holds state keeps it itself by default (uncontrolled), and
// lets the caller keep it (controlled) when the caller passes the value
// together with the callback that asks for changes, as h.Input does with
// Value and OnChange:
//
//	// Uncontrolled: DefaultSelected is the initial state.
//	box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})
//	// Controlled: the caller keeps the state.
//	on, setOn := tela.UseState(false)
//	sw := aria.UseSwitch(aria.ToggleOptions{Selected: on, OnChange: setOn.Set})
//
// A controlled control shows what the caller passes: a change that the
// caller does not store is refused, as with any controlled field.
//
// # Hooks
//
// Interactions: [UsePress] (mouse, touch, pen and keyboard as one press),
// [UseHover] (mouse and pen) and [UseFocusRing] (the focus, and whether to
// show it). Focus: [UseFocusScope] (contain, restore and auto-focus the
// focus, and a [FocusManager] that moves it). Controls: [UseButton],
// [UseToggleButton], [UseCheckbox] and [UseSwitch], which add the role,
// the aria state, the tab stop and the toggle state to a press.
//
// # Testing and agents
//
// The roles and states the hooks set are the ones the aria snapshot shows:
// a checkbox made with [UseCheckbox] prints as `checkbox "Name" [checked]`,
// and telatest finds it with ByRole("checkbox", "Name").
package aria
