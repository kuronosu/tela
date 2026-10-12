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

// paragraphsScene lays out the paragraphs of the phase-2 golden: mixed
// scripts, a bidi sentence with numbers, alignments, justification,
// truncation, line-clamp, decorations and inline boxes.
func paragraphsScene(t testing.TB) (*gfx.DisplayList, int, int) {
	c := layoutCollection(t)
	b := gfx.NewBuilder()
	const W = 560
	ink := gfx.RGB8(0x11, 0x18, 0x27)
	muted := gfx.RGB8(0x6b, 0x72, 0x80)
	frame := gfx.RGB8(0xe5, 0xe7, 0xeb)
	accent := gfx.RGB8(0x4f, 0x46, 0xe5)
	fams := []string{"Inter", "Noto Sans Arabic", "Noto Sans JP", "Noto Color Emoji"}
	st := func(size, lh float32, mods ...func(*TextStyle)) TextStyle {
		s := TextStyle{Families: fams, Size: size, LineHeight: LineHeightPixels(lh), Paint: ink}
		for _, m := range mods {
			m(&s)
		}
		return s
	}
	y := float32(16)
	// box draws the layout box outline and paints l at (x, y).
	place := func(l *Layout, x, w float32) float32 {
		h := l.Height()
		b.FillRect(gfx.RectXYWH(x-0.5, y-0.5, w+1, h+1), frame)
		b.FillRect(gfx.RectXYWH(x, y, w, h), gfx.White)
		for i := range l.boxes {
			if r, ok := l.BoxRect(i); ok {
				b.FillRRect(gfx.NewRRect(r.Translate(x, y), 3), accent)
			}
		}
		l.Paint(b, gfx.Pt(x, y))
		return h
	}
	layout := func(p *Paragraph, w float32) *Layout {
		l := NewLayout(c, p)
		l.BreakLines(w)
		return l
	}

	// Title with an underline and a semibold weight.
	title := &Paragraph{Default: st(24, 32)}
	title.AddText("Paragraph layout", st(24, 32, func(s *TextStyle) { s.Weight = 650 }))
	title.AddText(" golden", st(24, 32, func(s *TextStyle) {
		s.Weight = 300
		s.Paint = muted
		s.Decoration = TextDecoration{Lines: DecorationUnderline, Paint: accent}
	}))
	y += place(layout(title, W), 16, W) + 12

	// Mixed scripts.
	mixed := &Paragraph{Default: st(16, 24)}
	mixed.AddText("Latin text, ", st(16, 24))
	mixed.AddText("العربية تتصل حروفها", st(16, 24, func(s *TextStyle) { s.Paint = gfx.RGB8(0x04, 0x78, 0x57) }))
	mixed.AddText(", 日本語の文章、中文简体字们说, and emoji 😀🎉👨‍👩‍👧‍👦🇯🇵👍🏽 all in one paragraph that wraps across several lines.", st(16, 24))
	y += place(layout(mixed, W), 16, W) + 12

	// A bidi sentence with numbers, in both base directions.
	bidiText := "The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done."
	y += place(layout(&Paragraph{Text: bidiText, Default: st(16, 24)}, W), 16, W) + 8
	rtl := &Paragraph{Text: "مرحبا بكم في Tela version 2.0 مع 15 ميزة (جديدة) اليوم.", Style: ParagraphStyle{Direction: DirectionRTL}, Default: st(16, 24)}
	y += place(layout(rtl, W), 16, W) + 12

	// Alignments in narrow boxes.
	short := "Align me in a narrow box, please: the last line is short."
	const nw = 128
	var maxH float32
	for i, a := range []TextAlign{AlignLeft, AlignCenter, AlignRight, AlignJustify} {
		l := layout(&Paragraph{Text: short, Style: ParagraphStyle{Align: a}, Default: st(14, 20)}, nw)
		maxH = max(maxH, place(l, float32(16+i*(nw+16)), nw))
	}
	y += maxH + 12

	// Justified Arabic (right to left) and truncation with an ellipsis.
	ar := "العربية هي لغة سامية يتحدث بها أكثر من أربعمائة مليون شخص، وهي لغة رسمية في اثنتين وعشرين دولة."
	h1 := place(layout(&Paragraph{Text: ar, Style: ParagraphStyle{Direction: DirectionRTL, Align: AlignJustify}, Default: st(15, 24)}, 270), 16, 270)
	trunc := ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}
	yy := y
	h2 := place(layout(&Paragraph{Text: "A single line that is far too long to fit", Style: trunc, Default: st(15, 22)}, 250), 302, 250)
	y += h2 + 8
	h3 := place(layout(&Paragraph{Text: "مرحبا بكم في هذا السطر الطويل جدا الذي لا يتسع", Style: ParagraphStyle{Direction: DirectionRTL, WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, Default: st(15, 22)}, 250), 302, 250)
	y = yy + max(h1, h2+8+h3) + 12

	// line-clamp-2.
	clamp := &Paragraph{Style: ParagraphStyle{LineClamp: 2}, Default: st(14, 20)}
	clamp.AddText("Line clamp: this paragraph has much more text than two lines can hold, so the second line ends with an ellipsis placed right after the last word that fits, and the rest of the text, which would need a third and a fourth line, is hidden from view.", st(14, 20, func(s *TextStyle) { s.Paint = muted }))
	y += place(layout(clamp, W), 16, W) + 12

	// Decorations and an inline box.
	deco := &Paragraph{Default: st(16, 28)}
	deco.AddText("Underline", st(16, 28, func(s *TextStyle) { s.Decoration = TextDecoration{Lines: DecorationUnderline} }))
	deco.AddText(", ", st(16, 28))
	deco.AddText("line-through", st(16, 28, func(s *TextStyle) {
		s.Decoration = TextDecoration{Lines: DecorationLineThrough, Paint: gfx.RGB8(0xdc, 0x26, 0x26)}
	}))
	deco.AddText(", ", st(16, 28))
	deco.AddText("overline", st(16, 28, func(s *TextStyle) {
		s.Decoration = TextDecoration{Lines: DecorationOverline, Thickness: 2, Paint: accent}
	}))
	deco.AddText(", an icon ", st(16, 28))
	deco.AddBox(InlineBox{ID: 1, Width: 16, Height: 16, Align: BoxMiddle})
	deco.AddText(" in the text, and ", st(16, 28))
	deco.AddText("TRACKING", st(12, 28, func(s *TextStyle) {
		s.LetterSpacing = 1.2
		s.Weight = 600
		s.Paint = muted
	}))
	deco.AddText(" with tabular 0123456789.", st(16, 28, func(s *TextStyle) { s.Numeric = NumericTabular }))
	y += place(layout(deco, W), 16, W) + 16

	return b.Finish(), W + 32, int(math.Ceil(float64(y)))
}

func TestGoldenParagraphs(t *testing.T) {
	dl, w, h := paragraphsScene(t)
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, int(float32(w)*scale), int(float32(h)*scale)))
			raster.Clear(img, image.Rectangle{}, gfx.RGB8(0xf9, 0xfa, 0xfb))
			raster.Render(img, dl, raster.Options{Scale: scale})
			golden.Image(t, fmt.Sprintf("paragraphs_x%g", scale), img)
		})
	}
}
