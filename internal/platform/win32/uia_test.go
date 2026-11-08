package win32

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The OS-independent half of the UI Automation bridge (uia.go,
// uiatext.go): role mapping, patterns, the exposed tree, event diffing and
// text units. Pure Go: these run on every OS.

func TestUIARoleMapping(t *testing.T) {
	el := func(role string) *platform.AccessNode {
		return &platform.AccessNode{ID: 2, Kind: platform.AccessElement, Role: role}
	}
	for _, c := range []struct {
		role      string
		ct        int32
		localized string
		landmark  int32
	}{
		{"button", uiaButtonControlTypeID, "", 0},
		{"checkbox", uiaCheckBoxControlTypeID, "", 0},
		{"textbox", uiaEditControlTypeID, "", 0},
		{"searchbox", uiaEditControlTypeID, "search box", 0},
		{"heading", uiaTextControlTypeID, "heading", 0},
		{"link", uiaHyperlinkControlTypeID, "", 0},
		{"dialog", uiaWindowControlTypeID, "dialog", 0},
		{"navigation", uiaGroupControlTypeID, "navigation", uiaNavigationLandmarkTypeID},
		{"main", uiaGroupControlTypeID, "main", uiaMainLandmarkTypeID},
		{"banner", uiaGroupControlTypeID, "banner", uiaCustomLandmarkTypeID},
		{"switch", uiaButtonControlTypeID, "toggle switch", 0},
		{"slider", uiaSliderControlTypeID, "", 0},
		{"menuitemradio", uiaRadioButtonControlTypeID, "", 0},
		{"tablist", uiaTabControlTypeID, "", 0},
		{"generic", uiaGroupControlTypeID, "", 0},
		{"form", uiaGroupControlTypeID, "form", 0}, // no name: no landmark
		{"unknown-role", uiaGroupControlTypeID, "", 0},
	} {
		r := uiaRoleOf(el(c.role))
		if r.controlType != c.ct || r.localized != c.localized || r.landmark != c.landmark {
			t.Errorf("%s: got %+v, want control type %d, localized %q, landmark %d", c.role, r, c.ct, c.localized, c.landmark)
		}
	}
	named := el("form")
	named.Name = "Sign in"
	if r := uiaRoleOf(named); r.landmark != uiaFormLandmarkTypeID {
		t.Errorf("named form: landmark %d, want Form", r.landmark)
	}
	if r := uiaRoleOf(&platform.AccessNode{Kind: platform.AccessText}); r.controlType != uiaTextControlTypeID {
		t.Errorf("text: %+v", r)
	}
	// Every role of the vocabulary has a mapping.
	for role := range strings.FieldsSeq("alert alertdialog application article banner blockquote button caption cell checkbox code columnheader combobox complementary contentinfo definition deletion dialog directory document emphasis feed figure form generic grid gridcell group heading img insertion link list listbox listitem log main mark marquee math menu menubar menuitem menuitemcheckbox menuitemradio meter navigation note option paragraph progressbar radio radiogroup region row rowgroup rowheader scrollbar search searchbox separator slider spinbutton status strong subscript superscript switch tab table tablist tabpanel term textbox time timer toolbar tooltip tree treegrid treeitem") {
		if _, ok := uiaRoles[role]; !ok {
			t.Errorf("role %q has no UIA mapping", role)
		}
	}
}

func TestUIAPatterns(t *testing.T) {
	click := platform.AccessActionsOf(platform.AccessActionDefault, platform.AccessActionFocus)
	for _, c := range []struct {
		name string
		n    platform.AccessNode
		want []int32
	}{
		{"button", platform.AccessNode{Role: "button", Actions: click}, []int32{uiaInvokePatternID}},
		{"toggle button", platform.AccessNode{Role: "button", Pressed: platform.ToggleFalse, Actions: click}, []int32{uiaTogglePatternID}},
		{"checkbox", platform.AccessNode{Role: "checkbox", Checked: platform.ToggleTrue, Actions: click}, []int32{uiaTogglePatternID}},
		{"radio", platform.AccessNode{Role: "radio", Checked: platform.ToggleFalse, Actions: click}, []int32{uiaSelectionItemPatternID}},
		{"textbox", platform.AccessNode{Role: "textbox", HasSelection: true, Actions: platform.AccessActionsOf(platform.AccessActionSetValue, platform.AccessActionSetSelection, platform.AccessActionDefault)},
			[]int32{uiaValuePatternID, uiaTextPatternID}},
		{"slider", platform.AccessNode{Role: "slider", Value: "50", Range: platform.AccessRange{Now: 50, HasNow: true},
			Actions: platform.AccessActionsOf(platform.AccessActionIncrement, platform.AccessActionDecrement)}, []int32{uiaValuePatternID, uiaRangeValuePatternID}},
		{"combobox", platform.AccessNode{Role: "combobox", Expanded: platform.ToggleFalse, Actions: click}, []int32{uiaValuePatternID, uiaExpandCollapsePatternID}},
		{"listbox", platform.AccessNode{Role: "listbox"}, []int32{uiaSelectionPatternID}},
		{"option", platform.AccessNode{Role: "option", Selected: platform.ToggleTrue, Actions: click}, []int32{uiaSelectionItemPatternID}},
		{"link", platform.AccessNode{Role: "link", URL: "https://telaui.dev", Actions: click}, []int32{uiaInvokePatternID, uiaValuePatternID}},
		{"scroller", platform.AccessNode{Role: "generic", States: platform.AccessScrollable,
			Actions: platform.AccessActionsOf(platform.AccessActionScrollBy)}, []int32{uiaScrollPatternID}},
		{"menu button", platform.AccessNode{Role: "button", HasPopup: "menu", Expanded: platform.ToggleFalse, Actions: click},
			[]int32{uiaInvokePatternID, uiaExpandCollapsePatternID}},
	} {
		c.n.Kind, c.n.ID = platform.AccessElement, 5
		var got []int32
		for _, p := range []int32{uiaInvokePatternID, uiaSelectionPatternID, uiaValuePatternID, uiaRangeValuePatternID,
			uiaScrollPatternID, uiaExpandCollapsePatternID, uiaSelectionItemPatternID, uiaTextPatternID, uiaTogglePatternID, uiaScrollItemPatternID} {
			if uiaSupports(&c.n, p) {
				got = append(got, p)
			}
		}
		slices.Sort(got)
		slices.Sort(c.want)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: patterns %v, want %v", c.name, got, c.want)
		}
	}
	text := platform.AccessNode{Kind: platform.AccessText, Text: "Hi"}
	if !uiaSupports(&text, uiaTextPatternID) || uiaSupports(&text, uiaValuePatternID) {
		t.Error("a text node has the Text pattern and no Value")
	}
}

func TestUIAReadOnly(t *testing.T) {
	set := platform.AccessActionsOf(platform.AccessActionSetValue)
	step := platform.AccessActionsOf(platform.AccessActionIncrement, platform.AccessActionDecrement)
	for _, c := range []struct {
		name           string
		n              platform.AccessNode
		value, rangeRO bool
	}{
		{"editable", platform.AccessNode{Actions: set | step}, false, false},
		{"readonly", platform.AccessNode{Actions: set | step, States: platform.AccessReadOnly}, true, true},
		{"disabled", platform.AccessNode{Actions: set | step, States: platform.AccessDisabled}, true, true},
		{"no actions", platform.AccessNode{}, true, true},
		{"steps only", platform.AccessNode{Actions: step}, true, false},
	} {
		if got := uiaValueReadOnly(&c.n); got != c.value {
			t.Errorf("%s: Value.IsReadOnly %v", c.name, got)
		}
		if got := uiaRangeReadOnly(&c.n); got != c.rangeRO {
			t.Errorf("%s: RangeValue.IsReadOnly %v", c.name, got)
		}
	}
}

func TestUIAProperties(t *testing.T) {
	n := &platform.AccessNode{ID: 3, Kind: platform.AccessElement, Role: "heading", Name: "Settings", Level: 2,
		Description: "All of them", KeyShortcuts: "Control+S Meta+Shift+S", States: platform.AccessRequired | platform.AccessInvalid,
		PosInSet: 2, SetSize: 5, Bounds: platform.Rect{X: 10, Y: 10, W: 100, H: 20}}
	s := testSnapshot(t, n)
	for _, c := range []struct {
		id   int32
		want uiaVal
	}{
		{uiaControlTypePropertyID, uiaI(uiaTextControlTypeID)},
		{uiaLocalizedControlTypePropertyID, uiaS("heading")},
		{uiaNamePropertyID, uiaS("Settings")},
		{uiaFullDescriptionPropertyID, uiaS("All of them")},
		{uiaHelpTextPropertyID, uiaS("All of them")},
		{uiaAcceleratorKeyPropertyID, uiaS("Ctrl+S, Win+Shift+S")},
		{uiaLevelPropertyID, uiaI(2)},
		{uiaHeadingLevelPropertyID, uiaI(uiaHeadingLevel1 + 1)},
		{uiaPositionInSetPropertyID, uiaI(2)},
		{uiaSizeOfSetPropertyID, uiaI(5)},
		{uiaIsRequiredForFormPropertyID, uiaB(true)},
		{uiaIsDataValidForFormPropertyID, uiaB(false)},
		{uiaIsEnabledPropertyID, uiaB(true)},
		{uiaIsOffscreenPropertyID, uiaB(false)},
		{uiaFrameworkIDPropertyID, uiaS("Tela")},
		{uiaAriaRolePropertyID, uiaS("heading")},
		{uiaAriaPropertiesPropertyID, uiaS("level=2;posinset=2;required=true;setsize=5")},
		{uiaIsPasswordPropertyID, uiaB(false)},
		{uiaClassNamePropertyID, uiaVal{}},
	} {
		if got := uiaProperty(s, n, c.id); got != c.want {
			t.Errorf("property %d = %+v, want %+v", c.id, got, c.want)
		}
	}
	off := *n
	off.Bounds = platform.Rect{X: 900, Y: 10, W: 50, H: 20}
	if got := uiaProperty(s, &off, uiaIsOffscreenPropertyID); got != uiaB(true) {
		t.Errorf("a node right of the window: IsOffscreen %+v", got)
	}
	if got := uiaAriaProperties(&platform.AccessNode{Kind: platform.AccessElement, Role: "x", Current: "a=b;c"}); got != `current=a\=b\;c` {
		t.Errorf("escaping: %q", got)
	}
}

// testSnapshot returns the snapshot of a root (800x600) holding nodes.
func testSnapshot(t *testing.T, nodes ...*platform.AccessNode) *uiaSnapshot {
	t.Helper()
	root := platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Bounds: platform.Rect{W: 800, H: 600}}
	u := platform.AccessUpdate{Root: 1, Focus: 1}
	for _, n := range nodes {
		root.Children = append(root.Children, n.ID)
		u.Nodes = append(u.Nodes, *n)
	}
	u.Nodes = append([]platform.AccessNode{root}, u.Nodes...)
	tree, _, err := (*platform.AccessTree)(nil).Apply(&u)
	if err != nil {
		t.Fatal(err)
	}
	return newUIASnapshot(tree)
}

// loginTree is a small form: a generic wrapper (ignored) around a
// heading with a text, a labelled text field, a checkbox with a text
// child (presentational), and a button.
func loginTree() platform.AccessUpdate {
	click := platform.AccessActionsOf(platform.AccessActionDefault, platform.AccessActionFocus)
	return platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Bounds: platform.Rect{W: 400, H: 300}, Children: []platform.AccessID{2}},
		{ID: 2, Kind: platform.AccessElement, Role: "generic", Bounds: platform.Rect{W: 400, H: 300}, Children: []platform.AccessID{3, 5, 6, 8}},
		{ID: 3, Kind: platform.AccessElement, Role: "heading", Name: "Sign in", Level: 1, Bounds: platform.Rect{X: 10, Y: 10, W: 200, H: 30}, Children: []platform.AccessID{4}},
		{ID: 4, Kind: platform.AccessText, Text: "Sign  in", States: platform.AccessInline, Bounds: platform.Rect{X: 10, Y: 10, W: 100, H: 30}},
		{ID: 5, Kind: platform.AccessElement, Role: "textbox", Name: "Email", Value: "ana@example.com", HasSelection: true,
			Selection: platform.AccessSelection{Anchor: 3, Focus: 3}, States: platform.AccessFocusable,
			Actions: platform.AccessActionsOf(platform.AccessActionFocus, platform.AccessActionSetValue, platform.AccessActionSetSelection),
			Bounds:  platform.Rect{X: 10, Y: 50, W: 300, H: 30}},
		{ID: 6, Kind: platform.AccessElement, Role: "checkbox", Name: "Remember me", Checked: platform.ToggleFalse, States: platform.AccessFocusable,
			Actions: click, Bounds: platform.Rect{X: 10, Y: 90, W: 200, H: 20}, Children: []platform.AccessID{7}},
		{ID: 7, Kind: platform.AccessText, Text: "Remember me", Bounds: platform.Rect{X: 30, Y: 90, W: 100, H: 20}},
		{ID: 8, Kind: platform.AccessElement, Role: "button", Name: "Sign in", States: platform.AccessFocusable, Actions: click,
			Bounds: platform.Rect{X: 10, Y: 120, W: 100, H: 30}, Children: []platform.AccessID{9}},
		{ID: 9, Kind: platform.AccessText, Text: "Sign in", Bounds: platform.Rect{X: 20, Y: 125, W: 60, H: 20}},
	}}
}

func applyTree(t *testing.T, tree *platform.AccessTree, u platform.AccessUpdate) *platform.AccessTree {
	t.Helper()
	next, _, err := tree.Apply(&u)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestUIASnapshot(t *testing.T) {
	s := newUIASnapshot(applyTree(t, nil, loginTree()))
	// The generic wrapper is ignored: the root's children are its; the
	// checkbox and the button hide their texts; the heading keeps its.
	if got := s.kids[1]; !slices.Equal(got, []platform.AccessID{3, 5, 6, 8}) {
		t.Errorf("root's children %v", got)
	}
	if got := s.kids[3]; !slices.Equal(got, []platform.AccessID{4}) {
		t.Errorf("heading's children %v", got)
	}
	for _, id := range []platform.AccessID{2, 7, 9} {
		if _, ok := s.node(id); ok {
			t.Errorf("node %d is exposed", id)
		}
	}
	for _, c := range []struct {
		from platform.AccessID
		dir  int32
		want platform.AccessID
	}{
		{1, uiaNavigateFirstChild, 3}, {1, uiaNavigateLastChild, 8},
		{3, uiaNavigateNextSibling, 5}, {5, uiaNavigatePreviousSibling, 3}, {8, uiaNavigateNextSibling, 0},
		{3, uiaNavigatePreviousSibling, 0}, {4, uiaNavigateParent, 3}, {5, uiaNavigateParent, 1}, {1, uiaNavigateParent, 0},
		{6, uiaNavigateFirstChild, 0},
	} {
		if got := s.navigate(c.from, c.dir); got != c.want {
			t.Errorf("navigate(%d, %d) = %d, want %d", c.from, c.dir, got, c.want)
		}
	}
	for _, c := range []struct {
		x, y float32
		want platform.AccessID
	}{{15, 55, 5}, {15, 15, 4}, {150, 15, 3}, {390, 290, 1}, {25, 128, 8}} {
		if got := s.at(c.x, c.y); got != c.want {
			t.Errorf("at(%v, %v) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
	text, _ := s.node(4)
	if uiaName(text) != "Sign in" {
		t.Errorf("text name %q", uiaName(text))
	}
	// A focus inside a hidden subtree lands on the exposed ancestor.
	u := loginTree()
	u.Focus = 9
	s = newUIASnapshot(applyTree(t, nil, u))
	if s.focus != 8 {
		t.Errorf("focus %d, want the button", s.focus)
	}
	if newUIASnapshot(nil).root() != 0 {
		t.Error("the empty snapshot has a root")
	}
}

func eventString(evs []uiaEvent) string {
	var b strings.Builder
	for _, e := range evs {
		switch e.kind {
		case uiaEventAutomation:
			fmt.Fprintf(&b, "event %d on %d\n", e.id, e.node)
		case uiaEventProperty:
			fmt.Fprintf(&b, "property %d on %d: %+v -> %+v\n", e.id, e.node, e.old, e.new)
		case uiaEventStructure:
			fmt.Fprintf(&b, "structure on %d\n", e.node)
		case uiaEventNotification:
			fmt.Fprintf(&b, "notification %d %q\n", e.id, e.text)
		}
	}
	return b.String()
}

func TestUIAEvents(t *testing.T) {
	first := loginTree()
	t0 := applyTree(t, nil, first)
	s0 := newUIASnapshot(t0)
	// The first update raises the focus only.
	if got := eventString(uiaEvents(newUIASnapshot(nil), s0, &first)); got != "event 20005 on 1\n" {
		t.Errorf("first update: %s", got)
	}
	// The checkbox is checked, the field focused and its value changed,
	// a status announces, the button removed.
	full := loginTree()
	cb, field, root := full.Nodes[5], full.Nodes[4], full.Nodes[1]
	cb.Checked = platform.ToggleTrue
	field.Value, field.Selection = "ana@example.org", platform.AccessSelection{Anchor: 15, Focus: 15}
	field.States |= platform.AccessFocused
	root.Children = []platform.AccessID{3, 5, 6}
	u := platform.AccessUpdate{Root: 1, Focus: 5, Nodes: []platform.AccessNode{root, field, cb},
		Announcements: []platform.Announcement{{Text: "Saved"}, {Text: "Error", Assertive: true}}}
	s1 := newUIASnapshot(applyTree(t, t0, u))
	want := "structure on 1\n" +
		"property 30045 on 5: {kind:4 b:false i:0 f:0 s:ana@example.com} -> {kind:4 b:false i:0 f:0 s:ana@example.org}\n" +
		"event 20015 on 5\n" +
		"event 20014 on 5\n" +
		"property 30086 on 6: {kind:2 b:false i:0 f:0 s:} -> {kind:2 b:false i:1 f:0 s:}\n" +
		"event 20005 on 5\n" +
		"notification 2 \"Saved\"\n" +
		"notification 1 \"Error\"\n"
	if got := eventString(uiaEvents(s0, s1, &u)); got != want {
		t.Errorf("events:\n%s\nwant:\n%s", got, want)
	}
}

func TestUIASelectionAndMenuEvents(t *testing.T) {
	click := platform.AccessActionsOf(platform.AccessActionDefault)
	base := platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2}},
		{ID: 2, Kind: platform.AccessElement, Role: "listbox", Children: []platform.AccessID{3, 4}},
		{ID: 3, Kind: platform.AccessElement, Role: "option", Name: "A", Selected: platform.ToggleTrue, Actions: click},
		{ID: 4, Kind: platform.AccessElement, Role: "option", Name: "B", Selected: platform.ToggleFalse, Actions: click},
	}}
	t0 := applyTree(t, nil, base)
	s0 := newUIASnapshot(t0)
	if got := s0.selected(2); !slices.Equal(got, []platform.AccessID{3}) {
		t.Errorf("selected %v", got)
	}
	if got := s0.selectionContainer(4); got != 2 {
		t.Errorf("container %d", got)
	}
	a, b := base.Nodes[2], base.Nodes[3]
	a.Selected, b.Selected = platform.ToggleFalse, platform.ToggleTrue
	root := base.Nodes[0]
	root.Children = []platform.AccessID{2, 5}
	menu := platform.AccessNode{ID: 5, Kind: platform.AccessElement, Role: "menu"}
	u := platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{root, a, b, menu}}
	t1 := applyTree(t, t0, u)
	s1 := newUIASnapshot(t1)
	got := eventString(uiaEvents(s0, s1, &u))
	for _, w := range []string{"structure on 1", "event 20003 on 5", "event 20012 on 4", "property 30079 on 3"} {
		if !strings.Contains(got, w) {
			t.Errorf("events lack %q:\n%s", w, got)
		}
	}
	root.Children = []platform.AccessID{2}
	u2 := platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{root}}
	s2 := newUIASnapshot(applyTree(t, t1, u2))
	if got := eventString(uiaEvents(s1, s2, &u2)); !strings.Contains(got, "event 20007 on 5") {
		t.Errorf("menu closed missing:\n%s", got)
	}
}

func TestUIAUnitStarts(t *testing.T) {
	for _, c := range []struct {
		text string
		unit int32
		want []int
	}{
		{"", uiaUnitCharacter, []int{0}},
		{"abc", uiaUnitCharacter, []int{0, 1, 2, 3}},
		{"e\u0301x", uiaUnitCharacter, []int{0, 2, 3}},         // combining acute
		{"a😀b", uiaUnitCharacter, []int{0, 1, 3, 4}},           // surrogate pair
		{"👍🏽!", uiaUnitCharacter, []int{0, 4, 5}},              // skin tone modifier
		{"🇨🇴🇯🇵", uiaUnitCharacter, []int{0, 4, 8}},             // two flags
		{"👩\u200d💻x", uiaUnitCharacter, []int{0, 5, 6}},        // ZWJ sequence
		{"a\r\nb", uiaUnitCharacter, []int{0, 1, 3, 4}},        // CRLF is one
		{"hello world", uiaUnitWord, []int{0, 6, 11}},          // spaces join the word before
		{"hi, you", uiaUnitWord, []int{0, 2, 4, 7}},            // punctuation is a word
		{"  lead", uiaUnitWord, []int{0, 2, 6}},                // leading spaces
		{"one\ntwo", uiaUnitWord, []int{0, 3, 4, 7}},           // the break is a word
		{"one\ntwo\r\nthree", uiaUnitLine, []int{0, 4, 9, 14}}, // breaks end lines
		{"no break", uiaUnitLine, []int{0, 8}},                 //
		{"para\n", uiaUnitParagraph, []int{0, 5}},              // a final break
		{"whole text", uiaUnitDocument, []int{0, 10}},          //
		{"whole", uiaUnitFormat, []int{0, 5}},                  //
	} {
		if got := uiaUnitStarts(u16(c.text), c.unit); !slices.Equal(got, c.want) {
			t.Errorf("starts(%q, %d) = %v, want %v", c.text, c.unit, got, c.want)
		}
	}
}

func TestUIATextRangeMoves(t *testing.T) {
	tx := &uiaUnitText{text: u16("hello big world")}
	// Expand a caret to its word, line and document.
	if got := tx.expand(uiaTextRange{7, 7}, uiaUnitWord); got != (uiaTextRange{6, 10}) {
		t.Errorf("expand word = %v", got)
	}
	if got := tx.expand(uiaTextRange{3, 3}, uiaUnitCharacter); got != (uiaTextRange{3, 4}) {
		t.Errorf("expand char = %v", got)
	}
	if got := tx.expand(uiaTextRange{15, 15}, uiaUnitCharacter); got != (uiaTextRange{15, 15}) {
		t.Errorf("expand char at the end = %v (stays degenerate)", got)
	}
	if got := tx.expand(uiaTextRange{2, 4}, uiaUnitPage); got != (uiaTextRange{0, 15}) {
		t.Errorf("expand page = %v", got)
	}
	if got := tx.expand(uiaTextRange{15, 15}, uiaUnitWord); got != (uiaTextRange{10, 15}) {
		t.Errorf("expand word at the end = %v", got)
	}
	// Move a word range forward and back.
	r, moved := tx.move(uiaTextRange{0, 6}, uiaUnitWord, 1)
	if r != (uiaTextRange{6, 10}) || moved != 1 {
		t.Errorf("move word +1 = %v %d", r, moved)
	}
	r, moved = tx.move(r, uiaUnitWord, 5)
	if r != (uiaTextRange{15, 15}) || moved != 2 {
		t.Errorf("move word +5 = %v %d (stops at the end)", r, moved)
	}
	r, moved = tx.move(uiaTextRange{6, 10}, uiaUnitWord, -1)
	if r != (uiaTextRange{0, 6}) || moved != -1 {
		t.Errorf("move word -1 = %v %d", r, moved)
	}
	// A degenerate range moves as a caret.
	r, moved = tx.move(uiaTextRange{2, 2}, uiaUnitCharacter, 3)
	if r != (uiaTextRange{5, 5}) || moved != 3 {
		t.Errorf("move caret +3 = %v %d", r, moved)
	}
	r, moved = tx.move(uiaTextRange{0, 0}, uiaUnitCharacter, -1)
	if r != (uiaTextRange{0, 0}) || moved != 0 {
		t.Errorf("move caret -1 at the start = %v %d", r, moved)
	}
	// Endpoints.
	r, moved = tx.moveEndpoint(uiaTextRange{0, 0}, true, uiaUnitWord, 2)
	if r != (uiaTextRange{0, 10}) || moved != 2 {
		t.Errorf("end +2 words = %v %d", r, moved)
	}
	r, moved = tx.moveEndpoint(uiaTextRange{6, 10}, false, uiaUnitCharacter, 6)
	if r != (uiaTextRange{12, 12}) || moved != 6 {
		t.Errorf("start +6 chars past the end = %v %d (the end follows)", r, moved)
	}
	r, moved = tx.moveEndpoint(uiaTextRange{6, 10}, true, uiaUnitDocument, -1)
	if r != (uiaTextRange{0, 0}) || moved != -1 {
		t.Errorf("end -1 document = %v %d", r, moved)
	}
	// Ranges made for a longer text are clamped.
	if got := (uiaTextRange{3, 40}).clamp(15); got != (uiaTextRange{3, 15}) {
		t.Errorf("clamp = %v", got)
	}
	// Caret positions.
	lines := &uiaUnitText{text: u16("ab\ncd")}
	for pos, want := range map[int]int32{0: uiaCaretPositionBeginningOfLine, 1: uiaCaretPositionUnknown,
		2: uiaCaretPositionEndOfLine, 3: uiaCaretPositionBeginningOfLine, 5: uiaCaretPositionEndOfLine} {
		if got := lines.caretPosition(pos); got != want {
			t.Errorf("caret position at %d = %d, want %d", pos, got, want)
		}
	}
}

func TestUIATextOffsets(t *testing.T) {
	s := "a😀é"
	for cp, want := range map[int]int{0: 0, 1: 1, 2: 3, 3: 4, 9: 4} {
		if got := uiaCodePointsToUTF16(s, cp); got != want {
			t.Errorf("code point %d -> UTF-16 %d, want %d", cp, got, want)
		}
	}
	for off, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 2, 4: 3} {
		if got := uiaUTF16ToCodePoints(u16(s), off); got != want {
			t.Errorf("UTF-16 %d -> code point %d, want %d", off, got, want)
		}
	}
	text := u16("Hello wörld, hello")
	r, ok := uiaFind(text, uiaTextRange{0, len(text)}, "HELLO", false, true)
	if !ok || r != (uiaTextRange{0, 5}) {
		t.Errorf("find = %v %v", r, ok)
	}
	r, ok = uiaFind(text, uiaTextRange{0, len(text)}, "hello", true, false)
	if !ok || r != (uiaTextRange{13, 18}) {
		t.Errorf("find backward = %v %v", r, ok)
	}
	r, ok = uiaFind(text, uiaTextRange{6, 11}, "wörld", false, false)
	if !ok || r != (uiaTextRange{6, 11}) {
		t.Errorf("find in a subrange = %v %v", r, ok)
	}
	if _, ok := uiaFind(text, uiaTextRange{0, 4}, "Hello", false, false); ok {
		t.Error("found past the range")
	}
}

func TestUIAAcceleratorKey(t *testing.T) {
	for in, want := range map[string]string{"": "", "Control+S": "Ctrl+S", "Alt+F4 Meta+D": "Alt+F4, Win+D"} {
		if got := uiaAcceleratorKey(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
