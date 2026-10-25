package kit

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// TooltipStyle is the recipe of [Tooltip] (shadcn/ui's TooltipContent and
// its arrow).
type TooltipStyle struct {
	// Root is the tooltip: the foreground color as background, small text
	// and the entrance.
	Root string
	// Exit is what the tooltip transitions to as it closes.
	Exit string
	// Sides adds classes for the side of the trigger the tooltip is on
	// ("top", "right", "bottom" or "left"): the transform origin of the
	// entrance and the slide of 0.5rem it starts from.
	Sides map[string]string
	// Arrow is the arrow: a square turned 45 degrees, half under the
	// tooltip. ArrowSides adds, per side, the margin that sinks it 2 px
	// into the tooltip (shadcn's translate-y-[calc(-50%_-_2px)]); Floating
	// UI puts the arrow's center on the edge.
	Arrow      string
	ArrowSides map[string]string
}

func init() {
	defaultStyle.Tooltip = TooltipStyle{
		Root: "z-50 w-fit rounded-md bg-foreground px-3 py-1.5 text-xs text-balance text-background " +
			"transition duration-150 starting:scale-95 starting:opacity-0",
		Exit: "scale-95 opacity-0",
		Sides: map[string]string{
			"top":    "origin-bottom starting:translate-y-2",
			"right":  "origin-left starting:-translate-x-2",
			"bottom": "origin-top starting:-translate-y-2",
			"left":   "origin-right starting:translate-x-2",
		},
		Arrow: "z-50 size-2.5 rotate-45 rounded-[2px] bg-foreground",
		ArrowSides: map[string]string{
			"top":    "-mt-0.5",
			"right":  "ml-0.5",
			"bottom": "mt-0.5",
			"left":   "-ml-0.5",
		},
	}
}

// Tooltip is a small label that explains its trigger, shadcn/ui's Tooltip:
// it appears when the pointer rests on the trigger or the keyboard focuses
// it, points at it with an arrow, and goes away when the pointer leaves, a
// key is pressed, the trigger is pressed or Escape is. The trigger is
// described by it (aria-describedby), so assistive technologies read it
// after the trigger's own name.
//
//	kit.Tooltip{
//	    Trigger: kit.Button{Variant: kit.Outline, Size: kit.Icon, AriaLabel: "Add", Icon: plus},
//	    Content: h.Text("Add to library"),
//	}
//
// The Trigger slot is a [Trigger]. The tooltips of a window are a group, as
// in shadcn's TooltipProvider and React Aria: only one is open, and once one
// has opened, moving to another trigger shows its tooltip at once, whatever
// its Delay; there is nothing to wrap them in.
//
// A tooltip is for elements that already have a name and a tab stop; it
// should hold text, never controls. The tooltip exists only while it is
// open. Open, DefaultOpen and OnOpenChange are the usual controlled and
// uncontrolled pair.
type Tooltip struct {
	// Trigger is the element the tooltip is about.
	Trigger Trigger
	// Content is what the tooltip says: text, or a short component.
	Content tela.Component
	// Placement is the side of the trigger the tooltip is on: "top" (the
	// default), "bottom", "left" or "right"; a Floating UI placement such as
	// "top-start" also works. Align is where it sits along that side:
	// "start", "center" (the default) or "end". The tooltip flips to the
	// other side when there is no room.
	Placement, Align string
	// Offset is the distance in pixels between the trigger and the tooltip
	// (zero by default: the arrow touches the trigger).
	Offset float32
	// Delay is how long the pointer must rest on the trigger before the
	// tooltip opens. Zero opens it at once, as shadcn's provider does.
	// Keyboard focus never waits.
	Delay time.Duration
	// Disabled keeps the tooltip from opening.
	Disabled bool

	// Open points to the open state of a controlled tooltip; nil leaves the
	// state to the tooltip, which starts closed unless DefaultOpen is set.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)

	// Class is added after the recipe's Root classes, to the tooltip;
	// ArrowClass does the same for the arrow.
	Class      string
	ArrowClass string
	// Props are more props for the tooltip element.
	Props tela.Child
}

// Build returns the trigger and the tooltip's portal.
func (t Tooltip) Build() tela.Node {
	s := &UseStyle().Tooltip
	delay := t.Delay
	if delay <= 0 {
		delay = aria.NoDelay
	}
	tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{
		Delay: delay, Disabled: t.Disabled,
		Open: t.Open, DefaultOpen: t.DefaultOpen, OnOpenChange: t.OnOpenChange,
	})
	arrow := tela.UseElementRef()
	fl := aria.UseFloating(tip.Ref, aria.Placement(placementOf("Tooltip", t.Placement, t.Align, "top")),
		aria.Offset(t.Offset), aria.Flip(), aria.Shift(8), aria.Arrow(arrow))
	return h.Fragment(
		triggerOf(t.Trigger, tip.TriggerProps),
		h.Portal(h.Presence(h.If(tip.Open,
			h.Div(tip.TooltipProps, fl.Props, h.Exit(s.Exit),
				h.Class(s.Root, s.Sides[fl.Side], t.Class), t.Props,
				t.Content,
				h.Div(h.Ref(arrow), fl.ArrowProps, h.Class(s.Arrow, s.ArrowSides[fl.Side], t.ArrowClass)))))),
	)
}
