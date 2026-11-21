package devtools

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// The width of the details pane beside the trees: what the splitter
// between them may set, and where it starts (Chrome DevTools' sidebar is
// about as wide).
const (
	detailsMin     = 280
	detailsMax     = 960
	detailsDefault = 440
	// treeMin is the narrowest the tree beside the pane gets.
	treeMin = 300
)

// clampDetails keeps a width of the details pane in its bounds, and the
// tree beside it at treeMin within a window of width win (0: unknown).
func clampDetails(w, win float32) float32 {
	hi := float32(detailsMax)
	if win > 0 {
		hi = min(hi, win-treeMin)
	}
	return max(min(w, hi), detailsMin)
}

// DetailsSplit is a panel's tree (Main) and the details pane (Pane) with a
// splitter between them. Width is the pane's width as the person set it,
// kept within the bounds of the room the panel has; OnWidth sets it.
type DetailsSplit struct {
	Main, Pane tela.Child
	Width      float32
	OnWidth    func(float32)
}

// Build returns the panel.
func (d DetailsSplit) Build() tela.Node {
	box := tela.UseElementRef()
	room := tela.UseSize(box).Get().Width // read here: a resize builds again
	w := clampDetails(d.Width, room)
	return Div(Ref(box), Class("flex min-h-0 flex-1"),
		Div(Class("flex min-w-0 flex-1 flex-col"), d.Main),
		Splitter{Width: w, Room: room, OnWidth: d.OnWidth},
		Div(Class("flex shrink-0 flex-col bg-card"), Style(tela.Width(w)), d.Pane),
	)
}

// Splitter is the handle between a tree and the details pane, the WAI-ARIA
// window splitter: dragging it sets the pane's width, as do the arrow keys
// while it has the focus (Home and End go to the bounds), and a double
// click puts the default back. Width is the pane's width, Room the
// panel's (0: unknown), OnWidth sets the width.
type Splitter struct {
	Width, Room float32
	OnWidth     func(float32)
}

// Build returns the handle.
func (s Splitter) Build() tela.Node {
	ref := tela.UseElementRef()
	// The drag: the pointer that holds it, and where it started.
	drag := tela.UseRef(struct {
		on      bool
		pointer int
		x, w    float32
	}{})
	win := s.Room
	set := func(w float32) {
		if w = clampDetails(w, win); w != s.Width {
			s.OnWidth(w)
		}
	}
	end := func() {
		if drag.Current.on {
			drag.Current.on = false
			core.ReleasePointerCapture(ref, drag.Current.pointer)
		}
	}
	return Div(Ref(ref), Role("separator"), AriaOrientation("vertical"), AriaLabel("Details width"),
		AriaValueNow(float64(s.Width)), AriaValueMin(detailsMin), AriaValueMax(float64(clampDetails(detailsMax, win))), TabIndex(0),
		Class("group relative z-10 -mx-1 w-2 shrink-0 cursor-col-resize outline-none"),
		OnPointerDown(func(e tela.PointerEvent) {
			if e.Button != tela.MouseButtonLeft {
				return
			}
			drag.Current.on, drag.Current.pointer, drag.Current.x, drag.Current.w = true, e.PointerID, e.X, s.Width
			core.SetPointerCapture(ref, e.PointerID)
			e.PreventDefault()
		}),
		OnPointerMove(func(e tela.PointerEvent) {
			if d := drag.Current; d.on && e.PointerID == d.pointer {
				set(d.w + d.x - e.X) // the pane is on the right: leftwards widens it
			}
		}),
		OnPointerUp(end), OnPointerCancel(end),
		OnDoubleClick(func() { set(detailsDefault) }),
		OnKeyDown(func(e tela.KeyboardEvent) {
			step := float32(16)
			if e.Shift {
				step = 64
			}
			switch e.Key {
			case "ArrowLeft":
				set(s.Width + step)
			case "ArrowRight":
				set(s.Width - step)
			case "Home":
				set(detailsMin)
			case "End":
				set(detailsMax)
			default:
				return
			}
			e.PreventDefault()
		}),
		// A hairline that thickens in the accent color on hover, focus and
		// while dragged; the handle is wider than it, for the pointer.
		Div(Class("pointer-events-none absolute inset-y-0 left-1/2 w-px -translate-x-1/2 bg-border transition-[width,background-color]",
			"group-hover:w-[3px] group-hover:bg-primary/60 group-focus-visible:w-[3px] group-focus-visible:bg-primary",
			ifs(drag.Current.on, "w-[3px] bg-primary", ""))),
	)
}
