package core

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
)

func texture(x *Texture, c ...Child) Node {
	return Primitive(style.KindTexture, append([]Child{TextureSource(x)}, c...)...)
}

// solid returns a w x h image of one color.
func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

var (
	texRed   = color.RGBA{255, 0, 0, 255}
	texGreen = color.RGBA{0, 255, 0, 255}
	texBlue  = color.RGBA{0, 0, 255, 255}
)

// texReleases records the Release calls of a test, by name, in order.
type texReleases struct {
	mu    sync.Mutex
	names []string
}

func (r *texReleases) fn(name string) func() {
	return func() {
		r.mu.Lock()
		r.names = append(r.names, name)
		r.mu.Unlock()
	}
}

// got waits until the release goroutine of x has nothing left and returns
// what it ran since the last call.
func (r *texReleases) got(t *testing.T, x *Texture) string {
	t.Helper()
	waitFor(t, "the release goroutine to end", func() bool {
		x.mu.Lock()
		defer x.mu.Unlock()
		return !x.rqOn && len(x.rq) == 0
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	out := strings.Join(r.names, " ")
	r.names = nil
	return out
}

// waitFor polls cond, which another goroutine makes true.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(20 * time.Second); !cond(); {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Microsecond)
	}
}

// chanFence is a Fence a test signals: nil, or an error.
type chanFence struct {
	ch     chan error
	mu     sync.Mutex
	waits  int
	closed int
	cut    bool // its context was cancelled
}

func newChanFence() *chanFence { return &chanFence{ch: make(chan error, 1)} }

func (f *chanFence) Wait(ctx context.Context) error {
	f.mu.Lock()
	f.waits++
	f.mu.Unlock()
	select {
	case err := <-f.ch:
		return err
	case <-ctx.Done():
		f.mu.Lock()
		f.cut = true
		f.mu.Unlock()
		return ctx.Err()
	}
}

func (f *chanFence) closeFence() {
	f.mu.Lock()
	f.closed++
	f.mu.Unlock()
}

func (f *chanFence) state() (waits, closed int, cut bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.waits, f.closed, f.cut
}

// texApp is a window that shows a texture below a header, with a state that
// takes it away.
type texApp struct {
	tex    *Texture
	class  string
	mode   Setter[string] // "", "none" (display: none), "gone" (unmounted)
	closed Setter[bool]   // unmounts the component with its UseTexture
}

func (a *texApp) root() Node {
	inner := func() Node {
		mode, set := UseState("")
		a.mode = set
		a.tex = UseTexture()
		class := a.class
		if class == "" {
			class = "w-40 h-20 bg-black"
		}
		kids := []Child{Class("p-4"), div(Class("h-8"), Text("header"))}
		switch mode {
		case "none":
			kids = append(kids, texture(a.tex, Class(class+" hidden")))
		case "gone":
		default:
			kids = append(kids, texture(a.tex, Class(class)))
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

// expectIdle fails unless the window asks for nothing: no frame, no update,
// no timer.
func (x *winHarness) expectIdle(what string) {
	x.t.Helper()
	if x.b.Pending() || x.h.Tree().Dirty() {
		x.t.Fatalf("%s: the app is not idle: %+v", what, x.h.Tree().Pending(x.b.Now()))
	}
	if at, ok := x.h.TimerArmed(); ok {
		// The trim of the rasterizer's memory (one timer after the last
		// frame) is not work.
		if at > x.b.Now()+2*RasterSettle {
			x.t.Fatalf("%s: a timer is armed for %v", what, at)
		}
	}
}

func (x *winHarness) pixel(px, py int) color.RGBA {
	x.t.Helper()
	img := x.windows()[0].Frame()
	if img == nil {
		x.t.Fatal("nothing presented")
	}
	return img.RGBAAt(px, py)
}

// TestTextureShowsLatest shows frames, the last one submitted before each
// frame of the window, and asks for nothing while none arrives.
func TestTextureShowsLatest(t *testing.T) {
	var a texApp
	var rel texReleases
	x := newWinHarness(t, Options{Root: a.root()})
	// The element is at (16, 48), 160x80.
	if c := x.pixel(60, 80); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("before the first frame the element shows %v, want its background", c)
	}
	x.expectIdle("before the first frame")
	presents := x.h.Presents()
	if err := a.tex.SubmitImage(solid(8, 4, texRed), rel.fn("red")); err != nil {
		t.Fatal(err)
	}
	x.drain()
	if c := x.pixel(60, 80); c != texRed {
		t.Errorf("the element shows %v, want red", c)
	}
	if n := x.h.Presents() - presents; n != 1 {
		t.Errorf("%d frames presented for a frame of the texture, want 1", n)
	}
	x.expectIdle("after a frame")
	if err := errors.Join(a.tex.SubmitImage(solid(8, 4, texGreen), rel.fn("green")), a.tex.SubmitImage(solid(8, 4, texBlue), rel.fn("blue"))); err != nil {
		t.Fatal(err)
	}
	if got := rel.got(t, a.tex); got != "green" {
		t.Errorf("released %q after two submissions between frames, want green: the latest wins", got)
	}
	x.drain()
	if c := x.pixel(60, 80); c != texBlue {
		t.Errorf("the element shows %v, want blue", c)
	}
	if got := rel.got(t, a.tex); got != "red" {
		t.Errorf("released %q once blue is shown, want red", got)
	}
	st := a.tex.Stats()
	if st.Submitted != 3 || st.Shown != 2 || st.Dropped != 1 || st.Age <= 0 || st.MaxAge < st.Age {
		t.Errorf("stats %+v", st)
	}
	x.expectIdle("at the end")
	expectFullRender(t, x.windows()[0], x.h.Tree())
}

// A frame of a texture builds, styles and lays out nothing, places nothing,
// records one list and damages the element alone.
func TestTextureFrameDamagesOnlyTheElement(t *testing.T) {
	a := texApp{class: "w-40 h-20 bg-black p-2"}
	x := newWinHarness(t, Options{Root: a.root()})
	tr := x.h.Tree()
	for i, c := range []color.RGBA{texRed, texGreen, texBlue} {
		// Other sizes and ratios change nothing either.
		if err := a.tex.SubmitImage(solid(10+i, 5+7*i, c), nil); err != nil {
			t.Fatal(err)
		}
		before := tr.Stats()
		x.drain()
		after := tr.Stats()
		if i > 0 && tr.lay.recorded != 1 {
			t.Errorf("frame %d recorded %d lists, want the element's group alone", i, tr.lay.recorded)
		}
		want := before
		want.Frames++
		want.Paints++
		if after != want {
			t.Errorf("frame %d did %+v, want only a frame and a paint over %+v", i, after, before)
		}
		// The content box, 160x80 at (16, 48) without 8 px of padding, and
		// the pixel around it that every command's bounds have.
		d := tr.LastDamage()
		if d.Full || len(d.Shifts) > 0 || !slices.Equal(d.Rects, []image.Rectangle{image.Rect(23, 55, 169, 121)}) {
			t.Errorf("frame %d damaged %+v, want the content box and a pixel around it", i, d)
		}
	}
	expectFullRender(t, x.windows()[0], tr)
}

// A fast producer wakes the window once per frame, and allocates no spare
// beyond the bound.
func TestTextureFastProducer(t *testing.T) {
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	tr := x.h.Tree()
	for range 5 {
		for range 200 {
			if err := a.tex.SubmitImage(solid(16, 8, texRed), nil); err != nil {
				t.Fatal(err)
			}
		}
		tr.mu.Lock()
		n := len(tr.updates)
		tr.mu.Unlock()
		if n != 1 {
			t.Fatalf("200 submissions queued %d wakes, want 1", n)
		}
		x.drain()
	}
	st := a.tex.Stats()
	if st.Submitted != 1000 || st.Shown != 5 || st.Dropped != 995 {
		t.Errorf("stats %+v", st)
	}
	a.tex.mu.Lock()
	spare := len(a.tex.spare)
	a.tex.mu.Unlock()
	if spare > texMaxSpare {
		t.Errorf("%d spare buffers", spare)
	}
	if n := len(tr.lay.dmg.opaque); n != 0 {
		t.Errorf("the damage remembers the opacity of %d images: the frames state theirs", n)
	}
	x.expectIdle("after the producer stopped")
	// At rest, the spare pixels go.
	if tr.textureSpareBytes() == 0 {
		t.Error("a producer of copies keeps no spare")
	}
	x.b.Advance(2 * RasterSettle)
	x.b.Wake()
	x.drain()
	if n := tr.textureSpareBytes(); n != 0 {
		t.Errorf("%d bytes of spare pixels at rest", n)
	}
	x.expectIdle("at rest")
}

// While a texture is not on screen it wakes nothing: the latest frame waits,
// the others are released, and it is shown as soon as the element is.
func TestTextureOffScreen(t *testing.T) {
	for _, tc := range []struct {
		name       string
		hide, show func(x *winHarness, a *texApp, w WindowHandle, ref ElementRef)
	}{
		{"unmounted",
			func(_ *winHarness, a *texApp, _ WindowHandle, _ ElementRef) { a.mode.Set("gone") },
			func(_ *winHarness, a *texApp, _ WindowHandle, _ ElementRef) { a.mode.Set("") }},
		{"display none",
			func(_ *winHarness, a *texApp, _ WindowHandle, _ ElementRef) { a.mode.Set("none") },
			func(_ *winHarness, a *texApp, _ WindowHandle, _ ElementRef) { a.mode.Set("") }},
		{"scrolled out",
			func(_ *winHarness, _ *texApp, _ WindowHandle, ref ElementRef) { ref.ScrollTo(0, 1000) },
			func(_ *winHarness, _ *texApp, _ WindowHandle, ref ElementRef) { ref.ScrollTo(0, 0) }},
		{"hidden window",
			func(_ *winHarness, _ *texApp, w WindowHandle, _ ElementRef) { w.Hide() },
			func(_ *winHarness, _ *texApp, w WindowHandle, _ ElementRef) { w.Show() }},
		{"minimized window",
			func(_ *winHarness, _ *texApp, w WindowHandle, _ ElementRef) { w.Minimize() },
			func(_ *winHarness, _ *texApp, w WindowHandle, _ ElementRef) { w.Focus() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var a texApp
			var rel texReleases
			var w WindowHandle
			var ref ElementRef
			var setN Setter[int]
			content := a.root()
			x := newWinHarness(t, Options{Config: Config{InstantScroll: true}, Root: Component(func() Node {
				w, ref = UseWindow(), UseElementRef()
				n, set := UseState(0)
				setN = set
				return div(AttachRef(ref), Class("h-screen overflow-y-auto"), content, div(Class("h-[2000px]"), Textf("n=%d", n)))
			})})
			tr := x.h.Tree()
			submit := func(name string, c color.RGBA) {
				t.Helper()
				if err := a.tex.SubmitImage(solid(8, 4, c), rel.fn(name)); err != nil {
					t.Fatal(err)
				}
			}
			submit("first", texRed)
			x.drain()
			tc.hide(x, &a, w, ref)
			x.drain()
			x.drain() // the scrollbars, the windows' states
			rel.got(t, a.tex)
			presents, frames := x.h.Presents(), tr.Stats().Frames
			wakes := 0
			tr.SetWake(func() { wakes++; x.b.Wake() })
			for i, name := range []string{"a", "b", "c", "d"} {
				submit(name, []color.RGBA{texGreen, texBlue, texGreen, texBlue}[i])
				x.b.Drain(x.h)
			}
			if wakes != 0 {
				t.Errorf("%d wakes while the texture was not on screen", wakes)
			}
			if x.h.Presents() != presents || tr.Stats().Frames != frames {
				t.Errorf("%d presents and %d frames while the texture was not on screen", x.h.Presents()-presents, tr.Stats().Frames-frames)
			}
			if got := rel.got(t, a.tex); got != "a b c" {
				t.Errorf("released %q, want the older ones: a b c", got)
			}
			// A commit of a hidden window presents nothing, and takes no
			// frame.
			if strings.Contains(tc.name, "window") {
				shown := a.tex.Stats().Shown
				setN.Set(1)
				x.drain()
				if got := a.tex.Stats().Shown; got != shown {
					t.Errorf("a window without frames took a frame of the texture")
				}
			}
			tc.show(x, &a, w, ref)
			x.b.Drain(x.h)
			x.b.Advance(x.b.FrameInterval())
			x.b.Drain(x.h)
			if c := x.pixel(60, 80); c != texBlue {
				t.Errorf("shown again, the element shows %v, want the latest frame", c)
			}
			x.drain()
			if got := rel.got(t, a.tex); got != "first" {
				t.Errorf("released %q once the latest is shown, want first", got)
			}
			expectFullRender(t, x.windows()[0], tr)
		})
	}
}

// Frames submitted before the element mounts are shown at its first frame,
// and a frame taken by a tree without a window too.
func TestTextureSubmitBeforeMount(t *testing.T) {
	root := Component(func() Node {
		x := UseTexture()
		UseLayoutEffect(func() func() {
			if err := x.SubmitImage(solid(4, 4, texGreen), nil); err != nil {
				t.Error(err)
			}
			return nil
		}, Once)
		return texture(x)
	})
	_, img := Render(root, RenderOptions{Config: Config{Fonts: testFonts}, Width: 400, Height: 300})
	for _, p := range []image.Point{{0, 0}, {299, 149}} {
		if c := img.RGBAAt(p.X, p.Y); c != texGreen {
			t.Errorf("pixel %v is %v, want the frame: a texture is 300x150 by default", p, c)
		}
	}
	for _, p := range []image.Point{{300, 10}, {10, 150}} {
		if c := img.RGBAAt(p.X, p.Y); c == texGreen {
			t.Errorf("pixel %v is the frame's: a texture is 300x150 by default", p)
		}
	}
}

// The layout of a texture is a canvas's.
func TestTextureLayout(t *testing.T) {
	for _, tc := range []struct {
		class string
		w, h  float32
	}{
		{"", 300, 150},
		{"w-full", 400, 200},
		{"w-100", 400, 200},
		{"h-20", 160, 80},
		{"w-10 h-10", 40, 40},
		{"w-full aspect-video", 400, 225},
		{"w-full h-32", 400, 128},
		{"inline", 300, 150},
	} {
		var a texApp
		a.class = tc.class
		if a.class == "" {
			a.class = "block"
		}
		var ref ElementRef
		tr := NewTree(Config{Fonts: testFonts})
		tr.SetViewport(gfx.Size{W: 400, H: 300})
		tr.Mount(Component(func() Node {
			ref = UseElementRef()
			return texture(UseTexture(), AttachRef(ref), Class(a.class))
		}))
		tr.Frame()
		if b := ref.Bounds(); b.Width != tc.w || b.Height != tc.h {
			t.Errorf("%q is %gx%g, want %gx%g", tc.class, b.Width, b.Height, tc.w, tc.h)
		}
	}
}

// A frame of Size's pixels is drawn pixel for pixel, at any scale and
// position.
func TestTexturePixelExact(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2, 2.4} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var tex *Texture
			b := headless.New(headless.Options{Size: platform.Size{W: 211, H: 157}, Scale: scale})
			h, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: Component(func() Node {
				tex = UseTexture()
				return div(Class("p-[7.3px]"), texture(tex, Class("w-[101.7px] h-[63.2px] border border-red-500 p-[3.1px]")))
			})})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(h.stopTimer)
			b.Drain(h)
			size := tex.Size()
			if size.Scale != float32(scale) || size.Width <= 0 || size.Height <= 0 {
				t.Fatalf("size %+v", size)
			}
			// Noise: any resampling shows.
			src := image.NewRGBA(image.Rect(0, 0, size.Width, size.Height))
			for i := range src.Pix {
				src.Pix[i] = byte(i*i*31 + i*17)
				if i%4 == 3 {
					src.Pix[i] = 255
				}
			}
			if err := tex.SubmitImage(src, nil); err != nil {
				t.Fatal(err)
			}
			b.Advance(b.FrameInterval())
			b.Drain(h)
			// Where it is: the DrawImage of the frame.
			tr := h.Tree()
			var at image.Point
			found := false
			dl := tr.LastFrame()
			for i := range dl.Len() {
				if c, ok := dl.At(i).(gfx.DrawImage); ok {
					found = true
					at = image.Pt(int(gfx.RoundDevice(float64(c.Dst.X0)*scale)), int(gfx.RoundDevice(float64(c.Dst.Y0)*scale)))
					if w, h := gfx.RoundDevice(float64(c.Dst.Width())*scale), gfx.RoundDevice(float64(c.Dst.Height())*scale); int(w) != size.Width || int(h) != size.Height {
						t.Errorf("drawn %gx%g device pixels, Size says %dx%d", w, h, size.Width, size.Height)
					}
				}
			}
			if !found {
				t.Fatal("no DrawImage in the frame")
			}
			got := h.Window().(*headless.Window).Frame()
			for y := range size.Height {
				for x := range size.Width {
					if g, w := got.RGBAAt(at.X+x, at.Y+y), src.RGBAAt(x, y); g != w {
						t.Fatalf("pixel (%d, %d) of the frame is %v on screen, %v in the frame", x, y, g, w)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------
// Release.

// Every Submit call with a Release gets it called once, on another
// goroutine, also on an error and when the texture closes.
func TestTextureReleaseOnce(t *testing.T) {
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	var mu sync.Mutex
	counts := map[string]int{}
	goids := map[uint64]bool{}
	fn := func(name string) func() {
		return func() {
			mu.Lock()
			counts[name]++
			goids[goid()] = true
			mu.Unlock()
		}
	}
	img := solid(4, 4, texRed)
	short := &image.RGBA{Pix: make([]byte, 10), Stride: 16, Rect: image.Rect(0, 0, 4, 4)}
	narrow := &image.RGBA{Pix: make([]byte, 64), Stride: 8, Rect: image.Rect(0, 0, 4, 4)}
	for name, bad := range map[string]*image.RGBA{"nil": nil, "empty": image.NewRGBA(image.Rectangle{}), "short": short, "narrow": narrow} {
		if err := a.tex.SubmitImage(bad, fn(name)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if err := a.tex.Submit(nil, TextureFrame{Release: fn("nil buffer")}); err == nil {
		t.Error("no error for a nil buffer")
	}
	if st := a.tex.Stats(); st.Submitted != 0 {
		t.Errorf("refused frames count as submitted: %+v", st)
	}
	for i := range 7 {
		if err := a.tex.SubmitImage(img, fn(fmt.Sprint("ok", i))); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			x.drain()
		}
	}
	tex := a.tex
	a.closed.Set(true)
	x.drain()
	if err := tex.SubmitImage(img, fn("closed")); !errors.Is(err, ErrTextureClosed) {
		t.Errorf("SubmitImage after unmount: %v", err)
	}
	if err := tex.SubmitImage(img, nil); !errors.Is(err, ErrTextureClosed) {
		t.Errorf("SubmitImage without release after unmount: %v", err)
	}
	if _, err := tex.Import(Dmabuf{}); !errors.Is(err, ErrTextureClosed) {
		t.Errorf("Import after unmount: %v", err)
	}
	if s := tex.Size(); s != (TextureSize{}) {
		t.Errorf("Size after unmount: %+v", s)
	}
	var none texReleases
	none.got(t, tex)
	x.drain()
	mu.Lock()
	defer mu.Unlock()
	if len(counts) != 5+7+1 {
		t.Errorf("released %v", counts)
	}
	for name, n := range counts {
		if n != 1 {
			t.Errorf("%s was released %d times", name, n)
		}
	}
	if goids[goid()] {
		t.Error("a Release ran on the goroutine that submits")
	}
	x.expectIdle("after the unmount")
}

// The shown frame is held until a list without it was composed, and no
// longer: the frame that takes the next one lets go of it.
func TestTextureReleaseTiming(t *testing.T) {
	var a texApp
	var rel texReleases
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.Mount(a.root())
	tr.Frame()
	submit := func(name string, c color.RGBA) {
		t.Helper()
		if err := a.tex.SubmitImage(solid(4, 4, c), rel.fn(name)); err != nil {
			t.Fatal(err)
		}
	}
	draws := func() []color.RGBA {
		// What drawing the tree's current list reads.
		var out []color.RGBA
		dl := tr.LastFrame()
		for i := range dl.Len() {
			if c, ok := dl.At(i).(gfx.DrawImage); ok {
				p := c.Image.Pix()
				out = append(out, color.RGBA{p[0], p[1], p[2], p[3]})
			}
		}
		return out
	}
	submit("a", texRed)
	tr.Frame()
	submit("b", texGreen)
	if got := rel.got(t, a.tex); got != "" {
		t.Fatalf("released %q while the last list draws it", got)
	}
	if got := draws(); !slices.Equal(got, []color.RGBA{texRed}) {
		t.Fatalf("the list draws %v", got)
	}
	tr.Frame()
	if got := rel.got(t, a.tex); got != "a" {
		t.Fatalf("released %q by the frame that shows b, want a", got)
	}
	if got := draws(); !slices.Equal(got, []color.RGBA{texGreen}) {
		t.Fatalf("the list draws %v", got)
	}
	// Frames without a new one hold it.
	tr.requestRepaint()
	tr.Frame()
	if got := rel.got(t, a.tex); got != "" {
		t.Fatalf("released %q without a new frame", got)
	}
	// An unmounted element lets go of the list's frame, but the texture
	// keeps it for the next element.
	a.mode.Set("gone")
	tr.Frame()
	if got, d := rel.got(t, a.tex), draws(); got != "" || d != nil {
		t.Fatalf("released %q, the list draws %v", got, d)
	}
	a.mode.Set("")
	tr.Frame()
	if got := draws(); !slices.Equal(got, []color.RGBA{texGreen}) {
		t.Fatalf("mounted again, the list draws %v, want the last frame shown", got)
	}
	// Closed with its element in the same build: held until the list is
	// composed without it, released by that frame.
	a.closed.Set(true)
	tr.ApplyUpdates()
	if got := rel.got(t, a.tex); got != "" {
		t.Fatalf("released %q before the tree built", got)
	}
	tr.Frame()
	if got := rel.got(t, a.tex); got != "b" {
		t.Fatalf("released %q by the unmount, want b", got)
	}
	// A tree that unmounts with its root releases too.
	var c texApp
	tr = NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.Mount(c.root())
	tr.Frame()
	for _, name := range []string{"shown", "ready"} {
		if err := c.tex.SubmitImage(solid(4, 4, texRed), rel.fn(name)); err != nil {
			t.Fatal(err)
		}
		if name == "shown" {
			tr.Frame()
		}
	}
	tr.Unmount()
	if got := rel.got(t, c.tex); got != "ready shown" {
		t.Fatalf("released %q by Unmount, want ready shown", got)
	}
	if len(tr.tex.views) != 0 || len(c.tex.views) != 0 {
		t.Error("an unmounted tree keeps views of textures")
	}
}

// A Release may submit: it runs on the texture's goroutine, one at a time.
func TestTextureReleaseMaySubmit(t *testing.T) {
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	tex := a.tex
	var mu sync.Mutex
	in, most, n := 0, 0, 0
	var submit func()
	submit = func() {
		mu.Lock()
		in++
		most = max(most, in)
		n++
		again := n < 50
		mu.Unlock()
		if again {
			if err := tex.SubmitImage(solid(2, 2, texRed), submit); err != nil {
				t.Error(err)
			}
		}
		mu.Lock()
		in--
		mu.Unlock()
	}
	// Only Releases call submit: a call from here would count as one more.
	for _, release := range []func(){submit, submit, nil, nil} {
		if err := tex.SubmitImage(solid(2, 2, texRed), release); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "the Releases to run out", func() bool {
		x.b.Drain(x.h)
		x.b.Advance(x.b.FrameInterval())
		mu.Lock()
		done := n >= 50
		mu.Unlock()
		if !done {
			_ = tex.SubmitImage(solid(2, 2, texBlue), nil)
		}
		return done
	})
	mu.Lock()
	defer mu.Unlock()
	if most != 1 {
		t.Errorf("%d Releases ran at once", most)
	}
}

// With SubmitImage and no release, the image is the caller's again: later
// writes are not seen. With one, its pixels are shown without a copy, unless
// its rows are not contiguous.
func TestTextureSubmitImageCopies(t *testing.T) {
	var a texApp
	var rel texReleases
	x := newWinHarness(t, Options{Root: a.root()})
	shown := func() color.RGBA {
		x.drain()
		return x.pixel(60, 80)
	}
	img := solid(8, 8, texRed)
	_ = a.tex.SubmitImage(img, nil)
	copy(img.Pix, solid(8, 8, texGreen).Pix)
	if c := shown(); c != texRed {
		t.Errorf("without a release, a write after SubmitImage is seen: %v", c)
	}
	_ = a.tex.SubmitImage(img, rel.fn("img"))
	a.tex.mu.Lock()
	f := a.tex.mb.ready
	a.tex.mu.Unlock()
	if &f.img.Pix()[0] != &img.Pix[0] || f.pix != nil {
		t.Error("with a release and contiguous rows, the pixels were copied")
	}
	shown()

	// A sub-image: its rows are not contiguous, and it starts inside.
	big := solid(20, 20, texRed)
	for y := 5; y < 9; y++ {
		for px := 7; px < 13; px++ {
			big.SetRGBA(px, y, texBlue)
		}
	}
	sub := big.SubImage(image.Rect(7, 5, 13, 9)).(*image.RGBA)
	for _, release := range []func(){nil, rel.fn("sub")} {
		if err := a.tex.SubmitImage(sub, release); err != nil {
			t.Fatal(err)
		}
		a.tex.mu.Lock()
		f := a.tex.mb.ready
		a.tex.mu.Unlock()
		if f.w != 6 || f.h != 4 {
			t.Fatalf("a frame of %dx%d", f.w, f.h)
		}
		for i := 0; i < len(f.img.Pix()); i += 4 {
			if p := f.img.Pix()[i:]; p[2] != 255 || p[0] != 0 {
				t.Fatalf("pixel %d of the sub-image is %v", i/4, p[:4])
			}
		}
	}
	if c := shown(); c != texBlue {
		t.Errorf("the sub-image shows %v", c)
	}
	// Held until it is replaced, copied or not.
	if got := rel.got(t, a.tex); got != "img" {
		t.Errorf("released %q, want img", got)
	}
	_ = a.tex.SubmitImage(solid(8, 8, texRed), nil)
	shown()
	if got := rel.got(t, a.tex); got != "sub" {
		t.Errorf("released %q, want sub", got)
	}
}

// ---------------------------------------------------------------------
// Fences and imported buffers (with a fake importer).

// fakeTextures is a Host's textureImporter.
type fakeTextures struct {
	mu       sync.Mutex
	imported []*fakeTexture
	err      error
}

func (f *fakeTextures) textureDevice() TextureDevice {
	return TextureDevice{GPU: true, Name: "fake", Formats: []TextureFormat{{Fourcc: fourccAR24, Modifier: 42}}}
}

func (f *fakeTextures) importTexture(m TextureMemory, _ uint64) (textureContent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	d := m.(Dmabuf)
	c := &fakeTexture{pix: solid(d.Width, d.Height, texRed).Pix}
	f.imported = append(f.imported, c)
	return c, nil
}

// fakeTexture is memory of a fake device.
type fakeTexture struct {
	mu     sync.Mutex
	pix    []byte
	reads  int
	closed int
}

func (c *fakeTexture) Pixels() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	return slices.Clone(c.pix)
}

func (c *fakeTexture) closeTexture() {
	c.mu.Lock()
	c.closed++
	c.mu.Unlock()
}

func (c *fakeTexture) fill(k color.RGBA) {
	c.mu.Lock()
	for i := 0; i < len(c.pix); i += 4 {
		c.pix[i], c.pix[i+1], c.pix[i+2], c.pix[i+3] = k.R, k.G, k.B, k.A
	}
	c.mu.Unlock()
}

func (c *fakeTexture) state() (reads, closed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads, c.closed
}

// newTexImport is a window with a fake importer and n buffers imported.
func newTexImport(t *testing.T, a *texApp, n int) (*winHarness, *fakeTextures, []*TextureBuffer) {
	t.Helper()
	x := newWinHarness(t, Options{Root: a.root()})
	imp := &fakeTextures{}
	x.h.setTextureImporter(imp)
	bufs := make([]*TextureBuffer, n)
	for i := range bufs {
		var err error
		bufs[i], err = a.tex.Import(Dmabuf{Width: 8, Height: 4, Fourcc: fourccAR24, Modifier: 42, Planes: []DmabufPlane{{FD: 3, Stride: 32}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	return x, imp, bufs
}

func TestTextureFences(t *testing.T) {
	var a texApp
	var rel texReleases
	x, imp, bufs := newTexImport(t, &a, 5)
	tex := a.tex
	colors := []color.RGBA{texRed, texGreen, texBlue, {255, 255, 0, 255}, {0, 255, 255, 255}}
	var fences []*chanFence
	submit := func(i int) {
		t.Helper()
		imp.imported[i].fill(colors[i])
		f := newChanFence()
		fences = append(fences, f)
		if err := tex.Submit(bufs[i], TextureFrame{Fence: f, Release: rel.fn(fmt.Sprint(i))}); err != nil {
			t.Fatal(err)
		}
	}
	ready := func() int {
		tex.mu.Lock()
		defer tex.mu.Unlock()
		if tex.mb.ready == nil {
			return 0
		}
		return int(tex.mb.ready.seq)
	}
	presents := x.h.Presents()
	submit(0)
	submit(1)
	x.drain()
	if x.h.Presents() != presents {
		t.Error("a frame whose fence did not signal was shown")
	}
	x.expectIdle("while two frames wait")
	// The third replaces the newer of the two that wait.
	submit(2)
	if got := rel.got(t, tex); got != "1" {
		t.Errorf("released %q, want 1", got)
	}
	waitFor(t, "the dropped frame's wait to be cancelled", func() bool {
		_, closed, cut := fences[1].state()
		return cut && closed == 1
	})
	fences[0].ch <- nil
	waitFor(t, "frame 0 to be ready", func() bool { return ready() == 1 })
	x.drain()
	if c := x.pixel(60, 80); c != texRed {
		t.Errorf("shows %v, want frame 0", c)
	}
	// 2 waits; 3 is submitted and finishes first: 2 is dropped.
	submit(3)
	fences[3].ch <- nil
	waitFor(t, "frame 3 to be ready", func() bool { return ready() == 4 })
	if got := rel.got(t, tex); got != "2" {
		t.Errorf("released %q, want 2: a newer frame is ready", got)
	}
	x.drain()
	if c := x.pixel(60, 80); c != colors[3] {
		t.Errorf("shows %v, want frame 3", c)
	}
	if got := rel.got(t, tex); got != "0" {
		t.Errorf("released %q, want 0", got)
	}
	// A failed fence drops its frame: the shown one stays.
	submit(4)
	fences[4].ch <- errors.New("device lost")
	waitFor(t, "frame 4 to be dropped", func() bool { return tex.Stats().Dropped == 3 })
	if got := rel.got(t, tex); got != "4" {
		t.Errorf("released %q, want 4", got)
	}
	x.drain()
	if c := x.pixel(60, 80); c != colors[3] {
		t.Errorf("shows %v, want frame 3 still", c)
	}
	// Waiting frames are released when the component unmounts.
	fences = nil
	submit(0)
	a.closed.Set(true)
	x.drain()
	if got := rel.got(t, tex); got != "0 3" {
		t.Errorf("released %q by the unmount, want 0 3", got)
	}
	waitFor(t, "the wait to be cancelled", func() bool {
		waits, closed, cut := fences[0].state()
		return cut && closed == 1 && waits == 1
	})
	st := tex.Stats()
	if st.Submitted != 6 || st.Shown != 2 || st.Dropped != 4 {
		t.Errorf("stats %+v", st)
	}
	for i, c := range imp.imported {
		waitFor(t, "the buffers to be freed", func() bool {
			_, closed := c.state()
			return closed == 1
		})
		if reads, _ := c.state(); reads > 2 {
			t.Errorf("buffer %d was read %d times", i, reads)
		}
	}
}

// An imported buffer is drawn by an external image, a new one per frame, and
// freed once, when it is closed and no frame of it is held.
func TestTextureImportedBuffer(t *testing.T) {
	var a texApp
	var rel texReleases
	x, imp, bufs := newTexImport(t, &a, 3)
	tex := a.tex
	if d := tex.Device(); !d.GPU || d.Name != "fake" {
		t.Errorf("Device() = %+v", d)
	}
	imp.imported[0].fill(texGreen)
	if err := tex.Submit(bufs[0], TextureFrame{Release: rel.fn("0")}); err != nil {
		t.Fatal(err)
	}
	if err := tex.Submit(bufs[0], TextureFrame{Release: rel.fn("again")}); err == nil || !strings.Contains(err.Error(), "still held") {
		t.Errorf("a held buffer submitted again: %v", err)
	}
	if got := rel.got(t, tex); got != "again" {
		t.Errorf("released %q, want again", got)
	}
	x.drain()
	if c := x.pixel(60, 80); c != texGreen {
		t.Errorf("shows %v", c)
	}
	if v := tex.views[0].cur.img; v.External() != imp.imported[0] {
		t.Errorf("drawn by %T, want the imported content", v.External())
	}
	// Closed while shown: freed when its frame is replaced.
	if err := bufs[0].Close(); err != nil {
		t.Fatal(err)
	}
	x.drain()
	if _, closed := imp.imported[0].state(); closed != 0 {
		t.Fatal("a buffer on screen was freed")
	}
	if err := tex.Submit(bufs[0], TextureFrame{}); err == nil {
		t.Error("a closed buffer was submitted")
	}
	if err := tex.Submit(bufs[1], TextureFrame{Release: rel.fn("1")}); err != nil {
		t.Fatal(err)
	}
	x.drain()
	if got := rel.got(t, tex); got != "0" {
		t.Errorf("released %q, want 0", got)
	}
	if _, closed := imp.imported[0].state(); closed != 1 {
		t.Errorf("freed %d times, want once its frame was let go", closed)
	}
	// Closed at rest: freed at once, once.
	_, _ = bufs[2].Close(), bufs[2].Close()
	if _, closed := imp.imported[2].state(); closed != 1 {
		t.Errorf("freed %d times", closed)
	}
	// Of another texture.
	var b texApp
	y := newWinHarness(t, Options{Root: b.root()})
	if err := b.tex.Submit(bufs[1], TextureFrame{Release: rel.fn("other")}); err == nil {
		t.Error("a buffer of another texture was submitted")
	}
	if got := rel.got(t, b.tex); got != "other" {
		t.Errorf("released %q, want other", got)
	}
	y.h.End()
	// Quitting releases and frees everything.
	x.h.End()
	if got := rel.got(t, tex); got != "1" {
		t.Errorf("released %q at the end, want 1", got)
	}
	waitFor(t, "the buffer to be freed", func() bool {
		_, closed := imp.imported[1].state()
		return closed == 1
	})
}

// A frame drawn much smaller than it is has no mip levels: its image is the
// one drawn, its pixels are read once, as they are rasterized, and it names
// the texture's stream.
func TestTextureNoLevels(t *testing.T) {
	var a, other texApp
	a.class, other.class = "w-8 h-4 object-cover", "w-8 h-4"
	x, imp, _ := newTexImport(t, &a, 0)
	newWinHarness(t, Options{Root: other.root()})
	big := Dmabuf{Width: 640, Height: 320, Fourcc: fourccAR24, Modifier: 42, Planes: []DmabufPlane{{FD: 3, Stride: 2560}}}
	b, err := a.tex.Import(big)
	if err != nil {
		t.Fatal(err)
	}
	drawn := func() *gfx.Image {
		t.Helper()
		var out *gfx.Image
		for _, c := range x.h.Tree().lay.cur.dl.All() {
			if d, ok := c.(gfx.DrawImage); ok {
				if out != nil {
					t.Error("several images are drawn")
				}
				out = d.Image
			}
		}
		return out
	}
	for n := range 3 {
		if n == 1 {
			_ = a.tex.SubmitImage(solid(640, 320, texBlue), func() {})
		} else if err := a.tex.Submit(b, TextureFrame{}); err != nil {
			t.Fatal(err)
		}
		x.drain()
		f := a.tex.views[0].cur
		if got := drawn(); got != f.img || got.Width() != 640 {
			t.Errorf("frame %d is drawn by another image than its own", n)
		}
		if n == 1 && (f.img.Stream() == 0 || f.img.Stream() != a.tex.stream || a.tex.stream == other.tex.stream) {
			t.Errorf("the image is of the stream %d; the textures are %d and %d", f.img.Stream(), a.tex.stream, other.tex.stream)
		}
	}
	if reads, _ := imp.imported[0].state(); reads != 2 {
		t.Errorf("two frames were read %d times", reads)
	}
}

func TestTextureImportErrors(t *testing.T) {
	var a texApp
	x, imp, _ := newTexImport(t, &a, 0)
	plane := []DmabufPlane{{FD: 3, Stride: 4096}}
	for _, tc := range []struct {
		name, want  string
		m           TextureMemory
		unsupported bool
	}{
		{"nil", "pass the memory", nil, false},
		{"nil pointer", "nil *Dmabuf", (*Dmabuf)(nil), false},
		{"empty", "width and height", Dmabuf{Fourcc: fourccAR24, Planes: plane}, false},
		{"huge", "width and height", Dmabuf{Width: 1 << 20, Height: 1, Fourcc: fourccAR24, Planes: plane}, false},
		{"no planes", "planes", Dmabuf{Width: 1, Height: 1, Fourcc: fourccAR24}, false},
		{"no descriptor", "descriptor -1", Dmabuf{Width: 1, Height: 1, Fourcc: fourccAR24, Planes: []DmabufPlane{{FD: -1}}}, false},
		{"NV12", "AR24, XR24, AB24 and XB24", Dmabuf{Width: 64, Height: 64, Fourcc: 'N' | 'V'<<8 | '1'<<16 | '2'<<24, Planes: plane}, true},
	} {
		_, err := a.tex.Import(tc.m)
		if err == nil || !strings.Contains(err.Error(), tc.want) || errors.Is(err, ErrTextureUnsupported) != tc.unsupported {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	// What the device says, when neither it nor the copy takes it; an error
	// of another kind is the device's whatever the memory.
	imp.err = fmt.Errorf("the device takes modifier 42: %w", ErrTextureUnsupported)
	if _, err := a.tex.Import(Dmabuf{Width: 64, Height: 64, Fourcc: fourccAR24, Modifier: 7, Planes: plane}); !errors.Is(err, ErrTextureUnsupported) || !strings.Contains(err.Error(), "modifier 42") {
		t.Errorf("unsupported by the device: %v", err)
	}
	imp.err = errors.New("out of memory")
	if _, err := a.tex.Import(Dmabuf{Width: 64, Height: 64, Fourcc: fourccAR24, Planes: plane}); err != imp.err {
		t.Errorf("failed by the device: %v", err)
	}
	x.drain()
}

// ---------------------------------------------------------------------
// Sizes.

func TestTextureSize(t *testing.T) {
	var tex *Texture
	var w WindowHandle
	var setClass, setSecond Setter[string]
	x := newWinHarness(t, Options{Root: Component(func() Node {
		w = UseWindow()
		tex = UseTexture()
		class, set := UseState("w-40 h-20 p-2 border-4")
		second, set2 := UseState("")
		setClass, setSecond = set, set2
		kids := []Child{Class("p-1")}
		if class != "" {
			kids = append(kids, texture(tex, Key("a"), Class(class)))
		}
		if second != "" {
			kids = append(kids, texture(tex, Key("b"), Class(second)))
		}
		return div(kids...)
	})})
	var calls []TextureSize
	tex.OnResize(func(s TextureSize) { calls = append(calls, s) })
	expect := func(what string, size TextureSize, want ...TextureSize) {
		t.Helper()
		x.drain()
		if got := tex.Size(); got != size {
			t.Errorf("%s: Size %+v, want %+v", what, got, size)
		}
		if !slices.Equal(calls, want) {
			t.Errorf("%s: OnResize got %+v, want %+v", what, calls, want)
		}
		calls = nil
	}
	a := TextureSize{136, 56, 1}
	expect("at first", a, a)
	expect("nothing changed", a)
	setClass.Set("w-40 h-20")
	b := TextureSize{160, 80, 1}
	expect("resized", b, b)
	setClass.Set("w-40 h-20 rotate-45 scale-150")
	expect("a transform does not count", b)
	setSecond.Set("w-20 h-10")
	expect("a smaller element", b)
	setSecond.Set("w-60 h-10")
	expect("as large: the first", b)
	setSecond.Set("w-60 h-20")
	c := TextureSize{240, 80, 1}
	expect("the largest", c, c)
	main := x.windows()[0]
	main.SetScale(2)
	d := TextureSize{480, 160, 2}
	expect("scale 2", d, d)
	main.SetScale(1.25)
	e := TextureSize{300, 100, 1.25}
	expect("scale 1.25", e, e)
	main.SetScale(1)
	expect("scale 1", c, c)
	setSecond.Set("")
	expect("the largest is gone", b, b)
	setClass.Set("hidden")
	expect("display none", TextureSize{}, TextureSize{})
	setClass.Set("w-10 h-0")
	expect("empty", TextureSize{})
	setClass.Set("w-40 h-20")
	expect("back", b, b)
	w.Hide()
	expect("a hidden window", b)
	w.Show()
	expect("shown again", b)
	setClass.Set("")
	expect("unmounted", TextureSize{}, TextureSize{})
	tex.OnResize(func(TextureSize) { t.Error("the callback replaced was called") })
	tex.OnResize(nil)
	setClass.Set("w-40 h-20")
	expect("no callback", b)
	tex.OnResize(func(s TextureSize) { calls = append(calls, s) })
	expect("set while shown", b, b)
}

// OnResize set by a layout effect is called in the same frame, before its
// paint, and so is every later change: a frame submitted from it is shown at
// once, never a stretched one.
func TestTextureOnResizeSubmits(t *testing.T) {
	var tex *Texture
	var setClass Setter[string]
	x := newWinHarness(t, Options{Root: Component(func() Node {
		tex = UseTexture()
		class, set := UseState("w-40 h-20")
		setClass = set
		UseLayoutEffect(func() func() {
			tex.OnResize(func(s TextureSize) {
				img := solid(s.Width, s.Height, texGreen)
				img.SetRGBA(s.Width-1, s.Height-1, texBlue) // only unscaled
				if err := tex.SubmitImage(img, nil); err != nil {
					t.Error(err)
				}
			})
			return nil
		}, Once)
		return texture(tex, Class(class))
	})})
	if c := x.pixel(159, 79); c != texBlue {
		t.Errorf("the first frame shows %v at the corner", c)
	}
	frames := 0
	x.h.AfterFrame = func() {
		frames++
		if c := x.pixel(99, 49); c != texBlue {
			t.Errorf("frame %d after the resize shows %v at the corner", frames, c)
		}
	}
	setClass.Set("w-[100px] h-[50px]")
	x.b.Drain(x.h)
	x.b.Advance(x.b.FrameInterval())
	x.b.Drain(x.h)
	if frames == 0 {
		t.Fatal("no frame")
	}
	if st := tex.Stats(); st.Submitted != 2 || st.Shown != 2 {
		t.Errorf("stats %+v", st)
	}
}

// A texture shown in another window follows its scale and its frames.
func TestTextureInTwoWindows(t *testing.T) {
	var tex *Texture
	var rel texReleases
	var setWhere Setter[string]
	root := func() Node {
		tex = UseTexture()
		where, set := UseState("main")
		setWhere = set
		kids := []Child{}
		if where != "child" {
			kids = append(kids, texture(tex, Key("main"), Class("w-40 h-20")))
		}
		if where != "main" {
			kids = append(kids, Component(func() Node {
				UseWindowSpec(ChildWindowSpec{WindowSpec: WindowSpec{Title: "child", Width: 200, Height: 100}, OnClose: func() {},
					Root: div(texture(tex, Class("w-20 h-10")))})
				return nil
			}, Key("child")))
		}
		return div(kids...)
	}
	x := newWinHarness(t, Options{Root: Component(root)})
	show := func(name string, c color.RGBA) {
		t.Helper()
		if err := tex.SubmitImage(solid(4, 4, c), rel.fn(name)); err != nil {
			t.Fatal(err)
		}
		x.drain()
	}
	show("a", texRed)
	setWhere.Set("both")
	x.drain()
	child := x.child(0)
	if c := child.Frame().RGBAAt(10, 10); c != texRed {
		t.Errorf("the child window shows %v, want the shown frame", c)
	}
	if got := tex.Size(); got != (TextureSize{160, 80, 1}) {
		t.Errorf("the size is %+v, want the larger element's", got)
	}
	show("b", texGreen)
	if c, d := x.pixel(10, 10), child.Frame().RGBAAt(10, 10); c != texGreen || d != texGreen {
		t.Errorf("the windows show %v and %v, want green", c, d)
	}
	if got := rel.got(t, tex); got != "a" {
		t.Errorf("released %q, want a once both windows show b", got)
	}
	if st := tex.Stats(); st.Shown != 2 {
		t.Errorf("stats %+v: a frame two windows show counts once", st)
	}
	setWhere.Set("child")
	x.drain()
	if got := tex.Size(); got != (TextureSize{80, 40, 1}) {
		t.Errorf("moved to the child window, the size is %+v", got)
	}
	child.SetScale(2)
	x.drain()
	if got := tex.Size(); got != (TextureSize{160, 80, 2}) {
		t.Errorf("in a window at scale 2 the size is %+v", got)
	}
	show("c", texBlue)
	if d := child.Frame().RGBAAt(10, 10); d != texBlue {
		t.Errorf("the child window shows %v, want blue", d)
	}
	if got := rel.got(t, tex); got != "b" {
		t.Errorf("released %q, want b", got)
	}
	// A minimized window holds the frame its list draws, until it draws
	// again.
	setWhere.Set("both")
	x.drain()
	child = x.child(0)
	x.h.Handle(child).Minimize()
	x.drain()
	show("d", texRed)
	if got := rel.got(t, tex); got != "" {
		t.Errorf("released %q, which the minimized window's list draws", got)
	}
	x.h.Handle(child).Focus()
	x.drain()
	if got := rel.got(t, tex); got != "c" {
		t.Errorf("released %q once the window is restored, want c", got)
	}
	if d := child.Frame().RGBAAt(10, 10); d != texRed {
		t.Errorf("restored, the child window shows %v, want red", d)
	}
	// Closing the window that shows it lets go of its frame.
	setWhere.Set("main")
	x.drain()
	if len(tex.views) != 1 {
		t.Errorf("%d views after the child window closed", len(tex.views))
	}
	x.h.End()
	if got := rel.got(t, tex); got != "d" {
		t.Errorf("released %q at the end, want d", got)
	}
}

// ---------------------------------------------------------------------
// Pacing.

// texLoop runs n NextFrame calls on a goroutine and reports the frames.
type texLoop struct {
	frames chan FrameTime
	errs   chan error
	cancel context.CancelFunc
}

func startTexLoop(tex *Texture, n int, each func()) *texLoop {
	ctx, cancel := context.WithCancel(context.Background())
	l := &texLoop{frames: make(chan FrameTime, n), errs: make(chan error, 1), cancel: cancel}
	go func() {
		for range n {
			ft, err := tex.NextFrame(ctx)
			if err != nil {
				l.errs <- err
				return
			}
			if each != nil {
				each()
			}
			l.frames <- ft
		}
		l.errs <- nil
	}()
	return l
}

// waiting waits until a NextFrame call is in progress.
func waitingFrame(t *testing.T, tex *Texture) {
	t.Helper()
	waitFor(t, "a NextFrame call to wait", func() bool {
		tex.mu.Lock()
		defer tex.mu.Unlock()
		return len(tex.waiters) > 0
	})
}

func TestTextureNextFrame(t *testing.T) {
	var a texApp
	var w WindowHandle
	content := a.root()
	x := newWinHarness(t, Options{Root: Component(func() Node {
		w = UseWindow()
		return div(content)
	})})
	tex := a.tex
	// It paces a loop at the window's rate, and each frame is shown.
	const n = 6
	l := startTexLoop(tex, n, func() { _ = tex.SubmitImage(solid(4, 4, texRed), nil) })
	var times []FrameTime
	for len(times) < n {
		x.b.Drain(x.h) // the wake asks for a frame
		x.b.Advance(x.b.FrameInterval())
		x.b.Drain(x.h)
		waitFor(t, "the loop to ask for the next frame, or to end", func() bool {
			for len(l.frames) > 0 {
				times = append(times, <-l.frames)
			}
			tex.mu.Lock()
			defer tex.mu.Unlock()
			return len(tex.waiters) > 0 || len(times) == n
		})
	}
	if err := <-l.errs; err != nil {
		t.Fatal(err)
	}
	for i := 1; i < n; i++ {
		if d := times[i].Time.Sub(times[i-1].Time); d < x.b.FrameInterval() {
			t.Errorf("frames %v apart, the interval is %v", d, x.b.FrameInterval())
		}
	}
	if d := time.Until(times[n-1].Time); d > time.Hour || d < -time.Hour {
		t.Errorf("the time of a frame is %v from now: not on the clock of time.Now", d)
	}
	x.drain()
	x.expectIdle("once the loop stopped")
	st := tex.Stats()
	if st.Shown != n || st.Dropped != 0 {
		t.Errorf("a loop paced by NextFrame: stats %+v", st)
	}

	// Two callers get the same frame.
	l1, l2 := startTexLoop(tex, 1, nil), startTexLoop(tex, 1, nil)
	waitFor(t, "two calls to wait", func() bool {
		tex.mu.Lock()
		defer tex.mu.Unlock()
		return len(tex.waiters) == 2
	})
	x.drain()
	f1, f2 := <-l1.frames, <-l2.frames
	if f1 != f2 {
		t.Errorf("two callers got %v and %v", f1, f2)
	}

	// It blocks while the texture is not on screen, and asks for nothing.
	for _, tc := range []struct {
		name       string
		hide, show func()
	}{
		{"display none", func() { a.mode.Set("none") }, func() { a.mode.Set("") }},
		{"unmounted", func() { a.mode.Set("gone") }, func() { a.mode.Set("") }},
		{"hidden", func() { w.Hide() }, func() { w.Show() }},
		{"minimized", func() { w.Minimize() }, func() { w.Focus() }},
	} {
		tc.hide()
		x.drain()
		frames := x.h.Tree().Stats().Frames
		l := startTexLoop(tex, 1, nil)
		waitingFrame(t, tex)
		x.drain()
		select {
		case <-l.frames:
			t.Fatalf("%s: NextFrame returned", tc.name)
		default:
		}
		if got := x.h.Tree().Stats().Frames; got != frames {
			t.Errorf("%s: %d frames for a texture not on screen", tc.name, got-frames)
		}
		tc.show()
		x.drain()
		select {
		case <-l.frames:
		case <-time.After(20 * time.Second):
			t.Fatalf("%s: NextFrame did not return once shown again", tc.name)
		}
		x.drain()
		// A call made while a frame was asked for that never came.
		l = startTexLoop(tex, 1, nil)
		waitingFrame(t, tex)
		tc.hide()
		x.b.Drain(x.h)
		tc.show()
		x.drain()
		select {
		case <-l.frames:
		case <-time.After(20 * time.Second):
			t.Fatalf("%s: NextFrame did not return after hiding in between", tc.name)
		}
		x.drain()
	}

	// Its context ends it, and so does the unmount.
	l = startTexLoop(tex, 1, nil)
	waitingFrame(t, tex)
	l.cancel()
	if err := <-l.errs; !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
	tex.mu.Lock()
	left := len(tex.waiters)
	tex.mu.Unlock()
	if left != 0 {
		t.Errorf("%d calls wait after the cancellation", left)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error)
	go func() { _, err := tex.NextFrame(ctx); done <- err }()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("a context already done: %v", err)
	}
	a.mode.Set("none") // no frame for the call
	x.drain()
	l = startTexLoop(tex, 1, nil)
	waitingFrame(t, tex)
	a.closed.Set(true)
	x.drain()
	if err := <-l.errs; !errors.Is(err, ErrTextureClosed) {
		t.Errorf("unmounted: %v", err)
	}
	l = startTexLoop(tex, 1, nil)
	if err := <-l.errs; !errors.Is(err, ErrTextureClosed) {
		t.Errorf("after the unmount: %v", err)
	}
	x.expectIdle("at the end")
}

// NextFrame on the UI thread would wait for itself.
func TestTextureNextFrameOnUIThread(t *testing.T) {
	var a texApp
	newWinHarness(t, Options{Root: a.root()})
	defer func() {
		if r := fmt.Sprint(recover()); !strings.Contains(r, "UI thread") || !strings.Contains(r, "UseGo") {
			t.Errorf("recovered %q", r)
		}
	}()
	_, _ = a.tex.NextFrame(context.Background())
}

// The largest element on screen paces NextFrame, in whichever window.
func TestTextureNextFramePrimary(t *testing.T) {
	var tex *Texture
	x := newWinHarness(t, Options{Root: Component(func() Node {
		tex = UseTexture()
		return div(texture(tex, Class("w-40 h-20")), Component(func() Node {
			UseWindowSpec(ChildWindowSpec{WindowSpec: WindowSpec{Title: "child", Width: 200, Height: 200}, OnClose: func() {},
				Root: div(texture(tex, Class("w-50 h-50")))})
			return nil
		}))
	})})
	child := x.child(0)
	ct := x.h.TreeOf(child)
	primary := func() *Tree {
		tex.mu.Lock()
		defer tex.mu.Unlock()
		if tex.primary == nil {
			return nil
		}
		return tex.primary.t
	}
	if primary() != ct {
		t.Error("the window of the smaller element paces NextFrame")
	}
	l := startTexLoop(tex, 1, nil)
	waitingFrame(t, tex)
	x.h.Handle(child).Minimize() // its frame, asked for, never comes
	x.b.Drain(x.h)
	if primary() != x.h.Tree() {
		t.Error("a minimized window paces NextFrame")
	}
	x.drain()
	select {
	case <-l.frames:
	case <-time.After(20 * time.Second):
		t.Fatal("NextFrame did not go on with the other window")
	}
}

// ---------------------------------------------------------------------
// Elements.

func TestTextureSemantics(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(m string) { t.Error(m) }})
	tr.SetViewport(gfx.Size{W: 400, H: 600})
	tr.Mount(Component(func() Node {
		x, name := UseTexture(), UseElementRef()
		return div(
			texture(x),
			texture(x, AriaLabel("Globe")),
			p(AttachRef(name), Text("Map")),
			texture(x, AriaLabelledBy(name)),
			texture(x, AriaLabel("Game"), Role("application"), TabIndex(0)),
			texture(x, TabIndex(0)),
		)
	}))
	tr.Frame()
	want := "- img \"Globe\"\n- paragraph: Map\n- img \"Map\"\n- application \"Game\""
	if got := strings.TrimSpace(tr.AriaSnapshot()); got != want {
		t.Errorf("snapshot:\n%s\nwant:\n%s", got, want)
	}
	var roles []string
	for _, n := range tr.Inspect().Children[0].Children {
		roles = append(roles, n.Kind)
	}
	if !slices.Contains(roles, "texture") {
		t.Errorf("inspect names the boxes %v", roles)
	}
}

// A texture takes the pointer and the keyboard as any element.
func TestTextureInput(t *testing.T) {
	var tex *Texture
	var moved, clicked, keys int
	var ref ElementRef
	x := newWinHarness(t, Options{Root: Component(func() Node {
		tex, ref = UseTexture(), UseElementRef()
		return texture(tex, AttachRef(ref), Class("w-40 h-20"), TabIndex(0),
			On("pointermove", func(PointerEvent) { moved++ }),
			On("click", func(MouseEvent) { clicked++ }),
			On("keydown", func(KeyboardEvent) { keys++ }))
	})})
	main := x.windows()[0]
	main.SetFocused(true)
	x.click(main, 80, 40)
	if moved == 0 || clicked != 1 || !ref.Focused() {
		t.Errorf("%d moves, %d clicks, focused %v", moved, clicked, ref.Focused())
	}
	main.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeKeyA, Key: "a", Text: "a"})
	x.drain()
	if keys != 1 {
		t.Errorf("%d keys", keys)
	}
}

func TestTexturePanics(t *testing.T) {
	expect := func(what, want string, f func()) {
		t.Helper()
		defer func() {
			t.Helper()
			if r := fmt.Sprint(recover()); !strings.Contains(r, want) {
				t.Errorf("%s: recovered %q, want %q in it", what, r, want)
			}
		}()
		f()
	}
	expect("a nil texture", "tela.UseTexture", func() { texture(nil) })
	expect("no texture", "TextureSource", func() { Primitive(style.KindTexture) })
	expect("children", "overlay", func() { texture(&Texture{}, Text("x")) })
	expect("a texture on a div", "use Texture", func() { div(TextureSource(&Texture{})) })
	expect("alt", "Alt", func() { texture(&Texture{}, Alt("x")) })
	expect("a hook outside a build", "UseTexture", func() { UseTexture() })
}

// The same element can show another texture: it lets go of the first.
func TestTextureSwitched(t *testing.T) {
	var one, two *Texture
	var rel texReleases
	var setWhich Setter[int]
	x := newWinHarness(t, Options{Root: Component(func() Node {
		one, two = UseTexture(), UseTexture()
		none := UseTexture()
		which, set := UseState(0)
		setWhich = set
		return texture([]*Texture{one, two, none}[which], Class("w-40 h-20 bg-black"))
	})})
	if one == two {
		t.Fatal("two hooks gave one handle")
	}
	_ = one.SubmitImage(solid(2, 2, texRed), rel.fn("one"))
	_ = two.SubmitImage(solid(2, 2, texBlue), rel.fn("two"))
	x.drain()
	if c := x.pixel(80, 40); c != texRed {
		t.Errorf("shows %v", c)
	}
	same := one
	setWhich.Set(1)
	x.drain()
	if one != same {
		t.Error("another build gave another handle")
	}
	if c := x.pixel(80, 40); c != texBlue {
		t.Errorf("switched, shows %v", c)
	}
	if one.Size() != (TextureSize{}) || two.Size() != (TextureSize{160, 80, 1}) {
		t.Errorf("sizes %+v and %+v", one.Size(), two.Size())
	}
	expectFullRender(t, x.windows()[0], x.h.Tree())
	setWhich.Set(2)
	x.drain()
	if c := x.pixel(80, 40); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("a texture without a frame shows %v, want the background", c)
	}
	expectFullRender(t, x.windows()[0], x.h.Tree())
	setWhich.Set(0)
	x.drain()
	if c := x.pixel(80, 40); c != texRed {
		t.Errorf("switched back, shows %v, want its last frame", c)
	}
	if got := rel.got(t, one) + rel.got(t, two); got != "" {
		t.Errorf("released %q: the textures keep their shown frames", got)
	}
}

type textureProps struct{ Tex *Texture }

func textureChild(p textureProps) Node { return texture(p.Tex, Class("w-40 h-20")) }

// An instrumented tree names the hook and its state, and a prop that holds
// a texture by its type.
func TestTextureInstrumented(t *testing.T) {
	var x *Texture
	tr := NewTree(Config{Fonts: testFonts, Dev: true})
	tr.SetInstrument(true)
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	tr.Mount(Component(func() Node {
		x = UseTexture()
		return div(ComponentProps(textureChild, textureProps{x}))
	}))
	tr.Frame()
	_ = x.SubmitImage(solid(2, 2, texRed), nil)
	tr.Frame()
	var hooks []HookInfo
	var ps []PropInfo
	for _, e := range tr.Elements(true) {
		hooks, ps = append(hooks, e.Hooks...), append(ps, e.Props...)
	}
	if len(hooks) != 1 || hooks[0].Kind != "UseTexture" || hooks[0].Value != "160x80 at x1, 1 frames shown, 0 dropped" {
		t.Errorf("hooks %+v", hooks)
	}
	if len(ps) != 1 || ps[0].Value != "*Texture" {
		t.Errorf("props %+v", ps)
	}
}

func TestTextureDevice(t *testing.T) {
	var a texApp
	newWinHarness(t, Options{Root: a.root()})
	d := a.tex.Device()
	if d.GPU || d.Name != "" {
		t.Errorf("without a GPU: %+v", d)
	}
	if want := mappedFormats(); !slices.Equal(d.Formats, want) || !slices.Equal(d.ForeignFormats, want) {
		t.Errorf("formats %v and %v, want %v", d.Formats, d.ForeignFormats, want)
	}
}
