package platform

import "time"

// Event is the sealed set of events a backend delivers to the [Handler].
// Only this package defines event types; new kinds (text input, IME
// composition, touch gestures) are added here as new types, so adding one
// never changes the Backend or Window interfaces.
//
// The window-scoped events are delivered with the Window they belong to.
// App-scoped events ([WakeEvent], [SettingsEvent], [NotificationEvent],
// [TrayEvent]) are delivered with a nil Window.
type Event interface {
	isEvent()
}

// ConfigureEvent reports the window's logical size and scale. It is the
// first event every window receives, and it is delivered again whenever the
// size, the scale, or both change (a DPI change usually changes both at
// once, which is why there is no separate scale event).
//
// [Window.Metrics] changes only when a ConfigureEvent is delivered, so the
// handler always sees a consistent pair. After a ConfigureEvent the backend
// behaves as if [Window.RequestFrame] had been called: a [FrameEvent]
// follows (once the physical size is non-empty), and the next Present must
// be a full frame of size Metrics.Physical().
//
// An empty physical size means the window is not visible (for example
// minimized on Win32). No FrameEvents are delivered until it grows again;
// frame requests made meanwhile stay pending.
type ConfigureEvent struct {
	Metrics
}

// FrameEvent tells the app that now is a good time to draw the window: a
// requested frame is due (paced by the display's vsync / frame callbacks), or
// the OS needs new content after a ConfigureEvent.
//
// The app renders and calls [Window.Present], usually before returning. If
// nothing changed it may skip Present. Each FrameEvent consumes all frame
// requests made before it; to animate, call RequestFrame again while
// handling it (requestAnimationFrame style).
type FrameEvent struct {
	// Time is the frame clock: monotonic, never decreasing across the
	// FrameEvents of one backend, in the time base of [Backend.Now].
	// Animations advance with it. It is the best available estimate of
	// when the frame will be shown (vsync target where known, otherwise the
	// time the frame was dispatched).
	Time time.Duration
}

// CloseRequestedEvent reports that the user asked to close the window (close
// button, Alt+F4, Cmd+W via the window menu, WM_DELETE_WINDOW). The window
// stays open until the app calls [Window.Close].
type CloseRequestedEvent struct{}

// FocusEvent reports that the window gained or lost keyboard focus
// (activation). A window receives KeyEvents only between a FocusEvent with
// Focused true and the next one with Focused false; see the package
// documentation ("Keyboard") for what happens to held keys.
type FocusEvent struct {
	Focused bool
}

// WakeEvent is delivered, with a nil Window, after [Backend.Wake]. Wakes
// coalesce: many Wake calls may yield a single WakeEvent, but every Wake
// call is followed by at least one WakeEvent delivered after that call
// started. Apps use it to drain queues filled by other goroutines (state
// setters, tela.Post).
type WakeEvent struct{}

// SettingsEvent is delivered, with a nil Window, when [SystemSettings]
// change (for example the user toggles dark mode).
type SettingsEvent struct {
	Settings SystemSettings
}

// PointerAction is what happened to a pointer in a [PointerEvent].
type PointerAction uint8

const (
	// PointerMove: the pointer moved. While any button is held the window
	// keeps receiving moves even outside its bounds (implicit capture).
	PointerMove PointerAction = iota
	// PointerEnter: the pointer entered the content area.
	PointerEnter
	// PointerLeave: the pointer left the content area. Pos is the last
	// known position.
	PointerLeave
	// PointerDown: Button was pressed.
	PointerDown
	// PointerUp: Button was released.
	PointerUp
	// PointerCancel: the OS took the pointer sequence away (touch cancel,
	// lost capture). Treat as an aborted press, not as a release.
	PointerCancel
)

var pointerActionNames = [...]string{"move", "enter", "leave", "down", "up", "cancel"}

func (a PointerAction) String() string {
	if int(a) < len(pointerActionNames) {
		return pointerActionNames[a]
	}
	return "PointerAction(invalid)"
}

// PointerKind is the device behind a pointer.
type PointerKind uint8

const (
	PointerMouse PointerKind = iota // mouse or touchpad cursor
	PointerTouch                    // finger on a touchscreen
	PointerPen                      // stylus
)

var pointerKindNames = [...]string{"mouse", "touch", "pen"}

func (k PointerKind) String() string {
	if int(k) < len(pointerKindNames) {
		return pointerKindNames[k]
	}
	return "PointerKind(invalid)"
}

// PointerID distinguishes simultaneous pointers (touch points). The mouse is
// always 0.
type PointerID uint32

// PointerButton is a pointer button. Backends report the logical button:
// primary is whatever the OS treats as primary (left-handed setups swap it).
type PointerButton uint8

const (
	ButtonNone      PointerButton = iota
	ButtonPrimary                 // left mouse button, touch contact, pen tip
	ButtonSecondary               // right mouse button, pen barrel button
	ButtonMiddle                  // middle (wheel) button
	ButtonBack                    // X1 / back
	ButtonForward                 // X2 / forward
)

// Buttons is the set of held pointer buttons, one bit per PointerButton.
type Buttons uint8

// Has reports whether b is held.
func (s Buttons) Has(b PointerButton) bool { return b != ButtonNone && s&b.Mask() != 0 }

// Mask returns the Buttons bit of b (0 for ButtonNone).
func (b PointerButton) Mask() Buttons {
	if b == ButtonNone {
		return 0
	}
	return 1 << (b - 1)
}

// Modifiers is the set of held keyboard modifiers at the time of an input
// event (for a [KeyEvent], after the event).
//
// Only the four shortcut modifiers are reported, like winit's
// ModifiersState. AltGr (ISO_Level3_Shift on Linux, the right Alt key that
// Windows reports as Ctrl+Alt) is not one of them: it selects characters,
// which KeyEvent.Key and Text already reflect, and a backend must not report
// it as ModCtrl or ModAlt (the Win32 backend drops the fake left Ctrl that
// Windows sends with AltGr, as winit does). Lock states (Caps Lock, Num
// Lock) are not modifiers either: they are not held, and including them
// would make every comparison like Mods == ModCtrl fail while Num Lock is
// on.
//
// Pointer and scroll events report it as well: the state of the keyboard
// of the same seat (on Wayland, from wl_keyboard.modifiers). Backends
// without keyboard support yet leave it zero.
type Modifiers uint8

const (
	ModShift Modifiers = 1 << iota
	ModCtrl
	ModAlt
	ModSuper // Windows / Command / Super key
)

// PointerEvent is a pointer enter, leave, move, press, release or cancel.
type PointerEvent struct {
	Action PointerAction
	Kind   PointerKind
	ID     PointerID
	// Pos is in logical pixels relative to the content area's top-left.
	// It may lie outside the window while a button is held.
	Pos Point
	// Button is the button that changed, for PointerDown and PointerUp;
	// ButtonNone otherwise.
	Button PointerButton
	// Buttons is the set of buttons held after this event.
	Buttons Buttons
	Mods    Modifiers
	// Time is when the event happened, in the time base of [Backend.Now].
	Time time.Duration
}

// ScrollEvent is a mouse wheel or touchpad scroll.
//
// Signs follow the DOM WheelEvent: positive Y scrolls down (toward the end
// of the content), positive X scrolls right. Backends normalize the OS
// convention (Win32 WM_MOUSEWHEEL and X11 button 4 are "up = positive").
type ScrollEvent struct {
	Kind PointerKind
	// Pos is the pointer position in logical pixels.
	Pos Point
	// Delta is the precise scroll distance in logical pixels, as reported
	// by touchpads and high-resolution devices (Wayland axis value, macOS
	// scrollingDelta with hasPreciseScrollingDeltas). Zero when the OS only
	// reports wheel steps (Win32, X11 core buttons).
	Delta Point
	// Steps is the distance in wheel notches. High-resolution wheels report
	// fractions (Wayland axis_value120 / 120, Win32 WHEEL_DELTA / 120).
	// Zero for touchpads. When non-zero, the layers above convert it to a
	// distance (lines × line height) instead of trusting Delta, so a notch
	// scrolls the same amount on every platform.
	Steps Point
	// Phase marks the gesture boundaries of touchpad scrolling, which the
	// layers above need for kinetic scrolling (a fling after the fingers
	// lift, as GTK and browsers do): see [ScrollPhase].
	Phase ScrollPhase
	Mods  Modifiers
	// Time is when the event happened, in the time base of [Backend.Now].
	Time time.Duration
}

// ScrollPhase says where a [ScrollEvent] is in a scroll gesture.
//
// Only devices whose gestures have a reported end use the phases: a
// touchpad on Wayland (wl_pointer.axis_source finger, ended by
// wl_pointer.axis_stop when the fingers lift) and, when its backend
// arrives, macOS (NSEvent.phase). Wheels, and backends that cannot tell
// when a gesture ends (X11: XInput2 smooth scrolling has no stop signal),
// report ScrollPhaseNone, so the layers above never start a fling for
// them.
type ScrollPhase uint8

const (
	// ScrollPhaseNone is an event outside a gesture: wheel notches, or a
	// device or backend without gesture boundaries.
	ScrollPhaseNone ScrollPhase = iota
	// ScrollPhaseUpdate is an event of a touchpad gesture in progress
	// (fingers on the pad): more events follow, and a ScrollPhaseEnd
	// event ends the gesture.
	ScrollPhaseUpdate
	// ScrollPhaseEnd ends a gesture: the fingers left the touchpad. Delta
	// is usually zero (the stop marker alone); when the backend received
	// a last movement together with the stop, Delta carries it.
	ScrollPhaseEnd
)

var scrollPhaseNames = [...]string{"none", "update", "end"}

func (p ScrollPhase) String() string {
	if int(p) < len(scrollPhaseNames) {
		return scrollPhaseNames[p]
	}
	return "ScrollPhase(invalid)"
}

// WindowStateEvent reports a change of the window's [WindowState]:
// maximized, fullscreen, minimized (where the OS tells), client-side
// decorations, position (where the OS tells). [Window.State] changes only
// when one is delivered, like Metrics with ConfigureEvent.
//
// It is never delivered before the window's first ConfigureEvent. A window
// whose initial state is not the zero WindowState (created maximized or
// frameless, or at a known position) gets one right after its first
// ConfigureEvent. When one OS notification changes both the state and the
// size (maximizing), the WindowStateEvent is queued before the
// ConfigureEvent, so the FrameEvent that follows sees both. Bursts
// coalesce (a window dragged across the screen yields position updates at
// the loop's pace, not per OS message), and a change back to the delivered
// state before delivery yields nothing. ConfigureEvent semantics are
// unchanged: size and scale arrive only there.
type WindowStateEvent struct {
	State WindowState
}

// NotificationEventKind is what happened to a notification.
type NotificationEventKind uint8

const (
	// NotificationActivated: the user clicked the notification itself.
	NotificationActivated NotificationEventKind = iota
	// NotificationActionInvoked: the user clicked one of its actions
	// (NotificationEvent.Action).
	NotificationActionInvoked
	// NotificationClosed: the notification went away (dismissed by the
	// user, expired, or replaced) as reported by the OS. Not delivered
	// for CloseNotification, nor where the OS does not report it.
	NotificationClosed
)

var notificationEventNames = [...]string{"activated", "action", "closed"}

func (k NotificationEventKind) String() string {
	if int(k) < len(notificationEventNames) {
		return notificationEventNames[k]
	}
	return "NotificationEventKind(invalid)"
}

// NotificationEvent is delivered, with a nil Window, when the user
// interacts with a notification shown by [Desktop.Notify].
type NotificationEvent struct {
	ID   NotificationID
	Kind NotificationEventKind
	// Action is the NotificationAction.ID for NotificationActionInvoked.
	Action string
}

// TrayEventKind is what happened to a tray icon.
type TrayEventKind uint8

const (
	// TrayActivated: the primary activation of the icon (a primary click;
	// some Linux hosts open the menu instead and never send it).
	TrayActivated TrayEventKind = iota
	// TrayMenuItem: the user chose the menu item TrayEvent.Item.
	TrayMenuItem
)

var trayEventNames = [...]string{"activated", "menu-item"}

func (k TrayEventKind) String() string {
	if int(k) < len(trayEventNames) {
		return trayEventNames[k]
	}
	return "TrayEventKind(invalid)"
}

// TrayEvent is delivered, with a nil Window, when the user activates a tray
// icon added with [Desktop.NewTray] or one of its menu items.
type TrayEvent struct {
	Tray TrayID
	Kind TrayEventKind
	// Item is the activated MenuItem's ID for TrayMenuItem. Only enabled
	// items without a submenu are reported.
	Item MenuItemID
}

func (ConfigureEvent) isEvent()      {}
func (WindowStateEvent) isEvent()    {}
func (NotificationEvent) isEvent()   {}
func (TrayEvent) isEvent()           {}
func (FrameEvent) isEvent()          {}
func (CloseRequestedEvent) isEvent() {}
func (FocusEvent) isEvent()          {}
func (WakeEvent) isEvent()           {}
func (SettingsEvent) isEvent()       {}
func (PointerEvent) isEvent()        {}
func (ScrollEvent) isEvent()         {}
func (KeyEvent) isEvent()            {}
