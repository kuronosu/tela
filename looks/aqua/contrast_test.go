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

// textContrast measures the pixels of a control over the sheet's saturated
// backdrop: the most common color in the middle of the control is its fill
// (the glass), the pixel farthest from it in luminance is the text, and the
// pair is the contrast ratio. The sheet is the worst case: glass over a
// saturated sky, where it is a mid-tone.
func textContrast(t *testing.T, a *telatest.App, l telatest.Locator) (float64, color.RGBA) {
	t.Helper()
	r := a.Bounds(l)
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
	return worst, bg
}

// everyLook is the combinations of the designs the contrast checks cover: both
// designs, Tahoe's glass cards, and the mixes of controls.
func everyLook() []looks.Look {
	return []looks.Look{
		aqua.Look(aqua.Tahoe),
		aqua.Look(aqua.Tahoe, aqua.GlassContent()),
		aqua.Look(aqua.Sequoia),
		aqua.Look(aqua.Sequoia, aqua.Controls(aqua.Tahoe)),
		aqua.Look(aqua.Tahoe, aqua.Controls(aqua.Sequoia)),
	}
}

var schemes = []struct {
	name   string
	scheme tela.ColorScheme
}{{"light", tela.Light}, {"dark", tela.Dark}}

// TestDestructiveTextContrast: the red text of the Delete button passes WCAG
// AA for body text (4.5:1) over its fill in every design and scheme: the
// token destructive-text is a darker red in light, a lighter one in dark,
// where the button also takes a black tint over the saturated backdrop.
func TestDestructiveTextContrast(t *testing.T) {
	for _, l := range everyLook() {
		for _, s := range schemes {
			t.Run(l.Name+"/"+s.name, func(t *testing.T) {
				a := mountSheet(t, l, s.scheme)
				worst, bg := textContrast(t, a, telatest.ByRole("button", "Delete"))
				t.Logf("Delete: text over #%02x%02x%02x at %.2f:1", bg.R, bg.G, bg.B, worst)
				if worst < 4.5 {
					t.Errorf("Delete's text is %.2f:1 over its fill #%02x%02x%02x; WCAG AA needs 4.5:1", worst, bg.R, bg.G, bg.B)
				}
			})
		}
	}
}

// TestGlassControlsContrast: in Tahoe's dark the white text of the other glass
// controls (Cancel, an Outline toggle, a pressed toggle) reads at 4.5:1 too:
// the glass is tinted dark there instead of white, and the selected capsule is
// a mid gray.
func TestGlassControlsContrast(t *testing.T) {
	for _, l := range []looks.Look{
		aqua.Look(aqua.Tahoe),
		aqua.Look(aqua.Tahoe, aqua.GlassContent()),
		aqua.Look(aqua.Sequoia, aqua.Controls(aqua.Tahoe)),
	} {
		t.Run(l.Name, func(t *testing.T) {
			a := mountSheet(t, l, tela.Dark)
			for _, name := range []string{"Cancel", "Outline", "Pressed"} {
				worst, bg := textContrast(t, a, telatest.ByRole("button", name))
				t.Logf("%s: text over #%02x%02x%02x at %.2f:1", name, bg.R, bg.G, bg.B, worst)
				if worst < 4.5 {
					t.Errorf("%s's text is %.2f:1 over its glass #%02x%02x%02x; WCAG AA needs 4.5:1", name, worst, bg.R, bg.G, bg.B)
				}
			}
		})
	}
}
