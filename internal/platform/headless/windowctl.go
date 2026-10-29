package headless

import (
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// DefaultScreen is the logical size of the simulated screen when
// Options.Screen is zero: what maximized and fullscreen windows get.
var DefaultScreen = platform.Size{W: 1920, H: 1080}

// tokenPrefix starts the restore tokens the headless backend produces.
const tokenPrefix = "headless:"

// windowCtl is a window's control state, guarded by b.mu. The headless
// "window manager" grants every request at once and deterministically.
type windowCtl struct {
	screen   platform.Size        // maximized and fullscreen size
	min, max platform.Size        // current size limits
	osState  platform.WindowState // the "OS" state, maybe not delivered yet
	restore  platform.Size        // the size outside maximized/fullscreen
	visible  bool
	onTop    bool
	icon     []image.Image
	regions  []platform.HitRegion
	token    string
	parent   *Window

	titleBar     platform.TitleBarColors
	backdrop     platform.Backdrop
	styleChanges int // SetTitleBarColors and SetBackdrop calls

	regionChanges int
	activations   int
	captionDrags  int
	windowMenus   int

	// Caption presses: buttons pressed in a caption region and not yet
	// released (their pointer stream belongs to the "OS"), and the last
	// primary caption press for double clicks.
	captionHeld platform.Buttons
	captions    platform.CaptionClicks
}

// big reports whether the window is maximized or fullscreen (its size is
// the screen's, and SetSize changes the size it returns to).
func (c *windowCtl) big() bool { return c.osState.Maximized || c.osState.Fullscreen }

// clamp applies the current size limits.
func (c *windowCtl) clamp(s platform.Size) platform.Size { return platform.ClampSize(s, c.min, c.max) }

// resized returns size clamped to the limits, which becomes the size to
// return to unless the window is maximized or fullscreen (the user or
// window manager resized it there).
func (c *windowCtl) resized(size platform.Size) platform.Size {
	size = c.clamp(size)
	if !c.big() {
		c.restore = size
	}
	return size
}

// size is the size the window has in its current state.
func (c *windowCtl) size() platform.Size {
	switch {
	case c.osState.Fullscreen:
		return c.screen // fullscreen ignores the size limits
	case c.osState.Maximized:
		return c.clamp(c.screen)
	}
	return c.restore
}

// initCtl sets up the control state of a new window from its resolved
// options and returns its initial size; b.mu need not be held (the window
// is not shared yet).
func (w *Window) initCtl(opts platform.WindowOptions, screen platform.Size, n int) platform.Size {
	c := &w.ctl
	c.screen = screen
	c.min, c.max = opts.MinSize, opts.MaxSize
	c.restore = opts.Size
	c.visible = !opts.Hidden
	c.onTop = opts.AlwaysOnTop
	c.titleBar, c.backdrop = opts.TitleBar, opts.Backdrop
	c.osState = platform.WindowState{
		Maximized:             opts.Maximized,
		Fullscreen:            opts.Fullscreen,
		ClientSideDecorations: opts.Frameless,
		HasPosition:           opts.HasPosition,
		Position:              opts.Position,
	}
	if strings.HasPrefix(opts.RestoreToken, tokenPrefix) {
		c.token = opts.RestoreToken
	} else {
		c.token = tokenPrefix + strconv.Itoa(n)
	}
	if opts.Parent != nil {
		p, ok := opts.Parent.(*Window)
		if !ok || p.b != w.b {
			panic(fmt.Sprintf("headless: WindowOptions.Parent is a %T of another backend; the parent must be an open window of the same backend", opts.Parent))
		}
		if p.Closed() {
			panic("headless: WindowOptions.Parent is closed; the parent must be an open window")
		}
		c.parent = p
	}
	w.WindowState.SetHidden(!c.visible)
	return c.size()
}

// setOSState moves the window to state st as the OS would: the state event
// first, then the configure of the new size, and frames paused while
// minimized or hidden. Entering maximized or fullscreen from a normal
// state remembers the size to return to. b.mu must be held.
func (w *Window) setOSState(st platform.WindowState) {
	b, c := w.b, &w.ctl
	if w.WindowState.Closed() || st == c.osState {
		return
	}
	if !c.big() && (st.Maximized || st.Fullscreen) {
		c.restore = w.os.Size
	}
	minimizing := st.Minimized && !c.osState.Minimized
	c.osState = st
	b.loop.UpdateState(w, st)
	if size := c.size(); size != w.os.Size {
		w.os.Size = size
		b.loop.Enqueue(w, platform.ConfigureEvent{Metrics: w.os})
	}
	w.WindowState.SetHidden(!c.visible || st.Minimized)
	if minimizing && w.focused {
		w.queueFocus(false)
	}
	b.signal()
}

// changeState applies change to the window's OS state; see setOSState.
func (w *Window) changeState(change func(*platform.WindowState)) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	st := w.ctl.osState
	change(&st)
	w.setOSState(st)
}

// --- platform.Window: window control ---

// Minimize minimizes the window: frames pause and the focus is lost.
func (w *Window) Minimize() { w.changeState(func(s *platform.WindowState) { s.Minimized = true }) }

// SetMaximized maximizes the window to the screen size (Options.Screen,
// clamped to the size limits) or restores it; it also un-minimizes.
func (w *Window) SetMaximized(maximized bool) {
	w.changeState(func(s *platform.WindowState) { s.Maximized, s.Minimized = maximized, false })
}

// SetFullscreen makes the window fullscreen (the screen size) or leaves
// fullscreen; it also un-minimizes.
func (w *Window) SetFullscreen(fullscreen bool) {
	w.changeState(func(s *platform.WindowState) { s.Fullscreen, s.Minimized = fullscreen, false })
}

// SetVisible shows or hides the window. Hiding a focused window takes its
// focus; showing does not give it (Activate does).
func (w *Window) SetVisible(visible bool) {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	c := &w.ctl
	if w.WindowState.Closed() || c.visible == visible {
		return
	}
	c.visible = visible
	w.WindowState.SetHidden(!visible || c.osState.Minimized)
	if !visible && w.focused {
		w.queueFocus(false)
	}
	b.signal()
}

// SetAlwaysOnTop records the flag (AlwaysOnTop).
func (w *Window) SetAlwaysOnTop(onTop bool) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.WindowState.Closed() {
		w.ctl.onTop = onTop
	}
}

// SetMinSize changes the minimum size; a window outside the new limits is
// resized.
func (w *Window) SetMinSize(size platform.Size) {
	platform.CheckSize("SetMinSize size", size)
	w.setLimits(func(c *windowCtl) { c.min = size })
}

// SetMaxSize changes the maximum size; a window outside the new limits is
// resized.
func (w *Window) SetMaxSize(size platform.Size) {
	platform.CheckSize("SetMaxSize size", size)
	w.setLimits(func(c *windowCtl) { c.max = size })
}

func (w *Window) setLimits(change func(*windowCtl)) {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.WindowState.Closed() {
		return
	}
	c := &w.ctl
	change(c)
	c.restore = c.clamp(c.restore)
	if size := c.size(); size != w.os.Size {
		w.os.Size = size
		b.loop.Enqueue(w, platform.ConfigureEvent{Metrics: w.os})
		b.signal()
	}
}

// SetPosition moves the window; the state reports the position from then
// on (headless knows positions only once the app or SetState set one).
func (w *Window) SetPosition(pos image.Point) {
	w.changeState(func(s *platform.WindowState) { s.HasPosition, s.Position = true, pos })
}

// Activate is always granted: it un-minimizes the window and, if it is
// visible, moves the keyboard focus to it from every other window (with
// the FocusEvents a real window manager would cause). Activations counts
// the calls.
func (w *Window) Activate() {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.WindowState.Closed() {
		return
	}
	w.ctl.activations++
	st := w.ctl.osState
	st.Minimized = false
	w.setOSState(st)
	if !w.ctl.visible || w.focused {
		return
	}
	for _, o := range b.loop.Windows() {
		if o != w && o.focused {
			o.queueFocus(false)
		}
	}
	w.queueFocus(true)
	b.signal()
}

// SetIcon records a copy of the icon (Icon).
func (w *Window) SetIcon(images []image.Image) {
	platform.CheckIcon("SetIcon", images)
	icon := platform.CopyIcon(images)
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.WindowState.Closed() {
		w.ctl.icon = icon
	}
}

// SetTitleBarColors records the colors (TitleBar). The headless window
// draws no chrome, so they have no other effect.
func (w *Window) SetTitleBarColors(colors platform.TitleBarColors) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.WindowState.Closed() {
		w.ctl.titleBar = colors
		w.ctl.styleChanges++
	}
}

// SetBackdrop records the backdrop (Backdrop); it has no other effect.
func (w *Window) SetBackdrop(backdrop platform.Backdrop) {
	platform.CheckBackdrop("SetBackdrop backdrop", backdrop)
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.WindowState.Closed() {
		w.ctl.backdrop = backdrop
		w.ctl.styleChanges++
	}
}

// TitleBar returns the latest SetTitleBarColors (or the option).
func (w *Window) TitleBar() platform.TitleBarColors {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.titleBar
}

// Backdrop returns the latest SetBackdrop (or the option).
func (w *Window) Backdrop() platform.Backdrop {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.backdrop
}

// StyleChanges returns how many times SetTitleBarColors and SetBackdrop
// changed the window after it opened, for tests that check a request is
// made only when a field changes.
func (w *Window) StyleChanges() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.styleChanges
}

// SetHitRegions records the regions (HitRegions, HitRegionChanges); Inject
// honors their caption areas.
func (w *Window) SetHitRegions(regions []platform.HitRegion) {
	platform.CheckHitRegions(regions)
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if w.WindowState.Closed() || platform.EqualHitRegions(w.ctl.regions, regions) {
		return
	}
	w.ctl.regions = slices.Clone(regions)
	w.ctl.regionChanges++
}

// RestoreToken returns "headless:N" (N: the window's creation number in
// its backend), or the headless token given in WindowOptions.RestoreToken.
func (w *Window) RestoreToken() string {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.token
}

// --- caption presses ---

// captionPointer reports whether e belongs to the "OS" because of a
// caption region, and performs the OS action of a press; see Inject.
func (w *Window) captionPointer(e platform.PointerEvent) bool {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	c := &w.ctl
	if w.WindowState.Closed() {
		return false
	}
	if c.captionHeld != 0 {
		switch e.Action {
		case platform.PointerDown:
			c.captionHeld |= e.Button.Mask()
		case platform.PointerUp:
			c.captionHeld &^= e.Button.Mask()
		case platform.PointerCancel:
			c.captionHeld = 0
		}
		return true
	}
	if e.Action != platform.PointerDown || platform.HitTest(c.regions, e.Pos) != platform.HitCaption {
		return false
	}
	c.captionHeld = e.Button.Mask()
	switch e.Button {
	case platform.ButtonPrimary:
		if c.captions.Press(e.Time, e.Pos, b.settings) {
			st := c.osState
			st.Maximized = !st.Maximized
			w.setOSState(st)
			return true
		}
		c.captionDrags++
	case platform.ButtonSecondary:
		c.windowMenus++
	}
	return true
}

// --- injection ---

// SetState simulates the window manager changing the window's state (the
// user maximizing from the OS title bar, minimizing from the task bar,
// moving the window, the compositor switching to client-side
// decorations): a WindowStateEvent is queued if it changed, preceded by a
// ConfigureEvent for maximized and fullscreen changes as on Window's own
// methods.
func (w *Window) SetState(st platform.WindowState) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	w.setOSState(st)
}

// --- inspection ---

// OSState returns the window's current state, including changes not yet
// delivered as a WindowStateEvent (State returns the delivered one).
func (w *Window) OSState() platform.WindowState {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.osState
}

// Visible reports whether the window is shown (not hidden by SetVisible or
// WindowOptions.Hidden; a minimized window is still visible).
func (w *Window) Visible() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.visible
}

// AlwaysOnTop reports the latest SetAlwaysOnTop (or the option).
func (w *Window) AlwaysOnTop() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.onTop
}

// SizeLimits returns the current minimum and maximum sizes.
func (w *Window) SizeLimits() (min, max platform.Size) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.min, w.ctl.max
}

// RestoreSize returns the size the window returns to when it leaves
// maximized and fullscreen (its size when it is in neither).
func (w *Window) RestoreSize() platform.Size {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.restore
}

// Icon returns the icon set by SetIcon (copies, as NRGBA), or nil.
func (w *Window) Icon() []image.Image {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return slices.Clone(w.ctl.icon)
}

// HitRegions returns a copy of the latest regions set by SetHitRegions.
func (w *Window) HitRegions() []platform.HitRegion {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return slices.Clone(w.ctl.regions)
}

// HitRegionChanges returns how many SetHitRegions calls changed the
// regions: what a real backend would have sent to the OS.
func (w *Window) HitRegionChanges() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.regionChanges
}

// Activations returns how many times Activate was called.
func (w *Window) Activations() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.activations
}

// CaptionDrags returns how many injected primary presses in a caption
// region started an OS move (double clicks excluded).
func (w *Window) CaptionDrags() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.captionDrags
}

// WindowMenus returns how many injected secondary presses in a caption
// region opened the OS window menu.
func (w *Window) WindowMenus() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ctl.windowMenus
}

// Parent returns the window's parent (WindowOptions.Parent), or nil.
func (w *Window) Parent() *Window { return w.ctl.parent }

// Children returns the open child windows of w, in creation order.
func (w *Window) Children() []*Window {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*Window
	for _, c := range b.loop.Windows() {
		if c.ctl.parent == w {
			out = append(out, c)
		}
	}
	return out
}

// Blocked reports whether a visible modal child window of w is open: the
// state in which a real backend makes the OS refuse w's input. The
// headless backend still delivers injected input to w, so tests can check
// that core blocks it.
func (w *Window) Blocked() bool {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.loop.Windows() {
		if c.ctl.parent == w && c.opts.Modal && c.ctl.visible {
			return true
		}
	}
	return false
}
