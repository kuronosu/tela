//go:build darwin

package cocoa

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// errClosed is returned by NewWindow after Run returned.
var errClosed = errors.New("cocoa: the backend is closed (Run has returned)")

// Backend is the Cocoa backend. Create it with New on the main thread.
type Backend struct {
	id    uintptr // key in registry.backends: the info of the CF callbacks
	start time.Time

	// loop holds the open windows, the event queue, the wake/quit flags and
	// the frame pacing state shared with the other backends.
	loop *loop.Loop[*Window]

	runLoop uintptr // the main CFRunLoop
	srgb    uintptr // CGColorSpace of every presented image
	// pacer is the pacing timer: a Go timer that signals the run loop
	// source at the shared loop's deadline for frames no display link
	// paces, and at the stand-in deadline of a link that has not ticked.
	// A CFRunLoopTimer was used first, but on macos-latest it fired about
	// every 100 ms whatever its fire date (probably the system's timer
	// coalescing, which may scale with the repeating timer's huge unused
	// interval), pacing at 10 Hz; a Go timer runs on the backend's own
	// clock, so the frame is due when it fires.
	pacer    *time.Timer
	linkKind linkKind // the display link of the windows (link_darwin.go)
	clock    eventClock

	// mu guards source and stopped against shutdown, for notify, which
	// runs on any goroutine.
	mu      sync.Mutex
	source  uintptr // CFRunLoopSource: Wake, Quit and follow-up passes
	stopped bool

	// UI-thread state.
	handler    platform.Handler
	ran        bool
	running    bool // inside [NSApp run]
	stopping   bool // [NSApp stop:] was sent
	closed     bool
	pumping    bool // a delivery pass is running
	failed     bool // the handler panicked inside a callback
	failure    any  // the panic value, re-raised by Run
	timerArmed bool
	activated  bool        // the app was activated at a first window show
	zombies    []darwin.ID // released at the next pass (releaseLater)

	settings settingsState // system settings and their observer (settings_darwin.go)
	desk     desktopState  // desktop services (services_darwin.go)
}

var _ platform.Backend = (*Backend)(nil)

// registry maps the integers that the CF and CoreVideo callbacks receive
// to backends and windows, and the Objective-C objects that AppKit calls
// back (window delegates and content views) to windows. Go pointers are
// never handed to C.
var registry = struct {
	sync.Mutex
	next     uintptr
	backends map[uintptr]*Backend
	objects  map[darwin.ID]*Window
	links    map[uintptr]*Window // CVDisplayLink contexts
}{backends: map[uintptr]*Backend{}, objects: map[darwin.ID]*Window{}, links: map[uintptr]*Window{}}

func backendFor(id uintptr) *Backend {
	registry.Lock()
	defer registry.Unlock()
	return registry.backends[id]
}

func windowFor(obj darwin.ID) *Window {
	registry.Lock()
	defer registry.Unlock()
	return registry.objects[obj]
}

// New creates the backend. It must be called on the main thread, which it
// locks the calling goroutine to until Run returns (AppKit may only be used
// there). It returns an error wrapping platform.ErrUnavailable when the
// process is not part of a window server session (an SSH login, a
// daemon), where AppKit would abort.
func New() (*Backend, error) {
	if err := darwin.Load(); err != nil {
		return nil, fmt.Errorf("cocoa: %w (%w)", platform.ErrUnavailable, err)
	}
	if !darwin.IsMainThread() {
		return nil, errors.New("cocoa: New must be called on the main thread (AppKit runs only there); " +
			"call it from main, or from an init-locked main goroutine as tela.Run does")
	}
	session := darwin.CGSessionCopyCurrentDictionary()
	if session == 0 {
		return nil, fmt.Errorf("cocoa: no window server session (logged in over SSH or running as a daemon?); "+
			"run in a logged-in desktop session, or set TELA_BACKEND=headless: %w", platform.ErrUnavailable)
	}
	darwin.CFRelease(session)
	if err := setup(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	b := &Backend{start: time.Now(), runLoop: darwin.CFRunLoopGetMain(), linkKind: chooseLink()}
	b.loop = loop.New(loop.Options[*Window]{Now: b.Now, Notify: b.notify, Pace: b.paceHook})
	pool := darwin.PoolPush()
	b.clock.offset = b.Now() - time.Duration(uptime()*float64(time.Second))
	darwin.PoolPop(pool)

	registry.Lock()
	registry.next++
	b.id = registry.next
	registry.backends[b.id] = b
	registry.Unlock()

	src := darwin.CFRunLoopSourceContext{Info: b.id, Perform: callbacks.perform}
	b.source = darwin.CFRunLoopSourceCreate(0, 0, &src)
	b.srgb = darwin.CGColorSpaceCreateWithName(darwin.CGColorSpaceSRGB)
	if b.source == 0 || b.srgb == 0 {
		b.release()
		runtime.UnlockOSThread()
		return nil, errors.New("cocoa: creating the run loop source or the sRGB color space failed")
	}
	darwin.CFRunLoopAddSource(b.runLoop, b.source, darwin.CFRunLoopCommonModes)
	b.startSettings()
	return b, nil
}

// Name returns "cocoa".
func (b *Backend) Name() string { return "cocoa" }

// Now returns the time since the backend was created (monotonic). Safe from
// any goroutine.
func (b *Backend) Now() time.Duration { return time.Since(b.start) }

// Wake makes the loop deliver a WakeEvent. Safe from any goroutine; never
// blocks for long; coalesces.
func (b *Backend) Wake() { b.loop.Wake() }

// Quit makes Run return after the current handler call. Safe from any
// goroutine; idempotent.
func (b *Backend) Quit() { b.loop.Quit() }

// notify signals the run loop source, whose perform callback runs a
// delivery pass on the main thread. Signals coalesce until the source
// fires. Safe from any goroutine (CFRunLoopSourceSignal and CFRunLoopWakeUp
// are thread-safe); after shutdown it does nothing.
func (b *Backend) notify() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped || b.source == 0 {
		return
	}
	darwin.CFRunLoopSourceSignal(b.source)
	darwin.CFRunLoopWakeUp(b.runLoop)
}

// Run runs [NSApp run] until Quit. On return every window is closed. If the
// handler panicked, Run panics with the same value.
func (b *Backend) Run(h platform.Handler) error {
	if h == nil {
		panic("cocoa: Run called with a nil Handler")
	}
	if b.ran {
		panic("cocoa: Run called twice; a Backend runs one event loop")
	}
	b.ran = true
	b.handler = h
	defer b.shutdown()
	if !b.loop.Quitting() {
		b.running = true
		b.notify() // the first pass delivers the ConfigureEvents queued by NewWindow
		pool := darwin.PoolPush()
		darwin.Send(app.ns, sel.run)
		darwin.PoolPop(pool)
		b.running = false
	}
	if b.failed {
		panic(b.failure)
	}
	return nil
}

// stopApp makes [NSApp run] return: stop: takes effect after the event
// being processed, so an application-defined event is posted for it (a
// run loop source is not an event).
func (b *Backend) stopApp() {
	if b.stopping || !b.running {
		return
	}
	b.stopping = true
	darwin.Send(app.ns, sel.stop, darwin.ID(0))
	ev := darwin.SendClass(app.event, sel.otherEvent,
		uintptr(darwin.NSEventTypeApplicationDefined), darwin.CGPoint{}, uintptr(0), float64(0),
		uintptr(0), uintptr(0), uintptr(0), uintptr(0), uintptr(0))
	darwin.Send(app.ns, sel.postEventAtStart, ev, true)
}

// shutdown releases the OS resources when Run returns.
func (b *Backend) shutdown() {
	if b.closed {
		return
	}
	b.closed = true
	pool := darwin.PoolPush()
	b.stopDesktop() // ends the sheets before their windows close
	for _, w := range slices.Clone(b.loop.Windows()) {
		w.Close()
	}
	b.releaseZombies()
	b.stopSettings()
	darwin.PoolPop(pool)
	b.loop.Discard()
	b.release()
	runtime.UnlockOSThread()
}

// release stops the pacing timer, frees the run loop source and the color
// space and takes the backend out of the registry.
func (b *Backend) release() {
	if b.pacer != nil {
		b.pacer.Stop()
	}
	b.mu.Lock()
	b.stopped = true
	if b.source != 0 {
		darwin.CFRunLoopSourceInvalidate(b.source)
		darwin.CFRelease(b.source)
		b.source = 0
	}
	b.mu.Unlock()
	if b.srgb != 0 {
		darwin.CGColorSpaceRelease(b.srgb)
		b.srgb = 0
	}
	registry.Lock()
	delete(registry.backends, b.id)
	registry.Unlock()
}

// guard runs fn, a callback from AppKit or CoreFoundation, inside an
// autorelease pool, and turns a panic into a stop of the loop that Run
// re-raises: a Go panic must not unwind through Objective-C frames.
func (b *Backend) guard(fn func()) {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	defer func() {
		if r := recover(); r != nil {
			b.fail(r)
		}
	}()
	fn()
}

// fail records a panic of the handler and stops the loop; Run raises it
// again.
func (b *Backend) fail(r any) {
	if !b.failed {
		b.failed, b.failure = true, r
	}
	b.loop.Quit()
	b.stopApp()
}

// pump runs a delivery pass: the queued events, the due frames, starting
// the display links (or arming the pacing timer) for the requested ones
// that are not due and stopping the links nothing waits for, and, if the
// handler left work behind (events that arrived while it ran), a signal
// for a follow-up pass. It does nothing inside a pass (the handler is never
// called re-entrantly; what was queued meanwhile waits for the follow-up)
// and outside Run.
func (b *Backend) pump() {
	if b.pumping || b.handler == nil || !b.running {
		return
	}
	b.pumping = true
	func() {
		defer func() { b.pumping = false }()
		b.loop.Deliver(deliveryHook{b})
		// The full-screen and zoom requests of the pass, and those that
		// waited (windowctl_darwin.go). What AppKit reports meanwhile is
		// queued for the follow-up pass.
		for _, w := range b.loop.Windows() {
			w.applyControl()
		}
	}()
	idle := !b.loop.Ready()
	for _, w := range b.loop.Windows() {
		if idle {
			w.settleIME()
		}
		w.flushIME()
	}
	if b.loop.Quitting() {
		b.stopApp()
		return
	}
	b.arm()
	b.idleLinks()
	if b.loop.Ready() {
		b.notify()
	}
}

// deliveryHook is the handler of a delivery pass: the app's handler, and
// after each event what the backend does once the app has handled it (the
// IME's document goes back to the app's state, ime_darwin.go).
type deliveryHook struct{ b *Backend }

func (d deliveryHook) HandleEvent(pw platform.Window, e platform.Event) {
	d.b.handler.HandleEvent(pw, e)
	if w, ok := pw.(*Window); ok && w != nil {
		w.handledIME(e)
	}
}

// arm offers the requested frames that are not due yet to the display
// links (the loop's Pace hook) and sets the pacing timer to the shared
// loop's deadline for the others, or to the stand-in deadline of a link
// that has not ticked; it stops the timer when no frame waits (0% CPU when
// idle). The timer's goroutine only signals the run loop source, whose
// pass on the main thread does the work.
func (b *Backend) arm() {
	deadline, ok := b.loop.Pace()
	if t, has := b.linkDeadline(); has && (!ok || t < deadline) {
		deadline, ok = t, true
	}
	if !ok {
		if b.timerArmed {
			b.pacer.Stop()
			b.timerArmed = false
		}
		return
	}
	d := max(deadline-b.Now(), 0)
	if b.pacer == nil {
		b.pacer = time.AfterFunc(d, b.notify)
	} else {
		b.pacer.Reset(d)
	}
	b.timerArmed = true
}

// releaseLater releases objs at the next pass started by the run loop:
// a window closed from the handler may be closing inside one of its own
// methods (windowShouldClose: runs a pass), which must not see it freed.
func (b *Backend) releaseLater(objs ...darwin.ID) {
	b.zombies = append(b.zombies, objs...)
	b.notify()
}

func (b *Backend) releaseZombies() {
	for _, o := range b.zombies {
		darwin.Release(o)
	}
	b.zombies = b.zombies[:0]
}

// enqueue and reconfigure queue work for the next pass.
func (b *Backend) enqueue(w *Window, e platform.Event) { b.loop.Enqueue(w, e) }

func (b *Backend) reconfigure(w *Window, m platform.Metrics) { b.loop.Reconfigure(w, m) }

// --- process-wide setup ---

// sel holds the selectors the backend sends, registered once.
var sel = struct {
	alloc, init, run, stop, otherEvent, postEventAtStart                darwin.SEL
	sharedApplication, setActivationPolicy, setDelegate, activate       darwin.SEL
	initWithContentRect, setReleasedWhenClosed, setTabbingMode          darwin.SEL
	setRestorable, setTitle, initWithFrame, layer, setLayer, wantsLayer darwin.SEL
	setContentsGravity, setContentsScale, contentsAreFlipped            darwin.SEL
	setContentView, makeFirstResponder, center, bounds, backingScale    darwin.SEL
	setContentSize, setContentMinSize, setContentMaxSize                darwin.SEL
	makeKeyAndOrderFront, orderOut, close, invalidateCursorRects        darwin.SEL
	addCursorRect, setContents, begin, commit, setDisableActions        darwin.SEL
	windowNumber, renderInContext                                       darwin.SEL
}{
	alloc:                 darwin.Sel("alloc"),
	init:                  darwin.Sel("init"),
	run:                   darwin.Sel("run"),
	stop:                  darwin.Sel("stop:"),
	otherEvent:            darwin.Sel("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
	postEventAtStart:      darwin.Sel("postEvent:atStart:"),
	sharedApplication:     darwin.Sel("sharedApplication"),
	setActivationPolicy:   darwin.Sel("setActivationPolicy:"),
	setDelegate:           darwin.Sel("setDelegate:"),
	activate:              darwin.Sel("activateIgnoringOtherApps:"),
	initWithContentRect:   darwin.Sel("initWithContentRect:styleMask:backing:defer:"),
	setReleasedWhenClosed: darwin.Sel("setReleasedWhenClosed:"),
	setTabbingMode:        darwin.Sel("setTabbingMode:"),
	setRestorable:         darwin.Sel("setRestorable:"),
	setTitle:              darwin.Sel("setTitle:"),
	initWithFrame:         darwin.Sel("initWithFrame:"),
	layer:                 darwin.Sel("layer"),
	setLayer:              darwin.Sel("setLayer:"),
	wantsLayer:            darwin.Sel("setWantsLayer:"),
	setContentsGravity:    darwin.Sel("setContentsGravity:"),
	setContentsScale:      darwin.Sel("setContentsScale:"),
	contentsAreFlipped:    darwin.Sel("contentsAreFlipped"),
	setContentView:        darwin.Sel("setContentView:"),
	makeFirstResponder:    darwin.Sel("makeFirstResponder:"),
	center:                darwin.Sel("center"),
	bounds:                darwin.Sel("bounds"),
	backingScale:          darwin.Sel("backingScaleFactor"),
	setContentSize:        darwin.Sel("setContentSize:"),
	setContentMinSize:     darwin.Sel("setContentMinSize:"),
	setContentMaxSize:     darwin.Sel("setContentMaxSize:"),
	makeKeyAndOrderFront:  darwin.Sel("makeKeyAndOrderFront:"),
	orderOut:              darwin.Sel("orderOut:"),
	close:                 darwin.Sel("close"),
	invalidateCursorRects: darwin.Sel("invalidateCursorRectsForView:"),
	addCursorRect:         darwin.Sel("addCursorRect:cursor:"),
	setContents:           darwin.Sel("setContents:"),
	begin:                 darwin.Sel("begin"),
	commit:                darwin.Sel("commit"),
	setDisableActions:     darwin.Sel("setDisableActions:"),
	windowNumber:          darwin.Sel("windowNumber"),
	renderInContext:       darwin.Sel("renderInContext:"),
}

// app holds the shared NSApplication and the classes, set up once per
// process (an Objective-C class name exists once per process).
var app struct {
	ns                                darwin.ID // NSApp
	delegate                          darwin.ID // TelaAppDelegate instance
	window, windowDelegate, view      darwin.Class
	layer, transaction, cursor, event darwin.Class
	trackingArea, processInfo         darwin.Class
	runLoop, nsView                   darwin.Class
	// Window control (windowctl_darwin.go); effectView is 0 if AppKit
	// lacks NSVisualEffectView (backdrops then do nothing).
	screen, userDefaults, color, effectView darwin.Class
	// appSuper is the superclass of TelaApplication (NSApplication),
	// where its sendEvent: override sends super.
	appSuper darwin.Class
	// textInputClient reports that TelaView declares NSTextInputClient
	// (else it makes its own input context).
	textInputClient bool
}

// callbacks are the C function pointers of the run loop source and the
// CVDisplayLink output, created once (purego callbacks are never freed).
var callbacks struct{ perform, cvTick uintptr }

var (
	setupOnce sync.Once
	setupErr  error
)

// setup creates NSApp and registers the classes and callbacks, once.
func setup() error {
	setupOnce.Do(func() { setupErr = setupApp() })
	return setupErr
}

func setupApp() error {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)

	nsObject := darwin.GetClass("NSObject")
	app.layer = darwin.GetClass("CALayer")
	app.transaction = darwin.GetClass("CATransaction")
	app.cursor = darwin.GetClass("NSCursor")
	app.event = darwin.GetClass("NSEvent")
	app.trackingArea = darwin.GetClass("NSTrackingArea")
	app.processInfo = darwin.GetClass("NSProcessInfo")
	app.runLoop = darwin.GetClass("NSRunLoop")
	app.screen = darwin.GetClass("NSScreen")
	app.userDefaults = darwin.GetClass("NSUserDefaults")
	app.color = darwin.GetClass("NSColor")
	app.effectView = darwin.GetClass("NSVisualEffectView")
	nsWindow, nsView, nsApplication := darwin.GetClass("NSWindow"), darwin.GetClass("NSView"), darwin.GetClass("NSApplication")
	app.nsView, app.appSuper = nsView, nsApplication
	if nsObject == 0 || nsWindow == 0 || nsView == 0 || nsApplication == 0 || app.layer == 0 || app.transaction == 0 ||
		app.cursor == 0 || app.event == 0 || app.trackingArea == 0 || app.processInfo == 0 || app.runLoop == 0 ||
		app.screen == 0 || app.userDefaults == 0 || app.color == 0 {
		return fmt.Errorf("cocoa: AppKit or QuartzCore classes are missing: %w", platform.ErrUnavailable)
	}

	var err error
	// TelaWindow lets borderless (Frameless) windows become key and main,
	// which NSWindow refuses by default.
	app.window, err = darwin.RegisterClass("TelaWindow", nsWindow, nil, []darwin.Method{
		{Sel: darwin.Sel("canBecomeKeyWindow"), Fn: func(darwin.ID, darwin.SEL) bool { return true }},
		{Sel: darwin.Sel("canBecomeMainWindow"), Fn: func(darwin.ID, darwin.SEL) bool { return true }},
	})
	if err != nil {
		return fmt.Errorf("cocoa: registering TelaWindow: %w", err)
	}
	// The delegate protocols are not declared: AppKit calls delegates
	// through respondsToSelector:, and the protocol objects exist in the
	// runtime only when some loaded code references them.
	app.windowDelegate, err = darwin.RegisterClass("TelaWindowDelegate", nsObject, nil, windowDelegateMethods())
	if err != nil {
		return fmt.Errorf("cocoa: registering TelaWindowDelegate: %w", err)
	}
	// TelaView conforms to NSTextInputClient, which makes NSView give it
	// an NSTextInputContext; should the runtime not know the protocol,
	// the view makes its own context (ime_darwin.go).
	methods := append(append(viewMethods(), inputMethods()...), imeMethods()...)
	var protocols []string
	if app.textInputClient = darwin.HasProtocol("NSTextInputClient"); app.textInputClient {
		protocols = []string{"NSTextInputClient"}
	} else {
		methods = append(methods, inputContextMethod())
	}
	app.view, err = darwin.RegisterClass("TelaView", nsView, protocols, methods)
	if err != nil {
		return fmt.Errorf("cocoa: registering TelaView: %w", err)
	}
	// The application delegate does not declare NSApplicationDelegate:
	// declaring it made AppKit crash on macOS 14 shortly after activation
	// in Ebitengine (issue #3451); AppKit finds the methods anyway through
	// respondsToSelector:.
	appDelegate, err := darwin.RegisterClass("TelaAppDelegate", nsObject, nil, []darwin.Method{
		{Sel: darwin.Sel("applicationShouldTerminate:"), Fn: applicationShouldTerminate},
	})
	if err != nil {
		return fmt.Errorf("cocoa: registering TelaAppDelegate: %w", err)
	}

	// TelaApplication forwards the key releases that NSApplication's own
	// sendEvent: swallows (input_darwin.go). It must be the class of NSApp,
	// so it is registered before sharedApplication creates NSApp.
	appClass, err := darwin.RegisterClass("TelaApplication", nsApplication, nil, []darwin.Method{
		{Sel: darwin.Sel("sendEvent:"), Fn: appSendEvent},
	})
	if err != nil {
		return fmt.Errorf("cocoa: registering TelaApplication: %w", err)
	}

	callbacks.perform = darwin.NewCallback(func(info uintptr) {
		if b := backendFor(info); b != nil {
			b.guard(func() {
				b.releaseZombies()
				b.collectTicks()
				b.standIn()
				b.pump()
			})
		}
	})
	callbacks.cvTick = darwin.NewCallback(cvTick)

	// sharedApplication creates NSApp as an instance of the class it is
	// sent to, unless something in the process created it already (then
	// the Command key releases are not forwarded).
	app.ns = darwin.SendClass(appClass, sel.sharedApplication)
	if app.ns == 0 {
		return fmt.Errorf("cocoa: [NSApplication sharedApplication] returned nil: %w", platform.ErrUnavailable)
	}
	// An unbundled process starts as a background app whose windows cannot
	// become key; the regular policy gives it a Dock icon and focus.
	darwin.Send(app.ns, sel.setActivationPolicy, uintptr(darwin.NSApplicationActivationPolicyRegular))
	app.delegate = darwin.Send(darwin.SendClass(appDelegate, sel.alloc), sel.init)
	darwin.Send(app.ns, sel.setDelegate, app.delegate)
	return nil
}

// applicationShouldTerminate: answers a quit request (Cmd+Q, the app
// menu's Quit, the Dock's Quit, a logout): quitting is the app's policy,
// so the running backend delivers an app-scoped QuitRequestedEvent (the
// layers above run tela.App.OnQuit, or send every window a close
// request) and AppKit is told to cancel; the app quits with Quit.
func applicationShouldTerminate(_ darwin.ID, _ darwin.SEL, _ darwin.ID) uint {
	registry.Lock()
	var running *Backend
	for _, b := range registry.backends {
		if b.running {
			running = b
		}
	}
	registry.Unlock()
	if running != nil {
		running.guard(func() {
			running.enqueue(nil, platform.QuitRequestedEvent{})
			running.pump()
		})
	}
	return darwin.NSTerminateCancel
}
