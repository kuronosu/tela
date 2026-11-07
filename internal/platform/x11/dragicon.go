//go:build linux

package x11

import (
	"image"
	"strconv"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// The drag image of the app's drag (DragOptions.Image, ADR-232), shown as
// GTK 3 and Qt show theirs: an override-redirect window of the image's
// size (physical pixels, like the image) whose top-left follows the
// pointer minus the hotspot, raised at every move, with
// _NET_WM_WINDOW_TYPE_DND so compositors leave their effects off.
//
//   - With a compositing manager (an owner of _NET_WM_CM_S<screen>) and a
//     32-bit TrueColor visual, the window is ARGB and shows the image's
//     premultiplied pixels as they are (compositors read ARGB windows as
//     premultiplied): GTK 3's rule (gdk_screen_is_composited and the RGBA
//     visual) and Qt's (QShapedPixmapWindow with compositing).
//   - Otherwise it has the root visual, and a bounding shape keeps the
//     pixels at least half opaque, shown opaque (unpremultiplied), as
//     Qt's mask does without compositing; a fully opaque image needs no
//     shape. (GTK 3 draws the image over the theme background instead.)
//   - The image is the window's background pixmap: the server repaints it
//     after exposures by itself, with no Expose to answer (GTK sets its
//     icon as the background too).
//   - The input shape is empty (SHAPE 1.1, Qt's WindowTransparentForInput):
//     TranslateCoordinates skips windows outside their input shape, so the
//     walk that finds the XDND target (dragFind) sees through the image,
//     and nothing under it loses input. GTK 3 skips its icon by XID in its
//     own window cache instead, which TranslateCoordinates cannot do.
//     Without input shapes no image is shown (the contract makes it a
//     hint): the window would hide the target under the pointer.
//
// The window goes when the grabs end: at the drop, at a cancel, or when
// the drag fails.

// shapeExt is the SHAPE extension of the connection.
type shapeExt struct {
	checked bool
	input   bool // SHAPE 1.1 or later
	opcode  byte
}

const (
	shapeOpQueryVersion = 0
	shapeOpRectangles   = 1

	shapeSet   = 0
	shapeUnion = 1

	shapeBounding = 0
	shapeInput    = 2

	shapeYXBanded = 3

	// iconOpaque is the alpha from which a pixel of a drag image is in
	// its bounding shape (cairo's 50 %, GTK's region from a surface).
	iconOpaque = 128
)

// findARGBVisual returns the screen's 32-bit TrueColor visual with the
// alpha in the top byte (B, G, R, A bytes in a 32-bpp ZPixmap), or 0.
func findARGBVisual(setup *xproto.SetupInfo, screen *xproto.ScreenInfo) xproto.Visualid {
	bpp32 := false
	for _, f := range setup.PixmapFormats {
		if f.Depth == 32 && f.BitsPerPixel == 32 {
			bpp32 = true
		}
	}
	if !bpp32 {
		return 0
	}
	for _, d := range screen.AllowedDepths {
		if d.Depth != 32 {
			continue
		}
		for _, v := range d.Visuals {
			if v.Class == xproto.VisualClassTrueColor && v.RedMask == 0xff0000 && v.GreenMask == 0xff00 && v.BlueMask == 0xff {
				return v.VisualId
			}
		}
	}
	return 0
}

// composited reports whether a compositing manager runs on the screen
// (it owns _NET_WM_CM_S<screen>, EWMH; a round trip).
func (b *Backend) composited() bool {
	a := b.atom("_NET_WM_CM_S" + strconv.Itoa(b.screenNum))
	return a != 0 && b.xs.owner(a) != 0
}

// showDragIcon shows the drag image of ds with its hotspot at the
// pointer, (x, y) in root coordinates.
func (b *Backend) showDragIcon(ds *dragSource, img *image.RGBA, hot image.Point, x, y int16) {
	if img == nil || img.Rect.Empty() || !b.xs.inputShapes() {
		return
	}
	argb := b.argbVisual != 0 && b.composited()
	at := image.Pt(int(x)-hot.X, int(y)-hot.Y)
	r := image.Rectangle{Min: at, Max: at.Add(img.Rect.Size())}
	var bounding []xproto.Rectangle
	if !argb {
		bounding = opaqueRects(img)
	}
	win := b.xs.createDragIcon(r, argb, iconPixels(img, argb), bounding)
	if win == 0 {
		return
	}
	b.xs.changeProperty(win, b.atoms.netWMWindowType, xproto.AtomAtom, 32, card32s(uint32(b.atoms.netWMTypeDND)))
	b.xs.mapIcon(win)
	ds.icon, ds.iconHot, ds.iconAt = win, hot, at
}

// moveDragIcon moves the drag image with the pointer, at (x, y) in root
// coordinates.
func (b *Backend) moveDragIcon(ds *dragSource, x, y int16) {
	at := image.Pt(int(x)-ds.iconHot.X, int(y)-ds.iconHot.Y)
	if ds.icon == 0 || at == ds.iconAt {
		return
	}
	ds.iconAt = at
	b.xs.moveIcon(ds.icon, at.X, at.Y)
}

// hideDragIcon destroys the drag image's window.
func (b *Backend) hideDragIcon(ds *dragSource) {
	if ds.icon != 0 {
		b.xs.destroyIcon(ds.icon)
		ds.icon = 0
	}
}

// iconPixels converts a drag image to a 32-bpp ZPixmap (B, G, R, A
// bytes): premultiplied for the ARGB visual; for the root visual the
// pixels the bounding shape keeps are unpremultiplied, so their edge
// does not darken, and the rest are left black.
func iconPixels(img *image.RGBA, argb bool) []byte {
	size := img.Rect.Size()
	src := &image.RGBA{Pix: img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y):], Stride: img.Stride, Rect: image.Rectangle{Max: size}}
	pix := make([]byte, 4*size.X*size.Y)
	platform.CopyRGBAToBGRA(pix, 4*size.X, src, src.Rect)
	if argb {
		return pix
	}
	for i := 0; i < len(pix); i += 4 {
		a := uint32(pix[i+3])
		switch {
		case a < iconOpaque:
			pix[i], pix[i+1], pix[i+2] = 0, 0, 0
		case a < 255:
			for c := range 3 {
				pix[i+c] = byte(min(255, (uint32(pix[i+c])*255+a/2)/a))
			}
		}
		pix[i+3] = 0xff
	}
	return pix
}

// opaqueRects returns the bounding shape of a drag image shown without
// alpha: the runs of pixels at least iconOpaque opaque, as bands of rows
// with the same runs (YX-banded). nil when every pixel is in (no shape
// needed); empty when none is.
func opaqueRects(img *image.RGBA) []xproto.Rectangle {
	size := img.Rect.Size()
	rects := []xproto.Rectangle{}
	var band []xproto.Rectangle // the runs of the current band
	bandStart := 0
	flush := func() {
		rects = append(rects, band...)
		band = nil
	}
	full := true
	for y := range size.Y {
		var runs []xproto.Rectangle
		row := img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):]
		for x := 0; x < size.X; {
			if row[4*x+3] < iconOpaque {
				full = false
				x++
				continue
			}
			x0 := x
			for x < size.X && row[4*x+3] >= iconOpaque {
				x++
			}
			runs = append(runs, xproto.Rectangle{X: int16(x0), Width: uint16(x - x0)})
		}
		if sameRuns(runs, band) {
			for i := range band {
				band[i].Height++
			}
			continue
		}
		flush()
		bandStart = y
		for _, r := range runs {
			r.Y, r.Height = int16(bandStart), 1
			band = append(band, r)
		}
	}
	flush()
	if full {
		return nil
	}
	return rects
}

// sameRuns reports whether the runs of a row (Y and Height unset) are
// those of the band.
func sameRuns(runs, band []xproto.Rectangle) bool {
	if len(runs) != len(band) || len(band) == 0 {
		return false
	}
	for i := range runs {
		if runs[i].X != band[i].X || runs[i].Width != band[i].Width {
			return false
		}
	}
	return true
}

// shapeRectanglesRequest encodes a ShapeRectangles request: operation op
// of the rectangles (YX-banded) on the shape kind of win.
func shapeRectanglesRequest(opcode, op, kind byte, win xproto.Window, rects []xproto.Rectangle) []byte {
	req := newRequest(opcode, shapeOpRectangles, 16+8*len(rects))
	req[4], req[5], req[6] = op, kind, shapeYXBanded
	le.PutUint32(req[8:], uint32(win))
	for i, r := range rects {
		o := 16 + 8*i
		le.PutUint16(req[o:], uint16(r.X))
		le.PutUint16(req[o+2:], uint16(r.Y))
		le.PutUint16(req[o+4:], r.Width)
		le.PutUint16(req[o+6:], r.Height)
	}
	return req
}

// --- connServer ---

func (s connServer) inputShapes() bool {
	b := s.b
	if !b.shape.checked {
		b.shape.checked = true
		if e := b.queryExtension("SHAPE"); e.present {
			r, err := b.roundTrip(newRequest(e.opcode, shapeOpQueryVersion, 4), 12)
			if err == nil {
				major, minor := le.Uint16(r[8:]), le.Uint16(r[10:])
				b.shape = shapeExt{checked: true, input: major > 1 || major == 1 && minor >= 1, opcode: e.opcode}
			}
		}
	}
	return b.shape.input
}

func (s connServer) createDragIcon(r image.Rectangle, argb bool, pix []byte, bounding []xproto.Rectangle) xproto.Window {
	b := s.b
	depth, visual := b.depth, b.visual
	if argb {
		depth, visual = 32, b.argbVisual
		if b.argbCmap == 0 {
			// A window of another visual than its parent's needs a
			// colormap of that visual (and a border pixel).
			id, err := xproto.NewColormapId(b.conn)
			if err != nil {
				return 0
			}
			xproto.CreateColormap(b.conn, xproto.ColormapAllocNone, id, b.root, visual)
			b.argbCmap = id
		}
	}
	size := r.Size()
	pm, err := xproto.NewPixmapId(b.conn)
	if err != nil {
		return 0
	}
	gc, err := xproto.NewGcontextId(b.conn)
	if err != nil {
		return 0
	}
	win, err := xproto.NewWindowId(b.conn)
	if err != nil {
		return 0
	}
	xproto.CreatePixmap(b.conn, depth, pm, xproto.Drawable(b.root), uint16(size.X), uint16(size.Y))
	xproto.CreateGC(b.conn, gc, xproto.Drawable(pm), xproto.GcGraphicsExposures, []uint32{0})
	for _, c := range splitForRequests(image.Rectangle{Max: size}, b.maxRequest, putImageHeaderLen(b.bigRequests)) {
		big := b.bigRequests && putImageHeader+4*c.Dx()*c.Dy() > 0xffff*4
		b.reqBuf = encodePutImage(b.reqBuf, big, xproto.Drawable(pm), gc, depth, pix, size.X, c)
		b.conn.NewRequest(b.reqBuf, b.conn.NewCookie(false, false))
	}
	xproto.FreeGC(b.conn, gc)
	// Save-under, like GTK's temporary windows: the server may keep
	// what the image covers instead of making the windows below redraw.
	mask := uint32(xproto.CwBackPixmap | xproto.CwBorderPixel | xproto.CwOverrideRedirect | xproto.CwSaveUnder)
	values := []uint32{uint32(pm), 0, 1, 1}
	if argb {
		mask |= xproto.CwColormap
		values = append(values, uint32(b.argbCmap))
	}
	err = xproto.CreateWindowChecked(b.conn, depth, win, b.root, int16(r.Min.X), int16(r.Min.Y), uint16(size.X), uint16(size.Y), 0,
		xproto.WindowClassInputOutput, visual, mask, values).Check()
	xproto.FreePixmap(b.conn, pm) // the window holds its background
	if err != nil {
		return 0
	}
	b.sendRequest(shapeRectanglesRequest(b.shape.opcode, shapeSet, shapeInput, win, nil))
	if bounding != nil {
		// Split to the request size limit (the standard length field):
		// the first request sets the shape, the others add to it.
		per := (min(b.maxRequest, 0xffff*4) - 16) / 8
		op := byte(shapeSet)
		for len(bounding) > 0 || op == shapeSet {
			n := min(per, len(bounding))
			b.sendRequest(shapeRectanglesRequest(b.shape.opcode, op, shapeBounding, win, bounding[:n]))
			bounding, op = bounding[n:], shapeUnion
		}
	}
	return win
}

func (s connServer) moveIcon(win xproto.Window, x, y int) {
	xproto.ConfigureWindow(s.b.conn, win, xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowStackMode,
		[]uint32{uint32(int32(x)), uint32(int32(y)), xproto.StackModeAbove})
}

func (s connServer) mapIcon(win xproto.Window) { xproto.MapWindow(s.b.conn, win) }

func (s connServer) destroyIcon(win xproto.Window) { xproto.DestroyWindow(s.b.conn, win) }
