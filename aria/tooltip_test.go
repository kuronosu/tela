package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// tooltipPage has two buttons with tooltips, "Save" at (16, 100) and
// "Print" at (136, 100), and a popover-free empty area below.
func tooltipPage(o aria.TooltipTriggerOptions) func() tela.Node {
	return func() tela.Node {
		a := aria.UseTooltipTrigger(o)
		b := aria.UseTooltipTrigger(o)
		fa := aria.UseFloating(a.Ref, aria.Placement("top"), aria.Offset(6), aria.Flip(), aria.Shift(8))
		fb := aria.UseFloating(b.Ref, aria.Placement("top"), aria.Offset(6), aria.Flip(), aria.Shift(8))
		return Div(Class("relative h-full"),
			Button(a.TriggerProps, TestID("save"), Class("absolute left-4 top-[100px] h-8 w-24"), Text("Save")),
			Button(b.TriggerProps, TestID("print"), Class("absolute left-[136px] top-[100px] h-8 w-24"), Text("Print")),
			Portal(
				If(a.Open, Div(a.TooltipProps, fa.Props, TestID("tip-save"), Class("rounded bg-black px-2 py-1 text-xs text-white"), Text("Saves the file"))),
				If(b.Open, Div(b.TooltipProps, fb.Props, TestID("tip-print"), Class("rounded bg-black px-2 py-1 text-xs text-white"), Text("Prints the page")))))
	}
}

func tipOpen(app *telatest.App, id string) bool { return app.Count(telatest.ByTestID(id)) == 1 }

func TestTooltipDelay(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{}), telatest.Size(400, 300), telatest.NoSettle())
	save := telatest.ByTestID("save")
	app.Hover(save)
	app.Advance(1400 * time.Millisecond)
	if tipOpen(app, "tip-save") {
		t.Fatal("the tooltip opened before its delay")
	}
	app.Advance(200 * time.Millisecond)
	if !tipOpen(app, "tip-save") {
		t.Fatal("the tooltip did not open after 1.5 s")
	}
	app.ExpectAria(`
		- button "Save"
		- button "Print"
		- tooltip "Saves the file"
	`)
	// It is above its button, 6 px away.
	tip, btn := app.Bounds(telatest.ByTestID("tip-save")), app.Bounds(save)
	if tip.Bottom() != btn.Y-6 {
		t.Errorf("tooltip bottom %g, want %g", tip.Bottom(), btn.Y-6)
	}
	// Leaving closes it after the close delay.
	app.MouseMove(300, 250)
	app.Advance(400 * time.Millisecond)
	if !tipOpen(app, "tip-save") {
		t.Error("the tooltip closed before its close delay")
	}
	app.Advance(200 * time.Millisecond)
	if tipOpen(app, "tip-save") {
		t.Error("the tooltip stayed open after its close delay")
	}
}

// The second tooltip opens at once while the first is open, and for half a
// second after it closed; otherwise the delay applies again.
func TestTooltipWarmUpAndCoolDown(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{}), telatest.Size(400, 300), telatest.NoSettle())
	save, print := telatest.ByTestID("save"), telatest.ByTestID("print")
	app.Hover(save)
	app.Advance(1600 * time.Millisecond)
	if !tipOpen(app, "tip-save") {
		t.Fatal("the first tooltip did not open")
	}
	// Warm: the second one opens at once and the first closes.
	app.Hover(print)
	if !tipOpen(app, "tip-print") || tipOpen(app, "tip-save") {
		t.Errorf("warm: print %v, save %v; want true, false", tipOpen(app, "tip-print"), tipOpen(app, "tip-save"))
	}
	// Leave everything; the tooltip lingers 0.5 s and the group cools down.
	app.MouseMove(300, 250)
	app.Advance(300 * time.Millisecond)
	app.Hover(save) // cooling down, still warm
	if !tipOpen(app, "tip-save") || tipOpen(app, "tip-print") {
		t.Errorf("cool-down: save %v, print %v; want true, false", tipOpen(app, "tip-save"), tipOpen(app, "tip-print"))
	}
	app.MouseMove(300, 250)
	app.Advance(time.Second) // closed and cold
	if tipOpen(app, "tip-save") {
		t.Fatal("the tooltip stayed open")
	}
	app.Hover(print)
	app.Advance(1000 * time.Millisecond)
	if tipOpen(app, "tip-print") {
		t.Error("cold: the tooltip opened before its delay")
	}
	app.Advance(600 * time.Millisecond)
	if !tipOpen(app, "tip-print") {
		t.Error("cold: the tooltip did not open after its delay")
	}
}

// The pointer can move onto the tooltip: it stays open; pressing the trigger
// closes it.
func TestTooltipHoverAndPress(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{Delay: aria.NoDelay}), telatest.Size(400, 300), telatest.NoSettle())
	save := telatest.ByTestID("save")
	app.Hover(save)
	if !tipOpen(app, "tip-save") {
		t.Fatal("NoDelay: the tooltip did not open at once")
	}
	// Onto the tooltip, across the 6 px gap, within the close delay.
	tip := app.Bounds(telatest.ByTestID("tip-save"))
	app.MouseMove(tip.X+5, tip.Y+tip.Height+3)
	app.Advance(200 * time.Millisecond)
	app.MouseMove(tip.X+5, tip.Y+3)
	app.Advance(2 * time.Second)
	if !tipOpen(app, "tip-save") {
		t.Error("the tooltip closed while the pointer was on it")
	}
	app.MouseMove(300, 250)
	app.Advance(time.Second)
	if tipOpen(app, "tip-save") {
		t.Fatal("the tooltip stayed open")
	}
	// A press on the trigger closes it at once, and it does not come back
	// while the pointer stays.
	app.Hover(save)
	if !tipOpen(app, "tip-save") {
		t.Fatal("the tooltip did not open")
	}
	app.MouseDown()
	if tipOpen(app, "tip-save") {
		t.Error("pressing the trigger left the tooltip open")
	}
	app.MouseUp()
	app.Advance(time.Second)
	if tipOpen(app, "tip-save") {
		t.Error("the tooltip came back after a press")
	}
}

// Keyboard focus opens it at once, a pointer press that focuses does not,
// and Escape closes it.
func TestTooltipFocusAndEscape(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{}), telatest.Size(400, 300), telatest.NoSettle())
	app.Press("Tab")
	app.ExpectFocused(telatest.ByTestID("save"))
	if !tipOpen(app, "tip-save") {
		t.Fatal("the keyboard focus did not open the tooltip")
	}
	app.Press("Escape")
	if tipOpen(app, "tip-save") {
		t.Error("Escape left the tooltip open")
	}
	// Tab moves on: the next one opens, the first stays closed.
	app.Press("Tab")
	if !tipOpen(app, "tip-print") || tipOpen(app, "tip-save") {
		t.Errorf("Tab: print %v, save %v; want true, false", tipOpen(app, "tip-print"), tipOpen(app, "tip-save"))
	}
	app.Press("Tab") // out of the page
	if tipOpen(app, "tip-print") {
		t.Error("the tooltip stayed open after the focus left")
	}
	// A mouse press that focuses the trigger shows nothing once the pointer
	// stays: pressing closes, and the focus is not a keyboard focus.
	app.Click(telatest.ByTestID("save"))
	app.Advance(2 * time.Second)
	app.MouseMove(300, 250)
	app.Advance(time.Second)
	if tipOpen(app, "tip-save") {
		t.Error("a mouse press opened a tooltip")
	}
}

// Escape closes a tooltip opened by hover, with the focus elsewhere.
func TestTooltipEscapeAfterHover(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{Delay: aria.NoDelay}), telatest.Size(400, 300), telatest.NoSettle())
	app.Hover(telatest.ByTestID("save"))
	if !tipOpen(app, "tip-save") {
		t.Fatal("the tooltip did not open")
	}
	app.Press("Escape")
	if tipOpen(app, "tip-save") {
		t.Error("Escape left the tooltip open")
	}
}

func TestTooltipFocusOnlyTrigger(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{Trigger: "focus"}), telatest.Size(400, 300), telatest.NoSettle())
	app.Hover(telatest.ByTestID("save"))
	app.Advance(3 * time.Second)
	if tipOpen(app, "tip-save") {
		t.Error("hover opened a focus-only tooltip")
	}
	app.Press("Tab")
	if !tipOpen(app, "tip-save") {
		t.Error("focus did not open a focus-only tooltip")
	}
}

func TestTooltipDisabledAndControlled(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{Disabled: true, Delay: aria.NoDelay}),
		telatest.Size(400, 300), telatest.NoSettle())
	app.Hover(telatest.ByTestID("save"))
	app.Press("Tab")
	app.Advance(time.Second)
	if tipOpen(app, "tip-save") || tipOpen(app, "tip-print") {
		t.Error("a disabled tooltip opened")
	}

	var asked []bool
	page := func() tela.Node {
		open := false
		tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{
			Delay: aria.NoDelay, Open: &open, OnOpenChange: func(o bool) { asked = append(asked, o) },
		})
		return Div(Button(tip.TriggerProps, TestID("b"), Class("h-8 w-24"), Text("B")), If(tip.Open, Div(tip.TooltipProps, Text("Never"))))
	}
	c := telatest.MountFunc(t, page, telatest.Size(300, 200), telatest.NoSettle())
	c.Hover(telatest.ByTestID("b"))
	if c.Count(telatest.ByRole("tooltip", "Never")) != 0 {
		t.Error("a controlled tooltip opened without the caller")
	}
	if len(asked) == 0 || !asked[0] {
		t.Errorf("OnOpenChange got %v, want a request to open", asked)
	}
}

// A pending and an open tooltip cost no frames: only the deadline matters.
func TestTooltipIdle(t *testing.T) {
	app := telatest.MountFunc(t, tooltipPage(aria.TooltipTriggerOptions{}), telatest.Size(400, 300), telatest.NoSettle())
	app.Hover(telatest.ByTestID("save"))
	before := app.Stats().Frames
	app.Advance(1400 * time.Millisecond) // the delay is pending
	if got := app.Stats().Frames; got != before {
		t.Errorf("a pending tooltip ran %d frames", got-before)
	}
	app.Advance(300 * time.Millisecond) // it opens
	if !tipOpen(app, "tip-save") {
		t.Fatal("the tooltip did not open")
	}
	app.Advance(time.Second)
	before = app.Stats().Frames
	app.Advance(10 * time.Second) // open and idle
	if got := app.Stats().Frames; got != before {
		t.Errorf("an open tooltip ran %d frames", got-before)
	}
}

func TestTooltipTriggerOptionPanics(t *testing.T) {
	msg := recoverMount(t, func() tela.Node {
		aria.UseTooltipTrigger(aria.TooltipTriggerOptions{Trigger: "hover"})
		return Div()
	})
	if msg == "" {
		t.Error("an unknown Trigger did not panic")
	}
}
