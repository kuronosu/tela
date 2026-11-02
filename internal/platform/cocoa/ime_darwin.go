//go:build darwin

package cocoa

import (
	"unicode/utf16"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The IME through NSTextInputClient: TelaView's protocol methods, the
// session (IMESessionEvent), the routing of key events through
// interpretKeyEvents:, and SetTextInput / ResetTextInput. ime.go holds the
// OS-independent half (what each call makes of the composition). See
// "IME" in doc.go.
//
// Quirks handled, after winit (macOS view.rs) and Chromium
// (render_widget_host_view_cocoa.mm):
//
//   - interpretKeyEvents: calls the protocol back synchronously, and the
//     KeyEvent is only decided when it returns: a key the IME consumed
//     (setMarkedText:, a commit of marked text) is not delivered; one the
//     IME passed on (doCommandBySelector:) is; one that inserted its own
//     text (insertText: without a composition, the US layout) is delivered
//     with that text and no IMEEvent, so plain typing looks the same with
//     and without a text field.
//   - The input method also calls the client outside key events (a
//     candidate clicked with the mouse, the emoji picker, a commit when
//     the window resigns key): those calls run a delivery pass themselves.
//   - Command keys never go to the input method (shortcuts); Control keys
//     do, since IMEs use some during a composition (Kotoeri's Control+K).
//   - unmarkText keeps the marked text (NSTextView commits it; winit
//     cancels it, Chromium commits it too).
//   - Ending a session or ResetTextInput during a composition forgets the
//     preedit first and then tells the input context to discardMarkedText,
//     ignoring what the IME calls back meanwhile, so the app keeps its
//     preedit (the contract) and the IME keeps no composition that would
//     reappear in the next field.
//   - A password field gets no session (no interpretKeyEvents:, so no
//     IME) and turns the secure event input on while its window is key,
//     as NSSecureTextField does.

// imesel holds the selectors of the IME code.
var imesel = struct {
	interpretKeyEvents, inputContext, discardMarkedText, invalidateCoordinates darwin.SEL
	convertRectToView, convertRectToScreen, initWithClient                     darwin.SEL
}{
	interpretKeyEvents:    darwin.Sel("interpretKeyEvents:"),
	inputContext:          darwin.Sel("inputContext"),
	discardMarkedText:     darwin.Sel("discardMarkedText"),
	invalidateCoordinates: darwin.Sel("invalidateCharacterCoordinates"),
	convertRectToView:     darwin.Sel("convertRect:toView:"),
	convertRectToScreen:   darwin.Sel("convertRectToScreen:"),
	initWithClient:        darwin.Sel("initWithClient:"),
}

// imeTrace, when set (by tests), receives a line for every protocol call
// and key decision.
var imeTrace func(format string, args ...any)

func tracef(format string, args ...any) {
	if imeTrace != nil {
		imeTrace(format, args...)
	}
}

// imeWindow is the IME state of a window, on the main thread.
type imeWindow struct {
	state      *platform.TextInputState // the app's latest state (a clone); nil: no field
	active     bool                     // the session the app was told of
	secure     bool                     // this window turned the secure event input on
	input      imeInput                 // the composition and the document (ime.go)
	discarding bool                     // inside discardMarkedText: callbacks change nothing
	invalidate bool                     // invalidateCharacterCoordinates at the end of the pass
	ctx        darwin.ID                // own NSTextInputContext (owned), see textInputContext
}

// SetTextInput records the focused field's state and starts or ends the
// IME session; the input method is told that the caret moved
// (invalidateCharacterCoordinates) at most once per delivery pass. See
// platform.Window.
func (w *Window) SetTextInput(state *platform.TextInputState) {
	platform.CheckTextInputState(state)
	im := &w.ime
	if w.Closed() || im.state.Equal(state) {
		return
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	old := im.state
	im.state = state.Clone()
	if state == nil {
		// Another field may follow: the session restarts for it.
		w.endIMESession()
	} else {
		im.input.setDoc(state.Surrounding)
		if old == nil || old.CursorArea != state.CursorArea {
			im.invalidate = true
		}
	}
	w.syncIME()
}

// ResetTextInput forgets the composition and makes the input method
// discard its own: the app finished or removed it. See platform.Window.
func (w *Window) ResetTextInput() {
	im := &w.ime
	if w.Closed() || im.state == nil {
		return
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	if im.active && im.input.composing() {
		im.input.clear()
		w.discardMarked()
	}
	im.invalidate = true
	if !w.b.pumping {
		w.flushIME()
	}
}

// syncIME brings the session and the secure event input in line with the
// focus and the app's state.
func (w *Window) syncIME() {
	im := &w.ime
	field := !w.Closed() && w.focused && im.state != nil
	if !field || im.state.Purpose == platform.TextPurposePassword {
		w.endIMESession()
	} else if !im.active {
		im.active = true
		im.input.clear()
		im.input.setDoc(im.state.Surrounding)
		im.invalidate = true
		w.b.enqueue(w, platform.IMESessionEvent{Active: true})
	}
	if secure := field && im.state.Purpose == platform.TextPurposePassword; secure != im.secure {
		if darwin.SetSecureEventInput(secure) {
			im.secure = secure
		}
	}
	if !w.b.pumping {
		w.flushIME()
	}
}

// endIMESession ends the session the app was told of; a composition in
// progress is the app's now, and the input method discards its own.
func (w *Window) endIMESession() {
	im := &w.ime
	if !im.active {
		return
	}
	im.active = false
	composing := im.input.composing()
	im.input.clear()
	if !w.Closed() {
		w.b.enqueue(w, platform.IMESessionEvent{Active: false})
	}
	if composing {
		w.discardMarked()
	}
}

// closeIME releases the IME state of a closing window (its events are
// dropped).
func (w *Window) closeIME() {
	im := &w.ime
	if im.active && im.input.composing() {
		im.input.clear()
		w.discardMarked()
	}
	im.active = false
	if im.secure {
		darwin.SetSecureEventInput(false)
		im.secure = false
	}
	if im.ctx != 0 {
		w.b.releaseLater(im.ctx)
		im.ctx = 0
	}
}

// flushIME tells the input method that the caret area changed, once per
// pass.
func (w *Window) flushIME() {
	im := &w.ime
	if !im.invalidate {
		return
	}
	im.invalidate = false
	if !im.active || w.Closed() {
		return
	}
	if ctx := w.inputContext(); ctx != 0 {
		darwin.Send(ctx, imesel.invalidateCoordinates)
	}
}

// discardMarked makes the input context drop its composition; what the
// IME calls back meanwhile changes nothing.
func (w *Window) discardMarked() {
	ctx := w.inputContext()
	if ctx == 0 {
		return
	}
	tracef("discardMarkedText")
	w.ime.discarding = true
	defer func() { w.ime.discarding = false }()
	darwin.Send(ctx, imesel.discardMarkedText)
}

// inputContext returns the view's NSTextInputContext (borrowed).
func (w *Window) inputContext() darwin.ID { return darwin.Send(w.view, imesel.inputContext) }

// imeKeys reports whether a key down with modifierFlags goes through the
// input method: during a session, unless Command is held.
func (w *Window) imeKeys(flags uint64) bool {
	return w.focused && w.ime.active && flags&darwin.NSEventModifierFlagCommand == 0
}

// interpretKey hands a key down to the input method (interpretKeyEvents:,
// which calls the protocol methods back) and then delivers what came of
// it: IMEEvents, and the KeyEvent e unless the IME consumed the key (see
// imeInput.endKey).
func (w *Window) interpretKey(ev darwin.ID, e platform.KeyEvent) {
	im := &w.ime
	im.input.beginKey(e.Text)
	tracef("key %v %q: interpretKeyEvents:", e.Code, e.Text)
	darwin.Send(w.view, imesel.interpretKeyEvents, darwin.NSArray(ev))
	deliver, evs := im.input.endKey()
	if im.active {
		for _, x := range evs {
			w.b.enqueue(w, x)
		}
	}
	tracef("key %v: delivered %v, %d held commits", e.Code, deliver, len(evs))
	if deliver {
		w.deliverKey(e)
	}
}

// rangeFromNS converts an NSRange from AppKit (NSNotFound and absurd
// values become noRange).
func rangeFromNS(r darwin.NSRange) range16 {
	if r.Location >= darwin.NSNotFound || r.Location > 1<<31 {
		return noRange
	}
	return range16{int(r.Location), int(min(r.Length, 1<<31))}
}

// nsRange converts a range16 to an NSRange.
func nsRange(r range16) darwin.NSRange {
	if !r.valid() {
		return darwin.NSRange{Location: darwin.NSNotFound}
	}
	return darwin.NSRange{Location: uint(r.loc), Length: uint(r.n)}
}

// imeString returns the text of an NSString or NSAttributedString argument.
// Unpaired surrogates become U+FFFD, one code unit each, so the UTF-16
// offsets the IME sends with it stay valid.
func imeString(s darwin.ID) string { return string(utf16.Decode(darwin.StringUTF16(s))) }

// imeEdit runs f, a protocol call that changes the composition, for the
// view self when its window has an active session (and the backend is not
// discarding the composition itself), and queues the events f returns.
// Calls from interpretKeyEvents: leave the delivery to keyDown:'s pass;
// calls of their own (a candidate clicked, the emoji picker) run one.
func imeEdit(self darwin.ID, f func(w *Window) []platform.IMEEvent) {
	w := windowFor(self)
	if w == nil || w.Closed() {
		return
	}
	w.b.guard(func() {
		im := &w.ime
		if !im.active || im.discarding {
			tracef("  ignored (session active %v, discarding %v)", im.active, im.discarding)
			return
		}
		for _, e := range f(w) {
			tracef("  -> IMEEvent %+v", e)
			w.b.enqueue(w, e)
		}
		if im.input.key == nil {
			w.b.pump()
		}
	})
}

// imeQuery runs f, a protocol call that only reads, for the window of the
// view self. It pushes no autorelease pool, since some answers are
// autoreleased objects AppKit must get alive; a panic stops the loop.
func imeQuery(self darwin.ID, f func(w *Window)) {
	w := windowFor(self)
	if w == nil || w.Closed() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			w.b.fail(r)
		}
	}()
	f(w)
}

// imeMethods are TelaView's NSTextInputClient methods.
func imeMethods() []darwin.Method {
	return []darwin.Method{
		{Sel: darwin.Sel("insertText:replacementRange:"), Fn: func(self darwin.ID, _ darwin.SEL, str darwin.ID, repl darwin.NSRange) {
			text, r := imeString(str), rangeFromNS(repl)
			tracef("insertText:%q replacementRange:%v", text, r)
			imeEdit(self, func(w *Window) []platform.IMEEvent { return w.ime.input.insert(text, r) })
		}},
		// NSResponder's insertText:, which the key bindings may still call.
		{Sel: darwin.Sel("insertText:"), Fn: func(self darwin.ID, _ darwin.SEL, str darwin.ID) {
			text := imeString(str)
			tracef("insertText:%q", text)
			imeEdit(self, func(w *Window) []platform.IMEEvent { return w.ime.input.insert(text, noRange) })
		}},
		{Sel: darwin.Sel("setMarkedText:selectedRange:replacementRange:"), Fn: func(self darwin.ID, _ darwin.SEL, str darwin.ID, sel, repl darwin.NSRange) {
			text, s, r := imeString(str), rangeFromNS(sel), rangeFromNS(repl)
			tracef("setMarkedText:%q selectedRange:%v replacementRange:%v", text, s, r)
			imeEdit(self, func(w *Window) []platform.IMEEvent { return w.ime.input.setMarked(text, s, r) })
		}},
		{Sel: darwin.Sel("unmarkText"), Fn: func(self darwin.ID, _ darwin.SEL) {
			tracef("unmarkText")
			imeEdit(self, func(w *Window) []platform.IMEEvent { return w.ime.input.unmark() })
		}},
		// Implemented so that NSResponder's version, which walks up the
		// responder chain and beeps, never runs.
		{Sel: darwin.Sel("doCommandBySelector:"), Fn: func(self darwin.ID, _ darwin.SEL, cmd darwin.SEL) {
			tracef("doCommandBySelector:%s", darwin.SelName(cmd))
			imeEdit(self, func(w *Window) []platform.IMEEvent { w.ime.input.command(); return nil })
		}},
		{Sel: darwin.Sel("hasMarkedText"), Fn: func(self darwin.ID, _ darwin.SEL) bool {
			has := false
			imeQuery(self, func(w *Window) { has = w.ime.active && w.ime.input.composing() })
			return has
		}},
		{Sel: darwin.Sel("markedRange"), Fn: func(self darwin.ID, _ darwin.SEL) darwin.NSRange {
			r := noRange
			imeQuery(self, func(w *Window) {
				if w.ime.active {
					r = w.ime.input.markedRange()
				}
			})
			return nsRange(r)
		}},
		{Sel: darwin.Sel("selectedRange"), Fn: func(self darwin.ID, _ darwin.SEL) darwin.NSRange {
			r := noRange
			imeQuery(self, func(w *Window) {
				if w.ime.active {
					r = w.ime.input.selectedRange()
				}
			})
			return nsRange(r)
		}},
		{Sel: darwin.Sel("attributedSubstringForProposedRange:actualRange:"), Fn: func(self darwin.ID, _ darwin.SEL, rng darwin.NSRange, actual *darwin.NSRange) darwin.ID {
			var res darwin.ID
			imeQuery(self, func(w *Window) {
				if !w.ime.active {
					return
				}
				s, got, ok := w.ime.input.substring(rangeFromNS(rng))
				if !ok {
					return
				}
				if actual != nil {
					*actual = nsRange(got)
				}
				res = darwin.NSAttributedStringAutoreleased(s)
			})
			return res
		}},
		{Sel: darwin.Sel("validAttributesForMarkedText"), Fn: func(darwin.ID, darwin.SEL) darwin.ID {
			return darwin.NSArray()
		}},
		{Sel: darwin.Sel("firstRectForCharacterRange:actualRange:"), Fn: func(self darwin.ID, _ darwin.SEL, rng darwin.NSRange, actual *darwin.NSRange) darwin.CGRect {
			var res darwin.CGRect
			imeQuery(self, func(w *Window) {
				if actual != nil {
					*actual = rng
				}
				res = w.imeFirstRect()
			})
			return res
		}},
		{Sel: darwin.Sel("characterIndexForPoint:"), Fn: func(darwin.ID, darwin.SEL, darwin.CGPoint) uint {
			return darwin.NSNotFound
		}},
	}
}

// inputContextMethod is TelaView's inputContext when the runtime does not
// know the NSTextInputClient protocol (NSView makes a context only for
// views that conform to it): a context made for the view once, released
// with the window.
func inputContextMethod() darwin.Method {
	return darwin.Method{Sel: imesel.inputContext, Fn: func(self darwin.ID, _ darwin.SEL) darwin.ID {
		w := windowFor(self)
		if w == nil || w.Closed() {
			return 0
		}
		if w.ime.ctx == 0 {
			if cls := darwin.GetClass("NSTextInputContext"); cls != 0 {
				w.ime.ctx = darwin.Send(darwin.SendClass(cls, sel.alloc), imesel.initWithClient, self)
			}
		}
		return w.ime.ctx
	}}
}

// imeFirstRect answers firstRectForCharacterRange:: the field's cursor
// area in screen coordinates (the IME places its candidate window below
// it), or the view's top-left corner when it is unknown. Every range gets
// the cursor area, which covers the preedit or the caret (winit answers
// the same way).
func (w *Window) imeFirstRect() darwin.CGRect {
	var vr darwin.CGRect
	if r, ok := imeCursorRect(w.ime.state); ok {
		vr = darwin.CGRect{
			Origin: darwin.CGPoint{X: float64(r.X), Y: float64(r.Y)},
			Size:   darwin.CGSize{Width: float64(r.W), Height: float64(r.H)},
		}
	}
	inWindow := darwin.SendRect(w.view, imesel.convertRectToView, vr, darwin.ID(0))
	return darwin.SendRect(w.ns, imesel.convertRectToScreen, inWindow)
}
