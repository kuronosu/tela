package platform

import (
	"math"
	"reflect"
	"slices"
	"testing"
)

// fullNode returns a node with every field set.
func fullNode() AccessNode {
	return AccessNode{
		ID: 7, Kind: AccessElement, Role: "slider", Name: "Volume", Description: "Output", Value: "40",
		Text: "t", URL: "https://example.com", Placeholder: "p", KeyShortcuts: "Control+V",
		Range:  AccessRange{Now: 40, Min: 0, Max: 100, HasNow: true, HasMin: true, HasMax: true},
		States: AccessFocusable | AccessFocused, Checked: ToggleTrue, Pressed: ToggleMixed, Selected: ToggleFalse, Expanded: ToggleTrue,
		Actions: AccessActionsOf(AccessActionIncrement, AccessActionDecrement),
		Level:   2, PosInSet: 3, SetSize: 9, RowCount: 4, RowIndex: 2,
		Sort: "ascending", Current: "page", HasPopup: "menu", Orientation: "horizontal", AutoComplete: "list",
		Live: "polite", LiveRoot: 7, Controls: 8, ActiveDescendant: 9,
		Selection: AccessSelection{Anchor: 1, Focus: 2}, HasSelection: true,
		Bounds:   Rect{X: 1, Y: 2, W: 3, H: 4},
		Children: []AccessID{10, 11},
	}
}

// TestAccessNodeEqualCoversEveryField: changing any field of a node makes
// it unequal, so a field added to AccessNode but not to Equal fails here.
func TestAccessNodeEqualCoversEveryField(t *testing.T) {
	base := fullNode()
	if c := base.Clone(); !base.Equal(&c) {
		t.Fatal("a node is not equal to its clone")
	}
	typ := reflect.TypeOf(base)
	for i := range typ.NumField() {
		c := base.Clone()
		f := reflect.ValueOf(&c).Elem().Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(f.String() + "x")
		case reflect.Bool:
			f.SetBool(!f.Bool())
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			f.SetUint(f.Uint() + 1)
		case reflect.Int:
			f.SetInt(f.Int() + 1)
		case reflect.Struct:
			g := f.Field(0)
			switch g.Kind() {
			case reflect.Float32, reflect.Float64:
				g.SetFloat(g.Float() + 1)
			case reflect.Int:
				g.SetInt(g.Int() + 1)
			default:
				t.Fatalf("field %s: unexpected struct field kind %v", typ.Field(i).Name, g.Kind())
			}
		case reflect.Slice:
			f.Set(reflect.Append(f, reflect.ValueOf(AccessID(12))))
		default:
			t.Fatalf("field %s of kind %v: extend this test", typ.Field(i).Name, f.Kind())
		}
		if base.Equal(&c) {
			t.Errorf("Equal ignores AccessNode.%s", typ.Field(i).Name)
		}
	}
}

func TestAccessNodeIgnored(t *testing.T) {
	for _, c := range []struct {
		n    AccessNode
		want bool
	}{
		{AccessNode{Role: "generic"}, true},
		{AccessNode{Role: "none"}, true},
		{AccessNode{Role: "presentation", States: AccessFocusable}, true},
		{AccessNode{Role: "generic", States: AccessFocusable}, false},
		{AccessNode{Role: "generic", States: AccessScrollable}, false},
		{AccessNode{Role: "generic", Actions: AccessActionsOf(AccessActionDefault)}, false},
		{AccessNode{Role: "generic", Live: "polite"}, false},
		{AccessNode{Role: "generic", Name: "Toolbar"}, false},
		{AccessNode{Role: "button"}, false},
		{AccessNode{Kind: AccessText}, false},
		{AccessNode{Kind: AccessRoot}, false},
	} {
		if got := c.n.Ignored(); got != c.want {
			t.Errorf("%+v Ignored = %v", c.n, got)
		}
	}
}

func TestAccessActionsSet(t *testing.T) {
	s := AccessActionsOf(AccessActionDefault, AccessActionShowMenu)
	if !s.Has(AccessActionDefault) || !s.Has(AccessActionShowMenu) || s.Has(AccessActionFocus) || s.Has(0) || s.Has(200) {
		t.Errorf("set %v", s)
	}
	if got := s.String(); got != "default|show-menu" {
		t.Errorf("String %q", got)
	}
	if got := (AccessFocused | AccessInline | 1<<30).String(); got != "focused|inline|0x40000000" {
		t.Errorf("states %q", got)
	}
}

func TestCheckAccessUpdate(t *testing.T) {
	ok := AccessUpdate{Root: 1, Focus: 1, Nodes: []AccessNode{{ID: 1, Kind: AccessRoot, Children: []AccessID{2}}, {ID: 2, Role: "button"}}}
	CheckAccessUpdate(&ok)
	for _, c := range []struct {
		name, want string
		change     func(u *AccessUpdate)
	}{
		{"no root", "Root is 0", func(u *AccessUpdate) { u.Root = 0 }},
		{"no focus", "Focus is 0", func(u *AccessUpdate) { u.Focus = 0 }},
		{"ID 0", "has ID 0", func(u *AccessUpdate) { u.Nodes[1].ID = 0 }},
		{"twice", "listed twice", func(u *AccessUpdate) { u.Nodes[1].ID = 1 }},
		{"kind", "unknown kind", func(u *AccessUpdate) { u.Nodes[1].Kind = 9 }},
		{"element without role", "elements have a role", func(u *AccessUpdate) { u.Nodes[1].Role = "" }},
		{"text with role", "elements have a role", func(u *AccessUpdate) { u.Nodes[1].Kind = AccessText }},
		{"second root", "is a root but Root", func(u *AccessUpdate) { u.Nodes[1].Kind, u.Nodes[1].Role = AccessRoot, "" }},
		{"root not a root", "not a root", func(u *AccessUpdate) {
			u.Root, u.Focus, u.Nodes[0].Kind, u.Nodes[0].Role = 2, 2, AccessElement, "generic"
		}},
		{"text with children", "text node 2 has children", func(u *AccessUpdate) {
			u.Nodes[1].Kind, u.Nodes[1].Role, u.Nodes[1].Children = AccessText, "", []AccessID{3}
		}},
		{"toggle", "invalid toggle", func(u *AccessUpdate) { u.Nodes[1].Checked = 7 }},
		{"utf-8", "not valid UTF-8", func(u *AccessUpdate) { u.Nodes[1].Name = "\xff" }},
		{"NaN", "must be finite", func(u *AccessUpdate) { u.Nodes[1].Bounds.X = float32(math.NaN()) }},
		{"negative", "negative size", func(u *AccessUpdate) { u.Nodes[1].Bounds.W = -1 }},
		{"child 0", "child with ID 0", func(u *AccessUpdate) { u.Nodes[0].Children = []AccessID{0} }},
		{"announcement", "announcement 0", func(u *AccessUpdate) { u.Announcements = []Announcement{{Text: "\xfe"}} }},
	} {
		u := ok.Clone()
		c.change(&u)
		expectPanic(t, c.want, func() { CheckAccessUpdate(&u) })
	}
}

func TestCheckAccessActionEvent(t *testing.T) {
	if p := CheckAccessActionEvent(AccessActionEvent{Node: 3, Action: AccessActionScrollBy, DY: 20}); p != "" {
		t.Errorf("a valid action: %s", p)
	}
	for _, e := range []AccessActionEvent{
		{Node: 3},
		{Node: 3, Action: accessActionEnd},
		{Action: AccessActionFocus},
		{Node: 3, Action: AccessActionScrollBy, DX: float32(math.Inf(1))},
		{Node: 3, Action: AccessActionSetSelection, Selection: AccessSelection{Anchor: -1}},
		{Node: 3, Action: AccessActionSetValue, Value: "\xff"},
	} {
		if CheckAccessActionEvent(e) == "" {
			t.Errorf("%+v accepted", e)
		}
	}
}

// TestAccessTreeApply: updates build the tree, unreachable nodes go, and
// invalid updates leave the tree as it was.
func TestAccessTreeApply(t *testing.T) {
	var tr *AccessTree
	if tr.Len() != 0 || tr.Root() != 0 || tr.String() != "" {
		t.Fatal("the nil tree is not empty")
	}
	first := AccessUpdate{Root: 1, Focus: 1, Nodes: []AccessNode{
		{ID: 1, Kind: AccessRoot, Children: []AccessID{2, 4}},
		{ID: 2, Role: "list", Children: []AccessID{3}},
		{ID: 3, Role: "listitem", Children: []AccessID{5}},
		{ID: 5, Kind: AccessText, Text: "One"},
		{ID: 4, Role: "button", Name: "Add"},
		{ID: 9, Role: "button", Name: "Unreachable"},
	}}
	tr, removed, err := tr.Apply(&first)
	if err != nil || len(removed) != 0 {
		t.Fatalf("first update: %v, removed %v", err, removed)
	}
	want := "root\n  list\n    listitem\n      text \"One\"\n  button \"Add\"\n"
	if got := tr.String(); got != want || tr.Len() != 5 {
		t.Errorf("tree (%d nodes)\n%s\nwant\n%s", tr.Len(), got, want)
	}
	// Removing the list item drops its text too; the focus moves.
	next, removed, err := tr.Apply(&AccessUpdate{Root: 1, Focus: 4, Nodes: []AccessNode{
		{ID: 2, Role: "list"},
		{ID: 4, Role: "button", Name: "Add", States: AccessFocused},
	}})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(removed)
	if !slices.Equal(removed, []AccessID{3, 5}) || next.Len() != 3 || next.Focus() != 4 {
		t.Errorf("removed %v, %d nodes, focus %d", removed, next.Len(), next.Focus())
	}
	if n, _ := tr.Node(3); n == nil || tr.Len() != 5 {
		t.Error("Apply changed the tree it was called on")
	}
	for _, c := range []struct {
		name string
		u    AccessUpdate
	}{
		{"unknown child", AccessUpdate{Root: 1, Focus: 1, Nodes: []AccessNode{{ID: 2, Role: "list", Children: []AccessID{42}}}}},
		{"two parents", AccessUpdate{Root: 1, Focus: 1, Nodes: []AccessNode{{ID: 2, Role: "list", Children: []AccessID{4}}}}},
		{"cycle", AccessUpdate{Root: 1, Focus: 1, Nodes: []AccessNode{{ID: 4, Role: "button", Children: []AccessID{1}}}}},
		{"focus outside", AccessUpdate{Root: 1, Focus: 9}},
		{"unknown root", AccessUpdate{Root: 8, Focus: 8}},
	} {
		if got, _, err := next.Apply(&c.u); err == nil || got != next {
			t.Errorf("%s: err %v", c.name, err)
		}
	}
}
