package core

import (
	"context"
	"errors"
	"fmt"
	"image"
	"slices"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// External textures (ADR-315): content that the application renders itself
// (a 3D view, a decoded video, a map) shown by an element of the tree.
//
// UseTexture makes a Texture, a handle that lives as long as its
// component; a primitive of style.KindTexture (h.Texture) shows it as a
// replaced element, like an Img whose image changes. The application pushes
// frames to the handle from any goroutine: pixels (SubmitImage), or buffers
// of its own memory imported once (Import, Submit; texture_buffer.go).
//
// # Frames
//
// The mailbox (texture_mailbox.go) decides which frames are shown: the
// latest finished one wins. A frame that becomes ready wakes the windows
// that show the texture; each one takes the latest ready frame at the start
// of the paint phase of its next frame (texture_tree.go), so a producer
// faster than the window costs the window nothing.
//
// A frame that takes a new image runs no build, style or layout: the box
// gets a new gfx.Image and is stamped as changed, so the list that paints it
// (a group of its own, record.go) is recorded again, and the damage finds
// the DrawImage command unequal (a new image ID), which damages the element
// alone. An element has no natural size from its frames (it is 300x150 like
// a canvas), so their size never lays anything out.
//
// # When a frame is let go
//
// The application does not write to the pixels of a frame before its
// Release. Tela reads them while the CPU rasterizer (or the GPU path) draws a
// display list that holds the frame's image, on the UI thread, which waits
// for its workers and for the device. Which lists may be drawn:
//
//   - a window's frame draws the list Tree.Frame composed just before it, in
//     the same event (Host.frame), and so does everything that draws a
//     tree's current list (layerState.cur) at another time: the GPU's
//     fallback to the CPU, FramePixels, telatest;
//   - the list of the last rasterized frame (layerState.ras) is only
//     compared with the next one, command by command, and images are compared
//     by identity: its pixels are never read again;
//   - the framebuffer, the snapshots damaged rectangles start from
//     (damagebase.go), the cached filtered backdrops, a drag image and the
//     GPU's targets hold pixels of their own, drawn from earlier lists;
//   - a layer's list that is not recorded again holds the image its box had
//     when it was recorded: a box that gets a new image stamps it, so its
//     layer is recorded again before the frame is composed.
//
// So a frame is let go once no tree's current list draws it: the tree that
// takes another one lets go of it at the end of the paint phase that
// composed a list without it, before that list is rasterized. Every tree
// that shows the texture holds the frame it draws (texView), and the
// mailbox holds the shown one, which an element mounted later shows. A
// window that gets no frames (hidden, minimized) holds the frame its list
// draws until it draws again or closes.
//
// Old lists may keep the gfx.Image of a frame let go, and through it the
// slice of its pixels, reachable (the last rasterized frame, a cached
// backdrop): nothing reads them, and the buffer of a copy may be used for
// the next one.
//
// An image has no mip levels and states its opacity (gfx.NewImageNoCopy), so
// no cache keyed by image ID grows with the frames.
//
// # Release
//
// Release functions run on a goroutine of the texture (letGo), one at a
// time, in the order their frames were let go, never under a lock and never
// inside a call of the application's.

// ErrTextureClosed is returned by the methods of a Texture whose component
// unmounted (or whose app quit).
var ErrTextureClosed = errors.New("tela: the texture is closed: the component that called UseTexture unmounted")

// ErrTextureUnsupported is wrapped by the errors of Texture.Import for
// memory, formats and modifiers that cannot be shown, and returned by the
// Fence of SyncFile where sync files do not exist.
var ErrTextureUnsupported = errors.New("tela: not supported by textures here")

// Texture is a handle on content that the application renders itself,
// returned by UseTexture and shown by h.Texture. Its methods are safe from
// any goroutine, except where they say otherwise.
type Texture struct {
	// owner is the tree of the component, and host its Host (nil for a
	// tree without one); ui is the id of the UI thread's goroutine.
	owner *Tree
	host  *Host
	ui    uint64

	mu     sync.Mutex
	mb     texMailbox
	closed bool
	// views are the trees that show the texture (texture_tree.go), primary
	// the one whose window paces NextFrame: the largest on screen, nil for
	// none.
	views   []*texView
	primary *texView
	// size is what Size reports; onResize the callback of OnResize, and
	// notified the size last given to it, once told.
	size     TextureSize
	onResize func(TextureSize)
	notified TextureSize
	told     bool
	// waiters are the NextFrame calls in progress; asked reports that the
	// primary window was asked for a frame for them.
	waiters []*texWaiter
	asked   bool
	// age and maxAge are the ages of the frames presented (Stats).
	age, maxAge time.Duration
	// bufs are the open imported buffers; spare are pixel buffers of copies
	// let go, for the next ones.
	bufs  map[*TextureBuffer]struct{}
	spare [][]byte
	// rq are the Release calls and buffers to free that wait for the
	// release goroutine, which runs while rqOn is set.
	rq   []texRelease
	rqOn bool
}

// texRelease is work of the release goroutine: the memory of a closed
// buffer to free, then a Release to call (either may be nil).
type texRelease struct {
	free    *TextureBuffer
	release func()
}

// texMaxSpare bounds the spare pixel buffers of a texture: the copies of
// the frames it can hold besides the one shown.
const texMaxSpare = 3

// UseTexture returns a Texture, the same on every build, that is closed
// when the component unmounts.
func UseTexture() *Texture {
	el, i := nextSlot(hookTexture)
	if x, ok := existingSlot[*Texture](el, i, "UseTexture"); ok {
		return x
	}
	t := el.tree
	x := &Texture{owner: t, ui: goid()}
	if t.wv != nil {
		x.host = t.wv.host
	}
	el.hooks = append(el.hooks, x)
	return x
}

// unmount closes the texture: its component unmounted.
func (x *Texture) unmount(*Tree) { x.close() }

// insValue is what the instrumentation shows of the hook slot: the size it
// is shown at and its frames.
func (x *Texture) insValue() string {
	s, st := x.Size(), x.Stats()
	if s == (TextureSize{}) {
		return fmt.Sprintf("not shown, %d frames shown, %d dropped", st.Shown, st.Dropped)
	}
	return fmt.Sprintf("%dx%d at x%g, %d frames shown, %d dropped", s.Width, s.Height, s.Scale, st.Shown, st.Dropped)
}

// TextureSource sets the texture of a style.KindTexture primitive.
func TextureSource(x *Texture) Prop {
	if x == nil {
		panic("tela: h.Texture(nil): pass the texture that tela.UseTexture returns, such as tex := tela.UseTexture() in the component's body and then h.Texture(tex)")
	}
	return Prop{name: "TextureSource", apply: func(d *desc) { d.mediaProps().tex = x }}
}

// checkTexture panics for a Texture primitive without its texture, or with
// children.
func checkTexture(kind style.Kind, d *desc) {
	has := d.media != nil && d.media.tex != nil
	if kind != style.KindTexture {
		if has {
			panic(fmt.Sprintf("core: TextureSource on <%v>: use Texture", kind))
		}
		return
	}
	if !has {
		panic("core: a texture primitive without TextureSource")
	}
	for _, c := range d.children {
		if c.d != nil {
			panic("tela: h.Texture takes no children: what it shows are the frames of its texture. Place an overlay beside it: Div(Class(\"relative\"), Texture(tex, Class(\"w-full\")), Div(Class(\"absolute inset-0\"), ...))")
		}
	}
}

// ---------------------------------------------------------------------
// Submitting.

// SubmitImage shows img, whose pixels are finished, from the next frame of
// the window on. It never blocks and never runs release: it queues the
// frame, wakes the windows that show the texture and returns. Until a frame
// of a window takes it, another frame submitted replaces it: see
// Texture.Submit for exactly what happens to a frame that arrives while
// another one is pending.
//
// The pixels are 8-bit sRGB with premultiplied alpha, the first row at the
// top, as in every image.RGBA. With a release they are not copied when
// img's rows are contiguous (Stride is 4 times the width, as
// image.NewRGBA makes them): the application must not write to them until
// release is called. Otherwise, and always with a nil release, they are
// copied before SubmitImage returns, and img is the application's again.
//
// release (nil for none) is called exactly once, also when SubmitImage
// returns an error: see TextureFrame.Release.
//
// It returns ErrTextureClosed once the component unmounted, and an error
// for an image that is nil, empty or shorter than its bounds.
func (x *Texture) SubmitImage(img *image.RGBA, release func()) error {
	w, h, pix, err := rgbaPixels(img)
	if err == nil && x.isClosed() {
		err = ErrTextureClosed // before a copy for nothing
	}
	if err != nil {
		x.reject(release, nil)
		return err
	}
	f := &texFrame{w: w, h: h, release: release}
	if row := 4 * w; release == nil || img.Stride != row {
		f.pix = x.pixels(row * h)
		for y := range h {
			copy(f.pix[y*row:(y+1)*row], pix[y*img.Stride:])
		}
		pix = f.pix
	}
	f.img = gfx.NewImageNoCopy(w, h, pix[:4*w*h:4*w*h], false)
	return x.submit(f, nil)
}

// rgbaPixels returns the size of img and its pixels from its first one on.
func rgbaPixels(img *image.RGBA) (w, h int, pix []byte, err error) {
	if img == nil {
		return 0, 0, nil, errors.New("tela: Texture.SubmitImage(nil): pass the *image.RGBA to show")
	}
	b := img.Rect
	w, h = b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0, nil, fmt.Errorf("tela: Texture.SubmitImage: an image of %dx%d pixels has nothing to show", w, h)
	}
	if img.Stride < 4*w || len(img.Pix) < (h-1)*img.Stride+4*w {
		return 0, 0, nil, fmt.Errorf("tela: Texture.SubmitImage: a %dx%d image.RGBA with a Stride of %d and %d bytes of Pix: its rows take 4 bytes per pixel", w, h, img.Stride, len(img.Pix))
	}
	return w, h, img.Pix, nil // Pix starts at the pixel Rect.Min
}

// TextureFrame describes a frame of Texture.Submit.
type TextureFrame struct {
	// Fence tells when the buffer's pixels are finished: Tela waits for it
	// on a goroutine of its own, and never reads the buffer before. Nil
	// means they are finished already.
	Fence Fence
	// Release, when not nil, is called exactly once for the Submit call it
	// was given to, whatever happens to the frame: replaced by another one
	// after it was shown, dropped without being shown, the Fence failed,
	// Submit returned an error, the component unmounted, the app quit. From
	// then on Tela does not read the buffer, with the CPU or the GPU, and
	// the application may draw into it again. The frame on screen stays
	// held until another one replaces it, in every window that shows it (a
	// hidden or minimized one when it draws again).
	//
	// A frame dropped before its Fence signals is released at once: the
	// buffer is then free of Tela, not of the application's own GPU work.
	//
	// The Release calls of a texture run on a goroutine of Tela's, one at a
	// time, in the order Tela let go of the frames, which is not always the
	// order they were submitted in: a frame dropped is released before an
	// older one that is still on screen. A Release never runs inside a call
	// of the application's (Submit, Close), and may call any method of the
	// texture, Submit included; the next Release waits until it returns.
	Release func()
}

// Submit shows the pixels of buf, a buffer of this texture made by Import,
// from the next frame of the window on. It never blocks and never runs
// Release: it queues the frame and returns.
//
// What happens to a frame that arrives while another one is pending: the
// latest finished frame wins. A texture holds at most the frame it shows,
// one that is ready, for the next frame of the window, and two that wait.
//
//   - A frame that is finished becomes the ready one: at once, or when its
//     wait ends. If another was ready and no window's frame had taken it,
//     that one is dropped: released without being shown. So are the waiting
//     ones submitted before it: a frame never replaces one submitted after
//     it.
//   - A frame with a Fence waits for it, and one whose memory is mapped
//     (Import) is then copied, which counts as waiting. If two already wait,
//     it replaces the newer of the two, which is dropped: the older one is
//     always left to finish, so a producer that submits faster than its
//     fences signal still shows frames.
//   - A frame whose Fence fails is dropped.
//   - The next frame of a window that shows the texture takes the ready one
//     and shows it. The frame shown before is released as soon as that
//     window stopped drawing it. While nothing new is ready, the shown frame
//     stays, and costs nothing.
//   - While no element shows the texture on screen (unmounted, display:
//     none, scrolled out of view, a hidden or minimized window) the same
//     rules hold and no window is woken: the latest finished frame stays
//     ready, the others are dropped as they are replaced, and the element
//     shows it as soon as it is on screen.
//
// Three buffers are enough for frames that are finished when they are
// submitted, five for frames that wait: the application then always finds
// one that is free, or about to be. A producer that must drop nothing paces
// itself with NextFrame. Stats counts the frames.
//
// It returns ErrTextureClosed once the component unmounted, and an error for
// a buffer that is closed, of another texture, or still held by an earlier
// frame (its Release has not been called). On an error the Release of f is
// still called, and its Fence is not waited for.
func (x *Texture) Submit(buf *TextureBuffer, f TextureFrame) error {
	switch {
	case buf == nil:
		x.reject(f.Release, f.Fence)
		return errors.New("tela: Texture.Submit(nil, ...): pass a buffer of Texture.Import, or pixels to SubmitImage")
	case buf.x != x:
		x.reject(f.Release, f.Fence)
		return errors.New("tela: Texture.Submit: the buffer was imported by another texture; import its memory with this one")
	}
	return x.submit(&texFrame{w: buf.w, h: buf.h, opaque: buf.opaque, buf: buf, release: f.Release}, f.Fence)
}

// submit queues frame f, which waits for fence (nil for none) and for the
// copy of its pixels when its buffer is mapped memory.
func (x *Texture) submit(f *texFrame, fence Fence) error {
	x.mu.Lock()
	var err error
	switch b := f.buf; {
	case x.closed:
		err = ErrTextureClosed
	case b != nil && b.closed:
		err = errors.New("tela: Texture.Submit: the buffer is closed")
	case b != nil && b.held:
		err = errors.New("tela: Texture.Submit: the buffer is still held by an earlier frame; draw into another one until its Release is called")
	}
	if err != nil {
		if f.pix != nil {
			x.recycle(f.pix)
		}
		x.mu.Unlock()
		x.reject(f.release, fence)
		return err
	}
	f.at, f.refs = time.Now(), 1
	if f.buf != nil {
		f.buf.held = true
		if fence == nil && f.buf.mem == nil {
			f.img = f.buf.image() // the device reads the memory itself
		}
	}
	if f.img != nil {
		x.dropAll(x.mb.submit(f, true))
		wake := x.readyViews()
		x.mu.Unlock()
		wakeViews(wake)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	x.dropAll(x.mb.submit(f, false))
	x.mu.Unlock()
	go x.await(ctx, f, fence)
	return nil
}

// await waits, on a goroutine of its own, until the pixels of waiting frame
// f are finished: its fence, then the copy of its buffer's mapped memory.
func (x *Texture) await(ctx context.Context, f *texFrame, fence Fence) {
	defer crashRecover(PanicGo) // logtee.go
	var err error
	if fence != nil {
		err = fence.Wait(ctx)
		closeFence(fence)
	}
	var pix []byte
	if err == nil && f.buf.mem != nil {
		x.mu.Lock()
		if f.state != texWaiting {
			x.mu.Unlock()
			return // dropped meanwhile: nothing to copy
		}
		f.reading = true
		x.mu.Unlock()
		pix = x.pixels(4 * f.w * f.h)
		err = f.buf.mem.read(pix)
	}
	x.finish(f, pix, err)
}

// finish ends the wait of frame f, whose pixels are pix (nil for a buffer
// the device reads), or which failed with err.
func (x *Texture) finish(f *texFrame, pix []byte, err error) {
	x.mu.Lock()
	f.reading = false
	f.cancel() // frees the context
	if f.state != texWaiting {
		// Dropped while it was copied, or while its fence was waited for.
		if pix != nil {
			x.recycle(pix)
		}
		if f.refs == 0 {
			x.letGo(f)
		}
		x.mu.Unlock()
		return
	}
	if err != nil {
		if pix != nil {
			x.recycle(pix)
		}
		if x.mb.fail(f) {
			x.unref(f)
		}
		x.mu.Unlock()
		x.owner.warnOnce("texture-frame:"+err.Error(), func() string {
			return fmt.Sprintf("tela: a frame of a texture was dropped: %v", err)
		})
		return
	}
	if pix != nil {
		f.pix = pix
		f.img = gfx.NewImageNoCopy(f.w, f.h, pix, f.opaque)
	} else {
		f.img = f.buf.image()
	}
	replaced, _ := x.mb.finish(f)
	x.dropAll(replaced)
	wake := x.readyViews()
	x.mu.Unlock()
	wakeViews(wake)
}

// reject queues the Release of a Submit call that returns an error, and
// frees its fence.
func (x *Texture) reject(release func(), fence Fence) {
	closeFence(fence)
	if release == nil {
		return
	}
	x.mu.Lock()
	x.queue(texRelease{release: release})
	x.mu.Unlock()
}

// dropAll lets go of frames the mailbox dropped. x.mu is held.
func (x *Texture) dropAll(fs []*texFrame) {
	for _, f := range fs {
		x.drop(f)
	}
	clear(fs) // the mailbox's scratch
}

// drop lets go of frame f, which the mailbox dropped: its wait is
// cancelled. x.mu is held.
func (x *Texture) drop(f *texFrame) {
	if f.cancel != nil {
		f.cancel()
	}
	x.unref(f)
}

// unref forgets one holder of frame f: without any, and once its buffer is
// not being copied, it is let go. x.mu is held.
func (x *Texture) unref(f *texFrame) {
	if f.refs--; f.refs == 0 && !f.reading {
		x.letGo(f)
	}
}

// letGo ends frame f, which nothing reads any more: its buffer and its
// pixels may be used again, and its Release is queued. x.mu is held.
func (x *Texture) letGo(f *texFrame) {
	if f.done {
		return
	}
	f.done = true
	if f.pix != nil {
		x.recycle(f.pix)
		f.pix = nil
	}
	r := texRelease{release: f.release}
	if b := f.buf; b != nil {
		b.held = false
		if b.closed {
			r.free = b
		}
	}
	f.release = nil
	x.queue(r)
}

// queue adds work for the release goroutine, starting it when none runs.
// x.mu is held.
func (x *Texture) queue(r texRelease) {
	if r.free == nil && r.release == nil {
		return
	}
	x.rq = append(x.rq, r)
	if !x.rqOn {
		x.rqOn = true
		go x.runReleases()
	}
}

// runReleases is the release goroutine of a texture: it runs what is
// queued, in order, and ends when nothing is left. Its posts go to the
// texture's app, as those of a UseGo goroutine do (ADR-292).
func (x *Texture) runReleases() {
	defer crashRecover(PanicGo) // logtee.go
	run := func(context.Context) {
		for {
			x.mu.Lock()
			if len(x.rq) == 0 {
				x.rqOn = false
				x.mu.Unlock()
				return
			}
			r := x.rq[0]
			x.rq[0] = texRelease{}
			x.rq = x.rq[1:]
			if len(x.rq) == 0 {
				x.rq = nil
			}
			x.mu.Unlock()
			if r.free != nil {
				r.free.free()
			}
			if r.release != nil {
				r.release()
			}
		}
	}
	if posts.n.Load() > 1 {
		goWithApp(context.Background(), x.app(), run)
		return
	}
	run(context.Background())
}

// app returns the tree of the texture's app. Any goroutine.
func (x *Texture) app() *Tree {
	if x.host != nil {
		return x.host.tree
	}
	return x.owner
}

// pixels returns a buffer of n bytes for a copy, a spare one when there is
// one of that size.
func (x *Texture) pixels(n int) []byte {
	x.mu.Lock()
	for i := len(x.spare) - 1; i >= 0; i-- {
		if b := x.spare[i]; len(b) == n {
			x.spare = slices.Delete(x.spare, i, i+1)
			x.mu.Unlock()
			return b
		}
	}
	x.mu.Unlock()
	return make([]byte, n)
}

// recycle keeps the pixel buffer b for the next copy, in the place of the
// spares of another size (the texture was resized). x.mu is held.
func (x *Texture) recycle(b []byte) {
	if x.closed {
		return
	}
	x.spare = slices.DeleteFunc(x.spare, func(s []byte) bool { return len(s) != len(b) })
	if len(x.spare) < texMaxSpare {
		x.spare = append(x.spare, b)
	}
}

// spareBytes returns what the spare pixel buffers hold, and trim releases
// them: the windows that show the texture rest.
func (x *Texture) spareBytes() int {
	x.mu.Lock()
	defer x.mu.Unlock()
	n := 0
	for _, b := range x.spare {
		n += len(b)
	}
	return n
}

func (x *Texture) trim() {
	x.mu.Lock()
	x.spare = nil
	x.mu.Unlock()
}

func (x *Texture) isClosed() bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.closed
}

// close ends the texture: its frames are let go as soon as the windows that
// show it stop drawing them, and its buffers are closed. UI thread.
func (x *Texture) close() {
	x.mu.Lock()
	if x.closed {
		x.mu.Unlock()
		return
	}
	x.closed = true
	dropped, shown := x.mb.close()
	x.dropAll(dropped)
	if shown != nil {
		x.unref(shown)
	}
	for _, w := range x.waiters {
		close(w.ch)
	}
	x.waiters = nil
	x.onResize = nil
	x.size, x.primary = TextureSize{}, nil
	x.spare = nil
	for b := range x.bufs {
		x.closeBuffer(b)
	}
	views := slices.Clone(x.views)
	x.mu.Unlock()
	for _, v := range views {
		v.t.requestRepaint() // an element that still shows it stops drawing it
	}
}

// ---------------------------------------------------------------------
// Sizes and stats.

// TextureSize is the size a texture is shown at.
type TextureSize struct {
	// Width and Height are the content box of the element in device
	// pixels, snapped to them as it is painted: a frame of this size is
	// drawn pixel for pixel. Transforms do not count.
	Width, Height int
	// Scale is the display scale of its window: device pixels per logical
	// pixel.
	Scale float32
}

// Size returns the size the texture is shown at: of the largest element
// that shows it, when several do, and the zero TextureSize while none is
// laid out (none mounted, or in display: none) or its content box is empty.
// Scrolling the element out of view, and hiding or minimizing its window, do
// not change it. Safe from any goroutine; OnResize tells of changes.
func (x *Texture) Size() TextureSize {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.size
}

// OnResize makes f the function called when Size changes, in the place of
// the one set before; nil for none. f runs on the UI thread, after layout
// and before the frame is painted, so a frame it submits without a fence
// (SubmitImage) is shown in that same frame; until a frame of the new size
// arrives, the last one is stretched to the element by its object-fit. It is
// also called once with the current size, if the texture is shown, after it
// is set: in the frame in progress, or soon after. It is called with the
// zero TextureSize when the texture is not shown any more, and not called
// when nothing changed. A change of the display scale and a move to another
// window are changes of the size.
//
// Call it from an effect (tela.UseMount), or from a goroutine: in the
// component's body it would set another f, and call it, on every build.
func (x *Texture) OnResize(f func(TextureSize)) {
	x.mu.Lock()
	if x.closed {
		x.mu.Unlock()
		return
	}
	x.onResize, x.told = f, false
	due := f != nil && x.size != TextureSize{}
	x.mu.Unlock()
	if due {
		x.owner.enqueue(update{post: x.tellSize})
	}
}

// tellSize calls the callback of OnResize when the size changed since it
// was last called. UI thread.
func (x *Texture) tellSize() {
	x.mu.Lock()
	f, size := x.onResize, x.size
	due := f != nil && (x.told && size != x.notified || !x.told && size != TextureSize{})
	if due {
		x.notified, x.told = size, true
	}
	x.mu.Unlock()
	if due {
		f(size)
	}
}

// TextureStats counts the frames of a texture.
type TextureStats struct {
	// Submitted counts the frames queued (the calls of SubmitImage and
	// Submit without an error). Shown counts those a window's frame took,
	// and Dropped those replaced before one did, or whose Fence failed.
	// What is left, at most three frames, waits or is ready.
	Submitted, Shown, Dropped uint64
	// Age is how long before the last new frame was presented to the
	// display server it had been submitted (the wait for its Fence
	// included), and MaxAge the largest one: what Tela adds to the latency
	// of the application's content, without what the display server and the
	// screen add.
	Age, MaxAge time.Duration
}

// Stats returns the counters of the texture's frames.
func (x *Texture) Stats() TextureStats {
	x.mu.Lock()
	defer x.mu.Unlock()
	return TextureStats{
		Submitted: x.mb.submitted, Shown: x.mb.shownN, Dropped: x.mb.droppedN,
		Age: x.age, MaxAge: x.maxAge,
	}
}

// ---------------------------------------------------------------------
// Pacing.

// FrameTime tells when a window's frame is shown (Texture.NextFrame).
type FrameTime struct {
	// Time is the window's frame clock at the frame, the clock Tela's own
	// animations advance with: the best estimate of when the frame will be
	// on screen where the platform knows it, else when the frame started.
	// The difference of the Times of two frames is the time to advance an
	// animation by.
	//
	// It carries a monotonic clock reading. Content rendered for a frame
	// after NextFrame returns is usually shown by the frame after it, so
	// time.Until(ft.Time.Add(ft.Refresh)) is how long is left to render it.
	Time time.Time
	// Refresh is the interval between the frames of the window, 0 when it
	// is not known.
	Refresh time.Duration
}

// texWaiter is a NextFrame call in progress. ch gets the frame, or is
// closed when the texture is.
type texWaiter struct{ ch chan FrameTime }

// NextFrame blocks the calling goroutine until the window that shows the
// texture starts its next frame, and asks that window for one: a loop of
// NextFrame, rendering and submitting runs at the window's frame rate and
// drops nothing, and the window sleeps once the loop stops. Content
// submitted soon after it returns may still be shown by that frame, and
// otherwise by the next one.
//
// While no element shows the texture on screen (unmounted, display: none,
// scrolled out of view, a hidden or minimized window) it blocks, and costs
// nothing. With elements in several windows, the window of the largest one
// on screen paces it.
//
// It returns the error of ctx once ctx is done, and ErrTextureClosed when
// the component unmounts or the app quits. It panics on the UI thread,
// whose frame it waits for: call it on a goroutine (tela.UseGo).
func (x *Texture) NextFrame(ctx context.Context) (FrameTime, error) {
	if goid() == x.ui {
		panic("tela: Texture.NextFrame called on the UI thread, which would wait for itself: the frame it waits for runs there. Call it on a goroutine, such as tela.UseGo(func(ctx context.Context) { for { ft, err := tex.NextFrame(ctx); if err != nil { return }; render(ft) } }, tela.Once)")
	}
	if err := ctx.Err(); err != nil {
		return FrameTime{}, err
	}
	w := &texWaiter{ch: make(chan FrameTime, 1)}
	x.mu.Lock()
	if x.closed {
		x.mu.Unlock()
		return FrameTime{}, ErrTextureClosed
	}
	x.waiters = append(x.waiters, w)
	v := x.askable()
	x.mu.Unlock()
	if v != nil {
		v.t.enqueue(update{post: v.ask})
	}
	select {
	case ft, ok := <-w.ch:
		if !ok {
			return FrameTime{}, ErrTextureClosed
		}
		return ft, nil
	case <-ctx.Done():
		x.mu.Lock()
		if i := slices.Index(x.waiters, w); i >= 0 {
			x.waiters = slices.Delete(x.waiters, i, i+1)
		}
		x.mu.Unlock()
		return FrameTime{}, ctx.Err()
	}
}

// askable returns the view whose window must be asked for a frame for the
// waiters, noting that it was; nil when none waits, nothing is on screen or
// it was asked already. x.mu is held.
func (x *Texture) askable() *texView {
	if len(x.waiters) == 0 || x.asked || x.primary == nil {
		return nil
	}
	x.asked = true
	return x.primary
}

// ---------------------------------------------------------------------
// The device.

// TextureFormat is a pixel format of imported memory: a DRM fourcc and a
// DRM format modifier.
type TextureFormat struct {
	// Fourcc is the DRM fourcc, such as 0x34325241 (AR24, DRM_FORMAT_ARGB8888).
	Fourcc uint32
	// Modifier is the DRM format modifier; 0 is linear.
	Modifier uint64
}

// TextureDevice describes the device that draws the app's windows, so that
// the application renders on the same one, with a format it imports. The
// identities a device does not have are zero.
type TextureDevice struct {
	// GPU reports that Tela draws with a GPU and imports its memory: an
	// imported buffer is then sampled without a copy. Without it, the pixels
	// of an imported buffer are copied for every frame.
	GPU bool
	// Name is the device's name, and VendorID and DeviceID its PCI IDs.
	Name               string
	VendorID, DeviceID uint32
	// RenderNode is the path of the DRM render node on Linux
	// ("/dev/dri/renderD128"), and RenderMajor and RenderMinor its device
	// numbers.
	RenderNode               string
	RenderMajor, RenderMinor uint32
	// UUID is the Vulkan device UUID
	// (VkPhysicalDeviceIDProperties.deviceUUID), and LUID the adapter's
	// locally unique identifier on Windows (its deviceLUID).
	UUID [16]byte
	LUID [8]byte
	// MetalRegistryID is the registry ID of the Metal device on macOS.
	MetalRegistryID uint64
	// Formats are the formats Texture.Import takes for memory allocated on
	// this device, the preferred ones first, and ForeignFormats those for
	// memory of any other one.
	Formats, ForeignFormats []TextureFormat
}

// Device describes the device that draws the app's windows. Without a GPU
// (a default build, or the CPU rasterizer chosen or fallen back to) it has
// no identity, and its formats, on Linux, are those Import maps and copies:
// AR24, XR24, AB24 and XB24 in linear memory. The GPU opens with the first
// window: call it in an effect or a goroutine, not in a build.
func (x *Texture) Device() TextureDevice {
	if imp := x.importer(); imp != nil {
		return imp.textureDevice()
	}
	return TextureDevice{Formats: mappedFormats(), ForeignFormats: mappedFormats()}
}
