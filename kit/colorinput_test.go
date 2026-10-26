package kit_test

import (
	"fmt"
	"strings"
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
		{name: "default", tela: kit.ColorInput{DefaultValue: new(indigo)}},
		{name: "alpha", tela: kit.ColorInput{DefaultValue: new(tela.Hex("#f59e0b99")), Alpha: true}},
		{name: "disabled", tela: kit.ColorInput{DefaultValue: new(indigo), Disabled: true}},
		{name: "wide", width: 192, tela: kit.ColorInput{DefaultValue: new(tela.Hex("#22c55e")), Class: "w-full"}},
	}}
}

func TestColorInputGolden(t *testing.T) { colorInputSheet().golden(t) }

var (
	colorTrigger = telatest.ByRole("button", "#6366f1")
	colorPanelEl = telatest.ByRole("dialog", "Color picker")
)

func colorInputPage(c kit.ColorInput) func() tela.Node { return pageUnder(c) }

func TestColorInputOpenGolden(t *testing.T) {
	page := colorInputPage(kit.ColorInput{DefaultValue: new(indigo), Alpha: true, Swatches: presets})
	openGolden(t, "colorinput", 480, 640, page, func(a *telatest.App) { a.Click(colorTriggerAlpha) })
}

func TestColorInputOpenClose(t *testing.T) {
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: new(indigo)}), telatest.Theme(kit.Theme()))
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
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: new(indigo), Alpha: true, Swatches: presets,
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
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: new(indigo), Alpha: true}), telatest.Theme(kit.Theme()))
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
	// The button's code follows the drag (ADR-162): one layout per move for
	// its text, and no build.
	if st.Builds > 0 || st.Layouts > 20 {
		t.Errorf("%d builds and %d layouts while dragging in a ColorInput's panel, want none and at most one per move", st.Builds, st.Layouts)
	}
	app.MouseUp()
}

var colorTriggerAlpha = telatest.ByRole("button", "#6366f1ff")

// colorTriggerID names the default trigger by its test id, which the code
// and the name on it do not change (the name is the code, and it lags a drag
// until a build).
var colorTriggerID = telatest.ByTestID("color-trigger")

// The code on the button follows a drag in the panel as the swatch does: it
// is a paint-phase text, so a move changes it with no build, and the
// button's name (a build's) has caught up when the drag ends.
func TestColorInputCodeFollowsTheDrag(t *testing.T) {
	for _, alpha := range []bool{false, true} {
		t.Run(fmt.Sprintf("alpha=%v", alpha), func(t *testing.T) {
			var changed colorLog
			app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: new(indigo), Alpha: alpha,
				OnValueChange: changed.add, Props: h.TestID("color-trigger")}), telatest.Theme(kit.Theme()), telatest.NoSettle())
			app.Click(colorTriggerID)
			area := app.Bounds(areaGroup)
			app.MouseMove(at(area, 0.5, 0.5))
			app.MouseDown()
			shown := func(code string) int { return app.Count(telatest.ByText(code, telatest.Exact())) }
			seen := map[string]bool{}
			builds := 0
			for i := 1; i <= 8; i++ {
				fx := 0.5 - float32(i)*0.05
				st := app.Measure(func() { app.MouseMove(at(area, fx, 0.5+float32(i)*0.05)) })
				builds += st.Builds
				code := hexOf(changed.last(), alpha)
				if n := shown(code); n != 1 {
					t.Fatalf("move %d: the button shows its code %d times, want %q once; the window:\\n%s", i, n, code, app.AriaSnapshot())
				}
				seen[code] = true
			}
			if len(seen) < 4 {
				t.Errorf("the drag went through %d colors, want the code to change with it", len(seen))
			}
			if builds > 0 {
				t.Errorf("%d builds while dragging, want none: the code is painted, not built", builds)
			}
			app.MouseUp()
			// The drag's end builds: the name has the color the drag ended on.
			code := hexOf(changed.last(), alpha)
			app.Expect(colorTriggerID, telatest.HasName(code))
			if shown(code) != 1 {
				t.Errorf("after the drag the button does not show %q once", code)
			}
		})
	}
}

// The hex field follows a drag in the area, in a picker and in the panel of a
// ColorInput, uncontrolled and controlled: its text is a bound Value that the
// color state updates when the color moves, so a move builds nothing (the
// controlled picker builds as often as its state does, ADR-132: the same two
// builds as before the field followed the drag).
func TestColorHexFieldFollowsTheDrag(t *testing.T) {
	for _, tc := range []struct {
		name       string
		page       func(onChange func(tela.Color)) func() tela.Node
		open       bool
		maxBuilds  int
		maxLayouts int
	}{
		{"picker", func(on func(tela.Color)) func() tela.Node {
			return colorPage(kit.ColorPicker{DefaultValue: new(indigo), Alpha: true, OnValueChange: on})
		}, false, 0, 0},
		{"input", func(on func(tela.Color)) func() tela.Node {
			return colorInputPage(kit.ColorInput{DefaultValue: new(indigo), Alpha: true, OnValueChange: on, Props: h.TestID("color-trigger")})
		}, true, 0, 1}, // the layout is the button's code
		{"controlled", func(on func(tela.Color)) func() tela.Node {
			return func() tela.Node {
				c, setC := tela.UseState(indigo)
				return h.Div(h.Class("p-4"), kit.ColorPicker{Value: &c, Alpha: true, OnValueChange: func(n tela.Color) { on(n); setC.Set(n) }})
			}
		}, false, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var changed colorLog
			app := telatest.MountFunc(t, tc.page(changed.add), telatest.Theme(kit.Theme()), telatest.NoSettle())
			if tc.open {
				app.Click(colorTriggerID)
			}
			// The snapshot is read at once: Expect would let time pass, and the
			// build of a resting pointer would hide a field that lags.
			hexShows := func(code string) bool {
				return strings.Contains(app.AriaSnapshot(), `textbox "Hex": "`+code+`"`)
			}
			area := app.Bounds(areaGroup)
			app.MouseMove(at(area, 0.5, 0.5))
			app.MouseDown()
			for i := 1; i <= 8; i++ {
				st := app.Measure(func() { app.MouseMove(at(area, 0.5-float32(i)*0.05, 0.5+float32(i)*0.05)) })
				if code := hexOf(changed.last(), true); !hexShows(code) {
					t.Fatalf("move %d: the hex field does not show %s; the window:\n%s", i, code, app.AriaSnapshot())
				}
				if st.Builds > tc.maxBuilds || st.Layouts > tc.maxLayouts {
					t.Errorf("move %d: %d builds and %d layouts, want at most %d and %d", i, st.Builds, st.Layouts, tc.maxBuilds, tc.maxLayouts)
				}
			}
			app.MouseUp()
			if code := hexOf(changed.last(), true); !hexShows(code) {
				t.Errorf("after the drag the hex field does not show %s", code)
			}
		})
	}
}

// The field is not overwritten while the user types in it: a color that
// changes from outside (here a controlled value) is shown when the text is
// committed or reverted, and the field follows it again afterwards.
func TestColorHexFieldKeepsTheDraft(t *testing.T) {
	var setC tela.Setter[tela.Color]
	app := telatest.MountFunc(t, func() tela.Node {
		c, s := tela.UseState(indigo)
		setC = s
		return h.Div(h.Class("p-4"), kit.ColorPicker{Value: &c, OnValueChange: s.Set})
	}, telatest.Theme(kit.Theme()))
	app.Focus(hexField)
	app.Press("Control+a")
	app.Type("12")
	setC.Set(tela.Hex("#ff0000"))
	app.Expect(areaThumb, telatest.HasValue("Saturation 100%, Brightness 100%"))
	app.Expect(hexField, telatest.HasValue("12"))
	// "12" is not a code: Enter takes the color back.
	app.Press("Enter")
	app.Expect(hexField, telatest.HasValue("#ff0000"))
	// Focused but not typing, the field follows the color.
	setC.Set(tela.Hex("#00ff00"))
	app.Expect(hexField, telatest.HasValue("#00ff00"))
}

// The button keeps its width while the color changes: Inter is proportional,
// and the code has a minimum width that fits the widest code of its format
// (the digits are wider than the letters a to f).
func TestColorInputTriggerWidthIsStable(t *testing.T) {
	for _, alpha := range []bool{false, true} {
		t.Run(fmt.Sprintf("alpha=%v", alpha), func(t *testing.T) {
			codes := []string{"#111111", "#000000", "#ffffff", "#dddddd", "#6366f1", "#eeeeee", "#ff0000", "#bdbdbd"}
			app := telatest.MountFunc(t, func() tela.Node {
				c, setC := tela.UseState(tela.Hex(codes[0]))
				kids := []tela.Child{h.Class("flex flex-col items-start gap-2 p-4"),
					kit.ColorInput{Value: &c, OnValueChange: setC.Set, Alpha: alpha, Props: h.TestID("color-trigger")}}
				for _, code := range codes {
					kids = append(kids, h.Button(h.Text("Set "+code), h.OnClick(func() { setC.Set(tela.Hex(code)) })))
				}
				return h.Div(kids...)
			}, telatest.Theme(kit.Theme()))
			want := app.Bounds(colorTriggerID).Width
			for _, code := range codes {
				app.Click(telatest.ByRole("button", "Set "+code))
				name := hexOf(tela.Hex(code), alpha)
				app.Expect(colorTriggerID, telatest.HasName(name))
				if got := app.Bounds(colorTriggerID).Width; got < want-0.01 || got > want+0.01 {
					t.Errorf("the button is %.2fpx wide with %s and was %.2fpx wide with %s, want the same width", got, name, want, codes[0])
				}
			}
		})
	}
}

func TestColorInputControlledOpen(t *testing.T) {
	open := true
	app := telatest.MountFunc(t, func() tela.Node {
		return pageUnder(kit.ColorInput{DefaultValue: new(indigo), Open: &open, OnOpenChange: func(o bool) { open = o }})()
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
		return h.Div(h.Class("p-6"), kit.ColorInput{DefaultValue: new(indigo),
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
	app := telatest.MountFunc(t, colorInputPage(kit.ColorInput{DefaultValue: new(indigo), Disabled: true}), telatest.Theme(kit.Theme()))
	app.Expect(colorTrigger, telatest.Disabled(true))
	// It is a Popover's trigger (Popover.Disabled, ADR-145), not a bare
	// button: it says what it would open, and does not open it.
	app.Expect(colorTrigger, telatest.Expanded(false))
	app.ExpectCount(colorPanelEl, 0)
}

func TestColorInputIdle(t *testing.T) {
	expectIdle(t, colorInputPage(kit.ColorInput{DefaultValue: new(indigo), DefaultOpen: true, Alpha: true, Swatches: presets}), nil)
}

func ExampleColorInput() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.ColorInput{DefaultValue: new(tela.Hex("#6366f1")), Swatches: []tela.Color{tela.Hex("#ef4444"), tela.Hex("#22c55e")}})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "#6366f1"))
	app.Click(telatest.ByRole("button", "#22c55e"))
	app.Press("Escape")
	app.Expect(telatest.ByRole("button", "#22c55e"), telatest.Expanded(false))
	// Output:
}
