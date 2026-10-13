package main

import (
	"testing"

	"github.com/kuronosu/tela/examples/internal/exampletest"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

const windowW, windowH = 1280, 800

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		a := exampletest.Open(t, Kanban, s[0], s[1])
		a.CheckLogs()
	}
}

func TestGolden(t *testing.T) {
	exampletest.Golden(t, "board", Kanban, windowW, windowH)
}

// TestMove checks the model on its own.
func TestMove(t *testing.T) {
	b := Initial
	check := func(name string, got Board, want [3][]int) {
		t.Helper()
		for ci := range want {
			ids := got[ci].IDs()
			if len(ids) != len(want[ci]) {
				t.Errorf("%s: column %d is %v, want %v", name, ci, ids, want[ci])
				continue
			}
			for i := range ids {
				if ids[i] != want[ci][i] {
					t.Errorf("%s: column %d is %v, want %v", name, ci, ids, want[ci])
				}
			}
		}
	}
	check("initial", b, [3][]int{{1, 2, 3, 9}, {4, 5, 10}, {6, 7, 8}})
	check("to the end of another column", b.Move(1, Target{Col: 1, Valid: true}), [3][]int{{2, 3, 9}, {4, 5, 10, 1}, {6, 7, 8}})
	check("before a card", b.Move(1, Target{Col: 1, Before: 5, Valid: true}), [3][]int{{2, 3, 9}, {4, 1, 5, 10}, {6, 7, 8}})
	check("reorder to the end", b.Move(1, Target{Col: 0, Valid: true}), [3][]int{{2, 3, 9, 1}, {4, 5, 10}, {6, 7, 8}})
	check("reorder before another", b.Move(9, Target{Col: 0, Before: 2, Valid: true}), [3][]int{{1, 9, 2, 3}, {4, 5, 10}, {6, 7, 8}})
	check("before itself", b.Move(2, Target{Col: 0, Before: 2, Valid: true}), [3][]int{{1, 2, 3, 9}, {4, 5, 10}, {6, 7, 8}})
	check("nowhere", b.Move(2, Target{}), [3][]int{{1, 2, 3, 9}, {4, 5, 10}, {6, 7, 8}})
	var full Board
	for i := range full[1] {
		full[1][i] = 20 + i
	}
	full[0][0] = 1
	if got := full.Move(1, Target{Col: 1, Valid: true}); got != full {
		t.Error("a full column accepted another card")
	}
}

// lanes returns the three columns.
func lanes(a *exampletest.App) []core.InspectNode {
	ls := a.WithClass("min-w-0", "flex-1", "flex-col", "rounded-2xl")
	if len(ls) != 3 {
		a.T.Fatalf("%d lanes, want 3", len(ls))
	}
	return ls
}

// card returns the box of the card with this title.
func card(a *exampletest.App, title string) core.InspectNode {
	a.T.Helper()
	cs := a.All(func(n core.InspectNode) bool {
		return exampletest.HasClass(n, "rounded-xl") && exampletest.HasClass(n, "select-none") && exampletest.Contains(n, title)
	})
	if len(cs) != 1 {
		a.T.Fatalf("%d cards titled %q", len(cs), title)
	}
	return cs[0]
}

// laneOf returns the index of the column that holds the card.
func laneOf(a *exampletest.App, title string) int {
	c := card(a, title).Rect.Center()
	for i, l := range lanes(a) {
		if c.X >= l.Rect.X0 && c.X < l.Rect.X1 {
			return i
		}
	}
	a.T.Fatalf("card %q is outside every lane", title)
	return -1
}

// order returns the titles of a column, top to bottom.
func order(a *exampletest.App, lane int) []string {
	type entry struct {
		y     float32
		title string
	}
	var es []entry
	for _, id := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		title := tasks[id].Title
		cs := a.All(func(n core.InspectNode) bool {
			return exampletest.HasClass(n, "select-none") && exampletest.Contains(n, title)
		})
		if len(cs) == 1 && laneOf(a, title) == lane {
			es = append(es, entry{cs[0].Rect.Y0, title})
		}
	}
	for i := range es {
		for j := i + 1; j < len(es); j++ {
			if es[j].y < es[i].y {
				es[i], es[j] = es[j], es[i]
			}
		}
	}
	var out []string
	for _, e := range es {
		out = append(out, e.title)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStartLayout(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	want := [][]string{
		{"Design the onboarding flow", "Write the API reference", "Audit color contrast", "Migrate the settings page"},
		{"Implement dark mode", "Fix the login redirect bug", "Review the pricing copy"},
		{"Set up the CI pipeline", "Ship the v0.4 changelog", "Define the color tokens"},
	}
	for i, w := range want {
		if got := order(a, i); !equal(got, w) {
			t.Errorf("lane %d is %v, want %v", i, got, w)
		}
	}
}

// TestDragToAnotherColumn drags the first card of To do onto the empty
// part of In progress.
func TestDragToAnotherColumn(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	c := card(a, "Design the onboarding flow").Rect.Center()
	dst := lanes(a)[1].Rect
	a.Drag(c, gfx.Point{X: dst.Center().X, Y: dst.Y1 - 60}, 12)

	if got := order(a, 0); !equal(got, []string{"Write the API reference", "Audit color contrast", "Migrate the settings page"}) {
		t.Errorf("To do is %v", got)
	}
	want := []string{"Implement dark mode", "Fix the login redirect bug", "Review the pricing copy", "Design the onboarding flow"}
	if got := order(a, 1); !equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	if !a.HasText("3") || !a.HasText("4") {
		t.Error("the column counters did not update")
	}
	// The card is back in the flow: no lifted card, no insertion line.
	if n := a.WithClass("z-50"); len(n) != 0 {
		t.Errorf("%d lifted cards after the drop", len(n))
	}
	a.CheckLogs()
}

// TestDropBetweenCards drops onto a card of another column: the dragged
// card goes before it.
func TestDropBetweenCards(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	src := card(a, "Set up the CI pipeline").Rect.Center()
	dst := card(a, "Fix the login redirect bug").Rect.Center()
	a.Drag(src, dst, 12)
	if got, want := order(a, 1), []string{"Implement dark mode", "Set up the CI pipeline", "Fix the login redirect bug", "Review the pricing copy"}; !equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	if got, want := order(a, 2), []string{"Ship the v0.4 changelog", "Define the color tokens"}; !equal(got, want) {
		t.Errorf("Done is %v, want %v", got, want)
	}
}

// TestReorderInsideColumn moves the first card to the end of its column.
func TestReorderInsideColumn(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	src := card(a, "Design the onboarding flow").Rect.Center()
	l := lanes(a)[0].Rect
	a.Drag(src, gfx.Point{X: src.X, Y: l.Y1 - 40}, 12)
	want := []string{"Write the API reference", "Audit color contrast", "Migrate the settings page", "Design the onboarding flow"}
	if got := order(a, 0); !equal(got, want) {
		t.Errorf("To do is %v, want %v", got, want)
	}
}

// TestDropOutsideReturnsTheCard releases above the board.
func TestDropOutsideReturnsTheCard(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	src := card(a, "Audit color contrast").Rect.Center()
	a.Drag(src, gfx.Point{X: src.X + 300, Y: 20}, 12)
	if got := order(a, 0); !equal(got, []string{"Design the onboarding flow", "Write the API reference", "Audit color contrast", "Migrate the settings page"}) {
		t.Errorf("To do is %v after dropping outside", got)
	}
	if n := a.WithClass("z-50"); len(n) != 0 {
		t.Error("the card is still lifted")
	}
}

// TestClickDoesNotMove checks a click, with no drag, changes nothing.
func TestClickDoesNotMove(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	c := card(a, "Write the API reference").Rect.Center()
	builds := a.Tree().Stats().Builds
	a.Click(c.X, c.Y)
	if got := order(a, 0); len(got) != 4 || got[1] != "Write the API reference" {
		t.Errorf("To do is %v", got)
	}
	if got := a.Tree().Stats().Builds - builds; got != 0 {
		t.Errorf("a click built %d components", got)
	}
}

// TestCardFollowsThePointer is the point of the example: while the card is
// dragged inside one column, each pointer move only repaints it. It is
// lifted, drawn at the pointer, and nothing builds or lays out.
func TestCardFollowsThePointer(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	r := card(a, "Write the API reference").Rect
	c := r.Center()
	// A point inside the card's padding, near its top-left corner.
	px, py := int(r.X0)+8, int(r.Y0)+8
	if got := a.Image().RGBAAt(px, py); got.R < 250 {
		t.Fatalf("the card is not white at its corner: %v", got)
	}

	a.Move(c.X, c.Y)
	a.Down()
	// Move past the slop to start the drag, then settle in the To do column.
	a.Move(c.X+10, c.Y+30)
	a.Move(c.X+20, c.Y+60)
	if n := a.WithClass("relative", "z-50", "rotate-2"); len(n) != 1 {
		t.Fatalf("%d lifted cards while dragging, want 1", len(n))
	}
	if n := a.WithClass("h-[3px]", "bg-indigo-500"); len(n) != 1 {
		t.Errorf("%d insertion lines, want 1", len(n))
	}
	before := a.Tree().Stats()
	// Inside one card and one lane, so the drop target stays the same.
	for i := 1; i <= 10; i++ {
		a.Move(c.X+20+float32(i)*5, c.Y+60)
	}
	after := a.Tree().Stats()
	if after.Frames-before.Frames < 10 {
		t.Errorf("only %d frames for 10 moves", after.Frames-before.Frames)
	}
	if got := after.Builds - before.Builds; got != 0 {
		t.Errorf("%d component builds during pointer moves, want 0", got)
	}
	if got := after.Layouts - before.Layouts; got != 0 {
		t.Errorf("%d layouts during pointer moves, want 0: the card only translates", got)
	}
	if got := after.Paints - before.Paints; got < 10 {
		t.Errorf("%d paints for 10 moves", got)
	}
	// The card left its corner behind: the lane color shows there, and the
	// card's corner is now 70 px right and 60 px down, where the pointer is.
	if got := a.Image().RGBAAt(px, py); got.R > 248 {
		t.Errorf("the original corner is still white (%v): the card did not move", got)
	}
	if got := a.Image().RGBAAt(px+70, py+60); got.R < 250 || got.G < 250 {
		t.Errorf("the moved card's corner is not white: %v", got)
	}
	a.Up()
	a.CheckLogs()
}

// TestDropTargetFeedback checks the ring on the lane under the pointer.
func TestDropTargetFeedback(t *testing.T) {
	a := exampletest.Open(t, Kanban, windowW, windowH)
	c := card(a, "Implement dark mode").Rect.Center()
	dst := lanes(a)[2].Rect
	a.Move(c.X, c.Y)
	a.Down()
	a.Move(c.X+20, c.Y+20)
	a.Move(dst.Center().X, dst.Y1-100)
	ring := a.All(func(n core.InspectNode) bool {
		return exampletest.HasClass(n, "ring-indigo-400/70") && exampletest.HasClass(n, "rounded-2xl")
	})
	if len(ring) != 1 || ring[0].Rect != dst {
		t.Fatalf("highlighted lanes while hovering Done: %v", ring)
	}
	a.Up()
	if ring := a.WithClass("ring-indigo-400/70"); len(ring) != 0 {
		t.Error("a lane is still highlighted after the drop")
	}
}
