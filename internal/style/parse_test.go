package style

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

func eq[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

// at 16px per rem and a 1280x800 window.
func px(v float32) Dim { return DimPx(v) }

func TestLayoutClasses(t *testing.T) {
	tests := []struct {
		classes string
		check   func(t *testing.T, s *ComputedStyle)
	}{
		{"block", func(t *testing.T, s *ComputedStyle) { eq(t, "Display", s.Display, DisplayBlock) }},
		{"flex", func(t *testing.T, s *ComputedStyle) { eq(t, "Display", s.Display, DisplayFlex) }},
		{"inline-flex", func(t *testing.T, s *ComputedStyle) { eq(t, "Display", s.Display, DisplayInlineFlex) }},
		{"grid", func(t *testing.T, s *ComputedStyle) { eq(t, "Display", s.Display, DisplayGrid) }},
		{"hidden", func(t *testing.T, s *ComputedStyle) { eq(t, "Display", s.Display, DisplayNone) }},
		{"contents", func(t *testing.T, s *ComputedStyle) { eq(t, "Display", s.Display, DisplayContents) }},
		{"inline-block", func(t *testing.T, s *ComputedStyle) { eq(t, "Display", s.Display, DisplayInlineBlock) }},
		{"absolute", func(t *testing.T, s *ComputedStyle) { eq(t, "Position", s.Position, PositionAbsolute) }},
		{"sticky", func(t *testing.T, s *ComputedStyle) { eq(t, "Position", s.Position, PositionSticky) }},
		{"absolute inset-0", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Inset", s.Inset, AllSides(px(0)))
		}},
		{"top-4 -left-2 right-1/2 bottom-px", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Top", s.Inset.Top, px(16))
			eq(t, "Left", s.Inset.Left, px(-8))
			eq(t, "Right", s.Inset.Right, DimPct(50))
			eq(t, "Bottom", s.Inset.Bottom, px(1))
		}},
		{"inset-x-4 inset-y-auto", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Left", s.Inset.Left, px(16))
			eq(t, "Right", s.Inset.Right, px(16))
			eq(t, "Top", s.Inset.Top, Auto)
		}},
		{"start-2 end-3", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Left", s.Inset.Left, px(8))
			eq(t, "Right", s.Inset.Right, px(12))
		}},
		{"-top-1/2", func(t *testing.T, s *ComputedStyle) { eq(t, "Top", s.Inset.Top, DimPct(-50)) }},
		{"z-10", func(t *testing.T, s *ComputedStyle) { eq(t, "ZIndex", s.ZIndex, ZIndex{true, 10}) }},
		{"-z-10", func(t *testing.T, s *ComputedStyle) { eq(t, "ZIndex", s.ZIndex, ZIndex{true, -10}) }},
		{"z-[100]", func(t *testing.T, s *ComputedStyle) { eq(t, "ZIndex", s.ZIndex, ZIndex{true, 100}) }},
		{"z-10 z-auto", func(t *testing.T, s *ComputedStyle) { eq(t, "ZIndex", s.ZIndex, ZIndex{}) }},
		{"overflow-hidden", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.OverflowX, OverflowHidden)
			eq(t, "Y", s.OverflowY, OverflowHidden)
		}},
		{"overflow-x-auto overflow-y-scroll", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.OverflowX, OverflowAuto)
			eq(t, "Y", s.OverflowY, OverflowScroll)
		}},
		{"invisible", func(t *testing.T, s *ComputedStyle) { eq(t, "Visibility", s.Visibility, VisibilityHidden) }},
		{"invisible visible", func(t *testing.T, s *ComputedStyle) { eq(t, "Visibility", s.Visibility, VisibilityVisible) }},
		{"isolate", func(t *testing.T, s *ComputedStyle) { eq(t, "Isolate", s.Isolate, true) }},
		{"aspect-video", func(t *testing.T, s *ComputedStyle) { eq(t, "Aspect", s.AspectRatio, float32(1.7777778)) }},
		{"aspect-square", func(t *testing.T, s *ComputedStyle) { eq(t, "Aspect", s.AspectRatio, float32(1)) }},
		{"aspect-[4/3]", func(t *testing.T, s *ComputedStyle) { eq(t, "Aspect", s.AspectRatio, float32(4)/3) }},
		{"aspect-3/2", func(t *testing.T, s *ComputedStyle) { eq(t, "Aspect", s.AspectRatio, float32(1.5)) }},
		{"container", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Width", s.Width, DimPct(100))
			eq(t, "MaxWidth", s.MaxWidth, px(1280)) // a 1280 px window reaches the xl breakpoint
		}},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) {
			s := style(t, tt.classes)
			tt.check(t, s)
		})
	}
}

func TestContainerBreakpoints(t *testing.T) {
	c := mustCompile(t, "container")
	for _, tc := range []struct {
		width float32
		want  Dim
	}{{500, Auto}, {640, px(640)}, {900, px(768)}, {1300, px(1280)}, {2000, px(1536)}} {
		s := Compute(&Context{Viewport: gfx.Size{W: tc.width, H: 800}}, Input{Classes: []*Classes{c}})
		eq(t, "MaxWidth", s.MaxWidth, tc.want)
	}
}

func TestFlexGridClasses(t *testing.T) {
	tests := []struct {
		classes string
		check   func(t *testing.T, s *ComputedStyle)
	}{
		{"flex-row-reverse", func(t *testing.T, s *ComputedStyle) { eq(t, "Dir", s.FlexDirection, FlexRowReverse) }},
		{"flex-col", func(t *testing.T, s *ComputedStyle) { eq(t, "Dir", s.FlexDirection, FlexColumn) }},
		{"flex-wrap", func(t *testing.T, s *ComputedStyle) { eq(t, "Wrap", s.FlexWrap, FlexWrapOn) }},
		{"flex-wrap-reverse", func(t *testing.T, s *ComputedStyle) { eq(t, "Wrap", s.FlexWrap, FlexWrapReverse) }},
		{"flex-wrap flex-nowrap", func(t *testing.T, s *ComputedStyle) { eq(t, "Wrap", s.FlexWrap, FlexNoWrap) }},
		{"flex-1", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Grow", s.FlexGrow, 1)
			eq(t, "Shrink", s.FlexShrink, 1)
			eq(t, "Basis", s.FlexBasis, DimPct(0))
		}},
		{"flex-auto", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Grow", s.FlexGrow, 1)
			eq(t, "Basis", s.FlexBasis, Auto)
		}},
		{"flex-initial", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Grow", s.FlexGrow, 0)
			eq(t, "Shrink", s.FlexShrink, 1)
		}},
		{"flex-none", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Grow", s.FlexGrow, 0)
			eq(t, "Shrink", s.FlexShrink, 0)
		}},
		{"flex-[2_2_10%]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Grow", s.FlexGrow, 2)
			eq(t, "Shrink", s.FlexShrink, 2)
			eq(t, "Basis", s.FlexBasis, DimPct(10))
		}},
		{"grow shrink-0", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Grow", s.FlexGrow, 1)
			eq(t, "Shrink", s.FlexShrink, 0)
		}},
		{"grow-[3]", func(t *testing.T, s *ComputedStyle) { eq(t, "Grow", s.FlexGrow, 3) }},
		{"flex-grow flex-shrink-0", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Grow", s.FlexGrow, 1)
			eq(t, "Shrink", s.FlexShrink, 0)
		}},
		{"basis-1/3", func(t *testing.T, s *ComputedStyle) { eq(t, "Basis", s.FlexBasis, DimPct(100*1.0/3)) }},
		{"basis-64", func(t *testing.T, s *ComputedStyle) { eq(t, "Basis", s.FlexBasis, px(256)) }},
		{"basis-auto", func(t *testing.T, s *ComputedStyle) { eq(t, "Basis", s.FlexBasis, Auto) }},
		{"order-first -order-1 order-3", func(t *testing.T, s *ComputedStyle) { eq(t, "Order", s.Order, 3) }},
		{"order-first", func(t *testing.T, s *ComputedStyle) { eq(t, "Order", s.Order, -9999) }},
		{"-order-1", func(t *testing.T, s *ComputedStyle) { eq(t, "Order", s.Order, -1) }},
		{"justify-between items-center", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Justify", s.JustifyContent, AlignSpaceBetween)
			eq(t, "Items", s.AlignItems, AlignCenter)
		}},
		{"justify-end items-start self-end content-evenly", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Justify", s.JustifyContent, AlignFlexEnd)
			eq(t, "Items", s.AlignItems, AlignFlexStart)
			eq(t, "Self", s.AlignSelf, AlignFlexEnd)
			eq(t, "Content", s.AlignContent, AlignSpaceEvenly)
		}},
		{"items-baseline justify-items-center justify-self-start", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Items", s.AlignItems, AlignBaseline)
			eq(t, "JustifyItems", s.JustifyItems, AlignCenter)
			eq(t, "JustifySelf", s.JustifySelf, AlignStart)
		}},
		{"place-items-center place-content-between place-self-end", func(t *testing.T, s *ComputedStyle) {
			eq(t, "AlignItems", s.AlignItems, AlignCenter)
			eq(t, "JustifyItems", s.JustifyItems, AlignCenter)
			eq(t, "AlignContent", s.AlignContent, AlignSpaceBetween)
			eq(t, "JustifyContent", s.JustifyContent, AlignSpaceBetween)
			eq(t, "AlignSelf", s.AlignSelf, AlignEnd)
			eq(t, "JustifySelf", s.JustifySelf, AlignEnd)
		}},
		{"gap-4", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Row", s.RowGap, PxLP(16))
			eq(t, "Col", s.ColumnGap, PxLP(16))
			eq(t, "SpaceX", s.SpaceX, PxLP(0))
		}},
		{"gap-x-2 gap-y-[10px]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Col", s.ColumnGap, PxLP(8))
			eq(t, "Row", s.RowGap, PxLP(10))
		}},
		{"space-y-4", func(t *testing.T, s *ComputedStyle) {
			eq(t, "SpaceY", s.SpaceY, PxLP(16))
			eq(t, "SpaceX", s.SpaceX, PxLP(0))
			eq(t, "Row", s.RowGap, PxLP(0)) // not a gap
			eq(t, "Col", s.ColumnGap, PxLP(0))
		}},
		{"-space-x-2 -space-y-px", func(t *testing.T, s *ComputedStyle) {
			eq(t, "SpaceX", s.SpaceX, PxLP(-8))
			eq(t, "SpaceY", s.SpaceY, PxLP(-1))
		}},
		{"space-x-2 gap-x-1", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Col", s.ColumnGap, PxLP(4))
			eq(t, "SpaceX", s.SpaceX, PxLP(8)) // independent of the gap
		}},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) { tt.check(t, style(t, tt.classes)) })
	}
}

func TestGridClasses(t *testing.T) {
	fr1 := TrackSize{Min: TrackBreadth{Kind: BreadthLength}, Max: TrackBreadth{Kind: BreadthFr, Fr: 1}}
	line := func(k GridLineKind, n int) GridLine { return GridLine{Kind: k, N: n} }
	t.Run("grid-cols-3", func(t *testing.T) {
		s := style(t, "grid grid-cols-3")
		got := s.GridTemplateColumns.Entries
		if len(got) != 1 || !got[0].Repeat || got[0].Count != 3 || got[0].Kind != RepeatCount || got[0].Tracks[0] != fr1 {
			t.Errorf("GridTemplateColumns = %+v, want repeat(3, minmax(0, 1fr))", got)
		}
		if n := len(s.GridTemplateColumns.Expand()); n != 3 {
			t.Errorf("Expand() has %d tracks, want 3", n)
		}
		eq(t, "String", s.GridTemplateColumns.String(), "repeat(3, minmax(0px, 1fr))")
	})
	t.Run("grid-cols-[200px_1fr]", func(t *testing.T) {
		s := style(t, "grid-cols-[200px_1fr]")
		got := s.GridTemplateColumns.Expand()
		want := []TrackSize{
			{TrackBreadth{Kind: BreadthLength, LP: PxLP(200)}, TrackBreadth{Kind: BreadthLength, LP: PxLP(200)}},
			{Max: TrackBreadth{Kind: BreadthFr, Fr: 1}},
		}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("tracks = %+v, want %+v", got, want)
		}
	})
	t.Run("repeat auto-fill minmax", func(t *testing.T) {
		s := style(t, "grid-cols-[repeat(auto-fill,minmax(12rem,1fr))]")
		l := s.GridTemplateColumns.Entries
		if len(l) != 1 || l[0].Kind != RepeatAutoFill || !l[0].Repeat {
			t.Fatalf("list = %+v", l)
		}
		ts := l[0].Tracks[0]
		eq(t, "min", ts.Min, TrackBreadth{Kind: BreadthLength, LP: PxLP(192)})
		eq(t, "max", ts.Max, TrackBreadth{Kind: BreadthFr, Fr: 1})
	})
	t.Run("line names", func(t *testing.T) {
		s := style(t, "grid-cols-[[full-start]_1rem_[main-start_content]_1fr_[main-end]_1rem_[full-end]]")
		l := s.GridTemplateColumns
		eq(t, "String", l.String(), "[full-start] 16px [main-start content] 1fr [main-end] 16px [full-end]")
		if len(l.LineNames) != 4 || l.LineNames[1][1] != "content" {
			t.Errorf("LineNames = %q", l.LineNames)
		}
		s = style(t, "grid-rows-[repeat(2,[row]_auto_[gap])_[last]]")
		eq(t, "rows", s.GridTemplateRows.String(), "repeat(2, [row] auto [gap]) [last]")
		eq(t, "repeat names", len(s.GridTemplateRows.Entries[0].LineNames), 2)
	})
	t.Run("grid-cols-none", func(t *testing.T) {
		if s := style(t, "grid-cols-3 grid-cols-none"); !s.GridTemplateColumns.IsNone() {
			t.Errorf("GridTemplateColumns = %+v, want none", s.GridTemplateColumns)
		}
	})
	t.Run("grid-rows-[auto_1fr_auto]", func(t *testing.T) {
		s := style(t, "grid-rows-[auto_1fr_auto]")
		if n := len(s.GridTemplateRows.Expand()); n != 3 {
			t.Errorf("rows = %d, want 3", n)
		}
	})
	t.Run("placement", func(t *testing.T) {
		s := style(t, "col-span-2 row-start-2 row-end-4")
		eq(t, "ColStart", s.GridColumn.Start, line(GridLineSpan, 2))
		eq(t, "ColEnd", s.GridColumn.End, line(GridLineSpan, 2))
		eq(t, "RowStart", s.GridRow.Start, line(GridLineIndex, 2))
		eq(t, "RowEnd", s.GridRow.End, line(GridLineIndex, 4))
	})
	t.Run("col-N and negative lines", func(t *testing.T) {
		s := style(t, "col-3 -row-start-1 -row-end-2")
		eq(t, "col", s.GridColumn, GridPlacement{Start: line(GridLineIndex, 3)})
		eq(t, "row", s.GridRow, GridPlacement{line(GridLineIndex, -1), line(GridLineIndex, -2)})
		eq(t, "-col-2", style(t, "-col-2").GridColumn, GridPlacement{Start: line(GridLineIndex, -2)})
	})
	t.Run("col-span-full", func(t *testing.T) {
		s := style(t, "col-span-full")
		eq(t, "Start", s.GridColumn.Start, line(GridLineIndex, 1))
		eq(t, "End", s.GridColumn.End, line(GridLineIndex, -1))
	})
	t.Run("col-[1/3] and col-auto", func(t *testing.T) {
		s := style(t, "col-[1/3]")
		eq(t, "Start", s.GridColumn.Start, line(GridLineIndex, 1))
		eq(t, "End", s.GridColumn.End, line(GridLineIndex, 3))
		s = style(t, "col-span-2 col-auto")
		eq(t, "auto", s.GridColumn, GridPlacement{})
	})
	t.Run("named placements", func(t *testing.T) {
		main := GridLine{Kind: GridLineNamed, Name: "main"}
		eq(t, "col-[main]", style(t, "col-[main]").GridColumn, GridPlacement{main, main})
		s := style(t, "col-start-[main] col-end-[2_side] row-[span_header/footer]")
		eq(t, "start", s.GridColumn.Start, main)
		eq(t, "end", s.GridColumn.End, GridLine{Kind: GridLineNamed, N: 2, Name: "side"})
		eq(t, "row", s.GridRow, GridPlacement{GridLine{Kind: GridLineNamedSpan, N: 1, Name: "header"}, GridLine{Kind: GridLineNamed, Name: "footer"}})
		eq(t, "row String", s.GridRow.String(), "span header / footer")
		eq(t, "col-span-[name]", style(t, "col-span-[card]").GridColumn.Start, GridLine{Kind: GridLineNamedSpan, N: 1, Name: "card"})
		eq(t, "col-[main_/_span_2]", style(t, "col-[main_/_span_2]").GridColumn, GridPlacement{main, line(GridLineSpan, 2)})
	})
	t.Run("flow and auto tracks", func(t *testing.T) {
		s := style(t, "grid-flow-col-dense auto-rows-fr auto-cols-[minmax(0,2fr)]")
		eq(t, "Flow", s.GridAutoFlow, GridFlowColumnDense)
		eq(t, "AutoRows", s.GridAutoRows[0], TrackSize{Max: TrackBreadth{Kind: BreadthFr, Fr: 1}})
		eq(t, "AutoCols", s.GridAutoColumns[0], TrackSize{Min: TrackBreadth{Kind: BreadthLength}, Max: TrackBreadth{Kind: BreadthFr, Fr: 2}})
		eq(t, "dense", style(t, "grid-flow-dense").GridAutoFlow, GridFlowRowDense)
		eq(t, "two auto rows", len(style(t, "auto-rows-[10px_auto]").GridAutoRows), 2)
	})
	t.Run("errors", func(t *testing.T) {
		for _, c := range []string{"grid-cols-subgrid", "grid-cols-[repeat(x,1fr)]", "grid-cols-[[a]_[b]_1fr]",
			"grid-cols-[repeat(auto-fill,1fr)]", "grid-cols-[repeat(auto-fill,10px)_auto]",
			"grid-cols-[repeat(auto-fill,10px)_repeat(auto-fit,10px)]", "grid-cols-[[span]_1fr]",
			"auto-cols-[[a]_1fr]", "auto-rows-[repeat(2,1fr)]", "col-[span_0]", "col-start-[0]", "col-[1/2/3]", "row-[span]"} {
			if Compile(c).Err == nil {
				t.Errorf("%q compiled without error", c)
			}
		}
	})
}

func TestSizingClasses(t *testing.T) {
	tests := []struct {
		classes string
		check   func(t *testing.T, s *ComputedStyle)
	}{
		{"w-4 h-8", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, px(16))
			eq(t, "H", s.Height, px(32))
		}},
		{"w-px h-0.5", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, px(1))
			eq(t, "H", s.Height, px(2))
		}},
		{"w-13", func(t *testing.T, s *ComputedStyle) { eq(t, "W", s.Width, px(52)) }},
		{"w-1/2 h-2/3", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, DimPct(50))
			eq(t, "H", s.Height, DimPct(100*2.0/3))
		}},
		{"w-full h-full", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, DimPct(100))
			eq(t, "H", s.Height, DimPct(100))
		}},
		{"w-screen h-screen", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, px(1280))
			eq(t, "H", s.Height, px(800))
		}},
		{"h-dvh w-svw min-h-lvh", func(t *testing.T, s *ComputedStyle) {
			eq(t, "H", s.Height, px(800))
			eq(t, "W", s.Width, px(1280))
			eq(t, "MinH", s.MinHeight, px(800))
		}},
		{"w-min h-max", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, MinContent)
			eq(t, "H", s.Height, MaxContent)
		}},
		{"w-fit w-auto", func(t *testing.T, s *ComputedStyle) { eq(t, "W", s.Width, Auto) }},
		{"w-fit", func(t *testing.T, s *ComputedStyle) { eq(t, "W", s.Width, FitContent) }},
		{"w-[320px] h-[50%]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, px(320))
			eq(t, "H", s.Height, DimPct(50))
		}},
		{"w-[calc(100%-2rem)]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, DimOf(LP{Px: -32, Pct: 100}))
		}},
		{"w-[calc(100%_-_2rem)]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, DimOf(LP{Px: -32, Pct: 100}))
		}},
		{"w-[clamp(10rem,50vw,40rem)]", func(t *testing.T, s *ComputedStyle) { eq(t, "W", s.Width, px(640)) }},
		{"w-[min(20rem,50vw)]", func(t *testing.T, s *ComputedStyle) { eq(t, "W", s.Width, px(320)) }},
		{"w-[max(10rem,5vw)]", func(t *testing.T, s *ComputedStyle) { eq(t, "W", s.Width, px(160)) }},
		{"w-md", func(t *testing.T, s *ComputedStyle) { eq(t, "W", s.Width, px(448)) }},
		{"size-8", func(t *testing.T, s *ComputedStyle) {
			eq(t, "W", s.Width, px(32))
			eq(t, "H", s.Height, px(32))
		}},
		{"min-w-0 min-h-full", func(t *testing.T, s *ComputedStyle) {
			eq(t, "MinW", s.MinWidth, px(0))
			eq(t, "MinH", s.MinHeight, DimPct(100))
		}},
		{"max-w-3xl", func(t *testing.T, s *ComputedStyle) { eq(t, "MaxW", s.MaxWidth, px(768)) }},
		{"max-w-prose", func(t *testing.T, s *ComputedStyle) {
			if !near(s.MaxWidth.Px, 65*16*ChPerEm, 1e-2) { // 65ch
				t.Errorf("MaxW = %v", s.MaxWidth)
			}
		}},
		{"max-w-none", func(t *testing.T, s *ComputedStyle) { eq(t, "MaxW", s.MaxWidth, Auto) }},
		{"max-w-screen-md", func(t *testing.T, s *ComputedStyle) { eq(t, "MaxW", s.MaxWidth, px(768)) }},
		{"max-w-full max-h-96", func(t *testing.T, s *ComputedStyle) {
			eq(t, "MaxW", s.MaxWidth, DimPct(100))
			eq(t, "MaxH", s.MaxHeight, px(384))
		}},
		{"p-4", func(t *testing.T, s *ComputedStyle) { eq(t, "Padding", s.Padding, AllSides(PxLP(16))) }},
		{"px-2.5 py-1", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Padding", s.Padding, Sides[LP]{PxLP(4), PxLP(10), PxLP(4), PxLP(10)})
		}},
		{"pt-px pr-0 pb-[3px] pl-1", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Padding", s.Padding, Sides[LP]{PxLP(1), PxLP(0), PxLP(3), PxLP(4)})
		}},
		{"ps-2 pe-3", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Left", s.Padding.Left, PxLP(8))
			eq(t, "Right", s.Padding.Right, PxLP(12))
		}},
		{"p-[calc(1rem+2px)]", func(t *testing.T, s *ComputedStyle) { eq(t, "Padding", s.Padding.Top, PxLP(18)) }},
		{"m-auto", func(t *testing.T, s *ComputedStyle) { eq(t, "Margin", s.Margin, AllSides(Auto)) }},
		{"mx-auto my-4", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Left", s.Margin.Left, Auto)
			eq(t, "Top", s.Margin.Top, px(16))
		}},
		{"-mt-2 -ml-px mb-[5%]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Top", s.Margin.Top, px(-8))
			eq(t, "Left", s.Margin.Left, px(-1))
			eq(t, "Bottom", s.Margin.Bottom, DimPct(5))
		}},
		{"ms-4 me-auto", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Left", s.Margin.Left, px(16))
			eq(t, "Right", s.Margin.Right, Auto)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) { tt.check(t, style(t, tt.classes)) })
	}
}

func TestColorAndBackgroundClasses(t *testing.T) {
	white15 := gfx.White.MulAlpha(0.15)
	tests := []struct {
		classes string
		check   func(t *testing.T, s *ComputedStyle)
	}{
		{"bg-white", func(t *testing.T, s *ComputedStyle) { eq(t, "Bg", s.BackgroundColor, gfx.White) }},
		{"bg-white/15", func(t *testing.T, s *ComputedStyle) { eq(t, "Bg", s.BackgroundColor, white15) }},
		{"bg-white/[0.15]", func(t *testing.T, s *ComputedStyle) { eq(t, "Bg", s.BackgroundColor, white15) }},
		{"bg-white/[15%]", func(t *testing.T, s *ComputedStyle) { eq(t, "Bg", s.BackgroundColor, white15) }},
		{"bg-[#1e1e2e]", func(t *testing.T, s *ComputedStyle) { eq(t, "Bg", s.BackgroundColor, gfx.MustParseHex("#1e1e2e")) }},
		{"bg-[#1e1e2e]/50", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Bg", s.BackgroundColor, gfx.MustParseHex("#1e1e2e").MulAlpha(0.5))
		}},
		{"bg-[rgb(10_20_30/0.5)]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Bg", s.BackgroundColor, gfx.Color{R: 10.0 / 255, G: 20.0 / 255, B: 30.0 / 255, A: 0.5})
		}},
		{"bg-[color:rgb(255,0,0)]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Bg", s.BackgroundColor, gfx.Color{R: 1, A: 1})
		}},
		{"bg-[oklch(0.637_0.237_25.331)]", func(t *testing.T, s *ComputedStyle) {
			if !colorNear(s.BackgroundColor, gfx.OKLCH(0.637, 0.237, 25.331, 1), 1e-5) {
				t.Errorf("Bg = %v", s.BackgroundColor)
			}
		}},
		{"bg-[hsl(0_100%_50%)]", func(t *testing.T, s *ComputedStyle) {
			if !colorNear(s.BackgroundColor, gfx.Color{R: 1, A: 1}, 1e-5) {
				t.Errorf("Bg = %v", s.BackgroundColor)
			}
		}},
		{"bg-transparent", func(t *testing.T, s *ComputedStyle) { eq(t, "Bg", s.BackgroundColor, gfx.Transparent) }},
		{"text-red-500 bg-current", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Bg", s.BackgroundColor, s.Text.Color)
		}},
		{"text-red-500 bg-current/50", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Bg", s.BackgroundColor, s.Text.Color.MulAlpha(0.5))
		}},
		{"bg-cover bg-center bg-no-repeat bg-[url(/a_b.png)]", func(t *testing.T, s *ComputedStyle) {
			if len(s.Backgrounds) != 1 {
				t.Fatalf("Backgrounds = %+v", s.Backgrounds)
			}
			l := s.Backgrounds[0]
			eq(t, "URL", l.URL, "/a_b.png")
			eq(t, "Size", l.Size.Kind, SizeCover)
			eq(t, "PosX", l.PosX, PctLP(50))
			eq(t, "PosY", l.PosY, PctLP(50))
			eq(t, "Repeat", l.Repeat, RepeatNone)
		}},
		{"bg-[length:200px_100px] bg-[url(x.png)]", func(t *testing.T, s *ComputedStyle) {
			l := s.Backgrounds[0]
			eq(t, "Size", l.Size, BackgroundSize{Kind: SizeLength, W: px(200), H: px(100)})
		}},
		{"bg-left-top bg-[url(x.png)]", func(t *testing.T, s *ComputedStyle) {
			l := s.Backgrounds[0]
			eq(t, "PosX", l.PosX, PctLP(0))
			eq(t, "PosY", l.PosY, PctLP(0))
		}},
		{"bg-clip-text", func(t *testing.T, s *ComputedStyle) { eq(t, "Clip", s.BackgroundClip, BackgroundText) }},
		{"bg-linear-to-br from-indigo-500 to-fuchsia-600", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Kind", g.Kind, GradientLinear)
			eq(t, "Corner", g.Corner, CornerBottomRight)
			eq(t, "Interp", g.Interp, gfx.InterpOklab)
			if len(g.Stops) != 2 {
				t.Fatalf("Stops = %+v", g.Stops)
			}
			th := DefaultTheme()
			eq(t, "from", g.Stops[0].Color, th.Colors["indigo-500"])
			eq(t, "from pos", g.Stops[0].Pos, StopPos{Set: true, LP: PctLP(0)})
			eq(t, "to", g.Stops[1].Color, th.Colors["fuchsia-600"])
			eq(t, "to pos", g.Stops[1].Pos, StopPos{Set: true, LP: PctLP(100)})
		}},
		{"bg-gradient-to-r from-red-500 via-white to-blue-500", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Angle", g.Angle, 90)
			eq(t, "Corner", g.Corner, CornerNone)
			if len(g.Stops) != 3 {
				t.Fatalf("Stops = %+v", g.Stops)
			}
			eq(t, "via", g.Stops[1].Color, gfx.White)
			eq(t, "via pos", g.Stops[1].Pos.Pct, 50)
		}},
		{"bg-linear-45 from-10% from-red-500 via-30% via-white to-90% to-blue-500", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Angle", g.Angle, 45)
			eq(t, "from", g.Stops[0].Pos.Pct, 10)
			eq(t, "via", g.Stops[1].Pos.Pct, 30)
			eq(t, "to", g.Stops[2].Pos.Pct, 90)
		}},
		{"-bg-linear-45 from-black", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Angle", s.Backgrounds[0].Gradient.Angle, -45)
		}},
		{"bg-linear-to-t/srgb from-white", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Interp", g.Interp, gfx.InterpSRGB)
			eq(t, "Angle", g.Angle, 0)
			// to is transparent when only from is given.
			eq(t, "last", g.Stops[len(g.Stops)-1].Color, gfx.Transparent)
		}},
		{"bg-radial from-white to-black", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Kind", g.Kind, GradientRadial)
			eq(t, "CenterX", g.CenterX, PctLP(50))
			eq(t, "Size", g.Size, SizeFarthestCorner)
		}},
		{"bg-radial-[at_25%_75%] from-white to-black", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "CenterX", g.CenterX, PctLP(25))
			eq(t, "CenterY", g.CenterY, PctLP(75))
		}},
		{"bg-radial-[circle_closest-side_at_top] from-white", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Shape", g.Shape, ShapeCircle)
			eq(t, "Size", g.Size, SizeClosestSide)
			eq(t, "CenterY", g.CenterY, PctLP(0))
		}},
		{"bg-conic-90 from-red-500 to-blue-500", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Kind", g.Kind, GradientConic)
			eq(t, "Angle", g.Angle, 90)
		}},
		{"bg-conic-[from_45deg_at_25%_25%] from-red-500", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Angle", g.Angle, 45)
			eq(t, "CenterX", g.CenterX, PctLP(25))
		}},
		{"bg-linear-[25deg,#f00_5%,#ff0_60%,#0f0_90%]", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Angle", g.Angle, 25)
			if len(g.Stops) != 3 || g.Stops[1].Pos.Pct != 60 {
				t.Errorf("Stops = %+v", g.Stops)
			}
		}},
		{"bg-[linear-gradient(to_right,#000,#fff)]", func(t *testing.T, s *ComputedStyle) {
			g := s.Backgrounds[0].Gradient
			eq(t, "Angle", g.Angle, 90)
			eq(t, "stops", len(g.Stops), 2)
		}},
		{"bg-linear-to-r from-red-500 bg-none", func(t *testing.T, s *ComputedStyle) {
			eq(t, "layers", len(s.Backgrounds), 0)
		}},
		{"from-red-500", func(t *testing.T, s *ComputedStyle) {
			eq(t, "layers", len(s.Backgrounds), 0) // stops without a gradient paint nothing
		}},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) { tt.check(t, style(t, tt.classes)) })
	}
}

func TestBorderAndRadiusClasses(t *testing.T) {
	red := DefaultTheme().Colors["red-500"]
	tests := []struct {
		classes string
		check   func(t *testing.T, s *ComputedStyle)
	}{
		{"border", func(t *testing.T, s *ComputedStyle) {
			for i, b := range borderSides(s) {
				eq(t, "width", b.Width, 1)
				eq(t, "style", b.Style, BorderSolid)
				eq(t, "color", b.Color, gfx.Black) // currentColor of the root text
				_ = i
			}
		}},
		{"border-2", func(t *testing.T, s *ComputedStyle) { eq(t, "Top", s.Border.Top.Width, 2) }},
		{"border-t-4 border-x", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Top", s.Border.Top.Width, 4)
			eq(t, "Left", s.Border.Left.Width, 1)
			eq(t, "Right", s.Border.Right.Width, 1)
			eq(t, "Bottom", s.Border.Bottom.Width, 0)
		}},
		{"border-b border-s-2 border-e-8", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Bottom", s.Border.Bottom.Width, 1)
			eq(t, "Left", s.Border.Left.Width, 2)
			eq(t, "Right", s.Border.Right.Width, 8)
		}},
		{"border-[3px]", func(t *testing.T, s *ComputedStyle) { eq(t, "Top", s.Border.Top.Width, 3) }},
		{"border-[#fff]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "color", s.Border.Top.Color, gfx.White)
			eq(t, "width", s.Border.Top.Width, 0)
		}},
		{"border border-red-500", func(t *testing.T, s *ComputedStyle) { eq(t, "color", s.Border.Left.Color, red) }},
		{"border border-t-red-500/50 border-b-transparent", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Top", s.Border.Top.Color, red.MulAlpha(0.5))
			eq(t, "Bottom", s.Border.Bottom.Color, gfx.Transparent)
			eq(t, "Left", s.Border.Left.Color, gfx.Black)
		}},
		{"text-red-500 border border-current/50", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Top", s.Border.Top.Color, red.MulAlpha(0.5))
		}},
		{"border-2 border-dashed", func(t *testing.T, s *ComputedStyle) { eq(t, "Style", s.Border.Top.Style, BorderDashed) }},
		{"border-4 border-none", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Style", s.Border.Top.Style, BorderNone)
			eq(t, "Width", s.Border.Top.Width, 0)
		}},
		{"rounded", func(t *testing.T, s *ComputedStyle) { eq(t, "TL", s.Radius.TopLeft, PxLP(4)) }},
		{"rounded-lg", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Radius", s.Radius, Corners[LP]{PxLP(8), PxLP(8), PxLP(8), PxLP(8)})
		}},
		{"rounded-3xl", func(t *testing.T, s *ComputedStyle) { eq(t, "TL", s.Radius.TopLeft, PxLP(24)) }},
		{"rounded-full", func(t *testing.T, s *ComputedStyle) { eq(t, "TL", s.Radius.TopLeft, PxLP(radiusFull)) }},
		{"rounded-lg rounded-none", func(t *testing.T, s *ComputedStyle) { eq(t, "TL", s.Radius.TopLeft, PxLP(0)) }},
		{"rounded-t-xl", func(t *testing.T, s *ComputedStyle) {
			eq(t, "TL", s.Radius.TopLeft, PxLP(12))
			eq(t, "TR", s.Radius.TopRight, PxLP(12))
			eq(t, "BR", s.Radius.BottomRight, PxLP(0))
		}},
		{"rounded-bl-md rounded-r-sm", func(t *testing.T, s *ComputedStyle) {
			eq(t, "BL", s.Radius.BottomLeft, PxLP(6))
			eq(t, "TR", s.Radius.TopRight, PxLP(4))
			eq(t, "BR", s.Radius.BottomRight, PxLP(4))
		}},
		{"rounded-s-lg rounded-ee-xl", func(t *testing.T, s *ComputedStyle) {
			eq(t, "TL", s.Radius.TopLeft, PxLP(8))
			eq(t, "BL", s.Radius.BottomLeft, PxLP(8))
			eq(t, "BR", s.Radius.BottomRight, PxLP(12))
		}},
		{"rounded-[12px]", func(t *testing.T, s *ComputedStyle) { eq(t, "TL", s.Radius.TopLeft, PxLP(12)) }},
		{"rounded-[50%]", func(t *testing.T, s *ComputedStyle) { eq(t, "TL", s.Radius.TopLeft, PctLP(50)) }},
		{"outline", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Width", s.Outline.Width, 1)
			eq(t, "Style", s.Outline.Style, BorderSolid)
		}},
		{"outline-2 outline-offset-2 outline-red-500", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Width", s.Outline.Width, 2)
			eq(t, "Offset", s.Outline.Offset, 2)
			eq(t, "Color", s.Outline.Color, red)
		}},
		{"-outline-offset-2", func(t *testing.T, s *ComputedStyle) { eq(t, "Offset", s.Outline.Offset, -2) }},
		{"outline-2 outline-none", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Style", s.Outline.Style, BorderNone)
			eq(t, "Width", s.Outline.Width, 0)
		}},
		{"outline-dashed outline-[3px]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Style", s.Outline.Style, BorderDashed)
			eq(t, "Width", s.Outline.Width, 3)
		}},
		{"outline-hidden", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Width", s.Outline.Width, 2)
			eq(t, "Offset", s.Outline.Offset, 2)
			eq(t, "Color", s.Outline.Color, gfx.Transparent)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) { tt.check(t, style(t, tt.classes)) })
	}
}

func TestShadowAndRingClasses(t *testing.T) {
	th := DefaultTheme()
	indigo := th.Colors["indigo-500"]
	t.Run("shadow-sm", func(t *testing.T) {
		s := style(t, "shadow-sm")
		if len(s.BoxShadows) != 2 || s.BoxShadows[0] != th.Shadows["sm"][0] {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
	t.Run("shadow (bare)", func(t *testing.T) {
		s := style(t, "shadow")
		if len(s.BoxShadows) != 2 || s.BoxShadows[0] != th.Shadows["DEFAULT"][0] {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
	t.Run("shadow-none", func(t *testing.T) {
		if s := style(t, "shadow-lg shadow-none"); len(s.BoxShadows) != 0 {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
	t.Run("shadow color order independent", func(t *testing.T) {
		a := style(t, "shadow-lg shadow-indigo-500/50")
		b := style(t, "shadow-indigo-500/50 shadow-lg")
		if len(a.BoxShadows) != 2 || len(b.BoxShadows) != 2 {
			t.Fatalf("shadows = %+v / %+v", a.BoxShadows, b.BoxShadows)
		}
		for i := range a.BoxShadows {
			if a.BoxShadows[i] != b.BoxShadows[i] {
				t.Errorf("layer %d differs: %+v vs %+v", i, a.BoxShadows[i], b.BoxShadows[i])
			}
			eq(t, "color", a.BoxShadows[i].Color, indigo.MulAlpha(0.5))
		}
		eq(t, "geometry", a.BoxShadows[0].OffsetY, 10)
	})
	t.Run("shadow-inner is inset", func(t *testing.T) {
		s := style(t, "shadow-inner")
		if len(s.BoxShadows) != 1 || !s.BoxShadows[0].Inset {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
	t.Run("inset-shadow-sm", func(t *testing.T) {
		s := style(t, "inset-shadow-sm")
		if len(s.BoxShadows) != 1 || !s.BoxShadows[0].Inset || s.BoxShadows[0].OffsetY != 2 {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
	t.Run("arbitrary", func(t *testing.T) {
		s := style(t, "shadow-[0_10px_30px_rgba(0,0,0,0.3),inset_0_1px_0_#fff]")
		want := []Shadow{
			{OffsetY: 10, Blur: 30, Color: gfx.Color{A: 0.3}},
			{OffsetY: 1, Color: gfx.White, Inset: true},
		}
		// Inset layers are listed first, like Tailwind's --tw-inset-shadow.
		if len(s.BoxShadows) != 2 || s.BoxShadows[0] != want[1] || s.BoxShadows[1] != want[0] {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
	t.Run("arbitrary without color is currentColor", func(t *testing.T) {
		s := style(t, "text-red-500 shadow-[0_0_8px]")
		eq(t, "color", s.BoxShadows[0].Color, th.Colors["red-500"])
	})
	t.Run("ring", func(t *testing.T) {
		s := style(t, "ring-2 ring-indigo-500")
		if len(s.BoxShadows) != 1 {
			t.Fatalf("BoxShadows = %+v", s.BoxShadows)
		}
		eq(t, "ring", s.BoxShadows[0], Shadow{Spread: 2, Color: indigo})
	})
	t.Run("bare ring is 1px currentColor", func(t *testing.T) {
		s := style(t, "ring")
		eq(t, "ring", s.BoxShadows[0], Shadow{Spread: 1, Color: gfx.Black})
	})
	t.Run("ring offset", func(t *testing.T) {
		s := style(t, "ring-2 ring-offset-2 ring-indigo-500 ring-offset-slate-100")
		if len(s.BoxShadows) != 2 {
			t.Fatalf("BoxShadows = %+v", s.BoxShadows)
		}
		eq(t, "offset", s.BoxShadows[0], Shadow{Spread: 2, Color: th.Colors["slate-100"]})
		eq(t, "ring", s.BoxShadows[1], Shadow{Spread: 4, Color: indigo})
	})
	t.Run("ring offset defaults to white", func(t *testing.T) {
		s := style(t, "ring-2 ring-offset-2")
		eq(t, "offset", s.BoxShadows[0].Color, gfx.White)
	})
	t.Run("ring-inset and inset-ring", func(t *testing.T) {
		s := style(t, "ring-4 ring-inset inset-ring-2 inset-ring-white/20 shadow-md")
		if len(s.BoxShadows) != 4 {
			t.Fatalf("BoxShadows = %+v", s.BoxShadows)
		}
		eq(t, "inset ring", s.BoxShadows[0], Shadow{Spread: 2, Color: gfx.White.MulAlpha(0.2), Inset: true})
		eq(t, "ring", s.BoxShadows[1], Shadow{Spread: 4, Color: gfx.Black, Inset: true})
		eq(t, "shadow", s.BoxShadows[2].Inset, false)
	})
	t.Run("ring-0 removes the ring", func(t *testing.T) {
		if s := style(t, "ring-2 ring-0"); len(s.BoxShadows) != 0 {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
	t.Run("ring offset alone paints nothing", func(t *testing.T) {
		if s := style(t, "ring-offset-2"); len(s.BoxShadows) != 0 {
			t.Errorf("BoxShadows = %+v", s.BoxShadows)
		}
	})
}

func TestEffectClasses(t *testing.T) {
	th := DefaultTheme()
	t.Run("opacity", func(t *testing.T) {
		eq(t, "50", style(t, "opacity-50").Opacity, 0.5)
		eq(t, "0", style(t, "opacity-0").Opacity, 0)
		eq(t, "arb", style(t, "opacity-[.67]").Opacity, 0.67)
	})
	t.Run("blur", func(t *testing.T) {
		s := style(t, "blur-md")
		eq(t, "Set", s.Filter.Set, FilterBlur)
		eq(t, "Blur", s.Filter.Blur, 12)
		eq(t, "bare", style(t, "blur").Filter.Blur, 8)
		eq(t, "arb", style(t, "blur-[2px]").Filter.Blur, 2)
		eq(t, "none", style(t, "blur-md blur-none").Filter.Blur, 0)
	})
	t.Run("backdrop-blur", func(t *testing.T) {
		s := style(t, "backdrop-blur")
		eq(t, "Blur", s.BackdropFilter.Blur, 8)
		eq(t, "Filter untouched", s.Filter.IsEmpty(), true)
		l := s.BackdropFilter.List()
		if len(l) != 1 || l[0] != (gfx.Blur{StdDev: 8}) {
			t.Errorf("List = %+v", l)
		}
	})
	t.Run("amount filters", func(t *testing.T) {
		s := style(t, "brightness-125 contrast-50 saturate-150 grayscale invert-0 sepia hue-rotate-90 -hue-rotate-15")
		eq(t, "brightness", s.Filter.Brightness, 1.25)
		eq(t, "contrast", s.Filter.Contrast, 0.5)
		eq(t, "saturate", s.Filter.Saturate, 1.5)
		eq(t, "grayscale", s.Filter.Grayscale, 1)
		eq(t, "invert", s.Filter.Invert, 0)
		eq(t, "sepia", s.Filter.Sepia, 1)
		eq(t, "hue", s.Filter.HueRotate, -15)
		want := FilterBrightness | FilterContrast | FilterSaturate | FilterGrayscale | FilterInvert | FilterSepia | FilterHueRotate
		eq(t, "Set", s.Filter.Set, want)
		if n := len(s.Filter.List()); n != 7 {
			t.Errorf("List has %d filters, want 7", n)
		}
	})
	t.Run("filter order is fixed", func(t *testing.T) {
		s := style(t, "sepia saturate-150 blur-sm")
		l := s.Filter.List()
		if len(l) != 3 {
			t.Fatalf("List = %+v", l)
		}
		if _, ok := l[0].(gfx.Blur); !ok {
			t.Errorf("first filter is %T, want Blur", l[0])
		}
		if l[1] != gfx.Filter(gfx.Saturate(1.5)) || l[2] != gfx.Filter(gfx.Sepia(1)) {
			t.Errorf("order wrong: %+v", l)
		}
	})
	t.Run("drop-shadow", func(t *testing.T) {
		s := style(t, "drop-shadow-lg")
		if len(s.Filter.DropShadow) != 1 || s.Filter.DropShadow[0] != th.DropShadows["lg"][0] {
			t.Errorf("DropShadow = %+v", s.Filter.DropShadow)
		}
		l := s.Filter.List()
		ds, ok := l[0].(gfx.DropShadow)
		if !ok || ds.StdDev != 4 || ds.Offset.Y != 4 { // blur radius 4 is the standard deviation
			t.Errorf("filter = %+v", l)
		}
		c := style(t, "drop-shadow-lg drop-shadow-red-500/50")
		eq(t, "color", c.Filter.DropShadow[0].Color, th.Colors["red-500"].MulAlpha(0.5))
		two := style(t, "drop-shadow")
		eq(t, "bare layers", len(two.Filter.DropShadow), 2)
	})
	t.Run("filter-none", func(t *testing.T) {
		eq(t, "empty", style(t, "blur-md grayscale filter-none").Filter.IsEmpty(), true)
	})
	t.Run("mix-blend", func(t *testing.T) {
		eq(t, "multiply", style(t, "mix-blend-multiply").MixBlendMode, gfx.Multiply)
		eq(t, "color-dodge", style(t, "mix-blend-color-dodge").MixBlendMode, gfx.ColorDodge)
		eq(t, "plus-lighter", style(t, "mix-blend-plus-lighter").MixBlendMode, gfx.PlusLighter)
	})
}

func TestTypographyClasses(t *testing.T) {
	red := DefaultTheme().Colors["red-500"]
	tests := []struct {
		classes string
		check   func(t *testing.T, s *ComputedStyle)
	}{
		{"text-xl", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Size", s.Text.FontSize, 20)
			eq(t, "LH", s.Text.LineHeight, text.LineHeightMultiple(1.4))
		}},
		{"text-5xl", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Size", s.Text.FontSize, 48)
			eq(t, "LH", s.Text.LineHeight, text.LineHeightMultiple(1))
		}},
		{"text-sm/6", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Size", s.Text.FontSize, 14)
			eq(t, "LH", s.Text.LineHeight, text.LineHeightPixels(24))
		}},
		{"text-sm/tight", func(t *testing.T, s *ComputedStyle) {
			eq(t, "LH", s.Text.LineHeight, text.LineHeightMultiple(1.25))
		}},
		{"text-sm/[22px]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "LH", s.Text.LineHeight, text.LineHeightPixels(22))
		}},
		{"leading-6 text-sm", func(t *testing.T, s *ComputedStyle) {
			// leading-* wins over the line height of text-* in any order.
			eq(t, "LH", s.Text.LineHeight, text.LineHeightPixels(24))
		}},
		{"text-sm/7 leading-6", func(t *testing.T, s *ComputedStyle) {
			eq(t, "LH", s.Text.LineHeight, text.LineHeightPixels(24))
		}},
		{"leading-6 text-sm/7", func(t *testing.T, s *ComputedStyle) {
			eq(t, "LH", s.Text.LineHeight, text.LineHeightPixels(28))
		}},
		{"text-[14px]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Size", s.Text.FontSize, 14)
			eq(t, "LH keeps the inherited 1.5", s.Text.LineHeight, text.LineHeightMultiple(1.5))
		}},
		{"text-[#fff]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Color", s.Text.Color, gfx.White)
			eq(t, "Size", s.Text.FontSize, 16)
		}},
		{"text-[length:1.25rem]", func(t *testing.T, s *ComputedStyle) { eq(t, "Size", s.Text.FontSize, 20) }},
		{"text-[color:#f00]", func(t *testing.T, s *ComputedStyle) { eq(t, "Color", s.Text.Color, gfx.Color{R: 1, A: 1}) }},
		{"text-[120%]", func(t *testing.T, s *ComputedStyle) { eq(t, "Size", s.Text.FontSize, 19.2) }},
		{"text-red-500", func(t *testing.T, s *ComputedStyle) { eq(t, "Color", s.Text.Color, red) }},
		{"text-red-500/75", func(t *testing.T, s *ComputedStyle) { eq(t, "Color", s.Text.Color, red.MulAlpha(0.75)) }},
		{"text-center", func(t *testing.T, s *ComputedStyle) { eq(t, "Align", s.Text.TextAlign, text.AlignCenter) }},
		{"text-right", func(t *testing.T, s *ComputedStyle) { eq(t, "Align", s.Text.TextAlign, text.AlignRight) }},
		{"text-justify", func(t *testing.T, s *ComputedStyle) { eq(t, "Align", s.Text.TextAlign, text.AlignJustify) }},
		{"text-end", func(t *testing.T, s *ComputedStyle) { eq(t, "Align", s.Text.TextAlign, text.AlignEnd) }},
		{"font-bold", func(t *testing.T, s *ComputedStyle) { eq(t, "Weight", s.Text.FontWeight, 700) }},
		{"font-light font-semibold", func(t *testing.T, s *ComputedStyle) { eq(t, "Weight", s.Text.FontWeight, 600) }},
		{"font-[550]", func(t *testing.T, s *ComputedStyle) { eq(t, "Weight", s.Text.FontWeight, 550) }},
		{"font-mono", func(t *testing.T, s *ComputedStyle) {
			if len(s.Text.FontFamily) != 1 || s.Text.FontFamily[0] != "JetBrains Mono" {
				t.Errorf("Family = %v", s.Text.FontFamily)
			}
		}},
		{"font-serif", func(t *testing.T, s *ComputedStyle) { eq(t, "Family", s.Text.FontFamily[0], "serif") }},
		{"font-['Open_Sans',sans-serif]", func(t *testing.T, s *ComputedStyle) {
			if len(s.Text.FontFamily) != 2 || s.Text.FontFamily[0] != "Open Sans" || s.Text.FontFamily[1] != "sans-serif" {
				t.Errorf("Family = %v", s.Text.FontFamily)
			}
		}},
		{"italic", func(t *testing.T, s *ComputedStyle) { eq(t, "Style", s.Text.FontStyle, text.StyleItalic) }},
		{"italic not-italic", func(t *testing.T, s *ComputedStyle) { eq(t, "Style", s.Text.FontStyle, text.StyleNormal) }},
		{"font-stretch-condensed", func(t *testing.T, s *ComputedStyle) { eq(t, "Stretch", s.Text.FontStretch, 75) }},
		{"tracking-tight text-lg", func(t *testing.T, s *ComputedStyle) {
			// em of the element's own size, whatever the class order.
			if !near(s.Text.LetterSpacing, -0.025*18, 1e-5) {
				t.Errorf("Spacing = %v", s.Text.LetterSpacing)
			}
		}},
		{"text-lg tracking-wider", func(t *testing.T, s *ComputedStyle) {
			if !near(s.Text.LetterSpacing, 0.05*18, 1e-5) {
				t.Errorf("Spacing = %v", s.Text.LetterSpacing)
			}
		}},
		{"tracking-[2px]", func(t *testing.T, s *ComputedStyle) { eq(t, "Spacing", s.Text.LetterSpacing, 2) }},
		{"tracking-[0.1em]", func(t *testing.T, s *ComputedStyle) { eq(t, "Spacing", s.Text.LetterSpacing, 1.6) }},
		{"leading-none", func(t *testing.T, s *ComputedStyle) { eq(t, "LH", s.Text.LineHeight, text.LineHeightMultiple(1)) }},
		{"leading-relaxed", func(t *testing.T, s *ComputedStyle) { eq(t, "LH", s.Text.LineHeight, text.LineHeightMultiple(1.625)) }},
		{"leading-[1.2]", func(t *testing.T, s *ComputedStyle) { eq(t, "LH", s.Text.LineHeight, text.LineHeightMultiple(1.2)) }},
		{"leading-[2em] text-xl", func(t *testing.T, s *ComputedStyle) { eq(t, "LH", s.Text.LineHeight, text.LineHeightPixels(40)) }},
		{"uppercase", func(t *testing.T, s *ComputedStyle) { eq(t, "T", s.Text.TextTransform, text.TransformUppercase) }},
		{"capitalize normal-case", func(t *testing.T, s *ComputedStyle) { eq(t, "T", s.Text.TextTransform, text.TransformNone) }},
		{"underline", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Lines", s.Text.Decoration.Lines, text.DecorationUnderline)
			eq(t, "Color", s.Text.Decoration.Color, gfx.Black)
		}},
		{"line-through no-underline", func(t *testing.T, s *ComputedStyle) { eq(t, "Lines", s.Text.Decoration.Lines, 0) }},
		{"text-red-500 underline decoration-wavy decoration-2 decoration-blue-500 underline-offset-4", func(t *testing.T, s *ComputedStyle) {
			d := s.Text.Decoration
			eq(t, "Style", d.Style, DecorationWavy)
			eq(t, "Thickness", d.Thickness, 2)
			eq(t, "Color", d.Color, DefaultTheme().Colors["blue-500"])
			eq(t, "Offset", d.Offset, 4)
			eq(t, "OffsetSet", d.OffsetSet, true)
		}},
		{"text-red-500 underline", func(t *testing.T, s *ComputedStyle) { eq(t, "Color", s.Text.Decoration.Color, red) }},
		{"truncate", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Overflow", s.OverflowX, OverflowHidden)
			eq(t, "TextOverflow", s.TextOverflow, text.TextOverflowEllipsis)
			eq(t, "WS", s.Text.WhiteSpace, text.WhiteSpaceNowrap)
		}},
		{"text-ellipsis text-clip", func(t *testing.T, s *ComputedStyle) { eq(t, "TO", s.TextOverflow, text.TextOverflowClip) }},
		{"overflow-ellipsis", func(t *testing.T, s *ComputedStyle) { eq(t, "TO", s.TextOverflow, text.TextOverflowEllipsis) }},
		{"line-clamp-3", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Clamp", s.LineClamp, 3)
			eq(t, "Overflow", s.OverflowY, OverflowHidden)
		}},
		{"line-clamp-3 line-clamp-none", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Clamp", s.LineClamp, 0)
			eq(t, "Overflow", s.OverflowY, OverflowVisible)
		}},
		{"whitespace-pre-wrap", func(t *testing.T, s *ComputedStyle) { eq(t, "WS", s.Text.WhiteSpace, text.WhiteSpacePreWrap) }},
		{"whitespace-nowrap", func(t *testing.T, s *ComputedStyle) { eq(t, "WS", s.Text.WhiteSpace, text.WhiteSpaceNowrap) }},
		{"text-nowrap", func(t *testing.T, s *ComputedStyle) { eq(t, "WS", s.Text.WhiteSpace, text.WhiteSpaceNowrap) }},
		{"whitespace-nowrap text-wrap", func(t *testing.T, s *ComputedStyle) { eq(t, "WS", s.Text.WhiteSpace, text.WhiteSpaceNormal) }},
		{"whitespace-pre text-balance", func(t *testing.T, s *ComputedStyle) { eq(t, "WS", s.Text.WhiteSpace, text.WhiteSpacePreWrap) }},
		{"break-words", func(t *testing.T, s *ComputedStyle) { eq(t, "OW", s.Text.OverflowWrap, text.OverflowWrapBreakWord) }},
		{"break-all", func(t *testing.T, s *ComputedStyle) { eq(t, "WB", s.Text.WordBreak, text.WordBreakBreakAll) }},
		{"break-keep", func(t *testing.T, s *ComputedStyle) { eq(t, "WB", s.Text.WordBreak, text.WordBreakKeepAll) }},
		{"wrap-anywhere", func(t *testing.T, s *ComputedStyle) { eq(t, "OW", s.Text.OverflowWrap, text.OverflowWrapAnywhere) }},
		{"break-all break-normal", func(t *testing.T, s *ComputedStyle) { eq(t, "WB", s.Text.WordBreak, text.WordBreakNormal) }},
		{"tabular-nums", func(t *testing.T, s *ComputedStyle) { eq(t, "N", s.Text.Numeric, text.NumericTabular) }},
		{"tabular-nums proportional-nums slashed-zero", func(t *testing.T, s *ComputedStyle) {
			eq(t, "N", s.Text.Numeric, text.NumericProportional|text.NumericSlashedZero)
		}},
		{"ordinal normal-nums", func(t *testing.T, s *ComputedStyle) { eq(t, "N", s.Text.Numeric, 0) }},
		{"text-shadow-sm", func(t *testing.T, s *ComputedStyle) { eq(t, "layers", len(s.Text.Shadow), 3) }},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) { tt.check(t, style(t, tt.classes)) })
	}
}

func TestTransformAndMotionClasses(t *testing.T) {
	tests := []struct {
		classes string
		check   func(t *testing.T, s *ComputedStyle)
	}{
		{"translate-x-4 -translate-y-1/2", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.TranslateX, PxLP(16))
			eq(t, "Y", s.Transform.TranslateY, PctLP(-50))
		}},
		{"translate-4", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.TranslateX, PxLP(16))
			eq(t, "Y", s.Transform.TranslateY, PxLP(16))
		}},
		{"translate-x-full translate-y-[10px]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.TranslateX, PctLP(100))
			eq(t, "Y", s.Transform.TranslateY, PxLP(10))
		}},
		{"scale-95", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.ScaleX, 0.95)
			eq(t, "Y", s.Transform.ScaleY, 0.95)
		}},
		{"scale-x-110 scale-y-[1.02]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.ScaleX, 1.1)
			eq(t, "Y", s.Transform.ScaleY, 1.02)
		}},
		{"-scale-x-100", func(t *testing.T, s *ComputedStyle) { eq(t, "X", s.Transform.ScaleX, -1) }},
		{"rotate-45 -rotate-12 rotate-[0.25turn]", func(t *testing.T, s *ComputedStyle) { eq(t, "R", s.Transform.Rotate, 90) }},
		{"-rotate-12", func(t *testing.T, s *ComputedStyle) { eq(t, "R", s.Transform.Rotate, -12) }},
		{"skew-x-6 skew-y-3", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.SkewX, 6)
			eq(t, "Y", s.Transform.SkewY, 3)
		}},
		{"origin-top-left", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.OriginX, PctLP(0))
			eq(t, "Y", s.Transform.OriginY, PctLP(0))
		}},
		{"origin-[10px_20%]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "X", s.Transform.OriginX, PxLP(10))
			eq(t, "Y", s.Transform.OriginY, PctLP(20))
		}},
		{"rotate-45 transform-none", func(t *testing.T, s *ComputedStyle) { eq(t, "identity", s.Transform.IsIdentity(), true) }},
		{"transition", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Props", s.Transition.Properties, TransitionDefault)
			eq(t, "Duration", s.Transition.Duration, 150*time.Millisecond)
			eq(t, "Timing", s.Transition.Timing, CubicBezier{0.4, 0, 0.2, 1})
		}},
		{"transition-colors duration-300 ease-out delay-75", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Props", s.Transition.Properties, TransitionColors)
			eq(t, "Duration", s.Transition.Duration, 300*time.Millisecond)
			eq(t, "Timing", s.Transition.Timing, CubicBezier{0, 0, 0.2, 1})
			eq(t, "Delay", s.Transition.Delay, 75*time.Millisecond)
		}},
		{"duration-300", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Props", s.Transition.Properties, TransitionNone)
			eq(t, "Duration", s.Transition.Duration, 300*time.Millisecond)
		}},
		{"duration-300 transition-opacity", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Props", s.Transition.Properties, TransitionOpacity)
			eq(t, "Duration", s.Transition.Duration, 300*time.Millisecond) // order independent
		}},
		{"transition-all ease-linear duration-[1.5s]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Props", s.Transition.Properties, TransitionAll)
			eq(t, "Timing", s.Transition.Timing, CubicBezier{0, 0, 1, 1})
			eq(t, "Duration", s.Transition.Duration, 1500*time.Millisecond)
		}},
		{"transition-[width,height] ease-[cubic-bezier(0.1,0.2,0.3,0.4)]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Props", s.Transition.Properties, TransitionWidth|TransitionHeight)
			eq(t, "Timing", s.Transition.Timing, CubicBezier{0.1, 0.2, 0.3, 0.4})
		}},
		{"transition transition-none", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Props", s.Transition.Properties, TransitionNone)
			eq(t, "Duration", s.Transition.Duration, 0)
		}},
		{"animate-spin", func(t *testing.T, s *ComputedStyle) { eq(t, "Animation", s.Animation, "spin 1s linear infinite") }},
		{"animate-spin animate-none", func(t *testing.T, s *ComputedStyle) { eq(t, "Animation", s.Animation, "") }},
		{"animate-[wiggle_1s_ease-in-out_infinite]", func(t *testing.T, s *ComputedStyle) {
			eq(t, "Animation", s.Animation, "wiggle 1s ease-in-out infinite")
		}},
		{"cursor-pointer", func(t *testing.T, s *ComputedStyle) { eq(t, "Cursor", s.Cursor, CursorPointer) }},
		{"cursor-not-allowed", func(t *testing.T, s *ComputedStyle) { eq(t, "Cursor", s.Cursor, CursorNotAllowed) }},
		{"cursor-nwse-resize", func(t *testing.T, s *ComputedStyle) { eq(t, "Cursor", s.Cursor, CursorNWSEResize) }},
		{"pointer-events-none", func(t *testing.T, s *ComputedStyle) { eq(t, "PE", s.PointerEvents, PointerEventsNone) }},
		{"select-none", func(t *testing.T, s *ComputedStyle) { eq(t, "US", s.UserSelect, UserSelectNone) }},
		{"select-all", func(t *testing.T, s *ComputedStyle) { eq(t, "US", s.UserSelect, UserSelectAll) }},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) { tt.check(t, style(t, tt.classes)) })
	}
}
