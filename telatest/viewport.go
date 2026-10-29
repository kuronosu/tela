package telatest

import (
	"bytes"
	"encoding/json"
	"math"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Resize changes the logical size of the window, in pixels, as a user
// resizing it: the app keeps its state, the breakpoint variants (md:,
// max-lg:) and the layout follow the new size, and the app is settled
// afterwards (with [NoSettle], only the work due is handled). The display
// scale is kept; [App.SetScale] changes it. It fails the test for a size
// that is not positive. tela mcp's resize tool is built on it (ADR-174).
//
//	app.Resize(390, 844) // a phone-sized window
//	app.ExpectCount(telatest.ByRole("button", "Open menu"), 1)
func (a *App) Resize(width, height float32) {
	a.tb.Helper()
	a.mustBeOpen()
	if !(width > 0) || !(height > 0) || math.IsInf(float64(width), 0) || math.IsInf(float64(height), 0) {
		a.fatalf("Resize(%g, %g): give a positive width and height in logical pixels, such as Resize(800, 600)", width, height)
	}
	a.win.Resize(platform.Size{W: width, H: height})
	a.settleAfter()
}

// SetScale moves the window to a display with another scale (device
// pixels per logical pixel), as a user dragging it between monitors: the
// logical size and the state are kept and screenshots become
// width*scale x height*scale pixels. The app is settled afterwards (with
// [NoSettle], only the work due is handled). It fails the test for a
// scale that is not positive. [App.GoldenScales] compares goldens at
// several scales without keeping the change.
//
//	app.SetScale(2)
func (a *App) SetScale(scale float64) {
	a.tb.Helper()
	a.mustBeOpen()
	if !(scale > 0) || math.IsInf(scale, 0) {
		a.fatalf("SetScale(%g): the display scale must be positive, such as 1, 1.5 or 2", scale)
	}
	a.win.SetScale(scale)
	a.settleAfter()
}

// LayoutJSON returns the layout tree of the running app as indented JSON,
// ending with a newline, after settling it (under [NoSettle], handling only
// the work due): the output of `tela inspect --layout` and of the
// function [LayoutJSON], for the app as it is now, with the state its
// actions built up.
func (a *App) LayoutJSON() []byte {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	return a.layoutJSON("LayoutJSON", a.tree.Inspect())
}

// LayoutJSONOf returns the layout subtree of the element l locates, in the
// same JSON as [App.LayoutJSON] (indented, ending with a newline, rects in
// window coordinates before scroll offsets and transforms, with the
// painted "bounds" where they differ): the box of the element with
// everything inside it, as `tela inspect --layout` shows it at that place.
// An inline element, a text or a display: contents element gives the box
// of its nearest ancestor that has one. Like the other locator queries it
// settles the app first (under [NoSettle], handling only the work due), and
// fails the test when l matches no element or several. tela mcp's inspect
// tool is built on it (ADR-194).
//
//	js := app.LayoutJSONOf(telatest.ByRole("button", "Save"))
func (a *App) LayoutJSONOf(l Locator) []byte {
	a.tb.Helper()
	n := a.oneQuery(l)
	box, err := a.tree.InspectElement(n.ID)
	if err != nil {
		a.fatalf("LayoutJSONOf: %v", err)
	}
	return a.layoutJSON("LayoutJSONOf", box)
}

// layoutJSON encodes a layout node the way LayoutJSON does.
func (a *App) layoutJSON(what string, n core.InspectNode) []byte {
	a.tb.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(n); err != nil {
		a.fatalf("%s: %v", what, err)
	}
	return buf.Bytes()
}
