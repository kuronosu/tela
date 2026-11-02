//go:build darwin

package cocoa

import (
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Title bar overlays (WindowOptions.TitleBarOverlay, ADR-224, ADR-225): a
// frameless window (a titled window with a full-size content view and a
// transparent title bar, windowctl_darwin.go) whose standard buttons stay
// shown over the content. The app draws the rest of its title bar and
// declares it with hit regions; WindowState.TitleBarControls reports the
// buttons' rectangle, which the app leaves free. The buttons sit in the
// theme frame above TelaView, so they take the presses inside it.
//
// Placement, as Electron's trafficLightPosition (window_buttons_proxy.mm):
// by default the buttons are where AppKit puts them. When the app has a
// HitCaption region along the top that spans them (overlayCaptionHeight),
// they are centered vertically in its height, keeping AppKit's horizontal
// inset: the title bar container view (NSTitlebarContainerView) and the
// views between it and the buttons are made as tall as the caption, with
// their top edges kept, and each button's frame is moved. AppKit lays the
// title bar out again (resizes, key changes, full screen), so the place is
// applied again at the end of every delivery pass and at every delegate
// notification, both of which run applyControl; it changes nothing when
// the frames are right already. Every move is done in TelaView's
// coordinates and converted to each view's superview (convertRect:), so
// it does not depend on which of AppKit's views are flipped, or on how
// many sit between the buttons and the container (Tahoe's title bar adds
// views of its own: the chain is in the test logs).
//
// In full screen the buttons live in AppKit's auto-hiding title bar, not
// over the content: AppKit's place is restored before the window enters
// full screen, TitleBarControls is empty there (and while the transition
// runs and settles), and the place is applied again once the window is
// back and settled.

// osel holds the selectors of the overlay.
var osel = struct {
	superview, className, isHidden, isFlipped, convertRectFromView       darwin.SEL
	setFrame, updateTrackingAreas, setNeedsLayout, layoutSubtreeIfNeeded darwin.SEL
}{
	superview:             darwin.Sel("superview"),
	className:             darwin.Sel("className"),
	isHidden:              darwin.Sel("isHidden"),
	isFlipped:             darwin.Sel("isFlipped"),
	convertRectFromView:   darwin.Sel("convertRect:fromView:"),
	setFrame:              darwin.Sel("setFrame:"),
	updateTrackingAreas:   darwin.Sel("updateTrackingAreas"),
	setNeedsLayout:        darwin.Sel("setNeedsLayout:"),
	layoutSubtreeIfNeeded: darwin.Sel("layoutSubtreeIfNeeded"),
}

// overlayState is the window's title bar overlay: AppKit's standard
// buttons and the views above them, and AppKit's own layout of them while
// the backend has moved them.
type overlayState struct {
	found   bool
	buttons [3]darwin.ID // close, miniaturize, zoom (AppKit's)
	// views are the ancestors of the buttons up to the title bar
	// container, top-down (the container first); 0 views when AppKit has
	// no container (the buttons are then moved alone).
	views []darwin.ID
	chain string // the class names from the buttons up, for logs

	moved     bool // the views and buttons are not where AppKit put them
	viewH     []float64
	buttonsAt [3]darwin.CGPoint // the buttons' frame origins in AppKit's layout
}

var overlayButtons = [3]uintptr{darwin.NSWindowCloseButton, darwin.NSWindowMiniaturizeButton, darwin.NSWindowZoomButton}

// findButtons looks up the standard buttons and the views above them,
// once; it reports whether the window has the three buttons.
func (w *Window) findButtons() bool {
	o := &w.ctl.overlay
	if o.found {
		return o.buttons[0] != 0
	}
	o.found = true
	for i, b := range overlayButtons {
		if o.buttons[i] = darwin.Send(w.ns, wsel.standardWindowButton, b); o.buttons[i] == 0 {
			o.buttons = [3]darwin.ID{}
			o.chain = fmt.Sprintf("no standard button %d", b)
			return false
		}
	}
	var chain []string
	var below []darwin.ID // bottom-up, below the container
	container := darwin.ID(0)
	for v := darwin.Send(o.buttons[0], osel.superview); v != 0; v = darwin.Send(v, osel.superview) {
		name := darwin.GoString(darwin.Send(v, osel.className))
		chain = append(chain, name)
		if container == 0 {
			if strings.Contains(name, "TitlebarContainer") {
				container = v
			} else {
				below = append(below, v)
			}
		}
	}
	o.chain = strings.Join(chain, " < ")
	if container != 0 {
		o.views = append(o.views, container)
		for i := len(below) - 1; i >= 0; i-- {
			o.views = append(o.views, below[i])
		}
	}
	return true
}

// inView returns the rectangle of v in TelaView's coordinates (points, y
// down).
func (w *Window) inView(v darwin.ID) darwin.CGRect {
	return darwin.SendRect(w.view, osel.convertRectFromView, darwin.SendRect(v, sel.bounds), v)
}

// buttonsRect returns the union of the shown buttons' frames in TelaView's
// coordinates.
func (w *Window) buttonsRect() (platform.Rect, bool) {
	var u darwin.CGRect
	n := 0
	for _, b := range w.ctl.overlay.buttons {
		if b == 0 || darwin.SendBool(b, osel.isHidden) {
			continue
		}
		r := w.inView(b)
		if n == 0 {
			u = r
		} else {
			x0, y0 := math.Min(u.Origin.X, r.Origin.X), math.Min(u.Origin.Y, r.Origin.Y)
			x1 := math.Max(u.Origin.X+u.Size.Width, r.Origin.X+r.Size.Width)
			y1 := math.Max(u.Origin.Y+u.Size.Height, r.Origin.Y+r.Size.Height)
			u = darwin.CGRect{Origin: darwin.CGPoint{X: x0, Y: y0}, Size: darwin.CGSize{Width: x1 - x0, Height: y1 - y0}}
		}
		n++
	}
	if n == 0 {
		return platform.Rect{}, false
	}
	return platform.Rect{X: float32(u.Origin.X), Y: float32(u.Origin.Y), W: float32(u.Size.Width), H: float32(u.Size.Height)}, true
}

// titleBarControls is WindowState.TitleBarControls: the buttons'
// rectangle where placeButtons puts them (so a state read before AppKit's
// relayout is undone already says where they go), empty for other windows
// and in full screen.
func (w *Window) titleBarControls() platform.Rect {
	if !w.opts.TitleBarOverlay || w.ctl.fullscreen || w.fsBusy() || !w.findButtons() {
		return platform.Rect{}
	}
	if !w.shown {
		// Before the window first shows, AppKit may not have laid the
		// title bar out yet: the first state must have the real place.
		darwin.Send(darwin.Send(w.ctl.container, osel.superview), osel.layoutSubtreeIfNeeded)
	}
	r, ok := w.buttonsRect()
	if !ok {
		return platform.Rect{}
	}
	if h, ok := overlayCaptionHeight(w.ctl.regions, r); ok {
		r.Y = h/2 - r.H/2
	}
	return r
}

// placeButtons puts the buttons where titleBarControls says (see the top
// of the file) and reports whether it moved anything.
func (w *Window) placeButtons() bool {
	if !w.opts.TitleBarOverlay || w.Closed() || !w.shown || w.ctl.fullscreen || w.fsBusy() || !w.findButtons() {
		return false
	}
	o := &w.ctl.overlay
	r, ok := w.buttonsRect()
	if !ok {
		return false
	}
	h, ok := overlayCaptionHeight(w.ctl.regions, r)
	if !ok {
		return w.restoreButtons()
	}
	if !o.moved {
		// AppKit's layout, to go back to.
		o.moved = true
		o.viewH = o.viewH[:0]
		for _, v := range o.views {
			o.viewH = append(o.viewH, darwin.SendRect(v, wsel.frame).Size.Height)
		}
		for i, b := range o.buttons {
			o.buttonsAt[i] = darwin.SendRect(b, wsel.frame).Origin
		}
	}
	changed := false
	// The views from the container down cover the caption from the
	// window's top edge, so the buttons are inside them (drawn and hit).
	for i, v := range o.views {
		need := float64(h) // the caption's bottom edge, from the window's top
		if i == 0 {
			need = math.Max(need, o.viewH[0]) // the container keeps at least AppKit's height
		}
		vr := w.inView(v)
		if vr.Origin.Y+vr.Size.Height >= need-0.25 {
			continue
		}
		vr.Size.Height = need - vr.Origin.Y // the top edge stays
		darwin.Send(v, osel.setFrame, w.toSuperview(v, vr))
		changed = true
	}
	for _, b := range o.buttons {
		br := w.inView(b)
		y := float64(h)/2 - br.Size.Height/2
		if math.Abs(br.Origin.Y-y) < 0.25 {
			continue
		}
		br.Origin.Y = y
		darwin.Send(b, wsel.setFrameOrigin, w.toSuperview(b, br).Origin)
		changed = true
	}
	if changed {
		darwin.Send(darwin.Send(o.buttons[0], osel.superview), osel.updateTrackingAreas)
	}
	return changed
}

// toSuperview converts r from TelaView's coordinates to those of v's
// superview (where v's frame is).
func (w *Window) toSuperview(v darwin.ID, r darwin.CGRect) darwin.CGRect {
	return darwin.SendRect(darwin.Send(v, osel.superview), osel.convertRectFromView, r, w.view)
}

// restoreButtons puts the views and buttons back where AppKit had them
// (heights with their top edges kept, the buttons' origins) and has the
// title bar laid out again; it reports whether anything was moved.
func (w *Window) restoreButtons() bool {
	o := &w.ctl.overlay
	if !o.moved {
		return false
	}
	o.moved = false
	for i, v := range o.views {
		vr := w.inView(v)
		vr.Size.Height = o.viewH[i]
		darwin.Send(v, osel.setFrame, w.toSuperview(v, vr))
	}
	for i, b := range o.buttons {
		darwin.Send(b, wsel.setFrameOrigin, o.buttonsAt[i])
	}
	if len(o.views) > 0 {
		darwin.Send(o.views[0], osel.setNeedsLayout, true)
	}
	darwin.Send(darwin.Send(o.buttons[0], osel.superview), osel.updateTrackingAreas)
	return true
}

// overlayDescribe returns the overlay's views and frames, for logs.
func (w *Window) overlayDescribe() string {
	if !w.findButtons() {
		return "no buttons: " + w.ctl.overlay.chain
	}
	o := &w.ctl.overlay
	s := fmt.Sprintf("buttons in %s; moved %v", o.chain, o.moved)
	for i, v := range o.views {
		r := w.inView(v)
		s += fmt.Sprintf("; view %d %s at (%.1f,%.1f %.1fx%.1f), flipped %v", i, darwin.GoString(darwin.Send(v, osel.className)),
			r.Origin.X, r.Origin.Y, r.Size.Width, r.Size.Height, darwin.SendBool(v, osel.isFlipped))
	}
	for i, b := range o.buttons {
		r := w.inView(b)
		s += fmt.Sprintf("; button %d at (%.1f,%.1f %.1fx%.1f)", i, r.Origin.X, r.Origin.Y, r.Size.Width, r.Size.Height)
	}
	return s
}
