package text

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// scene is a test drawing of laid-out text for one display scale. Its
// layouts are laid out at that scale (Paragraph.Scale) and painted for it
// (PaintScaled), as core lays out and paints a window's text: line boxes
// are quantized in device pixels (ADR-159), so a layout made at scale 1
// and drawn at 2 puts its baselines up to a device pixel away from where
// the same text at x2 goes, and the x2 golden would not show Tela's x2
// text.
type scene struct {
	b       *gfx.Builder
	scale   float32
	layouts []*Layout
}

func newScene(scale float32) *scene { return &scene{b: gfx.NewBuilder(), scale: scale} }

// layout lays p out in c for the scene's scale.
func (s *scene) layout(c *Collection, p *Paragraph) *Layout {
	p.Scale = s.scale
	l := NewLayout(c, p)
	s.layouts = append(s.layouts, l)
	return l
}

// paint paints l with its top-left corner at origin, for the scene's
// scale.
func (s *scene) paint(l *Layout, origin gfx.Point) { l.PaintScaled(s.b, origin, s.scale) }

// The text goldens drawn at several scales lay their text out at the scale
// they are drawn at.
func TestGoldenScenesLayOutAtTheirScale(t *testing.T) {
	scenes := []struct {
		name  string
		build func(testing.TB, float32) (*scene, int, int)
	}{
		{"paragraphs", paragraphsScene},
		{"cursor", cursorScene},
		{"bitmap_emoji", bitmapScene},
		{"language_forms", languageFormsScene},
	}
	for _, sc := range scenes {
		for _, k := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s_x%g", sc.name, k), func(t *testing.T) {
				s, _, _ := sc.build(t, k)
				if len(s.layouts) == 0 {
					t.Fatal("the scene lays nothing out")
				}
				for i, l := range s.layouts {
					if l.Scale() != k {
						t.Errorf("layout %d is laid out at scale %g, drawn at %g", i, l.Scale(), k)
					}
				}
			})
		}
	}
}
