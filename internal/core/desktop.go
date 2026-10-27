package core

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/kuronosu/tela/internal/platform"
)

// What package desktop reaches through core (ADR-181): the running app's
// OS services and the window a request comes from, the app-scoped events
// of the backend (notifications, tray icons), per-app storage, windows
// opened by code (the drawn message box), a change notification of a
// window's placement, and a hook at the end of Run. Everything here runs
// on the UI thread unless a comment says otherwise.

// Desktop is the access of package desktop to the OS services of a
// running app (Host). The zero value is no app.
type Desktop struct{ h *Host }

// hostDesktop is the desktop state of a Host (Host.desk).
type hostDesktop struct {
	listeners []*appListener
	local     map[any]any
}

type appListener struct {
	f       func(platform.Event)
	stopped bool
}

// RunningDesktop returns the Desktop of the running app (the Tree that
// receives Post, Run's or telatest's). what names the call in the error
// when no app runs; in dev mode, a call from a goroutine other than the
// UI thread panics with the fix.
func RunningDesktop(what string) (Desktop, error) {
	h, err := runningHost(what)
	if err != nil {
		return Desktop{}, err
	}
	return Desktop{h}, nil
}

// DesktopOf returns the Desktop of the app window w belongs to; false for
// the zero handle, a tree without a window (tela snapshot) or an app that
// ended.
func DesktopOf(w WindowHandle) (Desktop, bool) {
	if w.t == nil || w.t.wv == nil || w.t.wv.host.ended {
		return Desktop{}, false
	}
	return Desktop{w.t.wv.host}, true
}

// RunningAppID returns the AppID of the running app (Options.AppID); false
// when no app runs or its AppID is empty. Safe from any goroutine.
func RunningAppID() (string, bool) {
	posts.mu.Lock()
	t := posts.target
	posts.mu.Unlock()
	if t == nil || t.wv == nil {
		return "", false
	}
	id := t.wv.host.appID
	return id, id != ""
}

// IsZero reports whether d is the zero Desktop (no app).
func (d Desktop) IsZero() bool { return d.h == nil }

// Services returns the backend's desktop services.
func (d Desktop) Services() platform.Desktop { return d.h.backend.Desktop() }

// Ended reports whether the app's loop is over (the backend must not be
// called).
func (d Desktop) Ended() bool { return d.h.ended }

// Window returns the window a request comes from: w when it is an open
// window of this app; else the window whose handler or effect is running
// (a click in a child window); else the focused window; else the main
// window.
func (d Desktop) Window(w WindowHandle) WindowHandle {
	h := d.h
	if w.t != nil {
		if v := w.t.wv; v != nil && v.host == h && v.alive() {
			return w
		}
	}
	for _, v := range h.views {
		if v.tree.handlerDepth > 0 {
			return WindowHandle{v.tree}
		}
	}
	for _, v := range h.views {
		if v.focused {
			return WindowHandle{v.tree}
		}
	}
	return WindowHandle{h.main.tree}
}

// PlatformWindow returns the platform window of w's window, or nil for a
// window that is not open in this app.
func (d Desktop) PlatformWindow(w WindowHandle) platform.Window {
	if w.t == nil {
		return nil
	}
	if v := w.t.wv; v != nil && v.host == d.h && v.alive() {
		return v.win
	}
	return nil
}

// Listen calls f with every app-scoped event of the backend
// (platform.NotificationEvent, platform.TrayEvent), on the UI thread,
// until stop is called. The listeners run in the order they were added;
// one added or stopped while an event is delivered takes effect with the
// next event.
func (d Desktop) Listen(f func(platform.Event)) (stop func()) {
	if f == nil {
		panic("core: Desktop.Listen(nil)")
	}
	dk := d.h.desktop()
	l := &appListener{f: f}
	dk.listeners = append(dk.listeners, l)
	return func() {
		if l.stopped {
			return
		}
		l.stopped = true
		dk.listeners = slices.DeleteFunc(dk.listeners, func(x *appListener) bool { return x == l })
	}
}

// Local returns the value stored for key in this app, making it with init
// the first time: per-app state of package desktop (the notifications it
// shows), dropped with the app.
func (d Desktop) Local(key any, init func() any) any {
	dk := d.h.desktop()
	if v, ok := dk.local[key]; ok {
		return v
	}
	if dk.local == nil {
		dk.local = map[any]any{}
	}
	v := init()
	dk.local[key] = v
	return v
}

// Warn reports a dev warning once per key, as the main window's tree
// reports its own (stderr, or the test log under telatest).
func (d Desktop) Warn(key string, msg func() string) { d.h.tree.warnOnce(key, msg) }

// Dev reports whether the app runs in dev mode.
func (d Desktop) Dev() bool { return d.h.tree.dev }

// desktop returns h's desktop state, made on first use.
func (h *Host) desktop() *hostDesktop {
	if h.desk == nil {
		h.desk = &hostDesktop{}
	}
	return h.desk
}

// appEvent delivers an app-scoped event (a notification's or a tray
// icon's) to the listeners of package desktop.
func (h *Host) appEvent(e platform.Event) {
	if h.desk == nil {
		return
	}
	for _, l := range slices.Clone(h.desk.listeners) {
		if !l.stopped {
			l.f(e)
		}
	}
}

// ---------------------------------------------------------------------
// The end of Run.

var runEnd struct {
	mu sync.Mutex
	fs []func()
}

// OnRunEnd registers f to run when Run returns, after the windows
// unmounted (their effects' cleanups ran): package desktop writes the
// settings it still holds there. Process-wide; safe from any goroutine.
func OnRunEnd(f func()) {
	runEnd.mu.Lock()
	defer runEnd.mu.Unlock()
	runEnd.fs = append(runEnd.fs, f)
}

// runEndHooks runs the functions given to OnRunEnd, in order.
func runEndHooks() {
	runEnd.mu.Lock()
	fs := slices.Clone(runEnd.fs)
	runEnd.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

// ---------------------------------------------------------------------
// Placement changes.

// placeWatch holds the placement listeners of a window (winView.watch).
type placeWatch struct {
	last WindowPlacement
	fs   []*placeListener
}

type placeListener struct {
	f       func(WindowPlacement)
	stopped bool
}

// WatchPlacement calls f on the UI thread with the placement of w's window
// (WindowHandle.Placement) each time it changes: the window was resized,
// moved where the OS reports positions, maximized or made fullscreen, or
// its restore token changed. Bursts are not coalesced (a drag reports
// every configure the loop delivers); callers debounce. stop ends it; a
// window without a platform window (tela snapshot) or a closed one never
// calls f.
func WatchPlacement(w WindowHandle, f func(WindowPlacement)) (stop func()) {
	if f == nil {
		panic("core: WatchPlacement(nil)")
	}
	if w.t == nil || w.t.wv == nil || !w.t.wv.alive() {
		return func() {}
	}
	v := w.t.wv
	if v.watch == nil {
		v.watch = &placeWatch{last: w.Placement()}
	}
	pw := v.watch
	l := &placeListener{f: f}
	pw.fs = append(pw.fs, l)
	return func() {
		if l.stopped {
			return
		}
		l.stopped = true
		pw.fs = slices.DeleteFunc(pw.fs, func(x *placeListener) bool { return x == l })
	}
}

// placementMoved tells the placement listeners of v about a change, after
// a ConfigureEvent or a WindowStateEvent (notePlacement).
func (v *winView) placementMoved() {
	pw := v.watch
	if pw == nil || len(pw.fs) == 0 {
		return
	}
	v.mu.Lock()
	p := v.place
	v.mu.Unlock()
	if p == pw.last {
		return
	}
	pw.last = p
	for _, l := range slices.Clone(pw.fs) {
		if !l.stopped {
			l.f(p)
		}
	}
}

// ---------------------------------------------------------------------
// Windows opened by code.

// CodeWindow is a window opened by code with OpenWindow rather than by a
// Window component in the tree (package desktop's drawn message box). It
// is a child of the window it was opened from, closes with it, and works
// like a Window component's otherwise: its spec's Root is built in a Tree
// of its own below the parent tree's root (contexts above the root reach
// it), OnClose runs on a close request, Modal blocks the parent.
type CodeWindow struct {
	t *Tree
	s *windowSlot
}

// OpenWindow opens a window showing spec.Root as a child of parent's
// window. UI thread, outside builds (a handler, an effect, a posted
// function). It fails for the zero handle, a tree without a platform
// window (tela snapshot) or a window the backend could not open.
func OpenWindow(parent WindowHandle, spec WindowSpec) (*CodeWindow, error) {
	t := parent.t
	if t == nil || t.wv == nil || !t.wv.alive() || t.root == nil {
		return nil, errors.New("tela: no open window to open a window from")
	}
	if spec.Root.d == nil {
		panic("core: OpenWindow: spec.Root is nil")
	}
	checkWindowSpec("tela.Window", spec.Title, &spec)
	s := &windowSlot{el: t.root, spec: spec, detached: true}
	t.wins = append(t.wins, s)
	t.openWindow(s)
	if s.view == nil {
		s.gone = true
		t.wins = slices.DeleteFunc(t.wins, func(x *windowSlot) bool { return x == s })
		return nil, fmt.Errorf("tela: cannot open the window %q", spec.Title)
	}
	return &CodeWindow{t: t, s: s}, nil
}

// Handle returns the window's handle (the zero handle once it closed).
func (c *CodeWindow) Handle() WindowHandle {
	if v := c.s.view; v != nil {
		return WindowHandle{v.tree}
	}
	return WindowHandle{}
}

// Update gives the window a new spec: the fields that changed are
// applied and Root is built again, as for a Window component (FixedSize,
// Frameless and Modal stay). It is applied with the parent tree's
// updates, never inside the window's own frame or handlers. Safe from any
// goroutine.
func (c *CodeWindow) Update(spec WindowSpec) {
	t, s := c.t, c.s
	t.enqueue(update{post: func() {
		if s.gone || s.view == nil {
			return
		}
		checkWindowSpec("tela.Window", spec.Title, &spec)
		s.spec = spec
		t.queueWindow(s)
		t.flushWindows()
	}})
}

// Close closes the window (its tree unmounts first). UI thread; inside
// the window's own handlers or frame it closes at the next wake.
func (c *CodeWindow) Close() {
	s := c.s
	if s.gone {
		return
	}
	s.unmount(c.t)
	c.t.flushWindows()
}

// Closed reports whether the window closed (Close, or its parent closed).
func (c *CodeWindow) Closed() bool { return c.s.gone || c.s.view == nil }

// closeDetached queues the windows opened by code from t for closing, when
// t unmounts (they close with their parent window).
func (t *Tree) closeDetached() {
	for _, s := range slices.Clone(t.wins) {
		if s.detached && !s.gone {
			s.unmount(t)
		}
	}
}
