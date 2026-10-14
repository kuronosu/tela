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
	if pe.Action != platform.PointerMove {
		w.b.enqueue(w, pe)
		return
	}
	w.b.coalesce(w, pe, func(e platform.Event) bool {
		prev, ok := e.(platform.PointerEvent)
		return ok && prev.Action == platform.PointerMove && prev.Buttons == pe.Buttons && prev.Mods == pe.Mods
	})
}

func (w *Window) scale() float64 { return scaleFromDPI(w.dpi) }

func (w *Window) onMouseMove(lparam uintptr) {
	x, y := pointFromLParam(lparam)
	w.mods = w.modifiers()
	inClient := x >= 0 && y >= 0 && x < w.client.X && y < w.client.Y
	if w.ptr.move(logicalPoint(x, y, w.scale()), inClient, w.emitFn) && !w.tracking {
		// Ask for WM_MOUSELEAVE once; Windows disarms it when it sends the
		// message. Only inside: arming it while the pointer is outside (a
		// captured drag) would make Windows send the leave at once.
		ev := trackMouseEvent{size: uint32(unsafe.Sizeof(trackMouseEvent{})), flags: tmeLeave, hwndTrack: w.hwnd}
		procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&ev)))
		w.tracking = true
	}
}

func (w *Window) onMouseLeave() {
	w.tracking = false
	w.mods = w.modifiers()
	w.ptr.leaveMessage(w.emitFn)
}

func (w *Window) onButton(msg uint32, wparam, lparam uintptr) {
	btn, down, ok := buttonFromMessage(msg, wparam)
	if !ok {
		return
	}
	x, y := pointFromLParam(lparam)
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
