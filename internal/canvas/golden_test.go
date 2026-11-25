package canvas

import (
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/internal/testutil/golden"
	"github.com/kuronosu/tela/internal/text"
)

var testFonts = stdfonts.NewCollection()

// textStyle is the style of the text of the tests.
var textStyle = text.TextStyle{Families: []string{"Inter", "sans-serif"}, Size: 16, Weight: 400, Stretch: 100}

// textFrame is frameOf with fonts.
func textFrame(w, h float64, dir text.Direction, scale float64) *Frame {
	f := frameOf(w, h)
	f.Fonts, f.Text, f.Dir, f.Scale = testFonts, &textStyle, dir, scale
	f.Font = CanvasFont{Family: JoinFamilies(textStyle.Families), Size: 16, Weight: 400}
	return f
}

// renderScene returns the pixels of a scene on white.
func renderScene(s *scene, scale float32) *image.RGBA {
	var r Recorder
	list, _ := r.Record(textFrame(s.w, s.h, s.dir, float64(scale)), func(c *Canvas) { s.draw(&pen{Canvas: c}) })
	b := gfx.NewBuilder()
	b.FillRect(gfx.Rect{X1: float32(s.w), Y1: float32(s.h)}, gfx.White)
	b.AppendList(list, nil)
	img := image.NewRGBA(image.Rect(0, 0, int(float32(s.w)*scale), int(float32(s.h)*scale)))
	raster.Render(img, b.Finish(), raster.Options{Scale: scale})
	return img
}

// Regenerate with TELA_UPDATE=1 and look at them.
func TestGoldens(t *testing.T) {
	for _, s := range scenes() {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s_x%g", s.name, scale), func(t *testing.T) {
				golden.Image(t, fmt.Sprintf("%s_x%g", s.name, scale), renderScene(&s, scale))
			})
		}
	}
}
