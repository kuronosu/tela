//go:build darwin

package cocoa

import (
	"errors"
	"image"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Present stores the damaged pixels of fb in the window's copy of the frame
// and makes the whole copy the layer's contents; see platform.Window. The
// window is shown by the first Present.
//
// The base backend copies the frame twice (into the retained copy, then
// into the CFData of the CGImage, which CoreGraphics keeps); an IOSurface
// shared with the layer, or the GPU backend's CAMetalLayer, removes the
// second copy later.
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
		}
	}
	for _, r := range rects {
		platform.CopyRGBA(w.back, fb, r)
	}
	w.updateInputMask(fb, rects) // a transparent window (transparent_darwin.go)

	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	img, err := w.b.newImage(w.back)
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
	darwin.Send(w.layer, sel.setContents, darwin.ID(img))
	darwin.SendClass(app.transaction, sel.commit)
	darwin.CGImageRelease(img) // the layer keeps it

	w.presented = true
	if !w.shown && w.visible {
		// Not before: the window would show unpainted.
		w.show()
		w.passIfFaint() // shown under a resting cursor (transparent_darwin.go)
	}
	return nil
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
