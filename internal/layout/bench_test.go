package layout

import "testing"

// benchTree builds a page-like tree of about n nodes: a block root with
// rows of four cards (flex rows of flex columns) holding a title, a
// wrapping paragraph and a fixed-size bar. It returns the root and the
// paragraphs.
func benchTree(n int) (*testNode, []*testNode) {
	root := box(styleOf(func(s *Style) { s.Padding = EdgesAll(Px(16)) }))
	var paragraphs []*testNode
	count := 1
	for count < n {
		row := box(styleOf(func(s *Style) {
			s.Display = DisplayFlex
			s.ColumnGap = Px(8)
			s.Padding = EdgesAll(Px(4))
			s.AlignItems = AlignStart
		}))
		for c := 0; c < 4; c++ {
			title := text("HHHH\u200bHHH")
			para := text("HHHHHH\u200bHHH\u200bHHHHH\u200bHH\u200bHHHHHHH\u200bHHHH\u200bHHH\u200bHHHHHH\u200bHH")
			bar := box(styleOf(func(s *Style) {
				s.Height = Px(4)
				s.Width = Percent(0.6)
			}))
			card := box(styleOf(func(s *Style) {
				s.Display = DisplayFlex
				s.FlexDirection = Column
				s.FlexGrow = 1
				s.FlexBasis = Px(0)
				s.RowGap = Px(4)
				s.Padding = EdgesAll(Px(6))
				s.Border = EdgesAll(Px(1))
			}), title, para, bar)
			row.children = append(row.children, card)
			paragraphs = append(paragraphs, para)
		}
		root.children = append(root.children, row)
		count += 1 + 4*4
	}
	return root, paragraphs
}

func resetData(n *testNode) {
	n.data = Data{}
	for _, c := range n.children {
		resetData(c)
	}
}

// BenchmarkLayoutCold lays out a 10,000-node tree from scratch.
func BenchmarkLayoutCold(b *testing.B) {
	root, _ := benchTree(10000)
	b.Logf("%d nodes", len(nodes(root)))
	var e Engine
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		resetData(root)
		b.StartTimer()
		e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	}
}

// BenchmarkLayoutIncremental changes one paragraph of a laid out
// 10,000-node tree and lays it out again.
func BenchmarkLayoutIncremental(b *testing.B) {
	root, paragraphs := benchTree(10000)
	var e Engine
	e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	texts := []func(MeasureInput) Measurement{
		ahemMeasure("HHHHHH\u200bHHH\u200bHHHHH\u200bHH"),
		ahemMeasure("HHH\u200bHHHHHHHHH\u200bHH\u200bHHHH\u200bHHHHHH\u200bHHHHHHHHH"),
	}
	p := paragraphs[len(paragraphs)/2]
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		p.measure = texts[i%2]
		i++
		e.MarkDirty(p)
		e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	}
}

// BenchmarkLayoutClean lays out a 10,000-node tree with nothing dirty.
func BenchmarkLayoutClean(b *testing.B) {
	root, _ := benchTree(10000)
	var e Engine
	e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	b.ReportAllocs()
	for b.Loop() {
		e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	}
}

// BenchmarkRound rounds a 10,000-node tree.
func BenchmarkRound(b *testing.B) {
	root, _ := benchTree(10000)
	var e Engine
	e.Compute(root, DefiniteSpace(1200), DefiniteSpace(800))
	b.ReportAllocs()
	for b.Loop() {
		Round(root)
	}
}
