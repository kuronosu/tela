package core

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
)

func TestHitNodeAndNodeClasses(t *testing.T) {
	root := div(Class("p-2 bg-white"),
		button(Class("rounded-lg  px-4 py-2"), Class("shadow-sm"), Text("Go")),
		div(Class("h-20 w-40"), Key("box")))
	tr := newTestTree(t, 300, 200, root, nil)
	tr.Frame()
	sem := tr.Semantics()

	var btn, box *semantics.Node
	sem.Walk(func(n *semantics.Node) bool {
		switch {
		case n.Role == "button":
			btn = n
		case n.Kind == semantics.KindElement && n.Role == "generic" && n.Bounds.Width() == 40*4 && n.Bounds.Height() == 20*4:
			box = n
		}
		return true
	})
	if btn == nil || box == nil {
		t.Fatalf("button %v, box %v in\n%s", btn, box, tr.AriaSnapshot())
	}

	// A point on the button's text hits the button; on the box, the box.
	c := btn.Bounds.Center()
	if got := tr.HitNode(c.X, c.Y); got != btn.ID {
		t.Errorf("HitNode(button center) = %d, want the button %d", got, btn.ID)
	}
	c = box.Bounds.Center()
	if got := tr.HitNode(c.X, c.Y); got != box.ID {
		t.Errorf("HitNode(box center) = %d, want the box %d", got, box.ID)
	}
	if got := tr.HitNode(-5, 3); got != 0 {
		t.Errorf("HitNode outside the window = %d, want 0", got)
	}

	if got, want := tr.NodeClasses(btn.ID), []string{"rounded-lg", "px-4", "py-2", "shadow-sm"}; !slices.Equal(got, want) {
		t.Errorf("NodeClasses(button) = %v, want %v", got, want)
	}
	if got := tr.NodeClasses(semantics.NodeID(1 << 40)); got != nil {
		t.Errorf("NodeClasses(unknown) = %v, want nil", got)
	}
}

func TestBreakClickSequence(t *testing.T) {
	var counts []int
	root := button(Class("size-20"), On("click", func(e MouseEvent) { counts = append(counts, e.ClickCount) }))
	tr := newTestTree(t, 100, 100, root, nil)
	tr.Frame()
	click := func() {
		for _, e := range []platform.PointerEvent{
			{Action: platform.PointerDown, Pos: platform.Point{X: 10, Y: 10}, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()},
			{Action: platform.PointerUp, Pos: platform.Point{X: 10, Y: 10}, Button: platform.ButtonPrimary},
		} {
			tr.HandlePointer(e)
		}
	}
	click()
	click()
	tr.BreakClickSequence()
	click()
	if want := []int{1, 2, 1}; !slices.Equal(counts, want) {
		t.Errorf("click counts = %v, want %v", counts, want)
	}
}
