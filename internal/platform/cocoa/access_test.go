package cocoa

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// Tests of the OS-independent half of the NSAccessibility bridge
// (access.go): exposure, roles, values, text ranges, the method gate and
// the notifications of an update.

func applyTree(t *testing.T, prev *platform.AccessTree, u platform.AccessUpdate) *platform.AccessTree {
	t.Helper()
	next, _, err := prev.Apply(&u)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return next
}

func el(id platform.AccessID, role, name string, kids ...platform.AccessID) platform.AccessNode {
	return platform.AccessNode{ID: id, Role: role, Name: name, Children: kids}
}

func txt(id platform.AccessID, s string) platform.AccessNode {
	return platform.AccessNode{ID: id, Kind: platform.AccessText, Text: s}
}

// sampleNodes is a small window: a heading with its text, a layout div
// (ignored) holding a button with a text child and a checkbox, a text
// field and a list with two options.
func sampleNodes() []platform.AccessNode {
	root := platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2, 4, 8, 9}}
	heading := el(2, "heading", "Settings", 3)
	heading.Level = 1
	heading.Bounds = platform.Rect{X: 10, Y: 10, W: 180, H: 24}
	div := el(4, "generic", "", 5, 7)
	div.Bounds = platform.Rect{X: 0, Y: 40, W: 200, H: 40}
	button := el(5, "button", "Save", 6)
	button.Actions = platform.AccessActionsOf(platform.AccessActionDefault, platform.AccessActionFocus)
	button.States = platform.AccessFocusable
	button.Bounds = platform.Rect{X: 10, Y: 40, W: 80, H: 30}
	check := el(7, "checkbox", "Notify")
	check.Checked = platform.ToggleTrue
	check.Bounds = platform.Rect{X: 100, Y: 40, W: 80, H: 30}
	field := el(8, "textbox", "Email")
	field.Value = "a😀b"
	field.HasSelection, field.Selection = true, platform.AccessSelection{Anchor: 1, Focus: 2}
	field.Actions = platform.AccessActionsOf(platform.AccessActionSetValue, platform.AccessActionSetSelection)
	field.Bounds = platform.Rect{X: 10, Y: 90, W: 180, H: 24}
	list := el(9, "listbox", "Fruit", 10, 11)
	list.Bounds = platform.Rect{X: 10, Y: 120, W: 180, H: 60}
	a, b := el(10, "option", "Apple"), el(11, "option", "Banana")
	a.Selected, b.Selected = platform.ToggleTrue, platform.ToggleFalse
	a.Bounds = platform.Rect{X: 10, Y: 120, W: 180, H: 30}
	b.Bounds = platform.Rect{X: 10, Y: 150, W: 180, H: 30}
	return []platform.AccessNode{root, heading, txt(3, "Settings"), div, button, txt(6, "Save"), check, field, list, a, b}
}

func TestAXTreeExposure(t *testing.T) {
	tree := applyTree(t, nil, platform.AccessUpdate{Nodes: sampleNodes(), Root: 1, Focus: 5})
	v := newAXTree(tree)
	t.Logf("tree:\n%s", tree)
	for _, c := range []struct {
		id   platform.AccessID
		kids []platform.AccessID
	}{
		{1, []platform.AccessID{2, 5, 7, 8, 9}}, // the div is ignored: its children are the root's
		{2, []platform.AccessID{3}},             // the heading's text is a static text
		{5, nil},                                // a button's children are presentational
		{9, []platform.AccessID{10, 11}},
	} {
		t.Logf("kids of %d: %v", c.id, v.kids[c.id])
		if !slices.Equal(v.kids[c.id], c.kids) {
			t.Errorf("kids of %d = %v, want %v", c.id, v.kids[c.id], c.kids)
		}
	}
	for id, want := range map[platform.AccessID]bool{1: false, 2: true, 3: true, 4: false, 5: true, 6: false, 10: true} {
		if got := v.exposed(id); got != want {
			t.Errorf("exposed(%d) = %v, want %v", id, got, want)
		}
	}
	for id, want := range map[platform.AccessID]platform.AccessID{5: 1, 3: 2, 10: 9} {
		if got := v.up[id]; got != want {
			t.Errorf("up[%d] = %d, want %d", id, got, want)
		}
	}
	// The hidden text of the button stands for the button; the div for the root.
	for id, want := range map[platform.AccessID]platform.AccessID{6: 5, 4: 1, 1: 1, 3: 3} {
		if got := v.exposedAncestor(id); got != want {
			t.Errorf("exposedAncestor(%d) = %d, want %d", id, got, want)
		}
	}
	if f := v.focus(); f != 5 {
		t.Errorf("focus = %d, want 5", f)
	}
	for _, c := range []struct {
		x, y float32
		want platform.AccessID
	}{{20, 50, 5}, {150, 50, 7}, {20, 15, 2}, {20, 160, 11}, {195, 5, 0}, {20, 85, 0}} {
		if got := v.hitTest(v.root(), c.x, c.y); got != c.want {
			t.Errorf("hitTest(%g, %g) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
	if got := v.selectedChildren(9); !slices.Equal(got, []platform.AccessID{10}) {
		t.Errorf("selectedChildren(listbox) = %v, want [10]", got)
	}
	empty := newAXTree(nil)
	if empty.root() != 0 || empty.exposed(1) || empty.hitTest(0, 1, 1) != 0 {
		t.Errorf("the empty tree exposes something")
	}
}

func TestAXRoles(t *testing.T) {
	type want struct{ role, subrole, desc string }
	cases := []struct {
		n    platform.AccessNode
		want want
	}{
		{el(1, "button", ""), want{"AXButton", "", ""}},
		{platform.AccessNode{Role: "button", Pressed: platform.ToggleFalse}, want{"AXCheckBox", "AXToggle", ""}},
		{platform.AccessNode{Role: "button", HasPopup: "menu"}, want{"AXPopUpButton", "", ""}},
		{el(1, "checkbox", ""), want{"AXCheckBox", "", ""}},
		{el(1, "switch", ""), want{"AXCheckBox", "AXSwitch", "switch"}},
		{el(1, "textbox", ""), want{"AXTextField", "", ""}},
		{platform.AccessNode{Role: "textbox", States: platform.AccessMultiline}, want{"AXTextArea", "", ""}},
		{platform.AccessNode{Role: "textbox", States: platform.AccessProtected}, want{"AXTextField", "AXSecureTextField", ""}},
		{el(1, "searchbox", ""), want{"AXTextField", "AXSearchField", ""}},
		{el(1, "heading", ""), want{"AXHeading", "", "heading"}},
		{el(1, "link", ""), want{"AXLink", "", ""}},
		{el(1, "img", ""), want{"AXImage", "", ""}},
		{el(1, "slider", ""), want{"AXSlider", "", ""}},
		{el(1, "spinbutton", ""), want{"AXIncrementor", "", ""}},
		{el(1, "progressbar", ""), want{"AXProgressIndicator", "", ""}},
		{el(1, "tab", ""), want{"AXRadioButton", "AXTabButton", "tab"}},
		{el(1, "tablist", ""), want{"AXTabGroup", "", ""}},
		{el(1, "dialog", ""), want{"AXGroup", "AXApplicationDialog", "dialog"}},
		{el(1, "alert", ""), want{"AXGroup", "AXApplicationAlert", "alert"}},
		{el(1, "main", ""), want{"AXGroup", "AXLandmarkMain", "main"}},
		{el(1, "navigation", ""), want{"AXGroup", "AXLandmarkNavigation", "navigation"}},
		{el(1, "list", ""), want{"AXList", "AXContentList", ""}},
		{el(1, "listbox", ""), want{"AXList", "", ""}},
		{el(1, "option", ""), want{"AXStaticText", "", ""}},
		{el(1, "menuitemcheckbox", ""), want{"AXMenuItem", "", ""}},
		{el(1, "tree", ""), want{"AXOutline", "", ""}},
		{el(1, "treeitem", ""), want{"AXRow", "AXOutlineRow", ""}},
		{el(1, "grid", ""), want{"AXTable", "", ""}},
		{el(1, "generic", ""), want{"AXGroup", "", ""}},
		{el(1, "made-up", ""), want{"AXGroup", "", ""}},
		{txt(1, "x"), want{"AXStaticText", "", ""}},
	}
	for _, c := range cases {
		r, s := axRole(&c.n)
		got := want{r, s, axRoleDescription(&c.n)}
		t.Logf("%-16q pressed=%v popup=%q states=[%v] -> %v", c.n.Role, c.n.Pressed, c.n.HasPopup, c.n.States, got)
		if got != c.want {
			t.Errorf("role %q (%+v): got %v, want %v", c.n.Role, c.n, got, c.want)
		}
	}
}

func TestAXValues(t *testing.T) {
	num := func(f float64) axValue { return axValue{kind: axNumberValue, num: f} }
	str := func(s string) axValue { return axValue{kind: axStringValue, str: s} }
	slider := platform.AccessNode{Role: "slider", Value: "30", Range: platform.AccessRange{Now: 30, HasNow: true, Min: 0, HasMin: true}}
	labeled := slider
	labeled.Value = "30 percent"
	cases := []struct {
		name string
		n    platform.AccessNode
		want axValue
		desc string
	}{
		{"text", txt(1, "Hello"), str("Hello"), ""},
		{"empty field", platform.AccessNode{Role: "textbox"}, str(""), ""},
		{"field", platform.AccessNode{Role: "textbox", Value: "abc"}, str("abc"), ""},
		{"checked", platform.AccessNode{Role: "checkbox", Checked: platform.ToggleTrue}, num(1), ""},
		{"mixed", platform.AccessNode{Role: "checkbox", Checked: platform.ToggleMixed}, num(2), ""},
		{"unset checkbox", platform.AccessNode{Role: "checkbox"}, num(0), ""},
		{"pressed", platform.AccessNode{Role: "button", Pressed: platform.ToggleTrue}, num(1), ""},
		{"button", platform.AccessNode{Role: "button"}, axValue{}, ""},
		{"tab", platform.AccessNode{Role: "tab", Selected: platform.ToggleTrue}, num(1), ""},
		{"heading", platform.AccessNode{Role: "heading", Level: 2}, num(2), ""},
		{"slider", slider, num(30), ""},
		{"valuetext", labeled, num(30), "30 percent"},
		{"no now", platform.AccessNode{Role: "progressbar", Value: "loading"}, str("loading"), "loading"},
		{"root", platform.AccessNode{Kind: platform.AccessRoot}, axValue{}, ""},
	}
	for _, c := range cases {
		got, desc := axValueOf(&c.n), axValueDescription(&c.n)
		t.Logf("%-14s -> %+v desc %q", c.name, got, desc)
		if got != c.want || desc != c.desc {
			t.Errorf("%s: got %+v %q, want %+v %q", c.name, got, desc, c.want, c.desc)
		}
	}
}

func TestAXTextRanges(t *testing.T) {
	s := "a😀b\ncd\n"
	if n := u16Len(s); n != 8 {
		t.Errorf("u16Len = %d, want 8", n)
	}
	for cp, want := range []int{0, 1, 3, 4, 5, 6, 7, 8, 8} {
		if got := cpToU16(s, cp); got != want {
			t.Errorf("cpToU16(%d) = %d, want %d", cp, got, want)
		}
	}
	// Offset 2 is inside the surrogate pair: it counts the characters before.
	for u, want := range []int{0, 1, 1, 2, 3, 4, 5, 6, 7, 7} {
		if got := u16ToCP(s, u); got != want {
			t.Errorf("u16ToCP(%d) = %d, want %d", u, got, want)
		}
	}
	for _, c := range []struct {
		loc, n int
		want   string
	}{{1, 2, "😀"}, {0, 4, "a😀b"}, {5, 10, "cd\n"}, {2, 1, "😀"}, {-1, 2, ""}, {9, 1, ""}} {
		if got := u16Slice(s, c.loc, c.n); got != c.want {
			t.Errorf("u16Slice(%d, %d) = %q, want %q", c.loc, c.n, got, c.want)
		}
	}
	for u, want := range []int{0, 0, 0, 0, 0, 1, 1, 1, 2, 2} {
		if got := lineForIndex(s, u); got != want {
			t.Errorf("lineForIndex(%d) = %d, want %d", u, got, want)
		}
	}
	for _, c := range []struct {
		line, loc, n int
		ok           bool
	}{{0, 0, 5, true}, {1, 5, 3, true}, {2, 8, 0, true}, {3, 0, 0, false}, {-1, 0, 0, false}} {
		loc, n, ok := rangeForLine(s, c.line)
		if loc != c.loc || n != c.n || ok != c.ok {
			t.Errorf("rangeForLine(%d) = %d, %d, %v; want %d, %d, %v", c.line, loc, n, ok, c.loc, c.n, c.ok)
		}
	}
	field := platform.AccessNode{Role: "textbox", Value: "a😀b", HasSelection: true, Selection: platform.AccessSelection{Anchor: 3, Focus: 1}}
	if loc, n, ok := selectionU16(&field); loc != 1 || n != 3 || !ok {
		t.Errorf("selectionU16 of a backward selection = %d, %d, %v; want 1, 3, true", loc, n, ok)
	}
	if _, _, ok := selectionU16(&platform.AccessNode{Role: "button"}); ok {
		t.Errorf("a button has a text selection")
	}
}

func TestAXAllows(t *testing.T) {
	tree := applyTree(t, nil, platform.AccessUpdate{Nodes: sampleNodes(), Root: 1, Focus: 5})
	v := newAXTree(tree)
	get := func(id platform.AccessID) *platform.AccessNode { n, _ := tree.Node(id); return n }
	cases := []struct {
		id      platform.AccessID
		sel     string
		allowed bool
	}{
		{5, axSelPress, true},
		{5, axSelSetFocused, true},
		{5, axSelSetValue, false},
		{5, axSelNumChars, false},
		{5, axSelIncrement, false},
		{2, axSelPress, false},
		{2, axSelValue, true}, // the level
		{3, axSelNumChars, true},
		{3, axSelLabel, false},
		{8, axSelSetValue, true},
		{8, axSelSetSelectedRange, true},
		{8, axSelStringForRange, true},
		{8, axSelPlaceholder, false},
		{9, axSelSelectedChildren, true},
		{10, axSelSelected, true},
		{10, axSelIndex, false},
		{7, axSelExpanded, false},
		{7, axSelRequired, true},
	}
	for _, c := range cases {
		allowed, known := axAllows(v, get(c.id), c.sel)
		if !known || allowed != c.allowed {
			t.Errorf("axAllows(%d %s, %s) = %v, %v; want %v, true", c.id, get(c.id).Role, c.sel, allowed, known, c.allowed)
		}
	}
	if _, known := axAllows(v, get(5), "accessibilityRole"); known {
		t.Errorf("accessibilityRole is gated; it applies to every element")
	}
}

func TestAXRowsAndIndex(t *testing.T) {
	root := platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2}}
	tree := el(2, "tree", "Files", 3, 5)
	a := el(3, "treeitem", "src", 4)
	a.Expanded, a.Level = platform.ToggleTrue, 1
	group := el(4, "group", "", 6)
	b := el(5, "treeitem", "docs")
	b.Level, b.PosInSet, b.SetSize = 1, 2, 2
	c := el(6, "treeitem", "main.go")
	c.Level = 2
	tr := applyTree(t, nil, platform.AccessUpdate{Nodes: []platform.AccessNode{root, tree, a, group, b, c}, Root: 1, Focus: 1})
	v := newAXTree(tr)
	rows := v.rows(2)
	t.Logf("rows %v; group exposed %v", rows, v.exposed(4))
	if !slices.Equal(rows, []platform.AccessID{3, 6, 5}) {
		t.Errorf("rows = %v, want [3 6 5] (tree order, nested items too)", rows)
	}
	for id, want := range map[platform.AccessID]int{3: 0, 6: 1, 5: 2} {
		if got, ok := v.index(id); got != want || !ok {
			t.Errorf("index(%d) = %d, %v; want %d", id, got, ok, want)
		}
	}
	if v.rows(3) != nil {
		t.Errorf("a tree item has rows")
	}
}

func noteStrings(notes []axNote) string {
	var out []string
	for _, n := range notes {
		out = append(out, n.String())
	}
	return strings.Join(out, "; ")
}

func TestAXNotes(t *testing.T) {
	first := platform.AccessUpdate{Nodes: sampleNodes(), Root: 1, Focus: 5}
	t1 := applyTree(t, nil, first)
	v1 := newAXTree(t1)
	got := noteStrings(axNotes(nil, v1, &first))
	t.Logf("first update: %s", got)
	if got != "AXLayoutChanged" {
		t.Errorf("first update posts %q, want only the layout change", got)
	}

	// The second update: the checkbox unchecked, the heading renamed, the
	// button removed (the div keeps the checkbox), the field's text and
	// selection changed, the other option selected, the focus on the
	// field, and two announcements.
	nodes := sampleNodes()
	byID := map[platform.AccessID]platform.AccessNode{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	check := byID[7]
	check.Checked = platform.ToggleFalse
	heading := byID[2]
	heading.Name = "Preferences"
	div := byID[4]
	div.Children = []platform.AccessID{7}
	field := byID[8]
	field.Value, field.Selection = "a😀bc", platform.AccessSelection{Anchor: 4, Focus: 4}
	a, b := byID[10], byID[11]
	a.Selected, b.Selected = platform.ToggleFalse, platform.ToggleTrue
	second := platform.AccessUpdate{
		Nodes: []platform.AccessNode{heading, div, check, field, a, b}, Root: 1, Focus: 8,
		Announcements: []platform.Announcement{{Text: "Saved", Assertive: true}, {Text: "  "}, {Text: "3 results"}},
	}
	t2 := applyTree(t, t1, second)
	v2 := newAXTree(t2)
	got = noteStrings(axNotes(v1, v2, &second))
	t.Logf("second update: %s", got)
	want := "AXUIElementDestroyed 5; AXLayoutChanged; AXTitleChanged 2; AXValueChanged 7; AXValueChanged 8; " +
		"AXSelectedTextChanged 8; AXSelectedChildrenChanged 9; AXFocusedUIElementChanged 8; " +
		`AXAnnouncementRequested "Saved" high; AXAnnouncementRequested "3 results" medium`
	if got != want {
		t.Errorf("second update posts\n %s\nwant\n %s", got, want)
	}

	// A tree item expanded, which adds a row; the focus back on no element.
	root := platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2}}
	tree := el(2, "tree", "Files", 3)
	item := el(3, "treeitem", "src")
	item.Expanded = platform.ToggleFalse
	u3 := platform.AccessUpdate{Nodes: []platform.AccessNode{root, tree, item}, Root: 1, Focus: 3}
	t3 := applyTree(t, nil, u3)
	item.Expanded, item.Children = platform.ToggleTrue, []platform.AccessID{4}
	u4 := platform.AccessUpdate{Nodes: []platform.AccessNode{item, el(4, "treeitem", "main.go")}, Root: 1, Focus: 1}
	t4 := applyTree(t, t3, u4)
	got = noteStrings(axNotes(newAXTree(t3), newAXTree(t4), &u4))
	t.Logf("expand: %s", got)
	if want := "AXLayoutChanged; AXRowCountChanged 2; AXRowExpanded 3; AXFocusedUIElementChanged"; got != want {
		t.Errorf("expanding posts %q, want %q", got, want)
	}
	if s := fmt.Sprint(axNoteKind(99)); s != "AXNote(99)" {
		t.Errorf("unknown note kind prints %q", s)
	}
}
