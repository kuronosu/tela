package aria_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// modalDialogPage is a trigger that opens a modal dialog with a title and
// two buttons, next to a button that the dialog makes inert.
func modalDialogPage(alert bool, ev *events) func() tela.Node {
	return func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{Type: "dialog"})
		ov := aria.UseOverlay(aria.OverlayOptions{Open: trig.Open, OnClose: trig.Close, Dismissable: !alert, Modal: true})
		role := "dialog"
		if alert {
			role = "alertdialog"
		}
		dlg := aria.UseDialog(aria.DialogOptions{Role: role, Modal: true, Ref: trig.OverlayRef})
		return Div(Class("p-4"),
			Button(trig.TriggerProps, TestID("trigger"), Class("h-8 w-24"), Text("Open")),
			Button(TestID("other"), OnClick(func() { ev.add("other") }), Class("ml-2 h-8 w-24"), Text("Other")),
			Portal(If(trig.Open,
				Div(ov.Props, dlg.Props, TestID("dialog"), Class("absolute left-[100px] top-[100px] h-40 w-64 bg-white p-4"),
					H2(dlg.TitleProps, Text("Delete project?")),
					Button(TestID("ok"), Class("h-8 w-16"), Text("OK")),
					Button(TestID("cancel"), Class("h-8 w-16"), Text("Cancel"))))))
	}
}

func TestDialogModal(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, modalDialogPage(false, &ev), telatest.Size(400, 300))
	trigger := telatest.ByTestID("trigger")
	app.Expect(trigger, telatest.Expanded(false))
	app.Click(trigger)

	// The dialog is named by its title, and the rest of the window is
	// inert: out of the snapshot, not pressed.
	app.ExpectAria(`
		- dialog "Delete project?":
		  - heading "Delete project?" [level=2]
		  - button "OK"
		  - button "Cancel"
	`)
	// The focus moved in (the first tabbable) and Tab stays inside.
	app.ExpectFocused(telatest.ByTestID("ok"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("cancel"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("ok"))
	app.Press("Shift+Tab")
	app.ExpectFocused(telatest.ByTestID("cancel"))

	// Escape closes it and the focus returns to the trigger.
	app.Press("Escape")
	app.ExpectAria(`
		- button "Open"
		- button "Other"
	`)
	app.ExpectFocused(trigger)
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

// A press outside a dismissable modal dialog closes it and does not click
// what is under the press.
func TestDialogOutsidePress(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, modalDialogPage(false, &ev), telatest.Size(400, 300))
	other := app.Bounds(telatest.ByTestID("other"))
	app.Click(telatest.ByTestID("trigger"))
	app.ClickAt(other.X+5, other.Y+5)
	app.ExpectNoText("Delete project?")
	if got := ev.take(); got != "" {
		t.Errorf("the dismissing press clicked what was under it: %q", got)
	}
	// With the dialog gone the button works again.
	app.Click(telatest.ByTestID("other"))
	if got := ev.take(); got != "other" {
		t.Errorf("events %q", got)
	}
}

// An alert dialog is not dismissed by a press outside it, but Escape closes
// it, and the page below stays inert.
func TestAlertDialogNotDismissedByOutsidePress(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, modalDialogPage(true, &ev), telatest.Size(400, 300))
	other := app.Bounds(telatest.ByTestID("other"))
	app.Click(telatest.ByTestID("trigger"))
	app.ExpectAria(`
		- alertdialog "Delete project?":
		  - heading "Delete project?" [level=2]
		  - button "OK"
		  - button "Cancel"
	`)
	app.ClickAt(other.X+5, other.Y+5)
	app.ClickAt(350, 250)
	app.ExpectText("Delete project?")
	if got := ev.take(); got != "" {
		t.Errorf("the inert page was clicked: %q", got)
	}
	app.Press("Escape")
	app.ExpectNoText("Delete project?")
}

// popoverPage has a trigger with a popover (non-modal unless modal), which
// holds a button and a second trigger whose popover is in a Portal of its
// own: presses in it are inside the first popover.
func popoverPage(modal bool, ev *events) func() tela.Node {
	return func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
		pop := aria.UsePopover(aria.PopoverOptions{
			Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close,
			Placement: "bottom-start", Offset: 4, Modal: modal,
		})
		return Div(Class("relative h-full"),
			Button(trig.TriggerProps, TestID("trigger"), Class("absolute left-4 top-4 h-8 w-24"), Text("Filters")),
			Button(TestID("page"), OnClick(func() { ev.add("page") }), Class("absolute left-[300px] top-[220px] h-10 w-20"), Text("Page")),
			Portal(If(trig.Open,
				Div(pop.Props, TestID("pop"), Class("h-32 w-48 bg-white p-2"),
					Button(TestID("inside"), OnClick(func() { ev.add("inside") }), Class("h-8 w-24"), Text("Inside")),
					tela.C(nestedPopover)))))
	}
}

func nestedPopover() tela.Node {
	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
	pop := aria.UsePopover(aria.PopoverOptions{
		Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close, Placement: "right-start",
	})
	return Fragment(
		Button(trig.TriggerProps, TestID("more"), Class("mt-2 h-8 w-24"), Text("More")),
		Portal(If(trig.Open,
			Div(pop.Props, TestID("inner"), Class("h-20 w-32 bg-white p-2"),
				Button(TestID("deep"), Class("h-8 w-16"), Text("Deep"))))))
}

// The popover is where its trigger says in the first frame that shows it,
// and goes away with Escape, a press outside, or a second press on its
// trigger.
func TestPopoverPlacementAndDismissal(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, popoverPage(false, &ev), telatest.Size(400, 300), telatest.NoSettle())
	trigger := telatest.ByTestID("trigger")
	app.Click(trigger)
	r := app.Bounds(telatest.ByTestID("pop"))
	if r.X != 16 || r.Y != 52 || r.Width != 192 || r.Height != 128 {
		t.Errorf("popover at %v, want (16, 52) 192x128 in its first frame", r)
	}
	app.Expect(trigger, telatest.Expanded(true))
	app.Press("Escape")
	if n := app.Count(telatest.ByTestID("pop")); n != 0 {
		t.Errorf("Escape left the popover open")
	}

	// A second press on the trigger closes it (it does not close and open
	// again).
	app.Click(trigger)
	if n := app.Count(telatest.ByTestID("pop")); n != 1 {
		t.Fatalf("the popover did not open")
	}
	app.Click(trigger)
	if n := app.Count(telatest.ByTestID("pop")); n != 0 {
		t.Errorf("a second press on the trigger left it open")
	}

	// A press outside closes it, and is kept from the page under it.
	app.Click(trigger)
	app.Click(telatest.ByTestID("page"))
	if n := app.Count(telatest.ByTestID("pop")); n != 0 {
		t.Errorf("a press outside left it open")
	}
	if got := ev.take(); got != "" {
		t.Errorf("the dismissing press reached the page: %q", got)
	}
	app.Click(telatest.ByTestID("page"))
	if got := ev.take(); got != "page" {
		t.Errorf("events %q", got)
	}
}

// The popovers opened from a popover are inside it: pressing there does not
// close the outer one, a press outside the inner one closes only that, and
// Escape closes the top one first.
func TestPopoverNested(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, popoverPage(false, &ev), telatest.Size(400, 300))
	has := func(id string) bool { return app.Count(telatest.ByTestID(id)) == 1 }
	app.Click(telatest.ByTestID("trigger"))
	app.Click(telatest.ByTestID("more"))
	if !has("pop") || !has("inner") {
		t.Fatalf("both popovers should be open:\n%s", app.AriaSnapshot())
	}
	// Inside the inner popover: nothing closes.
	app.Click(telatest.ByTestID("deep"))
	if !has("pop") || !has("inner") {
		t.Errorf("a press in the inner popover closed one:\n%s", app.AriaSnapshot())
	}
	// Inside the outer one, outside the inner one: only the inner closes.
	app.Click(telatest.ByTestID("inside"))
	if !has("pop") || has("inner") {
		t.Errorf("a press in the outer popover: pop %v inner %v, want true false", has("pop"), has("inner"))
	}
	if got := ev.take(); got != "" {
		t.Errorf("the dismissing press clicked: %q", got)
	}
	app.Click(telatest.ByTestID("more"))
	if !has("inner") {
		t.Fatalf("the inner popover did not open again")
	}
	// Escape closes the top one only.
	app.Press("Escape")
	if !has("pop") || has("inner") {
		t.Errorf("first Escape: pop %v inner %v, want true false", has("pop"), has("inner"))
	}
	app.Press("Escape")
	if has("pop") {
		t.Errorf("second Escape left the outer popover open")
	}
}

// A modal popover makes the page inert, and a press anywhere outside
// closes it, the trigger included.
func TestPopoverModal(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, popoverPage(true, &ev), telatest.Size(400, 300))
	app.Click(telatest.ByTestID("trigger"))
	app.ExpectAria(`
		- button "Inside"
		- button "More"
	`)
	app.ClickAt(30, 30) // the trigger
	app.ExpectNoText("Inside")
	if got := ev.take(); got != "" {
		t.Errorf("events %q", got)
	}
}

// Focus moving out of a popover to another element closes it; the trigger
// does not count (its press toggles).
func TestPopoverClosesOnBlur(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, popoverPage(false, &ev), telatest.Size(400, 300))
	app.Click(telatest.ByTestID("trigger"))
	app.Focus(telatest.ByTestID("inside"))
	app.Focus(telatest.ByTestID("trigger"))
	app.ExpectText("Inside")
	app.Focus(telatest.ByTestID("inside"))
	app.Focus(telatest.ByTestID("page"))
	app.ExpectNoText("Inside")
}

// A popover that exits fades out inside a Presence: it is inert at once
// (the page under it works) and gone when the exit ends.
func TestPopoverExitAnimation(t *testing.T) {
	var ev events
	page := func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
		pop := aria.UsePopover(aria.PopoverOptions{
			Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close, Placement: "bottom-start",
		})
		return Div(Class("relative h-full"),
			Button(trig.TriggerProps, TestID("trigger"), Class("absolute left-4 top-4 h-8 w-24"), Text("Open")),
			Button(TestID("page"), OnClick(func() { ev.add("page") }), Class("absolute left-[300px] top-[220px] h-10 w-20"), Text("Page")),
			Portal(Presence(If(trig.Open,
				Div(pop.Props, TestID("pop"), Exit("opacity-0"),
					Class("h-32 w-48 bg-white p-2 opacity-100 transition-opacity duration-200 starting:opacity-0"),
					Text("Content"))))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300), telatest.NoSettle())
	app.Click(telatest.ByTestID("trigger"))
	app.Advance(300 * time.Millisecond)
	app.ExpectText("Content")
	app.Press("Escape")
	app.Advance(50 * time.Millisecond)
	// Exiting: still painted, out of the snapshot, and the page works.
	if n := app.Count(telatest.ByText("Content")); n != 0 {
		t.Errorf("the exiting popover is still in the semantics tree")
	}
	app.Click(telatest.ByTestID("page"))
	if got := ev.take(); got != "page" {
		t.Errorf("the page under an exiting popover: %q", got)
	}
	app.Advance(400 * time.Millisecond)
	if n := app.Count(telatest.ByTestID("pop")); n != 0 {
		t.Errorf("the popover is still mounted after its exit")
	}
}

// UseOverlay alone: dismissal options.
func TestOverlayOptions(t *testing.T) {
	var closed int
	var ev events
	page := func() tela.Node {
		open, setOpen := tela.UseState(true)
		ov := aria.UseOverlay(aria.OverlayOptions{
			Open: open, OnClose: func() { closed++; setOpen.Set(false) }, Dismissable: true,
			KeyboardDismissDisabled:      true,
			ShouldCloseOnInteractOutside: func(e tela.PointerEvent) bool { return e.X > 200 },
		})
		return Div(Class("relative h-full"),
			Button(TestID("left"), OnClick(func() { ev.add("left") }), Class("absolute left-0 top-0 h-10 w-20"), Text("Left")),
			Button(TestID("right"), OnClick(func() { ev.add("right") }), Class("absolute left-[300px] top-0 h-10 w-20"), Text("Right")),
			If(open, Div(ov.Props, TestID("ov"), Class("absolute left-[100px] top-[100px] h-20 w-20"), Text("Overlay"))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300))
	app.Press("Escape") // taken by the overlay, which cannot be dismissed with it
	app.ExpectText("Overlay")
	app.Click(telatest.ByTestID("left")) // outside, but ShouldCloseOnInteractOutside says no
	app.ExpectText("Overlay")
	if got := ev.take(); got != "left" {
		t.Errorf("a press that does not dismiss must go through: %q", got)
	}
	app.Click(telatest.ByTestID("right"))
	app.ExpectNoText("Overlay")
	if closed != 1 {
		t.Errorf("OnClose ran %d times", closed)
	}
	if got := ev.take(); got != "" {
		t.Errorf("the dismissing press clicked: %q", got)
	}
}

// The trigger's open state: uncontrolled with a notification, and
// controlled.
func TestOverlayTriggerState(t *testing.T) {
	var seen []bool
	page := func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{
			Type: "menu", DefaultOpen: true, OnOpenChange: func(o bool) { seen = append(seen, o) },
		})
		return Div(Button(trig.TriggerProps, TestID("t"), Text("Menu")), If(trig.Open, Div(Ref(trig.OverlayRef), Text("Items"))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.ExpectText("Items")
	trigger := telatest.ByRole("button", "Menu")
	app.Expect(trigger, telatest.Expanded(true))
	app.Click(trigger)
	app.ExpectNoText("Items")
	app.Click(trigger)
	app.ExpectText("Items")
	if len(seen) != 2 || seen[0] || !seen[1] {
		t.Errorf("OnOpenChange got %v", seen)
	}

	var asked []bool
	var setOpen tela.Setter[bool]
	controlled := func() tela.Node {
		open, set := tela.UseState(false)
		setOpen = set
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{
			Open: &open, OnOpenChange: func(o bool) { asked = append(asked, o) },
		})
		return Div(Button(trig.TriggerProps, Text("Go")), If(trig.Open, Div(Text("Panel"))))
	}
	app2 := telatest.MountFunc(t, controlled, telatest.Size(300, 200))
	app2.Click(telatest.ByRole("button", "Go"))
	app2.ExpectNoText("Panel") // the caller did not store the request
	setOpen.Set(true)
	app2.ExpectText("Panel")
	if len(asked) != 1 || !asked[0] {
		t.Errorf("OnOpenChange got %v", asked)
	}
}

func TestOverlayTriggerPopupTypePanics(t *testing.T) {
	msg := recoverMount(t, func() tela.Node {
		aria.UseOverlayTrigger(aria.OverlayTriggerOptions{Type: "dialg"})
		return Div()
	})
	if !strings.Contains(msg, `did you mean "dialog"`) {
		t.Errorf("panic message %q", msg)
	}
}

func TestDialogRolePanics(t *testing.T) {
	msg := recoverMount(t, func() tela.Node {
		aria.UseDialog(aria.DialogOptions{Role: "modal"})
		return Div()
	})
	if !strings.Contains(msg, "alertdialog") {
		t.Errorf("panic message %q", msg)
	}
}

// recoverMount mounts a component and returns the message of the panic its
// build raised, or "".
func recoverMount(t *testing.T, page func() tela.Node, opts ...telatest.Option) (msg string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			if s, ok := r.(string); ok {
				msg = s
			} else if e, ok := r.(error); ok {
				msg = e.Error()
			}
		}
	}()
	telatest.MountFunc(t, page, append([]telatest.Option{telatest.Size(200, 100)}, opts...)...)
	return ""
}

// A dialog with nothing tabbable takes the focus itself.
func TestDialogWithoutTabbableContentTakesFocus(t *testing.T) {
	page := func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
		ov := aria.UseOverlay(aria.OverlayOptions{Open: trig.Open, OnClose: trig.Close, Dismissable: true, Modal: true})
		dlg := aria.UseDialog(aria.DialogOptions{Modal: true, AriaLabel: "Notice", Ref: trig.OverlayRef})
		return Div(Button(trig.TriggerProps, TestID("trigger"), Text("Open")),
			Portal(If(trig.Open, Div(ov.Props, dlg.Props, TestID("dialog"), Text("Saved.")))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	app.Click(telatest.ByTestID("trigger"))
	app.ExpectAria(`
		- dialog "Notice": Saved.
	`)
	app.ExpectFocused(telatest.ByTestID("dialog"))
	app.Press("Tab") // contained: nothing else to reach
	app.ExpectFocused(telatest.ByTestID("dialog"))
	app.Press("Escape")
	app.ExpectFocused(telatest.ByTestID("trigger"))
}

// The popover's arrow points at the trigger even when the popover shifted
// to stay inside the window.
func TestPopoverArrow(t *testing.T) {
	page := func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
		pop := aria.UsePopover(aria.PopoverOptions{
			Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close, Placement: "bottom", Offset: 8,
		})
		return Div(Class("relative h-full"),
			Button(trig.TriggerProps, TestID("trigger"), Class("absolute left-4 top-4 h-8 w-24"), Text("Info")),
			Portal(If(trig.Open,
				Div(pop.Props, TestID("pop"), Class("h-16 w-48 bg-white"),
					Div(pop.ArrowProps, TestID("arrow"), Class("size-2 rotate-45 bg-white")),
					Text("About")))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 300), telatest.NoSettle())
	app.Click(telatest.ByTestID("trigger"))
	trigger, pop, arrow := app.Bounds(telatest.ByTestID("trigger")), app.Bounds(telatest.ByTestID("pop")), app.Bounds(telatest.ByTestID("arrow"))
	if pop.X < 12-0.5 {
		t.Errorf("the popover left the window's padding: x %g", pop.X)
	}
	if got, want := arrow.X+arrow.Width/2, trigger.X+trigger.Width/2; got < want-1 || got > want+1 {
		t.Errorf("the arrow points at x %g, the trigger's center is %g", got, want)
	}
	if arrow.Y >= pop.Y {
		t.Errorf("the arrow (y %g) is not on the side that faces the trigger (popover y %g)", arrow.Y, pop.Y)
	}
}
