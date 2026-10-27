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
