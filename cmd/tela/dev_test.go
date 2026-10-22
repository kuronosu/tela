package main

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// The supervisor's tests: a real remote host on the headless backend, the
// supervisor's loop, and fake children (dev_fake_test.go) started from a
// fake builder.

const devWait = 15 * time.Second

// syncBuffer is a concurrency-safe log.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// recordView records the problems shown and cleared, besides printing them.
type recordView struct {
	term terminalView
	mu   sync.Mutex
	log  []string // "show <kind>: <summary>" or "clear"
	last problem
}

func (v *recordView) show(p problem) {
	v.term.show(p)
	v.mu.Lock()
	defer v.mu.Unlock()
	v.log = append(v.log, fmt.Sprintf("show %d: %s", p.kind, p.summary))
	v.last = p
}

func (v *recordView) lastProblem() problem {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.last
}

func (v *recordView) clear() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.log = append(v.log, "clear")
}

func (v *recordView) entries() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.log...)
}

type devHarness struct {
	t    *testing.T
	hb   *headless.Backend
	s    *supervisor
	fb   *fakeBuilder
	out  *syncBuffer
	view *recordView
	done chan int
	code int
	quit bool

	mu         sync.Mutex
	fakeErrors []string
}

func newDevHarness(t *testing.T, opts devOptions, configure func(*supervisorConfig)) *devHarness {
	t.Helper()
	if testing.Short() {
		t.Skip("starts processes")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	if opts.Pkg == "" {
		opts.Pkg = "./fake"
	}
	h := &devHarness{
		t:    t,
		hb:   headless.New(headless.Options{RealTime: true}),
		fb:   &fakeBuilder{exe: exe, modes: map[int]string{}, fail: map[int]string{}, block: map[int]chan struct{}{}},
		out:  &syncBuffer{},
		done: make(chan int, 1),
	}
	h.view = &recordView{}
	cfg := supervisorConfig{
		opts:              opts,
		builder:           h.fb,
		log:               h.out,
		stdout:            h.out,
		stderr:            h.out,
		env:               append(os.Environ(), "FAKE_EXPECT_DEV=1"),
		view:              h.view,
		firstFrameTimeout: 10 * time.Second,
		stopTimeout:       2 * time.Second,
	}
	if configure != nil {
		configure(&cfg)
	}
	h.s = newSupervisor(cfg)
	h.view.term = terminalView{h.s.log}
	host, err := remote.NewHost(h.hb, h.s.hostOptions())
	if err != nil {
		t.Fatal(err)
	}
	host.Handle("fake.error", func(c *remote.Child, p []byte) ([]byte, error) {
		h.mu.Lock()
		h.fakeErrors = append(h.fakeErrors, fmt.Sprintf("build %s: %s", c.Tag(), p))
		h.mu.Unlock()
		return nil, nil
	})
	h.s.attach(host, h.hb.Quit)
	go func() {
		err := host.Run()
		h.s.push(evHostDone{err: err})
	}()
	go func() { h.done <- h.s.run() }()
	t.Cleanup(func() {
		if !h.quit {
			h.s.push(evKey{'q'})
			h.wait()
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, e := range h.fakeErrors {
			t.Errorf("fake child: %s", e)
		}
		if t.Failed() || testing.Verbose() {
			t.Logf("output:\n%s", h.out.String())
		}
	})
	return h
}

// wait waits for the supervisor to return and checks it left no process.
func (h *devHarness) wait() int {
	h.t.Helper()
	select {
	case h.code = <-h.done:
	case <-time.After(devWait):
		h.t.Fatalf("tela dev did not quit; output:\n%s", h.out.String())
	}
	h.quit = true
	for gen, p := range h.s.procs {
		h.t.Errorf("build %d still running after quit", gen)
		p.cmd.Process.Kill()
	}
	return h.code
}

func (h *devHarness) window() *headless.Window {
	h.t.Helper()
	deadline := time.Now().Add(devWait)
	for {
		if ws := h.hb.Windows(); len(ws) > 0 {
			return ws[0]
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("no window; output:\n%s", h.out.String())
		}
		time.Sleep(time.Millisecond)
	}
}

// color returns the color the window shows, or false.
func (h *devHarness) color() (color.RGBA, bool) {
	ws := h.hb.Windows()
	if len(ws) == 0 {
		return color.RGBA{}, false
	}
	f := ws[0].Frame()
	if f == nil {
		return color.RGBA{}, false
	}
	return f.RGBAAt(1, 1), true
}

// expect waits until the window shows build gen with count and the clicks
// of this process.
func (h *devHarness) expect(what string, gen, count, clicks int) {
	h.t.Helper()
	want := color.RGBA{uint8(gen), uint8(count), uint8(clicks), 255}
	deadline := time.Now().Add(devWait)
	for {
		got, ok := h.color()
		if ok && got == want {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("%s: the window shows %v (ok %v), want build %d count %d clicks %d; output:\n%s", what, got, ok, gen, count, clicks, h.out.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// still checks that the window keeps showing build gen with count for d.
func (h *devHarness) still(what string, gen, count, clicks int, d time.Duration) {
	h.t.Helper()
	want := color.RGBA{uint8(gen), uint8(count), uint8(clicks), 255}
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if got, ok := h.color(); !ok || got != want {
			h.t.Fatalf("%s: the window shows %v, want build %d count %d clicks %d", what, got, gen, count, clicks)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (h *devHarness) click() {
	w := h.window()
	w.Inject(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: 10, Y: 10}, Button: platform.ButtonPrimary, Buttons: 1})
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 10, Y: 10}, Button: platform.ButtonPrimary})
}

func (h *devHarness) press(key string, code platform.KeyCode) {
	w := h.window()
	w.SetFocused(true)
	w.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: code, Key: platform.Key(key), Text: key})
	w.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: code, Key: platform.Key(key)})
}

// logged waits until the output contains s n times.
func (h *devHarness) logged(s string, n int) {
	h.t.Helper()
	deadline := time.Now().Add(devWait)
	for strings.Count(h.out.String(), s) < n {
		if time.Now().After(deadline) {
			h.t.Fatalf("waiting for %q (%d times); output:\n%s", s, n, h.out.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (h *devHarness) ui(fn func()) {
	h.t.Helper()
	ch := make(chan struct{})
	h.s.host.Post(func() { fn(); close(ch) })
	select {
	case <-ch:
	case <-time.After(devWait):
		h.t.Fatal("the host's UI thread did not run a posted function")
	}
}

func (h *devHarness) frozen() bool {
	var f bool
	h.ui(func() { f = h.s.host.Frozen() })
	return f
}

func TestDevFirstStartAndSwapWithState(t *testing.T) {
	h := newDevHarness(t, devOptions{Verbose: true}, nil)
	h.expect("first start", 1, 0, 0)
	h.logged("started in", 1)
	if w := h.window(); w.Title() != "fake 1" {
		t.Errorf("title %q", w.Title())
	}
	h.click()
	h.click()
	h.expect("after two clicks", 1, 2, 2)

	h.s.push(evKey{'r'})
	h.expect("reload keeps the counter", 2, 2, 0)
	h.logged("reloaded in", 1)
	h.logged("state: 1 kept, 2 reset", 1)
	h.logged("build 1 stopped (exited with status 0)", 1)
	if w := h.window(); w.Title() != "fake 2" {
		t.Errorf("title after the swap %q", w.Title())
	}
	// The new child gets the input.
	h.click()
	h.expect("click after the swap", 2, 3, 1)

	// R: fresh state.
	h.s.push(evKey{'R'})
	h.expect("reload without state", 3, 0, 0)
	h.logged("state reset", 1)
	if n := len(h.hb.Windows()); n != 1 {
		t.Errorf("%d real windows, want 1 (the window never closes)", n)
	}
	if got := h.view.entries(); len(got) != 0 {
		t.Errorf("problems shown: %v", got)
	}
}

func TestDevInputFrozenAndReplayed(t *testing.T) {
	h := newDevHarness(t, devOptions{}, nil)
	h.expect("first start", 1, 0, 0)
	h.click()
	h.expect("one click", 1, 1, 1)

	h.fb.set(2, "slow")
	h.s.push(evKey{'r'})
	deadline := time.Now().Add(devWait)
	for !h.frozen() {
		if time.Now().After(deadline) {
			t.Fatal("input never frozen")
		}
		time.Sleep(time.Millisecond)
	}
	// While frozen, the old child gets nothing: the click waits for the
	// new one.
	h.click()
	h.still("the old build while frozen", 1, 1, 1, 100*time.Millisecond)
	// The new build got the state (1) before the click, then the click.
	h.expect("the click replayed to the new build", 2, 2, 1)
	if h.frozen() {
		t.Error("still frozen after the swap")
	}
}

func TestDevNewBuildCrashesBeforeFirstFrame(t *testing.T) {
	for _, mode := range []string{"crash", "panic", "noframe"} {
		t.Run(mode, func(t *testing.T) {
			h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) {
				c.firstFrameTimeout = 700 * time.Millisecond
			})
			h.expect("first start", 1, 0, 0)
			h.click()
			h.expect("one click", 1, 1, 1)
			h.fb.set(2, mode)
			h.s.push(evKey{'r'})
			switch mode {
			case "crash":
				h.logged("the new build exited with status 3 before its first frame; the previous build stays", 1)
			case "panic":
				h.logged("the new build panicked before its first frame: startup boom; the previous build stays", 1)
			case "noframe":
				h.logged("the new build showed no frame in 700ms; the previous build stays", 1)
			}
			if h.frozen() {
				t.Error("still frozen after a failed swap")
			}
			// The old build stays and gets input.
			h.click()
			h.expect("the old build after a failed swap", 1, 2, 2)
			// The next good build replaces it, with its state, and clears
			// the problem.
			h.s.push(evKey{'r'})
			h.expect("the next good build", 3, 2, 0)
			kind := problemStart
			if mode == "panic" {
				kind = problemPanic
			}
			got := h.view.entries()
			if len(got) != 2 || !strings.HasPrefix(got[0], fmt.Sprintf("show %d:", kind)) || got[1] != "clear" {
				t.Errorf("problems %v", got)
			}
		})
	}
}

func TestDevRuntimeCrashThenFreshStart(t *testing.T) {
	h := newDevHarness(t, devOptions{}, nil)
	h.expect("first start", 1, 0, 0)
	h.click()
	h.expect("one click", 1, 1, 1)
	h.press("p", platform.CodeKeyP)
	h.logged("the app panicked: boom; save a change or press r to start it again", 1)
	// The window stays, with the last frame.
	h.still("the last frame after the crash", 1, 1, 1, 100*time.Millisecond)
	if last := h.view.lastProblem(); last.kind != problemPanic || !strings.Contains(last.text, "goroutine") {
		t.Errorf("panic problem %+v, want the payload with its stack", last)
	}
	// The next build starts fresh: no stale state.
	h.s.push(evKey{'r'})
	h.expect("fresh start", 2, 0, 0)
	h.logged("restarted in", 1)
	got := h.view.entries()
	if len(got) != 2 || got[1] != "clear" {
		t.Errorf("problems %v", got)
	}
}

// The child reports the state it kept a few frames after its first one,
// usually after the promotion: the reload line waits for it (a moment at
// most) instead of printing without the counts.
func TestDevStateReportAfterPromotion(t *testing.T) {
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) { c.reportWait = 5 * time.Second })
	h.expect("first start", 1, 0, 0)
	h.click()
	h.expect("one click", 1, 1, 1)
	h.fb.set(2, "late")
	h.s.push(evKey{'r'})
	h.expect("reload", 2, 1, 0)
	h.logged("state: 1 kept, 2 reset", 1)
	out := h.out.String()
	if n := strings.Count(out, "reloaded in"); n != 1 || strings.Contains(out, "(state kept)") {
		t.Errorf("want one reload line with the counts:\n%s", out)
	}
}

func TestDevStateReportMissing(t *testing.T) {
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) { c.reportWait = 100 * time.Millisecond })
	h.expect("first start", 1, 0, 0)
	h.click()
	h.expect("one click", 1, 1, 1)
	h.fb.set(2, "noreport")
	h.s.push(evKey{'r'})
	h.expect("reload", 2, 1, 0)
	h.logged("(state kept)", 1)
}

// The child runtime cannot recover a panic of a goroutine other than the
// UI thread's: the runtime prints it and exits with status 2, and the
// supervisor shows it like a reported one, from the end of standard error.
func TestDevGoroutinePanic(t *testing.T) {
	h := newDevHarness(t, devOptions{}, nil)
	h.expect("first start", 1, 0, 0)
	h.press("g", platform.CodeKeyG)
	h.logged("the app panicked: goroutine boom; save a change or press r to start it again", 1)
	if last := h.view.lastProblem(); last.kind != problemPanic || !strings.HasPrefix(last.text, "panic: goroutine boom") || !strings.Contains(last.text, "goroutine") {
		t.Errorf("problem %+v, want the runtime's panic text", last)
	}
	h.still("the last frame after the crash", 1, 0, 0, 100*time.Millisecond)
}

func TestDevNoState(t *testing.T) {
	h := newDevHarness(t, devOptions{NoState: true}, nil)
	h.expect("first start", 1, 0, 0)
	h.click()
	h.expect("one click", 1, 1, 1)
	h.s.push(evKey{'r'})
	h.expect("reload with --no-state", 2, 0, 0)
	if out := h.out.String(); strings.Contains(out, "state:") || strings.Contains(out, "kept") || strings.Contains(out, "state reset") {
		t.Errorf("--no-state printed something about state:\n%s", out)
	}
}

func TestDevStateNotSaved(t *testing.T) {
	h := newDevHarness(t, devOptions{}, nil)
	h.fb.set(1, "nosave")
	h.expect("first start", 1, 0, 0)
	h.click()
	h.expect("one click", 1, 1, 1)
	h.s.push(evKey{'r'})
	h.expect("reload without a save handler", 2, 0, 0)
	h.logged("state not kept (the app does not save state", 1)
}

func TestDevBuildFailure(t *testing.T) {
	h := newDevHarness(t, devOptions{}, nil)
	h.expect("first start", 1, 0, 0)
	h.fb.mu.Lock()
	h.fb.fail[2] = "./main.go:3:1: syntax error: unexpected }"
	h.fb.mu.Unlock()
	h.s.push(evKey{'r'})
	h.logged("build failed:\n./main.go:3:1: syntax error", 1)
	// The app keeps running.
	h.click()
	h.expect("the app after a failed build", 1, 1, 1)
	h.s.push(evKey{'r'})
	h.expect("the next good build", 3, 1, 0)
	got := h.view.entries()
	if len(got) != 2 || got[0] != fmt.Sprintf("show %d: build failed", problemBuild) || got[1] != "clear" {
		t.Errorf("problems %v", got)
	}
}

func TestDevNewerSaveCancelsBuild(t *testing.T) {
	h := newDevHarness(t, devOptions{}, nil)
	h.expect("first start", 1, 0, 0)
	block := make(chan struct{})
	h.fb.mu.Lock()
	h.fb.block[2] = block
	h.fb.mu.Unlock()
	h.s.push(evKey{'r'})
	deadline := time.Now().Add(devWait)
	for {
		h.fb.mu.Lock()
		n := h.fb.builds
		h.fb.mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("build 2 never started")
		}
		time.Sleep(time.Millisecond)
	}
	h.s.push(evBatch{})
	h.expect("the newer build", 3, 0, 0)
	h.fb.mu.Lock()
	cancelled := append([]int(nil), h.fb.cancelled...)
	h.fb.mu.Unlock()
	if len(cancelled) != 1 || cancelled[0] != 2 {
		t.Errorf("cancelled builds %v, want [2]", cancelled)
	}
	close(block)
}

func TestDevQuitPaths(t *testing.T) {
	t.Run("q", func(t *testing.T) {
		h := newDevHarness(t, devOptions{}, nil)
		h.expect("first start", 1, 0, 0)
		h.s.push(evKey{'q'})
		if code := h.wait(); code != 0 {
			t.Errorf("exit code %d", code)
		}
	})
	t.Run("signal", func(t *testing.T) {
		h := newDevHarness(t, devOptions{}, nil)
		h.expect("first start", 1, 0, 0)
		h.s.push(evSignal{})
		if code := h.wait(); code != 0 {
			t.Errorf("exit code %d", code)
		}
	})
	t.Run("window closed", func(t *testing.T) {
		h := newDevHarness(t, devOptions{}, nil)
		h.expect("first start", 1, 0, 0)
		h.window().RequestClose()
		if code := h.wait(); code != 0 {
			t.Errorf("exit code %d", code)
		}
	})
	t.Run("window closed after a crash", func(t *testing.T) {
		h := newDevHarness(t, devOptions{}, nil)
		h.expect("first start", 1, 0, 0)
		h.press("p", platform.CodeKeyP)
		h.logged("the app panicked", 1)
		h.window().RequestClose()
		if code := h.wait(); code != 0 {
			t.Errorf("exit code %d", code)
		}
	})
	t.Run("during a swap", func(t *testing.T) {
		h := newDevHarness(t, devOptions{}, nil)
		h.expect("first start", 1, 0, 0)
		h.fb.set(2, "noframe")
		h.s.push(evKey{'r'})
		for !h.frozen() {
			time.Sleep(time.Millisecond)
		}
		h.s.push(evKey{'q'})
		if code := h.wait(); code != 0 {
			t.Errorf("exit code %d", code)
		}
	})
	t.Run("host ends", func(t *testing.T) {
		h := newDevHarness(t, devOptions{}, nil)
		h.expect("first start", 1, 0, 0)
		h.hb.Quit()
		if code := h.wait(); code != 1 {
			t.Errorf("exit code %d, want 1", code)
		}
		h.logged("the window's event loop ended", 1)
	})
}

func TestDevForcedFlags(t *testing.T) {
	h := newDevHarness(t, devOptions{SizeSet: true, W: 300, H: 200, Dark: true, Scale: 2}, nil)
	w := h.window()
	deadline := time.Now().Add(devWait)
	for w.Frame() == nil {
		if time.Now().After(deadline) {
			t.Fatal("no frame")
		}
		time.Sleep(time.Millisecond)
	}
	if o := w.Options(); o.Size != (platform.Size{W: 300, H: 200}) {
		t.Errorf("window size %v, want the --size 300x200", o.Size)
	}
	if got, _ := h.color(); got != (color.RGBA{1, 0, 0x80, 255}) {
		t.Errorf("color %v: the child does not see dark mode", got)
	}
	var frame int
	h.ui(func() { frame = h.s.host.Frame(w).Rect.Dx() })
	if frame != 600 {
		t.Errorf("the child renders %d px wide, want 600 at --scale 2", frame)
	}
}

// TestDevChildEnv checks the environment of a child.
func TestDevChildEnv(t *testing.T) {
	s := newSupervisor(supervisorConfig{
		opts: devOptions{NoState: true},
		log:  &syncBuffer{},
		env:  []string{"PATH=/bin", "TELA_REMOTE=/stale", "TELA_DEVLOOP=0", "HOME=/h"},
	})
	h, err := remote.NewHost(headless.New(headless.Options{}), remote.HostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s.host = h
	env := s.childEnv("7", []string{"X=1"})
	want := []string{"PATH=/bin", "HOME=/h", "TELA_REMOTE=" + h.Addr(), "TELA_REMOTE_TAG=7", "TELA_DEVLOOP=1", "TELA_DEVLOOP_NO_STATE=1", "TELA_DEV=1", "X=1"}
	if strings.Join(env, "\n") != strings.Join(want, "\n") {
		t.Errorf("env\n%s\nwant\n%s", strings.Join(env, "\n"), strings.Join(want, "\n"))
	}
	s.cfg.env = []string{"TELA_DEV=0"}
	s.opts.NoState = false
	if env := s.childEnv("1", nil); !strings.Contains(strings.Join(env, " "), "TELA_DEV=0") || strings.Contains(strings.Join(env, " "), "TELA_DEV=1") {
		t.Errorf("the user's TELA_DEV is not kept: %v", env)
	}
}
