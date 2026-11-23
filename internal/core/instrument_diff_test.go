package core

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// ElementsSince gives the whole list first, then only what changed: a
// state change sends the text it changed (and the component's build
// count), a row mounted sends the row and the new order; an unknown
// version gets the whole list again.
func TestElementsSince(t *testing.T) {
	var set Setter[int]
	rows := func(n int) []Node {
		var out []Node
		for i := range n {
			out = append(out, Primitive(style.KindDiv, Key(i), Textf("row %d", i)))
		}
		return out
	}
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			n, s := UseState(2)
			set = s
			return Primitive(style.KindDiv, Key("list"), Textf("%d rows", n), List(rows(n)))
		})
	})
	full := tr.ElementsSince(0)
	if !full.Full || full.Version == 0 || len(full.Elements) != len(tr.Elements(false)) {
		t.Fatalf("first: full %v, version %d, %d elements", full.Full, full.Version, len(full.Elements))
	}
	if same := tr.ElementsSince(full.Version); same.Full || same.Version != full.Version || len(same.Elements) != 0 || same.Order != nil {
		t.Errorf("unchanged: %+v, want no change at the same version", same)
	}

	set.Set(3)
	tr.Frame()
	d := tr.ElementsSince(full.Version)
	if d.Full || d.Version == full.Version || d.Order == nil {
		t.Fatalf("a row more: full %v, version %d (was %d), order %v", d.Full, d.Version, full.Version, d.Order)
	}
	var texts []string
	for _, e := range d.Elements {
		if e.Kind == "text" {
			texts = append(texts, e.Text)
		}
	}
	slices.Sort(texts)
	if want := []string{"3 rows", "row 2"}; !slices.Equal(texts, want) {
		t.Errorf("texts sent %v, want %v (the others did not change)", texts, want)
	}
	for _, e := range d.Elements {
		if e.Key == "0" || e.Key == "1" {
			t.Errorf("row %s did not change and was sent: %+v", e.Key, e)
		}
	}
	// Applied to the first list, the change gives the list.
	if got := applyDiff(full.Elements, d); !slices.EqualFunc(got, tr.Elements(false), func(a, b ElementInfo) bool { return sameElement(&a, &b) }) {
		t.Error("the first list with the change applied is not the list")
	}
	if again := tr.ElementsSince(12345); !again.Full {
		t.Error("an unknown version did not get the whole list")
	}
	// The JSON a client reads.
	b, _ := json.Marshal(d)
	var back ElementsDiff
	if err := json.Unmarshal(b, &back); err != nil || back.Version != d.Version || len(back.Order) != len(d.Order) {
		t.Errorf("round trip: %v, %+v", err, back)
	}
}

// applyDiff applies d to old as a client does.
func applyDiff(old []ElementInfo, d ElementsDiff) []ElementInfo {
	if d.Full {
		return d.Elements
	}
	byID := map[ElementID]ElementInfo{}
	for _, e := range old {
		byID[e.ID] = e
	}
	for _, e := range d.Elements {
		byID[e.ID] = e
	}
	if d.Order == nil {
		out := slices.Clone(old)
		for i := range out {
			out[i] = byID[out[i].ID]
		}
		return out
	}
	out := make([]ElementInfo, len(d.Order))
	for i, id := range d.Order {
		out[i] = byID[id]
	}
	return out
}

// InspectDepth stops at the depth asked for and counts what it leaves out.
func TestInspectDepth(t *testing.T) {
	tr := newInsTree(t, func() Node {
		return Primitive(style.KindDiv, Key("outer"), Class("flex"),
			Primitive(style.KindDiv, Key("a"), Primitive(style.KindDiv), Primitive(style.KindDiv)),
			Primitive(style.KindDiv, Key("b")),
		)
	})
	whole := tr.Inspect()
	n, err := tr.InspectDepth(0, 0)
	if err != nil || len(n.Children) != 0 || n.More != len(whole.Children) || n.More == 0 || n.Kind != whole.Kind {
		t.Fatalf("the root alone: %+v, %v; want %d children counted", n, err, len(whole.Children))
	}
	// The count travels in the JSON the tools socket sends.
	b, _ := json.Marshal(n)
	var back InspectNode
	if err := json.Unmarshal(b, &back); err != nil || back.More != n.More {
		t.Errorf("JSON round trip: more %d, %v\n%s", back.More, err, b)
	}
	outer := idOfKey(t, tr, "outer")
	n, err = tr.InspectDepth(outer, 1)
	if err != nil || len(n.Children) != 2 || n.More != 0 {
		t.Fatalf("outer at depth 1: %d children, more %d, %v", len(n.Children), n.More, err)
	}
	if a := n.Children[0]; len(a.Children) != 0 || a.More != 2 {
		t.Errorf("a at the last level: %d children, more %d; want 2 counted", len(a.Children), a.More)
	}
	if b := n.Children[1]; b.More != 0 {
		t.Errorf("b has no children, more %d", b.More)
	}
	all, err := tr.InspectDepth(outer, -1)
	if err != nil || len(all.Children[0].Children) != 2 {
		t.Errorf("outer whole: %+v, %v", all, err)
	}
}
