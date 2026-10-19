package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// AnimateLayout makes the element animate every change of its laid-out
// box (Motion's layout prop): when a reorder, an expand or collapse, a
// sibling inserted or removed, or a breakpoint moves or resizes it, it is
// laid out at its new place at once but drawn moving there from where it
// was, through a transform (FLIP). Layout is not run again during the
// animation, so its frames only composite.
//
//	Map(items, func(it Item) tela.Node {
//	    return Div(Key(it.ID), AnimateLayout(), Class("rounded-lg bg-white p-3 shadow-sm"),
//	        Text(it.Title))
//	})
//
// The motion is a tween with the element's duration-*, ease-* (and
// delay-*) classes when it has one of the first two, like a transition;
// otherwise a spring (stiffness 500, damping 25: Motion's spring for x and
// y). A change during the animation starts from where the element is
// drawn, keeping its speed.
//
// A size change is drawn as a scale toward the new size, as in Motion: the
// element's rounded corners and box shadows are corrected so they keep
// their look, but its text and children stretch for the duration of the
// animation, unless they have AnimateLayout too: a child with it keeps its
// own size and its place in the parent (put it on a card's content to keep
// the text crisp while the card grows). A clip-path and background images
// scale with the element, as under a CSS transform.
//
// Scrolling, transforms (hover:scale-105) and the display scale do not
// move an element's layout, so they never start an animation. While it
// animates, the element is hit by the pointer, and ElementRef.Bounds and
// tela.UseBounds report its box, where it is drawn (like
// getBoundingClientRect under a CSS transform); scrolling it into view
// uses its final place. With reduced motion, and without a frame clock
// (tela snapshot), it is drawn at its new place at once. telatest waits
// for layout animations like for other animations.
func AnimateLayout() tela.Prop { return core.AnimateLayout() }

// LayoutID gives the element a shared layout identity (Motion's layoutId):
// when an element with a LayoutID mounts in the same commit in which
// another element with the same ID unmounts, or while one is exiting under
// a [Presence], the new element starts from the old one's box and
// animates to its own, as AnimateLayout does (a shared element
// transition). It is how a tab underline slides between tabs and a
// thumbnail grows into a detail view:
//
//	Map(tabs, func(t Tab) tela.Node {
//	    return Button(Key(t.ID), Class("relative px-3 py-2"), OnClick(...), Text(t.Label),
//	        If(t.ID == active, Div(LayoutID("underline"),
//	            Class("absolute inset-x-0 -bottom-px h-0.5 bg-indigo-500"))))
//	})
//
// An element with a LayoutID also animates its own layout changes, like
// AnimateLayout. IDs are global to the window: use distinct ones for
// unrelated transitions. The element that unmounts or exits is not moved:
// give an exiting one Exit classes (opacity-0) to fade it out while the
// new one grows from it.
func LayoutID(id string) tela.Prop { return core.LayoutID(id) }
