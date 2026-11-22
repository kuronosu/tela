package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/semantics"
)

// tela mcp --attach (ADR-299): the MCP server of mcp.go, driving the real
// windows of a running tela dev session through its tools socket
// (dev_tools.go) instead of a headless instance of the package. The tools
// are the headless ones plus a window argument and windows; this file is
// the session (the connection, the windows, the refs, the wait for the app
// to settle) and mcp_attach_tools.go the tools.
//
// What differs from the headless server, by design:
//
//   - The app, its clock and its window belong to the person at the desktop
//     and to tela dev: there is no rebuild on the next call (tela dev
//     rebuilds on save and tells the session), no resize, no open and no
//     prefer, and wait waits real time.
//   - An acting tool injects pointer, key and text events at real
//     coordinates (the element's painted bounds), the way a person's input
//     takes, waits until the app has settled (no frame for settleQuiet, at
//     most settleMax), and answers with the new snapshot.
//   - Refs are the semantics node IDs of the latest snapshot of a window,
//     as in the headless server (ADR-300), kept per window; after a reload
//     (a new child: IDs start again) they are matched again by role, name
//     and position.

// attachConn is what the session uses of a tools client; toolsClient
// implements it and the tests fake it.
type attachConn interface {
	Info(ctx context.Context) (toolsInfo, error)
	Windows(ctx context.Context) ([]toolsWindow, error)
	Element(ctx context.Context, window uint32, id core.ElementID) (core.ElementInfo, error)
	Elements(ctx context.Context, window uint32, detail bool) ([]core.ElementInfo, error)
	Inspect(ctx context.Context, window uint32, id core.ElementID) (core.InspectNode, error)
	Semantics(ctx context.Context, window uint32) (toolsSemantics, error)
	Profiles(ctx context.Context, window uint32, since int) ([]core.FrameProfile, error)
	Logs(ctx context.Context, since uint64) ([]toolsLogEntry, uint64, error)
	Screenshot(ctx context.Context, window uint32) (image.Image, error)
	Hit(ctx context.Context, window uint32, x, y float32, target core.ElementID) (toolsHit, error)
	Reveal(ctx context.Context, window uint32, id core.ElementID) (toolsReveal, error)
	Pointer(ctx context.Context, window uint32, action string, x, y float32, button string) error
	Click(ctx context.Context, window uint32, x, y float32) error
	Clicks(ctx context.Context, window uint32, x, y float32, button string, count int) error
	Scroll(ctx context.Context, window uint32, x, y, dx, dy float32) error
	Key(ctx context.Context, window uint32, combo, action string) error
	Text(ctx context.Context, window uint32, text string) error
	Done() <-chan struct{}
	Close() error
}

// How long the session waits for the app after an action.
const (
	// settleQuiet is the time without a new frame after which the app
	// counts as settled; settleFirst is how long it waits for the first
	// frame an action causes (a click on text draws none); settleMax
	// bounds the wait for an app that keeps drawing (an animation).
	settleQuiet = 100 * time.Millisecond
	settleFirst = 300 * time.Millisecond
	settleMax   = 5 * time.Second
	settlePoll  = 15 * time.Millisecond
	// attachWaitText bounds wait's wait for a text.
	attachWaitText = 10 * time.Second
	// attachWaitMax bounds wait's ms.
	attachWaitMax = 60 * time.Second
)

// attachWin is what the session keeps per window.
type attachWin struct {
	refs *refTable
	// cursor is the last frame seen (the app's frame numbers start again
	// with a reload); action are the frames the last acting call caused.
	cursor int
	action []core.FrameProfile
}

// attachSession is the mcpTools of tela mcp --attach. Its tool calls run
// one at a time, with mu held.
type attachSession struct {
	addr string
	dial func(addr string) (attachConn, error)
	logf func(string, ...any)

	quiet, first, max, poll time.Duration // settle*, replaceable in tests

	mu      sync.Mutex
	conn    attachConn
	pid     int // of the session the state below belongs to
	build   int
	info    toolsInfo
	wins    map[uint32]*attachWin
	nextRef int
	pending []string // notes for the next result
	// logSeen is the last log entry a result reported; logRead the last
	// one the logs tool listed.
	logSeen, logRead uint64
	logsInit         bool
}

func newAttachSession(addr string, logf func(string, ...any)) *attachSession {
	return &attachSession{
		addr: addr, logf: logf,
		dial: func(addr string) (attachConn, error) {
			c, err := dialTools(addr)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		quiet: settleQuiet, first: settleFirst, max: settleMax, poll: settlePoll,
		wins: map[uint32]*attachWin{},
	}
}

// attachFlag is the --attach flag: bare, or with the address of the
// session (--attach=ADDR); the address may also follow as the positional
// argument.
type attachFlag struct {
	on   *bool
	addr *string
}

func (f attachFlag) IsBoolFlag() bool { return true }

func (f attachFlag) String() string {
	if f.on == nil || !*f.on {
		return "false"
	}
	return "true"
}

func (f attachFlag) Set(v string) error {
	switch v {
	case "", "true", "1":
		*f.on = true
	case "false", "0":
		*f.on = false
	default:
		*f.on, *f.addr = true, v
	}
	return nil
}

var _ flag.Value = attachFlag{}

// serveAttach serves one client on in and out until in ends or ctx is
// done, as serveMCP does for the headless server.
func serveAttach(ctx context.Context, opts mcpOptions, in io.Reader, out, stderr io.Writer) error {
	logf := func(string, ...any) {}
	if opts.Verbose {
		var mu sync.Mutex
		logf = func(format string, args ...any) {
			mu.Lock()
			fmt.Fprintf(stderr, "tela mcp: "+format+"\n", args...)
			mu.Unlock()
		}
	}
	s := newAttachSession(opts.Addr, logf)
	defer s.close()
	// The server starts without a session too, as a client may start it
	// before tela dev: each call looks again. Say why none is attached.
	if c, err := s.dial(s.addr); err != nil {
		fmt.Fprintf(stderr, "tela mcp --attach: %v\n(the tools answer with this until a session runs)\n", err)
	} else {
		c.Close()
	}
	srv := &rpcServer{tools: s, name: "tela", version: mcpServerVersion(), logf: logf, out: out}
	return srv.serve(ctx, in)
}

func (s *attachSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

// win returns the state of window id.
func (s *attachSession) win(id uint32) *attachWin {
	w := s.wins[id]
	if w == nil {
		w = &attachWin{refs: newRefTable()}
		s.wins[id] = w
	}
	return w
}

// connect makes sure a connection to the session is up, looking for the
// session again when the last one ended.
func (s *attachSession) connect() error {
	if s.conn != nil {
		select {
		case <-s.conn.Done():
			s.conn.Close()
			s.conn = nil
			s.pending = append(s.pending, "The connection to tela dev ended; this is a new one.")
		default:
			return nil
		}
	}
	c, err := s.dial(s.addr)
	if err != nil {
		return err
	}
	s.conn = c
	return nil
}

// begin starts a call: it connects, asks the session about the app and,
// when tela dev built it again (a reload: a new child, whose element IDs
// start again) or a new session answers, makes the refs be matched again.
// ok is false when the call cannot go on (o holds the error).
func (s *attachSession) begin(ctx context.Context, o *mcpOut) bool {
	var info toolsInfo
	for try := 0; ; try++ {
		if err := s.connect(); err != nil {
			o.fail(err.Error())
			return false
		}
		var err error
		if info, err = s.conn.Info(ctx); err == nil {
			break
		}
		if ctx.Err() != nil {
			o.fail(ctx.Err().Error())
			return false
		}
		// The connection broke: look for the session once more.
		s.conn.Close()
		s.conn = nil
		if try > 0 {
			o.fail(err.Error())
			return false
		}
	}
	switch {
	case s.pid != 0 && info.PID != s.pid:
		s.pending = append(s.pending, fmt.Sprintf("Attached to another tela dev session (%s, pid %d): the windows and refs of the earlier one are gone.", info.Pkg, info.PID))
		s.reset()
	case s.build != 0 && info.Build != s.build:
		s.pending = append(s.pending, fmt.Sprintf("tela dev built the app again (build %d): the app restarted with the state tela dev kept; refs are matched again by role and name.", info.Build))
		s.remount()
	}
	s.pid, s.build, s.info = info.PID, info.Build, info
	o.notes = append(o.notes, s.pending...)
	s.pending = nil
	if !s.logsInit {
		// What the session logged before this client came is not news.
		if _, last, err := s.conn.Logs(ctx, ^uint64(0)); err == nil {
			s.logSeen, s.logRead, s.logsInit = last, last, true
		}
	}
	return true
}

// remount records a reload: the refs of every window are checked again
// against its next snapshot, and the frame numbers start again.
func (s *attachSession) remount() {
	for _, w := range s.wins {
		w.refs.remount()
		w.cursor, w.action = 0, nil
	}
}

// reset forgets everything a session told.
func (s *attachSession) reset() {
	s.wins = map[uint32]*attachWin{}
	s.logsInit = false
}

// window returns the window id (0: the first one the app shows).
func (s *attachSession) window(ctx context.Context, id uint32) (toolsWindow, error) {
	ws, err := s.conn.Windows(ctx)
	if err != nil {
		return toolsWindow{}, err
	}
	return pickWindow(ws, id)
}

// pickWindow chooses window id among ws; 0 is the first one shown by the
// running app.
func pickWindow(ws []toolsWindow, id uint32) (toolsWindow, error) {
	for _, w := range ws {
		if w.Active && (id == 0 || w.ID == id) {
			return w, nil
		}
	}
	if id == 0 {
		if len(ws) > 0 {
			return toolsWindow{}, errors.New("the app shows no window now (it crashed, or is starting: its last frame is window " + fmt.Sprint(ws[0].ID) + "); save a fix and tela dev starts it again")
		}
		return toolsWindow{}, errors.New("no window is open yet; tela dev is still starting the app, or it crashed")
	}
	for _, w := range ws {
		if w.ID == id {
			return toolsWindow{}, fmt.Errorf("window %d is not shown by the running app now (the last frame of a build that crashed); windows lists the open ones", id)
		}
	}
	var ids []string
	for _, w := range ws {
		if w.Active {
			ids = append(ids, fmt.Sprint(w.ID))
		}
	}
	return toolsWindow{}, fmt.Errorf("no window %d; the app shows %s (windows lists them)", id, strings.Join(ids, ", "))
}

// describeWindow is the line about the window a result is of.
func describeWindow(w toolsWindow) string {
	s := fmt.Sprintf("Window %d %q: %gx%g logical pixels at scale %g", w.ID, w.Title, round2(float64(w.Width)), round2(float64(w.Height)), w.Scale)
	if !w.Focused {
		s += ", without the keyboard focus (key and text input gives it the focus as the app sees it)"
	}
	return s
}

// ---------------------------------------------------------------------
// Snapshots and refs.

// attachLine is a line of a snapshot: the element it prints, if any.
type attachLine struct {
	parsed semantics.SnapshotLine
	ok     bool // an element with a role
	text   string
	node   uint64
	el     core.ElementID
}

// attachSnap is a snapshot of a window.
type attachSnap struct {
	win   toolsWindow
	yaml  string // with the refs
	lines []attachLine
	byRef map[string]*attachLine
}

// snapshot takes the aria snapshot of window w and gives refs to its
// elements.
func (s *attachSession) snapshot(ctx context.Context, w toolsWindow) (*attachSnap, error) {
	sem, err := s.conn.Semantics(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	yaml := strings.TrimRight(sem.YAML, "\n")
	var lines []string
	if yaml != "" {
		lines = strings.Split(yaml, "\n")
	}
	snap := &attachSnap{win: w, byRef: map[string]*attachLine{}}
	if len(sem.Nodes) != len(lines) || len(sem.Elements) != len(lines) {
		// An app of a tela that gives no node IDs: the text, no refs.
		snap.yaml = yaml
		for _, l := range lines {
			snap.lines = append(snap.lines, attachLine{text: l})
		}
		return snap, nil
	}
	if len(sem.Srcs) == len(lines) {
		for i, src := range sem.Srcs {
			if l, ok := semantics.ParseSnapshotLine(lines[i]); ok && src != "" {
				l.Src = src
				lines[i] = l.WithSuffix("")
			}
		}
		yaml = strings.Join(lines, "\n")
	}
	aw := s.win(w.ID)
	plines, cands, _, _ := prepareRefs(yaml)
	aw.refs.next = s.nextRef
	snap.yaml = aw.refs.assignNodes(plines, cands, sem.Nodes)
	s.nextRef = aw.refs.next
	if snap.yaml != "" {
		for i, l := range strings.Split(snap.yaml, "\n") {
			al := attachLine{text: l, node: sem.Nodes[i], el: sem.Elements[i]}
			al.parsed, al.ok = semantics.ParseSnapshotLine(l)
			snap.lines = append(snap.lines, al)
		}
		for i := range snap.lines {
			if ref, ok := aw.refs.byNode[snap.lines[i].node]; ok && snap.lines[i].node != 0 {
				snap.byRef[ref] = &snap.lines[i]
			}
		}
	}
	return snap, nil
}

// findRef returns the window whose refs include ref, looking in window
// first (the one the call is about).
func (s *attachSession) findRef(ref string, first uint32) (uint32, bool) {
	ref = strings.TrimSpace(ref)
	if w := s.wins[first]; w != nil && w.refs.issued[ref] {
		return first, true
	}
	ids := make([]uint32, 0, len(s.wins))
	for id := range s.wins {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if s.wins[id].refs.issued[ref] {
			return id, true
		}
	}
	return 0, false
}

// ---------------------------------------------------------------------
// Settling.

// frameCursor reads the frames drawn since the last call, so that the ones
// an action causes are those after it.
func (s *attachSession) frameCursor(ctx context.Context, win uint32) int {
	aw := s.win(win)
	if ps, err := s.conn.Profiles(ctx, win, aw.cursor); err == nil && len(ps) > 0 {
		aw.cursor = ps[len(ps)-1].Frame
	}
	return aw.cursor
}

// settle waits until the app has drawn no frame for s.quiet (s.first for
// the first one), at most s.max, and returns the frames drawn since
// cursor. settled is false when the app still drew when the time was up.
func (s *attachSession) settle(ctx context.Context, win uint32, from time.Time) (frames []core.FrameProfile, settled bool) {
	aw := s.win(win)
	last, got := from, false
	for {
		ps, err := s.conn.Profiles(ctx, win, aw.cursor)
		if err != nil {
			break // the next call says what is wrong
		}
		if len(ps) > 0 {
			frames = append(frames, ps...)
			aw.cursor = ps[len(ps)-1].Frame
			last, got = time.Now(), true
		}
		now := time.Now()
		wait := s.quiet
		if !got {
			wait = s.first
		}
		if now.Sub(last) >= wait {
			settled = true
			break
		}
		if now.Sub(from) >= s.max {
			break
		}
		select {
		case <-ctx.Done():
			return frames, true
		case <-time.After(s.poll):
		}
	}
	return frames, settled
}

// ---------------------------------------------------------------------
// Logs.

// collectLogs adds what the app logged since the last result to o: the dev
// warnings and errors of Tela, and the lines of the app's standard error
// that carry a level above info.
func (s *attachSession) collectLogs(ctx context.Context, o *mcpOut) {
	es, last, err := s.conn.Logs(ctx, s.logSeen)
	if err != nil {
		return
	}
	s.logSeen = max(s.logSeen, last)
	for _, e := range es {
		switch {
		case e.Source == "app" && e.Level >= 8:
			o.errs = appendNew(o.errs, e.Msg)
		case e.Source == "app" && e.Level >= 4:
			o.warns = appendNew(o.warns, e.Msg)
		case e.Level >= 4:
			o.other = appendNew(o.other, "stderr: "+e.Msg)
		}
	}
}

// instructions implements mcpTools.
func (s *attachSession) instructions() string {
	return `This server drives the REAL windows of a running tela dev session, through its tools socket: the app, its state and its clock are the person's at the desktop, and tela dev rebuilds it when the source is saved (the next result says so; refs are matched again by role and name). Tela is a pure-Go UI framework: React-like components (tela.UseState, h.Div, h.Button...) styled with Tailwind CSS v4 classes.
Start with windows (the IDs; every tool takes an optional window, default the first the app shows), then snapshot (structure and [ref=eN] of each element you can act on, with the [src=file:line] that made it) or screenshot (the presented frame: real pixels at the display's scale, the answer says it). click, hover, fill, press, scroll and drag inject real pointer, key and text events at the element's painted position, wait until the app has stopped drawing, and answer with the new snapshot. inspect, state, perf and logs read the running app: the layout tree, why a component rendered and what its hooks hold, the frames' time per phase, and what was logged.
Not available while attached: resize, open and prefer (the window and the system's preferences are the person's; use headless tela mcp for those), and wait advances nothing: it waits real time, or until a text appears. A person may also be using the window: the app can change between two calls.`
}
