//go:build linux

package wayland

import (
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// seat is a bound wl_seat: its pointer (this file) and its keyboard
// (keyboard.go).
type seat struct {
	b       *Backend
	name    uint32 // registry name
	wl      *protocols.WlSeat
	kbd     *kbd // nil without the keyboard capability
	pointer *protocols.WlPointer
	shape   *protocols.WpCursorShapeDeviceV1 // nil without wp_cursor_shape_v1
	ptr     pointerState
	// enterSerial is the serial of the latest wl_pointer.enter, required
	// by set_cursor and set_shape.
	enterSerial uint32
	// serial is the serial of the seat's latest input event (pointer
	// enter, leave and button; keyboard enter, leave and key), used by
	// wl_data_device.set_selection. Compositors accept a selection from
	// the client with keyboard focus whose serial is not older than the
	// current selection's (wlroots, Mutter), so the newest serial is the
	// one most likely to be accepted.
	serial uint32
	data   *dataDevice // nil without wl_data_device_manager
	ti     *textInput  // nil without zwp_text_input_manager_v3
}

func (b *Backend) addSeat(name, version uint32) {
	s := &seat{b: b, name: name}
	s.wl = protocols.WlRegistryBind[protocols.WlSeat](b.registry, name, min(version, seatVersion))
	s.wl.SetCapabilitiesHandler(s.onCapabilities)
	b.seats = append(b.seats, s)
	s.attachDataDevice()
	s.attachTextInput()
}

// inputSerial records the serial of an input event of the seat.
func (s *seat) inputSerial(serial uint32) {
	s.b.lastSerial = serial
	s.serial = serial
}

func (s *seat) onCapabilities(e protocols.WlSeatCapabilitiesEvent) {
	has := e.Capabilities.Has(protocols.WlSeatCapabilityPointer)
	switch {
	case has && s.pointer == nil:
		s.pointer = s.wl.GetPointer()
		s.listen()
		s.attachCursorShape()
	case !has && s.pointer != nil:
		s.releasePointer()
	}
	hasKbd := e.Capabilities.Has(protocols.WlSeatCapabilityKeyboard)
	switch {
	case hasKbd && s.kbd == nil:
		s.bindKeyboard()
	case !hasKbd && s.kbd != nil:
		s.kbd.release()
		s.kbd = nil
		s.ptr.mods = 0
	}
}

// attachCursorShape creates the cursor shape device for the pointer, once
// both exist.
func (s *seat) attachCursorShape() {
	if s.shape == nil && s.pointer != nil && s.b.cursorShape != nil {
		s.shape = s.b.cursorShape.GetPointer(s.pointer)
	}
}

func (s *seat) releasePointer() {
	if s.ptr.focus != nil {
		// The device went away mid-sequence: end it for the app.
		s.ptr.leave(s.b.Now())
		s.flushPointer()
	}
	if s.shape != nil {
		s.shape.Destroy()
		s.shape = nil
	}
	if s.pointer.Version() >= 3 {
		s.pointer.Release()
	}
	s.pointer = nil
}

func (s *seat) destroy() {
	if s.pointer != nil {
		s.releasePointer()
	}
	if s.kbd != nil {
		s.kbd.release()
		s.kbd = nil
	}
	if s.data != nil {
		s.data.release()
		s.data = nil
	}
	if s.ti != nil {
		s.ti.release()
		s.ti = nil
	}
	if !s.wl.Destroyed() && s.wl.Version() >= 5 {
		s.wl.Release()
	}
}

// listen installs the wl_pointer handlers. Events are accumulated in ptr
// and turned into platform events at wl_pointer.frame (version 5+), or after
// each event on older seats, which have no frame event.
func (s *seat) listen() {
	p := s.pointer
	b := s.b
	framed := p.Version() >= 5
	after := func() {
		if !framed {
			s.flushPointer()
		}
	}
	p.SetEnterHandler(func(e protocols.WlPointerEnterEvent) {
		s.inputSerial(e.Serial)
		s.enterSerial = e.Serial
		w := b.surfaces[e.Surface]
		if w == nil {
			return // not ours (or already destroyed)
		}
		s.ptr.enter(w, e.SurfaceX, e.SurfaceY, b.Now())
		// The cursor image is undefined on enter: set it now.
		s.applyCursor(w.cursor)
		after()
	})
	p.SetLeaveHandler(func(e protocols.WlPointerLeaveEvent) {
		s.inputSerial(e.Serial)
		s.ptr.leave(b.Now())
		after()
	})
	p.SetMotionHandler(func(e protocols.WlPointerMotionEvent) {
		s.ptr.motion(e.SurfaceX, e.SurfaceY, b.eventTime(e.Time))
		after()
	})
	p.SetButtonHandler(func(e protocols.WlPointerButtonEvent) {
		s.inputSerial(e.Serial)
		s.ptr.button(e.Button, e.State == protocols.WlPointerButtonStatePressed, b.eventTime(e.Time))
		after()
	})
	p.SetAxisHandler(func(e protocols.WlPointerAxisEvent) {
		s.ptr.axis(e.Axis, e.Value, b.eventTime(e.Time))
		after()
	})
	p.SetAxisDiscreteHandler(func(e protocols.WlPointerAxisDiscreteEvent) {
		s.ptr.axisDiscrete(e.Axis, e.Discrete)
	})
	p.SetAxisValue120Handler(func(e protocols.WlPointerAxisValue120Event) {
		s.ptr.axisValue120(e.Axis, e.Value120)
	})
	p.SetAxisSourceHandler(func(e protocols.WlPointerAxisSourceEvent) {
		s.ptr.axisSource(e.AxisSource)
	})
	p.SetAxisStopHandler(func(e protocols.WlPointerAxisStopEvent) {
		s.ptr.axisStop(e.Axis, b.eventTime(e.Time))
		after()
	})
	p.SetFrameHandler(func(protocols.WlPointerFrameEvent) { s.flushPointer() })
}

// flushPointer queues the events of the finished pointer frame.
func (s *seat) flushPointer() {
	for _, e := range s.ptr.frame() {
		s.b.enqueue(e.w, e.ev)
	}
}

// applyCursor shows c while the pointer is over a window of ours. With
// wp_cursor_shape_v1 any shape works, and CursorNone hides the cursor with
// wl_pointer.set_cursor(NULL). Without the protocol the cursor is left
// alone, including for CursorNone, because a hidden cursor could not be
// restored without loading an xcursor theme (a later task).
func (s *seat) applyCursor(c platform.Cursor) {
	if s.pointer == nil || s.shape == nil {
		return
	}
	if c == platform.CursorNone {
		s.pointer.SetCursor(s.enterSerial, nil, 0, 0)
		return
	}
	s.shape.SetShape(s.enterSerial, cursorShape(c))
}

// cursorShape maps a visible platform.Cursor to its wp_cursor_shape_v1
// shape. platform.Cursor follows the protocol's order, so the mapping is
// an offset of one (the protocol starts at 1).
func cursorShape(c platform.Cursor) protocols.WpCursorShapeDeviceV1Shape {
	return protocols.WpCursorShapeDeviceV1Shape(c) + 1
}

// Linux input event codes (linux/input-event-codes.h) for mouse buttons.
const (
	btnLeft    = 0x110
	btnRight   = 0x111
	btnMiddle  = 0x112
	btnSide    = 0x113
	btnExtra   = 0x114
	btnForward = 0x115
	btnBack    = 0x116
)

// pointerButton maps a Linux button code to a platform button. BTN_SIDE and
// BTN_EXTRA are what common mice send for back and forward (winit and
// Chromium map them the same way); BTN_BACK/BTN_FORWARD are rarer.
// Left-handed setups are swapped by libinput before the compositor, so
// BTN_LEFT is always the primary button.
func pointerButton(code uint32) platform.PointerButton {
	switch code {
	case btnLeft:
		return platform.ButtonPrimary
	case btnRight:
		return platform.ButtonSecondary
	case btnMiddle:
		return platform.ButtonMiddle
	case btnSide, btnBack:
		return platform.ButtonBack
	case btnExtra, btnForward:
		return platform.ButtonForward
	}
	return platform.ButtonNone
}

// windowEvent is a platform event addressed to a window.
type windowEvent struct {
	w  *Window
	ev platform.Event
}

// pointerState turns wl_pointer events into platform events. It knows no
// Wayland objects, so it is tested on its own.
type pointerState struct {
	focus   *Window
	pos     platform.Point
	buttons platform.Buttons
	// mods is the seat's keyboard modifier state, reported with every
	// pointer and scroll event (kbd.updatePointerMods keeps it current).
	mods platform.Modifiers

	// Events of the current frame, in order.
	out []windowEvent

	// Scroll accumulated in the current frame. Index 0 is the vertical
	// axis, 1 the horizontal one (the wl_pointer.axis enum values).
	delta    [2]float64
	value120 [2]int32
	discrete [2]int32
	hasDelta bool
	hasV120  bool
	hasDiscr bool
	// axisTime is the time of the frame's first axis or axis_stop event,
	// the only axis events with a time: axis_value120 and axis_discrete
	// come before the axis event they belong to (seat version 8 sends
	// value120 first), so they must not decide it.
	axisTime  time.Duration
	axisFocus *Window
	// source is the frame's wl_pointer.axis_source (hasSource), stop
	// reports a wl_pointer.axis_stop in the frame, and finger a touchpad
	// gesture in progress: its events have ScrollPhaseUpdate, and the
	// frame that stops it without moving ends it (ScrollPhaseEnd).
	source    protocols.WlPointerAxisSource
	hasSource bool
	stop      bool
	finger    bool
}

func (p *pointerState) enter(w *Window, x, y float64, now time.Duration) {
	p.focus = w
	p.pos = platform.Point{X: float32(x), Y: float32(y)}
	p.emit(platform.PointerEnter, platform.ButtonNone, now)
}

// leave ends the pointer's stay on the focused window. Wayland does not
// send leave during an implicit grab, so buttons still held mean the
// compositor took the pointer away (an interactive move, a grab by another
// client): the press is reported as cancelled, not released.
func (p *pointerState) leave(now time.Duration) {
	if p.focus == nil {
		return
	}
	if p.buttons != 0 {
		p.buttons = 0
		p.emit(platform.PointerCancel, platform.ButtonNone, now)
	}
	p.emit(platform.PointerLeave, platform.ButtonNone, now)
	p.focus = nil
	// The stop of a touchpad gesture, if any, will not be ours.
	p.finger = false
}

func (p *pointerState) motion(x, y float64, t time.Duration) {
	if p.focus == nil {
		return
	}
	p.pos = platform.Point{X: float32(x), Y: float32(y)}
	p.emit(platform.PointerMove, platform.ButtonNone, t)
}

func (p *pointerState) button(code uint32, pressed bool, t time.Duration) {
	btn := pointerButton(code)
	if p.focus == nil || btn == platform.ButtonNone {
		return
	}
	action := platform.PointerUp
	if pressed {
		if p.buttons.Has(btn) {
			return // duplicate press (two devices on one seat)
		}
		p.buttons |= btn.Mask()
		action = platform.PointerDown
	} else {
		if !p.buttons.Has(btn) {
			return // release of a press that happened elsewhere
		}
		p.buttons &^= btn.Mask()
	}
	p.emit(action, btn, t)
}

func (p *pointerState) axis(a protocols.WlPointerAxis, v float64, t time.Duration) {
	if a > 1 || p.focus == nil {
		return
	}
	if !p.hasDelta && !p.stop { // the frame's first event with a time
		p.axisTime = t
	}
	p.delta[a] += v
	p.hasDelta = true
	p.axisFocus = p.focus
}

func (p *pointerState) axisDiscrete(a protocols.WlPointerAxis, n int32) {
	if a > 1 || p.focus == nil {
		return
	}
	p.discrete[a] += n
	p.hasDiscr = true
	p.axisFocus = p.focus
}

func (p *pointerState) axisValue120(a protocols.WlPointerAxis, v int32) {
	if a > 1 || p.focus == nil {
		return
	}
	p.value120[a] += v
	p.hasV120 = true
	p.axisFocus = p.focus
}

// axisSource records the source of the frame's axis events.
func (p *pointerState) axisSource(src protocols.WlPointerAxisSource) {
	p.source, p.hasSource = src, true
}

// axisStop records the end of the movement on one axis: the fingers left
// the touchpad (only finger sources are guaranteed to send it; the
// others may or may not, so their sequences stay unterminated).
func (p *pointerState) axisStop(a protocols.WlPointerAxis, t time.Duration) {
	if a > 1 || p.focus == nil {
		return
	}
	if !p.hasDelta && !p.stop {
		p.axisTime = t
	}
	p.stop = true
	p.axisFocus = p.focus
}

func (p *pointerState) emit(a platform.PointerAction, btn platform.PointerButton, t time.Duration) {
	p.out = append(p.out, windowEvent{p.focus, platform.PointerEvent{
		Action:  a,
		Kind:    platform.PointerMouse,
		Pos:     p.pos,
		Button:  btn,
		Buttons: p.buttons,
		Mods:    p.mods,
		Time:    t,
	}})
}

// frame ends a pointer frame and returns its events: the pointer events in
// protocol order, then at most one ScrollEvent combining every axis event
// of the frame (a diagonal scroll is one event, as wl_pointer.frame
// intends).
//
// Signs: Wayland axis values are positive down and right, like the DOM, so
// they are used as is (winit negates them because its convention is the
// opposite). Steps come from axis_value120 / 120 (seat version 8) or
// axis_discrete (versions 5-7); a frame never has both.
//
// Phases (GTK 4's kinetic scrolling reads the same events): a frame whose
// axis_source is finger starts or continues a touchpad gesture, whose
// events have ScrollPhaseUpdate; a frame of that gesture with an axis_stop
// and no movement ends it with a ScrollPhaseEnd event without delta. A
// stop together with movement means that one axis stopped while the other
// goes on (the protocol's rule), so the gesture continues. Other sources
// may or may not send axis_stop, so their events have ScrollPhaseNone and
// their stops are ignored.
func (p *pointerState) frame() []windowEvent {
	out := p.out
	p.out = nil
	moved := p.hasDelta || p.hasV120 || p.hasDiscr
	if p.hasSource && moved {
		p.finger = p.source == protocols.WlPointerAxisSourceFinger
	}
	phase := platform.ScrollPhaseNone
	if p.finger {
		phase = platform.ScrollPhaseUpdate
		if p.stop && !moved {
			phase = platform.ScrollPhaseEnd
			p.finger = false
		}
	}
	if (moved || phase == platform.ScrollPhaseEnd) && p.axisFocus != nil {
		ev := platform.ScrollEvent{
			Kind:  platform.PointerMouse,
			Pos:   p.pos,
			Delta: platform.Point{X: float32(p.delta[1]), Y: float32(p.delta[0])},
			Phase: phase,
			Mods:  p.mods,
			Time:  p.axisTime,
		}
		switch {
		case p.hasV120:
			ev.Steps = platform.Point{X: float32(p.value120[1]) / 120, Y: float32(p.value120[0]) / 120}
		case p.hasDiscr:
			ev.Steps = platform.Point{X: float32(p.discrete[1]), Y: float32(p.discrete[0])}
		}
		if ev.Delta != (platform.Point{}) || ev.Steps != (platform.Point{}) || phase == platform.ScrollPhaseEnd {
			out = append(out, windowEvent{p.axisFocus, ev})
		}
	}
	p.delta, p.value120, p.discrete = [2]float64{}, [2]int32{}, [2]int32{}
	p.hasDelta, p.hasV120, p.hasDiscr = false, false, false
	p.hasSource, p.stop = false, false
	p.axisFocus = nil
	return out
}
