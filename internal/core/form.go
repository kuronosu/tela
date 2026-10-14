package core

import (
	"errors"
	"fmt"
	"slices"

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
// Id gives an element an identifier; when several mounted elements share
// one, it names the first of them in tree order (HTML's getElementById),
// and the others take over when it unmounts. A Label's control is the
// element For names, or else the first labelable element inside it
// (Input, Textarea, Button). The control's accessible name comes from its
// labels (semantics.go). The Tree indexes the mounted elements by Id and
// the mounted Labels by For, so neither lookup walks the tree.

// indexAdd records el under key in *m.
func indexAdd(m *map[string][]*Element, key string, el *Element) {
	if *m == nil {
		*m = map[string][]*Element{}
	}
	(*m)[key] = append((*m)[key], el)
}

// indexRemove forgets el under key in m.
func indexRemove(m map[string][]*Element, key string, el *Element) {
	els := m[key]
	i := slices.Index(els, el)
	if i < 0 {
		return
	}
	if len(els) == 1 {
		delete(m, key)
		return
	}
	m[key] = slices.Delete(els, i, i+1)
}

// treeBefore reports whether a comes before b in tree order (an ancestor
// before its descendants, siblings in order).
func treeBefore(a, b *Element) bool {
	x, y := a, b
	for x.depth > y.depth {
		x = x.parent
	}
	for y.depth > x.depth {
		y = y.parent
	}
	if x == y {
		return a.depth < b.depth
	}
	for x.parent != y.parent {
		x, y = x.parent, y.parent
	}
	if x.parent == nil {
		return false
	}
	for _, c := range x.parent.children {
		switch c {
		case x:
			return true
		case y:
			return false
		}
	}
	return false
}

// setID records el under its Id prop (mount, update) and forgets old.
func (t *Tree) setID(el *Element, old string) {
	id := el.node.d.id
	if old == id {
		return
	}
	if old != "" {
		indexRemove(t.ids, old, el)
	}
	if id != "" {
		if t.dev {
			for _, o := range t.ids[id] {
				if o != el && o.mounted {
					t.logf("tela: duplicate Id(%q) on %s and %s; IDs must be unique in a window", id, t.describe(o), t.describe(el))
					break
				}
			}
		}
		indexAdd(&t.ids, id, el)
	}
}

// forID returns the For prop of a node ("" for none).
func forID(d *desc) string {
	if d.fp == nil {
		return ""
	}
	return d.fp.forID
}

// setLabelFor records Label el under its For prop (mount, update) and
// forgets old.
func (t *Tree) setLabelFor(el *Element, old string) {
	id := forID(el.node.d)
	if old == id || el.box.kind != style.KindLabel {
		return
	}
	if old != "" {
		indexRemove(t.labels, old, el)
	}
	if id != "" {
		indexAdd(&t.labels, id, el)
	}
}

// unmountIndexed forgets primitive el in the indexes of IDs and labels.
func (t *Tree) unmountIndexed(el *Element) {
	if id := el.node.d.id; id != "" {
		indexRemove(t.ids, id, el)
	}
	if id := forID(el.node.d); id != "" && el.box.kind == style.KindLabel {
		indexRemove(t.labels, id, el)
	}
}

// byID returns the first mounted element in tree order with the ID id,
// or nil.
func (t *Tree) byID(id string) *Element {
	var first *Element
	for _, el := range t.ids[id] {
		if el.mounted && (first == nil || treeBefore(el, first)) {
			first = el
		}
	}
	return first
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
		for _, l := range t.labels[id] {
			if l.mounted {
				out = append(out, l)
			}
		}
		if len(out) > 1 {
			slices.SortFunc(out, func(a, b *Element) int {
				if treeBefore(a, b) {
					return -1
				}
				return 1
			})
		}
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
