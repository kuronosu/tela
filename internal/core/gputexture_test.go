//go:build telagpu && linux

package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/gpu"
	"github.com/kuronosu/tela/internal/gfx/gpu/gputest"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Textures drawn by the GPU (gputexture.go): an application renders on a
// device of its own (gputest.App), of the GPU that draws the window or of
// another one, for every pair of devices of the machine. The frames of the
// window are compared as gpuframe_test.go says.

// texDevice is a window's device that imports: it counts, and fails or is
// lost when a test says so.
type texDevice struct {
	*fakeDevice
	imp gpu.Importer
	// lost stands for a device lost, importErr for what an import answers.
	lost      atomic.Bool
	importErr error
	imports   atomic.Int32
	// onClose, when set, runs before Close.
	onClose func()
}

func (d *texDevice) DeviceID() gpu.DeviceID { return d.imp.DeviceID() }
func (d *texDevice) Lost() bool             { return d.lost.Load() || d.imp.Lost() }

func (d *texDevice) ImportFormats() (own, foreign []gpu.DmabufFormat) {
	if d.lost.Load() {
		return nil, nil
	}
	return d.imp.ImportFormats()
}

func (d *texDevice) Import(desc gpu.DmabufDescriptor) (*gpu.Imported, error) {
	if d.lost.Load() {
		return nil, gpu.ErrDeviceLost
	}
	if d.importErr != nil {
		return nil, d.importErr
	}
	d.imports.Add(1)
	return d.imp.Import(desc)
}

func (d *texDevice) Close() {
	if d.onClose != nil {
		d.onClose()
	}
	d.Device.Close()
}

// texGPU gives the Host the GPU of a device that imports, which the Host
// closes, in the place of the checker's.
func texGPU(c *gpuChecker, h *Host, win gpu.Device) *texDevice {
	c.fake.real = true
	d := &texDevice{fakeDevice: c.fake, imp: win.(gpu.Importer)}
	c.g = &GPU{dev: d, own: true}
	if h.texHooks == nil {
		h.SetTextureHooks(&TextureHooks{}) // texIdle
	}
	h.UseGPU(c.g)
	return d
}

func newTexRig(t *testing.T, win gpu.Device, root Node, w, h float32, scale float64, dm *dmabufConfig) (*gpuRig, *texDevice) {
	t.Helper()
	x := newGPURigOn(t, win, Options{Config: Config{InstantScroll: true}, Root: root}, w, h, scale, dm)
	return x, texGPU(x.c, x.h, win)
}

// How the window reads the frames of a buffer.
const (
	texSampled = iota // where they lie
	texCopied         // copied on the GPU
	texMapped         // copied by the CPU
)

func howRead(b *TextureBuffer) int {
	switch c, _ := b.content.(*gpuTexture); {
	case c == nil:
		return texMapped
	case c.im.Copied():
		return texCopied
	}
	return texSampled
}

// dmabuf describes a buffer of an application.
func dmabuf(b *gputest.Buffer) Dmabuf {
	d := Dmabuf{Width: b.Width, Height: b.Height, Fourcc: b.Format.Fourcc, Modifier: b.Format.Modifier}
	for _, p := range b.Planes {
		d.Planes = append(d.Planes, DmabufPlane{FD: p.FD, Offset: p.Offset, Stride: p.Stride})
	}
	return d
}

// texBuf is a buffer of a producer.
type texBuf struct {
	app *gputest.Buffer
	tb  *TextureBuffer
	// free: no frame of it is held, as its Releases tell.
	free atomic.Bool
}

// texProducer draws an application's frames into buffers of its own, and
// spoils each as soon as it is released.
type texProducer struct {
	t     *testing.T
	tex   *Texture
	bufs  []*texBuf
	spoil bool
	// opaque: the frames' pictures are.
	opaque bool
	// submitted and released count frames; double is set by a Release that
	// was called twice.
	submitted, released atomic.Int64
	double              atomic.Bool
}

// texPictures are the pictures of the frames, by size (texPicture).
var texPictures = map[image.Point][]*image.RGBA{}

func texPicture(w, h, n int) *image.RGBA {
	k := image.Pt(w, h)
	if texPictures[k] == nil {
		for i := range 6 {
			pic := image.NewRGBA(image.Rect(0, 0, w, h))
			for y := range h {
				for x := range w {
					a := 255
					if (x/12+y/12+i)%5 == 0 {
						a = 120
					}
					p := func(v int) uint8 { return uint8(v * a / 255) }
					pic.SetRGBA(x, y, color.RGBA{p((x*2 + i*40) % 256), p((y*3 + i*70) % 256), p((x + y + i*25) % 256), uint8(a)})
				}
			}
			texPictures[k] = append(texPictures[k], pic)
		}
	}
	return texPictures[k][n%len(texPictures[k])]
}

// addBuffer imports a buffer of the application's.
func (p *texProducer) addBuffer(b *gputest.Buffer) *texBuf {
	p.t.Helper()
	tb, err := p.tex.Import(dmabuf(b))
	if err != nil {
		p.t.Fatalf("%#x %#x: %v", b.Format.Fourcc, b.Format.Modifier, err)
	}
	out := &texBuf{app: b, tb: tb}
	out.free.Store(true)
	p.bufs = append(p.bufs, out)
	return out
}

// frame draws texPicture n into a free buffer and submits it; false when none is
// free.
func (p *texProducer) frame(n int, fence Fence) bool {
	p.t.Helper()
	for _, b := range p.bufs {
		if b.free.Load() {
			pic := texPicture(b.app.Width, b.app.Height, n)
			if p.opaque {
				pic = opaqueIf(pic, fourccXR24)
			}
			p.submit(b, pic, fence)
			return true
		}
	}
	return false
}

func (p *texProducer) submit(b *texBuf, pic *image.RGBA, fence Fence) {
	p.t.Helper()
	b.app.Draw(pic)
	b.free.Store(false)
	p.submitted.Add(1)
	err := p.tex.Submit(b.tb, TextureFrame{Fence: fence, Release: func() {
		p.released.Add(1)
		if p.spoil {
			b.app.Draw(solid(b.app.Width, b.app.Height, color.RGBA{255, 0, 255, 255}))
		}
		if b.free.Swap(true) {
			p.double.Store(true)
		}
	}})
	if err != nil {
		p.t.Fatal(err)
	}
}

// texIdle waits until the release goroutine of x has nothing left, and no
// frame waits: the windows were woken for those that ended.
func texIdle(t *testing.T, x *Texture) {
	t.Helper()
	waitFor(t, "a texture's goroutines", func() bool {
		if x.hooks().(*TextureHooks).InTransit() {
			return false
		}
		x.mu.Lock()
		defer x.mu.Unlock()
		return !x.rqOn && len(x.rq) == 0 && len(x.mb.waiting) == 0
	})
}

// opaqueIf returns the pixels a fourcc keeps of a picture.
func opaqueIf(pic *image.RGBA, fourcc uint32) *image.RGBA {
	out := image.NewRGBA(pic.Rect)
	copy(out.Pix, pic.Pix)
	if _, opaque, _ := textureFourcc(fourcc); opaque {
		for i := 3; i < len(out.Pix); i += 4 {
			out.Pix[i] = 255
		}
	}
	return out
}

// texAppBuffers are the buffers an application tries, n or more of them, for
// pictures of w x h: in the device's own layouts where it is the window's, and
// in rows of system memory that lie in every way.
func texAppBuffers(t *testing.T, app *gputest.App, win gpu.Importer, w, h, n int) []*gputest.Buffer {
	t.Helper()
	var out []*gputest.Buffer
	own, foreign := win.ImportFormats()
	formats := foreign
	if app.Same {
		formats = own
	}
	tiled := 0
	for _, f := range formats {
		if f.Modifier != gpu.ModifierLinear {
			if tiled++; tiled > 3 {
				continue
			}
		}
		if app.Exports(f) {
			out = append(out, app.NewBuffer(t, w, h, f))
		}
	}
	for i, f := range []uint32{fourccAR24, fourccXR24, fourccAB24, fourccXB24} {
		offset, stride := []int{0, 4, 4096, 2}[i], []int{4 * w, (4*w + 255) &^ 255, 4*w + 12, 4*w + 2}[i]
		out = append(out, app.NewSystemBuffer(t, w, h, f, offset, stride))
	}
	for len(out) < n {
		out = append(out, app.NewSystemBuffer(t, w, h, fourccAB24, 0, (4*w+255)&^255))
	}
	return out
}

// TestTextureGPUPixels shows buffers of an application on every device
// through textures under clips, opacity, transforms, filters, blend modes,
// masks, a backdrop filter and scrolled lists, in windows read back and
// presented without a copy, at x1 and x2: the window's frame is the one the
// CPU rasterizer draws of the same pictures submitted as pixels, within the
// tolerance of ADR-282.
func TestTextureGPUPixels(t *testing.T) {
	rounds := 3
	if testing.Short() {
		rounds = 2
	}
	gputest.EachPair(t, false, func(t *testing.T, app *gputest.App) {
		const pw, ph = 100, 72
		for i, c := range []struct {
			name  string
			scale float64
			dm    *dmabufConfig
		}{{"x1", 1, nil}, {"x2", 2, nil}, {"direct_x1", 1, &dmabufWayland}, {"direct_x2", 2, &dmabufX11}} {
			t.Run(c.name, func(t *testing.T) {
				win := app.Window(t)
				st := &sceneState{refs: make([]ElementRef, 4), setters: make([]Setter[int], 200), values: make([]*Value[float32], 4)}
				var texs []*Texture
				x, td := newTexRig(t, win, textureScene(st, &texs), 400, 420, c.scale, c.dm)
				x.settle("mount")
				// The reference: the same page with pixels, on the CPU.
				rst := &sceneState{refs: make([]ElementRef, 4), setters: make([]Setter[int], 200), values: make([]*Value[float32], 4)}
				var rtexs []*Texture
				rb := headless.New(headless.Options{Size: platform.Size{W: 400, H: 420}, Scale: c.scale})
				rh, err := NewHost(rb, Options{Config: Config{InstantScroll: true, Fonts: testFonts, Flavor: testFlavor}, Root: textureScene(rst, &rtexs)})
				if err != nil {
					t.Fatal(err)
				}
				defer rh.stopTimer()
				rb.Drain(rh)

				bufs := texAppBuffers(t, app, td.imp, pw, ph, len(texs))
				kinds := [3]int{}
				ps := make([]*texProducer, len(texs))
				for i := range ps {
					ps[i] = &texProducer{t: t, tex: texs[i]}
				}
				for i, b := range bufs {
					kinds[howRead(ps[i%len(ps)].addBuffer(b).tb)]++
				}
				worst, over2 := 0, 0.0
				for n := range rounds * len(texs) {
					// Two textures get a new frame, of each of their buffers in
					// turn.
					for _, k := range []int{n % len(ps), (n + 4) % len(ps)} {
						p := ps[k]
						b := p.bufs[n/len(ps)%len(p.bufs)]
						if !b.free.Load() {
							continue
						}
						pic := texPicture(pw, ph, n)
						p.submit(b, pic, nil)
						if err := rtexs[k].SubmitImage(opaqueIf(pic, b.app.Format.Fourcc), nil); err != nil {
							t.Fatal(err)
						}
						texIdle(t, p.tex)
					}
					for _, s := range []*sceneState{st, rst} {
						switch n % 6 {
						case 2:
							s.refs[0].ScrollBy(0, 23)
						case 3:
							s.values[0].Set(float32(7 * n))
						case 4:
							s.values[1].Set(float32(n%4) / 4)
						case 5:
							s.setters[0].Update(func(v int) int { return v + 1 })
						}
					}
					x.step(fmt.Sprintf("round %d", n))
					rb.Advance(rb.FrameInterval())
					rb.Drain(rh)
					got, want := x.main().Frame(), rh.Window().(*headless.Window).Frame()
					d := gfxtest.Compare(got, want)
					worst, over2 = max(worst, d.Max), max(over2, d.Over2)
					if !d.WithinGPU() {
						if dir := os.Getenv("TELA_DAMAGE_DIR"); dir != "" {
							save(dir+"/gpu.png", got)
							save(dir+"/cpu.png", want)
						}
						t.Fatalf("round %d: largest difference %d at %v, %.3f%% of the pixels over 2 levels", n, d.Max, d.Worst, 100*d.Over2)
					}
				}
				if c.dm != nil && !x.direct() {
					t.Error("the window is read back")
				}
				if dir := os.Getenv("TELA_DAMAGE_DIR"); dir != "" {
					save(fmt.Sprintf("%s/texture_%d_%s.png", dir, i, strings.ReplaceAll(app.Dev.Name(), " ", "_")), x.main().Frame())
				}
				t.Logf("%d buffers sampled, %d copied on the GPU, %d mapped; against the CPU at most %d levels, %.3f%% over 2; %s",
					kinds[texSampled], kinds[texCopied], kinds[texMapped], worst, 100*over2, x.c.summary())
				if kinds[texSampled]+kinds[texCopied] < 3 || app.Same && kinds[texSampled] == 0 {
					t.Error("the device imported too little of what it was given")
				}
				x.c.finish(x.h)
				for i, p := range ps {
					texIdle(t, p.tex)
					if p.released.Load() != p.submitted.Load() || p.double.Load() {
						t.Errorf("texture %d: %d frames submitted, %d released (twice: %v)", i, p.submitted.Load(), p.released.Load(), p.double.Load())
					}
				}
				if x.d != nil {
					for _, w := range x.d.wins {
						if len(w.bufs) != 0 {
							t.Errorf("%d buffers left registered", len(w.bufs))
						}
					}
				}
			})
		}
	})
}

// texListScene shows textures in a list whose pixels are shifted when it
// scrolls, under a glass panel that stays.
func texListScene(st *sceneState, texs *[]*Texture) Node {
	return Component(func() Node {
		st.refs[0] = UseElementRef()
		v, set := UseState(0)
		st.setters[0] = set
		ts := make([]*Texture, 6)
		for i := range ts {
			ts[i] = UseTexture()
		}
		*texs = ts
		list := []Child{AttachRef(st.refs[0]), Key("list"), Class("relative grow overflow-y-auto bg-white")}
		classes := []string{"h-16 w-full", "m-2 h-20 w-32 rounded-xl object-contain", "h-10 w-24 opacity-70", "m-1 h-24 w-40 rotate-3", "h-12 w-full object-none"}
		for i := range 30 {
			list = append(list, div(Key(i), Class("h-7 px-2 text-sm odd:bg-slate-50 hover:bg-sky-100"), Textf("Row %d", i)))
			if i%3 == 1 && i/3 < len(classes) {
				list = append(list, texture(ts[i/3], Key(fmt.Sprint("tex", i)), Class(classes[i/3])))
			}
		}
		return div(Class("relative flex h-screen flex-col bg-white"),
			div(Class("flex h-10 shrink-0 items-center gap-2 border-b px-2 text-sm"), Textf("State %d", v), texture(ts[5], Class("h-8 w-20 rounded"))),
			div(list...),
			div(Class("absolute right-3 bottom-3 h-16 w-28 rounded-xl border border-white/50 bg-white/20 p-2 text-xs backdrop-blur-sm"), Text("glass")))
	})
}

// texLedger knows which imported memory its application may write: the proof
// that the GPU reads no frame it let go of.
type texLedger struct {
	t  *testing.T
	ps []*texProducer
	// draws counts the draws that read imported memory, reads what they read.
	draws, reads int
}

// draw is fakeDevice.onDraw: around every draw of the windows' frames.
func (l *texLedger) draw(f *gpu.Frame, draw func() error) error {
	l.t.Helper()
	var used []*gpu.Imported
	used = append(used, f.Imports...)
	for i := range f.Images {
		if im := f.Images[i].From; im != nil {
			used = append(used, im)
		}
	}
	look := func(when string) {
		l.t.Helper()
		for _, p := range l.ps {
			// Every Release called so far has spoiled its buffer.
			waitFor(l.t, "the Releases", func() bool {
				p.tex.mu.Lock()
				defer p.tex.mu.Unlock()
				return !p.tex.rqOn && len(p.tex.rq) == 0
			})
			for _, b := range p.bufs {
				if c, _ := b.tb.content.(*gpuTexture); c != nil && b.free.Load() && slices.Contains(used, c.im) {
					l.t.Errorf("%s a draw, a buffer it reads is released", when)
				}
			}
		}
	}
	if len(used) > 0 {
		l.draws++
		l.reads += len(used)
	}
	look("before")
	err := draw()
	look("after")
	return err
}

// TestTextureGPURelease plays random sessions (scrolls, hover, Values read by
// paint, textures shown twice, resizes, hiding, rests, the debug overlays)
// over textures whose buffers the application spoils the moment they are
// released, while the window is still to draw the frame that let go of them:
// no draw reads a released buffer, a full frame drawn from the buffers on
// screen is what the window shows, and every frame is released exactly once.
func TestTextureGPURelease(t *testing.T) {
	steps := 80
	if testing.Short() {
		steps = 25
	}
	gputest.EachPair(t, false, func(t *testing.T, app *gputest.App) {
		const pw, ph = 100, 72
		var texs []*Texture
		var l *texLedger
		draws, shifted := 0, 0
		setup := func(x *damageHarness) {
			win := app.Window(x.t)
			c := newGPUChecker(x.t, win)
			td := texGPU(c, x.h, win)
			l = &texLedger{t: x.t}
			c.fake.onDraw = l.draw
			bufs := texAppBuffers(x.t, app, td.imp, pw, ph, 18) // they go after the Host
			x.checkFn = func(what string) {
				x.t.Helper()
				c.checkWindow(x.h, x.w, fmt.Sprintf("step %d (%s; last steps: %s)", x.step, what, strings.Join(x.trace, " | ")))
				x.frames, x.full, x.shifted, x.partial = c.full+c.shifted+c.partial, c.full, c.shifted, c.partial
				if x.step == 1 {
					for _, tex := range texs {
						l.ps = append(l.ps, &texProducer{t: x.t, tex: tex, spoil: true})
					}
					for i, b := range bufs {
						l.ps[i%len(l.ps)].addBuffer(b)
					}
				}
				// Frames for the next step.
				for range x.rng.IntN(4) {
					p := l.ps[x.rng.IntN(len(l.ps))]
					var fence Fence
					if x.rng.IntN(4) == 0 {
						f := newChanFence()
						f.ch <- nil
						fence = f
					}
					p.frame(x.rng.IntN(100), fence)
					if x.rng.IntN(2) == 0 {
						texIdle(x.t, p.tex)
					}
				}
			}
			l := l
			x.t.Cleanup(func() {
				c.finish(x.h)
				for i, p := range l.ps {
					texIdle(x.t, p.tex)
					st := p.tex.Stats()
					if p.released.Load() != p.submitted.Load() || p.double.Load() || st.Submitted != st.Shown+st.Dropped {
						x.t.Errorf("texture %d: %d frames submitted, %d released (twice: %v), %+v", i, p.submitted.Load(), p.released.Load(), p.double.Load(), st)
					}
				}
				draws += l.draws
				shifted += c.shifted
				x.t.Logf("%d draws read imported memory, %d buffers in all; %s", l.draws, l.reads, c.summary())
			})
		}
		ops := func(x *damageHarness, st *sceneState) string {
			switch x.rng.IntN(16) {
			case 0:
				WindowHandle{x.tree()}.Hide()
				x.b.Drain(x.h)
				WindowHandle{x.tree()}.Show()
				return "hide"
			case 1:
				x.b.Advance(2 * RasterSettle) // a rest, with its trim
				x.b.Wake()
				x.b.Drain(x.h)
				return "rest"
			case 2:
				f := DebugLayers
				if x.tree().Debug() != 0 {
					f = 0
				}
				x.tree().SetDebug(f)
				return "overlays"
			case 3:
				x.h.FramePixels(x.w)
				return "pixels"
			}
			return commonOps(x, st)
		}
		root := func(st *sceneState) Node { return textureScene(st, &texs) }
		list := func(st *sceneState) Node { return texListScene(st, &texs) }
		scales := []float64{1, 2}
		if testing.Short() {
			scales = scales[:1]
		}
		runSceneWith(t, "effects", root, ops, Config{InstantScroll: true}, steps, scales, setup)
		runSceneWith(t, "list", list, ops, Config{InstantScroll: true}, steps, scales, setup)
		if draws == 0 || shifted == 0 {
			t.Errorf("%d draws read imported memory, %d frames were shifted", draws, shifted)
		}
	})
}

// texCardApp is a page that shows one texture in a rounded, clipped card.
type texCardApp struct {
	tex    *Texture
	closed Setter[bool]
	mode   Setter[string] // "": shown, "gone"
	during func()         // in an effect
}

func (a *texCardApp) root() Node {
	inner := func() Node {
		a.tex = UseTexture()
		mode, set := UseState("")
		a.mode = set
		UseMount(func() func() {
			if a.during != nil {
				a.during()
			}
			return nil
		})
		kids := []Child{Class("h-screen bg-linear-to-br from-white to-indigo-200 p-4"), div(Class("h-8 text-sm"), Text("header"))}
		if mode == "" {
			kids = append(kids, div(Class("h-40 w-60 overflow-hidden rounded-2xl shadow-lg"), texture(a.tex, Class("size-full"))))
		}
		return div(kids...)
	}
	return Component(func() Node {
		closed, set := UseState(false)
		a.closed = set
		if closed {
			return div()
		}
		return Component(inner)
	})
}

// center returns the pixel in the middle of the card.
func (a *texCardApp) center(x *gpuRig) color.RGBA {
	s := x.main().Metrics().Scale
	return x.main().Frame().RGBAAt(int(136*s), int(130*s))
}

// shows reports whether the card, which a frame of its size in device pixels
// fills, shows picture pic pixel for pixel, its corners aside.
func (a *texCardApp) shows(x *gpuRig, pic *image.RGBA) bool {
	got := x.main().Frame()
	s := int(x.main().Metrics().Scale)
	for y := 20; y < pic.Rect.Dy()-20; y++ {
		i, j := got.PixOffset(16*s+20, 48*s+y), pic.PixOffset(20, y)
		if n := 4 * (pic.Rect.Dx() - 40); !bytes.Equal(got.Pix[i:i+n], pic.Pix[j:j+n]) {
			return false
		}
	}
	return true
}

// texArea returns the pixels a frame damaged.
func texArea(d Damage) int {
	n := 0
	for _, r := range d.Changed() {
		n += r.Dx() * r.Dy()
	}
	return n
}

// TestTextureGPUNoCopy shows frames of an application's buffers in a window
// presented without a copy: nothing is read back, nothing is uploaded, no
// image is created per frame and the renderer holds no memory for them, or
// one texture for rows it copies on the GPU; each frame damages the element
// alone, is shown pixel for pixel, and is released when the next one is drawn;
// at rest the window is idle.
func TestTextureGPUNoCopy(t *testing.T) {
	gputest.EachPair(t, false, func(t *testing.T, app *gputest.App) {
		const pw, ph = 480, 320 // the card's at x2
		kinds := []string{"system"}
		if app.Same {
			kinds = append(kinds, "own")
		}
		for _, kind := range kinds {
			t.Run(kind, func(t *testing.T) {
				win := app.Window(t)
				own, _ := win.(gpu.Importer).ImportFormats()
				var a texCardApp
				x, td := newTexRig(t, win, a.root(), 320, 240, 2, &dmabufWayland)
				x.settle("mount")
				made := win.(interface{ ImagesMade() int }).ImagesMade
				p := &texProducer{t: t, tex: a.tex, spoil: true, opaque: true}
				l := &texLedger{t: t, ps: []*texProducer{p}}
				x.c.fake.onDraw = l.draw
				for range 3 {
					if kind == "own" {
						p.addBuffer(app.NewBuffer(t, pw, ph, own[0]))
					} else {
						p.addBuffer(app.NewSystemBuffer(t, pw, ph, fourccAB24, 0, 4*pw))
					}
				}
				how := howRead(p.bufs[0].tb)
				if how == texMapped || kind == "own" && how != texSampled {
					t.Fatalf("the device reads the buffers as %d", how)
				}
				reads := func() int {
					n := 0
					for _, b := range p.bufs {
						n += int(b.tb.content.(*gpuTexture).reads.Load())
					}
					return n
				}
				before, held := 0, int64(0)
				for n := range 60 {
					if !p.frame(n, nil) {
						t.Fatalf("frame %d: no buffer is free", n)
					}
					x.b.Advance(x.b.FrameInterval())
					x.deliver()
					if !x.direct() {
						t.Fatalf("frame %d was read back", n)
					}
					if !a.shows(x, opaqueIf(texPicture(pw, ph, n), fourccXR24)) {
						t.Fatalf("frame %d is not shown pixel for pixel", n)
					}
					if n < 4 {
						before, held = made(), x.c.g.Memory().Images
						continue
					}
					if st := x.h.main.gpu.surf.Stats; st.UploadBytes != 0 || st.Pixels != 0 {
						t.Errorf("frame %d: %d bytes uploaded, %d pixels read back", n, st.UploadBytes, st.Pixels)
					}
					if d := x.h.Tree().LastDamage(); d.Full || texArea(d) > (pw+4)*(ph+4) {
						t.Errorf("frame %d damaged %+v", n, d)
					}
					waitFor(t, "the Release of the frame before", func() bool { return p.released.Load() == p.submitted.Load()-1 })
				}
				want := int64(0)
				if how == texCopied {
					want = 4 * pw * ph
				}
				if made() != before || held != want || x.c.g.Memory().Images != want {
					t.Errorf("%d images created by the frames after the first ones; %d bytes of images, then %d, want %d", made()-before, held, x.c.g.Memory().Images, want)
				}
				if n := reads(); n != 0 || td.imports.Load() != 3 {
					t.Errorf("the pixels were read %d times, and %d imports made for three buffers", n, td.imports.Load())
				}
				if st := a.tex.Stats(); st.Shown != 60 || st.Dropped != 0 {
					t.Errorf("%+v", st)
				}
				// At rest the window is idle, and a trim keeps the texture.
				x.settle("rest")
				presents := x.h.Presents()
				x.b.Advance(2 * RasterSettle)
				x.b.Wake()
				x.drain()
				if x.h.Presents() != presents || x.b.Pending() || x.h.Tree().Dirty() || x.c.g.Memory().Images != want {
					t.Errorf("at rest: %d presents, pending %v, %d bytes of images", x.h.Presents()-presents, x.b.Pending(), x.c.g.Memory().Images)
				}
				x.check("after the trim")
				x.c.finish(x.h)
				texIdle(t, a.tex)
				if p.released.Load() != p.submitted.Load() || p.double.Load() {
					t.Errorf("%d frames submitted, %d released", p.submitted.Load(), p.released.Load())
				}
			})
		}
	})
}

// TestTextureGPUStream shows frames of pixels, given by the application and
// copied from mapped memory (a buffer imported before the GPU opened, which
// stays so): one image holds them all.
func TestTextureGPUStream(t *testing.T) {
	gputest.EachPair(t, true, func(t *testing.T, app *gputest.App) {
		const pw, ph = 480, 320
		for _, kind := range []string{"pixels", "mapped"} {
			t.Run(kind, func(t *testing.T) {
				win := app.Window(t)
				var a texCardApp
				x := newGPURigOn(t, win, Options{Root: a.root()}, 320, 240, 2, &dmabufWayland)
				x.h.closeGPU()
				var b *TextureBuffer
				app0 := app.NewSystemBuffer(t, pw, ph, fourccXR24, 8, 4*pw+12)
				if kind == "mapped" {
					var err error
					if b, err = a.tex.Import(dmabuf(app0)); err != nil {
						t.Fatal(err)
					}
					if howRead(b) != texMapped {
						t.Fatal("a buffer imported without a GPU was imported by it")
					}
				}
				texGPU(x.c, x.h, win)
				x.settle("mount")
				made := win.(interface{ ImagesMade() int }).ImagesMade
				before := 0
				var rel texReleases
				for n := range 40 {
					pic := opaqueIf(texPicture(pw, ph, n), fourccXR24)
					if kind == "mapped" {
						app0.Draw(pic)
						if err := a.tex.Submit(b, TextureFrame{Release: rel.fn("x")}); err != nil {
							// Held until another frame replaces it: one does.
							if err := a.tex.SubmitImage(pic, nil); err != nil {
								t.Fatal(err)
							}
						}
						texIdle(t, a.tex)
					} else if err := a.tex.SubmitImage(pic, rel.fn("x")); err != nil {
						t.Fatal(err)
					}
					x.step(fmt.Sprintf("frame %d", n))
					texIdle(t, a.tex)
					if n < 3 {
						before = made()
					} else if st := x.h.main.gpu.surf.Stats; st.Uploads != 1 || st.UploadBytes != 4*pw*ph {
						t.Errorf("frame %d: %d uploads of %d bytes", n, st.Uploads, st.UploadBytes)
					}
					if got := x.c.g.Memory().Images; got != 4*pw*ph {
						t.Errorf("frame %d: %d bytes of images, want one frame's", n, got)
					}
				}
				if made() != before {
					t.Errorf("%d images created by the frames after the first ones", made()-before)
				}
				x.c.finish(x.h)
			})
		}
	})
}

// TestTextureGPUDevice checks what Texture.Device says: the device and what
// it imports, in the order it prefers, with what would be mapped last; as
// without a GPU before it opened, once every window fell back (until another
// one opens), once it is lost and once it closed; and of another device that
// takes its place.
func TestTextureGPUDevice(t *testing.T) {
	gputest.EachPair(t, false, func(t *testing.T, app *gputest.App) {
		win := app.Window(t)
		var a texCardApp
		var open Setter[bool]
		child := childWindow(div(Class("h-screen bg-white"), Text("child")))
		x := newGPURig(t, win, Options{Root: Component(func() Node {
			show, set := UseState(false)
			open = set
			if !show {
				return div(Class("h-screen"), a.root())
			}
			return div(Class("h-screen"), a.root(), child)
		})}, 320, 240, 1)
		plain := func(when string) {
			t.Helper()
			d := a.tex.Device()
			if d.GPU || d.Name != "" || d.RenderNode != "" || d.UUID != ([16]byte{}) || !slices.Equal(d.Formats, mappedFormats()) || !slices.Equal(d.ForeignFormats, mappedFormats()) {
				t.Errorf("%s: %+v", when, d)
			}
		}
		x.h.closeGPU()
		plain("without a GPU")
		texGPU(x.c, x.h, win)
		id := win.(gpu.Identified).DeviceID()
		d := a.tex.Device()
		if !d.GPU || d.Name != win.Name() || d.VendorID != id.VendorID || d.DeviceID != id.DeviceID || d.UUID != id.UUID || d.LUID != ([8]byte{}) {
			t.Errorf("%+v of %+v", d, id)
		}
		var st unix.Stat_t
		if !strings.HasPrefix(d.RenderNode, "/dev/dri/renderD") || unix.Stat(d.RenderNode, &st) != nil || st.Rdev != unix.Mkdev(d.RenderMajor, d.RenderMinor) || d.RenderMajor != id.Render.Major || d.RenderMinor != id.Render.Minor {
			t.Errorf("the render node %q (%d:%d) of %+v", d.RenderNode, d.RenderMajor, d.RenderMinor, id.Render)
		}
		own, foreign := win.(gpu.Importer).ImportFormats()
		for i, f := range own {
			if d.Formats[i] != (TextureFormat{f.Fourcc, f.Modifier}) {
				t.Errorf("format %d is %+v, want %+v", i, d.Formats[i], f)
			}
		}
		for i, f := range foreign {
			if d.ForeignFormats[i] != (TextureFormat{f.Fourcc, f.Modifier}) {
				t.Errorf("foreign format %d is %+v, want %+v", i, d.ForeignFormats[i], f)
			}
		}
		for _, f := range mappedFormats() {
			if !slices.Contains(d.Formats, f) || !slices.Contains(d.ForeignFormats, f) {
				t.Errorf("the format %+v, which is mapped, is not listed", f)
			}
		}
		if d.Formats[0].Modifier == 0 {
			t.Errorf("the preferred format %+v is linear", d.Formats[0])
		}
		// The slices are the caller's.
		d.Formats[0], d.ForeignFormats[0] = TextureFormat{}, TextureFormat{}
		if again := a.tex.Device(); again.Formats[0] == (TextureFormat{}) || again.ForeignFormats[0] == (TextureFormat{}) {
			t.Error("a change of the formats returned changes the next answer")
		}
		t.Logf("%s: %d formats, %d foreign ones", d.RenderNode, len(d.Formats), len(d.ForeignFormats))

		// The window falls back: nothing is drawn by the GPU any more.
		x.settle("mount")
		x.c.fake.fail = func(op string) error {
			x.c.fake.fail = nil
			return errInjected
		}
		x.main().Resize(platform.Size{W: 330, H: 240})
		x.step("the frame that fails")
		plain("after every window fell back")
		if x.h.GPU() == nil {
			t.Error("the GPU closed")
		}
		// A new window is drawn by it again.
		open.Set(true)
		x.settle("another window")
		if !a.tex.Device().GPU {
			t.Error("the GPU draws a window, and is not the device")
		}
		open.Set(false)
		x.settle("the window closed")
		// A lost device, before the Host knows.
		td := &texDevice{fakeDevice: x.c.fake, imp: app.Window(t).(gpu.Importer)}
		x.h.UseGPU(&GPU{dev: td})
		if !a.tex.Device().GPU {
			t.Error("a GPU that took the place of another one is not the device")
		}
		td.lost.Store(true)
		plain("on a lost device")
		td.lost.Store(false)
		if !a.tex.Device().GPU {
			t.Error("the device has no identity")
		}
		x.h.closeGPU()
		plain("after the GPU closed")
		x.c.finish(x.h)
	})
}

// TestTextureGPUImport checks what the import of a window with a GPU answers:
// what the device does not take falls back to the mapped copy when it is
// linear, an error that wraps ErrTextureUnsupported says what it is
// otherwise, and a wrong descriptor is an error that does not; an import on
// the UI thread, in a build, an effect and OnResize, is made there.
func TestTextureGPUImport(t *testing.T) {
	gputest.EachPair(t, true, func(t *testing.T, app *gputest.App) {
		win := app.Window(t)
		var a texCardApp
		x, td := newTexRig(t, win, a.root(), 320, 240, 1, nil)
		x.settle("mount")
		own, _ := td.imp.ImportFormats()
		good := app.NewSystemBuffer(t, 64, 32, fourccAR24, 0, 256)
		tiled := app.NewBuffer(t, 64, 32, own[0])
		edit := func(b *gputest.Buffer, f func(d *Dmabuf)) Dmabuf {
			d := dmabuf(b)
			d.Planes = slices.Clone(d.Planes)
			f(&d)
			return d
		}
		memfd := memfd(t, 1<<20)
		base := processFDs()
		for _, c := range []struct {
			name string
			d    Dmabuf
			want string // "gpu", "mapped", "unsupported" or "wrong"
		}{
			{"linear", dmabuf(good), "gpu"},
			{"tiled", dmabuf(tiled), "gpu"},
			{"rows of no whole pixels", edit(good, func(d *Dmabuf) { d.Planes[0].Offset = 2 }), "mapped"},
			{"an odd stride", edit(good, func(d *Dmabuf) { d.Planes[0].Stride, d.Height = 258, 8 }), "mapped"},
			{"an unknown modifier", edit(tiled, func(d *Dmabuf) { d.Modifier = 0x7f00000000000123 }), "unsupported"},
			{"NV12", edit(good, func(d *Dmabuf) { d.Fourcc = 0x3231564e }), "unsupported"},
			{"a plane more", edit(good, func(d *Dmabuf) { d.Planes = append(d.Planes, d.Planes[0]) }), "wrong"},
			{"a stride shorter than a row", edit(good, func(d *Dmabuf) { d.Planes[0].Stride = 128 }), "wrong"},
			{"rows beyond the memory", edit(good, func(d *Dmabuf) { d.Height = 4096 }), "wrong"},
			{"tiled rows beyond the memory", edit(tiled, func(d *Dmabuf) { d.Height = 16384 }), "wrong"},
			{"no dmabuf", edit(good, func(d *Dmabuf) { d.Planes[0].FD = memfd }), "wrong"},
			{"a closed descriptor", edit(good, func(d *Dmabuf) { d.Planes[0].FD = 1 << 20 }), "wrong"},
		} {
			b, err := a.tex.Import(c.d)
			got := "wrong"
			switch {
			case err == nil && b.content != nil:
				got = "gpu"
			case err == nil:
				got = "mapped"
			case errors.Is(err, ErrTextureUnsupported):
				got = "unsupported"
			}
			if got != c.want {
				t.Errorf("%s: %s (%v), want %s", c.name, got, err, c.want)
			}
			if err != nil && !strings.HasPrefix(err.Error(), "tela: Texture.Import: ") {
				t.Errorf("%s: %v", c.name, err)
			}
			if err == nil {
				b.Close()
			} else {
				t.Logf("%s: %v", c.name, err)
			}
		}
		// The device's own failures are errors, whatever the memory; a lost
		// one is as no device.
		td.importErr = errors.New("out of memory")
		if _, err := a.tex.Import(dmabuf(good)); err == nil || errors.Is(err, ErrTextureUnsupported) {
			t.Errorf("a failure of the device: %v", err)
		}
		td.importErr = fmt.Errorf("%w: gone", gpu.ErrDeviceLost)
		td.lost.Store(true)
		if b, err := a.tex.Import(dmabuf(good)); err != nil || b.mem == nil {
			t.Errorf("linear memory on a lost device: %v", err)
		} else {
			b.Close()
		}
		if _, err := a.tex.Import(dmabuf(tiled)); !errors.Is(err, ErrTextureUnsupported) {
			t.Errorf("tiled memory on a lost device: %v", err)
		}
		td.importErr = nil
		td.lost.Store(false)
		if got := processFDs(); got != base {
			t.Errorf("%d descriptors open after the buffers closed, %d before", got, base)
		}

		// From the UI thread.
		var got []*TextureBuffer
		imp := func() {
			b, err := a.tex.Import(dmabuf(good))
			if err != nil {
				t.Error(err)
				return
			}
			got = append(got, b)
		}
		a.tex.OnResize(func(TextureSize) { imp() })
		x.main().Resize(platform.Size{W: 330, H: 250})
		x.step("a resize")
		a.mode.Set("gone")
		x.settle("the element gone")
		a.mode.Set("")
		x.settle("the element back")
		a.during = imp
		a.closed.Set(true)
		x.settle("closed")
		a.closed.Set(false)
		x.settle("opened again") // a new texture, whose effect imports
		if len(got) < 3 {
			t.Errorf("%d imports on the UI thread", len(got))
		}
		for _, b := range got {
			if b.content == nil {
				t.Error("an import on the UI thread was not the GPU's")
			}
		}
		x.c.finish(x.h)
		texIdle(t, a.tex)
	})
}

// TestTextureGPUFallback makes the window of imported buffers fall back to
// the CPU rasterizer: its frames go on, read back through the device, tiled
// ones too, and memory imported from then on is mapped.
func TestTextureGPUFallback(t *testing.T) {
	gputest.EachPair(t, true, func(t *testing.T, app *gputest.App) {
		win := app.Window(t)
		var a texCardApp
		x, td := newTexRig(t, win, a.root(), 320, 240, 2, nil)
		x.settle("mount")
		own, _ := td.imp.ImportFormats()
		p := &texProducer{t: t, tex: a.tex, spoil: true}
		for range 2 {
			p.addBuffer(app.NewBuffer(t, 120, 80, own[0]))
			p.addBuffer(app.NewSystemBuffer(t, 120, 80, fourccAB24, 4, 4*120))
		}
		show := func(what string, i int, c color.RGBA) {
			t.Helper()
			waitFor(t, "a free buffer", p.bufs[i].free.Load)
			p.submit(p.bufs[i], solid(120, 80, c), nil)
			texIdle(t, a.tex)
			x.step(what)
			if got := a.center(x); got != c {
				t.Errorf("%s: shows %v, want %v", what, got, c)
			}
		}
		show("by the GPU", 0, texRed)
		x.c.fake.fail = func(op string) error {
			x.c.fake.fail = nil
			return errInjected
		}
		show("the frame that fails", 1, texGreen)
		if !x.view(x.main()).gpu.off || x.h.GPU() == nil {
			t.Fatal("the window did not fall back alone")
		}
		for n := range 4 {
			show(fmt.Sprintf("by the CPU %d", n), n, []color.RGBA{texBlue, texRed}[n%2])
		}
		for i, b := range p.bufs {
			if n := b.tb.content.(*gpuTexture).reads.Load(); n == 0 {
				t.Errorf("buffer %d was not read back", i)
			}
		}
		late, err := a.tex.Import(dmabuf(app.NewSystemBuffer(t, 120, 80, fourccAB24, 0, 4*120)))
		if err != nil || late.mem == nil {
			t.Errorf("imported after the fallback: %v", err)
		}
		if _, err := a.tex.Import(dmabuf(app.NewBuffer(t, 120, 80, own[0]))); !errors.Is(err, ErrTextureUnsupported) {
			t.Errorf("tiled memory imported after the fallback: %v", err)
		}
		x.c.finish(x.h)
		texIdle(t, a.tex)
		if p.released.Load() != p.submitted.Load() || p.double.Load() {
			t.Errorf("%d frames submitted, %d released", p.submitted.Load(), p.released.Load())
		}
	})
}

// TestTextureGPULost loses the device in the frame that draws an imported
// buffer, and between two frames (an import or a read found it out), with
// textures on screen and unmounting: nothing is released, and no buffer is
// closed, before the device has closed; linear buffers go on, mapped, tiled
// ones are refused with an error that says so, and nothing crashes; another
// device takes its place, and all of it works again.
func TestTextureGPULost(t *testing.T) {
	gputest.EachPair(t, true, func(t *testing.T, app *gputest.App) {
		for _, how := range []string{"draw", "draw_direct", "draw_tiled", "quiet", "quiet_unmount", "quiet_quit", "quiet_gone"} {
			t.Run(how, func(t *testing.T) {
				win := app.Window(t)
				var a texCardApp
				var dm *dmabufConfig
				if how == "draw_direct" {
					dm = &dmabufWayland
				}
				x, td := newTexRig(t, win, a.root(), 320, 240, 1, dm)
				x.settle("mount")
				own, _ := td.imp.ImportFormats()
				p := &texProducer{t: t, tex: a.tex}
				for range 3 {
					p.addBuffer(app.NewSystemBuffer(t, 120, 80, fourccAB24, 0, (4*120+255)&^255))
				}
				tiled := app.NewBuffer(t, 120, 80, own[0])
				tb := p.addBuffer(tiled)
				submit := func(i int, c color.RGBA) {
					t.Helper()
					waitFor(t, "a free buffer", p.bufs[i].free.Load)
					p.submit(p.bufs[i], solid(120, 80, c), nil)
				}
				show := func(what string, i int, c color.RGBA) {
					t.Helper()
					submit(i, c)
					texIdle(t, a.tex)
					x.b.Advance(x.b.FrameInterval())
					x.deliver()
					if got := a.center(x); got != c {
						t.Errorf("%s: shows %v, want %v", what, got, c)
					}
				}
				show("tiled", 3, texGreen)
				show("linear", 0, texRed)
				texIdle(t, a.tex)
				released := p.released.Load()
				closed := false
				td.onClose = func() {
					if closed {
						return
					}
					closed = true
					// Whatever was let go meanwhile has its Release called by
					// now.
					time.Sleep(20 * time.Millisecond)
					if got := p.released.Load(); got != released {
						t.Errorf("%d frames were released before the device closed", got-released)
					}
					if n := len(a.tex.bufs); n != 4 && how == "draw" {
						t.Errorf("%d buffers are open", n)
					}
				}
				tex := a.tex
				switch how {
				case "draw", "draw_direct", "draw_tiled":
					x.c.fake.onDraw = func(f *gpu.Frame, draw func() error) error {
						x.c.fake.onDraw = nil
						if len(f.Imports) == 0 && (len(f.Images) == 0 || f.Images[0].From == nil) {
							t.Error("the frame that loses the device reads no imported memory")
						}
						// The frame before was let go of, which no draw in
						// flight reads.
						texIdle(t, a.tex)
						if released++; p.released.Load() != released {
							t.Errorf("%d frames released before the draw, want %d", p.released.Load(), released)
						}
						if err := draw(); err != nil {
							return err
						}
						td.lost.Store(true)
						return fmt.Errorf("%w: %w", gpu.ErrDeviceLost, errInjected)
					}
					i := 1
					if how == "draw_tiled" {
						i = 3 // nothing reads it without the device
					}
					submit(i, texBlue) // drawn by the GPU, which may read it still
					x.b.Advance(x.b.FrameInterval())
					x.deliver()
					if got := a.center(x); (got == texBlue) != (i == 1) || got.A != 255 || got == (color.RGBA{0, 0, 0, 255}) {
						t.Errorf("the frame that lost the device shows %v", got)
					}
				case "quiet":
					td.lost.Store(true)
					show("the frame after the loss", 1, texBlue) // lets go of 0
				case "quiet_unmount":
					td.lost.Store(true)
					a.closed.Set(true)
					x.b.Advance(x.b.FrameInterval())
					x.deliver()
				case "quiet_gone":
					td.lost.Store(true)
					a.mode.Set("gone")
					x.b.Advance(x.b.FrameInterval())
					x.deliver()
				case "quiet_quit":
					td.lost.Store(true)
					x.h.end()
				}
				if !closed || x.h.GPU() != nil {
					t.Fatalf("the device closed: %v", closed)
				}
				if n := strings.Count(x.log.String(), "\n"); n != 1 || !strings.Contains(x.log.String(), "lost") {
					t.Errorf("the log says %q", x.log)
				}
				if how == "quiet_unmount" || how == "quiet_quit" {
					texIdle(t, tex)
					waitFor(t, "every Release", func() bool { return p.released.Load() == p.submitted.Load() })
					if p.double.Load() {
						t.Error("a frame was released twice")
					}
					if how == "quiet_unmount" {
						x.h.end()
					}
					return
				}
				a.mode.Set("")
				x.b.Advance(x.b.FrameInterval())
				x.deliver()

				// Without the device.
				if d := tex.Device(); d.GPU || !slices.Equal(d.Formats, mappedFormats()) {
					t.Errorf("Device() = %+v", d)
				}
				for n := range 6 {
					i := slices.IndexFunc(p.bufs[:3], func(b *texBuf) bool { return b.free.Load() })
					if i < 0 {
						t.Fatal("no buffer is free")
					}
					show(fmt.Sprintf("without the device %d", n), i, []color.RGBA{texRed, texGreen, texBlue}[n%3])
					if n := p.bufs[i].tb.content.(*gpuTexture).reads.Load(); n > 2 {
						t.Errorf("the frames of a mapped buffer are read %d times as they are drawn", n)
					}
				}
				waitFor(t, "a free buffer", tb.free.Load)
				var rel texReleases
				err := tex.Submit(tb.tb, TextureFrame{Release: rel.fn("refused")})
				if !errors.Is(err, ErrTextureUnsupported) || !strings.HasPrefix(err.Error(), "tela: Texture.Submit: ") {
					t.Errorf("a tiled buffer submitted without its device: %v", err)
				}
				t.Log(err)
				if got := rel.got(t, tex); got != "refused" {
					t.Errorf("released %q", got)
				}
				f := newChanFence()
				if err := tex.Submit(tb.tb, TextureFrame{Fence: f, Release: rel.fn("fence")}); err == nil || rel.got(t, tex) != "fence" {
					t.Errorf("with a fence: %v", err)
				}
				x.b.Advance(x.b.FrameInterval())
				x.deliver()
				if got := a.center(x); got != texBlue {
					t.Errorf("after a refused frame shows %v, want the last one", got)
				}
				if tex.Stats().Shown == 0 {
					t.Error("nothing shown")
				}

				// Another device: the buffers of the first one go on, and new
				// ones are its.
				win2 := app.Window(t)
				x.c.closeRef()
				x.c.fake = &fakeDevice{Device: win2}
				if x.d != nil {
					x.d.dev = x.c.fake
					x.d.setFormats(dm.fourccs)
				}
				td2 := texGPU(x.c, x.h, win2)
				if d := tex.Device(); !d.GPU {
					t.Errorf("Device() = %+v", d)
				}
				show("old buffers, on another device", slices.IndexFunc(p.bufs[:3], func(b *texBuf) bool { return b.free.Load() }), texGreen)
				if !x.h.Tree().lay.rasGPU {
					t.Error("the window is not drawn by the new device")
				}
				p2 := &texProducer{t: t, tex: tex}
				nb := p2.addBuffer(app.NewBuffer(t, 120, 80, own[0]))
				if howRead(nb.tb) != texSampled || td2.imports.Load() != 1 {
					t.Error("the new buffer is not the new device's")
				}
				p2.submit(nb, solid(120, 80, texBlue), nil)
				x.b.Advance(x.b.FrameInterval())
				x.deliver()
				if got := a.center(x); got != texBlue || nb.tb.content.(*gpuTexture).reads.Load() != 0 {
					t.Errorf("a buffer of the new device shows %v", got)
				}
				x.c.finish(x.h)
				texIdle(t, tex)
				if p.released.Load() != p.submitted.Load() || p.double.Load() {
					t.Errorf("%d frames submitted, %d released", p.submitted.Load(), p.released.Load())
				}
			})
		}
	})
}

// childWindow is a small window.
func childWindow(content Node) Node {
	return WithKey(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "other", Width: 260, Height: 180}, Root: content}), "other")
}

// TestTextureGPUShownTwice shows a texture in two windows, one of which falls
// back to the CPU rasterizer and is then hidden: a frame is released once
// neither draws it.
func TestTextureGPUShownTwice(t *testing.T) {
	gputest.EachPair(t, true, func(t *testing.T, app *gputest.App) {
		win := app.Window(t)
		var tex *Texture
		root := Component(func() Node {
			tex = UseTexture()
			return div(Class("h-screen bg-white p-2"), texture(tex, Class("h-20 w-32")),
				childWindow(div(Class("h-screen bg-slate-100 p-2"), texture(tex, Class("h-24 w-40 rounded-xl")))))
		})
		x, td := newTexRig(t, win, root, 320, 240, 1, nil)
		x.settle("mount")
		own, _ := td.imp.ImportFormats()
		l := &texLedger{t: t}
		p := &texProducer{t: t, tex: tex, spoil: true}
		l.ps = []*texProducer{p}
		for range 3 {
			p.addBuffer(app.NewBuffer(t, 100, 60, own[0]))
		}
		x.c.fake.onDraw = l.draw
		for n := range 8 {
			if !p.frame(n, nil) {
				t.Fatalf("frame %d: no buffer is free", n)
			}
			x.settle(fmt.Sprintf("frame %d", n))
			texIdle(t, tex)
		}
		// The child falls back: drawn by two renderers.
		child := x.b.Windows()[1]
		x.view(child).gpu.off = true
		for n := range 4 {
			if !p.frame(n, nil) {
				t.Fatalf("frame %d: no buffer is free", n)
			}
			x.settle(fmt.Sprintf("frame %d, one window on the CPU", n))
			texIdle(t, tex)
		}
		if l.draws == 0 || p.released.Load() != p.submitted.Load()-1 {
			t.Errorf("%d draws; %d frames submitted, %d released", l.draws, p.submitted.Load(), p.released.Load())
		}
		x.c.finish(x.h)
		texIdle(t, tex)
		if p.released.Load() != p.submitted.Load() || p.double.Load() {
			t.Errorf("%d frames submitted, %d released", p.submitted.Load(), p.released.Load())
		}
	})
}

// TestTextureGPUGoroutines imports, submits and closes buffers from
// goroutines while the window draws, resizes and quits: an OnResize that
// waits for the goroutine that imports does not wait for itself, no import
// hangs, and nothing is left. For the race detector.
func TestTextureGPUGoroutines(t *testing.T) {
	gputest.EachPair(t, false, func(t *testing.T, app *gputest.App) {
		for _, end := range []string{"unmount", "quit", "lost"} {
			t.Run(end, func(t *testing.T) {
				win := app.Window(t)
				var a texCardApp
				x, td := newTexRig(t, win, a.root(), 320, 240, 1, nil)
				x.settle("mount")
				tex := a.tex
				own, _ := td.imp.ImportFormats()
				shapes := []Dmabuf{dmabuf(app.NewSystemBuffer(t, 64, 32, fourccAB24, 0, 256)), dmabuf(app.NewSystemBuffer(t, 60, 32, fourccXR24, 4, 4*60+4))}
				if app.Same {
					shapes = append(shapes, dmabuf(app.NewBuffer(t, 64, 32, own[0])))
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var wg sync.WaitGroup
				var calls, released atomic.Int64
				for g := range 4 {
					rng := rand.New(rand.NewPCG(uint64(g), 7))
					wg.Go(func() {
						for ctx.Err() == nil {
							b, err := tex.Import(shapes[rng.IntN(len(shapes))])
							if errors.Is(err, ErrTextureClosed) {
								return
							}
							if err != nil {
								if end != "lost" || !errors.Is(err, ErrTextureUnsupported) {
									t.Error(err)
									return
								}
								continue
							}
							tex.Device()
							for range rng.IntN(3) {
								calls.Add(1)
								tex.Submit(b, TextureFrame{Release: func() { released.Add(1) }})
							}
							if rng.IntN(3) > 0 {
								b.Close()
							}
							time.Sleep(time.Duration(rng.IntN(300)) * time.Microsecond)
						}
					})
				}
				// The renderer: OnResize hands it the size and waits until it
				// takes it, and it imports meanwhile.
				sizes := make(chan TextureSize)
				tex.OnResize(func(s TextureSize) {
					select {
					case sizes <- s:
					case <-time.After(20 * time.Second):
						t.Error("the goroutine that imports waits for the UI thread")
					}
				})
				wg.Go(func() {
					var bufs []*TextureBuffer
					for {
						select {
						case <-ctx.Done():
							return
						case <-sizes:
						}
						for _, b := range bufs {
							b.Close()
						}
						bufs = bufs[:0]
						for range 3 {
							if b, err := tex.Import(shapes[0]); err == nil {
								bufs = append(bufs, b)
							}
						}
					}
				})
				n := 60
				if testing.Short() {
					n = 25
				}
				for i := range n {
					if i%3 == 0 {
						mode := "gone"
						if i%2 == 0 {
							mode = ""
						}
						a.mode.Set(mode)
					}
					x.b.Advance(x.b.FrameInterval())
					x.deliver()
					time.Sleep(time.Millisecond)
				}
				switch end {
				case "lost":
					a.mode.Set("")
					td.lost.Store(true)
					for range 10 {
						x.b.Advance(x.b.FrameInterval())
						x.deliver()
						time.Sleep(time.Millisecond)
					}
					if x.h.GPU() != nil {
						t.Error("a lost GPU was not closed")
					}
					x.h.end()
				case "quit":
					x.h.Quit()
					x.h.end()
				default:
					a.closed.Set(true)
					x.settle("unmount")
				}
				time.Sleep(5 * time.Millisecond)
				tex.OnResize(nil)
				cancel()
				wg.Wait()
				texIdle(t, tex)
				waitFor(t, "every Release", func() bool { return released.Load() == calls.Load() })
				if end == "unmount" {
					x.h.end()
				}
			})
		}
	})
}

// A frame whose device went away while its fence was waited for is copied
// from the mapping, and dropped when nothing reads it.
func TestTextureGPULostWhileWaiting(t *testing.T) {
	gputest.EachPair(t, true, func(t *testing.T, app *gputest.App) {
		win := app.Window(t)
		var a texCardApp
		x, td := newTexRig(t, win, a.root(), 320, 240, 1, nil)
		x.settle("mount")
		own, _ := td.imp.ImportFormats()
		p := &texProducer{t: t, tex: a.tex}
		p.addBuffer(app.NewSystemBuffer(t, 120, 80, fourccAB24, 0, (4*120+255)&^255))
		p.addBuffer(app.NewBuffer(t, 120, 80, own[0]))
		f0, f1 := newChanFence(), newChanFence()
		p.submit(p.bufs[0], solid(120, 80, texBlue), f0)
		p.submit(p.bufs[1], solid(120, 80, texRed), f1)
		td.lost.Store(true)
		f1.ch <- nil // newer, and unreadable: dropped, the older one stays
		waitFor(t, "the tiled frame to be dropped", func() bool { return a.tex.Stats().Dropped == 1 })
		f0.ch <- nil
		texIdle(t, a.tex)
		x.b.Advance(x.b.FrameInterval())
		x.deliver()
		if got := a.center(x); got != texBlue {
			t.Errorf("shows %v, want the linear frame", got)
		}
		if !bytes.Contains(x.log.Bytes(), []byte("lost")) {
			t.Errorf("the log says %q", x.log)
		}
		x.h.end()
		texIdle(t, a.tex)
		if p.released.Load() != 2 || p.double.Load() {
			t.Errorf("%d frames released", p.released.Load())
		}
	})
}
