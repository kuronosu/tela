package edit

import (
	"sort"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/text"
)

// Mask is the character that stands for each grapheme cluster of a
// password value in the displayed text (U+2022 BULLET, as in Chromium on
// Linux and Windows).
const Mask = "•"

// WordConvention selects how word movement and word deletion behave.
type WordConvention uint8

const (
	// WordsMacLinux moves to the end of the next word (alt+right on macOS,
	// ctrl+right in GTK and in Chromium on Linux and macOS), and
	// ctrl+Delete deletes to it.
	WordsMacLinux WordConvention = iota
	// WordsWindows moves to the start of the next word (ctrl+right on
	// Windows), and ctrl+Delete deletes to it. Moving and deleting
	// backward go to the start of the previous word in both conventions.
	WordsWindows
)

// Default history settings.
const (
	// DefaultUndoLimit is the number of undo steps kept when
	// Options.UndoLimit is zero (Blink's undo stack depth).
	DefaultUndoLimit = 1000
	// DefaultUndoDelay is the pause that closes a typing group when
	// Options.UndoDelay is zero (ProseMirror's newGroupDelay).
	DefaultUndoDelay = 500 * time.Millisecond
)

// Options configure an Editor. The zero value is a single-line, editable,
// unlimited field with the macOS/Linux word convention and default
// history.
type Options struct {
	// Multiline accepts line breaks (a textarea). Single-line fields (an
	// input) turn inserted line breaks into spaces and strip them from
	// values set by SetText.
	Multiline bool
	// Password displays a Mask per grapheme cluster instead of the text,
	// disables Copy and Cut, and makes Backspace delete whole grapheme
	// clusters.
	Password bool
	// ReadOnly refuses every change of the value; selection, Copy and
	// navigation still work (HTML readonly).
	ReadOnly bool
	// MaxLength, when positive, limits the length of the value in UTF-16
	// code units, like HTML maxlength: text the user inserts is truncated
	// on a grapheme cluster boundary to fit; values set by SetText may be
	// longer. Zero or negative means no limit.
	MaxLength int
	// Words selects the word movement and deletion convention.
	Words WordConvention
	// UndoLimit is the number of undo steps kept: zero means
	// DefaultUndoLimit, negative disables the history.
	UndoLimit int
	// UndoDelay is the pause between two edits that starts a new undo
	// step even when they would coalesce: zero means DefaultUndoDelay,
	// negative never splits by time.
	UndoDelay time.Duration
	// Now is the caller's clock (the platform frame clock). Nil means no
	// clock: edits never split by time.
	Now func() time.Duration
}

func (o *Options) undoLimit() int {
	if o.UndoLimit == 0 {
		return DefaultUndoLimit
	}
	return o.UndoLimit
}

func (o *Options) undoDelay() time.Duration {
	if o.UndoDelay == 0 {
		return DefaultUndoDelay
	}
	return o.UndoDelay
}

// Editor is an editable plain text: the committed value, a selection, an
// optional IME composition and the undo history. See the package
// documentation for the model and its contract with the caller.
//
// An Editor is not safe for concurrent use; it belongs to the UI thread.
type Editor struct {
	opts  Options
	value string
	// sel is the selection in displayed-text offsets (see Display).
	sel text.Selection

	composing  bool
	compAt     int    // value offset of the preedit
	preedit    string // sanitized preedit text
	preCursor  [2]int // cursor range in preedit bytes
	preHidden  bool   // the IME asked for no cursor
	compBefore snapshot

	display string
	bounds  []int // with Password: grapheme boundaries of the raw display
	version uint64

	gen  uint64 // state generation, for Plan
	hist history
}

// snapshot is a state of the value and its selection (value offsets).
type snapshot struct {
	value string
	sel   vsel
}

// vsel is a selection in value offsets.
type vsel struct{ anchor, focus text.Cursor }

func caretSel(off int, a text.Affinity) vsel {
	c := text.Cursor{Offset: off, Affinity: a}
	return vsel{c, c}
}

func (s vsel) ordered() (start, end int) {
	a, b := s.anchor.Offset, s.focus.Offset
	if a > b {
		a, b = b, a
	}
	return a, b
}

// New returns an editor holding value (cleaned like SetText) with the
// caret at its end and an empty history.
func New(value string, opts Options) *Editor {
	e := &Editor{opts: opts, gen: 1}
	e.value = sanitizeValue(value, opts.Multiline)
	e.rebuild()
	e.version = 1
	e.sel = e.toDisplaySel(caretSel(len(e.value), text.Downstream))
	return e
}

// Options returns the editor's options.
func (e *Editor) Options() Options { return e.opts }

// SetOptions changes the options. An active composition is canceled
// first (the caller resets the IME). Switching to single-line removes the
// line breaks of the value and resets the history (its steps can hold
// line breaks even when the value has none); toggling Password changes
// the displayed text (DisplayVersion). The selection keeps its value
// offsets.
func (e *Editor) SetOptions(o Options) Result {
	var r Result
	if e.composing {
		r = e.CancelComposition()
	}
	oldValue, oldSel := e.value, e.sel
	vs := e.valueSel()
	wasMulti := e.opts.Multiline
	e.opts = o
	if !o.Multiline && wasMulti {
		e.hist.clear()
	}
	if !o.Multiline && strings.ContainsRune(e.value, '\n') {
		e.value = sanitizeValue(e.value, false)
		e.hist.clear()
		vs = caretSel(len(e.value), text.Downstream)
	}
	if o.undoLimit() < 0 {
		e.hist.clear()
	} else {
		e.hist.trim(o.undoLimit())
	}
	dc := e.rebuild()
	e.sel = e.toDisplaySel(vs)
	r2 := e.report(oldValue, oldSel, dc)
	r.ValueChanged = r.ValueChanged || r2.ValueChanged
	r.SelectionChanged = r.SelectionChanged || r2.SelectionChanged
	r.DisplayChanged = r.DisplayChanged || r2.DisplayChanged
	e.gen++
	return r
}

// Value returns the committed value: the text without the preedit of an
// active composition.
func (e *Editor) Value() string { return e.value }

// Display returns the text to lay out and paint: the value with the
// preedit of an active composition inserted, masked in password mode.
// The layouts given to Plan and Do must be built from this string.
func (e *Editor) Display() string { return e.display }

// DisplayVersion returns a number that changes whenever Display changes
// (and only then), for the caller to know when to rebuild its layout.
func (e *Editor) DisplayVersion() uint64 { return e.version }

// Selection returns the selection in Display offsets, ready for the
// layout's Caret and SelectionRects. During a composition it is the
// cursor inside the preedit.
func (e *Editor) Selection() text.Selection { return e.sel }

// SelectionRange returns the selection in value offsets, ordered, and
// whether the focus is before the anchor (HTML selectionStart,
// selectionEnd and selectionDirection "backward"). During a composition
// it is collapsed where the preedit sits.
func (e *Editor) SelectionRange() (start, end int, backward bool) {
	vs := e.valueSel()
	start, end = vs.ordered()
	return start, end, vs.focus.Offset < vs.anchor.Offset
}

// SelectedText returns the selected part of the value ("" in password
// mode, where the value is not exposed, and during a composition).
func (e *Editor) SelectedText() string {
	if e.opts.Password || e.composing {
		return ""
	}
	s, t := e.valueSel().ordered()
	return e.value[s:t]
}

// Select sets the selection to the value offsets anchor and focus, moved
// to grapheme cluster boundaries (HTML setSelectionRange). It does
// nothing during a composition.
func (e *Editor) Select(anchor, focus int) Result {
	if e.composing {
		return Result{}
	}
	a := snapGrapheme(e.value, anchor, false)
	f := snapGrapheme(e.value, focus, false)
	vs := vsel{text.Cursor{Offset: a}, text.Cursor{Offset: f}}
	return e.applySelection(e.toDisplaySel(vs))
}

// SetTextMode tells SetText what to do with the undo history.
type SetTextMode uint8

const (
	// ResetHistory clears the undo and redo stacks: the new value is a new
	// document (the usual case for a value the application replaces).
	ResetHistory SetTextMode = iota
	// RecordHistory records the change as one undo step, so the user can
	// undo it (an application transform, such as formatting a number).
	RecordHistory
)

// SetText replaces the value programmatically (a controlled value). The
// text is cleaned like HTML's value sanitization (single-line fields
// strip line breaks, multi-line fields normalize them to LF) and may
// exceed MaxLength. A value equal to the current one changes nothing, not
// even the selection or the history, so echoing back the value an edit
// produced is free. Otherwise the caret goes to the end (HTML) and an
// active composition is canceled without a history step (the caller
// resets the IME). SetText reports no InputType: programmatic changes fire
// no input events.
func (e *Editor) SetText(s string, mode SetTextMode) Result {
	s = sanitizeValue(s, e.opts.Multiline)
	if s == e.value && !e.composing {
		return Result{}
	}
	oldValue, oldSel := e.value, e.sel
	base, before := e.value, e.valueSel()
	if e.composing {
		// The history knows the value from before the composition.
		base, before = e.compBefore.value, e.compBefore.sel
		e.endComposition()
	}
	if s == base {
		// The composition is gone and the value is back to what it was.
		e.value = s
		dc := e.rebuild()
		e.sel = e.toDisplaySel(before)
		e.gen++
		return e.report(oldValue, oldSel, dc)
	}
	after := caretSel(len(s), text.Downstream)
	switch mode {
	case RecordHistory:
		e.recordDiff(base, s, before, after)
	default:
		e.hist.clear()
	}
	e.value = s
	e.hist.open = false
	dc := e.rebuild()
	e.sel = e.toDisplaySel(after)
	e.gen++
	return e.report(oldValue, oldSel, dc)
}

// CanUndo and CanRedo report whether Undo and Redo would change the value.
func (e *Editor) CanUndo() bool { return !e.opts.ReadOnly && !e.composing && len(e.hist.undo) > 0 }
func (e *Editor) CanRedo() bool { return !e.opts.ReadOnly && !e.composing && len(e.hist.redo) > 0 }

// ClearHistory empties the undo and redo stacks.
func (e *Editor) ClearHistory() { e.hist.clear() }

// ValueToDisplay maps an offset in the value to the Display offset of the
// same position (an offset inside a masked grapheme cluster maps to its
// start; during a composition, the preedit sits after the offset where it
// was inserted).
func (e *Editor) ValueToDisplay(off int) int {
	return e.rawToDisplay(e.valueToRaw(max(0, min(off, len(e.value)))))
}

// DisplayToValue maps a Display offset to the value offset of the same
// position (offsets inside the preedit map to where it sits).
func (e *Editor) DisplayToValue(off int) int { return e.rawToValue(e.displayToRaw(off)) }

// RawValue returns the value with the preedit of an active composition
// inserted, unmasked: what a browser's input.value holds during a
// composition (the value of the input events fired while composing).
// Outside compositions it is Value.
func (e *Editor) RawValue() string { return e.raw() }

// raw returns the displayed text before masking.
func (e *Editor) raw() string {
	if !e.composing {
		return e.value
	}
	return e.value[:e.compAt] + e.preedit + e.value[e.compAt:]
}

// rebuild recomputes the displayed text and reports whether it changed
// (and the version with it).
func (e *Editor) rebuild() bool {
	raw := e.raw()
	d := raw
	e.bounds = nil
	if e.opts.Password {
		e.bounds = graphemeBounds(raw)
		d = strings.Repeat(Mask, len(e.bounds)-1)
	}
	if d == e.display {
		return false
	}
	e.display = d
	e.version++
	return true
}

func (e *Editor) rawToDisplay(r int) int {
	if !e.opts.Password {
		return r
	}
	i := sort.SearchInts(e.bounds, r)
	if i == len(e.bounds) || e.bounds[i] != r {
		i-- // inside a grapheme cluster: its start
	}
	return max(i, 0) * len(Mask)
}

func (e *Editor) displayToRaw(d int) int {
	d = max(0, min(d, len(e.display)))
	if !e.opts.Password {
		return d
	}
	return e.bounds[d/len(Mask)]
}

func (e *Editor) rawToValue(r int) int {
	if !e.composing || r <= e.compAt {
		return r
	}
	if r >= e.compAt+len(e.preedit) {
		return r - len(e.preedit)
	}
	return e.compAt
}

func (e *Editor) valueToRaw(v int) int {
	if e.composing && v > e.compAt {
		return v + len(e.preedit)
	}
	return v
}

func (e *Editor) toValue(c text.Cursor) text.Cursor {
	return text.Cursor{Offset: e.DisplayToValue(c.Offset), Affinity: c.Affinity}
}

func (e *Editor) toDisplay(c text.Cursor) text.Cursor {
	return text.Cursor{Offset: e.ValueToDisplay(c.Offset), Affinity: c.Affinity}
}

func (e *Editor) toDisplaySel(s vsel) text.Selection {
	return text.NewSelection(e.toDisplay(s.anchor), e.toDisplay(s.focus))
}

// valueSel returns the selection in value offsets.
func (e *Editor) valueSel() vsel {
	if e.composing {
		return caretSel(e.compAt, text.Downstream)
	}
	return vsel{e.toValue(e.sel.Anchor), e.toValue(e.sel.Focus)}
}

func sameSel(a, b text.Selection) bool { return a.Anchor == b.Anchor && a.Focus == b.Focus }

// report returns the changes from the value oldValue and the selection
// oldSel to the current state.
func (e *Editor) report(oldValue string, oldSel text.Selection, displayChanged bool) Result {
	return Result{
		ValueChanged:     e.value != oldValue,
		SelectionChanged: !sameSel(oldSel, e.sel),
		DisplayChanged:   displayChanged,
	}
}

func (e *Editor) now() time.Duration {
	if e.opts.Now == nil {
		return 0
	}
	return e.opts.Now()
}
