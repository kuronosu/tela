package headless

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// TestAccessBridge: the fake bridge takes updates only between delivered
// activations and deactivations, applies them, and refuses impossible
// injections and updates.
func TestAccessBridge(t *testing.T) {
	b := New(Options{})
	pw, err := b.NewWindow(platform.WindowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	var got []platform.Event
	h := platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.AccessibilityEvent, platform.AccessActionEvent:
			got = append(got, e)
		}
	})
	b.Drain(h)
	full := platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2}},
		{ID: 2, Role: "button", Name: "OK"},
	}}
	w.SetAccessibility(full)
	if w.AccessIgnored() != 1 || len(w.AccessUpdates()) != 0 || w.AccessTree() != nil {
		t.Errorf("an update before the activation: %d ignored, %d recorded", w.AccessIgnored(), len(w.AccessUpdates()))
	}
	expectPanic(t, "no assistive technology is active", func() {
		w.Inject(platform.AccessActionEvent{Node: 2, Action: platform.AccessActionDefault})
	})
	expectPanic(t, "alternate", func() { w.Inject(platform.AccessibilityEvent{Active: false}) })

	w.Inject(platform.AccessibilityEvent{Active: true})
	if w.Accessible() {
		t.Error("active before the event was delivered")
	}
	b.Drain(h)
	if !w.Accessible() {
		t.Fatal("not active after the event")
	}
	expectPanic(t, "neither in the update nor in the tree", func() {
		w.SetAccessibility(platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{full.Nodes[0]}})
	})
	w.SetAccessibility(full)
	w.SetAccessibility(platform.AccessUpdate{Root: 1, Focus: 2, Nodes: []platform.AccessNode{{ID: 2, Role: "button", Name: "OK", States: platform.AccessFocused}}})
	if us := w.AccessUpdates(); len(us) != 2 || w.AccessTree().Focus() != 2 || w.AccessTree().Len() != 2 {
		t.Errorf("%d updates, tree %v focus %d", len(us), w.AccessTree(), w.AccessTree().Focus())
	}
	w.Inject(platform.AccessActionEvent{Node: 2, Action: platform.AccessActionDefault})
	expectPanic(t, "unknown action", func() { w.Inject(platform.AccessActionEvent{Node: 2}) })
	w.Inject(platform.AccessibilityEvent{Active: false})
	b.Drain(h)
	if w.Accessible() || w.AccessTree() != nil {
		t.Error("the bridge kept its tree after the deactivation")
	}
	w.SetAccessibility(full)
	if w.AccessIgnored() != 2 || len(w.AccessUpdates()) != 2 {
		t.Errorf("after the deactivation: %d ignored, %d recorded", w.AccessIgnored(), len(w.AccessUpdates()))
	}
	want := []platform.Event{platform.AccessibilityEvent{Active: true},
		platform.AccessActionEvent{Node: 2, Action: platform.AccessActionDefault}, platform.AccessibilityEvent{Active: false}}
	if len(got) != len(want) {
		t.Fatalf("delivered %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}
