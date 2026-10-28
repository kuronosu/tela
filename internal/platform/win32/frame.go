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
	pt     image.Point // physical client pixels
	client image.Point // physical client size
	scale  float64
	// border is the width of the resize band in physical pixels, or 0 when
	// the window has none (decorated, FixedSize, maximized, fullscreen).
	border     int
	fullscreen bool
	regions    []platform.HitRegion
}

// clientHit answers WM_NCHITTEST for a point inside the client area: the
// app's window buttons (except at the outermost pixel), the resize band of a
// frameless window (it takes precedence over the other hit regions), then
// the app's hit regions. Corners take twice the band along
// each edge, so they are easier to grab, as Windows' own frames do.
func clientHit(in hitInput) uintptr {
	// A window button of the app takes the band, like the caption buttons
	// of a native frame, which leave only the outermost pixel of the window
	// to the resize edges: the hover over the maximize button of Windows 11
	// (the snap layouts flyout) starts from the hit test at the pointer, and
	// the pointer reaches a button across the band.
	if in.border > 0 && len(in.regions) > 0 {
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
	if len(in.regions) == 0 {
		return htClient
	}
	p := logicalPoint(in.pt.X, in.pt.Y, in.scale)
	return hitCodeOf(platform.HitTest(in.regions, p), in.fullscreen)
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
// IsZoomed, except in fullscreen, where it is the state the window returns
// to. pos is the content origin of the normal placement (see
// Window.updateState).
func reportedState(iconic, zoomed, fullscreen, fsMaximized, frameless bool, pos image.Point) platform.WindowState {
	st := platform.WindowState{
		Minimized:             iconic,
		Maximized:             zoomed,
		Fullscreen:            fullscreen,
		ClientSideDecorations: frameless,
		HasPosition:           true,
		Position:              pos,
	}
	if fullscreen {
		st.Maximized = fsMaximized
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
