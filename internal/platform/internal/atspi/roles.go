//go:build linux

package atspi

import (
	"math"
	"strconv"

	"github.com/kuronosu/tela/internal/platform"
)

// ARIA roles to AT-SPI roles after W3C Core-AAM 1.2 (the ATK/AT-SPI2
// column) and HTML-AAM, as Chromium and AccessKit map them. The
// exceptions that depend on a node's data are in atspiRole.
var ariaRoles = map[string]uint32{
	"alert":            roleNotification,
	"alertdialog":      roleAlert,
	"application":      roleEmbedded,
	"article":          roleArticle,
	"banner":           roleLandmark,
	"blockquote":       roleBlockQuote,
	"button":           rolePushButton,
	"caption":          roleCaption,
	"cell":             roleTableCell,
	"checkbox":         roleCheckBox,
	"code":             roleStatic,
	"columnheader":     roleColumnHeader,
	"combobox":         roleComboBox,
	"complementary":    roleLandmark,
	"contentinfo":      roleLandmark,
	"definition":       roleDescriptionValue,
	"deletion":         roleContentDeletion,
	"dialog":           roleDialog,
	"directory":        roleList,
	"document":         roleDocumentFrame,
	"emphasis":         roleStatic,
	"feed":             rolePanel,
	"figure":           rolePanel,
	"form":             roleForm,
	"generic":          roleSection,
	"grid":             roleTable,
	"gridcell":         roleTableCell,
	"group":            rolePanel,
	"heading":          roleHeading,
	"img":              roleImage,
	"image":            roleImage,
	"insertion":        roleContentInsertion,
	"link":             roleLink,
	"list":             roleList,
	"listbox":          roleListBox,
	"listitem":         roleListItem,
	"log":              roleLog,
	"main":             roleLandmark,
	"mark":             roleMark,
	"marquee":          roleMarquee,
	"math":             roleMath,
	"menu":             roleMenu,
	"menubar":          roleMenuBar,
	"menuitem":         roleMenuItem,
	"menuitemcheckbox": roleCheckMenuItem,
	"menuitemradio":    roleRadioMenuItem,
	"meter":            roleLevelBar,
	"navigation":       roleLandmark,
	"note":             roleComment,
	"option":           roleListItem,
	"paragraph":        roleParagraph,
	"progressbar":      roleProgressBar,
	"radio":            roleRadioButton,
	"radiogroup":       rolePanel,
	"region":           roleLandmark,
	"row":              roleTableRow,
	"rowgroup":         rolePanel,
	"rowheader":        roleRowHeader,
	"scrollbar":        roleScrollBar,
	"search":           roleLandmark,
	"searchbox":        roleEntry,
	"separator":        roleSeparator,
	"slider":           roleSlider,
	"spinbutton":       roleSpinButton,
	"status":           roleStatusBar,
	"strong":           roleStatic,
	"subscript":        roleSubscript,
	"superscript":      roleSuperscript,
	"switch":           roleSwitch,
	"tab":              rolePageTab,
	"tablist":          rolePageTabList,
	"tabpanel":         roleScrollPane,
	"table":            roleTable,
	"term":             roleDescriptionTerm,
	"textbox":          roleEntry,
	"time":             roleStatic,
	"timer":            roleTimer,
	"toolbar":          roleToolBar,
	"tooltip":          roleToolTip,
	"tree":             roleTree,
	"treegrid":         roleTreeTable,
	"treeitem":         roleTreeItem,
}

// atspiRole returns the AT-SPI role of n: the frame for the root, a label
// for a text, else the role of its ARIA role, with Core-AAM's
// exceptions: a protected text field is a password text, a button with
// aria-pressed a toggle button and one that opens a menu a push button
// menu. An unknown role is ROLE_UNKNOWN.
func atspiRole(n *platform.AccessNode) uint32 {
	switch n.Kind {
	case platform.AccessRoot:
		return roleFrame
	case platform.AccessText:
		return roleLabel
	}
	switch n.Role {
	case "textbox", "searchbox":
		if n.Has(platform.AccessProtected) {
			return rolePasswordText
		}
	case "button":
		if n.Pressed != platform.ToggleUnset {
			return roleToggleButton
		}
		if n.HasPopup == "menu" || n.HasPopup == "true" {
			return rolePushButtonMenu
		}
	}
	if r, ok := ariaRoles[n.Role]; ok {
		return r
	}
	return roleUnknown
}

// childrenPresentational reports whether the ARIA role makes its
// descendants presentational (WAI-ARIA 1.2 "Children Presentational:
// True"): their text is the element's name, and the element is a leaf
// for the AT, as Chromium exposes it.
func childrenPresentational(role string) bool {
	switch role {
	case "button", "checkbox", "img", "image", "math", "menuitemcheckbox", "menuitemradio", "meter",
		"progressbar", "radio", "scrollbar", "separator", "slider", "switch", "tab":
		return true
	}
	return false
}

// textField reports whether n is a text field: its Value is its text,
// read (and written, unless read-only) through Text and EditableText.
func textField(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement {
		return false
	}
	switch n.Role {
	case "textbox", "searchbox":
		return true
	case "combobox", "spinbutton":
		return n.HasSelection || n.Actions.Has(platform.AccessActionSetSelection)
	}
	return false
}

// editable reports whether the AT may change the text of field n.
func editable(n *platform.AccessNode) bool {
	return textField(n) && !n.Has(platform.AccessReadOnly) && !n.Has(platform.AccessDisabled) &&
		n.Actions.Has(platform.AccessActionSetValue)
}

// rangeWidget reports whether n exposes the Value interface.
func rangeWidget(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement {
		return false
	}
	switch n.Role {
	case "slider", "spinbutton", "progressbar", "scrollbar", "meter":
		return true
	}
	return n.Range.HasNow
}

// selectionContainer reports whether n exposes the Selection interface:
// a container whose items carry aria-selected (or checked, for a
// radiogroup's radios).
func selectionContainer(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement {
		return false
	}
	switch n.Role {
	case "listbox", "tablist", "tree", "treegrid", "grid", "menu", "menubar", "radiogroup":
		return true
	}
	return false
}

// textLevel reports whether an element of n's role, without anything that
// makes it worth an object of its own, only styles the text it holds, so
// that its text runs on in its parent's (text folding in view.go).
func textLevel(n *platform.AccessNode) bool {
	switch n.Role {
	case "strong", "emphasis", "code", "subscript", "superscript", "mark", "deletion", "insertion", "time":
	default:
		return false
	}
	return n.Name == "" && n.Description == "" && n.Live == "" &&
		n.States&(platform.AccessFocusable|platform.AccessFocused|platform.AccessScrollable) == 0 &&
		!n.Actions.Has(platform.AccessActionDefault)
}

// stateSet is the AT-SPI state bit set (the au of GetState: low word
// first).
type stateSet uint64

func (s stateSet) has(st int) bool { return s&(1<<st) != 0 }
func (s *stateSet) set(st int)     { *s |= 1 << st }

// words returns s as GetState returns it.
func (s stateSet) words() []uint32 { return []uint32{uint32(s), uint32(s >> 32)} }

// nodeStates returns the AT-SPI states of exposed node v of view w (the
// window's focus, the window's activity).
func nodeStates(w *snapshot, v *vnode) stateSet {
	var s stateSet
	n := v.n
	if n.Kind == platform.AccessRoot {
		s.set(stateEnabled)
		s.set(stateSensitive)
		s.set(stateVisible)
		s.set(stateShowing)
		s.set(stateResizable)
		if w.active {
			s.set(stateActive)
		}
		return s
	}
	if !n.Has(platform.AccessDisabled) {
		s.set(stateEnabled)
		s.set(stateSensitive)
	}
	// Hidden nodes are not sent: every node is visible; showing when it
	// is on the window's content area.
	s.set(stateVisible)
	if root := w.root(); root != nil && intersects(n.Bounds, root.n.Bounds) {
		s.set(stateShowing)
	}
	if n.Has(platform.AccessFocusable) {
		s.set(stateFocusable)
	}
	if n.ID == w.view.tree.Focus() && n.Kind != platform.AccessRoot {
		s.set(stateFocused)
	}
	switch n.Role {
	case "checkbox", "radio", "switch", "menuitemcheckbox", "menuitemradio":
		s.set(stateCheckable)
	}
	switch n.Checked {
	case platform.ToggleTrue:
		s.set(stateChecked)
	case platform.ToggleMixed:
		s.set(stateIndeterminate)
	}
	switch n.Pressed {
	case platform.ToggleTrue:
		s.set(statePressed)
	case platform.ToggleMixed:
		s.set(stateIndeterminate)
	}
	if n.Selected != platform.ToggleUnset {
		s.set(stateSelectable)
		if n.Selected == platform.ToggleTrue {
			s.set(stateSelected)
		}
	} else {
		switch n.Role {
		case "option", "tab", "treeitem", "row", "gridcell":
			s.set(stateSelectable)
		}
	}
	switch n.Expanded {
	case platform.ToggleTrue:
		s.set(stateExpandable)
		s.set(stateExpanded)
	case platform.ToggleFalse, platform.ToggleMixed:
		s.set(stateExpandable)
		s.set(stateCollapsed)
	}
	if n.HasPopup != "" && n.HasPopup != "false" {
		s.set(stateHasPopup)
	}
	if n.Has(platform.AccessBusy) {
		s.set(stateBusy)
	}
	if n.Has(platform.AccessInvalid) {
		s.set(stateInvalidEntry)
	}
	if n.Has(platform.AccessRequired) {
		s.set(stateRequired)
	}
	if n.Has(platform.AccessModal) {
		s.set(stateModal)
	}
	if n.Has(platform.AccessMultiSelectable) {
		s.set(stateMultiselectable)
	}
	switch n.Orientation {
	case "horizontal":
		s.set(stateHorizontal)
	case "vertical":
		s.set(stateVertical)
	}
	if textField(n) {
		if n.Has(platform.AccessMultiline) {
			s.set(stateMultiLine)
		} else {
			s.set(stateSingleLine)
		}
		if editable(n) {
			s.set(stateEditable)
		}
		if n.AutoComplete != "" && n.AutoComplete != "none" {
			s.set(stateSupportsAutocompletion)
		}
		s.set(stateSelectableText)
	}
	if n.Has(platform.AccessReadOnly) {
		s.set(stateReadOnly)
	}
	return s
}

// intersects reports whether a and b overlap, or a is an empty rectangle
// inside b.
func intersects(a, b platform.Rect) bool {
	if a.W == 0 || a.H == 0 {
		return a.X >= b.X && a.Y >= b.Y && a.X <= b.X+b.W && a.Y <= b.Y+b.H
	}
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

// nodeAttributes returns the object attributes of n (GetAttributes):
// what Chromium and Firefox report for the same ARIA data, which Orca
// reads (xml-roles for landmarks, the container-live family for live
// regions, level, posinset and setsize for items).
func nodeAttributes(t *platform.AccessTree, n *platform.AccessNode) map[string]string {
	a := map[string]string{"toolkit": "Tela"}
	if n.Kind != platform.AccessElement {
		return a
	}
	a["xml-roles"] = n.Role
	if n.Level > 0 {
		a["level"] = strconv.Itoa(n.Level)
	}
	if n.PosInSet > 0 {
		a["posinset"] = strconv.Itoa(n.PosInSet)
	}
	if n.SetSize > 0 {
		a["setsize"] = strconv.Itoa(n.SetSize)
	}
	if n.RowCount != 0 {
		a["rowcount"] = strconv.Itoa(n.RowCount)
	}
	if n.RowIndex > 0 {
		a["rowindex"] = strconv.Itoa(n.RowIndex)
	}
	if n.Placeholder != "" {
		a["placeholder-text"] = n.Placeholder
	}
	if n.KeyShortcuts != "" {
		a["keyshortcuts"] = n.KeyShortcuts
	}
	if n.Sort != "" {
		a["sort"] = n.Sort
	}
	if n.Current != "" {
		a["current"] = n.Current
	}
	if n.HasPopup != "" {
		a["haspopup"] = n.HasPopup
	}
	if n.AutoComplete != "" {
		a["autocomplete"] = n.AutoComplete
	}
	if n.Orientation != "" {
		a["orientation"] = n.Orientation
	}
	if n.Range.HasNow && n.Value != "" && n.Value != formatNumber(n.Range.Now) {
		a["valuetext"] = n.Value
	}
	if n.Live != "" {
		a["live"] = n.Live
		a["atomic"] = strconv.FormatBool(n.Has(platform.AccessAtomic))
	}
	if n.LiveRoot != 0 {
		if r, ok := t.Node(n.LiveRoot); ok && r.Live != "" {
			a["container-live"] = r.Live
			a["container-atomic"] = strconv.FormatBool(r.Has(platform.AccessAtomic))
			a["container-relevant"] = "additions text"
			if r.Has(platform.AccessBusy) {
				a["container-busy"] = "true"
			}
		}
	}
	return a
}

// formatNumber formats a range value as aria-valuenow's text: an integer
// without decimals.
func formatNumber(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// nodeAction is one action of the Action interface.
type nodeAction struct {
	name, description string
	action            platform.AccessAction
}

// nodeActions returns the actions n offers through the Action interface,
// the default first. The names are those Orca looks for ("click" first
// among its click-like actions, "show-context-menu" for a right click)
// and AccessKit's for the rest. Focus, scroll into view, set value and
// set selection are not actions on AT-SPI (Component.GrabFocus,
// Component.ScrollTo, EditableText, Text).
func nodeActions(n *platform.AccessNode) []nodeAction {
	if n.Kind != platform.AccessElement || n.Has(platform.AccessDisabled) {
		return nil
	}
	var out []nodeAction
	add := func(a platform.AccessAction, name, desc string) {
		if n.Actions.Has(a) {
			out = append(out, nodeAction{name, desc, a})
		}
	}
	add(platform.AccessActionDefault, "click", defaultDescription(n))
	if n.Expanded == platform.ToggleTrue {
		add(platform.AccessActionCollapse, "collapse", "Collapse")
	} else {
		add(platform.AccessActionExpand, "expand", "Expand")
	}
	add(platform.AccessActionIncrement, "increment", "Increment")
	add(platform.AccessActionDecrement, "decrement", "Decrement")
	add(platform.AccessActionShowMenu, "show-context-menu", "Show the context menu")
	return out
}

// defaultDescription describes the default action of n by its role.
func defaultDescription(n *platform.AccessNode) string {
	switch n.Role {
	case "checkbox", "switch", "menuitemcheckbox":
		if n.Checked == platform.ToggleTrue {
			return "Uncheck"
		}
		return "Check"
	case "radio", "menuitemradio", "option", "tab", "treeitem":
		return "Select"
	case "link":
		return "Jump"
	case "textbox", "searchbox", "combobox":
		return "Activate"
	}
	return "Click"
}
