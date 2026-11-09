package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

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
//   - a kit component that can be a trigger implements Trigger: [Button]
//     (Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Open")}),
//     [Toggle], [Checkbox], [Switch] and [Slider] (so that they can have a
//     tooltip), and the overlays [Tooltip], [Popover], [Dialog],
//     [AlertDialog] and [DropdownMenu], so that one element can have
//     several overlays: Tooltip{Content: h.Text("More actions"), Trigger:
//     kit.DropdownMenu[string]{Trigger: kit.Button{...}, Items: items}} is a
//     button with a tooltip that opens a menu (Radix's asChild on both
//     triggers; nest them either way);
//   - a trigger's own Disabled wins over the overlay's props, as the
//     overlay's Disabled does: a disabled trigger opens nothing, and a
//     Loading [Button] opens nothing either, from the pointer or the
//     keyboard, while it keeps its tab stop (a loading Button that a
//     [TriggerFunc] renders is the function's: wrap it in
//     [LoadingTrigger]);
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
// merged in, or nil for an empty slot. ref is the ref those props attach
// (the overlay hook's): a trigger that attaches a ref of its own (a kit
// overlay, Toggle, Checkbox, Switch, Slider) is given it to share, since
// an element holds one ref.
func triggerOf(t Trigger, p tela.Prop, ref tela.ElementRef) tela.Component {
	if t == nil {
		return nil
	}
	if rt, ok := t.(refTrigger); ok {
		return rt.withTrigger(p, ref)
	}
	return t.WithTriggerProps(p)
}

// LoadingTrigger returns t as a trigger that is busy while loading is
// true: the overlay that holds it opens nothing, from the pointer or the
// keyboard, and the trigger's element stays enabled and in the tab order
// with aria-busy, as a Loading [Button] in a trigger slot does by itself.
//
// Use it for a trigger the overlay cannot see is busy: a [TriggerFunc] (or
// a component of your own) that renders a loading Button with the props it
// is given. The overlay's hook is set up before the function runs, and the
// function may put the props on any element, so the Button's Loading does
// not reach the overlay:
//
//	kit.DropdownMenu[string]{
//	    Trigger: kit.LoadingTrigger(kit.TriggerFunc(func(p tela.Prop) tela.Node {
//	        return kit.Button{Loading: saving, Props: p, Children: h.Text("Export")}
//	    }), saving),
//	    Items: formats,
//	}
func LoadingTrigger(t Trigger, loading bool) Trigger {
	return loadingTrigger{t: t, loading: loading}
}

// loadingTrigger is a [LoadingTrigger].
type loadingTrigger struct {
	t       Trigger
	loading bool
}

// props adds to the overlay's props, which disable the element when its
// hook is disabled, what keeps a busy trigger focusable.
func (l loadingTrigger) props(p tela.Prop) tela.Prop {
	if !l.loading {
		return p
	}
	return tela.MergeProps(p, h.Disabled(false), h.TabIndex(0), h.AriaBusy(true))
}

func (l loadingTrigger) WithTriggerProps(p tela.Prop) tela.Component {
	return l.t.WithTriggerProps(l.props(p))
}

func (l loadingTrigger) withTrigger(p tela.Prop, ref tela.ElementRef) tela.Component {
	return triggerOf(l.t, l.props(p), ref)
}

func (l loadingTrigger) triggerBusy() bool { return l.loading || isBusy(l.t) }

// busyTrigger is a [Trigger] that ignores presses for now: a Loading
// [Button], or an overlay whose own Trigger is busy. The overlay that holds
// it in its slot disables its hook, so that neither the pointer nor the
// keyboard opens it; the trigger keeps its tab stop (a loading Button
// enables its element again after the overlay's props).
type busyTrigger interface{ triggerBusy() bool }

// isBusy reports whether the trigger t ignores presses for now
// (busyTrigger).
func isBusy(t Trigger) bool {
	b, ok := t.(busyTrigger)
	return ok && b.triggerBusy()
}

// refTrigger is a [Trigger] whose root element has a ref of its own: an
// aria hook's (the press of a Toggle, the track of a Slider, the trigger of
// an overlay nested in another's slot). The overlay that holds it in its
// slot passes its ref, which the trigger's hook takes as its Ref option, so
// both hooks share the one ref of the element.
type refTrigger interface {
	withTrigger(p tela.Prop, ref tela.ElementRef) tela.Component
}

// outerTrigger is what a component that is the Trigger of another
// overlay gets from it: the props for its root element and their ref.
type outerTrigger struct {
	props tela.Prop
	ref   tela.ElementRef
	set   bool
}

// merge returns the outer props merged before own, the component's own
// trigger props.
func (o outerTrigger) merge(own tela.Prop) tela.Prop {
	if !o.set {
		return own
	}
	return tela.MergeProps(o.props, own)
}

// child returns the outer props as a child, nil when there are none.
func (o outerTrigger) child() tela.Child {
	if !o.set {
		return nil
	}
	return o.props
}
