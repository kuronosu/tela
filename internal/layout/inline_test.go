package layout

import "testing"

// rowNode is an inline container whose content is its atoms on one line,
// aligned on their baselines (a paragraph of atomic inlines only).
type rowNode struct {
	testNode
	sizes []AtomSize // the last sizes given by Atoms.Size
	calls int
}

func (n *rowNode) InlineContent() bool { return true }

func (n *rowNode) MeasureInline(in MeasureInput, a *Atoms) Measurement {
	n.calls++
	var minW, maxW float32
	for i := range a.Len() {
		mn, mx := a.Intrinsic(i)
		minW, maxW = max(minW, mn), maxW+mx
	}
	var w float32
	switch {
	case in.HasWidth:
		w = in.Width
	case in.AvailableWidth.Kind == MinContentSpace:
		w = minW
	case in.AvailableWidth.Kind == MaxContentSpace:
		w = maxW
	default:
		w = min(maxW, max(minW, in.AvailableWidth.Value))
	}
	n.sizes = n.sizes[:0]
	var above, below float32
	for i := range a.Len() {
		s := a.Size(i, w)
		n.sizes = append(n.sizes, s)
		b := s.Height
		if s.HasBaseline {
			b = s.Baseline
		}
		above, below = max(above, b), max(below, s.Height-b)
	}
	var x float32
	for i, s := range n.sizes {
		b := s.Height
		if s.HasBaseline {
			b = s.Baseline
		}
		a.Place(i, x, above-b)
		x += s.Width
	}
	return Measurement{Width: w, Height: above + below, Baseline: above, HasBaseline: true, LastBaseline: above, HasLastBaseline: true}
}

// zwsp is the break opportunity of Ahem test text.
const zwsp = string(rune(0x200b))

// lastLines makes an Ahem text leaf report the baseline of its last line
// (8 px above the bottom of each 10 px line).
func lastLines(n *testNode) *testNode {
	m := n.measure
	n.measure = func(in MeasureInput) Measurement {
		r := m(in)
		r.LastBaseline, r.HasLastBaseline = r.Height-2, true
		return r
	}
	return n
}

func TestInlineContainerAtoms(t *testing.T) {
	// An inline-block of two lines of Ahem text ("HHHH HHHH" in 50px):
	// fit-content 40px, its baseline the second line's (18 from its top,
	// plus its 2px padding).
	block := box(styleOf(func(s *Style) {
		s.Width = FitContent
		s.Padding = EdgesAll(Px(2))
		s.Margin = Edges{Left: Px(3), Right: Px(3)}
	}), lastLines(text("HHHH"+zwsp+"HHHH")))
	// An overflow-hidden inline-block: its bottom margin edge is the
	// baseline.
	clipped := box(styleOf(func(s *Style) {
		s.Width = Px(20)
		s.Height = Px(30)
		s.OverflowX, s.OverflowY = OverflowHidden, OverflowHidden
	}))
	// An inline-flex: its first baseline, here synthesized from the
	// bottom of its first item (no item is baseline-aligned; CSS Flexbox
	// 8.5).
	flex := box(styleOf(func(s *Style) { s.Display = DisplayFlex; s.AlignItems = AlignCenter }),
		box(styleOf(func(s *Style) { s.Width = Px(10); s.Height = Px(24) })), text("HH"))
	row := &rowNode{testNode: testNode{style: DefaultStyle(), children: []*testNode{block, clipped, flex}}}
	var e Engine
	wrap := &wrapper{style: styleOf(func(s *Style) { s.Width = Px(50) }), kid: row}
	e.Compute(wrap, DefiniteSpace(200), DefiniteSpace(200))

	if !row.sizes[0].HasBaseline || row.sizes[0].Baseline != 2+18 {
		t.Errorf("inline-block: %+v, want the last line's baseline 20", row.sizes[0])
	}
	if row.sizes[0].Width != 40+4+6 {
		t.Errorf("inline-block margin box %g wide, want 50 (fit-content 40 + padding + margins)", row.sizes[0].Width)
	}
	if row.sizes[1].HasBaseline {
		t.Errorf("overflow-hidden inline-block has a baseline: %+v", row.sizes[1])
	}
	if !row.sizes[2].HasBaseline || row.sizes[2].Baseline != 24 {
		t.Errorf("inline-flex: %+v, want its first baseline 24", row.sizes[2])
	}
	// Placement: the baselines line up at max(20, 30, 24) = 30.
	wantRect(t, "block", block, [4]float32{3, 10, 44, 24})
	wantRect(t, "clipped", clipped, [4]float32{50, 0, 20, 30})
	wantRect(t, "flex", flex, [4]float32{70, 6, 30, 24})
	if l := row.data.Layout(); l.Size.H != 34 || !l.HasBaseline || l.FirstBaseline != 30 {
		t.Errorf("row %+v, want 34 tall with its baseline at 30", l)
	}

	// A change inside an atom lays the container out again.
	calls := row.calls
	block.children[0].measure = lastLines(text("HH")).measure
	e.MarkDirty(block.children[0])
	e.Compute(wrap, DefiniteSpace(200), DefiniteSpace(200))
	if row.calls == calls {
		t.Error("the inline container was not laid out again")
	}
	if got := row.sizes[0].Width; got != 20+4+6 {
		t.Errorf("after the change the inline-block is %g wide, want 30", got)
	}
}

// wrapper is a block with one child of any Node type.
type wrapper struct {
	style Style
	kid   Node
	data  Data
}

func (w *wrapper) LayoutStyle() *Style    { return &w.style }
func (w *wrapper) LayoutChildCount() int  { return 1 }
func (w *wrapper) LayoutChild(i int) Node { return w.kid }
func (w *wrapper) LayoutData() *Data      { return &w.data }
