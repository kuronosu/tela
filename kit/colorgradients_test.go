package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The color picker's gradients against Chromium's. A cell is the area or a
// track of the picker drawn from the classes of its recipe, with the colors
// the component paints (the hue's color under the area, the color's
// gradient over the alpha track), and the markup that gives Chromium the
// same classes and CSS gradients of the same colors. The picker itself has
// no shadcn markup to compare with (ADR-143), so this sheet checks what
// Tela's gradients and the repeating checkerboard get wrong, if anything.

func rgbCSS(c tela.Color, alpha float32) string {
	return fmt.Sprintf("rgb(%d %d %d / %g)", int(c.R*255+0.5), int(c.G*255+0.5), int(c.B*255+0.5), alpha)
}

func hueOf(c tela.Color) tela.Color {
	return aria.HSB{H: aria.ToHSB(c).H, S: 100, B: 100, A: 1}.Color()
}

func pickerAreaCell(name string, c tela.Color) cell {
	s := kit.DefaultStyle().ColorPicker
	bg := hueOf(c)
	return cell{name: name,
		tela: pickerBox{class: cn(s.Area, "w-64"), bg: bg, kids: []string{s.AreaSaturation, s.AreaBrightness}},
		html: fmt.Sprintf(`<div class=%q style="background-color: %s"><div class=%q></div><div class=%q></div></div>`,
			cn(s.Area, "w-64"), rgbCSS(bg, 1), s.AreaSaturation, s.AreaBrightness)}
}

func pickerHueCell() cell {
	s := kit.DefaultStyle().ColorPicker
	return cell{name: "hue-track", tela: pickerBox{class: s.Track + " w-64 " + s.HueTrack},
		// The recipe's class is Tela's bg-linear-[...]/srgb, which Tailwind has
		// no spelling for: Chromium gets the CSS it stands for.
		html: fmt.Sprintf(`<div class=%q style="background-image: linear-gradient(to right in srgb, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00)"></div>`, s.Track+" w-64")}
}

func pickerAlphaCell(name string, c tela.Color) cell {
	s := kit.DefaultStyle().ColorPicker
	class := s.Checkerboard + " " + s.Track + " " + s.AlphaTrack + " w-64"
	return cell{name: name, tela: pickerBox{class: class, alpha: &c},
		html: fmt.Sprintf(`<div class=%q><div class=%q style="background-image: linear-gradient(to right, %s, %s)"></div></div>`,
			class, s.AlphaFill, rgbCSS(c, 0), rgbCSS(c, 1))}
}

// pickerBox is one of the pieces above: a box with classes, a background
// color, children with classes, or the alpha fill of a color.
type pickerBox struct {
	class string
	bg    tela.Color
	kids  []string
	alpha *tela.Color
}

func (b pickerBox) Build() tela.Node {
	s := kit.DefaultStyle().ColorPicker
	var kids []tela.Child
	kids = append(kids, h.Class(b.class))
	if b.bg != (tela.Color{}) {
		kids = append(kids, h.Style(tela.Bg(b.bg)))
	}
	for _, k := range b.kids {
		kids = append(kids, h.Div(h.Class(k)))
	}
	if b.alpha != nil {
		clear, solid := *b.alpha, *b.alpha
		clear.A = 0
		kids = append(kids, h.Div(h.Class(s.AlphaFill),
			h.Style(tela.BackgroundGradient(tela.LinearGradient(90, tela.Stop(clear), tela.Stop(solid))))))
	}
	return h.Div(kids...)
}

func colorGradientSheet() *sheet {
	return &sheet{name: "colorgradients", width: 800, slot: 280, mean: [2]float64{0.5, 0.5}, cells: []cell{
		pickerAreaCell("area-indigo", indigo),
		pickerAreaCell("area-amber", tela.Hex("#f59e0b")),
		pickerHueCell(),
		pickerAlphaCell("alpha-indigo", indigo),
		pickerAlphaCell("alpha-red", tela.Hex("#ef4444")),
	}}
}

func TestColorGradientsGolden(t *testing.T)   { colorGradientSheet().golden(t) }
func TestChromiumColorGradients(t *testing.T) { colorGradientSheet().chromium(t) }
