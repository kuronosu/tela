//go:build darwin

package cocoa

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// These tests run the real backend and open windows. They need a window
// server session (macos-latest runners have one) and skip without it, like
// win32's tests without an interactive desktop. Only the clipboard tests
// touch the user's data (clipboardAllowed: CI or TELA_LIVE). AppKit runs on
// the main thread only: TestMain keeps the main goroutine there and serves
// onMain while the tests run on other goroutines.

func init() { runtime.LockOSThread() }

var mainCalls = make(chan func())

func TestMain(m *testing.M) {
	done := make(chan int, 1)
	go func() { done <- m.Run() }()
	for {
		select {
		case f := <-mainCalls:
			f()
		case code := <-done:
			os.Exit(code)
		}
	}
}

// onMain runs f on the main thread and returns when it returns; a panic in
// f is raised again on the caller. f must not call t.Fatal or t.Skip
// (runtime.Goexit would end the main goroutine): use t.Error and return.
func onMain(f func()) {
	var p any
	failed := false
	ch := make(chan struct{})
	mainCalls <- func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				p, failed = r, true
			}
		}()
		f()
	}
	<-ch
	if failed {
		panic(p)
	}
}

var logOnce = map[string]bool{}

// newBackend creates a backend on the main thread, or skips the test when
// there is no window server session.
func newBackend(t *testing.T) *Backend {
	t.Helper()
	var b *Backend
	var err error
	var version string
	onMain(func() {
		b, err = New()
		if err == nil {
			pool := darwin.PoolPush()
			info := darwin.SendClass(darwin.GetClass("NSProcessInfo"), darwin.Sel("processInfo"))
			version = darwin.GoString(darwin.Send(info, darwin.Sel("operatingSystemVersionString")))
			darwin.PoolPop(pool)
		}
	})
	if errors.Is(err, platform.ErrUnavailable) {
		t.Skipf("cocoa backend unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !logOnce["version"] {
		logOnce["version"] = true
		t.Logf("macOS %s, GOARCH=%s", version, runtime.GOARCH)
	}
	parkPointer(t)
	return b
}

// parkPointer moves the real cursor to the left edge of the main screen,
// at mid-height, before a test opens its windows: no test window reaches
// there (they are centered, or placed from x=60), so where the runner or
// an earlier test left the cursor adds no enter or move events to the
// exact pointer sequences that the input tests (TestPointer,
// TestDropTarget, TestCaptionRegions) build from synthetic events. It is
// not a hot corner. CGWarpMouseCursorPosition generates no event and needs
// no permission.
func parkPointer(t *testing.T) {
	onMain(func() {
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		screens := darwin.SendClass(darwin.GetClass("NSScreen"), darwin.Sel("screens"))
		if screens == 0 || darwin.SendUint(screens, darwin.Sel("count")) == 0 {
			return
		}
		main := darwin.Send(screens, darwin.Sel("objectAtIndex:"), uint(0))
		h := darwin.SendRect(main, darwin.Sel("frame")).Size.Height
		before := darwin.SendPoint(darwin.ID(app.event), darwin.Sel("mouseLocation"))
		if err := darwin.CGWarpMouseCursorPosition(darwin.CGPoint{X: 2, Y: h / 2}); err != 0 {
			t.Logf("note: parking the pointer failed (CGError %d); it stays at %v (screen)", err, before)
			return
		}
		if !logOnce["park"] {
			logOnce["park"] = true
			after := darwin.SendPoint(darwin.ID(app.event), darwin.Sel("mouseLocation"))
			t.Logf("parked the pointer at the main screen's left edge: from %v to %v (screen, y up)", before, after)
		}
	})
}

// TestConformance runs the shared platform suite. Every subtest runs; the
// clipboard ones replace the clipboard's content, so they run on CI or
// with TELA_LIVE only (clipboardAllowed).
func TestConformance(t *testing.T) {
	var skip []string
	if !clipboardAllowed() {
		skip = append(skip, "Clipboard", "ClipboardData")
	}
	platformtest.Run(t, platformtest.Config{
		New:        func(t *testing.T) platform.Backend { return newBackend(t) },
		OnUIThread: onMain,
		Skip:       skip,
	})
}

// TestNewOffMainThread checks that New refuses a thread other than the
// main one with an error instead of letting AppKit abort.
func TestNewOffMainThread(t *testing.T) {
	b, err := New()
	if err == nil {
		t.Fatalf("New on a test goroutine = %v, nil; want an error (AppKit runs on the main thread only)", b)
	}
	t.Logf("New off the main thread: %v", err)
}

// TestPanicInHandlerIsReraisedByRun: a panic in the handler, called from a
// run loop source callback, stops [NSApp run] and comes out of Run.
func TestPanicInHandlerIsReraisedByRun(t *testing.T) {
	b := newBackend(t)
	var got any
	onMain(func() {
		defer func() { got = recover() }()
		if _, err := b.NewWindow(platform.WindowOptions{Title: "panic", Size: platform.Size{W: 120, H: 80}}); err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		time.AfterFunc(10*time.Second, b.Quit) // a failure, not a hang
		_ = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
			if _, ok := e.(platform.FrameEvent); ok {
				panic("tela test panic")
			}
		}))
	})
	if got != "tela test panic" {
		t.Errorf("Run recovered %v, want the handler's panic value", got)
	}
}

func presentSolid(t *testing.T, w platform.Window, c color.RGBA) {
	t.Helper()
	fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
	for i := 0; i < len(fb.Pix); i += 4 {
		fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	if err := w.Present(fb, nil); err != nil {
		t.Errorf("Present: %v", err)
	}
}

// --- presentation read back ---

// The test pattern: four quadrants and a 1-pixel black and white checker
// strip across the middle, which any resampling (a scale mismatch, a
// gravity that stretches) would blur to gray.
var (
	quadTL = color.RGBA{220, 40, 40, 255}
	quadTR = color.RGBA{40, 180, 60, 255}
	quadBL = color.RGBA{40, 60, 200, 255}
	quadBR = color.RGBA{240, 240, 240, 255}
)

// checkerStrip returns the strip's rectangle in a frame of size p.
func checkerStrip(p image.Point) image.Rectangle {
	return image.Rect(p.X/4, p.Y/2-4, 3*p.X/4, p.Y/2+4)
}

func checkerColor(x, y int) color.RGBA {
	if (x+y)%2 == 0 {
		return color.RGBA{0, 0, 0, 255}
	}
	return color.RGBA{255, 255, 255, 255}
}

func testPattern(p image.Point) *image.RGBA {
	fb := image.NewRGBA(image.Rectangle{Max: p})
	strip := checkerStrip(p)
	for y := range p.Y {
		for x := range p.X {
			var c color.RGBA
			switch {
			case image.Pt(x, y).In(strip):
				c = checkerColor(x, y)
			case y < p.Y/2 && x < p.X/2:
				c = quadTL
			case y < p.Y/2:
				c = quadTR
			case x < p.X/2:
				c = quadBL
			default:
				c = quadBR
			}
			fb.SetRGBA(x, y, c)
		}
	}
	return fb
}

// readback is one way of reading the window's content back.
type readback struct {
	path string      // how the pixels were read
	pix  *image.RGBA // the content area, physical pixels, top-left origin; nil if refused
	note string
}

// comparison is a read back compared with the pattern.
type comparison struct {
	maxDiff    int    // largest channel difference at the sampled pixels
	placement  bool   // every quadrant sample is nearest to its own color
	checker    bool   // every strip pixel is nearer its black or white than the other
	upsideDown bool   // the quadrants match the pattern flipped vertically
	detail     string // the four quadrant samples
}

func (c comparison) ok(exact bool) bool {
	if exact {
		return c.placement && c.checker && c.maxDiff <= 1
	}
	return c.placement && c.checker
}

func channelDiff(a, b color.RGBA) int {
	d := 0
	for _, v := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}} {
		x := int(v[0]) - int(v[1])
		d = max(d, x, -x)
	}
	return d
}

func nearest(c color.RGBA, palette ...color.RGBA) int {
	best, bi := 1<<30, 0
	for i, p := range palette {
		if d := channelDiff(c, p); d < best {
			best, bi = d, i
		}
	}
	return bi
}

func compare(pix *image.RGBA) comparison {
	p := pix.Rect.Size()
	at := func(x, y int) color.RGBA { return pix.RGBAAt(pix.Rect.Min.X+x, pix.Rect.Min.Y+y) }
	palette := []color.RGBA{quadTL, quadTR, quadBL, quadBR}
	centers := []image.Point{{p.X / 4, p.Y / 4}, {3 * p.X / 4, p.Y / 4}, {p.X / 4, 3 * p.Y / 4}, {3 * p.X / 4, 3 * p.Y / 4}}
	var c comparison
	c.placement = true
	got := make([]int, 4)
	for i, ctr := range centers {
		// A 5x5 patch around each center.
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				s := at(ctr.X+dx, ctr.Y+dy)
				c.maxDiff = max(c.maxDiff, channelDiff(s, palette[i]))
				if nearest(s, palette...) != i {
					c.placement = false
				}
			}
		}
		s := at(ctr.X, ctr.Y)
		got[i] = nearest(s, palette...)
		c.detail += fmt.Sprintf(" q%d=%v", i, s)
	}
	c.upsideDown = got[0] == 2 && got[1] == 3 && got[2] == 0 && got[3] == 1
	c.checker = true
	black, white := color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 255}
	strip := checkerStrip(p)
	for y := strip.Min.Y; y < strip.Max.Y; y++ {
		for x := strip.Min.X; x < strip.Max.X; x++ {
			s, want := at(x, y), checkerColor(x, y)
			c.maxDiff = max(c.maxDiff, channelDiff(s, want))
			if (nearest(s, black, white) == 0) != (want == black) {
				c.checker = false
			}
		}
	}
	return c
}

// bitmap draws the CGImage img into a new sRGB RGBA image.
func bitmap(img uintptr, w, h int, space uintptr) *image.RGBA {
	ctx := darwin.CGBitmapContextCreate(nil, w, h, 8, w*4, space, darwin.CGImageAlphaPremultipliedLast)
	if ctx == 0 {
		return nil
	}
	defer darwin.CGContextRelease(ctx)
	darwin.CGContextDrawImage(ctx, darwin.CGRect{Size: darwin.CGSize{Width: float64(w), Height: float64(h)}}, img)
	return copyContext(ctx, w, h)
}

func copyContext(ctx uintptr, w, h int) *image.RGBA {
	data := darwin.CGBitmapContextGetData(ctx)
	if data == nil {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(out.Pix, unsafe.Slice((*byte)(data), w*h*4))
	return out
}

// captureWindow reads the window as the window server composited it
// (CGWindowListCreateImage of its window number), cropped to the content
// area: the capture holds the whole frame, title bar on top.
func captureWindow(w *Window) readback {
	r := readback{path: "CGWindowListCreateImage (window server)"}
	phys := w.Metrics().Physical()
	num := darwin.SendInt(w.ns, sel.windowNumber)
	img := darwin.CGWindowListCreateImage(darwin.CGRectNull, darwin.CGWindowListOptionIncludingWindow, uint32(num),
		darwin.CGWindowImageBoundsIgnoreFraming|darwin.CGWindowImageBestResolution)
	if img == 0 {
		r.note = fmt.Sprintf("window %d: the capture returned nil (refused, or the function is gone)", num)
		return r
	}
	defer darwin.CGImageRelease(img)
	iw, ih := darwin.CGImageGetWidth(img), darwin.CGImageGetHeight(img)
	full := bitmap(img, iw, ih, w.b.srgb)
	if full == nil || iw < phys.X || ih < phys.Y {
		r.note = fmt.Sprintf("window %d: capture %dx%d does not hold the %v content area", num, iw, ih, phys)
		return r
	}
	x0, y0 := (iw-phys.X)/2, ih-phys.Y
	sub := full.SubImage(image.Rect(x0, y0, x0+phys.X, y0+phys.Y)).(*image.RGBA)
	// A refused capture of our own window comes back blank.
	blank := true
	first := sub.RGBAAt(x0, y0)
	for y := sub.Rect.Min.Y; y < sub.Rect.Max.Y && blank; y += 7 {
		for x := sub.Rect.Min.X; x < sub.Rect.Max.X; x += 7 {
			if sub.RGBAAt(x, y) != first {
				blank = false
				break
			}
		}
	}
	r.note = fmt.Sprintf("window %d: capture %dx%d, content at (%d,%d)", num, iw, ih, x0, y0)
	if blank {
		r.note += fmt.Sprintf(", blank (%v): treated as refused", first)
		return r
	}
	r.pix = sub
	return r
}

// renderLayer renders the view's layer into a bitmap context
// (renderInContext:): what the layer holds, before the window server's
// color management.
func renderLayer(w *Window) readback {
	r := readback{path: "renderInContext: (layer)"}
	phys := w.Metrics().Physical()
	ctx := darwin.CGBitmapContextCreate(nil, phys.X, phys.Y, 8, phys.X*4, w.b.srgb, darwin.CGImageAlphaPremultipliedLast)
	if ctx == 0 {
		r.note = "CGBitmapContextCreate failed"
		return r
	}
	defer darwin.CGContextRelease(ctx)
	scale := w.Metrics().Scale
	// renderInContext: draws in the layer's coordinates; a bitmap context
	// has y up, so a layer with flipped geometry (y down) is drawn through
	// a flipped transform to come out as it is on screen.
	flipped := darwin.SendBool(w.layer, darwin.Sel("isGeometryFlipped"))
	if flipped {
		darwin.CGContextTranslateCTM(ctx, 0, float64(phys.Y))
		darwin.CGContextScaleCTM(ctx, 1, -1)
	}
	darwin.CGContextScaleCTM(ctx, scale, scale)
	darwin.Send(w.layer, sel.renderInContext, ctx)
	r.pix = copyContext(ctx, phys.X, phys.Y)
	r.note = fmt.Sprintf("%v pixels at scale %v, geometryFlipped %v, contentsAreFlipped %v, gravity %q",
		phys, scale, flipped, darwin.SendBool(w.layer, sel.contentsAreFlipped), darwin.GoString(w.gravity))
	return r
}

// TestPresentReadBack presents the test pattern and reads the window's
// pixels back: from the window server when it allows it (the real path to
// the screen, subject to its color management), else from the layer. It
// logs which path ran and the largest channel difference.
func TestPresentReadBack(t *testing.T) {
	b := newBackend(t)
	var w *Window
	var final, layer readback
	var cmp comparison
	attempts := 0
	start := time.Now()
	onMain(func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "tela readback", Size: platform.Size{W: 200, H: 160}})
		if err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		w = pw.(*Window)
		time.AfterFunc(30*time.Second, b.Quit) // a failure, not a hang
		presented := false
		_ = b.Run(platform.HandlerFunc(func(ew platform.Window, e platform.Event) {
			switch e.(type) {
			case platform.FrameEvent:
				// Every frame presents the pattern at the current size (a
				// configure after the window appears forces one).
				if err := ew.Present(testPattern(ew.Metrics().Physical()), nil); err != nil {
					t.Errorf("Present: %v", err)
				}
				if !presented {
					presented = true
					time.AfterFunc(300*time.Millisecond, b.Wake)
				}
			case platform.WakeEvent:
				// The window may still be fading in: retry for a while.
				attempts++
				pool := darwin.PoolPush()
				capture := captureWindow(w)
				final = capture
				exact := false
				if capture.pix == nil {
					// Refused or blank (screen recording permission):
					// read the layer instead, which has no color
					// management to excuse differences.
					l := renderLayer(w)
					final = readback{path: l.path, pix: l.pix, note: "capture: " + capture.note + "; layer: " + l.note}
					exact = true
				}
				darwin.PoolPop(pool)
				if final.pix != nil {
					cmp = compare(final.pix)
				}
				if (final.pix != nil && cmp.ok(exact)) || attempts >= 15 {
					pool := darwin.PoolPush()
					layer = renderLayer(w) // logged for comparison
					darwin.PoolPop(pool)
					b.Quit()
					return
				}
				time.AfterFunc(300*time.Millisecond, b.Wake)
			}
		}))
	})
	if w == nil {
		return
	}
	t.Logf("metrics %v (physical %v), %d attempt(s) in %v", w.Metrics(), w.Metrics().Physical(), attempts, time.Since(start).Round(time.Millisecond))
	t.Logf("path: %s; %s", final.path, final.note)
	if final.pix == nil {
		t.Fatalf("no read back path worked")
	}
	t.Logf("quadrant centers:%s", cmp.detail)
	t.Logf("max channel difference %d (placement %v, checker %v)", cmp.maxDiff, cmp.placement, cmp.checker)
	if layer.pix != nil && final.path != layer.path {
		lc := compare(layer.pix)
		t.Logf("for comparison, the layer (renderInContext:): max channel difference %d (placement %v, checker %v),%s",
			lc.maxDiff, lc.placement, lc.checker, lc.detail)
	}
	exact := final.path == "renderInContext: (layer)"
	switch {
	case cmp.upsideDown:
		t.Errorf("the frame is upside down (top quadrants at the bottom)")
	case !cmp.placement:
		t.Errorf("the quadrants are not where they were presented")
	case !cmp.checker:
		t.Errorf("the 1-pixel checker strip was not read back pixel for pixel (resampled: a scale or gravity mismatch?)")
	case exact && cmp.maxDiff > 1:
		t.Errorf("the layer's pixels differ from the frame by up to %d levels", cmp.maxDiff)
	case cmp.maxDiff > 2:
		t.Logf("note: the window server changed the pixels by up to %d levels (color management)", cmp.maxDiff)
	}
}
