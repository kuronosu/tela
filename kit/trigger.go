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
//     keyboard, while it keeps its tab stop, also when a [TriggerFunc]
//     renders it (it tells the overlay itself; [LoadingTrigger] is for a
//     busy element that is not a Button);
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
// a component of your own) that renders a busy element of its own, such
// as an h.Button with a spinner. A loading kit Button that the function
// renders needs none: it tells the overlay that holds the slot (ADR-260),
// which builds again with its hook disabled before the frame paints.
//
//	kit.DropdownMenu[string]{
//	    Trigger: kit.LoadingTrigger(kit.TriggerFunc(func(p tela.Prop) tela.Node {
//	        return h.Button(p, h.Class("..."), spinner(saving), h.Text("Export"))
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

// A loading Button that a TriggerFunc (or a component of your own)
// renders in a trigger slot tells the overlay that holds the slot through
// a context (ADR-260): the overlay's hook is set up before the function
// runs, so the Button reports itself in a layout effect, and the overlay
// builds again, before the frame paints, with its hook disabled and the
// props that keep the busy element focusable (LoadingTrigger's). An
// overlay in another's slot passes what its own slot reported on, and
// the content of an overlay has no slot: a loading Button in a dialog does
// not disable the dialog's trigger.

// slotState is the state of one overlay's trigger slot: how many of the
// components in it (Buttons, nested overlays) are busy, and the setter
// that rebuilds the overlay. Each reporter counts once, so a sibling that
// is not busy, or its unmount, leaves another one's report alone.
type slotState struct {
	busy int
	set  tela.Setter[bool]
}

// report records that one reporter of the slot, which was busy (was) or
// not, now is busy or not.
func (s *slotState) report(was, busy bool) {
	if s == nil || was == busy {
		return
	}
	if busy {
		s.busy++
	} else {
		s.busy--
	}
	s.set.Set(s.busy > 0)
}

// triggerSlot is the value of triggerSlotCtx: the slot a component is
// in, the zero value outside every trigger slot.
type triggerSlot struct{ s *slotState }

var triggerSlotCtx = tela.CreateContext(triggerSlot{})

// overlaySlot is an overlay's trigger slot as its Build uses it.
type overlaySlot struct {
	slot triggerSlot
	// dyn is whether a Button in the slot reported that it is loading.
	dyn bool
}

// useTriggerSlot is the hook of an overlay with a trigger slot: it
// returns the slot, and passes what the slot reported on to the slot the
// overlay itself is in, when it is another overlay's trigger.
func useTriggerSlot() overlaySlot {
	dyn, set := tela.UseState(false)
	s := tela.UseOnce(func() *slotState { return &slotState{} })
	s.set = set
	useReportBusy(dyn)
	return overlaySlot{slot: triggerSlot{s}, dyn: dyn}
}

// useReportBusy tells the trigger slot that the calling component is in,
// if any, whether it is busy, and that it is not once it unmounts or
// moves to another slot.
func useReportBusy(busy bool) {
	outer := tela.UseContext(triggerSlotCtx)
	told := tela.UseRef(false) // what this component told outer
	tela.UseLayoutEffect(func() func() {
		outer.s.report(told.Current, busy)
		told.Current = busy
		return nil
	}, busy, outer)
	tela.UseLayoutEffect(func() func() {
		return func() {
			outer.s.report(told.Current, false)
			told.Current = false
		}
	}, outer)
}

// props are the trigger's props p with what keeps a busy trigger
// focusable while a Button in the slot reported that it is loading.
func (o overlaySlot) props(p tela.Prop) tela.Prop {
	if !o.dyn {
		return p
	}
	return loadingTrigger{loading: true}.props(p)
}

// wrap returns the trigger's component inside the slot.
func (o overlaySlot) wrap(c tela.Component) tela.Child {
	if c == nil {
		return nil
	}
	return triggerSlotCtx.Provider(o.slot, c)
}

// outside returns the overlay's content outside every trigger slot.
func outsideSlot(c tela.Child) tela.Child {
	return triggerSlotCtx.Provider(triggerSlot{}, c)
}
