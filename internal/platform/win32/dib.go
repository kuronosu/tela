package win32

import (
	"image"

	"github.com/kuronosu/tela/internal/platform"
)

// bitmapInfoHeader is BITMAPINFOHEADER. The five trailing fields (biSizeImage,
// the resolution and the color table counts) are zero for an uncompressed
// bitmap without a palette.
type bitmapInfoHeader struct {
	size        uint32
	width       int32
	height      int32 // negative: top-down rows
	planes      uint16
	bitCount    uint16
	compression uint32
	_           [20]byte
}

// biRGB is BI_RGB, uncompressed pixels.
const biRGB = 0

// dib is the retained copy of a window's content: top-down 32-bit pixels in
// the byte order B, G, R, X that a BI_RGB DIB has. It answers WM_PAINT
// without involving the app, and is the bits argument of SetDIBitsToDevice.
//
// The X byte is 0xFF, except for a window that shows a system backdrop in
// its client area (alpha, ADR-187). The platform's windows are opaque, but
// DWM reads the alpha of the pixels that GDI draws into the part of a
// window that is "glass" (the whole client area while a composition change
// is being redone, and any margin of DwmExtendFrameIntoClientArea): a zero
// byte there shows the desktop through the window. A backdrop window keeps
// the premultiplied alpha of what the app renders, so the glass shows
// where the app draws nothing.
type dib struct {
	pix  []byte
	size image.Point
	// alpha keeps the alpha of the app's pixels instead of making them
	// opaque.
	alpha bool
}

// stride returns the bytes per row.
func (d *dib) stride() int { return 4 * d.size.X }

// resize makes the copy size px, discarding the content when the size
// changes (the first Present after a ConfigureEvent is a full frame, which
// fills it again), except for the alpha bytes, which stay opaque (or
// transparent for an alpha copy). It reports whether the size changed.
func (d *dib) resize(size image.Point) bool {
	if d.size == size && d.pix != nil {
		return false
	}
	d.size = size
	d.pix = make([]byte, 4*size.X*size.Y)
	if !d.alpha {
		d.opaque()
	}
	return true
}

// opaque sets every alpha byte to 0xFF.
func (d *dib) opaque() {
	for i := 3; i < len(d.pix); i += 4 {
		d.pix[i] = 0xFF
	}
}

// setAlpha chooses whether the copy keeps the alpha of the pixels it is
// given. It reports whether that changed; the next Present must then be a
// full one. Leaving alpha mode makes the copy opaque at once.
func (d *dib) setAlpha(on bool) bool {
	if d.alpha == on {
		return false
	}
	d.alpha = on
	if !on {
		d.opaque()
	}
	return true
}

// release frees the pixels (the alpha mode stays).
func (d *dib) release() { *d = dib{alpha: d.alpha} }

// update copies the rectangles r of fb, which must lie inside both fb and the
// copy, converting premultiplied RGBA to BGRA and making every pixel opaque
// (unless the copy keeps alpha, which stays premultiplied).
func (d *dib) update(fb *image.RGBA, rects []image.Rectangle) {
	stride := d.stride()
	for _, r := range rects {
		platform.CopyRGBAToBGRA(d.pix, stride, fb, r)
		if d.alpha {
			continue
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			row := d.pix[y*stride+4*r.Min.X : y*stride+4*r.Max.X]
			for i := 3; i < len(row); i += 4 {
				row[i] = 0xFF
			}
		}
	}
}

// region describes how to blit the rectangle r of the copy with
// SetDIBitsToDevice. The rectangle is clipped to the copy; clipped is empty
// (and bits nil) when nothing is left.
//
// bits starts at the first row of the rectangle and hdr describes a
// top-down bitmap as wide as the whole copy and as tall as the rectangle, so
// the call takes source x = clipped.Min.X, source y = 0 and as many scan
// lines as clipped.Dy(): the source rectangle then covers the bitmap's full
// height, which is unambiguous whether a DIB origin is its top or bottom row.
func (d *dib) region(r image.Rectangle) (clipped image.Rectangle, bits []byte, hdr bitmapInfoHeader) {
	clipped = r.Intersect(image.Rectangle{Max: d.size})
	if clipped.Empty() || d.pix == nil {
		return image.Rectangle{}, nil, hdr
	}
	hdr = bitmapInfoHeader{
		size:        uint32(bitmapInfoHeaderSize),
		width:       int32(d.size.X),
		height:      -int32(clipped.Dy()),
		planes:      1,
		bitCount:    32,
		compression: biRGB,
	}
	return clipped, d.pix[clipped.Min.Y*d.stride():], hdr
}

// bitmapInfoHeaderSize is sizeof(BITMAPINFOHEADER).
const bitmapInfoHeaderSize = 40
