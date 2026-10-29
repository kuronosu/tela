//go:build windows

package win32

import (
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// The glue between mouse messages and pointerState (pointer.go), which holds
// the rules. Positions arrive in physical client pixels and leave in logical
// ones.

// pointerEvent completes an event emitted by pointerState and queues it.
// Consecutive moves with the same buttons and modifiers coalesce into one.
func (w *Window) pointerEvent(pe platform.PointerEvent) {
	pe.Kind = platform.PointerMouse
	pe.Mods = w.mods
	pe.Time = w.b.Now()
	w.b.queuePointer(w, pe)
}

func (w *Window) scale() float64 { return scaleFromDPI(w.dpi) }

func (w *Window) onMouseMove(lparam uintptr) {
	x, y := pointFromLParam(lparam)
	w.mods = w.modifiers()
	if w.ptr.move(logicalPoint(x, y, w.scale()), w.inClient(x, y), w.emitFn) {
		// Only inside: arming the leave while the pointer is outside (a
		// captured drag) would make Windows send it at once.
		w.track(false)
	}
}

// track asks for the leave of the client area (WM_MOUSELEAVE) or of the
// non-client area (WM_NCMOUSELEAVE, over the app's title bar regions), once:
// Windows disarms it when it sends the message. A window tracks one of the
// two at a time; arming one replaces the other.
func (w *Window) track(nonClient bool) {
	if (nonClient && w.ncTracking) || (!nonClient && w.tracking) {
		return
	}
	ev := trackMouseEvent{size: uint32(unsafe.Sizeof(trackMouseEvent{})), flags: tmeLeave, hwndTrack: w.hwnd}
	if nonClient {
		ev.flags |= tmeNonClient
	}
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&ev)))
	w.tracking, w.ncTracking = !nonClient, nonClient
}

// onMouseLeave handles WM_MOUSELEAVE and WM_NCMOUSELEAVE. With hit regions
// the pointer may only have crossed between the client area and the app's
// title bar, which is no leave for the app (see pointerOverApp).
func (w *Window) onMouseLeave(nonClient bool) {
	if nonClient {
		w.ncTracking = false
	} else {
		w.tracking = false
	}
	if len(w.regions) > 0 && w.pointerOverApp() {
		return
	}
	w.mods = w.modifiers()
	w.ptr.leaveMessage(w.emitFn)
}

func (w *Window) onButton(msg uint32, wparam, lparam uintptr) {
	btn, down, ok := buttonFromMessage(msg, wparam)
	if !ok {
		return
	}
	x, y := pointFromLParam(lparam)
	w.button(btn, down, x, y)
}

// button handles a press or release at client position x, y (physical
// pixels), from a client mouse message or a non-client one over the app's
// window buttons.
func (w *Window) button(btn platform.PointerButton, down bool, x, y int) {
	w.mods = w.modifiers()
	switch w.ptr.button(btn, down, logicalPoint(x, y, w.scale()), w.emitFn) {
	case captureSet:
		// Keep receiving the pointer while a button is held, also outside
		// the window.
		procSetCapture.Call(w.hwnd)
	case captureRelease:
		w.releaseCapture()
	}
}

// onCaptureLost handles another window taking the mouse capture (an
// Alt+Tab, a dialog): the press sequence in progress is aborted. Releasing
// the capture ourselves ends up here too, with no buttons held: nothing to
// abort.
func (w *Window) onCaptureLost() {
	w.mods = w.modifiers()
	w.ptr.captureLost(w.emitFn)
}

// releaseCapture gives the mouse capture back if the window has it.
func (w *Window) releaseCapture() {
	if c, _, _ := procGetCapture.Call(); c == w.hwnd {
		procReleaseCapture.Call()
	}
}

// onWheel handles WM_MOUSEWHEEL and WM_MOUSEHWHEEL, whose position is in
// screen coordinates (unlike the other mouse messages).
func (w *Window) onWheel(msg uint32, wparam, lparam uintptr) {
	steps, ok := wheelSteps(msg, wparam)
	if !ok {
		return
	}
	sx, sy := pointFromLParam(lparam)
	x, y := screenToClient(w.hwnd, sx, sy)
	w.b.enqueue(w, platform.ScrollEvent{
		Kind:  platform.PointerMouse,
		Pos:   logicalPoint(x, y, w.scale()),
		Steps: steps,
		Mods:  w.modifiers(),
		Time:  w.b.Now(),
	})
}
