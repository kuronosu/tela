package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/watch"
	"github.com/kuronosu/tela/internal/devloop"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// The supervisor of tela dev (ADR-115). It owns the remote host (and so the
// real window) and runs builds of the app as child processes. Everything it
// decides happens on one goroutine (run), which takes events from a queue:
// file changes, keys, signals, build results, and what the host's
// callbacks report from the host's UI thread. Nothing it does blocks that
// thread: work for it goes through Host.Post, and slow requests to a child
// (the state) run on goroutines that report back through the queue.
//
// A swap, after a successful build:
//
//  1. Freeze the host's input (it is queued) when a child is alive.
//  2. Ask the active child for its state (devloop.KindStateSave), for at
//     most saveTimeout; on an error proceed without state and say so.
//  3. Start the new child with the state to seed it; it fetches it with
//     devloop.KindStateLoad, connects as a pending child and renders
//     offscreen.
//  4. At its first frame (at most firstFrameTimeout after the start),
//     promote it: it is shown and gets the queued input. Then stop the old
//     child: devloop.KindQuit, and after stopTimeout the connection is
//     closed and the process killed.
//
// If the new child exits or times out before its first frame, the input is
// unfrozen and the old child stays. When no child is alive (the app
// crashed), a build starts fresh: no state, nothing frozen.

// supervisorConfig are the parts the supervisor works with; the tests give
// fakes.
type supervisorConfig struct {
	opts    devOptions
	builder builder
	// log receives the supervisor's lines; stdout and stderr the
	// children's output, copied from pipes (never handed to a child).
	log, stdout, stderr io.Writer
	// env is the environment children start from.
	env []string
	// restore puts the terminal back (nil: nothing to restore); called
	// before a panic of the supervisor's goroutines propagates.
	restore func()
	// watchSet, setRoots and refreshDeps keep the watcher in step with the
	// app (nil in tests that do not watch).
	watchSet    *watchSet
	setRoots    func(roots []string)
	refreshDeps func() (depSet, error)
	// view shows problems (nil: the terminal).
	view problemView
	// overlay also draws them over the window (ADR-117).
	overlay bool
	// describe and watching complete the start line.
	describe, watching string

	// Timeouts; zero means devloop's, and reportWait the default below.
	saveTimeout, firstFrameTimeout, stopTimeout, reportWait time.Duration
}

// defaultReportWait is how long the line of a finished swap waits for the
// new child's state report, which the child sends once the state is
// restored (a few frames after its first one, so usually after the
// promotion).
const defaultReportWait = 300 * time.Millisecond

type supervisor struct {
	cfg  supervisorConfig
	opts devOptions
	log  *devLog
	view problemView
	ov   *overlayView // nil without cfg.overlay

	host     *remote.Host
	quitHost func()

	q queue

	// The supervisor goroutine's state.
	gen      int // generation of the latest build started
	building context.CancelFunc
	inflight int         // builds whose result has not arrived, cancelled ones included
	trig     *trigger    // why the running build was started
	ready    *readyBuild // a successful build waiting for its swap
	procs    map[int]*proc
	active   *proc // the child shown (nil: none alive)
	swap     *swapState
	held     *heldLine // the line of the latest swap, waiting for the state report
	problem  bool      // a problem is shown
	started  bool      // a child has been promoted once

	quitting   bool
	exitCode   int
	finished   bool
	hostDone   bool
	quitByTime *time.Timer

	stateMu sync.Mutex
	states  map[string][]byte // child tag -> the state to seed it with
}

// trigger is why a build runs, for its timing and its state policy.
type trigger struct {
	at    time.Time
	keep  bool // keep state (false after R)
	why   string
	first bool
}

// proc is a child process.
type proc struct {
	gen     int
	tag     string
	cmd     *exec.Cmd
	exe     string
	owned   bool // exe is a build of tela dev: delete it when done
	exited  chan struct{}
	waitErr error // valid once exited is closed

	child    *remote.Child
	tail     *tailWriter // the end of its standard error
	panic    string
	report   *devloop.StateReport
	stopping bool
	lostConn *time.Timer

	exitSeen     bool // the process exited
	disconnected bool // its connection ended
	settle       *time.Timer
	classified   bool // exited handled it
}

// exitSettle is how long an exit waits for the end of the child's
// connection (normally a few milliseconds after the exit).
const exitSettle = 500 * time.Millisecond

// swapState is a swap in progress.
type swapState struct {
	res    buildResult
	trig   trigger
	proc   *proc
	frozen bool
	// promoting: the promotion of proc is posted to the host. From then on
	// the host may show proc and detach the old child at any moment, so
	// only the promotion's result (evPromoted) decides the swap, not the
	// first-frame timeout.
	promoting bool
	state     string // "kept", "fresh", or why it was not kept
	timer     *time.Timer
	saved     time.Time
	started   time.Time
	first     time.Time
}

// Events of the supervisor's queue.
type (
	evBatch  struct{ b watch.Batch }
	evKey    struct{ r rune }
	evSignal struct{}
	evBuilt  struct{ res buildResult }
	evDeps   struct {
		d   depSet
		err error
	}
	evConnected    struct{ c *remote.Child }
	evFirstFrame   struct{ c *remote.Child }
	evDisconnected struct {
		c   *remote.Child
		err error
	}
	evExited  struct{ p *proc }
	evSettled struct{ p *proc }
	evSaved   struct {
		gen  int
		data []byte
		err  error
	}
	evPromoted struct {
		gen int
		err error
	}
	evTimeout        struct{ gen int }
	evLostConn       struct{ p *proc }
	evUnhandledClose struct{}
	evHostDone       struct{ err error }
	evHostError      struct{ err error }
	evPanic          struct {
		c    *remote.Child
		text string
	}
	evReport struct {
		c *remote.Child
		r devloop.StateReport
	}
	evReportDue   struct{ gen int }
	evAssets      struct{ dirs []string }
	evQuitTimeout struct{}
)

func newSupervisor(cfg supervisorConfig) *supervisor {
	if cfg.saveTimeout <= 0 {
		cfg.saveTimeout = devloop.SaveTimeout
	}
	if cfg.firstFrameTimeout <= 0 {
		cfg.firstFrameTimeout = devloop.FirstFrameTimeout
	}
	if cfg.stopTimeout <= 0 {
		cfg.stopTimeout = devloop.StopTimeout
	}
	if cfg.reportWait <= 0 {
		cfg.reportWait = defaultReportWait
	}
	s := &supervisor{
		cfg:    cfg,
		opts:   cfg.opts,
		log:    &devLog{w: cfg.log, verbose: cfg.opts.Verbose},
		procs:  map[int]*proc{},
		states: map[string][]byte{},
	}
	s.view = cfg.view
	if s.view == nil {
		s.view = terminalView{s.log}
	}
	if cfg.overlay {
		s.ov = newOverlayView(func(err error) { s.log.printf("%v", err) })
		s.view = multiView{s.view, s.ov}
	}
	s.q.init()
	return s
}

// hostOptions are the options of the remote host the supervisor drives:
// the forced flags and the callbacks, which only queue events.
func (s *supervisor) hostOptions() remote.HostOptions {
	o := remote.HostOptions{
		Scale:        s.opts.Scale,
		OnConnect:    func(c *remote.Child) { s.push(evConnected{c}) },
		OnFirstFrame: func(c *remote.Child) { s.push(evFirstFrame{c}) },
		OnDisconnect: func(c *remote.Child, err error) { s.push(evDisconnected{c, err}) },
		OnUnhandled: func(w platform.Window, e platform.Event) {
			if _, ok := e.(platform.CloseRequestedEvent); ok {
				s.push(evUnhandledClose{})
			}
		},
		OnError: func(err error) { s.push(evHostError{err}) },
	}
	if s.ov != nil {
		o.Input = s.ov.handleInput
	}
	if s.opts.SizeSet {
		size := platform.Size{W: float32(s.opts.W), H: float32(s.opts.H)}
		o.WindowOptions = func(id uint32, wo platform.WindowOptions) platform.WindowOptions {
			wo.Size = size
			if wo.MinSize.W > size.W || wo.MinSize.H > size.H {
				wo.MinSize = platform.Size{} // --size wins over the app's minimum
			}
			return wo
		}
	}
	if s.opts.Dark {
		o.Settings = func(st platform.SystemSettings) platform.SystemSettings {
			st.DarkMode = true
			return st
		}
	}
	return o
}

// attach gives the supervisor its host (created with hostOptions) and the
// function that ends the host's event loop, and registers the control
// handlers.
func (s *supervisor) attach(h *remote.Host, quitHost func()) {
	s.host, s.quitHost = h, quitHost
	if s.ov != nil {
		s.ov.attach(h)
	}
	h.Handle(devloop.KindStateLoad, func(c *remote.Child, _ []byte) ([]byte, error) {
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		data := s.states[c.Tag()]
		delete(s.states, c.Tag())
		return data, nil
	})
	h.Handle(devloop.KindPanic, func(c *remote.Child, p []byte) ([]byte, error) {
		s.push(evPanic{c, string(p)})
		return nil, nil
	})
	h.Handle(devloop.KindStateReport, func(c *remote.Child, p []byte) ([]byte, error) {
		if r, err := devloop.DecodeStateReport(p); err == nil {
			s.push(evReport{c, r})
		}
		return nil, nil
	})
	h.Handle(devloop.KindAssets, func(c *remote.Child, p []byte) ([]byte, error) {
		s.push(evAssets{devloop.DecodeAssets(p)})
		return nil, nil
	})
}

// push queues an event. Safe from any goroutine; never blocks.
func (s *supervisor) push(e any) { s.q.push(e) }

// goSafe runs f on a goroutine that restores the terminal before a panic
// takes the process down.
func (s *supervisor) goSafe(f func()) {
	go func() {
		defer s.recoverTerminal()
		f()
	}()
}

func (s *supervisor) recoverTerminal() {
	if r := recover(); r != nil {
		if s.cfg.restore != nil {
			s.cfg.restore()
		}
		panic(r)
	}
}

// forward turns the watcher's batches and errors into events.
func (s *supervisor) forward(w *watch.Watcher) {
	defer s.recoverTerminal()
	errs := w.Errors()
	for {
		select {
		case b, ok := <-w.Events():
			if !ok {
				return
			}
			s.push(evBatch{b})
		case err := <-errs:
			s.log.printf("watch: %v", err)
		}
	}
}

// readKeys turns the terminal's key presses into events.
func (s *supervisor) readKeys(r io.Reader) {
	defer s.recoverTerminal()
	br := bufio.NewReader(r)
	for {
		c, _, err := br.ReadRune()
		if err != nil {
			return
		}
		switch c {
		case 'r', 'R', 'q', 3: // 3: Ctrl+C when the terminal does not signal it
			s.push(evKey{c})
		}
	}
}

// run is the supervisor's loop; it returns the exit code of tela dev.
func (s *supervisor) run() int {
	defer s.recoverTerminal()
	what := s.opts.Pkg
	if s.cfg.describe != "" {
		what += " (" + s.cfg.describe + ")"
	}
	s.log.printf("running %s%s; r reload, R reload without state, q quit", what, s.cfg.watching)
	s.startBuild(trigger{at: time.Now(), keep: true, first: true})
	for !s.finished || !s.hostDone {
		s.handle(s.q.pop())
	}
	return s.exitCode
}

func (s *supervisor) handle(e any) {
	switch e := e.(type) {
	case evBatch:
		if s.quitting {
			return
		}
		why := describeChange(e.b)
		s.log.verbosef("changed: %s", why)
		s.startBuild(trigger{at: e.b.First, keep: true, why: why})
	case evKey:
		switch e.r {
		case 'r':
			s.startBuild(trigger{at: time.Now(), keep: true, why: "r"})
		case 'R':
			s.startBuild(trigger{at: time.Now(), keep: false, why: "R"})
		default:
			s.quit(0, "")
		}
	case evSignal:
		if s.quitting {
			s.forceQuit()
			return
		}
		s.quit(0, "")
	case evBuilt:
		s.built(e.res)
	case evDeps:
		if e.err != nil {
			s.log.verbosef("%v", e.err)
			return
		}
		if s.cfg.watchSet != nil {
			s.cfg.watchSet.setDeps(e.d)
			s.updateRoots()
		}
	case evConnected:
		p := s.procOf(e.c)
		if p == nil || p.stopping {
			e.c.Close()
			return
		}
		p.child = e.c
		s.log.verbosef("build %d connected (pid %d)", p.gen, e.c.PID())
	case evFirstFrame:
		s.firstFrame(e.c)
	case evPromoted:
		s.promoted(e.gen, e.err)
	case evSaved:
		s.saved(e.gen, e.data, e.err)
	case evTimeout:
		if sw := s.swap; sw != nil && sw.res.gen == e.gen && sw.proc != nil && !sw.promoting {
			s.swapFailed(problem{kind: problemStart, summary: fmt.Sprintf("the new build showed no frame in %v; %s", s.cfg.firstFrameTimeout, s.stays())})
		}
	case evDisconnected:
		p := s.procOf(e.c)
		if p == nil {
			return
		}
		p.disconnected = true
		if p.exitSeen {
			s.exited(p)
			return
		}
		if p.stopping {
			return
		}
		if e.err != nil {
			s.log.verbosef("build %d: connection lost: %v", p.gen, e.err)
		}
		// The process normally exits right after; a child that closed the
		// connection and kept running is useless: kill it.
		p.lostConn = time.AfterFunc(2*time.Second, func() { s.push(evLostConn{p}) })
	case evLostConn:
		if !isDone(e.p.exited) {
			e.p.cmd.Process.Kill()
		}
	case evExited:
		// What the child sent last (a panic report) arrives through the
		// host's UI thread, unordered with the process's exit: classify
		// the exit once the connection is gone, or soon after the exit
		// if it never connected.
		p := e.p
		p.exitSeen = true
		if p.disconnected {
			s.exited(p)
		} else {
			p.settle = time.AfterFunc(exitSettle, func() { s.push(evSettled{p}) })
		}
	case evSettled:
		s.exited(e.p)
	case evPanic:
		if p := s.procOf(e.c); p != nil {
			p.panic = e.text
		}
	case evReport:
		if p := s.procOf(e.c); p != nil {
			r := e.r
			p.report = &r
			if h := s.held; h != nil && h.sw.proc == p {
				s.flushHeld() // the line of the swap, now with the counts
			}
		}
	case evReportDue:
		if h := s.held; h != nil && h.sw.res.gen == e.gen {
			s.flushHeld()
		}
	case evAssets:
		if s.cfg.watchSet != nil {
			s.cfg.watchSet.setAssets(e.dirs)
			s.updateRoots()
			s.log.verbosef("assets: %s", strings.Join(e.dirs, ", "))
		}
	case evUnhandledClose:
		// The window was closed while no app was alive to handle it.
		s.quit(0, "")
	case evHostError:
		s.log.verbosef("host: %v", e.err)
	case evHostDone:
		s.hostDone = true
		if !s.quitting {
			msg := "the window's event loop ended"
			if e.err != nil {
				msg = fmt.Sprintf("the window's event loop failed: %v", e.err)
			}
			s.quit(1, msg)
		}
	case evQuitTimeout:
		s.forceQuit()
	}
}

func (s *supervisor) updateRoots() {
	if s.cfg.setRoots != nil && s.cfg.watchSet != nil {
		s.cfg.setRoots(s.cfg.watchSet.roots())
	}
}

// describeChange summarizes a batch for the verbose log.
func describeChange(b watch.Batch) string {
	if b.Overflow {
		return "many files (the change notifications overflowed)"
	}
	wd, _ := os.Getwd()
	var paths []string
	for _, p := range b.Paths {
		if rel, err := filepath.Rel(wd, p); err == nil && wd != "" && !strings.HasPrefix(rel, "..") {
			p = rel
		}
		paths = append(paths, p)
	}
	if len(paths) > 3 {
		return strings.Join(paths[:3], ", ") + fmt.Sprintf(" and %d more", len(paths)-3)
	}
	return strings.Join(paths, ", ")
}

func (s *supervisor) procOf(c *remote.Child) *proc {
	gen, err := strconv.Atoi(c.Tag())
	if err != nil {
		return nil
	}
	return s.procs[gen]
}

// --- builds ---

// startBuild starts a build, cancelling the one running.
func (s *supervisor) startBuild(t trigger) {
	if s.quitting {
		return
	}
	if s.building != nil {
		s.building()
		// A cancelled reload keeps the oldest trigger time (the timing
		// counts from the first save) and loses state if either wanted to.
		if s.trig != nil {
			t.at = minTime(t.at, s.trig.at)
			t.keep = t.keep && s.trig.keep
			t.first = t.first || s.trig.first
		}
		s.log.verbosef("build %d cancelled by a newer change", s.gen)
	}
	s.gen++
	gen := s.gen
	ctx, cancel := context.WithCancel(context.Background())
	s.building, s.trig = cancel, &t
	s.inflight++
	if !t.first || s.cfg.opts.Verbose {
		s.log.printf("building…")
	}
	b := s.cfg.builder
	s.goSafe(func() {
		res := b.build(ctx, gen)
		s.push(evBuilt{res})
	})
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func (s *supervisor) built(res buildResult) {
	s.inflight--
	if s.quitting {
		res.remove()
		s.maybeFinish()
		return
	}
	if res.gen != s.gen {
		// Cancelled or superseded.
		if res.err == nil {
			res.remove()
		}
		return
	}
	t := *s.trig
	s.building, s.trig = nil, nil
	if res.err != nil {
		if res.isCancelled() {
			return
		}
		s.show(problem{kind: problemBuild, summary: "build failed", text: res.output, gen: res.gen})
		return
	}
	s.log.verbosef("build %d done in %v", res.gen, ms(res.end.Sub(res.start)))
	if s.cfg.refreshDeps != nil {
		f := s.cfg.refreshDeps
		s.goSafe(func() {
			d, err := f()
			s.push(evDeps{d, err})
		})
	}
	if s.ready != nil {
		s.ready.res.remove()
	}
	s.ready = &readyBuild{res: res, trig: t}
	s.maybeSwap()
}

// readyBuild is a successful build waiting for a swap.
type readyBuild struct {
	res  buildResult
	trig trigger
}

// --- swaps ---

func (s *supervisor) maybeSwap() {
	if s.swap != nil || s.ready == nil || s.quitting {
		return
	}
	rb := s.ready
	s.ready = nil
	sw := &swapState{res: rb.res, trig: rb.trig}
	s.swap = sw
	old := s.alive()
	if old == nil {
		sw.state = "fresh"
		if s.started && s.cfg.opts.Verbose {
			s.log.verbosef("no app is running: build %d starts fresh", rb.res.gen)
		}
		s.startChild(sw, nil)
		return
	}
	sw.frozen = true
	s.host.Post(s.host.Freeze)
	switch {
	case s.opts.NoState:
		sw.state = "off"
		s.startChild(sw, nil)
	case !rb.trig.keep:
		sw.state = "reset"
		s.startChild(sw, nil)
	case old.child == nil:
		sw.state = "not kept (the running app is not connected)"
		s.startChild(sw, nil)
	default:
		c, gen, timeout := old.child, rb.res.gen, s.cfg.saveTimeout
		s.goSafe(func() {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			data, err := c.Request(ctx, devloop.KindStateSave, nil)
			s.push(evSaved{gen, data, err})
		})
	}
}

// alive returns the active child if its process is running.
func (s *supervisor) alive() *proc {
	if s.active != nil && !isDone(s.active.exited) {
		return s.active
	}
	return nil
}

func (s *supervisor) saved(gen int, data []byte, err error) {
	sw := s.swap
	if sw == nil || sw.res.gen != gen || sw.proc != nil {
		return
	}
	sw.saved = time.Now()
	switch {
	case err == nil:
		sw.state = "kept"
	case errors.Is(err, remote.ErrNoHandler):
		sw.state = "not kept (the app does not save state: was it built without the teladev runtime?)"
	case errors.Is(err, context.DeadlineExceeded):
		sw.state = fmt.Sprintf("not kept (the app did not save it within %v)", s.cfg.saveTimeout)
	default:
		sw.state = fmt.Sprintf("not kept (%v)", err)
	}
	if err != nil {
		data = nil
	}
	s.startChild(sw, data)
}

// startChild starts the process of the swap's build, seeded with state.
func (s *supervisor) startChild(sw *swapState, state []byte) {
	res := sw.res
	tag := strconv.Itoa(res.gen)
	if state != nil {
		s.stateMu.Lock()
		s.states[tag] = state
		s.stateMu.Unlock()
	}
	cmd := exec.Command(res.exe, res.args...)
	cmd.Env = s.childEnv(tag, res.env)
	// Neither writer is an *os.File, so exec gives the child pipes and
	// copies them here, each stream in order: a child, in its own process
	// group, never writes to the terminal itself (ADR-119). Without a
	// writer for its standard output it gets the null device.
	tail := &tailWriter{w: s.cfg.stderr}
	cmd.Stderr = tail
	if s.cfg.stdout != nil {
		cmd.Stdout = forwardWriter{s.cfg.stdout}
	}
	// A process the child started and left running keeps the pipes open:
	// Wait stops copying a moment after the child exits instead of
	// waiting for it.
	cmd.WaitDelay = childWaitDelay
	cmd.SysProcAttr = childSysProcAttr()
	sw.started = time.Now()
	if err := cmd.Start(); err != nil {
		s.swapFailed(problem{kind: problemStart, summary: fmt.Sprintf("cannot start the new build: %v", err)})
		return
	}
	p := &proc{gen: res.gen, tag: tag, cmd: cmd, exe: res.exe, owned: res.owned, tail: tail, exited: make(chan struct{})}
	s.procs[p.gen] = p
	sw.proc = p
	s.goSafe(func() {
		err := cmd.Wait()
		if errors.Is(err, exec.ErrWaitDelay) {
			err = nil // exited with status 0; the pipes were closed after WaitDelay
		}
		p.waitErr = err
		close(p.exited)
		s.push(evExited{p})
	})
	gen := res.gen
	sw.timer = time.AfterFunc(s.cfg.firstFrameTimeout, func() { s.push(evTimeout{gen}) })
	s.log.verbosef("build %d started (pid %d)", gen, cmd.Process.Pid)
}

// childEnv is the environment of a child (ADR-115, package devloop).
func (s *supervisor) childEnv(tag string, extra []string) []string {
	drop := []string{remote.EnvAddr, remote.EnvTag, devloop.EnvChild, devloop.EnvNoState}
	var env []string
	devSet := false
	for _, kv := range s.cfg.env {
		k, _, _ := strings.Cut(kv, "=")
		if containsKey(drop, k) {
			continue
		}
		devSet = devSet || envKeyEqual(k, "TELA_DEV")
		env = append(env, kv)
	}
	env = append(env,
		remote.EnvAddr+"="+s.host.Addr(),
		remote.EnvTag+"="+tag,
		devloop.EnvChild+"=1",
	)
	if s.opts.NoState {
		env = append(env, devloop.EnvNoState+"=1")
	}
	if !devSet {
		env = append(env, "TELA_DEV=1")
	}
	return append(env, extra...)
}

// envKeyEqual compares environment variable names (case-insensitive on
// Windows).
func envKeyEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func containsKey(keys []string, k string) bool {
	for _, x := range keys {
		if envKeyEqual(x, k) {
			return true
		}
	}
	return false
}

func (s *supervisor) firstFrame(c *remote.Child) {
	p := s.procOf(c)
	if p == nil {
		return
	}
	sw := s.swap
	if sw == nil || sw.proc != p || sw.promoting {
		return
	}
	sw.promoting = true
	sw.first = time.Now()
	gen := p.gen
	h := s.host
	h.Post(func() {
		err := h.Promote(c)
		s.push(evPromoted{gen, err})
	})
}

func (s *supervisor) promoted(gen int, err error) {
	sw := s.swap
	if sw == nil || sw.res.gen != gen {
		return
	}
	if err != nil {
		s.swapFailed(problem{kind: problemStart, summary: fmt.Sprintf("the new build went away before it was shown (%v); %s", err, s.stays())})
		return
	}
	now := time.Now()
	sw.timer.Stop()
	old := s.active
	s.active = sw.proc
	s.swap = nil
	if old != nil && old != sw.proc {
		s.stop(old)
	}
	if s.problem {
		s.problem = false
		s.view.clear()
	}
	s.flushHeld()
	if sw.state == "kept" && sw.proc.report == nil {
		// The child reports what it kept a few frames after its first one:
		// the line (timed now) waits for it, a moment at most.
		gen := sw.res.gen
		s.held = &heldLine{sw: sw, at: now,
			timer: time.AfterFunc(s.cfg.reportWait, func() { s.push(evReportDue{gen}) })}
	} else {
		s.report(sw, now)
	}
	s.started = true
	s.maybeSwap()
}

// heldLine is the line of a finished swap that waits for the state report.
type heldLine struct {
	sw    *swapState
	at    time.Time
	timer *time.Timer
}

// flushHeld prints the held line, with the child's state counts if its
// report arrived.
func (s *supervisor) flushHeld() {
	h := s.held
	if h == nil {
		return
	}
	s.held = nil
	h.timer.Stop()
	s.report(h.sw, h.at)
}

// report prints the line of a finished swap.
func (s *supervisor) report(sw *swapState, now time.Time) {
	p := sw.proc
	verb := "reloaded"
	switch {
	case sw.trig.first && !s.started:
		verb = "started"
	case sw.state == "fresh":
		verb = "restarted"
	}
	total := now.Sub(sw.trig.at)
	var state string
	switch sw.state {
	case "kept":
		state = "state kept"
		if r := p.report; r != nil {
			state = fmt.Sprintf("state: %d kept, %d reset", r.Kept, r.Reset)
		}
	case "fresh":
		if verb != "started" {
			state = "fresh state"
		}
	case "reset":
		state = "state reset"
	case "off":
	default:
		state = "state " + sw.state
	}
	line := fmt.Sprintf("%s in %v", verb, ms(total))
	if state != "" {
		line += " (" + state + ")"
	}
	s.log.printf("%s", line)
	if s.cfg.opts.Verbose {
		var parts []string
		res := sw.res
		parts = append(parts, fmt.Sprintf("save→build start %v", ms(res.start.Sub(sw.trig.at))))
		parts = append(parts, fmt.Sprintf("build %v", ms(res.end.Sub(res.start))))
		if !sw.saved.IsZero() {
			parts = append(parts, fmt.Sprintf("state save %v", ms(sw.saved.Sub(res.end))))
		}
		parts = append(parts, fmt.Sprintf("start→first frame %v", ms(sw.first.Sub(sw.started))))
		parts = append(parts, fmt.Sprintf("promote %v", ms(now.Sub(sw.first))))
		s.log.verbosef("timing: %s", strings.Join(parts, ", "))
	}
}

// stays says what the window shows after a failed swap.
func (s *supervisor) stays() string {
	if s.alive() != nil {
		return "the previous build stays"
	}
	return "the window keeps the last frame"
}

// swapFailed abandons the swap in progress and shows why.
func (s *supervisor) swapFailed(prob problem) {
	sw := s.swap
	if sw == nil {
		return
	}
	s.swap = nil
	if sw.timer != nil {
		sw.timer.Stop()
	}
	if p := sw.proc; p != nil && !isDone(p.exited) {
		s.stop(p)
	}
	if sw.frozen {
		s.host.Post(s.host.Unfreeze)
	}
	s.stateMu.Lock()
	delete(s.states, strconv.Itoa(sw.res.gen))
	s.stateMu.Unlock()
	prob.gen = sw.res.gen
	s.show(prob)
	s.maybeSwap()
}

// --- processes ---

// stop ends a child that is no longer wanted: KindQuit, then after
// stopTimeout the connection is closed and the process killed.
func (s *supervisor) stop(p *proc) {
	if p.stopping {
		return
	}
	p.stopping = true
	c, timeout := p.child, s.cfg.stopTimeout
	s.goSafe(func() {
		if c != nil {
			c.Notify(devloop.KindQuit, nil)
		}
		select {
		case <-p.exited:
			return
		case <-time.After(timeout):
		}
		if c != nil {
			c.Close()
		}
		p.cmd.Process.Kill()
	})
}

func (s *supervisor) exited(p *proc) {
	if p.classified {
		return
	}
	p.classified = true
	delete(s.procs, p.gen)
	if p.lostConn != nil {
		p.lostConn.Stop()
	}
	if p.settle != nil {
		p.settle.Stop()
	}
	// Windows cannot delete a running executable: remove it now.
	if p.owned {
		os.Remove(p.exe)
	}
	status := describeExit(p.waitErr)
	if p.panic == "" && p.waitErr != nil && !p.stopping && p.tail != nil {
		// A panic in another goroutine, or a fatal error, which the child
		// runtime cannot report: the runtime printed it on standard error.
		p.panic = p.tail.crash()
	}
	switch {
	case p.stopping:
		s.log.verbosef("build %d stopped (%s)", p.gen, status)
	case s.swap != nil && s.swap.proc == p:
		if p.panic != "" {
			s.swapFailed(problem{kind: problemPanic, text: p.panic,
				summary: fmt.Sprintf("the new build panicked before its first frame: %s; %s", panicValue(p.panic), s.stays())})
		} else {
			s.swapFailed(problem{kind: problemStart, summary: fmt.Sprintf("the new build %s before its first frame; %s", status, s.stays())})
		}
	case p == s.active:
		s.active = nil
		if p.waitErr == nil && p.panic == "" {
			s.quit(0, "")
			break
		}
		if p.panic != "" {
			s.show(problem{kind: problemPanic, text: p.panic, gen: p.gen,
				summary: fmt.Sprintf("the app panicked: %s; save a change or press r to start it again", panicValue(p.panic))})
		} else {
			s.show(problem{kind: problemExit, gen: p.gen,
				summary: fmt.Sprintf("the app %s; save a change or press r to start it again", status)})
		}
	}
	s.maybeFinish()
}

// panicValue is the value of a panic payload ("index out of range").
func panicValue(payload string) string {
	return strings.TrimPrefix(devloop.PanicSummary([]byte(payload)), "panic: ")
}

// describeExit says how a process ended, from cmd.Wait's error.
func describeExit(err error) string {
	if err == nil {
		return "exited with status 0"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return fmt.Sprintf("exited with status %d", code)
		}
		return "was killed (" + ee.String() + ")"
	}
	return err.Error()
}

// --- quitting ---

// quit stops everything; msg, if any, is printed first.
func (s *supervisor) quit(code int, msg string) {
	if s.quitting {
		return
	}
	s.flushHeld()
	if msg != "" {
		s.log.printf("%s", msg)
	}
	s.quitting = true
	s.exitCode = code
	if s.building != nil {
		s.building()
		s.building = nil
	}
	if sw := s.swap; sw != nil && sw.timer != nil {
		sw.timer.Stop()
	}
	for _, p := range s.procs {
		s.stop(p)
	}
	s.quitByTime = time.AfterFunc(s.cfg.stopTimeout+2*time.Second, func() { s.push(evQuitTimeout{}) })
	s.maybeFinish()
}

// forceQuit kills the children and finishes at once (a second Ctrl+C, or
// children that do not die).
func (s *supervisor) forceQuit() {
	if !s.quitting {
		s.quit(0, "")
	}
	for _, p := range s.procs {
		p.cmd.Process.Kill()
	}
	s.finish()
}

// finish closes the host and ends its event loop; run returns once the
// loop has ended.
// maybeFinish finishes a quit once no child and no build is left.
func (s *supervisor) maybeFinish() {
	if s.quitting && len(s.procs) == 0 && s.inflight == 0 {
		s.finish()
	}
}

func (s *supervisor) finish() {
	if s.finished {
		return
	}
	s.finished = true
	if s.quitByTime != nil {
		s.quitByTime.Stop()
	}
	s.host.Close()
	if s.quitHost != nil {
		s.quitHost()
	}
}

// --- problems ---

// problemKind is what went wrong.
type problemKind int

const (
	problemBuild problemKind = iota + 1 // the build failed: text is the compiler output
	problemPanic                        // the app panicked: text is devloop's panic payload
	problemExit                         // the app exited on its own with an error
	problemStart                        // a new build could not take over: text says why
)

// problem is what the dev loop shows until the next good build.
type problem struct {
	kind problemKind
	// summary is one line for the terminal ("build failed", "the app
	// panicked: index out of range").
	summary string
	// text is the full text: the compiler output, or the panic with its
	// stack (empty when the summary says it all).
	text string
	gen  int
	// live: a child is alive and shown, so the window stays usable under
	// the problem (set by show).
	live bool
}

// problemView shows the dev loop's problems: the one place that receives
// "build failed", "panic" and "cleared". The terminal view prints them;
// T5 adds an overlay drawn over the last good frame (Host.SetOverlay),
// composed with this one. show is called on the supervisor goroutine; an
// overlay must hand its drawing to the host's UI thread with Host.Post.
type problemView interface {
	show(p problem)
	clear()
}

func (s *supervisor) show(p problem) {
	s.flushHeld()
	s.problem = true
	p.live = s.active != nil
	s.view.show(p)
}

// terminalView prints problems on the terminal.
type terminalView struct{ log *devLog }

func (v terminalView) show(p problem) {
	if p.kind == problemBuild {
		v.log.printf("%s:\n%s", p.summary, p.text)
		return
	}
	// A panic's trace is already on the terminal: the child printed it on
	// its standard error, which tela dev forwards.
	v.log.printf("%s", p.summary)
}

func (v terminalView) clear() {}

// --- output ---

// devLog prints the supervisor's lines.
type devLog struct {
	mu      sync.Mutex
	w       io.Writer
	verbose bool
}

func (l *devLog) printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "tela dev: "+format+"\n", args...)
}

func (l *devLog) verbosef(format string, args ...any) {
	if l.verbose {
		l.printf(format, args...)
	}
}

// ms rounds a duration for the log.
func ms(d time.Duration) time.Duration {
	switch {
	case d >= 10*time.Second:
		return d.Round(100 * time.Millisecond)
	case d >= 10*time.Millisecond:
		return d.Round(time.Millisecond)
	}
	return d.Round(10 * time.Microsecond)
}

func isDone(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// --- the event queue ---

// queue is an unbounded FIFO: pushes never block (the host's UI thread
// pushes), pops wait.
type queue struct {
	mu     sync.Mutex
	items  []any
	signal chan struct{}
}

func (q *queue) init() { q.signal = make(chan struct{}, 1) }

func (q *queue) push(e any) {
	q.mu.Lock()
	q.items = append(q.items, e)
	q.mu.Unlock()
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

func (q *queue) pop() any {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			e := q.items[0]
			q.items[0] = nil
			q.items = q.items[1:]
			q.mu.Unlock()
			return e
		}
		q.mu.Unlock()
		<-q.signal
	}
}
