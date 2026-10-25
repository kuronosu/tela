package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// actionsOf is the ellipsis button of the card with this title.
func actionsOf(title string) telatest.Locator {
	return telatest.ByRole("button", "Actions for "+title)
}

func menuItem(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

// moveVia moves a card with its dropdown menu.
func moveVia(a *telatest.App, title, column string) {
	a.Click(actionsOf(title))
	a.Click(menuItem(column))
}

func TestMoveFromTheMenu(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(actionsOf("Design the onboarding flow"))
	a.ExpectRole("menu", "Actions for Design the onboarding flow")
	a.ExpectText("Move to")
	// The card's own column is not offered.
	a.ExpectCount(menuItem("To do"), 0)
	a.Click(menuItem("In progress"))

	want := []string{"Implement dark mode", "Fix the login redirect bug", "Review the pricing copy", "Design the onboarding flow"}
	if got := order(a, 1); !equal(got, want) {
		t.Errorf("In progress is %v, want %v", got, want)
	}
	a.ExpectText("Moved to In progress")
	a.ExpectCount(menuItem("Done"), 0) // the menu closed
}

func TestMoveFromTheKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Focus(actionsOf("Write the API reference"))
	a.Press("Enter")
	a.ExpectCount(menuItem("Done"), 1)
	// Enter opens the menu on its first item (In progress).
	a.Press("ArrowDown") // Done
	a.Press("Enter")
	if got, want := order(a, 2), []string{"Set up the CI pipeline", "Ship the v0.4 changelog", "Define the color tokens", "Write the API reference"}; !equal(got, want) {
		t.Errorf("Done is %v, want %v", got, want)
	}
}

// TestFullColumnIsDisabled fills In progress and checks the menu will not
// offer it any more.
func TestFullColumnIsDisabled(t *testing.T) {
	a := mount(t, windowW, windowH)
	for _, title := range []string{"Design the onboarding flow", "Write the API reference", "Audit color contrast", "Migrate the settings page", "Set up the CI pipeline"} {
		moveVia(a, title, "In progress")
	}
	if got := len(order(a, 1)); got != maxCards {
		t.Fatalf("In progress has %d cards, want %d", got, maxCards)
	}
	a.Click(actionsOf("Ship the v0.4 changelog"))
	a.Expect(menuItem("In progress"), telatest.Disabled(true))
	a.Expect(menuItem("To do"), telatest.Disabled(false))
	a.Press("Escape")
}

// TestContextMenuOpensFromTheKeyboard: the card is a ContextMenu area, so
// Shift+F10 from the card's button opens the same menu at the card's corner.
func TestContextMenuOpensFromTheKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Focus(actionsOf("Audit color contrast"))
	a.Press("Shift+F10")
	a.ExpectCount(menuItem("In progress"), 1)
	a.ExpectCount(menuItem("Edit card"), 1)
	a.ExpectCount(menuItem("Delete card"), 1)
	a.Press("Escape")
	a.ExpectCount(menuItem("Edit card"), 0)
}

func TestEditCard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(actionsOf("Write the API reference"))
	a.Click(menuItem("Edit card"))
	a.ExpectText("Edit card")
	a.ExpectValue(telatest.ByRole("textbox", "Title"), "Write the API reference")

	a.Fill(telatest.ByRole("textbox", "Title"), "Write the API guide")
	a.Click(telatest.ByRole("button", "Docs Tag"))
	a.Click(telatest.ByRole("option", "DevOps"))
	a.Click(telatest.ByRole("button", "Save changes"))

	a.ExpectNoText("Edit card")
	a.ExpectText("Card updated")
	if a.Count(cardLoc("Write the API guide")) != 1 || a.Count(cardLoc("Write the API reference")) != 0 {
		t.Fatalf("the card was not renamed")
	}
	a.ExpectText("DevOps")
	if got := laneOf(a, "Write the API guide"); got != 0 {
		t.Errorf("the edited card is in lane %d, want 0", got)
	}
}

func TestEditNeedsATitle(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(actionsOf("Write the API reference"))
	a.Click(menuItem("Edit card"))
	a.Fill(telatest.ByRole("textbox", "Title"), "")
	a.ExpectText("Give the card a title.")
	a.Expect(telatest.ByRole("button", "Save changes"), telatest.Disabled(true))

	a.Click(telatest.ByRole("button", "Cancel"))
	a.ExpectNoText("Give the card a title.")
	if a.Count(cardLoc("Write the API reference")) != 1 {
		t.Error("Cancel changed the card")
	}
	// Opening it again starts from the card, not from the abandoned draft.
	a.Click(actionsOf("Write the API reference"))
	a.Click(menuItem("Edit card"))
	a.ExpectValue(telatest.ByRole("textbox", "Title"), "Write the API reference")
}

func TestDeleteAsksFirst(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(actionsOf("Audit color contrast"))
	a.Click(menuItem("Delete card"))
	a.ExpectRole("alertdialog", "Delete this card?")
	a.ExpectText(`"Audit color contrast" will be removed from the board.`)

	a.Click(telatest.ByRole("button", "Cancel"))
	if got := order(a, 0); len(got) != 4 {
		t.Fatalf("Cancel deleted the card: %v", got)
	}

	a.Click(actionsOf("Audit color contrast"))
	a.Click(menuItem("Delete card"))
	a.Click(telatest.ByRole("button", "Delete card"))
	want := []string{"Design the onboarding flow", "Write the API reference", "Migrate the settings page"}
	if got := order(a, 0); !equal(got, want) {
		t.Errorf("To do is %v, want %v", got, want)
	}
	a.ExpectText("Card deleted")
}

// TestDeleteEverythingLeavesTheDropHint deletes the three cards of a
// column.
func TestDeleteEverythingLeavesTheDropHint(t *testing.T) {
	a := mount(t, windowW, windowH)
	for _, title := range []string{"Implement dark mode", "Fix the login redirect bug", "Review the pricing copy"} {
		a.Click(actionsOf(title))
		a.Click(menuItem("Delete card"))
		a.Click(telatest.ByRole("button", "Delete card"))
	}
	a.ExpectText("Drop cards here")
	// A card can still be dragged into the empty column.
	cx, cy := center(a, cardLoc("Set up the CI pipeline"))
	dxBox := a.Bounds(laneLoc(1))
	a.DragBy(cardLoc("Set up the CI pipeline"), dxBox.X+dxBox.Width/2-cx, dxBox.Y+60-cy)
	if got := order(a, 1); !equal(got, []string{"Set up the CI pipeline"}) {
		t.Errorf("In progress is %v", got)
	}
}

// TestMenuButtonDoesNotDrag: a click on the ellipsis is a click, not the
// start of a drag.
func TestMenuButtonDoesNotDrag(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(actionsOf("Write the API reference"))
	a.ExpectCount(telatest.ByClass("z-50", "rotate-2"), 0)
	a.Press("Escape")
	if got := order(a, 0); len(got) != 4 || got[1] != "Write the API reference" {
		t.Errorf("To do is %v", got)
	}
}

// TestGoldenStates are the board in dark mode, with a card's menu open, the
// edit dialog, the delete question and a toast, at scales 1 and 2.
func TestGoldenStates(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale), telatest.Dark()).Golden(fmt.Sprintf("board_dark_x%g.png", scale))

			a := mount(t, windowW, windowH, telatest.Scale(scale))
			a.Click(actionsOf("Design the onboarding flow"))
			a.ExpectText("Edit card")
			a.Golden(fmt.Sprintf("board_menu_x%g.png", scale))

			a.Click(menuItem("Edit card"))
			a.ExpectText("Change what the card says.")
			a.Golden(fmt.Sprintf("board_edit_x%g.png", scale))

			a.Click(telatest.ByRole("button", "Cancel"))
			a.Click(actionsOf("Audit color contrast"))
			a.Click(menuItem("Delete card"))
			a.ExpectText("Delete this card?")
			a.Golden(fmt.Sprintf("board_delete_x%g.png", scale))

			a.Click(telatest.ByRole("button", "Delete card"))
			a.MouseMove(5, 5)
			a.ExpectText("Card deleted")
			a.Golden(fmt.Sprintf("board_deleted_x%g.png", scale))
		})
	}
}

// TestAriaSnapshot pins what `tela inspect --aria --size 1280x800
// ./examples/board` prints (testdata/board.aria.yml): the columns, the
// cards with their titles and the menu button of each.
func TestAriaSnapshot(t *testing.T) {
	mount(t, windowW, windowH).AriaGolden("board.aria.yml")
}
