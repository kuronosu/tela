//go:build teladev

package edit

import (
	"github.com/kuronosu/tela/internal/text"
)

// The state of an Editor that `tela dev` keeps across a reload (ADR-113):
// the committed value, the selection and the undo and redo stacks. It
// exists only in builds with the teladev tag, like the rest of the dev
// loop, so release builds do not change.

// DevState is the state of an Editor kept by a reload: the committed value,
// the selection in value offsets and the undo history. During a
// composition they are the state the composition started from (its value
// and the selection the preedit replaces, which the history matches), and
// Composition is the composition itself, which RestoreDevComposition
// starts again (ADR-309); without it the restore is a clean cancel.
type DevState struct {
	Value         string
	Anchor, Focus text.Cursor
	Undo, Redo    []DevStep
	Composition   *DevComposition
}

// DevComposition is a composition in progress kept by a reload.
type DevComposition struct {
	// Value is the committed value during the composition (without the
	// selection it replaced, with what the IME deleted around it) and At
	// the byte offset where the preedit sits in it.
	Value string
	At    int
	// Preedit is the composition text and CursorStart, CursorEnd the
	// IME's cursor in it, in bytes; Hidden reports a hidden cursor.
	Preedit                string
	CursorStart, CursorEnd int
	Hidden                 bool
}

// DevStep is an undo step of a DevState.
type DevStep struct {
	At                int
	Removed, Inserted string
	// Before and After are the selections (anchor, focus) in value offsets
	// before and after the step.
	Before, After [2]text.Cursor
	Kind          uint8
}

// DevState returns the editor's state for a reload.
func (e *Editor) DevState() DevState {
	if e.composing {
		b := e.compBefore
		return DevState{
			Value:  b.value,
			Anchor: b.sel.anchor,
			Focus:  b.sel.focus,
			Undo:   devSteps(e.hist.undo),
			Redo:   devSteps(e.hist.redo),
			Composition: &DevComposition{
				Value: e.value, At: e.compAt, Preedit: e.preedit,
				CursorStart: e.preCursor[0], CursorEnd: e.preCursor[1], Hidden: e.preHidden,
			},
		}
	}
	vs := e.valueSel()
	return DevState{
		Value:  e.value,
		Anchor: vs.anchor,
		Focus:  vs.focus,
		Undo:   devSteps(e.hist.undo),
		Redo:   devSteps(e.hist.redo),
	}
}

// RestoreDevComposition starts the composition c again over the state
// RestoreDevState restored (the one it started from), as the previous
// build had it: the value during the composition, the preedit and the
// IME's cursor, with the composition's undo step to come. The caller gives
// it only to the field that has the focus, whose IME session goes on. It
// does nothing for a read-only editor, during a composition, or when c does
// not fit this editor (a value that cleaning changes, an offset off the
// value); like RestoreDevState it fires nothing and reports what changed.
func (e *Editor) RestoreDevComposition(c DevComposition) Result {
	if e.opts.ReadOnly || e.composing || sanitizeValue(c.Value, e.opts.Multiline) != c.Value ||
		c.At < 0 || c.At > len(c.Value) || clampRuneStart(c.Value, c.At) != c.At {
		return Result{}
	}
	oldValue, oldSel := e.value, e.sel
	e.compBefore = snapshot{value: e.value, sel: e.valueSel()}
	e.value = c.Value
	e.compAt = c.At
	e.composing = true
	e.hist.open = false
	e.preedit = sanitizeInput(c.Preedit, e.opts.Multiline)
	e.preHidden = c.Hidden
	cs, ce := c.CursorStart, c.CursorEnd
	if e.preHidden {
		cs, ce = len(e.preedit), len(e.preedit)
	}
	cs = snapGrapheme(e.preedit, clampRuneStart(e.preedit, cs), false)
	ce = snapGrapheme(e.preedit, clampRuneStart(e.preedit, ce), false)
	e.preCursor = [2]int{cs, ce}
	dc := e.rebuild()
	e.sel = e.compositionSel()
	e.gen++
	return e.report(oldValue, oldSel, dc)
}

func devSteps(steps []step) []DevStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]DevStep, len(steps))
	for i, st := range steps {
		out[i] = DevStep{
			At: st.at, Removed: st.removed, Inserted: st.inserted,
			Before: [2]text.Cursor{st.before.anchor, st.before.focus},
			After:  [2]text.Cursor{st.after.anchor, st.after.focus},
			Kind:   uint8(st.kind),
		}
	}
	return out
}

// RestoreDevState replaces the editor's value, selection and history with
// s, as a reload restores them: an active composition is canceled first
// (the caller resets the IME) and the value is cleaned like SetText's.
// When the cleaned value is the saved one, the selection is clamped to it
// and moved to grapheme boundaries, and the history is kept only when it
// is consistent with the value (every undo step, undone from the value,
// and every redo step, redone from it, finds the text it expects), so that
// a damaged state never breaks Undo. A value that cleaning changed (a
// Textarea's text restored into an Input loses its line breaks) keeps
// neither, since their offsets are the saved value's: the caret goes to
// the end, as SetText puts it. The history starts closed: the next edit
// opens a step of its own. It fires nothing and reports what changed,
// like SetText.
func (e *Editor) RestoreDevState(s DevState) Result {
	if e.composing {
		e.endComposition()
	}
	oldValue, oldSel := e.value, e.sel
	v := sanitizeValue(s.Value, e.opts.Multiline)
	e.value = v
	e.hist.clear()
	same := v == s.Value
	if same && e.opts.undoLimit() >= 0 {
		undo, ok := devHistory(v, s.Undo, true)
		redo, ok2 := devHistory(v, s.Redo, false)
		if ok && ok2 {
			e.hist.undo, e.hist.redo = undo, redo
			e.hist.trim(e.opts.undoLimit())
		}
	}
	e.hist.open = false
	dc := e.rebuild()
	vs := caretSel(len(v), text.Downstream)
	if same {
		vs = vsel{devCursor(v, s.Anchor), devCursor(v, s.Focus)}
	}
	e.sel = e.toDisplaySel(vs)
	e.gen++
	return e.report(oldValue, oldSel, dc)
}

// devCursor clamps c to value v, on a grapheme boundary.
func devCursor(v string, c text.Cursor) text.Cursor {
	return text.Cursor{Offset: snapGrapheme(v, c.Offset, false), Affinity: c.Affinity}
}

// devHistory checks steps against the value v and converts them: undo
// steps are undone from the last one (each must find its inserted text at
// its offset), redo steps redone from the last one (each must find its
// removed text). The selections of a step are clamped to the value it
// restores.
func devHistory(v string, steps []DevStep, undo bool) ([]step, bool) {
	if len(steps) == 0 {
		return nil, true
	}
	out := make([]step, len(steps))
	cur := v
	for i := len(steps) - 1; i >= 0; i-- {
		st := steps[i]
		have, want := st.Inserted, st.Removed // undo: replace inserted with removed
		if !undo {
			have, want = st.Removed, st.Inserted
		}
		if st.At < 0 || st.At > len(cur) || st.At+len(have) > len(cur) || cur[st.At:st.At+len(have)] != have {
			return nil, false
		}
		before := cur
		cur = cur[:st.At] + want + cur[st.At+len(have):]
		// before is the value with the step applied (undo) or not yet applied
		// (redo); cur the other one.
		applied, reverted := before, cur
		if !undo {
			applied, reverted = cur, before
		}
		out[i] = step{
			at: st.At, removed: st.Removed, inserted: st.Inserted,
			before: vsel{devCursor(reverted, st.Before[0]), devCursor(reverted, st.Before[1])},
			after:  vsel{devCursor(applied, st.After[0]), devCursor(applied, st.After[1])},
			kind:   groupKind(min(st.Kind, uint8(groupDelete))),
		}
	}
	return out, true
}
