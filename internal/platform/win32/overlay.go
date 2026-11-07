package win32

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/platform"
)

// TitleBarOverlay windows (ADR-236): the pure half. The window calls are in
// overlay_windows.go, and "Title bar overlay" in the package documentation
// describes the whole.
//
// Such a window is a decorated window whose caption is client area: its
// WM_NCCALCSIZE keeps the side and bottom frame of WS_OVERLAPPEDWINDOW and
// gives the caption to the client (Windows Terminal's NonClientIslandWindow
// does the same), the frame is extended into the client area at the top
// (DwmExtendFrameIntoClientArea) so DWM keeps drawing its caption buttons
// over the app's pixels, and DwmDefWindowProc answers the hit test, hover
// and presses of those buttons (the "Custom Window Frame Using DWM"
// sample). Inside the package it is a Frameless window (opts.Frameless is
// true, as OverlayAsFrameless makes it) with Window.overlay set: the app
// draws its title bar and declares it with hit regions, it has no menu bar,
// and the paths that keep DefWindowProc from painting a classic caption over
// the client area apply to it as well.

// buildOverlay is the first Windows build on which a TitleBarOverlay window
// keeps DWM's caption buttons: Windows 11 (21H2), where they are checked
// (live_overlay_test.go). Windows 10 has the same DWM calls, but how its
// buttons draw over an extended frame with the app's opaque pixels and a
// dark title bar is not verified, so there the window is Frameless and the
// app draws its own buttons, as before ADR-236.
const buildOverlay = 22000

// overlaySupported reports whether a TitleBarOverlay window can keep DWM's
// caption buttons: Windows build `build` is at least buildOverlay, DWM
// composes (without composition there is no extended frame and so no
// buttons), and dwmapi has DwmDefWindowProc and DwmGetWindowAttribute
// (dwm). Otherwise the window is Frameless.
func overlaySupported(build uint32, composing, dwm bool) bool {
	return build >= buildOverlay && composing && dwm
}

// overlayClient returns the client rectangle of a TitleBarOverlay window
// for WM_NCCALCSIZE: def, the client rectangle DefWindowProc computed from
// the proposed window rectangle (inside its side and bottom resize frame,
// below its caption), extended up to the top of the proposed rectangle, so
// the caption is client area. A maximized window (zoomed) is placed by
// Windows with its frame outside the monitor's work area, on every side:
// its client area is clipped to the work area (Windows Terminal adds the
// frame height to the top instead), so no content is lost above the monitor
// and the sides stay where DefWindowProc put them.
func overlayClient(proposed, def image.Rectangle, zoomed bool, work image.Rectangle) image.Rectangle {
	r := def
	r.Min.Y = proposed.Min.Y
	if zoomed {
		r = maximizedClient(r, work)
	}
	return r
}

// overlayOuter returns the outer rectangle of a TitleBarOverlay window for
// the client rectangle client, given adjusted, the outer rectangle of a
// decorated window with that client (AdjustWindowRectExForDpi): the same
// side and bottom frame, and no caption above the client.
func overlayOuter(client, adjusted image.Rectangle) image.Rectangle {
	adjusted.Min.Y = client.Min.Y
	return adjusted
}

// controlsRect converts DWMWA_CAPTION_BUTTON_BOUNDS into
// WindowState.TitleBarControls. bounds is relative to the window rectangle's
// top-left (GetWindowRect, which includes the invisible resize frame), off
// is the client origin minus the window origin (screen pixels), client the
// client size and scale the window's display scale. It returns the
// rectangle in logical client coordinates and in physical client pixels
// (for the hit test), both clipped to the client area; both are empty when
// nothing of the buttons is inside it.
func controlsRect(bounds image.Rectangle, off, client image.Point, scale float64) (platform.Rect, image.Rectangle) {
	px := bounds.Sub(off).Intersect(image.Rectangle{Max: client})
	if px.Empty() || !(scale > 0) {
		return platform.Rect{}, image.Rectangle{}
	}
	return platform.Rect{
		X: float32(float64(px.Min.X) / scale),
		Y: float32(float64(px.Min.Y) / scale),
		W: float32(float64(px.Dx()) / scale),
		H: float32(float64(px.Dy()) / scale),
	}, px
}

// captionDark reports whether DWM draws the caption buttons of a
// TitleBarOverlay window for a dark title bar (light glyphs:
// DWMWA_USE_IMMERSIVE_DARK_MODE). The buttons sit over the app's bar, not
// over a caption of the system's color, so the app's title bar background
// decides when it set one (the glyph color with the better contrast,
// WCAG's relative luminance), and the color scheme otherwise.
func captionDark(colors platform.TitleBarColors, schemeDark bool) bool {
	bg := colors.Background
	if !bg.Set {
		return schemeDark
	}
	// The contrast with white glyphs, (1.05)/(L+0.05), beats the contrast
	// with black ones, (L+0.05)/(0.05), below L = sqrt(1.05*0.05)-0.05.
	return relativeLuminance(bg) < math.Sqrt(1.05*0.05)-0.05
}

// relativeLuminance is WCAG 2's relative luminance of an sRGB color.
func relativeLuminance(c platform.WindowColor) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}
