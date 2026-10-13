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
}

func (b *Backend) addSeat(name, version uint32) {
	s := &seat{b: b, name: name}
	s.wl = protocols.WlRegistryBind[protocols.WlSeat](b.registry, name, min(version, seatVersion))
	s.wl.SetCapabilitiesHandler(s.onCapabilities)
	b.seats = append(b.seats, s)
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
		b.lastSerial = e.Serial
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
		b.lastSerial = e.Serial
		s.ptr.leave(b.Now())
		after()
	})
	p.SetMotionHandler(func(e protocols.WlPointerMotionEvent) {
		s.ptr.motion(e.SurfaceX, e.SurfaceY, b.eventTime(e.Time))
		after()
	})
	p.SetButtonHandler(func(e protocols.WlPointerButtonEvent) {
		b.lastSerial = e.Serial
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
	p.SetFrameHandler(func(protocols.WlPointerFrameEvent) { s.flushPointer() })
	// axis_source and axis_stop are not needed yet (kinetic scrolling is a
	// later task); the generated code drops events without a handler.
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
	delta     [2]float64
	value120  [2]int32
	discrete  [2]int32
	hasDelta  bool
	hasV120   bool
	hasDiscr  bool
	axisTime  time.Duration
	axisFocus *Window
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
	if !p.hasDelta && !p.hasV120 && !p.hasDiscr {
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
func (p *pointerState) frame() []windowEvent {
	out := p.out
	p.out = nil
	if (p.hasDelta || p.hasV120 || p.hasDiscr) && p.axisFocus != nil {
		ev := platform.ScrollEvent{
			Kind:  platform.PointerMouse,
			Pos:   p.pos,
			Delta: platform.Point{X: float32(p.delta[1]), Y: float32(p.delta[0])},
			Mods:  p.mods,
			Time:  p.axisTime,
		}
		switch {
		case p.hasV120:
			ev.Steps = platform.Point{X: float32(p.value120[1]) / 120, Y: float32(p.value120[0]) / 120}
		case p.hasDiscr:
			ev.Steps = platform.Point{X: float32(p.discrete[1]), Y: float32(p.discrete[0])}
		}
		if ev.Delta != (platform.Point{}) || ev.Steps != (platform.Point{}) {
			out = append(out, windowEvent{p.axisFocus, ev})
		}
	}
	p.delta, p.value120, p.discrete = [2]float64{}, [2]int32{}, [2]int32{}
	p.hasDelta, p.hasV120, p.hasDiscr = false, false, false
	p.axisFocus = nil
	return out
}
