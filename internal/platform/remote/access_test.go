package remote

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// TestAccessUpdateRoundTrip: an update with every field set, and random
// ones, survive the wire (a field added to AccessNode but not to its
// encoding fails here).
func TestAccessUpdateRoundTrip(t *testing.T) {
	trip := func(u platform.AccessUpdate) platform.AccessUpdate {
		t.Helper()
		e := newMsg(msgSetAccessibility)
		e.accessUpdate(&u)
		b := e.bytes()
		d := &dec{b: b[5:]}
		got := d.accessUpdate()
		if err := d.end(); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got
	}
	full := platform.AccessUpdate{Root: 1, Focus: 3, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{3}},
		{ID: 3, Role: "slider", Name: "Volume", Description: "d", Value: "40", Text: "t", URL: "u", Placeholder: "p", KeyShortcuts: "Control+V",
			Range:  platform.AccessRange{Now: 40, Min: -1.5, Max: 100, HasNow: true, HasMin: true, HasMax: true},
			States: platform.AccessFocused | platform.AccessInline, Checked: platform.ToggleMixed, Pressed: platform.ToggleTrue,
			Selected: platform.ToggleFalse, Expanded: platform.ToggleTrue, Actions: platform.AccessActionsOf(platform.AccessActionIncrement),
			Level: 2, PosInSet: 3, SetSize: -1, RowCount: 7, RowIndex: 2, Sort: "ascending", Current: "page", HasPopup: "menu",
			Orientation: "vertical", AutoComplete: "both", Live: "assertive", LiveRoot: 3, Controls: 9, ActiveDescendant: 10,
			Selection: platform.AccessSelection{Anchor: 4, Focus: 1}, HasSelection: true, Bounds: platform.Rect{X: -3, Y: 4.5, W: 10, H: 20}},
	}, Announcements: []platform.Announcement{{Text: "Saved", Assertive: true}, {Text: "Polite"}}}
	if got := trip(full); !reflect.DeepEqual(got, full) {
		t.Errorf("round trip\n got %+v\nwant %+v", got, full)
	}
	r := rand.New(rand.NewSource(2))
	for range 100 {
		v, _ := quick.Value(reflect.TypeOf(platform.AccessUpdate{}), r)
		nilEmpty(v)
		u := v.Interface().(platform.AccessUpdate)
		if got := trip(u); !reflect.DeepEqual(got, u) {
			t.Fatalf("random round trip\n got %+v\nwant %+v", got, u)
		}
	}
	// A count the message cannot hold fails instead of allocating.
	e := newMsg(msgSetAccessibility)
	e.u64(1)
	e.u64(1)
	e.u32(1 << 21)
	b := e.bytes()
	d := &dec{b: b[5:]}
	d.accessUpdate()
	if d.end() == nil {
		t.Error("a corrupt node count decoded")
	}
}

// accessUpdate is a small tree: a root with buttons named names, IDs from
// 2.
func accessUpdate(names ...string) platform.AccessUpdate {
	u := platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{{ID: 1, Kind: platform.AccessRoot}}}
	for i, n := range names {
		id := platform.AccessID(i + 2)
		u.Nodes[0].Children = append(u.Nodes[0].Children, id)
		u.Nodes = append(u.Nodes, platform.AccessNode{ID: id, Role: "button", Name: n})
	}
	return u
}

// TestAccessibilityForwarded: the host's bridge activation reaches the
// active child, its updates reach the real window with the child's
// serial in their IDs, actions come back with the child's IDs; a reload
// is activated at its promotion and replaces the tree, and the outgoing
// build's actions and updates are dropped.
func TestAccessibilityForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	wa := a.newWindow(platform.WindowOptions{Title: "a11y"})
	a.configured()
	hw := p.window(0)
	ca := p.child(0)

	// Updates before the activation are dropped by the host.
	a.ui(func() { wa.SetAccessibility(accessUpdate("Early")) })
	p.roundTrip(a, ca)
	if hw.AccessIgnored() != 0 || len(hw.AccessUpdates()) != 0 {
		t.Errorf("an update before the activation reached the window: %d ignored, %d recorded", hw.AccessIgnored(), len(hw.AccessUpdates()))
	}

	hw.Inject(platform.AccessibilityEvent{Active: true})
	if got := a.waitEvent("the activation", isType[platform.AccessibilityEvent]); got != (platform.AccessibilityEvent{Active: true}) {
		t.Fatalf("got %+v", got)
	}
	// An action before the child sent its tree is dropped.
	hw.Inject(platform.AccessActionEvent{Node: 1<<accessShift | 2, Action: platform.AccessActionDefault})
	a.ui(func() { wa.SetAccessibility(accessUpdate("Save", "Cancel")) })
	waitFor(t, "the tree on the real window", func() bool { return hw.AccessTree() != nil })
	tree := hw.AccessTree()
	const sa = platform.AccessID(1) << accessShift // child a's serial is 1
	if tree.Root() != sa|1 || tree.Len() != 3 {
		t.Fatalf("real window's tree: root %#x, %d nodes", tree.Root(), tree.Len())
	}
	if n, ok := tree.Node(sa | 3); !ok || n.Name != "Cancel" {
		t.Errorf("node %#x: %+v", sa|3, n)
	}
	for _, e := range p.settled(a) {
		if isType[platform.AccessActionEvent](e.e) {
			t.Errorf("an action reached a child that had not sent its tree: %+v", e.e)
		}
	}
	hw.Inject(platform.AccessActionEvent{Node: sa | 3, Action: platform.AccessActionSetValue, Value: "x"})
	if got := a.waitEvent("the action", isType[platform.AccessActionEvent]); got != (platform.AccessActionEvent{Node: 3, Action: platform.AccessActionSetValue, Value: "x"}) {
		t.Errorf("action %+v", got)
	}

	// A reload: pending, it is not told; at its promotion it is, and its
	// tree replaces the old one.
	p.ui(p.host.Freeze)
	b := p.dial("b").start()
	wb := b.newWindow(platform.WindowOptions{Title: "a11y"})
	cb := p.child(1)
	b.configured()
	hw.Inject(platform.AccessActionEvent{Node: sa | 2, Action: platform.AccessActionDefault}) // frozen: dropped
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	b.presentNow(wb, 1, nil)
	waitChan(t, cb.FirstFrame(), "B's first frame")
	for _, e := range p.settled(b) {
		if isType[platform.AccessibilityEvent](e.e) {
			t.Errorf("a pending child was told about the bridge: %+v", e.e)
		}
	}
	p.promote(cb)
	if got := b.waitEvent("B's activation", isType[platform.AccessibilityEvent]); got != (platform.AccessibilityEvent{Active: true}) {
		t.Fatalf("got %+v", got)
	}
	a.ui(func() { wa.SetAccessibility(accessUpdate("Stale")) }) // detached: dropped
	b.ui(func() { wb.SetAccessibility(accessUpdate("Reloaded")) })
	const sb = platform.AccessID(2) << accessShift
	waitFor(t, "B's tree", func() bool { return hw.AccessTree().Root() == sb|1 })
	if tree := hw.AccessTree(); tree.Len() != 2 {
		t.Errorf("after the reload's update: %v", tree)
	} else if n, _ := tree.Node(sb | 2); n.Name != "Reloaded" {
		t.Errorf("B's button %+v", n)
	}
	hw.Inject(platform.AccessActionEvent{Node: sa | 2, Action: platform.AccessActionDefault}) // the old build's node
	hw.Inject(platform.AccessActionEvent{Node: sb | 2, Action: platform.AccessActionFocus})
	if got := b.waitEvent("B's action", isType[platform.AccessActionEvent]); got != (platform.AccessActionEvent{Node: 2, Action: platform.AccessActionFocus}) {
		t.Errorf("B's action %+v (the old build's should have been dropped)", got)
	}
	for _, e := range a.take() {
		if isType[platform.AccessActionEvent](e.e) {
			t.Errorf("the outgoing build got an action: %+v", e.e)
		}
	}

	hw.Inject(platform.AccessibilityEvent{Active: false})
	if got := b.waitEvent("B's deactivation", isType[platform.AccessibilityEvent]); got != (platform.AccessibilityEvent{Active: false}) {
		t.Errorf("got %+v", got)
	}
	b.ui(func() { wb.SetAccessibility(accessUpdate("After")) })
	p.roundTrip(b, cb)
	if hw.AccessIgnored() != 0 {
		t.Errorf("%d updates reached an inactive bridge", hw.AccessIgnored())
	}
}
