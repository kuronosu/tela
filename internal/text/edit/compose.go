package edit

import (
	"github.com/kuronosu/tela/internal/text"
)

// This file holds IME composition (preedit). The preedit is not part of
// the value: it is shown in Display at the offset where the composition
// started, with its own cursor, until the IME commits or cancels it.
// Composition events are not cancelable (as in browsers), so these
// methods apply directly.

// Composition describes an active composition (Editor.Composition).
type Composition struct {
	// Text is the preedit text (shown masked in password mode).
	Text string
	// Start and End are the range of the preedit in Display, to underline.
	Start, End int
	// CursorStart and CursorEnd are the IME's cursor or highlighted range
	// in Display (inside the preedit); Editor.Selection is the same range.
	CursorStart, CursorEnd int
	// CursorHidden reports that the IME asked for no cursor.
	CursorHidden bool
}

// Composing reports whether a composition is active.
func (e *Editor) Composing() bool { return e.composing }

// Composition returns the active composition, if any.
func (e *Editor) Composition() (Composition, bool) {
	if !e.composing {
		return Composition{}, false
	}
	at := e.compAt
	return Composition{
		Text:         e.preedit,
		Start:        e.rawToDisplay(at),
		End:          e.rawToDisplay(at + len(e.preedit)),
		CursorStart:  e.rawToDisplay(at + e.preCursor[0]),
		CursorEnd:    e.rawToDisplay(at + e.preCursor[1]),
		CursorHidden: e.preHidden,
	}, true
}

// SetPreedit shows s as the composition text, with the IME's cursor (or
// highlighted range) at the bytes [cursorStart, cursorEnd) of s; either
// negative hides the cursor (text-input-v3), which then sits at the end
// of the preedit for drawing. The first preedit of a composition deletes
// the selection and starts the composition where it was (browsers replace
// the selection when composing starts). An empty preedit keeps the
// composition active. Inline line breaks become spaces in single-line
// fields; maxlength applies at commit.
//
// The result has InputType insertCompositionText and IsComposing set; the
// value changes only when a selection was deleted.
func (e *Editor) SetPreedit(s string, cursorStart, cursorEnd int) Result {
	if e.opts.ReadOnly {
		return Result{}
	}
	oldValue, oldSel := e.value, e.sel
	if !e.composing {
		vs := e.valueSel()
		a, b := vs.ordered()
		e.compBefore = snapshot{value: e.value, sel: vs}
		e.value = e.value[:a] + e.value[b:]
		e.compAt = a
		e.composing = true
		e.hist.open = false
	}
	e.preedit = sanitizeInput(s, e.opts.Multiline)
	e.preHidden = cursorStart < 0 || cursorEnd < 0
	if e.preHidden {
		cursorStart, cursorEnd = len(e.preedit), len(e.preedit)
	}
	cs := snapGrapheme(e.preedit, clampRuneStart(e.preedit, cursorStart), false)
	ce := snapGrapheme(e.preedit, clampRuneStart(e.preedit, cursorEnd), false)
	e.preCursor = [2]int{cs, ce}
	dc := e.rebuild()
	e.sel = e.compositionSel()
	e.gen++
	r := e.report(oldValue, oldSel, dc)
	r.InputType, r.Data, r.IsComposing = InputInsertCompositionText, e.preedit, true
	return r
}

// compositionSel returns the selection inside the preedit (display
// offsets): the IME's cursor range, attached to the preedit.
func (e *Editor) compositionSel() text.Selection {
	cur := func(off int) text.Cursor {
		a := text.Downstream
		if off > 0 {
			a = text.Upstream
		}
		return text.Cursor{Offset: e.rawToDisplay(e.compAt + off), Affinity: a}
	}
	return text.NewSelection(cur(e.preCursor[0]), cur(e.preCursor[1]))
}

// CommitComposition ends the composition by inserting s where the preedit
// was (insertFromComposition), truncated to MaxLength, with the caret
// after it. The whole composition, including the selection it replaced,
// is one undo step. Without an active composition, s is inserted over the
// selection like typing (insertText; IMEs commit text without a preedit
// for dead keys and direct input).
func (e *Editor) CommitComposition(s string) Result {
	if e.opts.ReadOnly {
		return Result{}
	}
	if !e.composing {
		var p Plan
		p.ed, p.gen = e, e.gen
		e.planInsert(&p, s, InputInsertText, groupTyping)
		return e.Apply(p)
	}
	s = sanitizeInput(s, e.opts.Multiline)
	if m := e.opts.MaxLength; m > 0 {
		s = truncateUTF16(s, m-UTF16Len(e.value))
	}
	oldValue, oldSel := e.value, e.sel
	at := e.compAt
	before := e.compBefore
	e.endComposition()
	e.value = e.value[:at] + s + e.value[at:]
	after := caretSel(snapGrapheme(e.value, at+len(s), true), text.Upstream)
	if s == "" {
		after = caretSel(snapGrapheme(e.value, at, false), text.Downstream)
	}
	e.recordDiff(before.value, e.value, before.sel, after)
	dc := e.rebuild()
	e.sel = e.toDisplaySel(after)
	e.gen++
	r := e.report(oldValue, oldSel, dc)
	r.InputType, r.Data = InputInsertFromComposition, s
	return r
}

// FinishComposition commits the current preedit as it is (Chromium's
// FinishComposingText), for a click or a focus change during a
// composition; the caller tells the IME to reset. It does nothing without
// a composition. Display does not change, so the layout stays valid.
func (e *Editor) FinishComposition() Result {
	if !e.composing {
		return Result{}
	}
	return e.CommitComposition(e.preedit)
}

// CancelComposition removes the preedit and ends the composition
// (deleteCompositionText). A selection deleted when the composition
// started stays deleted, as in Chromium; undo restores it.
func (e *Editor) CancelComposition() Result {
	if !e.composing {
		return Result{}
	}
	oldValue, oldSel := e.value, e.sel
	at := e.compAt
	before := e.compBefore
	e.endComposition()
	after := caretSel(snapGrapheme(e.value, at, false), text.Downstream)
	e.recordDiff(before.value, e.value, before.sel, after)
	dc := e.rebuild()
	e.sel = e.toDisplaySel(after)
	e.gen++
	r := e.report(oldValue, oldSel, dc)
	r.InputType = InputDeleteCompositionText
	return r
}

func (e *Editor) endComposition() {
	e.composing = false
	e.preedit = ""
	e.preCursor = [2]int{}
	e.preHidden = false
	e.compBefore = snapshot{}
	e.hist.open = false
}

// DeleteSurrounding deletes before bytes before the selection and after
// bytes after it, keeping the selection's text (text-input-v3
// delete_surrounding_text; during a composition, around the preedit, which
// stays). Lengths are clamped to the value and widened to whole UTF-8
// characters. Outside a composition the deletion is its own undo step;
// during one it belongs to the composition's step.
//
// InputType is deleteContentBackward, deleteContentForward or, for both
// sides, deleteContent (DeleteSurroundingType tells it beforehand).
func (e *Editor) DeleteSurrounding(before, after int) Result {
	vs, ds, de, it := e.surrounding(before, after)
	if it == InputNone {
		return Result{}
	}
	start, end := vs.ordered()
	oldValue, oldSel := e.value, e.sel
	e.value = oldValue[:ds] + oldValue[start:end] + oldValue[de:]
	if e.composing {
		e.compAt = ds
		dc := e.rebuild()
		e.sel = e.compositionSel()
		e.gen++
		r := e.report(oldValue, oldSel, dc)
		r.InputType, r.IsComposing = it, true
		return r
	}
	shift := func(c text.Cursor) text.Cursor {
		c.Offset = snapGrapheme(e.value, c.Offset-(start-ds), false)
		return c
	}
	nvs := vsel{shift(vs.anchor), shift(vs.focus)}
	e.record(step{at: ds, removed: oldValue[ds:de], inserted: oldValue[start:end], before: vs, after: nvs})
	dc := e.rebuild()
	e.sel = e.toDisplaySel(nvs)
	e.gen++
	r := e.report(oldValue, oldSel, dc)
	r.InputType = it
	return r
}

// DeleteSurroundingType returns the InputType DeleteSurrounding(before,
// after) would report, without deleting anything: InputNone when it would
// change nothing. The caller fires beforeinput with it before the
// deletion.
func (e *Editor) DeleteSurroundingType(before, after int) InputType {
	_, _, _, it := e.surrounding(before, after)
	return it
}

// surrounding returns the selection in value offsets, the range
// [ds, de) DeleteSurrounding(before, after) cuts the selection out of, and
// the deletion's InputType (InputNone for no change).
func (e *Editor) surrounding(before, after int) (vs vsel, ds, de int, it InputType) {
	if e.opts.ReadOnly {
		return vs, 0, 0, InputNone
	}
	vs = e.valueSel()
	start, end := vs.ordered()
	ds = clampRuneStart(e.value, start-max(before, 0))
	de = clampRuneEnd(e.value, end+max(after, 0))
	switch {
	case ds == start && de == end:
		it = InputNone
	case ds < start && de > end:
		it = InputDeleteContent
	case ds < start:
		it = InputDeleteContentBackward
	default:
		it = InputDeleteContentForward
	}
	return vs, ds, de, it
}
