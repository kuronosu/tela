package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1280, 800

// mount opens the screen; dev mode is on, so any dev message fails the test.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.MountFunc(t, Kanban, opts...)
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {700, 420}, {1600, 900}} {
		mount(t, s[0], s[1])
	}
}

func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale)).Golden(fmt.Sprintf("board_x%g.png", scale))
		})
	}
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

// laneLoc locates the i-th column.
func laneLoc(i int) telatest.Locator {
	return telatest.ByClass("min-w-0", "flex-1", "flex-col", "rounded-2xl").Nth(i)
}

// cardLoc locates the card with this title.
func cardLoc(title string) telatest.Locator {
	return telatest.ByClass("rounded-xl", "select-none").Filter(telatest.HasText(title))
}

// center returns the center of the element.
func center(a *telatest.App, l telatest.Locator) (x, y float32) {
	bxBox := a.Bounds(l)
	bx, by, w, h := bxBox.X, bxBox.Y, bxBox.Width, bxBox.Height
	return bx + w/2, by + h/2
}

// laneOf returns the index of the column that holds the card, or -1.
func laneOf(a *telatest.App, title string) int {
	cx, _ := center(a, cardLoc(title))
	for i := range 3 {
		xBox := a.Bounds(laneLoc(i))
		x, w := xBox.X, xBox.Width
		if cx >= x && cx < x+w {
			return i
		}
	}
	return -1
}

// order returns the titles of a column, top to bottom.
func order(a *telatest.App, lane int) []string {
	type entry struct {
		y     float32
		title string
	}
	var es []entry
	for _, id := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		title := tasks[id].Title
		if a.Count(cardLoc(title)) == 1 && laneOf(a, title) == lane {
			y := a.Bounds(cardLoc(title)).Y
			es = append(es, entry{y, title})
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
	a := mount(t, windowW, windowH)
	a.ExpectCount(telatest.ByClass("min-w-0", "flex-1", "flex-col", "rounded-2xl"), 3)
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
	a := mount(t, windowW, windowH)
	cx, cy := center(a, cardLoc("Design the onboarding flow"))
	dxBox := a.Bounds(laneLoc(1))
	dx, dy, dw, dh := dxBox.X, dxBox.Y, dxBox.Width, dxBox.Height
	a.DragBy(cardLoc("Design the onboarding flow"), dx+dw/2-cx, dy+dh-60-cy)

	if got := order(a, 0); !equal(got, []string{"Write the API reference", "Audit color contrast", "Migrate the settings page"}) {
		t.Errorf("To do is %v", got)
	}
	want := []string{"Implement dark mode", "Fix the login redirect bug", "Review the pricing copy", "Design the onboarding flow"}
	if got := order(a, 1); !equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	a.ExpectText("3")
	a.ExpectText("4")
	// The card is back in the flow: no lifted card, no insertion line.
	a.ExpectCount(telatest.ByClass("z-50"), 0)
}

// TestDropBetweenCards drops onto a card of another column: the dragged
// card goes before it.
func TestDropBetweenCards(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Drag(cardLoc("Set up the CI pipeline"), cardLoc("Fix the login redirect bug"))
	if got, want := order(a, 1), []string{"Implement dark mode", "Set up the CI pipeline", "Fix the login redirect bug", "Review the pricing copy"}; !equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	if got, want := order(a, 2), []string{"Ship the v0.4 changelog", "Define the color tokens"}; !equal(got, want) {
		t.Errorf("Done is %v, want %v", got, want)
	}
}

// TestReorderInsideColumn moves the first card to the end of its column.
func TestReorderInsideColumn(t *testing.T) {
	a := mount(t, windowW, windowH)
	_, cy := center(a, cardLoc("Design the onboarding flow"))
	lyBox := a.Bounds(laneLoc(0))
	ly, lh := lyBox.Y, lyBox.Height
	a.DragBy(cardLoc("Design the onboarding flow"), 0, ly+lh-40-cy)
	want := []string{"Write the API reference", "Audit color contrast", "Migrate the settings page", "Design the onboarding flow"}
	if got := order(a, 0); !equal(got, want) {
		t.Errorf("To do is %v, want %v", got, want)
	}
}

// TestDropOutsideReturnsTheCard releases above the board.
func TestDropOutsideReturnsTheCard(t *testing.T) {
	a := mount(t, windowW, windowH)
	_, cy := center(a, cardLoc("Audit color contrast"))
	a.DragBy(cardLoc("Audit color contrast"), 300, 20-cy)
	if got := order(a, 0); !equal(got, []string{"Design the onboarding flow", "Write the API reference", "Audit color contrast", "Migrate the settings page"}) {
		t.Errorf("To do is %v after dropping outside", got)
	}
	a.ExpectCount(telatest.ByClass("z-50"), 0)
}

// TestClickDoesNotMove checks a click, with no drag, changes nothing.
func TestClickDoesNotMove(t *testing.T) {
	a := mount(t, windowW, windowH)
	builds := a.Stats().Builds
	a.Click(cardLoc("Write the API reference"))
	if got := order(a, 0); len(got) != 4 || got[1] != "Write the API reference" {
		t.Errorf("To do is %v", got)
	}
	if got := a.Stats().Builds - builds; got != 0 {
		t.Errorf("a click built %d components", got)
	}
}

// TestCardFollowsThePointer is the point of the example: while the card is
// dragged inside one column, each pointer move only repaints it. It is
// lifted, drawn at the pointer, and nothing builds or lays out.
func TestCardFollowsThePointer(t *testing.T) {
	a := mount(t, windowW, windowH)
	card := cardLoc("Write the API reference")
	xBox := a.Bounds(card)
	x, y, w, h := xBox.X, xBox.Y, xBox.Width, xBox.Height
	cx, cy := x+w/2, y+h/2
	// A point inside the card's padding, near its top-left corner.
	px, py := int(x)+8, int(y)+8
	if got := a.Screenshot().RGBAAt(px, py); got.R < 250 {
		t.Fatalf("the card is not white at its corner: %v", got)
	}

	a.MouseMove(cx, cy)
	a.MouseDown()
	// Move past the slop to start the drag, then settle in the To do column.
	a.MouseMove(cx+10, cy+30)
	a.MouseMove(cx+20, cy+60)
	a.ExpectCount(telatest.ByClass("relative", "z-50", "rotate-2"), 1)
	a.ExpectCount(telatest.ByClass("h-[3px]", "bg-indigo-500"), 1)
	before := a.Stats()
	// Inside one card and one lane, so the drop target stays the same.
	for i := 1; i <= 10; i++ {
		a.MouseMove(cx+20+float32(i)*5, cy+60)
	}
	after := a.Stats()
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
	shot := a.Screenshot()
	if got := shot.RGBAAt(px, py); got.R > 248 {
		t.Errorf("the original corner is still white (%v): the card did not move", got)
	}
	if got := shot.RGBAAt(px+70, py+60); got.R < 250 || got.G < 250 {
		t.Errorf("the moved card's corner is not white: %v", got)
	}
	a.MouseUp()
}

// TestDropTargetFeedback checks the ring on the lane under the pointer.
func TestDropTargetFeedback(t *testing.T) {
	a := mount(t, windowW, windowH)
	cx, cy := center(a, cardLoc("Implement dark mode"))
	dxBox := a.Bounds(laneLoc(2))
	dx, dy, dw, dh := dxBox.X, dxBox.Y, dxBox.Width, dxBox.Height
	a.MouseMove(cx, cy)
	a.MouseDown()
	a.MouseMove(cx+20, cy+20)
	a.MouseMove(dx+dw/2, dy+dh-100)
	ring := telatest.ByClass("ring-indigo-400/70", "rounded-2xl")
	a.ExpectCount(ring, 1)
	if r := a.Bounds(ring); r.X != dx || r.Y != dy || r.Width != dw || r.Height != dh {
		t.Fatalf("the highlighted lane is (%v, %v, %v, %v) while hovering Done, want (%v, %v, %v, %v)", r.X, r.Y, r.Width, r.Height, dx, dy, dw, dh)
	}
	a.MouseUp()
	a.ExpectCount(telatest.ByClass("ring-indigo-400/70"), 0)
}
