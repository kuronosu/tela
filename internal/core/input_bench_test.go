package core

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// hoverList is bigList with a hover style on every row.
func hoverList(rows int) Node {
	row := func(i int) Node {
		return div(Class("flex items-center gap-2 px-2 py-1 hover:bg-slate-100"),
			span(Class("w-10 text-slate-500"), Textf("%d", i)),
			div(Class("grow"), Text(fmt.Sprintf("Item number %d", i))),
			div(Class("h-4 w-4 bg-sky-500")),
		)
	}
	items := make([]int, rows)
	for i := range items {
		items[i] = i
	}
	return div(Class("flex flex-col"),
		Map(items, func(i int) Node { return WithKey(ComponentProps(row, i), i) }))
}

// BenchmarkHitTest10k hit-tests points over a 10k-element list (rows of
// about 26 px; the viewport shows the first 23).
func BenchmarkHitTest10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, bigList(benchRows, setters), nil)
	tr.Frame()
	pts := [...][2]float32{{20, 10}, {200, 300}, {795, 590}, {400, 120}}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		p := pts[i&3]
		i++
		if tr.HitTest(p[0], p[1]) == nil {
			b.Fatal("nothing hit")
		}
	}
}

// BenchmarkHoverMove10k moves the mouse between two rows of a 10k-element
// list whose rows have hover: styles: hit-testing, the boundary diff and
// marking the two rows for restyling (no frame).
func BenchmarkHoverMove10k(b *testing.B) {
	tr := newTestTree(b, 800, 600, hoverList(benchRows), nil)
	tr.Frame()
	ys := [2]float32{40, 300}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		tr.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 300, Y: ys[i&1]}})
		i++ // the style queue keeps the two rows once (needsStyle)
	}
}

// BenchmarkHoverFrame10k is a hover change and the frame that follows:
// two rows restyled, then layout and the full paint (layers arrive with
// T7b).
func BenchmarkHoverFrame10k(b *testing.B) {
	tr := newTestTree(b, 800, 600, hoverList(benchRows), nil)
	tr.Frame()
	ys := [2]float32{40, 300}
	i := 0
	s0 := tr.Stats()
	b.ReportAllocs()
	for b.Loop() {
		tr.HandlePointer(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 300, Y: ys[i&1]}})
		i++
		tr.Frame()
	}
	s1 := tr.Stats()
	b.ReportMetric(float64(s1.Styles-s0.Styles)/float64(i), "restyles/op")
}

// buttonList is a list of rows with a button each: 4 primitives, 2 texts
// and a component per row.
func buttonList(rows int) Node {
	row := func(i int) Node {
		return div(Class("flex items-center gap-2 px-2 py-1"),
			span(Class("w-10 text-slate-500"), Textf("%d", i)),
			button(Class("grow focus-visible:ring-2"), Text(fmt.Sprintf("Item %d", i))),
			div(Class("h-4 w-4 bg-sky-500")),
		)
	}
	items := make([]int, rows)
	for i := range items {
		items[i] = i
	}
	return div(Class("flex flex-col"),
		Map(items, func(i int) Node { return WithKey(ComponentProps(row, i), i) }))
}

// BenchmarkTab10k moves the focus with Tab through a list of 1,500 rows
// with a button each (10,500 elements): the walk of the sequential order
// and the focus change (no frame).
func BenchmarkTab10k(b *testing.B) {
	tr := newTestTree(b, 800, 600, buttonList(1500), nil)
	tr.Frame()
	tab := platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeTab, Key: platform.KeyTab}
	b.ReportAllocs()
	for b.Loop() {
		tr.HandleKey(tab)
	}
}
