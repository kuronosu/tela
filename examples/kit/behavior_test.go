package main

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The tests of this file drive the demos with the pointer and check what
// they show: a gallery whose demos do not work is a poor demonstration.

// TestStateResetsBetweenPages: a page is built only while it is selected, so
// coming back to it starts afresh (the Tabs content is not kept alive).
func TestStateResetsBetweenPages(t *testing.T) {
	a := mount(t, Gallery{Page: "Button"})
	counter := telatest.ByRole("button", "Clicked 0 times")
	a.Click(counter)
	a.Click(telatest.ByRole("button", "Clicked 1 times"))
	a.ExpectText("Clicked 2 times")
	a.Click(tabFor("Badge"))
	a.Click(tabFor("Button"))
	a.ExpectText("Clicked 0 times")
}

// TestStylePickerChangesEveryPage: the Rounded style reaches the components
// of the page and the header itself, and Default takes it back.
func TestStylePickerChangesEveryPage(t *testing.T) {
	a := mount(t, Gallery{Page: "Button"})
	a.ExpectCount(telatest.ByClass("bg-violet-600"), 0)
	a.Click(rounded)
	a.Expect(rounded, telatest.Checked(true))
	if a.Count(telatest.ByClass("bg-violet-600")) == 0 {
		t.Error("no violet button after choosing Rounded")
	}
	a.Click(tabFor("Switch"))
	a.Click(telatest.ByRole("switch", "Default"))
	a.Expect(telatest.ByRole("switch", "Default"), telatest.Checked(true))
	a.Click(telatest.ByRole("radio", "Default"))
	a.ExpectCount(telatest.ByClass("bg-violet-600"), 0)
}

// TestDarkSwitchWithPointer flips the scheme with a click and back.
func TestDarkSwitchWithPointer(t *testing.T) {
	a := mount(t, Gallery{Page: "Card"})
	a.Click(darkSwitch)
	a.Expect(darkSwitch, telatest.Checked(true))
	if px := a.PixelAt(1000, 770); px.R > 40 {
		t.Errorf("not dark: %v", px)
	}
	a.Click(darkSwitch)
	if px := a.PixelAt(1000, 770); px.R < 200 {
		t.Errorf("not light: %v", px)
	}
}

func TestCheckboxSelectAll(t *testing.T) {
	a := mount(t, Canvas{Page: "Checkbox"})
	a.ExpectText("3 of 6 selected")
	a.Click(telatest.ByRole("checkbox", "Select all"))
	a.ExpectText("6 of 6 selected")
	a.Expect(telatest.ByRole("checkbox", "Select all"), telatest.Checked(true))
	a.Click(telatest.ByRole("checkbox", "Desktop"))
	a.ExpectText("5 of 6 selected")
	a.Expect(telatest.ByRole("checkbox", "Select all"), telatest.Checked(false))
}

func TestFieldErrorFollowsTheValue(t *testing.T) {
	a := mount(t, Canvas{Page: "Field"})
	email := telatest.ByRole("textbox", "Email")
	a.Expect(email, telatest.Invalid(true))
	a.ExpectText("Enter a valid email address.")
	a.Fill(email, "mei@example.com")
	a.Expect(email, telatest.Invalid(false))
	a.ExpectNoText("Enter a valid email address.")
}

func TestToastsAppear(t *testing.T) {
	a := mount(t, Canvas{Page: "Toast"})
	a.Click(telatest.ByRole("button", "Success"))
	a.ExpectText("Changes saved")
	a.Click(telatest.ByRole("button", "Delete the draft"))
	a.ExpectText("Draft deleted")
	a.Click(telatest.ByRole("button", "Undo"))
	a.ExpectText("Draft restored")
}

func TestEditProfileSavesAndCloses(t *testing.T) {
	a := mount(t, Canvas{Page: "Dialog"})
	edit := telatest.ByRole("button", "Edit profile")
	a.Click(edit)
	a.ExpectRole("dialog", "Edit profile")
	a.Fill(telatest.ByRole("textbox", "Name"), "Mei T.")
	a.Click(telatest.ByRole("button", "Save changes"))
	a.ExpectCount(telatest.ByRole("dialog", "Edit profile"), 0)
	a.ExpectText("Profile saved")
	a.ExpectFocused(edit)
}

func TestAlertDialogDecisions(t *testing.T) {
	a := mount(t, Canvas{Page: "AlertDialog"})
	trigger := telatest.ByRole("button", "Delete project")
	a.Click(trigger)
	a.ExpectRole("alertdialog", "Are you absolutely sure?")
	a.ExpectFocused(telatest.ByRole("button", "Cancel"))
	a.Click(telatest.ByRole("button", "Delete"))
	a.ExpectText("Deleted the project.")
	a.ExpectFocused(trigger)
}

func TestDatePickerChoosesADate(t *testing.T) {
	a := mount(t, Canvas{Page: "DatePicker"})
	a.Click(telatest.ByRole("button", "Date"))
	a.Click(telatest.ByRole("gridcell", "Friday, October 30, 2026"))
	a.ExpectText("Chosen: Friday, October 30, 2026")
	a.ExpectCount(telatest.ByRole("grid", "October 2026"), 0)
	a.ExpectRole("button", "Date, October 30, 2026")
}

func TestRangeCalendarTwoPresses(t *testing.T) {
	a := mount(t, Canvas{Page: "RangeCalendar"})
	a.ExpectText("From Oct 27, 2026 to Nov 3, 2026")
	a.Click(telatest.ByRole("button", "Clear"))
	a.ExpectText("No dates chosen.")
	a.Click(telatest.ByRole("gridcell", "Monday, October 26, 2026").Nth(0))
	a.Click(telatest.ByRole("gridcell", "Friday, October 30, 2026").Nth(0))
	a.ExpectText("From Oct 26, 2026 to Oct 30, 2026")
}

func TestTableSorts(t *testing.T) {
	a := mount(t, Canvas{Page: "Table"})
	first := func() string {
		snap := a.AriaSnapshot()
		_, rest, _ := strings.Cut(snap, "INV0")
		return "INV0" + rest[:2]
	}
	// Sorted by amount, descending, to start with: INV005 is the biggest.
	if got := first(); got != "INV005" {
		t.Errorf("first row = %s, want INV005", got)
	}
	a.Click(telatest.ByRole("button", "Invoice"))
	if got := first(); got != "INV001" {
		t.Errorf("after sorting by invoice, the first row is %s, want INV001", got)
	}
}

func TestSlidersMoveWithKeys(t *testing.T) {
	a := mount(t, Canvas{Page: "Slider"})
	volume := telatest.ByRole("slider", "Volume")
	a.Focus(volume)
	a.Press("ArrowRight")
	a.ExpectText("Volume: 41%")
	a.Press("End")
	a.ExpectText("Volume: 100%")
}

func TestRadioGroupPlans(t *testing.T) {
	a := mount(t, Canvas{Page: "RadioGroup"})
	a.ExpectText("Plan: pro")
	a.Click(telatest.ByRole("radio", "Free"))
	a.ExpectText("Plan: free")
}

func TestColorPickerHexField(t *testing.T) {
	a := mount(t, Canvas{Page: "ColorPicker"})
	hex := telatest.ByRole("textbox", "Hex").Nth(0)
	a.Fill(hex, "#ff0000")
	a.Press("Enter")
	a.ExpectText("#ff0000")
}

func TestTooltipOnHover(t *testing.T) {
	a := mount(t, Canvas{Page: "Tooltip"})
	a.ExpectCount(telatest.ByRole("tooltip", ""), 0)
	a.Hover(telatest.ByRole("button", "Download"))
	a.ExpectRole("tooltip", "Download")
	a.Hover(telatest.ByRole("button", "Share"))
	a.ExpectRole("tooltip", "Share a link")
}
