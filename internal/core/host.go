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
	"github.com/kuronosu/tela/internal/gfx/raster"
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
	// Root is the content of the window, usually Component(App).
	Root Node
	// ErrorLog receives platform errors (a failed Present). Nil means
	// stderr.
	ErrorLog io.Writer
}

// Run opens a window on the platform's native backend (platform/auto),
// mounts opts.Root and runs the event loop on the calling goroutine,
// which must be the main goroutine (the package locks it to the main OS
// thread in its init). It returns when the window is closed (app policy,
// ARCHITECTURE section 15) or the platform fails.
//
// Without opts.Fonts, the text uses the embedded fonts plus the system's
// fonts for the scripts the embedded ones cannot draw (ADR-035). Without
// opts.Debug, the debug overlays are the ones TELA_DEBUG names (debug.go).
// opts.Dev (with TELA_DEV) also becomes the process default of dev mode,
// which the node constructors check.
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
	b, err := auto.New()
	if err != nil {
		return err
	}
	h, err := NewHost(b, opts)
	if err != nil {
		return err
	}
	// The window's tree runs the functions given to Post, including the
	// ones posted before Run.
	stop := h.tree.ReceivePosts()
	err = b.Run(h)
	stop()
	h.stopTimer()
	// The loop is over: run the effect cleanups, without waking it or
	// asking the closed window for frames.
	h.tree.SetWake(nil)
	h.tree.SetRequestFrame(nil)
	h.tree.Unmount()
	return err
}

func errorLog(w io.Writer) io.Writer {
	if w == nil {
		return os.Stderr
	}
	return w
}

// Host connects a Tree to a platform window: it handles the backend's
// events (platform.Handler), runs the pipeline on frame events, rasterizes
// into a framebuffer it reuses, and presents. Frames are requested only
// when the tree has work (ARCHITECTURE section 8: no change, no frame).
type Host struct {
	backend platform.Backend
	win     platform.Window
	tree    *Tree
	log     io.Writer

	fb         *image.RGBA
	renderer   raster.Renderer
	scale      float64
	configured bool // a ConfigureEvent arrived since the last Present
	// FullFrames rasterizes every frame fully, as before damage tracking
	// (benchmarks and debugging).
	FullFrames bool
	// AfterFrame, when set, runs after each frame that rasterized,
	// presented or not (tests check the window's pixels there).
	AfterFrame func()
	presents   int

	// cursor is the cursor last set on the window.
	cursor platform.Cursor
	// imeField is the text field whose state the window's text input
	// has (Tree.TextInput), nil for none.
	imeField any
	// timer wakes the loop at timerAt (backend clock) for the gesture
	// arena (a long press); nil when nothing waits.
	timerMu sync.Mutex
	timer   *time.Timer
	timerAt time.Duration
}

// NewHost creates a window on backend b and mounts opts.Root in a new
// Tree. Pass the Host to b.Run.
func NewHost(b platform.Backend, opts Options) (*Host, error) {
	win, err := b.NewWindow(platform.WindowOptions{
		Title: opts.Title,
		AppID: opts.AppID,
		Size:  platform.Size{W: opts.Width, H: opts.Height},
	})
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
	// The frame clock drives the tree from its first frame on, so the
	// animations its first builds start (UseSpring) wait for it instead
	// of jumping to their targets as in a tree without a clock (Render).
	t.anim.driven = true
	h := &Host{backend: b, win: win, tree: t, log: errorLog(opts.ErrorLog)}
	t.SetWake(b.Wake)
	t.SetClipboard(b.Clipboard())
	// The window has keyboard focus from its first FocusEvent on.
	t.SetWindowFocused(false)
	t.Mount(opts.Root)
	// The first frame follows the window's first ConfigureEvent.
	t.SetRequestFrame(win.RequestFrame)
	return h, nil
}

// Tree returns the host's tree.
func (h *Host) Tree() *Tree { return h.tree }

// Window returns the host's window.
func (h *Host) Window() platform.Window { return h.win }

// Presents returns the number of frames presented.
func (h *Host) Presents() int { return h.presents }

// HandleEvent implements platform.Handler.
func (h *Host) HandleEvent(w platform.Window, e platform.Event) {
	switch e := e.(type) {
	case platform.WakeEvent:
		// State setters or the gesture timer woke the loop: apply the
		// updates, which requests a frame if a component must build, and
		// let a long press that is due fire.
		h.tree.ApplyUpdates()
		h.tick(h.backend.Now())
		return
	case platform.SettingsEvent:
		h.tree.SetEnvironment(e.Settings.DarkMode, e.Settings.ReducedMotion)
		return
	}
	if w != h.win {
		return
	}
	switch e := e.(type) {
	case platform.ConfigureEvent:
		// A FrameEvent follows every ConfigureEvent.
		h.configured = true
		h.tree.SetViewport(gfx.Size{W: e.Metrics.Size.W, H: e.Metrics.Size.H})
		h.tree.SetScale(float32(e.Metrics.Scale))
	case platform.FrameEvent:
		h.tick(e.Time)
		// Phase 2 of the pipeline: scroll animations, flings and fades
		// advance with the frame clock.
		h.tree.Animate(e.Time)
		h.frame(e.Time)
		h.updateCursor()
		h.syncTextInput()
		h.armTimer()
	case platform.PointerEvent:
		// A long press due before the event fires first.
		h.tick(e.Time)
		h.tree.HandlePointer(e)
		h.updateCursor()
		h.syncTextInput()
		h.armTimer()
	case platform.ScrollEvent:
		h.tick(e.Time)
		h.tree.HandleScroll(e)
		h.updateCursor()
		h.armTimer()
	case platform.KeyEvent:
		h.tree.HandleKey(e)
		h.updateCursor()
		h.syncTextInput()
		h.armTimer()
	case platform.IMEEvent:
		h.tree.HandleIME(e)
		h.syncTextInput()
		h.armTimer()
	case platform.IMESessionEvent:
		h.tree.HandleIMESession(e.Active)
		h.syncTextInput()
		h.armTimer()
	case platform.FocusEvent:
		h.tree.SetWindowFocused(e.Focused)
		h.syncTextInput()
		h.armTimer()
	case platform.CloseRequestedEvent:
		h.win.Close()
		h.backend.Quit()
	}
}

// tick lets the gesture arena act on the deadlines at or before now (a
// long press), then arms the timer for the next one.
func (h *Host) tick(now time.Duration) {
	if at, ok := h.tree.NextDeadline(); ok && at <= now {
		h.tree.TickGestures(now)
		h.updateCursor()
	}
	h.armTimer()
}

// armTimer arms one wake-up for the gesture arena's next deadline, or
// stops the timer when nothing waits (an idle app has no timer). The
// platform layer has no timer API: a goroutine timer calls Wake, and the
// WakeEvent compares the deadline with the backend clock, so a timer that
// fires early only re-arms.
func (h *Host) armTimer() {
	at, ok := h.tree.NextDeadline()
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

// TimerArmed reports whether the host waits for a gesture deadline, and
// when (backend clock).
func (h *Host) TimerArmed() (at time.Duration, ok bool) {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()
	return h.timerAt, h.timer != nil
}

// syncTextInput gives the window the text input state of the tree's
// focused text field (Tree.TextInput): nil first when the field changed,
// so the IME restarts its session for the new field (ADR-068), and a
// reset when core finished a composition itself. Backends ignore equal
// states.
func (h *Host) syncTextInput() {
	key, st, reset := h.tree.TextInput()
	if key != h.imeField {
		if h.imeField != nil {
			h.win.SetTextInput(nil)
		}
		h.imeField = key
	} else if reset && st != nil {
		h.win.ResetTextInput()
	}
	h.win.SetTextInput(st)
}

// updateCursor sets the window's cursor when the tree's changed.
func (h *Host) updateCursor() {
	if c := h.tree.Cursor(); c != h.cursor {
		h.cursor = c
		h.win.SetCursor(c)
	}
}

// frame runs the pipeline, rasterizes at the window's scale when anything
// changed, presents, and then runs the passive effects (UseEffect). now is
// the frame clock of the FrameEvent. With debug overlays on (debug.go), a
// frame also runs when only an overlay changed, and the next one is
// requested while an overlay needs it.
func (h *Host) frame(now time.Duration) {
	m := h.win.Metrics()
	phys := m.Physical()
	if phys.X <= 0 || phys.Y <= 0 {
		return
	}
	var kind DebugFrameKind
	var start time.Duration
	if dbg := h.tree.Debug(); dbg != 0 {
		if dbg&DebugFPS != 0 {
			start = h.tree.DebugNow()
		}
		defer func() {
			if dbg&DebugFPS != 0 {
				h.tree.DebugRecordFrame(now, h.tree.DebugNow()-start, kind)
			}
			if h.tree.DebugWantsFrame() {
				h.win.RequestFrame()
			}
		}()
	}
	h.tree.SetScale(float32(m.Scale)) // box edges snap to device pixels
	_, changed := h.tree.Frame()
	defer h.tree.FlushEffects()
	full := false
	if want := image.Rect(0, 0, phys.X, phys.Y); h.fb == nil || h.fb.Rect != want {
		h.fb = image.NewRGBA(want)
		changed, full = true, true
	}
	if m.Scale != h.scale {
		h.scale = m.Scale
		changed, full = true, true
	}
	if !changed && !h.configured && !h.tree.DebugPending() {
		return // nothing new to show
	}
	// The framebuffer keeps the previous frame: only the damage is
	// rasterized (layer.go), and presented. The first Present after a
	// ConfigureEvent is full for the platform anyway.
	d := h.tree.RasterizeFrame(h.fb, &h.renderer, full || h.FullFrames)
	if h.tree.Debug() != 0 {
		switch app := h.tree.LastDamage(); {
		case app.Full:
			kind = DebugFrameFull
		case !app.Empty():
			kind = DebugFrameIncremental
		}
	}
	if h.AfterFrame != nil {
		defer h.AfterFrame()
	}
	var damage []image.Rectangle
	if !h.configured {
		if d.Empty() {
			return // nothing visible changed
		}
		damage = d.Changed()
	}
	h.configured = false
	if err := h.win.Present(h.fb, damage); err != nil {
		fmt.Fprintln(h.log, "tela: present:", err)
		return
	}
	h.presents++
}

// NeedsFrame reports that a frame must still render: a ConfigureEvent
// arrived whose frame did not run yet.
func (h *Host) NeedsFrame() bool { return h.configured }
