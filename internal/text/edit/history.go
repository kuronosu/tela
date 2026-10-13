package edit

import (
	"time"
	"unicode"
	"unicode/utf8"
)

// This file holds the undo history. A step replaces the bytes of removed
// at offset at with inserted (undo does the reverse) and restores the
// selection it had before (undo) or after (redo).
//
// Coalescing: an edit joins the last step, instead of starting a new one,
// when all of these hold:
//
//   - the group is open: nothing but edits of the same group happened
//     since (a selection change by navigation, a click or Select closes
//     it, and so do undo, redo, paste, cut, word and line deletions,
//     compositions and SetText);
//   - both are typing (insertText, insertLineBreak), both are Backspace
//     (deleteContentBackward) or both are Delete (deleteContentForward);
//   - the new edit continues the last one: typing at the end of the text
//     the step inserted, without replacing a selection; Backspace right
//     before the text the step removed; Delete at the same offset;
//   - for typing, it does not start white space after text that is not
//     white space, so each word is its own step with the space before it
//     ("hello world" undoes " world", then "hello"; VS Code's rule);
//   - with a clock (Options.Now), at most Options.UndoDelay passed since
//     the last edit of the group (ProseMirror's newGroupDelay).
//
// Chromium coalesces a whole typing run, Backspaces included, until the
// selection changes, and has no timer; the kind, word and pause rules make
// undo steps smaller, as in VS Code and ProseMirror.

// groupKind is the coalescing class of an edit.
type groupKind uint8

const (
	groupNone groupKind = iota // never joins a step
	groupTyping
	groupBackspace
	groupDelete
)

type step struct {
	at                int
	removed, inserted string
	before, after     vsel
	kind              groupKind
	time              time.Duration
}

type history struct {
	undo, redo []step
	open       bool // the last undo step may take more edits
}

func (h *history) clear() {
	clear(h.undo)
	clear(h.redo)
	h.undo, h.redo, h.open = h.undo[:0], h.redo[:0], false
}

// trim drops the oldest steps beyond limit.
func (h *history) trim(limit int) {
	if n := len(h.undo) - limit; n > 0 {
		k := copy(h.undo, h.undo[n:])
		clear(h.undo[k:])
		h.undo = h.undo[:k]
	}
	if len(h.redo) > limit {
		// Redo pops from the end: keep the most recent undos, the end.
		n := len(h.redo) - limit
		k := copy(h.redo, h.redo[n:])
		clear(h.redo[k:])
		h.redo = h.redo[:k]
	}
}

// record adds an edit to the history, joining the last step when the
// coalescing rules allow it, and clears the redo stack.
func (e *Editor) record(st step) {
	limit := e.opts.undoLimit()
	if limit < 0 {
		return
	}
	h := &e.hist
	clear(h.redo)
	h.redo = h.redo[:0]
	st.time = e.now()
	if h.open && st.kind != groupNone && len(h.undo) > 0 {
		if e.join(&h.undo[len(h.undo)-1], &st) {
			return
		}
	}
	h.undo = append(h.undo, st)
	h.trim(limit)
	h.open = st.kind != groupNone
}

// join merges st into last when the coalescing rules allow it.
func (e *Editor) join(last, st *step) bool {
	if last.kind != st.kind {
		return false
	}
	if e.opts.Now != nil {
		if d := e.opts.undoDelay(); d >= 0 && st.time-last.time > d {
			return false
		}
	}
	switch st.kind {
	case groupTyping:
		if st.removed != "" || st.at != last.at+len(last.inserted) {
			return false
		}
		if startsSpace(st.inserted) && last.inserted != "" && !endsSpace(last.inserted) {
			return false
		}
		last.inserted += st.inserted
	case groupBackspace:
		if st.inserted != "" || last.inserted != "" || st.at+len(st.removed) != last.at {
			return false
		}
		last.at = st.at
		last.removed = st.removed + last.removed
	case groupDelete:
		if st.inserted != "" || last.inserted != "" || st.at != last.at {
			return false
		}
		last.removed += st.removed
	default:
		return false
	}
	last.after = st.after
	last.time = st.time
	return true
}

func startsSpace(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return s != "" && unicode.IsSpace(r)
}

func endsSpace(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return s != "" && unicode.IsSpace(r)
}

// recordDiff records the change from value a to value b as one step that
// never coalesces, as the smallest replacement (common prefix and suffix
// removed, on character boundaries).
func (e *Editor) recordDiff(a, b string, before, after vsel) {
	if a == b {
		return
	}
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	p = clampRuneStart(a, p)
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	// Keep the suffix on a character boundary of both strings.
	for s > 0 && (!runeStartAt(a, len(a)-s) || !runeStartAt(b, len(b)-s)) {
		s--
	}
	e.record(step{at: p, removed: a[p : len(a)-s], inserted: b[p : len(b)-s], before: before, after: after})
	e.hist.open = false
}

func runeStartAt(s string, i int) bool { return i >= len(s) || utf8.RuneStart(s[i]) }

// undo reverts the last step and returns the selection to restore.
func (e *Editor) undo() (vsel, bool) {
	h := &e.hist
	if len(h.undo) == 0 {
		return vsel{}, false
	}
	st := h.undo[len(h.undo)-1]
	h.undo[len(h.undo)-1] = step{}
	h.undo = h.undo[:len(h.undo)-1]
	e.value = e.value[:st.at] + st.removed + e.value[st.at+len(st.inserted):]
	h.redo = append(h.redo, st)
	h.open = false
	return st.before, true
}

// redo applies the last undone step again and returns the selection to
// restore.
func (e *Editor) redo() (vsel, bool) {
	h := &e.hist
	if len(h.redo) == 0 {
		return vsel{}, false
	}
	st := h.redo[len(h.redo)-1]
	h.redo[len(h.redo)-1] = step{}
	h.redo = h.redo[:len(h.redo)-1]
	e.value = e.value[:st.at] + st.inserted + e.value[st.at+len(st.removed):]
	h.undo = append(h.undo, st)
	h.open = false
	return st.after, true
}
