package core

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
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
// The tree is kept and handed out again while no Frame ran and nothing is
// dirty. A full build of a 10k-element tree takes a few milliseconds
// (BenchmarkSemantics10k); incremental updates (§8's needsSemantics marks)
// wait for a consumer that needs the tree every frame, the OS
// accessibility backends of phase 8.
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
	// frame is t.frame when root was built, valid that nothing was dirty
	// then.
	frame uint64
	valid bool
	// byID maps the node IDs of root to their elements.
	byID map[semantics.NodeID]*Element
	next uint64
	// Scratch state of the name computation and of text bounds.
	alt   altCtx
	rects []gfx.Rect
	// portal is the portal being built (its content follows the root's,
	// portal.go); others met inside the tree are skipped.
	portal *Element
}

// Semantics returns the semantics tree (see the comment at the top of
// semantics.go): roles, names and states follow the Elements as last built
// (a click's rebuild shows at once), styles, bounds and focusability the
// last Frame; elements mounted since the last Frame have no style yet and
// are left out. The tree must not be modified: while no Frame runs and
// nothing is dirty, the same tree is returned again. Call it on the UI
// thread.
func (t *Tree) Semantics() *semantics.Node {
	s := &t.sem
	if s.valid && s.root != nil && s.frame == t.frame && !t.Dirty() {
		return s.root
	}
	if s.byID == nil {
		s.byID = make(map[semantics.NodeID]*Element)
	} else {
		clear(s.byID)
	}
	root := &semantics.Node{Kind: semantics.KindRoot}
	if t.root != nil && t.rootBox.cs != nil {
		root.ID = t.semID(t.root)
		root.Bounds = gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}
		if sc := t.rootBox.scr; sc != nil && (sc.userX && sc.max.X > 0 || sc.userY && sc.max.Y > 0) {
			root.States |= semantics.StateScrollable
			root.Actions |= semantics.ActionScroll
		}
		inh := semInherit{blockify: blockifies(t.rootBox.cs)}
		s.portal = nil
		for _, c := range t.root.children {
			t.semElement(c, root, inh)
		}
		// The portals' content after the root's, as if appended to the
		// body (portal.go).
		for _, p := range t.portalsIn(t.root, nil) {
			s.portal = p
			t.semElement(p, root, semInherit{})
		}
		s.portal = nil
	}
	s.root, s.frame, s.valid = root, t.frame, !t.Dirty()
	return root
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

// semInherit is what an element's semantics take from its ancestors.
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
// parent.
func (t *Tree) semElement(el *Element, parent *semantics.Node, inh semInherit) {
	if el.exiting() {
		return // an exiting Presence child is gone for semantics (presence.go)
	}
	switch el.kind {
	case elemComponent:
		for _, c := range el.children {
			t.semElement(c, parent, inh)
		}
	case elemText:
		if el.text == "" {
			return
		}
		n := &semantics.Node{ID: t.semID(el), Kind: semantics.KindText, Text: el.text, Inline: true}
		sp := el.styleParent()
		if inh.hidden || el.inert || el.outside == outsideYes || sp == nil || sp.box.cs == nil || sp.box.cs.Visibility != style.VisibilityVisible {
			n.States |= semantics.StateHidden
		} else {
			n.Bounds = t.textBounds(el, inh.transformed)
		}
		parent.Children = append(parent.Children, n)
	case elemPrimitive:
		if isPortal(el) && el != t.sem.portal {
			return // built after the root's content
		}
		t.semPrimitive(el, parent, inh)
	}
}

// semPrimitive appends the node of primitive el and its subtree to
// parent.
func (t *Tree) semPrimitive(el *Element, parent *semantics.Node, inh semInherit) {
	bx := el.box
	cs := bx.cs
	if cs == nil {
		return // mounted since the last frame
	}
	d := el.node.d
	a := d.aria
	none := inh.none || cs.Display == style.DisplayNone
	// Inert (Inert, overlay.go) and outside the active modal overlay:
	// hidden, as React Aria's ariaHideOutside does.
	hidden := inh.hidden || none || a != nil && a.hidden || el.inert || el.outside == outsideYes
	role, explicit := t.semRole(el)
	n := &semantics.Node{ID: t.semID(el), Kind: semantics.KindElement, Role: role, ExplicitRole: explicit}
	if role == "" {
		n.Role = "generic"
	}
	n.Inline = cs.Display == style.DisplayInline && !inh.blockify
	if hidden || cs.Visibility != style.VisibilityVisible {
		n.States |= semantics.StateHidden
	}
	disabled := inh.disabled || el.istate&style.StateDisabled != 0
	ariaDis := inh.ariaDis
	if a != nil && a.disabled != semantics.ToggleUnset {
		ariaDis = a.disabled
	}
	if disabled || ariaDis == semantics.ToggleTrue {
		n.States |= semantics.StateDisabled
	}
	if t.foc.el == el {
		n.States |= semantics.StateFocused
	}
	if focusTarget(el) && t.focusable(el) {
		n.States |= semantics.StateFocusable
		n.Actions |= semantics.ActionFocus
	}
	if s := bx.scr; s != nil && !none && (s.userX && s.max.X > 0 || s.userY && s.max.Y > 0) {
		n.States |= semantics.StateScrollable
		n.Actions |= semantics.ActionScroll
	}
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
		n.Description = a.desc
		if n.Description == "" && a.describedBy != nil {
			if r := a.describedBy.el; r != nil && r.mounted && r.kind == elemPrimitive {
				n.Description = semantics.NormalizeWhiteSpace(t.describedText(r))
			}
		}
		n.Range = a.rng
		t.semWidget(a, n) // ariawidget.go
	}
	if n.Level = headingLevel(bx.kind); n.Level == 0 && a != nil && a.level >= 1 {
		n.Level = int(a.level)
	}
	if bx.kind == style.KindA {
		n.URL = d.href
	}
	if !disabled && (d.evMask&clickMask != 0 || clickRole(role)) {
		n.Actions |= semantics.ActionClick
	}
	if !none {
		n.Actions |= semantics.ActionScrollIntoView
	}
	parent.Children = append(parent.Children, n)

	transformed := inh.transformed || !paintStyle(bx).Transform.IsIdentity()
	cinh := semInherit{hidden: hidden, none: none, disabled: disabled, ariaDis: ariaDis, blockify: blockifies(cs), transformed: transformed}
	if cs.Display == style.DisplayContents {
		cinh.blockify = inh.blockify // its children are laid out by its parent
	}
	for _, c := range el.children {
		t.semElement(c, n, cinh)
	}

	n.Name = semantics.NormalizeWhiteSpace(t.accName(el, role, n.Has(semantics.StateHidden)))
	if f := bx.fld; f != nil {
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
	if a != nil && a.invalid != semantics.ToggleUnset {
		if a.invalid == semantics.ToggleTrue {
			n.States |= semantics.StateInvalid
		}
	} else if f := bx.fld; f != nil && el.istate&style.StateInvalid != 0 && !f.props().readOnly && !disabled {
		n.States |= semantics.StateInvalid
	}
	switch {
	case bx.fld != nil && (role == "textbox" || role == "searchbox" || role == "combobox"):
		// A text field with the combobox role (an editable combobox's input)
		// exposes its text, as Playwright prints it: `combobox "Fruit": App`.
		n.Value = exposedValue(bx.fld)
	case role == "textbox" || role == "searchbox":
		n.Value = textContent(el)
	case semantics.RangeRole(role) && a != nil:
		if a.hasText {
			n.Value = a.valueText
		} else if a.rng.HasNow {
			n.Value = jsNumberString(a.rng.Now)
		}
	}
	if !n.Has(semantics.StateHidden) {
		if t.ownBox(el, none) {
			n.Bounds = gfx.RectFromOriginSize(t.semOrigin(bx), bx.ld.Layout().Size)
			if transformed {
				n.Bounds = t.transformUp(bx, n.Bounds)
			}
		} else {
			for _, c := range n.Children {
				n.Bounds = n.Bounds.Union(c.Bounds)
			}
			if !none && bx.inPara != nil {
				// An inline element: its fragments (padding included).
				n.Bounds = n.Bounds.Union(t.inlineBounds(el, transformed))
			}
		}
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
		if a := d.aria; a != nil && (semantics.TrimSpace(a.label) != "" || len(a.labelledBy) > 0) {
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
	return d.hasTab || (el.box.kind == style.KindButton || el.box.kind.IsTextField()) && el.istate&style.StateDisabled == 0
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
// 2A to 2F).
func (t *Tree) textAlt(el *Element, c *altCtx, o altOpts) string {
	if c.visited[el] {
		return ""
	}
	child := o
	if o.target == altSelf {
		child.target = altDescendant
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
	var lb *Element // the element itself when it names itself, for the embedded-control rule below
	if a != nil {
		for _, ref := range a.labelledBy {
			if ref.el == el {
				lb = el
			}
		}
	}
	// 2B: aria-labelledby, once; each mounted reference gives its text and
	// the texts are joined by a space.
	if !o.inLB && a != nil && len(a.labelledBy) > 0 {
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
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	role, _ := t.semRole(el)
	// 2E: controls embedded in a label or in content give their value.
	if (o.inLB || o.target == altDescendant) && lb != el {
		switch {
		case role == "textbox" || role == "searchbox":
			c.visited[el] = true
			if f := el.box.fld; f != nil {
				return exposedValue(f)
			}
			return textContent(el)
		case role == "combobox" || role == "listbox":
			c.visited[el] = true
			return t.selectedOptions(el, role, c, child)
		case semantics.RangeRole(role):
			c.visited[el] = true
			switch {
			case a != nil && a.hasText:
				return a.valueText
			case a != nil && a.rng.HasNow:
				return jsNumberString(a.rng.Now)
			}
			return ""
		case role == "menu":
			c.visited[el] = true
			return ""
		}
	}
	// 2C: aria-label.
	if a != nil && semantics.TrimSpace(a.label) != "" {
		c.visited[el] = true
		return a.label
	}
	// 2D: the native text alternative of a text field (HTML-AAM): its
	// labels' content (without the field itself), then its placeholder.
	if f := el.box.fld; f != nil && !o.inLB {
		c.visited[el] = true
		if s := t.labelsText(el, c); s != "" {
			return s
		}
		return f.props().placeholder
	}
	// 2D: a button named for itself takes its labels' content first, then
	// its own content (HTML-AAM's button rule; buttons are labelable).
	if el.box.kind == style.KindButton && !o.inLB && o.target == altSelf {
		c.visited[el] = true
		s := t.labelsText(el, c)
		delete(c.visited, el) // step 2F reads the content below
		if s != "" {
			c.visited[el] = true
			return s
		}
	}
	// 2D: the native text alternative of an image.
	if role != "presentation" && role != "none" && el.box.kind == style.KindImg {
		c.visited[el] = true
		if a != nil && semantics.TrimSpace(a.alt) != "" {
			return a.alt
		}
		return ""
	}
	// 2F: the content.
	if semantics.NameFromContent(role, o.target == altDescendant) || o.inLB {
		c.visited[el] = true
		var b strings.Builder
		t.innerText(&b, el, c, child)
		s := b.String()
		if o.target == altSelf && semantics.TrimSpace(s) != "" || o.target != altSelf && s != "" {
			return s
		}
	}
	c.visited[el] = true
	return ""
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
	if p == nil || p.owner == nil || !p.owner.mounted || p.para == nil || p.para.lay == nil {
		return gfx.Rect{}
	}
	off, end := int(el.paraOff), int(el.paraOff)+len(el.text)
	if end > len(p.para.text) || p.para.text[off:end] != el.text {
		return gfx.Rect{} // a stale location: the text moved
	}
	rects := p.para.lay.SelectionRects(text.NewSelection(text.Cursor{Offset: off}, text.Cursor{Offset: end}), t.sem.rects[:0])
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
	if p == nil || p.owner == nil || !p.owner.mounted || p.para == nil || p.para.lay == nil {
		return gfx.Rect{}
	}
	rects := p.para.inlineRects(el, t.sem.rects[:0])
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
)

// actionElement returns the primitive that acts for node id: its element,
// or a text's parent element.
func (t *Tree) actionElement(id semantics.NodeID) (*Element, error) {
	el := t.sem.byID[id]
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
