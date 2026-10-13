package edit

import (
	"github.com/kuronosu/tela/internal/text"
)

// InputType names an edit as the W3C Input Events Level 2 inputType does,
// for the beforeinput and input events.
type InputType string

// Input types used by the editor.
const (
	InputNone                  InputType = ""
	InputInsertText            InputType = "insertText"
	InputInsertLineBreak       InputType = "insertLineBreak"
	InputInsertFromPaste       InputType = "insertFromPaste"
	InputInsertCompositionText InputType = "insertCompositionText"
	InputInsertFromComposition InputType = "insertFromComposition"
	InputDeleteContent         InputType = "deleteContent"
	InputDeleteContentBackward InputType = "deleteContentBackward"
	InputDeleteContentForward  InputType = "deleteContentForward"
	InputDeleteWordBackward    InputType = "deleteWordBackward"
	InputDeleteWordForward     InputType = "deleteWordForward"
	InputDeleteSoftLineBack    InputType = "deleteSoftLineBackward"
	InputDeleteSoftLineForward InputType = "deleteSoftLineForward"
	InputDeleteByCut           InputType = "deleteByCut"
	InputDeleteCompositionText InputType = "deleteCompositionText"
	InputHistoryUndo           InputType = "historyUndo"
	InputHistoryRedo           InputType = "historyRedo"
)

// CommandKind is what a Command does.
type CommandKind uint8

const (
	// CommandNone does nothing.
	CommandNone CommandKind = iota

	// InsertText types Command.Text over the selection (insertText,
	// coalesced into typing undo steps).
	InsertText
	// Paste inserts Command.Text over the selection (insertFromPaste, one
	// undo step).
	Paste
	// InsertLineBreak inserts a line feed (insertLineBreak, Enter in a
	// multi-line field); it does nothing in a single-line field, where
	// Enter belongs to the caller (form submission).
	InsertLineBreak
	// DeleteBackward is Backspace: the selection, or the code point
	// before the caret (whole emoji sequences; whole grapheme clusters in
	// password mode). See backspaceStart.
	DeleteBackward
	// DeleteForward is Delete: the selection, or the grapheme cluster
	// after the caret.
	DeleteForward
	// DeleteWordBackward deletes to the start of the previous word
	// (ctrl+Backspace, alt+Backspace on macOS). Needs a layout.
	DeleteWordBackward
	// DeleteWordForward deletes to the end of the next word, or to the
	// start of the next word with WordsWindows (ctrl+Delete). Needs a
	// layout.
	DeleteWordForward
	// DeleteToLineStart deletes to the start of the visual line
	// (cmd+Backspace on macOS); at the start of a line it deletes like
	// Backspace. Needs a layout.
	DeleteToLineStart
	// DeleteToLineEnd deletes to the end of the visual line (ctrl+K on
	// macOS); at the end of a line it deletes like Delete. Needs a layout.
	DeleteToLineEnd
	// Cut deletes the selection (deleteByCut) and returns it in
	// Plan.Clipboard and Result.Clipboard. Nothing in password mode.
	Cut
	// Copy returns the selection in Plan.Clipboard and Result.Clipboard
	// and changes nothing. Nothing in password mode.
	Copy
	// Undo and Redo walk the history (historyUndo, historyRedo).
	Undo
	Redo

	// MoveLeft and MoveRight move one grapheme cluster on screen
	// (bidi-aware; text.MoveLeft, text.MoveRight).
	MoveLeft
	MoveRight
	// MoveBackward and MoveForward move one grapheme cluster in logical
	// order.
	MoveBackward
	MoveForward
	// MoveWordLeft and MoveWordRight move by word on screen, following
	// Options.Words, mapped by the paragraph's direction.
	MoveWordLeft
	MoveWordRight
	// MoveLineStart and MoveLineEnd are Home and End (visual line).
	MoveLineStart
	MoveLineEnd
	// MoveUp and MoveDown move a line, keeping the goal column.
	MoveUp
	MoveDown
	// MovePageUp and MovePageDown move by Command.Height (the viewport
	// height), keeping the goal column.
	MovePageUp
	MovePageDown
	// MoveDocumentStart and MoveDocumentEnd are ctrl+Home and ctrl+End.
	MoveDocumentStart
	MoveDocumentEnd
	// SelectAll selects the whole text (no layout needed).
	SelectAll

	// ClickAt puts the caret at the point (Command.X, Command.Y), in
	// layout coordinates; with Extend (shift+click) it moves the focus
	// there, by the unit of a word, line or paragraph selection.
	ClickAt
	// SelectWordAt selects the word at the point (double click).
	SelectWordAt
	// SelectLineAt selects the visual line at the point.
	SelectLineAt
	// SelectParagraphAt selects the paragraph at the point (triple click,
	// as browsers do in a textarea; the whole value in a single-line
	// field).
	SelectParagraphAt
	// DragTo extends the selection to the point while dragging, by the
	// unit the selection was made with.
	DragTo
)

// NeedsLayout reports whether the command needs the layout of Display.
// Edits that delete by word or line need it only when the selection is
// collapsed; it is then still reported here.
func (k CommandKind) NeedsLayout() bool {
	switch k {
	case DeleteWordBackward, DeleteWordForward, DeleteToLineStart, DeleteToLineEnd:
		return true
	}
	return k >= MoveLeft && k != SelectAll
}

// Command is an editing or selection command, as key and pointer handling
// produce them.
type Command struct {
	Kind CommandKind
	// Text is the text of InsertText and Paste.
	Text string
	// Extend keeps the anchor (shift) for movements and ClickAt.
	Extend bool
	// X and Y are the point of pointer commands, in layout coordinates.
	X, Y float32
	// Height is the viewport height of MovePageUp and MovePageDown
	// (logical px).
	Height float32
}

type planOp uint8

const (
	opNone planOp = iota
	opSelect
	opEdit
	opCopy
	opUndo
	opRedo
)

// Plan is what a command would do, computed without changing the editor
// (Editor.Plan). The caller fires beforeinput with InputType and Data when
// InputType is set and, unless it is canceled, calls Editor.Apply.
type Plan struct {
	// InputType is the input type of an edit; empty for commands that do
	// not change the value (movements, selections, Copy), which fire no
	// beforeinput.
	InputType InputType
	// Data is the text an insertion inserts (after cleaning and maxlength
	// truncation); empty for deletions.
	Data string
	// Clipboard is the text Copy and Cut put on the clipboard.
	Clipboard string

	ed  *Editor
	gen uint64
	op  planOp
	sel text.Selection // opSelect: the new selection (display offsets)

	// opEdit: the bytes [start, end) of the value become insert.
	start, end int
	insert     string
	kind       groupKind
}

// Empty reports whether applying the plan would change nothing (and
// return no clipboard text).
func (p Plan) Empty() bool { return p.op == opNone }

// Do plans and applies cmd at once, for callers that have no beforeinput
// to fire.
func (e *Editor) Do(cmd Command, l *text.Layout) Result { return e.Apply(e.Plan(cmd, l)) }

// Plan computes what cmd would do, without changing the editor. l is the
// layout of Display (laid out and broken into lines as shown); commands
// that need a layout (NeedsLayout) do nothing without one, the others
// ignore it. A layout of another text is a programming error and panics.
//
// While a composition is active, every command is a no-op (the IME owns
// the keys; Parley's rule): the caller commits or cancels the composition
// first (FinishComposition before a click, for instance).
func (e *Editor) Plan(cmd Command, l *text.Layout) Plan {
	p := Plan{ed: e, gen: e.gen}
	if e.composing {
		return p
	}
	if l != nil && l.Text() != e.display {
		panic("edit: the layout was not built from Editor.Display; rebuild it when DisplayVersion changes")
	}
	ro := e.opts.ReadOnly
	switch cmd.Kind {
	case InsertText:
		if !ro {
			e.planInsert(&p, cmd.Text, InputInsertText, groupTyping)
		}
	case Paste:
		if !ro {
			e.planInsert(&p, cmd.Text, InputInsertFromPaste, groupNone)
		}
	case InsertLineBreak:
		if !ro && e.opts.Multiline {
			e.planInsert(&p, "\n", InputInsertLineBreak, groupTyping)
		}
	case DeleteBackward, DeleteForward, DeleteWordBackward, DeleteWordForward, DeleteToLineStart, DeleteToLineEnd:
		if !ro {
			e.planDelete(&p, cmd.Kind, l)
		}
	case Cut:
		if s, t := e.valueSel().ordered(); !ro && !e.opts.Password && s < t {
			p.op, p.start, p.end, p.kind = opEdit, s, t, groupNone
			p.InputType, p.Clipboard = InputDeleteByCut, e.value[s:t]
		}
	case Copy:
		if s, t := e.valueSel().ordered(); !e.opts.Password && s < t {
			p.op, p.Clipboard = opCopy, e.value[s:t]
		}
	case Undo:
		if !ro && len(e.hist.undo) > 0 {
			p.op, p.InputType = opUndo, InputHistoryUndo
		}
	case Redo:
		if !ro && len(e.hist.redo) > 0 {
			p.op, p.InputType = opRedo, InputHistoryRedo
		}
	case SelectAll:
		p.op = opSelect
		p.sel = text.NewSelection(text.Cursor{Offset: 0}, text.Cursor{Offset: len(e.display)})
	default:
		if l == nil || cmd.Kind < MoveLeft || cmd.Kind > DragTo {
			return p
		}
		p.op = opSelect
		p.sel = e.selectionFor(cmd, l)
	}
	return p
}

// selectionFor returns the selection that a movement or pointer command
// gives, from the layout.
func (e *Editor) selectionFor(cmd Command, l *text.Layout) text.Selection {
	s := e.sel
	switch cmd.Kind {
	case MoveLeft:
		return l.Move(s, text.MoveLeft, cmd.Extend)
	case MoveRight:
		return l.Move(s, text.MoveRight, cmd.Extend)
	case MoveBackward:
		return l.Move(s, text.MoveBackward, cmd.Extend)
	case MoveForward:
		return l.Move(s, text.MoveForward, cmd.Extend)
	case MoveWordLeft, MoveWordRight:
		if e.opts.Words == WordsWindows {
			// Windows: right goes to the start of the next word, left
			// to the start of the previous one (in reading order).
			forward := (cmd.Kind == MoveWordRight) != l.RTL()
			if forward {
				return l.Move(s, text.MoveNextWordStart, cmd.Extend)
			}
			return l.Move(s, text.MoveWordBackward, cmd.Extend)
		}
		if cmd.Kind == MoveWordLeft {
			return l.Move(s, text.MoveWordLeft, cmd.Extend)
		}
		return l.Move(s, text.MoveWordRight, cmd.Extend)
	case MoveLineStart:
		return l.Move(s, text.MoveLineStart, cmd.Extend)
	case MoveLineEnd:
		return l.Move(s, text.MoveLineEnd, cmd.Extend)
	case MoveUp:
		return l.Move(s, text.MoveUp, cmd.Extend)
	case MoveDown:
		return l.Move(s, text.MoveDown, cmd.Extend)
	case MovePageUp:
		return l.MovePage(s, -cmd.Height, cmd.Extend)
	case MovePageDown:
		return l.MovePage(s, cmd.Height, cmd.Extend)
	case MoveDocumentStart:
		return l.Move(s, text.MoveDocumentStart, cmd.Extend)
	case MoveDocumentEnd:
		return l.Move(s, text.MoveDocumentEnd, cmd.Extend)
	case ClickAt:
		if cmd.Extend {
			return l.ExtendSelection(s, cmd.X, cmd.Y)
		}
		return text.CaretSelection(l.HitTest(cmd.X, cmd.Y))
	case SelectWordAt:
		return l.SelectWordAt(cmd.X, cmd.Y)
	case SelectLineAt:
		return l.SelectLineAt(cmd.X, cmd.Y)
	case SelectParagraphAt:
		return l.SelectParagraphAt(cmd.X, cmd.Y)
	case DragTo:
		return l.ExtendSelection(s, cmd.X, cmd.Y)
	}
	return s
}

// planInsert plans the insertion of s over the selection.
func (e *Editor) planInsert(p *Plan, s string, it InputType, kind groupKind) {
	s = sanitizeInput(s, e.opts.Multiline)
	start, end := e.valueSel().ordered()
	if m := e.opts.MaxLength; m > 0 {
		avail := m - (UTF16Len(e.value) - UTF16Len(e.value[start:end]))
		s = truncateUTF16(s, avail)
	}
	if s == "" {
		return
	}
	p.op, p.start, p.end, p.insert, p.kind = opEdit, start, end, s, kind
	p.InputType, p.Data = it, s
}

// planDelete plans a deletion command: the selection when it is not
// empty, else the range the command covers from the caret.
func (e *Editor) planDelete(p *Plan, k CommandKind, l *text.Layout) {
	it, kind := deleteInputType(k)
	start, end := e.valueSel().ordered()
	if start == end {
		c := start
		switch k {
		case DeleteBackward:
			start = backspaceStart(e.value, c, e.opts.Password)
		case DeleteForward:
			end = nextGrapheme(e.value, c)
		default:
			if l == nil {
				return
			}
			var m text.Movement
			switch k {
			case DeleteWordBackward:
				m = text.MoveWordBackward
			case DeleteWordForward:
				m = text.MoveWordForward
				if e.opts.Words == WordsWindows {
					m = text.MoveNextWordStart
				}
			case DeleteToLineStart:
				m = text.MoveLineStart
			default:
				m = text.MoveLineEnd
			}
			t := e.DisplayToValue(l.Move(text.CaretSelection(e.sel.Focus), m, false).Focus.Offset)
			backward := k == DeleteWordBackward || k == DeleteToLineStart
			switch {
			case backward && t < c:
				start = t
			case !backward && t > c:
				end = t
			case k == DeleteToLineStart:
				// At the start of a line: delete like Backspace (the
				// line break before it).
				start = backspaceStart(e.value, c, e.opts.Password)
			case k == DeleteToLineEnd:
				end = nextGrapheme(e.value, c)
			}
		}
		if start == end {
			return
		}
	}
	p.op, p.start, p.end, p.kind = opEdit, start, end, kind
	p.InputType = it
}

func deleteInputType(k CommandKind) (InputType, groupKind) {
	switch k {
	case DeleteBackward:
		return InputDeleteContentBackward, groupBackspace
	case DeleteForward:
		return InputDeleteContentForward, groupDelete
	case DeleteWordBackward:
		return InputDeleteWordBackward, groupNone
	case DeleteWordForward:
		return InputDeleteWordForward, groupNone
	case DeleteToLineStart:
		return InputDeleteSoftLineBack, groupNone
	}
	return InputDeleteSoftLineForward, groupNone
}

// Result reports what an operation changed.
type Result struct {
	// ValueChanged reports a change of the committed value (fire the
	// input/change event).
	ValueChanged bool
	// SelectionChanged reports a change of the selection's cursors.
	SelectionChanged bool
	// DisplayChanged reports a change of Display (rebuild the layout).
	DisplayChanged bool
	// InputType and Data describe the edit, for the input event: empty
	// when the value did not change by an edit (movements, SetText).
	InputType InputType
	Data      string
	// IsComposing is set for changes made while a composition stays
	// active (preedit updates).
	IsComposing bool
	// Clipboard is the text Copy and Cut put on the clipboard.
	Clipboard string
}

// Apply carries out a plan made by Plan on this editor. A plan made
// before any other change of the editor (a stale plan, for instance after
// a beforeinput handler called SetText) does nothing.
func (e *Editor) Apply(p Plan) Result {
	if p.ed != e || p.gen != e.gen || p.op == opNone {
		return Result{}
	}
	switch p.op {
	case opCopy:
		return Result{Clipboard: p.Clipboard}
	case opSelect:
		return e.applySelection(p.sel)
	case opUndo, opRedo:
		oldValue, oldSel := e.value, e.sel
		var vs vsel
		if p.op == opUndo {
			vs, _ = e.undo()
		} else {
			vs, _ = e.redo()
		}
		dc := e.rebuild()
		e.sel = e.toDisplaySel(vs)
		e.gen++
		r := e.report(oldValue, oldSel, dc)
		r.InputType = p.InputType
		return r
	}
	r := e.replace(p.start, p.end, p.insert, p.kind)
	r.InputType, r.Data, r.Clipboard = p.InputType, p.Data, p.Clipboard
	return r
}

// applySelection sets the selection (display offsets). A change closes
// the typing group of the history.
func (e *Editor) applySelection(s text.Selection) Result {
	old := e.sel
	e.sel = s
	e.gen++
	r := Result{SelectionChanged: !sameSel(old, s)}
	if r.SelectionChanged {
		e.hist.open = false
	}
	return r
}

// replace replaces the bytes [start, end) of the value with s, records it
// in the history and puts the caret after the inserted text: Upstream,
// attached to it, and moved forward to a grapheme boundary when the
// insertion merged with what follows (a base before a combining mark);
// after a deletion, Downstream and moved back to a boundary.
func (e *Editor) replace(start, end int, s string, kind groupKind) Result {
	oldValue, oldSel := e.value, e.sel
	before := e.valueSel()
	removed := e.value[start:end]
	e.value = e.value[:start] + s + e.value[end:]
	var after vsel
	if s != "" {
		after = caretSel(snapGrapheme(e.value, start+len(s), true), text.Upstream)
	} else {
		after = caretSel(snapGrapheme(e.value, start, false), text.Downstream)
	}
	e.record(step{at: start, removed: removed, inserted: s, before: before, after: after, kind: kind})
	dc := e.rebuild()
	e.sel = e.toDisplaySel(after)
	e.gen++
	return e.report(oldValue, oldSel, dc)
}
