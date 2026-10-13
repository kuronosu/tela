//go:build linux

package x11

import (
	"image"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// translate turns one X event into queued platform events, or handles it
// directly when the app is not involved (Expose, _NET_WM_PING, Xft.dpi
// changes). It runs on the UI thread, outside the handler.
//
// Input timestamps are the receive time on Backend.Now's clock rather than
// the X server time in the event, which has another origin.
func (b *Backend) translate(e xevent) {
	if e.err != nil {
		// X errors of unchecked requests. The expected ones are races
		// with the window manager or with Close (BadWindow for a window
		// destroyed while a request was in flight); nothing here can be
		// recovered, and checked requests report their own errors.
		return
	}
	if !b.kbd.beforeEvent(e.ev) {
		return
	}
	switch ev := e.ev.(type) {
	case xgb.RawEvent:
		b.rawEvent(ev)

	case xproto.ExposeEvent:
		if w := b.windows[ev.Window]; w != nil {
			r := image.Rect(int(ev.X), int(ev.Y), int(ev.X)+int(ev.Width), int(ev.Y)+int(ev.Height))
			w.expose = w.expose.Union(r)
			// The server sends a series of rectangles; Count is the
			// number still to come. Repaint once per series (winit and
			// Gio coalesce the same way).
			if ev.Count == 0 {
				w.flushExpose()
			}
		}

	case xproto.ConfigureNotifyEvent:
		// Only the size matters. Note that real ConfigureNotify events
		// carry a position relative to the parent (the window manager's
		// frame) while synthetic ones sent by the window manager carry
		// root coordinates (ICCCM 4.1.5); winit tracks both to report
		// moves, which Tela does not need.
		if w := b.windows[ev.Window]; w != nil && ev.Event == ev.Window {
			size := image.Pt(int(ev.Width), int(ev.Height))
			if size != w.osSize {
				w.osSize = size
				b.queueConfigure(w)
			}
		}

	case xproto.MapNotifyEvent:
		if w := b.windows[ev.Window]; w != nil && ev.Event == ev.Window {
			w.SetHidden(false)
		}

	case xproto.UnmapNotifyEvent:
		// The window manager unmaps the client window when it is
		// iconified (ICCCM 4.1.4). Frames pause until it is mapped again.
		if w := b.windows[ev.Window]; w != nil && ev.Event == ev.Window && w.mapped {
			w.SetHidden(true)
		}

	// Core pointer events: only without XInput2 (windows then select
	// XI2 events instead).
	case xproto.ButtonPressEvent:
		if w := b.windows[ev.Event]; w != nil {
			b.ptrButton(b.corePtr(w, ev.State, ev.EventX, ev.EventY), uint32(ev.Detail), true)
		}
	case xproto.ButtonReleaseEvent:
		if w := b.windows[ev.Event]; w != nil {
			b.ptrButton(b.corePtr(w, ev.State, ev.EventX, ev.EventY), uint32(ev.Detail), false)
		}
	case xproto.MotionNotifyEvent:
		if w := b.windows[ev.Event]; w != nil {
			b.ptrMotion(b.corePtr(w, ev.State, ev.EventX, ev.EventY))
		}
	case xproto.EnterNotifyEvent:
		if w := b.windows[ev.Event]; w != nil {
			b.ptrEnter(b.corePtr(w, ev.State, ev.EventX, ev.EventY), ev.Mode, ev.Detail)
		}
	case xproto.LeaveNotifyEvent:
		if w := b.windows[ev.Event]; w != nil {
			b.ptrLeave(b.corePtr(w, ev.State, ev.EventX, ev.EventY), ev.Mode, ev.Detail)
		}

	case xproto.KeyPressEvent:
		b.kbd.keyPress(ev)
	case xproto.KeyReleaseEvent:
		b.kbd.keyRelease(ev)
	case xproto.KeymapNotifyEvent:
		b.kbd.keymapNotify(ev.Keys)
	case xproto.MappingNotifyEvent:
		if ev.Request == xproto.MappingKeyboard || ev.Request == xproto.MappingModifier {
			b.kbd.mappingChanged()
		}

	case xproto.FocusInEvent:
		if w := b.windows[ev.Event]; w != nil && realFocusChange(ev.Mode, ev.Detail) && !w.focused {
			b.kbd.focusIn(w)
			w.focused = true
			b.enqueue(w, platform.FocusEvent{Focused: true})
		}
	case xproto.FocusOutEvent:
		w := b.windows[ev.Event]
		switch {
		case w == nil || !w.focused:
		case realFocusChange(ev.Mode, ev.Detail):
			b.blur(w)
		case ev.Mode == xproto.NotifyModeGrab && ev.Detail != xproto.NotifyDetailPointer && ev.Detail != xproto.NotifyDetailInferior:
			// Another client grabbed the keyboard (a window manager's
			// Alt+Tab switcher): the window stays focused, but the
			// releases of the keys held now go to the grabber.
			if b.kbd.focus == w {
				b.kbd.releaseHeld()
			}
		}

	case xproto.ClientMessageEvent:
		w := b.windows[ev.Window]
		if w == nil || ev.Type != b.atoms.wmProtocols || ev.Format != 32 || len(ev.Data.Data32) == 0 {
			return
		}
		switch xproto.Atom(ev.Data.Data32[0]) {
		case b.atoms.wmDeleteWindow:
			b.enqueue(w, platform.CloseRequestedEvent{})
		case b.atoms.netWMPing:
			// EWMH _NET_WM_PING: send the message back to the root
			// window so the window manager knows the app is alive and
			// does not offer to kill it. Answered here, without the
			// app, like winit does.
			ev.Window = b.root
			xproto.SendEvent(b.conn, false, b.root,
				xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect, string(ev.Bytes()))
		}

	case xproto.PropertyNotifyEvent:
		if ev.Window == b.root && ev.Atom == xproto.AtomResourceManager {
			b.updateScale()
		}
	}
}

// realFocusChange filters FocusIn/FocusOut events that do not change which
// window has keyboard focus: the pairs generated when a keyboard grab
// starts or ends (NotifyGrab / NotifyUngrab, for example while a window
// manager shows its Alt+Tab switcher) and NotifyPointer events, which
// describe the window under the pointer. SDL and GLFW filter the same set.
func realFocusChange(mode, detail byte) bool {
	if mode == xproto.NotifyModeGrab || mode == xproto.NotifyModeUngrab {
		return false
	}
	return detail != xproto.NotifyDetailPointer && detail != xproto.NotifyDetailInferior
}

// ptr is a pointer event decoded from the core protocol or from XInput2.
type ptr struct {
	w    *Window
	x, y float64          // physical pixels, relative to the window
	held platform.Buttons // buttons held before the event
	mods platform.Modifiers
	// extra are the bits of held that come from w.extra (core events).
	extra platform.Buttons
}

// corePtr decodes the common fields of a core pointer event. The core
// state has no bits for the back and forward buttons, which the window
// tracks (w.extra).
func (b *Backend) corePtr(w *Window, state uint16, x, y int16) ptr {
	return ptr{w: w, x: float64(x), y: float64(y), held: buttonsFromState(state) | w.extra, mods: b.kbd.mods(uint32(state)), extra: w.extra}
}

// posf converts event coordinates (physical pixels) to logical pixels.
func (w *Window) posf(x, y float64) platform.Point {
	s := w.b.scale
	return platform.Point{X: float32(x / s), Y: float32(y / s)}
}

// pointer queues a pointer event, coalescing consecutive moves: a move
// that directly follows a queued move of the same window with the same
// buttons and modifiers replaces it.
func (b *Backend) pointer(w *Window, pe platform.PointerEvent) {
	pe.Kind = platform.PointerMouse
	pe.Time = b.Now()
	if pe.Action != platform.PointerMove {
		b.enqueue(w, pe)
		return
	}
	b.loop.Coalesce(w, pe, func(e platform.Event) bool {
		prev, ok := e.(platform.PointerEvent)
		return ok && prev.Action == platform.PointerMove && prev.Buttons == pe.Buttons && prev.Mods == pe.Mods
	})
}

// ptrMotion translates a pointer motion.
func (b *Backend) ptrMotion(p ptr) {
	w := p.w
	w.lastPos = w.posf(p.x, p.y)
	b.pointer(w, platform.PointerEvent{
		Action:  platform.PointerMove,
		Pos:     w.lastPos,
		Buttons: p.held,
		Mods:    p.mods,
	})
}

// ptrEnter translates the pointer entering a window.
func (b *Backend) ptrEnter(p ptr, mode, detail byte) {
	w := p.w
	if detail == xproto.NotifyDetailInferior {
		return
	}
	if w.leavePending {
		if mode == xproto.NotifyModeNormal && p.held != 0 {
			// Back inside before the buttons were released: the app
			// never saw the pointer leave.
			w.leavePending = false
			return
		}
		// The implicit grab ended without a ButtonRelease reaching
		// this window (NotifyUngrab, or no button held any more):
		// another grab took the release. End the old stay for the
		// app (cancel, then the deferred leave) before the enter.
		b.cancelPress(w, p.mods)
		b.leave(w, 0, p.mods)
		p.held &^= p.extra // cancelPress forgot them
	}
	if !w.inside {
		w.inside = true
		w.lastPos = w.posf(p.x, p.y)
		b.pointer(w, platform.PointerEvent{
			Action:  platform.PointerEnter,
			Pos:     w.lastPos,
			Buttons: p.held,
			Mods:    p.mods,
		})
	}
}

// ptrLeave translates the pointer leaving a window. Crossing modes are not
// filtered: a pointer grab by the window manager (NotifyGrab) also takes
// the pointer away from the app, and the matching NotifyUngrab enter
// brings it back. The inside flag drops duplicates.
func (b *Backend) ptrLeave(p ptr, mode, detail byte) {
	w := p.w
	if detail == xproto.NotifyDetailInferior || !w.inside {
		return
	}
	if p.held != 0 && mode == xproto.NotifyModeNormal {
		// X reports the crossing during the implicit grab of a held
		// button, while the window keeps receiving the moves. Wayland
		// (no wl_pointer.leave until the grab ends) and Win32
		// (SetCapture) report the leave after the release, so the
		// backend defers it to the release that ends the grab.
		w.leavePending = true
		return
	}
	w.lastPos = w.posf(p.x, p.y)
	if p.held != 0 || w.leavePending {
		// A grab (NotifyGrab: the window manager's Alt+drag, a
		// screenshot tool; NotifyUngrab: our implicit grab ended
		// without a release) took the pointer while the app still
		// had a press: its release will never reach this window.
		// Like the Wayland backend when the compositor takes the
		// pointer, abort the press, then report the leave.
		b.cancelPress(w, p.mods)
	}
	b.leave(w, 0, p.mods)
}

// ptrButton translates a button press or release (core button numbers,
// which XInput2 shares).
func (b *Backend) ptrButton(p ptr, detail uint32, press bool) {
	w := p.w
	pos := w.posf(p.x, p.y)
	w.lastPos = pos
	btn, steps, ok := mapButton(detail)
	if !ok {
		return
	}
	if steps != (platform.Point{}) {
		// Wheel buttons send a press and a release per notch; the press
		// is the scroll step.
		if press {
			b.enqueue(w, platform.ScrollEvent{
				Kind:  platform.PointerMouse,
				Pos:   pos,
				Steps: steps,
				Mods:  p.mods,
				Time:  b.Now(),
			})
		}
		return
	}
	// The held set is the state before the event, so the pressed or
	// released button is applied on top of it.
	held := p.held
	action := platform.PointerDown
	if press {
		held |= btn.Mask()
	} else {
		held &^= btn.Mask()
		action = platform.PointerUp
	}
	if btn == platform.ButtonBack || btn == platform.ButtonForward {
		if press {
			w.extra |= btn.Mask()
		} else {
			w.extra &^= btn.Mask()
		}
	}
	b.pointer(w, platform.PointerEvent{
		Action:  action,
		Pos:     pos,
		Button:  btn,
		Buttons: held,
		Mods:    p.mods,
	})
	if held == 0 && w.leavePending {
		// The release ended the implicit grab outside the window: deliver
		// the deferred leave now.
		w.leavePending = false
		if p.x < 0 || p.y < 0 || p.x >= float64(w.osSize.X) || p.y >= float64(w.osSize.Y) {
			b.leave(w, 0, p.mods)
		}
	}
}

// cancelPress queues a PointerCancel at the last known position for the
// press sequence the app is in, and forgets every held button: the back and
// forward buttons tracked in w.extra (no later event would clear them) and
// a deferred leave (the caller reports it).
func (b *Backend) cancelPress(w *Window, mods platform.Modifiers) {
	w.extra = 0
	w.leavePending = false
	b.pointer(w, platform.PointerEvent{
		Action: platform.PointerCancel,
		Pos:    w.lastPos,
		Mods:   mods,
	})
}

// leave queues a PointerLeave at the last known position.
func (b *Backend) leave(w *Window, held platform.Buttons, mods platform.Modifiers) {
	w.inside = false
	b.pointer(w, platform.PointerEvent{
		Action:  platform.PointerLeave,
		Pos:     w.lastPos,
		Buttons: held,
		Mods:    mods,
	})
}

// mapButton maps a core (or XInput2) button number. Buttons 1–3 are left,
// middle and right (the server's pointer mapping already swaps them for
// left-handed setups), 4–7 are wheel steps (up, down, left, right), and
// 8–9 are the back and forward side buttons, as in winit and GTK. Wheel
// steps follow the DOM sign convention: up and left are negative. ok is
// false for other buttons.
func mapButton(detail uint32) (btn platform.PointerButton, steps platform.Point, ok bool) {
	switch detail {
	case 1:
		return platform.ButtonPrimary, steps, true
	case 2:
		return platform.ButtonMiddle, steps, true
	case 3:
		return platform.ButtonSecondary, steps, true
	case 4:
		return platform.ButtonNone, platform.Point{Y: -1}, true
	case 5:
		return platform.ButtonNone, platform.Point{Y: 1}, true
	case 6:
		return platform.ButtonNone, platform.Point{X: -1}, true
	case 7:
		return platform.ButtonNone, platform.Point{X: 1}, true
	case 8:
		return platform.ButtonBack, steps, true
	case 9:
		return platform.ButtonForward, steps, true
	}
	return platform.ButtonNone, steps, false
}

// buttonsFromState returns the held buttons recorded in a core event state
// (Button1Mask … Button3Mask).
func buttonsFromState(state uint16) platform.Buttons {
	var held platform.Buttons
	if state&xproto.KeyButMaskButton1 != 0 {
		held |= platform.ButtonPrimary.Mask()
	}
	if state&xproto.KeyButMaskButton2 != 0 {
		held |= platform.ButtonMiddle.Mask()
	}
	if state&xproto.KeyButMaskButton3 != 0 {
		held |= platform.ButtonSecondary.Mask()
	}
	return held
}
