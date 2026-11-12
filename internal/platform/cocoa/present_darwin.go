//go:build darwin

package cocoa

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Presentation (ADR-278). The window keeps the frame as last presented
// (back, RGBA) and shows it through a swap chain of two or three
// IOSurfaces of its physical size (BGRA, tagged sRGB), which the layer
// takes as its contents without a copy: a Present copies its damage into
// back, then writes into a surface the layer does not show the rectangles
// that surface lacks (this frame's damage and what it missed while the
// others were shown; surfaces.go) and makes it the layer's contents.
// A caret blink of a 2880x1800 window copies a few kilobytes where the
// CGImage of the whole frame copied about 20 MB twice.
//
// Quirks, after Firefox's NativeLayerCA and SurfacePoolCA and Chromium's
// CALayer tree (Ebitengine and Gio present through Metal only, softbuffer
// through a CGImage per frame):
//
//   - CoreAnimation does not see a change to the surface it shows: setting
//     the same IOSurface again shows the old pixels (Chromium calls the
//     private setContentsChanged for that). The surface written is never
//     the one shown, so every Present sets another object.
//   - The window server may still read the surface shown before the last
//     one for a frame or so (IOSurfaceIsInUse): writing it then could tear.
//     A third surface is added while the one to write is in use; with
//     three, the one shown longest ago is written anyway.
//   - The CPU writes a surface only between IOSurfaceLock and
//     IOSurfaceUnlock; the lock also bumps its seed, which CoreAnimation
//     reads.
//   - The color space is the surface's "IOSurfaceColorSpace" value
//     (kCGColorSpaceSRGB), as Chromium and Firefox set it; untagged, the
//     pixels would be shown in the display's space.
//   - A resize or scale change that changes the physical size replaces the
//     chain (the layer keeps its contents until the next one is set).
//
// When IOSurface cannot be loaded or a surface cannot be created or
// locked, the window falls back for good to the CGImage of the whole
// frame of before (chainErr says why).

// presentCGImage makes every window present a CGImage of the whole
// frame, the fallback path (tests).
var presentCGImage = false

// Present stores the damaged pixels of fb in the window's copy of the frame
// and shows it; see platform.Window and the top of this file. The window is
// shown by the first Present.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	if w.Closed() {
		return nil
	}
	if w.Metrics().Scale == 0 {
		panic("cocoa: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, w.Metrics())
	full := w.NeedsFull() || w.back == nil || w.back.Rect != fb.Rect
	rects := platform.ClipDamage(damage, fb.Rect, full)
	if len(rects) == 0 {
		return nil // every damage rectangle was clipped away: nothing changed
	}
	if full {
		w.ClearNeedsFull()
		if w.back == nil || w.back.Rect != fb.Rect {
			w.back = image.NewRGBA(fb.Rect)
			w.trans.catch.Reset() // nothing caught in the new copy
		}
	}
	for _, r := range rects {
		platform.CopyRGBA(w.back, fb, r)
	}
	changed := rects
	// A transparent window's resize handles (transparent_darwin.go): new
	// handles change pixels outside the damage too.
	if c := w.catchHandles(rects); !c.Empty() {
		changed = append(slices.Clip(rects), c)
	}
	w.updateInputMask(fb, rects) // a transparent window (transparent_darwin.go)
	if err := w.showBack(changed); err != nil {
		return err
	}
	w.presented = true
	if !w.shown && w.visible {
		// Not before: the window would show unpainted.
		w.show()
		w.passIfFaint() // shown under a resting cursor (transparent_darwin.go)
	}
	return nil
}

// showBack shows the retained frame, whose rectangles changed differ from
// what was shown before, as the layer's contents.
func (w *Window) showBack(changed []image.Rectangle) error {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	contents, release, err := w.backContents(changed)
	if err != nil {
		return err
	}
	// The gravity pins the frame to the top-left corner pixel for pixel
	// (resize would resample it when the content size in points is not a
	// whole number of pixels). Its vertical sense follows
	// contentsAreFlipped, which depends on the view hierarchy: read it
	// here, where the layer is in the window.
	g := darwin.CAGravityTopLeft
	if darwin.SendBool(w.layer, sel.contentsAreFlipped) {
		g = darwin.CAGravityBottomLeft
	}
	darwin.SendClass(app.transaction, sel.begin)
	darwin.SendClass(app.transaction, sel.setDisableActions, true) // no implicit cross-fade
	if g != w.gravity {
		w.gravity = g
		darwin.Send(w.layer, sel.setContentsGravity, g)
	}
	darwin.Send(w.layer, sel.setContents, darwin.ID(contents))
	darwin.SendClass(app.transaction, sel.commit)
	if release {
		darwin.CGImageRelease(contents) // the layer keeps it
	}
	return nil
}

// backContents returns the layer's next contents: a surface of the chain
// brought up to date with the retained frame (owned by the chain), or a
// new CGImage of the whole frame (release: the caller releases it).
func (w *Window) backContents(changed []image.Rectangle) (contents uintptr, release bool, err error) {
	if w.chainErr == nil && presentCGImage {
		w.dropChain(errors.New("cocoa: the CGImage path was asked for (presentCGImage)"))
	}
	if w.chainErr == nil {
		s, err := w.writeSurface(changed)
		if err == nil {
			return s, false, nil
		}
		w.dropChain(err)
	}
	img, err := w.b.newImage(w.back)
	return img, true, err
}

// writeSurface writes the retained frame's changed rectangles into the
// chain, and what the surface to show lacks besides, and returns that
// surface.
func (w *Window) writeSurface(changed []image.Rectangle) (uintptr, error) {
	if err := darwin.LoadIOSurface(); err != nil {
		return 0, err
	}
	c := &w.chain
	if size := w.back.Rect.Size(); c.size != size || len(c.surfs) == 0 {
		releaseSurfaces(c.resize(size))
	} else {
		c.damage(changed)
	}
	i := c.next(darwin.IOSurfaceIsInUse)
	if i < 0 {
		s, err := newSurface(c.size)
		switch {
		case err == nil:
			i = c.add(s)
		case len(c.surfs) >= 2:
			// No third surface: write one the window server may
			// still use.
			i = c.next(func(uintptr) bool { return false })
		default:
			return 0, err
		}
	}
	s := c.surfs[i].ref
	if kr := darwin.IOSurfaceLock(s, 0, nil); kr != 0 {
		return 0, fmt.Errorf("cocoa: IOSurfaceLock failed (kern_return_t %#x)", uint32(kr))
	}
	base := darwin.IOSurfaceGetBaseAddress(s)
	stride := int(darwin.IOSurfaceGetBytesPerRow(s))
	if base == nil || stride < 4*c.size.X {
		darwin.IOSurfaceUnlock(s, 0, nil)
		return 0, fmt.Errorf("cocoa: an IOSurface of %v pixels has base address %p and %d bytes per row", c.size, base, stride)
	}
	dst := unsafe.Slice((*byte)(base), stride*c.size.Y)
	copied := 0
	for _, r := range c.stale(i) {
		platform.CopyRGBAToBGRA(dst, stride, w.back, r)
		copied += 4 * r.Dx() * r.Dy()
	}
	darwin.IOSurfaceUnlock(s, 0, nil)
	c.shownAt(i, copied)
	return s, nil
}

// newSurface returns a new BGRA IOSurface (+1) of size pixels in sRGB.
func newSurface(size image.Point) (uintptr, error) {
	s, err := darwin.NewIOSurfaceBGRA(size.X, size.Y, darwin.CGColorSpaceSRGB)
	if err != nil {
		return 0, err
	}
	if gw, gh := int(darwin.IOSurfaceGetWidth(s)), int(darwin.IOSurfaceGetHeight(s)); gw != size.X || gh != size.Y {
		darwin.CFRelease(s)
		return 0, fmt.Errorf("cocoa: IOSurfaceCreate of %v pixels made one of %dx%d", size, gw, gh)
	}
	return s, nil
}

// dropChain releases the window's surfaces and makes it present CGImages
// from now on, for the reason err.
func (w *Window) dropChain(err error) {
	releaseSurfaces(w.chain.resize(image.Point{}))
	w.chainErr = err
}

// releaseChain releases the window's surfaces (Close); the layer keeps
// the one it shows until it is released.
func (w *Window) releaseChain() { releaseSurfaces(w.chain.resize(image.Point{})) }

func releaseSurfaces(refs []uintptr) {
	for _, s := range refs {
		darwin.CFRelease(s)
	}
}

// newImage returns a CGImage (+1) holding a copy of fb's pixels:
// premultiplied RGBA in sRGB, Go's image.RGBA layout.
func (b *Backend) newImage(fb *image.RGBA) (uintptr, error) {
	size := fb.Rect.Size()
	data := darwin.CFDataCreate(0, &fb.Pix[0], len(fb.Pix))
	if data == 0 {
		return 0, errors.New("cocoa: CFDataCreate failed (out of memory?)")
	}
	provider := darwin.CGDataProviderCreateWithCFData(data)
	darwin.CFRelease(data) // the provider keeps it
	if provider == 0 {
		return 0, errors.New("cocoa: CGDataProviderCreateWithCFData failed")
	}
	img := darwin.CGImageCreate(size.X, size.Y, 8, 32, fb.Stride, b.srgb,
		darwin.CGImageAlphaPremultipliedLast, provider, 0, false, darwin.CGRenderingIntentDefault)
	darwin.CGDataProviderRelease(provider) // the image keeps it
	if img == 0 {
		return 0, errors.New("cocoa: CGImageCreate failed")
	}
	return img, nil
}
