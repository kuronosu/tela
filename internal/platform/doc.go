// Package platform is the lowest layer of Tela: the contract between the
// framework and one OS windowing system (a backend). It knows windows,
// events and framebuffers; it does not know gfx. A backend receives a
// finished *image.RGBA framebuffer plus damage rectangles and shows them.
//
// Backends live in subpackages: wayland, x11, win32, cocoa and headless.
// This package only defines the interfaces, the event set and a few helpers
// that every backend uses so they behave identically (sizing rule, option
// validation, framebuffer checks, damage clipping, pixel copies). It imports
// only the standard library. Choosing a backend at startup (TELA_BACKEND,
// WAYLAND_DISPLAY, DISPLAY) is done by the layer that imports the backends,
// since doing it here would create an import cycle.
//
// # Loop contract: the backend calls the app
//
// The backend owns the event loop. The app calls [Backend.Run] with a
// [Handler], and the backend calls Handler.HandleEvent for every event, on
// the UI thread, until [Backend.Quit]. There is no "poll events" or
// "drain Events()" API.
//
// This inversion is forced by two of the four platforms, and costs nothing
// on the other two:
//
//   - Cocoa: [NSApp run] owns the main loop and only ever calls back into the
//     app (delegates, NSView methods, run loop sources). A pull API would
//     need a nested run loop per pull.
//   - Win32: during an interactive move or resize, DefWindowProc runs its own
//     modal message loop and the app's GetMessageW loop is blocked until the
//     user releases the mouse. Only WndProc keeps running. With callbacks, a
//     WM_SIZE inside the modal loop becomes a ConfigureEvent followed by a
//     synchronous FrameEvent and Present, so the window repaints live during
//     the resize. A drained Events() queue would not be serviced until the
//     modal loop ends.
//   - Wayland and X11 are naturally a "read, dispatch, call the handler"
//     loop; winit (ApplicationHandler) and Gio (driver callbacks into
//     ProcessEvent) use the same model on every OS.
//
// One handler method receiving a sealed [Event] set keeps the interfaces
// stable: keyboard, text input and IME events (phases 4 and 5) are new event
// types, not new methods.
//
// The event set: ConfigureEvent, FrameEvent, CloseRequestedEvent,
// FocusEvent, WakeEvent, SettingsEvent, PointerEvent, ScrollEvent and
// KeyEvent.
//
// # Threading
//
//   - The UI thread is the goroutine that calls Run, locked to the OS main
//     thread (tela.Run does runtime.LockOSThread from an init function).
//     Cocoa requires the main thread; Win32 requires the thread that created
//     the windows. Linux backends do not need it but follow the same rule.
//   - All Backend, Window and Clipboard methods must be called on the UI
//     thread, before Run (on the same goroutine) or from inside the handler.
//     Exceptions, safe from any goroutine: [Backend.Wake], [Backend.Quit],
//     [Backend.Now].
//   - The handler is never called re-entrantly. If a method the handler calls
//     makes the OS deliver a message synchronously (Win32 SetWindowPos sends
//     WM_SIZE from inside the call), the backend queues the resulting event
//     and delivers it after the handler returns. Gio's "delivering" flag
//     solves the same problem.
//   - Other goroutines (raster workers, state setters, timers) talk to the UI
//     thread only by enqueueing work in their own structures and calling
//     Wake. Timers (long press, tooltips, cursor blink) are goroutines that
//     call Wake; the platform layer has no timer API.
//   - Panics in the handler propagate out of Run. Backends whose handler
//     calls cross foreign frames (Cocoa via purego, Win32 via
//     syscall.NewCallback) must recover in the callback, stop the native
//     loop, and re-panic from Run with the original value.
//
// # Frames and pacing (0% CPU when idle)
//
// The backend blocks in the OS until there is an input event, a Wake, or a
// requested frame that is due. It never polls on a timer while idle.
//
//   - [Window.RequestFrame] asks for one [FrameEvent]. Requests coalesce; a
//     FrameEvent consumes them all. Animations call RequestFrame again while
//     handling the FrameEvent.
//   - If the window has not drawn recently (no outstanding vsync or frame
//     callback, and no FrameEvent within about one refresh interval), the
//     FrameEvent is delivered on the next loop iteration, after the input
//     events of that iteration. Otherwise it is delivered when the display
//     is ready for a new frame. This holds even when the app skipped
//     Present: a handler that only calls RequestFrame is paced like one
//     that draws, and never makes the loop spin.
//   - A ConfigureEvent implies a frame request; that FrameEvent is delivered
//     even if pacing would delay it, because the OS is waiting for content
//     of the new size.
//   - Inside a FrameEvent the app lays out, paints, rasterizes and calls
//     [Window.Present]. Skipping Present is allowed when nothing changed.
//   - FrameEvent.Time is the frame clock (see [FrameEvent]); it is in the
//     time base of [Backend.Now], like input timestamps.
//
// # Shared loop
//
// The OS-independent part of the loop is one implementation, package
// internal/loop, used by every backend: the FIFO queue of window events, app
// events and deferred callbacks; coalesced Wake (one WakeEvent per pass) and
// Quit checks between deliveries; applying a ConfigureEvent to the window's
// Metrics before the handler sees it and forcing its frame; the frame-due
// rule above; and the deadline of the earliest timer-paced frame for the
// poll timeout. A backend keeps only its OS-specific I/O (reading the socket
// or the message queue, frame callbacks or display links) and decides when
// to run a delivery pass. internal/loop's documentation says how Win32 and
// Cocoa plug in.
//
// # Keyboard
//
// Key input arrives as [KeyEvent]s for the window that has keyboard focus.
// Every backend honors these rules; the headless backend enforces them on
// injected key events.
//
//   - Three descriptions (ADR-021, winit's model): Code is the physical key
//     ([KeyCode], a W3C "code" value), Key the logical key ([Key], a W3C
//     "key" value, reflecting every modifier but Ctrl), Text the produced
//     text (Ctrl ignored, never a control character, empty on release).
//     Linux backends map scancodes with the shared evdev table
//     (internal/evdev); every backend sets Location from Code.Location().
//   - Focus: a window gets KeyEvents only between a FocusEvent with Focused
//     true and the next one with Focused false. On Wayland keyboard focus
//     is wl_keyboard.enter/leave, which can arrive before or after the
//     xdg_toplevel activated state; the backend orders them so that the
//     FocusEvent comes first.
//   - Held keys when focus is lost: like winit (which does it on X11 and
//     Windows), the backend delivers, before the FocusEvent with Focused
//     false, a synthetic release (Synthetic true, no Text, no Mods) for
//     every key whose press it delivered and whose release it did not, in
//     press order, and stops key repeat. Tela requires this on every
//     backend, Wayland included, where the compositor only sends
//     wl_keyboard.leave.
//   - Keys already held when focus is gained produce no presses (winit
//     sends synthetic presses on X11 and Windows, which can fire shortcuts
//     such as Ctrl+W on refocus; Tela follows winit's Wayland behavior and
//     browsers instead), and their releases are dropped: every delivered
//     release follows a delivered press of the same Code (internal/keyboard
//     tracks this). Modifiers held at focus appear in the Mods of later
//     events.
//   - Repeat: the backend generates key repeat, never the app. A repeat is a
//     KeyPress with Repeat true and the Code of the held key; Key and Text
//     are recomputed for the current modifiers. Only the last pressed key
//     repeats, keys the keymap marks as non-repeating (modifiers) never do,
//     and repeat stops at the key's release, at the press of another key,
//     and when focus is lost. Wayland repeats client-side from
//     wl_keyboard.repeat_info (rate 0 disables repeat), with the repeat
//     deadline folded into the poll timeout like a paced frame (no timer
//     goroutine); X11 relies on the server's autorepeat with XKB detectable
//     auto-repeat (presses only, no fake releases in between); Win32 on
//     WM_KEYDOWN's previous-state bit; Cocoa on NSEvent.isARepeat.
//   - Mods is the modifier state after the event.
//   - Dead keys and compose sequences: the dead key's press has Key
//     KeyDead and no Text; intermediate presses have no Text; the final
//     press carries the composed text. Input method composition (phase 5)
//     arrives as its own events.
//
// # Coordinates, scale and framebuffer size
//
// Everything above this layer works in logical pixels (float32). A window's
// [Metrics] are its logical [Size] and its scale (physical pixels per
// logical pixel, possibly fractional). The framebuffer is allocated in
// physical pixels, and its size is always [Metrics.Physical], that is,
// [PhysicalSize](size, scale): logical × scale per axis, rounded half away
// from zero. All backends use this one function; none computes a buffer size
// any other way. Backends with logical-native sizes (Wayland, Cocoa,
// headless) report the OS size and let the rule produce the buffer size.
// Backends with physical-native sizes (Win32, X11) report
// [LogicalSize](physical, scale), which maps back to the same physical size.
//
// Physical / scale may differ from Size by less than a physical pixel, so
// renderers must cover the whole framebuffer (clear it to the root
// background) rather than only Size × scale.
//
// Pointer positions are logical pixels relative to the top-left of the
// content area: the area Tela draws, excluding decorations drawn by the OS.
//
// # Present
//
// Present(fb, damage) shows a frame. fb is premultiplied RGBA8 in sRGB
// (Go's image.RGBA convention), physical pixels, with fb.Rect equal to
// image.Rect(0, 0, Metrics().Physical()) exactly; anything else is a
// programmer error and panics ([CheckFramebuffer]). damage lists the changed
// rectangles in fb coordinates since the previous Present to the same
// window; nil or empty means the whole frame ([ClipDamage]). Backends read
// only the damaged pixels and keep the rest from the previous frame, so the
// app may rasterize only the damage. The first Present after a
// ConfigureEvent is always full.
//
// Ownership: the backend copies what it needs before Present returns (into
// a wl_shm buffer, an XImage/SHM segment, a DIB section, a CGImage), and
// never retains fb. The caller may reuse fb for the next frame immediately.
// Because the backend keeps its own copy, it can answer OS repaint requests
// (Win32 WM_PAINT, X11 Expose) without involving the app.
//
// A window is not shown until its first Present, which avoids a flash of
// unpainted content (Wayland maps a surface only when a buffer is attached;
// the other backends delay ShowWindow / XMapWindow / orderFront until then).
// Windows are opaque in phase 1; backends may ignore alpha.
//
// # Errors
//
// Platform and resource errors are returned as error: a missing display
// ([ErrUnavailable] from constructors), a lost connection (from Run), a
// failed buffer allocation (from Present). Programmer errors panic with an
// actionable message: invalid window options, a framebuffer of the wrong
// size, an invalid cursor, calling Run twice.
//
// # Mapping onto each OS
//
// Wayland (single fd). The UI thread loops: flush requests; dispatch queued
// events; deliver events to the handler; deliver due frames; then
// poll(display fd, eventfd) with no timeout. Wake writes the eventfd (an
// atomic flag avoids redundant writes); Quit sets a flag and wakes. Pacing
// uses wl_surface.frame: Present attaches the buffer, damages
// (wl_surface.damage_buffer, physical pixels), requests a frame callback and
// commits. A RequestFrame while a callback is outstanding waits for its done
// event; with none outstanding the FrameEvent is delivered on the next
// iteration if the window has not had one for an interval, otherwise the
// backend requests a frame callback with a commit that attaches no buffer
// and waits for it. Frame callbacks stop for hidden surfaces, which is the
// desired idle behavior. Scale: integer via wl_surface.preferred_buffer_scale (or
// wl_output.scale) and set_buffer_scale; fractional later via
// wp_fractional_scale_v1 (scale = n/120 as float64) and wp_viewporter
// (destination = logical size, buffer = PhysicalSize). xdg_surface.configure
// is acked right before the commit of the first buffer of the new size,
// which is the Present following the ConfigureEvent.
//
// X11 via jezek/xgb. xgb runs its own reader goroutine and exposes events
// through a blocking WaitForEvent, so the UI thread cannot poll the socket
// itself. A pump goroutine calls WaitForEvent and sends each event on a
// channel. The UI thread blocks in a select over that channel, a wake
// channel (buffer 1, non-blocking send from Wake), and a frame timer that is
// armed only while a frame is requested and due later (the timer period
// follows the monitor refresh rate; X11 has no frame callbacks without the
// Present extension). Sizes are physical (ConfigureNotify); the scale comes
// from Xft.dpi / 96 when set, else 1; logical = LogicalSize(physical,
// scale). Present uses PutImage (MIT-SHM later), keeping a copy for Expose.
//
// Win32. GetMessageW / TranslateMessage / DispatchMessageW on the UI thread;
// WndProc translates messages into events. Wake posts a private message to a
// message-only window: PostMessageW to a window, not PostThreadMessageW,
// because thread messages are dropped while a modal loop (move/resize, menus)
// runs, a known Win32 quirk. Pacing: a frame request arms a timer or a
// DwmFlush-paced post of a private message; both are dispatched inside
// modal loops too, so animations continue during a live resize. WM_SIZE and
// WM_DPICHANGED become a ConfigureEvent followed by a synchronous FrameEvent,
// so the resized window is painted before DefWindowProc continues. WM_PAINT
// is answered from the retained copy (SetDIBitsToDevice). Sizes are physical
// (per-monitor DPI v2); scale = dpi / 96. Quit posts WM_QUIT. Implicit
// pointer capture needs SetCapture / ReleaseCapture.
//
// Cocoa via purego. Run calls [NSApp run], which owns the loop; the backend
// is called back by NSApplication/NSWindow delegates and the NSView
// subclass, and calls the handler from there. Wake signals a CFRunLoopSource
// (CFRunLoopSourceSignal + CFRunLoopWakeUp, safe from any thread) added in
// kCFRunLoopCommonModes so it also fires during live resize (event tracking
// mode). Pacing: a frame request starts a display link (CVDisplayLink on
// its own thread signalling the same run loop source, or NSView.displayLink
// on macOS 14+); the link is stopped when no frame is requested. Sizes are
// points (logical); scale is backingScaleFactor. Present sets the layer's
// contents to a CGImage made from the copied pixels (RGBA premultiplied is
// native: kCGImageAlphaPremultipliedLast). Quit calls [NSApp stop:] and posts
// a dummy event so that run returns.
//
// Headless (subpackage headless). An in-memory backend with a controllable
// clock and an input injection API, used by tests, telatest and the CLI. It
// implements the same contract and also offers synchronous stepping.
package platform
