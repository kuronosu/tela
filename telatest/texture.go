package telatest

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Textures (ADR-318): an element that shows a tela.Texture (h.Texture) is
// found by a locator like any other, and the test acts as the application's
// producer: it submits frames, reads the size the element asks for and the
// counters, steps a producer that loops on Texture.NextFrame frame by
// frame, and, with a [FakeTextureDevice], plays the GPU that imports the
// application's dmabufs.

// texState is what the textures of an app report to the test: the Release
// functions the app gave (and the ones that ran), and the frames that let
// producers go.
type texState struct {
	// given counts the Release functions given to Submit and SubmitImage,
	// released the ones that were called, and double the calls beyond the
	// first of a function.
	given, released, double atomic.Int64
	// releasing is above 0 while the clock moves (App.Advance, StepTexture,
	// the waits): the frames then pace the goroutines in NextFrame.
	releasing atomic.Int32
	// hooks is what the Host calls when the textures are used.
	hooks *core.TextureHooks
}

func newTexState() *texState {
	s := &texState{}
	s.hooks = &core.TextureHooks{
		Release: func(release func()) func() {
			s.given.Add(1)
			var calls atomic.Int32
			return func() {
				if calls.Add(1) == 1 {
					s.released.Add(1)
				} else {
					s.double.Add(1)
				}
				release()
			}
		},
		HoldFrames: func() bool { return s.releasing.Load() == 0 },
	}
	return s
}

// setupTextures attaches the hooks of the app's textures and its fake
// device, before the first frame.
func (a *App) setupTextures() {
	a.host.SetTextureHooks(a.sh.tex.hooks)
	if d := a.cfg.texDevice; d != nil {
		a.host.SetFakeTextureDevice(d.Device, d.importMemory)
	}
}

// texturesInTransit reports whether a goroutine of Tela's is finishing a
// frame: its Fence ended, or its memory is being copied, and it will become
// the ready frame or be dropped. Settling waits for it, in real time. A
// frame whose Fence has not ended is not in transit: the test (or the
// application's GPU) ends that wait. A [FakeFence] says that it ended as
// soon as Signal or Fail returns; the Wait of another fence that returns
// in the same instant may be missed by the settling that looks at it.
func (a *App) texturesInTransit() bool {
	return a.sh.tex.hooks.InTransit()
}

// texturesParked reports whether goroutines wait in Texture.NextFrame for a
// texture that a window of the app paces.
func (a *App) texturesParked() bool {
	for _, w := range a.host.Windows() {
		if a.host.TreeOf(w).TexturesParked() {
			return true
		}
	}
	return false
}

// askTextureFrames asks the windows for the frames that the goroutines in
// NextFrame wait for.
func (a *App) askTextureFrames() {
	for _, w := range a.host.Windows() {
		a.host.TreeOf(w).AskTextureFrames()
	}
}

// checkReleased fails the test when a Release function was called twice or
// never, once the app unmounted: Tela promises exactly one call for every
// frame given to Submit and SubmitImage.
func (a *App) checkReleased() {
	s := a.sh.tex
	deadline := time.Now().Add(a.cfg.timeout)
	for s.given.Load() > s.released.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond) // the release goroutine runs in real time
	}
	if n := s.given.Load() - s.released.Load(); n > 0 {
		a.tb.Errorf("telatest: %d of the %d Release functions given to Texture.Submit and SubmitImage were not called after the app unmounted: Tela calls the Release of every frame once, whatever happens to the frame (this is a bug of Tela)", n, s.given.Load())
	}
	if n := s.double.Load(); n > 0 {
		a.tb.Errorf("telatest: %d Release function(s) given to Texture.Submit and SubmitImage were called more than once: Tela calls the Release of every frame exactly once (this is a bug of Tela)", n)
	}
}

// ---------------------------------------------------------------------
// The texture of an element.

// Texture returns the tela.Texture that the element l locates shows (the
// one UseTexture returned to the component that called h.Texture), waiting
// for the element as an action does. It fails the test when the element is
// not a texture, or several match. Use it for what the other methods do not
// cover: the handle takes every call of the application's own, such as
// Submit with a Fence, Import or Device.
//
// A texture is a replaced element that says nothing to accessibility
// unless it has a name: find it with [ByTestID] (h.TestID), or by its
// AriaLabel with [ByRole]("img", name).
//
//	app := telatest.Mount(t, Globe())
//	tex := app.Texture(telatest.ByTestID("globe"))
func (a *App) Texture(l Locator) *tela.Texture {
	a.tb.Helper()
	n := a.one(l)
	x := a.tree.NodeTexture(n.ID)
	if x == nil {
		a.fatalf("%s is not a texture: it matches %s, and Texture takes the element that h.Texture(tex, ...) makes. Locate it with telatest.ByTestID, or by its AriaLabel with telatest.ByRole(\"img\", name)", l, a.describeMatch(n))
	}
	return x
}

// SubmitTexture shows img in the texture that l locates, as the
// application's producer would with SubmitImage: the pixels are copied, so
// img is the test's again when it returns, and the frame is shown, the app
// settled, when it returns. It fails the test when the texture is closed or
// img is not an image of at least one pixel. Size it with [App.TextureSize]
// to see it pixel for pixel; any other size is fitted by the element's
// object-fit.
//
//	app.SubmitTexture(telatest.ByTestID("globe"), frame)
//	app.Golden("globe.png")
//
// Frames of the application's own producer and the ones it submits do not
// order themselves: an app with a producer that loops on NextFrame is
// stepped with [App.StepTexture].
func (a *App) SubmitTexture(l Locator, img *image.RGBA) {
	a.tb.Helper()
	x := a.Texture(l)
	if err := x.SubmitImage(img, nil); err != nil {
		a.fatalf("SubmitTexture(%s): %v", l, err)
	}
	a.settleAfter()
}

// TextureSize returns the size, in device pixels, and the scale that the
// texture of l is shown at: what the element asks its producer to render
// (tela.Texture.Size), after settling the app. It is the zero value while
// no element is laid out.
func (a *App) TextureSize(l Locator) tela.TextureSize {
	a.tb.Helper()
	x := a.Texture(l)
	a.settleQuery()
	return x.Size()
}

// TextureStats returns the counters of the texture of l: the frames
// submitted, shown and dropped, after settling the app. The ages are real
// time, which the test does not control: do not assert on them.
func (a *App) TextureStats(l Locator) tela.TextureStats {
	a.tb.Helper()
	x := a.Texture(l)
	a.settleQuery()
	return x.Stats()
}

// StepTexture runs one frame of the window that shows the texture of l for
// the producers that loop on its NextFrame, and waits until they came back:
// each one rendered and submitted what it renders for that frame, and waits
// in NextFrame for the next one. The next frame of the window then shows
// what they submitted, which is what the screenshot after StepTexture has. A
// loop of NextFrame is thus advanced frame by frame, deterministically, with
// the app's clock.
//
// Settling never lets a NextFrame loop go: the frames it runs leave the
// producers waiting, so an app with such a loop settles as any other, and
// [App.Pending] names it ([PendingNextFrame]). [App.Advance] and the waits
// of the expectations (frames of the clock) do let them go, as often as the
// window runs a frame, so what they show depends on how fast the producers
// run; step them with StepTexture to compare pixels.
//
// A frame of the window lets go every producer of the textures it shows, not
// only this one's; StepTexture waits for this texture's. It fails the test
// when no goroutine waits in NextFrame (the texture is not shown, or its
// producer has not started: give it a UseGo), or when they do not come back
// within the [Timeout] of real time: a producer that returns instead of
// waiting again (it ran its last frame) is stepped with [App.Advance].
//
//	app := telatest.Mount(t, Game())
//	for range 3 {
//	    app.StepTexture(telatest.ByTestID("screen"))
//	}
//	app.Golden("game-3.png")
func (a *App) StepTexture(l Locator) {
	a.tb.Helper()
	x := a.Texture(l)
	a.settle()
	n := a.waitParked(x, 1, func() string {
		return fmt.Sprintf("no goroutine waits in Texture.NextFrame for the texture of %s: StepTexture runs a frame for the producers that loop on it. The texture must be shown (an element that is displayed, in view) and its producer started, such as tela.UseGo(func(ctx context.Context) { for { ft, err := tex.NextFrame(ctx); if err != nil { return }; render(ft) } }, tela.Once)", l)
	})
	a.askTextureFrames()
	d := time.Duration(0)
	if at, ok := a.backend.NextFrameTime(); ok {
		d = max(at-a.backend.Now(), 0)
	}
	a.advance(d)
	a.waitParked(x, n, func() string {
		return fmt.Sprintf("the %d goroutine(s) of the texture of %s that were waiting in Texture.NextFrame did not wait in it again: a producer that ran its last frame returns instead (step the last one with App.Advance), and one that blocks on something else keeps StepTexture waiting", n, l)
	})
	a.settle()
}

// waitParked waits, in real time and without moving the clock, until at
// least n goroutines wait in NextFrame for x, and returns how many do. why
// says what is wrong when the Timeout passes.
func (a *App) waitParked(x *tela.Texture, n int, why func() string) int {
	a.tb.Helper()
	deadline := time.Now().Add(a.cfg.timeout)
	for round := 0; ; round++ {
		st := core.TextureStateOf(x)
		if st.OnScreen && st.Waiters >= n {
			return st.Waiters
		}
		if time.Now().After(deadline) {
			a.fatalf("%s\n(waited %v)", why(), a.cfg.timeout)
		}
		a.backend.Wake()
		if round < 50 {
			runtime.Gosched()
		} else {
			time.Sleep(time.Millisecond)
		}
	}
}

// ExpectReleases fails the test unless the Release functions that the app
// gave to Texture.Submit and SubmitImage add up: every frame that Tela does
// not hold any more had its Release called exactly once, and held are the
// frames it still holds (the one on screen, per texture shown with a
// Release; the ones waiting for the next frame or for a fence). It waits,
// settling the app, for the calls that run on a goroutine of Tela's, until
// the [Timeout]. Call it with 0 after the textures unmounted
// ([App.Close] checks it by itself when the test ends, and fails the test
// for a Release that never ran or ran twice).
//
//	app.SubmitTexture(loc, frame) // no Release: nothing to count
//	app.ExpectReleases(1)         // the frame on screen
func (a *App) ExpectReleases(held int) {
	a.tb.Helper()
	a.mustBeOpen()
	s := a.sh.tex
	a.waitFor(func() (bool, string) {
		if d := s.double.Load(); d > 0 {
			a.fatalf("ExpectReleases: %d Release function(s) were called more than once: Tela calls the Release of every frame exactly once (this is a bug of Tela)", d)
		}
		given, released := s.given.Load(), s.released.Load()
		if got := int(given - released); got != held {
			return false, fmt.Sprintf("ExpectReleases(%d): %d Release functions were given to Texture.Submit and SubmitImage and %d were called: Tela holds %d frame(s). It holds the frame on screen until another replaces it, and the ones that wait", held, given, released, got)
		}
		return true, ""
	})
}

// ---------------------------------------------------------------------
// The fake device.

// FakeTextureDevice plays the GPU that draws the app's windows and imports
// the memory of its textures, so that the code path of an application that
// renders with its own GPU and gives Tela dmabufs (tela.Texture.Import)
// runs headless, on any machine: [WithTextureDevice] makes the app's
// textures import with it, and tela.Texture.Device returns the device it
// was made with. Import accepts a tela.Dmabuf whose format the device takes
// ([FakeTextureDevice.Device]'s Formats and ForeignFormats; every format
// when it lists none) and makes a [FakeTextureBuffer] of it, whose pixels
// the test provides, in the place of the application's graphics API.
//
// Frames submitted from a buffer, with the [FakeFence] the test signals,
// come to the screen as those of a real device do.
//
//	dev := telatest.NewFakeTextureDevice(tela.TextureDevice{
//	    GPU: true, Name: "Fake GPU",
//	    Formats: []tela.TextureFormat{{Fourcc: 0x34325241}}, // AR24, linear
//	})
//	dev.OnImport(func(b *telatest.FakeTextureBuffer) { b.Fill(color.RGBA{0, 0, 255, 255}) })
//	app := telatest.Mount(t, Player(), telatest.WithTextureDevice(dev))
type FakeTextureDevice struct {
	mu      sync.Mutex
	dev     tela.TextureDevice
	imports []*FakeTextureBuffer
	onImp   func(*FakeTextureBuffer)
}

// NewFakeTextureDevice returns a device that Texture.Device describes as dev.
func NewFakeTextureDevice(dev tela.TextureDevice) *FakeTextureDevice {
	return &FakeTextureDevice{dev: dev}
}

// WithTextureDevice makes the textures of the app import their memory with
// d, a [FakeTextureDevice], and Texture.Device return d's description. It
// applies from the first frame of the app. A device serves one app at a time.
func WithTextureDevice(d *FakeTextureDevice) Option {
	return func(c *config) { c.texDevice = d }
}

// Device returns the description that Texture.Device gives: the one the
// device was made with, or last set with [FakeTextureDevice.SetDevice].
func (d *FakeTextureDevice) Device() tela.TextureDevice {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dev
}

// SetDevice changes the description that Texture.Device gives, as a device
// that is replaced does, and the formats the next imports are checked
// against.
func (d *FakeTextureDevice) SetDevice(dev tela.TextureDevice) {
	d.mu.Lock()
	d.dev = dev
	d.mu.Unlock()
}

// OnImport makes f the function called for every buffer Texture.Import makes
// on the device, on the goroutine that called Import, before it returns: the
// place to give the buffer its first pixels, as the application's renderer
// would. nil for none.
func (d *FakeTextureDevice) OnImport(f func(*FakeTextureBuffer)) {
	d.mu.Lock()
	d.onImp = f
	d.mu.Unlock()
}

// Imports returns the buffers imported on the device so far, in the order of
// the Import calls.
func (d *FakeTextureDevice) Imports() []*FakeTextureBuffer {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.imports)
}

// importMemory is what Texture.Import calls (core.FakeTextureImport).
func (d *FakeTextureDevice) importMemory(m core.TextureMemory) (func() []byte, func(), error) {
	dm, ok := m.(tela.Dmabuf)
	if !ok {
		return nil, nil, fmt.Errorf("telatest: the fake texture device imports a tela.Dmabuf, not %T: %w", m, tela.ErrTextureUnsupported)
	}
	dev := d.Device()
	if len(dev.Formats)+len(dev.ForeignFormats) > 0 {
		f := tela.TextureFormat{Fourcc: dm.Fourcc, Modifier: dm.Modifier}
		if !slices.Contains(dev.Formats, f) && !slices.Contains(dev.ForeignFormats, f) {
			return nil, nil, fmt.Errorf("telatest: the fake texture device %q does not import fourcc 0x%08x with modifier 0x%x: it takes %v (its TextureDevice.Formats and ForeignFormats): %w", dev.Name, dm.Fourcc, dm.Modifier, append(slices.Clone(dev.Formats), dev.ForeignFormats...), tela.ErrTextureUnsupported)
		}
	}
	b := &FakeTextureBuffer{dmabuf: dm, pix: make([]byte, 4*dm.Width*dm.Height)}
	d.mu.Lock()
	d.imports = append(d.imports, b)
	onImp := d.onImp
	d.mu.Unlock()
	if onImp != nil {
		onImp(b)
	}
	return b.read, b.close, nil
}

// FakeTextureBuffer is memory that a texture imported on a
// [FakeTextureDevice] (Texture.Import): the test gives it the pixels that
// the application's renderer would draw into it. Its methods may be called
// from any goroutine.
//
// As with a real buffer, the pixels are read when a frame submitted from it
// is drawn: set them before Submit, and do not change them before the frame's
// Release, so that every frame shows the pixels it was submitted with.
type FakeTextureBuffer struct {
	dmabuf tela.Dmabuf

	mu     sync.Mutex
	pix    []byte // premultiplied RGBA8, rows of 4*width bytes
	reads  int
	closed bool
}

// Dmabuf returns what the application gave Texture.Import: the size, the
// fourcc and the modifier of the buffer it describes.
func (b *FakeTextureBuffer) Dmabuf() tela.Dmabuf { return b.dmabuf }

// SetPixels gives the buffer the pixels of img, which it copies: its size
// must be the buffer's (Dmabuf().Width by Height). The pixels are
// premultiplied sRGB, the first row at the top, whatever the buffer's
// fourcc: a fake buffer has no byte order. It panics for another size.
func (b *FakeTextureBuffer) SetPixels(img *image.RGBA) {
	w, h := b.dmabuf.Width, b.dmabuf.Height
	if img == nil || img.Rect.Dx() != w || img.Rect.Dy() != h {
		panic(fmt.Sprintf("telatest: FakeTextureBuffer.SetPixels: the buffer is %dx%d: pass an image of that size (the frame the application's renderer would draw)", w, h))
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for y := range h {
		copy(b.pix[4*w*y:4*w*(y+1)], img.Pix[y*img.Stride:])
	}
}

// Fill gives every pixel of the buffer the color c.
func (b *FakeTextureBuffer) Fill(c color.Color) {
	r, g, bl, al := c.RGBA() // premultiplied, 16 bits
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := 0; i < len(b.pix); i += 4 {
		b.pix[i], b.pix[i+1], b.pix[i+2], b.pix[i+3] = byte(r>>8), byte(g>>8), byte(bl>>8), byte(al>>8)
	}
}

// Reads returns how many frames submitted from the buffer were drawn: each
// one reads the pixels once.
func (b *FakeTextureBuffer) Reads() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads
}

// Closed reports whether Tela gave the buffer back: it was closed
// (TextureBuffer.Close, or its texture's end) and no frame of it is held.
func (b *FakeTextureBuffer) Closed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// read returns a copy of the pixels, for a frame being drawn.
func (b *FakeTextureBuffer) read() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reads++
	return slices.Clone(b.pix)
}

func (b *FakeTextureBuffer) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
}

// FakeFence is a tela.Fence that the test ends: give it to
// TextureFrame.Fence, and the frame waits, as one whose GPU work is running
// does, until [FakeFence.Signal] (its pixels are finished) or
// [FakeFence.Fail] (the work failed, and the frame is dropped). Settling
// waits for what follows a Signal, so the frame is on screen when the next
// action or screenshot looks.
//
//	f := telatest.NewFakeFence()
//	tex.Submit(buf, tela.TextureFrame{Fence: f})
//	app.Pending() // ... a PendingFence: the frame waits
//	f.Signal()
//	app.Golden("frame.png") // shows it
type FakeFence struct {
	done  chan struct{}
	once  sync.Once
	err   error
	ended atomic.Bool
}

// NewFakeFence returns a fence that waits for [FakeFence.Signal] or
// [FakeFence.Fail].
func NewFakeFence() *FakeFence { return &FakeFence{done: make(chan struct{})} }

// Signal ends the fence: its frame's pixels are finished. Calling it again,
// or after Fail, does nothing.
func (f *FakeFence) Signal() {
	f.once.Do(func() {
		f.ended.Store(true)
		close(f.done)
	})
}

// Fail ends the fence with err, which Tela reports as the reason the frame
// was dropped: the GPU work failed. It does nothing after Signal.
func (f *FakeFence) Fail(err error) {
	if err == nil {
		err = errors.New("telatest: FakeFence.Fail")
	}
	f.once.Do(func() {
		f.err = err
		f.ended.Store(true)
		close(f.done)
	})
}

// Done reports whether Signal or Fail was called: the fence has ended even
// if the goroutine that waits for it has not returned from Wait yet. Tela's
// settling in tests reads it, to wait for that goroutine.
func (f *FakeFence) Done() bool { return f.ended.Load() }

// Wait implements tela.Fence.
func (f *FakeFence) Wait(ctx context.Context) error {
	select {
	case <-f.done:
		return f.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
