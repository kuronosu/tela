//go:build linux

package x11

import (
	"image"

	"github.com/jezek/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
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
	switch ev := e.ev.(type) {
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
			w.iconic = false
		}

	case xproto.UnmapNotifyEvent:
		// The window manager unmaps the client window when it is
		// iconified (ICCCM 4.1.4). Frames pause until it is mapped again.
		if w := b.windows[ev.Window]; w != nil && ev.Event == ev.Window && w.mapped {
			w.iconic = true
		}

	case xproto.ButtonPressEvent:
		if w := b.windows[ev.Event]; w != nil {
			b.button(w, ev.Detail, ev.State, ev.EventX, ev.EventY, true)
		}
	case xproto.ButtonReleaseEvent:
		if w := b.windows[ev.Event]; w != nil {
			b.button(w, ev.Detail, ev.State, ev.EventX, ev.EventY, false)
		}

	case xproto.MotionNotifyEvent:
		if w := b.windows[ev.Event]; w != nil {
			pos := w.pos(ev.EventX, ev.EventY)
			w.lastPos = pos
			b.pointer(w, platform.PointerEvent{
				Action:  platform.PointerMove,
				Pos:     pos,
				Buttons: buttonsFromState(ev.State) | w.extra,
				Mods:    modsFromState(ev.State),
			})
		}

	case xproto.EnterNotifyEvent:
		w := b.windows[ev.Event]
		if w == nil || ev.Detail == xproto.NotifyDetailInferior {
			return
		}
		mods := modsFromState(ev.State)
		if w.leavePending {
			held := buttonsFromState(ev.State) | w.extra
			if ev.Mode == xproto.NotifyModeNormal && held != 0 {
				// Back inside before the buttons were released: the app
				// never saw the pointer leave.
				w.leavePending = false
				return
			}
			// The implicit grab ended without a ButtonRelease reaching
			// this window (NotifyUngrab, or no button held any more):
			// another grab took the release. End the old stay for the
			// app (cancel, then the deferred leave) before the enter.
			b.cancelPress(w, mods)
			b.leave(w, 0, mods)
		}
		if !w.inside {
			w.inside = true
			w.lastPos = w.pos(ev.EventX, ev.EventY)
			b.pointer(w, platform.PointerEvent{
				Action:  platform.PointerEnter,
				Pos:     w.lastPos,
				Buttons: buttonsFromState(ev.State) | w.extra,
				Mods:    modsFromState(ev.State),
			})
		}
	case xproto.LeaveNotifyEvent:
		// Crossing modes are not filtered: a pointer grab by the window
		// manager (NotifyGrab) also takes the pointer away from the app,
		// and the matching NotifyUngrab Enter brings it back. The inside
		// flag drops duplicates.
		w := b.windows[ev.Event]
		if w == nil || ev.Detail == xproto.NotifyDetailInferior || !w.inside {
			return
		}
		held := buttonsFromState(ev.State) | w.extra
		mods := modsFromState(ev.State)
		if held != 0 && ev.Mode == xproto.NotifyModeNormal {
			// X reports the crossing during the implicit grab of a held
			// button, while the window keeps receiving the moves. Wayland
			// (no wl_pointer.leave until the grab ends) and Win32
			// (SetCapture) report the leave after the release, so the
			// backend defers it to the release that ends the grab.
			w.leavePending = true
			return
		}
		w.lastPos = w.pos(ev.EventX, ev.EventY)
		if held != 0 || w.leavePending {
			// A grab (NotifyGrab: the window manager's Alt+drag, a
			// screenshot tool; NotifyUngrab: our implicit grab ended
			// without a release) took the pointer while the app still
			// had a press: its release will never reach this window.
			// Like the Wayland backend when the compositor takes the
			// pointer, abort the press, then report the leave.
			b.cancelPress(w, mods)
		}
		b.leave(w, 0, mods)

	case xproto.FocusInEvent:
		if w := b.windows[ev.Event]; w != nil && realFocusChange(ev.Mode, ev.Detail) && !w.focused {
			w.focused = true
			b.enqueue(w, platform.FocusEvent{Focused: true})
		}
	case xproto.FocusOutEvent:
		if w := b.windows[ev.Event]; w != nil && realFocusChange(ev.Mode, ev.Detail) && w.focused {
			w.focused = false
			b.enqueue(w, platform.FocusEvent{Focused: false})
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

// pos converts event coordinates (physical pixels) to logical pixels.
func (w *Window) pos(x, y int16) platform.Point {
	s := w.b.scale
	return platform.Point{X: float32(float64(x) / s), Y: float32(float64(y) / s)}
}

// pointer queues a pointer event, coalescing consecutive moves: a move
// that directly follows a queued move of the same window with the same
// buttons and modifiers replaces it.
func (b *Backend) pointer(w *Window, pe platform.PointerEvent) {
	pe.Kind = platform.PointerMouse
	pe.Time = b.Now()
	if pe.Action == platform.PointerMove && len(b.queue) > 0 {
		last := &b.queue[len(b.queue)-1]
		if prev, ok := last.ev.(platform.PointerEvent); ok && last.w == w && prev.Action == platform.PointerMove &&
			prev.Buttons == pe.Buttons && prev.Mods == pe.Mods {
			last.ev = pe
			return
		}
	}
	b.enqueue(w, pe)
}

// button translates a core button press or release.
func (b *Backend) button(w *Window, detail xproto.Button, state uint16, x, y int16, press bool) {
	pos := w.pos(x, y)
	w.lastPos = pos
	mods := modsFromState(state)
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
				Mods:  mods,
				Time:  b.Now(),
			})
		}
		return
	}
	// state is the state before the event, so the pressed or released
	// button is applied on top of it. Back and forward have no bit in the
	// core state mask and are tracked by the window.
	held := buttonsFromState(state) | w.extra
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
		Mods:    mods,
	})
	if held == 0 && w.leavePending {
		// The release ended the implicit grab outside the window: deliver
		// the deferred leave now.
		w.leavePending = false
		if !image.Pt(int(x), int(y)).In(image.Rectangle{Max: w.osSize}) {
			b.leave(w, 0, mods)
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

// mapButton maps a core button number. Buttons 1–3 are left, middle and
// right (the server's pointer mapping already swaps them for left-handed
// setups), 4–7 are wheel steps (up, down, left, right), and 8–9 are the
// back and forward side buttons, as in winit and GTK. Wheel steps follow
// the DOM sign convention: up and left are negative. ok is false for other
// buttons.
func mapButton(detail xproto.Button) (btn platform.PointerButton, steps platform.Point, ok bool) {
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

// modsFromState returns the modifiers of a core event state. Mod1 is Alt
// and Mod4 is Super under every common modifier mapping (xkeyboard-config
// defaults); reading the real mapping (GetModifierMapping, or XKB state as
// winit does) comes with keyboard support in phase 4.
func modsFromState(state uint16) platform.Modifiers {
	var m platform.Modifiers
	if state&xproto.KeyButMaskShift != 0 {
		m |= platform.ModShift
	}
	if state&xproto.KeyButMaskControl != 0 {
		m |= platform.ModCtrl
	}
	if state&xproto.KeyButMaskMod1 != 0 {
		m |= platform.ModAlt
	}
	if state&xproto.KeyButMaskMod4 != 0 {
		m |= platform.ModSuper
	}
	return m
}
