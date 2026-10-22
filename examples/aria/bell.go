package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/examples/aria/icons"
	. "github.com/kuronosu/tela/h"
)

// Bell is an icon button with a tooltip: UseTooltipTrigger opens it when the
// pointer rests on the button (after Delay) or the keyboard focuses it (at
// once), closes it with Escape, and describes the button with it
// (aria-describedby). UseFloating places it above the button with an arrow
// that points at it.
type Bell struct {
	// Unread is the number the tooltip announces.
	Unread int
	// Open starts the tooltip open: a preview's seed.
	Open bool
}

func (b Bell) Build() tela.Node {
	tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{
		DefaultOpen: b.Open, Delay: 500 * time.Millisecond,
	})
	arrow := tela.UseElementRef()
	fl := aria.UseFloating(tip.Ref, aria.Placement("bottom"), aria.Offset(8),
		aria.Flip(), aria.Shift(8), aria.Arrow(arrow))

	return Fragment(
		Button(tip.TriggerProps, AriaLabel("Notifications"),
			Class(button, iconBtn, focusRing, "relative"),
			ic(icons.Bell, "size-4"),
			If(b.Unread > 0, Span(Class("absolute top-2 right-2.5 size-2 rounded-full bg-destructive ring-2 ring-background")))),
		Portal(Presence(If(tip.Open,
			Div(tip.TooltipProps, fl.Props, Exit("scale-95 opacity-0 duration-100"),
				Class("z-50 rounded-md bg-foreground px-3 py-1.5 text-xs text-background shadow-md",
					"transition duration-150 starting:scale-95 starting:opacity-0"),
				Textf("%d unread notifications", b.Unread),
				Div(Ref(arrow), fl.ArrowProps, Class("size-2.5 rotate-45 rounded-[2px] bg-foreground")),
			)))),
	)
}
