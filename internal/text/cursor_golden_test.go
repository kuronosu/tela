package text

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// cursorScene draws selections (under the text) and carets on a mixed
// bidi paragraph, a wrapped left-to-right paragraph, a right-to-left
// paragraph and a pre-wrap sample with hanging spaces, a ligature and an
// inline box. Upstream carets are orange, Downstream carets blue.
func cursorScene(t testing.TB) (*gfx.DisplayList, int, int) {
	c := layoutCollection(t)
	b := gfx.NewBuilder()
	const W = 330
	ink := gfx.RGB8(0x11, 0x18, 0x27)
	frame := gfx.RGB8(0xe5, 0xe7, 0xeb)
	selColor := gfx.RGBA8(0x3b, 0x82, 0xf6, 0x59)
	downColor := gfx.RGB8(0x25, 0x63, 0xeb)
	upColor := gfx.RGB8(0xea, 0x58, 0x0c)
	st := TextStyle{Families: allFamilies, Size: 16, LineHeight: LineHeightPixels(24), Paint: ink}
	y := float32(12)
	x0 := float32(12)
	panel := func(p *Paragraph, w float32, sel Selection, carets []Cursor) *Layout {
		l := NewLayout(c, p)
		l.BreakLines(w)
		h := l.Height()
		b.FillRect(gfx.RectXYWH(x0-0.5, y-0.5, w+1, h+1), frame)
		b.FillRect(gfx.RectXYWH(x0, y, w, h), gfx.White)
		for _, r := range l.SelectionRects(sel, nil) {
			b.FillRect(r.Translate(x0, y), selColor)
		}
		for i := range p.Boxes {
			if r, ok := l.BoxRect(i); ok {
				b.FillRRect(gfx.NewRRect(r.Translate(x0, y), 3), gfx.RGB8(0x4f, 0x46, 0xe5))
			}
		}
		l.Paint(b, gfx.Pt(x0, y))
		for _, cu := range carets {
			col := downColor
			if cu.Affinity == Upstream {
				col = upColor
			}
			b.FillRect(l.CursorRect(cu, 1.5).Translate(x0, y), col)
		}
		y += h + 12
		return l
	}

	// Mixed bidi: a selection from "120" into the Arabic phrase is split
	// into visual pieces; the two carets of the boundaries before "دولار"
	// (17) and after "بكم" (83).
	bidi := "The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done."
	panel(&Paragraph{Text: bidi, Default: st}, W, NewSelection(cur(13), cur(28)), []Cursor{up(17), cur(17), up(83), cur(83)})

	// Wrapped LTR: a selection over three lines extends each crossed line
	// by a space; carets at both sides of a soft wrap.
	l := NewLayout(c, &Paragraph{Text: latinText, Default: st})
	l.BreakLines(W)
	wrap := l.Line(1).TextStart
	panel(&Paragraph{Text: latinText, Default: st}, W, NewSelection(cur(20), cur(110)), []Cursor{cur(wrap - 1), cur(wrap)})

	// Right to left: the line-break extension is on the left.
	panel(&Paragraph{Text: arabicText, Style: ParagraphStyle{Direction: DirectionRTL}, Default: st}, W, NewSelection(cur(20), cur(90)), []Cursor{cur(0), cur(len(arabicText))})

	// Pre-wrap: hanging spaces at a soft wrap (Upstream at the line end,
	// Downstream at the next line's start), a caret inside the "->"
	// ligature, and an inline box selected alone.
	p := &Paragraph{Style: ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, Default: st}
	p.AddText("Hanging spaces wrap      here a->b ", st)
	p.AddBox(InlineBox{Width: 18, Height: 14, Align: BoxMiddle})
	p.AddText(" box", st)
	k := len("Hanging spaces wrap      ")
	lig := len("Hanging spaces wrap      here a-")
	box := len("Hanging spaces wrap      here a->b ")
	l = panel(p, 200, NewSelection(up(box), cur(box)), []Cursor{up(k), cur(k), cur(lig)})
	_ = l

	return b.Finish(), W + 24, int(math.Ceil(float64(y)))
}

func TestGoldenCursor(t *testing.T) {
	dl, w, h := cursorScene(t)
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, int(float32(w)*scale), int(float32(h)*scale)))
			raster.Clear(img, image.Rectangle{}, gfx.RGB8(0xf9, 0xfa, 0xfb))
			raster.Render(img, dl, raster.Options{Scale: scale})
			golden.Image(t, fmt.Sprintf("cursor_x%g", scale), img)
		})
	}
}
