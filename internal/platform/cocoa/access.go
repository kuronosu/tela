package cocoa

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/utf16x"
)

// The OS-independent half of the NSAccessibility bridge (ADR-246):
// which nodes the bridge exposes and how they nest, the mapping of ARIA
// roles onto AXRole and AXSubrole (W3C Core-AAM 1.2's AXAPI column), the
// values, the text ranges in UTF-16, which NSAccessibility methods apply
// to a node, and the notifications an update posts. access_darwin.go
// answers AppKit from these. See "Accessibility" in doc.go.

// axTree is a window's accessibility tree as the bridge exposes it: the
// platform tree and, computed once per update, the structure AppKit
// sees. It is immutable once built, like the AccessTree it wraps.
type axTree struct {
	tree *platform.AccessTree
	// parent is every node's parent in the platform tree (0 for the root).
	parent map[platform.AccessID]platform.AccessID
	// up is the exposed parent of each exposed node: the nearest ancestor
	// that is exposed, or the root (the content view). A node is exposed
	// when it has an entry.
	up map[platform.AccessID]platform.AccessID
	// kids are the exposed children of the root and of each exposed node,
	// in order: the children of ignored nodes take their place, and the
	// children of a node whose children are presentational are hidden.
	kids map[platform.AccessID][]platform.AccessID
}

// newAXTree computes what the bridge exposes of t (nil for the empty
// tree).
func newAXTree(t *platform.AccessTree) *axTree {
	v := &axTree{
		tree:   t,
		parent: make(map[platform.AccessID]platform.AccessID, t.Len()),
		up:     make(map[platform.AccessID]platform.AccessID, t.Len()),
		kids:   map[platform.AccessID][]platform.AccessID{},
	}
	root, ok := t.Node(t.Root())
	if !ok {
		return v
	}
	var collect func(anc platform.AccessID, n *platform.AccessNode, hidden bool)
	collect = func(anc platform.AccessID, n *platform.AccessNode, hidden bool) {
		for _, c := range n.Children {
			cn, ok := t.Node(c)
			if !ok {
				continue
			}
			v.parent[c] = n.ID
			switch {
			case hidden:
				collect(anc, cn, true)
			case cn.Ignored():
				collect(anc, cn, false)
			default:
				v.kids[anc] = append(v.kids[anc], c)
				v.up[c] = anc
				collect(c, cn, childrenPresentational(cn))
			}
		}
	}
	collect(root.ID, root, false)
	return v
}

// root returns the root's ID (0 for the empty tree).
func (v *axTree) root() platform.AccessID { return v.tree.Root() }

// exposed reports whether node id has an element of its own.
func (v *axTree) exposed(id platform.AccessID) bool {
	_, ok := v.up[id]
	return ok
}

// node returns the exposed node id, or nil.
func (v *axTree) node(id platform.AccessID) *platform.AccessNode {
	if !v.exposed(id) {
		return nil
	}
	n, _ := v.tree.Node(id)
	return n
}

// exposedAncestor returns id when it is exposed, else its nearest exposed
// ancestor, else the root: the element that stands for id.
func (v *axTree) exposedAncestor(id platform.AccessID) platform.AccessID {
	root := v.root()
	for id != 0 && id != root && !v.exposed(id) {
		id = v.parent[id]
	}
	if id == 0 {
		return root
	}
	return id
}

// focus returns the element of the focused node: its exposed ancestor,
// or the root when no element has the focus.
func (v *axTree) focus() platform.AccessID { return v.exposedAncestor(v.tree.Focus()) }

// hitTest returns the deepest exposed node under from (an exposed node or
// the root) whose bounds contain the point (x, y), in window logical
// pixels: the last such child wins, as it is painted on top. It returns
// from itself when no child contains the point, and 0 when from is the
// root and nothing does.
func (v *axTree) hitTest(from platform.AccessID, x, y float32) platform.AccessID {
	kids := v.kids[from]
	for i := len(kids) - 1; i >= 0; i-- {
		n, ok := v.tree.Node(kids[i])
		if !ok {
			continue
		}
		b := n.Bounds
		if x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H {
			return v.hitTest(kids[i], x, y)
		}
	}
	if from == v.root() {
		return 0
	}
	return from
}

// walk calls f for every exposed node under id in tree order.
func (v *axTree) walk(id platform.AccessID, f func(n *platform.AccessNode) bool) {
	for _, c := range v.kids[id] {
		n, ok := v.tree.Node(c)
		if !ok {
			continue
		}
		if f(n) {
			v.walk(c, f)
		}
	}
}

// rows returns the rows of a table, grid, treegrid or tree (AXRows): the
// exposed rows and tree items below it, nested tree items included,
// in tree order; nil for other nodes.
func (v *axTree) rows(id platform.AccessID) []platform.AccessID {
	n := v.node(id)
	if n == nil || !tableLike(n.Role) {
		return nil
	}
	var out []platform.AccessID
	v.walk(id, func(c *platform.AccessNode) bool {
		switch c.Role {
		case "row", "treeitem":
			out = append(out, c.ID)
			return c.Role == "treeitem" // nested tree items are rows too
		case "table", "grid", "treegrid", "tree":
			return false // a nested table has its own rows
		}
		return true
	})
	return out
}

// tableOf returns the nearest exposed table-like ancestor of id, or 0.
func (v *axTree) tableOf(id platform.AccessID) platform.AccessID {
	for a := v.up[id]; a != 0 && a != v.root(); a = v.up[a] {
		if n := v.node(a); n != nil && tableLike(n.Role) {
			return a
		}
	}
	return 0
}

// index returns AXIndex: a row's index among its table's rows, else an
// item's position in its set from 0.
func (v *axTree) index(id platform.AccessID) (int, bool) {
	n := v.node(id)
	if n == nil {
		return 0, false
	}
	if n.Role == "row" || n.Role == "treeitem" {
		if t := v.tableOf(id); t != 0 {
			if i := slices.Index(v.rows(t), id); i >= 0 {
				return i, true
			}
		}
	}
	if n.PosInSet > 0 {
		return n.PosInSet - 1, true
	}
	return 0, false
}

// selectedChildren returns the selected items of a container (AXSelected
// Children): the exposed nodes below id with aria-selected true, in tree
// order.
func (v *axTree) selectedChildren(id platform.AccessID) []platform.AccessID {
	var out []platform.AccessID
	v.walk(id, func(c *platform.AccessNode) bool {
		if c.Selected == platform.ToggleTrue {
			out = append(out, c.ID)
		}
		return true
	})
	return out
}

// --- roles ---

// axRoles maps ARIA roles onto AXRole and AXSubrole after W3C Core-AAM
// 1.2 (the AXAPI column) and HTML-AAM, as Chromium and AccessKit do.
// Roles missing here are AXGroup.
var axRoles = map[string][2]string{
	"alert":            {"AXGroup", "AXApplicationAlert"},
	"alertdialog":      {"AXGroup", "AXApplicationAlertDialog"},
	"application":      {"AXGroup", "AXLandmarkApplication"},
	"article":          {"AXGroup", "AXDocumentArticle"},
	"banner":           {"AXGroup", "AXLandmarkBanner"},
	"button":           {"AXButton", ""},
	"cell":             {"AXCell", ""},
	"checkbox":         {"AXCheckBox", ""},
	"code":             {"AXGroup", "AXCodeStyleGroup"},
	"columnheader":     {"AXCell", ""},
	"combobox":         {"AXComboBox", ""},
	"complementary":    {"AXGroup", "AXLandmarkComplementary"},
	"contentinfo":      {"AXGroup", "AXLandmarkContentInfo"},
	"definition":       {"AXGroup", "AXDefinition"},
	"deletion":         {"AXGroup", "AXDeleteStyleGroup"},
	"dialog":           {"AXGroup", "AXApplicationDialog"},
	"document":         {"AXGroup", "AXDocument"},
	"emphasis":         {"AXGroup", "AXEmphasisStyleGroup"},
	"form":             {"AXGroup", "AXLandmarkForm"},
	"grid":             {"AXTable", ""},
	"gridcell":         {"AXCell", ""},
	"heading":          {"AXHeading", ""},
	"image":            {"AXImage", ""},
	"img":              {"AXImage", ""},
	"insertion":        {"AXGroup", "AXInsertStyleGroup"},
	"link":             {"AXLink", ""},
	"list":             {"AXList", "AXContentList"},
	"listbox":          {"AXList", ""},
	"log":              {"AXGroup", "AXApplicationLog"},
	"main":             {"AXGroup", "AXLandmarkMain"},
	"marquee":          {"AXGroup", "AXApplicationMarquee"},
	"math":             {"AXGroup", "AXDocumentMath"},
	"menu":             {"AXMenu", ""},
	"menubar":          {"AXMenuBar", ""},
	"menuitem":         {"AXMenuItem", ""},
	"menuitemcheckbox": {"AXMenuItem", ""},
	"menuitemradio":    {"AXMenuItem", ""},
	"meter":            {"AXLevelIndicator", ""},
	"navigation":       {"AXGroup", "AXLandmarkNavigation"},
	"note":             {"AXGroup", "AXDocumentNote"},
	"option":           {"AXStaticText", ""},
	"progressbar":      {"AXProgressIndicator", ""},
	"radio":            {"AXRadioButton", ""},
	"radiogroup":       {"AXRadioGroup", ""},
	"region":           {"AXGroup", "AXLandmarkRegion"},
	"row":              {"AXRow", ""},
	"rowheader":        {"AXCell", ""},
	"scrollbar":        {"AXScrollBar", ""},
	"search":           {"AXGroup", "AXLandmarkSearch"},
	"searchbox":        {"AXTextField", "AXSearchField"},
	"separator":        {"AXSplitter", ""},
	"slider":           {"AXSlider", ""},
	"spinbutton":       {"AXIncrementor", ""},
	"status":           {"AXGroup", "AXApplicationStatus"},
	"strong":           {"AXGroup", "AXStrongStyleGroup"},
	"switch":           {"AXCheckBox", "AXSwitch"},
	"tab":              {"AXRadioButton", "AXTabButton"},
	"table":            {"AXTable", ""},
	"tablist":          {"AXTabGroup", ""},
	"tabpanel":         {"AXGroup", "AXTabPanel"},
	"term":             {"AXGroup", "AXTerm"},
	"textbox":          {"AXTextField", ""},
	"time":             {"AXGroup", "AXTimeGroup"},
	"timer":            {"AXGroup", "AXApplicationTimer"},
	"toolbar":          {"AXToolbar", ""},
	"tooltip":          {"AXGroup", "AXUserInterfaceTooltip"},
	"tree":             {"AXOutline", ""},
	"treegrid":         {"AXTable", ""},
	"treeitem":         {"AXRow", "AXOutlineRow"},
}

// axRole returns the AXRole and AXSubrole ("" for none) of n. Beyond the
// table: a text is AXStaticText; a toggle button (aria-pressed) is a
// checkbox with the AXToggle subrole, and a button with a menu popup an
// AXPopUpButton (Core-AAM); a multi-line text field is an AXTextArea, a
// password field an AXSecureTextField.
func axRole(n *platform.AccessNode) (role, subrole string) {
	switch n.Kind {
	case platform.AccessText:
		return "AXStaticText", ""
	case platform.AccessRoot:
		return "AXGroup", ""
	}
	switch n.Role {
	case "button":
		if n.Pressed != platform.ToggleUnset {
			return "AXCheckBox", "AXToggle"
		}
		if n.HasPopup == "menu" {
			return "AXPopUpButton", ""
		}
	case "textbox":
		switch {
		case n.Has(platform.AccessProtected):
			return "AXTextField", "AXSecureTextField"
		case n.Has(platform.AccessMultiline):
			return "AXTextArea", ""
		}
	}
	if r, ok := axRoles[n.Role]; ok {
		return r[0], r[1]
	}
	return "AXGroup", ""
}

// axRoleDescriptions are the AXRoleDescriptions of the roles whose AXRole
// and AXSubrole AppKit's NSAccessibilityRoleDescription does not describe
// (it says "group" for every web subrole), in the words of WebKit and
// Chromium. English only: AppKit has no localized strings for them.
var axRoleDescriptions = map[string]string{
	"alert":         "alert",
	"alertdialog":   "alert dialog",
	"application":   "application",
	"article":       "article",
	"banner":        "banner",
	"complementary": "complementary",
	"contentinfo":   "content information",
	"definition":    "definition",
	"deletion":      "deletion",
	"dialog":        "dialog",
	"document":      "document",
	"figure":        "figure",
	"form":          "form",
	"heading":       "heading",
	"insertion":     "insertion",
	"log":           "log",
	"main":          "main",
	"marquee":       "marquee",
	"math":          "math",
	"navigation":    "navigation",
	"note":          "note",
	"region":        "region",
	"search":        "search",
	"separator":     "separator",
	"status":        "status",
	"switch":        "switch",
	"tab":           "tab",
	"tabpanel":      "tab panel",
	"term":          "term",
	"timer":         "timer",
	"tooltip":       "tooltip",
}

// axRoleDescription returns the role description of n when the bridge
// gives its own, "" when AppKit's (NSAccessibilityRoleDescription of the
// role and subrole) is right.
func axRoleDescription(n *platform.AccessNode) string {
	if n.Kind != platform.AccessElement {
		return ""
	}
	return axRoleDescriptions[n.Role]
}

// childrenPresentational reports whether the children of n are not
// exposed: ARIA's "Children Presentational" roles (their name already
// says what the children show), and text fields, whose text is their
// value.
func childrenPresentational(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement {
		return false
	}
	switch n.Role {
	case "button", "checkbox", "img", "image", "math", "menuitemcheckbox", "menuitemradio", "meter",
		"progressbar", "radio", "scrollbar", "separator", "slider", "switch", "tab",
		"textbox", "searchbox":
		return true
	}
	return false
}

// tableLike reports whether role has rows (AXRows).
func tableLike(role string) bool {
	switch role {
	case "table", "grid", "treegrid", "tree":
		return true
	}
	return false
}

// selectContainer reports whether role holds selectable items
// (AXSelectedChildren).
func selectContainer(role string) bool {
	switch role {
	case "listbox", "tablist", "tree", "treegrid", "grid", "table", "menu", "menubar", "radiogroup":
		return true
	}
	return false
}

// rangeRole reports whether role has a numeric value (AXMinValue,
// AXMaxValue).
func rangeRole(role string) bool {
	switch role {
	case "slider", "progressbar", "scrollbar", "spinbutton", "meter":
		return true
	}
	return false
}

// toggleRole reports whether the AXValue of role is its aria-checked.
func toggleRole(role string) bool {
	switch role {
	case "checkbox", "switch", "radio", "menuitemcheckbox", "menuitemradio":
		return true
	}
	return false
}

// textField reports whether n is an editable text field: its Value is the
// text of the NSAccessibility text methods.
func textField(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement {
		return false
	}
	switch n.Role {
	case "textbox", "searchbox":
		return true
	case "combobox":
		return n.HasSelection
	}
	return false
}

// axText returns the text the NSAccessibility text methods read for n: a
// text field's value or a text's text.
func axText(n *platform.AccessNode) (string, bool) {
	switch {
	case n.Kind == platform.AccessText:
		return n.Text, true
	case textField(n):
		return n.Value, true
	}
	return "", false
}

// --- values ---

// axValueKind is the type of an AXValue.
type axValueKind uint8

const (
	axNoValue axValueKind = iota
	axStringValue
	axNumberValue
)

// axValue is an AXValue: a string or a number (an NSNumber).
type axValue struct {
	kind axValueKind
	str  string
	num  float64
}

// toggleNumber is the AXValue of a tri-state: 0 off, 1 on, 2 mixed.
func toggleNumber(t platform.AccessToggle) float64 {
	switch t {
	case platform.ToggleTrue:
		return 1
	case platform.ToggleMixed:
		return 2
	}
	return 0
}

// axValueOf returns the AXValue of n: a text's text, a text field's value
// (also when empty), a toggle's state (aria-checked, a toggle button's
// aria-pressed, a tab's aria-selected), a heading's level, a range
// widget's number, or another element's aria-valuetext.
func axValueOf(n *platform.AccessNode) axValue {
	switch {
	case n.Kind == platform.AccessText:
		return axValue{kind: axStringValue, str: n.Text}
	case n.Kind != platform.AccessElement:
		return axValue{}
	case textField(n):
		return axValue{kind: axStringValue, str: n.Value}
	case toggleRole(n.Role):
		return axValue{kind: axNumberValue, num: toggleNumber(n.Checked)}
	case n.Role == "button" && n.Pressed != platform.ToggleUnset:
		return axValue{kind: axNumberValue, num: toggleNumber(n.Pressed)}
	case n.Role == "tab":
		return axValue{kind: axNumberValue, num: toggleNumber(n.Selected)}
	case n.Role == "heading" && n.Level > 0:
		return axValue{kind: axNumberValue, num: float64(n.Level)}
	case rangeRole(n.Role) && n.Range.HasNow:
		return axValue{kind: axNumberValue, num: n.Range.Now}
	case n.Value != "":
		return axValue{kind: axStringValue, str: n.Value}
	}
	return axValue{}
}

// axValueDescription returns AXValueDescription: a range widget's
// aria-valuetext ("" when Value is only the number).
func axValueDescription(n *platform.AccessNode) string {
	if !rangeRole(n.Role) || n.Value == "" {
		return ""
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(n.Value), 64); err == nil && n.Range.HasNow && f == n.Range.Now {
		return ""
	}
	return n.Value
}

// --- text ranges ---

// NSAccessibility counts text in UTF-16 code units; AccessSelection in
// code points.

// u16Len returns the length of s in UTF-16 code units.
func u16Len(s string) int { return utf16x.Len(s) }

// cpToU16 returns the UTF-16 offset of the code point offset cp in s
// (clamped to s).
func cpToU16(s string, cp int) int {
	u := 0
	for _, r := range s {
		if cp <= 0 {
			break
		}
		cp--
		if r >= 0x10000 {
			u += 2
		} else {
			u++
		}
	}
	return u
}

// u16ToCP returns the code point offset of the UTF-16 offset u in s: an
// offset inside a surrogate pair counts the characters before it;
// offsets are clamped to s.
func u16ToCP(s string, u int) int {
	return utf8.RuneCountInString(s[:utf16x.ToByte(s, u)])
}

// u16Slice returns the text of s from UTF-16 offset loc, n units long,
// clamped to s; offsets inside a surrogate pair move to its start.
func u16Slice(s string, loc, n int) string {
	if loc < 0 || n < 0 {
		return ""
	}
	a := utf16x.ToByte(s, loc)
	b := utf16x.ToByte(s, loc+n)
	if b < a {
		return ""
	}
	return s[a:b]
}

// selectionU16 returns the selection of a text field as an NSRange in
// UTF-16 (location, length); false when it has none.
func selectionU16(n *platform.AccessNode) (loc, length int, ok bool) {
	text, isText := axText(n)
	if !isText || !n.HasSelection {
		return 0, 0, false
	}
	a, f := cpToU16(text, n.Selection.Anchor), cpToU16(text, n.Selection.Focus)
	return min(a, f), max(a, f) - min(a, f), true
}

// The tree carries no layout lines, so lines are hard lines: the text
// between line feeds, each line's range ending after its line feed
// (NSTextView's convention). A field that wraps a long line reports it
// as one line.

// lineForIndex returns the line of the UTF-16 offset u in s (offsets past
// the end are on the last line).
func lineForIndex(s string, u int) int {
	b := utf16x.ToByte(s, u)
	return strings.Count(s[:b], "\n")
}

// rangeForLine returns the UTF-16 range of line in s; false when s has no
// such line.
func rangeForLine(s string, line int) (loc, n int, ok bool) {
	if line < 0 {
		return 0, 0, false
	}
	start := 0
	for i := 0; i < line; i++ {
		j := strings.IndexByte(s[start:], '\n')
		if j < 0 {
			return 0, 0, false
		}
		start += j + 1
	}
	end := len(s)
	if j := strings.IndexByte(s[start:], '\n'); j >= 0 {
		end = start + j + 1
	}
	loc = utf16x.Len(s[:start])
	return loc, utf16x.Len(s[start:end]), true
}

// --- which methods apply ---

// The NSAccessibility methods whose answer depends on the node:
// isAccessibilitySelectorAllowed: answers from axAllows for these, so
// that a button does not claim to be a text field or a slider
// (TelaAccessElement implements all of them).
const (
	axSelPress            = "accessibilityPerformPress"
	axSelIncrement        = "accessibilityPerformIncrement"
	axSelDecrement        = "accessibilityPerformDecrement"
	axSelShowMenu         = "accessibilityPerformShowMenu"
	axSelSetFocused       = "setAccessibilityFocused:"
	axSelSetValue         = "setAccessibilityValue:"
	axSelSetSelectedRange = "setAccessibilitySelectedTextRange:"
	axSelSetExpanded      = "setAccessibilityExpanded:"
	axSelSetDisclosed     = "setAccessibilityDisclosed:"
	axSelSetSelected      = "setAccessibilitySelected:"
	axSelNumChars         = "accessibilityNumberOfCharacters"
	axSelSelectedText     = "accessibilitySelectedText"
	axSelSelectedRange    = "accessibilitySelectedTextRange"
	axSelStringForRange   = "accessibilityStringForRange:"
	axSelLineForIndex     = "accessibilityLineForIndex:"
	axSelRangeForLine     = "accessibilityRangeForLine:"
	axSelInsertionLine    = "accessibilityInsertionPointLineNumber"
	axSelVisibleRange     = "accessibilityVisibleCharacterRange"
	axSelFrameForRange    = "accessibilityFrameForRange:"
	axSelMinValue         = "accessibilityMinValue"
	axSelMaxValue         = "accessibilityMaxValue"
	axSelValueDescription = "accessibilityValueDescription"
	axSelExpanded         = "isAccessibilityExpanded"
	axSelDisclosed        = "isAccessibilityDisclosed"
	axSelDisclosureLevel  = "accessibilityDisclosureLevel"
	axSelSelected         = "isAccessibilitySelected"
	axSelSelectedChildren = "accessibilitySelectedChildren"
	axSelRows             = "accessibilityRows"
	axSelIndex            = "accessibilityIndex"
	axSelURL              = "accessibilityURL"
	axSelPlaceholder      = "accessibilityPlaceholderValue"
	axSelOrientation      = "accessibilityOrientation"
	axSelValue            = "accessibilityValue"
	axSelLabel            = "accessibilityLabel"
	axSelHelp             = "accessibilityHelp"
	axSelRoleDescription  = "accessibilityRoleDescription"
	axSelRequired         = "isAccessibilityRequired"
)

// axAllows reports whether the method sel applies to the exposed node n
// of v (allowed), when known; known is false for the methods that apply
// to every element (NSAccessibilityElement decides).
func axAllows(v *axTree, n *platform.AccessNode, sel string) (allowed, known bool) {
	has := n.Actions.Has
	_, isText := axText(n)
	switch sel {
	case axSelPress:
		return has(platform.AccessActionDefault), true
	case axSelIncrement:
		return has(platform.AccessActionIncrement), true
	case axSelDecrement:
		return has(platform.AccessActionDecrement), true
	case axSelShowMenu:
		return has(platform.AccessActionShowMenu), true
	case axSelSetFocused:
		return has(platform.AccessActionFocus), true
	case axSelSetValue:
		return textField(n) && has(platform.AccessActionSetValue) && !n.Has(platform.AccessReadOnly), true
	case axSelSetSelectedRange:
		return isText && has(platform.AccessActionSetSelection), true
	case axSelSetExpanded, axSelSetDisclosed:
		if sel == axSelSetDisclosed && n.Role != "treeitem" {
			return false, true
		}
		return n.Expanded != platform.ToggleUnset && (has(platform.AccessActionExpand) || has(platform.AccessActionCollapse)), true
	case axSelSetSelected:
		return n.Selected != platform.ToggleUnset && has(platform.AccessActionDefault), true
	case axSelNumChars, axSelSelectedText, axSelSelectedRange, axSelStringForRange, axSelLineForIndex,
		axSelRangeForLine, axSelInsertionLine, axSelVisibleRange, axSelFrameForRange:
		return isText, true
	case axSelMinValue:
		return rangeRole(n.Role) && n.Range.HasMin, true
	case axSelMaxValue:
		return rangeRole(n.Role) && n.Range.HasMax, true
	case axSelValueDescription:
		return axValueDescription(n) != "", true
	case axSelExpanded:
		return n.Expanded != platform.ToggleUnset, true
	case axSelDisclosed, axSelDisclosureLevel:
		return n.Role == "treeitem", true
	case axSelSelected:
		return n.Selected != platform.ToggleUnset, true
	case axSelSelectedChildren:
		return selectContainer(n.Role), true
	case axSelRows:
		return tableLike(n.Role), true
	case axSelIndex:
		_, ok := v.index(n.ID)
		return ok, true
	case axSelURL:
		return n.URL != "", true
	case axSelPlaceholder:
		return n.Placeholder != "", true
	case axSelOrientation:
		return n.Orientation != "", true
	case axSelValue:
		return axValueOf(n).kind != axNoValue, true
	case axSelLabel:
		return n.Name != "", true
	case axSelHelp:
		return n.Description != "", true
	case axSelRequired:
		return n.Kind == platform.AccessElement, true
	}
	return false, false
}

// --- the attribute API ---

// The AX server reads elements through the attribute API of the older
// NSAccessibility informal protocol (accessibilityAttributeNames,
// accessibilityAttributeValue: and the parameterized and settable
// variants). AppKit's own bridge from it to the methods of the
// NSAccessibility protocol does not see TelaAccessElement's methods (on
// macOS 26 it answered nil for AXRole and every other attribute but
// AXWindow, and an AX client saw no element), so the element implements
// the attribute API itself, mapping each attribute onto its own method
// (Chromium's and WebKit's elements implement this API too).

// axAttrKind says how an attribute's value comes from its method.
type axAttrKind uint8

const (
	attrObject      axAttrKind = iota // the method returns the object
	attrBool                          // a BOOL, boxed in an NSNumber
	attrInt                           // an NSInteger, boxed
	attrRange                         // an NSRange, in an NSValue
	attrRect                          // a CGRect, in an NSValue
	attrPosition                      // accessibilityFrame's origin, in an NSValue
	attrSize                          // accessibilityFrame's size, in an NSValue
	attrParent                        // accessibilityParent, its unignored ancestor
	attrOrientation                   // accessibilityOrientation as AXOrientation's string
)

// axAttr is one attribute: its name, the method that answers it, how,
// and the method that sets it ("" when it cannot be set).
type axAttr struct {
	name, sel string
	kind      axAttrKind
	set       string
}

// axAttrs are the attributes of TelaAccessElement, in the order
// accessibilityAttributeNames lists them.
var axAttrs = []axAttr{
	{"AXRole", "accessibilityRole", attrObject, ""},
	{"AXSubrole", "accessibilitySubrole", attrObject, ""},
	{"AXRoleDescription", axSelRoleDescription, attrObject, ""},
	{"AXDescription", axSelLabel, attrObject, ""},
	{"AXHelp", axSelHelp, attrObject, ""},
	{"AXValue", axSelValue, attrObject, axSelSetValue},
	{"AXValueDescription", axSelValueDescription, attrObject, ""},
	{"AXMinValue", axSelMinValue, attrObject, ""},
	{"AXMaxValue", axSelMaxValue, attrObject, ""},
	{"AXPlaceholderValue", axSelPlaceholder, attrObject, ""},
	{"AXEnabled", "isAccessibilityEnabled", attrBool, ""},
	{"AXFocused", "isAccessibilityFocused", attrBool, axSelSetFocused},
	{"AXRequired", axSelRequired, attrBool, ""},
	{"AXExpanded", axSelExpanded, attrBool, axSelSetExpanded},
	{"AXDisclosing", axSelDisclosed, attrBool, axSelSetDisclosed},
	{"AXDisclosureLevel", axSelDisclosureLevel, attrInt, ""},
	{"AXSelected", axSelSelected, attrBool, axSelSetSelected},
	{"AXModal", "isAccessibilityModal", attrBool, ""},
	{"AXParent", "accessibilityParent", attrParent, ""},
	{"AXChildren", "accessibilityChildren", attrObject, ""},
	{"AXSelectedChildren", axSelSelectedChildren, attrObject, ""},
	{"AXRows", axSelRows, attrObject, ""},
	{"AXIndex", axSelIndex, attrInt, ""},
	{"AXWindow", "accessibilityWindow", attrObject, ""},
	{"AXTopLevelUIElement", "accessibilityTopLevelUIElement", attrObject, ""},
	{"AXPosition", "accessibilityFrame", attrPosition, ""},
	{"AXSize", "accessibilityFrame", attrSize, ""},
	{"AXFrame", "accessibilityFrame", attrRect, ""},
	{"AXURL", axSelURL, attrObject, ""},
	{"AXOrientation", axSelOrientation, attrOrientation, ""},
	{"AXNumberOfCharacters", axSelNumChars, attrInt, ""},
	{"AXSelectedText", axSelSelectedText, attrObject, ""},
	{"AXSelectedTextRange", axSelSelectedRange, attrRange, axSelSetSelectedRange},
	{"AXInsertionPointLineNumber", axSelInsertionLine, attrInt, ""},
	{"AXVisibleCharacterRange", axSelVisibleRange, attrRange, ""},
}

// axParamAttrs are the parameterized attributes of text, each answered
// by the method named (AXAttributedStringForRange by
// accessibilityStringForRange:, without attributes; AXRangeForIndex by
// the bridge itself).
var axParamAttrs = []axAttr{
	{"AXStringForRange", axSelStringForRange, attrObject, ""},
	{"AXAttributedStringForRange", axSelStringForRange, attrObject, ""},
	{"AXLineForIndex", axSelLineForIndex, attrInt, ""},
	{"AXRangeForLine", axSelRangeForLine, attrRange, ""},
	{"AXBoundsForRange", axSelFrameForRange, attrRect, ""},
	{"AXRangeForIndex", axSelNumChars, attrRange, ""},
}

// axAttrApplies reports whether the attribute a applies to node n: its
// method is allowed (axAllows), and a subrole exists.
func axAttrApplies(v *axTree, n *platform.AccessNode, a axAttr) bool {
	if a.name == "AXSubrole" {
		_, s := axRole(n)
		return s != ""
	}
	if a.name == "AXModal" {
		return n.Has(platform.AccessModal)
	}
	allowed, known := axAllows(v, n, a.sel)
	return !known || allowed
}

// axAttributeNames returns the attribute names of n.
func axAttributeNames(v *axTree, n *platform.AccessNode) []string {
	var out []string
	for _, a := range axAttrs {
		if axAttrApplies(v, n, a) {
			out = append(out, a.name)
		}
	}
	return out
}

// axParamAttributeNames returns the parameterized attribute names of n:
// those of text, for a text field or a text.
func axParamAttributeNames(n *platform.AccessNode) []string {
	if _, ok := axText(n); !ok {
		return nil
	}
	out := make([]string, len(axParamAttrs))
	for i, a := range axParamAttrs {
		out[i] = a.name
	}
	return out
}

// axFindAttr returns the attribute called name among attrs.
func axFindAttr(attrs []axAttr, name string) (axAttr, bool) {
	for _, a := range attrs {
		if a.name == name {
			return a, true
		}
	}
	return axAttr{}, false
}

// axSettable reports whether the attribute name of n can be set: it has
// a setter that applies to n.
func axSettable(v *axTree, n *platform.AccessNode, name string) bool {
	a, ok := axFindAttr(axAttrs, name)
	if !ok || a.set == "" {
		return false
	}
	allowed, known := axAllows(v, n, a.set)
	return known && allowed
}

// axOrientationValue is the AXOrientation string of n.
func axOrientationValue(n *platform.AccessNode) string {
	switch n.Orientation {
	case "vertical":
		return "AXVerticalOrientation"
	case "horizontal":
		return "AXHorizontalOrientation"
	}
	return "AXUnknownOrientation"
}

// u16CharRange returns the UTF-16 range of the character at UTF-16
// offset u of s (AXRangeForIndex): two units for a surrogate pair; false
// past the end.
func u16CharRange(s string, u int) (loc, n int, ok bool) {
	if u < 0 || u >= u16Len(s) {
		return 0, 0, false
	}
	loc = cpToU16(s, u16ToCP(s, u))
	n = 1
	if r, _ := utf8.DecodeRuneInString(s[utf16x.ToByte(s, loc):]); r >= 0x10000 {
		n = 2
	}
	return loc, n, true
}

// --- coordinates off the main thread ---

// screenRectIn converts r, window logical pixels (points, y down) of a
// view whose bounds are at (vx, vy) with height vh in screen coordinates
// (points, y up), to screen coordinates.
func screenRectIn(vx, vy, vh float64, r platform.Rect) (x, y, w, h float64) {
	return vx + float64(r.X), vy + vh - float64(r.Y) - float64(r.H), float64(r.W), float64(r.H)
}

// viewPointIn converts the screen point (px, py) to the view's
// coordinates, the inverse of screenRectIn.
func viewPointIn(vx, vy, vh, px, py float64) (x, y float32) {
	return float32(px - vx), float32(vy + vh - py)
}

// --- name-based actions ---

// axActionTable maps the AX action names of the name-based API
// (accessibilityActionNames, accessibilityPerformAction:) onto the
// tree's actions, with English descriptions for the ones AppKit's
// NSAccessibilityActionDescription does not know.
var axActionTable = []struct {
	name   string
	action platform.AccessAction
	desc   string
}{
	{"AXPress", platform.AccessActionDefault, "press"},
	{"AXIncrement", platform.AccessActionIncrement, "increment"},
	{"AXDecrement", platform.AccessActionDecrement, "decrement"},
	{"AXShowMenu", platform.AccessActionShowMenu, "show menu"},
	{axScrollToVisible, platform.AccessActionScrollIntoView, "scroll to visible"},
}

// axScrollToVisible is the AX action of scrolling into view, which has
// no method of its own in the NSAccessibility protocol.
const axScrollToVisible = "AXScrollToVisible"

// axActionNames returns the AX action names of n's actions.
func axActionNames(n *platform.AccessNode) []string {
	var out []string
	for _, a := range axActionTable {
		if n.Actions.Has(a.action) {
			out = append(out, a.name)
		}
	}
	return out
}

// axActionNamed returns the action of an AX action name.
func axActionNamed(name string) (platform.AccessAction, bool) {
	for _, a := range axActionTable {
		if a.name == name {
			return a.action, true
		}
	}
	return 0, false
}

// axActionDescription returns the English description of an AX action
// name (the name itself when unknown).
func axActionDescription(name string) string {
	for _, a := range axActionTable {
		if a.name == name {
			return a.desc
		}
	}
	return name
}

// --- notifications ---

// axNoteKind is an NSAccessibility notification the bridge posts.
type axNoteKind uint8

const (
	noteDestroyed axNoteKind = iota
	noteLayout
	noteRowCount
	noteValue
	noteTitle
	noteSelectedText
	noteSelectedChildren
	noteExpanded
	noteRowExpanded
	noteRowCollapsed
	noteFocus
	noteAnnounce

	noteKindCount
)

// axNoteNames are the notifications' AppKit constants and their values
// (stable strings of the AX API, used when AppKit lacks a constant; the
// expanded one has no AppKit constant: WebKit's and Chromium's name).
var axNoteNames = [noteKindCount][2]string{
	{"NSAccessibilityUIElementDestroyedNotification", "AXUIElementDestroyed"},
	{"NSAccessibilityLayoutChangedNotification", "AXLayoutChanged"},
	{"NSAccessibilityRowCountChangedNotification", "AXRowCountChanged"},
	{"NSAccessibilityValueChangedNotification", "AXValueChanged"},
	{"NSAccessibilityTitleChangedNotification", "AXTitleChanged"},
	{"NSAccessibilitySelectedTextChangedNotification", "AXSelectedTextChanged"},
	{"NSAccessibilitySelectedChildrenChangedNotification", "AXSelectedChildrenChanged"},
	{"", "AXExpandedChanged"},
	{"NSAccessibilityRowExpandedNotification", "AXRowExpanded"},
	{"NSAccessibilityRowCollapsedNotification", "AXRowCollapsed"},
	{"NSAccessibilityFocusedUIElementChangedNotification", "AXFocusedUIElementChanged"},
	{"NSAccessibilityAnnouncementRequestedNotification", "AXAnnouncementRequested"},
}

func (k axNoteKind) String() string {
	if k < noteKindCount {
		return axNoteNames[k][1]
	}
	return "AXNote(" + strconv.Itoa(int(k)) + ")"
}

// axNote is one notification: its kind, the node it is about (0: the
// window, for the layout, a focus on no element and announcements) and
// an announcement's text and priority.
type axNote struct {
	kind      axNoteKind
	id        platform.AccessID
	text      string
	assertive bool
}

func (n axNote) String() string {
	s := n.kind.String()
	if n.id != 0 {
		s += " " + strconv.FormatUint(uint64(n.id), 10)
	}
	if n.kind == noteAnnounce {
		s += " " + strconv.Quote(n.text)
		if n.assertive {
			s += " high"
		} else {
			s += " medium"
		}
	}
	return s
}

// axNotes returns the notifications that update u, which turned the tree
// old (nil: the first update since the activation) into cur, posts, in
// order: the elements destroyed (exposed in old, not in cur), one layout
// change when the exposed structure changed (or for the first tree,
// which AppKit asked for and got empty), the row count changes of
// tables, then for each changed node that is exposed in both its value,
// title, text selection, the selection of its container and its expanded
// state, then the focus when it moved, and the announcements.
func axNotes(old, cur *axTree, u *platform.AccessUpdate) []axNote {
	var notes []axNote
	announce := func() []axNote {
		for _, a := range u.Announcements {
			if strings.TrimSpace(a.Text) != "" {
				notes = append(notes, axNote{kind: noteAnnounce, text: a.Text, assertive: a.Assertive})
			}
		}
		return notes
	}
	if old == nil {
		notes = append(notes, axNote{kind: noteLayout})
		return announce()
	}
	var destroyed []platform.AccessID
	for id := range old.up {
		if !cur.exposed(id) {
			destroyed = append(destroyed, id)
		}
	}
	slices.Sort(destroyed)
	for _, id := range destroyed {
		notes = append(notes, axNote{kind: noteDestroyed, id: id})
	}
	layout := len(destroyed) > 0 || old.root() != cur.root()
	checked := map[platform.AccessID]bool{}
	for i := range u.Nodes {
		id := u.Nodes[i].ID
		if !old.exposed(id) && cur.exposed(id) {
			layout = true
		}
		a := cur.exposedAncestor(id)
		if !checked[a] {
			checked[a] = true
			if !slices.Equal(old.kids[a], cur.kids[a]) {
				layout = true
			}
		}
	}
	if layout {
		notes = append(notes, axNote{kind: noteLayout})
		var tables []platform.AccessID
		cur.walk(cur.root(), func(n *platform.AccessNode) bool {
			if tableLike(n.Role) {
				tables = append(tables, n.ID)
			}
			return true
		})
		for _, t := range tables {
			if old.exposed(t) && len(old.rows(t)) != len(cur.rows(t)) {
				notes = append(notes, axNote{kind: noteRowCount, id: t})
			}
		}
	}
	containers := map[platform.AccessID]bool{}
	for i := range u.Nodes {
		n := &u.Nodes[i]
		o := old.node(n.ID)
		if o == nil || !cur.exposed(n.ID) {
			continue
		}
		if axValueOf(o) != axValueOf(n) || o.Value != n.Value || o.Range != n.Range {
			notes = append(notes, axNote{kind: noteValue, id: n.ID})
		}
		if o.Name != n.Name {
			notes = append(notes, axNote{kind: noteTitle, id: n.ID})
		}
		if _, isText := axText(n); isText && (o.Selection != n.Selection || o.HasSelection != n.HasSelection) {
			notes = append(notes, axNote{kind: noteSelectedText, id: n.ID})
		}
		if o.Selected != n.Selected {
			if c := cur.up[n.ID]; c != cur.root() && !containers[c] {
				containers[c] = true
				notes = append(notes, axNote{kind: noteSelectedChildren, id: c})
			}
		}
		if o.Expanded != n.Expanded {
			switch {
			case (n.Role == "treeitem" || n.Role == "row") && n.Expanded == platform.ToggleTrue:
				notes = append(notes, axNote{kind: noteRowExpanded, id: n.ID})
			case (n.Role == "treeitem" || n.Role == "row") && o.Expanded == platform.ToggleTrue:
				notes = append(notes, axNote{kind: noteRowCollapsed, id: n.ID})
			default:
				notes = append(notes, axNote{kind: noteExpanded, id: n.ID})
			}
		}
		if o.RowCount != n.RowCount && tableLike(n.Role) && !slices.Contains(notes, axNote{kind: noteRowCount, id: n.ID}) {
			notes = append(notes, axNote{kind: noteRowCount, id: n.ID})
		}
	}
	if old.tree.Focus() != cur.tree.Focus() {
		f := cur.focus()
		if f == cur.root() {
			f = 0
		}
		notes = append(notes, axNote{kind: noteFocus, id: f})
	}
	return announce()
}
