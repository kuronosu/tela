package core

import (
	"errors"
	"fmt"
	"image"
	"math"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
)

// Windows (ARCHITECTURE sections 6 and 15; ADR-178, ADR-180).
//
// An app has one element tree and one window per Window component, after
// Flutter's multi-view: the main window is the Host's, and a Window
// component (tela.Window) opens a window while it is mounted and closes it
// when it unmounts. Each window has a Tree of its own: its box tree,
// viewport and scale, layers, damage and framebuffer, focus, hover and
// pointer state, gesture arena, portals and overlay stack, cursor, text
// input and semantics, all as they are for one window. The Trees of an
// app are linked into one element tree:
//
//   - The root of a child window's Tree hangs below the Window element
//     that opened it (Tree.outer): UseContext finds the Providers above
//     the Window, and a Provider's new value builds its consumers in the
//     child window again, in that window's Tree.
//   - The Trees of an app track Value reads together (Tree.trk, the main
//     window's): a component of a child window that reads a Value of the
//     main window's tree builds again, and is laid out or repainted, in
//     its window when the Value changes.
//   - State flows down as props: the Window's Root is built by its parent,
//     so a closure over the parent's state is the child window's content.
//     Each Tree has its own update queue (the setters of its components),
//     all woken by the backend's Wake, which applies every queue.
//   - The theme and the class aliases (one parser) and the color scheme
//     are the app's: a child window starts with its parent's and follows
//     SetTheme and SetColorScheme (setParser, setScheme). The scale and
//     the viewport are each window's.
//
// A Window opens, changes and closes once the build lock is released
// (flushWindows, after the build phase, Mount and Unmount), since a child
// window's Tree builds under that lock too. A Tree without a window (Render,
// tela snapshot, tela inspect) opens no window: only the main window's
// content is rendered there. The fields of a mounted Window are applied
// when they change (title, sizes, always-on-top, visibility, icon, root
// class); FixedSize, Frameless and Modal are fixed when it opens.
//
// Modality: a visible Modal window blocks its parent window, the window
// it was declared in. Core drops the parent's input itself, whatever the
// backend does (ADR-179): presses, moves, scrolling, keys and the IME,
// after cancelling the pointers it held; a press there activates the
// modal window, and a close request there is refused.

// WindowSpec describes what every window has: the main window
// (Options.Window, from tela.App) and the windows opened by a Window
// component (tela.Window), which add ChildWindowSpec's fields.
type WindowSpec struct {
	// Title is the window title.
	Title string
	// Width and Height are the logical size of the content area; zero
	// means platform.DefaultWindowSize.
	Width, Height float32
	// MinWidth, MinHeight, MaxWidth and MaxHeight limit the size; zero is
	// no limit on that side.
	MinWidth, MinHeight, MaxWidth, MaxHeight float32
	// FixedSize keeps the user from resizing the window; Frameless removes
	// the OS title bar and borders; AlwaysOnTop keeps it above other
	// windows; Hidden keeps it hidden.
	FixedSize, Frameless, AlwaysOnTop, Hidden bool
	// Icon is the window icon at several sizes; empty is the default.
	Icon []image.Image
	// TitleBar are the colors of the OS title bar and border, Backdrop the
	// system material behind the window (Windows 11; ADR-187).
	TitleBar TitleBarColors
	Backdrop Backdrop
	// Placement is where the window opens: a saved placement, or zero.
	Placement WindowPlacement
}

// ChildWindowSpec describes a child window, the one a Window component
// (tela.Window, UseWindowSpec) or OpenWindow opens: a WindowSpec plus
// what only a child window has. The main window has the same things
// elsewhere: Config.RootClass, Options.Root and Options.OnClose; it is
// never modal.
type ChildWindowSpec struct {
	WindowSpec
	// Modal blocks the parent window while the child is visible; it is
	// fixed when the window opens.
	Modal bool
	// RootClass are the classes of the window's root.
	RootClass string
	// Root is the content of the window.
	Root Node
	// OnClose runs when the user asks to close the window.
	OnClose func()
}

// WindowState is the state of a window that a component can react to:
// UseWindow's State reads it, and a component that read it in its build
// builds again when it changes.
type WindowState struct {
	// Maximized, Fullscreen and Minimized are the window's OS state as
	// the OS reported it (Wayland cannot report minimized windows).
	Maximized, Fullscreen, Minimized bool
	// Focused: the window has the keyboard focus (it is the active
	// window).
	Focused bool
	// Hidden: the app hid the window (Hide, or Hidden).
	Hidden bool
	// ClientSideDecorations: the OS draws no title bar or borders for the
	// window (a Frameless window, or a Wayland compositor without
	// server-side decorations), so the app draws its own title bar.
	ClientSideDecorations bool
}

// WindowPlacement is where a window is: its size outside maximized and
// fullscreen, its position where the OS reports one, whether it is
// maximized or fullscreen, and the OS's session restore token. Save it
// when the app quits and give it back as the window's initial placement:
// the window opens where it was. The zero value is "nothing known".
type WindowPlacement struct {
	// Width and Height are the logical size of the content area outside
	// maximized and fullscreen (the size it returns to); zero is
	// unknown.
	Width, Height float32
	// X and Y are the top-left of the content area in physical pixels of
	// the OS's desktop, when HasPosition is set (never on Wayland, which
	// does not let apps place windows; RestoreToken does it there where
	// the compositor supports it).
	X, Y        int
	HasPosition bool
	// Maximized and Fullscreen are the state to open in.
	Maximized, Fullscreen bool
	// RestoreToken is an opaque token the OS uses to put the window back
	// ("" where none is needed).
	RestoreToken string
}

// WindowControl is what a window-control button of a custom title bar
// does when it is clicked (WindowControlProp).
type WindowControl uint8

const (
	// WindowMinimize minimizes the window.
	WindowMinimize WindowControl = iota + 1
	// WindowMaximize maximizes the window, or restores a maximized one.
	WindowMaximize
	// WindowClose asks to close the window, as the OS's close button
	// does: the window's OnClose runs (for the main window, App.OnClose,
	// or the app quits).
	WindowClose
)

func (c WindowControl) String() string {
	switch c {
	case WindowMinimize:
		return "WindowMinimize"
	case WindowMaximize:
		return "WindowMaximize"
	case WindowClose:
		return "WindowClose"
	}
	return "WindowControl(" + strconv.Itoa(int(c)) + ")"
}

// ---------------------------------------------------------------------
// The view of a window.

// winView is one window of a Host: the platform window, the Tree shown in
// it and what the Host keeps per window (framebuffer, cursor, text input,
// hit regions, state). UI thread, except dead and place (under mu).
type winView struct {
	host *Host
	win  platform.Window
	tree *Tree
	// slot is the Window component that opened the window, nil for the
	// main window; parent the window it was declared in.
	slot   *windowSlot
	parent *winView
	modal  bool

	// The pipeline (host.go).
	fb         *image.RGBA
	renderer   raster.Renderer
	scale      float64
	configured bool // a ConfigureEvent arrived since the last Present
	presents   int
	// Commits without frames (paused.go): sized, a ConfigureEvent arrived
	// (the tree has the window's size); stale, the tree painted since fb
	// was last rasterized, so the next frame rasterizes fully; commitAt,
	// the backend time before which no other such commit runs.
	sized, stale bool
	commitAt     time.Duration
	// cursor is the cursor last set on the window; imeField the text
	// field whose state the window's text input has (Tree.TextInput).
	cursor   platform.Cursor
	imeField any

	// regions are the hit regions sent last (winregion.go), regBuf the
	// scratch list they are computed into, regFrame the frame count of
	// the last computation.
	regions, regBuf []platform.HitRegion
	regFrame        int
	regInit         bool

	// os is the OS state delivered last (seeded with what the options
	// imply), focused the window focus, hidden whether the app hid it,
	// blocked whether its input is cut by a modal child.
	os      platform.WindowState
	focused bool
	hidden  bool
	blocked bool
	// opening: the window was asked to open maximized or fullscreen and
	// its first state has not arrived (the first ConfigureEvent comes
	// before it, with the big size, which is not the size to save).
	opening bool
	// dead: the window closed; requests of handles are dropped.
	dead atomic.Bool

	mu    sync.Mutex
	place WindowPlacement
	// watch holds the placement listeners (WatchPlacement, desktop.go).
	watch *placeWatch
}

// newView returns the view of window win, opened with options wo (the
// resolved size is the placement's until a ConfigureEvent says more).
func (h *Host) newView(win platform.Window, t *Tree, slot *windowSlot, spec *WindowSpec, wo platform.WindowOptions) *winView {
	v := &winView{host: h, win: win, tree: t, slot: slot, hidden: spec.Hidden, opening: wo.Maximized || wo.Fullscreen}
	v.os.ClientSideDecorations = spec.Frameless
	size := wo.Resolve().Size
	v.place = WindowPlacement{
		Width: size.W, Height: size.H,
		X: wo.Position.X, Y: wo.Position.Y, HasPosition: wo.HasPosition,
		Maximized: wo.Maximized, Fullscreen: wo.Fullscreen,
		RestoreToken: win.RestoreToken(),
	}
	t.wv = v
	v.syncCanvas()
	return v
}

// state returns the window's WindowState.
func (v *winView) state() WindowState {
	return WindowState{
		Maximized: v.os.Maximized, Fullscreen: v.os.Fullscreen, Minimized: v.os.Minimized,
		Focused: v.focused, Hidden: v.hidden, ClientSideDecorations: v.os.ClientSideDecorations,
	}
}

// syncState gives the readers of UseWindow's State the window's state
// (they build again when it changed).
func (v *winView) syncState() {
	if s := v.tree.wstate; s != nil {
		if st := v.state(); st != *s.cur.Load() {
			s.storeUI(st)
		}
	}
}

// notePlacement records the window's placement after a ConfigureEvent
// (size) or a WindowStateEvent (zero size): the size and the position
// only while it is neither maximized, fullscreen nor minimized.
func (v *winView) notePlacement(size platform.Size) {
	st := v.os
	token := v.win.RestoreToken()
	v.mu.Lock()
	defer v.mu.Unlock()
	p := &v.place
	p.RestoreToken = token
	if v.opening {
		return // the placement asked for stands until the state arrives
	}
	if !st.Maximized && !st.Fullscreen && !st.Minimized {
		if size.W > 0 && size.H > 0 {
			p.Width, p.Height = size.W, size.H
		}
		if st.HasPosition {
			p.X, p.Y, p.HasPosition = st.Position.X, st.Position.Y, true
		}
	}
	p.Maximized, p.Fullscreen = st.Maximized, st.Fullscreen
}

// setHidden shows or hides the window.
func (v *winView) setHidden(hidden bool) {
	if v.hidden == hidden {
		return
	}
	v.hidden = hidden
	v.win.SetVisible(!hidden)
	v.syncState()
}

// alive reports whether requests may still reach the window.
func (v *winView) alive() bool { return !v.dead.Load() && !v.host.ended }

// ---------------------------------------------------------------------
// UseWindow and the handle.

// WindowHandle is a window of the app: the one a component renders in
// (UseWindow), or the main window given to Options.OnClose. It is a small
// comparable value, the same for the window's whole life. Its requests
// (Minimize, Maximize, Hide, Close...) are safe from any goroutine: they
// are queued with the state updates of the window's tree and made on the
// UI thread, in order with them. The OS may refuse a request (a tiling
// window manager does not maximize); State tells what happened. A handle
// of a closed window, or of a tree without a window (tela snapshot), does
// nothing.
type WindowHandle struct{ t *Tree }

// windowUse is the slot of UseWindow.
type windowUse struct{}

// UseWindow returns the handle of the window the calling component
// renders in: the main window, or the window of the Window component
// above it.
func UseWindow() WindowHandle {
	el, i := nextSlot(hookWindow)
	if _, ok := existingSlot[*windowUse](el, i, "UseWindow"); !ok {
		el.hooks = append(el.hooks, &windowUse{})
	}
	el.tree.windowStateValue()
	return WindowHandle{el.tree}
}

// windowStateValue returns the Value State reads, made on first use (UI
// thread), owned by the root.
func (t *Tree) windowStateValue() *Value[WindowState] {
	if t.wstate == nil && t.root != nil {
		t.wstate = newValue(t.root, t.windowState())
	}
	return t.wstate
}

// windowState returns the state of the tree's window; a tree without one
// is a focused window (what tela snapshot shows).
func (t *Tree) windowState() WindowState {
	if v := t.wv; v != nil {
		return v.state()
	}
	return WindowState{Focused: t.foc.window}
}

// IsZero reports whether w is the zero WindowHandle (no window).
func (w WindowHandle) IsZero() bool { return w.t == nil }

// State returns the window's state. Read in a component's build, it
// subscribes the component: it builds again when the state changes.
// Elsewhere (handlers, effects, goroutines) it only reads it.
func (w WindowHandle) State() WindowState {
	t := w.t
	if t == nil {
		return WindowState{}
	}
	if s := t.wstate; s != nil {
		return s.Get()
	}
	return WindowState{Focused: true}
}

// Placement returns where the window is (WindowPlacement): what an app
// saves to open the window there next time. It is kept after the window
// closes. Safe from any goroutine.
func (w WindowHandle) Placement() WindowPlacement {
	if w.t == nil {
		return WindowPlacement{}
	}
	v := w.t.wv
	if v == nil {
		return WindowPlacement{}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.place
}

// request queues f for the UI thread, which runs it if the window is
// still open.
func (w WindowHandle) request(f func(v *winView)) {
	t := w.t
	if t == nil || t.wv == nil || t.wv.dead.Load() { // any goroutine: dead is atomic
		return
	}
	t.enqueue(update{post: func() {
		if v := t.wv; v.alive() {
			f(v)
		}
	}})
}

// Minimize asks the OS to minimize the window; Focus brings it back.
func (w WindowHandle) Minimize() { w.request(func(v *winView) { v.win.Minimize() }) }

// Maximize asks the OS to maximize the window.
func (w WindowHandle) Maximize() { w.request(func(v *winView) { v.win.SetMaximized(true) }) }

// Unmaximize asks the OS to restore a maximized window to its size.
func (w WindowHandle) Unmaximize() { w.request(func(v *winView) { v.win.SetMaximized(false) }) }

// ToggleMaximize maximizes the window, or restores it when it is
// maximized (what a title bar's maximize button does).
func (w WindowHandle) ToggleMaximize() {
	w.request(func(v *winView) { v.win.SetMaximized(!v.os.Maximized) })
}

// SetFullscreen asks the OS to make the window fullscreen or to leave
// fullscreen.
func (w WindowHandle) SetFullscreen(fullscreen bool) {
	w.request(func(v *winView) { v.win.SetFullscreen(fullscreen) })
}

// Hide hides the window. It stays open (mounted, with its state and
// effects) until it is shown again or closed; the app keeps running while
// its windows are hidden ("close to tray"). A hidden window draws nothing,
// but its components still build and its effects run; it draws the
// current state when it is shown.
func (w WindowHandle) Hide() { w.request(func(v *winView) { v.setHidden(true) }) }

// Show shows a hidden window and activates it.
func (w WindowHandle) Show() {
	w.request(func(v *winView) {
		v.setHidden(false)
		v.win.Activate()
	})
}

// Focus asks the OS to raise the window and give it the keyboard focus,
// restoring it if it is minimized. The OS may refuse (focus stealing
// prevention) and flash the window instead.
func (w WindowHandle) Focus() { w.request(func(v *winView) { v.win.Activate() }) }

// Close asks to close the window, as the user does with the title bar's
// close button: the window's OnClose runs (it removes the Window from the
// state that renders it); for the main window App.OnClose runs, or the app
// quits.
func (w WindowHandle) Close() { w.request(func(v *winView) { v.host.requestClose(v) }) }

// ---------------------------------------------------------------------
// Window components.

// windowSlot is the hook slot of a Window component: the window it opened
// and the spec of its last build (UI thread).
type windowSlot struct {
	el *Element
	// spec is the latest build's, applied the spec last applied to the
	// window.
	spec, applied ChildWindowSpec
	view          *winView
	// queued: in Tree.winQ; tried: opening was tried (it failed, or the
	// tree has no window); gone: the component unmounted; deferred: an
	// update waits in Host.later for the window's tree to leave its
	// dispatch or frame (updateWindow).
	queued, tried, gone, deferred bool
	// detached: opened by code (OpenWindow, desktop.go), not by a Window
	// component; el is then the parent tree's root.
	detached bool
}

func (s *windowSlot) unmount(t *Tree) {
	s.gone = true
	if i := slices.Index(t.wins, s); i >= 0 {
		t.wins = slices.Delete(t.wins, i, i+1)
	}
	t.queueWindow(s)
}

// UseWindowSpec opens the window spec describes while the calling
// component is mounted, keeps it up to date with the spec of every build
// and closes it when the component unmounts: the build of tela.Window.
func UseWindowSpec(spec ChildWindowSpec) {
	el, i := nextSlot(hookWindowSpec)
	t := el.tree
	s, ok := existingSlot[*windowSlot](el, i, "tela.Window")
	if !ok {
		s = &windowSlot{el: el}
		el.hooks = append(el.hooks, s)
		t.wins = append(t.wins, s)
	}
	if spec.Root.d == nil && t.dev {
		panic(fmt.Sprintf("tela: tela.Window %q has no Root: give it the content of the window, such as tela.Window{Title: %q, Root: Settings{}} for a struct component or Root: tela.C(settings) for a function", spec.Title, spec.Title))
	}
	checkWindowSpec("tela.Window", spec.Title, &spec.WindowSpec)
	s.spec = spec
	t.queueWindow(s)
}

// queueWindow queues s for flushWindows.
func (t *Tree) queueWindow(s *windowSlot) {
	if !s.queued {
		s.queued = true
		t.winQ = append(t.winQ, s)
	}
}

// flushWindows opens, updates and closes the windows of the Window
// components queued by the builds, once the build lock is released (a
// child window's Tree builds under it).
func (t *Tree) flushWindows() {
	for len(t.winQ) > 0 {
		q := t.winQ
		t.winQ = nil
		for _, s := range q {
			s.queued = false
			switch {
			case s.gone:
				t.closeWindow(s)
			case s.view != nil:
				t.updateWindow(s)
			case !s.tried:
				t.openWindow(s)
			}
		}
	}
}

// openWindow opens the window of Window component s, the child of the
// tree's window, with a Tree of its own below the Window element.
func (t *Tree) openWindow(s *windowSlot) {
	if s.spec.Root.d == nil {
		return // nothing to show yet (a release build; dev mode panics)
	}
	s.tried = true
	pv := t.wv
	if pv == nil || !pv.alive() {
		return // no window system (Render, tela snapshot)
	}
	h := pv.host
	spec := s.spec
	wo := spec.options(h.appID, pv.win)
	var win platform.Window
	var err error
	if devStateEnabled {
		win, err = devNewWindow(h.backend, wo, s.el) // tela dev: keyed by its element path (ADR-184)
	} else {
		win, err = h.backend.NewWindow(wo)
	}
	if err != nil {
		fmt.Fprintf(h.log, "tela: cannot open the window %q: %v\n", spec.Title, err)
		return
	}
	h.serial++
	ct := NewTree(t.childConfig(spec.RootClass, h.serial))
	ct.trk = t.trk
	ct.outer = s.el
	ct.anim.driven = true
	v := h.newView(win, ct, s, &spec.WindowSpec, wo)
	v.parent, v.modal = pv, wo.Modal
	s.view, s.applied = v, spec
	h.views = append(h.views, v)
	ct.SetWake(h.backend.Wake)
	ct.SetClipboard(h.backend.Clipboard())
	ct.SetWindowFocused(false)
	if len(spec.Icon) > 0 {
		win.SetIcon(spec.Icon)
	}
	ct.Mount(spec.Root)
	ct.windowStateValue()
	ct.SetRequestFrame(win.RequestFrame)
	if t.dev && spec.OnClose == nil {
		t.warnOnce("window-onclose:"+spec.Title, func() string {
			return fmt.Sprintf("tela: tela.Window %q has no OnClose, so closing it from its title bar does nothing: the window stays open while your state renders it. Set OnClose to update that state, such as OnClose: func() { setOpen(false) }, or to func() {} to keep the window open on purpose", spec.Title)
		})
	}
}

// childConfig returns the configuration of a child window's Tree: the
// parent's fonts, parser (theme and class aliases), environment, dev mode
// and messages, and an ID prefix of its own.
func (t *Tree) childConfig(rootClass string, n int) Config {
	debug := t.dbg.flags
	if debug == 0 {
		debug = DebugOff
	}
	return Config{
		Fonts: t.fonts, Parser: t.parser, RootClass: rootClass,
		Dark: t.env.sysDark, ReducedMotion: t.env.sysReduced, ColorScheme: t.env.scheme,
		Dev: t.dev, Log: t.log, Warn: t.warn,
		IDPrefix:          t.idPrefix + "w" + strconv.Itoa(n) + "-",
		ClassicScrollbars: t.scr.classic, InstantScroll: t.scr.instant,
		Debug: debug, DebugClock: t.dbg.clock,
	}
}

// updateWindow applies the fields of Window component s that changed
// since its window opened or was last updated, and builds its content
// again (Tree.Mount reconciles it).
func (t *Tree) updateWindow(s *windowSlot) {
	v := s.view
	if ct := v.tree; ct.handlerDepth > 0 || ct.inFrame {
		// The window's own tree is running a handler or a frame (code of
		// the child window built the parent synchronously, such as an
		// ElementRef.Click on the parent's element): Mount would rebuild
		// the elements its dispatch or pipeline is walking, so the window
		// is updated at the next wake, with the latest spec, as
		// closeWindow closes it.
		if !s.deferred {
			s.deferred = true
			h := v.host
			h.later = append(h.later, func() {
				if s.deferred {
					s.deferred = false
					if s.view != nil && !s.gone {
						t.updateWindow(s)
					}
				}
			})
			h.backend.Wake()
		}
		return
	}
	s.deferred = false
	old, spec := s.applied, s.spec
	s.applied = spec
	if v.alive() {
		win := v.win
		if spec.Title != old.Title {
			win.SetTitle(spec.Title)
		}
		if spec.MinWidth != old.MinWidth || spec.MinHeight != old.MinHeight {
			win.SetMinSize(platform.Size{W: spec.MinWidth, H: spec.MinHeight})
		}
		if spec.MaxWidth != old.MaxWidth || spec.MaxHeight != old.MaxHeight {
			win.SetMaxSize(platform.Size{W: spec.MaxWidth, H: spec.MaxHeight})
		}
		if (spec.Width != old.Width || spec.Height != old.Height) && spec.Width > 0 && spec.Height > 0 {
			win.SetSize(platform.Size{W: spec.Width, H: spec.Height})
		}
		if spec.AlwaysOnTop != old.AlwaysOnTop {
			win.SetAlwaysOnTop(spec.AlwaysOnTop)
		}
		if spec.Hidden != old.Hidden {
			v.setHidden(spec.Hidden)
		}
		if !sameIcon(spec.Icon, old.Icon) {
			win.SetIcon(spec.Icon)
		}
		if spec.TitleBar != old.TitleBar {
			win.SetTitleBarColors(spec.TitleBar.platform())
		}
		if spec.Backdrop != old.Backdrop {
			win.SetBackdrop(spec.Backdrop)
			v.syncCanvas()
		}
	}
	if t.dev && (spec.FixedSize != old.FixedSize || spec.Frameless != old.Frameless || spec.Modal != old.Modal) {
		t.warnOnce("window-fixed:"+spec.Title, func() string {
			return fmt.Sprintf("tela: tela.Window %q changed FixedSize, Frameless or Modal while open, which only apply when a window opens: the open window keeps its old ones. Give the Window a new key (tela.WithKey) to open it again with the new ones", spec.Title)
		})
	}
	ct := v.tree
	if spec.RootClass != old.RootClass {
		ct.rootClasses = nil
		if spec.RootClass != "" {
			ct.rootClasses = []string{spec.RootClass}
		}
	}
	ct.Mount(spec.Root)
}

// closeWindow closes the window of Window component s, which unmounted:
// its tree unmounts (its effects' cleanups run, its own child windows
// close first) and the platform window closes.
func (t *Tree) closeWindow(s *windowSlot) {
	v := s.view
	if v == nil {
		return
	}
	if ct := v.tree; ct.handlerDepth > 0 || ct.inFrame {
		// The window's own tree is running a handler or a frame (code of
		// the child window built the parent synchronously, such as an
		// ElementRef.Click on the parent's element): it closes at the next
		// wake, not under it.
		v.host.later = append(v.host.later, func() { t.closeWindow(s) })
		v.host.backend.Wake()
		return
	}
	s.view = nil
	v.dead.Store(true)
	h := v.host
	ct := v.tree
	ct.SetRequestFrame(nil)
	ct.Unmount()
	ct.SetWake(nil)
	h.removeView(v)
	if !h.ended {
		v.win.Close()
	}
}

// sameIcon reports whether a and b hold the same images.
func sameIcon(a, b []image.Image) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !safeEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// options returns the platform options of the child window spec
// describes, a child of parent (nil for a top-level window).
func (s *ChildWindowSpec) options(appID string, parent platform.Window) platform.WindowOptions {
	o := s.WindowSpec.options(appID, parent)
	o.Modal = s.Modal && parent != nil
	return o
}

// options returns the platform options of the window spec describes, a
// child of parent (nil for a top-level window); never modal.
func (s *WindowSpec) options(appID string, parent platform.Window) platform.WindowOptions {
	o := platform.WindowOptions{
		Title:       s.Title,
		AppID:       appID,
		Size:        platform.Size{W: s.Width, H: s.Height},
		MinSize:     platform.Size{W: s.MinWidth, H: s.MinHeight},
		MaxSize:     platform.Size{W: s.MaxWidth, H: s.MaxHeight},
		FixedSize:   s.FixedSize,
		AlwaysOnTop: s.AlwaysOnTop,
		Frameless:   s.Frameless,
		Hidden:      s.Hidden,
		TitleBar:    s.TitleBar.platform(),
		Backdrop:    s.Backdrop,
		Parent:      parent,
	}
	p := s.Placement
	if validSize(p.Width) && validSize(p.Height) && p.Width > 0 && p.Height > 0 {
		o.Size = platform.Size{W: p.Width, H: p.Height}
	}
	o.HasPosition, o.Position = p.HasPosition, image.Point{X: p.X, Y: p.Y}
	o.Maximized, o.Fullscreen = p.Maximized, p.Fullscreen
	o.RestoreToken = p.RestoreToken
	return o
}

// validSize reports whether v is a size the platform accepts.
func validSize(v float32) bool { return v >= 0 && v <= 1e7 && !math.IsNaN(float64(v)) }

// checkWindowSpec panics with an actionable message on sizes the platform
// would reject; kind and title name the window (tela.App, or tela.Window
// and its title). A saved Placement with a bad size is ignored instead
// (options).
func checkWindowSpec(kind, title string, s *WindowSpec) {
	what := func() string {
		if kind == "tela.App" {
			return kind
		}
		return fmt.Sprintf("%s %q", kind, title)
	}
	for _, f := range [...]struct {
		name string
		v    float32
	}{{"Width", s.Width}, {"Height", s.Height}, {"MinWidth", s.MinWidth}, {"MinHeight", s.MinHeight}, {"MaxWidth", s.MaxWidth}, {"MaxHeight", s.MaxHeight}} {
		if !validSize(f.v) {
			panic(fmt.Sprintf("tela: %s has %s %v; sizes are logical pixels, finite and not negative (0 means the default, or no limit)", what(), f.name, f.v))
		}
	}
	if !s.Backdrop.Valid() {
		panic(fmt.Sprintf("tela: %s has Backdrop %v; the valid backdrops are %s", what(), s.Backdrop, backdropList()))
	}
	if (s.Width == 0) != (s.Height == 0) {
		panic(fmt.Sprintf("tela: %s sets only one of Width and Height (%vx%v); set both, or neither for the default %vx%v", what(), s.Width, s.Height, platform.DefaultWindowSize.W, platform.DefaultWindowSize.H))
	}
}

// ---------------------------------------------------------------------
// Closing, quitting and modality (Host).

// requestClose runs what closing window v does: the user asked (the OS's
// close button, a WindowClose control, WindowHandle.Close). A child
// window runs its OnClose (nothing without one); the main window runs
// Options.OnClose, or the app quits.
func (h *Host) requestClose(v *winView) {
	if v == h.main {
		if h.onClose != nil {
			h.onClose(WindowHandle{v.tree})
			return
		}
		// The default: the main window closes (its children with it) and
		// the app quits; Run unmounts the trees.
		if !h.ended {
			v.dead.Store(true)
			v.win.Close()
		}
		h.Quit()
		return
	}
	if s := v.slot; s != nil && s.spec.OnClose != nil {
		// OnClose belongs to the component that declared the Window: it
		// runs with that tree's updates (on the next wake or frame), never
		// inside the child window's own dispatch, which a synchronous
		// unmount of the window would cut short.
		s.el.tree.enqueue(update{post: func() {
			if f := s.spec.OnClose; f != nil && !s.gone {
				f()
			}
		}})
	}
}

// Quit makes Run return: the loop ends and Run unmounts the windows,
// running the cleanups of their effects. Call it on the UI thread
// (core.Quit routes it there from any goroutine).
func (h *Host) Quit() {
	if !h.ended {
		h.backend.Quit()
	}
}

// Quit quits the running app (the Tree that receives Post, Run's): Run
// returns after unmounting its windows, which runs the cleanups of their
// effects. Safe from any goroutine; it is applied in order with the state
// updates queued before it. Before the app runs, it is applied when it
// starts (Run then returns at once).
func Quit() {
	postUpdate(update{postTree: func(t *Tree) {
		if v := t.wv; v != nil {
			v.host.Quit()
		}
	}})
}

// modalOf returns the visible modal child window of v, or nil.
func (h *Host) modalOf(v *winView) *winView {
	for _, c := range h.views[1:] {
		if c.parent == v && c.modal && !c.hidden {
			return c
		}
	}
	return nil
}

// cut reports whether an input event of window v (press: a pointer
// press) must be dropped because a modal child window blocks v. The first
// event dropped cancels the pointer sequences v's tree was following; a
// press activates the modal window, as the OS does.
func (h *Host) cut(v *winView, press bool, now time.Duration) bool {
	if len(h.views) == 1 {
		return false
	}
	m := h.modalOf(v)
	if m == nil {
		v.blocked = false
		return false
	}
	if !v.blocked {
		v.blocked = true
		v.tree.cancelPointers(now)
	}
	if press {
		m.win.Activate()
	}
	return true
}

// cancelPointers ends the pointer sequences the tree follows, as the OS
// does when it takes the input away: a pointercancel for the pointers
// that are pressed, a leave for those inside the window.
func (t *Tree) cancelPointers(now time.Duration) {
	keys := make([]pointerKey, 0, len(t.in.ptrs))
	for k := range t.in.ptrs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].kind != keys[j].kind {
			return keys[i].kind < keys[j].kind
		}
		return keys[i].id < keys[j].id
	})
	for _, k := range keys {
		ps := t.in.ptrs[k]
		if ps == nil {
			continue
		}
		pos := platform.Point{X: ps.pos.X, Y: ps.pos.Y}
		if ps.buttons != 0 || ps.pressing {
			t.HandlePointer(platform.PointerEvent{Action: platform.PointerCancel, Kind: k.kind, ID: k.id, Pos: pos, Time: now})
		}
		if ps := t.in.ptrs[k]; ps != nil && ps.inside {
			t.HandlePointer(platform.PointerEvent{Action: platform.PointerLeave, Kind: k.kind, ID: k.id, Pos: pos, Time: now})
		}
	}
}

// removeView forgets the closed window v.
func (h *Host) removeView(v *winView) {
	if i := slices.Index(h.views, v); i > 0 {
		h.views = slices.Delete(h.views, i, i+1)
	}
}

// viewOf returns the open window w, or nil.
func (h *Host) viewOf(w platform.Window) *winView {
	for _, v := range h.views {
		if v.win == w {
			return v
		}
	}
	return nil
}

// Windows returns the open windows: the main window, then the windows of
// Window components in the order they opened.
func (h *Host) Windows() []platform.Window {
	out := make([]platform.Window, len(h.views))
	for i, v := range h.views {
		out[i] = v.win
	}
	return out
}

// TreeOf returns the Tree shown in open window w, or nil.
func (h *Host) TreeOf(w platform.Window) *Tree {
	if v := h.viewOf(w); v != nil {
		return v.tree
	}
	return nil
}

// Handle returns the handle of open window w (the zero handle for an
// unknown window).
func (h *Host) Handle(w platform.Window) WindowHandle {
	if v := h.viewOf(w); v != nil {
		return WindowHandle{v.tree}
	}
	return WindowHandle{}
}

// ---------------------------------------------------------------------
// The default application.

// runningHost returns the Host of the running app (the Tree that receives
// Post, Run's or telatest's), or an error naming what for.
func runningHost(what string) (*Host, error) {
	posts.mu.Lock()
	t := posts.target
	posts.mu.Unlock()
	if t == nil || t.wv == nil {
		return nil, fmt.Errorf("tela: %s needs a running app: call it from a handler or an effect of an app started with tela.Run", what)
	}
	h := t.wv.host
	if h.ended {
		return nil, fmt.Errorf("tela: %s after the app quit", what)
	}
	if h.uiG != 0 && goid() != h.uiG {
		panic(fmt.Sprintf("tela: %s called from a goroutine other than the UI thread; call it in a handler or an effect, or from a goroutine with tela.Post(func() { ... })", what))
	}
	return h, nil
}

// parentWindow is the window a request of the app comes from: the focused
// window, else the main window.
func (h *Host) parentWindow() platform.Window {
	for _, v := range h.views {
		if v.focused {
			return v.win
		}
	}
	return h.main.win
}

// OpenURL opens uri (https:, mailto:, any scheme the desktop has an
// application for) in the user's default application: the browser, the
// mail client. A URL without a scheme or a file: URL is an error (open
// local files with OpenPath). Call it on the UI thread (a handler, an
// effect); in dev mode another goroutine panics with the fix.
func OpenURL(uri string) error {
	u, err := url.Parse(uri)
	switch {
	case err != nil:
		return fmt.Errorf("tela: OpenURL(%q): %w", uri, err)
	case u.Scheme == "":
		return fmt.Errorf("tela: OpenURL(%q): the URL has no scheme; write it whole, such as \"https://%s\"", uri, strings.TrimPrefix(uri, "//"))
	case strings.EqualFold(u.Scheme, "file"):
		return fmt.Errorf("tela: OpenURL(%q) is a file URL; open local files and folders with tela.OpenPath", uri)
	}
	h, err := runningHost("OpenURL")
	if err != nil {
		return err
	}
	if err := h.backend.Desktop().OpenURI(h.parentWindow(), uri); err != nil {
		return fmt.Errorf("tela: OpenURL(%q): %w", uri, err)
	}
	return nil
}

// OpenPath opens a local file in its default application, or a folder in
// the file manager. Call it on the UI thread (a handler, an effect); in
// dev mode another goroutine panics with the fix.
func OpenPath(path string) error {
	if path == "" {
		return errors.New("tela: OpenPath(\"\"): give the path of a file or a folder")
	}
	h, err := runningHost("OpenPath")
	if err != nil {
		return err
	}
	if err := h.backend.Desktop().OpenPath(h.parentWindow(), path); err != nil {
		return fmt.Errorf("tela: OpenPath(%q): %w", path, err)
	}
	return nil
}
