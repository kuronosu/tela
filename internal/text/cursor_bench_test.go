package text

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// thousandWordLayout is the 1,000-word paragraph broken at 600 px.
func thousandWordLayout(b *testing.B) *Layout {
	l := NewLayout(layoutCollection(b), &Paragraph{Text: thousandWords(), Default: inter16})
	l.BreakLines(600)
	return l
}

// BenchmarkHitTest1000Words hit-tests points spread over the 1,000-word
// paragraph (one HitTest per iteration).
func BenchmarkHitTest1000Words(b *testing.B) {
	l := thousandWordLayout(b)
	var pts [][2]float32
	for y := float32(3); y < l.Height(); y += 37 {
		for x := float32(-10); x < 620; x += 23 {
			pts = append(pts, [2]float32{x, y})
		}
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		p := pts[i%len(pts)]
		l.HitTest(p[0], p[1])
		i++
	}
}

// BenchmarkSelectionRects1000Words computes the rectangles of a selection
// of the middle half of the 1,000-word paragraph (about 90 lines).
func BenchmarkSelectionRects1000Words(b *testing.B) {
	l := thousandWordLayout(b)
	n := len(l.Text())
	s := NewSelection(cur(n/4), cur(3*n/4))
	var dst []gfx.Rect
	b.ReportAllocs()
	for b.Loop() {
		dst = l.SelectionRects(s, dst[:0])
	}
}

// BenchmarkSelectionRectsShort computes the rectangles of a selection of
// a few words in the 1,000-word paragraph.
func BenchmarkSelectionRectsShort(b *testing.B) {
	l := thousandWordLayout(b)
	n := len(l.Text())
	s := NewSelection(cur(n/2), cur(n/2+40))
	var dst []gfx.Rect
	b.ReportAllocs()
	for b.Loop() {
		dst = l.SelectionRects(s, dst[:0])
	}
}

// BenchmarkCaret1000Words computes carets at offsets spread over the
// 1,000-word paragraph.
func BenchmarkCaret1000Words(b *testing.B) {
	l := thousandWordLayout(b)
	n := len(l.Text())
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		l.Caret(Cursor{(i * 7919) % n, Affinity(i & 1)})
		i++
	}
}

// BenchmarkMove1000Words runs arrow, word and vertical movements from
// positions spread over the 1,000-word paragraph.
func BenchmarkMove1000Words(b *testing.B) {
	l := thousandWordLayout(b)
	n := len(l.Text())
	moves := []Movement{MoveRight, MoveLeft, MoveForward, MoveWordForward, MoveWordBackward, MoveDown, MoveUp, MoveLineEnd}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		l.Move(CaretSelection(cur((i*7919)%n)), moves[i%len(moves)], false)
		i++
	}
}
