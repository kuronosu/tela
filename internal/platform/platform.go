package platform

import (
	"errors"
	"image"
	"image/color"
	"math"
	"time"
)

// ErrUnavailable is wrapped by backend constructors when the backend cannot
// run in this environment (no WAYLAND_DISPLAY, no X server, wrong OS). The
// backend selector uses errors.Is(err, ErrUnavailable) to fall back to the
// next candidate; any other error is reported to the user.
var ErrUnavailable = errors.New("platform: backend unavailable")

// Backend is one platform implementation (wayland, x11, win32, cocoa,
// headless). See the package documentation for the loop and threading
// contract.
//
// Except where noted, methods must be called on the UI thread: the goroutine
// that calls (or will call) Run, locked to the OS main thread.
type Backend interface {
	// Name returns the backend name: "wayland", "x11", "win32", "cocoa" or
	// "headless".
	Name() string

	// NewWindow creates a window. It may be called before Run or from the
	// handler. The window is not shown until its first Present. Its first
	// event is a ConfigureEvent with the size the OS actually granted,
	// which may differ from opts.Size (tiling window managers). Invalid
	// options panic; OS failures are returned.
	NewWindow(opts WindowOptions) (Window, error)

	// Run runs the event loop, delivering events to h on the calling
	// goroutine, which must be the UI thread. It blocks until Quit is
	// called (nil) or the platform connection fails (non-nil error). It
	// does not return when the last window closes: the app decides when
	// to Quit. Run may be called once per Backend; when it returns, the
	// backend's OS resources are released and the backend must not be
	// used again.
	Run(h Handler) error

	// Quit makes Run return after the current handler call finishes. Events
	// not yet delivered are discarded. Safe from any goroutine; idempotent.
	Quit()

	// Wake makes the loop deliver a WakeEvent soon. Safe from any
	// goroutine, never blocks, coalesces (see WakeEvent).
	Wake()

	// Now returns the backend's monotonic clock, the time base of
	// FrameEvent.Time and input event timestamps. Its origin is
	// unspecified (headless: 0 at creation). Safe from any goroutine.
	Now() time.Duration

	// Clipboard returns the system clipboard: text, and Data of any MIME
	// type (see "Drag and drop and the clipboard" in the package
	// documentation).
	Clipboard() Clipboard

	// Settings returns the current system settings. Changes are reported
	// by SettingsEvent.
	Settings() SystemSettings

	// Desktop returns the OS services: opening URIs and files in other
	// applications, file dialogs, message boxes, notifications and tray
	// icons; global shortcuts, secrets, single instance and scheme and
	// file-type registration (see [Desktop] and "Desktop services" in the
	// package documentation). It is never nil; a service the backend
	// lacks returns [ErrUnsupported].
	Desktop() Desktop
}

// Window is a top-level window. Methods must be called on the UI thread.
// After Close, every method is a no-op (Present returns nil, Metrics keeps
// its last value) and no more events are delivered for the window.
//
// Window values are comparable and can be used as map keys.
type Window interface {
	// Metrics returns the logical size and scale delivered by the latest
	// ConfigureEvent; it is the zero Metrics before the first one. Size the
	// framebuffer with Metrics().Physical().
	Metrics() Metrics

	// RequestFrame asks for a FrameEvent. Requests coalesce: any number of
	// calls before the next FrameEvent yield one FrameEvent. The backend
	// delivers it when it is a good time to draw (vsync, frame callback);
	// if the window has not drawn recently it is delivered on the next
	// loop iteration. Without requests, no FrameEvents are delivered
	// (except after a ConfigureEvent), so an idle app costs 0% CPU.
	RequestFrame()

	// Present shows a new frame. See the package documentation for the
	// full contract. In short:
	//   - fb is premultiplied RGBA8 (Go's image.RGBA convention), sRGB, in
	//     physical pixels. fb.Rect must be exactly
	//     image.Rect(0, 0, Metrics().Physical()); otherwise Present panics.
	//   - damage lists the rectangles (physical pixels, fb coordinates)
	//     that changed since the previous Present to this window. nil or
	//     empty means the whole frame. Pixels outside the damage are not
	//     read: they keep their previous content. Rectangles are clipped
	//     to fb.Rect. The first Present after a ConfigureEvent is always
	//     treated as full, whatever damage says.
	//   - The backend copies what it needs before returning; the caller
	//     may reuse or modify fb immediately.
	// Windows are opaque unless they implement AlphaPresenter and report
	// true (a Windows 11 client-area backdrop); otherwise backends may
	// ignore alpha. Errors are resource
	// errors (buffer allocation, lost connection).
	Present(fb *image.RGBA, damage []image.Rectangle) error

	// SetTitle sets the window title.
	SetTitle(title string)

	// SetCursor sets the cursor shown while the pointer is over the
	// content area. Invalid values panic.
	SetCursor(c Cursor)

	// SetSize asks the OS to resize the content area to size (logical
	// pixels). The OS may refuse or pick another size; the outcome, if
	// any, arrives as a ConfigureEvent.
	SetSize(size Size)

	// SetTextInput tells the input method about the window's focused text
	// field: its caret area, purpose and surrounding text (see
	// [TextInputState] and "Text input and IME" in the package
	// documentation). nil means no text field has focus, which turns the
	// IME off for the window. Call it whenever the state changes; calls
	// with an equal state are free, and backends send at most one update
	// per loop iteration. The backend keeps a copy. Invalid states panic
	// ([CheckTextInputState]).
	SetTextInput(state *TextInputState)

	// ResetTextInput tells the IME that the app ended the composition in
	// progress itself (a click elsewhere in the field, a programmatic
	// value change; the app has already committed or removed the
	// preedit), so the IME must drop its own and start afresh from the
	// current state. It does nothing without a text input state.
	ResetTextInput()

	// Close destroys the window. Pending events for it are dropped. The
	// window's child windows (WindowOptions.Parent) are closed first, in
	// creation order: Win32 destroys owned windows with their owner, and
	// every backend behaves the same.
	Close()

	// --- Window control (phase 12, ADR-178; "Window control" in the
	// package documentation). Requests the OS may refuse: the outcome, if
	// any, arrives as a WindowStateEvent and/or a ConfigureEvent. ---

	// State returns the state delivered by the latest WindowStateEvent;
	// the zero WindowState before the first one.
	State() WindowState

	// Minimize asks the OS to minimize (iconify) the window. Activate
	// brings it back. Wayland cannot report the result
	// (WindowState.Minimized stays false there).
	Minimize()

	// SetMaximized asks the OS to maximize the window or restore it from
	// maximized.
	SetMaximized(maximized bool)

	// SetFullscreen asks the OS to make the window fullscreen (on its
	// current output) or leave fullscreen.
	SetFullscreen(fullscreen bool)

	// SetVisible shows or hides the window. A hidden window keeps
	// everything (size, state, title, retained frame) and gets no
	// FrameEvents; frame requests made while it is hidden are delivered
	// once it is shown again. Hiding is what "close to tray" uses. A
	// window created with WindowOptions.Hidden is shown by
	// SetVisible(true) (at its first Present after that).
	SetVisible(visible bool)

	// SetAlwaysOnTop asks the OS to keep the window above normal windows.
	// Wayland has no protocol for it (no-op).
	SetAlwaysOnTop(onTop bool)

	// SetMinSize and SetMaxSize change the limits of the content size
	// (logical pixels; zero, or a zero component, means no limit on that
	// axis), with WindowOptions.MinSize and MaxSize's rules. If the
	// current size is outside the new limits, the backend asks the OS for
	// the clamped size. Invalid sizes panic ([CheckSize]).
	SetMinSize(size Size)
	SetMaxSize(size Size)

	// SetPosition asks the OS to move the window so that the top-left of
	// its content area is at pos, in the OS's desktop coordinates
	// (physical pixels; points on Cocoa; the same space as
	// WindowState.Position). Wayland
	// clients cannot place windows: no-op there (see "Session restore").
	SetPosition(pos image.Point)

	// Activate asks the OS to raise the window, give it keyboard focus
	// and restore it if minimized. The OS may refuse (focus stealing
	// prevention) and flash or mark the window instead; a granted
	// activation arrives as a FocusEvent. It does not show a hidden
	// window.
	Activate()

	// SetIcon sets the window's icon (title bar, task bar, alt-tab) from
	// the same picture at several sizes; the backend picks what the OS
	// needs ([PickIcon]). nil or empty restores the default icon. The
	// images are copied before SetIcon returns. Call it before the first
	// Present to have the icon from the start. Invalid images panic
	// ([CheckIcon]).
	SetIcon(images []image.Image)

	// SetTitleBarColors sets the colors of the OS-drawn title bar and
	// border (Windows 11); colors that are not Set restore the system's.
	// Other backends ignore it. See "Window styling" in the package
	// documentation.
	SetTitleBarColors(colors TitleBarColors)

	// SetBackdrop sets the system material behind the window (Mica,
	// Acrylic: Windows 11 22H2); the window shows it in its title bar and
	// where it presents transparent pixels. Other backends ignore it.
	// An invalid Backdrop panics ([CheckBackdrop]).
	SetBackdrop(backdrop Backdrop)

	// SetHitRegions tells the OS which parts of the content area are a
	// custom title bar: caption (drag) areas and window buttons, in
	// logical pixels, window coordinates, later regions over earlier
	// ones (see [HitRegion] and "Custom title bars" in the package
	// documentation). The app calls it after every frame that changes
	// them; the backend keeps a copy and ignores equal calls. nil clears
	// them. Invalid regions panic ([CheckHitRegions]).
	SetHitRegions(regions []HitRegion)

	// RestoreToken returns the window's session restore token: an opaque
	// string the layer above saves with the window's geometry and passes
	// back as WindowOptions.RestoreToken at the next start, so the OS can
	// put the window back where it was. "" means the backend has none
	// (yet); see "Session restore" in the package documentation.
	RestoreToken() string

	// ActivateWithToken is Activate with an activation token the OS gave
	// another process of the app (LaunchEvent's Launch.Token), which lets
	// the window come to the front past focus stealing prevention:
	// xdg_activation_v1.activate on Wayland, the token's timestamp and
	// _NET_STARTUP_INFO "remove" on X11. "" is Activate. Win32 ignores
	// the token (the second process allowed the foreground change). See
	// "Single instance and launches" in the package documentation.
	ActivateWithToken(token string)

	// --- Desktop integration (phase 13, ADR-209, ADR-210). ---

	// SetDropEffect answers the drag over the window (DragEvent): effect
	// is what a drop would do (DropNone refuses; an effect outside the
	// drag's Allowed set counts as DropNone), and types are the offered
	// MIME types the app will read at the drop, in preference order (none:
	// the well-known types offered; DropTypes). The answer stays until the
	// next call or the end of the drag session; a session starts refused.
	// Call it while handling DragEnter and DragOver (Win32 must answer
	// from inside the OS callback); calls during DragDrop set the final
	// effect (DropNone: the app did not take the data, so a moving source
	// keeps it). Without a drag over the window it does nothing. An
	// invalid effect or type panics ([CheckDropEffect], [CheckMIME]). See
	// "Drag and drop and the clipboard" in the package documentation.
	SetDropEffect(effect DropEffect, types ...string)

	// StartDrag starts an OS drag of opts.Data from the window, which can
	// end over any window of any application. It must be called while a
	// pointer button pressed in the window is held (from the handler of
	// that sequence's PointerDown or PointerMove), else it returns
	// [ErrNoGrab]; the OS takes the pointer, and the window gets a
	// PointerCancel for it. When it returns nil, one DragDoneEvent with
	// the outcome follows. Backends without OS drags return
	// ErrUnsupported. Invalid options panic ([CheckDragOptions]).
	StartDrag(opts DragOptions) error

	// SetMenu sets the window's menu bar: the top-level items (usually
	// each with a Submenu). A chosen item arrives as a MenuEvent. nil or
	// empty removes the bar. The content area keeps its size where the
	// OS allows. Backends without native menu bars return ErrUnsupported:
	// Linux (Wayland and X11), frameless windows, Cocoa until its desktop
	// integration (its menu is the app's). The menu is copied. An invalid menu panics
	// ([CheckMenu]). See "Native menus" in the package documentation.
	SetMenu(menu []MenuItem) error

	// PopupMenu shows a native context menu with its top-left corner at
	// at (logical pixels, window coordinates). done receives the chosen
	// item's ID, or 0 and a nil error when the user dismissed the menu; it
	// runs exactly once, later, on the UI thread through the loop, unless
	// PopupMenu returned an error (then never) or Run returned first.
	// Backends without native menus return ErrUnsupported (Linux), and the
	// layers above draw the menu. An empty or invalid menu, or a nil done,
	// panics.
	PopupMenu(at Point, menu []MenuItem, done func(item MenuItemID, err error)) error
}

// Handler receives the backend's events on the UI thread. The window is nil
// for app-scoped events (WakeEvent, SettingsEvent, NotificationEvent,
// TrayEvent, ShortcutEvent, ShortcutStatusEvent, LaunchEvent,
// QuitRequestedEvent, AppMenuEvent).
//
// The handler may call any Backend or Window method, including NewWindow,
// Present and Close. Backends never call HandleEvent re-entrantly: events
// produced while the handler runs (for example a synchronous WM_SIZE caused
// by SetSize on Win32) are queued and delivered after it returns.
type Handler interface {
	HandleEvent(w Window, e Event)
}

// HandlerFunc adapts a function to a Handler.
type HandlerFunc func(w Window, e Event)

// HandleEvent calls f(w, e).
func (f HandlerFunc) HandleEvent(w Window, e Event) { f(w, e) }

// DefaultWindowSize is the logical size used when WindowOptions.Size is
// zero.
var DefaultWindowSize = Size{W: 800, H: 600}

// WindowOptions configures a new window. The zero value is valid: an 800x600
// resizable, decorated, visible top-level window with an empty title.
// WindowOptions values are comparable.
type WindowOptions struct {
	// Title is the window title.
	Title string
	// AppID identifies the application to the desktop: Wayland
	// xdg_toplevel.app_id, X11 WM_CLASS (instance and class), used to
	// match .desktop files and group windows. Use reverse-DNS
	// ("com.example.notes"). Empty means [DefaultAppID] (the executable
	// name as invoked), the same on every backend.
	AppID string
	// Size is the requested logical size of the content area. Zero means
	// DefaultWindowSize. The OS may grant another size.
	Size Size
	// MinSize is the minimum logical content size; zero means none, and
	// a zero component means no minimum on that axis.
	MinSize Size
	// MaxSize is the maximum logical content size; zero means none, and a
	// zero component means no maximum on that axis. Where it is below
	// MinSize, MinSize wins (like CSS min-width over max-width).
	MaxSize Size
	// FixedSize prevents the user from resizing the window. The OS may
	// still impose a size (tiling window managers); the app must handle
	// whatever ConfigureEvent arrives.
	FixedSize bool

	// Position, if HasPosition is set, is where the top-left of the
	// content area goes, in the OS's desktop coordinates (physical
	// pixels; points on Cocoa; see Window.SetPosition). Ignored on Wayland.
	Position    image.Point
	HasPosition bool
	// Maximized and Fullscreen open the window in that state (restoring a
	// saved window). Size is then the size it returns to.
	Maximized  bool
	Fullscreen bool
	// Hidden creates the window hidden: it is not shown at its first
	// Present but by SetVisible(true). It still receives its
	// ConfigureEvent; FrameEvents wait until it is shown.
	Hidden bool
	// AlwaysOnTop: see Window.SetAlwaysOnTop.
	AlwaysOnTop bool
	// Frameless removes the OS title bar and borders: the app draws its
	// own title bar and declares it with Window.SetHitRegions. The
	// backend keeps the window resizable at its edges (unless FixedSize)
	// and keeps the OS shadow and rounded corners where the OS draws them
	// for undecorated windows. See "Custom title bars" in the package
	// documentation.
	Frameless bool
	// TitleBarOverlay extends the content under a transparent OS title
	// bar and keeps the OS's window buttons over it (macOS: the traffic
	// lights): the app draws the rest of its title bar and declares it
	// with Window.SetHitRegions, as a Frameless window does, and
	// WindowState.TitleBarControls reports the buttons' rectangle. A
	// backend that cannot keep its buttons over the content treats it as
	// Frameless (OverlayAsFrameless): Wayland, X11 and Win32 for now. Set
	// at creation; it excludes Frameless (Resolve panics with both). See
	// "Custom title bars" in the package documentation.
	TitleBarOverlay bool

	// TitleBar and Backdrop: see Window.SetTitleBarColors and
	// Window.SetBackdrop. Backends apply them when they create the window.
	TitleBar TitleBarColors
	Backdrop Backdrop

	// Parent makes the window a child (transient, owned) window of
	// another open window of the same backend: it stays above its parent,
	// is minimized and closed with it, and is centered on it by the OS
	// when it has no position. nil: a top-level window.
	Parent Window
	// Modal makes the child window modal to Parent: while it is visible,
	// the parent receives no input. Requires Parent. See "Parent windows
	// and modality" in the package documentation.
	Modal bool

	// RestoreToken is the token Window.RestoreToken returned for this
	// window in a previous run, or "". Backends ignore tokens they did not
	// produce (another backend, another version, garbage). See "Session
	// restore" in the package documentation.
	RestoreToken string
}

// Resolve returns opts with defaults applied (zero Size becomes
// DefaultWindowSize, Size is clamped to MaxSize and then raised to
// MinSize). It panics with an actionable message on invalid values
// (negative, NaN or infinite sizes, a half-zero Size, Modal without
// Parent, Frameless with TitleBarOverlay). Backends call it at the top of NewWindow so every backend
// accepts and rejects the same options; checking that Parent is an open
// window of the same backend is left to the backend.
func (opts WindowOptions) Resolve() WindowOptions {
	CheckSize("WindowOptions.Size", opts.Size)
	CheckSize("WindowOptions.MinSize", opts.MinSize)
	CheckSize("WindowOptions.MaxSize", opts.MaxSize)
	CheckBackdrop("WindowOptions.Backdrop", opts.Backdrop)
	if opts.Size == (Size{}) {
		opts.Size = DefaultWindowSize
	}
	if (opts.Size.W == 0) != (opts.Size.H == 0) {
		panicf("platform: WindowOptions.Size %v has one zero dimension; set both W and H, or leave Size zero for the default %v", opts.Size, DefaultWindowSize)
	}
	if opts.Modal && opts.Parent == nil {
		panic("platform: WindowOptions.Modal without a Parent; a modal window is modal to its parent window: set Parent")
	}
	if opts.Frameless && opts.TitleBarOverlay {
		panic("platform: WindowOptions has both Frameless and TitleBarOverlay; Frameless removes the OS's window buttons and TitleBarOverlay keeps them over the content: set one")
	}
	opts.Size = opts.ClampSize(opts.Size)
	return opts
}

// OverlayAsFrameless returns opts with TitleBarOverlay turned into
// Frameless: what a backend that cannot keep the OS's window buttons over
// the content does with a TitleBarOverlay window (Wayland, X11, Win32;
// see "Custom title bars" in the package documentation). Such a window
// reports an empty WindowState.TitleBarControls. Other options are
// returned unchanged.
func (opts WindowOptions) OverlayAsFrameless() WindowOptions {
	if opts.TitleBarOverlay {
		opts.Frameless, opts.TitleBarOverlay = true, false
	}
	return opts
}

// ClampSize clamps s to opts.MaxSize and then raises it to opts.MinSize,
// per axis (zero components are no limit; the minimum wins). Backends use
// it for SetSize requests and for sizes they impose.
func (opts WindowOptions) ClampSize(s Size) Size { return ClampSize(s, opts.MinSize, opts.MaxSize) }

// ClampSize clamps s to max and then raises it to min, per axis; a zero
// component of min or max is no limit on that axis, and min wins where max
// is below it.
func ClampSize(s, min, max Size) Size {
	if max.W > 0 && s.W > max.W {
		s.W = max.W
	}
	if max.H > 0 && s.H > max.H {
		s.H = max.H
	}
	if s.W < min.W {
		s.W = min.W
	}
	if s.H < min.H {
		s.H = min.H
	}
	return s
}

// CheckSize panics with an actionable message if s has a negative, NaN or
// infinite component. what names the value in the message.
func CheckSize(what string, s Size) {
	for _, v := range [2]float32{s.W, s.H} {
		if v != v || v < 0 || v > 1e7 {
			panicf("platform: invalid %s %v; logical sizes must be finite, non-negative and below 1e7", what, s)
		}
	}
}

// SystemSettings are user preferences read from the OS. Zero values mean
// "not known", and the layers above apply their defaults.
type SystemSettings struct {
	// DarkMode is the user's color scheme preference (CSS
	// prefers-color-scheme: dark).
	DarkMode bool
	// ReducedMotion asks for minimal animation (CSS
	// prefers-reduced-motion: reduce).
	ReducedMotion bool
	// DoubleClickInterval is the maximum time between the clicks of a
	// double click. Zero means unknown: DefaultDoubleClickInterval
	// applies (DoubleClickTime).
	DoubleClickInterval time.Duration

	// AccentColor is the user's accent color (opaque sRGB) when
	// HasAccentColor is set.
	AccentColor    color.NRGBA
	HasAccentColor bool
	// HighContrast asks for a high-contrast presentation (CSS
	// prefers-contrast: more; Tailwind's contrast-more:).
	HighContrast bool
	// TextScale is the user's text size factor (Windows "Text size",
	// GNOME's text-scaling-factor): 1rem is the theme's rem times it.
	// Zero means unknown and is treated as 1 (TextScaleFactor).
	TextScale float64
}

// MaxTextScale bounds SystemSettings.TextScale; backends report larger OS
// values as MaxTextScale.
const MaxTextScale = 4

// TextScaleFactor returns s.TextScale, or 1 when it is unknown (not
// positive, or not a number).
func (s SystemSettings) TextScaleFactor() float64 {
	if s.TextScale > 0 && s.TextScale <= MaxTextScale {
		return s.TextScale
	}
	if s.TextScale > MaxTextScale {
		return MaxTextScale
	}
	return 1
}

// CheckSystemSettings panics with an actionable message if s has a
// negative DoubleClickInterval or a TextScale that is negative, not a
// number or above MaxTextScale. Backends report what the OS says within
// these limits; the headless backend checks the settings tests give it.
func CheckSystemSettings(s SystemSettings) {
	if s.DoubleClickInterval < 0 {
		panicf("platform: SystemSettings.DoubleClickInterval %v is negative; zero means unknown", s.DoubleClickInterval)
	}
	if math.IsNaN(s.TextScale) || s.TextScale < 0 || s.TextScale > MaxTextScale {
		panicf("platform: SystemSettings.TextScale %v is out of range; give a factor in (0, %d], or 0 for unknown", s.TextScale, MaxTextScale)
	}
}

// The double-click rule (ADR-192), shared by content clicks (core's click
// count for mice and pens) and the title bars that backends draw
// themselves (CaptionClicks): two presses count as a double click when the
// second comes at most the double-click time after the first and at most
// DoubleClickSlop logical pixels away from it. Touch has its own, larger
// limits in the gesture layer.
const (
	// DefaultDoubleClickInterval is the double-click time when the OS
	// does not report one: the default of Windows (GetDoubleClickTime)
	// and macOS. GTK's default is 400 ms.
	DefaultDoubleClickInterval = 500 * time.Millisecond
	// DoubleClickSlop is the farthest, in logical pixels, the second
	// press of a double click may be from the first (GTK's
	// gtk-double-click-distance; Windows uses a 4x4 rectangle).
	DoubleClickSlop = 5
)

// DoubleClickTime returns the double-click time of s: DoubleClickInterval,
// or DefaultDoubleClickInterval when it is unknown (not positive).
func (s SystemSettings) DoubleClickTime() time.Duration {
	return DoubleClickTime(s.DoubleClickInterval)
}

// DoubleClickTime returns interval, or DefaultDoubleClickInterval when it
// is unknown (not positive): the one rule for title bars and content
// (ADR-192).
func DoubleClickTime(interval time.Duration) time.Duration {
	if interval > 0 {
		return interval
	}
	return DefaultDoubleClickInterval
}

// Clipboard is the system clipboard (on X11, the CLIPBOARD selection).
// Methods must be called on the UI thread. It carries [Data]: the
// well-known MIME types mapped onto the OS's formats, and custom types
// (see "Drag and drop and the clipboard" in the package documentation).
//
// Reading is asynchronous because on Wayland and X11 the data comes from
// another client through the event loop; a synchronous read could deadlock
// (for example when the owner is this same process).
type Clipboard interface {
	// ReadText requests the clipboard's text. done is called later on the
	// UI thread, from the event loop and never from inside ReadText, with
	// the text ("" if the clipboard is empty or holds no text) or an
	// error.
	ReadText(done func(text string, err error))

	// WriteText puts text on the clipboard; the backend serves it to other
	// applications from the event loop for as long as it owns the
	// selection. It is Write(TextData(text)), except that Win32 writes
	// the text fields' line breaks as CR LF here, while Write keeps the
	// text as it is.
	WriteText(text string) error

	// ReadTypes requests the MIME types the clipboard offers, normalized
	// like DragEvent.Types, in the owner's order (nil when it is empty).
	// done is called like ReadText's. It returns an error at once, and
	// never calls done, when the request cannot be made (ErrUnsupported).
	ReadTypes(done func(types []string, err error)) error

	// Read requests the clipboard's content of one MIME type. done is
	// called like ReadText's with the bytes, or nil and a nil error when
	// the clipboard does not offer the type. It returns an error at once,
	// and never calls done, when the request cannot be made
	// (ErrUnsupported). An invalid type panics ([CheckMIME]).
	Read(mime string, done func(data []byte, err error)) error

	// Write puts data on the clipboard, replacing what it held; empty data
	// clears it. The backend copies data and serves it, converted to the
	// OS's formats, for as long as it owns the selection. Invalid data
	// panics ([CheckData]).
	Write(data Data) error
}
