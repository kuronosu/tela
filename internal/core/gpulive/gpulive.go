//go:build telagpu && linux

// Package gpulive is the rig of the live tests of windows the GPU draws
// (ADR-281, ADR-284): an app in a real window of the X11 or the Wayland
// backend, drawn by the GPU core opens for it, as Run does, and presented
// without a copy where the display server takes its buffers. It needs
// TELA_LIVE=1 and the build tag telagpu, and a test binary built where the
// compositor sees it:
//
//	go test -c -tags telagpu -o bin/core.test ./internal/core
//	TELA_LIVE=1 TELA_BACKEND=x11 DISPLAY=:0 bin/core.test -test.run TestLiveGPU -test.v
//
// The windows open silently on a workspace nobody looks at (Hyprland: a
// window rule for their class, added before they open), take no focus and
// get no input: the steps of a test change state from the UI thread.
//
// On X11 every step is checked with pixels: GetImage reads the server's
// copy of the window, also on a workspace no monitor shows (Xwayland), and
// it must be the frame the tree rasterized last, within the tolerance of
// ADR-282 of the CPU rasterizer's rendering of it. On Wayland nothing can
// read the window back, so the run checks what the protocol tells: no
// error, and the buffers released.
package gpulive

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/platformtest"
	"github.com/kuronosu/tela/internal/platform/wayland"
	"github.com/kuronosu/tela/internal/platform/x11"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// Config describes a run.
type Config struct {
	// Class is the app ID of the window, its class for the window rule.
	Class string
	// Options are the Host's: the root, the window's size, the theme.
	Options core.Options
	// Steps run one after the other on the UI thread, each once the
	// window came to rest after the one before and, on X11, shows its
	// frame. The first runs after the first frame.
	Steps []Step
}

// Step is one step of a run.
type Step struct {
	Name string
	Do   func(h *core.Host)
	// Unchecked: the window shows nothing to compare after the step (it
	// is hidden).
	Unchecked bool
}

// Result is what a run did.
type Result struct {
	// Backend is the backend's name, Device the GPU's.
	Backend, Device string
	// Window is how the frames reached the main window, at the end.
	Window core.GPUWindow
	// Checked counts the steps whose pixels were read back and compared,
	// and Worst is the largest difference from the CPU rasterizer's.
	Checked int
	Worst   gfxtest.Diff
	// Paced counts the frames the rig gave the window from its own timer,
	// because the compositor sent no frame callbacks (a Wayland window on
	// a hidden workspace).
	Paced int
	// Log is the Host's error log.
	Log string
}

// settle is how long a window must be at rest before a step is over, and
// limit how long a step may take: Xwayland answers a present for a window
// no monitor shows after a second or two.
const (
	settle = 300 * time.Millisecond
	limit  = 20 * time.Second
)

// Run plays cfg on the backend TELA_BACKEND names (x11 or wayland; the
// one of the session otherwise), and fails t for what goes wrong. It skips
// without TELA_LIVE=1, without the display, and when the windows cannot be
// kept off the workspace in use.
func Run(t *testing.T, cfg Config) Result {
	t.Helper()
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run in real windows")
	}
	name := os.Getenv("TELA_BACKEND")
	if name == "" {
		name = "wayland"
		if os.Getenv("WAYLAND_DISPLAY") == "" {
			name = "x11"
		}
	}
	platformtest.LockLive(t)
	place(t, cfg.Class)
	var b platform.Backend
	var err error
	switch name {
	case "x11":
		if os.Getenv("DISPLAY") == "" {
			t.Skip("DISPLAY is not set")
		}
		b, err = x11.New()
	case "wayland":
		if os.Getenv("WAYLAND_DISPLAY") == "" {
			t.Skip("WAYLAND_DISPLAY is not set")
		}
		b, err = wayland.New()
	default:
		t.Skipf("TELA_BACKEND=%s is neither x11 nor wayland", name)
	}
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	log := &bytes.Buffer{}
	opts := cfg.Options
	opts.AppID, opts.ErrorLog = cfg.Class, log
	h, err := core.NewHost(b, opts)
	if err != nil {
		t.Fatal(err)
	}
	h.OpenGPU()
	res := Result{Backend: name}
	if g := h.GPU(); g != nil {
		res.Device = g.Name()
	}
	stop := h.Tree().ReceivePosts()
	d := &driver{t: t, b: b, h: h, cfg: cfg, res: &res, since: time.Now(), started: time.Now()}
	h.AfterFrame = func() { d.frames++; d.since = time.Now() }
	// Nothing wakes a loop at rest: the driver looks at the window every
	// tick.
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				b.Wake()
			}
		}
	}()
	err = b.Run(d)
	close(done)
	stop()
	h.End()
	res.Log = log.String()
	if err != nil {
		t.Errorf("%s: the loop ended with %v", name, err)
	}
	return res
}

// place makes the windows of a class open silently on the hidden
// workspace on Hyprland, and skips the test when it cannot.
func place(t *testing.T, class string) {
	t.Helper()
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return
	}
	ws := os.Getenv("TELA_LIVE_WORKSPACE")
	if ws == "" {
		ws = "7"
	}
	rule := `tela_live_gpu = hl.window_rule({ name = "tela-live-gpu", match = { class = "` + class + `" }, workspace = "` + ws + ` silent" })`
	if out, err := exec.Command("hyprctl", "eval", rule).CombinedOutput(); err != nil || strings.Contains(strings.ToLower(string(out)), "error") {
		t.Skipf("hyprctl eval (window rule): %v %s; not opening a window that would take the focus", err, out)
	}
	t.Cleanup(func() {
		exec.Command("hyprctl", "eval", `if tela_live_gpu then tela_live_gpu:set_enabled(false) end`).Run()
	})
}

// driver handles the backend's events for the Host and plays the steps.
type driver struct {
	t   *testing.T
	b   platform.Backend
	h   *core.Host
	cfg Config
	res *Result
	// step is the step whose frame is awaited (0: the first frame),
	// started when it began, since the time of the last frame, and frames
	// the frames so far.
	step    int
	started time.Time
	since   time.Time
	frames  int
	done    bool
	// why is the last reason the window did not show the step's frame.
	why string
	// paced is the time of the last frame the rig gave the window itself.
	paced time.Time
	// want is the CPU rasterizer's rendering of the frame numbered wantAt.
	want   *image.RGBA
	wantAt int
}

func (d *driver) HandleEvent(w platform.Window, e platform.Event) {
	d.h.HandleEvent(w, e)
	if _, ok := e.(platform.WakeEvent); ok && !d.done {
		d.pace()
	}
	if _, ok := e.(platform.CloseRequestedEvent); ok && !d.done {
		d.t.Errorf("the window was closed during step %d", d.step)
		d.finish()
	}
	if !d.done {
		d.advance()
	}
}

// pace gives the window a frame from the rig's timer when the compositor
// sends none although the tree has work: a Wayland window on a workspace
// no monitor shows is suspended and gets no frame callbacks, so an app
// there draws nothing, by design. The frames paced this way are what the
// run can still check of the protocol: buffers taken, presented and
// released without an error.
func (d *driver) pace() {
	if d.res.Backend != "wayland" || time.Since(d.since) < 100*time.Millisecond || time.Since(d.paced) < 50*time.Millisecond {
		return
	}
	tr := d.h.Tree()
	if tr.Dirty() || tr.Animating() || d.h.NeedsFrame() || d.frames == 0 {
		d.paced = time.Now()
		d.res.Paced++
		d.h.HandleEvent(d.h.Window(), platform.FrameEvent{Time: d.b.Now()})
	}
}

func (d *driver) finish() {
	d.done = true
	// While the window is open: the loop closes it when it ends.
	d.res.Window = d.h.GPUWindow(d.h.Window())
	d.b.Quit()
}

// name returns the name of the step whose frame is awaited.
func (d *driver) name() string {
	if d.step == 0 {
		return "the first frame"
	}
	return d.cfg.Steps[d.step-1].Name
}

// advance goes to the next step once the window rests and shows the
// frame of the current one.
func (d *driver) advance() {
	t := d.t
	tr := d.h.Tree()
	if time.Since(d.started) > limit {
		t.Errorf("%s: no frame shown after %v (%d frames ran; %s)", d.name(), limit, d.frames, d.why)
		d.finish()
		return
	}
	unchecked := d.step > 0 && d.cfg.Steps[d.step-1].Unchecked
	switch {
	case unchecked:
		// A hidden window gets no frames, so its animations do not end:
		// the step is over after a while.
		if time.Since(d.started) < 2*settle {
			return
		}
	case d.frames == 0 || tr.Dirty() || tr.Animating() || d.h.NeedsFrame() || time.Since(d.since) < settle:
		d.why = "the window is not at rest"
		return
	}
	if !unchecked {
		if ok, why := d.check(); !ok {
			d.why = why
			return
		}
	}
	t.Logf("%s: shown after %v", d.name(), time.Since(d.started).Round(time.Millisecond))
	if d.step == len(d.cfg.Steps) {
		d.finish()
		return
	}
	d.step++
	d.started, d.since, d.why = time.Now(), time.Now(), ""
	d.cfg.Steps[d.step-1].Do(d.h)
}

// check compares what the window shows with the frame the tree
// rasterized last, where the backend can read it back. It reports false,
// and why, while the window does not show that frame (yet).
func (d *driver) check() (bool, string) {
	xw, ok := d.h.Window().(*x11.Window)
	if !ok {
		return true, ""
	}
	got, err := xw.Capture()
	if err != nil {
		return false, err.Error()
	}
	tr := d.h.Tree()
	if d.want == nil || d.wantAt != d.frames {
		// The CPU's rendering of the frame, once per frame: the window is
		// read until it shows it.
		d.want, d.wantAt = tr.Rasterize(tr.LastFrame(), tr.Scale()), d.frames
		force(d.want)
	}
	want := d.want
	if got.Rect != want.Rect {
		return false, fmt.Sprintf("the window is %v, the frame %v", got.Rect.Size(), want.Rect.Size())
	}
	m := gfxtest.Compare(got, want)
	if !m.WithinGPU() {
		if dir := os.Getenv("TELA_LIVE_DUMP"); dir != "" {
			golden.WritePNG(d.t, filepath.Join(dir, "live_window.png"), got)
			golden.WritePNG(d.t, filepath.Join(dir, "live_cpu.png"), want)
		}
		return false, fmt.Sprintf("the window differs from the CPU rasterizer's frame by up to %d levels at %v, %.3f%% of the pixels by more than %d", m.Max, m.Worst, 100*m.Over2, gfxtest.GPULevels)
	}
	d.res.Checked++
	if m.Max > d.res.Worst.Max {
		d.res.Worst = m
	}
	return true, ""
}

// force makes img opaque, as a window's pixels read back are.
func force(img *image.RGBA) {
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
}
