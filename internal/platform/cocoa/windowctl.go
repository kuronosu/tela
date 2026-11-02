package cocoa

import (
	"image"
	"math"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of Cocoa window control: the rules of caption presses, the
// title bar's double-click action, the content size limits, positions and
// the backdrop materials. It has no build constraint, so it is tested on
// every OS; windowctl_darwin.go reads AppKit and applies them.

// --- positions ---

// cocoaRect is an NSRect in AppKit's screen coordinates: points, y up
// from the bottom-left corner of the primary screen.
type cocoaRect struct{ X, Y, W, H float64 }

// Positions (WindowState.Position, SetPosition, WindowOptions.Position)
// are points on Cocoa, y down from the top-left corner of the primary
// screen: the CoreGraphics global display space (CGDisplayBounds,
// CGEvent locations). AppKit has no desktop space in pixels (every screen
// has its own backing scale, and windows move between them in points), so
// points are the one space where a position saved on one screen puts the
// window back in the same place on any other. They are rounded to whole
// points; a rounded position given back puts the content there exactly, so
// positions round-trip.

// desktopPosition returns the position of a content area whose screen
// rectangle is r, given the y of the primary screen's top edge in AppKit's
// coordinates (its height: the primary screen's origin is 0,0).
func desktopPosition(r cocoaRect, primaryTop float64) image.Point {
	return image.Pt(roundPoint(r.X), roundPoint(primaryTop-(r.Y+r.H)))
}

// frameTopLeft returns the AppKit point (setFrameTopLeftPoint:) that puts
// the top-left of the content at pos, given the window's current frame and
// content rectangles in screen coordinates (the offset between the two is
// the title bar, or nothing for a full-size content view).
func frameTopLeft(pos image.Point, frame, content cocoaRect, primaryTop float64) (x, y float64) {
	dx := content.X - frame.X
	dTop := (frame.Y + frame.H) - (content.Y + content.H)
	return float64(pos.X) - dx, primaryTop - float64(pos.Y) + dTop
}

// roundPoint rounds half away from zero, clamped to the int range.
func roundPoint(v float64) int {
	r := math.Round(v)
	switch {
	case r != r:
		return 0
	case r > math.MaxInt32:
		return math.MaxInt32
	case r < math.MinInt32:
		return math.MinInt32
	}
	return int(r)
}

// --- size limits ---

// noMaxSize is AppKit's "no maximum" for a size limit (FLT_MAX).
const noMaxSize = math.MaxFloat32

// sizeLimits returns the content size limits AppKit gets for the options'
// MinSize and MaxSize: the minimum, and the maximum raised to the minimum
// per axis (the minimum wins, as in platform.ClampSize), FLT_MAX where
// there is no maximum.
func sizeLimits(min, max platform.Size) (lo, hi [2]float64) {
	lo = [2]float64{float64(min.W), float64(min.H)}
	for i, v := range [2]float32{max.W, max.H} {
		switch {
		case v <= 0:
			hi[i] = noMaxSize
		default:
			hi[i] = math.Max(float64(v), lo[i])
		}
	}
	return lo, hi
}

// --- custom title bars ---

// captionAction is what a press does in a window with hit regions.
type captionAction uint8

const (
	// captionContent: the press is the app's (content, a hole, a button
	// region); it is delivered.
	captionContent captionAction = iota
	// captionDrag: a primary press in a caption region starts a window
	// drag (performWindowDragWithEvent:).
	captionDrag
	// captionDoubleClick: a primary press in a caption region that
	// completes a double click performs the title bar's double-click
	// action.
	captionDoubleClick
	// captionSwallow: another button (or a Control-click, which macOS
	// makes a context click) in a caption region: macOS has no window
	// menu, so nothing happens and nothing is delivered.
	captionSwallow
)

// captionInput is a window's state of caption presses: which buttons went
// down in a caption region (their sequence is the OS's: drags and the
// release are not delivered), and the double-click detector of primary
// presses there (platform.CaptionClicks, the content's rule, ADR-192).
type captionInput struct {
	held   platform.Buttons
	clicks platform.CaptionClicks
}

// press decides the press of btn at pos (logical, view coordinates) at
// time t. contentHeld are the buttons the app is told are held: a press
// while a content sequence runs belongs to that sequence.
func (c *captionInput) press(btn platform.PointerButton, pos platform.Point, mods platform.Modifiers, t time.Duration,
	regions []platform.HitRegion, contentHeld platform.Buttons, s platform.SystemSettings) captionAction {
	// A new press of a button ends its earlier sequence, whose release
	// the window server may have kept (a window drag).
	c.held &^= btn.Mask()
	if contentHeld != 0 || platform.HitTest(regions, pos) != platform.HitCaption {
		if btn == platform.ButtonPrimary {
			c.clicks.Reset() // a content click between two caption clicks
		}
		return captionContent
	}
	c.held |= btn.Mask()
	if btn != platform.ButtonPrimary || mods&platform.ModCtrl != 0 {
		if btn == platform.ButtonPrimary {
			c.clicks.Reset()
		}
		return captionSwallow
	}
	if c.clicks.Press(t, pos, s) {
		return captionDoubleClick
	}
	return captionDrag
}

// release reports whether the release of btn belongs to a caption press
// (and is not delivered).
func (c *captionInput) release(btn platform.PointerButton) bool {
	if !c.held.Has(btn) {
		return false
	}
	c.held &^= btn.Mask()
	return true
}

// drag reports whether a drag event (a move with buttons held) belongs to
// a caption press and is not delivered.
func (c *captionInput) drag(contentHeld platform.Buttons) bool {
	return c.held != 0 && contentHeld == 0
}

// moved records a move without buttons (mouseMoved:): every caption
// sequence has ended, whether or not its release reached the window.
func (c *captionInput) moved() { c.held = 0 }

// titleBarAction is what a double click on a title bar does.
type titleBarAction uint8

const (
	titleBarZoom titleBarAction = iota
	titleBarMinimize
	titleBarNone
)

func (a titleBarAction) String() string {
	switch a {
	case titleBarZoom:
		return "zoom"
	case titleBarMinimize:
		return "minimize"
	}
	return "none"
}

// doubleClickAction returns the title bar double-click action of the
// user's settings: the global default AppleActionOnDoubleClick (System
// Settings, Desktop & Dock, "Double-click a window's title bar to"), as
// Chromium and Firefox read it, with the older boolean
// AppleMiniaturizeOnDoubleClick when it is unset.
//
//   - "Maximize" is Zoom, the default before macOS 15. "Fill" (macOS 15
//     and later, the default there) fills the screen; a Tela window has no
//     standard frame of its own (windowWillUseStandardFrame:), so zoom:
//     fills the screen's visible frame and toggles back the same way.
//     Unknown values zoom too, the historical default.
//   - "Minimize" minimizes; "None" does nothing.
func doubleClickAction(action string, hasAction, miniaturize bool) titleBarAction {
	if !hasAction {
		if miniaturize {
			return titleBarMinimize
		}
		return titleBarZoom
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "minimize":
		return titleBarMinimize
	case "none":
		return titleBarNone
	}
	return titleBarZoom
}

// --- backdrops ---

// NSVisualEffectMaterial values of the backdrops, repeated here because
// internal/darwin exists only on darwin.
const (
	materialSidebar               = 7
	materialWindowBackground      = 12
	materialUnderWindowBackground = 21
)

// backdropMaterial returns the NSVisualEffectView material of a backdrop
// (platform.Backdrop documents the mapping), or false for the backdrops
// that keep the window opaque (BackdropDefault and BackdropNone).
func backdropMaterial(b platform.Backdrop) (int, bool) {
	switch b {
	case platform.BackdropMica:
		return materialWindowBackground, true
	case platform.BackdropMicaAlt:
		return materialUnderWindowBackground, true
	case platform.BackdropAcrylic:
		return materialSidebar, true
	}
	return 0, false
}
