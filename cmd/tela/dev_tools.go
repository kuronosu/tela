package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"net"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/devloop"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/keycombo"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// The tools socket of tela dev (ADR-299): what tela devtools and tela mcp
// --attach use to inspect and drive the running app. Clients speak JSON-RPC
// 2.0 with one message per line (the framing of tela mcp, mcp_rpc.go),
// several at once; each client's requests are answered in order. Where the
// socket is and how a client finds it: dev_tools_discover.go.
//
// Window-level calls are answered here: windows, screenshot (the frame the
// window presents, Host.Frame) and input, which is injected into the
// remote host as events of the real window (Host.HandleEvent on its UI
// thread, stamped with the backend's clock), so it takes the path a
// person's input takes: the error overlay, the focus and held-key views,
// the swap's input queue, hover. Calls about a window's tree are relayed
// to the active child with the devloop.KindTools* kinds, answered by the
// child runtime (internal/core/devrun_tools.go) on its UI thread; their
// results are passed through as the child encoded them.
//
// The instrumentation (internal/core/instrument.go) is on in the active
// child while a client holds it: from its first call about a tree, its
// instrument call or its subscription to picked, frames or logs, until it
// disconnects or calls instrument with on false. After a reload (a new
// child took over: element IDs start again), it is turned on in the new
// child.
//
// Methods (params → result; window is a window ID of windows, 0 or absent
// for the first window):
//
//	info                                  → {pkg, pid, started, build, protocol}
//	windows                               → {windows: [{id, title, width, height, scale, focused, visible, active}]}
//	subscribe     {events: [...]}         → {events}   picked, frames, logs, reload, windows
//	unsubscribe   {events: [...]}         → {events}   the ones still subscribed
//	instrument    {on}                    → {on}
//	elements      {window, detail}        → {elements: [ElementInfo]}
//	element       {window, id}            → ElementInfo
//	inspect       {window, id}            → InspectNode (id 0: the whole tree)
//	semantics     {window}                → {yaml, nodes, elements, srcs}
//	profiles      {window, since}         → {profiles: [FrameProfile]}
//	logs          {since}                 → {entries: [{seq, at, level, msg, source, window, build}], last}
//	pick          {window, on}            → {}         subscribes the caller to picked
//	highlight     {window, id}            → {}
//	screenshot    {window}                → {window, width, height, png}  (base64 PNG at the app's resolution)
//	pointer       {window, action, x, y, button, mods, count} → {}
//	scroll        {window, x, y, dx, dy, mods} → {}
//	key           {window, key, action}   → {}
//	text          {window, text}          → {}
//
// Notifications (to the clients subscribed): picked {window, id, element},
// frames {window, profiles}, logs {entries}, reload {build}, windows
// {windows}.
//
// What cannot be injected: IME composition (text commits are typed as
// keys), the OS's own menus, dialogs and notifications (the host's
// backend shows them; a tela.Menu popup or a kit dialog is the app's and
// takes input like the rest), drags between applications, and the focus
// of the real window: a key or text call on a window without the keyboard
// focus gives it the focus as the app sees it (a FocusEvent), not the OS's.

// toolsProtocol is the version info returns; it grows with the methods.
const toolsProtocol = 1

// toolsTimeout bounds a relayed request and a wait for the host's UI
// thread.
const toolsTimeout = 10 * time.Second

// toolsQueue bounds the messages waiting to be written to a client; a
// notification that finds it full is dropped.
const toolsQueue = 256

// toolsEvents are the notifications a client can subscribe to.
var toolsEvents = []string{"picked", "frames", "logs", "reload", "windows"}

// errNoApp is the error of a call about a tree while no app runs.
var errNoApp = errors.New("no app is running (it is starting, or it crashed: save a change or press r)")

// toolsServer is the tools socket of a supervisor.
type toolsServer struct {
	pkg  string
	dir  string // the directory of the discovery file
	logf func(format string, args ...any)
	log  toolsLog

	host    *remote.Host
	ln      net.Listener
	addr    string
	cleanup func()
	file    string
	started time.Time

	mu      sync.Mutex
	conns   map[*toolsConn]struct{}
	active  *remote.Child
	build   int
	windows []toolsWindow // as last told to the clients
	pending []toolsLogEntry
	armed   bool // a flush of pending runs
	closed  bool

	// insMu orders the instrument requests; instrumented is the child
	// the instrumentation is on in.
	insMu        sync.Mutex
	instrumented *remote.Child

	// UI thread: the buttons held by injected pointer events, per window.
	buttons map[platform.Window]platform.Buttons
}

// toolsConn is a client of the socket.
type toolsConn struct {
	ts   *toolsServer
	nc   net.Conn
	out  chan []byte
	done chan struct{}
	// Under ts.mu.
	subs  map[string]bool
	holds bool
}

// toolsWindow is a window as the clients see it.
type toolsWindow struct {
	ID      uint32  `json:"id"`
	Title   string  `json:"title"`
	Width   float32 `json:"width"`
	Height  float32 `json:"height"`
	Scale   float64 `json:"scale"`
	Focused bool    `json:"focused"`
	Visible bool    `json:"visible"`
	// Active: the running app shows it (false: the last frame of a build
	// that crashed).
	Active bool `json:"active"`
}

func newToolsServer(pkg, dir string, logf func(string, ...any)) *toolsServer {
	ts := &toolsServer{pkg: pkg, dir: dir, logf: logf, conns: map[*toolsConn]struct{}{}, buttons: map[platform.Window]platform.Buttons{}}
	ts.log.added = ts.logAdded
	return ts
}

// start listens, writes the discovery file and registers the child's
// notifications on h. Call it before h's loop runs.
func (ts *toolsServer) start(h *remote.Host) error {
	ts.host = h
	h.Handle(devloop.KindToolsPicked, func(c *remote.Child, p []byte) ([]byte, error) {
		ts.fromChild(c, "picked", p)
		return nil, nil
	})
	h.Handle(devloop.KindToolsFrames, func(c *remote.Child, p []byte) ([]byte, error) {
		ts.fromChild(c, "frames", p)
		return nil, nil
	})
	h.Handle(devloop.KindToolsLogs, func(c *remote.Child, p []byte) ([]byte, error) {
		if ts.isActive(c) {
			var b devloop.ToolsLogBatch
			if json.Unmarshal(p, &b) == nil {
				ts.appLogs([]devloop.ToolsLogBatch{b})
			}
		}
		return nil, nil
	})
	ln, addr, cleanup, err := toolsListen()
	if err != nil {
		return err
	}
	ts.ln, ts.addr, ts.cleanup, ts.started = ln, addr, cleanup, time.Now()
	dir := ts.dir
	if dir == "" {
		if dir, err = devSessionDir(); err != nil {
			ln.Close()
			cleanup()
			return err
		}
	}
	file, err := writeDevSession(dir, devSession{Addr: addr, Pkg: ts.pkg, PID: os.Getpid(), Started: ts.started})
	if err != nil {
		ln.Close()
		cleanup()
		return fmt.Errorf("discovery file: %w", err)
	}
	ts.file = file
	go ts.accept()
	return nil
}

// close stops listening, removes the discovery file and the socket and
// disconnects the clients. Safe from any goroutine; idempotent.
func (ts *toolsServer) close() {
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		return
	}
	ts.closed = true
	conns := make([]*toolsConn, 0, len(ts.conns))
	for c := range ts.conns {
		conns = append(conns, c)
	}
	ts.mu.Unlock()
	if ts.ln != nil {
		ts.ln.Close()
		os.Remove(ts.file)
		ts.cleanup()
	}
	for _, c := range conns {
		c.nc.Close()
	}
}

func (ts *toolsServer) accept() {
	for {
		nc, err := ts.ln.Accept()
		if err != nil {
			return
		}
		c := &toolsConn{ts: ts, nc: nc, out: make(chan []byte, toolsQueue), done: make(chan struct{}), subs: map[string]bool{}}
		ts.mu.Lock()
		if ts.closed {
			ts.mu.Unlock()
			nc.Close()
			return
		}
		ts.conns[c] = struct{}{}
		ts.mu.Unlock()
		go c.write()
		go c.serve()
	}
}

// --- the supervisor's side ---

// setActive tells the server the active child (nil when none runs) and
// its build generation, after a promotion or the active child's exit. A
// new child is a reload for the clients, whose element IDs start again.
func (ts *toolsServer) setActive(c *remote.Child, build int) {
	ts.mu.Lock()
	changed := ts.active != c
	ts.active, ts.build = c, build
	ts.mu.Unlock()
	if !changed {
		return
	}
	go ts.syncInstrument()
	if c != nil {
		ts.broadcast("reload", map[string]any{"build": build})
	}
}

func (ts *toolsServer) isActive(c *remote.Child) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return c != nil && c == ts.active
}

// windowsChanged runs on the host's UI thread when its windows changed
// (HostOptions.OnWindows): the clients are told when what they see
// changed.
func (ts *toolsServer) windowsChanged() {
	if ts.host == nil {
		return
	}
	ws := toolsWindows(ts.host.WindowInfos())
	ts.mu.Lock()
	same := slices.Equal(ws, ts.windows)
	ts.windows = ws
	ts.mu.Unlock()
	if !same {
		ts.broadcast("windows", map[string]any{"windows": ws})
	}
}

// toolsWindows lists the windows a child window was ever shown on.
func toolsWindows(infos []remote.WindowInfo) []toolsWindow {
	ws := []toolsWindow{}
	for _, wi := range infos {
		if wi.ID == 0 {
			continue
		}
		ws = append(ws, toolsWindow{ID: wi.ID, Title: wi.Title, Width: wi.Metrics.Size.W, Height: wi.Metrics.Size.H,
			Scale: wi.Metrics.Scale, Focused: wi.Focused, Visible: wi.Visible, Active: wi.Active})
	}
	return ws
}

// fromChild forwards a notification of the active child to the clients.
func (ts *toolsServer) fromChild(c *remote.Child, event string, payload []byte) {
	if !ts.isActive(c) || !json.Valid(payload) {
		return
	}
	ts.broadcast(event, json.RawMessage(payload))
}

// appLogs adds the log entries of the child's trees to the log.
func (ts *toolsServer) appLogs(bs []devloop.ToolsLogBatch) {
	ts.mu.Lock()
	build := ts.build
	ts.mu.Unlock()
	var es []toolsLogEntry
	for _, b := range bs {
		for _, raw := range b.Entries {
			var e struct {
				At    time.Time  `json:"at"`
				Level slog.Level `json:"level"`
				Msg   string     `json:"msg"`
			}
			if json.Unmarshal(raw, &e) == nil {
				es = append(es, toolsLogEntry{At: e.At, Level: e.Level, Msg: e.Msg, Source: "app", Window: b.Window, Build: build})
			}
		}
	}
	ts.log.add(es)
}

// logAdded batches the log entries for the clients subscribed to logs.
func (ts *toolsServer) logAdded(es []toolsLogEntry) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.pending = append(ts.pending, es...)
	if len(ts.pending) > toolsLogMax {
		ts.pending = ts.pending[len(ts.pending)-toolsLogMax:]
	}
	if ts.armed {
		return
	}
	ts.armed = true
	time.AfterFunc(devloop.ToolsBatch, func() {
		ts.mu.Lock()
		es := ts.pending
		ts.pending, ts.armed = nil, false
		ts.mu.Unlock()
		if len(es) > 0 {
			ts.broadcast("logs", map[string]any{"entries": es})
		}
	})
}

// --- instrumentation ---

// holders reports whether a client holds the instrumentation. ts.mu is
// held.
func (ts *toolsServer) holdersLocked() bool {
	for c := range ts.conns {
		if c.holds {
			return true
		}
	}
	return false
}

// hold makes c hold the instrumentation.
func (ts *toolsServer) hold(c *toolsConn, on bool) {
	ts.mu.Lock()
	c.holds = on
	ts.mu.Unlock()
}

// syncInstrument turns the instrumentation on in the active child while a
// client holds it, and off once none does. The error is the child's
// refusal to turn it on.
func (ts *toolsServer) syncInstrument() error {
	ts.insMu.Lock()
	defer ts.insMu.Unlock()
	ts.mu.Lock()
	child, want := ts.active, ts.holdersLocked() && !ts.closed
	ts.mu.Unlock()
	if ts.instrumented != nil && (!want || ts.instrumented != child) {
		old := ts.instrumented
		ts.instrumented = nil
		if old == child {
			ts.instrument(old, false) // the old child of a reload is stopping
		}
	}
	if want && child != nil && ts.instrumented != child {
		if err := ts.instrument(child, true); err != nil {
			ts.logf("tools: cannot turn the instrumentation on: %v", err)
			return err
		}
		ts.instrumented = child
	}
	return nil
}

// instrument sends KindToolsInstrument to c.
func (ts *toolsServer) instrument(c *remote.Child, on bool) error {
	p, _ := json.Marshal(devloop.ToolsParams{On: on})
	ctx, cancel := context.WithTimeout(context.Background(), toolsTimeout)
	defer cancel()
	_, err := c.Request(ctx, devloop.KindToolsInstrument, p)
	return childError(err)
}

// relay sends a request about a tree to the active child, with the
// instrumentation on, and returns its reply.
func (ts *toolsServer) relay(c *toolsConn, kind string, p devloop.ToolsParams) (json.RawMessage, error) {
	ts.hold(c, true)
	if err := ts.syncInstrument(); err != nil {
		return nil, err
	}
	ts.insMu.Lock()
	child := ts.instrumented
	ts.insMu.Unlock()
	if child == nil || !ts.isActive(child) {
		return nil, errNoApp
	}
	payload, _ := json.Marshal(p)
	ctx, cancel := context.WithTimeout(context.Background(), toolsTimeout)
	defer cancel()
	b, err := child.Request(ctx, kind, payload)
	if err != nil {
		return nil, childError(err)
	}
	if len(b) == 0 || !json.Valid(b) {
		return nil, errors.New("the app sent an invalid reply")
	}
	return json.RawMessage(b), nil
}

// childError makes an error of a child's request readable for a client.
func childError(err error) error {
	var re *remote.RemoteError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, remote.ErrNoHandler):
		return errors.New("the app does not answer DevTools requests: was it built with an older Tela?")
	case errors.As(err, &re):
		return errors.New(re.Msg)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("the app did not answer within %v", toolsTimeout)
	}
	return err
}

// --- the host's UI thread ---

// onUI runs fn on the host's UI thread and waits for it. After a timeout
// fn may still run: it must not write what the caller reads then.
func (ts *toolsServer) onUI(fn func()) error {
	done := make(chan struct{})
	ts.host.Post(func() {
		fn()
		close(done)
	})
	select {
	case <-done:
		return nil
	case <-time.After(toolsTimeout):
		return fmt.Errorf("the window's event loop did not answer within %v", toolsTimeout)
	}
}

// findWindow returns the real window of ID id (0: the first one): the one
// the active child shows first, else the last frame of a crashed build.
func findWindow(infos []remote.WindowInfo, id uint32) (remote.WindowInfo, error) {
	for _, active := range []bool{true, false} {
		for _, wi := range infos {
			if wi.ID != 0 && wi.Active == active && (id == 0 || wi.ID == id) {
				return wi, nil
			}
		}
	}
	if id == 0 {
		return remote.WindowInfo{}, errors.New("no window is open")
	}
	return remote.WindowInfo{}, fmt.Errorf("no window %d; windows lists them", id)
}

// inject builds events for window id on the UI thread and hands them to
// the host as the real window's.
func (ts *toolsServer) inject(id uint32, build func(wi remote.WindowInfo, now time.Duration) ([]platform.Event, error)) error {
	var err error
	if uerr := ts.onUI(func() {
		wi, ferr := findWindow(ts.host.WindowInfos(), id)
		if ferr != nil {
			err = ferr
			return
		}
		evs, berr := build(wi, ts.host.Backend().Now())
		if berr != nil {
			err = berr
			return
		}
		for _, e := range evs {
			ts.host.HandleEvent(wi.Window, e)
		}
	}); uerr != nil {
		return uerr
	}
	return err
}

// --- clients ---

// write writes the client's queue to its connection.
func (c *toolsConn) write() {
	for {
		select {
		case b := <-c.out:
			if _, err := c.nc.Write(b); err != nil {
				c.nc.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// send queues a message; a response waits for room, a notification is
// dropped when the queue is full.
func (c *toolsConn) send(b []byte, wait bool) {
	if wait {
		select {
		case c.out <- b:
		case <-c.done:
		}
		return
	}
	select {
	case c.out <- b:
	default:
	}
}

// broadcast sends a notification to the clients subscribed to event.
func (ts *toolsServer) broadcast(event string, params any) {
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": event, "params": params})
	if err != nil {
		return
	}
	b = append(b, '\n')
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for c := range ts.conns {
		if c.subs[event] {
			c.send(b, false)
		}
	}
}

// serve reads the client's requests and answers them in order.
func (c *toolsConn) serve() {
	ts := c.ts
	defer func() {
		ts.mu.Lock()
		delete(ts.conns, c)
		ts.mu.Unlock()
		close(c.done)
		c.nc.Close()
		go ts.syncInstrument()
	}()
	sc := bufio.NewScanner(c.nc)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if r := c.handle(line); r != nil {
			r.JSONRPC = "2.0"
			b, err := json.Marshal(r)
			if err != nil {
				b, _ = json.Marshal(rpcResponse{JSONRPC: "2.0", ID: r.ID, Error: &rpcError{Code: rpcInternalError, Message: "internal error: " + err.Error()}})
			}
			c.send(append(b, '\n'), true)
		}
	}
}

// toolsErr is the error of a call: a JSON-RPC error code and its message.
type toolsErr struct {
	code int
	msg  string
}

func (e *toolsErr) Error() string { return e.msg }

func toolsBadParams(format string, args ...any) error {
	return &toolsErr{rpcInvalidParams, "invalid params: " + fmt.Sprintf(format, args...)}
}

// handle answers one message; nil for a notification or a response.
func (c *toolsConn) handle(raw []byte) *rpcResponse {
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  *string         `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return &rpcResponse{ID: nullID, Error: &rpcError{Code: rpcParseError, Message: "parse error: one JSON-RPC object per line"}}
	}
	hasID := len(msg.ID) > 0 && !bytes.Equal(msg.ID, nullID)
	if !hasID {
		return nil // a notification (the server reads none) or a response
	}
	if !validID(msg.ID) {
		return &rpcResponse{ID: nullID, Error: &rpcError{Code: rpcInvalidRequest, Message: "invalid request: the id must be a string or a number"}}
	}
	if msg.JSONRPC != "2.0" || msg.Method == nil {
		return &rpcResponse{ID: msg.ID, Error: &rpcError{Code: rpcInvalidRequest, Message: `invalid request: give "jsonrpc": "2.0" and a "method"`}}
	}
	params := msg.Params
	if len(params) == 0 || bytes.Equal(params, nullID) {
		params = json.RawMessage("{}")
	}
	if params[0] != '{' {
		return &rpcResponse{ID: msg.ID, Error: &rpcError{Code: rpcInvalidParams, Message: "invalid params: params must be an object"}}
	}
	res, err := c.call(*msg.Method, params)
	if err != nil {
		code := -32000
		var te *toolsErr
		if errors.As(err, &te) {
			code = te.code
		}
		return &rpcResponse{ID: msg.ID, Error: &rpcError{Code: code, Message: err.Error()}}
	}
	if res == nil {
		res = struct{}{}
	}
	return &rpcResponse{ID: msg.ID, Result: res}
}

// toolsParams are the parameters of every method; each reads its own.
type toolsParams struct {
	Window uint32   `json:"window"`
	ID     uint64   `json:"id"`
	Detail bool     `json:"detail"`
	Since  uint64   `json:"since"`
	On     *bool    `json:"on"`
	Events []string `json:"events"`
	// Input.
	Action string   `json:"action"`
	X      float32  `json:"x"`
	Y      float32  `json:"y"`
	DX     float32  `json:"dx"`
	DY     float32  `json:"dy"`
	Button string   `json:"button"`
	Mods   []string `json:"mods"`
	Count  int      `json:"count"`
	Key    string   `json:"key"`
	Text   string   `json:"text"`
}

// call runs a method.
func (c *toolsConn) call(method string, raw json.RawMessage) (any, error) {
	var p toolsParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, toolsBadParams("%v", err)
	}
	ts := c.ts
	tree := func(kind string, q devloop.ToolsParams) (any, error) {
		q.Window = p.Window
		return ts.relay(c, kind, q)
	}
	switch method {
	case "info":
		ts.mu.Lock()
		build := ts.build
		ts.mu.Unlock()
		return map[string]any{"pkg": ts.pkg, "pid": os.Getpid(), "started": ts.started, "build": build, "protocol": toolsProtocol}, nil
	case "windows":
		var ws []toolsWindow
		if err := ts.onUI(func() { ws = toolsWindows(ts.host.WindowInfos()) }); err != nil {
			return nil, err
		}
		return map[string]any{"windows": ws}, nil
	case "subscribe", "unsubscribe":
		for _, e := range p.Events {
			if !slices.Contains(toolsEvents, e) {
				return nil, toolsBadParams("unknown event %q; the events are %v", e, toolsEvents)
			}
		}
		ts.mu.Lock()
		for _, e := range p.Events {
			c.subs[e] = method == "subscribe"
			if c.subs[e] && (e == "picked" || e == "frames" || e == "logs") {
				c.holds = true
			}
		}
		var subs []string
		for _, e := range toolsEvents {
			if c.subs[e] {
				subs = append(subs, e)
			}
		}
		ts.mu.Unlock()
		if method == "subscribe" {
			go ts.syncInstrument()
		}
		return map[string]any{"events": subs}, nil
	case "instrument":
		on := p.On == nil || *p.On
		ts.hold(c, on)
		if err := ts.syncInstrument(); err != nil {
			return nil, err
		}
		ts.insMu.Lock()
		got := ts.instrumented != nil
		ts.insMu.Unlock()
		if on && !got {
			return nil, errNoApp
		}
		return map[string]any{"on": got}, nil
	case "elements":
		return tree(devloop.KindToolsElements, devloop.ToolsParams{Detail: p.Detail})
	case "element":
		if p.ID == 0 {
			return nil, toolsBadParams("give the id of an element")
		}
		return tree(devloop.KindToolsElement, devloop.ToolsParams{ID: p.ID})
	case "inspect":
		return tree(devloop.KindToolsInspect, devloop.ToolsParams{ID: p.ID})
	case "semantics":
		return tree(devloop.KindToolsSemantics, devloop.ToolsParams{})
	case "profiles":
		return tree(devloop.KindToolsProfiles, devloop.ToolsParams{Since: int(p.Since)})
	case "pick":
		on := p.On == nil || *p.On
		if on {
			ts.mu.Lock()
			c.subs["picked"] = true
			ts.mu.Unlock()
		}
		return tree(devloop.KindToolsPick, devloop.ToolsParams{On: on})
	case "highlight":
		return tree(devloop.KindToolsHighlight, devloop.ToolsParams{ID: p.ID})
	case "logs":
		// The entries the child has not pushed yet come first.
		ts.insMu.Lock()
		child := ts.instrumented
		ts.insMu.Unlock()
		if child != nil && ts.isActive(child) {
			if raw, err := ts.relay(c, devloop.KindToolsLogs, devloop.ToolsParams{}); err == nil {
				var r struct {
					Logs []devloop.ToolsLogBatch `json:"logs"`
				}
				if json.Unmarshal(raw, &r) == nil {
					ts.appLogs(r.Logs)
				}
			}
		}
		es, last := ts.log.since(p.Since)
		return map[string]any{"entries": es, "last": last}, nil
	case "screenshot":
		return ts.screenshot(p.Window)
	case "pointer":
		return nil, ts.pointer(p)
	case "scroll":
		mods, err := modsOf(p.Mods)
		if err != nil {
			return nil, err
		}
		pos := platform.Point{X: p.X, Y: p.Y}
		return nil, ts.inject(p.Window, func(wi remote.WindowInfo, now time.Duration) ([]platform.Event, error) {
			return []platform.Event{
				platform.PointerEvent{Action: platform.PointerMove, Pos: pos, Buttons: ts.buttons[wi.Window], Mods: mods, Time: now},
				platform.ScrollEvent{Pos: pos, Delta: platform.Point{X: p.DX, Y: p.DY}, Mods: mods, Time: now},
			}, nil
		})
	case "key":
		var keys []platform.KeyEvent
		var err error
		fl := platform.HostFlavor()
		switch p.Action {
		case "":
			keys, err = keycombo.Press(p.Key, fl)
		case "press":
			keys, err = keycombo.Down(p.Key, fl)
		case "release":
			keys, err = keycombo.Up(p.Key, fl)
		default:
			return nil, toolsBadParams(`action %q: use "press", "release" or none for both`, p.Action)
		}
		if err != nil {
			return nil, toolsBadParams("key %q: %v", p.Key, err)
		}
		return nil, ts.keys(p.Window, keys)
	case "text":
		return nil, ts.keys(p.Window, keycombo.Type(p.Text))
	}
	return nil, &toolsErr{rpcMethodNotFound, "method not found: " + method}
}

// screenshot encodes the frame window id presents.
func (ts *toolsServer) screenshot(id uint32) (any, error) {
	var img *image.RGBA
	var wi remote.WindowInfo
	var err error
	if uerr := ts.onUI(func() {
		if wi, err = findWindow(ts.host.WindowInfos(), id); err == nil {
			img = ts.host.Frame(wi.Window)
		}
	}); uerr != nil {
		return nil, uerr
	}
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("window %d has presented no frame yet", wi.ID)
	}
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, img); err != nil {
		return nil, err
	}
	return map[string]any{"window": wi.ID, "width": img.Rect.Dx(), "height": img.Rect.Dy(),
		"png": base64.StdEncoding.EncodeToString(b.Bytes())}, nil
}

// modsOf resolves modifier names (keycombo's).
func modsOf(names []string) (platform.Modifiers, error) {
	var m platform.Modifiers
	for _, n := range names {
		mod, ok := keycombo.ModifierNamed(n, platform.HostFlavor())
		if !ok {
			return 0, toolsBadParams(`%q is not a modifier; use "Shift", "Control", "Alt", "Meta" or "ControlOrMeta"`, n)
		}
		m |= mod.Mod
	}
	return m, nil
}

// pointerButtons are the names of the buttons.
var pointerButtons = map[string]platform.PointerButton{
	"": platform.ButtonPrimary, "left": platform.ButtonPrimary, "primary": platform.ButtonPrimary,
	"right": platform.ButtonSecondary, "secondary": platform.ButtonSecondary,
	"middle": platform.ButtonMiddle, "back": platform.ButtonBack, "forward": platform.ButtonForward,
}

// pointer injects a move, a press, a release or clicks of the mouse at a
// logical point of the window. A press or a release moves the pointer
// there first.
func (ts *toolsServer) pointer(p toolsParams) error {
	btn, ok := pointerButtons[p.Button]
	if !ok {
		return toolsBadParams(`button %q: use "left", "right", "middle", "back" or "forward"`, p.Button)
	}
	mods, err := modsOf(p.Mods)
	if err != nil {
		return err
	}
	switch p.Action {
	case "move", "down", "up", "click":
	default:
		return toolsBadParams(`action %q: use "move", "down", "up" or "click"`, p.Action)
	}
	pos := platform.Point{X: p.X, Y: p.Y}
	return ts.inject(p.Window, func(wi remote.WindowInfo, now time.Duration) ([]platform.Event, error) {
		held := ts.buttons[wi.Window]
		ev := func(a platform.PointerAction, b platform.PointerButton) platform.PointerEvent {
			return platform.PointerEvent{Action: a, Pos: pos, Button: b, Buttons: held, Mods: mods, Time: now}
		}
		evs := []platform.Event{ev(platform.PointerMove, platform.ButtonNone)}
		down := func() {
			held |= btn.Mask()
			evs = append(evs, ev(platform.PointerDown, btn))
		}
		up := func() {
			held &^= btn.Mask()
			evs = append(evs, ev(platform.PointerUp, btn))
		}
		switch p.Action {
		case "down":
			down()
		case "up":
			up()
		case "click":
			for range max(p.Count, 1) {
				down()
				up()
			}
		}
		ts.buttons[wi.Window] = held
		return evs, nil
	})
}

// keys injects key events into window id, giving it the focus first as
// the app sees it when it has not.
func (ts *toolsServer) keys(id uint32, keys []platform.KeyEvent) error {
	return ts.inject(id, func(wi remote.WindowInfo, now time.Duration) ([]platform.Event, error) {
		var evs []platform.Event
		if !wi.Focused {
			evs = append(evs, platform.FocusEvent{Focused: true})
		}
		for _, k := range keys {
			k.Time = now
			evs = append(evs, k)
		}
		return evs, nil
	})
}
