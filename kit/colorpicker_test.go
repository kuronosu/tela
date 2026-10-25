package kit_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// There is no shadcn/ui color picker, so the picker has goldens and
// behavior tests and no Chromium page of its own; colorgradients_test.go
// compares its gradients with Chromium's (ADR-143).

var (
	indigo  = tela.Hex("#6366f1")
	presets = []tela.Color{tela.Hex("#ef4444"), tela.Hex("#f59e0b"), tela.Hex("#22c55e"), tela.Hex("#3b82f6"), tela.Hex("#a855f7"), tela.Hex("#000000"), tela.Hex("#ffffff")}
)

func colorPickerSheet() *sheet {
	return &sheet{name: "colorpicker", width: 800, cells: []cell{
		{name: "default", tela: kit.ColorPicker{DefaultValue: indigo}},
		{name: "alpha", tela: kit.ColorPicker{DefaultValue: tela.Hex("#f59e0b99"), Alpha: true, Swatches: presets}},
		{name: "gray", tela: kit.ColorPicker{DefaultValue: tela.Hex("#808080"), Swatches: presets}},
		{name: "selected", tela: kit.ColorPicker{DefaultValue: tela.Hex("#3b82f6"), Swatches: presets}},
		{name: "disabled", tela: kit.ColorPicker{DefaultValue: tela.Hex("#22c55e"), Alpha: true, Disabled: true}},
	}}
}

func TestColorPickerGolden(t *testing.T) { colorPickerSheet().golden(t) }

var (
	areaThumb  = telatest.ByRole("slider", "Color area")
	areaGroup  = telatest.ByRole("group", "Color area")
	hueThumb   = telatest.ByRole("slider", "Hue")
	hueGroup   = telatest.ByRole("group", "Hue")
	alphaThumb = telatest.ByRole("slider", "Alpha")
	alphaGroup = telatest.ByRole("group", "Alpha")
	hexField   = telatest.ByRole("textbox", "Hex")
)

// colorPage is a page with the picker, padded so that it sits at (16, 16).
func colorPage(p kit.ColorPicker) func() tela.Node {
	return func() tela.Node { return h.Div(h.Class("p-4"), p) }
}

// hexOf is the code of a color as the picker writes it.
func hexOf(c tela.Color, alpha bool) string { return aria.FormatHexColor(c, alpha) }

func hsb(hh, s, b float64) tela.Color { return aria.HSB{H: hh, S: s, B: b, A: 1}.Color() }

// colorLog records what a callback was called with.
type colorLog struct{ got []tela.Color }

func (l *colorLog) add(c tela.Color) { l.got = append(l.got, c) }
func (l *colorLog) last() tela.Color {
	if len(l.got) == 0 {
		return tela.Color{}
	}
	return l.got[len(l.got)-1]
}

// at is the point of a box at fractions fx, fy of its size.
func at(r tela.Rect, fx, fy float32) (float32, float32) {
	return r.X + r.Width*fx, r.Y + r.Height*fy
}

func TestColorPickerAreaMouse(t *testing.T) {
	var changed, committed colorLog
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: hsb(0, 100, 100),
		OnValueChange: changed.add, OnValueCommit: committed.add}), telatest.Theme(kit.Theme()))
	area := app.Bounds(areaGroup)
	if area.Width != 256 || area.Height != 160 {
		t.Fatalf("the area is %vx%v, want 256x160 (w-64 under the popover's p-4 and w-72)", area.Width, area.Height)
	}
	// A press moves the thumb to the point and goes on dragging it.
	app.MouseMove(at(area, 0.25, 0.5))
	app.MouseDown()
	app.Expect(areaThumb, telatest.HasValue("Saturation 25%, Brightness 50%"))
	app.MouseMove(at(area, 0.5, 0.25))
	app.MouseUp()
	app.Expect(areaThumb, telatest.HasValue("Saturation 50%, Brightness 75%"))
	want := hsb(0, 50, 75)
	app.Expect(hexField, telatest.HasValue(hexOf(want, false)))
	if len(committed.got) != 1 || hexOf(committed.last(), false) != hexOf(want, false) {
		t.Errorf("OnValueCommit got %v, want one call with %s", committed.got, hexOf(want, false))
	}
	if len(changed.got) < 2 || hexOf(changed.last(), false) != hexOf(want, false) {
		t.Errorf("OnValueChange got %d calls ending at %s, want it to end at %s", len(changed.got), hexOf(changed.last(), false), hexOf(want, false))
	}
	// The thumb is under the pointer: at 50% and 25% of the area.
	b := app.Bounds(areaThumb)
	cx, cy := b.X+b.Width/2, b.Y+b.Height/2
	wx, wy := at(area, 0.5, 0.25)
	if !near(cx, wx) || !near(cy, wy) {
		t.Errorf("the thumb is centered at (%v, %v), want (%v, %v)", cx, cy, wx, wy)
	}
	// Dragging out of the area keeps the drag and clamps at the edge.
	app.MouseMove(at(area, 0.5, 0.5))
	app.MouseDown()
	app.MouseMove(area.X+area.Width+80, area.Y-80)
	app.MouseUp()
	app.Expect(areaThumb, telatest.HasValue("Saturation 100%, Brightness 100%"))
}

func TestColorPickerHueAndAlphaMouse(t *testing.T) {
	var committed colorLog
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: hsb(30, 80, 90), Alpha: true, OnValueCommit: committed.add}))
	hue := app.Bounds(hueGroup)
	app.MouseMove(at(hue, 0.5, 0.5))
	app.MouseDown()
	app.MouseMove(at(hue, 0.75, 0.5))
	app.MouseUp()
	app.Expect(hueThumb, telatest.HasValue("Hue 270°"))
	// The area and the field follow the hue; saturation and brightness stay.
	app.Expect(areaThumb, telatest.HasValue("Saturation 80%, Brightness 90%"))
	app.Expect(hexField, telatest.HasValue(hexOf(hsb(270, 80, 90), true)))

	alpha := app.Bounds(alphaGroup)
	app.MouseMove(at(alpha, 0.2, 0.5))
	app.MouseDown()
	app.Expect(alphaThumb, telatest.HasValue("Alpha 20%"))
	app.MouseUp()
	// With Alpha the field has the opacity too.
	app.Expect(hexField, telatest.HasValue(hexOf(aria.HSB{H: 270, S: 80, B: 90, A: 0.2}.Color(), true)))
	if len(committed.got) != 2 {
		t.Errorf("OnValueCommit was called %d times, want 2 (hue, alpha)", len(committed.got))
	}
}

func TestColorPickerKeyboard(t *testing.T) {
	var committed colorLog
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: hsb(120, 50, 50), Alpha: true, OnValueCommit: committed.add}))
	app.Focus(areaThumb)
	app.Press("ArrowRight")
	app.Expect(areaThumb, telatest.HasValue("Saturation 51%, Brightness 50%"))
	app.Press("ArrowUp")
	app.Press("ArrowUp")
	app.Expect(areaThumb, telatest.HasValue("Saturation 51%, Brightness 52%"))
	app.Press("Shift+ArrowLeft")
	app.Expect(areaThumb, telatest.HasValue("Saturation 41%, Brightness 52%"))
	app.Press("PageUp") // a page of the Y channel
	app.Expect(areaThumb, telatest.HasValue("Saturation 41%, Brightness 62%"))
	app.Press("End") // a page of the X channel, as React Aria's area does
	app.Expect(areaThumb, telatest.HasValue("Saturation 51%, Brightness 62%"))
	app.Focus(hueThumb)
	app.Press("PageUp") // 15 degrees
	app.Expect(hueThumb, telatest.HasValue("Hue 135°"))
	app.Press("ArrowLeft")
	app.Expect(hueThumb, telatest.HasValue("Hue 134°"))
	app.Press("Home")
	app.Expect(hueThumb, telatest.HasValue("Hue 0°"))
	app.Focus(alphaThumb)
	app.Press("ArrowLeft")
	app.Expect(alphaThumb, telatest.HasValue("Alpha 99%"))
	app.Press("PageDown")
	app.Expect(alphaThumb, telatest.HasValue("Alpha 89%"))
	if len(committed.got) < 10 {
		t.Errorf("OnValueCommit was called %d times, want one for each key that changed the color", len(committed.got))
	}
}

func TestColorPickerHexField(t *testing.T) {
	var changed, committed colorLog
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: indigo, Alpha: true,
		OnValueChange: changed.add, OnValueCommit: committed.add}))
	app.Expect(hexField, telatest.HasValue("#6366f1ff"))
	app.Focus(hexField)
	app.Press("Control+a")
	app.Type("#f80\n") // three digits: the opacity stays
	app.Expect(hexField, telatest.HasValue("#ff8800ff"))
	app.Expect(areaThumb, telatest.HasValue("Saturation 100%, Brightness 100%"))
	app.Expect(hueThumb, telatest.HasValue("Hue 32°"))
	if len(committed.got) != 1 {
		t.Fatalf("OnValueCommit was called %d times after Enter, want 1", len(committed.got))
	}
	// Typing changes nothing until it is committed.
	app.Press("Control+a")
	app.Type("12")
	if len(changed.got) != 1 {
		t.Errorf("typing called OnValueChange (%d calls), want it to wait for Enter or blur", len(changed.got))
	}
	// A blur commits: eight digits set the opacity.
	app.Press("Control+a")
	app.Type("336699cc")
	app.Press("Tab")
	app.Expect(hexField, telatest.HasValue("#336699cc"))
	app.Expect(alphaThumb, telatest.HasValue("Alpha 80%"))
	if len(committed.got) != 2 {
		t.Errorf("OnValueCommit was called %d times after the blur, want 2", len(committed.got))
	}
	// Text that is not a color goes back to the color, and commits nothing.
	app.Focus(hexField)
	app.Press("Control+a")
	app.Type("12345\n")
	app.Expect(hexField, telatest.HasValue("#336699cc"))
	app.Press("Control+a")
	app.Press("Delete")
	app.Press("Enter")
	app.Expect(hexField, telatest.HasValue("#336699cc"))
	if len(committed.got) != 2 {
		t.Errorf("a refused code committed: %d calls, want 2", len(committed.got))
	}
}

func TestColorPickerSwatches(t *testing.T) {
	var committed colorLog
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: indigo, Swatches: presets, OnValueCommit: committed.add}))
	red := telatest.ByRole("button", "#ef4444")
	app.Expect(red, telatest.Pressed(false))
	app.Click(red)
	app.Expect(red, telatest.Pressed(true))
	app.Expect(hexField, telatest.HasValue("#ef4444"))
	if len(committed.got) != 1 || hexOf(committed.last(), false) != "#ef4444" {
		t.Errorf("OnValueCommit got %v, want #ef4444", committed.got)
	}
	// Black keeps the hue the picker had.
	app.Click(telatest.ByRole("button", "#000000"))
	app.Expect(hexField, telatest.HasValue("#000000"))
	app.Expect(hueThumb, telatest.HasValue("Hue 0°"))
	app.Click(red)
	app.Expect(red, telatest.Pressed(true))
	app.Click(red) // already the color: nothing to commit
	if len(committed.got) != 3 {
		t.Errorf("OnValueCommit was called %d times, want 3", len(committed.got))
	}
}

func TestColorPickerKeepsTheHue(t *testing.T) {
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: hsb(200, 70, 80)}))
	area := app.Bounds(areaGroup)
	// Down to black and across to the left edge (gray): the hue stays.
	app.MouseMove(at(area, 0.6, 0.9999))
	app.MouseDown()
	app.MouseUp()
	app.Expect(hexField, telatest.HasValue("#000000"))
	app.Expect(hueThumb, telatest.HasValue("Hue 200°"))
	app.MouseMove(at(area, 0, 0.5))
	app.MouseDown()
	app.MouseUp()
	app.Expect(hexField, telatest.HasValue("#808080"))
	app.Expect(hueThumb, telatest.HasValue("Hue 200°"))
	app.MouseMove(at(area, 0.6, 0.2))
	app.MouseDown()
	app.MouseUp()
	app.Expect(hueThumb, telatest.HasValue("Hue 200°"))
	app.Expect(areaThumb, telatest.HasValue("Saturation 60%, Brightness 80%"))
}

func TestColorPickerControlled(t *testing.T) {
	accept := true
	var asked colorLog
	app := telatest.MountFunc(t, func() tela.Node {
		c, setC := tela.UseState(hsb(0, 100, 100))
		return h.Div(h.Class("p-4"),
			kit.ColorPicker{Value: &c, OnValueChange: func(n tela.Color) {
				asked.add(n)
				if accept {
					setC.Set(n)
				}
			}},
			h.P(h.Text("color "+hexOf(c, false))),
			h.Button(h.Text("Green"), h.OnClick(func() { setC.Set(tela.Hex("#00ff00")) })))
	})
	app.ExpectText("color #ff0000")
	app.Focus(hueThumb)
	app.Press("Shift+ArrowRight") // Shift is a page of hue: 15 degrees
	app.ExpectText("color " + hexOf(hsb(15, 100, 100), false))
	// A change the caller refuses leaves the picker where it was.
	accept = false
	app.Press("ArrowRight")
	app.Expect(hueThumb, telatest.HasValue("Hue 15°"))
	if hexOf(asked.last(), false) != hexOf(hsb(16, 100, 100), false) {
		t.Errorf("the refused color was %s, want hue 16", hexOf(asked.last(), false))
	}
	// The caller can set the color: the picker shows it.
	app.Click(telatest.ByRole("button", "Green"))
	app.Expect(hexField, telatest.HasValue("#00ff00"))
	app.Expect(hueThumb, telatest.HasValue("Hue 120°"))
}

func TestColorPickerZeroDefaultIsBlack(t *testing.T) {
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{Alpha: true}))
	app.Expect(hexField, telatest.HasValue("#000000ff"))
}

func TestColorPickerDisabled(t *testing.T) {
	var changed colorLog
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: indigo, Alpha: true, Swatches: presets, Disabled: true, OnValueChange: changed.add}))
	area := app.Bounds(areaGroup)
	app.MouseMove(at(area, 0.1, 0.9))
	app.MouseDown()
	app.MouseUp()
	app.Expect(telatest.ByRole("button", "#ef4444"), telatest.Disabled(true))
	app.Expect(areaThumb, telatest.Disabled(true))
	app.Expect(hueThumb, telatest.Disabled(true))
	app.Expect(alphaThumb, telatest.Disabled(true))
	app.Expect(hexField, telatest.Disabled(true), telatest.HasValue("#6366f1ff"))
	if len(changed.got) != 0 {
		t.Errorf("a disabled picker changed %d times", len(changed.got))
	}
}

func TestColorPickerLabels(t *testing.T) {
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{Alpha: true, AreaLabel: "Couleur", HueLabel: "Teinte", AlphaLabel: "Opacité", HexLabel: "Code"}))
	app.ExpectAria(`
- group "Couleur":
  - slider "Couleur"
- group "Teinte":
  - slider "Teinte"
- group "Opacité":
  - slider "Opacité"
- textbox "Code": "#000000ff"
`)
}

func TestColorPickerAriaSnapshot(t *testing.T) {
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: indigo, Swatches: presets[:2]}))
	app.ExpectAria(`
- group "Color area":
  - slider "Color area"
- group "Hue":
  - slider "Hue"
- textbox "Hex": "#6366f1"
- button "#ef4444"
- button "#f59e0b"
`)
}

// Dragging repaints: the picker builds when a drag starts and ends, not on
// every move (ADR-132, ADR-135).
func TestColorPickerDragRepaintsOnly(t *testing.T) {
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: hsb(30, 50, 50), Alpha: true}))
	for _, g := range []struct {
		name string
		loc  telatest.Locator
		move func(r tela.Rect, i int) (float32, float32)
	}{
		{"area", areaGroup, func(r tela.Rect, i int) (float32, float32) {
			return r.X + 10 + float32(i)*10, r.Y + 20 + float32(i)*5
		}},
		{"hue", hueGroup, func(r tela.Rect, i int) (float32, float32) { return r.X + 10 + float32(i)*10, r.Y + r.Height/2 }},
		{"alpha", alphaGroup, func(r tela.Rect, i int) (float32, float32) { return r.X + 10 + float32(i)*10, r.Y + r.Height/2 }},
	} {
		r := app.Bounds(g.loc)
		x, y := g.move(r, 0)
		app.MouseMove(x, y)
		app.MouseDown()
		st := app.Measure(func() {
			for i := 1; i <= 20; i++ {
				app.MouseMove(g.move(r, i))
			}
		})
		t.Logf("%s: 20 moves of one drag: %d builds, %d layouts, %d paints (%d frames)", g.name, st.Builds, st.Layouts, st.Paints, st.Frames)
		if st.Builds > 0 {
			t.Errorf("%s: %d builds while dragging, want none", g.name, st.Builds)
		}
		if st.Layouts > 0 {
			t.Errorf("%s: %d layouts while dragging, want none", g.name, st.Layouts)
		}
		end := app.Measure(func() { app.MouseUp() })
		t.Logf("%s: the drag's end: %d builds", g.name, end.Builds)
	}
}

func TestColorPickerIdle(t *testing.T) {
	app := telatest.MountFunc(t, colorPage(kit.ColorPicker{DefaultValue: indigo, Alpha: true, Swatches: presets}), telatest.NoSettle())
	area := app.Bounds(areaGroup)
	app.MouseMove(at(area, 0.3, 0.3))
	app.MouseDown()
	app.MouseMove(at(area, 0.4, 0.4))
	app.MouseUp()
	app.Advance(3 * time.Second)
	before := app.Stats()
	app.Advance(2 * time.Second)
	if after := app.Stats(); after.Frames != before.Frames {
		t.Errorf("an idle picker still requests frames: %d in 2 s", after.Frames-before.Frames)
	}
}

func ExampleColorPicker() {
	t := &exampleT{}
	defer t.close()
	var saved []string
	page := func() tela.Node {
		c, setC := tela.UseState(tela.Hex("#6366f1"))
		return h.Div(h.Class("p-4"),
			kit.ColorPicker{Value: &c, OnValueChange: setC.Set, Alpha: true,
				Swatches:      []tela.Color{tela.Hex("#ef4444"), tela.Hex("#22c55e")},
				OnValueCommit: func(c tela.Color) { saved = append(saved, aria.FormatHexColor(c, true)) }},
			h.P(h.Textf("Chosen: %s", aria.FormatHexColor(c, false))))
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "#22c55e"))
	app.Focus(telatest.ByRole("slider", "Alpha"))
	app.Press("PageDown")
	app.ExpectText("Chosen: #22c55e")
	fmt.Println(strings.Join(saved, " "))
	// Output: #22c55eff #22c55ee5
}
