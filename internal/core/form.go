package core

import (
	"errors"
	"fmt"

	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text/edit"
)

// Forms, labels and element IDs (ADR-069), after HTML.
//
// # Activation
//
// Every click (a pointer click, Enter or Space on a button, ClickNode)
// that no handler prevented runs the activation behavior of the nearest
// element on its path that has one: a Button inside a Form submits it
// (Type "submit", the default) or resets it (Type "reset"; "button" does
// nothing); a Label focuses its control and clicks it, unless the click
// came from inside the control.
//
// # Submission
//
// Submitting a Form fires OnSubmit on it (it bubbles). Tela does not
// navigate and has no interactive validation: invalid fields (invalid:)
// do not block it. Enter in a single-line field of a form submits it
// implicitly (HTML): by clicking the form's first submit button when it
// has one (nothing when that button is disabled), else directly when the
// field is the form's only text field (with two or more text fields and
// no submit button, Enter does nothing). Reset restores the DefaultValue
// of the form's uncontrolled fields (controlled and bound ones show their
// Value) and fires nothing.
//
// # Labels and IDs
//
// Id gives an element an identifier (the last one mounted with an ID
// wins a duplicate); a Label's control is the element For names, or else
// the first labelable element inside it (Input, Textarea, Button). The
// control's accessible name comes from its labels (semantics.go).

// setID records el under its Id prop (mount, update) and forgets old.
func (t *Tree) setID(el *Element, old string) {
	id := el.node.d.id
	if old == id {
		return
	}
	if old != "" && t.ids[old] == el {
		delete(t.ids, old)
	}
	if id != "" {
		if t.ids == nil {
			t.ids = map[string]*Element{}
		}
		if t.dev {
			if o := t.ids[id]; o != nil && o != el && o.mounted {
				t.logf("tela: duplicate Id(%q) on %s and %s; IDs must be unique in a window", id, t.describe(o), t.describe(el))
			}
		}
		t.ids[id] = el
	}
}

// byID returns the mounted element with the ID id, or nil.
func (t *Tree) byID(id string) *Element {
	if el := t.ids[id]; el != nil && el.mounted {
		return el
	}
	return nil
}

// labelable reports whether el can be labeled by a Label.
func labelable(el *Element) bool {
	return el != nil && el.kind == elemPrimitive && (el.box.kind.IsTextField() || el.box.kind == style.KindButton)
}

// labelControl returns the control of Label label, or nil.
func (t *Tree) labelControl(label *Element) *Element {
	if fp := label.node.d.fp; fp != nil && fp.forID != "" {
		if el := t.byID(fp.forID); labelable(el) {
			return el
		}
		return nil
	}
	var found *Element
	var walk func(el *Element) bool
	walk = func(el *Element) bool {
		for _, c := range el.children {
			if c.kind == elemText {
				continue
			}
			if labelable(c) {
				found = c
				return false
			}
			if !walk(c) {
				return false
			}
		}
		return true
	}
	walk(label)
	return found
}

// labelsOf returns the Labels of control el in tree order: those whose
// For names it, and the Label around it without For.
func (t *Tree) labelsOf(el *Element) []*Element {
	var out []*Element
	id := el.node.d.id
	if id != "" && t.byID(id) == el {
		var walk func(e *Element)
		walk = func(e *Element) {
			for _, c := range e.children {
				if c.kind == elemPrimitive && c.box.kind == style.KindLabel {
					if fp := c.node.d.fp; fp != nil && fp.forID == id {
						out = append(out, c)
					}
				}
				walk(c)
			}
		}
		walk(t.root)
	}
	for a := el.styleParent(); a != nil; a = a.styleParent() {
		if a.box.kind == style.KindLabel {
			if fp := a.node.d.fp; (fp == nil || fp.forID == "") && t.labelControl(a) == el {
				out = append(out, a)
			}
			break
		}
	}
	return out
}

// formOf returns the Form el belongs to (its nearest Form ancestor), or
// nil.
func formOf(el *Element) *Element {
	for a := el.styleParent(); a != nil; a = a.styleParent() {
		if a.box.kind == style.KindForm {
			return a
		}
	}
	return nil
}

// dispatchClick fires a click at target (bubbling), flushes it as a
// discrete event and, unless a handler prevented it, runs the activation
// behavior of its path (see the comment at the top of form.go).
func (t *Tree) dispatchClick(target *Element, me MouseEvent) {
	st := fire(t, evClick, target, me, true)
	t.flushDiscrete()
	if st.defaultPrevented() {
		return
	}
	t.activate(target, me)
}

// activate runs the activation behavior of the nearest Button or Label at
// or above target.
func (t *Tree) activate(target *Element, me MouseEvent) {
	for el := mountedAncestor(target); el != nil; el = el.styleParent() {
		if el.kind != elemPrimitive {
			continue
		}
		switch el.box.kind {
		case style.KindButton:
			if el.istate&style.StateDisabled != 0 || outermostDisabled(el) != nil {
				return
			}
			form := formOf(el)
			if form == nil {
				return
			}
			switch buttonType(el.node.d) {
			case "submit":
				t.submitForm(form)
			case "reset":
				t.resetForm(form)
			}
			return
		case style.KindLabel:
			ctl := t.labelControl(el)
			if ctl == nil || isInside(target, ctl) || outermostDisabled(ctl) != nil {
				return
			}
			if t.focusable(ctl) {
				t.setFocus(ctl, focusMouse)
			}
			if ctl.mounted {
				t.dispatchClick(ctl, me)
			}
			return
		}
	}
}

// submitForm fires OnSubmit at form.
func (t *Tree) submitForm(form *Element) {
	if !form.mounted {
		return
	}
	fire(t, evSubmit, form, SubmitEvent{}, true)
	t.flushDiscrete()
}

// resetForm gives the uncontrolled fields of form their DefaultValue
// back.
func (t *Tree) resetForm(form *Element) {
	var walk func(el *Element)
	walk = func(el *Element) {
		for _, c := range el.children {
			if f := textField(c); f != nil {
				if fp := f.props(); !fp.hasValue && fp.bind == nil {
					t.setFieldValue(f, fp.defValue)
				}
			}
			walk(c)
		}
	}
	walk(form)
}

// implicitSubmit is Enter in single-line field f (HTML's implicit
// submission).
func (t *Tree) implicitSubmit(f *field) {
	form := formOf(f.el)
	if form == nil {
		return
	}
	var button *Element
	fields := 0
	var walk func(el *Element) bool
	walk = func(el *Element) bool {
		for _, c := range el.children {
			if c.kind == elemPrimitive {
				switch {
				case c.box.kind == style.KindButton && buttonType(c.node.d) == "submit":
					button = c
					return false
				case c.box.kind == style.KindInput:
					fields++
				}
			}
			if !walk(c) {
				return false
			}
		}
		return true
	}
	walk(form)
	if button != nil {
		if button.istate&style.StateDisabled != 0 || outermostDisabled(button) != nil {
			return
		}
		me := MouseEvent{Button: MouseButtonLeft}
		if bx := t.geomBox(button); bx != nil {
			c := windowRect(bx).Center()
			me.X, me.Y = c.X, c.Y
		}
		t.dispatchClick(button, me)
		return
	}
	if fields <= 1 {
		t.submitForm(form)
	}
}

// ---------------------------------------------------------------------
// ElementRef methods of text fields.

// field returns the text field the ref is attached to, or nil.
func (r ElementRef) field() *field {
	if el := r.element(); el != nil {
		return textField(el)
	}
	return nil
}

// Value returns the value of the text field the ref is attached to (with
// the text an IME is composing, as the DOM's input.value), "" for other
// elements: mainly for uncontrolled fields.
func (r ElementRef) Value() string {
	if f := r.field(); f != nil {
		return f.ed.RawValue()
	}
	return ""
}

// Select selects the whole value of the text field the ref is attached
// to (HTML's select()) and fires OnSelect; it does nothing on other
// elements.
func (r ElementRef) Select() {
	f := r.field()
	if f == nil || f.ed.Composing() {
		return
	}
	res := f.ed.Do(edit.Command{Kind: edit.SelectAll}, nil)
	t := f.t
	t.fieldChanged(f, res)
	t.fieldSelect(f, false)
}

// SetSelectionRange selects the bytes [start, end) of the value of the
// text field the ref is attached to (UTF-8 byte offsets, clamped to the
// value and moved to grapheme cluster boundaries; start > end selects
// nothing at end), as HTML's setSelectionRange, and fires OnSelect when
// it changed. It does nothing on other elements.
func (r ElementRef) SetSelectionRange(start, end int) {
	f := r.field()
	if f == nil || f.ed.Composing() {
		return
	}
	n := len(f.ed.Value())
	end = min(max(end, 0), n)
	start = min(max(start, 0), end)
	res := f.ed.Select(start, end)
	t := f.t
	t.fieldChanged(f, res)
	t.fieldSelect(f, false)
}

// SelectionRange returns the selection of the text field the ref is
// attached to in UTF-8 byte offsets of its value, ordered (HTML's
// selectionStart and selectionEnd); 0, 0 for other elements.
func (r ElementRef) SelectionRange() (start, end int) {
	if f := r.field(); f != nil {
		start, end, _ = f.ed.SelectionRange()
	}
	return start, end
}

// ---------------------------------------------------------------------
// Filling fields (telatest, tela mcp).

// ErrNotEditable is returned by FillNode for an element that is not an
// editable text field.
var ErrNotEditable = errors.New("core: the element is not an editable text field")

// FillNode fills the text field of node id like Playwright's fill: it
// focuses the field (ElementRef.Focus), selects its whole value and
// replaces it with s as one insertText edit (deleteContentBackward for an
// empty s), firing OnBeforeInput and OnChange; MaxLength applies. A
// disabled field gives ErrNodeDisabled, an element that is not an
// editable text field (or a ReadOnly one) ErrNotEditable.
func (t *Tree) FillNode(id semantics.NodeID, s string) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	f := textField(el)
	if f == nil {
		return fmt.Errorf("%w: %s", ErrNotEditable, t.describe(el))
	}
	if outermostDisabled(el) != nil {
		return fmt.Errorf("%w: %s", ErrNodeDisabled, t.describe(el))
	}
	if f.props().readOnly {
		return fmt.Errorf("%w: %s is read-only", ErrNotEditable, t.describe(el))
	}
	t.focusElement(el)
	if t.foc.el != el {
		return fmt.Errorf("%w: %s", ErrNotFocusable, t.describe(el))
	}
	if f.ed.Composing() {
		t.finishComposition(f)
		t.ime.reset = true
	}
	if !f.alive() {
		return fmt.Errorf("%w: %d", ErrNodeGone, id)
	}
	t.fieldChanged(f, f.ed.Do(edit.Command{Kind: edit.SelectAll}, nil))
	if s == "" {
		t.fieldDo(f, edit.Command{Kind: edit.DeleteBackward})
	} else {
		t.fieldDo(f, edit.Command{Kind: edit.InsertText, Text: s})
	}
	return nil
}

// FieldValue returns the value of the text field of node id (with the
// preedit of a composition) and whether the node is a text field.
func (t *Tree) FieldValue(id semantics.NodeID) (string, bool) {
	el, err := t.actionElement(id)
	if err != nil {
		return "", false
	}
	if f := textField(el); f != nil {
		return f.ed.RawValue(), true
	}
	return "", false
}
