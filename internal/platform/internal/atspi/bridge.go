//go:build linux

package atspi

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// Options configures a Bridge. Wake and Event are required.
type Options struct {
	// AppName names the application object (its accessible name). ""
	// means the executable's base name.
	AppName string
	// Wake unblocks the backend's wait so that its loop calls Dispatch.
	// It is called from the bridge's goroutines, must not block, and may
	// coalesce.
	Wake func()
	// Event queues a window event (AccessibilityEvent, AccessActionEvent)
	// for the handler: internal/loop Loop.Enqueue. Called from Dispatch
	// only, on the UI thread.
	Event func(w platform.Window, e platform.Event)
	// Address is the D-Bus address of the session bus, where org.a11y.Bus
	// says whether ATs are enabled and where the accessibility bus is. ""
	// means the session's: $DBUS_SESSION_BUS_ADDRESS, else
	// $XDG_RUNTIME_DIR/bus (never autolaunched). Tests point it at a
	// private bus.
	Address string
	// Getenv reads the environment (NO_AT_BRIDGE, AT_SPI_BUS_ADDRESS, the
	// session bus and the locale variables). nil means os.Getenv.
	Getenv func(key string) string
	// Logf reports what the bridge cannot do (an update the tree refuses,
	// a registration that failed). nil prints to standard error.
	Logf func(format string, args ...any)
	// Timeout bounds every D-Bus call the bridge makes. 0 means 5 s.
	Timeout time.Duration
}

// Bridge is the AT-SPI2 bridge of one process, shared by its windows.
// Create it with New; the backend calls NewWindow for each window,
// Dispatch on every loop iteration and Close when Run returns.
type Bridge struct {
	opts    Options
	getenv  func(string) string
	appName string

	// UI thread only.
	started  bool
	disabled bool // NO_AT_BRIDGE
	nextID   uint32
	closing  bool

	// mu guards the queues between the UI thread and the worker.
	mu     sync.Mutex
	closed bool
	inbox  []inboxItem // worker → UI thread (Dispatch)
	jobs   []job       // UI thread → worker
	kick   chan struct{}
	done   chan struct{}

	// wmu guards windows, which godbus goroutines read.
	wmu     sync.RWMutex
	windows []*Window // in creation order (the app's children)

	// What query goroutines read about the registration.
	unique  atomic.Pointer[string] // our unique name on the accessibility bus
	desktop atomic.Pointer[ref]    // the registry's desktop: the app's parent
	appID   atomic.Int32           // Application.Id, set by the registry
	gen     atomic.Uint64          // the worker's activation generation (worker.gen)

	// Worker only.
	wk worker
}

// inboxItem is an event for the UI thread: an activation change of window
// w (act) or an AT's action on it.
type inboxItem struct {
	w      *Window
	gen    uint64
	act    bool // an activation change; else action
	active bool
	action platform.AccessActionEvent
}

// jobKind is what the UI thread asks the worker.
type jobKind uint8

const (
	jobAdd jobKind = iota
	jobRemove
	jobUpdate
	jobWindow
	jobClose
)

// job is a unit of work for the worker.
type job struct {
	kind jobKind
	w    *Window
	gen  uint64
	tree *platform.AccessTree
	// prev is the tree the update was applied to, changed the IDs of the
	// nodes it held and removed those it removed (updateView).
	prev             *platform.AccessTree
	changed, removed []platform.AccessID
	ann              []platform.Announcement
	st               windowState
}

// windowState is what a backend tells about a window besides its tree.
type windowState struct {
	title     string
	active    bool
	scale     float64
	origin    image.Point
	hasOrigin bool
}

// New returns a bridge. It does no I/O and starts nothing: the first
// window starts it.
func New(opts Options) *Bridge {
	if opts.Wake == nil || opts.Event == nil {
		panic("atspi: Options.Wake and Event are required")
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.Logf == nil {
		opts.Logf = func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	name := opts.AppName
	if name == "" {
		name = filepath.Base(os.Args[0])
	}
	return &Bridge{
		opts:    opts,
		getenv:  opts.Getenv,
		appName: name,
		kick:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
}

// Window is the bridge's side of one platform window. Its methods run on
// the UI thread.
type Window struct {
	b    *Bridge
	win  platform.Window
	id   uint32
	path dbus.ObjectPath

	// UI thread only.
	tree     *platform.AccessTree
	uiActive bool
	uiGen    uint64
	st       windowState
	gone     bool
	refused  bool // an update was refused: the next ones are dropped until a new activation

	// snap is what queries read; the worker replaces it.
	snap atomic.Pointer[snapshot]
	// removed is set by the worker when the window left the app.
	removed atomic.Bool
}

// NewWindow registers platform window win with the bridge, starting the
// bridge on the first call. title is its title. UI thread.
func (b *Bridge) NewWindow(win platform.Window, title string) *Window {
	b.nextID++
	w := &Window{b: b, win: win, id: b.nextID, st: windowState{title: title, scale: 1}}
	w.path = dbus.ObjectPath(fmt.Sprintf("%sw%d", accessiblePrefix, w.id))
	w.snap.Store(&snapshot{view: buildView(nil), title: title, scale: 1})
	b.wmu.Lock()
	b.windows = append(b.windows, w)
	b.wmu.Unlock()
	if !b.started && !b.closing {
		b.started = true
		if v := b.getenv("NO_AT_BRIDGE"); v == "1" {
			b.disabled = true
		} else {
			go b.run()
		}
	}
	b.push(job{kind: jobAdd, w: w})
	return w
}

// Update gives the window's bridge the next accessibility update. It is
// dropped unless the window's bridge is active (between the
// AccessibilityEvents with Active true and false that Dispatch
// delivered). UI thread.
//
// An activation starts from the empty tree, but the events that tell core
// about it are queued: until core handles them it may still send diffs of
// its previous session (a deactivation and activation in one Dispatch, an
// update queued before them). Such a diff does not apply to the empty
// tree and is dropped; core's whole tree follows its Active true.
func (w *Window) Update(u platform.AccessUpdate) {
	platform.CheckAccessUpdate(&u)
	if w.gone || !w.uiActive || w.refused {
		return
	}
	t, removed, err := w.tree.Apply(&u)
	if err != nil && w.tree == nil {
		return // a diff from before the activation (above)
	}
	if err != nil {
		// Core sent something the tree cannot take: a bug. The AT keeps
		// the last good tree; the next activation starts over.
		w.refused = true
		w.b.opts.Logf("tela: accessibility: %v; the screen reader's view of the window stops here", err)
		return
	}
	changed := make([]platform.AccessID, len(u.Nodes))
	for i := range u.Nodes {
		changed[i] = u.Nodes[i].ID
	}
	prev := w.tree
	w.tree = t
	w.b.push(job{kind: jobUpdate, w: w, gen: w.uiGen, tree: t, prev: prev, changed: changed, removed: removed,
		ann: slices.Clone(u.Announcements)})
}

// SetTitle records the window's title, the frame's name. UI thread.
func (w *Window) SetTitle(title string) {
	if w.gone || w.st.title == title {
		return
	}
	w.st.title = title
	w.b.push(job{kind: jobWindow, w: w, st: w.st})
}

// SetFocused records whether the window has the keyboard focus (the
// frame's active state, window:activate and deactivate). UI thread.
func (w *Window) SetFocused(focused bool) {
	if w.gone || w.st.active == focused {
		return
	}
	w.st.active = focused
	w.b.push(job{kind: jobWindow, w: w, st: w.st})
}

// SetGeometry records the window's scale and, when known (X11), the
// screen position of its content area in physical pixels, for the
// extents of the Component interface. UI thread.
func (w *Window) SetGeometry(scale float64, origin image.Point, hasOrigin bool) {
	if !hasOrigin {
		origin = image.Point{}
	}
	if w.gone || (w.st.scale == scale && w.st.origin == origin && w.st.hasOrigin == hasOrigin) {
		return
	}
	w.st.scale, w.st.origin, w.st.hasOrigin = scale, origin, hasOrigin
	w.b.push(job{kind: jobWindow, w: w, st: w.st})
}

// Close removes the window from the application. UI thread.
func (w *Window) Close() {
	if w.gone {
		return
	}
	w.gone = true
	w.tree = nil
	w.b.push(job{kind: jobRemove, w: w})
}

// push queues a job for the worker. Dropped after Close and when the
// bridge is disabled.
func (b *Bridge) push(j job) {
	if b.disabled {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.jobs = append(b.jobs, j)
	b.mu.Unlock()
	select {
	case b.kick <- struct{}{}:
	default:
	}
}

// post queues an item for the UI thread and wakes the loop. Worker and
// godbus goroutines.
func (b *Bridge) post(it inboxItem) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.inbox = append(b.inbox, it)
	b.mu.Unlock()
	b.opts.Wake()
}

// Dispatch delivers the activation changes and the ATs' actions that
// arrived since the last call to the loop (Options.Event), in order. UI
// thread; the backend calls it on every iteration before its delivery
// pass.
func (b *Bridge) Dispatch() {
	b.mu.Lock()
	items := b.inbox
	b.inbox = nil
	b.mu.Unlock()
	for _, it := range items {
		w := it.w
		if w.gone {
			continue
		}
		if it.act {
			if it.active == w.uiActive {
				continue
			}
			w.uiActive, w.uiGen, w.tree, w.refused = it.active, it.gen, nil, false
			b.opts.Event(w.win, platform.AccessibilityEvent{Active: it.active})
			continue
		}
		if !w.uiActive || it.gen != w.uiGen {
			continue // asked of a tree that is gone
		}
		b.opts.Event(w.win, it.action)
	}
}

// Close unregisters the application from the registry and closes the
// accessibility connections. UI thread; it waits for the worker (each
// D-Bus call it makes is bounded by Options.Timeout).
func (b *Bridge) Close() {
	if b.closing {
		return
	}
	b.closing = true
	if !b.started || b.disabled {
		return
	}
	b.push(job{kind: jobClose})
	b.mu.Lock()
	b.closed = true
	b.inbox = nil
	b.mu.Unlock()
	<-b.done
}

// --- the worker ---

// worker is the state of the bridge's goroutine (Bridge.run): the D-Bus
// connections and the registration. Every signal the bridge emits is
// sent from it, in the order of the updates.
type worker struct {
	session *dbus.Conn
	a11y    *dbus.Conn
	// enabled is the session's org.a11y.Status (IsEnabled or
	// ScreenReaderEnabled); registered that the app is embedded in the
	// registry.
	enabled, registered  bool
	isEnabled, srEnabled bool
	a11ySig              chan *dbus.Signal
	// gen counts activations and deactivations: an update or an action
	// of another generation is dropped.
	gen uint64
	// listeners are the events ATs listen to (the registry's registered
	// events); all when unknown.
	listeners listenerSet
}

// run is the worker goroutine.
func (b *Bridge) run() {
	defer close(b.done)
	wk := &b.wk
	sessionSig := make(chan *dbus.Signal, 64)
	if b.connectSession(sessionSig) {
		b.readStatus()
		b.applyStatus()
	}
	for {
		select {
		case <-b.kick:
			if b.runJobs() {
				b.teardown()
				return
			}
		case s, ok := <-sessionSig:
			if !ok {
				sessionSig = nil
				continue
			}
			if b.sessionSignal(s) {
				b.applyStatus()
			}
		case s, ok := <-wk.a11ySig:
			if !ok {
				// The accessibility bus went away: as if disabled, until
				// the status says otherwise.
				wk.a11ySig = nil
				b.unregister(false)
				continue
			}
			b.a11ySignal(s)
		}
	}
}

// runJobs runs the queued jobs and reports whether one closes the bridge.
func (b *Bridge) runJobs() bool {
	b.mu.Lock()
	jobs := b.jobs
	b.jobs = nil
	b.mu.Unlock()
	for _, j := range jobs {
		switch j.kind {
		case jobClose:
			return true
		case jobAdd:
			b.windowAdded(j.w)
		case jobRemove:
			b.windowRemoved(j.w)
		case jobWindow:
			b.windowChanged(j.w, j.st)
		case jobUpdate:
			if b.wk.registered && j.gen == b.wk.gen {
				b.treeChanged(j.w, j)
			}
		}
	}
	return false
}

// teardown unregisters and closes the connections.
func (b *Bridge) teardown() {
	b.unregister(true)
	if b.wk.session != nil {
		b.wk.session.Close()
		b.wk.session = nil
	}
}

// sessionAddress returns the session bus address, "" for none.
func (b *Bridge) sessionAddress() string {
	if b.opts.Address != "" {
		return b.opts.Address
	}
	if a := b.getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" && a != "autolaunch:" {
		return a
	}
	dir := b.getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	p := filepath.Join(dir, "bus")
	if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
		return "unix:path=" + dbus.EscapeBusAddressValue(p)
	}
	return ""
}

func (b *Bridge) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), b.opts.Timeout)
}

// connectSession connects to the session bus and follows org.a11y.Status.
// Without a session bus (or with AT_SPI_BUS_ADDRESS set, which names the
// accessibility bus directly, as at-spi2-atk honors it) the bridge does
// nothing more until AT_SPI_BUS_ADDRESS registers it.
func (b *Bridge) connectSession(sig chan *dbus.Signal) bool {
	wk := &b.wk
	if b.getenv("AT_SPI_BUS_ADDRESS") != "" {
		// The accessibility bus is given: no status to follow, enabled.
		wk.isEnabled = true
		return true
	}
	addr := b.sessionAddress()
	if addr == "" {
		return false
	}
	conn, err := dbus.Connect(addr)
	if err != nil {
		return false
	}
	wk.session = conn
	conn.Signal(sig)
	ctx, cancel := b.ctx()
	defer cancel()
	conn.AddMatchSignalContext(ctx,
		dbus.WithMatchObjectPath(a11yBusPath),
		dbus.WithMatchInterface(propsIface),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchArg(0, statusIface))
	conn.AddMatchSignalContext(ctx,
		dbus.WithMatchSender(busIface),
		dbus.WithMatchInterface(busIface),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, a11yBusName))
	return true
}

// readStatus reads org.a11y.Status (a property that does not exist, as
// ScreenReaderEnabled on some at-spi2-core builds, is false).
func (b *Bridge) readStatus() {
	wk := &b.wk
	if wk.session == nil {
		return
	}
	ctx, cancel := b.ctx()
	defer cancel()
	var props map[string]dbus.Variant
	err := wk.session.Object(a11yBusName, a11yBusPath).CallWithContext(ctx, propsIface+".GetAll", 0, statusIface).Store(&props)
	if err != nil {
		wk.isEnabled, wk.srEnabled = false, false
		return
	}
	wk.isEnabled, _ = props["IsEnabled"].Value().(bool)
	wk.srEnabled, _ = props["ScreenReaderEnabled"].Value().(bool)
}

// sessionSignal handles a session bus signal and reports whether the
// status may have changed.
func (b *Bridge) sessionSignal(s *dbus.Signal) bool {
	wk := &b.wk
	switch s.Name {
	case propsIface + ".PropertiesChanged":
		if s.Path != a11yBusPath || len(s.Body) < 2 {
			return false
		}
		if iface, _ := s.Body[0].(string); iface != statusIface {
			return false
		}
		changed, _ := s.Body[1].(map[string]dbus.Variant)
		if v, ok := changed["IsEnabled"]; ok {
			wk.isEnabled, _ = v.Value().(bool)
		}
		if v, ok := changed["ScreenReaderEnabled"]; ok {
			wk.srEnabled, _ = v.Value().(bool)
		}
		return true
	case busIface + ".NameOwnerChanged":
		if len(s.Body) < 3 {
			return false
		}
		if name, _ := s.Body[0].(string); name != a11yBusName {
			return false
		}
		// The bus launcher restarted (or quit): read the status again; a
		// new accessibility bus needs a new registration.
		b.unregister(false)
		b.readStatus()
		return true
	}
	return false
}

// applyStatus registers when ATs are enabled and unregisters when not.
func (b *Bridge) applyStatus() {
	wk := &b.wk
	wk.enabled = wk.isEnabled || wk.srEnabled
	switch {
	case wk.enabled && !wk.registered:
		b.register()
	case !wk.enabled && wk.registered:
		b.unregister(false)
	}
}

// a11yAddress returns the accessibility bus address.
func (b *Bridge) a11yAddress() (string, error) {
	if a := b.getenv("AT_SPI_BUS_ADDRESS"); a != "" {
		return a, nil
	}
	ctx, cancel := b.ctx()
	defer cancel()
	var addr string
	err := b.wk.session.Object(a11yBusName, a11yBusPath).CallWithContext(ctx, a11yBusIface+".GetAddress", 0).Store(&addr)
	return addr, err
}

// register connects to the accessibility bus, embeds the application in
// the registry and activates every window.
func (b *Bridge) register() {
	wk := &b.wk
	addr, err := b.a11yAddress()
	if err != nil || addr == "" {
		b.opts.Logf("tela: accessibility: no accessibility bus (%v); screen readers cannot see the app", err)
		return
	}
	conn, err := dbus.Connect(addr, dbus.WithHandler(&handler{b: b}))
	if err != nil {
		b.opts.Logf("tela: accessibility: cannot connect to the accessibility bus %s: %v", addr, err)
		return
	}
	names := conn.Names()
	if len(names) == 0 {
		conn.Close()
		return
	}
	unique := names[0]
	b.unique.Store(&unique)
	wk.a11y = conn
	wk.a11ySig = make(chan *dbus.Signal, 64)
	conn.Signal(wk.a11ySig)

	ctx, cancel := b.ctx()
	defer cancel()
	conn.AddMatchSignalContext(ctx,
		dbus.WithMatchInterface(registryIface),
		dbus.WithMatchMember("EventListenerRegistered"))
	conn.AddMatchSignalContext(ctx,
		dbus.WithMatchInterface(registryIface),
		dbus.WithMatchMember("EventListenerDeregistered"))
	conn.AddMatchSignalContext(ctx,
		dbus.WithMatchSender(busIface),
		dbus.WithMatchInterface(busIface),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, registryName))

	if !b.embed(ctx) {
		b.closeA11y()
		return
	}
	wk.registered = true
	wk.gen++
	b.gen.Store(wk.gen)
	b.readListeners(ctx)
	// Every window starts from an empty tree; core sends a full one.
	b.wmu.RLock()
	ws := slices.Clone(b.windows)
	b.wmu.RUnlock()
	for _, w := range ws {
		if w.removed.Load() {
			continue
		}
		b.reset(w)
		b.post(inboxItem{w: w, gen: wk.gen, act: true, active: true})
	}
}

// embed embeds the application in the registry (Socket.Embed), which
// answers with the desktop, our parent.
func (b *Bridge) embed(ctx context.Context) bool {
	wk := &b.wk
	var desk ref
	err := wk.a11y.Object(registryName, desktopRootPath).CallWithContext(ctx, socketIface+".Embed", 0,
		ref{Name: *b.unique.Load(), Path: appRootPath}).Store(&desk)
	if err != nil {
		b.opts.Logf("tela: accessibility: the registry refused the application: %v", err)
		return false
	}
	b.desktop.Store(&desk)
	return true
}

// unregister undoes register: unembeds the application (unless the bus is
// gone), closes the connection and deactivates every window. closing
// skips the deactivation events (the app is ending).
func (b *Bridge) unregister(closing bool) {
	wk := &b.wk
	if wk.a11y == nil {
		return
	}
	if wk.registered && wk.a11y.Connected() {
		ctx, cancel := b.ctx()
		wk.a11y.Object(registryName, desktopRootPath).CallWithContext(ctx, socketIface+".Unembed", 0,
			ref{Name: *b.unique.Load(), Path: appRootPath})
		cancel()
	}
	was := wk.registered
	wk.registered = false
	b.closeA11y()
	if !was || closing {
		return
	}
	wk.gen++
	b.gen.Store(wk.gen)
	b.wmu.RLock()
	ws := slices.Clone(b.windows)
	b.wmu.RUnlock()
	for _, w := range ws {
		b.reset(w)
		b.post(inboxItem{w: w, gen: wk.gen, act: true, active: false})
	}
}

func (b *Bridge) closeA11y() {
	wk := &b.wk
	if wk.a11y != nil {
		wk.a11y.Close()
		wk.a11y = nil
	}
	wk.a11ySig = nil
	wk.listeners = listenerSet{}
}

// reset drops window w's tree.
func (b *Bridge) reset(w *Window) {
	old := w.snap.Load()
	s := *old
	s.view = buildView(nil)
	w.snap.Store(&s)
}

// a11ySignal handles a signal of the accessibility bus: listener changes
// and a registry that restarted.
func (b *Bridge) a11ySignal(s *dbus.Signal) {
	wk := &b.wk
	switch s.Name {
	case registryIface + ".EventListenerRegistered":
		if len(s.Body) >= 2 {
			bus, _ := s.Body[0].(string)
			ev, _ := s.Body[1].(string)
			wk.listeners.add(bus, ev)
		}
	case registryIface + ".EventListenerDeregistered":
		if len(s.Body) >= 2 {
			bus, _ := s.Body[0].(string)
			ev, _ := s.Body[1].(string)
			wk.listeners.remove(bus, ev)
		}
	case busIface + ".NameOwnerChanged":
		if len(s.Body) < 3 {
			return
		}
		name, _ := s.Body[0].(string)
		owner, _ := s.Body[2].(string)
		if name != registryName || owner == "" || !wk.registered {
			return
		}
		// A new registry: embed again (the windows keep their trees).
		ctx, cancel := b.ctx()
		defer cancel()
		if b.embed(ctx) {
			b.readListeners(ctx)
		}
	}
}

// readListeners reads the events ATs listen to. When the registry cannot
// tell, every event is sent.
func (b *Bridge) readListeners(ctx context.Context) {
	wk := &b.wk
	var evs []struct{ Bus, Event string }
	err := wk.a11y.Object(registryName, registryPath).CallWithContext(ctx, registryIface+".GetRegisteredEvents", 0).Store(&evs)
	wk.listeners = listenerSet{known: err == nil}
	for _, e := range evs {
		wk.listeners.add(e.Bus, e.Event)
	}
}

// --- windows (worker) ---

func (b *Bridge) windowAdded(w *Window) {
	wk := &b.wk
	if !wk.registered {
		return
	}
	b.post(inboxItem{w: w, gen: wk.gen, act: true, active: true})
	if idx := b.windowIndex(w); idx >= 0 {
		b.emit(appRootPath, eventObjectIface, "ChildrenChanged", "add", int32(idx), 0, dbus.MakeVariant(b.ref(w, nil)))
	}
	b.emit(w.path, eventWindowIface, "Create", "", 0, 0, dbus.MakeVariant(""))
}

func (b *Bridge) windowRemoved(w *Window) {
	idx := b.windowIndex(w)
	w.removed.Store(true)
	b.wmu.Lock()
	if i := slices.Index(b.windows, w); i >= 0 {
		b.windows = slices.Delete(b.windows, i, i+1)
	}
	b.wmu.Unlock()
	if !b.wk.registered {
		return
	}
	b.emit(w.path, eventWindowIface, "Destroy", "", 0, 0, dbus.MakeVariant(""))
	if idx >= 0 {
		b.emit(appRootPath, eventObjectIface, "ChildrenChanged", "remove", int32(idx), 0, dbus.MakeVariant(b.ref(w, nil)))
	}
}

// windowIndex returns w's index among the application's children, -1 when
// it is not one.
func (b *Bridge) windowIndex(w *Window) int {
	b.wmu.RLock()
	defer b.wmu.RUnlock()
	return slices.Index(b.windows, w)
}

// windowChanged applies a backend's change of a window's own state.
func (b *Bridge) windowChanged(w *Window, st windowState) {
	old := w.snap.Load()
	s := *old
	s.title, s.active, s.scale, s.origin, s.hasOrigin = st.title, st.active, st.scale, st.origin, st.hasOrigin
	w.snap.Store(&s)
	if b.wk.registered && !w.removed.Load() {
		b.windowEvents(w, old, &s)
	}
}

// treeChanged publishes window w's new tree and tells the AT what
// changed. The view follows the update's changes when the last one
// published is of the tree it was applied to (ADR-277); it is built in
// full otherwise (the first tree, a new root, an update after one the
// worker skipped).
func (b *Bridge) treeChanged(w *Window, j job) {
	t, ann := j.tree, j.ann
	old := w.snap.Load()
	s := *old
	var touched []platform.AccessID
	full := old.view.tree == nil || old.view.tree != j.prev || j.prev.Root() != t.Root()
	if full {
		s.view = buildView(t)
	} else {
		s.view, touched = updateView(old.view, t, j.changed, j.removed)
		if checkViews {
			if d := diffViews(s.view, buildView(t)); d != "" {
				panic("atspi: TELA_CHECK_SEMANTICS: the updated view differs from a full build: " + d)
			}
		}
	}
	w.snap.Store(&s)
	if w.removed.Load() {
		return
	}
	if old.view.tree == nil && s.active {
		// The first tree of an activation in a focused window: an AT that
		// just started learns the active window (Wayland has no other way
		// to tell it), then the focus (treeEvents).
		b.emit(w.path, eventWindowIface, "Activate", "", 0, 0, dbus.MakeVariant(s.title))
	}
	b.treeEvents(w, old, &s, touched, full)
	for _, a := range ann {
		pol := int32(livePolite)
		if a.Assertive {
			pol = liveAssertive
		}
		b.emit(w.path, eventObjectIface, "Announcement", "", pol, 0, dbus.MakeVariant(a.Text))
	}
}

// listenerSet holds the events ATs registered for, normalized
// (normalizeEvent).
type listenerSet struct {
	known bool
	evs   map[string]int // normalized event → listener count
}

func (l *listenerSet) add(bus, ev string) {
	if l.evs == nil {
		l.evs = map[string]int{}
	}
	l.evs[normalizeEvent(ev)]++
}

func (l *listenerSet) remove(bus, ev string) {
	k := normalizeEvent(ev)
	if l.evs[k] > 1 {
		l.evs[k]--
	} else {
		delete(l.evs, k)
	}
}

// wants reports whether an AT listens to the event of the normalized
// segments category, kind and detail ("object", "statechanged",
// "focused").
func (l *listenerSet) wants(ev string) bool {
	if !l.known {
		return true
	}
	for k := range l.evs {
		if k == "" || ev == k || strings.HasPrefix(ev, k+":") {
			return true
		}
	}
	return false
}

// normalizeEvent returns an event name as libatspi registers it
// ("Object:StateChanged:Focused", "window:activate", "object:") in one
// form: lowercase segments without dashes, empty trailing segments
// dropped ("object:statechanged:focused").
func normalizeEvent(ev string) string {
	ev = strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(ev))
	return strings.TrimRight(ev, ":")
}
