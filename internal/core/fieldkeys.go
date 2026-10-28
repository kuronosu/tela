package core

import (
	"runtime"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
	"github.com/kuronosu/tela/internal/text/edit"
)

// Keyboard and pointer editing of text fields (ADR-069).
//
// # Keys
//
// A keydown that no handler prevented runs the field's key binding
// (fieldBinding) before the other default actions. The bindings follow
// Chromium on each platform: Windows and Linux now (Ctrl shortcuts,
// Ctrl+arrows by word with the Windows or GTK word convention,
// Shift+Delete, Ctrl+Insert and Shift+Insert, Ctrl+Y and Ctrl+Shift+Z for
// redo; on Linux Up, Down, Page Up and Page Down in a single-line field go
// to its start or end, on Windows they do nothing), and macOS's Command
// and Option conventions (macBinding) for the Cocoa backend. A key's
// Text is typed when neither Ctrl nor Meta is held, nor Alt on Windows
// and Linux (Option types characters on macOS). Tab is not consumed
// (it moves the focus), nor are keys without a binding (Escape, function
// keys), which keep their default actions. During an IME composition and
// for KeyProcess keys nothing runs: the IME owns the keys.
//
// Edits run as edit commands: Plan, OnBeforeInput (canceled by
// PreventDefault), Apply, OnChange; movements and selections fire no
// input events. Enter in a single-line field submits its form
// (form.go).
//
// # Pointer
//
// A primary press on a field that focuses it (or keeps it focused) puts
// the caret under the pointer, extends the selection with Shift, selects
// a word on the second click and a paragraph (a whole Input) on the
// third; while the button is held with a mouse or a pen the selection
// follows the pointer, by the unit of the press, and the field scrolls
// when the pointer is past its edges (autoscroll: every frame the caret
// moves to the text under the pointer and is revealed, so the field
// scrolls by the distance past the edge per frame, as Blink's selection
// autoscroll does). A press finishes an IME composition first and resets
// the IME.

// fieldAction is what a key does in a text field.
type fieldAction uint8

const (
	actNone    fieldAction = iota // not handled: the other default actions run
	actConsume                    // handled, nothing to do (no page scroll)
	actCommand                    // run the command
	actPaste                      // paste from the clipboard
	actSubmit                     // implicit submission (form.go)
)

// keyFlavor selects the key bindings of a platform.
type keyFlavor uint8

const (
	flavorLinux keyFlavor = iota
	flavorWindows
	flavorMac
)

// keyFlavorOf is the flavor of the platform the program runs on.
var hostKeyFlavor = func() keyFlavor {
	switch runtime.GOOS {
	case "windows":
		return flavorWindows
	case "darwin":
		return flavorMac
	}
	return flavorLinux
}()

// shortcutLetter returns the lower-case Latin letter of a shortcut key:
// the logical key when it is a letter, else the physical key's ("KeyZ"),
// so Ctrl+Z works on non-Latin layouts as in Chromium.
func shortcutLetter(e KeyboardEvent) string {
	k := string(e.Key)
	if len(k) == 1 && (k[0] >= 'a' && k[0] <= 'z' || k[0] >= 'A' && k[0] <= 'Z') {
		return strings.ToLower(k)
	}
	if c := string(e.Code); len(c) == 4 && strings.HasPrefix(c, "Key") {
		return strings.ToLower(c[3:])
	}
	return ""
}

// fieldBinding returns what keydown e does in field f with the bindings
// of flavor fl; txt is the text the key produces.
func fieldBinding(fl keyFlavor, multi bool, e KeyboardEvent, txt string) (edit.Command, fieldAction) {
	if fl == flavorMac {
		return macBinding(multi, e, txt)
	}
	cmd := func(k edit.CommandKind) (edit.Command, fieldAction) {
		return edit.Command{Kind: k, Extend: e.Shift}, actCommand
	}
	ctrl := e.Ctrl && !e.Alt && !e.Meta
	plain := !e.Ctrl && !e.Alt && !e.Meta
	switch Key := e.Key; Key {
	case KeyValue(platform.KeyArrowLeft), KeyValue(platform.KeyArrowRight):
		left := Key == KeyValue(platform.KeyArrowLeft)
		switch {
		case ctrl && left:
			return cmd(edit.MoveWordLeft)
		case ctrl:
			return cmd(edit.MoveWordRight)
		case plain && left:
			return cmd(edit.MoveLeft)
		case plain:
			return cmd(edit.MoveRight)
		}
		return edit.Command{}, actNone
	case KeyValue(platform.KeyArrowUp), KeyValue(platform.KeyArrowDown), KeyValue(platform.KeyPageUp), KeyValue(platform.KeyPageDown):
		if e.Alt || e.Meta {
			return edit.Command{}, actNone
		}
		up := Key == KeyValue(platform.KeyArrowUp) || Key == KeyValue(platform.KeyPageUp)
		page := Key == KeyValue(platform.KeyPageUp) || Key == KeyValue(platform.KeyPageDown)
		switch {
		case multi && page && up:
			return cmd(edit.MovePageUp)
		case multi && page:
			return cmd(edit.MovePageDown)
		case multi && up:
			return cmd(edit.MoveUp)
		case multi:
			return cmd(edit.MoveDown)
		case fl == flavorWindows:
			return edit.Command{}, actConsume
		case up:
			return cmd(edit.MoveDocumentStart)
		}
		return cmd(edit.MoveDocumentEnd)
	case KeyValue(platform.KeyHome), KeyValue(platform.KeyEnd):
		home := Key == KeyValue(platform.KeyHome)
		switch {
		case ctrl && home:
			return cmd(edit.MoveDocumentStart)
		case ctrl:
			return cmd(edit.MoveDocumentEnd)
		case plain && home:
			return cmd(edit.MoveLineStart)
		case plain:
			return cmd(edit.MoveLineEnd)
		}
		return edit.Command{}, actNone
	case KeyValue(platform.KeyBackspace):
		switch {
		case ctrl:
			return edit.Command{Kind: edit.DeleteWordBackward}, actCommand
		case plain:
			return edit.Command{Kind: edit.DeleteBackward}, actCommand
		}
		return edit.Command{}, actNone
	case KeyValue(platform.KeyDelete):
		switch {
		case plain && e.Shift:
			return edit.Command{Kind: edit.Cut}, actCommand
		case ctrl && !e.Shift:
			return edit.Command{Kind: edit.DeleteWordForward}, actCommand
		case plain:
			return edit.Command{Kind: edit.DeleteForward}, actCommand
		}
		return edit.Command{}, actNone
	case KeyValue(platform.KeyInsert):
		switch {
		case ctrl && !e.Shift:
			return edit.Command{Kind: edit.Copy}, actCommand
		case plain && e.Shift:
			return edit.Command{}, actPaste
		}
		return edit.Command{}, actNone
	case KeyValue(platform.KeyEnter):
		switch {
		case e.Ctrl || e.Alt || e.Meta:
			return edit.Command{}, actNone
		case multi:
			return edit.Command{Kind: edit.InsertLineBreak}, actCommand
		}
		return edit.Command{}, actSubmit
	case KeyValue(platform.KeyTab), KeyValue(platform.KeyEscape):
		return edit.Command{}, actNone
	}
	if ctrl {
		switch shortcutLetter(e) {
		case "a":
			if !e.Shift {
				return edit.Command{Kind: edit.SelectAll}, actCommand
			}
		case "z":
			if e.Shift {
				return edit.Command{Kind: edit.Redo}, actCommand
			}
			return edit.Command{Kind: edit.Undo}, actCommand
		case "y":
			if !e.Shift {
				return edit.Command{Kind: edit.Redo}, actCommand
			}
		case "c":
			if !e.Shift {
				return edit.Command{Kind: edit.Copy}, actCommand
			}
		case "x":
			if !e.Shift {
				return edit.Command{Kind: edit.Cut}, actCommand
			}
		case "v":
			// Ctrl+Shift+V pastes as plain text: the same for plain text.
			return edit.Command{}, actPaste
		}
	}
	// Alt chords are shortcuts, not text (Chromium: WM_SYSCHAR on
	// Windows, GTK's accelerators on Linux). AltGr is never reported as
	// Alt, so its characters type; an Alt+numpad character arrives after
	// the Alt release, without Alt (win32).
	if txt != "" && plain {
		return edit.Command{Kind: edit.InsertText, Text: txt}, actCommand
	}
	return edit.Command{}, actNone
}

// macBinding is fieldBinding with macOS's conventions: Command for the
// clipboard, undo and select all; Option+arrows by word; Command+arrows
// to the line and text edges; Option+Backspace and Command+Backspace
// delete by word and to the line start.
func macBinding(multi bool, e KeyboardEvent, txt string) (edit.Command, fieldAction) {
	cmd := func(k edit.CommandKind) (edit.Command, fieldAction) {
		return edit.Command{Kind: k, Extend: e.Shift}, actCommand
	}
	switch e.Key {
	case KeyValue(platform.KeyArrowLeft), KeyValue(platform.KeyArrowRight):
		left := e.Key == KeyValue(platform.KeyArrowLeft)
		switch {
		case e.Meta && left:
			return cmd(edit.MoveLineStart)
		case e.Meta:
			return cmd(edit.MoveLineEnd)
		case e.Alt && left:
			return cmd(edit.MoveWordLeft)
		case e.Alt:
			return cmd(edit.MoveWordRight)
		case left:
			return cmd(edit.MoveLeft)
		}
		return cmd(edit.MoveRight)
	case KeyValue(platform.KeyArrowUp), KeyValue(platform.KeyArrowDown):
		up := e.Key == KeyValue(platform.KeyArrowUp)
		switch {
		case e.Meta && up:
			return cmd(edit.MoveDocumentStart)
		case e.Meta:
			return cmd(edit.MoveDocumentEnd)
		case multi && up:
			return cmd(edit.MoveUp)
		case multi:
			return cmd(edit.MoveDown)
		case up:
			return cmd(edit.MoveDocumentStart)
		}
		return cmd(edit.MoveDocumentEnd)
	case KeyValue(platform.KeyBackspace):
		switch {
		case e.Meta:
			return edit.Command{Kind: edit.DeleteToLineStart}, actCommand
		case e.Alt:
			return edit.Command{Kind: edit.DeleteWordBackward}, actCommand
		}
		return edit.Command{Kind: edit.DeleteBackward}, actCommand
	case KeyValue(platform.KeyDelete):
		if e.Alt {
			return edit.Command{Kind: edit.DeleteWordForward}, actCommand
		}
		return edit.Command{Kind: edit.DeleteForward}, actCommand
	case KeyValue(platform.KeyEnter):
		if multi {
			return edit.Command{Kind: edit.InsertLineBreak}, actCommand
		}
		return edit.Command{}, actSubmit
	case KeyValue(platform.KeyTab), KeyValue(platform.KeyEscape):
		return edit.Command{}, actNone
	}
	if e.Meta && !e.Ctrl {
		switch shortcutLetter(e) {
		case "a":
			return edit.Command{Kind: edit.SelectAll}, actCommand
		case "z":
			if e.Shift {
				return edit.Command{Kind: edit.Redo}, actCommand
			}
			return edit.Command{Kind: edit.Undo}, actCommand
		case "c":
			return edit.Command{Kind: edit.Copy}, actCommand
		case "x":
			return edit.Command{Kind: edit.Cut}, actCommand
		case "v":
			return edit.Command{}, actPaste
		}
	}
	if e.Ctrl && !e.Meta {
		switch shortcutLetter(e) {
		case "a":
			return cmd(edit.MoveLineStart)
		case "e":
			return cmd(edit.MoveLineEnd)
		case "k":
			return edit.Command{Kind: edit.DeleteToLineEnd}, actCommand
		}
	}
	if txt != "" && !e.Ctrl && !e.Meta {
		return edit.Command{Kind: edit.InsertText, Text: txt}, actCommand
	}
	return edit.Command{}, actNone
}

// fieldKey runs the default action of keydown e in the focused field f
// (see the comment at the top of fieldkeys.go). It reports whether the
// key was handled, so the other default actions do not run.
func (t *Tree) fieldKey(f *field, e KeyboardEvent, txt string) bool {
	if e.Key == KeyValue(platform.KeyProcess) {
		return true
	}
	if f.ed.Composing() {
		return e.Key != KeyValue(platform.KeyTab)
	}
	cmd, act := fieldBinding(hostKeyFlavor, f.multi, e, txt)
	switch act {
	case actNone:
		return false
	case actPaste:
		t.fieldPaste(f)
	case actSubmit:
		t.implicitSubmit(f)
	case actCommand:
		if cmd.Kind == edit.MovePageUp || cmd.Kind == edit.MovePageDown {
			if s := f.bx.scr; s != nil {
				cmd.Height = s.client.H
			}
		}
		t.fieldDo(f, cmd)
	}
	return true
}

// fieldDo runs an editing or selection command of the user on f: for an
// edit, OnBeforeInput (cancelable) then the edit and OnChange (fieldEdit).
func (t *Tree) fieldDo(f *field, cmd edit.Command) {
	var lay *text.Layout
	if cmd.Kind.NeedsLayout() {
		if lay = f.layout(); lay == nil {
			return
		}
	}
	p := f.ed.Plan(cmd, lay)
	if p.Empty() {
		return
	}
	if p.InputType != "" {
		st := fire(t, evBeforeInput, f.el, InputEvent{InputType: string(p.InputType), Data: p.Data}, true)
		t.flushDiscrete()
		if st.defaultPrevented() || !f.alive() {
			return
		}
	}
	r := f.ed.Apply(p)
	if r.Clipboard != "" && (cmd.Kind == edit.Copy || cmd.Kind == edit.Cut) {
		t.writeClipboard(r.Clipboard)
	}
	t.caretReset(f)
	t.fieldEdit(f, r, r.IsComposing)
	t.fieldSelect(f, true)
}

// fieldEdit updates f after an editor operation r of the user and, when
// it changed the value (with the preedit, RawValue), writes a bound Value
// and fires OnChange, then restores a controlled value (React).
// composing is the IsComposing of the event.
func (t *Tree) fieldEdit(f *field, r edit.Result, composing bool) {
	t.fieldChanged(f, r)
	raw := f.ed.RawValue()
	if raw == f.raw {
		return
	}
	f.raw = raw
	f.edited = true
	t.fieldStates(f)
	if f.bind != nil {
		f.bind.storeUI(raw)
	}
	it := r.InputType
	if it == "" {
		return
	}
	fire(t, evChange, f.el, ChangeEvent{Value: raw, InputType: string(it), Data: r.Data, IsComposing: composing}, true)
	t.flushDiscrete()
	t.restoreControlled(f)
}

// fieldSelect records the selection of f and fires OnSelect when it
// changed: on any change for code (user false), on a change to a
// non-empty selection for the user.
func (t *Tree) fieldSelect(f *field, user bool) {
	if !f.alive() {
		return
	}
	s, e, _ := f.ed.SelectionRange()
	if s == f.selStart && e == f.selEnd {
		return
	}
	f.selStart, f.selEnd = s, e
	if user && s == e {
		return
	}
	fire(t, evSelect, f.el, SelectEvent{Start: s, End: e}, true)
	t.flushDiscrete()
}

// checkSelect records the selection of f without firing OnSelect (a
// value set by code moves the caret silently).
func (f *field) checkSelect(bool) {
	f.selStart, f.selEnd, _ = f.ed.SelectionRange()
}

// ---------------------------------------------------------------------
// Pointer.

// fieldDrag is a selection being dragged with the pointer ps in f.
type fieldDrag struct {
	f  *field
	ps *pointerState
}

// fieldPoint returns the window point p in the coordinates of f's text
// layout.
func (t *Tree) fieldPoint(f *field, p gfx.Point) gfx.Point {
	l := t.localPoint(f.el, p)
	x, y := f.textOrigin()
	l = l.Sub(gfx.Point{X: x, Y: y})
	if s := f.bx.scr; s != nil && f.multi {
		l = l.Add(s.soff)
	}
	return l
}

// fieldPress handles a primary press of ps on target (see the comment at
// the top of fieldkeys.go), after the press moved the focus.
func (t *Tree) fieldPress(ps *pointerState, target *Element) {
	f := textField(target)
	if f == nil || !f.alive() || t.foc.el != target || outermostDisabled(target) != nil {
		return
	}
	if f.ed.Composing() {
		t.finishComposition(f)
		t.ime.reset = true
		if !f.alive() {
			return
		}
	}
	lay := f.layout()
	if lay == nil {
		return
	}
	pt := t.fieldPoint(f, ps.pos)
	cmd := edit.Command{Kind: edit.ClickAt, X: pt.X, Y: pt.Y, Extend: ps.mods&platform.ModShift != 0}
	switch {
	case ps.clickCount == 2:
		cmd = edit.Command{Kind: edit.SelectWordAt, X: pt.X, Y: pt.Y}
	case ps.clickCount >= 3:
		cmd = edit.Command{Kind: edit.SelectParagraphAt, X: pt.X, Y: pt.Y}
	}
	r := f.ed.Do(cmd, lay)
	t.caretReset(f)
	t.fieldChanged(f, r)
	t.fieldSelect(f, true)
	if ps.key.kind != platform.PointerTouch {
		t.in.fdrag = &fieldDrag{f: f, ps: ps}
	}
}

// fieldDragTo extends the dragged selection to the pointer.
func (t *Tree) fieldDragTo(d *fieldDrag) {
	f := d.f
	if !f.alive() || t.foc.el != f.el {
		t.in.fdrag = nil
		return
	}
	lay := f.layout()
	if lay == nil {
		return
	}
	pt := t.fieldPoint(f, d.ps.pos)
	r := f.ed.Do(edit.Command{Kind: edit.DragTo, X: pt.X, Y: pt.Y}, lay)
	if r.SelectionChanged {
		t.caretReset(f)
		t.fieldChanged(f, r)
		t.fieldSelect(f, true)
	}
}

// fieldPointerMove follows a pointer move: the drag of a selection.
func (t *Tree) fieldPointerMove(ps *pointerState) {
	if d := t.in.fdrag; d != nil && d.ps == ps {
		if !ps.pressing {
			t.in.fdrag = nil
			return
		}
		t.fieldDragTo(d)
	}
}

// fieldPointerUp ends the drag of a selection by ps.
func (t *Tree) fieldPointerUp(ps *pointerState) {
	if d := t.in.fdrag; d != nil && d.ps == ps {
		t.in.fdrag = nil
	}
}

// fieldAutoscrolling reports whether a selection drag is past the edges
// of its field, so frames must go on (autoscroll).
func (t *Tree) fieldAutoscrolling() bool {
	d := t.in.fdrag
	if d == nil || !d.f.alive() || !d.ps.pressing {
		return false
	}
	f := d.f
	pt := t.fieldPoint(f, d.ps.pos)
	l := f.bx.ld.Layout()
	if !f.multi {
		left := f.scrollX
		right := left + f.textWidth()
		lo, hi := f.scrollXRange()
		return pt.X < left && f.scrollX > lo || pt.X > right && f.scrollX < hi
	}
	s := f.bx.scr
	if s == nil {
		return false
	}
	top := s.soff.Y - l.Padding.Top
	bottom := top + s.client.H
	if pt.Y < top && s.pos.Y > 0 || pt.Y > bottom && s.pos.Y < s.max.Y {
		return true
	}
	// Horizontally (lines that do not wrap), in physical offsets: from 0
	// to the extent, or from minus it to 0 in a right-to-left field.
	left := s.soff.X - l.Padding.Left
	right := left + s.client.W
	px, lo, hi := s.flip(s.pos).X, float32(0), s.max.X
	if s.rtl {
		lo, hi = -s.max.X, 0
	}
	return pt.X < left && px > lo || pt.X > right && px < hi
}

// scrollXRange returns the smallest and largest horizontal scroll of an
// Input: from 0 to the overflow of its line, or, for a right-to-left
// line, whose overflow is on the left, from minus that overflow to 0.
func (f *field) scrollXRange() (lo, hi float32) {
	lay := f.layout()
	if lay == nil {
		return 0, 0
	}
	m := max(lay.MaxContentWidth()+caretWidth-f.textWidth(), 0)
	if lay.RTL() {
		return -m, 0
	}
	return 0, m
}

// animateFields runs one autoscroll step of a selection drag past the
// edges of its field (Animate).
func (t *Tree) animateFields() {
	if t.fieldAutoscrolling() {
		t.fieldDragTo(t.in.fdrag)
	}
}

// isTextFieldEl reports whether el is a text field.
func isTextFieldEl(el *Element) bool {
	return el != nil && el.kind == elemPrimitive && el.box != nil && el.box.kind.IsTextField()
}

// focusedField returns the focused text field, or nil.
func (t *Tree) focusedField() *field {
	el := t.foc.el
	if el == nil || !el.mounted {
		return nil
	}
	return textField(el)
}
