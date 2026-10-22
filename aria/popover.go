// Portions of this file follow React Aria's usePopover
// (https://github.com/adobe/react-spectrum, packages/@react-aria/overlays),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"cmp"
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// PopoverOptions configures [UsePopover].
type PopoverOptions struct {
	// Trigger is the ref of the element the popover is anchored to, the
	// trigger that opens it ([OverlayTriggerResult].Ref).
	Trigger tela.ElementRef
	// Open is whether the popover is open, and OnClose how it is closed:
	// see [OverlayOptions].
	Open    bool
	OnClose func()
	// Placement is where the popover goes around its trigger, one of
	// Floating UI's 12 placements such as "bottom-start"; "bottom" by
	// default. It flips to the opposite side when there is no room.
	Placement string
	// Offset is the distance in pixels between the trigger and the
	// popover.
	Offset float32
	// Floating are more options of [UseFloating], applied after the
	// popover's own (the placement, [Offset], [Flip] and [Shift] with 12 px
	// of room from the window's edges) and before the arrow's: a [Size]
	// that limits the height to the room available, say.
	Floating []FloatingOption
	// Modal makes the rest of the window inert while the popover is open,
	// as a menu does (React Aria's popovers are modal unless isNonModal
	// is set: here it is the other way round). A press on the trigger then
	// closes the popover, as it hits no element; a non-modal popover does
	// not close for the trigger's presses, which toggle it.
	Modal bool
	// KeyboardDismissDisabled keeps Escape from closing the popover.
	KeyboardDismissDisabled bool
	// ShouldCloseOnInteractOutside decides, for a press outside the popover
	// (and outside the popovers and dialogs opened from it), whether it
	// closes it; nil closes it for every press but the non-modal trigger's.
	ShouldCloseOnInteractOutside func(tela.PointerEvent) bool
	// Ref is the popover element's ref, for a hook used together with
	// others on the same element (the overlay trigger's OverlayRef, a
	// dialog); the zero value makes the hook create its own.
	Ref tela.ElementRef
}

// PopoverResult is what [UsePopover] returns.
type PopoverResult struct {
	// Props go to the popover's element: it is fixed to the window and
	// placed next to the trigger in the frame that shows it, it is on the
	// overlay stack (Escape and presses outside close the top one), and
	// the ref. Render the element in a Portal.
	Props tela.Prop
	// ArrowProps go to an arrow element inside the popover, which points at
	// the trigger.
	ArrowProps tela.Prop
	// Placement is the placement the popover is in, after flipping; Side
	// and Align split it. They follow the engine by one frame: see
	// [Floating].
	Placement, Side, Align string
	// Ref is the ref attached to the popover's element by Props.
	Ref tela.ElementRef
}

// UsePopover returns the props of a popover (React Aria's usePopover): a
// floating element positioned by [UseFloating] next to its trigger, and
// dismissed by [UseOverlay]: Escape, a press outside, or the focus going
// to another element closes it (never the trigger's press or focus, which
// toggle it). A press inside a popover opened from it, in a Portal or not,
// is inside.
//
//	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
//	pop := aria.UsePopover(aria.PopoverOptions{
//	    Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close,
//	    Placement: "bottom-start", Offset: 4,
//	})
//	return Fragment(
//	    Button(trig.TriggerProps, Text("Filters")),
//	    Portal(Presence(If(trig.Open,
//	        Div(pop.Props, Exit("opacity-0"),
//	            Class("w-64 rounded-md border bg-popover p-4 shadow-md transition starting:opacity-0"),
//	            Div(pop.ArrowProps, Class("size-2 rotate-45 border bg-popover")),
//	            Text("Content"))))))
//
// Put [UseDialog] on the same element, sharing the ref, when the popover
// holds a form or content to be read as a dialog.
func UsePopover(o PopoverOptions) PopoverResult {
	own, arrow := tela.UseElementRef(), tela.UseElementRef()
	ref := o.Ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	placement := cmp.Or(o.Placement, "bottom")
	opts := []FloatingOption{Placement(placement), Offset(o.Offset), Flip(), Shift(12)}
	opts = append(opts, o.Floating...)
	opts = append(opts, Arrow(arrow))
	fl := UseFloating(o.Trigger, opts...)

	trigger := o.Trigger
	ov := overlayProp(OverlayOptions{
		Open: o.Open, OnClose: o.OnClose, Dismissable: true, Modal: o.Modal,
		KeyboardDismissDisabled: o.KeyboardDismissDisabled, ShouldCloseOnBlur: true,
		ShouldCloseOnInteractOutside: func(e tela.PointerEvent) bool {
			if !o.Modal && trigger.Bounds().Contains(e.X, e.Y) {
				return false
			}
			return o.ShouldCloseOnInteractOutside == nil || o.ShouldCloseOnInteractOutside(e)
		},
	}, trigger.Focused)
	return PopoverResult{
		Props:      core.MergeProps(h.Ref(ref), fl.Props, ov),
		ArrowProps: core.MergeProps(h.Ref(arrow), fl.ArrowProps),
		Placement:  fl.Placement, Side: fl.Side, Align: fl.Align,
		Ref: ref,
	}
}
