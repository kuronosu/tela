package aria_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

func hueThumb() telatest.Locator   { return telatest.ByRole("slider", "Hue") }
func alphaThumb() telatest.Locator { return telatest.ByRole("slider", "Alpha") }

func TestColorSliderSnapshotAndValueText(t *testing.T) {
	p := picker{state: aria.ColorStateOptions{DefaultValue: new(tela.Hex("#336699").WithAlpha(0.8))}, parts: "hl"}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.ExpectAria(`
		- group "Hue":
		  - slider "Hue"
		- group "Alpha":
		  - slider "Alpha"
		- text: "color #336699cc"
	`)
	app.Expect(hueThumb(), telatest.HasValue("Hue 210°"))
	app.Expect(alphaThumb(), telatest.HasValue("Alpha 80%"))
}

func TestColorSliderKeyboard(t *testing.T) {
	var ends events
	p := picker{
		state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 100, S: 80, B: 80, A: 0.5}.Color())},
		hue:   aria.ColorSliderOptions{OnChangeEnd: func(c tela.Color) { ends.add(fmt.Sprintf("%.0f", aria.ToHSB(c).H)) }},
		parts: "hl",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	hue, alpha := hueThumb(), alphaThumb()
	app.Focus(hue)
	app.Press("ArrowRight")
	app.Expect(hue, telatest.HasValue("Hue 101°"))
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Expect(hue, telatest.HasValue("Hue 99°"))
	app.Press("Shift+ArrowRight") // a page of hue is 15 degrees
	app.Expect(hue, telatest.HasValue("Hue 114°"))
	app.Press("PageUp")
	app.Expect(hue, telatest.HasValue("Hue 129°"))
	app.Press("PageDown")
	app.Press("PageDown")
	app.Expect(hue, telatest.HasValue("Hue 99°"))
	app.Press("End")
	app.Expect(hue, telatest.HasValue("Hue 360°"))
	app.Press("Home")
	app.Expect(hue, telatest.HasValue("Hue 0°"))
	if got := ends.take(); got != "101 100 99 114 129 114 99 0 0" {
		// Hue 360 is reported as the color of hue 0.
		t.Errorf("OnChangeEnd: %s", got)
	}
	// The other channels did not change.
	app.ExpectText("color #cc292980")
	app.Focus(alpha)
	app.Press("ArrowRight")
	app.Expect(alpha, telatest.HasValue("Alpha 51%"))
	app.Press("PageUp") // a page of alpha is 0.1
	app.Expect(alpha, telatest.HasValue("Alpha 61%"))
	app.Press("End")
	app.Expect(alpha, telatest.HasValue("Alpha 100%"))
	app.Press("Home")
	app.Expect(alpha, telatest.HasValue("Alpha 0%"))
}

func TestColorSliderPointer(t *testing.T) {
	p := picker{state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 0, S: 100, B: 100, A: 1}.Color())}, parts: "hl"}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	hue, alpha := hueThumb(), alphaThumb()
	app.ClickAt(16+0.5*areaLen, hueY)
	app.Expect(hue, telatest.HasValue("Hue 180°"))
	app.ExpectFocused(hue)
	app.ClickAt(16+0.25*areaLen, alphaY)
	app.Expect(alpha, telatest.HasValue("Alpha 25%"))
	// Dragging the thumb past the ends clamps.
	x, y := center(app, hue)
	app.MouseMove(x, y)
	app.MouseDown()
	app.MouseMove(x+30, y+40)
	app.MouseMove(900, y)
	app.Expect(hue, telatest.HasValue("Hue 360°"))
	app.MouseMove(-90, y)
	app.Expect(hue, telatest.HasValue("Hue 0°"))
	app.MouseUp()
	app.ExpectText("color #ff000040")
}

func TestColorSliderTouch(t *testing.T) {
	for _, kind := range []platform.PointerKind{platform.PointerTouch, platform.PointerPen} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			p := picker{state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 90, S: 100, B: 100, A: 1}.Color())}, parts: "h"}
			a := mountRaw(t, tela.C(p.page()), 232, 400)
			value := func() string { return findNode(a.tree.Semantics(), "slider", "Hue").Value }
			x := float32(16 + 0.25*areaLen) // hue 90
			a.down(kind, x, hueY)
			a.move(kind, x+50, hueY+30)
			a.up(kind, x+50, hueY+30)
			if got := value(); got != "Hue 180°" {
				t.Errorf("after the drag: %q", got)
			}
		})
	}
}

// Hue and saturation sliders keep working at the gray axis: the hue the
// slider shows is the hue kept, not 0.
func TestColorSliderKeepsTheHueAtGray(t *testing.T) {
	p := picker{state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 200, S: 100, B: 100, A: 1}.Color())}, parts: "ah"}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	x, y := areaPoint(0, 60)
	app.ClickAt(x, y) // gray
	app.Expect(hueThumb(), telatest.HasValue("Hue 200°"))
	app.ClickAt(16+0.5*areaLen, hueY) // a gray has no color for the hue to change
	app.Expect(hueThumb(), telatest.HasValue("Hue 180°"))
	app.ExpectText("color #999999ff")
	x, y = areaPoint(99, 99)
	app.ClickAt(x, y)
	app.ExpectText("color #03fcfcff")
}

// The saturation, brightness and lightness sliders.
func TestColorSliderOtherChannels(t *testing.T) {
	var sat, bri, lit aria.ColorSliderResult
	var stops []tela.Color
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 0, S: 100, B: 100, A: 1}.Color())})
		sat = aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorSaturation})
		bri = aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorBrightness})
		lit = aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorLightness, Space: "hsl"})
		return Div(Class("p-4"),
			Div(sat.TrackProps, Class("h-3 w-[200px]"), Div(sat.ThumbProps, Class("size-5"))),
			Div(bri.TrackProps, Class("h-3 w-[200px]"), Div(bri.ThumbProps, Class("size-5"))),
			Div(lit.TrackProps, Class("h-3 w-[200px]"), Div(lit.ThumbProps, Class("size-5"))),
		)
	}
	app := telatest.MountFunc(t, page, telatest.Size(232, 200))
	s, b, l := telatest.ByRole("slider", "Saturation"), telatest.ByRole("slider", "Brightness"), telatest.ByRole("slider", "Lightness")
	app.Expect(s, telatest.HasValue("Saturation 100%"))
	app.Expect(b, telatest.HasValue("Brightness 100%"))
	app.Expect(l, telatest.HasValue("Lightness 50%"))
	app.Focus(b)
	app.Press("Shift+ArrowLeft")
	app.Expect(b, telatest.HasValue("Brightness 90%"))
	app.Expect(l, telatest.HasValue("Lightness 45%")) // hsb(0 100 90) is hsl(0 100 45)
	app.Focus(l)
	app.Press("Home")
	app.Expect(l, telatest.HasValue("Lightness 0%"))
	app.Press("ArrowRight")
	// The HSL saturation was kept at lightness 0: the color is pure red
	// again at 1% lightness.
	app.Expect(l, telatest.HasValue("Lightness 1%"))
	app.Press("End")
	app.Expect(l, telatest.HasValue("Lightness 100%"))
	app.Press("PageDown")
	app.Press("PageDown")
	app.Expect(l, telatest.HasValue("Lightness 80%"))
	if got := aria.FormatHexColor(aria.HSL{H: 0, S: 100, L: 80, A: 1}.Color(), false); got != "#ff9999" {
		t.Errorf("hsl(0 100 80) is %s", got)
	}
	// The saturation slider is the HSB's here.
	app.Expect(s, telatest.HasValue("Saturation 40%"))
	// Track colors, from the minimum to the maximum.
	stops = sat.TrackColors()
	if len(stops) != 2 {
		t.Fatalf("saturation track: %d stops", len(stops))
	}
	if got := aria.FormatHexColor(stops[0], false); got != "#ffffff" {
		t.Errorf("saturation track starts at %s", got)
	}
	if got := aria.FormatHexColor(stops[1], false); got != "#ff0000" {
		t.Errorf("saturation track ends at %s", got)
	}
	if bs := bri.TrackColors(); len(bs) != 2 || bs[0] != (tela.Color{A: 1}) {
		t.Errorf("brightness track: %v", bs)
	}
	if ls := lit.TrackColors(); len(ls) != 3 || aria.FormatHexColor(ls[0], false) != "#000000" || aria.FormatHexColor(ls[2], false) != "#ffffff" {
		t.Errorf("lightness track: %v", ls)
	}
}

func TestColorSliderTrackColors(t *testing.T) {
	var hue, alpha aria.ColorSliderResult
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: new(tela.Hex("#336699").WithAlpha(0.5))})
		hue = aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorHue})
		alpha = aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorAlpha})
		return Div(Class("p-4"), Div(hue.ThumbProps, Class("size-5")), Div(alpha.ThumbProps, Class("size-5")))
	}
	telatest.MountFunc(t, page, telatest.Size(100, 100))
	if hue.Channel != aria.ColorHue || hue.Min != 0 || hue.Max != 360 || hue.Step != 1 {
		t.Errorf("hue range: %+v", hue)
	}
	if alpha.Min != 0 || alpha.Max != 1 || alpha.Step != 0.01 {
		t.Errorf("alpha range: %g %g %g", alpha.Min, alpha.Max, alpha.Step)
	}
	hs := hue.TrackColors()
	if len(hs) != 7 {
		t.Fatalf("hue track: %d stops", len(hs))
	}
	// The hue stops keep the saturation and the brightness of the color.
	for i, c := range hs {
		got := aria.ToHSB(c)
		if !closeTo(got.S, 66.667, 0.05) || !closeTo(got.B, 60, 0.05) || !closeTo(got.H, float64(i*60%360), 0.05) {
			t.Errorf("hue stop %d: %+v", i, got)
		}
		if c.A != 0.5 {
			t.Errorf("hue stop %d has alpha %g, want the color's 0.5", i, c.A)
		}
	}
	as := alpha.TrackColors()
	if len(as) != 2 || as[0].A != 0 || as[1].A != 1 || aria.FormatHexColor(as[1], false) != "#336699" {
		t.Errorf("alpha track: %v", as)
	}
}

func TestColorSliderControlled(t *testing.T) {
	var set tela.Setter[tela.Color]
	page := func() tela.Node {
		c, s := tela.UseState(aria.HSB{H: 0, S: 100, B: 100, A: 1}.Color())
		set = s
		return picker{state: aria.ColorStateOptions{Value: &c, OnChange: s.Set}, parts: "h"}.usePicker()
	}
	app := telatest.MountFunc(t, page, telatest.Size(232, 400))
	hue := hueThumb()
	app.Focus(hue)
	app.Press("ArrowRight")
	app.Expect(hue, telatest.HasValue("Hue 1°"))
	app.ClickAt(16+0.5*areaLen, hueY)
	app.Expect(hue, telatest.HasValue("Hue 180°"))
	set.Set(tela.Hex("#0000ff"))
	app.Expect(hue, telatest.HasValue("Hue 240°"))
}

func TestColorSliderDisabledAndVertical(t *testing.T) {
	var changes int
	p := picker{
		state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 100, S: 100, B: 100, A: 1}.Color()), OnChange: func(tela.Color) { changes++ }},
		hue:   aria.ColorSliderOptions{Disabled: true}, parts: "h",
	}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Expect(hueThumb(), telatest.Disabled(true))
	app.ClickAt(16+0.8*areaLen, hueY)
	if changes != 0 {
		t.Errorf("%d changes of a disabled slider", changes)
	}

	vertical := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 100, S: 100, B: 100, A: 1}.Color())})
		hue := aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorHue, Orientation: "vertical"})
		return Div(Class("p-4"), Div(hue.TrackProps, Class("h-[200px] w-3"), Div(hue.ThumbProps, Class("size-5"))))
	}
	app = telatest.MountFunc(t, vertical, telatest.Size(100, 232))
	app.ClickAt(22, 16+0.25*areaLen) // the minimum is at the bottom
	app.Expect(hueThumb(), telatest.HasValue("Hue 270°"))
	app.Focus(hueThumb())
	app.Press("ArrowUp")
	app.Expect(hueThumb(), telatest.HasValue("Hue 271°"))
}

func TestColorSliderCustomText(t *testing.T) {
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 90, S: 100, B: 100, A: 1}.Color())})
		hue := aria.UseColorSlider(aria.ColorSliderOptions{
			State: st, Channel: aria.ColorHue, AriaLabel: "Matiz",
			FormatValue: func(ch aria.ColorChannel, v float64) string { return fmt.Sprintf("matiz %g grados", v) },
		})
		return Div(Class("p-4"), Div(hue.ThumbProps, Class("size-5")), Text(hue.ValueText()))
	}
	app := telatest.MountFunc(t, page, telatest.Size(200, 100))
	app.Expect(telatest.ByRole("slider", "Matiz"), telatest.HasValue("matiz 90 grados"))
	app.ExpectText("matiz 90 grados")
}

// A slider is named by a visible label.
func TestColorSliderLabelledBy(t *testing.T) {
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: new(tela.Black)})
		label := tela.UseElementRef()
		a := aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorAlpha, AriaLabelledBy: label})
		return Div(a.Props, Span(Ref(label), Text("Opacity")), Div(a.TrackProps, Class("w-[200px]"), Div(a.ThumbProps, Class("size-5"))))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.ExpectRole("slider", "Opacity")
	app.ExpectRole("group", "Opacity")
}

// Dragging a slider repaints only, and so does a drag of the area for the
// slider that follows the color.
func TestColorSliderDragDoesNotBuildPerMove(t *testing.T) {
	var builds int
	p := picker{state: aria.ColorStateOptions{DefaultValue: new(aria.HSB{H: 0, S: 100, B: 100, A: 1}.Color())}, parts: "ahl", builds: &builds}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	hue := hueThumb()
	start := builds
	x, y := center(app, hue)
	app.MouseMove(x, y)
	app.MouseDown()
	for i := 1; i <= 20; i++ {
		app.MouseMove(x+float32(i)*5, y)
		if gx, _ := center(app, hue); gx < x+float32(i)*5-1.5 || gx > x+float32(i)*5+1.5 {
			t.Fatalf("move %d: the thumb is centered at %g, want %g", i, gx, x+float32(i)*5)
		}
	}
	if during := builds - start; during > 1 {
		t.Errorf("%d builds while dragging, want the one of the drag start", during)
	}
	app.MouseUp()
	app.Expect(hue, telatest.HasValue("Hue 180°"))
	if total := builds - start; total > 2 {
		t.Errorf("%d builds for the whole drag, want 2", total)
	}
	// The area's thumb is where it was while the hue moved (the hue is
	// not its channel), and a drag of the area moves the sliders' state.
	th := areaThumb()
	app.Expect(th, telatest.HasValue("Saturation 100%, Brightness 100%"))
}

func TestColorSliderOptionErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		o    aria.ColorSliderOptions
		want []string
	}{
		"channel":    {aria.ColorSliderOptions{}, []string{"Channel is required", `"hue"`}},
		"typo":       {aria.ColorSliderOptions{Channel: "hu"}, []string{"Channel", `did you mean "hue"`}},
		"space":      {aria.ColorSliderOptions{Channel: aria.ColorLightness}, []string{"lightness", `Space "hsb"`, `"hsl"`}},
		"hslbright":  {aria.ColorSliderOptions{Channel: aria.ColorBrightness, Space: "hsl"}, []string{"brightness", `Space "hsl"`}},
		"space name": {aria.ColorSliderOptions{Channel: aria.ColorHue, Space: "rgb"}, []string{"Space", `"rgb"`}},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				msg := fmt.Sprint(recover())
				for _, w := range tc.want {
					if !strings.Contains(msg, w) {
						t.Errorf("message %q lacks %q", msg, w)
					}
				}
			}()
			telatest.MountFunc(t, func() tela.Node {
				tc.o.State = aria.UseColorState(aria.ColorStateOptions{})
				s := aria.UseColorSlider(tc.o)
				return Div(s.TrackProps)
			}, telatest.Size(100, 100))
		})
	}
	defer func() {
		if msg := fmt.Sprint(recover()); !strings.Contains(msg, "State is required") {
			t.Errorf("message %q", msg)
		}
	}()
	telatest.MountFunc(t, func() tela.Node {
		s := aria.UseColorSlider(aria.ColorSliderOptions{Channel: aria.ColorHue})
		return Div(s.TrackProps)
	}, telatest.Size(100, 100))
}
