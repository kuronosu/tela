//go:build linux

package x11

import (
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// TestLiveDragIcon creates the drag image window on the real server, both
// kinds it can (the root visual with a bounding shape; ARGB when a
// compositing manager runs), and checks its pixels with GetImage, its
// empty input shape, and that TranslateCoordinates sees through it (the
// XDND target lookup). The window is override-redirect, takes no input,
// and lives for a moment at the top-left of the screen; no input is sent.
func TestLiveDragIcon(t *testing.T) {
	b := liveBackend(t)
	defer b.conn.Close()
	if !b.xs.inputShapes() {
		t.Skip("the server has no SHAPE 1.1")
	}
	composited := b.composited()
	t.Logf("ARGB visual %#x, compositing manager %v", b.argbVisual, composited)
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			i := img.PixOffset(x, y)
			if y < 24 || x < 32 { // the bottom-right quarter is transparent
				img.Pix[i], img.Pix[i+1], img.Pix[i+2] = pattern(3, x, y)
				img.Pix[i+3] = 0xff
			}
		}
	}
	kinds := []bool{false}
	if b.argbVisual != 0 && composited {
		kinds = append(kinds, true)
	}
	for _, argb := range kinds {
		r := image.Rect(8, 8, 72, 56)
		var bounding []xproto.Rectangle
		if !argb {
			bounding = opaqueRects(img)
		}
		win := b.xs.createDragIcon(r, argb, iconPixels(img, argb), bounding)
		if win == 0 {
			t.Fatalf("argb %v: createDragIcon failed", argb)
		}
		b.xs.changeProperty(win, b.atoms.netWMWindowType, xproto.AtomAtom, 32, card32s(uint32(b.atoms.netWMTypeDND)))
		b.xs.mapIcon(win)
		b.xs.moveIcon(win, 10, 10)
		xproto.GetInputFocus(b.conn).Reply() // sync

		got, err := xproto.GetImage(b.conn, xproto.ImageFormatZPixmap, xproto.Drawable(win), 0, 0, 64, 48, ^uint32(0)).Reply()
		if err != nil {
			t.Fatalf("argb %v: GetImage: %v", argb, err)
		}
		bad := 0
		for y := range 24 {
			for x := range 64 {
				o := 4 * (y*64 + x)
				pr, pg, pb := pattern(3, x, y)
				if o+3 >= len(got.Data) || got.Data[o] != pb || got.Data[o+1] != pg || got.Data[o+2] != pr {
					bad++
				}
			}
		}
		if bad > 0 {
			t.Errorf("argb %v (depth %d): %d of the opaque pixels differ", argb, got.Depth, bad)
		}
		if n := liveShapeRects(t, b, win, shapeInput); n != 0 {
			t.Errorf("argb %v: the input shape has %d rectangles, want none", argb, n)
		}
		if !argb {
			if n := liveShapeRects(t, b, win, shapeBounding); n != len(bounding) {
				t.Errorf("bounding shape: %d rectangles, want %d", n, len(bounding))
			}
		}
		if _, _, child, ok := b.xs.translate(b.root, b.root, 20, 20); !ok || child == win {
			t.Errorf("argb %v: TranslateCoordinates found the image window (child %d, ok %v)", argb, child, ok)
		}
		b.xs.destroyIcon(win)
		xproto.GetInputFocus(b.conn).Reply()
	}
}

// liveShapeRects returns the number of rectangles of a shape of win
// (ShapeGetRectangles).
func liveShapeRects(t *testing.T, b *Backend, win xproto.Window, kind byte) int {
	t.Helper()
	req := newRequest(b.shape.opcode, 8, 12)
	le.PutUint32(req[4:], uint32(win))
	req[8] = kind
	r, err := b.roundTrip(req, 32)
	if err != nil {
		t.Fatalf("ShapeGetRectangles: %v", err)
	}
	return int(le.Uint32(r[8:]))
}

// TestLiveScrollDevices logs what the backend knows about the server's
// scroll devices: touchpad or wheel, and whether the driver posts the
// scroll stop (xi2.go). Under Xwayland every pointer is one
// "xwayland-pointer" device without the libinput properties.
func TestLiveScrollDevices(t *testing.T) {
	b := liveBackend(t)
	defer b.conn.Close()
	if !b.xi.ok {
		t.Skip("no XInput 2.1")
	}
	devs, err := b.xi.query(xiAllDevices)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		if len(d.axes) > 0 {
			t.Logf("device %d %q: %d scroll axes, touchpad %v, stops %v", d.id, d.name, len(d.axes), d.smooth, d.stops)
		}
	}
}
