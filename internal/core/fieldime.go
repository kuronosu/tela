package core

import (
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
	"github.com/kuronosu/tela/internal/text/edit"
)

// Text input, IME, clipboard and caret of text fields (ADR-068, ADR-069).
//
// # IME
//
// TextInput gives the window's text input state (platform.TextInputState)
// for the focused text field: its caret, or the preedit while composing,
// in window coordinates (after the scroll offsets and transforms), the
// purpose of its Type, Multiline for a Textarea and its committed value
// around the caret (none for passwords). A read-only field, or no focused
// field, is nil. The Host sets it after every event and frame, and resets
// the IME when core finished or removed a composition itself (a click in
// the field, a programmatic value).
//
// An IMEEvent is applied to the focused field with edit's mapping:
// DeleteSurrounding, then the commit (CommitComposition during a
// composition, else typing), then the new preedit (SetPreedit), or the
// end of a composition without a commit (CancelComposition). The
// deletion fires beforeinput first (deleteContentBackward,
// deleteContentForward or deleteContent, IsComposing during a
// composition), which does not cancel it (Chromium dispatches it and
// deletes anyway: the IME cannot be told its edit was refused), then
// OnChange. The events
// follow the UI Events order and Chromium's IsComposing: compositionstart
// (with the replaced selection), then per update beforeinput
// (insertCompositionText, not cancelable), compositionupdate and OnChange
// with IsComposing; a commit is a last update and compositionend. An IME
// session that ends (IMESessionEvent{Active: false}), a blur or a click
// in the field finishes the composition keeping its text (Chromium's
// FinishComposingText): compositionend only, since the value already
// held the text.
//
// # Clipboard
//
// Copy and cut write the platform clipboard (Config is given it by the
// Host); a paste reads it asynchronously and inserts the text, as one
// insertFromPaste edit, when it arrives, unless the field lost the focus
// meanwhile.
//
// # Caret
//
// The caret of the focused, editable field blinks while the window has
// the focus and the selection is collapsed: 530 ms on and 530 ms off
// (Chromium's interval on Linux and Windows), restarting visible at every
// edit, movement or focus, and it stops blinking, visible, 10 s after the
// last input, so an idle field needs no timer (ARCHITECTURE section 3,
// principle 5). Each toggle repaints only the field's layer.

// Caret timing.
const (
	caretBlink = 530 * time.Millisecond
	caretIdle  = 10 * time.Second
)

// caretState is the blinking caret of a Tree (UI thread).
type caretState struct {
	f  *field
	on bool
	// next is the next toggle (0: not blinking); since the last input.
	next, since time.Duration
}

// imeState is the text input state of a Tree (UI thread).
type imeState struct {
	// reset: core finished or removed a composition itself; the IME must
	// drop its own.
	reset bool
	// active reports the IME session of the window.
	active bool
}

// clipboard is the clipboard copy, cut and paste use (SetClipboard).
func (t *Tree) SetClipboard(c platform.Clipboard) { t.clip = c }

// writeClipboard puts s on the clipboard.
func (t *Tree) writeClipboard(s string) {
	if t.clip != nil {
		_ = t.clip.WriteText(s) // perf: a failed write loses the copy, as a full clipboard would
	}
}

// fieldPaste reads the clipboard and pastes it into f when it arrives.
func (t *Tree) fieldPaste(f *field) {
	if t.clip == nil || f.readOnly() {
		return
	}
	t.clip.ReadText(func(s string, err error) {
		if err != nil || s == "" || !f.alive() || t.foc.el != f.el || f.ed.Composing() {
			return // the field lost the focus meanwhile: drop it
		}
		t.fieldDo(f, edit.Command{Kind: edit.Paste, Text: s})
	})
}

// ---------------------------------------------------------------------
// Caret.

// caretReset makes the caret of f visible and restarts its blinking
// (an input: an edit, a movement, the focus).
func (t *Tree) caretReset(f *field) {
	now := t.scr.now
	t.caret = caretState{f: f, on: true, next: now + caretBlink, since: now}
	f.markPaint()
}

// caretBlinks reports whether the caret of the focused field blinks: the
// window has the focus, the field is editable and its selection is
// collapsed (a non-empty selection hides the caret, as in Chromium).
func (t *Tree) caretBlinks() bool {
	c := &t.caret
	f := c.f
	if f == nil || c.next == 0 || !f.alive() || t.foc.el != f.el || !t.foc.window || f.readOnly() {
		return false
	}
	if comp, ok := f.ed.Composition(); ok {
		return !comp.CursorHidden && comp.CursorStart == comp.CursorEnd
	}
	return f.ed.Selection().IsCollapsed()
}

// caretDeadline reports the next toggle of the caret (NextDeadline).
func (t *Tree) caretDeadline() (time.Duration, bool) {
	if !t.caretBlinks() {
		return 0, false
	}
	return t.caret.next, true
}

// tickCaret toggles the caret when its deadline is due (TickGestures),
// and stops it, visible, after caretIdle without input.
func (t *Tree) tickCaret(now time.Duration) {
	c := &t.caret
	if !t.caretBlinks() || now < c.next {
		return
	}
	if now-c.since >= caretIdle {
		c.on, c.next = true, 0
	} else {
		c.on = !c.on
		for c.next <= now {
			c.next += caretBlink
		}
	}
	c.f.markPaint()
}

// caretShown reports whether the caret of f is painted now.
func (t *Tree) caretShown(f *field) bool {
	c := &t.caret
	if c.f != f || !c.on || t.foc.el != f.el || !t.foc.window || f.readOnly() {
		return false
	}
	if comp, ok := f.ed.Composition(); ok {
		return !comp.CursorHidden && comp.CursorStart == comp.CursorEnd
	}
	return f.ed.Selection().IsCollapsed()
}

// ---------------------------------------------------------------------
// Focus.

// fieldFocused is called when f gains the focus: Tab selects its whole
// value (Chromium), the caret is revealed and starts blinking. A press on
// the field (mouseFocus) does not reveal the caret it had: the press puts
// the caret under the pointer, hit-tested in the text as it is shown, and
// reveals that one (fieldPress); revealing the old caret first scrolled a
// scrolled Textarea back to its start under the pointer. A label's press
// reveals its control's caret (form.go).
func (t *Tree) fieldFocused(f *field, typ focusType) {
	if typ == focusKeyboard {
		r := f.ed.Do(edit.Command{Kind: edit.SelectAll}, nil)
		t.fieldChanged(f, r)
		f.selStart, f.selEnd, _ = f.ed.SelectionRange()
	}
	if typ != focusMouse {
		f.reveal()
	}
	t.caretReset(f)
}

// fieldBlurred is called when f loses the focus: a composition is
// finished, keeping its text, and the selection is no longer painted.
func (t *Tree) fieldBlurred(f *field) {
	if f.ed.Composing() {
		t.finishComposition(f)
	}
	if t.caret.f == f {
		t.caret = caretState{}
	}
	f.markPaint()
}

// ---------------------------------------------------------------------
// IME events.

// HandleIME applies an IMEEvent to the focused text field (see the
// comment at the top of fieldime.go). Call it on the UI thread.
func (t *Tree) HandleIME(e platform.IMEEvent) {
	f := t.focusedField()
	if f == nil || f.readOnly() {
		return
	}
	if e.DeleteBefore > 0 || e.DeleteAfter > 0 {
		// beforeinput comes first, as for every edit, but cannot cancel
		// the deletion: Chromium ignores its result for IME deletions, and
		// the IME could not be told that its edit was refused.
		composing := f.ed.Composing()
		if it := f.ed.DeleteSurroundingType(e.DeleteBefore, e.DeleteAfter); it != edit.InputNone {
			fire(t, evBeforeInput, f.el, InputEvent{InputType: string(it), IsComposing: composing}, true)
			t.flushDiscrete()
			if !f.alive() || t.focusedField() != f {
				return
			}
			composing = f.ed.Composing()
			r := f.ed.DeleteSurrounding(e.DeleteBefore, e.DeleteAfter)
			t.caretReset(f)
			t.fieldEdit(f, r, composing)
			if !f.alive() {
				return
			}
		}
	}
	if e.Commit != "" {
		if f.ed.Composing() {
			t.composeStep(f, e.Commit, func() edit.Result { return f.ed.CommitComposition(e.Commit) })
			if !f.alive() {
				return
			}
			fire(t, evCompositionEnd, f.el, CompositionEvent{Data: e.Commit}, true)
			t.flushDiscrete()
		} else {
			t.fieldDo(f, edit.Command{Kind: edit.InsertText, Text: e.Commit})
		}
		if !f.alive() || t.focusedField() != f {
			return
		}
	}
	switch {
	case e.Preedit != "":
		if !f.ed.Composing() {
			fire(t, evCompositionStart, f.el, CompositionEvent{Data: f.ed.SelectedText()}, true)
			t.flushDiscrete()
			if !f.alive() {
				return
			}
		}
		cs, ce := e.CursorStart, e.CursorEnd
		t.composeStep(f, e.Preedit, func() edit.Result { return f.ed.SetPreedit(e.Preedit, cs, ce) })
	case f.ed.Composing() && e.Commit == "":
		// The IME canceled the composition.
		t.composeStep(f, "", f.ed.CancelComposition)
		if f.alive() {
			fire(t, evCompositionEnd, f.el, CompositionEvent{}, true)
			t.flushDiscrete()
		}
	}
}

// composeStep runs one update of a composition: beforeinput
// (insertCompositionText, not cancelable), compositionupdate with data,
// the update, and OnChange with IsComposing.
func (t *Tree) composeStep(f *field, data string, update func() edit.Result) {
	fire(t, evBeforeInput, f.el, InputEvent{InputType: string(edit.InputInsertCompositionText), Data: data, IsComposing: true}, true)
	t.flushDiscrete()
	if !f.alive() {
		return
	}
	fire(t, evCompositionUpdate, f.el, CompositionEvent{Data: data}, true)
	t.flushDiscrete()
	if !f.alive() {
		return
	}
	r := update()
	if r.InputType != "" && r.InputType != edit.InputDeleteCompositionText {
		r.InputType = edit.InputInsertCompositionText
	}
	t.caretReset(f)
	t.fieldEdit(f, r, true)
}

// HandleIMESession records the IME session of the window
// (IMESessionEvent): when it ends, a composition in progress is finished,
// keeping its text.
func (t *Tree) HandleIMESession(active bool) {
	t.ime.active = active
	if active {
		return
	}
	if f := t.focusedField(); f != nil && f.ed.Composing() {
		t.finishComposition(f)
	}
}

// finishComposition commits the preedit of f as it is and fires
// compositionend (Chromium's FinishComposingText: the value already held
// the text, so no input event).
func (t *Tree) finishComposition(f *field) {
	comp, ok := f.ed.Composition()
	if !ok {
		return
	}
	r := f.ed.FinishComposition()
	t.fieldEdit(f, r, false)
	if f.alive() {
		fire(t, evCompositionEnd, f.el, CompositionEvent{Data: comp.Text}, true)
		t.afterFocusEvent()
	}
}

// TextInput returns the text input state of the window: the focused text
// field (a comparable key, nil for none) and its state (nil for none or a
// read-only field), and whether the IME must be reset (ResetTextInput)
// since the last call. See the comment at the top of fieldime.go.
func (t *Tree) TextInput() (key any, state *platform.TextInputState, reset bool) {
	reset = t.ime.reset
	t.ime.reset = false
	f := t.focusedField()
	if f == nil || f.readOnly() || outermostDisabled(f.el) != nil {
		return nil, nil, reset
	}
	st := &platform.TextInputState{Multiline: f.multi, Purpose: f.purpose()}
	if r, ok := t.fieldCursorArea(f); ok {
		st.CursorArea = platform.Rect{X: r.X0, Y: r.Y0, W: max(r.Width(), 0), H: max(r.Height(), 0)}
	}
	if f.typ != fieldPassword {
		s, e, back := f.ed.SelectionRange()
		cur, anc := e, s
		if back {
			cur, anc = s, e
		}
		st.Surrounding = &platform.SurroundingText{Text: f.ed.Value(), Cursor: cur, Anchor: anc}
	}
	return f, st, reset
}

// purpose returns the IME purpose of the field's type.
func (f *field) purpose() platform.TextPurpose {
	switch f.typ {
	case fieldPassword:
		return platform.TextPurposePassword
	case fieldEmail:
		return platform.TextPurposeEmail
	case fieldNumber:
		return platform.TextPurposeNumber
	case fieldTel:
		return platform.TextPurposePhone
	case fieldURL:
		return platform.TextPurposeURL
	case fieldSearch:
		return platform.TextPurposeSearch
	}
	return platform.TextPurposeNormal
}

// fieldCursorArea returns the caret of f, or its preedit while composing,
// in window coordinates.
func (t *Tree) fieldCursorArea(f *field) (gfx.Rect, bool) {
	bx := f.bx
	if bx.pcs == nil || !t.inTree(bx) {
		return gfx.Rect{}, false
	}
	lay := f.layout()
	if lay == nil {
		return gfx.Rect{}, false
	}
	var r gfx.Rect
	if comp, ok := f.ed.Composition(); ok && comp.End > comp.Start {
		for i, q := range lay.SelectionRects(text.NewSelection(text.Cursor{Offset: comp.Start}, text.Cursor{Offset: comp.End}), nil) {
			if i == 0 {
				r = q
			} else {
				r = r.Union(q)
			}
		}
	} else {
		c := lay.Caret(f.ed.Selection().Focus)
		r = gfx.Rect{X0: c.X, Y0: c.Top, X1: c.X, Y1: c.Top + c.Height}
	}
	x, y := f.textOrigin()
	o := t.semOrigin(bx).Add(gfx.Point{X: x, Y: y})
	if s := bx.scr; s != nil && f.multi {
		o = o.Sub(s.soff)
	}
	return t.transformUp(bx, r.Translate(o.X, o.Y)), true
}
