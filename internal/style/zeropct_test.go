package style

import "testing"

// A zero percentage stays a percentage (ADR-157): Tailwind's flex-1 is
// flex: 1 1 0%, whose basis is content-based where the container's main
// size is indefinite, and h-[0%] in a parent of auto height is auto. An LP
// alone cannot tell 0% from 0px; Dim.ZeroPct and TrackBreadth.ZeroPct do.
func TestZeroPercentStaysAPercentage(t *testing.T) {
	tests := []struct {
		classes string
		get     func(*ComputedStyle) Dim
		pct     bool    // HasPercent
		px      float32 // the length part
		str     string  // String
	}{
		{"flex-1", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 0, "0%"},
		{"flex-2", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 0, "0%"},
		{"basis-[0%] grow", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 0, "0%"},
		{"basis-0", func(s *ComputedStyle) Dim { return s.FlexBasis }, false, 0, "0px"},
		{"basis-[0px]", func(s *ComputedStyle) Dim { return s.FlexBasis }, false, 0, "0px"},
		{"basis-[0]", func(s *ComputedStyle) Dim { return s.FlexBasis }, false, 0, "0px"},
		{"flex-[1_1_0%]", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 0, "0%"},
		{"flex-[1_1_0px]", func(s *ComputedStyle) Dim { return s.FlexBasis }, false, 0, "0px"},
		{"flex-[2]", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 0, "0%"},
		{"basis-[calc(0%_+_10px)]", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 10, "calc(0% + 10px)"},
		{"basis-[calc(50%_-_50%)]", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 0, "0%"},
		{"basis-1/2", func(s *ComputedStyle) Dim { return s.FlexBasis }, true, 0, "50%"},
		{"h-[0%]", func(s *ComputedStyle) Dim { return s.Height }, true, 0, "0%"},
		{"h-0", func(s *ComputedStyle) Dim { return s.Height }, false, 0, "0px"},
		{"w-[0%]", func(s *ComputedStyle) Dim { return s.Width }, true, 0, "0%"},
		{"max-h-[0%]", func(s *ComputedStyle) Dim { return s.MaxHeight }, true, 0, "0%"},
		{"min-h-[0%]", func(s *ComputedStyle) Dim { return s.MinHeight }, true, 0, "0%"},
		{"top-[0%]", func(s *ComputedStyle) Dim { return s.Inset.Top }, true, 0, "0%"},
		{"-top-[0%]", func(s *ComputedStyle) Dim { return s.Inset.Top }, true, 0, "0%"},
		{"top-0", func(s *ComputedStyle) Dim { return s.Inset.Top }, false, 0, "0px"},
		{"m-[0%]", func(s *ComputedStyle) Dim { return s.Margin.Left }, true, 0, "0%"},
		{"w-auto", func(s *ComputedStyle) Dim { return s.Width }, false, 0, "auto"},
	}
	for _, tt := range tests {
		t.Run(tt.classes, func(t *testing.T) {
			d := tt.get(style(t, tt.classes))
			if d.HasPercent() != tt.pct || d.Px != tt.px || d.String() != tt.str {
				t.Errorf("got %+v: HasPercent %v, Px %g, String %q; want %v, %g, %q",
					d, d.HasPercent(), d.Px, d.String(), tt.pct, tt.px, tt.str)
			}
			if tt.pct && d.Pct == 0 && d == DimPx(tt.px) {
				t.Errorf("%+v compares equal to %gpx", d, tt.px)
			}
		})
	}
}

func TestDimLP(t *testing.T) {
	// ZeroPct is set only when the percentage part is zero, so that == still
	// compares values.
	if d := DimLP(LP{Pct: 50}, true); d.ZeroPct || d != DimPct(50) {
		t.Errorf("DimLP(50%%, true) = %+v, want DimPct(50)", d)
	}
	if d := DimLP(LP{Px: 3}, false); d != DimPx(3) || d.HasPercent() {
		t.Errorf("DimLP(3px, false) = %+v, want DimPx(3)", d)
	}
	if d := DimPct(0); !d.ZeroPct || !d.HasPercent() || d == DimPx(0) {
		t.Errorf("DimPct(0) = %+v, want a zero percentage", d)
	}
	if d := DimOf(LP{}); d.HasPercent() {
		t.Errorf("DimOf(LP{}) = %+v has a percentage", d)
	}
	if (Dim{ZeroPct: true}).HasPercent() {
		t.Error("auto has a percentage")
	}
}

func TestZeroPercentGridTracks(t *testing.T) {
	s := style(t, "grid-rows-[0%_auto] grid-cols-[fit-content(0%)_0px_1fr]")
	rows := s.GridTemplateRows.Expand()
	if len(rows) != 2 || !rows[0].Min.ZeroPct || !rows[0].Max.ZeroPct || rows[1].Min.ZeroPct {
		t.Fatalf("rows = %+v, want a 0%% track and an auto one", rows)
	}
	eq(t, "rows", s.GridTemplateRows.String(), "0% auto")
	cols := s.GridTemplateColumns.Expand()
	if len(cols) != 3 || !cols[0].Max.ZeroPct || cols[1].Min.ZeroPct || cols[1].Max.ZeroPct {
		t.Fatalf("cols = %+v, want fit-content(0%%), 0px and 1fr", cols)
	}
	eq(t, "cols", s.GridTemplateColumns.String(), "fit-content(0%) 0px 1fr")
}
