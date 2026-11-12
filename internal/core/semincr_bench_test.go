package core

import (
	"fmt"
	"testing"
)

// The cost of a one-node change of the semantics in a tree of about
// 10,000 elements (ADR-277): a checkbox's label (AriaLabel), its checked
// state (AriaChecked) or a text, in one row component of many, after the
// frame that applies it. BenchmarkSemanticsChange10k builds the semantics
// tree; BenchmarkAccessChange10k also converts it into the accessibility
// update a bridge receives.

// semBenchRow is the state of a row of semBenchList.
type semBenchRow struct {
	label, text string
	checked     bool
}

// semBenchList returns 1,428 rows (7 elements each: the component, the
// row, a checkbox, two spans and their texts) in groups of 20; set
// receives each row's state setter.
func semBenchList(set []func(semBenchRow)) Node {
	row := func(i int) Node {
		st, s := UseStateSet(semBenchRow{label: fmt.Sprintf("Row %d", i), text: fmt.Sprintf("Item number %d", i)})
		set[i] = s
		return div(Class("flex items-center gap-2 px-2 py-1"),
			div(Class("size-4"), Role("checkbox"), AriaChecked(st.checked), AriaLabel(st.label)),
			span(Class("grow"), Text(st.text)),
			span(Class("w-10 text-slate-500"), Textf("%d", i)))
	}
	groups := make([]Child, 0, len(set)/20+1)
	for g := 0; g < len(set); g += 20 {
		rows := make([]Child, 0, 20)
		for i := g; i < min(g+20, len(set)); i++ {
			rows = append(rows, WithKey(ComponentProps(row, i), i))
		}
		groups = append(groups, div(Role("group"), Class("flex flex-col"), Fragment(rows...)))
	}
	return div(Class("flex flex-col"), Fragment(groups...))
}

// semBenchChanges are the changes of the benchmarks, each with its undo.
var semBenchChanges = []struct {
	name string
	f    func(i int, r semBenchRow) semBenchRow
}{
	{"label", func(i int, r semBenchRow) semBenchRow { r.label = fmt.Sprintf("Row %d (%d)", 714, i&1); return r }},
	{"checked", func(i int, r semBenchRow) semBenchRow { r.checked = i&1 == 0; return r }},
	{"text", func(i int, r semBenchRow) semBenchRow { r.text = fmt.Sprintf("Item number 714 (%d)", i&1); return r }},
}

// benchSemanticsChange runs the change named change in a loop, timing
// what measure does after each frame.
func benchSemanticsChange(b *testing.B, f func(i int, r semBenchRow) semBenchRow, measure func(tr *Tree)) {
	set := make([]func(semBenchRow), 1428)
	tr := newTestTree(b, 800, 600, semBenchList(set), nil)
	tr.Frame()
	measure(tr)
	r := semBenchRow{label: "Row 714", text: "Item number 714"}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		b.StopTimer()
		r = f(i, r)
		i++
		set[714](r)
		tr.Frame()
		b.StartTimer()
		measure(tr)
	}
}

func BenchmarkSemanticsChange10k(b *testing.B) {
	for _, c := range semBenchChanges {
		b.Run(c.name, func(b *testing.B) {
			benchSemanticsChange(b, c.f, func(tr *Tree) { tr.Semantics() })
		})
	}
}

func BenchmarkAccessChange10k(b *testing.B) {
	for _, c := range semBenchChanges {
		b.Run(c.name, func(b *testing.B) {
			a := &accessView{first: true}
			benchSemanticsChange(b, c.f, func(tr *Tree) {
				if _, ok := a.update(tr.Semantics()); !ok && !a.first {
					b.Fatal("no update")
				}
			})
		})
	}
}
