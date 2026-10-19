package core

import (
	"math"

	"github.com/kuronosu/tela/internal/style"
)

// Focus (ARCHITECTURE section 14) follows the HTML focus model.
//
// # Focusable elements
//
// A Button, an Input and a Textarea are focusable, as are the primitives
// with a TabIndex; the others are not. An element is not focusable while it or an ancestor is
// disabled (Disabled) or display: none, while it is visibility: hidden or
// display: contents, or before it has a computed style (it has no box
// yet). The sequential focus order (Tab) holds the focusable elements
// with a tab index of 0 or more (a Button's is 0): those with a positive
// index first, in ascending order, then those with 0, in tree order (HTML's
// rules); -1 makes an element focusable by click and by ElementRef.Focus
// only.
//
// # Moving the focus
//
// The focus moves by keyboard (Tab and Shift+Tab, keyboard.go), by mouse
// (a press of the primary button focuses the nearest focusable element at
// or above the target; a press elsewhere blurs the focused element, and
// PreventDefault on OnPointerDown or OnMouseDown keeps the focus where it
// is), by script (ElementRef.Focus and Blur, AutoFocus, the restoration
// of a focus scope) and by fix-up: a focused element that stops being
// focusable (disabled, hidden) is blurred, with OnBlur, as Chromium does.
// Unmounting the focused element leaves nothing focused, without OnBlur
// (React). Tab with nothing focused starts from the point the last press
// blurred the focus at, or from the element blurred last (HTML's
// sequential focus navigation starting point).
//
// A change fires OnBlur on the element losing the focus, then OnFocus on
// the element gaining it; both bubble (React's onFocus and onBlur, the
// DOM's focusout and focusin). A handler of OnBlur that moves the focus
// elsewhere cancels the rest of the change. After each event the queued
// updates are applied and components rebuilt, as after other discrete
// events (outside a frame).
//
// # Window focus
//
// While the window does not have keyboard focus, the focused element stays
// the tree's (ElementRef.Focused reports it) but does not match focus:,
// focus-visible: or focus-within:. Losing the window focus fires OnBlur on
// it and gaining it fires OnFocus again, as in browsers. A Host starts
// with an unfocused window (the backend's FocusEvent turns it on); a Tree
// alone starts with a focused one.
//
// # Variants
//
// The focused element matches focus:, it and its ancestors focus-within:,
// and it matches focus-visible: following Chromium's heuristic: unless the
// last focus that was not programmatic came from a pointer press, or a
// key that is not a modifier was pressed (without Ctrl, Alt or Meta) since
// the last pointer press. So Tab shows it, a click does not, a key typed
// after the click does, and a programmatic focus shows it when the last
// interaction was the keyboard (or there was none). group-focus:,
// peer-focus: and their -visible and -within forms follow (input.go's
// interaction states). A change restyles only the elements whose classes
// test the states that changed.
//
// # AutoFocus
//
// The first element mounted with AutoFocus(true) in a commit takes the
// focus after the style phase of the frame that shows it (before layout
// effects, like React's autoFocus at commit), unless another element
// gained the focus between its mount and then. A focus change by a
// Frame (fix-up, AutoFocus, scope restoration) fires its handlers inside
// the frame; their state updates apply in the next one.
//
// # Focus scopes
//
// A focus scope (FocusScope, reserved for package aria's UseFocusScope)
// on a primitive contains Tab navigation inside its subtree while it is
// mounted (the most recently mounted containing scope wins), and restores
// the focus to the element focused when it mounted when it unmounts, if
// the focus was inside it (or nowhere) then.

// focusType is how the focus moved.
type focusType uint8

const (
	focusNone     focusType = iota // no focus change yet
	focusKeyboard                  // sequential navigation (Tab)
	focusMouse                     // a pointer press
	focusScript                    // ElementRef.Focus, AutoFocus, scopes, fix-up
)

// focusState is the focus of a Tree (UI thread).
type focusState struct {
	// el is the focused element, nil for none.
	el *Element
	// window reports that the window has keyboard focus: el matches the
	// focus variants and receives focus events.
	window bool
	// visible reports that the focused element matches focus-visible:
	// (lastType != focusMouse || hadKey).
	visible  bool
	lastType focusType
	hadKey   bool
	// gen counts the elements that gained the focus (AutoFocus checks
	// that none did since its mount).
	gen uint64
	// shown is the element whose focus states are set, shownVis whether
	// focus-visible is among them.
	shown    *Element
	shownVis bool
	// start is the sequential navigation starting point while nothing is
	// focused.
	start *Element
	// auto is the AutoFocus element of the pending commit, mounted when gen
	// was autoGen.
	auto    *Element
	autoGen uint64
	// scopes are the mounted focus scopes, in mount order; restores the
	// elements to restore the focus to after unmounted scopes.
	scopes   []*focusScope
	restores []*Element
	// space is the Button whose Space press waits for its release.
	space *Element
	// tab is the scratch list of tab order walks.
	tab []tabEntry
}

// Focused returns the focused element, or nil.
func (t *Tree) Focused() *Element { return t.foc.el }

// WindowFocused reports whether the tree's window has keyboard focus.
func (t *Tree) WindowFocused() bool { return t.foc.window }

// SetWindowFocused records whether the window has keyboard focus (a
// platform FocusEvent): losing it fires OnBlur on the focused element and
// removes its focus states, gaining it gives them back with OnFocus.
func (t *Tree) SetWindowFocused(focused bool) {
	f := &t.foc
	if f.window == focused {
		return
	}
	if focused {
		// An element that stopped being focusable while the window was
		// inactive is blurred silently: it never received OnFocus.
		t.fixFocus()
	}
	f.window = focused
	if fld := t.focusedField(); fld != nil {
		// The caret shows and the selection's colors follow the window.
		if focused {
			t.caretReset(fld)
		} else {
			fld.markPaint()
		}
	}
	if !focused {
		t.cancelSpace()
		t.syncFocus()
		if el := f.el; el != nil && el.mounted {
			dispatch(t, evBlur, el, FocusEvent{}, true, false)
			t.afterFocusEvent()
		}
		return
	}
	t.syncFocus()
	if el := f.el; el != nil && el.mounted {
		dispatch(t, evFocus, el, FocusEvent{}, true, false)
		t.afterFocusEvent()
	}
}

// afterFocusEvent applies the updates of focus handlers and builds, as
// after other discrete events; inside a frame the frame goes on with the
// focus states and the updates wait for the next one.
func (t *Tree) afterFocusEvent() {
	if !t.inFrame {
		t.flushDiscrete()
	}
}

// ---------------------------------------------------------------------
// Focusability.

// focusTarget reports whether el takes the focus at all: a Button or an
// element with a TabIndex.
func focusTarget(el *Element) bool {
	return el.kind == elemPrimitive && (el.node.d.hasTab || el.box.kind == style.KindButton || el.box.kind.IsTextField())
}

// tabRank returns the position group of el in the sequential focus order
// (a positive tab index, or MaxInt32 for 0) and whether it is in it.
func tabRank(el *Element) (int32, bool) {
	d := el.node.d
	ti := int32(0)
	if d.hasTab {
		ti = d.tabIndex
	} else if el.box.kind != style.KindButton && !el.box.kind.IsTextField() {
		return 0, false
	}
	switch {
	case ti < 0:
		return 0, false
	case ti == 0:
		return math.MaxInt32, true
	}
	return ti, true
}

// focusable reports whether el can take the focus now (see the comment at
// the top of focus.go).
func (t *Tree) focusable(el *Element) bool {
	if el == nil || !el.mounted || el.parent == nil || !focusTarget(el) {
		return false
	}
	cs := el.box.cs
	if cs == nil || cs.Display == style.DisplayContents || cs.Visibility != style.VisibilityVisible {
		return false
	}
	if el.inert {
		return false // in an exiting Presence child (presence.go)
	}
	for a := el; a != nil; a = a.styleParent() {
		acs := a.box.cs
		if acs == nil || acs.Display == style.DisplayNone || a.istate&style.StateDisabled != 0 {
			return false
		}
	}
	return true
}

// fixFocus blurs the focused element when it is no longer focusable
// (disabled, hidden), firing OnBlur as Chromium does.
func (t *Tree) fixFocus() {
	if el := t.foc.el; el != nil && !t.focusable(el) {
		t.setFocus(nil, focusScript)
	}
}

// ---------------------------------------------------------------------
// Focus changes.

// setFocus moves the focus to el (nil: blur), as typ says: the steps at
// the top of focus.go.
func (t *Tree) setFocus(el *Element, typ focusType) {
	f := &t.foc
	if el == f.el {
		return
	}
	if typ != focusScript {
		f.lastType = typ
	}
	f.visible = f.lastType != focusMouse || f.hadKey || isTextFieldEl(el)
	t.cancelSpace()
	old := f.el
	g := f.gen
	if old != nil {
		// The states follow at the end (syncFocus), so the ancestors that
		// keep focus-within: are not restyled.
		f.el = nil
		f.start = old
		if fld := textField(old); fld != nil && old.mounted {
			t.fieldBlurred(fld)
			if f.gen != g || f.el != nil {
				return // a handler moved the focus
			}
		}
		if f.window && old.mounted {
			dispatch(t, evBlur, old, FocusEvent{}, true, false)
			t.afterFocusEvent()
			if f.gen != g || f.el != nil {
				return // a handler moved the focus
			}
		}
	}
	if el == nil || !el.mounted {
		t.syncFocus()
		return
	}
	f.gen++
	f.el, f.start = el, nil
	t.syncFocus()
	t.revealFocused(el, typ)
	if fld := textField(el); fld != nil {
		t.fieldFocused(fld, typ)
	}
	if f.window {
		dispatch(t, evFocus, el, FocusEvent{}, true, false)
		t.afterFocusEvent()
	}
}

// revealFocused scrolls the focused element into view after the next
// layout (HTML's focusing steps), in every scroll container around it,
// except for pointer focus, whose target is already under the pointer.
func (t *Tree) revealFocused(el *Element, typ focusType) {
	if typ == focusMouse {
		return
	}
	t.scr.reveal = el
	t.requestRepaint()
}

// updateVisible recomputes focus-visible: after a key or a pointer press.
func (t *Tree) updateVisible() {
	f := &t.foc
	// Text fields match focus-visible however they were focused
	// (Chromium).
	if v := f.lastType != focusMouse || f.hadKey || isTextFieldEl(f.el); v != f.visible {
		f.visible = v
		t.syncFocus()
	}
}

// noteMouse records a pointer press for the focus-visible heuristic.
func (t *Tree) noteMouse() {
	if t.foc.hadKey {
		t.foc.hadKey = false
		t.updateVisible()
	}
}

// syncFocus makes the focus states (focus:, focus-visible:, focus-within:)
// match the focused element, changing only the elements whose states
// differ.
func (t *Tree) syncFocus() {
	f := &t.foc
	var want *Element
	if f.window && f.el != nil && f.el.mounted {
		want = f.el
	}
	vis := want != nil && f.visible
	if want == f.shown {
		if want != nil && vis != f.shownVis {
			t.setState(want, style.StateFocusVisible, vis)
		}
		f.shownVis = vis
		return
	}
	ca := commonAncestor(f.shown, want)
	if f.shown != nil {
		t.focusStates(f.shown, false, false, ca)
	}
	if want != nil {
		t.focusStates(want, true, vis, ca)
	}
	f.shown, f.shownVis = want, vis
}

// focusStates turns the focus states of el on or off: focus: and
// focus-visible: (vis) on el, focus-within: on el and its ancestors below
// stop.
func (t *Tree) focusStates(el *Element, on, vis bool, stop *Element) {
	t.setState(el, style.StateFocus, on)
	t.setState(el, style.StateFocusVisible, on && vis)
	for a := el; a != nil && a != stop; a = a.styleParent() {
		t.setState(a, style.StateFocusWithin, on)
	}
}

// mouseFocus is the focus change of a primary press on target: the
// nearest focusable element at or above it takes the focus; with none,
// the focused element is blurred and the press becomes the starting point
// of sequential navigation.
func (t *Tree) mouseFocus(target *Element) {
	el := mountedAncestor(target)
	if el == nil {
		return
	}
	for x := el; x != nil; x = x.styleParent() {
		if t.focusable(x) {
			t.setFocus(x, focusMouse)
			return
		}
	}
	t.setFocus(nil, focusMouse)
	t.foc.lastType = focusMouse
	t.updateVisible()
	if t.foc.el == nil {
		t.foc.start = el
	}
}

// focusElement focuses el programmatically (ElementRef.Focus): nothing
// happens when el is not focusable.
func (t *Tree) focusElement(el *Element) {
	if t.focusable(el) {
		t.setFocus(el, focusScript)
	}
}

// ---------------------------------------------------------------------
// Sequential navigation.

// tabEntry is a focusable element of the sequential order: its rank (tab
// index group) and its position in tree order.
type tabEntry struct {
	el        *Element
	rank, idx int32
}

func (a tabEntry) less(b tabEntry) bool {
	return a.rank < b.rank || a.rank == b.rank && a.idx < b.idx
}

// tabWalker collects the sequential order of a subtree.
type tabWalker struct {
	out      []tabEntry
	n        int32
	start    *Element
	startIdx int32
	found    bool
}

func (w *tabWalker) walk(el *Element, hidden, inert bool) {
	switch el.kind {
	case elemText:
		return
	case elemComponent:
		for _, c := range el.children {
			w.walk(c, hidden, inert)
		}
		return
	}
	if !el.mounted {
		return
	}
	idx := w.n
	w.n++
	if el == w.start {
		w.startIdx, w.found = idx, true
	}
	cs := el.box.cs
	if cs == nil || cs.Display == style.DisplayNone {
		hidden = true
	}
	if el.istate&style.StateDisabled != 0 || el.inert {
		inert = true
	}
	if !hidden && !inert && el.parent != nil && cs.Display != style.DisplayContents &&
		cs.Visibility == style.VisibilityVisible {
		if r, ok := tabRank(el); ok {
			w.out = append(w.out, tabEntry{el, r, idx})
		}
	}
	if hidden && (w.start == nil || w.found) {
		return // nothing in it is focusable, and the start is elsewhere
	}
	for _, c := range el.children {
		w.walk(c, hidden, inert)
	}
}

// moveFocus moves the focus to the next element of the sequential order
// (the previous one when backward), wrapping around, inside the active
// containing focus scope if any.
func (t *Tree) moveFocus(backward bool) {
	f := &t.foc
	root := t.root
	if s := t.activeScope(); s != nil {
		root = s.el
	}
	start := f.el
	if start == nil {
		start = f.start
	}
	if start != nil && !start.mounted {
		start = nil
	}
	w := tabWalker{out: f.tab[:0], start: start}
	w.walk(root, false, false)
	f.tab = w.out
	defer func() {
		clear(f.tab)
		f.tab = f.tab[:0]
	}()
	if len(w.out) == 0 {
		return
	}
	var next *Element
	if w.found {
		// The starting point's key: its own rank when it is in the order,
		// else as a tab index of 0 (HTML).
		key := tabEntry{rank: math.MaxInt32, idx: w.startIdx}
		if r, ok := tabRank(start); ok {
			key.rank = r
		}
		var best *tabEntry
		for i := range w.out {
			e := &w.out[i]
			if backward {
				if e.less(key) && (best == nil || best.less(*e)) {
					best = e
				}
			} else if key.less(*e) && (best == nil || e.less(*best)) {
				best = e
			}
		}
		if best != nil {
			next = best.el
		}
	}
	if next == nil { // wrap around, or no starting point
		best := w.out[0]
		for _, e := range w.out[1:] {
			if backward == best.less(e) {
				best = e
			}
		}
		next = best.el
	}
	t.setFocus(next, focusKeyboard)
}

// ---------------------------------------------------------------------
// Focus work of a commit: fix-up, AutoFocus, scope restoration.

// focusWork does the focus work queued by the builds: it blurs a focused
// element that stopped being focusable, focuses the AutoFocus element of
// the commit once it has a style, and restores the focus of unmounted
// scopes.
func (t *Tree) focusWork() {
	f := &t.foc
	t.fixFocus()
	if a := f.auto; a != nil {
		switch {
		case !a.mounted:
			f.auto = nil
		case a.box.cs == nil:
			// Not styled yet (mounted by a discrete event's build): wait
			// for the frame.
		default:
			f.auto = nil
			if f.gen == f.autoGen {
				t.focusElement(a)
			}
		}
	}
	for len(f.restores) > 0 {
		el := f.restores[0]
		f.restores = f.restores[1:]
		if f.el == nil {
			t.focusElement(el)
		}
	}
	f.restores = nil
}

// focusPending reports whether focusWork has anything to do.
func (t *Tree) focusPending() bool {
	return t.foc.el != nil || t.foc.auto != nil || len(t.foc.restores) > 0
}

// mountFocus records the focus props of a primitive being mounted.
func (t *Tree) mountFocus(el *Element) {
	d := el.node.d
	if d.ref != nil {
		d.ref.el = el
		if len(d.ref.follow) > 0 {
			t.queueRefSync(d.ref)
		}
	}
	if d.autoFocus && t.foc.auto == nil {
		t.foc.auto, t.foc.autoGen = el, t.foc.gen
	}
	if d.scope != nil {
		t.addScope(el, *d.scope)
	}
}

// updateFocus records the focus props of a primitive updated from old.
func (t *Tree) updateFocus(el *Element, old *desc) {
	d := el.node.d
	if old.ref != d.ref {
		if old.ref != nil && old.ref.el == el {
			old.ref.el = nil
		}
		if d.ref != nil {
			d.ref.el = el
			if len(d.ref.follow) > 0 {
				t.queueRefSync(d.ref)
			}
		}
	}
	switch {
	case d.scope != nil && el.fscope == nil:
		t.addScope(el, *d.scope)
	case d.scope == nil && el.fscope != nil:
		t.removeScope(el.fscope)
		el.fscope = nil
	case d.scope != nil:
		el.fscope.opts = *d.scope
	}
}

// unmountFocus handles the unmount of primitive el: its ElementRef is
// detached, its focus scope ends (queueing the restoration), and when it
// is focused nothing is (no OnBlur; React).
func (t *Tree) unmountFocus(el *Element) {
	f := &t.foc
	if r := el.node.d.ref; r != nil && r.el == el {
		r.el = nil
	}
	if s := el.fscope; s != nil {
		t.removeScope(s)
		el.fscope = nil
		if s.opts.restore && s.prev != nil && (f.el == nil || isInside(f.el, el)) {
			f.restores = append(f.restores, s.prev)
		}
	}
	if f.space == el {
		f.space = nil
	}
	if f.auto == el {
		f.auto = nil
	}
	if f.start == el {
		f.start = nil
	}
	if f.el == el {
		f.el = nil
		t.syncFocus()
	}
}

// resetFocus forgets the focus (the tree's content is gone).
func (t *Tree) resetFocus() {
	f := &t.foc
	t.cancelSpace()
	f.el = nil
	t.syncFocus()
	f.start, f.auto, f.restores, f.scopes = nil, nil, nil, nil
}

// isInside reports whether el is anc or one of its descendants.
func isInside(el, anc *Element) bool {
	for x := el; x != nil; x = x.parent {
		if x == anc {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// Focus scopes (reserved for package aria).

// scopeOpts are the options of a focus scope.
type scopeOpts struct {
	contain, restore bool
}

// focusScope is a mounted focus scope.
type focusScope struct {
	el   *Element
	opts scopeOpts
	// prev was focused when the scope mounted.
	prev *Element
}

// FocusScope returns a prop that makes a primitive a focus scope while it
// is mounted: with contain, Tab and Shift+Tab move the focus only among
// the focusable elements of its subtree (wrapping around inside it; the
// most recently mounted containing scope wins); with restore, when it
// unmounts while the focus is inside it (or nowhere), the focus returns
// to the element focused when it mounted, if that element is still
// focusable.
//
// Reserved for package aria (UseFocusScope, phase 7); it is not exposed
// by tela or h.
func FocusScope(contain, restore bool) Prop {
	o := &scopeOpts{contain: contain, restore: restore}
	return Prop{name: "FocusScope", apply: func(d *desc) { d.scope = o }}
}

func (t *Tree) addScope(el *Element, o scopeOpts) {
	s := &focusScope{el: el, opts: o, prev: t.foc.el}
	el.fscope = s
	t.foc.scopes = append(t.foc.scopes, s)
}

func (t *Tree) removeScope(s *focusScope) {
	ss := t.foc.scopes
	for i, x := range ss {
		if x == s {
			copy(ss[i:], ss[i+1:])
			ss[len(ss)-1] = nil
			t.foc.scopes = ss[:len(ss)-1]
			return
		}
	}
}

// activeScope returns the most recently mounted containing scope, or nil.
func (t *Tree) activeScope() *focusScope {
	ss := t.foc.scopes
	for i := len(ss) - 1; i >= 0; i-- {
		if ss[i].opts.contain && ss[i].el.mounted {
			return ss[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Props.

// TabIndex makes a primitive focusable and sets its place in the
// sequential focus order (see the comment at the top of focus.go): 0 in
// tree order, -1 out of it (focusable by click and ElementRef.Focus
// only), a positive index before every 0, in ascending order.
func TabIndex(i int) Prop {
	v := int32(max(min(i, math.MaxInt32-1), math.MinInt32))
	return Prop{name: "TabIndex", apply: func(d *desc) { d.tabIndex, d.hasTab = v, true }}
}

// AutoFocus focuses the element after the commit that mounts it, when on
// (see the comment at the top of focus.go).
func AutoFocus(on bool) Prop {
	return Prop{name: "AutoFocus", apply: func(d *desc) { d.autoFocus = on }}
}

// ---------------------------------------------------------------------
// Element references.

// elemRef is the slot of one UseElementRef call: the primitive the ref is
// attached to while it is mounted.
type elemRef struct {
	el *Element
	// follow are the UseScroll slots that follow el's scroll offsets.
	follow []*scrollFollow
}

// ElementRef is a handle on the primitive it is attached to (with
// AttachRef; h.Ref in the public API), returned by UseElementRef: the same
// comparable value on every build of the hook slot. Its methods act on
// the element while it is mounted and do nothing (or report false)
// otherwise, or for the zero ElementRef. Call them on the UI thread: in
// event handlers and effects.
type ElementRef struct {
	r *elemRef
}

// element returns the mounted element of r, or nil.
func (r ElementRef) element() *Element {
	if r.r == nil || r.r.el == nil || !r.r.el.mounted {
		return nil
	}
	return r.r.el
}

// Focus focuses the element, if it is focusable (HTML's focus()): OnBlur
// runs on the element that loses the focus and OnFocus on this one.
func (r ElementRef) Focus() {
	if el := r.element(); el != nil {
		el.tree.focusElement(el)
	}
}

// Blur removes the focus from the element if it has it (HTML's blur()):
// nothing is focused afterwards.
func (r ElementRef) Blur() {
	if el := r.element(); el != nil && el.tree.foc.el == el {
		el.tree.setFocus(nil, focusScript)
	}
}

// Focused reports whether the element is the focused element of its
// window (also while the window itself does not have keyboard focus).
func (r ElementRef) Focused() bool {
	el := r.element()
	return el != nil && el.tree.foc.el == el
}

// UseElementRef returns the ElementRef of this hook slot, the same on
// every build. Attach it to a primitive with AttachRef.
func UseElementRef() ElementRef {
	el, i := nextSlot(hookElementRef)
	if i < len(el.hooks) {
		r, ok := el.hooks[i].(*elemRef)
		if !ok {
			el.tree.slotMismatch(el, i, "UseElementRef")
		}
		return ElementRef{r}
	}
	r := &elemRef{}
	el.hooks = append(el.hooks, r)
	return ElementRef{r}
}

// AttachRef attaches r to the primitive (h.Ref): while it is mounted, r's
// methods act on it. A ref attached to several elements follows the last
// one mounted or updated with it. It panics on the zero ElementRef.
func AttachRef(r ElementRef) Prop {
	if r.r == nil {
		panic("tela: Ref(zero ElementRef): pass the ElementRef returned by tela.UseElementRef")
	}
	return Prop{name: "Ref", apply: func(d *desc) { d.ref = r.r }}
}
