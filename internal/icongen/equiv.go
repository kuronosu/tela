package icongen

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/svg"
)

// Equivalent reports an error when Tela draws the SVG documents a and b
// differently: it parses both and compares what the document says about
// itself (intrinsic size, viewBox, clipping, use of currentColor) and the
// display lists they draw into boxes of several sizes and positions, with
// several colors and the root overrides of the SVG paint classes
// (fill-*, stroke-*, stroke-[w]). The lists print every number exactly.
func Equivalent(a, b []byte) error {
	da, err := svg.Parse(a)
	if err != nil {
		return fmt.Errorf("the first document: %w", err)
	}
	db, err := svg.Parse(b)
	if err != nil {
		return fmt.Errorf("the second document: %w", err)
	}
	return equivalentDocs(da, db)
}

func equivalentDocs(a, b *svg.Doc) error {
	aw, ah, ar := a.Intrinsic()
	bw, bh, br := b.Intrinsic()
	if aw != bw || ah != bh || ar != br {
		return fmt.Errorf("intrinsic size %gx%g (ratio %g) became %gx%g (ratio %g)", aw, ah, ar, bw, bh, br)
	}
	if a.Size() != b.Size() {
		return fmt.Errorf("size %v became %v", a.Size(), b.Size())
	}
	avb, aok := a.ViewBox()
	bvb, bok := b.ViewBox()
	if avb != bvb || aok != bok {
		return fmt.Errorf("viewBox %v became %v", avb, bvb)
	}
	if a.Clips() != b.Clips() || a.UsesCurrentColor() != b.UsesCurrentColor() {
		return fmt.Errorf("clipping or currentColor use changed")
	}
	red := gfx.MustParseHex("#dc2626")
	rects := []gfx.Rect{
		gfx.RectXYWH(0, 0, 24, 24),
		gfx.RectXYWH(3.3, 7.1, 16, 16),
		gfx.RectXYWH(10, 2, 97.7, 31.9),
	}
	opts := []svg.Options{
		{CurrentColor: gfx.Black},
		{CurrentColor: red},
		{CurrentColor: red, Root: svg.Root{
			Fill:   svg.Paint{Kind: svg.PaintCurrentColor},
			Stroke: svg.Paint{Kind: svg.PaintNone},
		}},
		{CurrentColor: gfx.Black, Root: svg.Root{
			Fill:           svg.Paint{Kind: svg.PaintColor, Color: gfx.MustParseHex("#2563eb")},
			Stroke:         svg.Paint{Kind: svg.PaintColor, Color: red},
			StrokeWidth:    1.5,
			HasStrokeWidth: true,
			Color:          gfx.MustParseHex("#16a34a"),
			HasColor:       true,
		}},
	}
	for _, r := range rects {
		for i, o := range opts {
			la, lb := drawString(a, r, o), drawString(b, r, o)
			if la != lb {
				return fmt.Errorf("the display lists differ in box %v with options #%d:\n%s\n--- became ---\n%s", r, i, la, lb)
			}
		}
	}
	return nil
}

func drawString(d *svg.Doc, r gfx.Rect, o svg.Options) string {
	b := gfx.NewBuilder()
	d.Draw(b, r, o)
	return b.Finish().String()
}
