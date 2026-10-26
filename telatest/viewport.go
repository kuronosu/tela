package telatest

import (
	"bytes"
	"encoding/json"
	"math"

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
	a.settleQuery()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(a.tree.Inspect()); err != nil {
		a.fatalf("LayoutJSON: %v", err)
	}
	return buf.Bytes()
}
