package core

import (
	"fmt"
	"image"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/auto"
	"github.com/kuronosu/tela/internal/text"
)

func init() {
	// The UI thread must be the main OS thread (Cocoa requires it, Win32
	// requires the thread that created the windows). Locking in init
	// keeps the main goroutine on the main thread until Run.
	runtime.LockOSThread()
}

// Options configures Run and NewHost.
type Options struct {
	Config
	// Title is the window title.
	Title string
	// AppID identifies the application to the desktop (see
	// platform.WindowOptions.AppID).
	AppID string
	// Width and Height are the requested logical size of the window; zero
	// means platform.DefaultWindowSize.
	Width, Height float32
	// MinWidth, MinHeight, MaxWidth and MaxHeight limit the window's size
	// (zero: no limit on that side); FixedSize, Frameless, AlwaysOnTop
	// and Hidden are those of WindowSpec; Icon is the window icon at
	// several sizes; Placement is where the window opens (a saved
	// WindowPlacement, or zero).
	MinWidth, MinHeight, MaxWidth, MaxHeight  float32
	FixedSize, Frameless, AlwaysOnTop, Hidden bool
	Icon                                      []image.Image
	Placement                                 WindowPlacement
	// TitleBar are the colors of the OS title bar and border, Backdrop the
	// system material behind the window (Windows 11; ADR-187).
	TitleBar TitleBarColors
	Backdrop Backdrop
	// OnClose runs when the user asks to close the main window (the OS's
	// close button, a WindowClose control, WindowHandle.Close). Nil quits
	// the app.
	OnClose func(WindowHandle)
	// Root is the content of the window, usually Component(App).
	Root Node
	// DevSave and DevRestore are the app's state outside the tree for
	// tela dev (tela.App.DevState): used only by a teladev build running
	// under tela dev, ignored otherwise (ADR-114).
	DevSave    func() ([]byte, error)
	DevRestore func([]byte) error
	// ErrorLog receives platform errors (a failed Present). Nil means
	// stderr.
	ErrorLog io.Writer
}

// mainSpec returns the WindowSpec of the main window. Options repeats the
// WindowSpec fields so that its literals stay flat; a field added to
// WindowSpec goes here, in Options, tela.App and tela.Window too
// (TestOptionsCarryEveryWindowField and tela's TestWindowFieldsReachCore
// fail until it does).
func (o *Options) mainSpec() WindowSpec {
	return WindowSpec{
		Title: o.Title, Width: o.Width, Height: o.Height,
		MinWidth: o.MinWidth, MinHeight: o.MinHeight, MaxWidth: o.MaxWidth, MaxHeight: o.MaxHeight,
		FixedSize: o.FixedSize, Frameless: o.Frameless, AlwaysOnTop: o.AlwaysOnTop, Hidden: o.Hidden,
		Icon: o.Icon, Placement: o.Placement, TitleBar: o.TitleBar, Backdrop: o.Backdrop,
	}
}

// Run opens a window on the platform's native backend (platform/auto),
// mounts opts.Root and runs the event loop on the calling goroutine,
// which must be the main goroutine (the package locks it to the main OS
// thread in its init). It returns when the app quits (closing the main
// window without Options.OnClose, or Quit; ARCHITECTURE section 15) or
// the platform fails, after unmounting every window.
//
// Without opts.Fonts, the text uses the embedded fonts plus the system's
// fonts for the scripts the embedded ones cannot draw (ADR-035). Without
// opts.Debug, the debug overlays are the ones TELA_DEBUG names (debug.go).
// opts.Dev (with TELA_DEV) also becomes the process default of dev mode,
// which the node constructors called outside any build check (devNow).
func Run(opts Options) error {
	runtime.LockOSThread()
	// The app's dev mode is the process default of the node constructors
	// (one app per process; ADR-085).
	devAny.Store(configDev(opts.Dev))
	if opts.Fonts == nil {
		fonts := text.NewDefaultCollection()
		if err := fonts.EnableSystemFonts(text.SystemFontOptions{}); err != nil {
			fmt.Fprintln(errorLog(opts.ErrorLog), "tela: system fonts disabled:", err)
		}
		opts.Fonts = fonts
	}
	if devStateEnabled {
		// tela dev: report a panic of the UI thread to the supervisor
		// and exit with status 2 (ADR-114).
		defer devRecover()
	}
	var b platform.Backend
	var err error
	if devStateEnabled {
		// Under tela dev the backend is the supervisor's window, not a
		// native one; nil, nil when the process is not a tela dev child.
		if b, err = devBackend(opts); err != nil {
			return err
		}
	}
	if b == nil {
		if b, err = auto.New(); err != nil {
			return err
		}
	}
	h, err := NewHost(b, opts)
	if err != nil {
		return err
	}
	// The window's tree runs the functions given to Post, including the
	// ones posted before Run.
	stop := h.tree.ReceivePosts()
	if devStateEnabled {
		err = devRun(b, h)
	} else {
		err = b.Run(h)
	}
	stop()
	h.stopTimer()
	h.end()
	runEndHooks() // desktop.go: package desktop writes what it holds
	return err
}

// end runs after the loop: the windows unmount, running the cleanups of
// their effects, without waking the loop, asking for frames or calling
// the backend (it is gone once Run returned).
func (h *Host) end() {
	h.ended = true
	for _, v := range h.views {
		v.tree.SetWake(nil)
		v.tree.SetRequestFrame(nil)
	}
	h.tree.Unmount() // the child windows close with it (window.go)
}

func errorLog(w io.Writer) io.Writer {
	if w == nil {
		return os.Stderr
	}
	return w
}

// Host connects the Trees of an app to its platform windows: it handles
// the backend's events (platform.Handler), runs the pipeline of a window
// on its frame events, rasterizes into a framebuffer it reuses per
// window, and presents. Frames are requested only when a tree has work
// (ARCHITECTURE section 8: no change, no frame). The main window is the
// Host's own; the windows of Window components are opened and closed by
// their trees (window.go).
type Host struct {
	backend platform.Backend
	// win and tree are the main window's.
	win  platform.Window
	tree *Tree
	log  io.Writer

	// main is the main window's view; views are the open windows, the
	// main window first, then the others in the order they opened.
	main  *winView
	views []*winView
	// appID is the AppID of every window; onClose is Options.OnClose.
	appID   string
	onClose func(WindowHandle)
	// ended: Run's loop is over, the backend must not be called.
	ended bool
	// serial counts the windows opened (UseID prefixes of their trees).
	serial int
	// uiG is the UI thread's goroutine in dev mode (0 otherwise), for the
	// check of OpenURL and OpenPath.
	uiG uint64
	// later is work deferred to the next WakeEvent (a window that must not
	// close under its own tree's dispatch, window.go).
	later []func()
	// desk is package desktop's state (desktop.go), nil until used.
	desk *hostDesktop

	// FullFrames rasterizes every frame fully, as before damage tracking
	// (benchmarks and debugging).
	FullFrames bool
	// AfterFrame, when set, runs after each frame of the main window that
	// rasterized, presented or not (tests check the window's pixels
	// there).
	AfterFrame func()

	// timer wakes the loop at timerAt (backend clock) for the gesture
	// arenas (a long press); nil when nothing waits.
	timerMu sync.Mutex
	timer   *time.Timer
	timerAt time.Duration
}

// NewHost creates the main window on backend b and mounts opts.Root in a
// new Tree. Pass the Host to b.Run.
func NewHost(b platform.Backend, opts Options) (*Host, error) {
	spec := opts.mainSpec()
	checkWindowSpec("tela.App", "", &spec)
	wo := spec.options(opts.AppID, nil)
	win, err := b.NewWindow(wo)
	if err != nil {
		return nil, err
	}
	s := b.Settings()
	cfg := opts.Config
	cfg.Dark, cfg.ReducedMotion = s.DarkMode, s.ReducedMotion
	if cfg.Debug == 0 {
		// TELA_DEBUG selects overlays unless the options say otherwise
		// (DebugOff). A malformed value is reported once and its valid
		// flags still apply.
		var err error
		if cfg.Debug, err = DebugFromEnv(); err != nil {
			fmt.Fprintln(errorLog(opts.ErrorLog), "tela:", err)
		}
	}
	t := NewTree(cfg)
	if devStateEnabled {
		t.devAdoptSeed() // tela dev: the state given to SetDevSeed (ADR-113)
	}
	// The frame clock drives the tree from its first frame on, so the
	// animations its first builds start (UseSpring) wait for it instead
	// of jumping to their targets as in a tree without a clock (Render).
	t.anim.driven = true
	h := &Host{backend: b, win: win, tree: t, log: errorLog(opts.ErrorLog), appID: opts.AppID, onClose: opts.OnClose}
	if t.dev {
		h.uiG = goid()
	}
	h.main = h.newView(win, t, nil, &spec, wo)
	h.views = []*winView{h.main}
	t.SetWake(b.Wake)
	t.SetClipboard(b.Clipboard())
	// The window has keyboard focus from its first FocusEvent on.
	t.SetWindowFocused(false)
	if len(spec.Icon) > 0 {
		win.SetIcon(spec.Icon)
	}
	t.Mount(opts.Root)
	t.windowStateValue()
	// The first frame follows the window's first ConfigureEvent.
	t.SetRequestFrame(win.RequestFrame)
	return h, nil
}

// Tree returns the main window's tree.
func (h *Host) Tree() *Tree { return h.tree }

// Window returns the main window.
func (h *Host) Window() platform.Window { return h.win }

// Presents returns the number of frames presented in the main window.
func (h *Host) Presents() int { return h.main.presents }

// HandleEvent implements platform.Handler. After each event, the windows
// that get no frames commit the work it left them (paused.go).
func (h *Host) HandleEvent(w platform.Window, e platform.Event) {
	h.handle(w, e)
	h.commitPaused()
}

// handle handles one event of the backend.
func (h *Host) handle(w platform.Window, e platform.Event) {
	switch e := e.(type) {
	case platform.WakeEvent:
		// State setters or the gesture timer woke the loop: apply the
		// updates of every window's tree, which requests a frame of the
		// windows whose components must build, and let a long press that
		// is due fire.
		if len(h.later) > 0 {
			later := h.later
			h.later = nil
			for _, f := range later {
				f()
			}
		}
		for i := 0; i < len(h.views); i++ {
			h.views[i].tree.ApplyUpdates()
		}
		h.tick(h.backend.Now())
		return
	case platform.SettingsEvent:
		for i := 0; i < len(h.views); i++ {
			h.views[i].tree.SetEnvironment(e.Settings.DarkMode, e.Settings.ReducedMotion)
		}
		return
	case platform.NotificationEvent, platform.TrayEvent:
		h.appEvent(e) // package desktop's listeners (desktop.go)
		return
	}
	v := h.main
	if w != v.win {
		if v = h.viewOf(w); v == nil {
			return // a closed window
		}
	}
	t := v.tree
	switch e := e.(type) {
	case platform.ConfigureEvent:
		// A FrameEvent follows every ConfigureEvent.
		v.configured, v.sized = true, true
		if s := e.Metrics.Size; s.W > 0 && s.H > 0 {
			// An empty size is a window the OS does not show (Win32 reports
			// a minimized window's client area as 0x0): the tree keeps the
			// last size, which its commits without frames lay out at.
			t.SetViewport(gfx.Size{W: s.W, H: s.H})
		}
		t.SetScale(float32(e.Metrics.Scale))
		v.notePlacement(e.Metrics.Size)
		v.placementMoved()
	case platform.WindowStateEvent:
		v.os, v.opening = e.State, false
		v.notePlacement(platform.Size{})
		v.placementMoved()
		v.syncState()
	case platform.FrameEvent:
		// The state of a new window comes before its first frame, if
		// it comes at all (an OS that refused to maximize sends none).
		v.opening = false
		h.tick(e.Time)
		// Phase 2 of the pipeline: scroll animations, flings and fades
		// advance with the frame clock.
		t.Animate(e.Time)
		h.frame(v, e.Time)
		if devStateEnabled && v == h.main {
			devAfterFrame(h) // tela dev: the state report
		}
		h.updateCursor(v)
		h.syncTextInput(v)
		h.syncRegions(v)
		h.armTimer()
	case platform.PointerEvent:
		// A long press due before the event fires first.
		h.tick(e.Time)
		if h.cut(v, e.Action == platform.PointerDown, e.Time) {
			return
		}
		t.HandlePointer(e)
		h.updateCursor(v)
		h.syncTextInput(v)
		h.armTimer()
	case platform.ScrollEvent:
		h.tick(e.Time)
		if h.cut(v, false, e.Time) {
			return
		}
		t.HandleScroll(e)
		h.updateCursor(v)
		h.armTimer()
	case platform.KeyEvent:
		if h.cut(v, false, e.Time) {
			return
		}
		t.HandleKey(e)
		h.updateCursor(v)
		h.syncTextInput(v)
		h.armTimer()
	case platform.IMEEvent:
		if h.cut(v, false, h.backend.Now()) {
			return
		}
		t.HandleIME(e)
		h.syncTextInput(v)
		h.armTimer()
	case platform.IMESessionEvent:
		t.HandleIMESession(e.Active)
		h.syncTextInput(v)
		h.armTimer()
	case platform.FocusEvent:
		v.focused = e.Focused
		t.SetWindowFocused(e.Focused)
		v.syncState()
		h.syncTextInput(v)
		h.armTimer()
	case platform.CloseRequestedEvent:
		if len(h.views) > 1 {
			if m := h.modalOf(v); m != nil {
				m.win.Activate() // a modal window is open over it
				return
			}
		}
		h.requestClose(v)
	}
}

// tick lets the gesture arenas act on the deadlines at or before now (a
// long press), then arms the timer for the next one.
func (h *Host) tick(now time.Duration) {
	for i := 0; i < len(h.views); i++ {
		v := h.views[i]
		if at, ok := v.tree.NextDeadline(); ok && at <= now {
			v.tree.TickGestures(now)
			h.updateCursor(v)
		}
	}
	h.armTimer()
}

// nextDeadline returns the earliest deadline of the windows' trees, and
// of the commits that windows without frames wait for (paused.go).
func (h *Host) nextDeadline() (at time.Duration, ok bool) {
	for _, v := range h.views {
		if d, has := v.tree.NextDeadline(); has && (!ok || d < at) {
			at, ok = d, true
		}
	}
	// A commit due now runs after the event being handled (commitPaused):
	// only a later one needs the timer.
	if d, has := h.PausedWork(); has && d > h.backend.Now() && (!ok || d < at) {
		at, ok = d, true
	}
	return at, ok
}

// armTimer arms one wake-up for the gesture arenas' next deadline (or the
// next paced commit of a window without frames), or
// stops the timer when nothing waits (an idle app has no timer). The
// platform layer has no timer API: a goroutine timer calls Wake, and the
// WakeEvent compares the deadline with the backend clock, so a timer that
// fires early only re-arms.
func (h *Host) armTimer() {
	at, ok := h.nextDeadline()
	h.timerMu.Lock()
	defer h.timerMu.Unlock()
	if !ok {
		if h.timer != nil {
			h.timer.Stop()
			h.timer = nil
		}
		return
	}
	if h.timer != nil && h.timerAt == at {
		return
	}
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timerAt = at
	h.timer = time.AfterFunc(max(at-h.backend.Now(), 0), h.backend.Wake)
}

// stopTimer stops the gesture timer.
func (h *Host) stopTimer() {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
}

// TimerArmed reports whether the host waits for a gesture deadline or a
// paced commit of a window without frames, and when (backend clock).
func (h *Host) TimerArmed() (at time.Duration, ok bool) {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()
	return h.timerAt, h.timer != nil
}

// syncTextInput gives window v the text input state of its tree's
// focused text field (Tree.TextInput): nil first when the field changed,
// so the IME restarts its session for the new field (ADR-068), and a
// reset when core finished a composition itself. Backends ignore equal
// states.
func (h *Host) syncTextInput(v *winView) {
	key, st, reset := v.tree.TextInput()
	if key != v.imeField {
		if v.imeField != nil {
			v.win.SetTextInput(nil)
		}
		v.imeField = key
	} else if reset && st != nil {
		v.win.ResetTextInput()
	}
	v.win.SetTextInput(st)
}

// updateCursor sets window v's cursor when its tree's changed.
func (h *Host) updateCursor(v *winView) {
	if c := v.tree.Cursor(); c != v.cursor {
		v.cursor = c
		v.win.SetCursor(c)
	}
}

// syncRegions gives window v the hit regions of its custom title bar
// (winregion.go) after a frame that ran the pipeline, when they changed.
// A tree without title bar props costs one comparison.
func (h *Host) syncRegions(v *winView) {
	t := v.tree
	if t.reg.n == 0 && len(v.regions) == 0 {
		return
	}
	if f := t.stats.Frames; v.regInit && f == v.regFrame {
		return
	}
	v.regInit, v.regFrame = true, t.stats.Frames
	out := t.hitRegions(v.regBuf[:0])
	v.regBuf = out
	if platform.EqualHitRegions(out, v.regions) {
		return
	}
	v.regions = append(v.regions[:0], out...)
	if len(v.regions) == 0 {
		v.win.SetHitRegions(nil)
		return
	}
	v.win.SetHitRegions(v.regions)
}

// frame runs window v's pipeline, rasterizes at the window's scale when
// anything changed, presents, and then runs the passive effects
// (UseEffect). now is the frame clock of the FrameEvent. With debug
// overlays on (debug.go), a frame also runs when only an overlay changed,
// and the next one is requested while an overlay needs it.
func (h *Host) frame(v *winView, now time.Duration) {
	m := v.win.Metrics()
	phys := m.Physical()
	if phys.X <= 0 || phys.Y <= 0 {
		return
	}
	t := v.tree
	var kind DebugFrameKind
	var start time.Duration
	if dbg := t.Debug(); dbg != 0 {
		if dbg&DebugFPS != 0 {
			start = t.DebugNow()
		}
		defer func() {
			if dbg&DebugFPS != 0 {
				t.DebugRecordFrame(now, t.DebugNow()-start, kind)
			}
			if t.DebugWantsFrame() {
				v.win.RequestFrame()
			}
		}()
	}
	t.SetScale(float32(m.Scale)) // box edges snap to device pixels
	_, changed := t.Frame()
	defer t.FlushEffects()
	full := false
	if want := image.Rect(0, 0, phys.X, phys.Y); v.fb == nil || v.fb.Rect != want {
		v.fb = image.NewRGBA(want)
		changed, full = true, true
	}
	if m.Scale != v.scale {
		v.scale = m.Scale
		changed, full = true, true
	}
	if v.stale {
		// The tree committed without frames (paused.go) since fb was
		// rasterized: fb is brought to the current frame whole.
		v.stale = false
		changed, full = true, true
	}
	if !changed && !v.configured && !t.DebugPending() {
		return // nothing new to show
	}
	// The framebuffer keeps the previous frame: only the damage is
	// rasterized (layer.go), and presented. The first Present after a
	// ConfigureEvent is full for the platform anyway.
	d := t.RasterizeFrame(v.fb, &v.renderer, full || h.FullFrames)
	if t.Debug() != 0 {
		switch app := t.LastDamage(); {
		case app.Full:
			kind = DebugFrameFull
		case !app.Empty():
			kind = DebugFrameIncremental
		}
	}
	if h.AfterFrame != nil && v == h.main {
		defer h.AfterFrame()
	}
	var damage []image.Rectangle
	if !v.configured {
		if d.Empty() {
			return // nothing visible changed
		}
		damage = d.Changed()
	}
	v.configured = false
	if err := v.win.Present(v.fb, damage); err != nil {
		fmt.Fprintln(h.log, "tela: present:", err)
		return
	}
	v.presents++
}

// NeedsFrame reports that a frame must still render: a ConfigureEvent
// arrived whose frame did not run yet, in any window that gets frames (the
// platform paces neither hidden nor minimized windows: their frame waits
// until they are shown; their builds and effects do not, see PausedWork).
func (h *Host) NeedsFrame() bool {
	for _, v := range h.views {
		if v.configured && !v.hidden && !v.os.Minimized {
			return true
		}
	}
	return false
}
