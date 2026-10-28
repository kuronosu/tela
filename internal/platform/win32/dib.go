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
// The X byte is always 0xFF. The platform's windows are opaque, but DWM
// reads the alpha of the pixels that GDI draws into the part of a window
// that is "glass" (the margin of DwmExtendFrameIntoClientArea that a
// frameless window has, and the whole client area while a frame extension
// or a composition change is being redone): a zero byte there shows the
// desktop through the window.
type dib struct {
	pix  []byte
	size image.Point
}

// stride returns the bytes per row.
func (d *dib) stride() int { return 4 * d.size.X }

// resize makes the copy size px, discarding the content when the size
// changes (the first Present after a ConfigureEvent is a full frame, which
// fills it again), except for the alpha bytes, which stay opaque. It reports
// whether the size changed.
func (d *dib) resize(size image.Point) bool {
	if d.size == size && d.pix != nil {
		return false
	}
	d.size = size
	d.pix = make([]byte, 4*size.X*size.Y)
	for i := 3; i < len(d.pix); i += 4 {
		d.pix[i] = 0xFF
	}
	return true
}

// release frees the pixels.
func (d *dib) release() { *d = dib{} }

// update copies the rectangles r of fb, which must lie inside both fb and the
// copy, converting premultiplied RGBA to BGRA and making every pixel opaque.
func (d *dib) update(fb *image.RGBA, rects []image.Rectangle) {
	stride := d.stride()
	for _, r := range rects {
		platform.CopyRGBAToBGRA(d.pix, stride, fb, r)
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
