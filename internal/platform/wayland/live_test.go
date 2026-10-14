//go:build linux

package wayland

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// Live tests talk to the real compositor and open windows on the user's
// screen. They only run with TELA_LIVE=1 and a WAYLAND_DISPLAY, and keep
// windows open for a few seconds at most.

func skipUnlessLive(t *testing.T) {
	t.Helper()
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run against the real compositor (opens windows)")
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("WAYLAND_DISPLAY is not set")
	}
}

func liveBackend(t *testing.T) *Backend {
	t.Helper()
	skipUnlessLive(t)
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

func TestLiveConformance(t *testing.T) {
	skipUnlessLive(t)
	platformtest.Run(t, platformtest.Config{
		// The Clipboard subtest replaces the session's clipboard with its
		// test text.
		New: func(t *testing.T) platform.Backend { return liveBackend(t) },
	})
}

// hyprClient is the part of `hyprctl clients -j` the tests read.
type hyprClient struct {
	At    [2]int `json:"at"`
	Size  [2]int `json:"size"`
	Pid   int    `json:"pid"`
	Title string `json:"title"`
}

// findWindow returns our window's logical geometry from Hyprland (read
// only; the tests never change Hyprland's state).
func findWindow(title string) (hyprClient, error) {
	out, err := exec.Command("hyprctl", "clients", "-j").Output()
	if err != nil {
		return hyprClient{}, fmt.Errorf("hyprctl clients: %w", err)
	}
	var clients []hyprClient
	if err := json.Unmarshal(out, &clients); err != nil {
		return hyprClient{}, err
	}
	for _, c := range clients {
		if c.Pid == os.Getpid() && c.Title == title {
			return c, nil
		}
	}
	return hyprClient{}, fmt.Errorf("window %q of pid %d not found in hyprctl clients", title, os.Getpid())
}

// screenshot captures a logical rectangle of the screen with grim.
func screenshot(c hyprClient) (image.Image, error) {
	geom := fmt.Sprintf("%d,%d %dx%d", c.At[0], c.At[1], c.Size[0], c.Size[1])
	out, err := exec.Command("grim", "-g", geom, "-t", "png", "-").Output()
	if err != nil {
		return nil, fmt.Errorf("grim -g %q: %w", geom, err)
	}
	return png.Decode(bytes.NewReader(out))
}

// Test pattern, in fractions of the window: four quadrants, and two
// rectangles presented later with partial damage.
var (
	quadColors = [4]color.RGBA{
		{230, 30, 30, 255},  // top left: red
		{30, 200, 60, 255},  // top right: green
		{40, 60, 220, 255},  // bottom left: blue
		{240, 220, 40, 255}, // bottom right: yellow
	}
	patch1      = [4]float64{0.40, 0.40, 0.60, 0.60} // magenta, center
	patch1Color = color.RGBA{220, 40, 220, 255}
	patch2      = [4]float64{0.05, 0.05, 0.15, 0.15} // cyan, top left
	patch2Color = color.RGBA{40, 220, 230, 255}
)

func fracRect(f [4]float64, size image.Point) image.Rectangle {
	return image.Rect(int(f[0]*float64(size.X)), int(f[1]*float64(size.Y)), int(f[2]*float64(size.X)), int(f[3]*float64(size.Y)))
}

func fill(fb *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(fb.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := fb.PixOffset(x, y)
			fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
}

func quadrants(fb *image.RGBA) {
	s := fb.Rect.Max
	h := image.Point{X: s.X / 2, Y: s.Y / 2}
	fill(fb, image.Rect(0, 0, h.X, h.Y), quadColors[0])
	fill(fb, image.Rect(h.X, 0, s.X, h.Y), quadColors[1])
	fill(fb, image.Rect(0, h.Y, h.X, s.Y), quadColors[2])
	fill(fb, image.Rect(h.X, h.Y, s.X, s.Y), quadColors[3])
}

func TestLivePixels(t *testing.T) {
	for _, format := range []string{"xbgr8888", "xrgb8888"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("TELA_WAYLAND_SHM_FORMAT", format)
			livePixels(t, format)
		})
	}
}

func livePixels(t *testing.T, format string) {
	b := liveBackend(t)
	if got := b.format.String(); got != format && format == "xrgb8888" {
		t.Fatalf("format = %s, want %s", got, format)
	}
	t.Logf("shm format %v", b.format)
	title := "tela live pixels " + format
	win, err := b.NewWindow(platform.WindowOptions{Title: title, AppID: "dev.telaui.livetest", Size: platform.Size{W: 400, H: 300}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)

	type shot struct {
		img image.Image
		c   hyprClient
		err error
	}
	shots := make(chan shot, 1)
	var (
		fb       *image.RGBA
		phase    int // 0 full pattern, 1 waiting to settle, 2 patch1, 3 patch2, 4 capturing
		settle   *time.Timer
		captured shot
		metrics  platform.Metrics
		bufSize  image.Point
	)
	time.AfterFunc(15*time.Second, b.Quit) // safety net
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			t.Logf("configure %v", e.Metrics)
			phase = 0 // the compositor may reconfigure after mapping: start over
		case platform.FrameEvent:
			switch phase {
			case 0:
				fb = image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
				quadrants(fb)
				if err := w.Present(fb, nil); err != nil {
					t.Fatalf("Present: %v", err)
				}
				phase = 1
				// Let Hyprland finish its open animation and any
				// reconfigure before the partial updates.
				if settle != nil {
					settle.Stop()
				}
				settle = time.AfterFunc(800*time.Millisecond, b.Wake)
			case 2:
				// Partial present: everything outside the damage is
				// garbage and must not reach the screen.
				fill(fb, fb.Rect, color.RGBA{0, 0, 0, 255})
				r := fracRect(patch1, fb.Rect.Max)
				fill(fb, r, patch1Color)
				if err := w.Present(fb, []image.Rectangle{r}); err != nil {
					t.Fatalf("Present: %v", err)
				}
				phase = 3
				w.RequestFrame()
			case 3:
				fill(fb, fb.Rect, color.RGBA{0, 0, 0, 255})
				r := fracRect(patch2, fb.Rect.Max)
				fill(fb, r, patch2Color)
				if err := w.Present(fb, []image.Rectangle{r}); err != nil {
					t.Fatalf("Present: %v", err)
				}
				phase = 4
				metrics, bufSize = w.Metrics(), w.chain.size
				go func() {
					time.Sleep(400 * time.Millisecond)
					c, err := findWindow(title)
					var img image.Image
					if err == nil {
						img, err = screenshot(c)
					}
					shots <- shot{img, c, err}
					b.Wake()
				}()
			}
		case platform.WakeEvent:
			switch phase {
			case 1:
				phase = 2
				w.RequestFrame()
			case 4:
				select {
				case captured = <-shots:
					b.Quit()
				default:
				}
			}
		case platform.CloseRequestedEvent:
			t.Errorf("window closed by the user")
			b.Quit()
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if captured.err != nil {
		t.Fatal(captured.err)
	}
	if captured.img == nil {
		t.Fatal("no screenshot was taken (timed out)")
	}

	// Buffer size: logical × scale, integer scale.
	scale := float64(b.maxOutputScale())
	t.Logf("metrics %v, buffer %v, output scale %v, hyprland geometry %v at %v, screenshot %v",
		metrics, bufSize, scale, captured.c.Size, captured.c.At, captured.img.Bounds().Size())
	if metrics.Scale != scale {
		t.Errorf("window scale %v, want the output scale %v", metrics.Scale, scale)
	}
	wantBuf := image.Point{X: int(metrics.Size.W * float32(metrics.Scale)), Y: int(metrics.Size.H * float32(metrics.Scale))}
	if bufSize != wantBuf || bufSize != metrics.Physical() {
		t.Errorf("buffer %v, want %v (logical %v × scale %v)", bufSize, wantBuf, metrics.Size, metrics.Scale)
	}
	if captured.c.Size != [2]int{int(metrics.Size.W), int(metrics.Size.H)} {
		t.Errorf("Hyprland reports %v, the window has %v", captured.c.Size, metrics.Size)
	}

	// Pixels: sample well inside each region (borders and rounding sit at
	// the edges).
	img := captured.img
	if dir := os.Getenv("TELA_LIVE_SHOTS"); dir != "" {
		var buf bytes.Buffer
		png.Encode(&buf, img)
		path := dir + "/wayland-live-" + format + ".png"
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Errorf("saving the screenshot: %v", err)
		} else {
			t.Logf("screenshot saved to %s", path)
		}
	}
	bounds := img.Bounds()
	if bounds.Dx() != bufSize.X || bounds.Dy() != bufSize.Y {
		t.Errorf("screenshot is %v, buffer %v: the compositor scaled the buffer", bounds.Size(), bufSize)
	}
	sample := func(fx, fy float64) color.RGBA {
		x := bounds.Min.X + int(fx*float64(bounds.Dx()))
		y := bounds.Min.Y + int(fy*float64(bounds.Dy()))
		r, g, bl, _ := img.At(x, y).RGBA()
		return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}
	}
	checks := []struct {
		name   string
		fx, fy float64
		want   color.RGBA
	}{
		{"top-left quadrant", 0.25, 0.30, quadColors[0]},
		{"top-right quadrant", 0.75, 0.25, quadColors[1]},
		{"bottom-left quadrant", 0.25, 0.75, quadColors[2]},
		{"bottom-right quadrant", 0.75, 0.80, quadColors[3]},
		{"patch 1 (partial present)", 0.50, 0.50, patch1Color},
		{"patch 2 (partial present into a reused buffer)", 0.10, 0.10, patch2Color},
		{"between the patches (not damaged)", 0.30, 0.42, quadColors[0]},
	}
	for _, c := range checks {
		got := sample(c.fx, c.fy)
		if !near(got, c.want, 24) {
			t.Errorf("%s at (%.2f, %.2f): screen %v, want %v", c.name, c.fx, c.fy, got, c.want)
		} else {
			t.Logf("%s: screen %v, want %v", c.name, got, c.want)
		}
	}
}

func near(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) bool { return abs(int(x)-int(y)) <= tol }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// cpuTime returns the process's user + system CPU time.
func cpuTime(t *testing.T) time.Duration {
	var ru unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &ru); err != nil {
		t.Fatal(err)
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

func TestLiveIdleCPU(t *testing.T) {
	b := liveBackend(t)
	const idle = 2 * time.Second
	win, err := b.NewWindow(platform.WindowOptions{Title: "tela live idle", AppID: "dev.telaui.livetest", Size: platform.Size{W: 320, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	var (
		start, end      time.Duration
		measuring, done bool
		eventsWhileIdle int
		framesWhileIdle int
	)
	time.AfterFunc(10*time.Second, b.Quit)
	time.AfterFunc(700*time.Millisecond, b.Wake) // start measuring once mapped and settled
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		if measuring {
			eventsWhileIdle++
		}
		switch e.(type) {
		case platform.FrameEvent:
			if measuring {
				framesWhileIdle++
			}
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			quadrants(fb)
			if err := w.Present(fb, nil); err != nil {
				t.Fatalf("Present: %v", err)
			}
		case platform.WakeEvent:
			switch {
			case !measuring && !done:
				measuring = true
				eventsWhileIdle = 0
				start = cpuTime(t)
				time.AfterFunc(idle, b.Wake)
			case measuring:
				end = cpuTime(t)
				measuring, done = false, true
				b.Quit()
			}
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !done {
		t.Fatal("the idle measurement did not complete")
	}
	used := end - start
	t.Logf("CPU time over %v of idle: %v (%.3f%%); %d events, %d frames delivered meanwhile",
		idle, used, 100*used.Seconds()/idle.Seconds(), eventsWhileIdle-1, framesWhileIdle)
	// Go's runtime (sysmon) wakes up now and then; anything near a busy
	// loop would cost the full 2 s.
	if used > 20*time.Millisecond {
		t.Errorf("idle CPU time %v over %v; the loop is not blocking", used, idle)
	}
}
