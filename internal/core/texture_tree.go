package core

import (
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
)

// Textures in a tree (ADR-315; texture.go has the design).
//
// A tree keeps a texView for each texture its elements show. In a Frame:
//
//   - after layout, syncTextureSizes measures the content boxes and calls
//     OnResize;
//   - before the paint phase, bindTextures takes the ready frame of each
//     texture, gives its image to the boxes and stamps them, so that the
//     lists that paint them are recorded again (record.go);
//   - after it, releaseTextures lets go of the frames the composed list no
//     longer draws;
//   - at its end, texturesAfterFrame looks at which textures are on screen.
//
// A window that gets no frames (paused.go) commits without taking frames: it
// presents nothing.
//
// # Waking
//
// A texture is on screen in a tree when one of its boxes is displayed and
// visible, meets the window through the overflow clips around it, and the
// window gets frames. Only then does a ready frame wake the tree (once until
// its next Frame: texView.woken), and NextFrame ask for a frame. What can
// put a texture on screen is a Frame (a mount, a layout, a scroll) or a
// window that is shown again: both look for a ready frame and for waiters
// then (texView.setOn), so nothing has to poll.

// texView is a texture shown by the elements of a tree. boxes, cur, old and
// visible belong to the UI thread; on, woken and size to the texture's
// lock.
type texView struct {
	x     *Texture
	t     *Tree
	boxes []*Box
	// cur is the frame the boxes draw, nil for none, and old the frames
	// that lists composed before may draw: the view holds them all
	// (texFrame.refs).
	cur *texFrame
	old []*texFrame
	// visible: a box is displayed and meets the window, as of the last
	// frame; on: and the window gets frames.
	visible, on bool
	// woken: the tree was woken, or asked to repaint, for a ready frame
	// it has not taken.
	woken bool
	// size is the size of the largest box.
	size TextureSize
	// wake and ask are its functions for Tree.enqueue, made once.
	wake, ask func()
}

// texTree is the state of a Tree's textures (UI thread).
type texTree struct {
	views []*texView
	// frames is the frame count at the last look at what is on screen.
	frames int
	// ask: a frame was asked for (askFrame) while one ran.
	ask bool
}

// updateTexture makes box b of a Texture element show its node's texture,
// or the new one of a later build.
func (t *Tree) updateTexture(el *Element) {
	b := el.box
	x := el.node.d.media.tex
	if b.med != nil && b.med.tex != nil {
		if b.med.tex.x == x {
			return
		}
		t.dropTextureBox(b)
	}
	v := t.textureView(x)
	v.boxes = append(v.boxes, b)
	b.med = &boxMedia{tex: v}
	b.img = nil // bindTextures gives it the frame
	t.markMediaPaint(b)
}

// textureView returns the view of x in t, a new one at its first box.
func (t *Tree) textureView(x *Texture) *texView {
	for _, v := range t.tex.views {
		if v.x == x {
			return v
		}
	}
	v := &texView{x: x, t: t}
	v.wake = func() {
		x.mu.Lock()
		due := v.due()
		v.woken = due
		x.mu.Unlock()
		if due {
			t.requestRepaint()
		}
	}
	v.ask = t.askFrame
	t.tex.views = append(t.tex.views, v)
	t.tex.frames = -1
	x.mu.Lock()
	x.views = append(x.views, v)
	x.mu.Unlock()
	return v
}

// dropTextureBox forgets box b of a texture: it unmounts, or shows another
// texture. The next paint lets go of the frame when it was the last one.
func (t *Tree) dropTextureBox(b *Box) {
	v := b.med.tex
	if i := slices.Index(v.boxes, b); i >= 0 {
		v.boxes = slices.Delete(v.boxes, i, i+1)
	}
	b.med, b.img = nil, nil
}

// bindTextures gives the boxes of the textures the frames to paint: with
// take, the ready ones, which become the shown ones. It stamps the boxes
// whose image changed, so that the lists that paint them are recorded again
// by the paint that follows.
func (t *Tree) bindTextures(take bool) {
	L := &t.lay
	for _, v := range t.tex.views {
		live := false
		for _, b := range v.boxes {
			if t.inTree(b) {
				live = true
				break
			}
		}
		x := v.x
		x.mu.Lock()
		var cur *texFrame
		if live && !x.closed {
			if take {
				if _, old := x.mb.take(); old != nil {
					x.unref(old)
				}
				v.woken = false
			}
			cur = x.mb.shown
		}
		if cur != v.cur {
			if v.cur != nil {
				v.old = append(v.old, v.cur)
			}
			if v.cur = cur; cur != nil {
				cur.refs++
			}
		}
		x.mu.Unlock()
		var img *gfx.Image
		if cur != nil {
			img = cur.img
		}
		for _, b := range v.boxes {
			if b.img == img && (b.rinN == 0 || b.rin.img == img) {
				continue
			}
			b.img = img
			if b.rinN > 0 {
				b.rin.img = img
				if b.rinAt != L.epoch {
					b.rinAt = L.epoch
					L.nchg++
				}
			}
		}
	}
}

// releaseTextures lets go of the frames that the list just composed does not
// draw, and forgets the textures no element shows any more.
func (t *Tree) releaseTextures() {
	gone := false
	for _, v := range t.tex.views {
		v.release()
		gone = gone || len(v.boxes) == 0
	}
	if gone {
		t.forgetTextures()
	}
}

// release lets go of the frames of earlier lists.
func (v *texView) release() {
	if len(v.old) == 0 {
		return
	}
	v.x.mu.Lock()
	for _, f := range v.old {
		v.x.unref(f)
	}
	v.x.mu.Unlock()
	clear(v.old)
	v.old = v.old[:0]
}

// forgetTextures removes the views without boxes, which hold no frame.
func (t *Tree) forgetTextures() {
	var gone []*texView
	t.tex.views = slices.DeleteFunc(t.tex.views, func(v *texView) bool {
		if len(v.boxes) > 0 {
			return false
		}
		gone = append(gone, v)
		return true
	})
	for _, v := range gone {
		v.setSize(TextureSize{})
		v.setOn(false)
		x := v.x
		x.mu.Lock()
		if i := slices.Index(x.views, v); i >= 0 {
			x.views = slices.Delete(x.views, i, i+1)
		}
		x.mu.Unlock()
	}
}

// dropTextures lets go of every frame the tree's lists draw: it unmounted,
// and nothing draws its last list any more.
func (t *Tree) dropTextures() {
	for _, v := range t.tex.views {
		for _, b := range slices.Clone(v.boxes) {
			t.dropTextureBox(b)
		}
		if v.cur != nil {
			v.old = append(v.old, v.cur)
			v.cur = nil
		}
		v.release()
	}
	t.forgetTextures()
}

// ---------------------------------------------------------------------
// Sizes.

// syncTextureSizes measures the textures after a layout, in the frame and
// before its paint, and calls OnResize for the ones whose size changed.
func (t *Tree) syncTextureSizes() {
	for i := 0; i < len(t.tex.views); i++ { // OnResize runs the app's code
		v := t.tex.views[i]
		var size TextureSize
		for _, b := range v.boxes {
			if s := t.textureBoxSize(b); s.area() > size.area() {
				size = s
			}
		}
		v.setSize(size)
	}
}

// area returns the area of s in device pixels.
func (s TextureSize) area() int { return s.Width * s.Height }

// textureBoxSize returns the content box of b in device pixels, snapped as
// it will be painted (painter.contentRRect); zero when it is not laid out or
// empty. The boxes may not be placed yet.
func (t *Tree) textureBoxSize(b *Box) TextureSize {
	if b.cs == nil || !t.inTree(b) {
		return TextureSize{}
	}
	p := painter{t: t, scale: float64(t.scale)}
	l := b.ld.Layout()
	g := p.geomOf(b.cs, gfx.RectFromOriginSize(layoutOrigin(b), l.Size))
	r := p.contentRRectOf(&g, l.Padding).Rect
	s := TextureSize{
		Width:  int(gfx.RoundDevice(float64(r.X1)*p.scale) - gfx.RoundDevice(float64(r.X0)*p.scale)),
		Height: int(gfx.RoundDevice(float64(r.Y1)*p.scale) - gfx.RoundDevice(float64(r.Y0)*p.scale)),
		Scale:  t.scale,
	}
	if s.Width <= 0 || s.Height <= 0 {
		return TextureSize{}
	}
	return s
}

// layoutOrigin returns the position of b as placing it computes it
// (Box.u): the layout positions added from the root down.
func layoutOrigin(b *Box) gfx.Point {
	o := b.ld.Layout().Location
	if b.parent != nil {
		o = o.Add(layoutOrigin(b.parent))
	}
	return o
}

// setSize records the size of v's largest box, and calls OnResize when the
// size of the texture changed (or was not told yet).
func (v *texView) setSize(s TextureSize) {
	x := v.x
	x.mu.Lock()
	v.size = s
	var size TextureSize
	if !x.closed {
		for _, o := range x.views {
			if o.size.area() > size.area() {
				size = o.size
			}
		}
	}
	x.size = size
	x.mu.Unlock()
	x.tellSize()
}

// ---------------------------------------------------------------------
// On screen.

// texturesAfterFrame ends a Frame for the textures, outside it: the frame
// asked for while it ran is requested, and after a paint the textures on
// screen are found again.
func (t *Tree) texturesAfterFrame() {
	s := &t.tex
	if s.ask {
		s.ask = false
		t.askFrame()
	}
	if len(s.views) == 0 || s.frames == t.stats.Frames {
		return
	}
	s.frames = t.stats.Frames
	for _, v := range s.views {
		v.visible = slices.ContainsFunc(v.boxes, t.textureBoxVisible)
	}
	t.syncTexturesOn()
}

// textureBoxVisible reports whether box b can be seen in the window, as of
// the last place: it is displayed and visible, and its border box meets the
// window through the overflow clips around it.
func (t *Tree) textureBoxVisible(b *Box) bool {
	if b.pcs == nil || b.pcs.Visibility != style.VisibilityVisible || b.hiddenAtom || b.ls.Display == layout.DisplayNone || !t.inTree(b) {
		return false
	}
	r := t.boxWindowRect(b)
	return !r.Intersect(t.clipOf(b, gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H})).Empty()
}

// syncTexturesOn tells the textures of t whether they are on screen: after a
// frame, and when the window is hidden, minimized or shown again (Host).
func (t *Tree) syncTexturesOn() {
	if len(t.tex.views) == 0 {
		return
	}
	paused := t.wv != nil && t.wv.paused()
	for _, v := range t.tex.views {
		v.setOn(v.visible && !paused)
	}
}

// setOn records whether v is on screen, chooses the view that paces
// NextFrame, and asks for what a texture on screen waits for: a paint for a
// ready frame, or for one that another window took meanwhile, and a frame
// for the waiters. UI thread.
func (v *texView) setOn(on bool) {
	x := v.x
	x.mu.Lock()
	v.on = on
	repaint := v.due()
	if repaint {
		v.woken = true
	}
	var best *texView
	if !x.closed {
		for _, o := range x.views {
			if o.on && (best == nil || o.size.area() > best.size.area()) {
				best = o
			}
		}
	}
	if best != x.primary {
		x.primary, x.asked = best, false
	}
	ask := x.askable()
	x.mu.Unlock()
	if repaint {
		v.t.requestRepaint()
	}
	if ask != nil {
		ask.ask() // the trees of an app share the UI thread
	}
}

// due reports whether v, on screen, has another frame to show than the one
// its list draws. The texture's lock is held; UI thread.
func (v *texView) due() bool {
	m := &v.x.mb
	return v.on && !v.x.closed && (m.ready != nil || m.shown != v.cur)
}

// readyViews returns the views to wake for a frame that became ready: those
// on screen that were not woken since they last took one. x.mu is held;
// wakeViews wakes them without it.
func (x *Texture) readyViews() []*texView {
	var out []*texView
	for _, v := range x.views {
		if v.on && !v.woken {
			v.woken = true
			out = append(out, v)
		}
	}
	return out
}

// wakeViews wakes the trees of views: their next frame takes the ready
// frame. Any goroutine.
func wakeViews(views []*texView) {
	for _, v := range views {
		v.t.enqueue(update{post: v.wake})
	}
}

// askFrame asks for a frame of t's window without any work for it
// (NextFrame). UI thread.
func (t *Tree) askFrame() {
	if t.inFrame {
		t.tex.ask = true // afterFrame asks
		return
	}
	if t.asked || t.onDirty == nil {
		return
	}
	t.asked = true
	t.onDirty()
}

// startTextureFrame tells the NextFrame calls that wait for t's window that
// a frame of it starts, at ft.
func (t *Tree) startTextureFrame(ft FrameTime) {
	hold := t.holdTextureFrames() // tests only
	for _, v := range t.tex.views {
		x := v.x
		x.mu.Lock()
		if x.primary == v {
			x.asked = false
			if !hold { // a test's frame that does not pace the producers
				for _, w := range x.waiters {
					w.ch <- ft // buffered: each one gets one frame
				}
				clear(x.waiters)
				x.waiters = x.waiters[:0]
			}
		}
		x.mu.Unlock()
	}
}

// textureSpareBytes returns what the textures of t keep only for the next
// frames, and trimTextures releases it (Tree.TrimRaster).
func (t *Tree) textureSpareBytes() int {
	n := 0
	for _, v := range t.tex.views {
		n += v.x.spareBytes()
	}
	return n
}

func (t *Tree) trimTextures() {
	for _, v := range t.tex.views {
		v.x.trim()
	}
}

// texturesPresented notes that the window presented the tree's frame: the
// ages of the frames it shows for the first time.
func (t *Tree) texturesPresented() {
	for _, v := range t.tex.views {
		f := v.cur
		if f == nil {
			continue
		}
		x := v.x
		x.mu.Lock()
		if !f.presented {
			f.presented = true
			x.age = time.Since(f.at)
			x.maxAge = max(x.maxAge, x.age)
		}
		x.mu.Unlock()
	}
}
