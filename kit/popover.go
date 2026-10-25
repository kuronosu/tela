package kit

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// PopoverStyle is the recipe of [Popover] (shadcn/ui's PopoverContent).
type PopoverStyle struct {
	// Root is the panel: surface, border, shadow, padding and the
	// entrance. Its width is w-72 (18rem); override it with Class.
	Root string
	// Exit is what the panel transitions to as it closes.
	Exit string
	// Sides adds classes for the side of the trigger the panel is on
	// ("top", "right", "bottom" or "left"): the transform origin of the
	// entrance, which scales out of the trigger's edge, and the slide of
	// 0.5rem it starts from (shadcn's data-[side=bottom]:slide-in-from-top-2
	// and the like).
	Sides map[string]string
}

func init() {
	defaultStyle.Popover = PopoverStyle{
		Root: "z-50 w-72 rounded-md border border-border bg-popover p-4 text-popover-foreground shadow-md outline-hidden " +
			"transition duration-150 starting:scale-95 starting:opacity-0",
		Exit: "scale-95 opacity-0",
		Sides: map[string]string{
			"top":    "origin-bottom starting:translate-y-2",
			"right":  "origin-left starting:-translate-x-2",
			"bottom": "origin-top starting:-translate-y-2",
			"left":   "origin-right starting:translate-x-2",
		},
	}
}

// NoOffset as the Offset of a [Popover] puts the panel against its trigger:
// the zero value stands for shadcn's default of 4 px.
const NoOffset float32 = -1

// Popover is a panel anchored to its trigger, shadcn/ui's Popover: it opens
// next to the trigger, scales and fades in, and closes with Escape, a press
// outside it or the focus moving elsewhere. It does not block the page
// (it is not modal): the rest of the window stays in the accessibility tree
// and can take the focus. The press that closes it is not delivered to
// what is under it, as in React Aria. The focus moves into the panel when
// it opens and returns to the trigger when it closes. A press on the
// trigger toggles it.
//
//	kit.Popover{
//	    Trigger:   kit.Button{Variant: kit.Outline, Children: h.Text("Dimensions")},
//	    Placement: "bottom", Align: "start",
//	    Children:  dimensionsForm,
//	}
//
// The Trigger slot is a [Trigger]: a [Button], or anything made one with
// [TriggerFunc]. The panel is placed with Floating UI's algorithm: it flips
// to the other side when there is no room and shifts to stay in the window.
// Open, DefaultOpen and OnOpenChange are the usual controlled and
// uncontrolled pair; the panel exists only while it is open.
type Popover struct {
	// Trigger is the element the popover is anchored to and that opens it.
	Trigger Trigger
	// Children is the content of the panel.
	Children tela.Component
	// Placement is the side of the trigger the panel opens on: "bottom"
	// (the default), "top", "left" or "right"; a Floating UI placement such
	// as "bottom-start" also works. Align is where it sits along that side:
	// "start", "center" (the default) or "end".
	Placement, Align string
	// Offset is the distance in pixels between the trigger and the panel;
	// zero is 4 and [NoOffset] puts them together.
	Offset float32
	// Modal makes the rest of the window inert while the popover is open,
	// as a dialog does. The default is a popover that leaves the page
	// usable.
	Modal bool
	// AriaLabel names the panel (it is a dialog for assistive technologies).
	AriaLabel string

	// Open points to the open state of a controlled popover; nil leaves the
	// state to the popover, which starts closed unless DefaultOpen is set.
	// OnOpenChange is called with the next state when the trigger, Escape or
	// a press outside asks for a change.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)

	// Class is added after the recipe's Root classes, to the panel.
	Class string
	// Props are more props for the panel element.
	Props tela.Child
}

// Build returns the trigger and the popover's portal.
func (p Popover) Build() tela.Node {
	s := &UseStyle().Popover
	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{
		Open: p.Open, DefaultOpen: p.DefaultOpen, OnOpenChange: p.OnOpenChange,
	})
	offset := p.Offset
	switch {
	case offset == 0:
		offset = 4
	case offset < 0:
		offset = 0
	}
	pop := aria.UsePopover(aria.PopoverOptions{
		Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close,
		Placement: placementOf("Popover", p.Placement, p.Align, "bottom"), Offset: offset, Modal: p.Modal,
	})
	dlg := aria.UseDialog(aria.DialogOptions{Modal: p.Modal, AriaLabel: p.AriaLabel, Ref: trig.OverlayRef})
	return h.Fragment(
		triggerOf(p.Trigger, trig.TriggerProps),
		h.Portal(h.Presence(h.If(trig.Open,
			h.Div(pop.Props, dlg.Props, h.Exit(s.Exit),
				h.Class(s.Root, s.Sides[pop.Side], p.Class), p.Props, p.Children)))),
	)
}

// placementOf joins the side and the alignment of a floating component
// into a Floating UI placement, def being the side when none is given. A
// side that already carries an alignment ("bottom-start") keeps it unless
// align overrides it. A misspelled alignment panics in dev mode.
func placementOf(component, placement, align, def string) string {
	placement = cmp.Or(placement, def)
	switch align {
	case "", "center", "start", "end":
	default:
		if devMode() {
			msg := fmt.Sprintf("kit: %s.Align %q is not an alignment; use \"start\", \"center\" or \"end\"", component, align)
			if c := closest(align, []string{"start", "center", "end"}); c != "" {
				msg += fmt.Sprintf(" (did you mean %q?)", c)
			}
			panic(msg)
		}
		align = ""
	}
	if align == "" {
		return placement
	}
	side, _, _ := strings.Cut(placement, "-")
	if align == "center" {
		return side
	}
	return side + "-" + align
}
