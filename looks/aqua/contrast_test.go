package aqua_test

import (
	"image/color"
	"math"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/aqua"
	"github.com/kuronosu/tela/telatest"
)

// relLuminance is the WCAG 2 relative luminance of an sRGB color.
func relLuminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// TestDestructiveTextContrast measures the pixels of the Delete button over
// the sheet's saturated backdrop: the most common color is the glass, the
// pixel farthest from it in luminance is the text, and the pair must pass
// WCAG AA for body text (4.5:1) in Tahoe's controls in dark.
func TestDestructiveTextContrast(t *testing.T) {
	for _, l := range []looks.Look{
		aqua.Look(aqua.Tahoe),
		aqua.Look(aqua.Tahoe, aqua.GlassContent()),
		aqua.Look(aqua.Sequoia, aqua.Controls(aqua.Tahoe)),
	} {
		t.Run(l.Name, func(t *testing.T) {
			a := mountSheet(t, l, tela.Dark)
			r := a.Bounds(telatest.ByRole("button", "Delete"))
			img := a.Screenshot()
			sc := float64(img.Bounds().Dx()) / 900
			count := map[color.RGBA]int{}
			for y := int(float64(r.Y+r.Height*0.25) * sc); y < int(float64(r.Y+r.Height*0.75)*sc); y++ {
				for x := int(float64(r.X+r.Width*0.2) * sc); x < int(float64(r.X+r.Width*0.8)*sc); x++ {
					count[img.RGBAAt(x, y)]++
				}
			}
			var bg color.RGBA
			best := 0
			for c, n := range count {
				if n > best {
					bg, best = c, n
				}
			}
			bl, worst := relLuminance(bg), 0.0
			for c := range count {
				hi, lo := math.Max(relLuminance(c), bl), math.Min(relLuminance(c), bl)
				worst = math.Max(worst, (hi+0.05)/(lo+0.05))
			}
			t.Logf("Delete: text over #%02x%02x%02x at %.2f:1", bg.R, bg.G, bg.B, worst)
			if worst < 4.5 {
				t.Errorf("Delete's text is %.2f:1 over its glass #%02x%02x%02x; WCAG AA needs 4.5:1", worst, bg.R, bg.G, bg.B)
			}
		})
	}
}
