package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Presence keeps a removed child mounted until its exit animation ends,
// then unmounts it (Motion's AnimatePresence). Children are matched by key
// (or by position, which h.If keeps): when a build no longer has a child,
// the child exits if it has something to animate, its [Exit] classes or
// a component that uses tela.UsePresence; a child without either goes at
// once.
//
//	Presence(
//	    If(open, Div(Key("dialog"), Role("dialog"),
//	        Class("rounded-xl bg-white p-6 shadow-xl transition duration-200 ease-out "+
//	            "opacity-100 scale-100 starting:opacity-0 starting:scale-95"),
//	        Exit("opacity-0 scale-95"),
//	        Text("Saved"))),
//	)
//
// The entrance is the starting: variant (CSS @starting-style), which works
// on any element that starts being rendered, inside a Presence or not: its
// first style is computed with the starting: classes, and its transitions
// run from it to its normal style. The exit applies the Exit classes on
// top of the element's classes, so the same transitions run to them; the
// child unmounts when they end (at once with reduced motion or without a
// transition).
//
// An exiting child keeps its place in the layout (Motion's default, not
// popLayout): position it absolutely, or inside an overlay, when the
// content around it should not wait. It shows its last build: the
// parent's builds do not update it, but its own components still build.
// It is inert while it exits: the pointer goes through it to what is
// below, it cannot take the focus (focus inside it is lost and its focus
// scopes end, as on unmount), and it is gone from the semantics tree, so
// aria snapshots and telatest locators no longer see it. Adding the child
// back while it exits cancels the exit: the transitions run back from
// where they are, as in Motion.
//
// Several children can exit at once, each at its old position. A Presence
// has no box: its children are children of its parent. It takes only Key
// as a prop. A nested Presence's children do not exit with an outer
// child; they unmount with it.
func Presence(children ...tela.Child) tela.Node { return core.Presence(children...) }

// Exit sets the classes an element takes when the child of a [Presence]
// it is in is removed: they apply on top of its classes, and its
// transitions (transition, duration-*, ease-*, delay-*) run to them before
// the child unmounts. The usual exit is the reverse of the entrance:
//
//	Div(Class("transition duration-150 starting:opacity-0 starting:translate-y-2"),
//	    Exit("opacity-0 translate-y-2"), ...)
//
// Any element of the removed child may have Exit classes (a dialog's
// backdrop and panel can fade differently); the child unmounts when all
// their transitions have ended. Outside a Presence, Exit does nothing. It
// may repeat; the strings concatenate. Unknown classes are reported in dev
// mode like those of Class.
func Exit(classes string) tela.Prop { return core.Exit(classes) }
