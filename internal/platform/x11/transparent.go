//go:build linux

package x11

import (
	"image"
	"strconv"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Transparent windows (WindowOptions.Transparent, ADR-249).
//
//   - A transparent window is created with the screen's 32-bit TrueColor
//     visual (findARGBVisual), a colormap of that visual and a border
//     pixel (a window of another visual than its parent's needs both, or
//     CreateWindow fails with BadMatch): its PutImage pixels keep the
//     framebuffer's premultiplied alpha, which compositing managers read
//     as premultiplied (GTK 3's and Qt's RGBA windows, winit's
//     with_transparent). A screen without such a visual gives the root
//     visual, and the window stays opaque.
//   - The alpha shows only while a compositing manager runs (an owner of
//     _NET_WM_CM_S<screen>, EWMH); without one the server shows the
//     pixels' RGB, black where they are transparent. So the window
//     reports WindowState.Transparent only while there is one, and core
//     renders it opaque otherwise. The backend follows the manager while
//     a transparent window exists: it selects StructureNotify on the
//     selection owner's window, whose DestroyNotify is the manager
//     quitting, and a new manager announces itself with the MANAGER
//     client message on the root window (ICCCM 2.8), which the root's
//     StructureNotify mask brings. A manager that takes the selection
//     without sending MANAGER (some do not: picom before v10, xcompmgr)
//     is seen through XFixes 2.0 SelectSelectionInput on the selection
//     (xfixes.go), as GTK 3 and Qt watch it: XFixesSelectionNotify
//     reports every new owner and the owner's end. Without XFixes only
//     DestroyNotify and MANAGER remain.
//   - Its input shape (SHAPE 1.1, ShapeInput) is the mask of its frames
//     (platform.InputMask, the pixels at least platform.InputAlpha opaque,
//     updated in Present from the damage), sent when it changes: the
//     server delivers no pointer events outside it and gives them to the
//     windows below. Without a manager, or without SHAPE 1.1, it takes
//     input everywhere (the input shape is reset to cover the window).
//     Xwayland passes input shapes to the compositor.
//   - No resize edges: its edges are usually the transparent margin. The
//     app's resize handles (HitResize… regions, h.WindowResize) are in
//     the input shape too while the window can be resized (contract v12,
//     ADR-261), over transparent pixels as well (the outside of a rounded
//     card's corner): the shape is the banded union of the mask and the
//     handles (InputMask.RectsWith), set again when either changes.
//   - A backdrop that is a material (Mica, Mica Alt, Acrylic;
//     Backdrop.ClientBackdrop) on a window shown with alpha sets
//     _KDE_NET_WM_BLUR_BEHIND_REGION (CARDINAL[] of x, y, width, height in
//     window pixels): the input mask's rectangles, so KWin's blur effect
//     blurs what is behind the card, not the margin (the property is
//     deleted for an empty mask, which KWin would read as the whole
//     window). Other window managers ignore it, and no protocol of KDE's
//     is vendored: the property is a name and a format (ADR-261).
//   - _NET_WM_BYPASS_COMPOSITOR is not set: the manager must composite it.

// cmState is the compositing manager, tracked from the first
// transparent window on.
type cmState struct {
	tracking bool
	sel      xproto.Atom   // _NET_WM_CM_S<screen>
	manager  xproto.Atom   // MANAGER
	owner    xproto.Window // the selection's owner, 0 for none
}

// fullShape is an input shape that covers any window: the default shape.
var fullShape = []xproto.Rectangle{{Width: 0xffff, Height: 0xffff}}

// maxShapeRects bounds the rectangles of an input shape (platform.InputMask.Rects).
const maxShapeRects = 2048

// compositing reports whether a compositing manager runs on the screen,
// starting to follow it at the first call.
func (b *Backend) compositing() bool {
	if !b.cm.tracking {
		b.cm.tracking = true
		b.cm.sel = b.atom("_NET_WM_CM_S" + strconv.Itoa(b.screenNum))
		b.cm.manager = b.atom("MANAGER")
		// StructureNotify on the root brings the MANAGER messages, beside
		// the PropertyChange the backend always selects there.
		b.xs.selectInput(b.root, xproto.EventMaskPropertyChange|xproto.EventMaskStructureNotify)
		b.xs.watchSelection(b.cm.sel) // before reading the owner: no change is missed
		b.refreshCM()
	}
	return b.cm.owner != 0
}

// refreshCM reads the manager selection's owner again (a round trip),
// selects StructureNotify on a new owner's window (its DestroyNotify is
// the manager's end), and reports whether the owner changed.
func (b *Backend) refreshCM() bool {
	var owner xproto.Window
	if b.cm.sel != 0 {
		owner = b.xs.owner(b.cm.sel)
	}
	if owner == b.cm.owner {
		return false
	}
	b.cm.owner = owner
	if owner != 0 {
		b.xs.selectInput(owner, xproto.EventMaskStructureNotify)
	}
	return true
}

// cmEvent handles the X events that tell a compositing manager started
// or quit, and reports whether ev was one.
func (b *Backend) cmEvent(ev xgb.Event) bool {
	if !b.cm.tracking {
		return false
	}
	switch ev := ev.(type) {
	case xproto.DestroyNotifyEvent:
		if ev.Window != b.cm.owner || ev.Window == 0 {
			return false
		}
	case xproto.ClientMessageEvent:
		if ev.Window != b.root || ev.Type != b.cm.manager || b.cm.manager == 0 || ev.Format != 32 ||
			len(ev.Data.Data32) < 2 || xproto.Atom(ev.Data.Data32[1]) != b.cm.sel {
			return false
		}
	case selectionNotify:
		if ev.selection != b.cm.sel || b.cm.sel == 0 {
			return false
		}
	default:
		return false
	}
	if b.refreshCM() {
		for _, w := range b.windows {
			w.syncTransparent()
		}
	}
	return true
}

// initTransparent sets up a new transparent window's state; argb tells
// whether it has the ARGB visual.
func (w *Window) initTransparent(argb bool) {
	w.argb = argb
	w.osState.Transparent = argb && w.b.compositing()
	w.backdrop = w.opts.Backdrop
}

// syncTransparent reports the window's transparency after the
// compositing manager changed: an ARGB window is transparent while one
// runs. Its input shape is reset while it is opaque, and set from the
// next Present's mask once transparent again (core renders it fully when
// the state changes).
func (w *Window) syncTransparent() {
	if !w.argb || w.Closed() {
		return
	}
	on := w.b.cm.owner != 0
	if on == w.osState.Transparent {
		return
	}
	w.osState.Transparent = on
	if !on {
		w.input = platform.InputMask{}
		w.handles = nil
		w.setInputShape(fullShape)
		w.syncBlur()
	}
	w.b.loop.UpdateState(w, w.osState)
}

// syncInputShape updates the input mask of a transparent window shown
// with alpha from the rectangles of fb just copied, and sets the input
// shape (and the blur region) when it changed.
func (w *Window) syncInputShape(fb *image.RGBA, rects []image.Rectangle) {
	if !w.argb || !w.osState.Transparent {
		return
	}
	maskChanged := w.input.Update(fb, rects)
	handles := w.handleRects()
	if maskChanged || !platform.SameRects(handles, w.handles) {
		w.handles = handles
		w.setInputShape(shapeRects(w.input.RectsWith(handles, maxShapeRects)))
	}
	if maskChanged {
		w.syncBlur()
	}
}

// handleRects returns the app's resize handles in window pixels, nil
// while the window cannot be resized (FixedSize, maximized, fullscreen)
// or before its first frame.
func (w *Window) handleRects() []image.Rectangle {
	if w.opts.FixedSize || w.osState.Maximized || w.osState.Fullscreen {
		return nil
	}
	return platform.HandleRects(w.regions, w.b.scale, w.input.Size())
}

// syncHandles sets the input shape again after the hit regions or the
// window's state changed, when the handles in it did.
func (w *Window) syncHandles() {
	if !w.argb || !w.osState.Transparent || w.input.Size() == (image.Point{}) {
		return
	}
	handles := w.handleRects()
	if platform.SameRects(handles, w.handles) {
		return
	}
	w.handles = handles
	w.setInputShape(shapeRects(w.input.RectsWith(handles, maxShapeRects)))
}

// maxBlurRects bounds the rectangles of the blur region.
const maxBlurRects = 256

// syncBlur sets _KDE_NET_WM_BLUR_BEHIND_REGION from the input mask while
// the window is shown with alpha and its backdrop is a material, and
// deletes it otherwise (or for an empty mask).
func (w *Window) syncBlur() {
	var data []byte
	if w.argb && w.osState.Transparent && w.backdrop.ClientBackdrop() {
		for _, r := range w.input.Rects(maxBlurRects) {
			data = append(data, card32s(uint32(r.Min.X), uint32(r.Min.Y), uint32(r.Dx()), uint32(r.Dy()))...)
		}
	}
	if len(data) == 0 && !w.blurred {
		return
	}
	prop := w.b.atom("_KDE_NET_WM_BLUR_BEHIND_REGION")
	if prop == 0 {
		return
	}
	if len(data) == 0 {
		w.b.xs.deleteProperty(w.id, prop)
		w.blurred = false
		return
	}
	w.b.xs.changeProperty(w.id, prop, xproto.AtomCardinal, 32, data)
	w.blurred = true
}

// setInputShape sets the window's input shape, where the server has
// input shapes.
func (w *Window) setInputShape(rects []xproto.Rectangle) {
	if w.b.xs.inputShapes() {
		w.b.xs.setInputShape(w.id, rects)
	}
}

// shapeRects converts the mask's rectangles (YX-banded, physical pixels:
// the window's coordinates) to a SHAPE rectangle list.
func shapeRects(rects []image.Rectangle) []xproto.Rectangle {
	out := make([]xproto.Rectangle, 0, len(rects))
	for _, r := range rects {
		out = append(out, xproto.Rectangle{X: int16(r.Min.X), Y: int16(r.Min.Y), Width: uint16(r.Dx()), Height: uint16(r.Dy())})
	}
	return out
}

// argbColormap returns the colormap of the ARGB visual, made at first use;
// 0 when the server refuses an ID.
func (b *Backend) argbColormap() xproto.Colormap {
	if b.argbCmap == 0 {
		id, err := xproto.NewColormapId(b.conn)
		if err != nil {
			return 0
		}
		xproto.CreateColormap(b.conn, xproto.ColormapAllocNone, id, b.root, b.argbVisual)
		b.argbCmap = id
	}
	return b.argbCmap
}

// --- connServer ---

func (s connServer) setInputShape(win xproto.Window, rects []xproto.Rectangle) {
	b := s.b
	// Split to the request size limit (the standard length field): the
	// first request sets the shape, the others add to it.
	per := (min(b.maxRequest, 0xffff*4) - 16) / 8
	op := byte(shapeSet)
	for len(rects) > 0 || op == shapeSet {
		n := min(per, len(rects))
		b.sendRequest(shapeRectanglesRequest(b.shape.opcode, op, shapeInput, win, rects[:n]))
		rects, op = rects[n:], shapeUnion
	}
}
