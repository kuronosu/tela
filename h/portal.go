package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Portal shows its children in a layer above everything else in the
// window, like React's createPortal into document.body: use it for
// modals, drawers, menus, tooltips and toasts that must not be clipped or
// covered by the boxes around the component that renders them.
//
// The layer covers the window and is laid out against it, so fixed and
// absolute content inside it positions against the window
// (Div(Class("fixed inset-0 ..."))). It is a stacking context of its own
// above the window's content, whatever its z-index; portals mounted
// later are above earlier ones. The layer itself is never hit: clicks on
// its empty parts reach the content under it.
//
// Everything else follows the component tree, as in React: the children
// keep their component state, read the context of the Portal's ancestors,
// inherit their text style (color, font) from the Portal's parent element,
// and their events bubble to the Portal's ancestors (an OnClick on a Div
// around the Portal receives clicks from inside it). Tab order and the
// aria snapshot place the portal's content after the window's content,
// like content appended to the body.
//
// Portal takes children and a Key; other props panic (style its content).
//
//	func Confirm(onClose func()) tela.Node {
//	    return Portal(
//	        Div(Class("fixed inset-0 flex items-center justify-center bg-black/40"),
//	            Div(Role("dialog"), AriaLabel("Confirm"), Class("rounded-xl bg-white p-6 shadow-xl"),
//	                Text("Delete this file?"),
//	                Button(OnClick(func() { onClose() }), Text("Cancel")))))
//	}
func Portal(children ...tela.Child) tela.Node { return core.Portal(children...) }
