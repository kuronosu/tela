//go:build linux

package atspi

import (
	"image"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestRoleMapping(t *testing.T) {
	for _, c := range []struct {
		n    platform.AccessNode
		want uint32
	}{
		{platform.AccessNode{Kind: platform.AccessRoot}, roleFrame},
		{platform.AccessNode{Kind: platform.AccessText}, roleLabel},
		{platform.AccessNode{Role: "button"}, rolePushButton},
		{platform.AccessNode{Role: "button", Pressed: platform.ToggleFalse}, roleToggleButton},
		{platform.AccessNode{Role: "button", HasPopup: "menu"}, rolePushButtonMenu},
		{platform.AccessNode{Role: "button", HasPopup: "dialog"}, rolePushButton},
		{platform.AccessNode{Role: "textbox"}, roleEntry},
		{platform.AccessNode{Role: "textbox", States: platform.AccessProtected}, rolePasswordText},
		{platform.AccessNode{Role: "searchbox"}, roleEntry},
		{platform.AccessNode{Role: "alert"}, roleNotification},
		{platform.AccessNode{Role: "alertdialog"}, roleAlert},
		{platform.AccessNode{Role: "dialog"}, roleDialog},
		{platform.AccessNode{Role: "checkbox"}, roleCheckBox},
		{platform.AccessNode{Role: "switch"}, roleSwitch},
		{platform.AccessNode{Role: "option"}, roleListItem},
		{platform.AccessNode{Role: "listbox"}, roleListBox},
		{platform.AccessNode{Role: "tab"}, rolePageTab},
		{platform.AccessNode{Role: "tabpanel"}, roleScrollPane},
		{platform.AccessNode{Role: "navigation"}, roleLandmark},
		{platform.AccessNode{Role: "generic"}, roleSection},
		{platform.AccessNode{Role: "meter"}, roleLevelBar},
		{platform.AccessNode{Role: "status"}, roleStatusBar},
		{platform.AccessNode{Role: "img"}, roleImage},
		{platform.AccessNode{Role: "strong"}, roleStatic},
		{platform.AccessNode{Role: "no-such-role"}, roleUnknown},
	} {
		if got := atspiRole(&c.n); got != c.want {
			t.Errorf("%q kind %v: role %d (%s), want %d (%s)", c.n.Role, c.n.Kind, got, roleName(got), c.want, roleName(c.want))
		}
	}
	if roleName(rolePushButton) != "button" || roleName(rolePasswordText) != "password text" || roleName(roleSwitch) != "switch" {
		t.Errorf("role names %q %q %q", roleName(rolePushButton), roleName(rolePasswordText), roleName(roleSwitch))
	}
	// Every role the semantics package knows maps to something.
	for role := range ariaRoles {
		if r := ariaRoles[role]; r == roleInvalid || r >= roleLastDefined {
			t.Errorf("%q maps to %d", role, r)
		}
	}
}

func TestStateMapping(t *testing.T) {
	tree, _, err := (*platform.AccessTree)(nil).Apply(&platform.AccessUpdate{Root: 1, Focus: 2, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2, 3, 4, 5}, Bounds: platform.Rect{W: 100, H: 100}},
		{ID: 2, Role: "checkbox", Checked: platform.ToggleMixed, States: platform.AccessFocusable | platform.AccessFocused | platform.AccessRequired | platform.AccessInvalid,
			Bounds: platform.Rect{X: 1, Y: 1, W: 10, H: 10}},
		{ID: 3, Role: "button", Pressed: platform.ToggleTrue, Expanded: platform.ToggleFalse, HasPopup: "menu", States: platform.AccessDisabled,
			Bounds: platform.Rect{X: 200, Y: 1, W: 10, H: 10}},
		{ID: 4, Role: "textbox", States: platform.AccessMultiline | platform.AccessReadOnly, Bounds: platform.Rect{X: 1, Y: 20, W: 10, H: 10}},
		{ID: 5, Role: "option", Selected: platform.ToggleTrue, Bounds: platform.Rect{X: 1, Y: 40, W: 10, H: 10}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	s := &snapshot{view: buildView(tree), scale: 1}
	check := func(id platform.AccessID, have, lack []int) {
		t.Helper()
		st := nodeStates(s, s.node(id))
		for _, h := range have {
			if !st.has(h) {
				t.Errorf("node %d lacks %s", id, stateNames[h])
			}
		}
		for _, l := range lack {
			if st.has(l) {
				t.Errorf("node %d has %s", id, stateNames[l])
			}
		}
	}
	check(2, []int{stateEnabled, stateSensitive, stateFocusable, stateFocused, stateCheckable, stateIndeterminate,
		stateRequired, stateInvalidEntry, stateShowing, stateVisible}, []int{stateChecked})
	check(3, []int{statePressed, stateExpandable, stateCollapsed, stateHasPopup, stateVisible},
		[]int{stateEnabled, stateSensitive, stateExpanded, stateShowing, stateFocused})
	check(4, []int{stateMultiLine, stateReadOnly, stateSelectableText}, []int{stateEditable, stateSingleLine})
	check(5, []int{stateSelectable, stateSelected}, nil)
	check(1, []int{stateEnabled, stateShowing, stateResizable}, []int{stateActive})
	s.active = true
	check(1, []int{stateActive}, nil)
	if w := (stateSet(1)<<stateFocused | 1<<stateReadOnly).words(); w[0] != 1<<stateFocused || w[1] != 1<<(stateReadOnly-32) {
		t.Errorf("words %v", w)
	}
}

func TestAttributes(t *testing.T) {
	tree, _, _ := (*platform.AccessTree)(nil).Apply(&platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2}},
		{ID: 2, Role: "log", Live: "polite", LiveRoot: 2, Children: []platform.AccessID{3}},
		{ID: 3, Role: "listitem", Level: 2, PosInSet: 1, SetSize: 3, LiveRoot: 2, KeyShortcuts: "Control+S"},
	}})
	a, _ := tree.Node(3)
	got := nodeAttributes(tree, a)
	for k, v := range map[string]string{"xml-roles": "listitem", "level": "2", "posinset": "1", "setsize": "3",
		"container-live": "polite", "container-atomic": "false", "keyshortcuts": "Control+S"} {
		if got[k] != v {
			t.Errorf("attribute %s = %q, want %q (all %v)", k, got[k], v, got)
		}
	}
	r, _ := tree.Node(2)
	if got := nodeAttributes(tree, r); got["live"] != "polite" {
		t.Errorf("region attributes %v", got)
	}
}

func TestActions(t *testing.T) {
	n := &platform.AccessNode{Role: "combobox", Expanded: platform.ToggleTrue, Actions: platform.AccessActionsOf(
		platform.AccessActionDefault, platform.AccessActionExpand, platform.AccessActionCollapse, platform.AccessActionShowMenu, platform.AccessActionFocus)}
	var names []string
	for _, a := range nodeActions(n) {
		names = append(names, a.name)
	}
	if !slices.Equal(names, []string{"click", "collapse", "show-context-menu"}) {
		t.Errorf("actions %v", names)
	}
	n.States = platform.AccessDisabled
	if len(nodeActions(n)) != 0 {
		t.Errorf("a disabled node offers actions")
	}
}

// TestViewFolding: ignored containers lift their children, presentational
// children are hidden, text-only elements fold their text.
func TestViewFolding(t *testing.T) {
	inline := platform.AccessInline
	tree, _, err := (*platform.AccessTree)(nil).Apply(&platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2}},
		{ID: 2, Role: "generic", Children: []platform.AccessID{3, 6, 10, 12, 14}},
		// A paragraph with a strong word: one text.
		{ID: 3, Role: "paragraph", Children: []platform.AccessID{4, 5, 7}},
		{ID: 4, Kind: platform.AccessText, Text: "Sign in to your ", States: inline},
		{ID: 5, Role: "strong", States: inline, Children: []platform.AccessID{8}},
		{ID: 8, Kind: platform.AccessText, Text: "account", States: inline},
		{ID: 7, Kind: platform.AccessText, Text: ".", States: inline},
		// A button: a leaf.
		{ID: 6, Role: "button", Name: "OK", Children: []platform.AccessID{9}},
		{ID: 9, Kind: platform.AccessText, Text: "OK", States: inline},
		// A group with a label in an ignored div and a link: not folded.
		{ID: 10, Role: "group", Children: []platform.AccessID{11, 13}},
		{ID: 11, Role: "generic", Children: []platform.AccessID{15}},
		{ID: 15, Kind: platform.AccessText, Text: "Email  me", States: inline},
		{ID: 13, Role: "link", Name: "here", Actions: platform.AccessActionsOf(platform.AccessActionDefault), Children: []platform.AccessID{16}},
		{ID: 16, Kind: platform.AccessText, Text: "here", States: inline},
		// Two blocks in a heading.
		{ID: 12, Role: "heading", Children: []platform.AccessID{17, 18}},
		{ID: 17, Role: "generic", Children: []platform.AccessID{19}},
		{ID: 19, Kind: platform.AccessText, Text: "Line one", States: inline},
		{ID: 18, Role: "generic", Children: []platform.AccessID{20}},
		{ID: 20, Kind: platform.AccessText, Text: "Line two", States: inline},
		// A text field.
		{ID: 14, Role: "textbox", Value: "ana"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	v := buildView(tree)
	if got := v.root.children; !slices.Equal(got, []platform.AccessID{3, 6, 10, 12, 14}) {
		t.Errorf("root children %v", got)
	}
	if p := v.nodes[3]; !p.hasText || p.text != "Sign in to your account." || len(p.children) != 0 {
		t.Errorf("paragraph %+v", p)
	}
	if b := v.nodes[6]; b.hasText || len(b.children) != 0 {
		t.Errorf("button %+v", b)
	}
	g := v.nodes[10]
	if !slices.Equal(g.children, []platform.AccessID{15, 13}) || g.hasText {
		t.Errorf("group %+v", g)
	}
	if l := v.nodes[15]; l.role != roleLabel || l.name() != "Email me" || l.text != "Email  me" || l.parent != 10 || l.index != 0 {
		t.Errorf("label %+v name %q", l, l.name())
	}
	if l := v.nodes[13]; !l.hasText || l.text != "here" || l.index != 1 {
		t.Errorf("link %+v", l)
	}
	if h := v.nodes[12]; h.text != "Line one\nLine two" {
		t.Errorf("heading text %q", h.text)
	}
	if f := v.nodes[14]; !f.hasText || f.text != "ana" {
		t.Errorf("field %+v", f)
	}
	if v.nodes[2] != nil || v.nodes[11] != nil || v.nodes[4] != nil {
		t.Errorf("ignored or folded nodes are exposed")
	}
	// Hit testing goes to the deepest node.
	empty := buildView(nil)
	if empty.root == nil || len(empty.root.children) != 0 {
		t.Errorf("empty view %+v", empty.root)
	}
}

func TestTextGranularity(t *testing.T) {
	text := []rune("Hello, world. Don't stop!\nNext line 3.14 here")
	str := func(s, e int) string { return string(text[s:e]) }
	for _, c := range []struct {
		offset int
		gran   uint32
		want   string
	}{
		{0, granChar, "H"},
		{2, granWord, "Hello, "},
		{5, granWord, "Hello, "},
		{8, granWord, "world. "},
		{15, granWord, "Don't "},
		{37, granWord, "3.14 "},
		{3, granSentence, "Hello, world. "},
		{16, granSentence, "Don't stop!\n"},
		{30, granSentence, "Next line 3.14 here"},
		{3, granLine, "Hello, world. Don't stop!\n"},
		{len(text), granLine, "Next line 3.14 here"},
		{30, granParagraph, "Next line 3.14 here"},
	} {
		s, e := stringAt(text, c.offset, c.gran)
		if got := str(s, e); got != c.want {
			t.Errorf("stringAt(%d, %d) = %q, want %q", c.offset, c.gran, got, c.want)
		}
	}
	for _, c := range []struct {
		offset, which int
		typ           uint32
		want          string
	}{
		{8, 0, boundaryWordEnd, ", world"},
		{8, -1, boundaryWordStart, "Hello, "},
		{8, 1, boundaryWordStart, "Don't "},
		{3, 1, boundaryLineStart, "Next line 3.14 here"},
		{30, 1, boundaryLineStart, ""},
		{30, 0, boundaryLineEnd, "\nNext line 3.14 here"},
		{3, 0, boundarySentenceEnd, "Hello, world."},
		{0, -1, boundaryChar, ""},
	} {
		s, e := segment(text, c.offset, c.typ, c.which)
		if got := str(s, e); got != c.want {
			t.Errorf("segment(%d, %d, %d) = %q, want %q", c.offset, c.typ, c.which, got, c.want)
		}
	}
	if s, e := stringAt(nil, 3, granWord); s != 0 || e != 0 {
		t.Errorf("empty text: %d, %d", s, e)
	}
	if s, e := clampRange(text, 3, -1); s != 3 || e != len(text) {
		t.Errorf("clampRange %d %d", s, e)
	}
}

func TestListeners(t *testing.T) {
	var l listenerSet
	if !l.wants("object:statechanged:focused") {
		t.Errorf("unknown listeners must let everything through")
	}
	l = listenerSet{known: true}
	l.add(":1.2", "Object:StateChanged:Focused")
	l.add(":1.2", "Window:Activate:")
	l.add(":1.2", "object:children-changed")
	for ev, want := range map[string]bool{
		"object:statechanged:focused":          true,
		"object:statechanged:checked":          false,
		"window:activate":                      true,
		"window:deactivate":                    false,
		"object:childrenchanged:add":           true,
		"object:propertychange:accessiblename": false,
	} {
		if got := l.wants(ev); got != want {
			t.Errorf("wants(%s) = %v", ev, got)
		}
	}
	l.remove(":1.2", "object:children-changed")
	if l.wants("object:childrenchanged:add") {
		t.Errorf("a removed listener still listens")
	}
	l.add(":1.3", "")
	if !l.wants("object:anything") {
		t.Errorf("a listener for everything")
	}
}

func TestExtents(t *testing.T) {
	s := &snapshot{view: buildView(nil), scale: 2, origin: imagePt(100, 50), hasOrigin: true}
	r := platform.Rect{X: 10, Y: 20, W: 30, H: 5}
	if x, y, w, h := s.extents(r, coordWindow, nil); x != 20 || y != 40 || w != 60 || h != 10 {
		t.Errorf("window %d %d %d %d", x, y, w, h)
	}
	if x, y, _, _ := s.extents(r, coordScreen, nil); x != 120 || y != 90 {
		t.Errorf("screen %d %d", x, y)
	}
	if x, y := s.point(120, 90, coordScreen, nil); x != 10 || y != 20 {
		t.Errorf("point %v %v", x, y)
	}
	rect := charRect(platform.Rect{W: 100, H: 20}, []rune("ab\ncdef"), 4)
	if rect != (platform.Rect{X: 25, Y: 10, W: 25, H: 10}) {
		t.Errorf("charRect %v", rect)
	}
	if o := offsetAt(platform.Rect{W: 100, H: 20}, []rune("ab\ncdef"), 30, 12); o != 4 {
		t.Errorf("offsetAt %d", o)
	}
}

func imagePt(x, y int) image.Point { return image.Pt(x, y) }

// A deactivation and an activation in one Dispatch reach core after a
// diff of its previous session: the bridge drops that diff and takes the
// whole tree core sends next.
func TestStaleDiffAfterReactivation(t *testing.T) {
	var logs []string
	b := New(Options{
		Wake:   func() {},
		Event:  func(platform.Window, platform.Event) {},
		Getenv: func(k string) string { return map[string]string{"NO_AT_BRIDGE": "1"}[k] },
		Logf:   func(f string, a ...any) { logs = append(logs, f) },
	})
	w := b.NewWindow(nil, "w")
	full := platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2}},
		{ID: 2, Role: "button", Name: "OK"},
	}}
	b.post(inboxItem{w: w, act: true, active: true, gen: 1})
	b.Dispatch()
	w.Update(full)
	b.post(inboxItem{w: w, act: true, active: false, gen: 1})
	b.post(inboxItem{w: w, act: true, active: true, gen: 2})
	b.Dispatch()
	w.Update(platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{{ID: 2, Role: "button", Name: "Cancel"}}})
	if w.refused || len(logs) != 0 {
		t.Fatalf("the stale diff was refused (logs %q)", logs)
	}
	w.Update(full)
	if w.tree.Len() != 2 {
		t.Fatalf("the whole tree after the stale diff: %d nodes, want 2", w.tree.Len())
	}
	// A diff that does not apply to a tree the bridge holds is a bug.
	w.Update(platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{3}}}})
	if !w.refused || len(logs) != 1 {
		t.Errorf("a bad diff over a held tree: refused %v, logs %q", w.refused, logs)
	}
}
