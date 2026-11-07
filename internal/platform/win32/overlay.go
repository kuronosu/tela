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
// (DwmExtendFrameIntoClientArea), and DwmDefWindowProc answers the hit
// test, hover and presses of DWM's caption buttons (the "Custom Window
// Frame Using DWM" sample). DWM draws those buttons in the extended frame,
// under the client pixels, over a fill of DWMWA_CAPTION_COLOR: they show
// only where the presented pixels are transparent (seen on Windows 11
// 26200). So the window presents a transparent hole over the buttons'
// rectangle (blitParts) and sets the caption color to the color of the
// app's bar (captionBackground), which makes the buttons sit on the bar.
// Maximized, the client area keeps its top at the window's top, above the
// monitor (overlayClient), and the app's content starts below that hidden
// band (overlayInset). Inside the package it is a Frameless window
// (opts.Frameless is true, as OverlayAsFrameless makes it) with
// Window.overlay set: the app draws its title bar and declares it with hit
// regions, it has no menu bar, and the paths that keep DefWindowProc from
// painting a classic caption over the client area apply to it as well.

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
// its sides and bottom are clipped to the work area, but its top stays at
// the window's top, above the work area by the frame's thickness (Windows
// Terminal does the same). DWM only draws and answers its caption buttons
// when the client area starts at the window's top (seen on Windows 11
// 26200: with the top clipped to the work area DwmDefWindowProc answered
// nothing and drew nothing), so that hidden band is kept and the backend
// presents the app below it (overlayInset).
func overlayClient(proposed, def image.Rectangle, zoomed bool, work image.Rectangle) image.Rectangle {
	r := def
	r.Min.Y = proposed.Min.Y
	if zoomed {
		r = maximizedClient(r, work)
		r.Min.Y = proposed.Min.Y
	}
	return r
}

// overlayInset returns the hidden band of a maximized TitleBarOverlay
// window: the rows of its client area (whose top is clientTop, screen
// pixels, and height height) above its monitor's work area, which
// overlayClient keeps for DWM. The app does not see them: its content
// starts below the band, its size is the client size without it, and every
// position is translated by it (Window.top). Zero when the client area is
// inside the work area (not maximized) or the work area is unknown; never
// the whole client area.
func overlayInset(clientTop, height int, work image.Rectangle) int {
	if work.Empty() || height <= 1 {
		return 0
	}
	return min(max(work.Min.Y-clientTop, 0), height-1)
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
// is the origin of the app's area (the client origin, below the hidden band
// of a maximized window) minus the window origin (screen pixels), client
// the app's size and scale the window's display scale. It returns the
// rectangle in the app's logical coordinates and physical pixels (for the
// hit test), both clipped to the app's area; both are empty when nothing of
// the buttons is inside it.
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
// over a caption of the system's color, so the color behind them decides
// (captionBackground: the app's title bar background, else the bar's color
// sampled from its pixels) with the glyph color of the better contrast
// (WCAG's relative luminance); the color scheme decides when there is none.
func captionDark(bg platform.WindowColor, schemeDark bool) bool {
	if !bg.Set {
		return schemeDark
	}
	// The contrast with white glyphs, (1.05)/(L+0.05), beats the contrast
	// with black ones, (L+0.05)/(0.05), below L = sqrt(1.05*0.05)-0.05.
	return relativeLuminance(bg) < math.Sqrt(1.05*0.05)-0.05
}

// captionGap is how far left of DWM's buttons, in logical pixels, the color
// of the app's title bar is sampled (captionSample).
const captionGap = 4

// captionSample returns the color of the app's title bar next to DWM's
// buttons, from the retained pixels pix (B, G, R, X rows of stride bytes,
// size px, the app's coordinates): the pixel captionGap logical pixels left
// of the buttons' rectangle ctl (physical pixels), in its middle row. DWM
// paints DWMWA_CAPTION_COLOR behind its buttons, where the presented pixels
// are transparent; with the color the bar has right beside them, the
// buttons sit on the bar seamlessly. ok is false when the point is outside
// the pixels (no buttons, a bar narrower than the gap).
func captionSample(pix []byte, stride int, size image.Point, ctl image.Rectangle, scale float64) (c platform.WindowColor, ok bool) {
	if ctl.Empty() || !(scale > 0) {
		return platform.WindowColor{}, false
	}
	x := ctl.Min.X - max(1, int(captionGap*scale+0.5))
	y := ctl.Min.Y + ctl.Dy()/2
	if x < 0 || y < 0 || x >= size.X || y >= size.Y {
		return platform.WindowColor{}, false
	}
	i := y*stride + 4*x
	if i+3 >= len(pix) {
		return platform.WindowColor{}, false
	}
	return platform.WindowColor{R: pix[i+2], G: pix[i+1], B: pix[i], Set: true}, true
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
