package main

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The board view's tests, from examples/board (ADR-195): the model, the
// drag between columns, between cards and inside a column, dropping
// outside, the drop-target ring, a click that moves nothing, the card that
// follows the pointer without builds or layouts, the menus (by pointer and
// keyboard, a full column disabled, the context menu from the keyboard)
// and the edit and delete dialogs.

// mountBoard opens the board view.
func mountBoard(t *testing.T, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mountPreview(t, "2 Board", opts...)
}

// TestMove checks the model on its own.
func TestMove(t *testing.T) {
	b := initialBoard
	check := func(name string, got Board, want [3][]int) {
		t.Helper()
		for ci := range want {
			if ids := got[ci].IDs(); !slices.Equal(ids, want[ci]) {
				t.Errorf("%s: column %d is %v, want %v", name, ci, ids, want[ci])
			}
		}
	}
	check("initial", b, [3][]int{{1, 2, 3, 4}, {8, 6, 9, 5}, {10, 11, 12, 13}})
	check("to the end of another column", b.Move(1, Target{Col: 1, Valid: true}), [3][]int{{2, 3, 4}, {8, 6, 9, 5, 1}, {10, 11, 12, 13}})
	check("before a card", b.Move(1, Target{Col: 1, Before: 6, Valid: true}), [3][]int{{2, 3, 4}, {8, 1, 6, 9, 5}, {10, 11, 12, 13}})
	check("reorder to the end", b.Move(1, Target{Col: 0, Valid: true}), [3][]int{{2, 3, 4, 1}, {8, 6, 9, 5}, {10, 11, 12, 13}})
	check("reorder before another", b.Move(4, Target{Col: 0, Before: 2, Valid: true}), [3][]int{{1, 4, 2, 3}, {8, 6, 9, 5}, {10, 11, 12, 13}})
	check("before itself", b.Move(2, Target{Col: 0, Before: 2, Valid: true}), [3][]int{{1, 2, 3, 4}, {8, 6, 9, 5}, {10, 11, 12, 13}})
	check("nowhere", b.Move(2, Target{}), [3][]int{{1, 2, 3, 4}, {8, 6, 9, 5}, {10, 11, 12, 13}})
	check("remove", b.Remove(9), [3][]int{{1, 2, 3, 4}, {8, 6, 5}, {10, 11, 12, 13}})
	check("add", b.Add(20, 2), [3][]int{{1, 2, 3, 4}, {8, 6, 9, 5}, {10, 11, 12, 13, 20}})
	var full Board
	for i := range full[1] {
		full[1][i] = 14 + i
	}
	full[0][0] = 1
	if got := full.Move(1, Target{Col: 1, Valid: true}); got != full {
		t.Error("a full column accepted another card")
	}
	if got := initialTasks.nextKey(); got != "TEL-137" {
		t.Errorf("the next key is %s, want TEL-137", got)
	}
}

// laneLoc locates the i-th column.
func laneLoc(i int) telatest.Locator { return telatest.ByTestID("lane").Nth(i) }

// cardLoc locates the card with this title.
func cardLoc(title string) telatest.Locator {
	return telatest.ByTestID("card").Filter(telatest.HasText(title))
}

// centerOf returns the center of the element.
func centerOf(a *telatest.App, l telatest.Locator) (x, y float32) {
	r := a.Bounds(l)
	return r.X + r.Width/2, r.Y + r.Height/2
}

// laneOf returns the index of the column that holds the card, or -1.
func laneOf(a *telatest.App, title string) int {
	cx, _ := centerOf(a, cardLoc(title))
	for i := range 3 {
		r := a.Bounds(laneLoc(i))
		if cx >= r.X && cx < r.X+r.Width {
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
	for _, task := range titles() {
		if a.Count(cardLoc(task)) == 1 && laneOf(a, task) == lane {
			es = append(es, entry{a.Bounds(cardLoc(task)).Y, task})
		}
	}
	slices.SortFunc(es, func(a, b entry) int {
		switch {
		case a.y < b.y:
			return -1
		case a.y > b.y:
			return 1
		}
		return 0
	})
	var out []string
	for _, e := range es {
		out = append(out, e.title)
	}
	return out
}

// titles are the titles a card can have in these tests.
func titles() []string {
	out := []string{"Ship the CSV and JSON export", "Draft the Q4 plan"}
	for _, t := range initialTasks {
		if t.Title != "" {
			out = append(out, t.Title)
		}
	}
	return out
}

func TestStartLayout(t *testing.T) {
	a := mountBoard(t)
	a.ExpectCount(telatest.ByTestID("lane"), 3)
	want := [][]string{
		{"Fix failed invoice retries", "Ship the CSV export", "Review the onboarding emails", "Interview three churned customers"},
		{"Audit color contrast", "Dark mode for the invoice PDF", "Migrate settings to forms", "Publish the API changelog"},
		{"Set up the staging pipeline", "Write the Q3 revenue summary", "Tidy the pricing page copy", "Webhook signature checks"},
	}
	for i, w := range want {
		if got := order(a, i); !slices.Equal(got, w) {
			t.Errorf("lane %d is %v, want %v", i, got, w)
		}
	}
}

// TestDragToAnotherColumn drags the first card of To do onto the empty end
// of In progress.
func TestDragToAnotherColumn(t *testing.T) {
	a := mountBoard(t)
	cx, cy := centerOf(a, cardLoc("Fix failed invoice retries"))
	d := a.Bounds(laneLoc(1))
	a.DragBy(cardLoc("Fix failed invoice retries"), d.X+d.Width/2-cx, d.Y+d.Height-40-cy)

	if got := order(a, 0); !slices.Equal(got, []string{"Ship the CSV export", "Review the onboarding emails", "Interview three churned customers"}) {
		t.Errorf("To do is %v", got)
	}
	want := []string{"Audit color contrast", "Dark mode for the invoice PDF", "Migrate settings to forms", "Publish the API changelog", "Fix failed invoice retries"}
	if got := order(a, 1); !slices.Equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	// The card is back in the flow: no lifted card, no insertion line.
	a.ExpectCount(telatest.ByClass("z-50", "rotate-2"), 0)
	a.ExpectCount(telatest.ByClass("h-[3px]", "bg-primary"), 0)
}

// TestDropBetweenCards drops onto a card of another column: the dragged
// card goes before it.
func TestDropBetweenCards(t *testing.T) {
	a := mountBoard(t)
	a.Drag(cardLoc("Set up the staging pipeline"), cardLoc("Dark mode for the invoice PDF"))
	want := []string{"Audit color contrast", "Set up the staging pipeline", "Dark mode for the invoice PDF", "Migrate settings to forms", "Publish the API changelog"}
	if got := order(a, 1); !slices.Equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	if got := order(a, 2); !slices.Equal(got, []string{"Write the Q3 revenue summary", "Tidy the pricing page copy", "Webhook signature checks"}) {
		t.Errorf("Done is %v", got)
	}
	a.ExpectText("3 of 12 done") // the sprint follows the board
}

// TestReorderInsideColumn moves the first card to the end of its column.
func TestReorderInsideColumn(t *testing.T) {
	a := mountBoard(t)
	_, cy := centerOf(a, cardLoc("Fix failed invoice retries"))
	l := a.Bounds(laneLoc(0))
	a.DragBy(cardLoc("Fix failed invoice retries"), 0, l.Y+l.Height-40-cy)
	want := []string{"Ship the CSV export", "Review the onboarding emails", "Interview three churned customers", "Fix failed invoice retries"}
	if got := order(a, 0); !slices.Equal(got, want) {
		t.Errorf("To do is %v, want %v", got, want)
	}
}

// TestDropOutsideReturnsTheCard releases above the board.
func TestDropOutsideReturnsTheCard(t *testing.T) {
	a := mountBoard(t)
	_, cy := centerOf(a, cardLoc("Review the onboarding emails"))
	a.DragBy(cardLoc("Review the onboarding emails"), 300, 30-cy)
	want := []string{"Fix failed invoice retries", "Ship the CSV export", "Review the onboarding emails", "Interview three churned customers"}
	if got := order(a, 0); !slices.Equal(got, want) {
		t.Errorf("To do is %v after dropping outside", got)
	}
	a.ExpectCount(telatest.ByClass("z-50", "rotate-2"), 0)
}

// TestClickDoesNotMove checks that a click, with no drag, changes nothing.
func TestClickDoesNotMove(t *testing.T) {
	a := mountBoard(t)
	builds := a.Stats().Builds
	a.Click(cardLoc("Ship the CSV export"))
	if got := order(a, 0); len(got) != 4 || got[1] != "Ship the CSV export" {
		t.Errorf("To do is %v", got)
	}
	if got := a.Stats().Builds - builds; got != 0 {
		t.Errorf("a click built %d components", got)
	}
}

// TestCardFollowsThePointer: while the card is dragged inside one column,
// each pointer move only repaints it. It is lifted, drawn at the pointer,
// and nothing builds or lays out.
func TestCardFollowsThePointer(t *testing.T) {
	a := mountBoard(t)
	card := cardLoc("Ship the CSV export")
	r := a.Bounds(card)
	cx, cy := r.X+r.Width/2, r.Y+r.Height/2
	// A point on the card's title, near its top-left corner.
	px, py := int(r.X)+20, int(r.Y)+50
	shot0 := a.Screenshot()

	a.MouseMove(cx, cy)
	a.MouseDown()
	// Move past the slop to start the drag, then stay in the To do column.
	a.MouseMove(cx+10, cy+30)
	a.MouseMove(cx+20, cy+60)
	a.ExpectCount(telatest.ByClass("z-50", "rotate-2"), 1)
	a.ExpectCount(telatest.ByClass("h-[3px]", "bg-primary"), 1)
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
	// The card left its place: the pixels there changed, and so did the
	// pixels 70 px right and 60 px down, where it is drawn now.
	shot := a.Screenshot()
	if shot.RGBAAt(px, py) == shot0.RGBAAt(px, py) {
		t.Errorf("the card's title is still drawn at (%d, %d): the card did not move", px, py)
	}
	if shot.RGBAAt(px+70, py+60) == shot0.RGBAAt(px+70, py+60) {
		t.Errorf("nothing changed at the card's new place (%d, %d)", px+70, py+60)
	}
	a.MouseUp()
}

// TestDropTargetFeedback checks the ring on the column under the pointer.
func TestDropTargetFeedback(t *testing.T) {
	a := mountBoard(t)
	cx, cy := centerOf(a, cardLoc("Audit color contrast"))
	d := a.Bounds(laneLoc(2))
	a.MouseMove(cx, cy)
	a.MouseDown()
	a.MouseMove(cx+20, cy+20)
	a.MouseMove(d.X+d.Width/2, d.Y+d.Height-40)
	ring := telatest.ByClass("ring-primary/50", "rounded-2xl")
	a.ExpectCount(ring, 1)
	if r := a.Bounds(ring); r != d {
		t.Fatalf("the highlighted column is %+v while hovering Done, want %+v", r, d)
	}
	a.MouseUp()
	a.ExpectCount(telatest.ByClass("ring-primary/50"), 0)
}

// actionsOf is the ellipsis button of the card with this title.
func actionsOf(title string) telatest.Locator { return telatest.ByRole("button", "Actions for "+title) }

func menuItem(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

// moveVia moves a card with its dropdown menu.
func moveVia(a *telatest.App, title, column string) {
	a.Click(actionsOf(title))
	a.Click(menuItem(column))
}

func TestMoveFromTheMenu(t *testing.T) {
	a := mountBoard(t)
	a.Click(actionsOf("Fix failed invoice retries"))
	a.ExpectRole("menu", "Actions for Fix failed invoice retries")
	a.ExpectText("Move to")
	a.ExpectCount(menuItem("To do"), 0) // the card's own column is not offered
	a.Click(menuItem("In progress"))
	want := []string{"Audit color contrast", "Dark mode for the invoice PDF", "Migrate settings to forms", "Publish the API changelog", "Fix failed invoice retries"}
	if got := order(a, 1); !slices.Equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	a.ExpectText("Moved to In progress")
	a.ExpectCount(menuItem("Done"), 0) // the menu closed
}

func TestMoveFromTheKeyboard(t *testing.T) {
	a := mountBoard(t)
	a.Focus(actionsOf("Ship the CSV export"))
	a.Press("Enter")
	a.ExpectCount(menuItem("Done"), 1)
	// Enter opens the menu on its first item (In progress).
	a.Press("ArrowDown") // Done
	a.Press("Enter")
	want := []string{"Set up the staging pipeline", "Write the Q3 revenue summary", "Tidy the pricing page copy", "Webhook signature checks", "Ship the CSV export"}
	if got := order(a, 2); !slices.Equal(got, want) {
		t.Errorf("Done is %v, want %v", got, want)
	}
}

// TestFullColumnIsDisabled fills In progress and checks that the menu does
// not offer it any more.
func TestFullColumnIsDisabled(t *testing.T) {
	a := mountBoard(t)
	for _, title := range []string{"Fix failed invoice retries", "Ship the CSV export", "Review the onboarding emails", "Interview three churned customers"} {
		moveVia(a, title, "In progress")
	}
	if got := len(order(a, 1)); got != maxCards {
		t.Fatalf("In progress has %d cards, want %d", got, maxCards)
	}
	a.ExpectText("Drop cards here") // To do is empty
	a.Expect(telatest.ByRole("button", "New task in In progress"), telatest.Disabled(true))
	a.Click(actionsOf("Set up the staging pipeline"))
	a.Expect(menuItem("In progress"), telatest.Disabled(true))
	a.Expect(menuItem("To do"), telatest.Disabled(false))
	a.Press("Escape")
}

// TestContextMenuOpensFromTheKeyboard: the card is a ContextMenu area, so
// Shift+F10 from the card's button opens the same menu.
func TestContextMenuOpensFromTheKeyboard(t *testing.T) {
	a := mountBoard(t)
	a.Focus(actionsOf("Review the onboarding emails"))
	a.Press("Shift+F10")
	a.ExpectCount(menuItem("In progress"), 1)
	a.ExpectCount(menuItem("Edit task"), 1)
	a.ExpectCount(menuItem("Delete task"), 1)
	a.Press("Escape")
	a.ExpectCount(menuItem("Edit task"), 0)
}

func TestEditTask(t *testing.T) {
	a := mountBoard(t)
	a.Click(actionsOf("Ship the CSV export"))
	a.Click(menuItem("Edit task"))
	a.ExpectRole("dialog", "Edit task")
	a.ExpectValue(telatest.ByRole("textbox", "Title"), "Ship the CSV export")

	a.Fill(telatest.ByRole("textbox", "Title"), "Ship the CSV and JSON export")
	a.Click(telatest.ByRole("button", "Product Team"))
	a.Click(telatest.ByRole("option", "Platform"))
	a.Click(telatest.ByRole("button", "Save changes"))

	a.ExpectCount(telatest.ByRole("dialog", "Edit task"), 0)
	a.ExpectText("Task updated")
	if a.Count(cardLoc("Ship the CSV and JSON export")) != 1 || a.Count(cardLoc("Ship the CSV export")) != 0 {
		t.Fatalf("the card was not renamed")
	}
	if got := laneOf(a, "Ship the CSV and JSON export"); got != 0 {
		t.Errorf("the edited card is in lane %d, want 0", got)
	}
	a.Click(telatest.ByRole("radio", "Table"))
	a.ExpectText("Ship the CSV and JSON export") // the table shows the same tasks
}

func TestEditNeedsATitle(t *testing.T) {
	a := mountBoard(t)
	a.Click(actionsOf("Ship the CSV export"))
	a.Click(menuItem("Edit task"))
	a.Fill(telatest.ByRole("textbox", "Title"), "")
	a.ExpectText("Give the task a title.")
	a.Expect(telatest.ByRole("button", "Save changes"), telatest.Disabled(true))

	a.Click(telatest.ByRole("button", "Cancel"))
	a.ExpectNoText("Give the task a title.")
	if a.Count(cardLoc("Ship the CSV export")) != 1 {
		t.Error("Cancel changed the card")
	}
	// Opening it again starts from the card, not from the abandoned draft.
	a.Click(actionsOf("Ship the CSV export"))
	a.Click(menuItem("Edit task"))
	a.ExpectValue(telatest.ByRole("textbox", "Title"), "Ship the CSV export")
}

func TestDeleteAsksFirst(t *testing.T) {
	a := mountBoard(t)
	a.Click(actionsOf("Review the onboarding emails"))
	a.Click(menuItem("Delete task"))
	a.ExpectRole("alertdialog", "Delete this task?")
	a.ExpectText(`"Review the onboarding emails" will be removed from the board.`)

	a.Click(telatest.ByRole("button", "Cancel"))
	if got := order(a, 0); len(got) != 4 {
		t.Fatalf("Cancel deleted the card: %v", got)
	}

	a.Click(actionsOf("Review the onboarding emails"))
	a.Click(menuItem("Delete task"))
	a.Click(telatest.ByRole("button", "Delete task"))
	want := []string{"Fix failed invoice retries", "Ship the CSV export", "Interview three churned customers"}
	if got := order(a, 0); !slices.Equal(got, want) {
		t.Errorf("To do is %v, want %v", got, want)
	}
	a.ExpectText("Task deleted")
	a.ExpectText("11 tasks")
}

// TestDeleteEverythingLeavesTheDropHint deletes the cards of a column; a
// card can still be dragged into it.
func TestDeleteEverythingLeavesTheDropHint(t *testing.T) {
	a := mountBoard(t)
	for _, title := range []string{"Audit color contrast", "Dark mode for the invoice PDF", "Migrate settings to forms", "Publish the API changelog"} {
		a.Click(actionsOf(title))
		a.Click(menuItem("Delete task"))
		a.Click(telatest.ByRole("button", "Delete task"))
	}
	a.ExpectText("Drop cards here")
	cx, cy := centerOf(a, cardLoc("Set up the staging pipeline"))
	d := a.Bounds(laneLoc(1))
	a.DragBy(cardLoc("Set up the staging pipeline"), d.X+d.Width/2-cx, d.Y+80-cy)
	if got := order(a, 1); !slices.Equal(got, []string{"Set up the staging pipeline"}) {
		t.Errorf("In progress is %v", got)
	}
}

// TestMenuButtonDoesNotDrag: a click on the ellipsis is a click, not the
// start of a drag.
func TestMenuButtonDoesNotDrag(t *testing.T) {
	a := mountBoard(t)
	a.Click(actionsOf("Ship the CSV export"))
	a.ExpectCount(telatest.ByClass("z-50", "rotate-2"), 0)
	a.Press("Escape")
	if got := order(a, 0); len(got) != 4 || got[1] != "Ship the CSV export" {
		t.Errorf("To do is %v", got)
	}
}

// TestGoldenBoardMenu is a card's menu open, in the dark scheme.
func TestGoldenBoardMenu(t *testing.T) {
	a := mountBoard(t, telatest.Dark())
	a.Click(actionsOf("Fix failed invoice retries"))
	a.ExpectText("Edit task")
	a.Golden("board_menu_dark.png")
}
