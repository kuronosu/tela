package text

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

var (
	layoutCollOnce sync.Once
	layoutColl     *Collection
)

// layoutCollection is testCollection shared by the layout tests (font
// parsing is the slow part).
func layoutCollection(t testing.TB) *Collection {
	layoutCollOnce.Do(func() { layoutColl = testCollection(t) })
	return layoutColl
}

// simple lays out s in one style.
func simple(t testing.TB, s string, ps ParagraphStyle, st TextStyle, width float32) *Layout {
	t.Helper()
	p := &Paragraph{Text: s, Style: ps, Default: st}
	l := NewLayout(layoutCollection(t), p)
	l.BreakLines(width)
	return l
}

// lineTexts returns the source text of each line (logical order, trailing
// white space included).
func lineTexts(l *Layout) []string {
	var out []string
	for i := range l.NumLines() {
		ln := l.Line(i)
		out = append(out, l.Text()[ln.TextStart:ln.TextEnd])
	}
	return out
}

// visual returns the visible text of each line in visual order: clusters
// left to right, the characters of each cluster in logical order, "…" for
// an ellipsis and "[box]" for inline boxes.
func visual(l *Layout) []string {
	var out []string
	var runs []LineRun
	for i := range l.NumLines() {
		var b strings.Builder
		runs = l.LineRuns(i, runs[:0])
		for _, r := range runs {
			switch r.Kind {
			case RunEllipsis:
				b.WriteString("…")
			case RunBox:
				b.WriteString("[box]")
			default:
				if r.Level%2 == 0 {
					for c := r.ClusterStart; c < r.ClusterEnd; c++ {
						cl := l.Cluster(c)
						b.WriteString(l.Text()[cl.TextStart:cl.TextEnd])
					}
				} else {
					for c := r.ClusterEnd - 1; c >= r.ClusterStart; c-- {
						cl := l.Cluster(c)
						b.WriteString(l.Text()[cl.TextStart:cl.TextEnd])
					}
				}
			}
		}
		out = append(out, b.String())
	}
	return out
}

// dump describes a layout for failure messages.
func dump(l *Layout) string {
	var b strings.Builder
	fmt.Fprintf(&b, "width %g height %g (max %g, min-content %g, max-content %g)\n", l.Width(), l.Height(), l.MaxWidth(), l.MinContentWidth(), l.MaxContentWidth())
	vis := visual(l)
	for i := range l.NumLines() {
		ln := l.Line(i)
		fmt.Fprintf(&b, "  line %d: %q x=%g w=%g hang=%g top=%g h=%g base=%g break=%d visual=%q\n", i, l.Text()[ln.TextStart:ln.TextEnd], ln.X, ln.Width, ln.Hanging, ln.Top, ln.Height, ln.Baseline, ln.Break, vis[i])
	}
	return b.String()
}
