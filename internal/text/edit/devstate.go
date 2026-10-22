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
// the selection in value offsets and the undo history. A composition in
// progress is not kept (its preedit is dropped and the selection collapses
// where it was).
type DevState struct {
	Value         string
	Anchor, Focus text.Cursor
	Undo, Redo    []DevStep
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
	vs := e.valueSel()
	return DevState{
		Value:  e.value,
		Anchor: vs.anchor,
		Focus:  vs.focus,
		Undo:   devSteps(e.hist.undo),
		Redo:   devSteps(e.hist.redo),
	}
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
// (the caller resets the IME), the value is cleaned like SetText's, the
// selection is clamped to it and moved to grapheme boundaries, and the
// history is kept only when it is consistent with the value (every undo
// step, undone from the value, and every redo step, redone from it, finds
// the text it expects), so that a damaged state never breaks Undo. The
// history starts closed: the next edit opens a step of its own. It fires
// nothing and reports what changed, like SetText.
func (e *Editor) RestoreDevState(s DevState) Result {
	if e.composing {
		e.endComposition()
	}
	oldValue, oldSel := e.value, e.sel
	v := sanitizeValue(s.Value, e.opts.Multiline)
	e.value = v
	e.hist.clear()
	if v == s.Value && e.opts.undoLimit() >= 0 {
		undo, ok := devHistory(v, s.Undo, true)
		redo, ok2 := devHistory(v, s.Redo, false)
		if ok && ok2 {
			e.hist.undo, e.hist.redo = undo, redo
			e.hist.trim(e.opts.undoLimit())
		}
	}
	e.hist.open = false
	dc := e.rebuild()
	vs := vsel{devCursor(v, s.Anchor), devCursor(v, s.Focus)}
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
