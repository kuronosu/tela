package core

import "github.com/kuronosu/tela/internal/style"

// The focus manager (ADR-096): React Aria's FocusManager for package aria
// (UseFocusScope, menus, toolbars, grids). It moves the focus among the
// focusable elements of a subtree: the element a ref is attached to and
// its descendants in tree order, the content of the Portals inside it
// after its own (their order in the sequential focus navigation,
// focus.go). Hidden, disabled and inert elements, and those outside the
// active modal overlay (overlay.go), are not candidates. The moves are
// programmatic focus changes (ElementRef.Focus): focus-visible: follows
// the last interaction, so arrow keys show the ring.

// FocusOptions configures the moves of the focus manager.
type FocusOptions struct {
	// Tabbable restricts the candidates to the sequential focus order
	// (tab index 0 or more); otherwise every focusable element counts,
	// TabIndex(-1) included.
	Tabbable bool
	// Wrap goes around at the ends: FocusNext from the last candidate
	// focuses the first, FocusPrevious from the first the last.
	Wrap bool
	// From is where FocusNext and FocusPrevious start; the zero
	// ElementRef starts from the focused element. A start outside the
	// subtree starts before its first candidate (FocusNext) or after its
	// last one (FocusPrevious).
	From ElementRef
}

// FocusFirst focuses the first candidate of the subtree of the element
// scope is attached to and reports whether it found one.
func FocusFirst(scope ElementRef, o FocusOptions) bool {
	el := scope.element()
	if el == nil {
		return false
	}
	return el.tree.focusFirstIn(el, o.Tabbable)
}

// FocusLast focuses the last candidate of the subtree of scope.
func FocusLast(scope ElementRef, o FocusOptions) bool {
	el := scope.element()
	if el == nil {
		return false
	}
	t := el.tree
	cands, _, _ := t.focusCands(el, o.Tabbable, nil)
	if len(cands) == 0 {
		return false
	}
	return t.focusCand(cands[len(cands)-1].el)
}

// FocusNext focuses the candidate after o.From (or the focused element)
// in the subtree of scope, wrapping around with o.Wrap, and reports
// whether it found one.
func FocusNext(scope ElementRef, o FocusOptions) bool {
	return focusStep(scope, o, false)
}

// FocusPrevious focuses the candidate before o.From (or the focused
// element) in the subtree of scope, wrapping around with o.Wrap.
func FocusPrevious(scope ElementRef, o FocusOptions) bool {
	return focusStep(scope, o, true)
}

// ContainsFocus reports whether the focused element is the element r is
// attached to or inside its subtree (the element tree: Portals rendered
// from inside it count).
func ContainsFocus(r ElementRef) bool {
	el := r.element()
	if el == nil {
		return false
	}
	f := el.tree.foc.el
	return f != nil && f.mounted && isInside(f, el)
}

func focusStep(scope ElementRef, o FocusOptions, backward bool) bool {
	el := scope.element()
	if el == nil {
		return false
	}
	t := el.tree
	from := o.From.element()
	if from == nil {
		from = t.foc.el
	}
	cands, at, found := t.focusCands(el, o.Tabbable, from)
	if len(cands) == 0 {
		return false
	}
	var next *Element
	switch {
	case !found && backward:
		next = cands[len(cands)-1].el
	case !found:
		next = cands[0].el
	case backward:
		for i := len(cands) - 1; i >= 0; i-- {
			if cands[i].idx < at {
				next = cands[i].el
				break
			}
		}
		if next == nil && o.Wrap {
			next = cands[len(cands)-1].el
		}
	default:
		for _, c := range cands {
			if c.idx > at {
				next = c.el
				break
			}
		}
		if next == nil && o.Wrap {
			next = cands[0].el
		}
	}
	if next == nil {
		return false
	}
	return t.focusCand(next)
}

// focusFirstIn focuses the first candidate of scope's subtree (tabbable
// ones only with tabbable) and reports whether it found one.
func (t *Tree) focusFirstIn(scope *Element, tabbable bool) bool {
	cands, _, _ := t.focusCands(scope, tabbable, nil)
	if len(cands) == 0 {
		return false
	}
	return t.focusCand(cands[0].el)
}

// focusCand focuses el programmatically and reports whether it has the
// focus.
func (t *Tree) focusCand(el *Element) bool {
	t.focusElement(el)
	return t.foc.el == el
}

// focusEntry is a candidate of the focus manager and its position in the
// walk.
type focusEntry struct {
	el  *Element
	idx int32
}

// focusCands returns the candidates of scope's subtree in tree order
// (the content of the portals inside it after its own), and the position
// of from in the walk when it is in the subtree.
// perf: a walk of the subtree per move; collections with many items may
// want an index.
func (t *Tree) focusCands(scope *Element, tabbable bool, from *Element) (out []focusEntry, at int32, found bool) {
	var n int32
	var walk func(el, top *Element)
	walk = func(el, top *Element) {
		switch el.kind {
		case elemText:
			return
		case elemComponent:
			for _, c := range el.children {
				walk(c, top)
			}
			return
		}
		if !el.mounted || el != top && isPortal(el) {
			return // a portal's content comes after (below)
		}
		idx := n
		n++
		if el == from {
			at, found = idx, true
		}
		if cs := el.box.cs; (cs == nil || cs.Display == style.DisplayNone || el.inert || el.outside == outsideYes) && (from == nil || found) {
			return // nothing in it is a candidate, and the start is elsewhere
		}
		if focusTarget(el) && t.focusable(el) {
			if _, ok := tabRank(el); ok || !tabbable {
				out = append(out, focusEntry{el, idx})
			}
		}
		for _, c := range el.children {
			walk(c, top)
		}
	}
	walk(scope, scope)
	for _, p := range t.portalsIn(scope, nil) {
		walk(p, p)
	}
	return out, at, found
}
