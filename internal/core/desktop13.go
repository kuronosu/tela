package core

import (
	"io"
	"slices"
	"sync"

	"github.com/kuronosu/tela/internal/platform"
)

// Phase 13 (ADR-213): what package desktop reaches through core besides
// desktop.go: the start of Run (single instance), the services from any
// goroutine (secrets, registration), window-scoped events (menu bars),
// raising a window with an activation token, and the clipboard.

// RunStart is what a function given to OnRunStart receives.
type RunStart struct {
	// Backend is the app's backend: no window is open yet.
	Backend platform.Backend
	// AppID is Options.AppID (tela.App.AppID), "" when not set.
	AppID string
	// Dev reports dev mode.
	Dev bool
	// DevChild: the process is a tela dev child (its backend is the
	// supervisor's remote backend). Reload: it was seeded with the state
	// of the previous build, so it is not the session's first start.
	DevChild, Reload bool
	// Log receives what the function reports before the app's trees
	// exist (its dev warnings).
	Log io.Writer
}

var runStart struct {
	mu sync.Mutex
	fs []func(RunStart) bool
}

// OnRunStart registers f to run in Run after the backend exists and
// before the main window opens (package desktop claims the single
// instance there). f returns false to end Run at once with a nil error,
// opening nothing: the process is a second instance that forwarded its
// launch. Process-wide; safe from any goroutine. telatest does not run
// these functions (it does not call Run).
func OnRunStart(f func(RunStart) bool) {
	runStart.mu.Lock()
	defer runStart.mu.Unlock()
	runStart.fs = append(runStart.fs, f)
}

// runStartHooks runs the functions given to OnRunStart, in order, until
// one returns false.
func runStartHooks(rs RunStart) bool {
	runStart.mu.Lock()
	fs := slices.Clone(runStart.fs)
	runStart.mu.Unlock()
	for _, f := range fs {
		if !f(rs) {
			return false
		}
	}
	return true
}

// RunningServices returns the OS services of the running app for the
// calls that are safe from any goroutine (secrets, registration), the
// app's Desktop, and whether the caller runs on the app's UI thread,
// which is known in dev mode only (false otherwise). ok is false when no
// app runs. Safe from any goroutine; only Desktop.WarnAnyGoroutine may be
// called on d off the UI thread.
func RunningServices() (svc platform.Desktop, d Desktop, onUI, ok bool) {
	posts.mu.Lock()
	t := posts.target
	posts.mu.Unlock()
	if t == nil || t.wv == nil {
		return nil, Desktop{}, false, false
	}
	h := t.wv.host
	return h.services, Desktop{h}, h.uiG != 0 && goid() == h.uiG, true
}

// WarnAnyGoroutine reports a dev warning once per key, like Warn, from
// any goroutine: at once on the UI thread, else through the main tree's
// update queue.
func (d Desktop) WarnAnyGoroutine(key string, msg func() string) {
	h := d.h
	if h.uiG != 0 && goid() == h.uiG {
		h.tree.warnOnce(key, msg)
		return
	}
	h.tree.enqueue(update{post: func() { h.tree.warnOnce(key, msg) }})
}

// winListener is a listener of ListenWindow.
type winListener struct {
	win     *winView
	f       func(platform.Event)
	stopped bool
}

// ListenWindow calls f with the window-scoped desktop events of w's
// window (platform.MenuEvent), on the UI thread, until stop is called. A
// handle of no open window of this app never calls f.
func (d Desktop) ListenWindow(w WindowHandle, f func(platform.Event)) (stop func()) {
	if f == nil {
		panic("core: Desktop.ListenWindow(nil)")
	}
	if w.t == nil || w.t.wv == nil || w.t.wv.host != d.h || !w.t.wv.alive() {
		return func() {}
	}
	dk := d.h.desktop()
	l := &winListener{win: w.t.wv, f: f}
	dk.winListeners = append(dk.winListeners, l)
	return func() {
		if l.stopped {
			return
		}
		l.stopped = true
		dk.winListeners = slices.DeleteFunc(dk.winListeners, func(x *winListener) bool { return x == l })
	}
}

// windowEvent delivers a window-scoped desktop event of v to the
// listeners of package desktop.
func (h *Host) windowEvent(v *winView, e platform.Event) {
	if h.desk == nil {
		return
	}
	for _, l := range slices.Clone(h.desk.winListeners) {
		if !l.stopped && l.win == v {
			l.f(e)
		}
	}
}

// Raise brings w's window to the front for a launch another process
// forwarded: it shows the window if hidden and activates it with the
// launch's activation token (platform.Window.ActivateWithToken, which
// also restores a minimized window). Safe from any goroutine; nothing
// for a window that is not open.
func (d Desktop) Raise(w WindowHandle, token string) {
	w.request(func(v *winView) {
		v.setHidden(false)
		v.win.ActivateWithToken(token)
	})
}

// DevChild reports whether this process is a tela dev child (its
// executable is a temporary dev build). Safe from any goroutine.
func DevChild() bool {
	child, _ := devChildState()
	return child
}

// MainWindow returns the handle of the app's main window.
func (d Desktop) MainWindow() WindowHandle { return WindowHandle{d.h.main.tree} }

// Clipboard returns the backend's clipboard.
func (d Desktop) Clipboard() platform.Clipboard { return d.h.backend.Clipboard() }

// NormDataType returns the MIME type a DataTransfer format names (HTML's
// "text" is platform.MIMEText, "url" and "Files" platform.MIMEURIList;
// spaces are removed), for package desktop's clipboard reads.
func NormDataType(format string) string { return normType(format) }
