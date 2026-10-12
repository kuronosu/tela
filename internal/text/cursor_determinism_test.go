package text

import "testing"

// cursorDigest hashes the cursor geometry of the determinism corpus
// (snapped cursors and carets of every offset, hit-testing on a grid,
// selection rectangles and movement), recorded on amd64. Every
// architecture must reproduce it (ADR-027); check with
//
//	GOARCH=arm64 go test -short -exec qemu-aarch64-static -run Deterministic ./internal/text/
func cursorDigest(t testing.TB) uint64 {
	d := newDigest()
	for _, it := range determinismCorpus() {
		ps := ParagraphStyle{Align: AlignJustify}
		if r := []rune(it.text)[0]; r >= 0x600 && r < 0x700 {
			ps.Direction = DirectionRTL
		}
		l := simple(t, it.text, ps, it.style, it.width)
		for off := 0; off <= len(it.text); off++ {
			for _, a := range []Affinity{Downstream, Upstream} {
				c := l.SnapCursor(Cursor{off, a})
				ca := l.Caret(Cursor{off, a})
				d.f64(float64(c.Offset), float64(c.Affinity), float64(ca.Line))
				d.f32(ca.X, ca.Top, ca.Height, ca.Ascent, ca.Descent)
			}
		}
		for li := range l.NumLines() {
			ln := l.Line(li)
			for x := float32(-5); x < l.Width()+5; x += 3.3 {
				c := l.HitTest(x, ln.Top+ln.Height/2)
				d.f64(float64(c.Offset), float64(c.Affinity))
			}
		}
		n := len(it.text)
		for _, s := range []Selection{l.SelectAll(), NewSelection(cur(n/3), cur(2*n/3)), NewSelection(cur(n-5), up(7))} {
			for _, r := range l.SelectionRects(s, nil) {
				d.f32(r.X0, r.Y0, r.X1, r.Y1)
			}
		}
		for _, m := range []Movement{MoveRight, MoveLeft, MoveWordForward, MoveDown} {
			s := CaretSelection(cur(n / 2))
			for range 40 {
				s = l.Move(s, m, false)
				d.f64(float64(s.Focus.Offset), float64(s.Focus.Affinity))
			}
		}
	}
	return d.h.Sum64()
}

const wantCursorDigest uint64 = 0x74d9c47d593262b7

func TestCursorDeterministic(t *testing.T) {
	if got := cursorDigest(t); got != wantCursorDigest {
		t.Errorf("cursorDigest = %#x, want %#x (the same on every architecture)", got, wantCursorDigest)
	}
}
