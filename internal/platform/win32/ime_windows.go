//go:build windows

package win32

import (
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// The IME through IMM32: the session (which input context the window has),
// the WM_IME_* messages, and the placement of the IME's windows. See "IME"
// in doc.go and ime.go for the OS-independent half.

// IME window messages and the IMM32 values only the Windows half uses
// (imm.h, winuser.h).
const (
	wmIMEStartComposition = 0x010D
	wmIMEEndComposition   = 0x010E
	wmIMEComposition      = 0x010F
	wmIMESetContext       = 0x0281
	wmIMEChar             = 0x0286
	wmIMERequest          = 0x0288

	// WM_IME_SETCONTEXT lParam: the IME draws its own composition window.
	iscShowUICompositionWindow = 0x80000000

	// ImmNotifyIME.
	niCompositionStr = 0x0015
	cpsComplete      = 0x0001
	cpsCancel        = 0x0004

	// ImmAssociateContextEx.
	iaceDefault = 0x0010

	// WM_IME_REQUEST wParam.
	imrReconvertString   = 0x0004
	imrQueryCharPosition = 0x0006
	imrDocumentFeed      = 0x0007
)

// isIMEMessage reports whether msg is one of the WM_IME_* messages (for
// keyTrace).
func isIMEMessage(msg uint32) bool {
	return msg >= wmIMEStartComposition && msg <= wmIMEComposition || msg >= wmIMESetContext && msg <= 0x0291
}

// imeWindow is the IME state of a window. All of it lives on the UI thread.
type imeWindow struct {
	state   *platform.TextInputState // the app's latest state (a clone); nil: no field
	restart bool                     // SetTextInput(nil) since the last sync: a new field follows
	reset   bool                     // ResetTextInput since the last sync
	dirty   bool                     // something changed that the next sync applies

	attached bool        // the default input context is associated with the window
	active   bool        // the session the app was told of (IMESessionEvent)
	comp     imeComposer // the composition the app was told of
	caret    bool        // a system caret exists for the composition

	placed     bool            // the IME's windows were placed for placedRect and placedLang
	placedRect image.Rectangle // physical client pixels
	placedLang uint16

	syncing, again bool // syncIME is running; it must run once more
}

// Structures of IMM32.
type (
	compositionForm struct {
		style uint32
		pos   point
		area  rect
	}
	candidateForm struct {
		index uint32
		style uint32
		pos   point
		area  rect
	}
	imeCharPosition struct {
		size       uint32
		charPos    uint32
		pt         point
		lineHeight uint32
		document   rect
	}
)

// SetTextInput records the focused field's state; the end of the current
// delivery pass applies it (syncIME), so the OS gets at most one update per
// pass. See platform.Window.
func (w *Window) SetTextInput(state *platform.TextInputState) {
	platform.CheckTextInputState(state)
	im := &w.ime
	if w.Closed() || im.state.Equal(state) {
		return
	}
	if state == nil {
		// A later state is another field: restart the session even if
		// both calls fall in one pass.
		im.restart = true
	}
	im.state = state.Clone()
	w.imeChanged()
}

// ResetTextInput makes the next sync cancel the IME's composition: the app
// finished or removed it itself. See platform.Window.
func (w *Window) ResetTextInput() {
	if w.Closed() || w.ime.state == nil {
		return
	}
	w.ime.reset = true
	w.imeChanged()
}

// imeChanged schedules a sync: at the end of the delivery pass that is
// running, or at once outside one (before Run).
func (w *Window) imeChanged() {
	w.ime.dirty = true
	if !w.b.pumping {
		w.syncIME()
	}
}

// syncIME brings the input context and the session in line with the focus
// and the app's state. It may run IMM32 calls that send WM_IME_* messages
// back to the window, which queue events and can change the state again:
// it then runs once more.
func (w *Window) syncIME() {
	im := &w.ime
	if im.syncing {
		im.again = true
		return
	}
	im.syncing = true
	defer func() { im.syncing = false }()
	for {
		im.again, im.dirty = false, false
		w.syncIMEOnce()
		if !im.again {
			return
		}
	}
}

func (w *Window) syncIMEOnce() {
	im := &w.ime
	attach := !w.Closed() && im.state != nil && im.state.Purpose != platform.TextPurposePassword
	if im.restart || !attach || !w.focused {
		w.endIMESession()
	}
	im.restart = false
	if attach != im.attached && !w.Closed() {
		// No context, no IME: a password is typed on the keyboard layout
		// and the IME neither sees nor learns it (winit and Chromium turn
		// the IME off the same way).
		im.attached = attach
		flags := uintptr(0)
		if attach {
			flags = iaceDefault
		}
		procImmAssociateContextEx.Call(w.hwnd, 0, flags)
		im.placed = false
	}
	if im.reset {
		im.reset = false
		if im.active && im.comp.open() {
			// Cleared first: the messages the cancel sends change nothing.
			im.comp.clear()
			w.notifyIME(cpsCancel)
		}
	}
	if attach && w.focused && !im.active {
		im.active = true
		im.comp.clear()
		w.b.enqueue(w, platform.IMESessionEvent{Active: true})
	}
	if attach {
		w.placeIMEWindows(false)
	}
}

// endIMESession ends the session the app was told of. A composition in
// progress is completed in the IME (Chromium's CleanupComposition) after
// the session is over, so the result it sends is not delivered: the app
// keeps its preedit as text when a session ends (the platform contract),
// and the input context, which the thread's windows share, is left without
// a composition that would come back in the next field or at the next
// focus.
func (w *Window) endIMESession() {
	im := &w.ime
	if !im.active {
		return
	}
	im.active = false
	open := im.comp.open()
	im.comp.clear()
	w.destroyIMECaret()
	if !w.Closed() {
		w.b.enqueue(w, platform.IMESessionEvent{Active: false})
	}
	if open {
		w.notifyIME(cpsComplete)
	}
}

// closeIME forgets the IME state of a closing window. Its events are
// dropped, so the session ends without one.
func (w *Window) closeIME() {
	im := &w.ime
	if im.active && im.comp.open() {
		im.comp.clear()
		w.notifyIME(cpsCancel)
	}
	im.active = false
	w.destroyIMECaret()
}

// notifyIME sends NI_COMPOSITIONSTR with action (CPS_COMPLETE, CPS_CANCEL)
// to the window's input context.
func (w *Window) notifyIME(action uintptr) {
	himc, _, _ := procImmGetContext.Call(w.hwnd)
	if himc == 0 {
		return
	}
	procImmNotifyIME.Call(himc, niCompositionStr, action, 0)
	procImmReleaseContext.Call(w.hwnd, himc)
}

// imeLang is the primary language of the thread's input locale.
func imeLang() uint16 { return layoutLang(currentLayout()) }

// placeIMEWindows tells the IME where the caret is: the composition and
// candidate windows (placeIME) and the system caret, in physical client
// pixels. It does nothing for an unknown (zero) cursor area, and nothing
// when the caret and the language are those of the last call, unless
// force (a new composition: IMEs may reset the forms).
func (w *Window) placeIMEWindows(force bool) {
	im := &w.ime
	if im.state == nil || im.state.CursorArea == (platform.Rect{}) || !im.attached {
		return
	}
	r := imeCaretRect(im.state.CursorArea, scaleFromDPI(w.dpi))
	lang := imeLang()
	if !force && im.placed && r == im.placedRect && lang == im.placedLang {
		return
	}
	himc, _, _ := procImmGetContext.Call(w.hwnd)
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(w.hwnd, himc)
	im.placed, im.placedRect, im.placedLang = true, r, lang
	p := placeIME(r, lang)
	if keyTrace != nil {
		keyTrace("place IME windows at %v (lang %#x, force %v)", r, lang, force)
	}
	cf := compositionForm{style: cfsPoint, pos: point{int32(p.comp.X), int32(p.comp.Y)}, area: rectOf(r)}
	procImmSetCompositionWindow.Call(himc, uintptr(unsafe.Pointer(&cf)))
	for _, c := range p.cands {
		f := candidateForm{style: c.style, pos: point{int32(c.pos.X), int32(c.pos.Y)}, area: rectOf(c.area)}
		procImmSetCandidateWindow.Call(himc, uintptr(unsafe.Pointer(&f)))
	}
	if im.caret {
		procSetCaretPos.Call(uintptr(p.caret.X), uintptr(p.caret.Y))
	}
}

func rectOf(r image.Rectangle) rect {
	return rect{int32(r.Min.X), int32(r.Min.Y), int32(r.Max.X), int32(r.Max.Y)}
}

// createIMECaret creates the (hidden) system caret that some IMEs read
// instead of the candidate form, for the languages whose IMEs do so.
func (w *Window) createIMECaret() {
	if w.ime.caret || !needsSystemCaret(imeLang()) {
		return
	}
	if r, _, _ := procCreateCaret.Call(w.hwnd, 0, 1, 1); r != 0 {
		w.ime.caret = true
	}
}

func (w *Window) destroyIMECaret() {
	if w.ime.caret {
		w.ime.caret = false
		procDestroyCaret.Call()
	}
}

// imeMessage handles the WM_IME_* messages. handled false sends the message
// to DefWindowProc.
func (w *Window) imeMessage(msg uint32, wparam, lparam uintptr) (ret uintptr, handled bool) {
	im := &w.ime
	switch msg {
	case wmIMESetContext:
		// Tela draws the preedit inline: the IME must not show its own
		// composition window. Its candidate windows stay.
		if wparam != 0 {
			lparam &^= iscShowUICompositionWindow
		}
		return defWindowProc(w.hwnd, uintptr(msg), wparam, lparam), true

	case wmIMEStartComposition:
		// Not for DefWindowProc, which would open the system composition
		// window.
		if im.active {
			im.comp.start()
			w.createIMECaret()
			w.placeIMEWindows(true)
		}
		return 0, true

	case wmIMEComposition:
		// Never for DefWindowProc either: it would draw the composition
		// and turn the result into WM_IME_CHAR and WM_CHAR messages, which
		// would type it a second time.
		if im.active {
			w.imeComposition(uint32(lparam))
		}
		return 0, true

	case wmIMEEndComposition:
		if im.active {
			if ev, ok := im.comp.end(); ok {
				w.b.enqueue(w, ev)
			}
		}
		w.destroyIMECaret()
		return 0, false // DefWindowProc lets the IME release its window

	case wmIMEChar:
		// The characters of a result come with WM_IME_COMPOSITION, which
		// never reaches DefWindowProc; an IME that sends them anyway would
		// type them twice (Firefox eats them too).
		return 0, true

	case wmIMERequest:
		return w.imeRequest(wparam, lparam)
	}
	return 0, false
}

// imeComposition reads what a WM_IME_COMPOSITION says changed and queues
// the IMEEvent it makes.
func (w *Window) imeComposition(flags uint32) {
	himc, _, _ := procImmGetContext.Call(w.hwnd)
	if himc == 0 {
		return
	}
	u := compUpdate{flags: flags, cursor: -1}
	if flags&gcsResultStr != 0 {
		u.result = immString(himc, gcsResultStr)
	}
	if compChanged(flags) {
		u.comp = immString(himc, gcsCompStr)
		u.attrs = immBytes(himc, gcsCompAttr)
		r, _, _ := procImmGetCompositionStringW.Call(himc, gcsCursorPos, 0, 0)
		u.cursor = int(int32(r)) // IMM_ERROR_NODATA and IMM_ERROR_GENERAL are negative
	}
	procImmReleaseContext.Call(w.hwnd, himc)
	if ev, ok := w.ime.comp.update(u); ok {
		w.b.enqueue(w, ev)
	}
	// The caret area the app reports trails the preedit by a frame; this
	// keeps the candidate window where the IME last saw it.
	w.placeIMEWindows(false)
}

// immBytes returns one of the composition values (ImmGetCompositionStringW
// answers in bytes; negative values are errors).
func immBytes(himc uintptr, index uintptr) []byte {
	n, _, _ := procImmGetCompositionStringW.Call(himc, index, 0, 0)
	if int32(n) <= 0 {
		return nil
	}
	buf := make([]byte, int32(n))
	n, _, _ = procImmGetCompositionStringW.Call(himc, index, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if int32(n) <= 0 {
		return nil
	}
	return buf[:min(int(int32(n)), len(buf))]
}

// immString returns a composition string as UTF-16 code units.
func immString(himc uintptr, index uintptr) []uint16 {
	b := immBytes(himc, index)
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return u
}

// imeRequest answers WM_IME_REQUEST: the caret position (for the candidate
// window of IMEs that ask instead of reading the forms), the text around
// the composition (IMR_DOCUMENTFEED) and a selection to reconvert
// (IMR_RECONVERTSTRING), from the app's state. Requests the state cannot
// answer return 0 (not supported).
func (w *Window) imeRequest(wparam, lparam uintptr) (uintptr, bool) {
	im := &w.ime
	switch wparam {
	case imrQueryCharPosition:
		// Every character is at the caret area: the app reports one
		// rectangle (the caret, or the preedit it belongs to).
		p := (*imeCharPosition)(ptrFromUintptr(lparam))
		if p == nil || p.size < uint32(unsafe.Sizeof(*p)) || !im.active || im.state == nil || im.state.CursorArea == (platform.Rect{}) {
			return 0, true
		}
		r := imeCaretRect(im.state.CursorArea, scaleFromDPI(w.dpi))
		p.pt = clientToScreen(w.hwnd, r.Min)
		p.lineHeight = uint32(r.Dy())
		c := getClientRect(w.hwnd)
		tl := clientToScreen(w.hwnd, image.Pt(int(c.left), int(c.top)))
		br := clientToScreen(w.hwnd, image.Pt(int(c.right), int(c.bottom)))
		p.document = rect{tl.x, tl.y, br.x, br.y}
		return 1, true

	case imrDocumentFeed, imrReconvertString:
		if !im.active || im.state == nil {
			return 0, true
		}
		var data []byte
		if wparam == imrDocumentFeed {
			data = documentFeed(im.state.Surrounding, im.comp.preedit)
		} else {
			data = reconvertSelection(im.state.Surrounding, im.comp.open())
		}
		if data == nil {
			return 0, true
		}
		if lparam == 0 {
			return uintptr(len(data)), true // the size the IME allocates
		}
		dst := ptrFromUintptr(lparam)
		if *(*uint32)(dst) < uint32(len(data)) {
			return 0, true
		}
		copy(unsafe.Slice((*byte)(dst), len(data)), data)
		// The documentation says to return the structure; some
		// applications return its size (Chromium returns the pointer).
		return lparam, true
	}
	return 0, false
}

func clientToScreen(hwnd uintptr, p image.Point) point {
	pt := point{int32(p.X), int32(p.Y)}
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
	return pt
}
