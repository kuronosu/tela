package win32

import (
	"image"
	"image/draw"

	xdraw "golang.org/x/image/draw"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of window control (windowctl.go): hit-testing for custom
// title bars and frameless windows, the non-client mouse messages, placement
// math, the reported state and icon pixels. It has no build constraint, so it
// is tested on every OS.

// Hit-test codes of WM_NCHITTEST.
const (
	htClient      = 1
	htCaption     = 2
	htMinButton   = 8
	htMaxButton   = 9
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17
	htClose       = 20
)

// Non-client mouse messages. Their wParam is the hit-test code (for the X
// buttons its low word, the button in the high word) and their lParam the
// pointer in screen coordinates.
const (
	wmNCMouseMove     = 0x00A0
	wmNCLButtonDown   = 0x00A1
	wmNCLButtonUp     = 0x00A2
	wmNCLButtonDblClk = 0x00A3
	wmNCRButtonDown   = 0x00A4
	wmNCRButtonUp     = 0x00A5
	wmNCRButtonDblClk = 0x00A6
	wmNCMButtonDown   = 0x00A7
	wmNCMButtonUp     = 0x00A8
	wmNCMButtonDblClk = 0x00A9
	wmNCXButtonDown   = 0x00AB
	wmNCXButtonUp     = 0x00AC
	wmNCXButtonDblClk = 0x00AD
	wmNCMouseLeave    = 0x02A2
)

// hitCodeOf returns the WM_NCHITTEST answer for a hit region kind. In
// fullscreen a caption is content: the window cannot be moved or
// maximized from it (Chromium does the same).
func hitCodeOf(k platform.HitKind, fullscreen bool) uintptr {
	switch k {
	case platform.HitCaption:
		if fullscreen {
			return htClient
		}
		return htCaption
	case platform.HitMinimize:
		return htMinButton
	case platform.HitMaximize:
		return htMaxButton
	case platform.HitClose:
		return htClose
	}
	return htClient
}

// resizeHitCode returns the WM_NCHITTEST answer of a resize edge (a side
// or a corner): DefWindowProc then shows the resize cursor and starts its
// sizing loop at a press, as over a native frame.
func resizeHitCode(e platform.Edge) uintptr {
	switch e {
	case platform.EdgeTop:
		return htTop
	case platform.EdgeBottom:
		return htBottom
	case platform.EdgeLeft:
		return htLeft
	case platform.EdgeRight:
		return htRight
	case platform.EdgeTop | platform.EdgeLeft:
		return htTopLeft
	case platform.EdgeTop | platform.EdgeRight:
		return htTopRight
	case platform.EdgeBottom | platform.EdgeLeft:
		return htBottomLeft
	case platform.EdgeBottom | platform.EdgeRight:
		return htBottomRight
	}
	return htClient
}

// isAppHit reports whether a hit-test code answered for a point of the
// client area belongs to the app's own title bar: the pointer is still over
// content the app draws, so moves are delivered and crossing between it and
// the client area is no leave.
func isAppHit(code uintptr) bool {
	switch code {
	case htClient, htCaption, htMinButton, htMaxButton, htClose:
		return true
	}
	return false
}

// isAppButton reports whether a hit-test code is one of the app's window
// buttons, whose presses the backend delivers instead of DefWindowProc.
func isAppButton(code uintptr) bool {
	return code == htMinButton || code == htMaxButton || code == htClose
}

// hitInput is what clientHit needs to answer WM_NCHITTEST for a point of the
// client area.
type hitInput struct {
	pt     image.Point // the app's physical pixels (Window.screenToApp)
	client image.Point // the app's physical size
	scale  float64
	// border is the width of the resize band in physical pixels, or 0 when
	// the window has none (decorated, FixedSize, maximized, fullscreen).
	border     int
	fullscreen bool
	regions    []platform.HitRegion
	// topOnly: the band is only along the top, the other edges being the
	// OS's frame outside the client area (a TitleBarOverlay window).
	topOnly bool
	// controls is the rectangle of the OS's caption buttons in the app's
	// physical pixels (a TitleBarOverlay window), empty otherwise. A point
	// there that DWM did not claim as one of its buttons is caption.
	controls image.Rectangle
	// resizable: the window can be resized (not FixedSize, maximized or
	// fullscreen), so the app's resize handles (HitResize… regions,
	// h.WindowResize) answer the resize codes; otherwise they are content.
	resizable bool
}

// regionCode is the WM_NCHITTEST answer of the region kind k at a point.
func (in *hitInput) regionCode(k platform.HitKind) uintptr {
	if e := platform.KindEdge(k); e != 0 {
		if in.resizable {
			return resizeHitCode(e)
		}
		return htClient
	}
	return hitCodeOf(k, in.fullscreen)
}

// clientHit answers WM_NCHITTEST for a point inside the client area: the
// app's window buttons (except at the outermost pixel), the resize band of a
// frameless window (it takes precedence over the other hit regions), the
// rectangle of the OS's caption buttons of a TitleBarOverlay window (which
// replaces every region there), then the app's hit regions. Corners take
// twice the band along each edge, so they are easier to grab, as Windows'
// own frames do; along the top of a TitleBarOverlay window a corner takes
// the band's width, and the side band outside the client area adds the
// rest.
func clientHit(in hitInput) uintptr {
	if in.pt.Y < 0 {
		// The hidden band of a maximized TitleBarOverlay window, above
		// the app's area (overlayInset; reachable from a monitor above):
		// part of its title bar.
		return hitCodeOf(platform.HitCaption, in.fullscreen)
	}
	inControls := in.pt.In(in.controls)
	// A window button of the app takes the band, like the caption buttons
	// of a native frame, which leave only the outermost pixel of the window
	// to the resize edges: the hover over the maximize button of Windows 11
	// (the snap layouts flyout) starts from the hit test at the pointer, and
	// the pointer reaches a button across the band.
	if in.border > 0 && len(in.regions) > 0 && !inControls {
		thin := max(1, int(in.scale+0.5))
		x, y, w, h := in.pt.X, in.pt.Y, in.client.X, in.client.Y
		if x >= thin && y >= thin && x < w-thin && y < h-thin {
			code := hitCodeOf(platform.HitTest(in.regions, logicalPoint(x, y, in.scale)), in.fullscreen)
			if isAppButton(code) {
				return code
			}
		}
	}
	if b := in.border; b > 0 {
		x, y, w, h := in.pt.X, in.pt.Y, in.client.X, in.client.Y
		left, right := x < b, x >= w-b
		top, bottom := y < b, y >= h-b
		nearLeft, nearRight := x < 2*b, x >= w-2*b
		nearTop, nearBottom := y < 2*b, y >= h-2*b
		if in.topOnly {
			left, right, bottom = false, false, false
			nearLeft, nearRight = x < b, x >= w-b
		}
		switch {
		case (top && nearLeft) || (left && nearTop):
			return htTopLeft
		case (top && nearRight) || (right && nearTop):
			return htTopRight
		case (bottom && nearLeft) || (left && nearBottom):
			return htBottomLeft
		case (bottom && nearRight) || (right && nearBottom):
			return htBottomRight
		case top:
			return htTop
		case bottom:
			return htBottom
		case left:
			return htLeft
		case right:
			return htRight
		}
	}
	if inControls {
		// Around DWM's buttons (which it answers itself): the caption
		// they sit in.
		return hitCodeOf(platform.HitCaption, in.fullscreen)
	}
	if len(in.regions) == 0 {
		return htClient
	}
	p := logicalPoint(in.pt.X, in.pt.Y, in.scale)
	return in.regionCode(platform.HitTest(in.regions, p))
}

// captionWasClick reports whether a primary press in the app's caption
// that DefWindowProc's move loop took (WM_NCLBUTTONDOWN with HTCAPTION,
// SC_MOVE) was a click (ADR-252): the loop returns at the release, and a
// click neither moved the window (its rectangle before and after),
// changed its maximized state (a drag off a maximized window restores
// it; Aero Snap maximizes) nor took the cursor farther than slop
// physical pixels from the press (a drag there and back). Windows posts
// no WM_NCLBUTTONUP after the loop, so this is the only way to tell.
func captionWasClick(before, after image.Rectangle, zoomedBefore, zoomedAfter bool, press, release image.Point, slop int) bool {
	if before != after || zoomedBefore != zoomedAfter {
		return false
	}
	d := release.Sub(press)
	return d.X*d.X+d.Y*d.Y < slop*slop
}

// ncButtonFromMessage maps a non-client mouse button message to the client
// button it stands for, whether it is a press (a double click is a press:
// the app counts its own clicks), and the hit-test code it was sent for. ok
// is false for other messages and unknown X buttons.
func ncButtonFromMessage(msg uint32, wparam uintptr) (btn platform.PointerButton, down bool, hit uintptr, ok bool) {
	hit = wparam
	switch msg {
	case wmNCLButtonDown, wmNCLButtonDblClk:
		return platform.ButtonPrimary, true, hit, true
	case wmNCLButtonUp:
		return platform.ButtonPrimary, false, hit, true
	case wmNCRButtonDown, wmNCRButtonDblClk:
		return platform.ButtonSecondary, true, hit, true
	case wmNCRButtonUp:
		return platform.ButtonSecondary, false, hit, true
	case wmNCMButtonDown, wmNCMButtonDblClk:
		return platform.ButtonMiddle, true, hit, true
	case wmNCMButtonUp:
		return platform.ButtonMiddle, false, hit, true
	case wmNCXButtonDown, wmNCXButtonDblClk, wmNCXButtonUp:
		down = msg != wmNCXButtonUp
		hit = uintptr(loword(wparam))
		switch hiword(wparam) {
		case xButton1:
			return platform.ButtonBack, down, hit, true
		case xButton2:
			return platform.ButtonForward, down, hit, true
		}
	}
	return platform.ButtonNone, false, hit, false
}

// reportedState is the WindowState the backend reports. Maximized is
// IsZoomed, except where the window is not showing that state: minimized
// (IsZoomed is false while a window is iconic), it is restoreMax, the
// WPF_RESTORETOMAXIMIZED flag of its placement; in fullscreen it is
// fsMaximized. Both are the state the window returns to, which the
// platform contract reports (X11 keeps _NET_WM_STATE_MAXIMIZED on an
// iconified window), so minimizing a maximized window does not lose its
// maximized placement. pos is the content origin of the normal placement
// (see Window.updateState).
func reportedState(iconic, zoomed, restoreMax, fullscreen, fsMaximized, frameless bool, pos image.Point) platform.WindowState {
	st := platform.WindowState{
		Minimized:             iconic,
		Maximized:             zoomed,
		Fullscreen:            fullscreen,
		ClientSideDecorations: frameless,
		HasPosition:           true,
		Position:              pos,
	}
	switch {
	case fullscreen:
		st.Maximized = fsMaximized
	case iconic:
		st.Maximized = restoreMax
	}
	return st
}

// centerOver returns the top-left of a window of outer size size centered
// over the parent's window rectangle, moved inside the work area of the
// parent's monitor when it would stick out (a dialog's placement).
func centerOver(parent image.Rectangle, size image.Point, work image.Rectangle) image.Point {
	p := image.Pt(parent.Min.X+(parent.Dx()-size.X)/2, parent.Min.Y+(parent.Dy()-size.Y)/2)
	if work.Empty() {
		return p
	}
	p.X = max(min(p.X, work.Max.X-size.X), work.Min.X)
	p.Y = max(min(p.Y, work.Max.Y-size.Y), work.Min.Y)
	return p
}

// visibleBorder is the width in physical pixels of the frame border that
// DWM draws on Windows 10 and 11, at every DPI: the rest of the side and
// bottom frame (SM_CXSIZEFRAME plus SM_CXPADDEDBORDER) is an invisible
// resize band where the window's shadow is (Chromium's GetFrameThickness,
// ui/base/win/hwnd_metrics.cc).
const visibleBorder = 1

// visibleFrame returns the part of a window's outer rectangle (what
// GetWindowRect and AdjustWindowRectExForDpi give) that shows on screen,
// for the client rectangle client inside it: the outer rectangle without
// the invisible resize borders on the left, right and bottom (7 physical
// pixels each at 96 DPI: what DWMWA_EXTENDED_FRAME_BOUNDS reports once
// the window was shown, about (7, 0, 7, 7) inside GetWindowRect). The top
// has none: its resize band lies inside the caption. A side without frame
// (a frameless window, whose whole window is client area) has nothing to
// remove.
//
// It is computed instead of measured because DwmGetWindowAttribute answers
// with GetWindowRect itself until a window is first shown, and the fit of
// a new window must be done before that to avoid a visible jump.
func visibleFrame(outer, client image.Rectangle) image.Rectangle {
	inv := func(frame int) int { return max(frame-visibleBorder, 0) }
	return image.Rect(outer.Min.X+inv(client.Min.X-outer.Min.X), outer.Min.Y,
		outer.Max.X-inv(outer.Max.X-client.Max.X), outer.Max.Y-inv(outer.Max.Y-client.Max.Y))
}

// fitVisible returns how far a window must move for its visible frame to
// be grabbable on the monitors present: platform.FitOnScreens on
// visibleFrame(outer, client) over the work areas.
func fitVisible(outer, client image.Rectangle, works []image.Rectangle) image.Point {
	vis := visibleFrame(outer, client)
	return platform.FitOnScreens(vis, works).Min.Sub(vis.Min)
}

// maximizedClient returns the client rectangle of a maximized frameless
// window from the window rectangle Windows proposes: Windows puts the
// resize borders of a maximized window outside the work area, and a window
// without a non-client area would draw over them (onto the next monitor and
// under the task bar), so the client area is clipped to the work area, as
// Chromium does.
func maximizedClient(proposed, work image.Rectangle) image.Rectangle {
	if work.Empty() {
		return proposed
	}
	if r := proposed.Intersect(work); !r.Empty() {
		return r
	}
	return proposed
}

// iconBGRA returns img scaled to side x side as the pixels of a 32-bit
// top-down icon DIB: B, G, R, A, not premultiplied (what CreateIconIndirect
// expects of a color bitmap with an alpha channel). Scaling is Catmull-Rom
// on premultiplied pixels, so transparent edges do not darken.
func iconBGRA(img image.Image, side int) []byte {
	b := img.Bounds()
	var src image.Image = img
	if b.Dx() != side || b.Dy() != side {
		dst := image.NewRGBA(image.Rect(0, 0, side, side))
		xdraw.CatmullRom.Scale(dst, dst.Rect, img, b, xdraw.Src, nil)
		src = dst
	}
	n := image.NewNRGBA(image.Rect(0, 0, side, side))
	draw.Draw(n, n.Rect, src, src.Bounds().Min, draw.Src)
	out := make([]byte, 4*side*side)
	for i := 0; i < len(out); i += 4 {
		out[i+0] = n.Pix[i+2]
		out[i+1] = n.Pix[i+1]
		out[i+2] = n.Pix[i+0]
		out[i+3] = n.Pix[i+3]
	}
	return out
}
