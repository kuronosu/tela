package kit

import "github.com/kuronosu/tela"

// Trigger is what a trigger slot takes: the Trigger field of [Dialog],
// [AlertDialog], [Popover] and [Tooltip], and of the menus, selects and
// pickers that open something. It is a component whose root element can
// receive the props of a trigger: the press that opens the overlay, the
// aria-haspopup, aria-expanded and aria-controls that say what it does, the
// ref the overlay is placed against, the hover and focus handlers of a
// tooltip. The overlay builds those props with its aria hook and calls
// WithTriggerProps to put them on the trigger.
//
// One rule holds for every trigger slot of the kit:
//
//   - a kit component that can be a trigger implements Trigger, and [Button]
//     does: Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Open")};
//   - a component of your own implements it with one method, merging p into
//     the props of its root element (the way a kit component merges it with
//     its Props field);
//   - anything else, an element or a function component, is made a Trigger
//     with [TriggerFunc], which receives the props and puts them on the
//     element it chooses.
//
// A Trigger is the equivalent of Radix's asChild: the overlay does not wrap
// the trigger in an element of its own, so the button you wrote is the one
// the screen reader finds, with its name and its tab stop.
type Trigger interface {
	// WithTriggerProps returns the component with p applied to its root
	// element, after the props the component sets itself, so that p's
	// handlers run along with the component's own.
	WithTriggerProps(p tela.Prop) tela.Component
}

// TriggerFunc makes a function a [Trigger]: it is given the props of the
// trigger and returns the element that carries them. Use it for a trigger
// that is not a kit component, such as an avatar or a link:
//
//	kit.Popover{
//	    Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
//	        return h.Button(p, cls("rounded-full"), kit.Avatar{Fallback: "AB"})
//	    }),
//	    Children: profile,
//	}
//
// The function runs as a component of its own, so it may use hooks. The
// props make the element a button that can be pressed and focused; give a
// different role with h.Role after p when the element is something else.
type TriggerFunc func(p tela.Prop) tela.Node

// WithTriggerProps returns the component that calls f with p.
func (f TriggerFunc) WithTriggerProps(p tela.Prop) tela.Component {
	return triggerRender{f: f, p: p}
}

// triggerRender is the component of a [TriggerFunc] with its props.
type triggerRender struct {
	f TriggerFunc
	p tela.Prop
}

func (r triggerRender) Build() tela.Node { return r.f(r.p) }

// triggerOf returns the component of a trigger slot with the trigger props
// merged in, or nil for an empty slot.
func triggerOf(t Trigger, p tela.Prop) tela.Component {
	if t == nil {
		return nil
	}
	return t.WithTriggerProps(p)
}
