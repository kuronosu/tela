package core

import (
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

func debugLabelScene() Node {
	return div(Class("flex gap-2 p-4"),
		button(Class("rounded bg-indigo-500 px-3 py-1 text-white"), Text("Go")),
		button(Class("rounded border px-3 py-1"), Text("Cancel")),
	)
}

// TestDebugLabelsAtDisplayScale: the overlay labels are laid out at the
// display scale, as the application's text is: the baseline of a label
// sits in its pill where a paragraph of the label's style puts it at that
// scale (its line box quantized in device pixels, ADR-159), not where it
// would at scale 1. They were laid out at scale 1, half a logical pixel
// too low at x2.
func TestDebugLabelsAtDisplayScale(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		ref := text.NewLayout(testFonts, &text.Paragraph{
			Text:  "button",
			Style: text.ParagraphStyle{WhiteSpace: text.WhiteSpacePre},
			Default: text.TextStyle{
				Families: []string{"JetBrains Mono", "monospace"}, Size: labelSize, Weight: 500,
				LineHeight: text.LineHeightPixels(labelLine),
			},
			Scale: scale,
		})
		ref.BreakLines(float32(math.Inf(1)))
		want := ref.Line(0).Baseline

		tr := NewTree(Config{Fonts: testFonts, Debug: DebugSemantics})
		tr.SetViewport(gfx.Size{W: 240, H: 100})
		tr.SetScale(scale)
		tr.Mount(debugLabelScene())
		for i := 0; i < MaxRenderFrames; i++ {
			tr.Frame()
			if !tr.Dirty() {
				break
			}
		}
		ol := tr.buildOverlay(image.Rect(0, 0, int(240*scale+0.5), int(100*scale+0.5)))
		var pill gfx.Rect
		labels := 0
		for _, c := range ol.All() {
			switch c := c.(type) {
			case gfx.FillRRect:
				if c.Paint == gfx.Paint(colPill) {
					pill = c.RRect.Rect
				}
			case gfx.DrawGlyphRun:
				if c.Paint != gfx.Paint(colPillText) {
					continue
				}
				labels++
				if got := c.Origin.Y - pill.Y0 - labelPadY; math.Abs(float64(got-want)) > 1e-4 {
					t.Errorf("x%g: label baseline %g below its line top, want %g (the label's line box at the display scale)", scale, got, want)
				}
			}
		}
		if labels < 2 {
			t.Errorf("x%g: %d labels drawn, want one per button", scale, labels)
		}
		tr.Unmount()
	}
}
