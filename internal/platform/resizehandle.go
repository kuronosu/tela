package platform

import (
	"image"
	"math"
)

// The app's resize handles (HitResizeTop to HitResizeBottomRight regions,
// h.WindowResize) beyond hit-testing (contract v12, ADR-261): the input of
// a Transparent window, and the smallest size an interactive resize leaves.
// See "Custom title bars" and "Transparent windows" in the package
// documentation.

// HandleRects returns the rectangles of the resize handles among regions,
// multiplied by scale (1 for logical pixels, the window's scale for its
// framebuffer's physical pixels), rounded outward and cut to a frame of
// the given size; nil when there is none. A Transparent window that can be
// resized takes input over them, beside its InputMask, even where its
// pixels are transparent (the outside of a rounded card's corner).
func HandleRects(regions []HitRegion, scale float64, size image.Point) []image.Rectangle {
	var out []image.Rectangle
	bounds := image.Rectangle{Max: size}
	for _, r := range regions {
		if KindEdge(r.Kind) == 0 {
			continue
		}
		x0 := math.Floor(float64(r.Rect.X) * scale)
		y0 := math.Floor(float64(r.Rect.Y) * scale)
		x1 := math.Ceil(float64(r.Rect.X+r.Rect.W) * scale)
		y1 := math.Ceil(float64(r.Rect.Y+r.Rect.H) * scale)
		ir := image.Rect(int(x0), int(y0), int(x1), int(y1)).Intersect(bounds)
		if !ir.Empty() {
			out = append(out, ir)
		}
	}
	return out
}

// SameRects reports whether a and b hold the same rectangles in the same
// order.
func SameRects(a, b []image.Rectangle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// HandleAlpha is the alpha, of 255, that backends whose window system
// passes the pointer through fully transparent pixels by itself (Win32
// layered windows, Cocoa non-opaque windows) give the transparent pixels
// under the resize handles of a Transparent window, in their own copy of
// the presented frame: a premultiplied {0, 0, 0, 1}, which darkens what is
// below by less than half a percent, so the window server hit-tests the
// window there and the press reaches the handle (HandleCatch).
const HandleAlpha = 1

// HandleCatch is the resize handles a backend caught in its own copy of a
// Transparent window's frames (HandleAlpha under the handles' transparent
// pixels). The zero value has caught none. It is not safe for concurrent
// use.
type HandleCatch struct{ rects []image.Rectangle }

// Update catches the handles now (HandleRects, physical pixels inside the
// image) in pix, four bytes per pixel with the alpha last (premultiplied
// RGBA or BGRA), rows stride bytes apart, after the backend copied a
// frame's damage into it, releasing the ones caught before when they
// changed. When they did, it returns the bounds of the old and the new
// handles, which the caller presents beside the damage; otherwise it
// changed only pixels of the damage it just copied, and returns an empty
// rectangle.
func (c *HandleCatch) Update(pix []byte, stride int, now []image.Rectangle) image.Rectangle {
	var dirty image.Rectangle
	if !SameRects(now, c.rects) {
		releaseHandles(pix, stride, c.rects)
		for _, r := range c.rects {
			dirty = dirty.Union(r)
		}
		for _, r := range now {
			dirty = dirty.Union(r)
		}
		c.rects = append(c.rects[:0], now...)
	}
	catchHandles(pix, stride, now)
	return dirty
}

// Contains reports whether p (physical pixels) is on a caught handle.
func (c *HandleCatch) Contains(p image.Point) bool {
	for _, r := range c.rects {
		if p.In(r) {
			return true
		}
	}
	return false
}

// Reset forgets the caught handles (a new image of another size).
func (c *HandleCatch) Reset() { c.rects = c.rects[:0] }

// catchHandles sets the pixels of rects whose alpha is zero to the color
// 0 with HandleAlpha.
func catchHandles(pix []byte, stride int, rects []image.Rectangle) {
	for _, r := range rects {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			row := pix[y*stride+4*r.Min.X : y*stride+4*r.Max.X]
			for i := 3; i < len(row); i += 4 {
				if row[i] == 0 {
					row[i-3], row[i-2], row[i-1], row[i] = 0, 0, 0, HandleAlpha
				}
			}
		}
	}
}

// releaseHandles undoes catchHandles over rects: their pixels of color 0
// with HandleAlpha become fully transparent again (a frame's own pixel of
// that value, the faintest black, becomes transparent too, which shows no
// difference and takes no input either way).
func releaseHandles(pix []byte, stride int, rects []image.Rectangle) {
	for _, r := range rects {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			row := pix[y*stride+4*r.Min.X : y*stride+4*r.Max.X]
			for i := 3; i < len(row); i += 4 {
				if row[i] == HandleAlpha && row[i-3]|row[i-2]|row[i-1] == 0 {
					row[i] = 0
				}
			}
		}
	}
}

// minHandleSpan is the smallest extent, in logical pixels, ResizeFloor
// leaves the resize handles along an axis: the two corners of
// h.WindowResize (16 each) and of the backends' own band (resizeCorner).
const minHandleSpan = 2 * resizeCorner

// ResizeFloor returns the smallest size, in logical pixels, that the
// interactive resizes a backend bounds itself (Win32's minimum tracking
// size of a frameless or transparent window, the Cocoa backend's own
// resize loop) leave a window of the given size whose hit regions are
// regions, on the axes the app's MinSize does not limit: the size at which
// the extent of the resize handles along each axis is minHandleSpan, the
// window's margin around them staying as it is (a transparent window's
// shadow margin), so the handles can always be grabbed again. Along an
// axis, the extent runs from the left (top) handles' outer side to the
// right (bottom) handles' (the window's own edge on a side without any;
// both sides with no handle on that axis at all, as on a window without
// handles, whose own edges are its band). A window already smaller keeps
// its size on that axis: the floor never grows a window.
func ResizeFloor(size Size, regions []HitRegion) Size {
	// lo and hi per axis (0: x, 1: y), and whether a handle set them.
	lo := [2]float32{0, 0}
	hi := [2]float32{size.W, size.H}
	var hasLo, hasHi [2]bool
	for _, r := range regions {
		e := KindEdge(r.Kind)
		if e == 0 {
			continue
		}
		x0, y0, x1, y1 := r.Rect.X, r.Rect.Y, r.Rect.X+r.Rect.W, r.Rect.Y+r.Rect.H
		side := func(axis int, low, high Edge, a0, a1 float32) {
			if e&low != 0 {
				if !hasLo[axis] || a0 < lo[axis] {
					lo[axis] = a0
				}
				hasLo[axis] = true
			}
			if e&high != 0 {
				if !hasHi[axis] || a1 > hi[axis] {
					hi[axis] = a1
				}
				hasHi[axis] = true
			}
		}
		side(0, EdgeLeft, EdgeRight, x0, x1)
		side(1, EdgeTop, EdgeBottom, y0, y1)
	}
	full := [2]float32{size.W, size.H}
	var out [2]float32
	for i := range 2 {
		ext := max(hi[i]-lo[i], 0)
		out[i] = max(min(full[i]-ext+minHandleSpan, full[i]), 0)
	}
	return Size{W: out[0], H: out[1]}
}
