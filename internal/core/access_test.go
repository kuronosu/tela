package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// accessHarness drives a window whose accessibility bridge a test turns
// on and off (access.go), through the headless backend's fake bridge.
type accessHarness struct {
	*winHarness
	w    *headless.Window
	logs []string
}

// newAccess mounts root in a 400x300 headless window, with dev messages
// collected, without turning the bridge on.
func newAccess(t *testing.T, root Node) *accessHarness {
	t.Helper()
	x := &accessHarness{}
	opts := Options{Root: root}
	opts.Log = func(m string) { x.logs = append(x.logs, m) }
	x.winHarness = newWinHarness(t, opts)
	x.w = x.windows()[0]
	return x
}

// on starts an assistive technology on the window and delivers what
// follows.
func (x *accessHarness) on() {
	x.w.Inject(platform.AccessibilityEvent{Active: true})
	x.drain()
}

func (x *accessHarness) off() {
	x.w.Inject(platform.AccessibilityEvent{Active: false})
	x.drain()
}

// act sends the bridge's action and delivers what follows.
func (x *accessHarness) act(e platform.AccessActionEvent) {
	x.w.Inject(e)
	x.drain()
}

func (x *accessHarness) updates() []platform.AccessUpdate { return x.w.AccessUpdates() }

// last returns the latest update, failing when there are not want.
func (x *accessHarness) last(want int) platform.AccessUpdate {
	x.t.Helper()
	us := x.updates()
	if len(us) != want {
		x.t.Fatalf("%d updates, want %d", len(us), want)
	}
	return us[len(us)-1]
}

// node returns the node of the bridge's tree with role (or kind, for the
// root and texts) and name (or text), failing when there is none.
func (x *accessHarness) node(role, name string) *platform.AccessNode {
	x.t.Helper()
	var found *platform.AccessNode
	x.w.AccessTree().Walk(func(n *platform.AccessNode, _ int) bool {
		r, nm := n.Role, n.Name
		if n.Kind != platform.AccessElement {
			r, nm = n.Kind.String(), n.Text
		}
		if found == nil && r == role && nm == name {
			found = n
		}
		return true
	})
	if found == nil {
		x.t.Fatalf("no %s %q in the bridge's tree:\n%v", role, name, x.w.AccessTree())
	}
	return found
}

// describe lists the nodes of an update as "role name", for messages.
func describe(nodes []platform.AccessNode) string {
	var out []string
	for _, n := range nodes {
		r, nm := n.Role, n.Name
		if n.Kind != platform.AccessElement {
			r, nm = n.Kind.String(), n.Text
		}
		out = append(out, fmt.Sprintf("%s %q", r, nm))
	}
	return strings.Join(out, ", ")
}

// checkbox is a custom checkbox: a focusable div that toggles on click.
func checkbox(label string) Node {
	return Component(func() Node {
		on, set := UseState(false)
		return div(Role("checkbox"), AriaChecked(on), TabIndex(0), On("click", func(MouseEvent) { set.Set(!on) }), Text(label))
	})
}

// TestAccessActivationSendsTheWholeTree: nothing is built or sent before
// an assistive technology looks; the activation sends every node that is
// not hidden, and an idle window sends nothing more.
func TestAccessActivationSendsTheWholeTree(t *testing.T) {
	x := newAccess(t, div(h1(Text("Settings")), checkbox("Notifications"), button(Text("Save")),
		div(Class("hidden"), Text("gone")), div(AriaHidden(true), button(Text("Hidden")))))
	if n := len(x.updates()); n != 0 || x.w.AccessIgnored() != 0 {
		t.Fatalf("%d updates and %d ignored before the activation", n, x.w.AccessIgnored())
	}
	if x.h.tree.sem.root != nil {
		t.Fatal("the semantics tree was built without an assistive technology")
	}
	x.on()
	u := x.last(1)
	tr := x.w.AccessTree()
	if len(u.Nodes) != tr.Len() || u.Focus != u.Root || len(u.Announcements) != 0 {
		t.Errorf("first update: %d nodes for a tree of %d, focus %d root %d, %d announcements", len(u.Nodes), tr.Len(), u.Focus, u.Root, len(u.Announcements))
	}
	want := `root
  generic
    heading "Settings"
      text "Settings" [inline]
    checkbox "Notifications" [focusable]
      text "Notifications" [inline]
    button "Save" [focusable]
      text "Save" [inline]
`
	if got := tr.String(); got != want {
		t.Errorf("tree\n%s\nwant\n%s", got, want)
	}
	if h := x.node("heading", "Settings"); h.Level != 1 || !h.Actions.Has(platform.AccessActionScrollIntoView) {
		t.Errorf("heading %+v", h)
	}
	if b := x.node("button", "Save"); !b.Actions.Has(platform.AccessActionDefault) || !b.Actions.Has(platform.AccessActionFocus) || b.Bounds.W <= 0 || b.Bounds.H <= 0 {
		t.Errorf("button %+v", b)
	}
	// More frames and wakes change nothing: no update.
	x.w.RequestFrame()
	x.b.Wake()
	x.drain()
	x.last(1)
}

// TestAccessChangeSendsTheChangedNode: a checkbox click sends the
// checkbox alone; focus moves send the nodes whose state changed and the
// new focus.
func TestAccessChangeSendsTheChangedNode(t *testing.T) {
	x := newAccess(t, div(checkbox("Notifications"), button(Text("Save"))))
	x.on()
	cb := x.node("checkbox", "Notifications")
	x.act(platform.AccessActionEvent{Node: cb.ID, Action: platform.AccessActionDefault})
	u := x.last(2)
	if len(u.Nodes) != 1 || u.Nodes[0].ID != cb.ID || u.Nodes[0].Checked != platform.ToggleTrue {
		t.Errorf("after the click: nodes %s, want the checked checkbox alone", describe(u.Nodes))
	}
	if u.Focus != u.Root {
		t.Errorf("a click focused %d", u.Focus)
	}

	// Tab moves the focus to the checkbox, then to the button.
	x.w.Inject(platform.FocusEvent{Focused: true})
	tab := func() {
		x.w.Inject(platform.KeyEvent{Action: platform.KeyPress, Key: platform.KeyTab, Code: platform.CodeTab})
		x.w.Inject(platform.KeyEvent{Action: platform.KeyRelease, Key: platform.KeyTab, Code: platform.CodeTab})
		x.drain()
	}
	tab()
	u = x.last(3)
	if u.Focus != cb.ID || len(u.Nodes) != 1 || !u.Nodes[0].Has(platform.AccessFocused) {
		t.Errorf("after Tab: focus %d, nodes %s; want the checkbox %d focused", u.Focus, describe(u.Nodes), cb.ID)
	}
	tab()
	u = x.last(4)
	save := x.node("button", "Save")
	if u.Focus != save.ID || len(u.Nodes) != 2 || x.w.AccessTree().Focus() != save.ID {
		t.Errorf("after the second Tab: focus %d, nodes %s; want the button %d and the checkbox", u.Focus, describe(u.Nodes), save.ID)
	}
}

// TestAccessRemoval: a node removed from the tree is no longer reachable
// from the root (its parent is sent with its new children), and the
// bridge drops it.
func TestAccessRemoval(t *testing.T) {
	x := newAccess(t, Component(func() Node {
		show, set := UseState(true)
		return div(button(Text("Toggle"), On("click", func(MouseEvent) { set.Set(!show) })),
			If(show, p(Text("Details"))))
	}))
	x.on()
	para := x.node("paragraph", "").ID
	x.act(platform.AccessActionEvent{Node: x.node("button", "Toggle").ID, Action: platform.AccessActionDefault})
	u := x.last(2)
	if len(u.Nodes) != 1 || u.Nodes[0].Role != "generic" || len(u.Nodes[0].Children) != 1 {
		t.Errorf("after the removal: nodes %s; want the parent with one child", describe(u.Nodes))
	}
	if _, ok := x.w.AccessTree().Node(para); ok {
		t.Error("the removed paragraph is still in the bridge's tree")
	}
	// It comes back as a new node, sent whole.
	x.act(platform.AccessActionEvent{Node: x.node("button", "Toggle").ID, Action: platform.AccessActionDefault})
	u = x.last(3)
	if got := describe(u.Nodes); got != `generic "", paragraph "", text "Details"` {
		t.Errorf("after showing it again: %s", got)
	}
}

// TestAccessAnnouncements: live regions announce what changed after the
// activation: an atomic region its whole text, another its additions,
// assertive ones assertive; nothing at the activation, nothing while
// busy.
func TestAccessAnnouncements(t *testing.T) {
	var (
		setSaved  func(int)
		setLog    func([]string)
		setAlert  func(bool)
		setBusy   func(bool)
		setFeed   func(string)
		setStatic func(string)
	)
	x := newAccess(t, div(
		Component(func() Node {
			n, set := UseState(0)
			setSaved = set.Set
			return div(Role("status"), Text("Saved "), span(Textf("%d", n)), Text(" files"))
		}),
		Component(func() Node {
			lines, set := UseState([]string{"First"})
			setLog = set.Set
			items := make([]Child, len(lines))
			for i, l := range lines {
				items[i] = p(Key(l), Text(l))
			}
			return div(AriaLive("polite"), Fragment(items...))
		}),
		Component(func() Node {
			show, set := UseState(false)
			setAlert = set.Set
			return div(If(show, div(Role("alert"), Text("Disk full"))))
		}),
		Component(func() Node {
			busy, set := UseState(false)
			setBusy = set.Set
			feed, setF := UseState("")
			setFeed = setF.Set
			return div(AriaLive("polite"), AriaBusy(busy), Text(feed))
		}),
		Component(func() Node {
			s, set := UseState("Ready")
			setStatic = set.Set
			return div(Role("alert"), Text(s))
		}),
	))
	x.on()
	if u := x.last(1); len(u.Announcements) != 0 {
		t.Fatalf("the activation announced %+v", u.Announcements)
	}
	step := func(name string, change func(), want ...platform.Announcement) {
		t.Helper()
		before := len(x.updates())
		change()
		x.drain()
		var got []platform.Announcement
		for _, u := range x.updates()[before:] {
			got = append(got, u.Announcements...)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: announced %+v, want %+v", name, got, want)
		}
	}
	step("atomic status", func() { setSaved(2) }, platform.Announcement{Text: "Saved 2 files"})
	step("an addition", func() { setLog([]string{"First", "Second"}) }, platform.Announcement{Text: "Second"})
	step("two additions around an old line", func() { setLog([]string{"Zero", "First", "Second", "Third"}) },
		platform.Announcement{Text: "Zero Third"})
	step("a removal", func() { setLog([]string{"Third"}) })
	step("an inserted alert", func() { setAlert(true) }, platform.Announcement{Text: "Disk full", Assertive: true})
	step("an alert removed", func() { setAlert(false) })
	step("busy", func() { setBusy(true); setFeed("Loading") })
	step("still busy", func() { setFeed("Loading more") })
	step("no longer busy", func() { setBusy(false) }, platform.Announcement{Text: "Loading more"})
	step("an alert present at the activation", func() { setStatic("Ready") })
	step("its text changes", func() { setStatic("Offline") }, platform.Announcement{Text: "Offline", Assertive: true})
	step("emptied", func() { setStatic("") })
}

// TestAccessActions: each action of the bridge runs the node's action.
func TestAccessActions(t *testing.T) {
	clicks, menus := 0, 0
	var keys []string
	rows := make([]Child, 30)
	for i := range rows {
		rows[i] = div(Class("h-10"), Textf("Row %d", i))
	}
	rows[25] = button(Class("h-10"), Text("Target"))
	x := newAccess(t, div(
		button(Text("Count"), On("click", func(MouseEvent) { clicks++ })),
		input(Key("email"), AriaLabel("Email")),
		input(Key("name"), AriaLabel("Name"), DefaultValue("Ana Lucía")),
		Component(func() Node { return stepSlider("Volume", &keys) }),
		Component(func() Node {
			open, set := UseState(false)
			return button(AriaExpanded(open), On("click", func(MouseEvent) { set.Set(!open) }), Text("More"))
		}),
		div(Role("region"), AriaLabel("Menu area"), Class("h-10"), On("contextmenu", func(MouseEvent) { menus++ })),
		div(Key("list"), Role("list"), AriaLabel("Rows"), Class("h-40 overflow-y-auto"), Fragment(rows...)),
	))
	x.on()
	do := func(role, name string, a platform.AccessAction, set func(*platform.AccessActionEvent)) {
		t.Helper()
		e := platform.AccessActionEvent{Node: x.node(role, name).ID, Action: a}
		if set != nil {
			set(&e)
		}
		x.act(e)
	}
	do("button", "Count", platform.AccessActionDefault, nil)
	if clicks != 1 {
		t.Errorf("%d clicks after the default action", clicks)
	}
	do("button", "Count", platform.AccessActionFocus, nil)
	if f := x.w.AccessTree().Focus(); f != x.node("button", "Count").ID {
		t.Errorf("focus %d after the focus action", f)
	}
	do("textbox", "Email", platform.AccessActionSetValue, func(e *platform.AccessActionEvent) { e.Value = "ana@example.com" })
	if n := x.node("textbox", "Email"); n.Value != "ana@example.com" {
		t.Errorf("Email %q after set value", n.Value)
	}
	do("textbox", "Name", platform.AccessActionSetSelection, func(e *platform.AccessActionEvent) { e.Selection = platform.AccessSelection{Anchor: 9, Focus: 4} })
	if n := x.node("textbox", "Name"); !n.HasSelection || n.Selection != (platform.AccessSelection{Anchor: 9, Focus: 4}) {
		t.Errorf("Name selection %+v (set %v)", n.Selection, n.HasSelection)
	}
	do("slider", "Volume", platform.AccessActionIncrement, nil)
	do("slider", "Volume", platform.AccessActionIncrement, nil)
	do("slider", "Volume", platform.AccessActionDecrement, nil)
	if n := x.node("slider", "Volume"); n.Value != "6" || n.Range.Now != 6 {
		t.Errorf("slider %q (%v) after +1 +1 -1 from 5", n.Value, n.Range.Now)
	}
	do("button", "More", platform.AccessActionExpand, nil)
	if n := x.node("button", "More"); n.Expanded != platform.ToggleTrue || !n.Actions.Has(platform.AccessActionCollapse) {
		t.Errorf("after expand %+v", n)
	}
	do("button", "More", platform.AccessActionCollapse, nil)
	if n := x.node("button", "More"); n.Expanded != platform.ToggleFalse {
		t.Errorf("after collapse %v", n.Expanded)
	}
	do("region", "Menu area", platform.AccessActionShowMenu, nil)
	if menus != 1 {
		t.Errorf("%d context menus after show menu", menus)
	}
	list := x.node("list", "Rows")
	top := list.Bounds.Y
	do("button", "Target", platform.AccessActionScrollIntoView, nil)
	if tg := x.node("button", "Target"); tg.Bounds.Y+tg.Bounds.H != top+160 {
		t.Errorf("target at %v after scroll into view; the list's bottom is %v", tg.Bounds, top+160)
	}
	y := keyed(x.h.tree, "list").box.scr.pos.Y
	do("list", "Rows", platform.AccessActionScrollBy, func(e *platform.AccessActionEvent) { e.DY = -30 })
	if got := keyed(x.h.tree, "list").box.scr.pos.Y; got != y-30 {
		t.Errorf("list scrolled to %v after scrolling by -30 from %v", got, y)
	}
	// A node that is gone, and an action a node cannot do: logged, no
	// harm.
	x.logs = nil
	x.act(platform.AccessActionEvent{Node: 99999, Action: platform.AccessActionDefault})
	do("button", "Count", platform.AccessActionIncrement, nil)
	if len(x.logs) != 2 || !strings.Contains(x.logs[0], "node 99999") || !strings.Contains(x.logs[1], "increment") {
		t.Errorf("logs %q", x.logs)
	}
}

// TestAccessDeactivation: once the assistive technology stops, nothing is
// sent; the next activation sends the whole tree again.
func TestAccessDeactivation(t *testing.T) {
	var setN func(int)
	x := newAccess(t, Component(func() Node {
		n, set := UseState(0)
		setN = set.Set
		return div(Role("status"), Textf("%d new", n))
	}))
	x.on()
	x.off()
	if x.h.main.acc != nil {
		t.Error("the accessibility state is kept after the deactivation")
	}
	setN(3)
	x.drain()
	x.last(1)
	if x.w.AccessIgnored() != 0 {
		t.Errorf("%d updates sent to an inactive bridge", x.w.AccessIgnored())
	}
	x.on()
	u := x.last(2)
	if len(u.Nodes) != x.w.AccessTree().Len() || len(u.Announcements) != 0 {
		t.Errorf("reactivation: %d nodes of %d, announcements %+v", len(u.Nodes), x.w.AccessTree().Len(), u.Announcements)
	}
	if n := x.node("text", "3 new"); n == nil {
		t.Error("the reactivation's tree is not current")
	}
}

// TestAccessIdleCostsNothing: without an assistive technology the check
// after each event allocates nothing and builds no semantics; with one
// and nothing changed, it allocates nothing either.
func TestAccessIdleCostsNothing(t *testing.T) {
	var setN func(int)
	x := newAccess(t, Component(func() Node {
		n, set := UseState(0)
		setN = set.Set
		return div(button(Textf("%d", n)))
	}))
	setN(1)
	x.drain()
	if x.h.tree.sem.root != nil {
		t.Fatal("frames built the semantics tree without an assistive technology")
	}
	if a := testing.AllocsPerRun(100, x.h.syncAccess); a != 0 {
		t.Errorf("syncAccess without a bridge: %v allocations", a)
	}
	x.on()
	if a := testing.AllocsPerRun(100, x.h.syncAccess); a != 0 {
		t.Errorf("syncAccess with an idle bridge: %v allocations", a)
	}
}

// TestAccessDeferredName: a Value read by an AriaLabelFn changes the
// name without a frame; the update follows in the same loop turn.
func TestAccessDeferredName(t *testing.T) {
	var pct *Value[int]
	x := newAccess(t, Component(func() Node {
		pct = UseValue(10)
		return div(div(Role("progressbar"), AriaLabelFn(func() string { return fmt.Sprintf("Upload %d%%", pct.Get()) })))
	}))
	x.on()
	presents := x.w.Presents()
	pct.Set(55)
	x.drain()
	u := x.last(2)
	if len(u.Nodes) != 1 || u.Nodes[0].Name != "Upload 55%" {
		t.Errorf("nodes %s, want the progressbar renamed", describe(u.Nodes))
	}
	if x.w.Presents() != presents {
		t.Errorf("the name change presented %d frames", x.w.Presents()-presents)
	}
}

// TestAccessChildWindow: a child window has its own tree and bridge.
func TestAccessChildWindow(t *testing.T) {
	x := newAccess(t, div(Text("main content"), win(ChildWindowSpec{
		WindowSpec: WindowSpec{Title: "Child", Width: 200, Height: 100},
		OnClose:    func() {}, Root: div(button(Text("Child button"))),
	})))
	c := x.child(0)
	c.Inject(platform.AccessibilityEvent{Active: true})
	x.drain()
	if n := len(x.updates()); n != 0 {
		t.Errorf("the main window got %d updates", n)
	}
	if got := c.AccessTree().String(); !strings.Contains(got, `button "Child button"`) || strings.Contains(got, "main content") {
		t.Errorf("child window tree:\n%s", got)
	}
}
