//go:build darwin

package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/auto"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// A real tela dev whose host is the Cocoa backend: the window is a real
// NSWindow on the main thread (served by serveTests), the children are
// real builds of testdata/devapp on the remote backend, and input comes
// as NSEvents handed to AppKit in-process (-[NSApp sendEvent:], as the
// cocoa package's tests do; nothing is posted to the system). It needs a
// window server session (macos-latest runners have one) and skips without
// it. Every step logs what it saw and how long it took, since it runs only
// on CI.

// cocoaE2E is an e2e whose host runs on the Cocoa backend.
type cocoaE2E struct {
	*e2e
	b       platform.Backend
	host    *remote.Host
	runErr  chan error      // host.Run's result
	runDone <-chan struct{} // the main thread is free again

	quitOnce sync.Once
	quitCode int
	quitErr  error // of host.Run, after quitting

	// The visible NSWindows, sampled on the UI thread every
	// windowSampleEvery while sampling (a close and reopen of the window
	// during a swap shows up as a sample without it, or with another
	// number).
	smu       sync.Mutex
	samples   []string
	sampling  bool
	stopSmpl  chan struct{}
	sampleWG  sync.WaitGroup
	sampleErr string // the first sample that differs from the first one
}

const windowSampleEvery = 20 * time.Millisecond

// startCocoaE2E starts tela dev on the Cocoa backend, or skips without a
// window server session.
func startCocoaE2E(t *testing.T) *cocoaE2E {
	t.Helper()
	t.Setenv(auto.EnvBackend, "") // the native backend, whatever the runner sets
	e := newE2E(t, true)          // with the error overlay, as tela dev runs
	c := &cocoaE2E{e2e: e, runErr: make(chan error, 1), stopSmpl: make(chan struct{})}

	type result struct {
		err        error
		mainThread bool
	}
	ready := make(chan result, 1)
	c.runDone = onMainAsync(func() {
		started := false
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("panic on the main thread: %v\n%s", r, debug.Stack())
				if !started {
					ready <- result{err: err}
					return
				}
				e.s.push(evHostDone{err: err})
				c.runErr <- err
			}
		}()
		// What devMain does, on the main thread like tela dev's main.
		b, err := auto.New()
		if err != nil {
			ready <- result{err: err}
			return
		}
		h, err := remote.NewHost(b, e.s.hostOptions())
		if err != nil {
			ready <- result{err: err}
			return
		}
		e.s.attach(h, b.Quit)
		c.b, c.host = b, h
		started = true
		ready <- result{mainThread: darwin.IsMainThread()}
		err = h.Run()
		e.s.push(evHostDone{err: err})
		c.runErr <- err
	})
	r := <-ready
	if errors.Is(r.err, platform.ErrUnavailable) {
		t.Skipf("no window server session: %v", r.err)
	}
	if r.err != nil {
		t.Fatalf("starting the host: %v", r.err)
	}
	if name := c.b.Name(); name != "cocoa" {
		c.b.Quit()
		<-c.runDone
		t.Fatalf("the host's backend is %q, want cocoa", name)
	}
	t.Logf("host on the %s backend, NewHost and Run on the main thread: %v", c.b.Name(), r.mainThread)
	if !r.mainThread {
		t.Errorf("the host does not run on the main thread")
	}
	s := c.b.Settings()
	t.Logf("system settings forwarded to the children: dark mode %v, reduced motion %v, double click %v",
		s.DarkMode, s.ReducedMotion, s.DoubleClickTime())
	e.start()
	t.Cleanup(func() {
		c.stopSampling()
		code, err := c.quit()
		c.host.Close()
		if code != 0 || err != nil {
			t.Errorf("tela dev exited with %d, host.Run returned %v", code, err)
		}
		if t.Failed() || testing.Verbose() {
			t.Logf("output:\n%s", e.out.String())
		}
	})
	return c
}

// quit quits tela dev as the q key does and waits for the event loop to
// return; it returns the exit code and host.Run's error. Idempotent.
func (c *cocoaE2E) quit() (int, error) {
	c.quitOnce.Do(func() {
		c.quitCode = -1
		c.s.push(evKey{'q'})
		select {
		case c.quitCode = <-c.done:
		case <-time.After(30 * time.Second):
			c.t.Error("tela dev did not quit within 30s; quitting the backend")
			c.b.Quit()
		}
		select {
		case c.quitErr = <-c.runErr:
		case <-time.After(30 * time.Second):
			c.t.Error("the Cocoa event loop did not return within 30s of quitting")
			return
		}
		<-c.runDone
	})
	return c.quitCode, c.quitErr
}

// onUI runs f on the host's UI thread (the main thread, inside the event
// loop) and waits for it. f must not call t.Fatal or t.Skip.
func (c *cocoaE2E) onUI(f func()) {
	c.t.Helper()
	done := make(chan struct{})
	c.host.Post(func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				c.t.Errorf("panic on the UI thread: %v\n%s", r, debug.Stack())
			}
		}()
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		f()
	})
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		c.t.Fatalf("the host's UI thread did not run a posted call within 30s; output:\n%s", c.out.String())
	}
}

// --- the window ---

// nsWindow is what AppKit says of one of the app's windows.
type nsWindow struct {
	ns      darwin.ID
	num     int
	title   string
	visible bool
	content darwin.CGSize // the content view's bounds, points
	scale   float64       // backingScaleFactor
}

func (w nsWindow) String() string {
	return fmt.Sprintf("NSWindow %d %q visible=%v content=%gx%g scale=%g", w.num, w.title, w.visible, w.content.Width, w.content.Height, w.scale)
}

var esel = struct {
	sharedApplication, windows, count, objectAtIndex, windowNumber, title, isVisible   darwin.SEL
	contentView, bounds, backingScaleFactor, layer, isGeometryFlipped, renderInContext darwin.SEL
	convertPointToView, sendEvent, mouseEvent, processInfo, systemUptime               darwin.SEL
}{
	sharedApplication:  darwin.Sel("sharedApplication"),
	windows:            darwin.Sel("windows"),
	count:              darwin.Sel("count"),
	objectAtIndex:      darwin.Sel("objectAtIndex:"),
	windowNumber:       darwin.Sel("windowNumber"),
	title:              darwin.Sel("title"),
	isVisible:          darwin.Sel("isVisible"),
	contentView:        darwin.Sel("contentView"),
	bounds:             darwin.Sel("bounds"),
	backingScaleFactor: darwin.Sel("backingScaleFactor"),
	layer:              darwin.Sel("layer"),
	isGeometryFlipped:  darwin.Sel("isGeometryFlipped"),
	renderInContext:    darwin.Sel("renderInContext:"),
	convertPointToView: darwin.Sel("convertPoint:toView:"),
	sendEvent:          darwin.Sel("sendEvent:"),
	mouseEvent:         darwin.Sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
	processInfo:        darwin.Sel("processInfo"),
	systemUptime:       darwin.Sel("systemUptime"),
}

func nsApp() darwin.ID {
	return darwin.SendClass(darwin.GetClass("NSApplication"), esel.sharedApplication)
}

// appWindows lists [NSApp windows]. Main thread, inside a pool.
func appWindows() []nsWindow {
	arr := darwin.Send(nsApp(), esel.windows)
	n := darwin.SendUint(arr, esel.count)
	ws := make([]nsWindow, 0, n)
	for i := range n {
		ns := darwin.Send(arr, esel.objectAtIndex, uint(i))
		w := nsWindow{
			ns:      ns,
			num:     darwin.SendInt(ns, esel.windowNumber),
			title:   darwin.GoString(darwin.Send(ns, esel.title)),
			visible: darwin.SendBool(ns, esel.isVisible),
			scale:   darwin.SendFloat(ns, esel.backingScaleFactor),
		}
		if v := darwin.Send(ns, esel.contentView); v != 0 {
			w.content = darwin.SendRect(v, esel.bounds).Size
		}
		ws = append(ws, w)
	}
	return ws
}

func visibleWindows() []nsWindow {
	return slices.DeleteFunc(appWindows(), func(w nsWindow) bool { return !w.visible })
}

// cocoaState is the host's real window, its NSWindow, the frame it shows
// and the window's pixels read back, taken in one UI thread call.
type cocoaState struct {
	pw     platform.Window // the host's real window (nil: none yet)
	nhost  int             // the host's real windows
	ns     nsWindow        // the only visible NSWindow
	nvis   int             // visible NSWindows
	frame  *image.RGBA     // host.Frame: the shown child's frame, before the overlay
	active int             // the active child's PID (0: none)
	reads  []winReadback   // the window server's capture, then the layer
}

// state returns the cocoaState; it reads the pixels back if read is set.
func (c *cocoaE2E) state(read bool) cocoaState {
	c.t.Helper()
	var st cocoaState
	c.onUI(func() {
		ws := c.host.Windows()
		st.nhost = len(ws)
		if len(ws) > 0 {
			st.pw = ws[0]
			st.frame = c.host.Frame(ws[0])
		}
		vis := visibleWindows()
		st.nvis = len(vis)
		if len(vis) > 0 {
			st.ns = vis[0]
		}
		if a := c.host.Active(); a != nil {
			st.active = a.PID()
		}
		if read && st.pw != nil && st.nvis > 0 {
			st.reads = readWindow(st.ns, st.pw.Metrics().Physical())
		}
	})
	return st
}

// startSampling records the visible NSWindows every windowSampleEvery.
func (c *cocoaE2E) startSampling() {
	c.smu.Lock()
	c.sampling = true
	c.smu.Unlock()
	c.sampleWG.Add(1)
	go func() {
		defer c.sampleWG.Done()
		tick := time.NewTicker(windowSampleEvery)
		defer tick.Stop()
		for {
			select {
			case <-c.stopSmpl:
				return
			case <-tick.C:
				c.host.Post(func() {
					pool := darwin.PoolPush()
					defer darwin.PoolPop(pool)
					var nums []string
					for _, w := range visibleWindows() {
						nums = append(nums, fmt.Sprint(w.num))
					}
					s := strings.Join(nums, ",")
					c.smu.Lock()
					defer c.smu.Unlock()
					if !c.sampling {
						return
					}
					if len(c.samples) > 0 && s != c.samples[0] && c.sampleErr == "" {
						c.sampleErr = fmt.Sprintf("sample %d: visible windows [%s], the first sample had [%s]", len(c.samples), s, c.samples[0])
					}
					c.samples = append(c.samples, s)
				})
			}
		}
	}()
}

// stopSampling stops the sampler and returns the samples and the first
// mismatch. Idempotent.
func (c *cocoaE2E) stopSampling() ([]string, string) {
	c.smu.Lock()
	was := c.sampling
	c.sampling = false
	c.smu.Unlock()
	if was {
		close(c.stopSmpl)
		c.sampleWG.Wait()
	}
	c.smu.Lock()
	defer c.smu.Unlock()
	return slices.Clone(c.samples), c.sampleErr
}

// --- input ---

func uptime() float64 {
	return darwin.SendFloat(darwin.SendClass(darwin.GetClass("NSProcessInfo"), esel.processInfo), esel.systemUptime)
}

// click clicks the primary button at the view point p (points, top-left
// origin) of the window: a mouseMoved: to the content view (AppKit routes
// moves through the tracking area, which needs the real pointer), then a
// press and a release through -[NSApp sendEvent:], the path of real
// clicks.
func (c *cocoaE2E) click(win nsWindow, x, y float64) {
	c.t.Helper()
	c.onUI(func() {
		view := darwin.Send(win.ns, esel.contentView)
		wp := darwin.SendPoint(view, esel.convertPointToView, darwin.CGPoint{X: x, Y: y}, darwin.ID(0))
		ts := uptime()
		event := darwin.GetClass("NSEvent")
		mk := func(typ int, dt float64, clicks int) darwin.ID {
			return darwin.SendClass(event, esel.mouseEvent, uint(typ), wp, uint(0), ts+dt,
				win.num, darwin.ID(0), 0, clicks, float32(1))
		}
		move, down, up := mk(darwin.NSEventTypeMouseMoved, 0, 0), mk(darwin.NSEventTypeLeftMouseDown, 0.01, 1), mk(darwin.NSEventTypeLeftMouseUp, 0.03, 1)
		if move == 0 || down == 0 || up == 0 {
			c.t.Errorf("building the mouse events failed (nil NSEvent)")
			return
		}
		darwin.Send(view, darwin.Sel("mouseMoved:"), move)
		darwin.Send(nsApp(), esel.sendEvent, down)
		darwin.Send(nsApp(), esel.sendEvent, up)
	})
}

// --- pixels ---

// winReadback is the window's content read back, physical pixels.
type winReadback struct {
	path string
	pix  *image.RGBA // nil if no path worked
	note string
}

// exact reports whether the pixels come from the layer, before the window
// server's color management.
func (r winReadback) exact() bool { return strings.HasPrefix(r.path, "renderInContext") }

// readWindow reads the content of win back two ways, as the cocoa
// package's TestPresentReadBack does: from the window server
// (CGWindowListCreateImage of its number, cropped to the content area: what
// reaches the screen, if the system allows the capture) and from the
// content view's layer (renderInContext:: what the window holds, before
// the window server's color management). A path that did not work has a
// nil pix and says why. Main thread, inside a pool.
func readWindow(win nsWindow, phys image.Point) []winReadback {
	space := darwin.CGColorSpaceCreateWithName(darwin.CGColorSpaceSRGB)
	if space == 0 {
		return []winReadback{{path: "none", note: "no sRGB color space"}}
	}
	defer darwin.CGColorSpaceRelease(space)
	return []winReadback{captureContent(win, phys, space), renderContent(win, phys, space)}
}

func captureContent(win nsWindow, phys image.Point, space uintptr) winReadback {
	r := winReadback{path: "CGWindowListCreateImage (window server)"}
	img := darwin.CGWindowListCreateImage(darwin.CGRectNull, darwin.CGWindowListOptionIncludingWindow, uint32(win.num),
		darwin.CGWindowImageBoundsIgnoreFraming|darwin.CGWindowImageBestResolution)
	if img == 0 {
		r.note = "the capture returned nil (refused, or the function is gone)"
		return r
	}
	defer darwin.CGImageRelease(img)
	iw, ih := darwin.CGImageGetWidth(img), darwin.CGImageGetHeight(img)
	full := drawImage(img, iw, ih, space)
	if full == nil || iw < phys.X || ih < phys.Y {
		r.note = fmt.Sprintf("capture %dx%d does not hold the %v content area", iw, ih, phys)
		return r
	}
	// The capture holds the whole frame, title bar on top.
	x0, y0 := (iw-phys.X)/2, ih-phys.Y
	sub := full.SubImage(image.Rect(x0, y0, x0+phys.X, y0+phys.Y)).(*image.RGBA)
	if uniform(sub) {
		r.note = fmt.Sprintf("capture %dx%d is blank (%v): refused", iw, ih, sub.RGBAAt(x0, y0))
		return r
	}
	r.pix = rebase(sub)
	r.note = fmt.Sprintf("window %d: capture %dx%d, content at (%d,%d)", win.num, iw, ih, x0, y0)
	return r
}

func renderContent(win nsWindow, phys image.Point, space uintptr) winReadback {
	r := winReadback{path: "renderInContext: (layer)"}
	layer := darwin.Send(darwin.Send(win.ns, esel.contentView), esel.layer)
	if layer == 0 {
		r.note = "the content view has no layer"
		return r
	}
	ctx := darwin.CGBitmapContextCreate(nil, phys.X, phys.Y, 8, phys.X*4, space, darwin.CGImageAlphaPremultipliedLast)
	if ctx == 0 {
		r.note = "CGBitmapContextCreate failed"
		return r
	}
	defer darwin.CGContextRelease(ctx)
	// renderInContext: draws in the layer's coordinates and a bitmap
	// context has y up: a flipped layer goes through a flipped transform.
	flipped := darwin.SendBool(layer, esel.isGeometryFlipped)
	if flipped {
		darwin.CGContextTranslateCTM(ctx, 0, float64(phys.Y))
		darwin.CGContextScaleCTM(ctx, 1, -1)
	}
	darwin.CGContextScaleCTM(ctx, win.scale, win.scale)
	darwin.Send(layer, esel.renderInContext, ctx)
	r.pix = contextPixels(ctx, phys.X, phys.Y)
	r.note = fmt.Sprintf("layer %v at scale %g, geometryFlipped %v", phys, win.scale, flipped)
	return r
}

func drawImage(img uintptr, w, h int, space uintptr) *image.RGBA {
	ctx := darwin.CGBitmapContextCreate(nil, w, h, 8, w*4, space, darwin.CGImageAlphaPremultipliedLast)
	if ctx == 0 {
		return nil
	}
	defer darwin.CGContextRelease(ctx)
	darwin.CGContextDrawImage(ctx, darwin.CGRect{Size: darwin.CGSize{Width: float64(w), Height: float64(h)}}, img)
	return contextPixels(ctx, w, h)
}

func contextPixels(ctx uintptr, w, h int) *image.RGBA {
	data := darwin.CGBitmapContextGetData(ctx)
	if data == nil {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(out.Pix, unsafe.Slice((*byte)(data), w*h*4))
	return out
}

// rebase copies img into an image whose bounds start at (0, 0).
func rebase(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(image.Rectangle{Max: img.Rect.Size()})
	for y := range out.Rect.Max.Y {
		copy(out.Pix[y*out.Stride:], img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):][:out.Rect.Max.X*4])
	}
	return out
}

func uniform(img *image.RGBA) bool {
	first := img.RGBAAt(img.Rect.Min.X, img.Rect.Min.Y)
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y += 7 {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x += 7 {
			if img.RGBAAt(x, y) != first {
				return false
			}
		}
	}
	return true
}

// frameDiff compares the read back with the frame on a grid: the share of
// samples within tol levels per channel, the share beyond far levels, and
// the largest difference.
type frameDiff struct {
	within, beyond float64
	maxDiff        int
	n              int
}

func (d frameDiff) String() string {
	return fmt.Sprintf("%d samples: %.1f%% within tolerance, %.1f%% far off, max channel difference %d", d.n, 100*d.within, 100*d.beyond, d.maxDiff)
}

func compareFrame(got, want *image.RGBA, tol, far int) frameDiff {
	var d frameDiff
	if got == nil || want == nil || got.Rect.Size() != want.Rect.Size() {
		return d
	}
	in, off := 0, 0
	for y := 0; y < want.Rect.Dy(); y += 3 {
		for x := 0; x < want.Rect.Dx(); x += 3 {
			a := got.RGBAAt(got.Rect.Min.X+x, got.Rect.Min.Y+y)
			b := want.RGBAAt(want.Rect.Min.X+x, want.Rect.Min.Y+y)
			diff := chanDiff(a, b)
			d.maxDiff = max(d.maxDiff, diff)
			if diff <= tol {
				in++
			}
			if diff > far {
				off++
			}
			d.n++
		}
	}
	if d.n > 0 {
		d.within, d.beyond = float64(in)/float64(d.n), float64(off)/float64(d.n)
	}
	return d
}

func chanDiff(a, b color.RGBA) int {
	d := 0
	for _, v := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}} {
		x := int(v[0]) - int(v[1])
		d = max(d, x, -x)
	}
	return d
}

// shown is the outcome of waiting for the window to show something.
type shown struct {
	rb       winReadback // the read back that satisfied the condition
	st       cocoaState
	diff     frameDiff
	attempts int
}

// waitShown reads the window back until cond holds for one of the read
// backs (the window server's capture first) against the frame the host
// shows (the window fades in, the window server composites later than
// Present returns), for at most 15 s; it logs what each path saw.
func (c *cocoaE2E) waitShown(what string, cond func(st cocoaState, d frameDiff) bool) shown {
	c.t.Helper()
	start := time.Now()
	var sh shown
	report := func() string {
		var parts []string
		for _, rb := range sh.st.reads {
			if rb.pix == nil {
				parts = append(parts, fmt.Sprintf("%s: no pixels (%s)", rb.path, rb.note))
				continue
			}
			parts = append(parts, fmt.Sprintf("%s: %s (%s)", rb.path, compareFrame(rb.pix, sh.st.frame, tolerance(rb), 64), rb.note))
		}
		return strings.Join(parts, "; ")
	}
	for {
		sh.attempts++
		sh.st = c.state(true)
		if sh.st.frame != nil {
			for _, rb := range sh.st.reads {
				if rb.pix == nil {
					continue
				}
				d := compareFrame(rb.pix, sh.st.frame, tolerance(rb), 64)
				if cond(sh.st, d) {
					sh.rb, sh.diff = rb, d
					break
				}
			}
			if sh.rb.pix != nil {
				break
			}
		}
		if time.Since(start) > 15*time.Second {
			c.t.Fatalf("%s: not seen in the window after %d read backs in %v; host windows %d, visible NSWindows %d, frame %v; last read backs: %s",
				what, sh.attempts, time.Since(start).Round(time.Millisecond), sh.st.nhost, sh.st.nvis, sh.st.frame != nil, report())
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.t.Logf("%s: seen through %s after %d read back(s) in %v; %s", what, sh.rb.path, sh.attempts, time.Since(start).Round(time.Millisecond), report())
	return sh
}

// tolerance is the channel difference a read back may have from the frame:
// the window server's color management may change the captured pixels a
// little; the layer holds them exactly.
func tolerance(rb winReadback) int {
	if rb.exact() {
		return 1
	}
	return 32
}

// showsFrame: the window shows the child's frame (no overlay).
func showsFrame(_ cocoaState, d frameDiff) bool { return d.within >= 0.99 }

func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// TestDevEndToEndCocoa runs tela dev on testdata/devapp with the Cocoa
// backend as its host (ADR-111, ADR-112, ADR-222): the first build's frame
// reaches the real window, clicks reach the app through AppKit, a source
// change swaps the child under the same NSWindow (one window number,
// visible in every sample taken during the swaps) keeping its state, a
// build error draws the overlay over the last good frame and the fix
// removes it, and quitting returns from the event loop with the window
// closed and every child gone.
func TestDevEndToEndCocoa(t *testing.T) {
	c := startCocoaE2E(t)
	started := time.Now()
	c.expect(0, "devapp: counter", "devapp: counter n=0 pkg=0")
	t.Logf("first build and start (to the first build's first line): %v", time.Since(started).Round(time.Millisecond))

	// 1. The first frame reaches the window, which has the app's title and
	// size.
	first := c.waitShown("the first frame", showsFrame)
	t.Logf("first build to the frame on screen: %v", time.Since(started).Round(time.Millisecond))
	win, pw := first.st.ns, first.st.pw
	m := pw.Metrics()
	t.Logf("%v; host window metrics %v (physical %v)", win, m, m.Physical())
	if first.st.nhost != 1 || first.st.nvis != 1 {
		t.Errorf("%d host windows and %d visible NSWindows, want 1 and 1", first.st.nhost, first.st.nvis)
	}
	if win.title != "devapp" {
		t.Errorf("the window's title is %q, want devapp (App.Title)", win.title)
	}
	if m.Size != (platform.Size{W: 480, H: 400}) {
		t.Errorf("the window's size is %v, want 480x400 (App.Width, App.Height)", m.Size)
	}
	if float32(win.content.Width) != m.Size.W || float32(win.content.Height) != m.Size.H || win.scale != m.Scale {
		t.Errorf("AppKit's content view is %gx%g at scale %g, the host's window says %v", win.content.Width, win.content.Height, win.scale, m)
	}
	c.startSampling()

	// The header's color, where the edit below changes it.
	header := image.Pt(int(20*m.Scale), int(40*m.Scale)) // below the header's text line
	blue := first.st.frame.RGBAAt(header.X, header.Y)
	t.Logf("header at %v: frame %v, window %v", header, blue, first.rb.pix.RGBAAt(header.X, header.Y))

	// 2. Clicks reach the app through AppKit and the host.
	t0 := time.Now()
	c.click(win, 60, 68)
	c.expect(0, "devapp: counter", "devapp: counter n=1 pkg=1")
	t.Logf("click to the app's rebuild: %v", time.Since(t0).Round(time.Millisecond))
	c.click(win, 60, 68)
	c.expect(0, "devapp: counter", "devapp: counter n=2 pkg=2")
	c.click(win, 60, 148)
	c.expect(0, "devapp: sidebar", "devapp: sidebar n=1")
	pid1 := first.st.active

	// 3. Edit a class and a text: the new build takes over the same
	// NSWindow and keeps the state.
	took, mark := c.edit(func(s string) string {
		s = strings.Replace(s, "bg-blue-500", "bg-green-500", 1)
		return strings.Replace(s, `Text("Version A")`, `Text("Version B")`, 1)
	})
	t.Logf("save to swapped (class and text): %v", took.Round(time.Millisecond))
	swapped := c.waitShown("the new build's frame", func(st cocoaState, d frameDiff) bool {
		return st.frame.RGBAAt(header.X, header.Y) != blue && d.within >= 0.99
	})
	green := swapped.st.frame.RGBAAt(header.X, header.Y)
	shownHeader := swapped.rb.pix.RGBAAt(header.X, header.Y)
	t.Logf("header after the swap: frame %v, window %v (was %v)", green, shownHeader, blue)
	if chanDiff(shownHeader, green) >= chanDiff(shownHeader, blue) {
		t.Errorf("the window's header is %v, nearer the old build's %v than the new one's %v", shownHeader, blue, green)
	}
	if swapped.st.pw != pw || swapped.st.ns.ns != win.ns || swapped.st.ns.num != win.num || swapped.st.nvis != 1 {
		t.Errorf("after the swap the window is %v (host window %p, %d visible), want the same %v (host window %p)",
			swapped.st.ns, swapped.st.pw, swapped.st.nvis, win, pw)
	}
	c.expect(mark, "devapp: counter", "devapp: counter n=2 pkg=2")
	c.expect(mark, "devapp: restore", "devapp: restore 2")
	c.expect(mark, "devapp: sidebar", "devapp: sidebar n=1")
	pid2 := swapped.st.active
	t.Logf("child %d replaced by %d", pid1, pid2)
	if pid2 == pid1 || pid2 == 0 {
		t.Errorf("the active child is %d after the swap, was %d", pid2, pid1)
	}
	c.until("the old child to exit", func() bool { return processGone(pid1) })

	// The new build gets the clicks on the same window.
	mark = len(c.out.String())
	c.click(win, 60, 68)
	c.expect(mark, "devapp: counter", "devapp: counter n=3 pkg=3")

	// 4. A compile error: the overlay is drawn over the last good frame in
	// the real window.
	b, err := os.ReadFile(c.src)
	if err != nil {
		t.Fatal(err)
	}
	good := string(b)
	t0 = time.Now()
	if err := os.WriteFile(c.src, []byte(good+"\nfunc broken( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.until("the build error", func() bool {
		return slices.ContainsFunc(c.view.entries(), func(l string) bool { return strings.HasPrefix(l, "show 1: build failed") })
	})
	t.Logf("save to the build error: %v", time.Since(t0).Round(time.Millisecond))
	ov := c.waitShown("the error overlay", func(_ cocoaState, d frameDiff) bool { return d.beyond >= 0.2 })
	if ov.st.ns.num != win.num || ov.st.nvis != 1 {
		t.Errorf("with the overlay the window is %v (%d visible), want %v", ov.st.ns, ov.st.nvis, win)
	}

	// 5. The fix: the overlay goes, the new build keeps the state.
	mark = len(c.out.String())
	t0 = time.Now()
	if err := os.WriteFile(c.src, []byte(good+"\n// fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.until("the problem to clear", func() bool {
		es := c.view.entries()
		return len(es) > 0 && es[len(es)-1] == "clear"
	})
	t.Logf("save to the fixed build: %v", time.Since(t0).Round(time.Millisecond))
	fixed := c.waitShown("the fixed build without the overlay", showsFrame)
	c.expect(mark, "devapp: counter", "devapp: counter n=3 pkg=3")
	c.expect(mark, "devapp: restore", "devapp: restore 3")
	if fixed.st.ns.num != win.num || fixed.st.pw != pw || fixed.st.nvis != 1 {
		t.Errorf("after the fix the window is %v (%d visible), want %v", fixed.st.ns, fixed.st.nvis, win)
	}
	pid3 := fixed.st.active

	// The window never closed: every sample during the swaps saw it alone.
	samples, mismatch := c.stopSampling()
	t.Logf("%d samples of the visible windows, every %v: first [%s]", len(samples), windowSampleEvery, strings.Join(samples[:min(1, len(samples))], ""))
	if len(samples) < 10 {
		t.Errorf("only %d samples of the visible windows", len(samples))
	}
	if len(samples) > 0 && samples[0] != fmt.Sprint(win.num) {
		t.Errorf("the first sample saw windows [%s], want [%d]", samples[0], win.num)
	}
	if mismatch != "" {
		t.Errorf("the window did not stay up through the swaps: %s", mismatch)
	}

	// 6. Quit: the event loop returns, the window is closed and no child
	// is left.
	t0 = time.Now()
	code, runErr := c.quit()
	t.Logf("quit to the event loop's return: %v (exit code %d, host.Run %v)", time.Since(t0).Round(time.Millisecond), code, runErr)
	if code != 0 || runErr != nil {
		t.Errorf("tela dev exited with %d and host.Run returned %v, want 0 and nil", code, runErr)
	}
	var after []nsWindow
	<-onMainAsync(func() {
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		after = visibleWindows()
	})
	t.Logf("visible windows after Run returned: %v", after)
	if len(after) != 0 {
		t.Errorf("%d windows still visible after the event loop returned: %v", len(after), after)
	}
	// The supervisor reaps its children before it returns; allow the
	// kernel a moment anyway.
	deadline := time.Now().Add(5 * time.Second)
	for _, pid := range []int{pid2, pid3} {
		for pid != 0 && !processGone(pid) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if pid != 0 && !processGone(pid) {
			t.Errorf("child %d still runs 5s after tela dev quit", pid)
		}
	}
}
