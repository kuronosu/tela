package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// ColorInput has no shadcn/ui original either: a golden sheet of its
// closed states, goldens of the open window, and behavior tests.

func colorInputSheet() *sheet {
	return &sheet{name: "colorinput", width: 800, slot: 200, cells: []cell{
		{name: "default", tela: kit.ColorInput{DefaultValue: indigo}},
		{name: "alpha", tela: kit.ColorInput{DefaultValue: tela.Hex("#f59e0b99"), Alpha: true}},
		{name: "disabled", tela: kit.ColorInput{DefaultValue: indigo, Disabled: true}},
		{name: "wide", width: 192, tela: kit.ColorInput{DefaultValue: tela.Hex("#22c55e"), Class: "w-full"}},
	}}
}

func TestColorInputGolden(t *testing.T) { colorInputSheet().golden(t) }

var (
	colorTrigger = telatest.ByRole("button", "#6366f1")
	colorPanelEl = telatest.ByRole("dialog", "Color picker")
)

func colorInputPage(c kit.ColorInput) func() tela.Node { return pageUnder(c) }

func TestColorInputOpenGolden(t *testing.T) {
	page := colorInputPage(kit.ColorInput{DefaultValue: indigo, Alpha: true, Swatches: presets})
	openGolden(t, "colorinput", 480, 640, page, func(a *telatest.App) { a.Click(colorTriggerAlpha) })
}

func TestColorInputOpenClose(t *testing.T) {
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: indigo}), telatest.Theme(kit.Theme()))
	app.ExpectCount(colorPanelEl, 0)
	app.Expect(colorTrigger, telatest.HasPopup("dialog"), telatest.Expanded(false))
	app.Click(colorTrigger)
	app.ExpectCount(colorPanelEl, 1)
	app.Expect(colorTrigger, telatest.Expanded(true))
	// The focus went into the panel.
	app.ExpectFocused(areaThumb)
	// Escape closes it, and the focus goes back to the trigger.
	app.Press("Escape")
	app.ExpectCount(colorPanelEl, 0)
	app.ExpectFocused(colorTrigger)
	// A press outside closes it too.
	app.Click(colorTrigger)
	app.ExpectCount(colorPanelEl, 1)
	app.Click(telatest.ByRole("button", "Save changes"))
	app.ExpectCount(colorPanelEl, 0)
}

// The color lives in the ColorInput: what was chosen is still there when
// the panel is opened again, and the trigger shows it.
func TestColorInputKeepsTheColor(t *testing.T) {
	var changed, committed colorLog
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: indigo, Alpha: true, Swatches: presets,
		OnValueChange: changed.add, OnValueCommit: committed.add}), telatest.Theme(kit.Theme()))
	app.Expect(telatest.ByRole("button", "#6366f1ff"), telatest.Expanded(false))
	app.Click(telatest.ByRole("button", "#6366f1ff"))
	app.Click(telatest.ByRole("button", "#22c55e"))
	app.Expect(telatest.ByRole("button", "#22c55eff"), telatest.Expanded(true))
	app.Expect(hexField, telatest.HasValue("#22c55eff"))
	// Drag in the area: the button's code follows once the drag ends.
	area := app.Bounds(areaGroup)
	app.MouseMove(at(area, 0.5, 0.5))
	app.MouseDown()
	app.MouseMove(at(area, 0.25, 0.25))
	app.MouseUp()
	app.Press("Escape")
	app.ExpectCount(colorPanelEl, 0)
	// The trigger shows the color the drag ended on.
	chosen := hexOf(committed.last(), true)
	app.Click(telatest.ByRole("button", chosen))
	app.ExpectCount(colorPanelEl, 1)
	app.Expect(areaThumb, telatest.HasValue("Saturation 25%, Brightness 75%"))
	if len(committed.got) != 2 {
		t.Errorf("OnValueCommit was called %d times (swatch, drag), want 2", len(committed.got))
	}
	if len(changed.got) < 3 {
		t.Errorf("OnValueChange was called %d times, want one per change", len(changed.got))
	}
}

func TestColorInputDragRepaintsOnly(t *testing.T) {
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: indigo, Alpha: true}), telatest.Theme(kit.Theme()))
	app.Click(colorTriggerAlpha)
	area := app.Bounds(areaGroup)
	app.MouseMove(at(area, 0.1, 0.1))
	app.MouseDown()
	st := app.Measure(func() {
		for i := 1; i <= 20; i++ {
			app.MouseMove(area.X+area.Width*0.1+float32(i)*8, area.Y+area.Height*0.1+float32(i)*4)
		}
	})
	t.Logf("20 moves of a drag in the popover: %d builds, %d layouts, %d paints", st.Builds, st.Layouts, st.Paints)
	if st.Builds > 0 || st.Layouts > 0 {
		t.Errorf("%d builds and %d layouts while dragging in a ColorInput's panel, want none", st.Builds, st.Layouts)
	}
	app.MouseUp()
}

var colorTriggerAlpha = telatest.ByRole("button", "#6366f1ff")

func TestColorInputControlledOpen(t *testing.T) {
	open := true
	app := telatest.MountFunc(t, func() tela.Node {
		return pageUnder(kit.ColorInput{DefaultValue: indigo, Open: &open, OnOpenChange: func(o bool) { open = o }})()
	}, telatest.Theme(kit.Theme()))
	app.ExpectCount(colorPanelEl, 1)
	app.Press("Escape")
	if open {
		t.Errorf("Escape did not ask to close")
	}
}

func TestColorInputControlledValue(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		c, setC := tela.UseState(tela.Hex("#336699"))
		return h.Div(h.Class("p-4 flex flex-col gap-4 items-start"),
			kit.ColorInput{Value: &c, OnValueChange: setC.Set},
			h.P(h.Text("now "+hexOf(c, false))),
			h.Button(h.Text("Reset"), h.OnClick(func() { setC.Set(tela.Hex("#ff0000")) })))
	}, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Reset"))
	app.ExpectText("now #ff0000")
	app.Click(telatest.ByRole("button", "#ff0000"))
	app.Expect(hexField, telatest.HasValue("#ff0000"))
	app.Expect(hueThumb, telatest.HasValue("Hue 0°"))
	// Choosing in the panel is asked of the caller.
	app.Click(hueThumb)
	app.Press("Shift+ArrowRight")
	app.ExpectText("now " + hexOf(hsb(15, 100, 100), false))
}

func TestColorInputCustomTrigger(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-6"), kit.ColorInput{DefaultValue: indigo,
			Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
				return h.Button(p, h.Class("rounded-full bg-primary size-8"), h.AriaLabel("Pick a color"))
			})})
	}, telatest.Theme(kit.Theme()))
	pick := telatest.ByRole("button", "Pick a color")
	app.Click(pick)
	app.ExpectCount(colorPanelEl, 1)
	app.Press("Escape")
	app.ExpectFocused(pick)
}

func TestColorInputDisabled(t *testing.T) {
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: indigo, Disabled: true}), telatest.Theme(kit.Theme()))
	app.Expect(colorTrigger, telatest.Disabled(true))
}

func TestColorInputIdle(t *testing.T) {
	expectIdle(t, colorInputPage(kit.ColorInput{DefaultValue: indigo, DefaultOpen: true, Alpha: true, Swatches: presets}), nil)
}

func ExampleColorInput() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.ColorInput{DefaultValue: tela.Hex("#6366f1"), Swatches: []tela.Color{tela.Hex("#ef4444"), tela.Hex("#22c55e")}})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "#6366f1"))
	app.Click(telatest.ByRole("button", "#22c55e"))
	app.Press("Escape")
	app.Expect(telatest.ByRole("button", "#22c55e"), telatest.Expanded(false))
	// Output:
}
