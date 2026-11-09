package semantics

import "github.com/kuronosu/tela/internal/gfx"

// NodeID identifies a node. The node of an element keeps its ID for the
// life of the element, across builds of the tree, so a handle taken from
// one tree still names the same element in a later one.
type NodeID uint64

// Kind is the kind of a node.
type Kind uint8

const (
	// KindRoot is the root of a tree: the window's content. It has no
	// role (Playwright's "fragment").
	KindRoot Kind = iota + 1
	// KindElement is a primitive element.
	KindElement
	// KindText is a text; Text holds it as written (white space is
	// collapsed by consumers such as Snapshot).
	KindText
)

func (k Kind) String() string {
	switch k {
	case KindRoot:
		return "root"
	case KindElement:
		return "element"
	case KindText:
		return "text"
	}
	return "?"
}

// Toggle is a tri-state ARIA value (aria-checked, aria-pressed,
// aria-selected, aria-expanded): not set, false, true or mixed.
type Toggle uint8

const (
	ToggleUnset Toggle = iota
	ToggleFalse
	ToggleTrue
	ToggleMixed
)

// Of returns ToggleTrue or ToggleFalse.
func Of(b bool) Toggle {
	if b {
		return ToggleTrue
	}
	return ToggleFalse
}

func (t Toggle) String() string {
	switch t {
	case ToggleFalse:
		return "false"
	case ToggleTrue:
		return "true"
	case ToggleMixed:
		return "mixed"
	}
	return "unset"
}

// State is a set of boolean node states.
type State uint16

const (
	// StateFocused: the element is the focused element of its window
	// (also while the window itself is not focused).
	StateFocused State = 1 << iota
	// StateFocusable: the element can take the focus now.
	StateFocusable
	// StateDisabled: the element is disabled, by Tela's Disabled (on it or
	// an ancestor) or by AriaDisabled.
	StateDisabled
	// StateHidden: the node is hidden for accessibility: display: none or
	// AriaHidden on it or an ancestor, or visibility other than visible.
	StateHidden
	// StateScrollable: the element is a scroll container the user can
	// scroll (overflow auto or scroll) whose content overflows.
	StateScrollable
	// StateMultiline: a text field that takes several lines (Textarea).
	StateMultiline
	// StateReadOnly: a text field the user cannot edit (ReadOnly), or an
	// element with AriaReadOnly(true).
	StateReadOnly
	// StateRequired: AriaRequired(true), a field the user must fill.
	StateRequired
	// StateBusy: AriaBusy(true), the element is being updated.
	StateBusy
	// StateInvalid: the element's value fails validation, as Chromium's
	// accessibility tree computes it: AriaInvalid when it is set (true or
	// false), else a text field's own validity (the invalid: variant: a
	// type mismatch of an email or url value, a maxlength exceeded by an
	// edit; never for a read-only or disabled field). Node.Invalid holds the prop alone, which is what aria
	// snapshots print.
	StateInvalid
	// StateProtected: a text field whose value is not shown (an Input of
	// type password): Value holds the masked text (one bullet per
	// grapheme cluster), and bridges expose the field as a password one
	// (AT-SPI's password text, UIA's IsPassword, AXSecureTextField).
	StateProtected
)

// Action is a set of actions a node supports (ARCHITECTURE section 16).
// Package core runs each one (ClickNode, FocusNode, IncrementNode...).
type Action uint16

const (
	// ActionClick: a click does something (a button, a link, an element
	// with click handlers).
	ActionClick Action = 1 << iota
	// ActionFocus: the element is focusable.
	ActionFocus
	// ActionScrollIntoView: the element is laid out, so its scroll
	// containers can reveal it.
	ActionScrollIntoView
	// ActionScroll: the element scrolls (StateScrollable).
	ActionScroll
	// ActionSetValue: an editable text field (telatest's Fill).
	ActionSetValue
	// ActionIncrement and ActionDecrement: a range widget the user moves
	// with the arrow keys (slider, spinbutton, scrollbar) that is
	// focusable and neither disabled nor read-only. Core synthesizes the
	// arrow key presses, as Chromium does for ARIA range widgets.
	ActionIncrement
	ActionDecrement
	// ActionExpand and ActionCollapse: an element with AriaExpanded that is
	// not disabled, collapsed (ActionExpand) or expanded (ActionCollapse).
	// Core clicks it, as Chromium does.
	ActionExpand
	ActionCollapse
	// ActionShowMenu: an element that is not disabled with a context-menu
	// handler (OnContextMenu, as package aria's UseContextMenuTrigger
	// sets): core opens its menu as the ContextMenu key does.
	ActionShowMenu
	// ActionSetSelection: a text field (Input, Textarea) whose caret and
	// selection can be moved (Node.Selection), editable or read-only.
	ActionSetSelection
)

// TextSelection is the caret and selection of a text field, as offsets in
// Unicode code points into the node's Value (not bytes: bridges convert
// them to the unit of their platform, UTF-16 for UI Automation and
// NSAccessibility, code points for AT-SPI). Anchor is where the selection
// started and Focus the caret; they are equal for a collapsed caret, and
// Focus < Anchor for a backward selection. During an IME composition the
// caret sits in the composed text, which Value includes.
type TextSelection struct {
	Anchor, Focus int
}

// Range holds the values of a range widget (slider, progressbar,
// spinbutton, scrollbar, meter): aria-valuenow, -valuemin and -valuemax.
type Range struct {
	Now, Min, Max          float64
	HasNow, HasMin, HasMax bool
}

// Node is a semantics node. A tree is built by package core and must not be
// modified by its consumers: core may hand out the same tree again while
// nothing changed.
type Node struct {
	ID   NodeID
	Kind Kind
	// Role is the ARIA role of an element: explicit (Role prop) or
	// implicit (HTML-AAM: "button", "heading", "img", "link",
	// "paragraph"...); "generic" for containers without one; "none" or
	// "presentation" for elements that are not exposed. Empty for the
	// root and texts.
	Role string
	// ExplicitRole reports a role given with the Role prop.
	ExplicitRole bool
	// Decorative marks an icon (h.Icon) that is hidden for accessibility
	// only because it is decorative (it has no AriaLabel): it is
	// StateHidden, but it is on screen and keeps its Bounds, so tests can
	// locate it by class or test ID (ADR-263).
	Decorative bool
	// Name and Description are the accessible name (W3C AccName 1.2:
	// AriaLabelledBy, AriaLabel, Alt, content) with white space collapsed,
	// and the AriaDescription.
	Name, Description string
	// Value is the value of a textbox (its text content) or a range
	// widget (AriaValueText, else AriaValueNow); empty otherwise.
	Value string
	// Range holds the numeric values of a range widget.
	Range Range
	// Text is the text of a KindText node.
	Text string
	// URL is the destination of a link (Href).
	URL string
	// Level is the heading level (H1 to H6, or AriaLevel), or AriaLevel
	// for other roles; 0 when there is none.
	Level int
	// PosInSet and SetSize are the position (from 1) of an item in its set
	// and the size of the set (-1: unknown), 0 when the node is not in a
	// set: AriaPosInSet and AriaSetSize when they are given, else computed
	// as Chromium does for the items of a set (FillSetPositions): options
	// of a listbox, menu items of a menu or menubar, tabs of a tablist,
	// radios of a radiogroup, tree items of a tree, list items of a list,
	// and the items of a group inside one of those. A set container keeps
	// its own AriaSetSize, which sizes its items. Aria snapshots do not
	// print them.
	PosInSet, SetSize int
	// RowCount and RowIndex are the AriaRowCount of a table, grid or
	// treegrid (-1: unknown) and the AriaRowIndex of a row (from 1); 0 when
	// unset. Aria snapshots do not print them.
	RowCount, RowIndex int
	// Checked, Pressed, Selected and Expanded are the ARIA states set by
	// the props of the same names.
	Checked, Pressed, Selected, Expanded Toggle
	// Invalid is the AriaInvalid prop as given (unset, false or true):
	// what Playwright's aria snapshot prints, since it reads the
	// aria-invalid attribute alone. StateInvalid is the state assistive
	// technologies get, which also covers a text field's own validity.
	Invalid Toggle
	// Sort is the AriaSort of a column or row header: "ascending",
	// "descending" or "other"; "" when unset or "none". Aria snapshots do
	// not print it.
	Sort string
	// Current is the AriaCurrent of the current item of a set: "page",
	// "step", "location", "date", "time" or "true"; "" when unset or
	// "false". Aria snapshots do not print it (Playwright's does not).
	Current string
	States  State
	Actions Action

	// Live is the politeness of a live region root: "polite" or
	// "assertive", from AriaLive, else implicit by role (alert is
	// assertive, status and log are polite; timer and marquee are off); ""
	// is off. Atomic is AriaAtomic, else true for alert and status (ARIA's
	// implicit values): an assistive technology reads the whole region on
	// a change, not only what changed. LiveRoot is the node ID of the live
	// region the node is in: its own for a region root, its nearest
	// ancestor's, or 0 outside every region (AriaLive("off") ends a
	// region for its subtree). Aria snapshots print none of them.
	Live     string
	Atomic   bool
	LiveRoot NodeID
	// KeyShortcuts is AriaKeyShortcuts: the keys that activate or focus
	// the element, in ARIA's syntax ("Control+S Meta+S"); "" for none.
	KeyShortcuts string
	// Placeholder is the Placeholder of a text field (Input, Textarea).
	Placeholder string
	// Selection is the caret and selection of a text field (Input,
	// Textarea) that exposes its text as Value; nil for other nodes.
	Selection *TextSelection

	// The widget props (aria-haspopup, -modal, -multiselectable,
	// -orientation, -autocomplete, -controls, -activedescendant; ADR-096),
	// which aria snapshots do not print. HasPopup is "menu", "listbox",
	// "tree", "grid" or "dialog" ("true" is "menu"); Orientation
	// "horizontal" or "vertical"; AutoComplete "inline", "list" or "both";
	// "" is unset or the default. Controls and ActiveDescendant are the
	// IDs of the referenced elements' nodes, 0 for none (the node may be
	// hidden or absent from the tree).
	HasPopup                   string
	Modal, MultiSelectable     bool
	Orientation, AutoComplete  string
	Controls, ActiveDescendant NodeID
	// Inline reports that the element is laid out inline (CSS display
	// inline in a paragraph): its text runs on with its neighbors'. Block
	// boundaries separate words when texts are concatenated (names,
	// snapshots).
	Inline bool
	// Bounds is the border box in window coordinates (logical pixels),
	// with transforms and scroll offsets applied: the bounding rectangle
	// of the transformed box. For an inline element or a text, it bounds
	// its text on screen; for an element without a box of its own
	// (display: contents), its children. Zero for hidden nodes.
	Bounds   gfx.Rect
	Children []*Node
}

// Has reports whether all of the states s are set.
func (n *Node) Has(s State) bool { return n.States&s == s }

// Can reports whether n supports all of the actions a.
func (n *Node) Can(a Action) bool { return n.Actions&a == a }

// Walk calls f for n and its descendants in tree order; when f returns
// false the descendants of that node are skipped.
func (n *Node) Walk(f func(*Node) bool) {
	if !f(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(f)
	}
}

// Find returns the node with the given ID in the tree rooted at n, or nil.
func (n *Node) Find(id NodeID) *Node {
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if f := c.Find(id); f != nil {
			return f
		}
	}
	return nil
}

// Flattened reports whether n is an element without a role of its own in
// an aria snapshot: an implicit generic container, or none/presentation.
// Its children belong to its parent there.
func (n *Node) Flattened() bool {
	if n.Kind != KindElement {
		return false
	}
	switch n.Role {
	case "none", "presentation":
		return true
	case "generic":
		return !n.ExplicitRole
	}
	return false
}
