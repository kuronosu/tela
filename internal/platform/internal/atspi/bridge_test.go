//go:build linux

package atspi

import (
	"slices"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// el returns an element node.
func el(id platform.AccessID, role, name string, children ...platform.AccessID) platform.AccessNode {
	return platform.AccessNode{ID: id, Role: role, Name: name, Children: children,
		Bounds: platform.Rect{X: 10, Y: float32(id) * 20, W: 100, H: 20}}
}

// txt returns a text node.
func txt(id platform.AccessID, text string) platform.AccessNode {
	return platform.AccessNode{ID: id, Kind: platform.AccessText, Text: text, States: platform.AccessInline,
		Bounds: platform.Rect{X: 10, Y: float32(id) * 20, W: 50, H: 20}}
}

// loginTree is the first update of a small form.
func loginTree() platform.AccessUpdate {
	root := platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2},
		Bounds: platform.Rect{W: 400, H: 300}}
	box := el(2, "generic", "", 3, 5, 6, 8)
	heading := el(3, "heading", "Welcome", 4)
	heading.Level = 1
	email := el(5, "textbox", "Email")
	email.States = platform.AccessFocusable
	email.Placeholder = "you@example.com"
	email.Actions = platform.AccessActionsOf(platform.AccessActionFocus, platform.AccessActionSetValue,
		platform.AccessActionSetSelection, platform.AccessActionScrollIntoView)
	button := el(6, "button", "Sign in", 7)
	button.States = platform.AccessFocusable
	button.Actions = platform.AccessActionsOf(platform.AccessActionDefault, platform.AccessActionFocus)
	check := el(8, "checkbox", "Remember me")
	check.Checked = platform.ToggleFalse
	check.States = platform.AccessFocusable
	check.Actions = platform.AccessActionsOf(platform.AccessActionDefault, platform.AccessActionFocus)
	return platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		root, box, heading, txt(4, "Welcome"), email, button, txt(7, "Sign in"), check}}
}

// activate starts an AT on a fresh bus and returns everything, with the
// window's first update applied.
func activate(t *testing.T, events ...string) (*harness, *Window, *fakeLauncher, *fakeRegistry, *at) {
	t.Helper()
	addr := privateBus(t)
	l := startLauncher(t, addr, false)
	r := startRegistry(t, addr, events...)
	h := newHarness(t, addr, nil)
	win := &stubWin{name: "main"}
	w := h.b.NewWindow(win, "Login")

	// Nothing registers while no AT is enabled.
	time.Sleep(100 * time.Millisecond)
	h.b.Dispatch()
	if emb, _ := r.plugs(); len(emb) != 0 || len(h.events) != 0 {
		t.Fatalf("registered with no AT enabled: embeds %v, events %v", emb, h.events)
	}

	l.setEnabled(true)
	d := h.next("activation")
	if d.w != win || d.e != (platform.AccessibilityEvent{Active: true}) {
		t.Fatalf("got %v on %v, want the activation", d.e, d.w.name)
	}
	emb, _ := r.plugs()
	if len(emb) != 1 || emb[0].Path != appRootPath {
		t.Fatalf("embedded %v", emb)
	}
	a := newAT(t, addr)
	a.app = emb[0].Name
	w.Update(loginTree())
	// The tree reaches the worker asynchronously.
	eventually(t, "the tree", func() bool { return w.snap.Load().view.tree != nil })
	return h, w, l, r, a
}

func TestBridgeTree(t *testing.T) {
	_, w, _, r, a := activate(t)

	// The application.
	if got := a.role(appRootPath); got != roleApplication {
		t.Errorf("app role %d", got)
	}
	if got := a.name(appRootPath); got != "tela-test" {
		t.Errorf("app name %q", got)
	}
	parent := refOf(a.prop(appRootPath, ifaceAccessible, "Parent"))
	if parent.Path != desktopRootPath {
		t.Errorf("app parent %v, want the desktop", parent)
	}
	eventually(t, "the registry's Id", func() bool { return a.prop(appRootPath, ifaceApplication, "Id").(int32) == 7 })
	if got := a.prop(appRootPath, ifaceApplication, "ToolkitName"); got != "Tela" {
		t.Errorf("toolkit %v", got)
	}

	// The frame.
	frames := a.children(appRootPath)
	if len(frames) != 1 || frames[0].Path != w.path {
		t.Fatalf("app children %v", frames)
	}
	f := frames[0].Path
	if a.role(f) != roleFrame || a.name(f) != "Login" {
		t.Errorf("frame role %d name %q", a.role(f), a.name(f))
	}
	if p := refOf(a.prop(f, ifaceAccessible, "Parent")); p.Path != appRootPath {
		t.Errorf("frame parent %v", p)
	}
	var app ref
	a.must(f, ifaceAccessible+".GetApplication", nil, &app)
	if app.Path != appRootPath || app.Name != a.app {
		t.Errorf("GetApplication %v", app)
	}

	// Its children: the generic box is left out.
	kids := a.children(f)
	var roles []uint32
	var names []string
	for _, k := range kids {
		roles = append(roles, a.role(k.Path))
		names = append(names, a.name(k.Path))
	}
	if !slices.Equal(roles, []uint32{roleHeading, roleEntry, rolePushButton, roleCheckBox}) ||
		!slices.Equal(names, []string{"Welcome", "Email", "Sign in", "Remember me"}) {
		t.Fatalf("frame children roles %v names %q", roles, names)
	}
	heading, email, button, check := kids[0].Path, kids[1].Path, kids[2].Path, kids[3].Path
	if p := refOf(a.prop(heading, ifaceAccessible, "Parent")); p.Path != f {
		t.Errorf("heading parent %v", p)
	}
	var idx int32
	a.must(check, ifaceAccessible+".GetIndexInParent", nil, &idx)
	if idx != 3 {
		t.Errorf("checkbox index %d", idx)
	}
	count := a.prop(button, ifaceAccessible, "ChildCount").(int32)
	if count != 0 {
		t.Errorf("a button has %d children; its content is its name", count)
	}
	var attrs map[string]string
	a.must(heading, ifaceAccessible+".GetAttributes", nil, &attrs)
	if attrs["level"] != "1" || attrs["xml-roles"] != "heading" {
		t.Errorf("heading attributes %v", attrs)
	}
	a.must(email, ifaceAccessible+".GetAttributes", nil, &attrs)
	if attrs["placeholder-text"] != "you@example.com" {
		t.Errorf("email attributes %v", attrs)
	}
	var text string
	a.must(heading, ifaceText+".GetText", []any{int32(0), int32(-1)}, &text)
	if text != "Welcome" {
		t.Errorf("heading text %q", text)
	}
	var ifaces []string
	a.must(email, ifaceAccessible+".GetInterfaces", nil, &ifaces)
	for _, want := range []string{ifaceAccessible, ifaceComponent, ifaceText, ifaceEditableText} {
		if !slices.Contains(ifaces, want) {
			t.Errorf("email interfaces %v lack %s", ifaces, want)
		}
	}
	st := a.states(check)
	for _, s := range []int{stateEnabled, stateSensitive, stateVisible, stateShowing, stateFocusable, stateCheckable} {
		if !st.has(s) {
			t.Errorf("checkbox lacks %s", stateNames[s])
		}
	}
	if st.has(stateChecked) || st.has(stateFocused) {
		t.Errorf("checkbox states %b", st)
	}
	if st := a.states(email); !st.has(stateEditable) || !st.has(stateSingleLine) {
		t.Errorf("email states lack editable or single-line")
	}

	// Component: window coordinates are the node's bounds (scale 1).
	var e extents
	a.must(button, ifaceComponent+".GetExtents", []any{uint32(coordWindow)}, &e)
	if e != (extents{10, 120, 100, 20}) {
		t.Errorf("button extents %v", e)
	}
	var hit ref
	a.must(f, ifaceComponent+".GetAccessibleAtPoint", []any{int32(20), int32(165), uint32(coordWindow)}, &hit)
	if hit.Path != check {
		t.Errorf("hit %v, want the checkbox %v", hit, check)
	}

	// A node that is not exposed (the generic box) or does not exist.
	var states []uint32
	a.must(w.path+"_99", ifaceAccessible+".GetState", nil, &states)
	if !(stateSet(states[0]) | stateSet(states[1])<<32).has(stateDefunct) {
		t.Errorf("a missing node is not defunct: %v", states)
	}
	if err := a.call(w.path+"_99", ifaceAccessible+".GetRole", nil, new(uint32)); err == nil {
		t.Errorf("GetRole on a missing node succeeded")
	}
	_ = r
}

func refOf(v any) ref {
	// A (so) in a variant decodes as []any.
	if s, ok := v.([]any); ok && len(s) == 2 {
		name, _ := s[0].(string)
		path, _ := s[1].(dbus.ObjectPath)
		return ref{name, path}
	}
	return ref{}
}

func TestBridgeActions(t *testing.T) {
	h, w, _, _, a := activate(t)
	kids := a.children(w.path)
	email, button, check := kids[1].Path, kids[2].Path, kids[3].Path

	var names []actionInfo
	a.must(button, ifaceAction+".GetActions", nil, &names)
	var name string
	a.must(button, ifaceAction+".GetName", []any{int32(0)}, &name)
	if len(names) != 1 || name != "click" {
		t.Fatalf("button actions %v, first %q", names, name)
	}
	var ok bool
	a.must(button, ifaceAction+".DoAction", []any{int32(0)}, &ok)
	if !ok {
		t.Fatal("DoAction answered false")
	}
	d := h.next("the click")
	if d.e != (platform.AccessActionEvent{Node: 6, Action: platform.AccessActionDefault}) {
		t.Errorf("got %+v", d.e)
	}
	a.must(check, ifaceComponent+".GrabFocus", nil, &ok)
	if d := h.next("the focus"); d.e != (platform.AccessActionEvent{Node: 8, Action: platform.AccessActionFocus}) {
		t.Errorf("got %+v", d.e)
	}
	a.must(email, ifaceEditableText+".SetTextContents", []any{"ana@example.com"}, &ok)
	if d := h.next("the value"); d.e != (platform.AccessActionEvent{Node: 5, Action: platform.AccessActionSetValue, Value: "ana@example.com"}) {
		t.Errorf("got %+v", d.e)
	}
	a.must(email, ifaceText+".SetSelection", []any{int32(0), int32(1), int32(3)}, &ok)
	want := platform.AccessActionEvent{Node: 5, Action: platform.AccessActionSetSelection, Selection: platform.AccessSelection{Anchor: 0, Focus: 0}}
	if d := h.next("the selection"); d.e != want {
		t.Errorf("got %+v, want %+v (clamped to the empty text)", d.e, want)
	}
	// A node without the action says no and sends nothing.
	a.must(kids[0].Path, ifaceComponent+".GrabFocus", nil, &ok)
	if ok {
		t.Errorf("GrabFocus on a heading answered true")
	}

	// Actions of an earlier activation are dropped.
	a.must(button, ifaceAction+".DoAction", []any{int32(0)}, &ok)
	eventually(t, "the queued click", func() bool {
		h.b.mu.Lock()
		defer h.b.mu.Unlock()
		return len(h.b.inbox) > 0
	})
	w.uiGen++ // as if a new activation had been delivered
	h.b.Dispatch()
	if len(h.events) != 0 {
		t.Errorf("an action of another activation was delivered: %v", h.events)
	}
}

func TestBridgeEvents(t *testing.T) {
	_, w, _, _, a := activate(t)
	a.drain()
	f := w.path
	node := func(id platform.AccessID) dbus.ObjectPath {
		return w.objectPath(&vnode{n: &platform.AccessNode{ID: id}})
	}

	// The checkbox is checked and takes the focus.
	u := loginTree()
	u.Nodes = u.Nodes[7:]
	u.Nodes[0].Checked = platform.ToggleTrue
	u.Nodes[0].States |= platform.AccessFocused
	u.Focus = 8
	w.Update(u)
	evs := a.waitEvent("focused", func(e event) bool { return e.name == "StateChanged" && e.detail == "focused" })
	if !hasEvent(evs, node(8), "StateChanged", "checked", 1) {
		t.Errorf("no checked event in %v", evs)
	}
	if e := evs[len(evs)-1]; e.path != node(8) || e.d1 != 1 {
		t.Errorf("focus event %v", e)
	}

	// The button is renamed, a child is added and the email gets text with
	// a caret.
	u = loginTree()
	box := u.Nodes[1]
	box.Children = []platform.AccessID{3, 5, 6, 8, 9}
	button := u.Nodes[5]
	button.Name = "Signing in…"
	email := u.Nodes[4]
	email.Value = "ana"
	email.Selection, email.HasSelection = platform.AccessSelection{Anchor: 3, Focus: 3}, true
	status := el(9, "status", "")
	status.Live = "polite"
	status.Children = []platform.AccessID{10}
	w.Update(platform.AccessUpdate{Root: 1, Focus: 8, Nodes: []platform.AccessNode{box, button, email, status, txt(10, "Saved")},
		Announcements: []platform.Announcement{{Text: "Saved"}}})
	evs = a.waitEvent("the announcement", func(e event) bool { return e.name == "Announcement" })
	for _, want := range []struct {
		path         dbus.ObjectPath
		name, detail string
		d1           int32
		data         any
	}{
		{f, "ChildrenChanged", "add", 4, nil},
		{node(6), "PropertyChange", "accessible-name", 0, "Signing in…"},
		{node(5), "TextChanged", "insert", 0, "ana"},
		{node(5), "TextCaretMoved", "", 3, nil},
		{f, "Announcement", "", livePolite, "Saved"},
	} {
		found := false
		for _, e := range evs {
			if e.path == want.path && e.name == want.name && e.detail == want.detail && e.d1 == want.d1 &&
				(want.data == nil || e.data == want.data) {
				found = true
			}
		}
		if !found {
			t.Errorf("no %s %s:%s(%d) %v in %v", want.path, want.name, want.detail, want.d1, want.data, evs)
		}
	}
	// libatspi updates its cache from the event: the child is the new one.
	for _, e := range evs {
		if e.name == "ChildrenChanged" {
			if r := refOf(e.data); r.Path != node(9) {
				t.Errorf("children-changed child %v", e.data)
			}
		}
	}

	// A removal and a deletion.
	box.Children = []platform.AccessID{3, 5, 8}
	email.Value = "a"
	email.Selection = platform.AccessSelection{Anchor: 1, Focus: 1}
	w.Update(platform.AccessUpdate{Root: 1, Focus: 8, Nodes: []platform.AccessNode{box, email}})
	evs = a.waitEvent("the caret", func(e event) bool { return e.name == "TextCaretMoved" })
	if !hasEvent(evs, f, "ChildrenChanged", "remove", 4) || !hasEvent(evs, f, "ChildrenChanged", "remove", 2) {
		t.Errorf("no removals of the status (4) then the button (2) in %v", evs)
	}
	if !hasEvent(evs, node(5), "TextChanged", "delete", 1) {
		t.Errorf("no deletion in %v", evs)
	}
	if kids := a.children(f); len(kids) != 3 {
		t.Errorf("frame children %v after the removal", kids)
	}
}

func hasEvent(evs []event, path dbus.ObjectPath, name, detail string, d1 int32) bool {
	for _, e := range evs {
		if e.path == path && e.name == name && e.detail == detail && e.d1 == d1 {
			return true
		}
	}
	return false
}

func TestBridgeWindowFocus(t *testing.T) {
	_, w, _, _, a := activate(t)
	a.drain()
	w.SetTitle("Sign in")
	w.SetFocused(true)
	evs := a.waitEvent("activate", func(e event) bool { return e.name == "Activate" })
	if !hasEvent(evs, w.path, "PropertyChange", "accessible-name", 0) || !hasEvent(evs, w.path, "StateChanged", "active", 1) {
		t.Errorf("events %v", evs)
	}
	if !a.states(w.path).has(stateActive) {
		t.Errorf("an active window's frame is not active")
	}
	w.SetFocused(false)
	a.waitEvent("deactivate", func(e event) bool { return e.name == "Deactivate" })
}

// TestBridgeListeners: with the registry's list of registered events,
// only the events an AT listens to go out.
func TestBridgeListeners(t *testing.T) {
	_, w, _, _, a := activate(t, "Object:StateChanged:Focused", "Window:Activate:")
	a.drain()
	u := loginTree()
	u.Nodes = u.Nodes[7:]
	u.Nodes[0].Checked = platform.ToggleTrue
	u.Nodes[0].States |= platform.AccessFocused
	u.Focus = 8
	w.Update(u)
	evs := a.waitEvent("focused", func(e event) bool { return e.name == "StateChanged" && e.detail == "focused" })
	if len(evs) != 1 {
		t.Errorf("events %v, want only the focus", evs)
	}
	w.SetFocused(true)
	evs = a.waitEvent("activate", func(e event) bool { return e.name == "Activate" })
	for _, e := range evs {
		if e.name == "StateChanged" && e.detail == "active" {
			t.Errorf("state-changed:active went out without a listener")
		}
	}
}

func TestBridgeDeactivation(t *testing.T) {
	h, w, l, r, a := activate(t)
	l.setEnabled(false)
	d := h.next("the deactivation")
	if d.e != (platform.AccessibilityEvent{Active: false}) {
		t.Fatalf("got %v", d.e)
	}
	if _, un := r.plugs(); len(un) != 1 {
		t.Errorf("unembedded %v", un)
	}
	// Updates are dropped while inactive.
	w.Update(loginTree())
	if w.tree != nil {
		t.Errorf("an update was applied while inactive")
	}
	// Enabled again: a new activation from an empty tree.
	l.setEnabled(true)
	if d := h.next("the activation"); d.e != (platform.AccessibilityEvent{Active: true}) {
		t.Fatalf("got %v", d.e)
	}
	w.Update(loginTree())
	eventually(t, "the tree", func() bool { return w.snap.Load().view.tree != nil })
	emb, _ := r.plugs()
	a.app = emb[len(emb)-1].Name
	if kids := a.children(w.path); len(kids) != 4 {
		t.Errorf("frame children %v", kids)
	}
}

func TestBridgeWindows(t *testing.T) {
	h, w, _, _, a := activate(t)
	a.drain()
	win2 := &stubWin{name: "second"}
	w2 := h.b.NewWindow(win2, "Preferences")
	if d := h.next("the second window's activation"); d.w != win2 || d.e != (platform.AccessibilityEvent{Active: true}) {
		t.Fatalf("got %v on %v", d.e, d.w.name)
	}
	evs := a.waitEvent("the new frame", func(e event) bool { return e.name == "ChildrenChanged" })
	if e := evs[len(evs)-1]; e.path != appRootPath || e.detail != "add" || e.d1 != 1 {
		t.Errorf("event %v", e)
	}
	if kids := a.children(appRootPath); len(kids) != 2 || kids[1].Path != w2.path {
		t.Errorf("app children %v", kids)
	}
	w.Close()
	evs = a.waitEvent("the frame removed", func(e event) bool { return e.name == "ChildrenChanged" })
	if e := evs[len(evs)-1]; e.path != appRootPath || e.detail != "remove" || e.d1 != 0 {
		t.Errorf("event %v", e)
	}
	if kids := a.children(appRootPath); len(kids) != 1 || kids[0].Path != w2.path {
		t.Errorf("app children %v", kids)
	}
	if err := a.call(w.path, ifaceAccessible+".GetRole", nil, new(uint32)); err == nil {
		t.Errorf("a closed window's frame still answers")
	}
}

// TestBridgeNoSessionBus: without a session bus nothing happens, and
// Close returns.
func TestBridgeNoSessionBus(t *testing.T) {
	h := newHarness(t, "unix:path=/nonexistent/tela-bus", nil)
	h.b.NewWindow(&stubWin{}, "x")
	time.Sleep(50 * time.Millisecond)
	h.b.Dispatch()
	if len(h.events) != 0 {
		t.Errorf("events %v", h.events)
	}
	done := make(chan struct{})
	go func() { h.b.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
}

// TestBridgeDisabled: NO_AT_BRIDGE=1 starts nothing.
func TestBridgeDisabled(t *testing.T) {
	addr := privateBus(t)
	startLauncher(t, addr, true)
	r := startRegistry(t, addr)
	h := newHarness(t, addr, map[string]string{"NO_AT_BRIDGE": "1"})
	h.b.NewWindow(&stubWin{}, "x")
	time.Sleep(200 * time.Millisecond)
	h.b.Dispatch()
	if emb, _ := r.plugs(); len(emb) != 0 || len(h.events) != 0 {
		t.Errorf("embeds %v events %v", emb, h.events)
	}
}
