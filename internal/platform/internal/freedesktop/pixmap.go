//go:build linux

package freedesktop

import (
	"image"
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// pixmap is a StatusNotifierItem icon, D-Bus type (iiay): width, height
// and ARGB32 pixels in network byte order (A, R, G, B bytes), not
// premultiplied, rows top to bottom.
type pixmap struct {
	Width, Height int32
	Data          []byte
}

// argbPixmap converts img to a StatusNotifierItem pixmap.
func argbPixmap(img image.Image) pixmap {
	n := platform.IconNRGBA(img)
	w, h := n.Rect.Dx(), n.Rect.Dy()
	out := make([]byte, 0, 4*w*h)
	for y := range h {
		row := n.Pix[y*n.Stride : y*n.Stride+4*w]
		for x := 0; x < len(row); x += 4 {
			out = append(out, row[x+3], row[x], row[x+1], row[x+2])
		}
	}
	return pixmap{Width: int32(w), Height: int32(h), Data: out}
}

// maxPixmapSide bounds the images sent as tray pixmaps: tray icons are
// drawn at 16 to 64 logical pixels, and every image is sent with each
// property read.
const maxPixmapSide = 256

// trayPixmaps converts the icon images to pixmaps, smallest first: every
// image up to maxPixmapSide (hosts pick the size they draw), or the
// smallest one when all are larger.
func trayPixmaps(images []image.Image) []pixmap {
	sorted := slices.Clone(images)
	slices.SortStableFunc(sorted, func(a, b image.Image) int {
		return min(a.Bounds().Dx(), a.Bounds().Dy()) - min(b.Bounds().Dx(), b.Bounds().Dy())
	})
	var out []pixmap
	for _, img := range sorted {
		if b := img.Bounds(); max(b.Dx(), b.Dy()) <= maxPixmapSide || len(out) == 0 {
			out = append(out, argbPixmap(img))
		}
	}
	return out
}

func equalPixmaps(a, b []pixmap) bool {
	return slices.EqualFunc(a, b, func(x, y pixmap) bool {
		return x.Width == y.Width && x.Height == y.Height && slices.Equal(x.Data, y.Data)
	})
}

// imageData is the notification image-data hint, D-Bus type (iiibiiay):
// width, height, rowstride, has alpha, bits per sample, channels and RGBA
// bytes, not premultiplied.
type imageData struct {
	Width, Height, Rowstride int32
	HasAlpha                 bool
	BitsPerSample, Channels  int32
	Data                     []byte
}

// notificationImage converts img to the image-data hint.
func notificationImage(img image.Image) imageData {
	n := platform.IconNRGBA(img) // tightly packed: Stride is 4 × width
	return imageData{
		Width:         int32(n.Rect.Dx()),
		Height:        int32(n.Rect.Dy()),
		Rowstride:     int32(n.Stride),
		HasAlpha:      true,
		BitsPerSample: 8,
		Channels:      4,
		Data:          n.Pix,
	}
}
