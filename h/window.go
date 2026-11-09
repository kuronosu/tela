package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// WindowDrag makes an element a drag area of the window's title bar, for
// a frameless window that draws its own (tela.App.Frameless,
// tela.Window.Frameless), or any window: pressing it and moving moves the
// window, a double click maximizes or restores it and a right click opens
// the window menu, as in the OS's own title bar. Its interactive elements
// keep working: buttons, links, text fields, focusable elements and
// elements with press or drag handlers are left out of the drag area by
// themselves, and WindowNoDrag leaves out any other.
//
// A window whose content has a WindowDrag area draws its own title bar,
// so it gets no fallback title bar where the OS draws none (GNOME on
// Wayland; see tela.App). A click on the drag area that does not move the
// window is a press outside the open overlays (a dismissable dialog or
// popover closes), on every platform.
//
//	Div(Class("flex h-10 items-center gap-2 px-3"), WindowDrag(),
//	    Span(Class("grow text-sm font-medium"), Text("Notes")),
//	    Button(WindowControl(tela.WindowClose), AriaLabel("Close"), Text("×")),
//	)
func WindowDrag() tela.Prop { return core.WindowDrag() }

// WindowNoDrag leaves an element out of the WindowDrag area around it: a
// press there reaches the element instead of moving the window. Elements
// that take presses (buttons, fields, links) need no WindowNoDrag; use it
// for a plain area that should not drag, such as a tab strip with its own
// gestures. A WindowDrag inside it is a drag area again.
func WindowNoDrag() tela.Prop { return core.WindowNoDrag() }

// WindowControl makes an element a window-control button of a custom
// title bar: a click on it minimizes the window (tela.WindowMinimize),
// maximizes or restores it (tela.WindowMaximize) or asks to close it
// (tela.WindowClose: the window's OnClose runs, as with the OS's close
// button). The OS also learns what the button is (Windows 11 shows its
// snap layouts over the maximize button). Give it a label for assistive
// technologies (AriaLabel).
//
//	Button(WindowControl(tela.WindowMaximize), AriaLabel("Maximize"), Icon(lucide.Square, Class("size-4")))
func WindowControl(c tela.WindowControl) tela.Prop { return core.WindowControlProp(c) }

// WindowResize makes the inside of an element's edges the window's resize
// handles: pressing there and moving resizes the window from that side, or
// diagonally from a corner, and the pointer shows the resize cursors, as
// over an OS window's border. It is for windows whose OS draws no resize
// border: a transparent window (tela.App.Transparent), whose edges are the
// transparent margin around the card the app draws, and any frameless one.
// Put it on the card; the band lies on the card's own pixels (5 logical
// pixels wide unless tela.ResizeBand says otherwise, corners 16 along each
// side), on top of what is inside it, so presses there resize instead of
// reaching the content; the corners take the press over their whole
// square, outside a rounded card's curve too. Overlays (popovers, menus,
// dialogs: anything in an h.Portal) over the band take the press instead.
// tela.ResizeEdges keeps only some sides. The window does not resize while
// it is maximized or fullscreen, nor with FixedSize, and a resize stops
// before the handles would come closer than two corners (the margin around
// the card kept) unless MinWidth and MinHeight say otherwise.
//
//	Div(Class("absolute inset-4 rounded-2xl bg-white shadow-xl"), WindowResize(),
//	    Div(Class("h-10"), WindowDrag(), Text("Notes")),
//	    ...
//	)
func WindowResize(opts ...tela.ResizeOption) tela.Prop { return core.WindowResize(opts...) }
