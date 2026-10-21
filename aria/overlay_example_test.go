package aria_test

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleUseButton_submit() {
	// A Div made a button that submits its Form, as an HTML submit button
	// does.
	page := func() tela.Node {
		sent, setSent := tela.UseState(0)
		send := aria.UseButton(aria.ButtonOptions{Type: "submit"})
		return Form(OnSubmit(func(tela.SubmitEvent) { setSent.Update(func(n int) int { return n + 1 }) }),
			Div(send.Props, Class("rounded-md bg-primary px-3 py-1.5"), Text("Send")),
			Textf("Sent %d times", sent))
	}
	app := telatest.MountFunc(&exampleTB{}, page)
	app.Click(telatest.ByRole("button", "Send"))
	app.Press("Enter")
	app.ExpectText("Sent 2 times")
	fmt.Println("submitted twice: mouse and Enter")
	// Output: submitted twice: mouse and Enter
}

func ExampleToggleOptions() {
	// An uncontrolled switch that tells the app about every change, and a
	// controlled one: Selected points to the app's own state.
	page := func() tela.Node {
		saved, setSaved := tela.UseState("none")
		on, setOn := tela.UseState(true)
		auto := aria.UseSwitch(aria.ToggleOptions{
			DefaultSelected: true,
			OnChange:        func(v bool) { setSaved.Set(fmt.Sprint(v)) },
		})
		manual := aria.UseSwitch(aria.ToggleOptions{Selected: &on, OnChange: setOn.Set})
		return Div(
			Button(auto.Props, AriaLabel("Auto"), Class("h-6 w-11")),
			Button(manual.Props, AriaLabel("Manual"), Class("h-6 w-11")),
			Textf("auto saved: %s, manual: %v", saved, on))
	}
	app := telatest.MountFunc(&exampleTB{}, page)
	app.Click(telatest.ByRole("switch", "Auto"))
	app.Click(telatest.ByRole("switch", "Manual"))
	app.ExpectText("auto saved: false, manual: false")
	fmt.Println("auto saved: false, manual: false")
	// Output: auto saved: false, manual: false
}

func ExampleUseOverlay() {
	// A modal overlay: Escape and a press outside close it, and the page
	// below is inert while it is open.
	page := func() tela.Node {
		open, setOpen := tela.UseState(true)
		ov := aria.UseOverlay(aria.OverlayOptions{
			Open: open, OnClose: func() { setOpen.Set(false) }, Dismissable: true, Modal: true,
		})
		return Div(Class("relative h-full"),
			Button(Class("h-8 w-24"), Text("Page")),
			If(open, Div(ov.Props, Class("absolute left-[100px] top-[100px] h-20 w-40 bg-white"), Text("Overlay"))))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(300, 200))
	fmt.Println(app.AriaSnapshot())
	app.Press("Escape")
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - text: Overlay
	// - button "Page"
}

func ExampleUseDialog() {
	// A modal dialog named by its title: it takes the focus when it opens,
	// keeps Tab inside, and gives the focus back when it closes.
	page := func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
		ov := aria.UseOverlay(aria.OverlayOptions{Open: trig.Open, OnClose: trig.Close, Dismissable: true, Modal: true})
		dlg := aria.UseDialog(aria.DialogOptions{Modal: true, Ref: trig.OverlayRef})
		return Div(
			Button(trig.TriggerProps, Text("Delete")),
			Portal(If(trig.Open,
				Div(ov.Props, dlg.Props, Class("w-64 bg-white p-4"),
					H2(dlg.TitleProps, Text("Delete project?")),
					Button(Text("Cancel")),
					Button(OnClick(trig.Close), Text("Delete"))))))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(300, 200))
	app.Click(telatest.ByRole("button", "Delete"))
	fmt.Println(app.AriaSnapshot())
	app.Press("Escape")
	app.ExpectFocused(telatest.ByRole("button", "Delete"))
	// Output:
	// - dialog "Delete project?":
	//   - heading "Delete project?" [level=2]
	//   - button "Cancel"
	//   - button "Delete"
}

func ExampleUseOverlayTrigger() {
	// The trigger holds the state and says what it opens and whether it is
	// open; OnOpenChange follows an uncontrolled overlay.
	page := func() tela.Node {
		log, setLog := tela.UseState("")
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{
			Type:         "menu",
			OnOpenChange: func(open bool) { setLog.Set(fmt.Sprintf("open=%v", open)) },
		})
		return Div(
			Button(trig.TriggerProps, Text("Actions")),
			If(trig.Open, Div(Ref(trig.OverlayRef), Text("Items"))),
			Text(log))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(300, 200))
	app.Click(telatest.ByRole("button", "Actions"))
	app.Expect(telatest.ByRole("button", "Actions"), telatest.Expanded(true))
	app.ExpectText("open=true")
	fmt.Println("open=true")
	// Output: open=true
}

func ExampleUsePopover() {
	// A popover 4 px below its trigger; a press outside it closes it.
	page := func() tela.Node {
		trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
		pop := aria.UsePopover(aria.PopoverOptions{
			Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close,
			Placement: "bottom-start", Offset: 4,
		})
		return Div(Class("relative h-full"),
			Button(trig.TriggerProps, Class("absolute left-4 top-4 h-8 w-24"), Text("Filters")),
			Portal(If(trig.Open,
				Div(pop.Props, TestID("pop"), Class("h-24 w-48 rounded-md border bg-white p-2"), Text("Content")))))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Filters"))
	r := app.Bounds(telatest.ByTestID("pop"))
	fmt.Printf("popover at (%.0f, %.0f), %.0fx%.0f\n", r.X, r.Y, r.Width, r.Height)
	app.ClickAt(350, 250)
	app.ExpectNoText("Content")
	fmt.Println("closed by a press outside")
	// Output:
	// popover at (16, 52), 192x96
	// closed by a press outside
}

func ExampleUseMenuTrigger() {
	// ArrowDown opens the menu focusing its first item, ArrowUp its last.
	page := func() tela.Node {
		mt := aria.UseMenuTrigger(aria.MenuTriggerOptions{})
		pop := aria.UsePopover(aria.PopoverOptions{
			Trigger: mt.Ref, Ref: mt.MenuRef, Open: mt.Open, OnClose: mt.Close, Placement: "bottom-start", Modal: true,
		})
		return Div(Class("relative h-full"),
			Button(mt.TriggerProps, Class("absolute left-4 top-4 h-8 w-24"), Text("Actions")),
			Portal(If(mt.Open,
				Div(pop.Props, mt.MenuProps, Role("menu"), Class("w-40 bg-white"),
					Textf("focus the %s item", mt.FocusStrategy)))))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(400, 300))
	app.Focus(telatest.ByRole("button", "Actions"))
	app.Press("ArrowDown")
	app.ExpectText("focus the first item")
	app.Press("Escape")
	app.Press("ArrowUp")
	app.ExpectText("focus the last item")
	fmt.Println("ArrowDown: first, ArrowUp: last")
	// Output: ArrowDown: first, ArrowUp: last
}

func ExampleUseTooltipTrigger() {
	// The tooltip opens 1.5 s after the pointer rests on its trigger, and
	// closes half a second after it leaves.
	page := func() tela.Node {
		tip := aria.UseTooltipTrigger(aria.TooltipTriggerOptions{})
		fl := aria.UseFloating(tip.Ref, aria.Placement("top"), aria.Offset(6), aria.Flip(), aria.Shift(8))
		return Div(Class("relative h-full"),
			Button(tip.TriggerProps, Class("absolute left-4 top-[100px] h-8 w-24"), Text("Save")),
			Portal(If(tip.Open,
				Div(tip.TooltipProps, fl.Props, Class("rounded bg-black px-2 py-1 text-xs text-white"), Text("Saves the file")))))
	}
	app := telatest.MountFunc(&exampleTB{}, page, telatest.Size(400, 300), telatest.NoSettle())
	count := func() int { return app.Count(telatest.ByRole("tooltip", "Saves the file")) }
	app.Hover(telatest.ByRole("button", "Save"))
	app.Advance(time.Second)
	fmt.Println("after 1 s:", count())
	app.Advance(600 * time.Millisecond)
	fmt.Println("after 1.6 s:", count())
	app.MouseMove(300, 250)
	app.Advance(600 * time.Millisecond)
	fmt.Println("0.6 s after leaving:", count())
	// Output:
	// after 1 s: 0
	// after 1.6 s: 1
	// 0.6 s after leaving: 0
}
