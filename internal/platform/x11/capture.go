//go:build linux

package x11

import (
	"fmt"
	"image"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Capture reads the window's pixels from the X server (GetImage) and
// returns them as an image of the window's size in physical pixels, with
// an alpha of 255: what the server holds of the window, whichever present
// put it there (PutImage, or a pixmap of dmabufs). It is a round trip, for
// the UI thread. For the live tests of the layers above, which compare
// what a real server shows with what they rendered: the server answers
// for a mapped window also where no monitor shows it (Xwayland on a
// hidden workspace), and with an error for an unmapped one.
func (w *Window) Capture() (*image.RGBA, error) {
	if w.Closed() {
		return nil, fmt.Errorf("x11: Capture of a closed window")
	}
	b := w.b
	drawable := xproto.Drawable(w.id)
	geo, err := xproto.GetGeometry(b.conn, drawable).Reply()
	if err != nil {
		return nil, fmt.Errorf("x11: GetGeometry: %w", err)
	}
	width, height := int(geo.Width), int(geo.Height)
	reply, err := xproto.GetImage(b.conn, xproto.ImageFormatZPixmap, drawable, 0, 0, geo.Width, geo.Height, 0xffffffff).Reply()
	if err != nil {
		return nil, fmt.Errorf("x11: GetImage: %w", err)
	}
	if len(reply.Data) < 4*width*height {
		return nil, fmt.Errorf("x11: GetImage returned %d bytes for a %dx%d window", len(reply.Data), width, height)
	}
	// The server's 32-bpp ZPixmap layout: B, G, R, X (checked by New).
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < 4*width*height; i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = reply.Data[i+2], reply.Data[i+1], reply.Data[i], 255
	}
	return img, nil
}
