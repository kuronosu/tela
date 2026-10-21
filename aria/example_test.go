package aria_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// exampleT stands in for the *testing.T that examples do not have.
type exampleT struct {
	testing.TB // nil: the methods telatest uses are below
	cleanups   []func()
}

func (e *exampleT) Helper()                      {}
func (e *exampleT) Name() string                 { return "example" }
func (e *exampleT) Cleanup(f func())             { e.cleanups = append(e.cleanups, f) }
func (e *exampleT) Logf(string, ...any)          {}
func (e *exampleT) Errorf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Fatalf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Failed() bool                 { return false }
func (e *exampleT) close() {
	for i := len(e.cleanups) - 1; i >= 0; i-- {
		e.cleanups[i]()
	}
}

func ExampleUsePress() {
	t := &exampleT{}
	defer t.close()
	save := func() tela.Node {
		saved, setSaved := tela.UseState(0)
		press := aria.UsePress(aria.PressOptions{
			OnPress: func(e aria.PressEvent) { setSaved.Update(func(n int) int { return n + 1 }) },
		})
		return Div(press.Props, Role("button"), TabIndex(0),
			Class("rounded-md px-3 py-1.5"), If(press.Pressed, Class("scale-95")),
			Textf("Saved %d times", saved))
	}
	app := telatest.MountFunc(t, save)
	app.Click(telatest.ByRole("button", "Saved 0 times"))
	app.Press("Enter") // the button has the focus after the click
	app.Press("Space")
	app.ExpectText("Saved 3 times")
	fmt.Println("3 presses: mouse, Enter, Space")
	// Output: 3 presses: mouse, Enter, Space
}

func ExamplePressEvent_ContinuePropagation() {
	t := &exampleT{}
	defer t.close()
	var log []string
	page := func() tela.Node {
		press := aria.UsePress(aria.PressOptions{
			// Let the card's own handlers see the press too.
			OnPressStart: func(e aria.PressEvent) { e.ContinuePropagation() },
		})
		return Div(OnPointerDown(func() { log = append(log, "the card saw the pointer down") }),
			Button(press.Props, Text("Like")))
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Like"))
	fmt.Println(log)
	// Output: [the card saw the pointer down]
}

func ExampleUseHover() {
	t := &exampleT{}
	defer t.close()
	card := func() tela.Node {
		hover := aria.UseHover(aria.HoverOptions{})
		return Div(hover.Props, Class("h-20 w-40"),
			If(hover.Hovered, Text("Hovered")), If(!hover.Hovered, Text("Idle")))
	}
	app := telatest.MountFunc(t, card)
	app.ExpectText("Idle")
	app.Hover(telatest.ByText("Idle"))
	app.ExpectText("Hovered")
	fmt.Println("hovered")
	// Output: hovered
}

func ExampleUseFocusRing() {
	t := &exampleT{}
	defer t.close()
	field := func() tela.Node {
		ring := aria.UseFocusRing(aria.FocusRingOptions{})
		return Button(ring.Props, Class("px-3 py-1.5"),
			If(ring.FocusVisible, Text("Ring on")), If(!ring.FocusVisible, Text("Ring off")))
	}
	app := telatest.MountFunc(t, field)
	app.Click(telatest.ByRole("button", "Ring off")) // a click does not show the ring
	app.ExpectText("Ring off")
	app.Press("Shift+Tab") // the keyboard does
	app.Press("Tab")
	app.ExpectText("Ring on")
	fmt.Println("the ring follows the keyboard")
	// Output: the ring follows the keyboard
}

func ExampleUseFocusScope() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		open, setOpen := tela.UseState(false)
		return Div(
			Button(OnClick(func() { setOpen.Set(true) }), Text("Open")),
			If(open, tela.C(settingsDialog)))
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Open"))
	app.ExpectFocused(telatest.ByRole("button", "First")) // AutoFocus
	app.Press("Tab")
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "First")) // Contain wraps around
	fmt.Println("the focus stays inside the dialog")
	// Output: the focus stays inside the dialog
}

func settingsDialog() tela.Node {
	scope := aria.UseFocusScope(aria.FocusScopeOptions{Contain: true, Restore: true, AutoFocus: true})
	return Div(scope.Props, Role("dialog"), AriaLabel("Settings"),
		Button(Text("First")), Button(Text("Second")))
}

func ExampleFocusManager() {
	t := &exampleT{}
	defer t.close()
	toolbar := func() tela.Node {
		scope := aria.UseFocusScope(aria.FocusScopeOptions{})
		return Div(scope.Props, Role("toolbar"), AriaLabel("Format"),
			OnKeyDown(func(e tela.KeyboardEvent) {
				switch e.Key {
				case tela.KeyArrowRight:
					e.PreventDefault()
					scope.Manager.FocusNext(aria.FocusOptions{Wrap: true})
				case tela.KeyArrowLeft:
					e.PreventDefault()
					scope.Manager.FocusPrevious(aria.FocusOptions{Wrap: true})
				}
			}),
			Button(Text("Bold")), Button(Text("Italic")), Button(Text("Underline")))
	}
	app := telatest.MountFunc(t, toolbar)
	app.Focus(telatest.ByRole("button", "Underline"))
	app.Press("ArrowRight") // wraps around
	app.ExpectFocused(telatest.ByRole("button", "Bold"))
	app.Press("ArrowLeft")
	app.ExpectFocused(telatest.ByRole("button", "Underline"))
	fmt.Println("the arrows move the focus")
	// Output: the arrows move the focus
}

func ExampleUseButton() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		clicks, setClicks := tela.UseState(0)
		btn := aria.UseButton(aria.ButtonOptions{
			OnPress: func(aria.PressEvent) { setClicks.Update(func(n int) int { return n + 1 }) },
		})
		// A Div that is a button: role, tab stop and the press come with the hook.
		return Div(btn.Props, Class("rounded-md bg-primary px-3 py-1.5 text-primary-foreground"),
			If(btn.Pressed, Class("scale-95")), Textf("Clicked %d", clicks))
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Clicked 0"))
	fmt.Println(app.AriaSnapshot())
	// Output: - button "Clicked 1"
}

func ExampleUseToggleButton() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		bold, setBold := tela.UseState(false)
		tb := aria.UseToggleButton(aria.ToggleOptions{Selected: &bold, OnChange: setBold.Set})
		return Button(tb.Props, Class("rounded px-2 aria-pressed:bg-accent"), Text("Bold"))
	}
	app := telatest.MountFunc(t, page)
	fmt.Println(app.AriaSnapshot())
	app.Click(telatest.ByRole("button", "Bold"))
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - button "Bold"
	// - button "Bold" [pressed]
}

func ExampleUseCheckbox() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})
		return Div(Class("flex items-center gap-2"),
			Button(box.Props, Class("size-4 rounded border checked:bg-primary")),
			Label(For(box.ID), Text("Notifications")))
	}
	app := telatest.MountFunc(t, page)
	fmt.Println(app.AriaSnapshot())
	app.Click(telatest.ByText("Notifications")) // the Label toggles it
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - checkbox "Notifications" [checked]
	// - text: Notifications
	// - checkbox "Notifications"
	// - text: Notifications
}

func ExampleUseSwitch() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		on, setOn := tela.UseState(false)
		sw := aria.UseSwitch(aria.ToggleOptions{Selected: &on, OnChange: setOn.Set})
		return Button(sw.Props, AriaLabel("Wi-Fi"), Class("h-6 w-11 rounded-full bg-muted checked:bg-primary"))
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("switch", "Wi-Fi"))
	fmt.Println(app.AriaSnapshot())
	// Output: - switch "Wi-Fi" [checked]
}
