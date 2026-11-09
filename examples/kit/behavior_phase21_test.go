package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/examples/kit/icons"
	"github.com/kuronosu/tela/telatest"
)

// The tests of the demos of phase 21: the password reveal in an Input's
// EndContent, an Alert's Action, a Field's ErrorContent, the animation
// longhands, a submenu's ContentClass and a LoadingTrigger around a custom
// trigger. Timers run on the frame clock, so those pages are mounted with
// NoSettle and the test moves the clock.

func TestPasswordRevealNamesFollowTheState(t *testing.T) {
	for _, c := range []struct{ tag, show, hide string }{
		{"en", "Show password", "Hide password"},
		{"es", "Mostrar contraseña", "Ocultar contraseña"},
		{"ar", "إظهار كلمة المرور", "إخفاء كلمة المرور"},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := mount(t, Canvas{Page: "Input", Locale: c.tag})
			a.Click(telatest.ByRole("button", c.show))
			a.ExpectCount(telatest.ByRole("button", c.show), 0)
			a.Click(telatest.ByRole("button", c.hide))
			a.ExpectRole("button", c.show)
		})
	}
}

// The field is the same input before and after: what was typed stays, and
// the button is in the field (its box is inside the input's).
func TestPasswordRevealKeepsTheTextAndSitsInsideTheField(t *testing.T) {
	a := mount(t, Canvas{Page: "Input"})
	field := telatest.ByRole("textbox", "Your password")
	a.Fill(field, "hunter2")
	btn := telatest.ByRole("button", "Show password")
	in, bb := a.Bounds(field), a.Bounds(btn)
	if bb.X < in.X || bb.X+bb.Width > in.X+in.Width || bb.Y < in.Y || bb.Y+bb.Height > in.Y+in.Height {
		t.Errorf("the reveal button %+v is not inside the field %+v", bb, in)
	}
	a.Click(btn)
	a.Expect(field, telatest.HasValue("hunter2"))
}

func TestAlertActionRetryAndDismiss(t *testing.T) {
	a := mount(t, Canvas{Page: "Alert"}, telatest.NoSettle())
	retry := telatest.ByRole("button", "Retry")
	a.Click(retry)
	if got := loading(a); got != 1 {
		t.Errorf("%d buttons are loading after Retry, want 1", got)
	}
	a.Advance(2100 * time.Millisecond)
	if got := loading(a); got != 0 {
		t.Errorf("Retry is still loading after two seconds")
	}
	a.Click(telatest.ByRole("button", "Dismiss"))
	a.ExpectCount(retry, 0)
	a.ExpectNoText("The upload failed")
	a.Click(telatest.ByRole("button", "Show the alert again"))
	a.ExpectRole("button", "Retry")
	a.ExpectText("The upload failed")
}

func TestAlertActionIsTranslated(t *testing.T) {
	a := mount(t, Canvas{Page: "Alert", Locale: "es"})
	a.ExpectRole("button", "Reintentar")
	a.Click(telatest.ByRole("button", "Descartar"))
	a.ExpectRole("button", "Mostrar la alerta de nuevo")
}

func TestFieldRichErrorLinkFillsTheName(t *testing.T) {
	a := mount(t, Canvas{Page: "Field"})
	name := telatest.ByRole("textbox", "Username").Nth(1) // the second is the rich error's
	a.Expect(name, telatest.HasValue("mei"))
	a.ExpectText("That username is taken.")
	a.Click(telatest.ByRole("link", "Use mei.tanaka"))
	a.Expect(name, telatest.HasValue("mei.tanaka"))
	a.ExpectCount(telatest.ByRole("link", "Use mei.tanaka"), 0)
	// Typing "mei" again brings the error back.
	a.Fill(name, "mei")
	a.ExpectRole("link", "Use mei.tanaka")
}

func TestSeparatorCaptionAlignDemo(t *testing.T) {
	a := mount(t, Canvas{Page: "Separator", Locale: "es"})
	a.ExpectText("Ayer")
	a.ExpectText("La semana pasada")
}

// The animation classes are on the icon while it plays and come off when it
// is over; nothing asks for frames after that.
func TestReplayPlaysThenRests(t *testing.T) {
	a := mount(t, Replay{Label: "Go", Icon: icons.Bell, For: 1600 * time.Millisecond,
		Classes: "animate-bounce repeat-3 direction-alternate fill-mode-both duration-500"}, telatest.NoSettle())
	cls := telatest.ByClass("repeat-3")
	a.ExpectCount(cls, 0)
	a.Click(telatest.ByRole("button", "Go"))
	a.ExpectCount(cls, 1)
	a.Advance(time.Second)
	a.ExpectCount(cls, 1)
	a.Advance(time.Second)
	a.ExpectCount(cls, 0)
	before := a.Stats()
	a.Advance(2 * time.Second)
	if after := a.Stats(); after.Frames != before.Frames {
		t.Errorf("the page asks for frames after the animation: %d more", after.Frames-before.Frames)
	}
}

func TestSkeletonReplaysStartWithAPress(t *testing.T) {
	a := mount(t, Canvas{Page: "Skeleton"}, telatest.NoSettle())
	cls := telatest.ByClass("repeat-3")
	a.ExpectCount(cls, 0)
	a.Click(telatest.ByRole("button", "Refresh"))
	a.ExpectCount(cls, 1)
	a.Advance(3 * time.Second)
	a.ExpectCount(cls, 0)
	a.Click(telatest.ByRole("button", "Ring the bell"))
	a.ExpectCount(cls, 1)
	a.Advance(3 * time.Second)
	a.ExpectCount(cls, 0)
}

func TestSubmenuContentClass(t *testing.T) {
	a := mount(t, Canvas{Page: "Menubar"})
	a.Click(telatest.ByRole("menuitem", "File").Nth(1)) // the second bar's
	wide := telatest.ByClass("min-w-64")
	a.ExpectCount(wide, 0)
	a.Click(telatest.ByRole("menuitem", "Export as"))
	a.ExpectRole("menuitem", "PDF document")
	a.ExpectCount(wide, 1)
	a.Click(telatest.ByRole("menuitem", "PNG image"))
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
}

func TestExportTriggerIsBusyWhileItWorks(t *testing.T) {
	a := mount(t, Canvas{Page: "DropdownMenu"}, telatest.NoSettle())
	export := telatest.ByRole("button", "Export")
	a.ExpectText("Choose a format.")
	a.Click(export)
	a.Click(telatest.ByRole("menuitem", "CSV"))
	a.ExpectText("Exporting CSV...")
	a.Advance(400 * time.Millisecond) // the menu's exit
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	// Busy: pressing it opens nothing, and it keeps the focus order.
	r := a.Bounds(export)
	a.ClickAt(r.X+r.Width/2, r.Y+r.Height/2) // a pointer press, as a user's
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.Focus(export)
	a.Press("Enter")
	a.ExpectCount(telatest.ByRole("menu", ""), 0)
	a.Advance(2100 * time.Millisecond)
	a.ExpectText("Choose a format.")
	a.Click(export)
	a.ExpectRole("menuitem", "PDF")
	a.Press("Escape")
	before := a.Stats()
	a.Advance(2 * time.Second)
	if after := a.Stats(); after.Builds != before.Builds {
		t.Errorf("the export menu rebuilds while idle")
	}
}

func TestExportTextIsTranslated(t *testing.T) {
	a := mount(t, Canvas{Page: "DropdownMenu", Locale: "es"}, telatest.NoSettle())
	a.Click(telatest.ByRole("button", "Exportar"))
	a.Click(telatest.ByRole("menuitem", "PDF"))
	a.ExpectText("Exportando PDF...")
}
