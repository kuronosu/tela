package core

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
)

// What telatest needs of external textures (ADR-318): to find the texture
// an element shows, to see which frames and goroutines are pending, to
// count Release calls, to pace NextFrame by hand and to play the GPU that
// imports memory.
//
// Nothing here runs in an application: the Host's hooks are nil outside
// tests, and the functions below are called only by telatest, so the linker
// leaves them out (examples/hello's size budget, ADR-290). The accessors are
// functions and not methods of Texture, which tela.Texture exports.

// TextureHooks is what a test harness attaches to a Host's textures
// (Host.SetTextureHooks). Its functions are called from any goroutine, and
// each may be nil.
type TextureHooks struct {
	// Release wraps the Release function of a frame given to Submit or
	// SubmitImage, and returns the one Tela calls (once, if it keeps its
	// promise). It is called for the frames whose Release is not nil, also
	// when the Submit call returns an error.
	Release func(release func()) func()
	// HoldFrames reports whether the start of a window's frame leaves the
	// goroutines that wait in Texture.NextFrame waiting. telatest holds
	// them in the frames that settling runs, and lets them go in the frames
	// of App.Advance and App.StepTexture, so that a producer paced by the
	// window never makes an app unable to settle.
	HoldFrames func() bool

	// mu guards await and blocked: await counts the goroutines that wait
	// for the pixels of a frame (its fence, the copy of its memory), and
	// blocked the fences they are inside the Wait of.
	mu      sync.Mutex
	await   int
	blocked map[*hookedFence]struct{}
}

// InTransit reports whether a goroutine of Tela's is finishing a frame: its
// Fence ended (or is about to: a fence that has a Done method, as telatest's
// FakeFence, says when it did) or its memory is being copied, so the frame
// is about to become the ready one or be dropped. A frame whose fence has
// not ended is not in transit.
func (k *TextureHooks) InTransit() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	n := k.await
	for f := range k.blocked {
		if !f.done() {
			n--
		}
	}
	return n > 0
}

// textureHooker is what the Host's textures call: the methods of
// *TextureHooks. It is an interface so that a program that does not attach
// hooks does not link the type, nor what it holds.
type textureHooker interface {
	hookRelease(release func()) func()
	hookFence(f Fence) Fence
	hookAwait(delta int)
	holdFrames() bool
}

// SetTextureHooks attaches hooks to the Host's textures, nil to detach
// them. Call it before the first frame.
func (h *Host) SetTextureHooks(k *TextureHooks) {
	if k == nil {
		h.texHooks = nil
		return
	}
	h.texHooks = k
}

// hooks returns the hooks of the texture's Host, nil for none.
func (x *Texture) hooks() textureHooker {
	if x.host == nil {
		return nil
	}
	return x.host.texHooks
}

// hookRelease returns the Release to give the frame: release itself, or the
// hooks' wrapper of it.
func (x *Texture) hookRelease(release func()) func() {
	if release == nil {
		return nil
	}
	if k := x.hooks(); k != nil {
		return k.hookRelease(release)
	}
	return release
}

// hookFence returns the Fence to wait for: f itself, or one that tells the
// hooks when its Wait blocks.
func (x *Texture) hookFence(f Fence) Fence {
	if f == nil {
		return nil
	}
	if k := x.hooks(); k != nil {
		return k.hookFence(f)
	}
	return f
}

// hookAwait tells the hooks that a goroutine that waits for the pixels of a
// frame started (1) or ended (-1).
func (x *Texture) hookAwait(delta int) {
	if k := x.hooks(); k != nil {
		k.hookAwait(delta)
	}
}

// holdTextureFrames reports whether the window's frame that starts leaves
// the NextFrame calls waiting.
func (t *Tree) holdTextureFrames() bool {
	if t.wv == nil || t.wv.host.texHooks == nil {
		return false
	}
	return t.wv.host.texHooks.holdFrames()
}

func (k *TextureHooks) hookRelease(release func()) func() {
	if k.Release == nil {
		return release
	}
	return k.Release(release)
}

func (k *TextureHooks) hookFence(f Fence) Fence { return &hookedFence{Fence: f, k: k} }

func (k *TextureHooks) hookAwait(delta int) {
	k.mu.Lock()
	k.await += delta
	k.mu.Unlock()
}

func (k *TextureHooks) holdFrames() bool { return k.HoldFrames != nil && k.HoldFrames() }

// hookedFence is a Fence whose Wait tells the hooks that it blocks.
type hookedFence struct {
	Fence
	k *TextureHooks
}

func (f *hookedFence) Wait(ctx context.Context) error {
	f.k.mu.Lock()
	if f.k.blocked == nil {
		f.k.blocked = map[*hookedFence]struct{}{}
	}
	f.k.blocked[f] = struct{}{}
	f.k.mu.Unlock()
	defer func() {
		f.k.mu.Lock()
		delete(f.k.blocked, f)
		f.k.mu.Unlock()
	}()
	return f.Fence.Wait(ctx)
}

func (f *hookedFence) closeFence() { closeFence(f.Fence) }

// done reports whether the fence says that it ended: it has a Done method
// that returns true.
func (f *hookedFence) done() bool {
	d, ok := f.Fence.(interface{ Done() bool })
	return ok && d.Done()
}

// ---------------------------------------------------------------------
// Inspection.

// NodeTexture returns the texture that the element of semantics node id
// shows in the last semantics tree, nil when it shows none.
func (t *Tree) NodeTexture(id semantics.NodeID) *Texture {
	el := t.liveElement(id)
	if el == nil || el.kind != elemPrimitive || el.node.d == nil || el.node.d.media == nil {
		return nil
	}
	return el.node.d.media.tex
}

// TextureTestState is what TextureStateOf reports.
type TextureTestState struct {
	// Waiting counts the frames that wait for their fences or for the copy
	// of their memory; Ready tells whether a finished frame waits for the
	// window's next frame.
	Waiting int
	Ready   bool
	// Waiters counts the goroutines in NextFrame, and OnScreen tells
	// whether an element shows the texture on screen, which is when the
	// window is asked for a frame for them.
	Waiters  int
	OnScreen bool
}

// TextureStateOf returns the state of x's mailbox and of its pacing. Any
// goroutine.
func TextureStateOf(x *Texture) TextureTestState {
	x.mu.Lock()
	defer x.mu.Unlock()
	return TextureTestState{
		Waiting: len(x.mb.waiting), Ready: x.mb.ready != nil,
		Waiters: len(x.waiters), OnScreen: x.primary != nil && !x.closed,
	}
}

// AskTextureFrames asks the window for the frame that the goroutines
// waiting in NextFrame wait for, whether or not one was asked before: a
// held frame (HoldFrames) leaves none requested, and a goroutine that just
// started to wait may not have had its request handled yet. UI thread.
func (t *Tree) AskTextureFrames() {
	for _, v := range slices.Clone(t.tex.views) {
		x := v.x
		x.mu.Lock()
		ask := len(x.waiters) > 0 && x.primary == v
		if ask {
			x.asked = true
		}
		x.mu.Unlock()
		if ask {
			v.ask()
		}
	}
}

// TexturesParked reports whether goroutines wait in NextFrame for a texture
// that this tree's window paces.
func (t *Tree) TexturesParked() bool {
	for _, v := range t.tex.views {
		x := v.x
		x.mu.Lock()
		parked := len(x.waiters) > 0 && x.primary == v
		x.mu.Unlock()
		if parked {
			return true
		}
	}
	return false
}

// OnlyTextureFrames reports that the frame the tree's window was asked for
// is one that goroutines in NextFrame wait for, and nothing else needs one:
// as with endless animations, nothing waits for those frames (telatest does
// not settle for them, and an app that loops on NextFrame never ends).
func (t *Tree) OnlyTextureFrames() bool {
	return t.TexturesParked() && !t.changed && !t.repaint && !t.updatesQueued() && !t.Animating() && !t.scrollAnimating()
}

// texturePending adds what the textures of t shown on screen keep pending
// (Tree.Pending).
func (t *Tree) texturePending(add func(PendingKind, string)) {
	for _, v := range t.tex.views {
		x := v.x
		x.mu.Lock()
		// One report per texture: from its window's tree, or, for a
		// texture that is not on screen, from its first view.
		reports := !x.closed && (x.primary == v || x.primary == nil && len(x.views) > 0 && x.views[0] == v)
		waiters, waiting := 0, len(x.mb.waiting)
		if x.primary != nil {
			waiters = len(x.waiters)
		}
		x.mu.Unlock()
		if !reports {
			continue
		}
		if waiters > 0 {
			add(PendingNextFrame, fmt.Sprintf("%d goroutine(s) wait in Texture.NextFrame for the next frame of the window: a texture on screen whose producer renders at the window's pace asks for a frame at every vsync", waiters))
		}
		if waiting > 0 {
			add(PendingFence, fmt.Sprintf("%d frame(s) of a texture wait for their fences, or for the copy of their memory: they are shown once the wait ends", waiting))
		}
	}
}

// ---------------------------------------------------------------------
// The device.

// FakeTextureImport plays what a GPU does with memory that Texture.Import
// gives it: it returns the pixels of the imported picture, read when a
// frame that shows it is drawn (premultiplied RGBA8, rows of 4*width bytes;
// nil draws nothing), and free, called once when the buffer is closed and no
// frame of it is held (nil for none). An error that wraps
// ErrTextureUnsupported lets Import fall back to mapping the memory, as with
// a real device.
type FakeTextureImport func(m TextureMemory) (pixels func() []byte, free func(), err error)

// SetFakeTextureDevice makes the Host's textures import their memory with
// imp, on a device that Texture.Device describes as dev() (called for each
// description), in the place of what draws the windows (nothing, in a
// test). A nil imp removes it.
func (h *Host) SetFakeTextureDevice(dev func() TextureDevice, imp FakeTextureImport) {
	if imp == nil {
		h.setTextureImporter(nil)
		return
	}
	h.setTextureImporter(fakeImporter{dev: dev, imp: imp})
}

type fakeImporter struct {
	dev func() TextureDevice
	imp FakeTextureImport
}

func (f fakeImporter) textureDevice() TextureDevice {
	d := f.dev()
	d.Formats = slices.Clone(d.Formats)
	d.ForeignFormats = slices.Clone(d.ForeignFormats)
	return d
}

func (f fakeImporter) importTexture(m TextureMemory) (textureContent, error) {
	pixels, free, err := f.imp(m)
	if err != nil {
		return nil, err
	}
	return &fakeContent{pixels: pixels, free: free}, nil
}

// fakeContent is the content of an imported buffer of the fake device.
type fakeContent struct {
	pixels func() []byte
	free   func()
}

var _ gfx.External = (*fakeContent)(nil)

func (c *fakeContent) Pixels() []byte {
	if c.pixels == nil {
		return nil
	}
	return c.pixels()
}

func (c *fakeContent) closeTexture() {
	if c.free != nil {
		c.free()
	}
}
