package main

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/watch"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/platform/remote"
	"github.com/kuronosu/tela/internal/testutil/wine"
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
	return newDevHarnessHost(t, opts, configure, nil)
}

// newDevHarnessHost is newDevHarness with a function that may rewrite the
// remote host's options (to wrap the supervisor's callbacks).
func newDevHarnessHost(t *testing.T, opts devOptions, configure func(*supervisorConfig), hostOpts func(*devHarness, *remote.HostOptions)) *devHarness {
	t.Helper()
	if testing.Short() {
		t.Skip("starts processes")
	}
	wine.Skip(t, "no AF_UNIX sockets (WSAEAFNOSUPPORT), so the remote host cannot listen")
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
	ho := h.s.hostOptions()
	if hostOpts != nil {
		hostOpts(h, &ho)
	}
	host, err := remote.NewHost(h.hb, ho)
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

// waitOutput waits until the output contains one of subs, for at most
// devWait. It may run on the host's UI thread, so it does not fail the
// test: the test's own checks report what is missing.
func (h *devHarness) waitOutput(subs ...string) {
	deadline := time.Now().Add(devWait)
	for time.Now().Before(deadline) {
		out := h.out.String()
		for _, s := range subs {
			if strings.Contains(out, s) {
				return
			}
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
	h.logged("tela dev: started in", 1)
	if out := h.out.String(); strings.Contains(out, "restarted in") {
		t.Errorf("the first start is reported as a restart:\n%s", out)
	}
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

// TestDevSlowStartHint: a reload's build that connects more than slowStart
// after its start prints the Defender hint (on Windows; the threshold is
// set here on every OS), once per session, naming the exclusions as
// PowerShell literals. The first start of a session never does, however
// slow. Every build here waits fakeSlow before it connects, more than the
// threshold, so nothing depends on how fast a process starts. Without
// slowStart (other systems) nothing is printed.
func TestDevSlowStartHint(t *testing.T) {
	const hint = "Add-MpPreference -ExclusionPath"
	slowFirst := func(h *devHarness, _ *remote.HostOptions) { h.fb.set(1, "slowstart") }
	t.Run("on", func(t *testing.T) {
		h := newDevHarnessHost(t, devOptions{}, func(c *supervisorConfig) {
			c.slowStart = fakeSlow - 100*time.Millisecond
			c.exclusions = []string{`C:\Users\dev\AppData\Local\Temp\tela-dev`, `C:\it's\go-build`}
		}, slowFirst)
		h.expect("first start", 1, 0, 0)
		if out := h.out.String(); strings.Contains(out, hint) {
			t.Errorf("a hint for the first start:\n%s", out)
		}
		h.fb.set(2, "slowstart")
		h.s.push(evKey{'r'})
		h.expect("a slow reload", 2, 0, 0)
		h.logged(hint, 1)
		h.fb.set(3, "slowstart")
		h.s.push(evKey{'r'})
		h.expect("another slow reload", 3, 0, 0)
		h.logged("reloaded in", 2)
		out := h.out.String()
		if n := strings.Count(out, hint); n != 1 {
			t.Errorf("the hint was printed %d times, want once:\n%s", n, out)
		}
		for _, want := range []string{
			"tela dev: build 2 took ",
			"Microsoft Defender",
			hint + ` 'C:\Users\dev\AppData\Local\Temp\tela-dev', 'C:\it''s\go-build'` + "\n",
			"never changes these settings",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("the output lacks %q:\n%s", want, out)
			}
		}
	})
	t.Run("off", func(t *testing.T) {
		h := newDevHarnessHost(t, devOptions{}, nil, slowFirst)
		h.expect("first start", 1, 0, 0)
		h.fb.set(2, "slowstart")
		h.s.push(evKey{'r'})
		h.expect("a slow reload", 2, 0, 0)
		h.logged("reloaded in", 1)
		if out := h.out.String(); strings.Contains(out, hint) {
			t.Errorf("a hint without slowStart:\n%s", out)
		}
	})
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

// A first-frame timeout that the supervisor handles after it posted the
// promotion of the new build (the timer fired between the first frame and
// the promotion's result) must not undo the swap: the host has already
// shown the new child and detached the old one, so failing the swap would
// stop the child on screen and keep a detached one as the active app.
func TestDevTimeoutAfterPromotionPosted(t *testing.T) {
	h := newDevHarnessHost(t, devOptions{}, nil, func(h *devHarness, o *remote.HostOptions) {
		first := o.OnFirstFrame
		o.OnFirstFrame = func(c *remote.Child) {
			first(c)
			if c.Tag() == "2" {
				// Queued after the first frame and before the promotion's
				// result, which the host's UI thread (running this) can
				// only push once this returns.
				h.s.push(evTimeout{2})
			}
		}
	})
	h.expect("first start", 1, 0, 0)
	h.click()
	h.expect("one click", 1, 1, 1)
	h.s.push(evKey{'r'})
	h.expect("the new build", 2, 1, 0)
	h.logged("reloaded in", 1)
	// The new build is the app: it stays alive and gets the input.
	h.click()
	h.expect("a click after the swap", 2, 2, 1)
	h.still("the new build", 2, 2, 1, 200*time.Millisecond)
	if got := h.view.entries(); len(got) != 0 {
		t.Errorf("problems shown: %v", got)
	}
}

// The new build exits after the supervisor posted its promotion and before
// the promotion's result. The promotion decides what the exit is: if the
// host showed the new build (and detached the old child), a crash of the
// app now shown, after which the next build starts fresh, with no reload
// line for that build; if not (the host saw the connection end first), a
// failed swap that keeps the old child, reported as a build that showed a
// frame and went away before it could take over. Before, the exit always
// failed the swap, so a shown build left the detached old child as the app.
func TestDevExitAfterPromotionPosted(t *testing.T) {
	const pending, failed = "while its promotion was pending", "before its first frame"
	for _, shown := range []bool{true, false} {
		t.Run(fmt.Sprintf("shown=%v", shown), func(t *testing.T) {
			h := newDevHarnessHost(t, devOptions{Verbose: true}, nil, func(h *devHarness, o *remote.HostOptions) {
				first, disconnect := o.OnFirstFrame, o.OnDisconnect
				kill := func(c *remote.Child) {
					if p, err := os.FindProcess(c.PID()); err == nil {
						p.Kill()
					}
				}
				// promote hands the first frame of build 2 to the supervisor
				// and returns once the supervisor holds the build's exit for
				// the promotion's result. It runs on the host's UI thread:
				// what is posted to it meanwhile (the promotion, the end of
				// the connection) runs once it returns, in order.
				promote := func(c *remote.Child) {
					first(c)
					// Once the supervisor has handled the first frame (and
					// posted the promotion), it logs the marker queued
					// after it.
					h.s.push(evHostError{errors.New("marker")})
					h.waitOutput("host: marker")
					if shown {
						kill(c)
					}
					h.waitOutput(pending, failed)
				}
				o.OnFirstFrame = func(c *remote.Child) {
					switch {
					case c.Tag() != "2":
						first(c)
					case shown:
						promote(c)
					default:
						// The supervisor learns of the first frame once the
						// host has seen the connection end (below), so the
						// promotion it then posts fails.
						kill(c)
					}
				}
				o.OnDisconnect = func(c *remote.Child, err error) {
					if c.Tag() == "2" && !shown {
						// The host has detached build 2; its exit reaches the
						// supervisor before this disconnect does.
						promote(c)
					}
					disconnect(c, err)
				}
			})
			h.expect("first start", 1, 0, 0)
			h.click()
			h.expect("one click", 1, 1, 1)
			h.s.push(evKey{'r'})
			if !shown {
				killed := "was killed (signal: killed)"
				if runtime.GOOS == "windows" {
					// Process.Kill is TerminateProcess with exit code 1:
					// Windows has no signals to report.
					killed = "exited with status 1"
				}
				msg := "the new build showed its first frame, then " + killed + " before it could take over; the previous build stays"
				h.logged("before it could take over; the previous build stays", 1)
				if out := h.out.String(); !strings.Contains(out, pending) || strings.Contains(out, failed) || strings.Contains(out, "reloaded in") {
					t.Errorf("want the exit to wait for the failed promotion:\n%s", out)
				}
				// The old build stays and gets input.
				h.click()
				h.expect("the old build", 1, 2, 2)
				if got := h.view.entries(); len(got) != 1 || got[0] != fmt.Sprintf("show %d: %s", problemStart, msg) {
					t.Errorf("problems %v, want %q", got, msg)
				}
				return
			}
			h.logged("; save a change or press r to start it again", 1)
			h.logged("build 1 stopped", 1)
			// The build shown is gone: the crash, and no reload line for it.
			if out := h.out.String(); !strings.Contains(out, pending) || strings.Contains(out, failed) || strings.Contains(out, "reloaded in") {
				t.Errorf("want the swap to finish, its exit to wait for it, and no reload line:\n%s", out)
			}
			// The window keeps the killed build's last frame.
			h.still("the last frame of build 2", 2, 1, 0, 100*time.Millisecond)
			if got := h.view.entries(); len(got) != 1 || !strings.HasPrefix(got[0], fmt.Sprintf("show %d: the app ", problemExit)) {
				t.Errorf("problems %v", got)
			}
			// The next build starts fresh.
			h.s.push(evKey{'r'})
			h.expect("fresh start", 3, 0, 0)
			h.logged("restarted in", 1)
		})
	}
}

// A quit while the promotion of a new build is pending ends the swap: the
// promotion's result, which the host's loop may still deliver, is ignored,
// whether the new build had already exited or not. Before, a successful
// result completed the swap after the quit: the build being stopped became
// the active app and a reload line followed the quit.
func TestDevQuitWhilePromotionPending(t *testing.T) {
	for _, exited := range []bool{true, false} {
		t.Run(fmt.Sprintf("exited=%v", exited), func(t *testing.T) {
			h := newDevHarnessHost(t, devOptions{Verbose: true}, nil, func(h *devHarness, o *remote.HostOptions) {
				first := o.OnFirstFrame
				o.OnFirstFrame = func(c *remote.Child) {
					first(c)
					if c.Tag() != "2" {
						return
					}
					// On the host's UI thread: the promotion the supervisor
					// posts runs once this returns, after the quit.
					h.s.push(evHostError{errors.New("posted")})
					h.waitOutput("host: posted")
					if exited {
						if p, err := os.FindProcess(c.PID()); err == nil {
							p.Kill()
						}
						h.waitOutput("while its promotion was pending")
					}
					h.s.push(evKey{'q'})
					h.s.push(evHostError{errors.New("quitting")})
					h.waitOutput("host: quitting")
				}
			})
			h.expect("first start", 1, 0, 0)
			h.s.push(evKey{'r'})
			if code := h.wait(); code != 0 {
				t.Errorf("exit code %d", code)
			}
			if p := h.s.active; p != nil && p.gen == 2 {
				t.Error("build 2 became the active app after the quit")
			}
			if out := h.out.String(); strings.Contains(out, "reloaded in") || !strings.Contains(out, "host: quitting") {
				t.Errorf("want the quit to end the swap, with no reload line:\n%s", out)
			}
			if got := h.view.entries(); len(got) != 0 {
				t.Errorf("problems shown: %v", got)
			}
		})
	}
}

// The children's standard output and error are pipes that the supervisor
// copies to its own, even when those are files (a terminal, in tela dev): a
// child in its own process group that wrote to the terminal would be
// stopped by SIGTTOU under stty tostop. Each stream arrives whole and in
// order.
func TestDevChildOutputForwarded(t *testing.T) {
	dir := t.TempDir()
	files := map[string]*os.File{}
	for _, name := range []string{"out", "err"} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		files[name] = f
	}
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) {
		c.stdout, c.stderr = files["out"], files["err"]
	})
	h.fb.set(1, "stdio")
	h.expect("first start", 1, 0, 0)
	h.s.push(evKey{'q'})
	h.wait()
	for name, f := range files {
		data, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		var want strings.Builder
		for i := range fakeLines {
			fmt.Fprintf(&want, "%s %d\n", name, i)
		}
		if string(data) != want.String() {
			t.Errorf("std%s of the child: %d bytes, want %d in order:\n%.300s", name, len(data), want.Len(), data)
		}
	}
}

// Without a writer for the children's standard output (nil), a child gets
// the null device, not a pipe and a goroutine that discard what it writes.
func TestDevChildOutputDiscarded(t *testing.T) {
	h := newDevHarness(t, devOptions{}, func(c *supervisorConfig) { c.stdout = nil })
	h.fb.set(1, "nullout")
	h.expect("first start", 1, 0, 0)
	h.s.push(evKey{'q'})
	h.wait()
}

// A process a child started and left running keeps the child's output
// pipes open. The child's exit is classified without waiting for them, and
// the output is detached a moment later, with a line that says so (always,
// not only with -v) instead of cutting it silently.
func TestDevChildOutputHeldByAnotherProcess(t *testing.T) {
	h := newDevHarness(t, devOptions{Verbose: true}, nil)
	h.fb.set(1, "orphan")
	h.expect("first start", 1, 0, 0)
	h.logged("holder tick", 1) // forwarded while build 1 runs
	h.s.push(evKey{'r'})
	h.expect("the new build", 2, 0, 0)
	const stopped, detached = "build 1 stopped (exited with status 0)", "a process started by build 1 still holds its output"
	h.logged(stopped, 1)
	h.logged(detached, 1)
	time.Sleep(5 * fakeTick)
	out := h.out.String()
	i, j := strings.Index(out, stopped), strings.Index(out, detached)
	if i > j {
		t.Errorf("the exit was classified after the output was detached:\n%s", out)
	}
	// A line the holder wrote while the output was being cut may follow.
	if n := strings.Count(out[j:], "holder tick"); n > 1 {
		t.Errorf("%d lines of the holder after the output was detached:\n%s", n, out)
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

// The supervisor's build rules need no host and no child process: these
// tests call handle directly on a supervisor with a fake builder, the way
// run does, so they are deterministic and run under -short. What is not
// started (a swap, which would freeze the host and spawn a process) is
// replaced by the state a swap leaves behind, set by hand.

// directSupervisor is a supervisor with a fake builder, no host and no
// children.
func directSupervisor(t *testing.T) (*supervisor, *fakeBuilder, *recordView, *syncBuffer) {
	t.Helper()
	fb := &fakeBuilder{modes: map[int]string{}, fail: map[int]string{}, block: map[int]chan struct{}{}}
	out := &syncBuffer{}
	view := &recordView{}
	s := newSupervisor(supervisorConfig{opts: devOptions{Pkg: "./fake"}, builder: fb, log: out, stdout: out, stderr: out, view: view})
	view.term = terminalView{s.log}
	return s, fb, view, out
}

// runningApp makes s believe build gen is alive and shown. Its exited
// channel is open: closing it is the app's exit.
func runningApp(s *supervisor, gen int) *proc {
	p := &proc{gen: gen, exited: make(chan struct{})}
	s.procs[gen] = p
	s.active = p
	s.gen = gen
	return p
}

// next pops the supervisor's next queued event, failing the test when none
// comes.
func next(t *testing.T, s *supervisor) any {
	t.Helper()
	ch := make(chan any, 1)
	go func() { ch <- s.q.pop() }()
	select {
	case e := <-ch:
		return e
	case <-time.After(devWait):
		t.Fatal("no event in the supervisor's queue")
		return nil
	}
}

func TestDevBuildFailure(t *testing.T) {
	s, fb, view, out := directSupervisor(t)
	app := runningApp(s, 1)
	fb.fail[2] = "./main.go:3:1: syntax error: unexpected }"

	s.handle(evKey{'r'})
	built, ok := next(t, s).(evBuilt)
	if !ok || built.res.gen != 2 || built.res.err == nil {
		t.Fatalf("the build event is %+v, want a failed build 2", built)
	}
	s.handle(built)
	if got := out.String(); !strings.Contains(got, "build failed:\n./main.go:3:1: syntax error") {
		t.Errorf("log %q does not show the compiler output", got)
	}
	// The problem is shown over a live app, which no swap touches: a swap
	// would freeze the host, and there is none.
	if p := view.lastProblem(); p.kind != problemBuild || p.gen != 2 || !p.live || !strings.Contains(p.text, "syntax error") {
		t.Errorf("problem %+v, want the build error over a live app", p)
	}
	if !s.problem || s.swap != nil || s.ready != nil || s.building != nil || s.inflight != 0 {
		t.Errorf("after the failure: problem %v swap %v ready %v building %v inflight %d", s.problem, s.swap, s.ready, s.building != nil, s.inflight)
	}
	if s.active != app || s.alive() != app {
		t.Errorf("the running app changed: active %v, alive %v", s.active, s.alive())
	}

	// The next good build is shown on promotion, which clears the problem.
	// The swap that brought it that far is set by hand; the old app stops.
	close(app.exited)
	next3 := &proc{gen: 3, exited: make(chan struct{})}
	s.procs[3] = next3
	s.gen = 3
	s.swap = &swapState{res: buildResult{gen: 3}, proc: next3, state: "reset", promoting: true, timer: time.NewTimer(time.Hour)}
	s.handle(evPromoted{gen: 3})
	if s.problem || s.swap != nil || s.active != next3 {
		t.Errorf("after the promotion: problem %v swap %v active %v", s.problem, s.swap, s.active)
	}
	got := view.entries()
	if len(got) != 2 || got[0] != fmt.Sprintf("show %d: build failed", problemBuild) || got[1] != "clear" {
		t.Errorf("problems %v", got)
	}
}

func TestDevNewerSaveCancelsBuild(t *testing.T) {
	s, fb, _, _ := directSupervisor(t)
	// A swap is in progress, so a finished build waits in s.ready instead
	// of starting one (which needs a host).
	runningApp(s, 1)
	s.swap = &swapState{res: buildResult{gen: 1}}
	fb.block[2] = make(chan struct{}) // never released: only cancelling ends it

	s.handle(evKey{'r'})
	if s.gen != 2 || s.building == nil {
		t.Fatalf("gen %d building %v, want build 2 running", s.gen, s.building != nil)
	}
	first := s.trig.at
	s.handle(evBatch{b: watch.Batch{First: first.Add(-time.Second)}})
	if s.gen != 3 || s.inflight != 2 {
		t.Fatalf("gen %d inflight %d, want build 3 started with build 2 still to report", s.gen, s.inflight)
	}
	// Both results arrive, in either order: the cancelled build 2 is
	// ignored and build 3 is the one that waits for the swap.
	for range 2 {
		s.handle(next(t, s))
	}
	if s.inflight != 0 || s.building != nil {
		t.Errorf("inflight %d building %v after both results", s.inflight, s.building != nil)
	}
	if s.ready == nil || s.ready.res.gen != 3 {
		t.Fatalf("ready %+v, want build 3", s.ready)
	}
	if !s.ready.trig.at.Equal(first.Add(-time.Second)) {
		t.Errorf("trigger time %v, want the oldest save's %v", s.ready.trig.at, first.Add(-time.Second))
	}
	fb.mu.Lock()
	cancelled, builds := append([]int(nil), fb.cancelled...), fb.builds
	fb.mu.Unlock()
	if len(cancelled) != 1 || cancelled[0] != 2 || builds != 2 {
		t.Errorf("cancelled builds %v of %d, want [2] of 2", cancelled, builds)
	}
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
	wine.Skip(t, "no AF_UNIX sockets (WSAEAFNOSUPPORT), so the remote host cannot listen")
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
