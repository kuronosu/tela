package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// The tests of the demos of phase 20's pieces. A page with a timer is
// mounted with NoSettle, so the test moves the frame clock itself.

// loading counts the buttons that are Loading: the recipe puts
// pointer-events-none on them (a Disabled one has the variant only); the
// icons inside every button have it too, and ByClass finds them (ADR-263).
func loading(a *telatest.App) int {
	return a.Count(telatest.ByRole("button", "").Filter(telatest.HasClass("pointer-events-none")))
}

func TestLoadingButtonsSaveForTwoSeconds(t *testing.T) {
	a := mount(t, Canvas{Page: "Button"}, telatest.NoSettle())
	for _, name := range []string{"Save changes", "Save", "Continue"} {
		btn := telatest.ByRole("button", name).Nth(0)
		if name == "Continue" {
			btn = telatest.ByRole("button", name).Nth(1) // the first is the Icons demo's
		}
		a.Click(btn)
		if got := loading(a); got != 1 {
			t.Errorf("after pressing %q, %d buttons are loading, want 1", name, got)
		}
		a.Advance(time.Second)
		if got := loading(a); got != 1 {
			t.Errorf("%q stopped loading after one second", name)
		}
		a.Advance(1200 * time.Millisecond)
		if got := loading(a); got != 0 {
			t.Errorf("%q is still loading after two seconds", name)
		}
	}
}

func TestSubmitButtonLoadsWhileTheFormSubmits(t *testing.T) {
	a := mount(t, Canvas{Page: "Button"}, telatest.NoSettle())
	a.Click(telatest.ByRole("button", "Subscribe"))
	if got := loading(a); got != 1 {
		t.Errorf("the submit button is not loading after the click (%d loading)", got)
	}
	a.Advance(2100 * time.Millisecond)
	if got := loading(a); got != 0 {
		t.Errorf("the submit button is still loading after two seconds")
	}
}

func TestDropdownSubmenusNest(t *testing.T) {
	a := mount(t, Canvas{Page: "DropdownMenu"})
	a.Click(telatest.ByRole("button", "Team"))
	a.Click(telatest.ByRole("menuitem", "Invite users"))
	a.ExpectRole("menuitem", "Email")
	a.Click(telatest.ByRole("menuitem", "More..."))
	a.ExpectRole("menuitem", "Slack")
	a.Click(telatest.ByRole("menuitem", "Slack"))
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.ExpectText("Chose: slack")
}

func TestContextClickSelectsTheOptionFirst(t *testing.T) {
	a := mount(t, Canvas{Page: "ContextMenu"})
	a.ExpectText("Nothing is selected.")
	a.Click(telatest.ByRole("option", "notes.md"), telatest.Right())
	a.ExpectRole("menu", "Files")
	a.ExpectText("Selected: notes.md")
	a.Press("Escape")
	// Another option, not selected: the selection moves to it.
	a.Click(telatest.ByRole("option", "photo.png"), telatest.Right())
	a.ExpectText("Selected: photo.png")
	a.Click(telatest.ByRole("menuitem", "Rename"))
	a.ExpectText("Rename: photo.png")
}

func TestContextMenuSubmenuAndFocusableArea(t *testing.T) {
	a := mount(t, Canvas{Page: "ContextMenu"})
	area := telatest.ByRole("region", "Page area")
	a.Focus(area)
	a.Press("Shift+F10")
	a.ExpectRole("menu", "Page")
	a.Click(telatest.ByRole("menuitem", "Share"))
	a.Click(telatest.ByRole("menuitem", "Messages"))
	a.ExpectText("Chose: messages")
}

func TestMenubarRoleItemsAndFind(t *testing.T) {
	a := mount(t, Canvas{Page: "Menubar"})
	a.Click(telatest.ByRole("menuitem", "Edit").Nth(1)) // the second bar's
	for _, item := range []string{"Undo", "Redo", "Cut", "Copy", "Paste", "Select All", "Find..."} {
		a.ExpectRole("menuitem", item)
	}
	a.Click(telatest.ByRole("menuitem", "Find..."))
	a.ExpectText("Find was chosen.")
}

func TestMenubarRoleItemsFollowTheLanguage(t *testing.T) {
	a := mount(t, Canvas{Page: "Menubar", Locale: "es"})
	a.Click(telatest.ByRole("menuitem", "Edición").Nth(1))
	for _, item := range []string{"Deshacer", "Cortar", "Copiar", "Pegar", "Seleccionar todo"} {
		a.ExpectRole("menuitem", item)
	}
}

func TestTooltipOnOtherTriggers(t *testing.T) {
	a := mount(t, Canvas{Page: "Tooltip"})
	for _, c := range []struct {
		trigger telatest.Locator
		tip     string
	}{
		{telatest.ByRole("button", "More options"), "More options"},
		{telatest.ByRole("button", "Bold"), "Bold, Ctrl+B"},
		{telatest.ByRole("checkbox", "Accept the terms"), "You accept the terms of service"},
		{telatest.ByRole("switch", "Airplane mode"), "Turns off every connection"},
	} {
		a.Hover(c.trigger)
		a.ExpectRole("tooltip", c.tip)
	}
	// A disabled trigger never opens its tooltip.
	a.MouseMove(1100, 700)
	a.Advance(time.Second)
	a.ExpectCount(telatest.ByRole("tooltip", ""), 0)
	b := a.Bounds(telatest.ByRole("button", "Disabled button"))
	a.MouseMove(b.X+b.Width/2, b.Y+b.Height/2)
	a.Advance(time.Second)
	a.ExpectCount(telatest.ByRole("tooltip", ""), 0)
	// The dropdown's button still opens its menu.
	a.Click(telatest.ByRole("button", "More options"))
	a.ExpectRole("menu", "")
}

func TestDownloadBarRunsOnlyAfterAPress(t *testing.T) {
	a := mount(t, Canvas{Page: "Progress"}, telatest.NoSettle())
	text := telatest.ByTestID("download-text")
	a.Advance(5 * time.Second)
	a.ExpectCount(text.Filter(telatest.HasText("0%")), 1)
	a.Click(telatest.ByRole("button", "Start the download"))
	a.Advance(1500 * time.Millisecond)
	a.ExpectCount(text.Filter(telatest.HasText("0%")), 0) // the bar and its number moved
	a.Advance(5 * time.Second)
	a.Expect(telatest.ByRole("progressbar", "Download"), telatest.HasValue("100%"))
	a.ExpectCount(text.Filter(telatest.HasText("100%")), 1)
	a.ExpectRole("button", "Download again")
}

func TestTweenBarEases(t *testing.T) {
	a := mount(t, Canvas{Page: "Progress"}, telatest.NoSettle())
	bar := telatest.ByRole("progressbar", "Disk cleanup")
	a.Click(telatest.ByRole("button", "+25%"))
	a.Advance(2 * time.Second)
	a.Expect(bar, telatest.HasValue("50%"))
	a.Click(telatest.ByRole("button", "Reset"))
	a.Advance(2 * time.Second)
	a.Expect(bar, telatest.HasValue("0%"))
}

func TestSlidingPillChoosesARange(t *testing.T) {
	a := mount(t, Canvas{Page: "ToggleGroup"})
	a.ExpectText("Showing the last week.")
	a.Click(telatest.ByRole("radio", "Month").Nth(0))
	a.ExpectText("Showing the last month.")
	a.Expect(telatest.ByRole("radio", "Month").Nth(0), telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "Week").Nth(0), telatest.Checked(false))
}

func TestSearchPutsTheTableInTheOtherState(t *testing.T) {
	a := mount(t, Canvas{Page: "Table"})
	rel := telatest.ByRole("columnheader", "Relevance")
	a.Expect(rel, telatest.Sort("other"))
	a.Click(telatest.ByRole("button", "Component"))
	a.Expect(telatest.ByRole("columnheader", "Component"), telatest.Sort("ascending"))
	a.Expect(rel, telatest.Sort("none"))
	a.Fill(telatest.ByRole("searchbox", "Search components"), "")
	a.ExpectText("Avatar")
}

func TestAlwaysSortedNeverGoesBackToUnsorted(t *testing.T) {
	a := mount(t, Canvas{Page: "Table"})
	name := telatest.ByRole("button", "Name").Nth(0)
	head := telatest.ByRole("columnheader", "Name").Nth(0)
	a.Expect(head, telatest.Sort("ascending"))
	a.Click(name)
	a.Expect(head, telatest.Sort("descending"))
	a.Click(name)
	a.Expect(head, telatest.Sort("ascending"))
}

// The message box is drawn by Tela and follows the app's locale: its buttons
// and title come out in the window's language, and the previous locale is
// back once it is answered.
func TestMessageBoxIsTranslated(t *testing.T) {
	prev := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(prev) })
	for _, c := range []struct{ tag, show, answer, readout string }{
		{"en", "Show a message box", "Yes", `You chose "Yes".`},
		{"es", "Mostrar un cuadro de mensaje", "Sí", "Elegiste «Sí»."},
		{"ar", "إظهار مربع رسالة", "نعم", "اخترت «نعم»."},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := mount(t, Canvas{Page: "Internationalization", Locale: c.tag}, telatest.WithUnsupported(telatest.MessageBoxes)) // drawn, as on Linux
			a.Click(telatest.ByRole("button", c.show))
			boxes := 0
			for _, w := range a.Windows() {
				boxes += w.Count(telatest.ByRole("button", c.answer))
			}
			if boxes != 1 {
				t.Errorf("%d message box buttons named %q, want 1", boxes, c.answer)
			}
			a.AnswerMessage(desktop.AnswerYes)
			a.ExpectText(c.readout)
			if got := i18n.Locale(); got != prev {
				t.Errorf("the locale is %q after the box, want %q back", got, prev)
			}
		})
	}
}

func TestToasterNamesFollowTheLanguage(t *testing.T) {
	for _, c := range []struct{ tag, region, trigger, close string }{
		{"es", "Notificaciones", "Éxito", "Cerrar notificación"},
		{"ar", "الإشعارات", "نجاح", "إغلاق الإشعار"},
	} {
		a := mount(t, Canvas{Page: "Toast", Locale: c.tag})
		a.ExpectRole("region", c.region)
		a.Click(telatest.ByRole("button", c.trigger))
		a.ExpectRole("button", c.close)
	}
}

func TestDigitsOfALocale(t *testing.T) {
	a := mount(t, Canvas{Page: "Internationalization"})
	a.ExpectText("١٬٢٣٤٫٥٠") // ar-EG; bare ar writes Latin digits
	a.ExpectText("1,234.50 · 14")
}
