package text

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// thousandWords is a 1,000-word paragraph of English text.
func thousandWords() string {
	words := strings.Fields(latinText + " " + strings.Repeat("the quick brown fox jumps over a lazy dog while typography keeps every line readable ", 10))
	var b strings.Builder
	for i := range 1000 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(words[i%len(words)])
	}
	return b.String()
}

// BenchmarkLayout1000Words shapes and breaks a 1,000-word paragraph.
func BenchmarkLayout1000Words(b *testing.B) {
	c := layoutCollection(b)
	p := &Paragraph{Text: thousandWords(), Default: inter16}
	NewLayout(c, p) // warm the face caches
	b.ReportAllocs()
	for b.Loop() {
		l := NewLayout(c, p)
		l.BreakLines(600)
	}
}

// BenchmarkRebreak breaks the 1,000-word paragraph at a new width (no
// shaping).
func BenchmarkRebreak(b *testing.B) {
	l := NewLayout(layoutCollection(b), &Paragraph{Text: thousandWords(), Default: inter16})
	widths := []float32{600, 480, 720, 333}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		l.BreakLines(widths[i%len(widths)])
		i++
	}
}

// Benchmark200Labels lays out 200 short UI labels.
func Benchmark200Labels(b *testing.B) {
	c := layoutCollection(b)
	base := []string{"Save", "Cancel", "Settings", "Profile", "Sign out", "Notifications", "Dark mode", "Search…", "Delete item", "Rename"}
	ps := make([]*Paragraph, 200)
	for i := range ps {
		ps[i] = &Paragraph{Text: fmt.Sprintf("%s %d", base[i%len(base)], i), Default: inter16}
	}
	inf := float32(math.Inf(1))
	for _, p := range ps {
		NewLayout(c, p)
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, p := range ps {
			l := NewLayout(c, p)
			l.BreakLines(inf)
		}
	}
}

// BenchmarkPaint1000Words records the display list of the broken
// 1,000-word paragraph.
func BenchmarkPaint1000Words(b *testing.B) {
	l := NewLayout(layoutCollection(b), &Paragraph{Text: thousandWords(), Default: inter16})
	l.BreakLines(600)
	b.ReportAllocs()
	for b.Loop() {
		bl := gfx.NewBuilder()
		l.Paint(bl, gfx.Pt(0, 0))
		bl.Finish()
	}
}
