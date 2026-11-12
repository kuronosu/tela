package platform

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform/internal/idmap"
)

// Accessibility (platform contract v5, ADR-246). See "Accessibility" in
// the package documentation for the model; this file holds its types.
//
// The tree is flat data: package platform sits below package semantics,
// so it cannot use semantics.Node, and the bridges keep copies of it on
// other threads, so it holds no pointers into the layers above. Every
// node is an AccessNode value naming its children by AccessID.

// AccessID identifies a node of a window's accessibility tree. It is
// never 0 for a node (0 means none in the fields that refer to one) and
// stays the same for the life of the element the node stands for, across
// updates, so a bridge keeps its objects by ID.
type AccessID uint64

// AccessKind is the kind of an [AccessNode].
type AccessKind uint8

const (
	// AccessElement is an element; Role says what it is.
	AccessElement AccessKind = iota
	// AccessRoot is the root of the tree: the window's content. It has
	// no Role; bridges expose it as the window (AT-SPI frame, the UIA
	// fragment root, the window's content view).
	AccessRoot
	// AccessText is a run of text, Text, inside its parent element: the
	// text a bridge's text interface reads (AT-SPI Text, UIA TextPattern,
	// AXValue of static text).
	AccessText

	accessKindCount
)

var accessKindNames = [...]string{"element", "root", "text"}

func (k AccessKind) String() string {
	if k < accessKindCount {
		return accessKindNames[k]
	}
	return fmt.Sprintf("AccessKind(%d)", uint8(k))
}

// AccessToggle is a tri-state ARIA value (aria-checked, -pressed,
// -selected, -expanded): not set, false, true or mixed. Its values are
// those of semantics.Toggle.
type AccessToggle uint8

const (
	ToggleUnset AccessToggle = iota
	ToggleFalse
	ToggleTrue
	ToggleMixed
)

var accessToggleNames = [...]string{"unset", "false", "true", "mixed"}

func (t AccessToggle) String() string {
	if int(t) < len(accessToggleNames) {
		return accessToggleNames[t]
	}
	return fmt.Sprintf("AccessToggle(%d)", uint8(t))
}

// AccessStates is a set of boolean states of an [AccessNode].
type AccessStates uint32

const (
	// AccessFocused: the element has the keyboard focus of the window
	// (also while the window itself is not focused); AccessUpdate.Focus
	// names it.
	AccessFocused AccessStates = 1 << iota
	// AccessFocusable: the element can take the focus now.
	AccessFocusable
	// AccessDisabled: the element is disabled (Disabled, AriaDisabled).
	AccessDisabled
	// AccessScrollable: a scroll container the user can scroll whose
	// content overflows.
	AccessScrollable
	// AccessMultiline: a text field that takes several lines (Textarea).
	AccessMultiline
	// AccessReadOnly: a text field the user cannot edit, or AriaReadOnly.
	AccessReadOnly
	// AccessRequired: AriaRequired.
	AccessRequired
	// AccessBusy: AriaBusy: the element is being updated; a live region
	// that is busy announces nothing until it is not.
	AccessBusy
	// AccessInvalid: the element's value fails validation (AriaInvalid,
	// or a text field's own validity).
	AccessInvalid
	// AccessProtected: a password field. Value holds the masked text (one
	// bullet per grapheme cluster); bridges expose it as a password field
	// (AT-SPI password text, UIA IsPassword, AXSecureTextField).
	AccessProtected
	// AccessModal: AriaModal, a modal dialog.
	AccessModal
	// AccessMultiSelectable: AriaMultiSelectable.
	AccessMultiSelectable
	// AccessAtomic: a live region (Live set) whose whole text is read on a
	// change (aria-atomic; implicit for alert and status).
	AccessAtomic
	// AccessInline: the node is laid out inline (a text, a Span in a
	// paragraph): its text runs on with its neighbors'. Between nodes
	// that are not inline, text consumers put a word break.
	AccessInline

	accessStateEnd
)

var accessStateNames = [...]string{"focused", "focusable", "disabled", "scrollable", "multiline", "readonly",
	"required", "busy", "invalid", "protected", "modal", "multiselectable", "atomic", "inline"}

func (s AccessStates) String() string {
	var b strings.Builder
	for i, name := range accessStateNames {
		if s&(1<<i) != 0 {
			if b.Len() > 0 {
				b.WriteByte('|')
			}
			b.WriteString(name)
		}
	}
	if rest := s &^ (accessStateEnd - 1); rest != 0 {
		if b.Len() > 0 {
			b.WriteByte('|')
		}
		fmt.Fprintf(&b, "%#x", uint32(rest))
	}
	return b.String()
}

// AccessAction is an action an assistive technology (AT) asks a node to
// perform ([AccessActionEvent]); a node lists the ones it supports in
// AccessNode.Actions.
type AccessAction uint8

const (
	// AccessActionDefault is the default action, a click: HTML's
	// element.click() (AT-SPI's "click", UIA Invoke and Toggle, AXPress).
	AccessActionDefault AccessAction = iota + 1
	// AccessActionFocus moves the keyboard focus to the node.
	AccessActionFocus
	// AccessActionSetValue replaces the value of a text field with
	// AccessActionEvent.Value (UIA ValuePattern.SetValue, AT-SPI
	// EditableText.SetTextContents, setting AXValue).
	AccessActionSetValue
	// AccessActionIncrement and AccessActionDecrement step a range widget
	// (slider, spinbutton, scrollbar) by one key press.
	AccessActionIncrement
	AccessActionDecrement
	// AccessActionExpand and AccessActionCollapse expand or collapse an
	// element with Expanded set.
	AccessActionExpand
	AccessActionCollapse
	// AccessActionScrollIntoView scrolls the containers around the node so
	// that it is visible.
	AccessActionScrollIntoView
	// AccessActionScrollBy scrolls a scroll container (AccessScrollable)
	// by AccessActionEvent.DX and DY logical pixels.
	AccessActionScrollBy
	// AccessActionSetSelection sets the caret and selection of a text
	// field to AccessActionEvent.Selection.
	AccessActionSetSelection
	// AccessActionShowMenu opens the context menu of the node, as the
	// ContextMenu key does (AXShowMenu, AT-SPI's "showmenu").
	AccessActionShowMenu

	accessActionEnd
)

var accessActionNames = [...]string{"", "default", "focus", "set-value", "increment", "decrement",
	"expand", "collapse", "scroll-into-view", "scroll-by", "set-selection", "show-menu"}

// Valid reports whether a is one of the AccessAction constants.
func (a AccessAction) Valid() bool { return a > 0 && a < accessActionEnd }

func (a AccessAction) String() string {
	if a.Valid() {
		return accessActionNames[a]
	}
	return fmt.Sprintf("AccessAction(%d)", uint8(a))
}

// AccessActions is the set of actions a node supports: bit 1<<a for each
// AccessAction a.
type AccessActions uint16

// AccessActionsOf returns the set of actions as.
func AccessActionsOf(as ...AccessAction) AccessActions {
	var s AccessActions
	for _, a := range as {
		s |= 1 << a
	}
	return s
}

// Has reports whether the set holds action a.
func (s AccessActions) Has(a AccessAction) bool { return a.Valid() && s&(1<<a) != 0 }

func (s AccessActions) String() string {
	var b strings.Builder
	for a := AccessActionDefault; a < accessActionEnd; a++ {
		if s.Has(a) {
			if b.Len() > 0 {
				b.WriteByte('|')
			}
			b.WriteString(a.String())
		}
	}
	return b.String()
}

// AccessRange holds the numeric values of a range widget (slider,
// progressbar, spinbutton, scrollbar, meter): aria-valuenow, -valuemin and
// -valuemax, each with whether it is set.
type AccessRange struct {
	Now, Min, Max          float64
	HasNow, HasMin, HasMax bool
}

// AccessSelection is the caret and selection of a text field, as offsets
// in Unicode code points into the node's Value (not bytes: bridges
// convert to their platform's unit, UTF-16 for UI Automation and
// NSAccessibility, code points for AT-SPI). Anchor is where the selection
// started and Focus the caret: equal for a collapsed caret, Focus < Anchor
// for a backward selection.
type AccessSelection struct {
	Anchor, Focus int
}

// AccessNode is one node of a window's accessibility tree, flat data
// derived from semantics.Node (internal/semantics documents each value).
// Strings are empty and numbers 0 when unset.
type AccessNode struct {
	ID   AccessID
	Kind AccessKind
	// Role is the ARIA role of an element ("button", "heading",
	// "textbox"...): explicit or implicit after HTML-AAM; "generic" for
	// containers without one, "none" or "presentation" for elements
	// that are not exposed (see [AccessNode.Ignored]). Empty for the root
	// and texts. Bridges map it after W3C Core-AAM.
	Role string
	// Name and Description are the accessible name (W3C AccName) and
	// description, white space collapsed.
	Name, Description string
	// Value is the value of a text field (masked for AccessProtected) or
	// of a range widget (aria-valuetext, else aria-valuenow).
	Value string
	// Text is the text of an AccessText node, as written.
	Text string
	// URL is the destination of a link.
	URL string
	// Placeholder is the placeholder of a text field.
	Placeholder string
	// KeyShortcuts is aria-keyshortcuts ("Control+S Meta+S").
	KeyShortcuts string
	// Range holds a range widget's values.
	Range AccessRange
	// States, toggles and the supported actions.
	States                               AccessStates
	Checked, Pressed, Selected, Expanded AccessToggle
	Actions                              AccessActions
	// Level is the heading level or aria-level.
	Level int
	// PosInSet and SetSize are an item's position (from 1) in its set and
	// the set's size (-1: unknown); 0 outside a set.
	PosInSet, SetSize int
	// RowCount is aria-rowcount of a table or grid (-1: unknown) and
	// RowIndex aria-rowindex of a row (from 1).
	RowCount, RowIndex int
	// Sort is aria-sort of a header ("ascending", "descending", "other")
	// and Current aria-current ("page", "step", "location", "date",
	// "time", "true").
	Sort, Current string
	// HasPopup is aria-haspopup ("menu", "listbox", "tree", "grid",
	// "dialog"), Orientation aria-orientation ("horizontal", "vertical"),
	// AutoComplete aria-autocomplete ("inline", "list", "both").
	HasPopup, Orientation, AutoComplete string
	// Live is the politeness of a live region's root: "polite" or
	// "assertive" (explicit, or implied by the roles alert, status and
	// log); "" for every other node. AccessAtomic goes with it. LiveRoot
	// is the ID of the live region the node is in (its own for a region's
	// root), 0 outside every region. Core computes the announcements
	// (AccessUpdate.Announcements); bridges that also report regions to
	// the AT (AT-SPI's container-live attributes) read these.
	Live     string
	LiveRoot AccessID
	// Controls and ActiveDescendant are aria-controls and
	// aria-activedescendant: the IDs of the referenced nodes (0 for none).
	// The node may be missing from the tree (hidden).
	Controls, ActiveDescendant AccessID
	// Selection is the caret and selection of a text field when
	// HasSelection is set.
	Selection    AccessSelection
	HasSelection bool
	// Bounds is the node's rectangle in logical pixels, window
	// coordinates (the content area's top-left is the origin), with
	// scrolling and transforms applied: the bounding box of what it shows.
	// It may lie outside the window (scrolled away).
	Bounds Rect
	// Children are the node's children in order.
	Children []AccessID
}

// Has reports whether all of the states s are set.
func (n *AccessNode) Has(s AccessStates) bool { return n.States&s == s }

// Ignored reports whether n is a node most bridges leave out of the tree
// they expose, making its children their parent's (AccessKit's common
// filter, as Chromium ignores layout-only divs): an element with the role
// "none" or "presentation", or a "generic" container that is neither
// focusable, scrollable, clickable nor a live region and has no name or
// description. The root and texts are never ignored. Core sends these
// nodes anyway, so IDs and bounds stay available and every bridge
// decides with the same rule.
func (n *AccessNode) Ignored() bool {
	if n.Kind != AccessElement {
		return false
	}
	switch n.Role {
	case "none", "presentation":
		return true
	case "generic":
		return n.States&(AccessFocusable|AccessFocused|AccessScrollable) == 0 && !n.Actions.Has(AccessActionDefault) &&
			n.Live == "" && n.Name == "" && n.Description == ""
	}
	return false
}

// Equal reports whether n and o hold the same values, children included.
// Core sends a node again only when it is not Equal to the one it sent.
func (n *AccessNode) Equal(o *AccessNode) bool {
	return n.ID == o.ID && n.Kind == o.Kind && n.Role == o.Role &&
		n.Name == o.Name && n.Description == o.Description && n.Value == o.Value &&
		n.Text == o.Text && n.URL == o.URL && n.Placeholder == o.Placeholder &&
		n.KeyShortcuts == o.KeyShortcuts && n.Range == o.Range && n.States == o.States &&
		n.Checked == o.Checked && n.Pressed == o.Pressed && n.Selected == o.Selected &&
		n.Expanded == o.Expanded && n.Actions == o.Actions && n.Level == o.Level &&
		n.PosInSet == o.PosInSet && n.SetSize == o.SetSize &&
		n.RowCount == o.RowCount && n.RowIndex == o.RowIndex &&
		n.Sort == o.Sort && n.Current == o.Current && n.HasPopup == o.HasPopup &&
		n.Orientation == o.Orientation && n.AutoComplete == o.AutoComplete &&
		n.Live == o.Live && n.LiveRoot == o.LiveRoot &&
		n.Controls == o.Controls && n.ActiveDescendant == o.ActiveDescendant &&
		n.Selection == o.Selection && n.HasSelection == o.HasSelection &&
		n.Bounds == o.Bounds && slices.Equal(n.Children, o.Children)
}

// Clone returns a copy of n that shares nothing with it.
func (n AccessNode) Clone() AccessNode {
	n.Children = slices.Clone(n.Children)
	return n
}

// Announcement is a message for the user's assistive technology to speak
// without moving the focus: the change of a live region's content.
// Assertive interrupts what the AT is saying (aria-live="assertive",
// role alert); otherwise it waits (polite).
type Announcement struct {
	Text      string
	Assertive bool
}

// AccessUpdate is one change of a window's accessibility tree, given to
// [Window.SetAccessibility].
type AccessUpdate struct {
	// Nodes are the nodes that are new or changed since the previous
	// update, in no required order (core sends them parents first). The
	// first update after the bridge became active holds every node of the
	// tree. A node that is no longer reachable from Root through Children
	// is gone, with the nodes only it reached; an ID that comes back later
	// is a new node.
	Nodes []AccessNode
	// Root is the ID of the tree's root (an AccessRoot node).
	Root AccessID
	// Focus is the node that has the window's keyboard focus, Root when no
	// element has it. Bridges report focus changes to the AT when it
	// differs from the previous update's.
	Focus AccessID
	// Announcements are the live region changes to speak, in order.
	Announcements []Announcement
}

// Clone returns a copy of u that shares nothing with it.
func (u AccessUpdate) Clone() AccessUpdate {
	if u.Nodes != nil {
		nodes := make([]AccessNode, len(u.Nodes))
		for i := range u.Nodes {
			nodes[i] = u.Nodes[i].Clone()
		}
		u.Nodes = nodes
	}
	u.Announcements = slices.Clone(u.Announcements)
	return u
}

// CheckAccessUpdate panics with an actionable message if u is not a valid
// update: Root or Focus 0, a node with ID 0 or listed twice, an unknown
// kind or toggle, a kind and Role that do not go together, a child ID of
// 0, a string that is not valid UTF-8, or bounds that are not finite or
// have a negative size. It checks u alone; whether its children are known
// depends on the updates before it ([AccessTree.Apply] checks that).
// Every backend calls it at the top of SetAccessibility.
func CheckAccessUpdate(u *AccessUpdate) {
	if u.Root == 0 {
		panicf("platform: SetAccessibility: AccessUpdate.Root is 0; it must name the root node")
	}
	if u.Focus == 0 {
		panicf("platform: SetAccessibility: AccessUpdate.Focus is 0; it is the Root when no element has the focus")
	}
	seen := make(map[AccessID]struct{}, len(u.Nodes))
	for i := range u.Nodes {
		n := &u.Nodes[i]
		if n.ID == 0 {
			panicf("platform: SetAccessibility: node %d of the update has ID 0", i)
		}
		if _, dup := seen[n.ID]; dup {
			panicf("platform: SetAccessibility: node %d is listed twice in one update", n.ID)
		}
		seen[n.ID] = struct{}{}
		if n.Kind >= accessKindCount {
			panicf("platform: SetAccessibility: node %d has the unknown kind %d", n.ID, n.Kind)
		}
		if (n.Kind == AccessElement) != (n.Role != "") {
			panicf("platform: SetAccessibility: node %d is a %v with the role %q; elements have a role, the root and texts none", n.ID, n.Kind, n.Role)
		}
		if n.Kind == AccessRoot && n.ID != u.Root {
			panicf("platform: SetAccessibility: node %d is a root but Root is %d", n.ID, u.Root)
		}
		if n.Kind == AccessText && len(n.Children) > 0 {
			panicf("platform: SetAccessibility: text node %d has children", n.ID)
		}
		if n.Kind != AccessRoot && n.ID == u.Root {
			panicf("platform: SetAccessibility: the Root node %d is a %v, not a root", n.ID, n.Kind)
		}
		for _, t := range [...]AccessToggle{n.Checked, n.Pressed, n.Selected, n.Expanded} {
			if t > ToggleMixed {
				panicf("platform: SetAccessibility: node %d has the invalid toggle %d", n.ID, t)
			}
		}
		for _, s := range [...]string{n.Role, n.Name, n.Description, n.Value, n.Text, n.URL, n.Placeholder, n.KeyShortcuts,
			n.Sort, n.Current, n.HasPopup, n.Orientation, n.AutoComplete, n.Live} {
			if !utf8.ValidString(s) {
				panicf("platform: SetAccessibility: node %d has a string that is not valid UTF-8: %q", n.ID, s)
			}
		}
		b := n.Bounds
		for _, v := range [4]float32{b.X, b.Y, b.W, b.H} {
			if v != v || math.IsInf(float64(v), 0) {
				panicf("platform: SetAccessibility: node %d has the bounds %v; coordinates must be finite", n.ID, b)
			}
		}
		if b.W < 0 || b.H < 0 {
			panicf("platform: SetAccessibility: node %d has the bounds %v with a negative size", n.ID, b)
		}
		for _, c := range n.Children {
			if c == 0 {
				panicf("platform: SetAccessibility: node %d has a child with ID 0", n.ID)
			}
		}
	}
	for i, a := range u.Announcements {
		if !utf8.ValidString(a.Text) {
			panicf("platform: SetAccessibility: announcement %d is not valid UTF-8", i)
		}
	}
}

// CheckAccessActionEvent returns why e could not come from a bridge, or
// "": an unknown action, node 0, a scroll that is not finite or a
// negative selection offset.
func CheckAccessActionEvent(e AccessActionEvent) string {
	switch {
	case !e.Action.Valid():
		return fmt.Sprintf("unknown action %d", e.Action)
	case e.Node == 0:
		return "node 0"
	case e.DX != e.DX || e.DY != e.DY || math.IsInf(float64(e.DX), 0) || math.IsInf(float64(e.DY), 0):
		return fmt.Sprintf("scroll by %g, %g is not finite", e.DX, e.DY)
	case e.Selection.Anchor < 0 || e.Selection.Focus < 0:
		return fmt.Sprintf("negative selection %+v", e.Selection)
	case !utf8.ValidString(e.Value):
		return "a value that is not valid UTF-8"
	}
	return ""
}

// AccessTree is an accessibility tree as a bridge holds it: the result of
// the updates applied so far. It is immutable: Apply returns a new tree,
// so a bridge can answer queries from one tree on other threads while the
// UI thread makes the next (ADR-246's push model). The nil *AccessTree is
// the empty tree, before the first update. Headless keeps one per window
// to check core's updates.
//
// The trees of successive updates share their structure (a persistent
// map, ADR-277): Apply costs about what the update changed, and a node the
// update did not name keeps its *AccessNode, so a bridge can tell the
// nodes that changed by pointer.
type AccessTree struct {
	nodes       idmap.Map[accessEntry]
	root, focus AccessID
}

// accessEntry is a node of an AccessTree and its parent (0 for the root).
type accessEntry struct {
	n      *AccessNode
	parent AccessID
}

// Apply returns the tree after update u (checked with CheckAccessUpdate
// first), and the IDs of the nodes it removed (no longer reachable from
// the root), in no particular order. It fails, leaving t as it is, when
// the update names a child that neither it nor t holds, when a node would
// be reached twice (two parents, or a cycle), or when Focus is not in the
// tree. Its cost follows the update: the nodes it holds, their children
// lists and the nodes it removes, not the tree's size (the first update,
// or one with a new root, builds the whole tree).
func (t *AccessTree) Apply(u *AccessUpdate) (*AccessTree, []AccessID, error) {
	CheckAccessUpdate(u)
	if t != nil && t.root == u.Root {
		if nt, removed, ok := t.applyDiff(u); ok {
			return nt, removed, nil
		}
	}
	return t.applyFull(u)
}

// applyFull is Apply walking the whole new tree from its root.
func (t *AccessTree) applyFull(u *AccessUpdate) (*AccessTree, []AccessID, error) {
	var old idmap.Map[accessEntry]
	if t != nil {
		old = t.nodes
	}
	changed := make(map[AccessID]*AccessNode, len(u.Nodes))
	for i := range u.Nodes {
		n := u.Nodes[i].Clone()
		changed[n.ID] = &n
	}
	get := func(id AccessID) *AccessNode {
		if n := changed[id]; n != nil {
			return n
		}
		e, _ := old.Get(uint64(id))
		return e.n
	}
	root := get(u.Root)
	if root == nil {
		return t, nil, fmt.Errorf("platform: AccessTree: the root %d is not in the tree", u.Root)
	}
	if root.Kind != AccessRoot {
		return t, nil, fmt.Errorf("platform: AccessTree: the root %d is a %v", u.Root, root.Kind)
	}
	var b idmap.Builder[accessEntry]
	type item struct{ id, parent AccessID }
	stack := []item{{u.Root, 0}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		id := it.id
		n := get(id)
		if n == nil {
			return t, nil, fmt.Errorf("platform: AccessTree: node %d, a child of %d, is neither in the update nor in the tree", id, it.parent)
		}
		if _, dup := b.Get(uint64(id)); dup {
			return t, nil, fmt.Errorf("platform: AccessTree: node %d is reached twice (children of %d and of another node, or a cycle)", id, it.parent)
		}
		if n.Kind == AccessRoot && id != u.Root {
			return t, nil, fmt.Errorf("platform: AccessTree: node %d, a child of %d, is a root", id, it.parent)
		}
		b.Set(uint64(id), accessEntry{n: n, parent: it.parent})
		for i := len(n.Children) - 1; i >= 0; i-- {
			stack = append(stack, item{n.Children[i], id})
		}
	}
	nodes := b.Map()
	if !nodes.Has(uint64(u.Focus)) {
		return t, nil, fmt.Errorf("platform: AccessTree: the focus %d is not in the tree", u.Focus)
	}
	var removed []AccessID
	old.Range(func(k uint64, _ accessEntry) bool {
		if !nodes.Has(k) {
			removed = append(removed, AccessID(k))
		}
		return true
	})
	return &AccessTree{nodes: nodes, root: u.Root, focus: u.Focus}, removed, nil
}

// applyDiff is Apply from the children lists the update changed: the
// nodes they dropped (and their subtrees) are removed, those they gained
// are new or move from the parent that dropped them. ok is false when the
// update is anything else (a child with two parents, a cycle, an unknown
// node, a node attached below one that leaves the tree, a focus outside
// it): applyFull then decides, with its error.
func (t *AccessTree) applyDiff(u *AccessUpdate) (nt *AccessTree, removed []AccessID, ok bool) {
	old := t.nodes
	changed := make(map[AccessID]*AccessNode, len(u.Nodes))
	for i := range u.Nodes {
		n := u.Nodes[i].Clone()
		changed[n.ID] = &n
	}
	lost := map[AccessID]AccessID{}   // a child dropped → the parent that dropped it
	gained := map[AccessID]AccessID{} // a child added → its new parent
	var set map[AccessID]struct{}
	for id, n := range changed {
		var was []AccessID
		if e, in := old.Get(uint64(id)); in {
			was = e.n.Children
		}
		if slices.Equal(was, n.Children) {
			continue
		}
		if set == nil {
			set = map[AccessID]struct{}{}
		}
		clear(set)
		for _, c := range n.Children {
			set[c] = struct{}{}
		}
		if len(set) != len(n.Children) {
			return nil, nil, false // a child listed twice
		}
		for _, c := range was {
			if _, kept := set[c]; !kept {
				lost[c] = id
			}
		}
		clear(set)
		for _, c := range was {
			set[c] = struct{}{}
		}
		for _, c := range n.Children {
			if _, had := set[c]; had {
				continue
			}
			if _, twice := gained[c]; twice {
				return nil, nil, false
			}
			gained[c] = id
		}
	}
	// Every child gained exists, and moves only from the parent that
	// dropped it.
	for c := range gained {
		if c == u.Root {
			return nil, nil, false
		}
		if e, in := old.Get(uint64(c)); in {
			if p, dropped := lost[c]; !dropped || p != e.parent {
				return nil, nil, false
			}
		} else if changed[c] == nil {
			return nil, nil, false
		}
	}
	// alive reports whether node id is in the new tree.
	const (
		unknown = iota
		visiting
		yes
		no
	)
	state := map[AccessID]uint8{u.Root: yes}
	var alive func(id AccessID) (bool, bool)
	alive = func(id AccessID) (in, valid bool) {
		switch state[id] {
		case yes:
			return true, true
		case no:
			return false, true
		case visiting:
			return false, false // a cycle
		}
		state[id] = visiting
		var p AccessID
		if g, ok := gained[id]; ok {
			p = g
		} else if _, dropped := lost[id]; !dropped {
			if e, in := old.Get(uint64(id)); in {
				p = e.parent
			}
		}
		in, valid = p != 0, true
		if in {
			in, valid = alive(p)
		}
		if in {
			state[id] = yes
		} else {
			state[id] = no
		}
		return in, valid
	}
	for _, p := range gained {
		if in, valid := alive(p); !in || !valid {
			return nil, nil, false
		}
	}
	if changed[u.Root] != nil && changed[u.Root].Kind != AccessRoot {
		return nil, nil, false
	}
	if _, in := changed[u.Focus]; !in && !old.Has(uint64(u.Focus)) {
		return nil, nil, false
	}
	if in, valid := alive(u.Focus); !in || !valid {
		return nil, nil, false
	}
	b := idmap.NewBuilder(old)
	// The nodes that leave: those dropped and not gained elsewhere, with
	// their subtrees (as the old tree has them), but for what moves.
	var drop func(id AccessID)
	drop = func(id AccessID) {
		if _, moved := gained[id]; moved {
			return
		}
		e, in := b.Get(uint64(id))
		if !in {
			return
		}
		b.Delete(uint64(id))
		removed = append(removed, id)
		for _, c := range e.n.Children {
			drop(c)
		}
	}
	for c := range lost {
		drop(c)
	}
	for id, n := range changed {
		in, valid := alive(id)
		if !valid {
			return nil, nil, false
		}
		if !in {
			continue // not reachable: not in the tree, as applyFull leaves it
		}
		p := AccessID(0)
		if id != u.Root {
			if g, ok := gained[id]; ok {
				p = g
			} else {
				e, _ := old.Get(uint64(id))
				p = e.parent
			}
		}
		b.Set(uint64(id), accessEntry{n: n, parent: p})
	}
	for c, p := range gained {
		if changed[c] != nil {
			continue
		}
		e, _ := old.Get(uint64(c))
		e.parent = p
		b.Set(uint64(c), e)
	}
	return &AccessTree{nodes: b.Map(), root: u.Root, focus: u.Focus}, removed, true
}

// Root returns the ID of the root, 0 for the empty tree.
func (t *AccessTree) Root() AccessID {
	if t == nil {
		return 0
	}
	return t.root
}

// Focus returns the ID of the focused node (the root when no element has
// the focus), 0 for the empty tree.
func (t *AccessTree) Focus() AccessID {
	if t == nil {
		return 0
	}
	return t.focus
}

// Len returns the number of nodes.
func (t *AccessTree) Len() int {
	if t == nil {
		return 0
	}
	return t.nodes.Len()
}

// Node returns the node with ID id. The node must not be modified.
func (t *AccessTree) Node(id AccessID) (*AccessNode, bool) {
	if t == nil {
		return nil, false
	}
	e, ok := t.nodes.Get(uint64(id))
	return e.n, ok
}

// Parent returns the parent of node id (0 for the root) and whether the
// node is in the tree.
func (t *AccessTree) Parent(id AccessID) (AccessID, bool) {
	if t == nil {
		return 0, false
	}
	e, ok := t.nodes.Get(uint64(id))
	return e.parent, ok
}

// Walk calls f for every node in tree order, with its depth (0 for the
// root); when f returns false the node's descendants are skipped.
func (t *AccessTree) Walk(f func(n *AccessNode, depth int) bool) {
	if t == nil {
		return
	}
	var walk func(id AccessID, depth int)
	walk = func(id AccessID, depth int) {
		n, _ := t.Node(id)
		if !f(n, depth) {
			return
		}
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(t.root, 0)
}

// String returns the tree as indented lines, for tests and debugging:
// each node's kind or role, its quoted name or text, and its states.
func (t *AccessTree) String() string {
	var b strings.Builder
	t.Walk(func(n *AccessNode, depth int) bool {
		b.WriteString(strings.Repeat("  ", depth))
		switch n.Kind {
		case AccessElement:
			b.WriteString(n.Role)
		default:
			b.WriteString(n.Kind.String())
		}
		if n.Name != "" {
			fmt.Fprintf(&b, " %q", n.Name)
		}
		if n.Text != "" {
			fmt.Fprintf(&b, " %q", n.Text)
		}
		if n.Value != "" {
			fmt.Fprintf(&b, " value=%q", n.Value)
		}
		if n.States != 0 {
			fmt.Fprintf(&b, " [%v]", n.States)
		}
		b.WriteByte('\n')
		return true
	})
	return b.String()
}
