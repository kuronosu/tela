package core

import (
	"fmt"
	"testing"
)

// bigList returns a list of rows, each a component with its own label
// state: 7 Elements per row (component, row, span, text, div, text, div)
// and 2 paragraphs. setters receives each row's label setter.
func bigList(rows int, setters []func(string)) Node {
	row := func(i int) Node {
		label, set := UseState(fmt.Sprintf("Item number %d", i))
		setters[i] = set
		return div(Class("flex items-center gap-2 px-2 py-1"),
			span(Class("w-10 text-slate-500"), Textf("%d", i)),
			div(Class("grow"), Text(label)),
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

// benchRows makes about 10,000 Elements.
const benchRows = 1428

func TestIncrementalUpdateWork(t *testing.T) {
	setters := make([]func(string), 200)
	tr := newTestTree(t, 600, 800, bigList(len(setters), setters), nil)
	tr.Frame()
	s0 := tr.Stats()
	setters[150]("changed")
	tr.Frame()
	s1 := tr.Stats()
	if d := s1.Builds - s0.Builds; d != 1 {
		t.Errorf("%d builds, want 1", d)
	}
	if d := s1.Styles - s0.Styles; d != 0 {
		t.Errorf("%d style computations, want 0", d)
	}
	if d := s1.Shapes - s0.Shapes; d != 1 {
		t.Errorf("%d paragraphs shaped, want 1", d)
	}
	if d := s1.Structures - s0.Structures; d != 1 {
		t.Errorf("%d box structures, want 1", d)
	}
	if d := s1.Mounts - s0.Mounts; d != 0 {
		t.Errorf("%d mounts, want 0", d)
	}
	root := tr.Inspect()
	row := root.Children[0].Children[150]
	if row.Children[1].Text != "changed" {
		t.Errorf("row 150: %s", dump(&row))
	}
}

func BenchmarkMount10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	b.ReportAllocs()
	var mounts int
	for b.Loop() {
		tr := newTestTree(b, 800, 600, bigList(benchRows, setters), nil)
		tr.Frame()
		mounts = tr.Stats().Mounts
	}
	b.ReportMetric(float64(mounts), "elements")
}

func BenchmarkUpdateText10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, bigList(benchRows, setters), nil)
	tr.Frame()
	labels := [2]string{"Updated label", "Another label"}
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		setters[benchRows/2](labels[i&1])
		i++
		tr.Frame()
	}
}

// BenchmarkIdleFrame10k is a frame with nothing dirty.
func BenchmarkIdleFrame10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, bigList(benchRows, setters), nil)
	tr.Frame()
	b.ReportAllocs()
	for b.Loop() {
		tr.Frame()
	}
}
