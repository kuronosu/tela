//go:build linux

package x11

import (
	"errors"
	"fmt"
	"image"
	"net"
	"os"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// Live tests open real windows on $DISPLAY. They only run with TELA_LIVE=1.
// They verify pixels with GetImage on the window itself, which reads the
// X server's copy of the window and does not depend on the compositor
// (under Xwayland, every toplevel has its own pixmap, so overlapping
// windows and compositor scaling do not affect it).

func liveBackend(t *testing.T) *Backend {
	t.Helper()
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run against the real X server")
	}
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY is not set")
	}
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

func TestLiveConformance(t *testing.T) {
	platformtest.Run(t, platformtest.Config{
		New: func(t *testing.T) platform.Backend { return liveBackend(t) },
		// X11 selections are not implemented yet; the clipboard reports
		// ErrClipboardUnsupported (tested by TestClipboardIsAsyncAndUnsupported).
		Skip: []string{"Clipboard"},
	})
}

func TestLiveServerInfo(t *testing.T) {
	b := liveBackend(t)
	defer b.conn.Close()
	t.Logf("scale %v (Xft.dpi %v), BIG-REQUESTS %v, PutImage limit %d bytes, root %dx%d",
		b.scale, b.scale*defaultDPI, b.bigRequests, b.maxRequest, b.screen.WidthInPixels, b.screen.HeightInPixels)
}

// pattern returns the test pattern color at (x, y), opaque.
func pattern(seed, x, y int) (r, g, b uint8) {
	return uint8(x*7 ^ y + seed), uint8(y*3 + x*seed), uint8((x + y) * 5)
}

// fill draws the pattern into fb inside r.
func fill(fb *image.RGBA, seed int, r image.Rectangle) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := fb.PixOffset(x, y)
			fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2] = pattern(seed, x, y)
			fb.Pix[i+3] = 0xff
		}
	}
}

// expectation describes the expected content of a window: the seed of the
// pattern inside each rectangle, later rectangles on top.
type expectation struct {
	r    image.Rectangle
	seed int
}

func expectedAt(exp []expectation, x, y int) (int, bool) {
	seed, ok := 0, false
	for _, e := range exp {
		if image.Pt(x, y).In(e.r) {
			seed, ok = e.seed, true
		}
	}
	return seed, ok
}

// checkWindow reads the window with GetImage and compares it with exp. It
// returns an error describing the first mismatch.
func checkWindow(b *Backend, w *Window, exp []expectation) error {
	geo, err := xproto.GetGeometry(b.conn, xproto.Drawable(w.id)).Reply()
	if err != nil {
		return fmt.Errorf("GetGeometry: %v", err)
	}
	size := w.pixSize
	if int(geo.Width) != size.X || int(geo.Height) != size.Y {
		return fmt.Errorf("window is %dx%d, last presented frame %v", geo.Width, geo.Height, size)
	}
	img, err := xproto.GetImage(b.conn, xproto.ImageFormatZPixmap, xproto.Drawable(w.id), 0, 0,
		uint16(size.X), uint16(size.Y), 0xffffffff).Reply()
	if err != nil {
		return fmt.Errorf("GetImage: %v", err)
	}
	if img.Depth != 24 || len(img.Data) < 4*size.X*size.Y {
		return fmt.Errorf("GetImage returned depth %d with %d bytes for %v", img.Depth, len(img.Data), size)
	}
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			seed, ok := expectedAt(exp, x, y)
			if !ok {
				continue
			}
			r, g, bl := pattern(seed, x, y)
			p := img.Data[4*(y*size.X+x):]
			// ZPixmap, 32 bpp, LSBFirst: B, G, R, unused.
			if p[0] != bl || p[1] != g || p[2] != r {
				return fmt.Errorf("pixel (%d,%d) = BGR %d,%d,%d, want %d,%d,%d (seed %d)", x, y, p[0], p[1], p[2], bl, g, r, seed)
			}
		}
	}
	return nil
}

// TestLivePixels presents known patterns and reads them back from the X
// server: a full frame (one BIG-REQUESTS PutImage when available), a
// repaint after Expose from the retained copy, and a partial frame split
// into many tiny requests.
func TestLivePixels(t *testing.T) {
	b := liveBackend(t)
	timeout := time.AfterFunc(20*time.Second, b.Quit)
	defer timeout.Stop()

	nw, err := b.NewWindow(platform.WindowOptions{Title: "tela x11 pixels", AppID: "dev.tela.x11test",
		Size: platform.Size{W: 320, H: 240}, FixedSize: true})
	if err != nil {
		t.Fatal(err)
	}
	w := nw.(*Window)
	var fb *image.RGBA
	var exp []expectation
	stage, retries := 0, 0
	partial := image.Rect(13, 7, 301, 229)
	retry := func(err error) {
		retries++
		if retries > 30 {
			t.Errorf("stage %d: %v", stage, err)
			b.Quit()
			return
		}
		time.AfterFunc(100*time.Millisecond, b.Wake)
	}
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			// Every frame (the first one and those after a window
			// manager resize) is a full frame with seed 1.
			p := w.Metrics().Physical()
			fb = image.NewRGBA(image.Rectangle{Max: p})
			fill(fb, 1, fb.Rect)
			if err := w.Present(fb, nil); err != nil {
				t.Errorf("Present: %v", err)
			}
			exp = []expectation{{fb.Rect, 1}}
			stage, retries = 1, 0
			time.AfterFunc(200*time.Millisecond, b.Wake)
		case platform.WakeEvent:
			if err := checkWindow(b, w, exp); err != nil {
				retry(err)
				return
			}
			retries = 0
			switch stage {
			case 1:
				t.Logf("full frame %v verified (BIG-REQUESTS %v)", w.pixSize, b.bigRequests)
				// Scribble over the window behind the backend's back,
				// then make the server send Expose: the backend must
				// repaint from its retained copy.
				gc, _ := xproto.NewGcontextId(b.conn)
				xproto.CreateGC(b.conn, gc, xproto.Drawable(w.id), xproto.GcForeground, []uint32{0x00ff00})
				xproto.PolyFillRectangle(b.conn, xproto.Drawable(w.id), gc, []xproto.Rectangle{{X: 20, Y: 20, Width: 100, Height: 80}})
				xproto.FreeGC(b.conn, gc)
				if checkWindow(b, w, exp) == nil {
					t.Errorf("drawing over the window did not change it; the Expose check proves nothing")
				}
				xproto.ClearArea(b.conn, true, w.id, 0, 0, 0, 0)
				stage = 2
				time.AfterFunc(100*time.Millisecond, b.Wake)
			case 2:
				t.Logf("Expose repaint from the retained copy verified")
				// A partial frame through tiny requests: rows do not fit,
				// so each row is split into 37-pixel runs.
				saved := b.maxRequest
				b.maxRequest = putImageHeaderLen(b.bigRequests) + 4*37
				fill(fb, 2, fb.Rect) // only the damage may be read
				if err := w.Present(fb, []image.Rectangle{partial}); err != nil {
					t.Errorf("Present: %v", err)
				}
				b.maxRequest = saved
				exp = append(exp, expectation{partial.Intersect(fb.Rect), 2})
				stage = 3
				time.AfterFunc(100*time.Millisecond, b.Wake)
			case 3:
				t.Logf("partial frame through %d tiny PutImage requests verified",
					len(splitForRequests(partial, putImageHeaderLen(b.bigRequests)+4*37, putImageHeaderLen(b.bigRequests))))
				b.Quit()
			}
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stage != 3 {
		t.Fatalf("stopped at stage %d", stage)
	}
}

// TestLivePresentCost measures full-frame Present at 1280x720 (BGRX
// conversion plus PutImage), including the server's processing, which a
// GetInputFocus round trip at the end waits for.
func TestLivePresentCost(t *testing.T) {
	b := liveBackend(t)
	timeout := time.AfterFunc(20*time.Second, b.Quit)
	defer timeout.Stop()
	nw, err := b.NewWindow(platform.WindowOptions{Title: "tela x11 present cost", Size: platform.Size{W: 1280, H: 720}, FixedSize: true})
	if err != nil {
		t.Fatal(err)
	}
	const n = 60
	err = b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok {
			return
		}
		fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
		fill(fb, 4, fb.Rect)
		_ = w.Present(fb, nil) // maps the window
		time.Sleep(100 * time.Millisecond)
		start, cpu0 := time.Now(), cpuTime(t)
		for range n {
			if err := w.Present(fb, nil); err != nil {
				t.Errorf("Present: %v", err)
			}
		}
		xproto.GetInputFocus(b.conn).Reply()
		d, cpu := time.Since(start), cpuTime(t)-cpu0
		t.Logf("%d full Presents of %v: %v each (process CPU %v each), %d request(s) per frame",
			n, fb.Rect.Size(), d/n, cpu/n, len(splitForRequests(fb.Rect, b.maxRequest, putImageHeaderLen(b.bigRequests))))
		b.Quit()
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = nw
}

// cpuTime returns the CPU time (user + system) used by the process.
func cpuTime(t *testing.T) time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		t.Fatal(err)
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// TestLiveIdleCPU checks the "0% CPU when idle" criterion: after a window
// is presented and nothing is requested, the loop sleeps without timers.
func TestLiveIdleCPU(t *testing.T) {
	b := liveBackend(t)
	timeout := time.AfterFunc(10*time.Second, b.Quit)
	defer timeout.Stop()
	nw, err := b.NewWindow(platform.WindowOptions{Title: "tela x11 idle", Size: platform.Size{W: 200, H: 150}, FixedSize: true})
	if err != nil {
		t.Fatal(err)
	}
	const idle = 2 * time.Second
	var cpu0, cpuIdle time.Duration
	var iter0, iterIdle int64
	var wall0 time.Time
	phase := 0
	err = b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			fill(fb, 3, fb.Rect)
			_ = w.Present(fb, nil)
			if phase == 0 {
				phase = 1
				// Let the window manager map and settle the window.
				time.AfterFunc(500*time.Millisecond, b.Wake)
			}
		case platform.WakeEvent:
			switch phase {
			case 1:
				phase = 2
				cpu0, iter0, wall0 = cpuTime(t), b.iterations.Load(), time.Now()
				time.AfterFunc(idle, b.Wake)
			case 2:
				cpuIdle, iterIdle = cpuTime(t)-cpu0, b.iterations.Load()-iter0
				t.Logf("idle for %v: process CPU %v (%.2f%%), %d loop iterations",
					time.Since(wall0).Round(time.Millisecond), cpuIdle, 100*cpuIdle.Seconds()/idle.Seconds(), iterIdle)
				b.Quit()
			}
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = nw
	if phase != 2 {
		t.Fatalf("did not reach the idle phase")
	}
	// Generous bound: pointer motion over the window or a reconfigure by
	// the window manager may cost a little. A ticking loop at 60 Hz costs
	// 120 iterations in 2 s.
	if cpuIdle > 40*time.Millisecond {
		t.Errorf("idle CPU %v over %v, want ~0", cpuIdle, idle)
	}
	if iterIdle > 30 {
		t.Errorf("%d loop iterations while idle, want only those caused by input", iterIdle)
	}
}

// TestLiveConnectionLost runs the backend over a socket the test controls
// and closes it under the running loop: Run must return an error.
func TestLiveConnectionLost(t *testing.T) {
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run against the real X server")
	}
	m := regexp.MustCompile(`^:(\d+)(\.\d+)?$`).FindStringSubmatch(os.Getenv("DISPLAY"))
	if m == nil {
		t.Skip("DISPLAY is not a local display (:N)")
	}
	sock, err := net.Dial("unix", "/tmp/.X11-unix/X"+m[1])
	if err != nil {
		t.Skipf("cannot dial the X socket: %v", err)
	}
	silenceXgbNoise()
	conn, err := xgb.NewConnNet(sock)
	if err != nil {
		t.Skipf("X handshake over a raw socket failed (the server may need an Xauthority cookie): %v", err)
	}
	b, err := newBackend(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.NewWindow(platform.WindowOptions{Title: "tela x11 lost"}); err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(10*time.Second, b.Quit)
	err = b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); ok {
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			_ = w.Present(fb, nil)
			time.AfterFunc(100*time.Millisecond, func() { sock.Close() })
		}
	}))
	if !errors.Is(err, errConnLost) {
		t.Fatalf("Run after the connection was closed = %v, want %v", err, errConnLost)
	}
}
