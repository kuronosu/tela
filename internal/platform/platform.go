package platform

import (
	"errors"
	"image"
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

	// Clipboard returns the system clipboard.
	Clipboard() Clipboard

	// Settings returns the current system settings. Changes are reported
	// by SettingsEvent.
	Settings() SystemSettings
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
	// Windows are opaque: backends may ignore alpha. Errors are resource
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

	// Close destroys the window. Pending events for it are dropped.
	Close()
}

// Handler receives the backend's events on the UI thread. The window is nil
// for app-scoped events (WakeEvent, SettingsEvent).
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
// resizable window with an empty title.
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
	// MinSize is the minimum logical content size; zero means none.
	MinSize Size
	// FixedSize prevents the user from resizing the window. The OS may
	// still impose a size (tiling window managers); the app must handle
	// whatever ConfigureEvent arrives.
	FixedSize bool
}

// Resolve returns opts with defaults applied (zero Size becomes
// DefaultWindowSize, Size is raised to MinSize). It panics with an
// actionable message on invalid values (negative, NaN or infinite sizes, a
// half-zero Size). Backends call it at the top of NewWindow so every backend
// accepts and rejects the same options.
func (opts WindowOptions) Resolve() WindowOptions {
	CheckSize("WindowOptions.Size", opts.Size)
	CheckSize("WindowOptions.MinSize", opts.MinSize)
	if opts.Size == (Size{}) {
		opts.Size = DefaultWindowSize
	}
	if (opts.Size.W == 0) != (opts.Size.H == 0) {
		panicf("platform: WindowOptions.Size %v has one zero dimension; set both W and H, or leave Size zero for the default %v", opts.Size, DefaultWindowSize)
	}
	opts.Size = opts.Size.atLeast(opts.MinSize)
	return opts
}

// ClampSize raises s to opts.MinSize per axis. Backends use it for SetSize
// requests and for sizes they impose.
func (opts WindowOptions) ClampSize(s Size) Size { return s.atLeast(opts.MinSize) }

func (s Size) atLeast(min Size) Size {
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
	// double click. Zero means unknown.
	DoubleClickInterval time.Duration
}

// Clipboard is the system clipboard (on X11, the CLIPBOARD selection).
// Methods must be called on the UI thread.
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
	// selection.
	WriteText(text string) error
}
