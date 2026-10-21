package telatest

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/semantics"
)

// Stats counts the work of the app's tree since it was mounted: frames,
// component builds and skipped builds, style, layout and paint passes. A
// test uses the difference between two readings to assert how narrow an
// update was.
type Stats = core.Stats

// idPrefix starts the strings of tela.UseID in mounted apps, so they do
// not depend on how many trees the process made before.
const idPrefix = "tt-"

// App is a UI mounted in a headless window: Tela's real pipeline and event
// dispatch with a window that is a record in memory and a clock that only
// moves when the app waits or the test calls [App.Advance].
//
// Actions and expectations settle the app first: they apply the pending
// state updates and run frames, advancing the clock to each frame, until
// nothing is dirty and no animation or scroll fling is running. A held
// pointer's long-press timer, and the fade of overlay scrollbars, fire only
// when the clock reaches them, with [App.Advance] or while an expectation
// waits.
//
// An App must be used from the goroutine of its test; the state setters
// and tela.Post may be called from any goroutine.
type App struct {
	tb      testing.TB
	cfg     config
	backend *headless.Backend
	win     *headless.Window
	host    *core.Host
	tree    *core.Tree
	stop    func() // ends the delivery of tela.Post
	closed  bool
	// busy: the last settle gave up waiting for the app to go quiet.
	busy bool

	// The pointer: its last position, whether it ever entered the window,
	// and the buttons held.
	pos    platform.Point
	hasPos bool
	held   platform.Buttons

	// dev is whether the tree runs in dev mode (the options and
	// TELA_DEV).
	dev bool

	mu    sync.Mutex // guards logs and warns (dev messages may come from any goroutine)
	logs  []string   // dev errors
	warns []string   // dev warnings
}

// Mount shows root in a headless window and returns the running app. The
// window starts focused, so key events reach the focused element; it is
// unmounted when the test ends, running the cleanups of its effects. root
// is a node or a component: a struct component value, or a function
// component wrapped in tela.C or tela.CP (or use [MountFunc]).
//
//	app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))
//	app := telatest.Mount(t, Card{Title: "Inbox"})
//
// Mount makes the app receive the functions given to tela.Post, which are
// delivered to one tree per process: do not mount apps that use Post from
// parallel tests.
func Mount(tb testing.TB, root tela.Component, opts ...Option) *App {
	tb.Helper()
	cfg := newConfig(opts)
	if err := cfg.validate(); err != nil {
		tb.Fatalf("telatest: %v", err)
	}
	rootNode, err := rootOf(root)
	if err != nil {
		tb.Fatalf("telatest: Mount: %v", err)
	}
	a := &App{tb: tb, cfg: cfg, dev: cfg.devMode()}
	size := platform.Size{W: cfg.width, H: cfg.height}
	a.backend = headless.New(headless.Options{
		Size:     size,
		Scale:    cfg.scale,
		Settings: platform.SystemSettings{DarkMode: cfg.dark, ReducedMotion: cfg.reducedMotion},
	})
	cc := cfg.coreConfig()
	cc.IDPrefix = idPrefix
	cc.Log = a.log
	cc.Warn = a.warn
	host, err := core.NewHost(a.backend, core.Options{
		Config: cc,
		Title:  "telatest",
		Width:  cfg.width, Height: cfg.height,
		Root:     rootNode,
		ErrorLog: testWriter{tb},
	})
	if err != nil {
		tb.Fatalf("telatest: cannot create the headless window: %v", err)
	}
	a.host, a.tree = host, host.Tree()
	a.win = host.Window().(*headless.Window)
	if cfg.checkDamage {
		host.AfterFrame = a.checkDamage
	}
	a.win.SetFocused(true)
	a.stop = a.tree.ReceivePosts()
	tb.Cleanup(a.Close)
	a.settleAfter()
	return a
}

// checkDamage compares the window with a full rendering of the tree
// ([CheckDamage]) when the tree has nothing left to draw.
func (a *App) checkDamage() {
	tr := a.tree
	if tr.Dirty() || tr.Debug() != 0 {
		return // the window shows an older state than the tree, or overlays
	}
	got := a.win.Frame()
	want := tr.Rasterize(tr.ReferenceFrame(false), tr.Scale())
	if got == nil || got.Rect != want.Rect {
		a.tb.Fatalf("telatest: damage check: the window holds %v, a full rendering %v", got.Bounds(), want.Rect)
	}
	for y := got.Rect.Min.Y; y < got.Rect.Max.Y; y++ {
		g := got.Pix[got.PixOffset(0, y):][:4*got.Rect.Dx()]
		w := want.Pix[want.PixOffset(0, y):][:4*want.Rect.Dx()]
		if string(g) == string(w) {
			continue
		}
		for i := range g {
			if g[i] != w[i] {
				x := got.Rect.Min.X + i/4
				a.tb.Fatalf("telatest: damage check: pixel (%d, %d) is %v in the window, %v in a full rendering (last damage %+v)",
					x, y, got.RGBAAt(x, y), want.RGBAAt(x, y), tr.LastDamage())
			}
		}
	}
}

// testWriter sends the host's error log (a failed present) to the test log.
type testWriter struct{ tb testing.TB }

// Write logs p as part of the test's output; it makes testWriter an
// io.Writer for the host's error log.
func (w testWriter) Write(p []byte) (int, error) {
	w.tb.Logf("%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// Close unmounts the app now, running the cleanups of its effects. The
// test does it when it ends; call it to test unmounting. Using the app
// afterwards fails the test.
func (a *App) Close() {
	if a.closed {
		return
	}
	a.closed = true
	a.stop()
	a.tree.Unmount()
	a.win.Close()
	a.backend.Quit()
	a.flushDev()
}

// log collects a dev error.
func (a *App) log(msg string) {
	a.mu.Lock()
	a.logs = append(a.logs, msg)
	a.mu.Unlock()
}

// warn collects a dev warning.
func (a *App) warn(msg string) {
	a.mu.Lock()
	a.warns = append(a.warns, msg)
	a.mu.Unlock()
}

// flushDev reports the dev messages logged since the last call: errors
// fail the test in dev mode unless allowed ([AllowDev]) and are test logs
// otherwise; warnings are test logs.
func (a *App) flushDev() {
	a.tb.Helper()
	a.mu.Lock()
	logs, warns := a.logs, a.warns
	a.logs, a.warns = nil, nil
	a.mu.Unlock()
	for _, m := range logs {
		if a.dev && !a.devAllowed(m) {
			a.tb.Errorf("telatest: dev message: %s\n(telatest.AllowDev(%q) lets a test accept it)", m, firstLine(m))
		} else {
			a.tb.Logf("dev message: %s", m)
		}
	}
	for _, m := range warns {
		a.tb.Logf("dev warning: %s", m)
	}
}

func (a *App) devAllowed(m string) bool {
	for _, s := range a.cfg.allowDev {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 40 {
		line = line[:40]
	}
	return line
}

// fatalf fails the test with a message, followed by the app's aria
// snapshot.
func (a *App) fatalf(format string, args ...any) {
	a.tb.Helper()
	a.tb.Fatalf("telatest: %s%s", fmt.Sprintf(format, args...), a.snapshotSuffix())
}

// snapshotSuffix returns the aria snapshot, indented, for error messages.
func (a *App) snapshotSuffix() string {
	if a.closed {
		return ""
	}
	snap := semantics.Snapshot(a.tree.Semantics())
	if snap == "" {
		return "\n\naria snapshot: (empty: nothing visible has a role or text)"
	}
	lines := strings.Split(snap, "\n")
	const maxLines = 60
	more := ""
	if len(lines) > maxLines {
		more = fmt.Sprintf("\n  ... (%d more lines)", len(lines)-maxLines)
		lines = lines[:maxLines]
	}
	return "\n\naria snapshot:\n  " + strings.Join(lines, "\n  ") + more
}

func (a *App) mustBeOpen() {
	a.tb.Helper()
	if a.closed {
		a.tb.Fatalf("telatest: the app was closed (App.Close or the end of the test); mount it again")
	}
}

// ---------------------------------------------------------------------
// The clock and settling.

// deliver handles everything the backend has ready at the current time.
func (a *App) deliver() { a.backend.Drain(a.host) }

// pendingWork reports whether the app has work: queued events, a frame
// requested, or a dirty tree.
func (a *App) pendingWork() bool {
	if a.backend.Pending() || a.tree.Dirty() || a.host.NeedsFrame() {
		return true
	}
	// A frame requested for the debug overlays alone (a fading flash, the
	// fps meter), or for infinite animations alone (a spinner, a pulse),
	// is not work the app must finish. Finite transitions and animations
	// are: settle advances the clock until they end.
	return a.win.FrameRequested() && !a.tree.DebugWantsFrame() && !a.tree.OnlyEndlessAnimations()
}

// busyFrames is how many frames settle runs once the app has shown that it
// never goes quiet.
const busyFrames = 3

// settle runs the app until it is quiet: state updates applied, frames run
// (the clock advancing to each frame the window waits for) and animations
// and flings finished.
//
// An app that never goes quiet (an animation that never ends, a goroutine
// that sets state every few milliseconds) cannot be waited for, as in
// Playwright: after the timeout, in app time or in real time, settle logs
// it once and from then on runs only a few frames per call until a call
// finds the app quiet again.
func (a *App) settle() {
	a.tb.Helper()
	a.mustBeOpen()
	a.backend.Wake() // posted functions and state set from other goroutines
	start, realStart := a.backend.Now(), time.Now()
	quietLimit := min(a.cfg.timeout, DefaultTimeout)
	limit := quietLimit
	if a.busy {
		limit = busyFrames * a.backend.FrameInterval()
	}
	stalls := 0
	for {
		a.deliver()
		if !a.pendingWork() {
			a.busy = false
			break
		}
		now := a.backend.Now()
		var step time.Duration
		if next, ok := a.backend.NextFrameTime(); ok && next > now {
			step = next - now
		}
		if step == 0 {
			// Dirty or pending without a frame to wait for: another
			// round delivers it. A tree that stays dirty with nothing
			// delivered would spin forever.
			if stalls++; stalls > 100 {
				a.fatalf("the app did not settle: the tree reports pending work but no frame is requested (is the window zero-sized?)")
			}
			a.backend.Wake()
			continue
		}
		stalls = 0
		if now+step-start > limit || (!a.busy && time.Since(realStart) > quietLimit) {
			if !a.busy {
				a.tb.Logf("telatest: the app was still requesting frames after %v: an endless animation, or a goroutine that "+
					"updates state all the time. Actions will not wait for it to go quiet; use App.Advance(d) to run a fixed time", quietLimit)
			}
			a.busy = true
			break
		}
		a.backend.Advance(step)
		a.backend.Wake()
	}
	a.flushDev()
}

// settleAfter settles the app after the events of an action (or at
// Mount): fully, or under [NoSettle] only the work due, leaving running
// animations where they are.
func (a *App) settleAfter() {
	a.tb.Helper()
	if a.cfg.noSettle {
		a.flush()
		return
	}
	a.settle()
}

// flush handles the work due without waiting for animations: the events
// queued, the state updates and the frames they request, advancing the
// clock only to those frames, until the tree has nothing left to do but
// the frames of running animations (NoSettle).
func (a *App) flush() {
	a.tb.Helper()
	a.mustBeOpen()
	a.backend.Wake()
	start, realStart := a.backend.Now(), time.Now()
	limit := min(a.cfg.timeout, DefaultTimeout)
	for stalls := 0; ; {
		a.deliver()
		if !a.backend.Pending() && !a.tree.Dirty() && !a.host.NeedsFrame() {
			break
		}
		now := a.backend.Now()
		var step time.Duration
		if next, ok := a.backend.NextFrameTime(); ok && next > now {
			step = next - now
		}
		if step == 0 {
			if stalls++; stalls > 100 {
				a.fatalf("the app did not settle: the tree reports pending work but no frame is requested (is the window zero-sized?)")
			}
			a.backend.Wake()
			continue
		}
		stalls = 0
		if now+step-start > limit || time.Since(realStart) > limit {
			break // a goroutine that sets state all the time
		}
		a.backend.Advance(step)
		a.backend.Wake()
	}
	a.flushDev()
}

// advance moves the app clock forward by d, running every frame and
// deadline (a long press, scrollbars starting to fade) on the way.
func (a *App) advance(d time.Duration) {
	a.tb.Helper()
	target := a.backend.Now() + d
	a.backend.Wake() // let the deadlines that are already due fire
	for {
		a.deliver()
		now := a.backend.Now()
		if now >= target {
			break
		}
		next := target
		if t, ok := a.backend.NextFrameTime(); ok && t > now && t < next {
			next = t
		}
		if t, ok := a.tree.NextDeadline(); ok && t > now && t < next {
			next = t
		}
		a.backend.Advance(next - now)
		a.backend.Wake()
	}
	a.flushDev()
}

// Advance runs the app's clock forward by d: the frames, animations,
// scroll animations and flings, long-press timers and other deadlines that
// fall within d happen in order, each frame at its own time. It does not
// settle the app afterwards: work requested at the very end is handled by
// the next action, expectation or Advance.
func (a *App) Advance(d time.Duration) {
	a.tb.Helper()
	a.mustBeOpen()
	if d < 0 {
		a.tb.Fatalf("telatest: Advance(%v): the clock cannot go backwards; pass a positive duration such as 300*time.Millisecond", d)
	}
	a.advance(d)
}

// idle lets time pass between two checks of a wait: one frame of app time,
// and a moment for other goroutines (a ticker that sets state) to run.
func (a *App) idle(round int) {
	a.advance(a.backend.FrameInterval())
	if round < 50 {
		runtime.Gosched()
	} else {
		time.Sleep(time.Millisecond)
	}
}

// settleQuery brings the app up to date for a query or an expectation:
// fully, or under [NoSettle] only the work due, so that reading the app
// never moves its animations (ADR-097).
func (a *App) settleQuery() {
	a.tb.Helper()
	if a.cfg.noSettle {
		a.flush()
		return
	}
	a.settle()
}

// waitFor is wait for expectations and queries: the app is settled before
// each try, but under [NoSettle] only the work due is handled.
func (a *App) waitFor(check func() (done bool, why string)) {
	a.tb.Helper()
	a.wait(a.settleQuery, check)
}

// waitSettled is wait for the target of an action, which settles the app
// fully even under [NoSettle]: the animations an earlier step started end
// before the action acts (ADR-080).
func (a *App) waitSettled(check func() (done bool, why string)) {
	a.tb.Helper()
	a.wait(a.settle, check)
}

// wait calls settle and then check until check reports done, or fails the
// test with the message check returned at the timeout. A failed try lets a
// frame of app time pass before the next. A check that cannot succeed by
// waiting fails the test itself.
func (a *App) wait(settle func(), check func() (done bool, why string)) {
	a.tb.Helper()
	deadline := time.Now().Add(a.cfg.timeout)
	for round := 0; ; round++ {
		settle()
		done, why := check()
		if done {
			return
		}
		if time.Now().After(deadline) {
			a.fatalf("%s\n(waited %v)", why, a.cfg.timeout)
		}
		a.idle(round)
	}
}

// WaitFor waits until cond reports true, settling the app and letting a
// frame of app time and a moment of real time pass between tries, until the
// timeout (see [Timeout]); under [NoSettle] the first try handles only the
// work due, and the clock moves only between failed tries. It is for
// conditions the locators cannot see:
// a value computed from [App.Bounds], or Go state that a goroutine sets.
// what names the condition in the failure message ("the progress bar
// reached 3 s"). cond runs on the test goroutine.
func (a *App) WaitFor(what string, cond func() bool) {
	a.tb.Helper()
	a.mustBeOpen()
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		if cond() {
			return true, ""
		}
		return false, "timed out waiting for " + what
	})
}

// ---------------------------------------------------------------------
// Inspection.

// Screenshot returns a copy of the window's content, in physical pixels
// (the logical size times the scale), after settling the app; with
// [NoSettle], the content at the current time, running animations where
// they are.
func (a *App) Screenshot() *image.RGBA {
	a.tb.Helper()
	a.settleAfter()
	img := a.win.Frame()
	if img == nil {
		a.fatalf("the window has not presented a frame yet (is the window zero-sized?)")
	}
	return img
}

// AriaSnapshot returns the app's semantics tree as a Playwright aria
// snapshot, the YAML that `tela inspect` prints, without a trailing
// newline, after settling the app (under [NoSettle], handling only the
// work due: running animations stay where they are). For a counter:
//
//	snap := app.AriaSnapshot()
//	// - heading "Count: 1" [level=1]
//	// - button "Increment"
func (a *App) AriaSnapshot() string {
	a.tb.Helper()
	a.settleQuery()
	return semantics.Snapshot(a.tree.Semantics())
}

// Stats returns the work counters of the app's tree, after settling it
// (under [NoSettle], handling only the work due).
func (a *App) Stats() Stats {
	a.tb.Helper()
	a.settleQuery()
	return a.tree.Stats()
}

// Cursor returns the CSS name of the mouse cursor the window shows ("default",
// "pointer", "not-allowed"...), after settling the app (under [NoSettle],
// handling only the work due).
func (a *App) Cursor() string {
	a.tb.Helper()
	a.settleQuery()
	return a.win.Cursor().String()
}

// Now returns the app's clock: the time since the window was created, as
// moved by waits and [App.Advance].
func (a *App) Now() time.Duration { return a.backend.Now() }
