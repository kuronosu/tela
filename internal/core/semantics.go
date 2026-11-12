package core

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// The semantics phase (ARCHITECTURE sections 8 and 16) derives the
// semantics tree (internal/semantics) from the Element and Box trees of
// the last Frame: roles, accessible names, states, actions and bounds.
//
// # When it runs
//
// On demand only: Semantics builds the whole tree when a consumer asks
// (tela inspect --aria, telatest, tela mcp, the semantics overlay) and
// nothing runs per frame, so a tree that nobody inspects costs nothing.
// The tree is kept and handed out again while nothing it shows changed.
// The first build is a full one (a 10k-element tree takes a few
// milliseconds, BenchmarkSemantics10k); later ones are incremental
// (semincr.go, ADR-277): the changes mark the elements whose nodes they
// may change (§8's needsSemantics), and only those, their ancestors and
// what depends on them are built again, the rest of the tree being shared
// with the last one.
//
// # Staleness
//
// A mark makes the last tree stale, a change of a Value read by an
// AriaLabelFn, AriaValueNowFn or AriaValueTextFn (obsSem) included, which
// needs no frame: it rebuilds nothing, it only marks its element.
// SemanticsStale reports it, and the listener set with OnSemanticsStale
// is told of the changes that need no frame, so that a consumer that
// keeps a copy of the tree (the OS accessibility bridges) knows to build
// it again; after a Frame the consumer checks SemanticsStale itself.
//
// # Nodes
//
// Every primitive with a computed style is an element node and every
// non-empty text a text node (components are transparent); node IDs are
// stable for the life of the element. Roles are explicit (Role, the
// first valid role of the list) or implicit, after HTML-AAM: Button is a
// button, H1 to H6 headings with their level, P a paragraph, Img an img
// (presentation with Alt("") unless it is labelled or has a TabIndex), A
// with Href a link, Strong, Em and Code strong, emphasis and code; the
// other primitives (Div, Span, the focusable elements with TabIndex
// included) are generic; Input and Textarea are textboxes (an Input of type search a
// searchbox) and a Form is a form when it has a name. "none" and
// "presentation" give way to the
// implicit role on focusable or labelled elements (ARIA's conflict
// resolution).
//
// Accessible names follow W3C AccName 1.2 as Playwright computes it
// (roleUtils.ts): AriaLabelledBy (the referenced element's AriaLabel or
// content, even when it is hidden), then AriaLabel, then Alt for images,
// then the content for the roles named from content (button, heading,
// link, checkbox, switch, tab, option, menuitem, cell...): the texts of
// the descendants that are not hidden, with block-level boundaries as
// spaces, a descendant contributing its own AriaLabel, Alt or content
// (when its role allows it), a nested textbox its text and a range widget
// its value. Names of roles that prohibit naming (paragraph, generic,
// strong...) are empty. White space is collapsed.
//
// States: disabled (Disabled on the element or an ancestor, or the
// nearest AriaDisabled), focused, focusable, hidden (display: none or
// AriaHidden on it or an ancestor, visibility other than visible, inert
// or outside the active modal overlay: overlay.go), scrollable (a
// user-scrollable container whose content overflows), the widget props
// (ariawidget.go), and the AriaChecked, AriaPressed, AriaSelected,
// AriaExpanded and AriaLevel props (H1 to H6 keep their own level, as in
// Playwright). Bounds are in
// window coordinates: layout positions, minus the scroll offsets painted
// last of the containers on the containing-block chain, transformed by
// every transform around the box; inline elements and texts bound their
// text's selection rectangles.
//
// # Actions
//
// ClickNode, FocusNode and ScrollNodeIntoView act on the element of a
// node of the last tree (a text acts on its parent element). They are the
// internal API of telatest and tela mcp: a click is HTML's
// element.click(), the DOM click path (OnClick on the element, bubbling)
// followed by the flush of a discrete event, without pointer events or a
// focus change; focus is ElementRef.Focus; scrolling into view is
// ElementRef.ScrollIntoView.

// semState is the semantics tree of a Tree (UI thread).
type semState struct {
	root *semantics.Node
	// pending reports a mark since root was built (semincr.go); focus,
	// viewport and rootEl are the focused element, the viewport and the
	// root element it was built with.
	pending  bool
	focus    *Element
	viewport gfx.Size
	rootEl   *Element
	// gen numbers the passes of the builds; reuse reports that the pass
	// may keep the nodes of unmarked elements, checking that it is the
	// full build of TELA_CHECK_SEMANTICS (which changes no element's
	// state); last is the root the last pass built.
	gen      uint64
	reuse    bool
	checking bool
	last     *semantics.Node
	// volatile holds the elements whose semantics read elements outside
	// their subtree; labels reports a change of the IDs or of a Label's
	// For since the last build.
	volatile map[*Element]semVolatile
	labels   bool
	// byID maps node IDs to the mounted elements that have them.
	byID map[semantics.NodeID]*Element
	next uint64
	// Scratch state of the name computation and of text bounds.
	alt   altCtx
	rects []gfx.Rect
	// portal is the portal being built (its content follows the root's,
	// portal.go); others met inside the tree are skipped.
	portal *Element
	// fns caches the results of the semantics functions of each element
	// (AriaLabelFn, AriaValueNowFn, AriaValueTextFn) evaluated in the build
	// (semFns); cleared at the start of a build and when one is stale.
	fns map[*Element]semFnVals
	// onStale is the listener of OnSemanticsStale, told is set once it was
	// told about the tree last built.
	onStale func()
	told    bool
}

// SemanticsStale reports whether the tree Semantics last returned is out
// of date: something it shows changed since (an element was built,
// restyled, laid out, moved or focused), or a Value an AriaLabelFn (or
// AriaValueNowFn, AriaValueTextFn) read changed. Call it on the UI
// thread.
func (t *Tree) SemanticsStale() bool { return t.semPending() }

// OnSemanticsStale sets the function called on the UI thread when the
// semantics tree becomes stale without a frame: a Value read by an
// AriaLabelFn, AriaValueNowFn or AriaValueTextFn changed (the hook point
// of the OS accessibility bridges, ADR-246). It is called once per tree
// built: again only after Semantics built a new one. It must not build the
// tree itself (it may run inside ApplyUpdates or a frame); it should
// schedule that. Changes that request a frame are not reported: check
// SemanticsStale after each Frame. nil removes the listener.
func (t *Tree) OnSemanticsStale(f func()) { t.sem.onStale = f }

// semanticsStale marks the semantics of el stale (a Value its AriaLabelFn
// read changed) and tells the listener.
func (t *Tree) semanticsStale(el *Element) {
	s := &t.sem
	t.semMark(el, semSelf)
	if s.onStale != nil && !s.told {
		s.told = true
		s.onStale()
	}
}

// ariaLabel returns the aria-label of primitive el, whose ARIA props are
// a (nil without): its AriaLabel, or the result of its AriaLabelFn
// (semFns).
func (t *Tree) ariaLabel(el *Element, a *ariaProps) string {
	if a == nil {
		return ""
	}
	if a.labelFn == nil {
		return a.label
	}
	return t.semFns(el, a).label
}

// semFnVals are the results of the semantics functions of an element.
type semFnVals struct {
	label string  // AriaLabelFn's
	now   float64 // AriaValueNowFn's
	text  string  // AriaValueTextFn's
}

// semFns returns the results of the semantics functions of primitive el,
// whose ARIA props a have some (hasSemFn): evaluated once per build of the
// semantics tree, all of them with the Values they read tracked together
// (obsSem: a change makes the tree stale, nothing else).
func (t *Tree) semFns(el *Element, a *ariaProps) semFnVals {
	s := &t.sem
	if v, ok := s.fns[el]; ok {
		return v
	}
	mark := t.beginTrack()
	if s.checking {
		defer t.endTrack(mark) // the check reads, it does not subscribe
	}
	var v semFnVals
	func() {
		done := false
		defer func() {
			if !done { // a function panicked: keep the old subscriptions
				t.endTrack(mark)
			}
		}()
		if a.labelFn != nil {
			v.label = a.labelFn()
		}
		if a.nowFn != nil {
			v.now = a.nowFn()
		}
		if a.textFn != nil {
			v.text = a.textFn()
		}
		done = true
	}()
	if s.checking {
		return v
	}
	t.observe(&el.vobs, obsSem, el, nil, t.endTrack(mark))
	if s.fns == nil {
		s.fns = make(map[*Element]semFnVals)
	}
	s.fns[el] = v
	return v
}

// Semantics returns the semantics tree (see the comment at the top of
// semantics.go): roles, names and states follow the Elements as last built
// (a click's rebuild shows at once), styles, bounds and focusability the
// last Frame; elements mounted since the last Frame have no style yet and
// are left out. The tree must not be modified: while no Frame runs and
// nothing is dirty, the same tree is returned again. Call it on the UI
// thread.
func (t *Tree) Semantics() *semantics.Node {
	if t.semPending() {
		t.buildSemantics()
	}
	if checkSemantics {
		t.checkSemanticsTree(t.sem.root)
	}
	return t.sem.root
}

// AriaSnapshot returns the Playwright aria snapshot of the semantics tree
// (semantics.Snapshot): the YAML of `tela inspect --aria`.
func (t *Tree) AriaSnapshot() string { return semantics.Snapshot(t.Semantics()) }

// semID returns the node ID of el, assigning one, and records el under it.
func (t *Tree) semID(el *Element) semantics.NodeID {
	if el.semID == 0 {
		t.sem.next++
		el.semID = t.sem.next
	}
	id := semantics.NodeID(el.semID)
	t.sem.byID[id] = el
	return id
}

// semInherit is what an element's semantics take from its ancestors. It
// is comparable: an element whose inherited state is the one of its last
// build may keep its node (semincr.go).
type semInherit struct {
	hidden   bool // display: none or AriaHidden on an ancestor
	none     bool // display: none on an ancestor: no layout
	disabled bool // Disabled on an ancestor
	ariaDis  semantics.Toggle
	// blockify: the parent lays its children out as flex or grid items,
	// which are block-level whatever their display (CSS).
	blockify bool
	// transformed: an ancestor has a transform, so bounds go through
	// transformUp.
	transformed bool
	// invisible: the parent primitive's visibility is not visible (its
	// texts are hidden).
	invisible bool
	// unnumbered: an ancestor's node is hidden, so the sets below are not
	// numbered (semantics.FillSetPositions skips hidden subtrees).
	unnumbered bool
	// inLabel: a Label is an ancestor (the labels of a field or a button,
	// semTrackVolatile).
	inLabel bool
	// liveRoot is the node ID of the live region around (0 for none).
	liveRoot semantics.NodeID
}

// blockifies reports whether a box with style cs makes its children
// block-level (flex and grid containers).
func blockifies(cs *style.ComputedStyle) bool {
	switch cs.Display {
	case style.DisplayFlex, style.DisplayInlineFlex, style.DisplayGrid, style.DisplayInlineGrid:
		return true
	}
	return false
}

// semElement appends the nodes of el (components: of their children) to
// parent. force builds the whole subtree again (its bounds may have
// moved); otherwise an element that can keep its last node keeps it with
// its subtree (semincr.go).
func (t *Tree) semElement(el *Element, parent *semantics.Node, inh semInherit, force bool) {
	if el.exiting() {
		return // an exiting Presence child is gone for semantics (presence.go)
	}
	if el.sem.flags&semGeom != 0 {
		force = true
	}
	switch el.kind {
	case elemComponent:
		sticky := false
		for _, c := range el.children {
			t.semElement(c, parent, inh, force)
			sticky = sticky || c.sem.flags != 0
		}
		if !t.sem.checking {
			el.sem.flags = 0
			if sticky {
				el.sem.flags = semKids
			}
		}
	case elemText:
		if t.semReuse(el, parent, inh, force) {
			return
		}
		old, oldGen := el.sem.node, el.sem.gen
		var n *semantics.Node
		if el.text != "" {
			n = &semantics.Node{ID: t.semID(el), Kind: semantics.KindText, Text: el.text, Inline: true, LiveRoot: inh.liveRoot}
			if inh.hidden || inh.invisible || el.inert || el.outside == outsideYes {
				n.States |= semantics.StateHidden
			} else {
				n.Bounds = t.textBounds(el, inh.transformed)
			}
		}
		if n = t.semKeep(el, old, oldGen, n, inh, false); n != nil {
			parent.Children = append(parent.Children, n)
		}
	case elemPrimitive:
		if isPortal(el) && el != t.sem.portal {
			return // built after the root's content
		}
		if t.semReuse(el, parent, inh, force) {
			return
		}
		t.semPrimitive(el, parent, inh, force)
	}
}

// semPrimitive appends the node of primitive el and its subtree to
// parent.
func (t *Tree) semPrimitive(el *Element, parent *semantics.Node, inh semInherit, force bool) {
	s := &t.sem
	bx := el.box
	cs := bx.cs
	if cs == nil {
		return // mounted since the last frame
	}
	old, oldGen := el.sem.node, el.sem.gen
	d := el.node.d
	role, explicit := t.semRole(el)
	n := &semantics.Node{ID: t.semID(el), Kind: semantics.KindElement, Role: role, ExplicitRole: explicit}
	if role == "" {
		n.Role = "generic"
	}
	if old != nil && d.virt != nil && d.virt.role == virtList && old.Role != n.Role {
		force = true // the implicit roles of its items follow its role (implicitRole)
	}
	n.Inline = cs.Display == style.DisplayInline && !inh.blockify
	cinh := t.semStates(el, n, inh)
	t.semAriaFields(el, n)
	cinh.liveRoot = semLive(d.aria, n, inh.liveRoot) // aria_live.go
	if el.vobs != nil && !d.aria.hasSemFn() && !s.checking {
		dropObserver(el.vobs) // an AriaLabelFn (or a value's) removed by a build
	}
	if bx.kind == style.KindA {
		n.URL = d.href
	}
	if !cinh.disabled && (d.evMask&clickMask != 0 || clickRole(role)) {
		n.Actions |= semantics.ActionClick
	}
	if !cinh.none {
		n.Actions |= semantics.ActionScrollIntoView
	}
	semWidgetActions(el, n)
	idx := len(parent.Children)
	parent.Children = append(parent.Children, n)

	cinh.transformed = inh.transformed || !paintStyle(bx).Transform.IsIdentity()
	cinh.blockify = blockifies(cs)
	if cs.Display == style.DisplayContents {
		cinh.blockify = inh.blockify // its children are laid out by its parent
	}
	cinh.invisible = cs.Visibility != style.VisibilityVisible
	cinh.unnumbered = inh.unnumbered || n.Has(semantics.StateHidden)
	cinh.inLabel = inh.inLabel || bx.kind == style.KindLabel
	sticky := false
	for _, c := range el.children {
		t.semElement(c, n, cinh, force)
		sticky = sticky || c.sem.flags != 0
	}
	if !cinh.unnumbered && !s.checking {
		// The node is the element's while its items are numbered
		// (semWritable); semKeep decides which one stays.
		el.sem.node, el.sem.gen = n, s.gen
		t.semNumber(n)
	}

	n.Name = semantics.NormalizeWhiteSpace(t.accName(el, role, n.Has(semantics.StateHidden)))
	semFieldStates(el, n, cinh.disabled)
	n.Value = t.semValue(el, role)
	semFieldText(el, n, role)
	if n.Has(semantics.StateHidden) && decorativeIcon(el, inh) {
		n.Decorative = true
	}
	if !n.Has(semantics.StateHidden) || n.Decorative {
		t.semBounds(el, n, cinh.none, cinh.transformed)
	}
	parent.Children[idx] = t.semKeep(el, old, oldGen, n, inh, sticky)
	t.semTrackVolatile(el, inh)
}

// decorativeIcon reports whether primitive el, hidden for accessibility,
// is an icon that only its own aria-hidden hides (the default of h.Icon
// without a name): it is displayed, visible and not inert.
func decorativeIcon(el *Element, inh semInherit) bool {
	a := el.node.d.aria
	return el.box.kind == style.KindSVG && a != nil && a.hidden && !inh.hidden && !inh.none &&
		el.box.cs.Display != style.DisplayNone && el.box.cs.Visibility == style.VisibilityVisible &&
		!el.inert && el.outside != outsideYes
}

// childInherit returns what the children of primitive el inherit from it
// and from its ancestors (inh): hidden, display: none and disabled. The
// caller sets blockify and transformed.
func childInherit(el *Element, inh semInherit) semInherit {
	a := el.node.d.aria
	none := inh.none || el.box.cs.Display == style.DisplayNone
	ci := semInherit{none: none, ariaDis: inh.ariaDis}
	// Inert (Inert, overlay.go) and outside the active modal overlay:
	// hidden, as React Aria's ariaHideOutside does.
	ci.hidden = inh.hidden || none || a != nil && a.hidden || el.inert || el.outside == outsideYes
	ci.disabled = inh.disabled || el.istate&style.StateDisabled != 0
	if a != nil && a.disabled != semantics.ToggleUnset {
		ci.ariaDis = a.disabled
	}
	return ci
}

// semStates sets the states of node n of primitive el that depend on its
// ancestors and on the tree (hidden, disabled, focused, focusable,
// scrollable) with their actions, and returns what el's children inherit
// (childInherit).
func (t *Tree) semStates(el *Element, n *semantics.Node, inh semInherit) semInherit {
	ci := childInherit(el, inh)
	if ci.hidden || el.box.cs.Visibility != style.VisibilityVisible {
		n.States |= semantics.StateHidden
	}
	if ci.disabled || ci.ariaDis == semantics.ToggleTrue {
		n.States |= semantics.StateDisabled
	}
	if t.foc.el == el {
		n.States |= semantics.StateFocused
	}
	if focusTarget(el) && t.focusable(el) {
		n.States |= semantics.StateFocusable
		n.Actions |= semantics.ActionFocus
	}
	if s := el.box.scr; s != nil && !ci.none && (s.userX && s.max.X > 0 || s.userY && s.max.Y > 0) {
		n.States |= semantics.StateScrollable
		n.Actions |= semantics.ActionScroll
	}
	return ci
}

// semAriaFields sets the fields of node n that come from the ARIA props of
// primitive el (aria.go, ariawidget.go), and its level (H1 to H6 keep
// their own).
func (t *Tree) semAriaFields(el *Element, n *semantics.Node) {
	a := el.node.d.aria
	if a != nil {
		if a.required == semantics.ToggleTrue {
			n.States |= semantics.StateRequired
		}
		if a.readOnly == semantics.ToggleTrue {
			n.States |= semantics.StateReadOnly
		}
		if a.busy == semantics.ToggleTrue {
			n.States |= semantics.StateBusy
		}
		n.Checked, n.Pressed, n.Selected, n.Expanded = a.checked, a.pressed, a.selected, a.expanded
		n.Invalid, n.Sort, n.Current = a.invalid, a.sort, a.current
		if style.AriaTokenUnset("sort", a.sort) {
			n.Sort = "" // none
		}
		n.Description = t.semDescription(a)
		n.Range = a.rng
		if a.nowFn != nil {
			if v := t.semFns(el, a).now; v == v { // NaN is no value
				n.Range.Now, n.Range.HasNow = v, true
			}
		}
		t.semWidget(a, n) // ariawidget.go
	}
	if n.Level = headingLevel(el.box.kind); n.Level == 0 && a != nil && a.level >= 1 {
		n.Level = int(a.level)
	}
	if a != nil {
		n.RowCount, n.RowIndex = int(a.rowCount), int(a.rowIndex)
	}
}

// semWidgetActions sets the actions of node n of primitive el that its
// role and states give (after semStates and semAriaFields): increment and
// decrement for a focusable range widget the user moves (slider,
// spinbutton, scrollbar) that is neither disabled nor read-only, expand or
// collapse for an element with AriaExpanded, show menu for one with a
// context-menu handler; none on a disabled element.
func semWidgetActions(el *Element, n *semantics.Node) {
	if n.Has(semantics.StateDisabled) {
		return
	}
	switch n.Role {
	case "slider", "spinbutton", "scrollbar":
		if n.Has(semantics.StateFocusable) && !n.Has(semantics.StateReadOnly) {
			n.Actions |= semantics.ActionIncrement | semantics.ActionDecrement
		}
	}
	switch n.Expanded {
	case semantics.ToggleFalse:
		n.Actions |= semantics.ActionExpand
	case semantics.ToggleTrue:
		n.Actions |= semantics.ActionCollapse
	}
	if el.node.d.evMask&evContextMenu.mask() != 0 {
		n.Actions |= semantics.ActionShowMenu
	}
}

// fieldTextRole reports whether a text field with role exposes its text
// as the node's Value (semValue), so that the node has a selection.
func fieldTextRole(role string) bool {
	switch role {
	case "textbox", "searchbox", "combobox":
		return true
	}
	return false
}

// semDisabled reports whether the tree marks el disabled
// (semantics.StateDisabled, see semStates): it or an ancestor is disabled,
// or the nearest AriaDisabled among them is true. The tree gives such a
// node no increment, decrement, expand, collapse or show menu action.
func semDisabled(el *Element) bool {
	if outermostDisabled(el) != nil {
		return true
	}
	for e := el; e != nil; e = e.styleParent() {
		if a := e.node.d.aria; a != nil && a.disabled != semantics.ToggleUnset {
			return a.disabled == semantics.ToggleTrue
		}
	}
	return false
}

// semFieldText sets the text field data of node n of primitive el, whose
// role is role: the placeholder, the protected state of a password, and
// the caret and selection when the node's Value is the field's text.
func semFieldText(el *Element, n *semantics.Node, role string) {
	f := el.box.fld
	if f == nil {
		return
	}
	n.Placeholder = f.props().placeholder
	if f.typ == fieldPassword {
		n.States |= semantics.StateProtected
	}
	if !fieldTextRole(role) {
		return // its Value is not its text (semValue)
	}
	// The exposed value is the editor's Display (the raw value, or its
	// mask for a password), so the selection's Display offsets index it.
	disp := f.ed.Display()
	sel := f.ed.Selection()
	n.Selection = &semantics.TextSelection{
		Anchor: runeOffset(disp, sel.Anchor.Offset),
		Focus:  runeOffset(disp, sel.Focus.Offset),
	}
	n.Actions |= semantics.ActionSetSelection
}

// runeOffset returns the number of code points of s before byte offset
// off (clamped to s).
func runeOffset(s string, off int) int {
	off = max(0, min(off, len(s)))
	return utf8.RuneCountInString(s[:off])
}

// byteOffset returns the byte offset of code point i of s (clamped to s).
func byteOffset(s string, i int) int {
	if i <= 0 {
		return 0
	}
	for b := range s {
		if i == 0 {
			return b
		}
		i--
	}
	return len(s)
}

// semDescription returns the description of an element with ARIA props a:
// its AriaDescription, else the text of the mounted primitive its
// AriaDescribedBy references.
func (t *Tree) semDescription(a *ariaProps) string {
	if a.desc != "" || a.describedBy == nil {
		return a.desc
	}
	if r := a.describedBy.el; r != nil && r.mounted && r.kind == elemPrimitive {
		return semantics.NormalizeWhiteSpace(t.describedText(r))
	}
	return ""
}

// semFieldStates sets the states and actions of node n that come from the
// text field of primitive el, if any, and its invalid state; disabled is
// el's semantics disability (on el or an ancestor).
func semFieldStates(el *Element, n *semantics.Node, disabled bool) {
	f := el.box.fld
	if f != nil {
		if f.multi {
			n.States |= semantics.StateMultiline
		}
		if f.props().readOnly {
			n.States |= semantics.StateReadOnly
		} else if !disabled {
			n.Actions |= semantics.ActionSetValue
		}
	}
	// Invalid as Chromium's accessibility tree has it (GetInvalidState,
	// ADR-131): the AriaInvalid prop when it is set, true or false, else a
	// text field's own validity (the invalid: variant's state: a type
	// mismatch, a maxlength exceeded by an edit), except for a read-only or
	// disabled field, which HTML bars from constraint validation (the
	// state is never set on one, fieldStates; the test here also covers a
	// disabled semantics ancestor).
	if a := el.node.d.aria; a != nil && a.invalid != semantics.ToggleUnset {
		if a.invalid == semantics.ToggleTrue {
			n.States |= semantics.StateInvalid
		}
	} else if f != nil && el.istate&style.StateInvalid != 0 && !f.props().readOnly && !disabled {
		n.States |= semantics.StateInvalid
	}
}

// semValue returns the value of primitive el, whose role is role: a text
// field's text, a textbox's content, a range widget's value.
func (t *Tree) semValue(el *Element, role string) string {
	switch {
	case el.box.fld != nil && (role == "textbox" || role == "searchbox" || role == "combobox"):
		// A text field with the combobox role (an editable combobox's input)
		// exposes its text, as Playwright prints it: `combobox "Fruit": App`.
		return exposedValue(el.box.fld)
	case role == "textbox" || role == "searchbox":
		return textContent(el)
	case semantics.RangeRole(role):
		return t.rangeValue(el, el.node.d.aria)
	}
	return ""
}

// rangeValue returns the value of range widget el with ARIA props a (nil
// without): its AriaValueText (or AriaValueTextFn's), else its
// AriaValueNow (or AriaValueNowFn's) as JavaScript prints it, else "".
func (t *Tree) rangeValue(el *Element, a *ariaProps) string {
	switch {
	case a == nil:
	case a.textFn != nil:
		return t.semFns(el, a).text
	case a.hasText:
		return a.valueText
	case a.nowFn != nil:
		if v := t.semFns(el, a).now; v == v {
			return jsNumberString(v)
		}
	case a.rng.HasNow:
		return jsNumberString(a.rng.Now)
	}
	return ""
}

// semBounds sets the bounds of node n of primitive el, which is not
// hidden, after its children's (see the comment at the top of
// semantics.go); none and transformed are as in semInherit, el included.
func (t *Tree) semBounds(el *Element, n *semantics.Node, none, transformed bool) {
	bx := el.box
	if t.ownBox(el, none) {
		n.Bounds = gfx.RectFromOriginSize(t.semOrigin(bx), bx.ld.Layout().Size)
		if transformed {
			n.Bounds = t.transformUp(bx, n.Bounds)
		}
		return
	}
	for _, c := range n.Children {
		n.Bounds = n.Bounds.Union(c.Bounds)
	}
	if !none && bx.inPara != nil {
		// An inline element: its fragments (padding included).
		n.Bounds = n.Bounds.Union(t.inlineBounds(el, transformed))
	}
}

// semRole returns the role of el and whether it is explicit (see the
// comment at the top of semantics.go); "" is no role (generic).
func (t *Tree) semRole(el *Element) (string, bool) {
	a := el.node.d.aria
	implicit := t.implicitRole(el)
	if a != nil && a.role != "" {
		if r, _ := semantics.ParseRole(a.role); r != "" {
			if (r == "none" || r == "presentation") && (globalAria(a, implicit) || ariaFocusable(el)) {
				return implicit, false
			}
			return r, true
		}
	}
	return implicit, false
}

// implicitRole returns the HTML-AAM role of primitive el.
func (t *Tree) implicitRole(el *Element) string {
	d := el.node.d
	if v := d.virt; v != nil {
		// The parts of a VirtualList: a list of listitems. Role on the
		// list replaces "list", and its items are listitems only while it
		// is a list (the role of an item's content is its own).
		switch v.role {
		case virtList:
			return "list"
		case virtItem:
			if v.vl != nil && len(v.vl.el.children) > 0 {
				if r, _ := t.semRole(v.vl.el.children[0]); r == "list" {
					return "listitem"
				}
			}
		}
		return ""
	}
	switch el.box.kind {
	case style.KindButton:
		return "button"
	case style.KindH1, style.KindH2, style.KindH3, style.KindH4, style.KindH5, style.KindH6:
		return "heading"
	case style.KindP:
		return "paragraph"
	case style.KindStrong:
		return "strong"
	case style.KindEm:
		return "emphasis"
	case style.KindCode:
		return "code"
	case style.KindA:
		if d.href != "" {
			return "link"
		}
	case style.KindImg:
		if a := d.aria; a != nil && a.hasAlt && a.alt == "" && !globalAria(a, "") && !d.hasTab {
			return "presentation"
		}
		return "img"
	case style.KindSVG:
		return "img"
	case style.KindInput:
		if fieldTypeOf(d) == fieldSearch {
			return "searchbox"
		}
		return "textbox"
	case style.KindTextarea:
		return "textbox"
	case style.KindForm:
		// A form is a landmark only when it has a name (HTML-AAM).
		if a := d.aria; a != nil && (semantics.TrimSpace(t.ariaLabel(el, a)) != "" || len(a.labelledBy) > 0) {
			return "form"
		}
	}
	return ""
}

// exposedValue returns the value a text field exposes: its value (with the
// preedit of a composition), masked for passwords as it is shown.
func exposedValue(f *field) string {
	if f.typ == fieldPassword {
		return f.ed.Display()
	}
	return f.ed.RawValue()
}

// globalAria reports whether a has a global ARIA attribute for role: one
// that keeps a presentational element exposed.
func globalAria(a *ariaProps, role string) bool {
	if a == nil {
		return false
	}
	return a.hiddenSet || (a.hasLabel || len(a.labelledBy) > 0) && !semantics.LabelProhibited(role) ||
		a.w != nil && a.w.controls != nil || // aria-controls (ariawidget.go)
		a.busy != semantics.ToggleUnset // aria-busy, set or not, as Playwright checks the attribute
}

// ariaFocusable reports whether el is focusable for ARIA's presentational
// role conflict: a Button that is not disabled, or an element with a
// TabIndex.
func ariaFocusable(el *Element) bool {
	d := el.node.d
	return d.hasTab || nativeFocusable(el.box.kind) && el.istate&style.StateDisabled == 0
}

// headingLevel returns the level of an H1 to H6 kind, 0 for the others.
func headingLevel(k style.Kind) int {
	switch k {
	case style.KindH1:
		return 1
	case style.KindH2:
		return 2
	case style.KindH3:
		return 3
	case style.KindH4:
		return 4
	case style.KindH5:
		return 5
	case style.KindH6:
		return 6
	}
	return 0
}

// clickRole reports whether role is a widget a click activates.
func clickRole(role string) bool {
	switch role {
	case "button", "link", "checkbox", "switch", "radio", "tab", "menuitem", "menuitemcheckbox",
		"menuitemradio", "option", "treeitem":
		return true
	}
	return false
}

// jsNumberString formats v like JavaScript's String(number), for the
// values of range widgets.
func jsNumberString(v float64) string {
	if a := math.Abs(v); a == 0 || a >= 1e-6 && a < 1e21 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// textContent returns the texts of el's subtree concatenated (the DOM's
// textContent).
func textContent(el *Element) string {
	var b strings.Builder
	var walk func(e *Element)
	walk = func(e *Element) {
		for _, c := range e.children {
			if c.kind == elemText {
				b.WriteString(c.text)
			} else {
				walk(c)
			}
		}
	}
	walk(el)
	return b.String()
}

// ---------------------------------------------------------------------
// Accessible names (AccName 1.2 after Playwright's
// getTextAlternativeInternal).

// altCtx is the state of one name computation.
type altCtx struct {
	includeHidden bool
	visited       map[*Element]bool
}

// altTarget says where an element is in a name computation.
type altTarget uint8

const (
	altOther      altTarget = iota // inside a labelledby traversal
	altSelf                        // the element being named
	altDescendant                  // inside the element being named
)

// altOpts are the options of one step of a name computation.
type altOpts struct {
	target altTarget
	// inLB: inside the traversal of an AriaLabelledBy reference; lbHidden:
	// that reference is hidden, so hidden content counts.
	inLB, lbHidden bool
}

// accName returns the accessible name of el, whose role is role (""
// without one), before white space is collapsed. includeHidden counts
// hidden content (for hidden nodes).
func (t *Tree) accName(el *Element, role string, includeHidden bool) string {
	if semantics.NameProhibited(role) {
		return ""
	}
	c := &t.sem.alt
	c.includeHidden = includeHidden
	if c.visited == nil {
		c.visited = map[*Element]bool{}
	} else {
		clear(c.visited)
	}
	return t.textAlt(el, c, altOpts{target: altSelf})
}

// describedText returns the text of the element an AriaDescribedBy
// references, computed like the text of an AriaLabelledBy reference.
func (t *Tree) describedText(r *Element) string {
	c := &t.sem.alt
	c.includeHidden = false
	if c.visited == nil {
		c.visited = map[*Element]bool{}
	} else {
		clear(c.visited)
	}
	return t.textAlt(r, c, altOpts{inLB: true, lbHidden: t.hiddenForAria(r, false)})
}

// textAlt computes the text alternative of primitive el (AccName's steps
// 2A to 2F). Each step's helper reports whether it decided the result.
func (t *Tree) textAlt(el *Element, c *altCtx, o altOpts) string {
	if c.visited[el] {
		return ""
	}
	// 2A: hidden content counts only in the traversal of a hidden
	// reference. The element being named was checked by its caller and a
	// reference before its traversal, so only the element itself is
	// checked (its ancestors up to there are not hidden).
	if !c.includeHidden && !o.lbHidden && o.target != altSelf && t.hiddenForAria(el, true) {
		c.visited[el] = true
		return ""
	}
	a := el.node.d.aria
	if s, ok := t.altLabelledBy(a, c, o); ok {
		return s
	}
	role, _ := t.semRole(el)
	if s, ok := t.altEmbeddedControl(el, a, role, c, o); ok {
		return s
	}
	// 2C: aria-label (or AriaLabelFn's).
	if l := t.ariaLabel(el, a); semantics.TrimSpace(l) != "" {
		c.visited[el] = true
		return l
	}
	if s, ok := t.altNative(el, a, role, c, o); ok {
		return s
	}
	if s, ok := t.altContent(el, role, c, o); ok {
		return s
	}
	c.visited[el] = true
	return ""
}

// child returns the options of the steps inside the element o is for:
// the content of the element being named is altDescendant.
func (o altOpts) child() altOpts {
	if o.target == altSelf {
		o.target = altDescendant
	}
	return o
}

// altLabelledBy is step 2B: aria-labelledby, once; each mounted reference
// gives its text and the texts are joined by a space. a is the element's
// ARIA props (nil without).
func (t *Tree) altLabelledBy(a *ariaProps, c *altCtx, o altOpts) (string, bool) {
	if o.inLB || a == nil || len(a.labelledBy) == 0 {
		return "", false
	}
	var parts []string
	for _, ref := range a.labelledBy {
		r := ref.el
		if r == nil || !r.mounted || r.kind != elemPrimitive {
			continue
		}
		if s := t.textAlt(r, c, altOpts{inLB: true, lbHidden: t.hiddenForAria(r, false)}); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

// altEmbeddedControl is step 2E: controls embedded in a label or in
// content give their value. An element that names itself
// (labelsItself) is not embedded in its own label.
func (t *Tree) altEmbeddedControl(el *Element, a *ariaProps, role string, c *altCtx, o altOpts) (string, bool) {
	if !o.inLB && o.target != altDescendant || labelsItself(el, a) {
		return "", false
	}
	switch {
	case role == "textbox" || role == "searchbox":
		c.visited[el] = true
		if f := el.box.fld; f != nil {
			return exposedValue(f), true
		}
		return textContent(el), true
	case role == "combobox" || role == "listbox":
		c.visited[el] = true
		return t.selectedOptions(el, role, c, o.child()), true
	case semantics.RangeRole(role):
		c.visited[el] = true
		return t.rangeValue(el, a), true
	case role == "menu":
		c.visited[el] = true
		return "", true
	}
	return "", false
}

// labelsItself reports whether el, with ARIA props a, is one of its own
// AriaLabelledBy references, for the embedded-control rule.
func labelsItself(el *Element, a *ariaProps) bool {
	if a == nil {
		return false
	}
	for _, ref := range a.labelledBy {
		if ref.el == el {
			return true
		}
	}
	return false
}

// altNative is step 2D: the native text alternative (HTML-AAM) of a text
// field, of a button named for itself and of an image.
func (t *Tree) altNative(el *Element, a *ariaProps, role string, c *altCtx, o altOpts) (string, bool) {
	// 2D: the native text alternative of a text field (HTML-AAM): its
	// labels' content (without the field itself), then its placeholder.
	if f := el.box.fld; f != nil && !o.inLB {
		c.visited[el] = true
		if s := t.labelsText(el, c); s != "" {
			return s, true
		}
		return f.props().placeholder, true
	}
	// 2D: a button named for itself takes its labels' content first, then
	// its own content (HTML-AAM's button rule; buttons are labelable).
	if el.box.kind == style.KindButton && !o.inLB && o.target == altSelf {
		c.visited[el] = true
		s := t.labelsText(el, c)
		delete(c.visited, el) // step 2F reads the content (altContent)
		if s != "" {
			c.visited[el] = true
			return s, true
		}
	}
	// 2D: the native text alternative of an image.
	if role != "presentation" && role != "none" && el.box.kind == style.KindImg {
		c.visited[el] = true
		if a != nil && semantics.TrimSpace(a.alt) != "" {
			return a.alt, true
		}
		return "", true
	}
	return "", false
}

// altContent is step 2F: the content, for the roles named from it and
// inside a labelledby traversal.
func (t *Tree) altContent(el *Element, role string, c *altCtx, o altOpts) (string, bool) {
	if !semantics.NameFromContent(role, o.target == altDescendant) && !o.inLB {
		return "", false
	}
	c.visited[el] = true
	var b strings.Builder
	t.innerText(&b, el, c, o.child())
	s := b.String()
	if o.target == altSelf && semantics.TrimSpace(s) != "" || o.target != altSelf && s != "" {
		return s, true
	}
	return "", false
}

// innerText appends the text alternatives of el's children (texts and
// primitives, through components): block-level children are surrounded
// by spaces.
func (t *Tree) innerText(b *strings.Builder, el *Element, c *altCtx, o altOpts) {
	for _, ch := range el.children {
		switch ch.kind {
		case elemText:
			b.WriteString(ch.text)
		case elemComponent:
			t.innerText(b, ch, c, o)
		case elemPrimitive:
			tok := t.textAlt(ch, c, o)
			if inlineDisplay(ch) {
				b.WriteString(tok)
			} else {
				b.WriteByte(' ')
				b.WriteString(tok)
				b.WriteByte(' ')
			}
		}
	}
}

// selectedOptions returns the names of the selected options of a listbox
// (or of the listbox inside a combobox), joined by spaces.
func (t *Tree) selectedOptions(el *Element, role string, c *altCtx, o altOpts) string {
	var parts []string
	var walk func(e *Element, inList bool)
	walk = func(e *Element, inList bool) {
		for _, ch := range e.children {
			if ch.kind == elemText {
				continue
			}
			in := inList
			if ch.kind == elemPrimitive {
				r, _ := t.semRole(ch)
				if !in && r == "listbox" {
					in = true
				}
				if in && r == "option" && ch.node.d.aria != nil && ch.node.d.aria.selected == semantics.ToggleTrue {
					parts = append(parts, t.textAlt(ch, c, o))
					continue
				}
			}
			walk(ch, in)
		}
	}
	walk(el, role == "listbox")
	return strings.Join(parts, " ")
}

// hiddenForAria reports whether el is hidden for accessibility: not
// styled, display: none or AriaHidden on it (or, unless only, an
// ancestor), or visibility other than visible.
func (t *Tree) hiddenForAria(el *Element, only bool) bool {
	cs := el.box.cs
	if cs == nil || cs.Visibility != style.VisibilityVisible {
		return true
	}
	for e := el; e != nil; e = e.styleParent() {
		ecs := e.box.cs
		if ecs == nil || ecs.Display == style.DisplayNone && e.parent != nil {
			return true
		}
		if a := e.node.d.aria; a != nil && a.hidden {
			return true
		}
		if only {
			break
		}
	}
	return false
}

// inlineDisplay reports whether primitive el is laid out inline: display
// inline, not blockified by a flex or grid parent.
func inlineDisplay(el *Element) bool {
	cs := el.box.cs
	if cs == nil || cs.Display != style.DisplayInline {
		return false
	}
	p := el.styleParent()
	return p == nil || p.box.cs == nil || !blockifies(p.box.cs)
}

// ---------------------------------------------------------------------
// Bounds.

// ownBox reports whether el's box is in the box tree (not inline content
// of a paragraph, not display: contents, displayed); none reports a
// display: none ancestor or el itself.
func (t *Tree) ownBox(el *Element, none bool) bool {
	bx := el.box
	if none || bx.cs == nil || bx.cs.Display == style.DisplayContents || bx.inPara != nil || !bx.built {
		return false
	}
	return bx.parent != nil || bx == t.rootBox
}

// semOrigin returns the top-left corner of bx's border box in window
// coordinates before transforms: its layout position moved by the scroll
// offsets painted last of the scroll containers on its containing-block
// chain (fixed boxes do not move with the viewport), as paint.go's place
// computes it.
func (t *Tree) semOrigin(bx *Box) gfx.Point {
	var o gfx.Point
	for b := bx; b != nil; b = b.parent {
		o = o.Add(b.ld.Layout().Location)
	}
	for b := bx; ; {
		o = o.Add(ownShift(b)) // a stuck box (sticky.go), a floating one (floating.go)
		cb := containingBlock(b)
		if cb == nil {
			break
		}
		if cs := paintStyle(b); cs != nil && cs.Position == style.PositionFixed && cb == t.rootBox {
			break
		}
		if cb.scr != nil {
			o = o.Sub(cb.scr.soff)
		}
		b = cb
	}
	return o
}

// paintStyle returns the paint-phase style of bx (with the deferred props
// of its last paint), or its computed style before it was painted.
func paintStyle(bx *Box) *style.ComputedStyle {
	if bx.pcs != nil {
		return bx.pcs
	}
	return bx.cs
}

// boxWindowRect returns the bounding rectangle of bx's transformed border
// box in window coordinates.
func (t *Tree) boxWindowRect(bx *Box) gfx.Rect {
	r := gfx.RectFromOriginSize(t.semOrigin(bx), bx.ld.Layout().Size)
	return t.transformUp(bx, r)
}

// transformUp maps r, in the untransformed window coordinates of bx's
// subtree, through the transforms of bx and its ancestors (innermost
// first, as paint nests them; the root's is not painted).
func (t *Tree) transformUp(bx *Box, r gfx.Rect) gfx.Rect {
	for b := bx; b != nil && b != t.rootBox; b = b.parent {
		cs := paintStyle(b)
		if cs == nil || cs.Transform.IsIdentity() {
			continue
		}
		box := gfx.RectFromOriginSize(t.semOrigin(b), b.ld.Layout().Size)
		r = cs.Transform.Matrix(box).TransformRect(r)
	}
	return r
}

// textBounds returns the bounds of text element el: the union of the
// selection rectangles of its text in the paragraph that lays it out, in
// window coordinates; zero when it is not laid out. transformed reports
// that a box around it may have a transform.
func (t *Tree) textBounds(el *Element, transformed bool) gfx.Rect {
	p := el.paraBox
	if !semLaidOut(p) {
		return gfx.Rect{}
	}
	off, end := int(el.paraOff), int(el.paraOff)+len(el.text)
	if end > len(p.para.text) || p.para.text[off:end] != el.text {
		return gfx.Rect{} // a stale location: the text moved
	}
	return t.paraBounds(p, p.para.lay.SelectionRects(text.NewSelection(text.Cursor{Offset: off}, text.Cursor{Offset: end}), t.sem.rects[:0]), transformed)
}

// elemBounds returns the bounds of el as the semantics tree gives them.
func (t *Tree) elemBounds(el *Element) gfx.Rect {
	switch el.kind {
	case elemText:
		return t.textBounds(el, true)
	case elemPrimitive:
		if t.ownBox(el, false) && t.inTree(el.box) {
			return t.boxWindowRect(el.box)
		}
	}
	var u gfx.Rect
	for _, c := range el.children {
		if c.kind == elemPrimitive && (c.box.cs == nil || c.box.cs.Display == style.DisplayNone) {
			continue
		}
		u = u.Union(t.elemBounds(c))
	}
	if el.kind == elemPrimitive && el.box.inPara != nil {
		u = u.Union(t.inlineBounds(el, true))
	}
	return u
}

// inlineBounds returns the union of the border boxes of the fragments of
// inline element el (inline.go) in window coordinates, zero when it has
// none; transformed reports that a box around it may have a transform.
func (t *Tree) inlineBounds(el *Element, transformed bool) gfx.Rect {
	p := el.box.inPara
	if !semLaidOut(p) {
		return gfx.Rect{}
	}
	return t.paraBounds(p, p.para.inlineRects(el, t.sem.rects[:0]), transformed)
}

// semLaidOut reports whether paragraph box p (nil for none) is laid out
// for the semantics tree: its owner is mounted and its text has a layout.
func semLaidOut(p *Box) bool {
	return p != nil && p.owner != nil && p.owner.mounted && p.para != nil && p.para.lay != nil
}

// paraBounds returns the union of rects (in the content box of paragraph
// box p, in t.sem.rects, which it gets back emptied for reuse) in window
// coordinates, zero when it is empty; transformed reports that a box
// around p may have a transform.
func (t *Tree) paraBounds(p *Box, rects []gfx.Rect, transformed bool) gfx.Rect {
	var u gfx.Rect
	for _, r := range rects {
		u = u.Union(r)
	}
	clear(rects)
	t.sem.rects = rects[:0]
	if u.Empty() {
		return gfx.Rect{}
	}
	l := p.ld.Layout()
	o := t.semOrigin(p).Add(gfx.Point{X: l.Border.Left + l.Padding.Left, Y: l.Border.Top + l.Padding.Top})
	if p.scr != nil {
		o = o.Sub(p.scr.soff) // a paragraph that scrolls its own text
	}
	u = u.Translate(o.X, o.Y)
	if transformed {
		u = t.transformUp(p, u)
	}
	return u
}

// ---------------------------------------------------------------------
// Actions (internal API of telatest and tela mcp).

// Errors of the semantics actions.
var (
	// ErrNodeGone: the node is not in the last semantics tree, or its
	// element was unmounted since.
	ErrNodeGone = errors.New("core: no such semantics node (unknown, or its element was unmounted)")
	// ErrNodeDisabled: the element (or an ancestor) is Disabled.
	ErrNodeDisabled = errors.New("core: the element is disabled")
	// ErrNotFocusable: the element cannot take the focus.
	ErrNotFocusable = errors.New("core: the element is not focusable")
	// ErrNoAction: the element does not support the action (the node
	// lacks its semantics.Action bit: a decrement of a button, an expand of
	// an element without AriaExpanded, a scroll of one that does not
	// scroll).
	ErrNoAction = errors.New("core: the element does not support the action")
)

// liveElement returns the element of node id in the last semantics tree,
// or nil when the node is unknown or its element was unmounted since: an
// unmounted element keeps its parent, so a lookup must check this before
// climbing from it.
func (t *Tree) liveElement(id semantics.NodeID) *Element {
	if el := t.sem.byID[id]; el != nil && el.mounted {
		return el
	}
	return nil
}

// actionElement returns the primitive that acts for node id: its element,
// or a text's parent element.
func (t *Tree) actionElement(id semantics.NodeID) (*Element, error) {
	el := t.liveElement(id)
	if el != nil && el.kind == elemText {
		el = el.styleParent()
	}
	if el == nil || !el.mounted || el.kind != elemPrimitive {
		return nil, fmt.Errorf("%w: %d", ErrNodeGone, id)
	}
	return el, nil
}

// ClickNode clicks the element of node id like HTML's element.click():
// OnClick runs on it and bubbles (a MouseEvent with the primary button,
// a click count of 1 and the center of the element's bounds as its
// position), then the queued updates are applied and components rebuilt,
// as after a real click. There are no pointer events, no :active state
// and no focus change. A disabled element is not clicked
// (ErrNodeDisabled).
func (t *Tree) ClickNode(id semantics.NodeID) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	if outermostDisabled(el) != nil {
		return fmt.Errorf("%w: %s", ErrNodeDisabled, t.describe(el))
	}
	me := MouseEvent{Button: MouseButtonLeft, ClickCount: 1}
	if r := t.elemBounds(el); !r.Empty() {
		c := r.Center()
		me.X, me.Y = c.X, c.Y
	}
	t.dispatchClick(el, me)
	return nil
}

// FocusNode focuses the element of node id (ElementRef.Focus): OnBlur and
// OnFocus run. An element that cannot take the focus gives
// ErrNotFocusable.
func (t *Tree) FocusNode(id semantics.NodeID) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	if !t.focusable(el) {
		return fmt.Errorf("%w: %s", ErrNotFocusable, t.describe(el))
	}
	t.focusElement(el)
	return nil
}

// ScrollNodeIntoView scrolls every scroll container around the element of
// node id so that it is visible (ElementRef.ScrollIntoView): at once when
// the layout is current, else after the next layout.
func (t *Tree) ScrollNodeIntoView(id semantics.NodeID) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	t.intoView(el)
	return nil
}

// IncrementNode and DecrementNode step the range widget of node id (a
// slider, spinbutton or scrollbar) the way Chromium does for ARIA range
// widgets: they focus it (ElementRef.Focus) and press and release an arrow
// key through the normal key path (HandleKey), so its key handlers move it
// as for a user. The arrow is ArrowUp or ArrowDown for a vertical widget
// (AriaOrientation("vertical"), a spinbutton, a scrollbar without
// orientation) and ArrowRight or ArrowLeft for a horizontal one (a slider
// without orientation), mirrored in a right-to-left element, where package
// aria's sliders decrease with ArrowRight. Another role gives ErrNoAction,
// as does AriaReadOnly(true); a disabled element (natively or with
// AriaDisabled, semDisabled) ErrNodeDisabled; one that cannot take the
// focus ErrNotFocusable.
func (t *Tree) IncrementNode(id semantics.NodeID) error { return t.stepNode(id, true) }

// DecrementNode: see IncrementNode.
func (t *Tree) DecrementNode(id semantics.NodeID) error { return t.stepNode(id, false) }

func (t *Tree) stepNode(id semantics.NodeID, up bool) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	role, _ := t.semRole(el)
	a := el.node.d.aria
	switch {
	case role != "slider" && role != "spinbutton" && role != "scrollbar":
		return fmt.Errorf("%w: %s (role %q) is not a slider, spinbutton or scrollbar", ErrNoAction, t.describe(el), role)
	case a != nil && a.readOnly == semantics.ToggleTrue:
		return fmt.Errorf("%w: %s is read-only (AriaReadOnly)", ErrNoAction, t.describe(el))
	case semDisabled(el):
		return fmt.Errorf("%w: %s", ErrNodeDisabled, t.describe(el))
	case !t.focusable(el):
		return fmt.Errorf("%w: %s", ErrNotFocusable, t.describe(el))
	}
	vertical := role != "slider"
	if a != nil && a.w != nil && a.w.orientation != "" {
		vertical = a.w.orientation == "vertical"
	}
	key, code := platform.KeyArrowDown, platform.CodeArrowDown
	switch {
	case vertical && up:
		key, code = platform.KeyArrowUp, platform.CodeArrowUp
	case !vertical:
		right := up
		if el.box.cs.Text.Direction == text.DirectionRTL {
			right = !right
		}
		key, code = platform.KeyArrowLeft, platform.CodeArrowLeft
		if right {
			key, code = platform.KeyArrowRight, platform.CodeArrowRight
		}
	}
	t.pressKey(el, key, code)
	return nil
}

// pressKey focuses el, unless it has the focus, and presses and releases
// a key through HandleKey, as a synthesized key press of an accessibility
// action.
func (t *Tree) pressKey(el *Element, key platform.Key, code platform.KeyCode) {
	if t.foc.el != el {
		t.focusElement(el)
	}
	t.HandleKey(platform.KeyEvent{Action: platform.KeyPress, Key: key, Code: code, Time: t.scr.now})
	t.HandleKey(platform.KeyEvent{Action: platform.KeyRelease, Key: key, Code: code, Time: t.scr.now})
}

// ExpandNode and CollapseNode expand or collapse the element of node id,
// one with AriaExpanded, by clicking it (ClickNode), as Chromium does,
// and only when its state would change: expanding an expanded element
// does nothing. An element without AriaExpanded gives ErrNoAction, a
// disabled one (natively or with AriaDisabled) ErrNodeDisabled.
func (t *Tree) ExpandNode(id semantics.NodeID) error { return t.setExpanded(id, true) }

// CollapseNode: see ExpandNode.
func (t *Tree) CollapseNode(id semantics.NodeID) error { return t.setExpanded(id, false) }

func (t *Tree) setExpanded(id semantics.NodeID, expand bool) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	a := el.node.d.aria
	if a == nil || a.expanded == semantics.ToggleUnset {
		return fmt.Errorf("%w: %s has no AriaExpanded", ErrNoAction, t.describe(el))
	}
	if semDisabled(el) {
		return fmt.Errorf("%w: %s", ErrNodeDisabled, t.describe(el))
	}
	if (a.expanded == semantics.ToggleTrue) == expand {
		return nil
	}
	return t.ClickNode(semantics.NodeID(el.semID))
}

// ShowMenuNode opens the context menu of node id as the ContextMenu key
// does on a focused element in a browser: it focuses the element when it
// can take the focus, dispatches a keydown and a keyup of the ContextMenu
// key to it (they bubble), and, unless a handler prevented the keydown's
// default (package aria's UseContextMenuTrigger opens its menu there, with
// the first item focused), fires OnContextMenu on it with the secondary
// button at the center of its bounds. An element without a context-menu
// handler gives ErrNoAction, a disabled one (natively or with
// AriaDisabled) ErrNodeDisabled.
func (t *Tree) ShowMenuNode(id semantics.NodeID) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	if el.node.d.evMask&evContextMenu.mask() == 0 {
		return fmt.Errorf("%w: %s has no OnContextMenu", ErrNoAction, t.describe(el))
	}
	if semDisabled(el) {
		return fmt.Errorf("%w: %s", ErrNodeDisabled, t.describe(el))
	}
	if t.foc.el != el && t.focusable(el) {
		t.focusElement(el)
	}
	ke := KeyboardEvent{Key: KeyValue(platform.KeyContextMenu), Code: CodeValue(platform.CodeContextMenu.String())}
	st := fire(t, evKeyDown, el, ke, true)
	t.flushDiscrete()
	prevented := st.defaultPrevented()
	if el.mounted {
		fire(t, evKeyUp, el, ke, true)
		t.flushDiscrete()
	}
	if prevented || !el.mounted {
		return nil
	}
	me := MouseEvent{Button: MouseButtonRight}
	if r := t.elemBounds(el); !r.Empty() {
		c := r.Center()
		me.X, me.Y = c.X, c.Y
	}
	fire(t, evContextMenu, el, me, true)
	t.flushDiscrete()
	return nil
}

// SetTextSelectionNode sets the caret and selection of the text field of
// node id to anchor and focus, in code points of the node's Value (as in
// semantics.TextSelection; clamped to it and moved to grapheme cluster
// boundaries), without focusing it, as HTML's setSelectionRange with a
// direction, and fires OnSelect when it changed. It does nothing during
// an IME composition. An element that is not a text field, or a field
// whose role does not expose its text as the Value (a spinbutton: its
// Value is the number, fieldTextRole), gives ErrNoAction.
func (t *Tree) SetTextSelectionNode(id semantics.NodeID, anchor, focus int) error {
	el, err := t.actionElement(id)
	if err != nil {
		return err
	}
	f := textField(el)
	if f == nil {
		return fmt.Errorf("%w: %s is not a text field", ErrNoAction, t.describe(el))
	}
	if role, _ := t.semRole(el); !fieldTextRole(role) {
		return fmt.Errorf("%w: %s (role %q) does not expose its text", ErrNoAction, t.describe(el), role)
	}
	if f.ed.Composing() {
		return nil
	}
	disp := f.ed.Display()
	va := f.ed.DisplayToValue(byteOffset(disp, anchor))
	vf := f.ed.DisplayToValue(byteOffset(disp, focus))
	t.fieldChanged(f, f.ed.Select(va, vf))
	t.fieldSelect(f, false)
	return nil
}

// ScrollNodeBy scrolls the scroll container of node id (the root's node
// scrolls the window's content) by dx, dy logical pixels from its current
// offsets, as ElementRef.ScrollBy: positive dx scrolls toward the right
// and positive dy down, in either direction. A node that does not scroll
// (no semantics.ActionScroll) gives ErrNoAction.
func (t *Tree) ScrollNodeBy(id semantics.NodeID, dx, dy float32) error {
	el := t.liveElement(id)
	if el == nil {
		return fmt.Errorf("%w: %d", ErrNodeGone, id)
	}
	var s *scroller
	if el == t.root {
		s = t.rootBox.scr
	} else if el.kind == elemPrimitive {
		s = el.box.scr
	}
	if s == nil || !(s.userX && s.max.X > 0 || s.userY && s.max.Y > 0) {
		return fmt.Errorf("%w: %s does not scroll", ErrNoAction, t.describe(el))
	}
	t.scriptedRequest(scrollReq{s: s, p: s.flip(gfx.Point{X: dx, Y: dy}), by: true})
	return nil
}

// labelsText joins the text alternatives of el's labels (Label elements
// with For(el's id) and the Label wrapping it), or returns "".
func (t *Tree) labelsText(el *Element, c *altCtx) string {
	var b strings.Builder
	for _, lab := range t.labelsOf(el) {
		if s := t.textAlt(lab, c, altOpts{target: altDescendant, inLB: true}); semantics.TrimSpace(s) != "" {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(s)
		}
	}
	return b.String()
}
