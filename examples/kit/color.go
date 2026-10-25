package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of the color controls. The look of both is the kit's own:
// shadcn/ui has no color picker.

// brand are the swatches of the demos.
var brand = []tela.Color{
	tela.Hex("#ef4444"), tela.Hex("#f97316"), tela.Hex("#eab308"), tela.Hex("#22c55e"),
	tela.Hex("#06b6d4"), tela.Hex("#3b82f6"), tela.Hex("#8b5cf6"), tela.Hex("#ec4899"),
}

// hexOf writes a color as #rrggbb, or #rrggbbaa when it is not opaque.
func hexOf(c tela.Color) string {
	b := func(v float32) int { return int(max(0, min(1, v))*255 + 0.5) }
	if c.A < 1 {
		return fmt.Sprintf("#%02x%02x%02x%02x", b(c.R), b(c.G), b(c.B), b(c.A))
	}
	return fmt.Sprintf("#%02x%02x%02x", b(c.R), b(c.G), b(c.B))
}

// chip is a rounded rectangle of a color.
func chip(c tela.Color, class string) tela.Node {
	return Div(Class("rounded-md border border-border", class), Style(tela.Bg(c)))
}

func colorPickerPage() tela.Node {
	accent, setAccent := tela.UseState(tela.Hex("#6366f1"))
	overlay, setOverlay := tela.UseState(tela.Hex("#3b82f6").WithAlpha(0.6))
	return grid(
		demo("Basic", "An area for saturation and brightness, a hue slider and a hex field. The arrow keys move a thumb (Shift or Page Up and Down move it further).", rowTop,
			kit.ColorPicker{Value: &accent, OnValueChange: setAccent.Set},
			Div(Class("flex flex-col gap-3"),
				chip(accent, "h-20 w-40"),
				Span(Class("font-mono text-sm"), Text(hexOf(accent))))),
		demo("Alpha and swatches", "Alpha adds the opacity slider and lets the hex field take #rrggbbaa; Swatches are preset colors.", rowTop,
			kit.ColorPicker{Value: &overlay, OnValueChange: setOverlay.Set, Alpha: true, Swatches: brand},
			Div(Class("flex flex-col gap-3"),
				Div(Class("flex h-20 w-40 items-center justify-center rounded-md border border-border bg-muted text-sm"),
					Div(Class("flex size-12 items-center justify-center rounded-full"), Style(tela.Bg(overlay)))),
				Span(Class("font-mono text-sm"), Text(hexOf(overlay))))),
		demo("Disabled", "", "",
			kit.ColorPicker{DefaultValue: tela.Hex("#22c55e"), Disabled: true}),
	)
}

func colorInputPage() tela.Node {
	fill, setFill := tela.UseState(tela.Hex("#6366f1"))
	saved, setSaved := tela.UseState("")
	return grid(
		demo("Basic", "The same picker behind a button that shows the color. The color lives in the ColorInput, so the swatch follows a drag in the panel.", colStart,
			kit.ColorInput{DefaultValue: tela.Hex("#6366f1"), OnValueCommit: func(c tela.Color) { setSaved.Set("Saved " + hexOf(c)) }},
			readout(map[bool]string{true: "Nothing saved yet.", false: saved}[saved == ""])),
		demo("With alpha and swatches", "", "",
			kit.ColorInput{DefaultValue: tela.Hex("#ec4899").WithAlpha(0.8), Alpha: true, Swatches: brand}),
		demo("As a form row", "Draw the caption yourself: a Field's label points to a control by id, which ColorInput does not take yet.", col,
			Div(Class("flex w-full max-w-xs flex-col gap-2"),
				Span(Class("text-sm leading-none font-medium"), Text("Brand color")),
				Div(Class("flex items-center gap-3"),
					kit.ColorInput{Value: &fill, OnValueChange: setFill.Set, Swatches: brand},
					chip(fill, "size-9")),
				muted("Used for buttons and links."))),
		demo("Disabled", "", "",
			kit.ColorInput{DefaultValue: tela.Hex("#22c55e"), Disabled: true}),
	)
}
