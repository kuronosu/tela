package platform

import (
	"fmt"
	"image"
)

// Helpers shared by backends so that every Present validates and interprets
// its arguments the same way.

// CheckFramebuffer panics with an actionable message if fb is not a
// framebuffer for a window with metrics m: fb must be non-nil and fb.Rect
// must be exactly image.Rect(0, 0, m.Physical()).
func CheckFramebuffer(fb *image.RGBA, m Metrics) {
	if fb == nil {
		panic("platform: Present called with a nil framebuffer; allocate one with image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})")
	}
	want := image.Rectangle{Max: m.Physical()}
	if fb.Rect != want {
		panic(fmt.Sprintf("platform: Present framebuffer bounds %v do not match the window: want %v (logical %v at scale %g); "+
			"reallocate the framebuffer from Metrics().Physical() after every ConfigureEvent",
			fb.Rect, want, m.Size, m.Scale))
	}
	if len(fb.Pix) < fb.Stride*(want.Dy()-1)+4*want.Dx() || fb.Stride < 4*want.Dx() {
		panic(fmt.Sprintf("platform: Present framebuffer has an invalid Pix/Stride (len %d, stride %d) for bounds %v", len(fb.Pix), fb.Stride, fb.Rect))
	}
}

// ClipDamage returns damage clipped to bounds, with empty rectangles
// dropped. nil or empty damage, or a full flag, yields []image.Rectangle{bounds}
// (the whole frame). The result is a new slice; damage is not modified.
// It returns nil only when bounds is empty, or when every rectangle was
// clipped away (nothing to present).
func ClipDamage(damage []image.Rectangle, bounds image.Rectangle, full bool) []image.Rectangle {
	if bounds.Empty() {
		return nil
	}
	if full || len(damage) == 0 {
		return []image.Rectangle{bounds}
	}
	out := make([]image.Rectangle, 0, len(damage))
	for _, r := range damage {
		r = r.Intersect(bounds)
		if !r.Empty() {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CopyRGBA copies the pixels of r (which must lie inside both images) from
// src to dst, row by row, honoring each image's Stride and origin.
func CopyRGBA(dst, src *image.RGBA, r image.Rectangle) {
	n := 4 * r.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		so := src.PixOffset(r.Min.X, y)
		do := dst.PixOffset(r.Min.X, y)
		copy(dst.Pix[do:do+n], src.Pix[so:so+n])
	}
}

// CopyRGBAToBGRA copies the pixels of r from src into dst, a BGRA buffer
// (byte order B, G, R, A; stride dstStride bytes; pixel (0, 0) at dst[0]),
// swapping red and blue. Premultiplication is preserved, so dst is in the
// format of wl_shm ARGB8888/XRGB8888, X11 32-bpp ZPixmap on little-endian
// servers, and Win32 32-bit top-down DIBs.
func CopyRGBAToBGRA(dst []byte, dstStride int, src *image.RGBA, r image.Rectangle) {
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := src.Pix[src.PixOffset(r.Min.X, y):]
		d := dst[y*dstStride+4*r.Min.X:]
		n := 4 * r.Dx()
		_ = s[n-1]
		_ = d[n-1]
		for i := 0; i < n; i += 4 {
			d[i+0] = s[i+2]
			d[i+1] = s[i+1]
			d[i+2] = s[i+0]
			d[i+3] = s[i+3]
		}
	}
}

func panicf(format string, args ...any) { panic(fmt.Sprintf(format, args...)) }
