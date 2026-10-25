package aria_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleParseHexColor() {
	c, err := aria.ParseHexColor("1E1E2E")
	fmt.Println(aria.FormatHexColor(c, false), err)
	_, err = aria.ParseHexColor("#12345")
	fmt.Println(err)
	// Output:
	// #1e1e2e <nil>
	// aria: invalid hex color "#12345": expected 3, 4, 6 or 8 hex digits (#rgb, #rgba, #rrggbb or #rrggbbaa), got 5
}

func ExampleToHSB() {
	hsb := aria.ToHSB(tela.Hex("#336699"))
	fmt.Printf("hue %.0f, saturation %.0f%%, brightness %.0f%%\n", hsb.H, hsb.S, hsb.B)
	fmt.Println(aria.FormatHexColor(aria.HSB{H: 210, S: 66.667, B: 60, A: 1}.Color(), false))
	// Output:
	// hue 210, saturation 67%, brightness 60%
	// #336699
}

func ExampleUseColorArea() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: aria.HSB{H: 220, S: 50, B: 50, A: 1}.Color()})
		area := aria.UseColorArea(aria.ColorAreaOptions{State: st, AriaLabel: "Color"})
		size := tela.UseSize(area.AreaRef)
		return Div(area.AreaProps, Class("relative size-48 rounded-md bg-muted"),
			Div(area.ThumbProps, Class("absolute left-0 top-0 -ml-2.5 -mt-2.5 size-5 rounded-full border-2 border-white"),
				Style(tela.TranslateFn(func() (x, y float32) {
					px, py := area.ThumbPercent()
					sz := size.Get()
					return float32(px) / 100 * sz.Width, float32(py) / 100 * sz.Height
				}))))
	}
	app := telatest.MountFunc(t, page)
	thumb := telatest.ByRole("slider", "Color")
	app.Focus(thumb)
	app.Press("Shift+ArrowRight") // a page of saturation
	app.Press("ArrowDown")
	fmt.Println(app.AriaSnapshot())
	app.Expect(thumb, telatest.HasValue("Saturation 60%, Brightness 49%"))
	// Output:
	// - group "Color":
	//   - slider "Color"
}

func ExampleUseColorSlider() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: aria.HSB{H: 220, S: 80, B: 90, A: 1}.Color()})
		hue := aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorHue})
		track := tela.UseSize(hue.TrackRef)
		return Div(hue.Props, Class("w-64 p-4"),
			Div(hue.TrackProps, Class("relative h-3 w-full rounded-full bg-muted"),
				Div(hue.ThumbProps, Class("absolute -top-1 -ml-2.5 size-5 rounded-full border-2 border-white"),
					Style(tela.TranslateFn(func() (x, y float32) {
						return float32(hue.ThumbPercent()) / 100 * track.Get().Width, 0
					})))))
	}
	app := telatest.MountFunc(t, page)
	thumb := telatest.ByRole("slider", "Hue")
	app.Focus(thumb)
	app.Press("PageUp") // a page of hue is 15 degrees
	fmt.Println(app.AriaSnapshot())
	app.Expect(thumb, telatest.HasValue("Hue 235°"))
	// Output:
	// - group "Hue":
	//   - slider "Hue"
}

func ExampleUseColorField() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: tela.Hex("#6366f1")})
		hex := aria.UseColorField(aria.ColorFieldOptions{State: st, AriaLabel: "Hex"})
		return Div(Class("p-4"),
			Input(hex.InputProps, Class("w-28 rounded-md border px-2 font-mono")),
			Span(Text("color "+aria.FormatHexColor(st.Color(), false))))
	}
	app := telatest.MountFunc(t, page)
	field := telatest.ByRole("textbox", "Hex")
	app.Focus(field)
	app.Press("Control+a")
	app.Type("ff8800\n") // Enter commits; so does leaving the field
	app.ExpectText("color #ff8800")
	app.Press("Control+a")
	app.Type("oops\n") // not a color: reverted
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - textbox "Hex": "#ff8800"
	// - text: "color #ff8800"
}
